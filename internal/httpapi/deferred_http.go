package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
)

const deferredPath = "/_internal/deferred"

func isDeferredPath(path string) bool {
	return strings.HasPrefix(path, deferredPath)
}

func deferredRoute(path, method string) (string, string, string) {
	if len(path) > len(deferred.Prefix)+64 {
		return "unknown", "", ""
	}
	if path == deferred.Prefix+"/capabilities" {
		return "capabilities", "", "GET"
	}
	if path == deferred.Prefix+"/submit" {
		return "submit", "", "POST"
	}
	if !strings.HasPrefix(path, deferred.Prefix+"/jobs/") {
		return "unknown", "", ""
	}
	parts := strings.Split(strings.TrimPrefix(path, deferred.Prefix+"/jobs/"), "/")
	if len(parts) == 1 && parts[0] != "" {
		if method == http.MethodDelete {
			return "delete", parts[0], "GET, DELETE"
		}
		return "status", parts[0], "GET, DELETE"
	}
	if len(parts) == 2 && parts[0] != "" {
		switch parts[1] {
		case "result", "read":
			return parts[1], parts[0], "GET"
		}
	}
	return "unknown", "", ""
}

func deferredMetricsPath(path, method string) string {
	route, _, _ := deferredRoute(path, method)
	switch route {
	case "capabilities", "submit":
		return deferred.Prefix + "/" + route
	case "status", "delete":
		return deferred.Prefix + "/jobs/{id}"
	case "read", "result":
		return deferred.Prefix + "/jobs/{id}/" + route
	default:
		return deferredPath + "/unknown"
	}
}

func deferredOwner(principal *auth.Principal) (deferred.Owner, error) {
	var owner deferred.Owner
	if keyID, workspaceID, ok := principal.DatabaseKey(); ok {
		owner = deferred.Owner{Kind: "key", ID: keyID.String(), Workspace: workspaceID.String()}
	} else if name, ok := principal.CredentialID().StaticClientName(); ok {
		if workspaceID, scoped := principal.WorkspaceScope().ID(); !scoped || workspaceID != store.SystemWorkspaceID {
			return owner, auth.ErrInvalidToken
		}
		owner = deferred.Owner{Kind: "static", ID: name, Tenant: principal.Tenant}
	} else {
		return owner, auth.ErrInvalidToken
	}
	if deferred.ValidateOwner(owner) != nil {
		return deferred.Owner{}, auth.ErrInvalidToken
	}
	return owner, nil
}

func deferredReceiverLimits(deps Dependencies, resolved modelresolver.ResolveResult) (deferred.ReceiverLimits, error) {
	var limits deferred.ReceiverLimits
	check := func(p config.Provider, model config.Model) error {
		if p.DeferredReceiver == nil || config.HasCallerDeferredTransport(p) || model.DeferredTransport != nil || p.ForwardAuthorization ||
			!config.HasCapability(config.EffectiveCapabilities(p.Type, model), config.CapabilityChat) {
			return fmt.Errorf("every deferred target must be receiver-opted-in and chat-capable without caller deferral")
		}
		receiver := p.DeferredReceiver
		if receiver.ExecutionTimeout > 0 && (limits.ExecutionTimeout == 0 || receiver.ExecutionTimeout < limits.ExecutionTimeout) {
			limits.ExecutionTimeout = receiver.ExecutionTimeout
		}
		if receiver.MaxResultBytes > 0 && (limits.MaxResultBytes == 0 || receiver.MaxResultBytes < limits.MaxResultBytes) {
			limits.MaxResultBytes = receiver.MaxResultBytes
		}
		return nil
	}
	if resolved.Kind == modelresolver.KindDirect {
		err := check(resolved.Provider, resolved.Model)
		return limits, err
	}
	if resolved.Kind != modelresolver.KindAlias || len(resolved.Alias.Targets) == 0 {
		return limits, fmt.Errorf("deferred alias has no eligible target pool")
	}
	for _, target := range resolved.Alias.Targets {
		var p config.Provider
		var model config.Model
		var ok bool
		if deps.MultiTenancy.Enabled {
			p, model, ok = deps.Resolver.ModelInWorkspace(target.Provider, target.Model, resolved.Workspace)
		} else {
			p, model, ok = deps.Resolver.Model(target.Provider, target.Model)
		}
		if !ok {
			return limits, fmt.Errorf("deferred alias target is unavailable")
		}
		if err := check(p, model); err != nil {
			return limits, err
		}
	}
	return limits, nil
}

func deferredResponseHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", deferred.MediaType)
	w.Header().Set("X-Aiproxy-Deferred-Version", "1")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Vary", "Authorization")
}

func deferredAcceptance(route string) deferred.Acceptance {
	switch route {
	case "capabilities":
		return deferred.AcceptanceNotAccepted
	case "submit", "unknown":
		return deferred.AcceptanceIndeterminate
	default:
		return deferred.AcceptanceAccepted
	}
}

func deferredFailure(status int, code, message string, acceptance deferred.Acceptance) *deferred.ProtocolError {
	max := deferred.MaxErrorBytes - len(code)
	if max < 0 {
		code, message = "invalid_request", "invalid error metadata"
	} else if len(message) > max {
		message = message[:max]
		for !utf8.ValidString(message) {
			message = message[:len(message)-1]
		}
	}
	return deferred.NewProtocolError(status, deferred.ErrorDetail{Type: code, Message: message}, acceptance)
}

func (h *Handler) writeDeferredFailure(deps Dependencies, w http.ResponseWriter, route string, failure *deferred.ProtocolError) {
	deferredResponseHeaders(w)
	if failure.RetryAfter != "" {
		w.Header().Set("Retry-After", failure.RetryAfter)
	} else if failure.Wire.PollAfterMS > 0 {
		w.Header().Set("Retry-After", "1")
	}
	deps.Metrics.RecordDeferredControl(route, failure.Wire.Error.Type)
	if route == "submit" {
		deps.Metrics.RecordDeferredAdmission(failure.Wire.Error.Type)
	}
	var value any = failure.Wire
	if failure.Admission {
		e := apiError{}
		e.Error.Type, e.Error.Message, e.Error.BlockID = failure.Wire.Error.Type, failure.Wire.Error.Message, failure.Wire.Error.BlockID
		value = e
	}
	data, err := json.Marshal(value)
	if err != nil {
		return
	}
	data = append(data, '\n')
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(failure.Status)
	n, err := w.Write(data)
	if err == nil && n == len(data) {
		err = flushDeferredResponse(w)
	}
	if err != nil || n != len(data) {
		deps.Metrics.RecordDeferredDeliveryError(route)
	}
}

func flushDeferredResponse(w http.ResponseWriter) error {
	err := http.NewResponseController(w).Flush()
	if errors.Is(err, http.ErrNotSupported) {
		return nil
	}
	return err
}

type deferredStatusWriter struct {
	w       http.ResponseWriter
	status  int
	started bool
}

func (w *deferredStatusWriter) Write(data []byte) (int, error) {
	w.started = true
	w.w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.w.WriteHeader(w.status)
	n, err := w.w.Write(data)
	if err == nil && n == len(data) {
		err = flushDeferredResponse(w.w)
	}
	return n, err
}

func (h *Handler) writeDeferredValue(deps Dependencies, w http.ResponseWriter, control *deferred.Control, route string, status int, value any, logger *slog.Logger) {
	out := &deferredStatusWriter{w: w, status: status}
	if err := control.Encode(out, value); err != nil {
		deps.Metrics.RecordDeferredDeliveryError(route)
		logger.Warn("deferred control delivery failed", "route", route, "error", err)
		if !out.started {
			h.writeDeferredFailure(deps, w, route, deferredFailure(503, "deferred_saturated", "cannot encode deferred response", deferredAcceptance(route)))
		}
		return
	}
	deps.Metrics.RecordDeferredControl(route, "success")
}

func deferredQuery(r *http.Request, names ...string) (url.Values, error) {
	if len(r.URL.RawQuery) > deferred.MaxMetadataBytes || strings.Count(r.URL.RawQuery, "&") >= len(names) && r.URL.RawQuery != "" {
		return nil, fmt.Errorf("invalid deferred query")
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) > len(names) {
		return nil, fmt.Errorf("invalid deferred query")
	}
	for name, values := range query {
		if len(values) != 1 {
			return nil, fmt.Errorf("invalid deferred query")
		}
		found := false
		for _, allowed := range names {
			found = found || allowed == name
		}
		if !found {
			return nil, fmt.Errorf("invalid deferred query")
		}
	}
	return query, nil
}

