package httpapi

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
)

type cooldownOutcome struct {
	status    int
	delay     time.Duration
	hasDelay  bool
	err       error
	streaming bool
	body      string
}

type cooldownScriptAdapter struct {
	mu     sync.Mutex
	calls  []string
	behave map[string]func(call int) cooldownOutcome
	counts map[string]int
}

func (a *cooldownScriptAdapter) Do(ctx context.Context, r provider.Request) (*provider.Result, error) {
	a.mu.Lock()
	a.calls = append(a.calls, r.APIKey)
	a.counts[r.APIKey]++
	call := a.counts[r.APIKey]
	fn := a.behave[r.APIKey]
	a.mu.Unlock()
	outcome := cooldownOutcome{status: http.StatusOK}
	if fn != nil {
		outcome = fn(call)
	}
	if outcome.err != nil {
		return nil, outcome.err
	}
	body := outcome.body
	if body == "" {
		body = `{"id":"chatcmpl_ok"}`
	}
	status := outcome.status
	if status == 0 {
		status = http.StatusOK
	}
	if outcome.streaming {
		return &provider.Result{
			StatusCode:    status,
			Header:        http.Header{"Content-Type": []string{"text/event-stream"}},
			Streaming:     true,
			StreamBody:    io.NopCloser(strings.NewReader("data: hello\n\ndata: [DONE]\n\n")),
			Stream:        provider.NewStreamCompletion(),
			RetryDelay:    outcome.delay,
			HasRetryDelay: outcome.hasDelay,
		}, nil
	}
	return &provider.Result{
		StatusCode:    status,
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          []byte(body),
		RetryDelay:    outcome.delay,
		HasRetryDelay: outcome.hasDelay,
	}, nil
}

func (a *cooldownScriptAdapter) snapshot() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, len(a.calls))
	copy(out, a.calls)
	return out
}

type cooldownFixture struct {
	rt       *config.Runtime
	resolver *modelresolver.Resolver
	adapter  *cooldownScriptAdapter
	metrics  *observability.Metrics
	health   *providerhealth.Tracker
	handler  http.Handler
	now      time.Time
	current  time.Time
}

func newCooldownFixture(t *testing.T, rt *config.Runtime, adapter *cooldownScriptAdapter, withHealth bool) *cooldownFixture {
	t.Helper()
	f := &cooldownFixture{rt: rt, adapter: adapter, metrics: observability.NewMetrics()}
	f.now = time.Now()
	f.current = f.now
	f.resolver = modelresolver.New(rt)
	f.resolver.Cooldowns().SetNowFunc(func() time.Time { return f.current })
	deps := Dependencies{
		Resolver:     f.resolver,
		Adapter:      adapter,
		Auth:         auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:      rt.Catalog,
		Metrics:      f.metrics,
		MetricsToken: metricsTestToken,
	}
	if withHealth {
		f.health = providerhealth.New(f.metrics, config.ProviderHealth{})
		f.health.SetProviders(rt.Catalog)
		deps.Health = f.health
	}
	if adapter.counts == nil {
		adapter.counts = make(map[string]int)
	}
	f.handler = NewHandler(deps)
	return f
}

func (f *cooldownFixture) advance(d time.Duration) {
	f.current = f.current.Add(d)
}

func (f *cooldownFixture) post(t *testing.T, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, path, bytes.NewReader([]byte(body)))
	f.handler.ServeHTTP(w, r)
	return w
}

func (f *cooldownFixture) observe(t *testing.T, alias, providerName string, delay time.Duration) {
	t.Helper()
	prov, model, ok := f.rt.Catalog.Model(providerName, "m")
	if !ok {
		t.Fatalf("model %s/m not found", providerName)
	}
	f.resolver.Cooldowns().Observe(modelresolver.CooldownFingerprintFor(alias, prov, model), delay)
}

func cooldownChatBody(model string) string {
	return `{"model":"` + model + `","messages":[]}`
}

func TestAliasCooldownSkipsCoolingTarget(t *testing.T) {
	for _, algorithm := range []config.Algorithm{config.AlgorithmRoundRobin, config.AlgorithmLeastConnections} {
		t.Run(string(algorithm), func(t *testing.T) {
			rt := twoProviderAliasRT(algorithm, []int{429})
			adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
				"key1": func(call int) cooldownOutcome {
					return cooldownOutcome{status: http.StatusTooManyRequests, delay: time.Minute, hasDelay: true}
				},
			}}
			f := newCooldownFixture(t, rt, adapter, false)
			w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
			if w.Code != http.StatusOK {
				t.Fatalf("req1 status = %d, body=%s", w.Code, w.Body.String())
			}
			if got := adapter.snapshot(); len(got) != 2 || got[0] != "key1" || got[1] != "key2" {
				t.Fatalf("req1 calls = %v, want key1 then key2", got)
			}
			w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
			if w.Code != http.StatusOK {
				t.Fatalf("req2 status = %d, body=%s", w.Code, w.Body.String())
			}
			if got := adapter.snapshot(); len(got) != 3 || got[2] != "key2" {
				t.Fatalf("calls = %v, want third call to key2 only", got)
			}
			body := collectMetrics(t, f.handler)
			if got := metricValue(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`); got != 0 {
				t.Fatalf("p1 inflight = %v, want 0", got)
			}
			if got := metricValue(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p2"}`); got != 0 {
				t.Fatalf("p2 inflight = %v, want 0", got)
			}
		})
	}
}

