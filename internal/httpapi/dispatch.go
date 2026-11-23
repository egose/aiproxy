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
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func (h *Handler) dispatchDirect(deps Dependencies, ctx context.Context, op provider.Operation, r modelresolver.ResolveResult, inbound *http.Request, body []byte, logger *slog.Logger) (*provider.Result, error) {
	passthrough := r.Kind == modelresolver.KindModelPassthrough
	target, upstreamModel := r.Model.Name, r.Model.UpstreamName
	protocol := r.Model.Protocol
	publicModel := r.Provider.Name + "/" + target
	if passthrough {
		target, upstreamModel = modelPassthroughTargetIdentity(r), r.PassthroughSuffix
		protocol = r.PassthroughRule.Protocol()
		publicModel = r.Provider.Name + "/" + upstreamModel
	}
	if deps.MultiTenancy.Enabled {
		if _, scoped := r.Workspace.ID(); !scoped || r.Workspace != r.Provider.Workspace ||
			(!passthrough && !deps.Resolver.ModelVisibleInWorkspace(publicModel, r.Workspace)) {
			return nil, modelresolver.ErrUnknownModel{Model: publicModel}
		}
	}
	er := encryptedReasoningForTarget(deps, r.Provider, r.Model, nil)
	if encryptedReasoningApplies(op) && !er.Passthrough {
		stripped, changed, err := stripDispatchOpaqueBlocks(ctx, op, body)
		if err != nil {
			return nil, err
		}
		if changed {
			body = stripped
		}
	}
	if deps.Metrics != nil {
		deps.Metrics.RecordProviderSelection(op, r.Provider.Name+"/"+target, r.Provider.Name, target)
	}
	execution := deferredExecutionFromContext(ctx)
	var targetModel config.Model
	var transport *config.DeferredTransport
	if !passthrough {
		targetModel = r.Model
		transport = config.EffectiveDeferredTransport(r.Provider, targetModel)
	}
	request := provider.Request{
		Operation:            op,
		ProviderType:         r.Provider.Type,
		PublicModel:          publicModel,
		BaseURL:              r.Provider.BaseURL,
		APIKey:               r.Provider.APIKey,
		CopilotToken:         r.Provider.CopilotToken,
		UpstreamModel:        upstreamModel,
		ModelProtocol:        protocol,
		ModelPassthrough:     passthrough,
		UserAgent:            r.Provider.UserAgent,
		ForwardUserAgent:     r.Provider.ForwardUserAgent,
		ForwardHeaders:       r.Provider.ForwardHeaders,
		UpstreamHeaders:      deps.Resolver.UpstreamHeaders(r.Provider, targetModel, nil),
		ForwardAuthorization: r.Provider.ForwardAuthorization,
		Version:              deps.Version,
		Client:               clientForProvider(deps, r.Provider),
		DeferredTransport:    transport,
		ExecutionLimits:      execution.executionLimits(),
		ObserveBody:          execution.bodyObserver(),
	}
	recovery := false
	for {
		if recovery {
			if err := allowReasoningRecovery(ctx); err != nil {
				return nil, err
			}
		}
		request.Body = body
		request.Inbound = cloneRequestWithBody(ctx, inbound, body)
		if deps.AccessLog && logger != nil {
			logger.Info("upstream request started",
				"provider", r.Provider.Name,
				"provider_type", r.Provider.Type,
				"upstream_model", upstreamModel,
			)
		}
		start := time.Now()
		result, err := deps.Adapter.Do(ctx, request)
		if result != nil {
			result.Provider = r.Provider.Name
			result.UpstreamModel = target
		}
		if deps.Metrics != nil && !provider.IsPeerControlOutcome(result, err) && (result == nil || !result.Streaming) {
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
				"upstream_model", upstreamModel,
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
				"upstream_model", upstreamModel,
			})
			h.instrumentUpstreamResponseSize(deps, op, r.Provider.Name, result, err)
			return result, nil
		}
		h.recordProviderHealth(deps, ctx, r.Provider.Name, result, err, false)
		h.instrumentUpstreamResponseSize(deps, op, r.Provider.Name, result, err)
		if err == nil && !recovery {
			stripped, changed, rewriteErr := recoverDispatchOpaqueBlocks(ctx, op, result, body, er)
			if rewriteErr != nil {
				closeResult(result)
				return nil, rewriteErr
			}
			if changed {
				closeResult(result)
				body, recovery = stripped, true
				if deps.AccessLog && logger != nil {
					logger.Warn("upstream rejected encrypted reasoning, retrying stripped", "provider", r.Provider.Name)
				}
				continue
			}
		}
		return result, err
	}
}

