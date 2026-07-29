package httpapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func (h *Handler) dispatchDirect(deps Dependencies, ctx context.Context, op provider.Operation, r modelresolver.ResolveResult, inbound *http.Request) (*provider.Result, error) {
	if deps.Metrics != nil {
		deps.Metrics.RecordProviderSelection(op, r.Provider.Name+"/"+r.Model.Name, r.Provider.Name, r.Model.Name)
	}
	start := time.Now()
	result, err := deps.Adapter.Do(ctx, provider.Request{
		Operation:     op,
		ProviderType:  r.Provider.Type,
		PublicModel:   r.Provider.Name + "/" + r.Model.Name,
		BaseURL:       r.Provider.BaseURL,
		APIKey:        r.Provider.APIKey,
		UpstreamModel: r.Model.UpstreamName,
		Inbound:       inbound,
		Client:        deps.Client,
	})
	if deps.Metrics != nil {
		status := 0
		if result != nil {
			status = result.StatusCode
		}
		deps.Metrics.RecordUpstream(op, r.Provider.Name, status, err, time.Since(start).Seconds())
	}
	h.instrumentUpstreamResponseSize(deps, op, r.Provider.Name, result, err)
	return result, err
}

func (h *Handler) dispatchAlias(deps Dependencies, ctx context.Context, op provider.Operation, r modelresolver.ResolveResult, inbound *http.Request, logger *slog.Logger) (*provider.Result, error) {
	body, err := io.ReadAll(inbound.Body)
	if err != nil {
		return nil, err
	}
	var lastErr error
	for i := 0; i < len(r.Alias.Targets); i++ {
		t := r.Selector.Select()
		prov, ok := deps.Resolver.Provider(t.Provider)
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
		if deps.Metrics != nil {
			deps.Metrics.RecordProviderSelection(op, "alias/"+r.Alias.Name, t.Provider, t.Model)
			deps.Metrics.AddAliasInFlight(r.Alias.Name, t.Provider, t.Model, 1)
		}
		var releaseOnce sync.Once
		releaseTarget := func() {
			releaseOnce.Do(func() {
				r.Selector.Release(t)
				if deps.Metrics != nil {
					deps.Metrics.AddAliasInFlight(r.Alias.Name, t.Provider, t.Model, -1)
				}
			})
		}
		req := inbound.Clone(ctx)
		req.Body = io.NopCloser(bytes.NewReader(body))
		start := time.Now()
		result, err := deps.Adapter.Do(ctx, provider.Request{
			Operation:     op,
			ProviderType:  prov.Type,
			PublicModel:   "alias/" + r.Alias.Name,
			BaseURL:       prov.BaseURL,
			APIKey:        prov.APIKey,
			UpstreamModel: model.UpstreamName,
			Inbound:       req,
			Client:        deps.Client,
		})
		if deps.Metrics != nil {
			status := 0
			if result != nil {
				status = result.StatusCode
			}
			deps.Metrics.RecordUpstream(op, t.Provider, status, err, time.Since(start).Seconds())
		}
		h.instrumentUpstreamResponseSize(deps, op, t.Provider, result, err)
		if err != nil {
			releaseTarget()
			var invalid provider.ErrInvalidRequest
			if errors.As(err, &invalid) {
				return nil, err
			}
			lastErr = err
			if i+1 < len(r.Alias.Targets) && deps.Metrics != nil {
				deps.Metrics.RecordAliasRetry(r.Alias.Name, t.Provider, t.Model, "error")
			}
			logger.Warn("alias target failed", "error", err)
			continue
		}
		existingClose := result.OnClose
		result.OnClose = func() {
			if existingClose != nil {
				existingClose()
			}
			releaseTarget()
		}
		if result.StatusCode >= 500 && i+1 < len(r.Alias.Targets) {
			closeResult(result)
			lastErr = fmt.Errorf("upstream returned status %d", result.StatusCode)
			if deps.Metrics != nil {
				deps.Metrics.RecordAliasRetry(r.Alias.Name, t.Provider, t.Model, "upstream_5xx")
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

func (h *Handler) instrumentUpstreamResponseSize(deps Dependencies, op provider.Operation, providerName string, result *provider.Result, err error) {
	if deps.Metrics == nil || result == nil {
		return
	}
	if result.Streaming && result.StreamBody != nil {
		counter := &countingReadCloser{ReadCloser: result.StreamBody}
		existingClose := result.OnClose
		result.StreamBody = counter
		result.OnClose = func() {
			if existingClose != nil {
				existingClose()
			}
			deps.Metrics.RecordUpstreamResponseSize(op, providerName, result.StatusCode, nil, counter.BytesRead())
		}
		return
	}
	deps.Metrics.RecordUpstreamResponseSize(op, providerName, result.StatusCode, err, len(result.Body))
}

type countingReadCloser struct {
	io.ReadCloser
	bytesRead int64
}

func (r *countingReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		atomic.AddInt64(&r.bytesRead, int64(n))
	}
	return n, err
}

func (r *countingReadCloser) BytesRead() int {
	return int(atomic.LoadInt64(&r.bytesRead))
}
