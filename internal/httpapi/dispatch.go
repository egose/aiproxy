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

	"github.com/egose/aiproxy/internal/alias"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func (h *Handler) dispatchDirect(deps Dependencies, ctx context.Context, op provider.Operation, r modelresolver.ResolveResult, inbound *http.Request, body []byte, logger *slog.Logger) (*provider.Result, error) {
	if deps.AccessLog && logger != nil {
		logger.Info("upstream request started",
			"provider", r.Provider.Name,
			"provider_type", r.Provider.Type,
			"upstream_model", r.Model.UpstreamName,
		)
	}
	if deps.Metrics != nil {
		deps.Metrics.RecordProviderSelection(op, r.Provider.Name+"/"+r.Model.Name, r.Provider.Name, r.Model.Name)
	}
	if inbound != nil {
		inbound = cloneRequestWithBody(ctx, inbound, body)
	}
	start := time.Now()
	result, err := deps.Adapter.Do(ctx, provider.Request{
		Operation:     op,
		ProviderType:  r.Provider.Type,
		PublicModel:   r.Provider.Name + "/" + r.Model.Name,
		BaseURL:       r.Provider.BaseURL,
		APIKey:        r.Provider.APIKey,
		CopilotToken:  r.Provider.CopilotToken,
		UpstreamModel: r.Model.UpstreamName,
		ModelProtocol: r.Model.Protocol,
		UserAgent:     r.Provider.UserAgent,
		Version:       deps.Version,
		Body:          body,
		Inbound:       inbound,
		Client:        clientForProvider(deps, r.Provider),
	})
	if deps.Metrics != nil && (result == nil || !result.Streaming) {
		status := 0
		if result != nil {
			status = result.StatusCode
		}
		deps.Metrics.RecordUpstream(op, r.Provider.Name, status, err, time.Since(start).Seconds())
	}
	if deps.AccessLog && logger != nil && (result == nil || !result.Streaming) {
		attrs := []any{
			"provider", r.Provider.Name,
			"provider_type", r.Provider.Type,
			"upstream_model", r.Model.UpstreamName,
			"duration_ms", time.Since(start).Milliseconds(),
		}
		if result != nil {
			attrs = append(attrs, "status", result.StatusCode)
		}
		if err != nil {
			attrs = append(attrs, "error", err)
		}
		logger.Info("upstream request finished", attrs...)
	}
	if result != nil && result.Streaming && err == nil {
		h.attachStreamFinalizers(deps, ctx, op, r.Provider.Name, result, start, logger, []any{
			"provider", r.Provider.Name,
			"provider_type", r.Provider.Type,
			"upstream_model", r.Model.UpstreamName,
		})
		h.instrumentUpstreamResponseSize(deps, op, r.Provider.Name, result, err)
		return result, nil
	}
	h.recordProviderHealth(deps, ctx, r.Provider.Name, result, err, false)
	h.instrumentUpstreamResponseSize(deps, op, r.Provider.Name, result, err)
	return result, err
}

type aliasPoolTarget struct {
	target      alias.Target
	provider    config.Provider
	model       config.Model
	fingerprint modelresolver.CooldownFingerprint
	ok          bool
}

func resolveAliasPool(deps Dependencies, aliasName string, targets []config.AliasTarget) []aliasPoolTarget {
	pool := make([]aliasPoolTarget, 0, len(targets))
	for _, target := range targets {
		entry := aliasPoolTarget{target: alias.Target{Provider: target.Provider, Model: target.Model}}
		if deps.Resolver != nil {
			prov, model, ok := deps.Resolver.Model(target.Provider, target.Model)
			if ok {
				entry.provider = prov
				entry.model = model
				entry.fingerprint = modelresolver.CooldownFingerprintFor(aliasName, prov, model)
				entry.ok = true
			}
		}
		pool = append(pool, entry)
	}
	return pool
}

func poolTargetFor(pool []aliasPoolTarget, t alias.Target) *aliasPoolTarget {
	for i := range pool {
		if pool[i].target == t {
			return &pool[i]
		}
	}
	return nil
}