type aliasPoolTarget struct {
	target      alias.Target
	provider    config.Provider
	model       config.Model
	fingerprint modelresolver.CooldownFingerprint
	ok          bool
}

func resolveAliasPool(deps Dependencies, aliasName string, targets []config.AliasTarget, workspace identity.WorkspaceScope) []aliasPoolTarget {
	pool := make([]aliasPoolTarget, 0, len(targets))
	for _, target := range targets {
		entry := aliasPoolTarget{target: alias.Target{Provider: target.Provider, Model: target.Model}}
		if deps.Resolver != nil {
			var prov config.Provider
			var model config.Model
			var ok bool
			if deps.MultiTenancy.Enabled {
				prov, model, ok = deps.Resolver.ModelInWorkspace(target.Provider, target.Model, workspace)
			} else {
				prov, model, ok = deps.Resolver.Model(target.Provider, target.Model)
			}
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

func observeCooldownResult(cooldowns *modelresolver.CooldownStore, ctx context.Context, fp modelresolver.CooldownFingerprint, result *provider.Result) {
	if cooldowns == nil || result == nil || !result.HasRetryDelay || provider.IsPeerControlOutcome(result, nil) {
		return
	}
	if ctx != nil && ctx.Err() != nil {
		return
	}
	delay := result.RetryDelay
	if !result.RetryDeadline.IsZero() {
		delay = time.Until(result.RetryDeadline)
	}
	if delay > 0 {
		cooldowns.Observe(fp, delay)
	}
}

func observeCooldownError(cooldowns *modelresolver.CooldownStore, ctx context.Context, fp modelresolver.CooldownFingerprint, err error) {
	if cooldowns == nil || err == nil || provider.IsPeerControlOutcome(nil, err) {
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
	publicModel := "alias/" + r.Alias.Name
	if deps.MultiTenancy.Enabled && (r.Workspace != r.Alias.Workspace || !deps.Resolver.ModelVisibleInWorkspace(publicModel, r.Workspace)) {
		return nil, modelresolver.ErrUnknownModel{Model: publicModel}
	}
	var lastErr error
	var lastErrorResult *provider.Result
	tried := make(map[alias.Target]bool, len(r.Alias.Targets))
	retryCodes := make(map[int]bool, len(r.Alias.RetryStatusCodes))
	for _, code := range r.Alias.RetryStatusCodes {
		retryCodes[code] = true
	}
	pool := resolveAliasPool(deps, r.Alias.Name, r.Alias.Targets, r.Workspace)
	stripApplies := encryptedReasoningApplies(op)
	explicitPolicy := r.Alias.EncryptedReasoning != nil
	aliasPrestripped := false
	recoveryUsed := false
	var cooldowns *modelresolver.CooldownStore
	if deps.Resolver != nil {
		cooldowns = deps.Resolver.Cooldowns()
	}
	if remaining, ok := allCoolingRemaining(cooldowns, pool); ok {
		return provider.SyntheticCooldownResult(remaining), nil
	}
	sessionKey, hasSession := "", false
	var affinityPool []alias.Target
	if r.Alias.SessionAffinity != nil && inbound != nil {
		if key, ok := alias.SessionKey(r.Alias, inbound.Header); ok {
			sessionKey, hasSession = key, true
			for _, entry := range pool {
				if entry.ok {
					affinityPool = append(affinityPool, entry.target)
				}
			}
			if len(affinityPool) == 0 {
				hasSession = false
			}
		}
	}
	var pending *provider.Result
	var pendingRetryProvider, pendingRetryModel, pendingRetryReason string
	var hasPendingRetry bool
	commitPending := func() {
		if hasPendingRetry && deps.Metrics != nil {
			deps.Metrics.RecordAliasRetry(r.Alias.Name, pendingRetryProvider, pendingRetryModel, pendingRetryReason)
		}
		hasPendingRetry = false
		if pending != nil {
			closeResult(pending)
			pending = nil
		}
	}
	affinityPending := hasSession
targetLoop:
	for {
		if recoveryUsed {
			if err := ctx.Err(); err != nil {
				closeResult(pending)
				return nil, err
			}
		}
		var t alias.Target
		var releaseLease func()
		if affinityPending {
			affinityPending = false
			if preferred, ok := alias.PreferredTarget(affinityPool, sessionKey); ok {
				if acquired, release, ok := r.Selector.AcquireSpecific(preferred, coolingExclusions(cooldowns, pool, tried)); ok {
					t, releaseLease = acquired, release
				}
			}
		}
		if releaseLease == nil && (t == alias.Target{}) {
			t, releaseLease = r.Selector.Acquire(coolingExclusions(cooldowns, pool, tried))
		}
		if t.Provider == "" && t.Model == "" {
			if pending != nil {
				return pending, nil
			}
			break
		}
		tried[t] = true
		entry := poolTargetFor(pool, t)
		if entry == nil || !entry.ok {
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
		prov, model, fp := entry.provider, entry.model, entry.fingerprint
		if _, cooling := cooldowns.Remaining(fp); cooling {
			releaseLease()
			continue
		}
		if deps.Health != nil && !deps.Health.IsHealthyContext(ctx, t.Provider) {
			releaseLease()
			lastErr = fmt.Errorf("alias has no healthy targets")
			continue
		}
		commitPending()
		lastErrorResult = nil
		targetLogger := logger.With("target", t.Provider+"/"+t.Model)
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
		currentBody := body
		er := encryptedReasoningForTarget(deps, prov, model, &r.Alias)
		if stripApplies && !er.Passthrough && (!explicitPolicy || !aliasPrestripped) {
			stripped, changed, err := stripDispatchOpaqueBlocks(ctx, op, currentBody)
			if err != nil {
				releaseTarget()
				return nil, err
			}
			if changed {
				currentBody = stripped
				if explicitPolicy {
					body = stripped
				}
			}
			if explicitPolicy {
				aliasPrestripped = true
			}
		}
		execution := deferredExecutionFromContext(ctx)
		request := provider.Request{
			Operation:            op,
			ProviderType:         prov.Type,
			PublicModel:          "alias/" + r.Alias.Name,
			BaseURL:              prov.BaseURL,
			APIKey:               prov.APIKey,
			CopilotToken:         prov.CopilotToken,
			UpstreamModel:        model.UpstreamName,
			ModelProtocol:        model.Protocol,
			UserAgent:            prov.UserAgent,
			ForwardUserAgent:     prov.ForwardUserAgent,
			ForwardHeaders:       prov.ForwardHeaders,
			UpstreamHeaders:      deps.Resolver.UpstreamHeaders(prov, model, &r.Alias),
			ForwardAuthorization: prov.ForwardAuthorization,
			Version:              deps.Version,
			Client:               clientForProvider(deps, prov),
			DeferredTransport:    config.EffectiveDeferredTransport(prov, model),
			ExecutionLimits:      execution.executionLimits(),
			ObserveBody:          execution.bodyObserver(),
		}
		recoveryPending := false
		for {
			if recoveryPending {
				if err := allowReasoningRecovery(ctx); err != nil {
					releaseTarget()
					return nil, err
				}
				recoveryUsed = true
				recoveryPending = false
			}
			request.Body = currentBody
			request.Inbound = cloneRequestWithBody(ctx, inbound, currentBody)
			if deps.AccessLog {
				targetLogger.Info("upstream request started",
					"alias", r.Alias.Name,
					"provider", t.Provider,
					"provider_type", prov.Type,
					"upstream_model", model.UpstreamName,
				)
			}
			start := time.Now()
			result, err := deps.Adapter.Do(ctx, request)
			if result != nil {
				result.Provider = t.Provider
				result.UpstreamModel = model.Name
			}
			if deps.Metrics != nil && !provider.IsPeerControlOutcome(result, err) && (result == nil || !result.Streaming) {
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
				if provider.IsPeerControlOutcome(result, err) {
					if pending != nil {
						closeResult(pending)
					}
					return result, err
				}
				closeResult(result)
				var unsupported provider.ErrUnsupportedOperation
				if execution != nil && (ctx.Err() != nil || errors.Is(err, deferred.ErrBudgetExceeded) || errors.Is(err, deferred.ErrBudgetClosed) || errors.As(err, &unsupported)) {
					if pending != nil {
						closeResult(pending)
					}
					return nil, err
				}
				var invalid provider.ErrInvalidRequest
				if errors.As(err, &invalid) {
					if pending != nil {
						closeResult(pending)
						pending = nil
					}
					return result, err
				}
				observeCooldownError(cooldowns, ctx, fp, err)
				lastErr = err
				lastErrorResult = result
				pendingRetryProvider, pendingRetryModel, pendingRetryReason = t.Provider, t.Model, "error"
				hasPendingRetry = true
				targetLogger.Warn("alias target failed", "error", err)
				continue targetLoop
			}
			observeCooldownResult(cooldowns, ctx, fp, result)
			if !recoveryUsed {
				stripped, changed, err := recoverDispatchOpaqueBlocks(ctx, op, result, currentBody, er)
				if err != nil {
					closeResult(result)
					releaseTarget()
					return nil, err
				}
				if changed {
					closeResult(result)
					recoveryPending = true
					if explicitPolicy {
						body = stripped
					}
					currentBody = stripped
					targetLogger.Warn("alias target rejected encrypted reasoning, retrying stripped")
					continue
				}
			}
			existingClose := result.OnClose
			result.OnClose = func() {
				if existingClose != nil {
					existingClose()
				}
				releaseTarget()
			}
			if !provider.IsPeerControlOutcome(result, nil) && !result.Streaming && retryCodes[result.StatusCode] {
				if remaining, ok := allCoolingRemaining(cooldowns, pool); ok {
					if pending != nil {
						closeResult(pending)
						pending = nil
					}
					closeResult(result)
					return provider.SyntheticCooldownResult(remaining), nil
				}
				reason := "upstream_status"
				if result.StatusCode >= 500 {
					reason = "upstream_5xx"
				}
				pending = result
				pendingRetryProvider, pendingRetryModel, pendingRetryReason = t.Provider, t.Model, reason
				hasPendingRetry = true
				lastErr = fmt.Errorf("upstream returned status %d", result.StatusCode)
				targetLogger.Warn("alias target returned retryable status, retrying", "status", result.StatusCode)
				continue targetLoop
			}
			return result, nil
		}
	}
	if lastErr == nil {
		lastErr = errors.New("alias has no healthy targets")
	}
	return lastErrorResult, lastErr
}

func clientForProvider(deps Dependencies, provider config.Provider) *http.Client {
	if deps.ClientForProvider != nil {
		return deps.ClientForProvider(provider)
	}
	return deps.Client
}

func (h *Handler) recordProviderHealth(deps Dependencies, ctx context.Context, providerName string, result *provider.Result, err error, downstreamCanceled bool) {
	if deps.Health == nil || providerName == "" || provider.IsPeerControlOutcome(result, err) {
		return
	}
	if downstreamCanceled {
		return
	}
	if execution := deferredExecutionFromContext(ctx); execution != nil {
		execution.recordHealth(ctx, providerName, result, err)
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
		if deps.Metrics != nil && !provider.IsPeerControlOutcome(result, streamErr) {
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
	if deps.Metrics == nil || result == nil || provider.IsPeerControlOutcome(result, err) {
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
			if !provider.IsPeerControlOutcome(result, err) {
				deps.Metrics.RecordUpstreamResponseSize(op, providerName, result.StatusCode, err, counter.BytesRead())
			}
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