func TestAliasCooldownRemainingTime(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests, delay: 10 * time.Second, hasDelay: true}
		},
		"key2": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusOK, delay: 30 * time.Second, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("req1 status = %d, body=%s", w.Code, w.Body.String())
	}
	f.advance(4 * time.Second)
	before := len(adapter.snapshot())
	w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("req2 status = %d, body=%s", w.Code, w.Body.String())
	}
	if got := len(adapter.snapshot()); got != before {
		t.Fatalf("all-cooling request made %d upstream calls, want 0", got-before)
	}
	if got := w.Header().Get("Retry-After"); got != "6" {
		t.Fatalf("Retry-After = %q, want 6", got)
	}
	if got := w.Header().Get("Retry-After-Ms"); got != "6000" {
		t.Fatalf("retry-after-ms = %q, want 6000", got)
	}
	if !strings.Contains(w.Body.String(), "retry after 6000ms") || !strings.Contains(w.Body.String(), "upstream_rate_limited") {
		t.Fatalf("body = %s", w.Body.String())
	}
}

func TestAliasCooldownSubSecondRounding(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests, delay: 250 * time.Millisecond, hasDelay: true}
		},
		"key2": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusOK, delay: 300 * time.Millisecond, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("req1 status = %d, body=%s", w.Code, w.Body.String())
	}
	f.advance(100 * time.Millisecond)
	w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("req2 status = %d, body=%s", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Retry-After-Ms"); got != "150" {
		t.Fatalf("retry-after-ms = %q, want 150", got)
	}
	if got := w.Header().Get("Retry-After"); got != "1" {
		t.Fatalf("Retry-After = %q, want 1", got)
	}
}

func TestAliasCooldownTerminalRetryableBecomesSynthetic(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests, delay: time.Minute, hasDelay: true, body: `{"error":{"type":"busy"}}`}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, true)
	f.observe(t, "a", "p2", time.Minute)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "busy") {
		t.Fatalf("discarded body leaked: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "upstream_rate_limited") {
		t.Fatalf("body = %s", w.Body.String())
	}
	if got := adapter.snapshot(); len(got) != 1 || got[0] != "key1" {
		t.Fatalf("calls = %v, want single key1 call", got)
	}
	body := collectMetrics(t, f.handler)
	if strings.Contains(body, "aiproxy_alias_retries_total") {
		t.Fatalf("synthetic terminal recorded alias retry\n%s", body)
	}
	if !f.health.IsHealthy("p1") || !f.health.IsHealthy("p2") {
		t.Fatalf("cooldown mutated provider health")
	}
}

func TestAliasCooldownSuccessVerbatimThenSynthetic(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusOK, delay: time.Minute, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	f.resolver.Cooldowns().SetNowFunc(func() time.Time { return f.current })
	f.observe(t, "a", "p2", time.Minute)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("success status = %d, body=%s", w.Code, w.Body.String())
	}
	w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("follow-up status = %d, body=%s", w.Code, w.Body.String())
	}
	if got := len(adapter.snapshot()); got != 1 {
		t.Fatalf("calls = %d, want 1", got)
	}
}

func TestAliasCooldownNonRetryableVerbatim(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusBadRequest, delay: time.Minute, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	f.observe(t, "a", "p2", time.Minute)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("follow-up status = %d, body=%s", w.Code, w.Body.String())
	}
}

func TestAliasCooldownExpiredTargetCalledAgain(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			if call == 1 {
				return cooldownOutcome{status: http.StatusTooManyRequests, delay: 10 * time.Second, hasDelay: true}
			}
			return cooldownOutcome{status: http.StatusOK}
		},
		"key2": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusOK, delay: time.Minute, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	if w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a")); w.Code != http.StatusOK {
		t.Fatalf("req1 status = %d", w.Code)
	}
	f.advance(11 * time.Second)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("req2 status = %d, body=%s", w.Code, w.Body.String())
	}
	got := adapter.snapshot()
	if len(got) != 3 || got[2] != "key1" {
		t.Fatalf("calls = %v, want expired key1 called again", got)
	}
}

