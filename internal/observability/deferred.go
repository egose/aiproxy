package observability

import (
	"sync"

	"github.com/egose/aiproxy/internal/deferred"
	"github.com/prometheus/client_golang/prometheus"
)

type deferredMetrics struct {
	mu         sync.RWMutex
	source     func() deferred.Stats
	admissions *prometheus.CounterVec
	controls   *prometheus.CounterVec
	deliveries *prometheus.CounterVec
	jobs       *prometheus.Desc
	bytes      *prometheus.Desc
	ownership  *prometheus.Desc
	enabled    *prometheus.Desc
	cancelled  *prometheus.Desc
	expired    *prometheus.Desc
	finalizers *prometheus.Desc
}

func newDeferredMetrics() *deferredMetrics {
	return &deferredMetrics{
		admissions: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_deferred_admissions_total", Help: "Deferred admission decisions, excluding inference completion.",
		}, []string{"outcome"}),
		controls: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_deferred_controls_total", Help: "Deferred control requests by bounded route and outcome.",
		}, []string{"route", "outcome"}),
		deliveries: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "aiproxy_deferred_delivery_errors_total", Help: "Failed finite deferred control response transfers.",
		}, []string{"route"}),
		jobs:       prometheus.NewDesc("aiproxy_deferred_jobs", "Retained deferred jobs by state.", []string{"state"}, nil),
		bytes:      prometheus.NewDesc("aiproxy_deferred_bytes", "Deferred memory reservations by ownership category.", []string{"kind"}, nil),
		ownership:  prometheus.NewDesc("aiproxy_deferred_ownership", "Outstanding bounded deferred resource ownership.", []string{"kind"}, nil),
		enabled:    prometheus.NewDesc("aiproxy_deferred_enabled", "Whether new deferred admission is enabled.", nil, nil),
		cancelled:  prometheus.NewDesc("aiproxy_deferred_cancellations_total", "Deferred jobs sealed as cancelled.", nil, nil),
		expired:    prometheus.NewDesc("aiproxy_deferred_expirations_total", "Deferred retained results expired.", nil, nil),
		finalizers: prometheus.NewDesc("aiproxy_deferred_finalization_failures_total", "Failed or expired deferred finalization work.", nil, nil),
	}
}

func (d *deferredMetrics) Describe(ch chan<- *prometheus.Desc) {
	d.admissions.Describe(ch)
	d.controls.Describe(ch)
	d.deliveries.Describe(ch)
	for _, desc := range []*prometheus.Desc{d.jobs, d.bytes, d.ownership, d.enabled, d.cancelled, d.expired, d.finalizers} {
		ch <- desc
	}
}

func (d *deferredMetrics) Collect(ch chan<- prometheus.Metric) {
	d.admissions.Collect(ch)
	d.controls.Collect(ch)
	d.deliveries.Collect(ch)
	d.mu.RLock()
	source := d.source
	d.mu.RUnlock()
	if source == nil {
		return
	}
	s := source()
	enabled := 0.0
	if s.Enabled {
		enabled = 1
	}
	ch <- prometheus.MustNewConstMetric(d.enabled, prometheus.GaugeValue, enabled)
	for _, item := range []struct {
		name  string
		value int
	}{
		{"running", s.RunningJobs}, {"headers_ready", s.HeadersReadyJobs}, {"completed", s.CompletedJobs},
		{"failed", s.FailedJobs}, {"cancelled", s.CancelledJobs},
	} {
		ch <- prometheus.MustNewConstMetric(d.jobs, prometheus.GaugeValue, float64(item.value), item.name)
	}
	for _, item := range []struct {
		name  string
		value int64
	}{
		{"fixed", s.FixedBytes}, {"input", s.InputBytes}, {"output", s.OutputBytes}, {"work", s.WorkBytes},
		{"ingress", s.IngressExtraBytes}, {"reserved", s.ReservedBytes},
		{"retained", s.AvailableBytes},
	} {
		ch <- prometheus.MustNewConstMetric(d.bytes, prometheus.GaugeValue, float64(item.value), item.name)
	}
	for _, item := range []struct {
		name  string
		value int
	}{
		{"jobs", s.Jobs}, {"running", s.Running}, {"admissions", s.Admissions}, {"entries", s.Entries},
		{"owners", s.Owners}, {"controls", s.Controls}, {"deletes", s.Deletes},
		{"finalization_credits", s.FinalizationCredits}, {"finalizing", s.Finalizing},
	} {
		ch <- prometheus.MustNewConstMetric(d.ownership, prometheus.GaugeValue, float64(item.value), item.name)
	}
	ch <- prometheus.MustNewConstMetric(d.cancelled, prometheus.CounterValue, float64(s.Cancellations))
	ch <- prometheus.MustNewConstMetric(d.expired, prometheus.CounterValue, float64(s.Expirations))
	ch <- prometheus.MustNewConstMetric(d.finalizers, prometheus.CounterValue, float64(s.FinalizationFailures))
}

func (m *Metrics) SetDeferredSource(source func() deferred.Stats) {
	if m == nil {
		return
	}
	m.deferred.mu.Lock()
	m.deferred.source = source
	m.deferred.mu.Unlock()
}

func deferredMetricOutcome(outcome string) string {
	switch outcome {
	case "accepted", "replayed", "success", "auth_failed", "forbidden", "rate_limited", "quota_unavailable",
		"budget_exceeded", "tpm_exceeded", "secret_blocked", "scan_incomplete", "attempt_conflict",
		"submission_in_progress", "deferred_control_limited", "deferred_owner_limit", "deferred_saturated",
		"deferred_disabled", "unsupported_deferred_version", "generation_mismatch", "submit_scope_expired",
		"job_unavailable", "job_released", "job_expired", "invalid_cursor", "request_too_large",
		"deferred_limit_exceeded", "unsupported_operation", "method_not_allowed", "not_found":
		return outcome
	default:
		return "rejected"
	}
}

func deferredMetricRoute(route string) string {
	switch route {
	case "capabilities", "submit", "status", "result", "read", "delete":
		return route
	default:
		return "unknown"
	}
}

func (m *Metrics) RecordDeferredAdmission(outcome string) {
	if m != nil {
		m.deferred.admissions.WithLabelValues(deferredMetricOutcome(outcome)).Inc()
	}
}

func (m *Metrics) RecordDeferredControl(route, outcome string) {
	if m != nil {
		m.deferred.controls.WithLabelValues(deferredMetricRoute(route), deferredMetricOutcome(outcome)).Inc()
	}
}

func (m *Metrics) RecordDeferredDeliveryError(route string) {
	if m != nil {
		m.deferred.deliveries.WithLabelValues(deferredMetricRoute(route)).Inc()
	}
}