func (h *Handler) handleDeferred(deps Dependencies, w http.ResponseWriter, original *http.Request, logger *slog.Logger, requestID string, start time.Time, releaseRuntime func()) (bool, bool) {
	if !isDeferredPath(original.URL.Path) {
		return false, false
	}
	deferredResponseHeaders(w)
	route, id, allow := deferredRoute(original.URL.Path, original.Method)
	fail := func(status int, code, message string) {
		h.writeDeferredFailure(deps, w, route, deferredFailure(status, code, message, deferredAcceptance(route)))
	}
	timeout := config.DefaultDeferred().ControlTimeout
	if deps.DeferredStore != nil {
		timeout = deps.DeferredStore.Options().ControlTimeout
	}
	deadline := time.Now().Add(timeout)
	ctx, cancel := context.WithDeadline(original.Context(), deadline)
	defer cancel()
	r := original.WithContext(ctx)
	controller := http.NewResponseController(w)
	for _, err := range []error{controller.SetReadDeadline(deadline), controller.SetWriteDeadline(deadline)} {
		if err != nil && !errors.Is(err, http.ErrNotSupported) {
			fail(503, "deferred_saturated", "cannot bound deferred control I/O")
			return true, false
		}
	}
	defer controller.SetReadDeadline(time.Time{})
	defer controller.SetWriteDeadline(time.Time{})
	if (original.URL.Path != deferred.Prefix && !strings.HasPrefix(original.URL.Path, deferred.Prefix+"/")) ||
		len(r.Header.Values("X-Aiproxy-Deferred-Version")) != 1 || r.Header.Get("X-Aiproxy-Deferred-Version") != "1" {
		fail(426, "unsupported_deferred_version", "unsupported deferred protocol version")
		return true, false
	}
	if route == "unknown" {
		fail(404, "not_found", "unknown deferred endpoint")
		return true, false
	}
	if r.Method != http.MethodGet && route != "submit" && route != "delete" || route == "submit" && r.Method != http.MethodPost {
		w.Header().Set("Allow", allow)
		fail(405, "method_not_allowed", "unsupported deferred method")
		return true, false
	}
	if route != "submit" && (r.ContentLength > 0 || len(r.TransferEncoding) > 0) {
		fail(400, "invalid_body", "deferred control requests have no body")
		return true, false
	}
	principal, err := h.authenticateInference(deps, r)
	if err != nil {
		if errors.Is(err, ErrServingStale) {
			fail(503, "serving_stale", ErrServingStale.Error())
			return true, false
		}
		fail(401, "auth_failed", "current inference bearer authentication is required")
		return true, false
	}
	owner, err := deferredOwner(principal)
	if err != nil {
		fail(401, "auth_failed", "deferred controls require an inference identity")
		return true, false
	}
	if deps.DeferredStore == nil {
		if id != "" {
			fail(404, "job_unavailable", "deferred job is unavailable")
		} else {
			fail(404, "deferred_disabled", "deferred admission is disabled")
		}
		return true, false
	}
	if route == "submit" {
		return true, h.submitDeferred(deps, w, r, principal, owner, logger, requestID, start, releaseRuntime)
	}
	control, err := deps.DeferredStore.AcquireControl(owner, route == "delete")
	if err != nil {
		h.writeDeferredControlError(deps, w, route, err)
		return true, false
	}
	defer control.Release()
	if route == "capabilities" {
		if !deps.DeferredStore.Options().Enabled {
			fail(404, "deferred_disabled", "deferred admission is disabled")
			return true, false
		}
		query, err := deferredQuery(r, "model")
		if err != nil || deferred.ValidatePublicModel(query.Get("model")) != nil {
			fail(400, "invalid_model", "one valid deferred model is required")
			return true, false
		}
		model := query.Get("model")
		resolved, denial := resolveInferenceModel(deps, principal, model)
		if denial != nil {
			fail(denial.status, denial.code, denial.message)
			return true, false
		}
		limits, err := deferredReceiverLimits(deps, resolved)
		if err != nil {
			fail(400, "invalid_request", err.Error())
			return true, false
		}
		capability, err := deps.DeferredStore.Capability(control, model, limits)
		if err != nil {
			h.writeDeferredControlError(deps, w, route, err)
		} else {
			h.writeDeferredValue(deps, w, control, route, 200, capability, logger)
		}
		return true, false
	}
	generation := r.Header.Get("X-Aiproxy-Deferred-Generation")
	if len(r.Header.Values("X-Aiproxy-Deferred-Generation")) != 1 {
		fail(400, "invalid_request", "one deferred generation is required")
		return true, false
	}
	model, err := deps.DeferredStore.Model(control, generation, id)
	if err != nil {
		h.writeDeferredControlError(deps, w, route, err)
		return true, false
	}
	if !deps.Authorizer.Allow(principal, model) {
		fail(403, "forbidden", modelForbiddenMessage)
		return true, false
	}
	logger = logger.With("job_id", id)
	queryNames := []string{}
	if route == "read" {
		queryNames = []string{"cursor", "limit"}
	}
	query, err := deferredQuery(r, queryNames...)
	if err != nil {
		fail(400, "invalid_cursor", "invalid deferred query")
		return true, false
	}
	switch route {
	case "status":
		info, err := deps.DeferredStore.Status(control, generation, id)
		if err != nil {
			h.writeDeferredControlError(deps, w, route, err)
		} else {
			h.writeDeferredValue(deps, w, control, route, 200, info, logger)
		}
	case "result":
		manifest, terminal, err := deps.DeferredStore.Result(control, generation, id)
		if err != nil {
			h.writeDeferredControlError(deps, w, route, err)
		} else if terminal {
			h.writeDeferredValue(deps, w, control, route, 200, manifest, logger)
		} else {
			h.writeDeferredValue(deps, w, control, route, 202, manifest.JobInfo, logger)
		}
	case "read":
		cursor, cursorErr := deferred.ParseCursor(query.Get("cursor"))
		limit, limitErr := deferred.ParseReadLimit(query.Get("limit"))
		if cursorErr != nil || limitErr != nil || query.Has("limit") && query.Get("limit") == "" {
			fail(400, "invalid_cursor", "invalid deferred read cursor or limit")
			break
		}
		frame, err := deps.DeferredStore.Read(control, generation, id, cursor, limit)
		if err != nil {
			h.writeDeferredControlError(deps, w, route, err)
		} else {
			h.writeDeferredValue(deps, w, control, route, 200, frame, logger)
		}
	case "delete":
		if err := deps.DeferredStore.Delete(control, generation, id); err != nil {
			h.writeDeferredControlError(deps, w, route, err)
		} else {
			deps.Metrics.RecordDeferredControl(route, "success")
			w.WriteHeader(http.StatusNoContent)
			if err := flushDeferredResponse(w); err != nil {
				deps.Metrics.RecordDeferredDeliveryError(route)
			}
		}
	}
	return true, false
}

