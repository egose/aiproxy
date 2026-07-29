package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
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
	upstreamHeaderTimeout = 60 * time.Second
)

type BuildOptions struct {
	ConfigPath string
	Version    string
}

type App struct {
	mu       sync.RWMutex
	Config   *config.Runtime
	Server   *http.Server
	handler  *httpapi.Handler
	metrics  *observability.Metrics
	logger   *slog.Logger
	adapter  provider.Adapter
	client   *http.Client
	health   *providerhealth.Tracker
	usage    *accounting.Aggregator
	buildOpt BuildOptions
}

func Build(ctx context.Context, opts BuildOptions) (*App, error) {
	logger := observability.NewLogger(nil)
	slog.SetDefault(logger)

	rt, err := loadRuntime(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	observability.LogStartup(logger, rt)

	adapter := provider.New()
	metrics := observability.NewMetrics()
	metrics.SetBuildInfo(opts.Version)
	metrics.RecordConfig(rt)
	usage := accounting.NewAggregator()
	health := providerhealth.New(metrics, rt.ProviderHealth)
	health.SetProviders(rt.ProviderByName)

	httpClient := newHTTPClient()

	handler := httpapi.NewHandler(buildDependencies(rt, logger, adapter, metrics, health, usage, httpClient))

	server := &http.Server{
		Handler: handler,
	}
	applyServerConfig(server, rt.Listener)

	return &App{Config: rt, Server: server, handler: handler, metrics: metrics, logger: logger, adapter: adapter, client: httpClient, health: health, usage: usage, buildOpt: opts}, nil
}

func (a *App) Run(ctx context.Context) error {
	a.logger.Info("starting server", "address", a.Server.Addr)

	reloadCh := make(chan os.Signal, 1)
	signal.Notify(reloadCh, syscall.SIGHUP)
	defer signal.Stop(reloadCh)

	errCh := make(chan error, 1)
	go func() {
		if err := a.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := a.Server.Shutdown(shutdownCtx); err != nil {
				return fmt.Errorf("server shutdown: %w", err)
			}
			a.logger.Info("server stopped")
			return nil
		}
	}
}

func (a *App) Reload() error {
	rt, err := loadRuntime(a.buildOpt.ConfigPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}

	a.mu.RLock()
	current := a.Config
	a.mu.RUnlock()
	if current != nil && (rt.Listener.Address != current.Listener.Address || rt.Listener.Timeouts != current.Listener.Timeouts) {
		return fmt.Errorf("listener changes require restart")
	}

	a.metrics.SetBuildInfo(a.buildOpt.Version)
	a.metrics.RecordConfig(rt)
	a.health = reloadHealthTracker(a.health, a.metrics, current, rt)
	a.health.SetProviders(rt.ProviderByName)
	a.handler.UpdateDependencies(buildDependencies(rt, a.logger, a.adapter, a.metrics, a.health, a.usage, a.client))
	a.mu.Lock()
	a.Config = rt
	a.mu.Unlock()
	observability.LogStartup(a.logger, rt)
	return nil
}

func buildDependencies(rt *config.Runtime, logger *slog.Logger, adapter provider.Adapter, metrics *observability.Metrics, health *providerhealth.Tracker, usage accounting.Recorder, httpClient *http.Client) httpapi.Dependencies {
	return httpapi.Dependencies{
		Resolver:    modelresolver.New(rt),
		Adapter:     adapter,
		Auth:        auth.NewAuthenticator(rt.Auth),
		Authorizer:  auth.NewAuthorizer(rt.Auth),
		Client:      httpClient,
		Catalog:     httpapi.BuildModelCatalog(rt),
		Metrics:     metrics,
		Providers:   rt.ProviderByName,
		Health:      health,
		RateLimiter: ratelimit.New(rt.Auth),
		Accounting:  accounting.NewMulti(metrics, usage),
		Usage:       aOrUsage(usage),
		Logger:      logger,
	}
}

func aOrUsage(usage accounting.Recorder) accounting.Reader {
	if reader, ok := usage.(accounting.Reader); ok {
		return reader
	}
	return nil
}

func loadRuntime(path string) (*config.Runtime, error) {
	return config.LoadFile(path)
}

func newHTTPClient() *http.Client {
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
