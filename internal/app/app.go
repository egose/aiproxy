package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/healthcheck"
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/ratelimit"
)

const (
	defaultReadTimeout    = 30 * time.Second
	defaultMaxHeaderBytes = 1 << 20
)

type BuildOptions struct {
	ConfigPath    string
	ConfigFromEnv bool
	Version       string
	LogOutput     io.Writer
}

type App struct {
	mu                    sync.RWMutex
	reloadMu              sync.Mutex
	closeOnce             sync.Once
	closeErr              error
	Config                *config.Runtime
	Server                *http.Server
	handler               *httpapi.Handler
	metrics               *observability.Metrics
	logger                *slog.Logger
	adapter               provider.Adapter
	resolver              *modelresolver.Resolver
	clients               *upstreamClientPool
	health                *providerhealth.Tracker
	healthchecks          *healthcheck.Manager
	rateLimiter           ratelimit.Limiter
	usage                 *accounting.Aggregator
	logs                  *observability.LogBuffer
	payloadLog            *payloadlog.Logger
	buildOpt              BuildOptions
	startTime             time.Time
	dashboardTokenMinted  bool
	dashboardTokenWritten bool
	listen                func(network, address string) (net.Listener, error)
	shutdownTimeout       time.Duration
}

func Build(ctx context.Context, opts BuildOptions) (*App, error) {
	rt, err := loadRuntime(opts)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	dashboardTokenMinted, _, err := ensureDashboardToken(rt, config.Dashboard{}, false)
	if err != nil {
		return nil, fmt.Errorf("dashboard token: %w", err)
	}
	dashboardEnabled := rt.Dashboard.Enabled
	logOutput := opts.LogOutput
	if logOutput == nil {
		logOutput = os.Stderr
	}
	var logs *observability.LogBuffer
	if dashboardEnabled {
		logs = observability.NewLogBuffer(500)
	}
	logger := observability.NewLogger(logOutput, observability.LoggerOptions{
		Level:  observability.ParseLevel(string(rt.Logging.Level)),
		Buffer: logs,
	})

	adapter := provider.New()
	metrics := observability.NewMetrics()
	metrics.SetBuildInfo(opts.Version)
	metrics.RecordConfig(rt)
	usage := accounting.NewAggregator()
	health := providerhealth.New(metrics, rt.ProviderHealth)
	health.SetProviders(rt.Catalog)
	healthchecks := healthcheck.New(health, metrics, opts.Version)
	healthchecks.SetProviders(rt.Catalog)
	rateLimiter := ratelimit.New(rt.Auth)

	payloadLog, err := payloadlog.New(rt.Logging.PayloadLog)
	if err != nil {
		return nil, fmt.Errorf("payload log: %w", err)
	}

	httpClients := newUpstreamClientPool()

	startTime := time.Now()
	resolver := modelresolver.New(rt)
	handler := httpapi.NewHandler(buildDependencies(rt, resolver, logger, adapter, metrics, health, healthchecks, rateLimiter, usage, httpClients, logs, startTime, opts.Version, payloadLog))
	server := &http.Server{
		Handler: handler,
	}
	applyServerConfig(server, rt.Listener)

	return &App{Config: rt, Server: server, handler: handler, metrics: metrics, logger: logger, adapter: adapter, resolver: resolver, clients: httpClients, health: health, healthchecks: healthchecks, rateLimiter: rateLimiter, usage: usage, logs: logs, payloadLog: payloadLog, buildOpt: opts, startTime: startTime, dashboardTokenMinted: dashboardTokenMinted}, nil
}

func (a *App) Run(ctx context.Context) error {
	return a.RunReady(ctx, nil)
}

