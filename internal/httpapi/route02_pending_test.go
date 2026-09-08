package httpapi

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func route02GaugeZero(t *testing.T, body, name string) {
	t.Helper()
	if !strings.Contains(body, name) {
		return
	}
	if got := metricValue(t, body, name); got != 0 {
		t.Fatalf("%s = %v, want 0\n%s", name, got, body)
	}
}

func TestRoute02CoolingFallbackReturnsTerminalResponse(t *testing.T) {
	cases := []struct {
		name       string
		retryCodes []int
		outcome    cooldownOutcome
		wantStatus int
		wantBody   string
	}{
		{
			name:       "429 without advice",
			retryCodes: []int{429},
			outcome:    cooldownOutcome{status: http.StatusTooManyRequests, body: `{"error":"a-busy"}`},
			wantStatus: http.StatusTooManyRequests,
			wantBody:   "a-busy",
		},
		{
			name:       "429 zero advice",
			retryCodes: []int{429},
			outcome:    cooldownOutcome{status: http.StatusTooManyRequests, delay: 0, hasDelay: true, body: `{"error":"a-zero"}`},
			wantStatus: http.StatusTooManyRequests,
			wantBody:   "a-zero",
		},
		{
			name:       "retryable 5xx without advice",
			retryCodes: []int{500, 502, 503, 504},
			outcome:    cooldownOutcome{status: http.StatusBadGateway, body: `{"error":"a-bad-gateway"}`},
			wantStatus: http.StatusBadGateway,
			wantBody:   "a-bad-gateway",
		},
	}
	for _, algorithm := range []config.Algorithm{config.AlgorithmRoundRobin, config.AlgorithmLeastConnections} {
		for _, tc := range cases {
			t.Run(string(algorithm)+"/"+tc.name, func(t *testing.T) {
				rt := twoProviderAliasRT(algorithm, tc.retryCodes)
				outcome := tc.outcome
				adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
					"key1": func(call int) cooldownOutcome { return outcome },
				}}
				f := newCooldownFixture(t, rt, adapter, false)
				f.observe(t, "a", "p2", time.Minute)
				w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
				if w.Code != tc.wantStatus {
					t.Fatalf("status = %d, want %d body=%s", w.Code, tc.wantStatus, w.Body.String())
				}
				if !strings.Contains(w.Body.String(), tc.wantBody) {
					t.Fatalf("body = %s, want substring %q", w.Body.String(), tc.wantBody)
				}
				if got := adapter.snapshot(); len(got) != 1 || got[0] != "key1" {
					t.Fatalf("calls = %v, want single key1 call", got)
				}
				body := collectMetrics(t, f.handler)
				if strings.Contains(body, "aiproxy_alias_retries_total") {
					t.Fatalf("recorded alias retry without subsequent attempt\n%s", body)
				}
				route02GaugeZero(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`)
				route02GaugeZero(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p2"}`)
				w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
				if w.Code != tc.wantStatus {
					t.Fatalf("follow-up status = %d, want %d body=%s; lease or cooldown leaked", w.Code, tc.wantStatus, w.Body.String())
				}
				if got := adapter.snapshot(); len(got) != 2 || got[0] != "key1" || got[1] != "key1" {
					t.Fatalf("follow-up calls = %v, want second key1 call proving lease release", got)
				}
			})
		}
	}
}

func TestRoute02EligibleFallbackStillRetries(t *testing.T) {
	for _, algorithm := range []config.Algorithm{config.AlgorithmRoundRobin, config.AlgorithmLeastConnections} {
		t.Run(string(algorithm), func(t *testing.T) {
			rt := twoProviderAliasRT(algorithm, []int{429})
			adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
				"key1": func(call int) cooldownOutcome {
					return cooldownOutcome{status: http.StatusTooManyRequests, body: `{"error":"busy"}`}
				},
			}}
			f := newCooldownFixture(t, rt, adapter, false)
			w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
			if w.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%s; want failover to eligible p2", w.Code, w.Body.String())
			}
			if got := adapter.snapshot(); len(got) != 2 || got[0] != "key1" || got[1] != "key2" {
				t.Fatalf("calls = %v, want key1 then key2", got)
			}
			body := collectMetrics(t, f.handler)
			if !strings.Contains(body, `aiproxy_alias_retries_total{alias="a",model="m",provider="p1",reason="upstream_status"} 1`) {
				t.Fatalf("missing p1 upstream_status retry\n%s", body)
			}
			route02GaugeZero(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`)
			route02GaugeZero(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p2"}`)
		})
	}
}

func TestRoute02ValidAdviceCompletingAllCoolingIsSynthetic(t *testing.T) {
	for _, algorithm := range []config.Algorithm{config.AlgorithmRoundRobin, config.AlgorithmLeastConnections} {
		t.Run(string(algorithm), func(t *testing.T) {
			rt := twoProviderAliasRT(algorithm, []int{429})
			adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
				"key1": func(call int) cooldownOutcome {
					return cooldownOutcome{status: http.StatusTooManyRequests, delay: time.Minute, hasDelay: true, body: `{"error":"busy"}`}
				},
			}}
			f := newCooldownFixture(t, rt, adapter, false)
			f.observe(t, "a", "p2", time.Minute)
			w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
			if w.Code != http.StatusTooManyRequests {
				t.Fatalf("status = %d, body=%s; want synthetic 429", w.Code, w.Body.String())
			}
			if strings.Contains(w.Body.String(), "busy") {
				t.Fatalf("discarded body leaked: %s", w.Body.String())
			}
			if !strings.Contains(w.Body.String(), "upstream_rate_limited") {
				t.Fatalf("body = %s, want synthetic upstream_rate_limited", w.Body.String())
			}
			if got := w.Header().Get("Retry-After"); got == "" {
				t.Fatalf("missing Retry-After header")
			}
			if got := w.Header().Get("Retry-After-Ms"); got == "" {
				t.Fatalf("missing Retry-After-Ms header")
			}
			if got := adapter.snapshot(); len(got) != 1 || got[0] != "key1" {
				t.Fatalf("calls = %v, want single key1 call", got)
			}
			body := collectMetrics(t, f.handler)
			if strings.Contains(body, "aiproxy_alias_retries_total") {
				t.Fatalf("synthetic terminal recorded alias retry\n%s", body)
			}
			route02GaugeZero(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`)
			route02GaugeZero(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p2"}`)
		})
	}
}
