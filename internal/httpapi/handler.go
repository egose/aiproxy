package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"sync"
	"time"

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

func NewHandler(deps Dependencies) *Handler {
	deps = normalizeDependencies(deps)
	return &Handler{deps: deps}
}

type Handler struct {
	mu   sync.RWMutex
	deps Dependencies
}

func normalizeDependencies(deps Dependencies) Dependencies {
	if deps.Logger == nil {
		deps.Logger = slog.Default()
	}
	if deps.Adapter == nil {
		deps.Adapter = provider.New()
	}
	return deps
}

func (h *Handler) UpdateDependencies(deps Dependencies) {
	deps = normalizeDependencies(deps)
	h.mu.Lock()
	h.deps = deps
	h.mu.Unlock()
}

func (h *Handler) current() Dependencies {
	h.mu.RLock()
	deps := h.deps
	h.mu.RUnlock()
	return deps
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	deps := h.current()
	rw := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
	start := time.Now()
	requestBytes := 0
	defer func() {
		if deps.Metrics != nil {
			path := metricsPathLabel(r)
			deps.Metrics.RecordHTTP(r.Method, path, rw.statusCode, time.Since(start).Seconds())
			deps.Metrics.RecordHTTPSize(r.Method, path, rw.statusCode, requestBytes, rw.bytesWritten)
		}
	}()

	requestID := observability.RequestID(r.Header.Get("X-Request-Id"))
	rw.Header().Set("X-Request-Id", requestID)
	logger := deps.Logger.With("request_id", requestID)

	if h.handleHealth(deps, rw, r) {
		return
	}
	if h.handleMetrics(deps, rw, r) {
		return
	}
	if h.handleModels(deps, rw, r, logger) {
		return
	}

	op, ok := operationFromRequest(r)
	if !ok {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusNotFound, "not_found", "unknown endpoint")
		return
	}

	if _, err := deps.Auth.Authenticate(r); err != nil {
		logger.Warn("auth failed", "error", err)
		h.writeRequestError(deps.Metrics, rw, r, http.StatusUnauthorized, "auth_failed", err.Error())
		return
	}

	r.Body = http.MaxBytesReader(rw, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	requestBytes = len(body)
	publicModel := extractModel(body)
	if publicModel == "" {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadRequest, "invalid_model", "model field is required")
		return
	}
	logger = logger.With("model", publicModel)

	resolved, err := deps.Resolver.Resolve(publicModel)
	if err != nil {
		logger.Info("resolve failed", "error", err)
		h.writeRequestError(deps.Metrics, rw, r, http.StatusNotFound, "model_not_found", err.Error())
		return
	}
	if err := ensureOperationSupported(op, resolved, deps.Providers); err != nil {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadRequest, "unsupported_operation", err.Error())
		return
	}

	r.Body = io.NopCloser(bytes.NewReader(body))

	var result *provider.Result
	if resolved.Kind == modelresolver.KindDirect {
		result, err = h.dispatchDirect(deps, r.Context(), op, resolved, r)
	} else {
		result, err = h.dispatchAlias(deps, r.Context(), op, resolved, r, logger)
	}
	if err != nil {
		logger.Error("upstream failed", "error", err)
		var unsupported provider.ErrUnsupportedOperation
		if errors.As(err, &unsupported) {
			h.writeRequestError(deps.Metrics, rw, r, http.StatusBadRequest, "unsupported_operation", err.Error())
			return
		}
		var invalid provider.ErrInvalidRequest
		if errors.As(err, &invalid) {
			h.writeRequestError(deps.Metrics, rw, r, http.StatusBadRequest, "invalid_request", err.Error())
			return
		}
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadGateway, "upstream_error", err.Error())
		return
	}
	if result == nil {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadGateway, "upstream_error", "no healthy target")
		return
	}

	if result.Streaming {
		streamStart := time.Now()
		h.writeResult(rw, result)
		if deps.Metrics != nil {
			deps.Metrics.RecordHTTPStream(r.Method, metricsPathLabel(r), rw.statusCode, time.Since(streamStart).Seconds())
		}
		return
	}
	h.writeResult(rw, result)
}

type statusRecorder struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int
}

func (w *statusRecorder) WriteHeader(statusCode int) {
	w.statusCode = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *statusRecorder) Write(p []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(p)
	w.bytesWritten += n
	return n, err
}

func (w *statusRecorder) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func metricsPathLabel(r *http.Request) string {
	switch r.URL.Path {
	case "/healthz", "/readyz", "/metrics", "/v1/models":
		return r.URL.Path
	}
	if _, ok := operationFromRequest(r); ok {
		return r.URL.Path
	}
	return "unknown"
}

func (h *Handler) handleHealth(deps Dependencies, w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path == "/healthz" {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return true
	}
	if r.URL.Path == "/readyz" {
		if len(deps.Providers) == 0 {
			if deps.Metrics != nil {
				deps.Metrics.SetReady(false)
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return true
		}
		if deps.Metrics != nil {
			deps.Metrics.SetReady(true)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return true
	}
	return false
}

func (h *Handler) handleMetrics(deps Dependencies, w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/metrics" || r.Method != http.MethodGet {
		return false
	}
	if deps.Metrics == nil {
		h.writeRequestError(nil, w, r, http.StatusNotFound, "not_found", "metrics not configured")
		return true
	}
	deps.Metrics.Handler().ServeHTTP(w, r)
	return true
}

func (h *Handler) handleModels(deps Dependencies, w http.ResponseWriter, r *http.Request, logger *slog.Logger) bool {
	if r.URL.Path != "/v1/models" || r.Method != http.MethodGet {
		return false
	}
	if _, err := deps.Auth.Authenticate(r); err != nil {
		logger.Warn("auth failed", "error", err)
		h.writeRequestError(deps.Metrics, w, r, http.StatusUnauthorized, "auth_failed", err.Error())
		return true
	}
	h.writeModels(w, deps.Catalog)
	return true
}

func extractModel(body []byte) string {
	var probe struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Model
}
