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
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/deferred"
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
	"github.com/egose/aiproxy/internal/tlsprofile"
	"github.com/egose/aiproxy/internal/upstreamhttp"
	"github.com/egose/aiproxy/internal/upstreampolicy"
)

const (
	defaultReadTimeout    = 30 * time.Second
	defaultMaxHeaderBytes = 1 << 20
)

type BuildOptions struct {
	ConfigPath       string
	ConfigFromEnv    bool
	Version          string
	LogOutput        io.Writer
	transportFactory func(upstreamhttp.Options) (upstreamhttp.Transport, error)
}

type App struct {
	mu                    sync.RWMutex
	reloadMu              sync.Mutex
	closeOnce             sync.Once
	closeErr              error
	closed                bool
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
	copilotCleanup        *copilotFlowCleanup
	copilotDeviceClient   func() *copilotlogin.Client
	buildOpt              BuildOptions
	startTime             time.Time
	dashboardTokenMinted  bool
	dashboardTokenWritten bool
	listen                func(network, address string) (net.Listener, error)
	shutdownTimeout       time.Duration
	deferredStore         *deferred.Store
	runtimeResources      *httpapi.RuntimeResources
	resourceManager       *runtimeResourceManager
	reconcileCancel       context.CancelFunc
	reconcileDone         chan struct{}
}

type candidateResources struct {
	clients   *upstreamClientPool
	probes    *healthcheck.Prepared
	health    *providerhealth.Tracker
	disk      *payloadlog.Logger
	mongo     *mongolog.Logger
	store     *store.Store
	deferred  *deferred.Store
	resources *httpapi.RuntimeResources
	owners    []*httpapi.ResourceOwner
}

func (c *candidateResources) cleanup(err *error) {
	if *err == nil {
		return
	}
	var cleanupErr error
	if c.probes != nil {
		c.probes.Close()
	}
	if c.clients != nil {
		c.clients.Retire()
	}
	if c.deferred != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cleanupErr = errors.Join(cleanupErr, c.deferred.Close(ctx))
		cancel()
	}
	if c.resources != nil {
		c.resources.Retire()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		cleanupErr = errors.Join(cleanupErr, waitCandidateOwners(ctx, c.owners))
		cancel()
	} else {
		if c.mongo != nil {
			cleanupErr = errors.Join(cleanupErr, c.mongo.Close())
		}
		if c.disk != nil {
			cleanupErr = errors.Join(cleanupErr, c.disk.Close())
		}
		if c.health != nil {
			cleanupErr = errors.Join(cleanupErr, c.health.Close())
		}
		if c.store != nil {
			cleanupErr = errors.Join(cleanupErr, c.store.Close())
		}
	}
	if cleanupErr != nil {
		*err = errors.Join(*err, fmt.Errorf("candidate cleanup: %w", cleanupErr))
	}
}

