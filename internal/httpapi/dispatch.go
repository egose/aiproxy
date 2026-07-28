package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func (h *handler) dispatchDirect(ctx context.Context, op provider.Operation, r modelresolver.ResolveResult, inbound *http.Request) (*provider.Result, error) {
	if h.deps.Metrics != nil {
		h.deps.Metrics.RecordProviderSelection(op, r.Provider.Name+"/"+r.Model.Name, r.Provider.Name, r.Model.Name)
	}
	start := time.Now()
	result, err := h.deps.Adapter.Do(ctx, provider.Request{
		Operation:     op,
		ProviderType:  r.Provider.Type,
		PublicModel:   r.Provider.Name + "/" + r.Model.Name,
		BaseURL:       r.Provider.BaseURL,
		APIKey:        r.Provider.APIKey,
		UpstreamModel: r.Model.UpstreamName,
		Inbound:       inbound,
		Client:        h.deps.Client,
	})
	if h.deps.Metrics != nil {
		status := 0
		if result != nil {
			status = result.StatusCode
		}
		h.deps.Metrics.RecordUpstream(op, r.Provider.Name, status, err, time.Since(start).Seconds())
	}
	return result, err
}

func (h *handler) dispatchAlias(ctx context.Context, op provider.Operation, r modelresolver.ResolveResult, inbound *http.Request, logger *slog.Logger) (*provider.Result, error) {
	body, err := io.ReadAll(inbound.Body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i := 0; i < len(r.Alias.Targets); i++ {
		t := r.Selector.Select()
		prov, ok := h.deps.Resolver.Provider(t.Provider)
		if !ok {
			lastErr = fmt.Errorf("alias target provider %q not found", t.Provider)
			continue
		}
		model, ok := prov.ModelByName[t.Model]
		if !ok {
			lastErr = fmt.Errorf("alias target model %q not found on provider %q", t.Model, t.Provider)
			continue
		}
		logger = logger.With("target", t.Provider+"/"+t.Model)
		if h.deps.Metrics != nil {
			h.deps.Metrics.RecordProviderSelection(op, "alias/"+r.Alias.Name, t.Provider, t.Model)
		}
		req := inbound.Clone(ctx)
		req.Body = io.NopCloser(bytes.NewReader(body))
		start := time.Now()
		result, err := h.deps.Adapter.Do(ctx, provider.Request{
			Operation:     op,
			ProviderType:  prov.Type,
			PublicModel:   "alias/" + r.Alias.Name,
			BaseURL:       prov.BaseURL,
			APIKey:        prov.APIKey,
			UpstreamModel: model.UpstreamName,
			Inbound:       req,
			Client:        h.deps.Client,
		})
		if h.deps.Metrics != nil {
			status := 0
			if result != nil {
				status = result.StatusCode
			}
			h.deps.Metrics.RecordUpstream(op, t.Provider, status, err, time.Since(start).Seconds())
		}
		if err != nil {
			r.Selector.Release(t)
			var invalid provider.ErrInvalidRequest
			if errors.As(err, &invalid) {
				return nil, err
			}
			lastErr = err
			if i+1 < len(r.Alias.Targets) && h.deps.Metrics != nil {
				h.deps.Metrics.RecordAliasRetry(r.Alias.Name, t.Provider, t.Model, "error")
			}
			logger.Warn("alias target failed", "error", err)
			continue
		}
		result.OnClose = func() { r.Selector.Release(t) }
		if result.StatusCode >= 500 && i+1 < len(r.Alias.Targets) {
			closeResult(result)
			lastErr = fmt.Errorf("upstream returned status %d", result.StatusCode)
			if h.deps.Metrics != nil {
				h.deps.Metrics.RecordAliasRetry(r.Alias.Name, t.Provider, t.Model, "upstream_5xx")
			}
			logger.Warn("alias target returned 5xx, retrying", "status", result.StatusCode)
			continue
		}
		return result, nil
	}
	if lastErr == nil {
		lastErr = errors.New("alias has no healthy targets")
	}
	return nil, lastErr
}