func (a *App) RunReady(ctx context.Context, ready func() error) (runErr error) {
	cleanupDone := false
	var listener net.Listener
	listenerOwned := false
	cleanup := func() error {
		if cleanupDone {
			return nil
		}
		cleanupDone = true
		var err error
		if listenerOwned && listener != nil {
			if closeErr := listener.Close(); closeErr != nil && !errors.Is(closeErr, net.ErrClosed) {
				err = errors.Join(err, fmt.Errorf("listener close: %w", closeErr))
			}
		}
		if closeErr := a.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("app close: %w", closeErr))
		}
		return err
	}
	defer func() {
		if cleanupErr := cleanup(); cleanupErr != nil {
			if runErr != nil {
				runErr = errors.Join(runErr, cleanupErr)
			} else {
				runErr = cleanupErr
			}
		}
	}()

	a.logger.Info("starting server", "address", a.Server.Addr)
	listen := a.listen
	if listen == nil {
		listen = net.Listen
	}
	listener, err := listen("tcp", a.Server.Addr)
	if err != nil {
		return err
	}
	listenerOwned = true
	if err := a.persistDashboardTokenIfNeeded(); err != nil {
		return fmt.Errorf("dashboard token: %w", err)
	}
	observability.LogStartup(a.logger, a.Config)
	if ready != nil {
		if err := ready(); err != nil {
			return err
		}
	}

	reloadCh := make(chan os.Signal, 1)
	if signals := reloadSignals(); len(signals) > 0 {
		signal.Notify(reloadCh, signals...)
		defer signal.Stop(reloadCh)
	}

	errCh := make(chan error, 1)
	listenerOwned = false
	go func() {
		if err := a.Server.Serve(listener); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	for {
		select {
		case err := <-errCh:
			return err
		case <-reloadCh:
			if err := a.Reload(); err != nil {
				a.logger.Error("config reload failed", "error", err)
			} else {
				a.logger.Info("config reloaded")
			}
		case <-ctx.Done():
			a.logger.Info("shutting down server")
			shutdownTimeout := a.shutdownTimeout
			if shutdownTimeout <= 0 {
				shutdownTimeout = 10 * time.Second
			}
			shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancel()
			if err := a.Server.Shutdown(shutdownCtx); err != nil {
				shutdownErr := fmt.Errorf("server shutdown: %w", err)
				if closeErr := a.Server.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
					shutdownErr = errors.Join(shutdownErr, fmt.Errorf("server close: %w", closeErr))
				}
				return shutdownErr
			}
			if err := cleanup(); err != nil {
				return err
			}
			a.logger.Info("server stopped")
			return nil
		}
	}
}

func (a *App) Reload() error {
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()

	rt, err := loadRuntime(a.buildOpt)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	a.mu.RLock()
	current := a.Config
	currentResolver := a.resolver
	a.mu.RUnlock()
	if current != nil && (rt.Listener.Address != current.Listener.Address || rt.Listener.Timeouts != current.Listener.Timeouts) {
		return fmt.Errorf("listener changes require restart")
	}
	if current != nil && rt.Logging.Level != current.Logging.Level {
		return fmt.Errorf("logging level changes require restart")
	}
	if current != nil && rt.Dashboard.Enabled && !current.Dashboard.Enabled {
		return fmt.Errorf("enabling dashboard requires restart")
	}
	var currentDashboard config.Dashboard
	if current != nil {
		currentDashboard = current.Dashboard
	}
	dashboardTokenMinted, dashboardTokenPublished, err := ensureDashboardToken(rt, currentDashboard, true)
	if err != nil {
		return fmt.Errorf("dashboard token: %w", err)
	}

	nextResolver := modelresolver.NewWithPrevious(rt, currentResolver)
	nextHealth := reloadHealthTracker(a.health, a.metrics, current, rt)
	nextHealth.SetProviders(rt.Catalog)
	a.healthchecks.SetTracker(nextHealth)
	a.healthchecks.SetProviders(rt.Catalog)
	nextPayloadLog, err := reloadPayloadLog(a.payloadLog, current, rt)
	if err != nil {
		return fmt.Errorf("payload log: %w", err)
	}
	nextRateLimiter := a.rateLimiter
	if current == nil || !ratelimit.ConfigEqual(current.Auth, rt.Auth) {
		nextRateLimiter = ratelimit.New(rt.Auth)
	}
	a.metrics.SetBuildInfo(a.buildOpt.Version)
	a.metrics.RecordConfig(rt)
	a.handler.UpdateDependencies(buildDependencies(rt, nextResolver, a.logger, a.adapter, a.metrics, nextHealth, a.healthchecks, nextRateLimiter, a.usage, a.clients, a.logs, a.startTime, a.buildOpt.Version, nextPayloadLog))
	oldHealth := a.health
	oldPayloadLog := a.payloadLog
	a.mu.Lock()
	a.Config = rt
	a.resolver = nextResolver
	a.health = nextHealth
	a.rateLimiter = nextRateLimiter
	a.payloadLog = nextPayloadLog
	a.dashboardTokenMinted = dashboardTokenMinted
	a.dashboardTokenWritten = a.dashboardTokenWritten || dashboardTokenPublished
	a.mu.Unlock()
	if oldHealth != nil && oldHealth != nextHealth {
		_ = oldHealth.Close()
	}
	if oldPayloadLog != nil && oldPayloadLog != nextPayloadLog {
		_ = oldPayloadLog.Close()
	}
	observability.LogStartup(a.logger, rt)
	return nil
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	a.closeOnce.Do(func() {
		a.mu.RLock()
		clients := a.clients
		health := a.health
		healthchecks := a.healthchecks
		payloadLog := a.payloadLog
		a.mu.RUnlock()
		if healthchecks != nil {
			healthchecks.Close()
		}
		if clients != nil {
			clients.CloseIdleConnections()
		}
		if health != nil {
			a.closeErr = health.Close()
		}
		if payloadLog != nil {
			if err := payloadLog.Close(); err != nil && a.closeErr == nil {
				a.closeErr = err
			}
		}
	})
	return a.closeErr
}