func Build(ctx context.Context, opts BuildOptions) (_ *App, err error) {
	var candidate candidateResources
	defer candidate.cleanup(&err)

	rt, err := loadRuntime(opts)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	httpClients, err := prepareUpstreamClientPool(rt.Catalog, opts.transportFactory)
	if err != nil {
		return nil, err
	}
	candidate.clients = httpClients
	metrics := observability.NewMetrics()
	healthchecks := healthcheck.New(nil, metrics, opts.Version)
	preparedProbes, err := healthchecks.Prepare(rt.Catalog, opts.transportFactory)
	if err != nil {
		return nil, err
	}
	candidate.probes = preparedProbes
	adminStore, err := openAdminStore(ctx, rt)
	if err != nil {
		return nil, err
	}
	candidate.store = adminStore
	var dynKeys []auth.DynamicClient
	var servingSnapshot dbmerge.Merged
	if adminStore != nil {
		merged, err := dbmerge.MergeCatalog(ctx, adminStore, rt)
		if err != nil {
			return nil, fmt.Errorf("database catalog: %w", err)
		}
		rt.Catalog = config.NewCatalog(merged.Providers, merged.DisabledProviders, merged.Aliases)
		if err := httpClients.prepareProviders(rt.Catalog); err != nil {
			return nil, err
		}
		preparedProbes.Close()
		preparedProbes, err = healthchecks.Prepare(rt.Catalog, opts.transportFactory)
		if err != nil {
			return nil, err
		}
		candidate.probes = preparedProbes
		dynKeys = merged.Keys
		servingSnapshot = merged
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
	metrics.SetBuildInfo(opts.Version)
	metrics.RecordConfig(rt)
	usage := accounting.NewAggregator()
	health := providerhealth.New(metrics, rt.ProviderHealth)
	candidate.health = health
	rateLimiter := ratelimit.New(rt.Auth)

	payloadLog, payloadMongo, payloadRecorder, err := openPayloadSinks(rt.Logging.PayloadLog)
	if err != nil {
		return nil, err
	}
	candidate.disk, candidate.mongo = payloadLog, payloadMongo

	scanner, err := guardrails.New(guardrailPolicy(rt))
	if err != nil {
		return nil, fmt.Errorf("ingress guardrails: %w", err)
	}
	quarantine := guardrails.NewQuarantine(quarantinePolicy(rt))
	exceptions, err := loadGuardrailExceptions(rt, scanner)
	if err != nil {
		return nil, err
	}

	startTime := time.Now()
	resolver := modelresolver.New(rt)
	deps := buildDependencies(rt, resolver, logger, adapter, metrics, health, healthchecks, rateLimiter, usage, httpClients, logs, startTime, opts.Version, payloadRecorder, payloadDirOf(payloadLog), rt.Logging.PayloadLog.RedactGuardrailBodies, scanner, quarantine, exceptions, adminStore, dynKeys)
	app := &App{Config: rt, metrics: metrics, logger: logger, adapter: adapter, resolver: resolver, clients: httpClients, health: health, healthchecks: healthchecks, rateLimiter: rateLimiter, quotaTracker: httpapi.NewQuotaTracker(adminStore), usage: usage, logs: logs, payloadLog: payloadLog, payloadMongo: payloadMongo, payloadRecorder: payloadRecorder, guardrails: scanner, quarantine: quarantine, exceptions: exceptions, adminStore: adminStore, buildOpt: opts, startTime: startTime, dashboardTokenMinted: dashboardTokenMinted}
	if rt.Deferred.Enabled {
		app.deferredStore, err = deferred.New(context.Background(), deferred.OptionsFromConfig(rt.Deferred))
		if err != nil {
			return nil, err
		}
		candidate.deferred = app.deferredStore
	}
	app.resourceManager = newRuntimeResourceManager()
	app.runtimeResources, candidate.owners, err = app.resourceManager.prepare(runtimeResourceList(httpClients, health, payloadLog, payloadMongo, adminStore))
	if err != nil {
		return nil, err
	}
	candidate.resources = app.runtimeResources
	deps.DeferredStore = app.deferredStore
	deps.RuntimeResources = app.runtimeResources
	deps.Quota = app.quotaTracker
	deps.RequestReload = app.Reload
	deps.CopilotDeviceClient = app.copilotDeviceClient
	deps.AppliedServingRevisions = func() store.ServingRevisions { return app.handler.SnapshotDependencies().ServingRevisions }
	deps.ServingRevisions = servingSnapshot.Revisions
	deps.KeysConfirmedAt, deps.CatalogConfirmedAt = servingSnapshot.ConfirmedAt, servingSnapshot.ConfirmedAt
	app.handler = httpapi.NewHandler(deps)
	server := &http.Server{
		Handler: app.handler,
	}
	applyServerConfig(server, rt.Listener)
	app.Server = server
	health.SetProviders(rt.Catalog)
	if err := preparedProbes.Activate(health); err != nil {
		return nil, err
	}
	if adminStore != nil {
		revisions, err := adminStore.ServingRevisions(ctx)
		if err != nil {
			return nil, err
		}
		if revisions != servingSnapshot.Revisions {
			return nil, dbmerge.ErrSnapshotChanged
		}
		if time.Since(servingSnapshot.ConfirmedAt) >= httpapi.ServingFreshnessDeadline {
			return nil, httpapi.ErrServingStale
		}
		app.copilotCleanup = startCopilotFlowCleanup(adminStore, logger)
		app.startServingReconciliation()
	}
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	return a.RunReady(ctx, nil)
}

func (a *App) RunReady(ctx context.Context, ready func() error) (runErr error) {
	cleanupDone := false
	var closeContext context.Context
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
		var closeErr error
		if closeContext == nil {
			closeErr = a.Close()
		} else {
			closeErr = a.CloseContext(closeContext)
		}
		if closeErr != nil {
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
			closeContext = shutdownCtx
			a.BeginShutdown()
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

func (a *App) Reload() (err error) {
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()
	if a.closed {
		return errors.New("app is closed")
	}
	if a.adminStore != nil {
		if err := a.refreshServingKeys(); err != nil {
			return fmt.Errorf("database keys: %w", err)
		}
	}
	var candidate candidateResources
	defer candidate.cleanup(&err)

	rt, err := loadRuntime(a.buildOpt)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	var dynKeys []auth.DynamicClient
	var servingSnapshot dbmerge.Merged
	if a.adminStore != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		merged, err := dbmerge.MergeCatalog(ctx, a.adminStore, rt)
		cancel()
		if err != nil {
			return fmt.Errorf("database catalog: %w", err)
		}
		rt.Catalog = config.NewCatalog(merged.Providers, merged.DisabledProviders, merged.Aliases)
		dynKeys = merged.Keys
		servingSnapshot = merged
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
	nextClients, err := prepareUpstreamClientPool(rt.Catalog, a.buildOpt.transportFactory)
	if err != nil {
		return err
	}
	candidate.clients = nextClients
	preparedProbes, err := a.healthchecks.Prepare(rt.Catalog, a.buildOpt.transportFactory)
	if err != nil {
		return err
	}
	candidate.probes = preparedProbes
	var currentDashboard config.Dashboard
	if current != nil {
		currentDashboard = current.Dashboard
	}
	nextResolver := modelresolver.NewWithPrevious(rt, currentResolver)
	nextHealth := reloadHealthTracker(a.health, a.metrics, current, rt)
	if nextHealth != a.health {
		candidate.health = nextHealth
	}
	nextPayloadLog, nextPayloadMongo, nextPayloadRecorder, err := reloadPayloadSinks(a.payloadLog, a.payloadMongo, current, rt)
	if err != nil {
		return err
	}
	if nextPayloadLog != a.payloadLog {
		candidate.disk = nextPayloadLog
	}
	if nextPayloadMongo != a.payloadMongo {
		candidate.mongo = nextPayloadMongo
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
	nextDeferred := a.deferredStore
	var deferredUpdate *deferred.Update
	if nextDeferred == nil && rt.Deferred.Enabled {
		initial := deferred.OptionsFromConfig(rt.Deferred)
		initial.Enabled = false
		nextDeferred, err = deferred.New(context.Background(), initial)
		if err != nil {
			return err
		}
		candidate.deferred = nextDeferred
	}
	if nextDeferred != nil {
		deferredUpdate, err = nextDeferred.PrepareUpdate(deferred.OptionsFromConfig(rt.Deferred))
		if err != nil {
			return err
		}
		defer deferredUpdate.Cancel()
	}
	nextResources, owners, err := a.resourceManager.prepare(runtimeResourceList(nextClients, nextHealth, nextPayloadLog, nextPayloadMongo, a.adminStore))
	if err != nil {
		return err
	}
	candidate.resources, candidate.owners = nextResources, owners
	var deps httpapi.Dependencies
	a.mu.RLock()
	copilotDeviceClient := a.copilotDeviceClient
	a.mu.RUnlock()
	oldClients := a.clients
	oldResources := a.runtimeResources
	nextQuota := a.handler.SnapshotDependencies().Quota
	a.mu.Lock()
	activate := func() error {
		if a.adminStore != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			revisions, err := a.adminStore.ServingRevisions(ctx)
			cancel()
			if err != nil {
				return err
			}
			if revisions != servingSnapshot.Revisions {
				return dbmerge.ErrSnapshotChanged
			}
			if time.Since(servingSnapshot.ConfirmedAt) >= httpapi.ServingFreshnessDeadline {
				return httpapi.ErrServingStale
			}
		}
		dashboardTokenMinted, dashboardTokenPublished, err := ensureDashboardToken(rt, currentDashboard, true)
		if err != nil {
			return fmt.Errorf("dashboard token: %w", err)
		}
		if err := preparedProbes.Activate(nextHealth); err != nil {
			return err
		}
		nextHealth.SetProviders(rt.Catalog)
		a.metrics.SetBuildInfo(a.buildOpt.Version)
		a.metrics.RecordConfig(rt)
		deps = buildDependencies(rt, nextResolver, a.logger, a.adapter, a.metrics, nextHealth, a.healthchecks, nextRateLimiter, a.usage, nextClients, a.logs, a.startTime, a.buildOpt.Version, nextPayloadRecorder, payloadDirOf(nextPayloadLog), rt.Logging.PayloadLog.RedactGuardrailBodies, nextGuardrails, nextQuarantine, nextExceptions, a.adminStore, dynKeys)
		deps.DeferredStore, deps.RuntimeResources = nextDeferred, nextResources
		deps.Quota = a.quotaTracker
		deps.RequestReload = a.Reload
		deps.CopilotDeviceClient = copilotDeviceClient
		deps.AppliedServingRevisions = func() store.ServingRevisions { return a.handler.SnapshotDependencies().ServingRevisions }
		deps.ServingRevisions = servingSnapshot.Revisions
		deps.KeysConfirmedAt, deps.CatalogConfirmedAt = servingSnapshot.ConfirmedAt, servingSnapshot.ConfirmedAt
		deps.Quota = nextQuota
		a.Config = rt
		a.resolver = nextResolver
		a.clients = nextClients
		a.health = nextHealth
		a.rateLimiter = nextRateLimiter
		a.payloadLog = nextPayloadLog
		a.payloadMongo = nextPayloadMongo
		a.payloadRecorder = nextPayloadRecorder
		a.guardrails = nextGuardrails
		a.quarantine = nextQuarantine
		a.exceptions = nextExceptions
		a.deferredStore, a.runtimeResources = nextDeferred, nextResources
		a.dashboardTokenMinted = dashboardTokenMinted
		a.dashboardTokenWritten = a.dashboardTokenWritten || dashboardTokenPublished
		return nil
	}
	err = a.handler.ActivateDependencies(func() (httpapi.Dependencies, error) {
		if deferredUpdate != nil {
			err := deferredUpdate.Commit(activate)
			return deps, err
		}
		err := activate()
		return deps, err
	})
	a.mu.Unlock()
	if err != nil {
		return err
	}
	if oldClients != nil {
		oldClients.Retire()
	}
	oldResources.Retire()
	observability.LogStartup(a.logger, rt)
	return nil
}

func (a *App) Close() error {
	if a == nil {
		return nil
	}
	timeout := a.shutdownTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return a.CloseContext(ctx)
}

func (a *App) BeginShutdown() {
	if a == nil {
		return
	}
	if a.reconcileCancel != nil {
		a.reconcileCancel()
	}
	a.reloadMu.Lock()
	defer a.reloadMu.Unlock()
	a.closed = true
	a.mu.RLock()
	jobs, handler := a.deferredStore, a.handler
	a.mu.RUnlock()
	if jobs != nil {
		jobs.BeginShutdown()
	}
	if handler != nil {
		handler.StopAdmission()
	}
}

func (a *App) DeferredStore() *deferred.Store {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.deferredStore
}

func (a *App) CloseContext(ctx context.Context) error {
	if a == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if a.reconcileCancel != nil {
		a.reconcileCancel()
	}
	a.closeOnce.Do(func() {
		a.reloadMu.Lock()
		defer a.reloadMu.Unlock()
		a.closed = true
		a.mu.RLock()
		clients := a.clients
		health := a.health
		healthchecks := a.healthchecks
		payloadLog := a.payloadLog
		payloadMongo := a.payloadMongo
		adminStore := a.adminStore
		cleanup := a.copilotCleanup
		jobs, resources, manager := a.deferredStore, a.runtimeResources, a.resourceManager
		a.mu.RUnlock()
		if jobs != nil {
			jobs.BeginShutdown()
		}
		if a.handler != nil {
			a.handler.StopAdmission()
		}
		if cleanup != nil {
			cleanup.stop()
		}
		if healthchecks != nil {
			healthchecks.Close()
		}
		if clients != nil {
			clients.Retire()
		}
		if manager != nil {
			supplemental, _, err := manager.prepare(runtimeResourceList(clients, health, payloadLog, payloadMongo, adminStore))
			a.closeErr = errors.Join(a.closeErr, err)
			manager.beginShutdown()
			resources.Retire()
			if supplemental != nil {
				supplemental.Retire()
			}
			if jobs != nil {
				a.closeErr = errors.Join(a.closeErr, jobs.Close(ctx))
			}
			a.closeErr = errors.Join(a.closeErr, manager.wait(ctx))
			return
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
		if jobs != nil {
			a.closeErr = errors.Join(a.closeErr, jobs.Close(ctx))
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

func buildDependencies(rt *config.Runtime, resolver *modelresolver.Resolver, logger *slog.Logger, adapter provider.Adapter, metrics *observability.Metrics, health *providerhealth.Tracker, healthchecks *healthcheck.Manager, rateLimiter ratelimit.Limiter, usage accounting.Recorder, clients *upstreamClientPool, logs *observability.LogBuffer, startTime time.Time, version string, payloadRecorder payloadlog.Recorder, payloadDir string, redactGuardrailBodies bool, scanner *guardrails.Scanner, quarantine *guardrails.Quarantine, exceptions *guardrails.Exceptions, adminStore *store.Store, dynKeys []auth.DynamicClient) httpapi.Dependencies {
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
		Resolver:              resolver,
		Adapter:               adapter,
		Auth:                  auth.NewAuthenticatorWithClients(rt.Auth, dynKeys),
		Authorizer:            auth.NewAuthorizerWithClients(rt.Auth, dynKeys),
		Client:                clients.Client(config.DefaultUpstreamHeaderTimeout),
		ClientForProvider:     clients.ClientForProvider,
		Catalog:               rt.Catalog,
		Metrics:               metrics,
		MetricsToken:          rt.Metrics.Token,
		Health:                health,
		RateLimiter:           rateLimiter,
		Accounting:            accounting.NewMulti(metrics, usage),
		Usage:                 aOrUsage(usage),
		AccessLog:             rt.Logging.AccessLog,
		HasAccessLog:          true,
		RedactGuardrailBodies: redactGuardrailBodies,
		PayloadLog:            payloadRecorder,
		Logger:                logger,
		Dashboard:             dashboard,
		WebUI:                 rt.WebUI,
		MultiTenancy:          rt.MultiTenancy,
		AdminStore:            adminStore,
		AdminAuthConfig:       rt.Auth,
		Version:               version,
		Guardrails:            scanner,
		Quarantine:            quarantine,
		Exceptions:            exceptions,
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
	clients     map[upstreamClientKey]*http.Client
	factory     func(upstreamhttp.Options) (upstreamhttp.Transport, error)
	unavailable *http.Client
}

type upstreamClientKey struct {
	timeout time.Duration
	profile string
	policy  string
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
	if err := st.EnsureSystemWorkspace(ctx); err != nil {
		_ = st.Close()
		return nil, fmt.Errorf("database seed system workspace: %w", err)
	}
	return st, nil
}

func newUpstreamClientPool(catalog config.Catalog) (*upstreamClientPool, error) {
	return prepareUpstreamClientPool(catalog, nil)
}

func prepareUpstreamClientPool(catalog config.Catalog, factory func(upstreamhttp.Options) (upstreamhttp.Transport, error)) (*upstreamClientPool, error) {
	if factory == nil {
		factory = upstreamhttp.NewTransport
	}
	pool := &upstreamClientPool{
		clients:     make(map[upstreamClientKey]*http.Client),
		factory:     factory,
		unavailable: &http.Client{Transport: unavailableUpstreamTransport{}, CheckRedirect: upstreamhttp.CheckRedirect},
	}
	if err := pool.prepare(config.DefaultUpstreamHeaderTimeout, tlsprofile.Native()); err != nil {
		pool.Retire()
		return nil, err
	}
	if err := pool.prepareProviders(catalog); err != nil {
		pool.Retire()
		return nil, err
	}
	return pool, nil
}

func (p *upstreamClientPool) prepareProviders(catalog config.Catalog) error {
	for _, provider := range catalog.Providers() {
		options, err := upstreamhttp.OptionsForProvider(provider, provider.UpstreamHeaderTimeout)
		if err != nil {
			return fmt.Errorf("upstream transports: %w", err)
		}
		if err := p.prepare(provider.UpstreamHeaderTimeout, options.Profile, options.OutboundPolicy); err != nil {
			return fmt.Errorf("provider %q: %w", provider.Name, err)
		}
	}
	return nil
}

func upstreamKey(timeout time.Duration, profile tlsprofile.Profile, policies ...*upstreampolicy.Policy) upstreamClientKey {
	if timeout <= 0 {
		timeout = config.DefaultUpstreamHeaderTimeout
	}
	var policy *upstreampolicy.Policy
	if len(policies) > 0 {
		policy = policies[0]
	}
	return upstreamClientKey{timeout: timeout, profile: profile.Identity(), policy: policy.Identity()}
}

func (p *upstreamClientPool) prepare(timeout time.Duration, profile tlsprofile.Profile, policies ...*upstreampolicy.Policy) error {
	var policy *upstreampolicy.Policy
	if len(policies) > 0 {
		policy = policies[0]
	}
	key := upstreamKey(timeout, profile, policy)
	if p.clients[key] != nil {
		return nil
	}
	transport, err := p.factory(upstreamhttp.Options{Profile: profile, ResponseHeaderTimeout: key.timeout, OutboundPolicy: policy})
	if err != nil {
		if transport != nil {
			transport.CloseIdleConnections()
		}
		return fmt.Errorf("upstream transports: %w", err)
	}
	if transport == nil {
		return errors.New("upstream transports: factory returned no transport")
	}
	p.clients[key] = &http.Client{Transport: upstreamhttp.NewRetiringTransport(transport), CheckRedirect: upstreamhttp.CheckRedirect}
	return nil
}

func (p *upstreamClientPool) ClientForProvider(provider config.Provider) *http.Client {
	options, err := upstreamhttp.OptionsForProvider(provider, provider.UpstreamHeaderTimeout)
	if err != nil {
		return p.unavailable
	}
	if client := p.clients[upstreamKey(provider.UpstreamHeaderTimeout, options.Profile, options.OutboundPolicy)]; client != nil {
		return client
	}
	return p.unavailable
}

func (p *upstreamClientPool) Client(timeout time.Duration) *http.Client {
	if client := p.clients[upstreamKey(timeout, tlsprofile.Native())]; client != nil {
		return client
	}
	return p.unavailable
}

func (p *upstreamClientPool) CloseIdleConnections() {
	for _, client := range p.clients {
		client.CloseIdleConnections()
	}
}

func (p *upstreamClientPool) Retire() {
	for _, client := range p.clients {
		if transport, ok := client.Transport.(interface{ Retire() }); ok {
			transport.Retire()
		} else {
			client.CloseIdleConnections()
		}
	}
}

type unavailableUpstreamTransport struct{}

func (unavailableUpstreamTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
	return nil, errors.New("upstream transport was not prepared for this provider")
}

func newHTTPClient(upstreamHeaderTimeout time.Duration) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = func(r *http.Request) (*url.URL, error) {
		return http.ProxyFromEnvironment(r)
	}
	transport.ResponseHeaderTimeout = upstreamHeaderTimeout
	return &http.Client{Transport: transport, CheckRedirect: upstreamhttp.CheckRedirect}
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