func allCoolingRemaining(cooldowns *modelresolver.CooldownStore, pool []aliasPoolTarget) (time.Duration, bool) {
	if len(pool) == 0 {
		return 0, false
	}
	var earliest time.Duration
	for i, entry := range pool {
		if !entry.ok {
			return 0, false
		}
		remaining, ok := cooldowns.Remaining(entry.fingerprint)
		if !ok {
			return 0, false
		}
		if i == 0 || remaining < earliest {
			earliest = remaining
		}
	}
	return earliest, true
}

func coolingExclusions(cooldowns *modelresolver.CooldownStore, pool []aliasPoolTarget, tried map[alias.Target]bool) map[alias.Target]bool {
	exclude := make(map[alias.Target]bool, len(pool))
	for target := range tried {
		exclude[target] = true
	}
	for _, entry := range pool {
		if !entry.ok {
			continue
		}
		if _, cooling := cooldowns.Remaining(entry.fingerprint); cooling {
			exclude[entry.target] = true
		}
	}
	return exclude
}

func hasUntriedTarget(pool []aliasPoolTarget, tried map[alias.Target]bool) bool {
	for _, entry := range pool {
		if !tried[entry.target] {
			return true
		}
	}
	return false
}

func observeCooldownResult(cooldowns *modelresolver.CooldownStore, ctx context.Context, fp modelresolver.CooldownFingerprint, result *provider.Result) {
	if cooldowns == nil || result == nil || !result.HasRetryDelay {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	cooldowns.Observe(fp, result.RetryDelay)
}

func observeCooldownError(cooldowns *modelresolver.CooldownStore, ctx context.Context, fp modelresolver.CooldownFingerprint, err error) {
	if cooldowns == nil || err == nil {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	var invalid provider.ErrInvalidRequest
	if errors.As(err, &invalid) {
		return
	}
	var unsupported provider.ErrUnsupportedOperation
	if errors.As(err, &unsupported) {
		return
	}
	if delay, ok := provider.CooldownDelayFromError(err); ok {
		cooldowns.Observe(fp, delay)
	}
}

func (h *Handler) dispatchAlias(deps Dependencies, ctx context.Context, op provider.Operation, r modelresolver.ResolveResult, inbound *http.Request, body []byte, logger *slog.Logger) (*provider.Result, error) {
	var lastErr error
	tried := make(map[alias.Target]bool, len(r.Alias.Targets))
	retryCodes := make(map[int]bool, len(r.Alias.RetryStatusCodes))
	for _, code := range r.Alias.RetryStatusCodes {
		retryCodes[code] = true
	}
	pool := resolveAliasPool(deps, r.Alias.Name, r.Alias.Targets)
	var cooldowns *modelresolver.CooldownStore
	if deps.Resolver != nil {
		cooldowns = deps.Resolver.Cooldowns()
	}
	if remaining, ok := allCoolingRemaining(cooldowns, pool); ok {
		return provider.SyntheticCooldownResult(remaining), nil
	}
	for {
		t, releaseLease := r.Selector.Acquire(coolingExclusions(cooldowns, pool, tried))
		if t.Provider == "" && t.Model == "" {
			break
		}
		tried[t] = true
		entry := poolTargetFor(pool, t)
		var prov config.Provider
		var model config.Model
		var fp modelresolver.CooldownFingerprint
		var ok bool
		if entry != nil && entry.ok {
			prov, model, fp, ok = entry.provider, entry.model, entry.fingerprint, true
		} else if deps.Resolver != nil {
			prov, model, ok = deps.Resolver.Model(t.Provider, t.Model)
			if ok {
				fp = modelresolver.CooldownFingerprintFor(r.Alias.Name, prov, model)
			}
		}
		if !ok {
			if deps.Resolver == nil {
				lastErr = fmt.Errorf("alias target provider %q not found", t.Provider)
			} else if _, providerOK := deps.Resolver.Provider(t.Provider); !providerOK {
				lastErr = fmt.Errorf("alias target provider %q not found", t.Provider)
			} else {
				lastErr = fmt.Errorf("alias target model %q not found on provider %q", t.Model, t.Provider)
			}
			releaseLease()
			continue
		}
		if _, cooling := cooldowns.Remaining(fp); cooling {
			releaseLease()
			continue
		}
		if deps.Health != nil && !deps.Health.IsHealthyContext(ctx, t.Provider) {
			releaseLease()
			lastErr = fmt.Errorf("alias has no healthy targets")
			continue
		}
		targetLogger := logger.With("target", t.Provider+"/"+t.Model)
		if deps.AccessLog {
			targetLogger.Info("upstream request started",
				"alias", r.Alias.Name,
				"provider", t.Provider,
				"provider_type", prov.Type,
				"upstream_model", model.UpstreamName,
			)
		}
		if deps.Metrics != nil {
			deps.Metrics.RecordProviderSelection(op, "alias/"+r.Alias.Name, t.Provider, t.Model)
			deps.Metrics.AddAliasInFlight(r.Alias.Name, t.Provider, t.Model, 1)
		}
		var releaseOnce sync.Once
		releaseTarget := func() {
			releaseOnce.Do(func() {
				releaseLease()
				if deps.Metrics != nil {
					deps.Metrics.AddAliasInFlight(r.Alias.Name, t.Provider, t.Model, -1)
				}
			})
		}
		req := cloneRequestWithBody(ctx, inbound, body)
		start := time.Now()
		result, err := deps.Adapter.Do(ctx, provider.Request{
			Operation:     op,
			ProviderType:  prov.Type,
			PublicModel:   "alias/" + r.Alias.Name,
			BaseURL:       prov.BaseURL,
			APIKey:        prov.APIKey,
			CopilotToken:  prov.CopilotToken,
			UpstreamModel: model.UpstreamName,
			ModelProtocol: model.Protocol,
			UserAgent:     prov.UserAgent,
			Version:       deps.Version,
			Body:          body,
			Inbound:       req,
			Client:        clientForProvider(deps, prov),
		})
		if deps.Metrics != nil && (result == nil || !result.Streaming) {
			status := 0
			if result != nil {
				status = result.StatusCode
			}
			deps.Metrics.RecordUpstream(op, t.Provider, status, err, time.Since(start).Seconds())
		}
		if deps.AccessLog && (result == nil || !result.Streaming) {
			attrs := []any{
				"alias", r.Alias.Name,
				"provider", t.Provider,
				"provider_type", prov.Type,
				"upstream_model", model.UpstreamName,
				"duration_ms", time.Since(start).Milliseconds(),
			}
			if result != nil {
				attrs = append(attrs, "status", result.StatusCode)
			}
			if err != nil {
				attrs = append(attrs, "error", err)
			}
			targetLogger.Info("upstream request finished", attrs...)
		}
		if result != nil && result.Streaming && err == nil {
			h.attachStreamFinalizers(deps, ctx, op, t.Provider, result, start, targetLogger, []any{
				"alias", r.Alias.Name,
				"provider", t.Provider,
				"provider_type", prov.Type,
				"upstream_model", model.UpstreamName,
			})
		} else {
			h.recordProviderHealth(deps, ctx, t.Provider, result, err, false)
		}
		h.instrumentUpstreamResponseSize(deps, op, t.Provider, result, err)
		if err != nil {
			releaseTarget()
			var invalid provider.ErrInvalidRequest
			if errors.As(err, &invalid) {
				return nil, err
			}
			observeCooldownError(cooldowns, ctx, fp, err)
			lastErr = err
			if hasUntriedTarget(pool, tried) && deps.Metrics != nil {
				deps.Metrics.RecordAliasRetry(r.Alias.Name, t.Provider, t.Model, "error")
			}
			targetLogger.Warn("alias target failed", "error", err)
			continue
		}
		observeCooldownResult(cooldowns, ctx, fp, result)
		existingClose := result.OnClose
		result.OnClose = func() {
			if existingClose != nil {
				existingClose()
			}
			releaseTarget()
		}
		if retryCodes[result.StatusCode] {
			if remaining, ok := allCoolingRemaining(cooldowns, pool); ok {
				closeResult(result)
				return provider.SyntheticCooldownResult(remaining), nil
			}
			if !hasUntriedTarget(pool, tried) {
				return result, nil
			}
			closeResult(result)
			lastErr = fmt.Errorf("upstream returned status %d", result.StatusCode)
			if deps.Metrics != nil {
				reason := "upstream_status"
				if result.StatusCode >= 500 {
					reason = "upstream_5xx"
				}
				deps.Metrics.RecordAliasRetry(r.Alias.Name, t.Provider, t.Model, reason)
			}
			targetLogger.Warn("alias target returned retryable status, retrying", "status", result.StatusCode)
			continue
		}
		return result, nil
	}
	if lastErr == nil {
		lastErr = errors.New("alias has no healthy targets")
	}
	return nil, lastErr
}

func clientForProvider(deps Dependencies, provider config.Provider) *http.Client {
	if deps.ClientForProvider != nil {
		return deps.ClientForProvider(provider)
	}
	return deps.Client
}

func (h *Handler) recordProviderHealth(deps Dependencies, ctx context.Context, providerName string, result *provider.Result, err error, downstreamCanceled bool) {
	if deps.Health == nil || providerName == "" {
		return
	}
	if downstreamCanceled {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	if err != nil {
		var invalid provider.ErrInvalidRequest
		if errors.As(err, &invalid) {
			return
		}
		var unsupported provider.ErrUnsupportedOperation
		if errors.As(err, &unsupported) {
			return
		}
		deps.Health.MarkFailureContext(ctx, providerName)
		return
	}
	if result != nil && result.StatusCode >= 500 {
		deps.Health.MarkFailureContext(ctx, providerName)
		return
	}
	deps.Health.MarkSuccessContext(ctx, providerName)
}

func (h *Handler) attachStreamFinalizers(deps Dependencies, ctx context.Context, op provider.Operation, providerName string, result *provider.Result, start time.Time, logger *slog.Logger, logAttrs []any) {
	existingClose := result.OnClose
	result.OnClose = func() {
		if existingClose != nil {
			existingClose()
		}
		outcome := provider.StreamOutcome{}
		if result.Stream != nil {
			outcome = result.Stream.Wait()
			result.Usage = outcome.Usage
		}
		streamErr := outcome.Err
		if outcome.DownstreamCanceled {
			streamErr = nil
		}
		if deps.Metrics != nil {
			deps.Metrics.RecordUpstream(op, providerName, result.StatusCode, streamErr, time.Since(start).Seconds())
		}
		if deps.AccessLog && logger != nil {
			attrs := append([]any{}, logAttrs...)
			attrs = append(attrs, "duration_ms", time.Since(start).Milliseconds(), "status", result.StatusCode)
			if outcome.Err != nil {
				attrs = append(attrs, "error", outcome.Err)
			}
			if outcome.DownstreamCanceled {
				attrs = append(attrs, "downstream_canceled", true)
			}
			logger.Info("upstream request finished", attrs...)
		}
		h.recordProviderHealth(deps, ctx, providerName, result, streamErr, outcome.DownstreamCanceled)
	}
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
			err := error(nil)
			if result.Stream != nil {
				outcome := result.Stream.Wait()
				if !outcome.DownstreamCanceled {
					err = outcome.Err
				}
			}
			deps.Metrics.RecordUpstreamResponseSize(op, providerName, result.StatusCode, err, counter.BytesRead())
		}
		return
	}
	deps.Metrics.RecordUpstreamResponseSize(op, providerName, result.StatusCode, err, len(result.Body))
}

type countingReadCloser struct {
	io.ReadCloser
	bytesRead int64
}

func cloneRequestWithBody(ctx context.Context, inbound *http.Request, body []byte) *http.Request {
	if inbound == nil {
		return nil
	}
	req := inbound.Clone(ctx)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	return req
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
