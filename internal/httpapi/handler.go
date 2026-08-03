package httpapi

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/ratelimit"
)

type Dependencies struct {
	Resolver     *modelresolver.Resolver
	Adapter      provider.Adapter
	Auth         auth.Authenticator
	Authorizer   auth.Authorizer
	Client       *http.Client
	Catalog      []ModelCard
	Metrics      *observability.Metrics
	Providers    map[string]config.Provider
	Health       *providerhealth.Tracker
	RateLimiter  ratelimit.Limiter
	Accounting   accounting.Recorder
	Usage        accounting.Reader
	AccessLog    bool
	HasAccessLog bool
	Logger       *slog.Logger
	Dashboard    config.Dashboard
	Logs         *observability.LogBuffer

	// Dashboard surface — only meaningful when Dashboard.Token is set.
	DashboardVersion   string
	DashboardAddress   string
	DashboardAuthMode  string
	DashboardStartTime time.Time
	DashboardProviders []config.Provider
	DashboardDisabled  []config.Provider
	DashboardAliases   []config.Alias
}

const maxRequestBodyBytes int64 = 8 << 20

const (
	accountingModelInvalidBody = "_invalid_model"
	accountingModelForbidden   = "_forbidden_model"
	accountingModelNotFound    = "_unresolved_model"
)

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
	if deps.Authorizer == nil {
		deps.Authorizer = auth.AuthorizerFunc(func(*auth.Principal, string) bool { return true })
	}
	if deps.RateLimiter == nil {
		deps.RateLimiter = ratelimit.New(config.Auth{})
	}
	if deps.Accounting == nil {
		if deps.Metrics != nil {
			deps.Accounting = deps.Metrics
		} else {
			deps.Accounting = accounting.NewNoop()
		}
	}
	if !deps.HasAccessLog {
		deps.AccessLog = true
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
	requestID := observability.RequestID(r.Header.Get("X-Request-Id"))
	rw.Header().Set("X-Request-Id", requestID)
	logger := deps.Logger.With("request_id", requestID)
	requestBytes := 0
	var principal *auth.Principal
	accountingModel := ""
	var publicModel string
	var op provider.Operation
	opKnown := false
	responseStreaming := false
	var result *provider.Result
	defer func() {
		if opKnown {
			usage := provider.Usage{}
			if result != nil {
				usage = result.Usage
			}
			deps.Accounting.Record(accounting.Event{
				Timestamp:        time.Now(),
				Tenant:           principalTenant(principal),
				Client:           principalName(principal),
				Model:            accountingModel,
				Operation:        op.String(),
				StatusCode:       rw.statusCode,
				PromptTokens:     usage.PromptTokens,
				CompletionTokens: usage.CompletionTokens,
				TotalTokens:      usage.TotalTokens,
				Duration:         time.Since(start),
			})
		}
	}()
	defer func() {
		if deps.AccessLog {
			logAttrs := []any{
				"status", rw.statusCode,
				"duration_ms", time.Since(start).Milliseconds(),
				"response_bytes", rw.bytesWritten,
			}
			if opKnown {
				logAttrs = append(logAttrs, "operation", op.String())
			}
			if publicModel != "" {
				logAttrs = append(logAttrs, "public_model", publicModel)
			}
			if principal != nil {
				if principal.Name != "" {
					logAttrs = append(logAttrs, "client", principal.Name)
				}
				if principal.Tenant != "" {
					logAttrs = append(logAttrs, "tenant", principal.Tenant)
				}
			}
			logPath := r.URL.Path
			isDashboard := strings.HasPrefix(logPath, "/_internal/dashboard/")
			if responseStreaming {
				if isDashboard {
					logger.Debug("response stream finished", logAttrs...)
				} else {
					logger.Info("response stream finished", logAttrs...)
				}
			} else {
				if isDashboard {
					logger.Debug("response sent", logAttrs...)
				} else {
					logger.Info("response sent", logAttrs...)
				}
			}
		}
		if deps.Metrics != nil {
			path := metricsPathLabel(r)
			deps.Metrics.RecordHTTP(r.Method, path, rw.statusCode, time.Since(start).Seconds())
			deps.Metrics.RecordHTTPSize(r.Method, path, rw.statusCode, requestBytes, rw.bytesWritten)
		}
	}()

	if h.handleHealth(deps, rw, r) {
		return
	}
	if h.handleMetrics(deps, rw, r) {
		return
	}
	if h.handleBilling(deps, rw, r, logger) {
		return
	}
	if h.handleDashboard(deps, rw, r) {
		return
	}
	if h.handleModels(deps, rw, r, logger) {
		return
	}

	var ok bool
	op, ok = operationFromRequest(r)
	if !ok {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusNotFound, "not_found", "unknown endpoint")
		return
	}
	opKnown = true
	logger = logger.With("operation", op.String())

	var err error
	principal, err = deps.Auth.Authenticate(r)
	if err != nil {
		logger.Warn("auth failed", "error", err)
		h.writeRequestError(deps.Metrics, rw, r, http.StatusUnauthorized, "auth_failed", err.Error())
		return
	}
	if !h.allowRequest(deps, rw, r, principal) {
		return
	}

	r.Body = http.MaxBytesReader(rw, r.Body, maxRequestBodyBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadRequest, "invalid_body", err.Error())
		return
	}
	requestBytes = len(body)
	publicModel = extractModelForRequest(r.Header.Get("Content-Type"), body)
	if publicModel == "" {
		accountingModel = accountingModelInvalidBody
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadRequest, "invalid_model", "model field is required")
		return
	}
	logger = logger.With("public_model", publicModel)
	if principal != nil {
		logger = logger.With("client", principalName(principal), "tenant", principalTenant(principal))
	}
	if deps.AccessLog {
		logger.Info("request received", "method", r.Method, "path", r.URL.Path, "request_bytes", requestBytes)
	}
	if !deps.Authorizer.Allow(principal, publicModel) {
		accountingModel = accountingModelForbidden
		h.writeRequestError(deps.Metrics, rw, r, http.StatusForbidden, "forbidden", "client is not allowed to access this model")
		return
	}

	resolved, err := deps.Resolver.Resolve(publicModel)
	if err != nil {
		accountingModel = accountingModelNotFound
		logger.Info("resolve failed", "error", err)
		h.writeRequestError(deps.Metrics, rw, r, http.StatusNotFound, "model_not_found", err.Error())
		return
	}
	accountingModel = publicModel
	if err := ensureOperationSupported(op, resolved, deps.Providers); err != nil {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadRequest, "unsupported_operation", err.Error())
		return
	}

	if resolved.Kind == modelresolver.KindDirect {
		result, err = h.dispatchDirect(deps, r.Context(), op, resolved, r, body, logger)
	} else {
		result, err = h.dispatchAlias(deps, r.Context(), op, resolved, r, body, logger)
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
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadGateway, "upstream_error", "upstream request failed")
		return
	}
	if result == nil {
		h.writeRequestError(deps.Metrics, rw, r, http.StatusBadGateway, "upstream_error", "no healthy target")
		return
	}

	if result.Streaming {
		responseStreaming = true
		streamStart := time.Now()
		if deps.AccessLog {
			logger.Info("response stream started", "status", result.StatusCode)
		}
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
	case "/healthz", "/readyz", "/metrics", "/v1/models", "/v1/billing/usage":
		return r.URL.Path
	}
	if strings.HasPrefix(r.URL.Path, "/_internal/dashboard") {
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
				deps.Metrics.SetReadyWithReason(false, "no_active_providers")
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return true
		}
		if deps.Health != nil && !deps.Health.AnyHealthyContext(r.Context(), deps.Providers) {
			if deps.Metrics != nil {
				deps.Metrics.SetReadyWithReason(false, "no_healthy_providers")
			}
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte("not ready"))
			return true
		}
		if deps.Metrics != nil {
			deps.Metrics.SetReadyWithReason(true, "active_providers")
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
	principal, err := deps.Auth.Authenticate(r)
	if err != nil {
		logger.Warn("auth failed", "error", err)
		h.writeRequestError(deps.Metrics, w, r, http.StatusUnauthorized, "auth_failed", err.Error())
		return true
	}
	if !h.allowRequest(deps, w, r, principal) {
		return true
	}
	h.writeModels(w, filterModelCatalog(deps.Catalog, deps.Authorizer, principal))
	return true
}