func (h *Handler) writeDeferredControlError(deps Dependencies, w http.ResponseWriter, route string, err error) {
	var failure *deferred.ProtocolError
	if !errors.As(err, &failure) {
		failure = deferredFailure(503, "deferred_saturated", "deferred control failed", deferredAcceptance(route))
	}
	copy := *failure
	if route != "submit" && route != "capabilities" {
		copy.Wire.Acceptance = deferredAcceptance(route)
	}
	h.writeDeferredFailure(deps, w, route, &copy)
}

type admissionResponse struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *admissionResponse) Header() http.Header         { return w.header }
func (w *admissionResponse) WriteHeader(status int)      { w.status = status }
func (w *admissionResponse) Write(p []byte) (int, error) { return w.body.Write(p) }

func (w *admissionResponse) failure() *deferred.ProtocolError {
	var e apiError
	if json.Unmarshal(w.body.Bytes(), &e) != nil || e.Error.Type == "" {
		return deferredFailure(400, "invalid_request", "deferred admission failed", deferred.AcceptanceNotAccepted)
	}
	failure := deferredFailure(w.status, e.Error.Type, e.Error.Message, deferred.AcceptanceNotAccepted)
	failure.Wire.Error.BlockID = e.Error.BlockID
	if deferred.ValidateErrorDetail(failure.Wire.Error) != nil {
		failure.Wire.Error.Message = "deferred admission failed"
	}
	failure.Admission = true
	failure.RetryAfter = w.header.Get("Retry-After")
	return failure
}

func syntheticInferenceRequest(ctx context.Context, headers http.Header, body []byte) *http.Request {
	return (&http.Request{
		Method: http.MethodPost, URL: &url.URL{Path: "/v1/chat/completions"},
		Header: headers, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body)),
	}).WithContext(ctx)
}

