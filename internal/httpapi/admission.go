package httpapi

import (
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

type inferenceAdmission struct {
	Body            []byte
	PublicModel     string
	AccountingModel string
	ReasoningEffort string
	Resolved        modelresolver.ResolveResult
	GuardrailLog    *guardrailLogState
	Receiver        deferred.ReceiverLimits
	Logger          *slog.Logger
}

type admissionMemory struct {
	budget   deferred.ByteBudget
	releases []func()
}

func (m *admissionMemory) reserve(n int64) error {
	release, err := m.budget.Reserve(n)
	if err != nil {
		return err
	}
	m.releases = append(m.releases, release)
	return nil
}

func (m *admissionMemory) close() {
	if m == nil {
		return
	}
	for _, release := range m.releases {
		release()
	}
	m.releases = nil
}

func (h *Handler) authenticateInference(deps Dependencies, r *http.Request) (*auth.Principal, error) {
	if deps.Auth == nil {
		return nil, auth.ErrNoToken
	}
	return deps.Auth.Authenticate(r)
}

func (h *Handler) readInferenceInput(deps Dependencies, w http.ResponseWriter, r *http.Request, principal *auth.Principal, input []byte, limit int64) ([]byte, bool) {
	if !h.allowRequest(deps, w, r, principal) {
		return nil, false
	}
	if input != nil {
		if int64(len(input)) > limit {
			h.writeRequestError(deps.Metrics, w, r, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds admission limit")
			return nil, false
		}
		return input, true
	}
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			h.writeRequestError(deps.Metrics, w, r, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds 8 MiB limit")
		} else {
			h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, "invalid_body", err.Error())
		}
		return nil, false
	}
	return body, true
}

func (h *Handler) admitInferenceBody(deps Dependencies, w http.ResponseWriter, r *http.Request, op provider.Operation, principal *auth.Principal, body []byte, logger *slog.Logger, limits *provider.ExecutionLimits, memory *admissionMemory) (inferenceAdmission, bool) {
	admission := inferenceAdmission{Body: body, Logger: logger}
	if limits != nil {
		shape, _, err := deferred.InspectChatInput(body, limits.RequestBytes, deferred.JSONDepth, deferred.JSONTokens)
		if err != nil {
			admission.AccountingModel = accountingModelInvalidBody
			h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, "invalid_body", err.Error())
			return admission, false
		}
		if err := memory.reserve(int64(len(body))*8 + int64(shape.Tokens)*256 + 16<<10); err != nil {
			h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, "deferred_limit_exceeded", "deferred admission work allowance exceeded")
			return admission, false
		}
	}
	if limits != nil {
		admission.PublicModel, _ = deferred.ChatInputModel(body)
	} else {
		admission.PublicModel = extractModelForRequest(r.Header.Get("Content-Type"), body)
	}
	if admission.PublicModel == "" {
		admission.AccountingModel = accountingModelInvalidBody
		h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, "invalid_model", "model field is required")
		return admission, false
	}
	publicModel := admission.PublicModel
	logger = logger.With("public_model", publicModel)
	admission.ReasoningEffort = extractReasoningEffort(body)
	if admission.ReasoningEffort == "" && op == provider.OpMessages {
		admission.ReasoningEffort = extractMessagesEffort(body)
	}
	if admission.ReasoningEffort != "" {
		logger = logger.With("reasoning_effort", admission.ReasoningEffort)
	}
	if principal != nil {
		logger = logger.With("client", principalName(principal), "tenant", principalTenant(principal))
	}
	admission.Logger = logger
	if deps.AccessLog {
		logger.Info("request received", "method", r.Method, "path", r.URL.Path, "request_bytes", len(body))
	}
	if !deps.Authorizer.Allow(principal, publicModel) {
		admission.AccountingModel = accountingModelForbidden
		h.writeRequestError(deps.Metrics, w, r, http.StatusForbidden, "forbidden", "client is not allowed to access this model")
		return admission, false
	}
	resolved, err := deps.Resolver.Resolve(publicModel)
	if err != nil {
		admission.AccountingModel = accountingModelNotFound
		logger.Info("resolve failed", "error", err)
		h.writeRequestError(deps.Metrics, w, r, http.StatusNotFound, "model_not_found", err.Error())
		return admission, false
	}
	admission.Resolved = resolved
	admission.AccountingModel = publicModel
	if !h.allowQuota(deps, w, r, principal, publicModel) {
		return admission, false
	}
	if err := ensureOperationSupported(op, resolved, deps.Catalog); err != nil {
		h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, "unsupported_operation", err.Error())
		return admission, false
	}
	if limits != nil {
		receiver, err := deferredReceiverLimits(deps, resolved)
		if err != nil {
			h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, "invalid_request", err.Error())
			return admission, false
		}
		admission.Receiver = receiver
	}
	if deps.Guardrails != nil && guardrailCovered(op) {
		if limits != nil {
			shape, _ := deferred.InspectJSON(body, deferred.JSONDepth, deferred.JSONTokens)
			if err := memory.reserve(int64(len(body))*16 + int64(shape.Tokens)*512 + limits.RequestBytes*4 + 64<<10); err != nil {
				h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, "deferred_limit_exceeded", "deferred guardrail work allowance exceeded")
				return admission, false
			}
		}
		blocked, out, logState := h.checkGuardrails(deps, w, r, op, body, deps.Guardrails, publicModel, logger, deps.RedactGuardrailBodies, limits)
		admission.GuardrailLog, admission.Body = logState, out
		if blocked {
			return admission, false
		}
		if limits != nil {
			if _, err := deferred.ValidateChatInput(out, limits.RequestBytes); err != nil {
				h.writeRequestError(deps.Metrics, w, r, http.StatusBadRequest, "deferred_limit_exceeded", "deferred rewritten input exceeds limits")
				return admission, false
			}
		}
	}
	return admission, true
}
