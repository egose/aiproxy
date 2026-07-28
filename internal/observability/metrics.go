package observability

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	registry *prometheus.Registry
	handler  http.Handler

	providerSelections *prometheus.CounterVec
	aliasRetries       *prometheus.CounterVec
	upstreamRequests   *prometheus.CounterVec
	upstreamLatency    *prometheus.HistogramVec
	skippedProviders   *prometheus.GaugeVec
	providerCount      prometheus.Gauge
	disabledCount      prometheus.Gauge
	aliasCount         prometheus.Gauge
	readiness          prometheus.Gauge
}

func NewMetrics() *Metrics {
	registry := prometheus.NewRegistry()
	m := &Metrics{
		registry: registry,
		providerSelections: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_provider_selections_total",
			Help: "Total number of provider/model selections made by the proxy.",
		}, []string{"operation", "public_model", "provider", "model"}),
		aliasRetries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_alias_retries_total",
			Help: "Total number of alias retry events.",
		}, []string{"alias", "provider", "model", "reason"}),
		upstreamRequests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_upstream_requests_total",
			Help: "Total number of upstream provider requests by outcome.",
		}, []string{"operation", "provider", "outcome"}),
		upstreamLatency: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "aiproxy_upstream_request_duration_seconds",
			Help:    "Latency of upstream provider requests by outcome.",
			Buckets: prometheus.DefBuckets,
		}, []string{"operation", "provider", "outcome"}),
		skippedProviders: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "aiproxy_skipped_provider_info",
			Help: "Static gauge for providers skipped during startup because they are not active.",
		}, []string{"name", "type"}),
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
	}
	registry.MustRegister(
		m.providerSelections,
		m.aliasRetries,
		m.upstreamRequests,
		m.upstreamLatency,
		m.skippedProviders,
		m.providerCount,
		m.disabledCount,
		m.aliasCount,
		m.readiness,
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

func (m *Metrics) RecordConfig(rt *config.Runtime) {
	if m == nil || rt == nil {
		return
	}
	m.providerCount.Set(float64(len(rt.Providers)))
	m.disabledCount.Set(float64(len(rt.DisabledProviders)))
	m.aliasCount.Set(float64(len(rt.Aliases)))
	for _, p := range rt.DisabledProviders {
		m.skippedProviders.WithLabelValues(p.Name, string(p.Type)).Set(1)
	}
	m.SetReady(len(rt.Providers) > 0)
}

func (m *Metrics) SetReady(ready bool) {
	if m == nil {
		return
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

func (m *Metrics) RecordUpstream(op provider.Operation, providerName string, statusCode int, err error, seconds float64) {
	if m == nil {
		return
	}
	outcome := outcomeFor(statusCode, err)
	opName := op.String()
	m.upstreamRequests.WithLabelValues(opName, providerName, outcome).Inc()
	m.upstreamLatency.WithLabelValues(opName, providerName, outcome).Observe(seconds)
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
