package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/provider"
)

type deferredExecutionKey struct{}

type deferredHealthRecord struct {
	name    string
	healthy bool
	next    *deferredHealthRecord
}

type deferredExecution struct {
	deps            Dependencies
	principal       auth.Principal
	resolved        modelresolver.ResolveResult
	body            []byte
	model           string
	effort          string
	requestID       string
	start           time.Time
	logger          *slog.Logger
	memory          *admissionMemory
	limits          *provider.ExecutionLimits
	sources         deferredSource
	healthHead      *deferredHealthRecord
	healthTail      *deferredHealthRecord
	healthRemaining int
	entry           *payloadlog.Entry
	responsePrefix  []byte
	responseTotal   int
	captureLimit    int64
	snapshotLimit   int64
	event           accounting.Event
	completionKind  deferred.OutcomeKind
}

func deferredExecutionFromContext(ctx context.Context) *deferredExecution {
	execution, _ := ctx.Value(deferredExecutionKey{}).(*deferredExecution)
	return execution
}

func (e *deferredExecution) executionLimits() *provider.ExecutionLimits {
	if e == nil {
		return nil
	}
	return e.limits
}

func (e *deferredExecution) bodyObserver() func(io.ReadCloser) io.ReadCloser {
	if e == nil {
		return nil
	}
	return e.sources.track
}

func deferredCaptureLimit(configured int, ceiling int64) int64 {
	if configured <= 0 {
		return ceiling
	}
	return min(int64(configured), ceiling)
}

func prepareDeferredExecution(deps Dependencies, principal *auth.Principal, admission inferenceAdmission, input deferred.Input, limits *provider.ExecutionLimits, memory *admissionMemory, requestID string, start time.Time) (*deferredExecution, error) {
	if len(principal.Name) > 256 || len(principal.Tenant) > 256 || len(admission.ReasoningEffort) > deferred.MaxMetadataBytes {
		return nil, fmt.Errorf("deferred completion identity metadata exceeds limits")
	}
	if err := memory.reserve(deferred.JobMetadataBytes + int64(len(admission.Resolved.Alias.Targets))*1024); err != nil {
		return nil, err
	}
	for _, target := range admission.Resolved.Alias.Targets {
		if err := memory.reserve((int64(len(target.Provider)) + int64(len(target.Model))) * 8); err != nil {
			return nil, err
		}
	}
	copy := *principal
	copy.Name, copy.Tenant = strings.Clone(copy.Name), strings.Clone(copy.Tenant)
	e := &deferredExecution{
		deps: deps, principal: copy, resolved: admission.Resolved, body: admission.Body,
		model: input.PublicModel, effort: strings.Clone(admission.ReasoningEffort), requestID: strings.Clone(requestID),
		start: start, logger: admission.Logger, memory: memory,
		healthRemaining: max(1, len(admission.Resolved.Alias.Targets)) + 1,
	}
	if deps.PayloadLog == nil {
		return e, nil
	}
	configured := deps.PayloadLog.MaxBodyBytes()
	requestLimit := deferredCaptureLimit(configured, input.Limits.RequestBytes)
	e.captureLimit = deferredCaptureLimit(configured, input.Limits.ResultBytes)
	e.snapshotLimit = requestLimit
	safeBody := input.Body
	if deps.Guardrails != nil {
		state := admission.GuardrailLog
		if !deps.RedactGuardrailBodies || state == nil || state.incomplete {
			safeBody, e.snapshotLimit = nil, 0
		} else if len(state.redactions) > 0 {
			safeBody = rewriteGuardrailBody(input.Body, state.redactions, limits)
			if safeBody == nil {
				e.snapshotLimit = 0
			}
		}
	}
	requestBytes := min(int64(len(safeBody)), requestLimit)
	if err := memory.reserve((requestBytes+e.snapshotLimit+e.captureLimit)*8 + int64(deferred.JSONTokens)*64 + 4*deferred.JobMetadataBytes); err != nil {
		return nil, err
	}
	e.entry = &payloadlog.Entry{
		RequestID: requestID, Method: http.MethodPost, Path: "/v1/chat/completions",
		Operation: provider.OpChatCompletions.String(), PublicModel: e.model,
		Client: copy.Name, Tenant: copy.Tenant, ReasoningEffort: e.effort,
		Request: payloadlog.EntrySide{
			Headers: payloadlog.RedactHeaders(http.Header(input.Headers)),
			Body:    payloadlog.EncodeBodyWithContentType(safeBody, int(requestLimit), "application/json"),
		},
	}
	return e, nil
}

