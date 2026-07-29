package observability

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry
	handler  http.Handler

	httpRequests       *prometheus.CounterVec
	httpLatency        *prometheus.HistogramVec
	httpRequestBytes   *prometheus.HistogramVec
	httpResponseBytes  *prometheus.HistogramVec
	httpStreams        *prometheus.CounterVec
	httpStreamLatency  *prometheus.HistogramVec
	httpErrors         *prometheus.CounterVec
	usageEvents        *prometheus.CounterVec
	providerSelections *prometheus.CounterVec
	aliasRetries       *prometheus.CounterVec
	aliasInflight      *prometheus.GaugeVec
	upstreamRequests   *prometheus.CounterVec
	upstreamLatency    *prometheus.HistogramVec
	upstreamRespBytes  *prometheus.HistogramVec
	providerHealthy    *prometheus.GaugeVec
	skippedProviders   *prometheus.GaugeVec
	buildInfo          *prometheus.GaugeVec
	authModeInfo       *prometheus.GaugeVec
	providersByType    *prometheus.GaugeVec
	aliasesByAlgorithm *prometheus.GaugeVec
	providerCount      prometheus.Gauge
	disabledCount      prometheus.Gauge
	aliasCount         prometheus.Gauge
	readiness          prometheus.Gauge
	readyReason        *prometheus.GaugeVec
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		registry: registry,
		httpRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_http_requests_total",
			Help: "Total number of inbound HTTP requests by method, path, and status.",
		}, []string{"method", "path", "status"}),
		httpLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aiproxy_http_request_duration_seconds",
			Help:    "End-to-end latency of inbound HTTP requests by method, path, and status.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),
		httpRequestBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aiproxy_http_request_body_bytes",
			Help:    "Size of inbound HTTP request bodies by method and path.",
			Buckets: prometheus.ExponentialBuckets(64, 2, 12),
		}, []string{"method", "path"}),
		httpResponseBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aiproxy_http_response_body_bytes",
			Help:    "Size of outbound HTTP response bodies by method, path, and status.",
			Buckets: prometheus.ExponentialBuckets(64, 2, 12),
		}, []string{"method", "path", "status"}),
		httpStreams: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_http_stream_responses_total",
			Help: "Total number of streaming HTTP responses served by method, path, and status.",
		}, []string{"method", "path", "status"}),
		httpStreamLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aiproxy_http_stream_duration_seconds",
			Help:    "End-to-end duration of streaming HTTP responses by method, path, and status.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "path", "status"}),
		httpErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_http_errors_total",
			Help: "Total number of proxy-generated HTTP error responses by method, path, status, and error type.",
		}, []string{"method", "path", "status", "error_type"}),
		usageEvents: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_usage_events_total",
			Help: "Total number of request accounting events by tenant, client, model, operation, and status.",
		}, []string{"tenant", "client", "model", "operation", "status"}),
		providerSelections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_provider_selections_total",
			Help: "Total number of provider/model selections made by the proxy.",
		}, []string{"operation", "public_model", "provider", "model"}),
		aliasRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_alias_retries_total",
			Help: "Total number of alias retry events.",
		}, []string{"alias", "provider", "model", "reason"}),
		aliasInflight: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_alias_inflight_requests",
			Help: "Current number of in-flight alias requests by alias target.",
		}, []string{"alias", "provider", "model"}),
		upstreamRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_upstream_requests_total",
			Help: "Total number of upstream provider requests by outcome.",
		}, []string{"operation", "provider", "outcome"}),
		upstreamLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aiproxy_upstream_request_duration_seconds",
			Help:    "Latency of upstream provider requests by outcome.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation", "provider", "outcome"}),
		upstreamRespBytes: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aiproxy_upstream_response_body_bytes",
			Help:    "Size of upstream provider response bodies by operation, provider, and outcome.",
			Buckets: prometheus.ExponentialBuckets(64, 2, 12),
		}, []string{"operation", "provider", "outcome"}),
		providerHealthy: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_provider_healthy",
			Help: "Whether a provider is currently considered healthy for shared routing state (1=yes, 0=no).",
		}, []string{"name"}),
		skippedProviders: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_skipped_provider_info",
			Help: "Static gauge for providers skipped during startup because they are not active.",
		}, []string{"name", "type"}),
		buildInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_build_info",
			Help: "Static gauge describing the running build version.",
		}, []string{"version"}),
		authModeInfo: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_auth_mode_info",
			Help: "Static gauge describing the configured inbound auth mode.",
		}, []string{"mode"}),
		providersByType: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_providers_by_type",
			Help: "Number of configured providers by provider type and runtime state.",
		}, []string{"type", "state"}),
		aliasesByAlgorithm: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_aliases_by_algorithm",
			Help: "Number of configured aliases by selection algorithm.",
		}, []string{"algorithm"}),
		providerCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "aiproxy_active_providers",
			Help: "Number of active providers in the runtime.",
		}),
		disabledCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "aiproxy_disabled_providers",
			Help: "Number of disabled providers in the runtime.",
		}),
		aliasCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "aiproxy_aliases",
			Help: "Number of configured aliases in the runtime.",
		}),
		readiness: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "aiproxy_ready",
			Help: "Whether the proxy is ready to serve traffic (1=yes, 0=no).",
		}),
		readyReason: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_ready_reason_info",
			Help: "Static gauge describing the current readiness reason.",
		}, []string{"reason"}),
	}
	registry.MustRegister(
		m.httpRequests,
		m.httpLatency,
		m.httpRequestBytes,
		m.httpResponseBytes,
		m.httpStreams,
		m.httpStreamLatency,
		m.httpErrors,
		m.usageEvents,
		m.providerSelections,
		m.aliasRetries,
		m.aliasInflight,
		m.upstreamRequests,
		m.upstreamLatency,
		m.upstreamRespBytes,
		m.providerHealthy,
		m.skippedProviders,
		m.buildInfo,
		m.authModeInfo,
		m.providersByType,
		m.aliasesByAlgorithm,
		m.providerCount,
		m.disabledCount,
		m.aliasCount,
		m.readiness,
		m.readyReason,
	)
	m.handler = promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
	return m
}