func TestAliasCooldownMixedHealthNotAllCooling(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmLeastConnections, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}}
	f := newCooldownFixture(t, rt, adapter, true)
	f.health.MarkFailure("p2")
	f.observe(t, "a", "p1", time.Minute)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, body=%s; want 502 exhaustion, not 429", w.Code, w.Body.String())
	}
	if len(adapter.snapshot()) != 0 {
		t.Fatalf("calls = %v, want none", adapter.snapshot())
	}
}

func TestAliasCooldownHeaderFreePoolFailsOver(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	for i := 0; i < 2; i++ {
		w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
		if w.Code != http.StatusOK {
			t.Fatalf("req%d status = %d", i+1, w.Code)
		}
	}
	if got := adapter.snapshot(); len(got) != 4 {
		t.Fatalf("calls = %v, want repeated failover without exclusion", got)
	}
}

func TestDirectIgnoresAndNeverPopulatesCooldown(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests, delay: time.Minute, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("alias req status = %d", w.Code)
	}
	before := len(adapter.snapshot())
	w = f.post(t, "/v1/chat/completions", `{"model":"p1/m","messages":[]}`)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("direct status = %d, body=%s", w.Code, w.Body.String())
	}
	if got := len(adapter.snapshot()); got != before+1 {
		t.Fatalf("direct request skipped cooling target")
	}
	directCalls := len(adapter.snapshot())
	w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("alias follow-up status = %d", w.Code)
	}
	got := adapter.snapshot()
	if len(got) != directCalls+1 || got[len(got)-1] != "key2" {
		t.Fatalf("direct advice polluted alias state: %v", got)
	}
}

func TestDirectNeverFailover(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusBadGateway}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	w := f.post(t, "/v1/chat/completions", `{"model":"p1/m","messages":[]}`)
	if w.Code != http.StatusBadGateway {
		t.Fatalf("direct status = %d", w.Code)
	}
	if got := adapter.snapshot(); len(got) != 1 {
		t.Fatalf("direct calls = %v, want exactly one", got)
	}
}

func TestAliasCooldownIsolationAcrossAliases(t *testing.T) {
	rt := &config.Runtime{Catalog: config.NewCatalog([]config.Provider{
		{Type: config.ProviderTypeOpenAI, Name: "p1", APIKey: "key1", BaseURL: "https://x", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
		{Type: config.ProviderTypeOpenAI, Name: "p2", APIKey: "key2", BaseURL: "https://x", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
		{Type: config.ProviderTypeOpenAI, Name: "p3", APIKey: "key3", BaseURL: "https://x", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
	}, nil, []config.Alias{
		{Name: "a", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{429}, Targets: []config.AliasTarget{{Provider: "p1", Model: "m"}, {Provider: "p2", Model: "m"}}},
		{Name: "b", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{429}, Targets: []config.AliasTarget{{Provider: "p1", Model: "m"}, {Provider: "p3", Model: "m"}}},
	})}
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests, delay: time.Minute, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	if w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a")); w.Code != http.StatusOK {
		t.Fatalf("alias a status = %d", w.Code)
	}
	if w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/b")); w.Code != http.StatusOK {
		t.Fatalf("alias b status = %d", w.Code)
	}
	got := adapter.snapshot()
	if len(got) != 4 || got[0] != "key1" || got[1] != "key2" || got[2] != "key1" || got[3] != "key3" {
		t.Fatalf("calls = %v, want alias b to still call p1", got)
	}
}

func TestAliasCooldownSharedAcrossOperations(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests, delay: time.Minute, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	if w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a")); w.Code != http.StatusOK {
		t.Fatalf("chat status = %d", w.Code)
	}
	w := f.post(t, "/v1/responses", `{"model":"alias/a","input":"hi"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("responses status = %d, body=%s", w.Code, w.Body.String())
	}
	got := adapter.snapshot()
	if len(got) != 3 || got[2] != "key2" {
		t.Fatalf("calls = %v, want responses to skip cooling p1", got)
	}
}

func TestAliasCooldownStreaming(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusOK, delay: time.Minute, hasDelay: true, streaming: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	w := f.post(t, "/v1/chat/completions", `{"model":"alias/a","stream":true,"messages":[]}`)
	if w.Code != http.StatusOK {
		t.Fatalf("stream status = %d, body=%s", w.Code, w.Body.String())
	}
	w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("follow-up status = %d", w.Code)
	}
	got := adapter.snapshot()
	if len(got) != 2 || got[0] != "key1" || got[1] != "key2" {
		t.Fatalf("calls = %v, want stream advice to exclude p1", got)
	}
}

func TestAliasCooldownTranslateErrorRecordsAdvice(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	translateErr := &provider.CooldownError{Err: errors.New("translate response: bad payload"), Delay: time.Minute, OK: true}
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{err: translateErr}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("req1 status = %d, body=%s", w.Code, w.Body.String())
	}
	w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("req2 status = %d", w.Code)
	}
	got := adapter.snapshot()
	if len(got) != 3 || got[2] != "key2" {
		t.Fatalf("calls = %v, want translate failure advice to exclude p1", got)
	}
}

func TestAliasCooldownCanceledRecordsNothing(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusOK, delay: time.Minute, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(cooldownChatBody("alias/a")))).WithContext(ctx)
	f.handler.ServeHTTP(w, r)
	for i := 0; i < 2; i++ {
		w = f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
		if w.Code != http.StatusOK {
			t.Fatalf("follow-up %d status = %d", i, w.Code)
		}
	}
	got := adapter.snapshot()
	if len(got) != 3 || got[1] != "key2" || got[2] != "key1" {
		t.Fatalf("calls = %v, want canceled advice discarded so p1 called again", got)
	}
	body := collectMetrics(t, f.handler)
	if got := metricValue(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`); got != 0 {
		t.Fatalf("p1 inflight = %v, want 0", got)
	}
}