func (e *deferredExecution) recordHealth(ctx context.Context, name string, result *provider.Result, err error) {
	if e.deps.Health == nil || name == "" {
		return
	}
	if e.completionKind == deferred.OutcomeCancelled || e.completionKind == deferred.OutcomeExecutionTimeout ||
		e.completionKind == deferred.OutcomeLimitExceeded || e.completionKind == "" && ctx.Err() != nil {
		return
	}
	var invalid provider.ErrInvalidRequest
	var unsupported provider.ErrUnsupportedOperation
	var limit provider.ErrExecutionLimit
	if errors.As(err, &invalid) || errors.As(err, &unsupported) || errors.As(err, &limit) ||
		errors.Is(err, deferred.ErrBudgetExceeded) || errors.Is(err, deferred.ErrBudgetClosed) {
		return
	}
	if e.healthRemaining == 0 {
		return
	}
	e.healthRemaining--
	healthy := err == nil && (result == nil || result.StatusCode < 500)
	e.deps.Health.RecordLocal(name, healthy)
	record := &deferredHealthRecord{name: strings.Clone(name), healthy: healthy}
	if e.healthTail == nil {
		e.healthHead = record
	} else {
		e.healthTail.next = record
	}
	e.healthTail = record
}

func (e *deferredExecution) work(h *Handler) deferred.Work {
	return func(ctx context.Context, writer *deferred.Writer) (outcome deferred.Outcome) {
		input := writer.Input()
		e.limits = &provider.ExecutionLimits{
			RequestBytes: input.Limits.RequestBytes, ResponseBytes: input.Limits.ResultBytes,
			SnapshotBytes: min(e.snapshotLimit, input.Limits.RequestBytes), Work: writer.WorkBudget(),
			JSONDepth: deferred.JSONDepth, JSONTokens: deferred.JSONTokens,
			SSELineBytes: deferred.SSELineBytes, SSEEventBytes: deferred.SSEEventBytes, SSEDataLines: deferred.SSEDataLines,
		}
		e.captureLimit = min(e.captureLimit, input.Limits.ResultBytes)
		e.logger = e.logger.With("job_id", writer.Receipt().ID)
		ctx = context.WithValue(ctx, deferredExecutionKey{}, e)
		outcome = deferred.Outcome{Kind: deferred.OutcomeUpstreamError, Error: &deferred.ErrorDetail{Type: "upstream_error", Message: "upstream request failed"}}
		var result *provider.Result
		var head *deferred.ResponseHead
		defer func() {
			if recover() != nil {
				outcome = deferred.Outcome{Kind: deferred.OutcomeUpstreamError, Error: &deferred.ErrorDetail{Type: "upstream_error", Message: "deferred execution failed"}}
			}
			usage := provider.Usage{}
			if result != nil {
				usage = result.Usage
				if result.Stream != nil {
					usage = result.Stream.Snapshot().Usage
				}
			}
			outcome.Usage = deferredUsage(usage)
			outcome = writer.Finish(outcome)
			e.completionKind = outcome.Kind
			if result != nil && result.Stream != nil {
				var streamErr error
				if outcome.Error != nil {
					streamErr = errors.New(outcome.Error.Type)
				}
				result.Stream.Complete(streamErr, outcome.Kind == deferred.OutcomeCancelled)
				result.Usage = result.Stream.Wait().Usage
			}
			e.publishCompletion(h, outcome, result, head)
			if result != nil {
				closeResult(result)
			}
			e.body, e.resolved = nil, modelresolver.ResolveResult{}
		}()
		if err := writer.SetFinalizer(func(completionCtx context.Context) error {
			defer e.memory.close()
			return e.finalize(h, completionCtx)
		}); err != nil {
			e.memory.close()
			return outcome
		}
		if err := writer.SetSource(&e.sources); err != nil {
			return outcome
		}
		if e.entry != nil && e.captureLimit > 0 {
			e.responsePrefix = make([]byte, 0, int(e.captureLimit))
		}
		inbound := syntheticInferenceRequest(ctx, http.Header(input.Headers), e.body)
		var err error
		if e.resolved.Kind == modelresolver.KindDirect {
			result, err = h.dispatchDirect(e.deps, ctx, provider.OpChatCompletions, e.resolved, inbound, e.body, e.logger)
		} else {
			result, err = h.dispatchAlias(e.deps, ctx, provider.OpChatCompletions, e.resolved, inbound, e.body, e.logger)
		}
		if err != nil {
			return deferredExecutionFailure(err)
		}
		if result == nil {
			return outcome
		}
		if result.StreamBody != nil {
			result.StreamBody = e.sources.track(result.StreamBody)
		}
		selectedHead, body, err := e.response(result)
		if err != nil {
			return deferredExecutionFailure(err)
		}
		if err := writer.SetResponse(selectedHead); err != nil {
			return deferredExecutionFailure(err)
		}
		head = &selectedHead
		if !selectedHead.Streaming {
			n, err := writer.Write(body)
			e.capture(body[:n])
			if err != nil {
				return deferredExecutionFailure(err)
			}
		} else {
			if err := e.copyStream(writer, result); err != nil {
				return deferredExecutionFailure(err)
			}
		}
		if result.Stream != nil {
			result.Stream.Complete(nil, false)
			if stream := result.Stream.Wait(); stream.Err != nil {
				return deferredExecutionFailure(stream.Err)
			}
		}
		return deferred.Outcome{Kind: deferred.OutcomeResponse}
	}
}