func (h *Handler) handleBilling(deps Dependencies, w http.ResponseWriter, r *http.Request, logger *slog.Logger) bool {
	if r.URL.Path != "/v1/billing/usage" || r.Method != http.MethodGet {
		return false
	}
	principal, err := deps.Auth.Authenticate(r)
	if err != nil {
		logger.Warn("auth failed", "error", err)
		h.writeRequestError(deps.Metrics, w, r, http.StatusUnauthorized, "auth_failed", err.Error())
		return true
	}
	if !h.allowRequest(deps, w, r, principal) {
		return true
	}
	if deps.Usage == nil {
		h.writeRequestError(deps.Metrics, w, r, http.StatusNotFound, "not_found", "billing usage not configured")
		return true
	}
	summaries := accounting.FilterSummaries(deps.Usage.Summaries(), principalTenant(principal), principalName(principal))
	h.writeBillingUsage(w, summaries)
	return true
}

func (h *Handler) allowRequest(deps Dependencies, w http.ResponseWriter, r *http.Request, principal *auth.Principal) bool {
	if deps.RateLimiter == nil {
		return true
	}
	allowed, retryAfter := deps.RateLimiter.Allow(principalRateLimitKey(principal))
	if allowed {
		return true
	}
	if retryAfter > 0 {
		seconds := int(retryAfter / time.Second)
		if retryAfter%time.Second != 0 {
			seconds++
		}
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	}
	h.writeRequestError(deps.Metrics, w, r, http.StatusTooManyRequests, "rate_limited", "rate limit exceeded")
	return false
}

