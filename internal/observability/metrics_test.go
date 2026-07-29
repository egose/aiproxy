package observability

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestMetricsAliasInFlightGauge(t *testing.T) {
	m := NewMetrics()
	m.AddAliasInFlight("pool", "openai", "gpt-4o-mini", 1)
	m.AddAliasInFlight("pool", "openai", "gpt-4o-mini", -1)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	want := `aiproxy_alias_inflight_requests{alias="pool",model="gpt-4o-mini",provider="openai"} 0`
	if !strings.Contains(body, want) {
		t.Fatalf("metrics output missing %q\n%s", want, body)
	}
}

func TestMetricsRecordConfigStartupState(t *testing.T) {
	m := NewMetrics()
	m.RecordConfig(&config.Runtime{
		Auth:              config.Auth{Mode: config.AuthModeBearerStatic},
		Providers:         []config.Provider{{Type: config.ProviderTypeOpenAI}, {Type: config.ProviderTypeGemini}},
		DisabledProviders: []config.Provider{{Type: config.ProviderTypeAnthropic}},
		Aliases:           []config.Alias{{Algorithm: config.AlgorithmRoundRobin}, {Algorithm: config.AlgorithmLeastConnections}},
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`aiproxy_auth_mode_info{mode="none"} 0`,
		`aiproxy_auth_mode_info{mode="bearer_static"} 1`,
		`aiproxy_ready_reason_info{reason="active_providers"} 1`,
		`aiproxy_ready_reason_info{reason="no_active_providers"} 0`,
		`aiproxy_providers_by_type{state="active",type="openai"} 1`,
		`aiproxy_providers_by_type{state="active",type="gemini"} 1`,
		`aiproxy_providers_by_type{state="disabled",type="anthropic"} 1`,
		`aiproxy_aliases_by_algorithm{algorithm="round_robin"} 1`,
		`aiproxy_aliases_by_algorithm{algorithm="least_connections"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestMetricsSetReadyFalseExportsReason(t *testing.T) {
	m := NewMetrics()
	m.SetReady(false)

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`aiproxy_ready 0`,
		`aiproxy_ready_reason_info{reason="active_providers"} 0`,
		`aiproxy_ready_reason_info{reason="no_active_providers"} 1`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("metrics output missing %q\n%s", want, body)
		}
	}
}

func TestMetricsBuildInfo(t *testing.T) {
	m := NewMetrics()
	m.SetBuildInfo("1.2.3")

	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/metrics", nil)
	m.Handler().ServeHTTP(w, r)

	if w.Code != 200 {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	want := `aiproxy_build_info{version="1.2.3"} 1`
	if !strings.Contains(body, want) {
		t.Fatalf("metrics output missing %q\n%s", want, body)
	}
}
