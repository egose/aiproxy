package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

const (
	defaultReadTimeout    = 30 * time.Second
	defaultMaxHeaderBytes = 1 << 20
)

type BuildOptions struct {
	ConfigPath string
	Version    string
}

type App struct {
	Config *config.Runtime
	Server *http.Server
}

func Build(ctx context.Context, opts BuildOptions) (*App, error) {
	logger := observability.NewLogger(nil)
	slog.SetDefault(logger)

	rt, err := config.LoadFile(opts.ConfigPath)
	if err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	observability.LogStartup(logger, rt)

	authenticator := auth.NewAuthenticator(rt.Auth)
	resolver := modelresolver.New(rt)
	adapter := provider.New()
	metrics := observability.NewMetrics()
	metrics.RecordConfig(rt)

	httpClient := &http.Client{Timeout: 5 * time.Minute}

	handler := httpapi.NewHandler(httpapi.Dependencies{
		Resolver:  resolver,
		Adapter:   adapter,
		Auth:      authenticator,
		Client:    httpClient,
		Catalog:   httpapi.BuildModelCatalog(rt),
		Metrics:   metrics,
		Providers: rt.ProviderByName,
		Logger:    logger,
	})

	server := &http.Server{
		Addr:           rt.Listener.Address,
		Handler:        handler,
		ReadTimeout:    defaultReadTimeout,
		MaxHeaderBytes: defaultMaxHeaderBytes,
	}
	if rt.Listener.Timeouts.ReadHeader > 0 {
		server.ReadHeaderTimeout = rt.Listener.Timeouts.ReadHeader
	}
	if rt.Listener.Timeouts.Idle > 0 {
		server.IdleTimeout = rt.Listener.Timeouts.Idle
	}
	if rt.Listener.Timeouts.Write > 0 {
		server.WriteTimeout = rt.Listener.Timeouts.Write
	}

	return &App{Config: rt, Server: server}, nil
}

func (a *App) Run(ctx context.Context) error {
	logger := slog.Default()
	logger.Info("starting server", "address", a.Server.Addr)

	errCh := make(chan error, 1)
	go func() {
		if err := a.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		logger.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := a.Server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("server shutdown: %w", err)
		}
		logger.Info("server stopped")
		return nil
	}
}