func (a *App) persistDashboardTokenIfNeeded() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.dashboardTokenMinted || a.dashboardTokenWritten {
		return nil
	}
	if err := persistDashboardToken(a.Config.Dashboard.Token); err != nil {
		return err
	}
	a.dashboardTokenWritten = true
	return nil
}

func buildDependencies(rt *config.Runtime, resolver *modelresolver.Resolver, logger *slog.Logger, adapter provider.Adapter, metrics *observability.Metrics, health *providerhealth.Tracker, healthchecks *healthcheck.Manager, rateLimiter ratelimit.Limiter, usage accounting.Recorder, clients *upstreamClientPool, logs *observability.LogBuffer, startTime time.Time, version string, payloadLog *payloadlog.Logger) httpapi.Dependencies {
	if resolver == nil {
		resolver = modelresolver.New(rt)
	}
	dashboard := dashrpc.NewRuntimeSource(rt.Dashboard, version, rt.Listener.Address, string(rt.Auth.Mode), startTime, rt.Catalog, aOrAggregator(usage), health, logs)
	dashboard.SetCooldownSource(cooldownSourceFor(resolver))
	dashboard.SetHealthcheckSource(healthcheckSourceFor(healthchecks))
	return httpapi.Dependencies{
		Resolver:          resolver,
		Adapter:           adapter,
		Auth:              auth.NewAuthenticator(rt.Auth),
		Authorizer:        auth.NewAuthorizer(rt.Auth),
		Client:            clients.Client(config.DefaultUpstreamHeaderTimeout),
		ClientForProvider: clients.ClientForProvider,
		Catalog:           rt.Catalog,
		Metrics:           metrics,
		MetricsToken:      rt.Metrics.Token,
		Health:            health,
		RateLimiter:       rateLimiter,
		Accounting:        accounting.NewMulti(metrics, usage),
		Usage:             aOrUsage(usage),
		AccessLog:         rt.Logging.AccessLog,
		HasAccessLog:      true,
		PayloadLog:        payloadLog,
		Logger:            logger,
		Dashboard:         dashboard,
		Version:           version,
	}
}

func healthcheckSourceFor(manager *healthcheck.Manager) func() []dashrpc.HealthcheckStatus {
	return func() []dashrpc.HealthcheckStatus {
		if manager == nil {
			return nil
		}
		statuses := manager.Snapshot()
		if len(statuses) == 0 {
			return nil
		}
		out := make([]dashrpc.HealthcheckStatus, 0, len(statuses))
		for _, st := range statuses {
			out = append(out, dashrpc.HealthcheckStatus{
				Provider:    st.Provider,
				Configured:  st.Configured,
				Checked:     st.Checked,
				Healthy:     st.Healthy,
				StatusCode:  st.StatusCode,
				Message:     st.Message,
				Path:        st.Path,
				LastChecked: st.LastChecked,
			})
		}
		return out
	}
}

func cooldownSourceFor(resolver *modelresolver.Resolver) func() []dashrpc.CooldownInfo {
	return func() []dashrpc.CooldownInfo {
		if resolver == nil {
			return nil
		}
		active := resolver.Cooldowns().Active()
		if len(active) == 0 {
			return nil
		}
		out := make([]dashrpc.CooldownInfo, 0, len(active))
		for _, c := range active {
			out = append(out, dashrpc.CooldownInfo{
				Alias:       c.Alias,
				Provider:    c.Provider,
				Model:       c.Model,
				RemainingMs: c.Remaining.Milliseconds(),
			})
		}
		return out
	}
}