func TestAliasCooldownSyntheticVisibleInHTTPMetrics(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}}
	f := newCooldownFixture(t, rt, adapter, true)
	f.observe(t, "a", "p1", time.Minute)
	f.observe(t, "a", "p2", time.Minute)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status = %d", w.Code)
	}
	if len(adapter.snapshot()) != 0 {
		t.Fatalf("calls = %v, want zero upstream calls", adapter.snapshot())
	}
	body := collectMetrics(t, f.handler)
	if !strings.Contains(body, "aiproxy_http_requests_total") {
		t.Fatalf("http requests metric missing\n%s", body)
	}
	if strings.Contains(body, `aiproxy_upstream_requests_total{operation="chat_completions",outcome=`) && strings.Contains(body, `provider="p1"`) {
		t.Fatalf("skipped target must have no upstream attribution\n%s", body)
	}
	if !f.health.IsHealthy("p1") || !f.health.IsHealthy("p2") {
		t.Fatalf("synthetic 429 mutated provider health")
	}
}

func TestAliasCooldownRetryableWithoutAdviceKeepsFailover(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{500, 502, 503, 504})
	calls := 0
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			calls++
			return cooldownOutcome{status: http.StatusBadGateway}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if calls != 1 {
		t.Fatalf("p1 calls = %d", calls)
	}
}

func TestAliasCooldownConcurrent(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmLeastConnections, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests, delay: 50 * time.Millisecond, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, true)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 10; j++ {
				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(cooldownChatBody("alias/a"))))
				f.handler.ServeHTTP(w, r)
				if w.Code != http.StatusOK && w.Code != http.StatusTooManyRequests {
					t.Errorf("status = %d", w.Code)
				}
			}
		}()
	}
	wg.Wait()
	body := collectMetrics(t, f.handler)
	for _, providerName := range []string{"p1", "p2"} {
		if got := metricValue(t, body, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="`+providerName+`"}`); got != 0 {
			t.Fatalf("%s inflight = %v, want 0", providerName, got)
		}
	}
}

func TestAliasCooldownReloadRetainsAcrossResolver(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, []int{429})
	adapter := &cooldownScriptAdapter{counts: map[string]int{}, behave: map[string]func(int) cooldownOutcome{
		"key1": func(call int) cooldownOutcome {
			return cooldownOutcome{status: http.StatusTooManyRequests, delay: time.Minute, hasDelay: true}
		},
	}}
	f := newCooldownFixture(t, rt, adapter, false)
	if w := f.post(t, "/v1/chat/completions", cooldownChatBody("alias/a")); w.Code != http.StatusOK {
		t.Fatalf("req1 status = %d", w.Code)
	}
	next := modelresolver.NewWithPrevious(rt, f.resolver)
	next.Cooldowns().SetNowFunc(func() time.Time { return f.current })
	reloaded := NewHandler(Dependencies{
		Resolver: next,
		Adapter:  adapter,
		Auth:     auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:  rt.Catalog,
		Metrics:  observability.NewMetrics(),
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(cooldownChatBody("alias/a"))))
	reloaded.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("reloaded status = %d", w.Code)
	}
	got := adapter.snapshot()
	if len(got) != 3 || got[2] != "key2" {
		t.Fatalf("calls = %v, want reloaded resolver to skip cooling p1", got)
	}
	f.observe(t, "a", "p2", time.Minute)
	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(cooldownChatBody("alias/a"))))
	reloaded.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("old-resolver write leaked into replacement: status = %d", w.Code)
	}
	got = adapter.snapshot()
	if got[len(got)-1] != "key2" {
		t.Fatalf("calls = %v, want replacement to still call p2", got)
	}
}
