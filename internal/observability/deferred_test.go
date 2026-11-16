package observability

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/egose/aiproxy/internal/deferred"
)

func TestDeferredMetricsBoundLabelsAndReportAllOwnershipAndTerminalCounters(t *testing.T) {
	m := NewMetrics()
	m.SetDeferredSource(func() deferred.Stats {
		return deferred.Stats{
			Enabled: true, Jobs: 5, Running: 2, Admissions: 1, Entries: 7, Owners: 2, Controls: 3, Deletes: 1,
			FinalizationCredits: 2, Finalizing: 2, FixedBytes: 100, InputBytes: 10, OutputBytes: 20,
			WorkBytes: 30, IngressExtraBytes: 40, ReservedBytes: 200, AvailableBytes: 12,
			RunningJobs: 1, HeadersReadyJobs: 1, CompletedJobs: 1, FailedJobs: 1, CancelledJobs: 1,
			Cancellations: 4, Expirations: 3, FinalizationFailures: 2,
		}
	})
	for index := range 100 {
		private := fmt.Sprintf("private-job-%d", index)
		m.RecordDeferredAdmission(private)
		m.RecordDeferredControl("/jobs/"+private, private)
		m.RecordDeferredDeliveryError(private)
	}
	m.RecordDeferredAdmission("accepted")
	m.RecordDeferredControl("read", "success")
	m.RecordDeferredControl("read", "deferred_control_limited")
	m.RecordDeferredDeliveryError("read")
	families, err := m.registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	for _, family := range families {
		name := family.GetName()
		if !strings.HasPrefix(name, "aiproxy_deferred_") {
			continue
		}
		counts[name] = len(family.Metric)
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if strings.Contains(label.GetValue(), "private-job-") || strings.Contains(label.GetValue(), "/jobs/") {
					t.Fatalf("request-controlled deferred metric label: %s %s", name, label.GetValue())
				}
			}
		}
	}
	for name, count := range map[string]int{
		"aiproxy_deferred_admissions_total": 2, "aiproxy_deferred_controls_total": 3,
		"aiproxy_deferred_delivery_errors_total": 2, "aiproxy_deferred_jobs": 5,
		"aiproxy_deferred_bytes": 7, "aiproxy_deferred_ownership": 9,
	} {
		if counts[name] != count {
			t.Fatalf("%s has %d series, want bounded %d", name, counts[name], count)
		}
	}
	w := httptest.NewRecorder()
	m.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	for _, expected := range []string{
		`aiproxy_deferred_admissions_total{outcome="rejected"} 100`,
		`aiproxy_deferred_controls_total{outcome="rejected",route="unknown"} 100`,
		`aiproxy_deferred_jobs{state="headers_ready"} 1`,
		`aiproxy_deferred_bytes{kind="reserved"} 200`,
		`aiproxy_deferred_bytes{kind="retained"} 12`,
		`aiproxy_deferred_ownership{kind="finalizing"} 2`,
		`aiproxy_deferred_cancellations_total 4`,
		`aiproxy_deferred_expirations_total 3`,
		`aiproxy_deferred_finalization_failures_total 2`,
	} {
		if !strings.Contains(w.Body.String(), expected) {
			t.Fatalf("missing deferred metric %s", expected)
		}
	}
}

func TestDeferredMetricSourcePublicationAndScrapesAreConcurrentSafe(t *testing.T) {
	m := NewMetrics()
	var group sync.WaitGroup
	for worker := range 4 {
		group.Add(1)
		go func() {
			defer group.Done()
			for range 20 {
				if worker%2 == 0 {
					m.SetDeferredSource(func() deferred.Stats { return deferred.Stats{Enabled: true} })
					m.RecordDeferredControl("read", "success")
				} else if _, err := m.registry.Gather(); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	group.Wait()
}