func (e *deferredExecution) response(result *provider.Result) (deferred.ResponseHead, []byte, error) {
	head := deferred.ResponseHead{StatusCode: result.StatusCode, Headers: result.Header, Streaming: result.Streaming}
	if result.HasRetryDelay && !result.RetryObservedAt.IsZero() && !result.RetryDeadline.IsZero() {
		observed, deadline := result.RetryObservedAt.UnixMilli(), result.RetryDeadline.UnixMilli()
		remaining := max(int64(0), deadline-time.Now().UnixMilli())
		head.Retry = &deferred.RetryAdvice{ObservedAtMS: observed, DeadlineMS: deadline, RemainingMS: min(remaining, deadline-observed)}
	}
	body := result.Body
	if result.StatusCode < 400 || isJSONContentType(result.Header.Get("Content-Type")) {
		return head, body, nil
	}
	if result.Streaming && result.StreamBody != nil {
		limit := min(maxUpstreamErrorBodyBytes, e.limits.ResponseBytes)
		if err := e.memory.reserve(limit*4 + 16<<10); err != nil {
			return head, nil, err
		}
		read, err := io.ReadAll(io.LimitReader(result.StreamBody, limit))
		if err != nil {
			return head, nil, err
		}
		body = read
	}
	if err := e.memory.reserve(int64(len(body))*2 + 16<<10); err != nil {
		return head, nil, err
	}
	envelope := upstreamErrorEnvelope(result.StatusCode, body)
	size, err := deferred.JSONEncodedSize(envelope, e.limits.ResponseBytes-1, deferred.JSONDepth, deferred.JSONTokens)
	if err != nil {
		return head, nil, provider.ErrExecutionLimit{Kind: "normalized response", Limit: e.limits.ResponseBytes}
	}
	if err := e.memory.reserve(max(int64(512), size*4) + 16<<10); err != nil {
		return head, nil, err
	}
	body, err = json.Marshal(envelope)
	if err != nil {
		return head, nil, err
	}
	body = append(body, '\n')
	head.Headers, head.Streaming = http.Header{"Content-Type": {"application/json"}}, false
	return head, body, nil
}

