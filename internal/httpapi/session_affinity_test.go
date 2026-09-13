package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
)

func affinityTestRT(affinity *config.SessionAffinity) *config.Runtime {
	return &config.Runtime{Catalog: config.NewCatalog([]config.Provider{
		{Type: config.ProviderTypeOpenAI, Name: "p1", APIKey: "key1", BaseURL: "https://x", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
		{Type: config.ProviderTypeOpenAI, Name: "p2", APIKey: "key2", BaseURL: "https://x", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
	}, nil, []config.Alias{{
		Name:             "a",
		Algorithm:        config.AlgorithmRoundRobin,
		RetryStatusCodes: []int{500},
		SessionAffinity:  affinity,
		Targets:          []config.AliasTarget{{Provider: "p1", Model: "m"}, {Provider: "p2", Model: "m"}},
	}})}
}

func affinityTestHandler(t *testing.T, rt *config.Runtime, adapter *statusByAPIKeyAdapter) http.Handler {
	t.Helper()
	return NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  adapter,
		Auth:     auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:  rt.Catalog,
		Metrics:  observability.NewMetrics(),
	})
}

func affinityTestRequest(t *testing.T, h http.Handler, headers map[string]string) {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias/a","messages":[]}`))
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s headers=%v", w.Code, w.Body.String(), headers)
	}
}

func TestAliasSessionAffinityPinsSession(t *testing.T) {
	adapter := &statusByAPIKeyAdapter{}
	h := affinityTestHandler(t, affinityTestRT(&config.SessionAffinity{}), adapter)
	for i := 0; i < 10; i++ {
		affinityTestRequest(t, h, map[string]string{"x-opencode-session": "sticky-session-1"})
	}
	calls := adapter.Calls()
	if len(calls) != 10 {
		t.Fatalf("calls = %d, want 10", len(calls))
	}
	for _, c := range calls {
		if c != calls[0] {
			t.Fatalf("calls = %v, want all pinned to one provider", calls)
		}
	}
}

func TestAliasSessionAffinityDistributesAcrossSessions(t *testing.T) {
	adapter := &statusByAPIKeyAdapter{}
	h := affinityTestHandler(t, affinityTestRT(&config.SessionAffinity{}), adapter)
	sessions := []string{"s-alpha", "s-beta", "s-gamma", "s-delta", "s-epsilon", "s-zeta", "s-eta", "s-theta", "s-iota", "s-kappa"}
	for _, s := range sessions {
		affinityTestRequest(t, h, map[string]string{"x-opencode-session": s})
	}
	seen := map[string]bool{}
	for _, c := range adapter.Calls() {
		seen[c] = true
	}
	if len(seen) != 2 {
		t.Fatalf("calls spread across %d providers, want 2: %v", len(seen), adapter.Calls())
	}
}

func TestAliasSessionAffinityFallsBackWithoutHeader(t *testing.T) {
	adapter := &statusByAPIKeyAdapter{}
	h := affinityTestHandler(t, affinityTestRT(&config.SessionAffinity{}), adapter)
	affinityTestRequest(t, h, nil)
	affinityTestRequest(t, h, nil)
	calls := adapter.Calls()
	if len(calls) != 2 || calls[0] == calls[1] {
		t.Fatalf("calls without session = %v, want round-robin alternation", calls)
	}
}

func TestAliasSessionAffinityFailoverToHealthyTarget(t *testing.T) {
	adapter := &statusByAPIKeyAdapter{rules: map[string]int{}}
	h := affinityTestHandler(t, affinityTestRT(&config.SessionAffinity{}), adapter)
	affinityTestRequest(t, h, map[string]string{"x-opencode-session": "probe"})
	probed := adapter.Calls()[0]
	failing := map[string]int{probed: http.StatusInternalServerError}
	adapter2 := &statusByAPIKeyAdapter{rules: failing}
	h2 := affinityTestHandler(t, affinityTestRT(&config.SessionAffinity{}), adapter2)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias/a","messages":[]}`))
	r.Header.Set("x-opencode-session", "probe")
	h2.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want failover success", w.Code, w.Body.String())
	}
	calls := adapter2.Calls()
	if len(calls) != 2 {
		t.Fatalf("calls = %v, want preferred then failover target", calls)
	}
	if calls[0] != probed {
		t.Fatalf("first call = %q, want affinity-preferred %q", calls[0], probed)
	}
	if calls[1] == calls[0] {
		t.Fatalf("calls = %v, want failover to the other provider", calls)
	}
}

func TestAliasSessionAffinityCustomHeaders(t *testing.T) {
	adapter := &statusByAPIKeyAdapter{}
	rt := affinityTestRT(&config.SessionAffinity{Headers: []string{"x-custom-session"}})
	h := affinityTestHandler(t, rt, adapter)
	affinityTestRequest(t, h, map[string]string{"x-opencode-session": "ignored"})
	affinityTestRequest(t, h, map[string]string{"x-opencode-session": "ignored"})
	calls := adapter.Calls()
	if len(calls) != 2 || calls[0] == calls[1] {
		t.Fatalf("calls with non-listed header = %v, want no affinity (alternation)", calls)
	}
	for i := 0; i < 6; i++ {
		affinityTestRequest(t, h, map[string]string{"x-custom-session": "custom-1"})
	}
	calls = adapter.Calls()[2:]
	for _, c := range calls {
		if c != calls[0] {
			t.Fatalf("calls with custom header = %v, want pinning", adapter.Calls())
		}
	}
}