func (m *Metrics) Handler() http.Handler {
	if m == nil {
		return http.NotFoundHandler()
	}
	return m.handler
}

func (m *Metrics) SetBuildInfo(version string) {
	if m == nil {
		return
	}
	if version == "" {
		version = "dev"
	}
	m.buildInfo.WithLabelValues(version).Set(1)
}

func (m *Metrics) RecordConfig(rt *config.Runtime) {
	if m == nil || rt == nil {
		return
	}
	m.providerCount.Set(float64(len(rt.Providers)))
	m.disabledCount.Set(float64(len(rt.DisabledProviders)))
	m.aliasCount.Set(float64(len(rt.Aliases)))
	for _, mode := range []config.AuthMode{config.AuthModeNone, config.AuthModeBearerStatic} {
		value := 0.0
		if rt.Auth.Mode == mode {
			value = 1
		}
		m.authModeInfo.WithLabelValues(string(mode)).Set(value)
	}
	providerTypeCounts := make(map[string]float64)
	for _, providerType := range []config.ProviderType{
		config.ProviderTypeOpenAI,
		config.ProviderTypeOpenAICompatible,
		config.ProviderTypeAnthropic,
		config.ProviderTypeGemini,
	} {
		providerTypeCounts[string(providerType)+":active"] = 0
		providerTypeCounts[string(providerType)+":disabled"] = 0
	}
	for _, p := range rt.Providers {
		providerTypeCounts[string(p.Type)+":active"]++
		m.providerHealthy.WithLabelValues(p.Name).Set(1)
	}
	for _, p := range rt.DisabledProviders {
		providerTypeCounts[string(p.Type)+":disabled"]++
	}
	for key, count := range providerTypeCounts {
		parts := strings.SplitN(key, ":", 2)
		m.providersByType.WithLabelValues(parts[0], parts[1]).Set(count)
	}
	aliasAlgorithmCounts := make(map[string]float64)
	for _, algorithm := range []config.Algorithm{config.AlgorithmRoundRobin, config.AlgorithmLeastConnections} {
		aliasAlgorithmCounts[string(algorithm)] = 0
	}
	for _, alias := range rt.Aliases {
		aliasAlgorithmCounts[string(alias.Algorithm)]++
	}
	for algorithm, count := range aliasAlgorithmCounts {
		m.aliasesByAlgorithm.WithLabelValues(algorithm).Set(count)
	}
	for _, p := range rt.DisabledProviders {
		m.skippedProviders.WithLabelValues(p.Name, string(p.Type)).Set(1)
	}
	if len(rt.Providers) > 0 {
		m.SetReadyWithReason(true, "active_providers")
	} else {
		m.SetReadyWithReason(false, "no_active_providers")
	}
}

func (m *Metrics) SetProviderHealthy(name string, healthy bool) {
	if m == nil || name == "" {
		return
	}
	if healthy {
		m.providerHealthy.WithLabelValues(name).Set(1)
		return
	}
	m.providerHealthy.WithLabelValues(name).Set(0)
}

