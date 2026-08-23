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
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/ratelimit"
)

const (
	defaultReadTimeout    = 30 * time.Second
	defaultMaxHeaderBytes = 1 << 20
)

type BuildOptions struct {
	ConfigPath string
	Version    string
	LogOutput  io.Writer
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
	rateLimiter           ratelimit.Limiter
	usage                 *accounting.Aggregator
	logs                  *observability.LogBuffer
	buildOpt              BuildOptions
	startTime             time.Time
	dashboardTokenMinted  bool
	dashboardTokenWritten bool
	listen                func(network, address string) (net.Listener, error)
	shutdownTimeout       time.Duration
}

func Build(ctx context.Context, opts BuildOptions) (*App, error) {
	rt, err := loadRuntime(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	dashboardTokenMinted, err := ensureDashboardToken(rt, "", false)
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
	rateLimiter := ratelimit.New(rt.Auth)

	httpClients := newUpstreamClientPool()

	startTime := time.Now()
	resolver := modelresolver.New(rt)
	handler := httpapi.NewHandler(buildDependencies(rt, resolver, logger, adapter, metrics, health, rateLimiter, usage, httpClients, logs, startTime, opts.Version))
	server := &http.Server{
		Handler: handler,
	}
	applyServerConfig(server, rt.Listener)

	return &App{Config: rt, Server: server, handler: handler, metrics: metrics, logger: logger, adapter: adapter, resolver: resolver, clients: httpClients, health: health, rateLimiter: rateLimiter, usage: usage, logs: logs, buildOpt: opts, startTime: startTime, dashboardTokenMinted: dashboardTokenMinted}, nil
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

	rt, err := loadRuntime(a.buildOpt.ConfigPath)
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
	dashboardTokenMinted, err := ensureDashboardToken(rt, current.Dashboard.Token, true)
	if err != nil {
		return fmt.Errorf("dashboard token: %w", err)
	}

	nextResolver := modelresolver.NewWithPrevious(rt, currentResolver)
	nextHealth := reloadHealthTracker(a.health, a.metrics, current, rt)
	nextHealth.SetProviders(rt.Catalog)
	nextRateLimiter := a.rateLimiter
	if current == nil || !ratelimit.ConfigEqual(current.Auth, rt.Auth) {
		nextRateLimiter = ratelimit.New(rt.Auth)
	}
	a.metrics.SetBuildInfo(a.buildOpt.Version)
	a.metrics.RecordConfig(rt)
	a.handler.UpdateDependencies(buildDependencies(rt, nextResolver, a.logger, a.adapter, a.metrics, nextHealth, nextRateLimiter, a.usage, a.clients, a.logs, a.startTime, a.buildOpt.Version))
	oldHealth := a.health
	a.mu.Lock()
	a.Config = rt
	a.resolver = nextResolver
	a.health = nextHealth
	a.rateLimiter = nextRateLimiter
	a.dashboardTokenMinted = dashboardTokenMinted
	a.dashboardTokenWritten = a.dashboardTokenWritten || dashboardTokenMinted
	a.mu.Unlock()
	if oldHealth != nil && oldHealth != nextHealth {
		_ = oldHealth.Close()
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
		a.mu.RUnlock()
		if clients != nil {
			clients.CloseIdleConnections()
		}
		if health != nil {
			a.closeErr = health.Close()
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
	if err := dashrpc.PersistToken(a.Config.Dashboard.Token); err != nil {
		return err
	}
	a.dashboardTokenWritten = true
	return nil
}

func buildDependencies(rt *config.Runtime, resolver *modelresolver.Resolver, logger *slog.Logger, adapter provider.Adapter, metrics *observability.Metrics, health *providerhealth.Tracker, rateLimiter ratelimit.Limiter, usage accounting.Recorder, clients *upstreamClientPool, logs *observability.LogBuffer, startTime time.Time, version string) httpapi.Dependencies {
	if resolver == nil {
		resolver = modelresolver.New(rt)
	}
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
		Logger:            logger,
		Dashboard:         dashrpc.NewRuntimeSource(rt.Dashboard, version, rt.Listener.Address, string(rt.Auth.Mode), startTime, rt.Catalog, aOrAggregator(usage), health, logs),
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

func loadRuntime(path string) (*config.Runtime, error) {
	return config.LoadFile(path)
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

func ensureDashboardToken(rt *config.Runtime, existing string, persist bool) (bool, error) {
	if !rt.Dashboard.Enabled {
		return false, nil
	}
	if rt.Dashboard.Token != "" {
		return false, nil
	}
	if existing != "" {
		rt.Dashboard.Token = existing
		return false, nil
	}
	token, err := dashrpc.MintToken()
	if err != nil {
		return false, err
	}
	if persist {
		if err := dashrpc.PersistToken(token); err != nil {
			return false, err
		}
	}
	rt.Dashboard.Token = token
	return true, nil
}
