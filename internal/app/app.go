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
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/healthcheck"
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/mongolog"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/ratelimit"
	"github.com/egose/aiproxy/internal/store"
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
	quotaTracker          *httpapi.QuotaTracker
	usage                 *accounting.Aggregator
	logs                  *observability.LogBuffer
	payloadLog            *payloadlog.Logger
	payloadMongo          *mongolog.Logger
	payloadRecorder       payloadlog.Recorder
	guardrails            *guardrails.Scanner
	quarantine            *guardrails.Quarantine
	exceptions            *guardrails.Exceptions
	adminStore            *store.Store
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
	adminStore, err := openAdminStore(ctx, rt)
	if err != nil {
		return nil, err
	}
	var dynKeys []auth.DynamicClient
	if adminStore != nil {
		merged, err := dbmerge.MergeCatalog(ctx, adminStore, rt.Catalog)
		if err != nil {
			_ = adminStore.Close()
			return nil, fmt.Errorf("database catalog: %w", err)
		}
		rt.Catalog = config.NewCatalog(merged.Providers, merged.DisabledProviders, merged.Aliases)
		dynKeys = merged.Keys
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

	payloadLog, payloadMongo, payloadRecorder, err := openPayloadSinks(rt.Logging.PayloadLog)
	if err != nil {
		return nil, err
	}

	scanner, err := guardrails.New(guardrailPolicy(rt))
	if err != nil {
		return nil, fmt.Errorf("ingress guardrails: %w", err)
	}
	quarantine := guardrails.NewQuarantine(quarantinePolicy(rt))
	exceptions, err := loadGuardrailExceptions(rt, scanner)
	if err != nil {
		return nil, err
	}

	httpClients := newUpstreamClientPool()

	startTime := time.Now()
	resolver := modelresolver.New(rt)
	deps := buildDependencies(rt, resolver, logger, adapter, metrics, health, healthchecks, rateLimiter, usage, httpClients, logs, startTime, opts.Version, payloadRecorder, payloadDirOf(payloadLog), scanner, quarantine, exceptions, adminStore, dynKeys)
	app := &App{Config: rt, metrics: metrics, logger: logger, adapter: adapter, resolver: resolver, clients: httpClients, health: health, healthchecks: healthchecks, rateLimiter: rateLimiter, quotaTracker: httpapi.NewQuotaTracker(adminStore), usage: usage, logs: logs, payloadLog: payloadLog, payloadMongo: payloadMongo, payloadRecorder: payloadRecorder, guardrails: scanner, quarantine: quarantine, exceptions: exceptions, adminStore: adminStore, buildOpt: opts, startTime: startTime, dashboardTokenMinted: dashboardTokenMinted}
	deps.Quota = app.quotaTracker
	deps.RequestReload = app.Reload
	app.handler = httpapi.NewHandler(deps)
	server := &http.Server{
		Handler: app.handler,
	}
	applyServerConfig(server, rt.Listener)
	app.Server = server
	return app, nil
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
	var dynKeys []auth.DynamicClient
	if a.adminStore != nil {
		merged, err := dbmerge.MergeCatalog(context.Background(), a.adminStore, rt.Catalog)
		if err != nil {
			return fmt.Errorf("database catalog: %w", err)
		}
		rt.Catalog = config.NewCatalog(merged.Providers, merged.DisabledProviders, merged.Aliases)
		dynKeys = merged.Keys
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
	if current != nil && rt.MultiTenancy.Enabled != current.MultiTenancy.Enabled {
		return fmt.Errorf("toggling multi_tenancy requires restart")
	}
	if current != nil && rt.MultiTenancy.Enabled && rt.Database.URL != current.Database.URL {
		return fmt.Errorf("changing database url requires restart")
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
	nextPayloadLog, nextPayloadMongo, nextPayloadRecorder, err := reloadPayloadSinks(a.payloadLog, a.payloadMongo, current, rt)
	if err != nil {
		return err
	}
	nextGuardrails, err := guardrails.New(guardrailPolicy(rt))
	if err != nil {
		return fmt.Errorf("ingress guardrails: %w", err)
	}
	nextExceptions, err := loadGuardrailExceptions(rt, nextGuardrails)
	if err != nil {
		return err
	}
	nextQuarantine := a.quarantine
	if current == nil || current.IngressGuardrails.Quarantine != rt.IngressGuardrails.Quarantine {
		qpolicy := quarantinePolicy(rt)
		if err := qpolicy.Validate(); err != nil {
			return fmt.Errorf("ingress guardrails quarantine: %w", err)
		}
		nextQuarantine = guardrails.NewQuarantine(qpolicy)
	}
	nextRateLimiter := a.rateLimiter
	if current == nil || !ratelimit.ConfigEqual(current.Auth, rt.Auth) {
		nextRateLimiter = ratelimit.New(rt.Auth)
	}
	a.metrics.SetBuildInfo(a.buildOpt.Version)
	a.metrics.RecordConfig(rt)
	deps := buildDependencies(rt, nextResolver, a.logger, a.adapter, a.metrics, nextHealth, a.healthchecks, nextRateLimiter, a.usage, a.clients, a.logs, a.startTime, a.buildOpt.Version, nextPayloadRecorder, payloadDirOf(nextPayloadLog), nextGuardrails, nextQuarantine, nextExceptions, a.adminStore, dynKeys)
	deps.Quota = a.quotaTracker
	deps.RequestReload = a.Reload
	a.handler.UpdateDependencies(deps)
	oldHealth := a.health
	oldPayloadLog := a.payloadLog
	oldPayloadMongo := a.payloadMongo
	a.mu.Lock()
	a.Config = rt
	a.resolver = nextResolver
	a.health = nextHealth
	a.rateLimiter = nextRateLimiter
	a.payloadLog = nextPayloadLog
	a.payloadMongo = nextPayloadMongo
	a.payloadRecorder = nextPayloadRecorder
	a.guardrails = nextGuardrails
	a.quarantine = nextQuarantine
	a.exceptions = nextExceptions
	a.dashboardTokenMinted = dashboardTokenMinted
	a.dashboardTokenWritten = a.dashboardTokenWritten || dashboardTokenPublished
	a.mu.Unlock()
	if oldHealth != nil && oldHealth != nextHealth {
		_ = oldHealth.Close()
	}
	if oldPayloadLog != nil && oldPayloadLog != nextPayloadLog {
		_ = oldPayloadLog.Close()
	}
	if oldPayloadMongo != nil && oldPayloadMongo != nextPayloadMongo {
		_ = oldPayloadMongo.Close()
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
		payloadMongo := a.payloadMongo
		adminStore := a.adminStore
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
		if payloadMongo != nil {
			if err := payloadMongo.Close(); err != nil && a.closeErr == nil {
				a.closeErr = err
			}
		}
		if adminStore != nil {
			if err := adminStore.Close(); err != nil && a.closeErr == nil {
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

func buildDependencies(rt *config.Runtime, resolver *modelresolver.Resolver, logger *slog.Logger, adapter provider.Adapter, metrics *observability.Metrics, health *providerhealth.Tracker, healthchecks *healthcheck.Manager, rateLimiter ratelimit.Limiter, usage accounting.Recorder, clients *upstreamClientPool, logs *observability.LogBuffer, startTime time.Time, version string, payloadRecorder payloadlog.Recorder, payloadDir string, scanner *guardrails.Scanner, quarantine *guardrails.Quarantine, exceptions *guardrails.Exceptions, adminStore *store.Store, dynKeys []auth.DynamicClient) httpapi.Dependencies {
	if resolver == nil {
		resolver = modelresolver.New(rt)
	}
	dashboard := dashrpc.NewRuntimeSource(rt.Dashboard, version, rt.Listener.Address, string(rt.Auth.Mode), startTime, rt.Catalog, aOrAggregator(usage), health, logs)
	dashboard.SetCooldownSource(cooldownSourceFor(resolver))
	dashboard.SetHealthcheckSource(healthcheckSourceFor(healthchecks))
	if payloadDir != "" {
		dashboard.SetPayloadSource(payloadDir, true)
	}
	if quarantine != nil {
		dashboard.SetBlockSource(quarantineAdapter{quarantine})
	}
	return httpapi.Dependencies{
		Resolver:          resolver,
		Adapter:           adapter,
		Auth:              auth.NewAuthenticatorWithClients(rt.Auth, dynKeys),
		Authorizer:        auth.NewAuthorizerWithClients(rt.Auth, dynKeys),
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
		PayloadLog:        payloadRecorder,
		Logger:            logger,
		Dashboard:         dashboard,
		WebUI:             rt.WebUI,
		MultiTenancy:      rt.MultiTenancy,
		AdminStore:        adminStore,
		AdminAuthConfig:   rt.Auth,
		Version:           version,
		Guardrails:        scanner,
		Quarantine:        quarantine,
		Exceptions:        exceptions,
	}
}

func guardrailPolicy(rt *config.Runtime) guardrails.Policy {
	if rt == nil {
		return guardrails.Policy{}
	}
	g := rt.IngressGuardrails
	return guardrails.Policy{
		Enabled:      g.Enabled,
		Mode:         guardrails.Mode(g.Mode),
		MaxTextBytes: g.MaxTextBytes,
		MaxStrings:   g.MaxStrings,
	}
}

func loadGuardrailExceptions(rt *config.Runtime, scanner *guardrails.Scanner) (*guardrails.Exceptions, error) {
	if rt == nil || !rt.IngressGuardrails.Enabled {
		return nil, nil
	}
	path := config.ResolveGuardrailExceptionsPath(rt.IngressGuardrails)
	placeholder := config.ResolveGuardrailPlaceholder(rt.IngressGuardrails)
	exceptions, err := guardrails.LoadExceptions(path, placeholder)
	if err != nil {
		return nil, fmt.Errorf("ingress guardrails exceptions: %w", err)
	}
	if scanner != nil && placeholder != "" {
		res := scanner.Scan(context.Background(), []string{placeholder})
		if res.Outcome == guardrails.OutcomeFlagged {
			return nil, fmt.Errorf("ingress guardrails: redact_placeholder is itself flagged as a secret")
		}
	}
	return exceptions, nil
}

func quarantinePolicy(rt *config.Runtime) guardrails.QuarantinePolicy {
	if rt == nil || !rt.IngressGuardrails.Enabled {
		return guardrails.QuarantinePolicy{}
	}
	q := rt.IngressGuardrails.Quarantine
	return guardrails.QuarantinePolicy{
		Enabled:    q.Enabled,
		MaxEntries: q.MaxEntries,
		TTL:        q.TTL,
		MaxSnippet: q.MaxSnippet,
	}
}

type quarantineAdapter struct {
	q *guardrails.Quarantine
}

func (a quarantineAdapter) ListBlocks() []dashrpc.BlockSummary {
	summaries := a.q.List()
	out := make([]dashrpc.BlockSummary, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, dashrpc.BlockSummary{
			BlockID:      s.BlockID,
			Timestamp:    s.Timestamp.Format(time.RFC3339Nano),
			Operation:    s.Operation,
			PublicModel:  s.PublicModel,
			RuleIDs:      append([]string(nil), s.RuleIDs...),
			FindingCount: s.FindingCount,
		})
	}
	return out
}

func (a quarantineAdapter) TakeBlock(blockID string) (dashrpc.BlockCapture, bool) {
	capture, ok := a.q.Take(blockID)
	if !ok {
		return dashrpc.BlockCapture{}, false
	}
	out := dashrpc.BlockCapture{
		BlockID:     capture.BlockID,
		Timestamp:   capture.Timestamp.Format(time.RFC3339Nano),
		Operation:   capture.Operation,
		PublicModel: capture.PublicModel,
		RuleIDs:     append([]string(nil), capture.RuleIDs...),
	}
	for _, f := range capture.Findings {
		sha := f.SecretSHA
		if sha == "" && f.Secret != "" {
			sha = guardrails.Fingerprint(f.Secret)
		}
		out.Findings = append(out.Findings, dashrpc.BlockFinding{
			RuleID:      f.RuleID,
			Description: f.Description,
			Secret:      f.Secret,
			SecretSHA:   sha,
			Match:       f.Match,
			Line:        f.Line,
		})
	}
	return out, true
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

func openAdminStore(ctx context.Context, rt *config.Runtime) (*store.Store, error) {
	if rt == nil || !rt.RequiresDB() {
		return nil, nil
	}
	st, err := store.Open(ctx, rt.Database.URL)
	if err != nil {
		return nil, fmt.Errorf("database: %w", err)
	}
	status, err := st.Status(ctx)
	if err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("database: %w", err)
	}
	if len(status.Pending) > 0 {
		_ = st.Close()
		return nil, fmt.Errorf("database: %d pending migrations (run aiproxy migrate up)", len(status.Pending))
	}
	if _, err := store.EnsureAdmin(ctx, st); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("database seed admin: %w", err)
	}
	if err := st.EnsureSystemOrg(ctx); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("database seed system org: %w", err)
	}
	return st, nil
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

func payloadDirOf(disk *payloadlog.Logger) string {
	if disk == nil {
		return ""
	}
	return disk.Dir()
}

func openPayloadSinks(cfg config.PayloadLog) (disk *payloadlog.Logger, mongo *mongolog.Logger, rec payloadlog.Recorder, err error) {
	if !cfg.Enabled {
		return nil, nil, nil, nil
	}
	if cfg.Dir != "" {
		disk, err = payloadlog.New(cfg)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("payload log: %w", err)
		}
	}
	if cfg.Mongo.URI != "" {
		mongo, err = mongolog.New(mongolog.OptionsFromConfig(cfg))
		if err != nil {
			if disk != nil {
				_ = disk.Close()
			}
			return nil, nil, nil, fmt.Errorf("payload mongodb: %w", err)
		}
	}
	var members []payloadlog.Recorder
	if disk != nil {
		members = append(members, disk)
	}
	if mongo != nil {
		members = append(members, mongo)
	}
	return disk, mongo, payloadlog.Combine(members...), nil
}

func reloadPayloadSinks(disk *payloadlog.Logger, mongo *mongolog.Logger, current, next *config.Runtime) (*payloadlog.Logger, *mongolog.Logger, payloadlog.Recorder, error) {
	var currentCfg config.PayloadLog
	if current != nil {
		currentCfg = current.Logging.PayloadLog
	}
	if next.Logging.PayloadLog == currentCfg {
		var members []payloadlog.Recorder
		if disk != nil {
			members = append(members, disk)
		}
		if mongo != nil {
			members = append(members, mongo)
		}
		return disk, mongo, payloadlog.Combine(members...), nil
	}
	if !next.Logging.PayloadLog.Enabled {
		return nil, nil, nil, nil
	}
	return openPayloadSinks(next.Logging.PayloadLog)
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