func (m *Metrics) RemoveProviderHealthy(name string) {
	if m == nil || name == "" {
		return
	}
	m.providerHealthy.DeleteLabelValues(name)
}

func (m *Metrics) RecordHTTP(method, path string, statusCode int, seconds float64) {
	if m == nil {
		return
	}
	status := strconv.Itoa(statusCode)
	m.httpRequests.WithLabelValues(method, path, status).Inc()
	m.httpLatency.WithLabelValues(method, path, status).Observe(seconds)
}

func (m *Metrics) RecordHTTPSize(method, path string, statusCode int, requestBytes, responseBytes int) {
	if m == nil {
		return
	}
	status := strconv.Itoa(statusCode)
	m.httpRequestBytes.WithLabelValues(method, path).Observe(float64(requestBytes))
	m.httpResponseBytes.WithLabelValues(method, path, status).Observe(float64(responseBytes))
}

func (m *Metrics) RecordHTTPStream(method, path string, statusCode int, seconds float64) {
	if m == nil {
		return
	}
	status := strconv.Itoa(statusCode)
	m.httpStreams.WithLabelValues(method, path, status).Inc()
	m.httpStreamLatency.WithLabelValues(method, path, status).Observe(seconds)
}

func (m *Metrics) RecordHTTPError(method, path string, statusCode int, errType string) {
	if m == nil {
		return
	}
	status := strconv.Itoa(statusCode)
	m.httpErrors.WithLabelValues(method, path, status, errType).Inc()
}

func (m *Metrics) Record(event accounting.Event) {
	if m == nil || event.Operation == "" {
		return
	}
	tenant := event.Tenant
	if tenant == "" {
		tenant = "anonymous"
	}
	client := event.Client
	if client == "" {
		client = "anonymous"
	}
	model := event.Model
	if model == "" {
		model = "unknown"
	}
	m.usageEvents.WithLabelValues(tenant, client, model, event.Operation, strconv.Itoa(event.StatusCode)).Inc()
}

func (m *Metrics) SetReady(ready bool) {
	if ready {
		m.SetReadyWithReason(true, "active_providers")
		return
	}
	m.SetReadyWithReason(false, "no_active_providers")
}

func (m *Metrics) SetReadyWithReason(ready bool, reason string) {
	if m == nil {
		return
	}
	for _, candidate := range []string{"active_providers", "no_active_providers", "no_healthy_providers"} {
		value := 0.0
		if candidate == reason {
			value = 1
		}
		m.readyReason.WithLabelValues(candidate).Set(value)
	}
	if ready {
		m.readiness.Set(1)
		return
	}
	m.readiness.Set(0)
}

func (m *Metrics) RecordProviderSelection(op provider.Operation, publicModel, providerName, modelName string) {
	if m == nil {
		return
	}
	m.providerSelections.WithLabelValues(op.String(), publicModel, providerName, modelName).Inc()
}

func (m *Metrics) RecordAliasRetry(aliasName, providerName, modelName, reason string) {
	if m == nil {
		return
	}
	m.aliasRetries.WithLabelValues(aliasName, providerName, modelName, reason).Inc()
}

func (m *Metrics) AddAliasInFlight(aliasName, providerName, modelName string, delta float64) {
	if m == nil {
		return
	}
	m.aliasInflight.WithLabelValues(aliasName, providerName, modelName).Add(delta)
}

func (m *Metrics) RecordUpstream(op provider.Operation, providerName string, statusCode int, err error, seconds float64) {
	if m == nil {
		return
	}
	outcome := outcomeFor(statusCode, err)
	opName := op.String()
	m.upstreamRequests.WithLabelValues(opName, providerName, outcome).Inc()
	m.upstreamLatency.WithLabelValues(opName, providerName, outcome).Observe(seconds)
}

func (m *Metrics) RecordUpstreamResponseSize(op provider.Operation, providerName string, statusCode int, err error, bytes int) {
	if m == nil {
		return
	}
	outcome := outcomeFor(statusCode, err)
	m.upstreamRespBytes.WithLabelValues(op.String(), providerName, outcome).Observe(float64(bytes))
}

func outcomeFor(statusCode int, err error) string {
	if err != nil {
		var invalid provider.ErrInvalidRequest
		if errors.As(err, &invalid) {
			return "invalid_request"
		}
		var unsupported provider.ErrUnsupportedOperation
		if errors.As(err, &unsupported) {
			return "unsupported_operation"
		}
		return "transport_error"
	}
	if statusCode == 0 {
		return "unknown"
	}
	if statusCode >= 500 {
		return "http_5xx"
	}
	if statusCode >= 400 {
		return "http_" + strconv.Itoa(statusCode)
	}
	return "success"
}