func (e *deferredExecution) copyStream(writer *deferred.Writer, result *provider.Result) error {
	release, err := writer.WorkBudget().Reserve(32 << 10)
	if err != nil {
		return err
	}
	defer release()
	buffer := make([]byte, 32<<10)
	emptyReads := 0
	for {
		n, readErr := result.StreamBody.Read(buffer)
		if n > 0 {
			written, err := writer.Write(buffer[:n])
			e.capture(buffer[:written])
			if err != nil {
				return err
			}
			emptyReads = 0
		} else {
			emptyReads++
			if emptyReads >= 100 {
				return io.ErrNoProgress
			}
		}
		if result.Stream != nil {
			select {
			case <-result.Stream.Done():
				if outcome := result.Stream.Wait(); outcome.Err != nil {
					return outcome.Err
				}
			default:
			}
		}
		if readErr == io.EOF {
			return nil
		}
		if readErr != nil {
			return readErr
		}
	}
}

func (e *deferredExecution) capture(data []byte) {
	e.responseTotal += len(data)
	if e.entry == nil {
		return
	}
	n := min(len(data), cap(e.responsePrefix)-len(e.responsePrefix))
	e.responsePrefix = append(e.responsePrefix, data[:n]...)
}

func deferredUsage(usage provider.Usage) deferred.Usage {
	return deferred.Usage{
		PromptTokens: max(int64(0), usage.PromptTokens), CompletionTokens: max(int64(0), usage.CompletionTokens),
		TotalTokens: max(int64(0), usage.TotalTokens), CachedTokens: max(int64(0), usage.CachedTokens),
		CacheCreationTokens: max(int64(0), usage.CacheCreationTokens), CacheReadTokens: max(int64(0), usage.CacheReadTokens),
	}
}

func deferredExecutionFailure(err error) deferred.Outcome {
	var invalid provider.ErrInvalidRequest
	var unsupported provider.ErrUnsupportedOperation
	var limit provider.ErrExecutionLimit
	if errors.As(err, &invalid) {
		return deferred.Outcome{Kind: deferred.OutcomeInvalidRequest, Error: &deferred.ErrorDetail{Type: "invalid_request", Message: "upstream adapter rejected the request"}}
	}
	if errors.As(err, &unsupported) {
		return deferred.Outcome{Kind: deferred.OutcomeInvalidRequest, Error: &deferred.ErrorDetail{Type: "unsupported_operation", Message: "upstream adapter does not support this request"}}
	}
	if errors.As(err, &limit) || errors.Is(err, deferred.ErrBudgetExceeded) || errors.Is(err, deferred.ErrBudgetClosed) {
		return deferred.Outcome{Kind: deferred.OutcomeLimitExceeded, Error: &deferred.ErrorDetail{Type: "deferred_limit_exceeded", Message: "deferred execution allowance exceeded"}}
	}
	return deferred.Outcome{Kind: deferred.OutcomeUpstreamError, Error: &deferred.ErrorDetail{Type: "upstream_error", Message: "upstream request failed"}}
}