func aOrUsage(usage accounting.Recorder) accounting.Reader {
	if reader, ok := usage.(accounting.Reader); ok {
		return reader
	}
	return nil
}

func aOrAggregator(usage accounting.Recorder) *accounting.Aggregator {
	if aggregator, ok := usage.(*accounting.Aggregator); ok {
		return aggregator
	}
	return nil
}

func loadRuntime(opts BuildOptions) (*config.Runtime, error) {
	if opts.ConfigFromEnv {
		return config.LoadEnv()
	}
	return config.LoadFile(opts.ConfigPath)
}

type upstreamClientPool struct {
	mu      sync.Mutex
	clients map[time.Duration]*http.Client
}

func newUpstreamClientPool() *upstreamClientPool {
	return &upstreamClientPool{clients: make(map[time.Duration]*http.Client)}
}

func (p *upstreamClientPool) ClientForProvider(provider config.Provider) *http.Client {
	return p.Client(provider.UpstreamHeaderTimeout)
}

func (p *upstreamClientPool) Client(timeout time.Duration) *http.Client {
	if timeout <= 0 {
		timeout = config.DefaultUpstreamHeaderTimeout
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if client := p.clients[timeout]; client != nil {
		return client
	}
	client := newHTTPClient(timeout)
	p.clients[timeout] = client
	return client
}

func (p *upstreamClientPool) CloseIdleConnections() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, client := range p.clients {
		if transport, ok := client.Transport.(*http.Transport); ok {
			transport.CloseIdleConnections()
		}
	}
}

func newHTTPClient(upstreamHeaderTimeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(r *http.Request) (*url.URL, error) {
		return http.ProxyFromEnvironment(r)
	}
	transport.ResponseHeaderTimeout = upstreamHeaderTimeout
	return &http.Client{Transport: transport}
}

func reloadPayloadLog(existing *payloadlog.Logger, current, next *config.Runtime) (*payloadlog.Logger, error) {
	var currentCfg config.PayloadLog
	if current != nil {
		currentCfg = current.Logging.PayloadLog
	}
	if next.Logging.PayloadLog == currentCfg {
		return existing, nil
	}
	if !next.Logging.PayloadLog.Enabled {
		return nil, nil
	}
	return payloadlog.New(next.Logging.PayloadLog)
}

func reloadHealthTracker(existing *providerhealth.Tracker, metrics *observability.Metrics, current, next *config.Runtime) *providerhealth.Tracker {
	if existing != nil && current != nil && current.ProviderHealth == next.ProviderHealth {
		return existing
	}
	return providerhealth.New(metrics, next.ProviderHealth)
}

func applyServerConfig(server *http.Server, listener config.Listener) {
	server.Addr = listener.Address
	server.ReadTimeout = defaultReadTimeout
	server.MaxHeaderBytes = defaultMaxHeaderBytes
	server.ReadHeaderTimeout = 0
	server.IdleTimeout = 0
	server.WriteTimeout = 0
	if listener.Timeouts.ReadHeader > 0 {
		server.ReadHeaderTimeout = listener.Timeouts.ReadHeader
	}
	if listener.Timeouts.Idle > 0 {
		server.IdleTimeout = listener.Timeouts.Idle
	}
	if listener.Timeouts.Write > 0 {
		server.WriteTimeout = listener.Timeouts.Write
	}
}

var persistDashboardToken = dashrpc.PersistToken

func ensureDashboardToken(rt *config.Runtime, current config.Dashboard, persist bool) (minted bool, published bool, err error) {
	if !rt.Dashboard.Enabled {
		return false, false, nil
	}
	if rt.Dashboard.Token != "" {
		return false, false, nil
	}
	if current.Token != "" {
		rt.Dashboard.Token = current.Token
		if !current.TokenFromConfig || !persist {
			return false, false, nil
		}
		if err := persistDashboardToken(current.Token); err != nil {
			rt.Dashboard.Token = ""
			return false, false, err
		}
		return false, true, nil
	}
	token, err := dashrpc.MintToken()
	if err != nil {
		return false, false, err
	}
	if persist {
		if err := persistDashboardToken(token); err != nil {
			return false, false, err
		}
		rt.Dashboard.Token = token
		return true, true, nil
	}
	rt.Dashboard.Token = token
	return true, false, nil
}