func (h *Handler) submitDeferred(deps Dependencies, w http.ResponseWriter, r *http.Request, principal *auth.Principal, owner deferred.Owner, logger *slog.Logger, requestID string, start time.Time, releaseRuntime func()) bool {
	fail := func(err error) { h.writeDeferredControlError(deps, w, "submit", err) }
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != deferred.MediaType || r.URL.RawQuery != "" {
		fail(deferredFailure(400, "invalid_body", "deferred submit requires its JSON media type and no query", deferred.AcceptanceNotAccepted))
		return false
	}
	ingress, err := deps.DeferredStore.AcquireIngress(owner, r.ContentLength)
	if err != nil {
		fail(err)
		return false
	}
	defer ingress.Release()
	request, err := ingress.ReadSubmit(r.Body)
	if err != nil {
		fail(err)
		return false
	}
	model, err := deferred.ChatInputModel(request.Body)
	if err != nil {
		fail(deferredFailure(400, "invalid_body", err.Error(), deferred.AcceptanceNotAccepted))
		return false
	}
	preparation, replay, err := deps.DeferredStore.Begin(ingress, model, request)
	if err != nil {
		fail(err)
		return false
	}
	if preparation != nil {
		defer preparation.Abort()
	}
	if replay != nil {
		if !deps.Authorizer.Allow(principal, replay.PublicModel) {
			fail(deferredFailure(403, "forbidden", modelForbiddenMessage, deferred.AcceptanceAccepted))
			return false
		}
		w.Header().Set("Location", deferred.Prefix+"/jobs/"+replay.ID)
		deps.Metrics.RecordDeferredAdmission("replayed")
		h.writeDeferredValue(deps, w, ingress.Control, "submit", 202, *replay, logger)
		return false
	}
	input, releaseInput, err := preparation.Input()
	if err != nil {
		fail(err)
		return false
	}
	defer releaseInput()
	memory := &admissionMemory{budget: preparation.WorkBudget()}
	transferred := false
	defer func() {
		if !transferred {
			memory.close()
		}
	}()
	if err := memory.reserve(deferred.JobMetadataBytes); err != nil {
		fail(err)
		return false
	}
	inbound := syntheticInferenceRequest(r.Context(), http.Header(input.Headers), input.Body)
	response := &admissionResponse{header: make(http.Header)}
	body, allowed := h.readInferenceInput(deps, response, inbound, principal, input.Body, input.Limits.RequestBytes)
	limits := &provider.ExecutionLimits{RequestBytes: input.Limits.RequestBytes, ResponseBytes: input.Limits.ResultBytes, Work: preparation.WorkBudget()}
	admission := inferenceAdmission{}
	if allowed {
		admission, allowed = h.admitInferenceBody(deps, response, inbound, provider.OpChatCompletions, principal, body, logger, limits, memory)
	}
	if !allowed {
		failure := rejectDeferredPreparation(preparation, response.failure())
		fail(failure)
		return false
	}
	job, err := prepareDeferredExecution(deps, principal, admission, input, limits, memory, requestID, start)
	if err != nil {
		failure := deferredFailure(400, "deferred_limit_exceeded", "deferred preparation allowance exceeded", deferred.AcceptanceNotAccepted)
		fail(rejectDeferredPreparation(preparation, failure))
		return false
	}
	releaseInput()
	receipt, err := h.commitDeferred(r.Context(), deps, preparation, admission.Receiver, job.work(h), releaseRuntime)
	if err != nil {
		fail(err)
		return false
	}
	transferred = true
	w.Header().Set("Location", deferred.Prefix+"/jobs/"+receipt.ID)
	deps.Metrics.RecordDeferredAdmission("accepted")
	h.writeDeferredValue(deps, w, ingress.Control, "submit", 202, receipt, logger)
	return true
}

func rejectDeferredPreparation(preparation *deferred.Preparation, failure *deferred.ProtocolError) *deferred.ProtocolError {
	if err := preparation.Reject(failure); err != nil {
		var existing *deferred.ProtocolError
		if errors.As(err, &existing) {
			return existing
		}
	}
	return failure
}

func (h *Handler) commitDeferred(ctx context.Context, deps Dependencies, preparation *deferred.Preparation, receiver deferred.ReceiverLimits, work deferred.Work, releaseRuntime func()) (deferred.Receipt, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.deps.revision != deps.revision {
		failure := deferredFailure(503, "deferred_saturated", "deferred admission policy changed during preparation", deferred.AcceptanceNotAccepted)
		if h.deps.DeferredStore == nil || !h.deps.DeferredStore.Options().Enabled {
			failure = deferredFailure(404, "deferred_disabled", "deferred admission is disabled", deferred.AcceptanceNotAccepted)
		}
		return deferred.Receipt{}, rejectDeferredPreparation(preparation, failure)
	}
	if err := preparation.ConstrainReceiver(receiver); err != nil {
		return deferred.Receipt{}, err
	}
	if ctx.Err() != nil {
		failure := deferredFailure(503, "deferred_saturated", "deferred admission control request expired or was cancelled", deferred.AcceptanceNotAccepted)
		return deferred.Receipt{}, rejectDeferredPreparation(preparation, failure)
	}
	return preparation.Commit(work, releaseRuntime)
}