func (e *deferredExecution) publishCompletion(h *Handler, outcome deferred.Outcome, result *provider.Result, head *deferred.ResponseHead) {
	status := http.StatusBadGateway
	switch outcome.Kind {
	case deferred.OutcomeResponse:
		status = head.StatusCode
	case deferred.OutcomeInvalidRequest:
		status = http.StatusBadRequest
	case deferred.OutcomeExecutionTimeout:
		status = http.StatusGatewayTimeout
	}
	u := outcome.Usage
	event := accounting.Event{
		RequestID: e.requestID, PublicModel: e.model, Model: e.model, Operation: provider.OpChatCompletions.String(),
		Timestamp: time.Now(), Client: e.principal.Name, Tenant: e.principal.Tenant,
		StatusCode: status, ReasoningEffort: e.effort, Duration: time.Since(e.start),
		PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens, TotalTokens: u.TotalTokens,
		CachedTokens: u.CachedTokens, CacheCreationTokens: u.CacheCreationTokens, CacheReadTokens: u.CacheReadTokens,
	}
	if result != nil {
		event.Provider, event.UpstreamModel = result.Provider, result.UpstreamModel
	}
	e.event = event
	e.deps.Accounting.Record(event)
	usage := provider.Usage{PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens, TotalTokens: u.TotalTokens,
		CachedTokens: u.CachedTokens, CacheCreationTokens: u.CacheCreationTokens, CacheReadTokens: u.CacheReadTokens}
	h.recordLocalQuotaUsage(e.deps, &e.principal, e.model, usage)
	if e.entry != nil {
		entry := e.entry
		entry.Status, entry.DurationMs = status, event.Duration.Milliseconds()
		entry.Provider, entry.UpstreamModel = event.Provider, event.UpstreamModel
		if head != nil {
			entry.Streaming = head.Streaming
			headers, _ := deferred.FilterResponseHeaders(head.Headers)
			entry.Response.Headers = payloadlog.RedactHeaders(headers)
			entry.Response.Body = payloadlog.EncodeSnapshot(e.responsePrefix, int(e.captureLimit), head.Headers.Get("Content-Type"), e.responseTotal > len(e.responsePrefix))
			entry.Response.Body.Bytes = e.responseTotal
		}
		if result != nil && e.snapshotLimit > 0 {
			entry.UpstreamRequest, _ = payloadUpstreamRequest(result, int(e.snapshotLimit), false)
		}
		if outcome.Error != nil {
			entry.Error = outcome.Error.Type
		}
	}
	e.logger.Info("deferred inference finished", "status", status, "outcome", outcome.Kind, "duration_ms", event.Duration.Milliseconds(), "response_bytes", e.responseTotal)
}

func (e *deferredExecution) finalize(h *Handler, ctx context.Context) error {
	var failure error
	for record := e.healthHead; record != nil; record = record.next {
		if err := e.deps.Health.PersistContext(ctx, record.name, record.healthy); err != nil {
			e.logger.Warn("deferred finalization failed", "sink", "health", "error", err)
			failure = errors.Join(failure, err)
		}
	}
	usage := provider.Usage{PromptTokens: e.event.PromptTokens, CompletionTokens: e.event.CompletionTokens, TotalTokens: e.event.TotalTokens,
		CachedTokens: e.event.CachedTokens, CacheCreationTokens: e.event.CacheCreationTokens, CacheReadTokens: e.event.CacheReadTokens}
	if err := h.persistQuotaUsageContext(ctx, e.deps, &e.principal, e.model, e.event.UpstreamModel, e.event.Provider, usage); err != nil {
		e.logger.Warn("deferred finalization failed", "sink", "quota", "error", err)
		failure = errors.Join(failure, err)
	}
	if e.entry != nil {
		if err := payloadlog.RecordContext(ctx, e.deps.PayloadLog, *e.entry); err != nil {
			e.logger.Warn("deferred finalization failed", "sink", "payload", "error", err)
			failure = errors.Join(failure, err)
		}
	}
	e.responsePrefix, e.entry, e.healthHead, e.healthTail = nil, nil, nil, nil
	return failure
}

type deferredSource struct {
	mu      sync.Mutex
	current *deferredSourceBody
	closed  bool
}

type deferredSourceBody struct {
	io.ReadCloser
	owner *deferredSource
	once  sync.Once
	err   error
}

func (s *deferredSource) track(body io.ReadCloser) io.ReadCloser {
	owned := &deferredSourceBody{ReadCloser: body, owner: s}
	s.mu.Lock()
	s.current = owned
	closed := s.closed
	s.mu.Unlock()
	if closed {
		_ = owned.Close()
	}
	return owned
}

func (b *deferredSourceBody) Close() error {
	b.once.Do(func() {
		b.err = b.ReadCloser.Close()
		b.owner.mu.Lock()
		if b.owner.current == b {
			b.owner.current = nil
		}
		b.owner.mu.Unlock()
	})
	return b.err
}

func (s *deferredSource) Close() error {
	s.mu.Lock()
	s.closed = true
	current := s.current
	s.mu.Unlock()
	if current != nil {
		return current.Close()
	}
	return nil
}
