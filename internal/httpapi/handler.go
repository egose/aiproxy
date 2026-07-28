package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

type Dependencies struct {
	Resolver  *modelresolver.Resolver
	Adapter   provider.Adapter
	Auth      auth.Authenticator
	Client    *http.Client
	Catalog   []ModelCard
	Metrics   *observability.Metrics
	Providers map[string]config.Provider
	Logger    *slog.Logger
}

const maxRequestBodyBytes int64 = 8 << 20

func NewHandler(deps Dependencies) http.Handler {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Adapter == nil {
		deps.Adapter = provider.New()
	}
	return &handler{deps: deps}
}

type handler struct {
	deps Dependencies
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := observability.RequestID(r.Header.Get("X-Request-Id"))
	w.Header().Set("X-Request-Id", requestID)
	logger := h.deps.Logger.With("request_id", requestID)

	if h.handleHealth(w, r) {
		return
	}
	if h.handleMetrics(w, r) {
		return
	}
	if h.handleModels(w, r, logger) {
		return
	}

	op, ok := operationFromRequest(r)
	if !ok {
		h.writeError(w, http.StatusNotFound, "not_found", "unknown endpoint")
		return
	}

	if _, err := h.deps.Auth.Authenticate(r); err != nil {
		logger.Warn("auth failed", "error", err)
		h.writeError(w, http.StatusUnauthorized, "auth_failed", err.Error())
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	publicModel := extractModel(body)
	if publicModel == "" {
		h.writeError(w, http.StatusBadRequest, "invalid_model", "model field is required")
		return
	}
	logger = logger.With("model", publicModel)

	resolved, err := h.deps.Resolver.Resolve(publicModel)
	if err != nil {
		logger.Info("resolve failed", "error", err)
		h.writeError(w, http.StatusNotFound, "model_not_found", err.Error())
		return
	}
	if err := ensureOperationSupported(op, resolved, h.deps.Providers); err != nil {
		h.writeError(w, http.StatusBadRequest, "unsupported_operation", err.Error())
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(body))

	var result *provider.Result
	if resolved.Kind == modelresolver.KindDirect {
		result, err = h.dispatchDirect(r.Context(), op, resolved, r)
	} else {
		result, err = h.dispatchAlias(r.Context(), op, resolved, r, logger)
	}
	if err != nil {
		logger.Error("upstream failed", "error", err)
		var unsupported provider.ErrUnsupportedOperation
		if errors.As(err, &unsupported) {
			h.writeError(w, http.StatusBadRequest, "unsupported_operation", err.Error())
			return
		}
		var invalid provider.ErrInvalidRequest
		if errors.As(err, &invalid) {
			h.writeError(w, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		h.writeError(w, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if result == nil {
		h.writeError(w, http.StatusBadGateway, "upstream_error", "no healthy target")
		return
	}

	h.writeResult(w, result)
}

func (h *handler) handleHealth(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return true
	}
	if r.URL.Path == "/readyz" {
		if len(h.deps.Providers) == 0 {
			if h.deps.Metrics != nil {
				h.deps.Metrics.SetReady(false)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return true
		}
		if h.deps.Metrics != nil {
			h.deps.Metrics.SetReady(true)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return true
	}
	return false
}

func (h *handler) handleMetrics(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/metrics" || r.Method != http.MethodGet {
		return false
	}
	if h.deps.Metrics == nil {
		h.writeError(w, http.StatusNotFound, "not_found", "metrics not configured")
		return true
	}
	h.deps.Metrics.Handler().ServeHTTP(w, r)
	return true
}

func (h *handler) handleModels(w http.ResponseWriter, r *http.Request, logger *slog.Logger) bool {
	if r.URL.Path != "/v1/models" || r.Method != http.MethodGet {
		return false
	}
	if _, err := h.deps.Auth.Authenticate(r); err != nil {
		logger.Warn("auth failed", "error", err)
		h.writeError(w, http.StatusUnauthorized, "auth_failed", err.Error())
		return true
	}
	h.writeModels(w)
	return true
}

func extractModel(body []byte) string {
	var probe struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Model
}