func principalRateLimitKey(principal *auth.Principal) string {
	if principal == nil || principal.Name == "" {
		return "anonymous"
	}
	return principal.Name
}

func principalName(principal *auth.Principal) string {
	if principal == nil {
		return ""
	}
	return principal.Name
}

func principalTenant(principal *auth.Principal) string {
	if principal == nil {
		return ""
	}
	return principal.Tenant
}

func extractModelForRequest(contentType string, body []byte) string {
	if strings.HasPrefix(contentType, "multipart/form-data;") {
		boundary := multipartContentBoundary(contentType)
		if boundary == "" {
			return ""
		}
		reader := multipart.NewReader(bytes.NewReader(body), boundary)
		for {
			part, err := reader.NextPart()
			if err == io.EOF {
				return ""
			}
			if err != nil {
				return ""
			}
			if part.FormName() != "model" {
				part.Close()
				continue
			}
			data, err := io.ReadAll(part)
			part.Close()
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(data))
		}
	}
	return extractModel(body)
}

func multipartContentBoundary(contentType string) string {
	parts := strings.Split(contentType, ";")
	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, "boundary=") {
			return strings.Trim(strings.TrimPrefix(part, "boundary="), `"`)
		}
	}
	return ""
}

func extractModel(body []byte) string {
	var probe struct {
		Model string `json:"model"`
	}
	_ = json.Unmarshal(body, &probe)
	return probe.Model
}

func filterModelCatalog(catalog []ModelCard, authorizer auth.Authorizer, principal *auth.Principal) []ModelCard {
	if authorizer == nil {
		out := make([]ModelCard, len(catalog))
		copy(out, catalog)
		return out
	}
	out := make([]ModelCard, 0, len(catalog))
	for _, card := range catalog {
		if authorizer.Allow(principal, card.ID) {
			out = append(out, card)
		}
	}
	return out
}
