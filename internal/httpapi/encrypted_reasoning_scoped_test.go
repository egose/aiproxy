package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

const scopedReasoningMismatch = `{"error":{"message":"reasoning encrypted_content was not issued to this caller"}}`

func scopedReasoningPolicy(mode string) *config.EncryptedReasoning {
	declaration := &config.EncryptedReasoningDeclaration{}
	switch mode {
	case "":
		return nil
	case "reset":
	case "strip", "strip-retry":
		value := false
		declaration.Passthrough = &value
	case "retry":
	default:
		panic("unknown reasoning fixture policy: " + mode)
	}
	if mode == "retry" || mode == "strip-retry" {
		value := config.EncryptedReasoningStripAndRetry
		declaration.OnCallerMismatch = &value
	}
	return config.NewEncryptedReasoning(declaration)
}

func scopedReasoningBodies(op provider.Operation, model string) ([]byte, []byte) {
	if op == provider.OpResponses {
		return []byte(`{ "model":"` + model + `","input":[{"type":"reasoning","encrypted_content":"blob","summary":[]},{"type":"message","role":"user","content":"hi"}],"reasoning":{"effort":"high"}}`),
			[]byte(`{"model":"` + model + `","input":[{"type":"message","role":"user","content":"hi"}],"reasoning":{"effort":"high"}}`)
	}
	return []byte(`{ "model":"` + model + `","messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"blob"},{"type":"text","text":"hi"}],"tool_calls":[{"id":"call-1","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call-1","content":"done"}],"reasoning_effort":"high"}`),
		[]byte(`{"model":"` + model + `","messages":[{"role":"assistant","content":[{"type":"text","text":"hi"}],"tool_calls":[{"id":"call-1","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call-1","content":"done"}],"reasoning_effort":"high"}`)
}

func assertScopedReasoningBody(t *testing.T, got, original, stripped []byte, wantStripped bool) {
	t.Helper()
	if !wantStripped {
		if !bytes.Equal(got, original) {
			t.Fatalf("intact body changed:\ngot  %s\nwant %s", got, original)
		}
		return
	}
	var actual, expected any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(stripped, &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("stripped body:\ngot  %s\nwant %s", got, stripped)
	}
}

type scopedReasoningAttempt struct {
	Context    context.Context
	Request    provider.Request
	Result     *provider.Result
	Closes     atomic.Int32
	BodyCloses atomic.Int32
}

type scopedReasoningProbe struct {
	t        *testing.T
	mu       sync.Mutex
	attempts []*scopedReasoningAttempt
	do       func(int, context.Context, provider.Request) (*provider.Result, error)
}

func (p *scopedReasoningProbe) Do(ctx context.Context, request provider.Request) (*provider.Result, error) {
	p.t.Helper()
	if request.Inbound == nil {
		p.t.Error("dispatch did not clone an inbound request")
	} else {
		body, err := io.ReadAll(request.Inbound.Body)
		if err != nil || !bytes.Equal(body, request.Body) || request.Inbound.ContentLength != int64(len(request.Body)) {
			p.t.Errorf("inbound/dispatch body mismatch: err=%v length=%d body=%s dispatch=%s", err, request.Inbound.ContentLength, body, request.Body)
		}
		if request.Inbound.Context() != ctx {
			p.t.Error("inbound context differs from the dispatch context")
		}
	}
	attempt := &scopedReasoningAttempt{Context: ctx, Request: request}
	attempt.Request.Body = bytes.Clone(request.Body)
	p.mu.Lock()
	call := len(p.attempts)
	p.attempts = append(p.attempts, attempt)
	p.mu.Unlock()
	result, err := p.do(call, ctx, request)
	if result != nil {
		existing := result.OnClose
		result.OnClose = func() {
			if existing != nil {
				existing()
			}
			attempt.Closes.Add(1)
		}
		if result.StreamBody != nil {
			result.StreamBody = &deferredDeliveryBody{ReadCloser: result.StreamBody, closes: &attempt.BodyCloses}
		}
	}
	p.mu.Lock()
	attempt.Result = result
	p.mu.Unlock()
	return result, err
}

func (p *scopedReasoningProbe) snapshot() []*scopedReasoningAttempt {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*scopedReasoningAttempt(nil), p.attempts...)
}

func scopedReasoningResult(status int) *provider.Result {
	body := fmt.Sprintf(`{"status":%d}`, status)
	if status == http.StatusBadRequest {
		body = scopedReasoningMismatch
	}
	return &provider.Result{
		StatusCode: status, Body: []byte(body),
		Header: http.Header{"Content-Type": {"application/json"}, "X-Upstream-Marker": {"original", "second"}},
		Usage:  provider.Usage{PromptTokens: 4, CompletionTokens: 6, TotalTokens: 10},
	}
}

func scopedReasoningRuntime(root, aliasPolicy string, providers, models []string) *config.Runtime {
	count := max(2, len(providers), len(models))
	var configured []config.Provider
	var targets []config.AliasTarget
	for i := 0; i < count; i++ {
		name := fmt.Sprintf("p%d", i+1)
		p := config.Provider{
			Type: config.ProviderTypeOpenAICompatible, Name: name,
			APIKey: "credential-" + name, CopilotToken: "copilot-" + name, BaseURL: "https://" + name + ".invalid/v1",
			UserAgent: "fixture-" + name, ForwardUserAgent: true, ForwardHeaders: []string{"X-Fixture"},
			Models: []config.Model{{Name: "m", UpstreamName: "upstream-" + name, Protocol: config.ModelProtocolChat}},
		}
		if i < len(providers) {
			p.EncryptedReasoning = scopedReasoningPolicy(providers[i])
		}
		if i < len(models) {
			p.Models[0].EncryptedReasoning = scopedReasoningPolicy(models[i])
		}
		configured = append(configured, p)
		targets = append(targets, config.AliasTarget{Provider: name, Model: "m"})
	}
	return &config.Runtime{
		EncryptedReasoning: scopedReasoningPolicy(root),
		Catalog: config.NewCatalog(configured, nil, []config.Alias{{
			Name: "a", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{503},
			Targets: targets, EncryptedReasoning: scopedReasoningPolicy(aliasPolicy),
		}}),
	}
}

func scopedReasoningHandler(t *testing.T, rt *config.Runtime, probe *scopedReasoningProbe) (*Handler, *accounting.MemoryRecorder, map[string]*http.Client) {
	t.Helper()
	probe.t = t
	recorder := &accounting.MemoryRecorder{}
	clients := make(map[string]*http.Client)
	for _, p := range rt.Catalog.Providers() {
		clients[p.Name] = &http.Client{}
	}
	return NewHandler(Dependencies{
		Resolver: modelresolver.New(rt), Catalog: rt.Catalog, Adapter: probe,
		Auth:    auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Metrics: observability.NewMetrics(), MetricsToken: metricsTestToken, Accounting: recorder,
		HasAccessLog: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Version: "reasoning-test",
		ClientForProvider: func(p config.Provider) *http.Client { return clients[p.Name] },
	}), recorder, clients
}

func scopedReasoningPost(h http.Handler, ctx context.Context, op provider.Operation, body []byte) *httptest.ResponseRecorder {
	path := "/v1/chat/completions"
	if op == provider.OpResponses {
		path = "/v1/responses"
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	request.Header["X-Fixture"] = []string{"inbound", "second"}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, request)
	return w
}

func assertScopedReasoningAttempt(t *testing.T, attempt *scopedReasoningAttempt, rt *config.Runtime, clients map[string]*http.Client, ctx context.Context, op provider.Operation, publicModel, target string) {
	t.Helper()
	prov, model, ok := rt.Catalog.Model(target, "m")
	if !ok {
		t.Fatal("missing fixture target")
	}
	r := attempt.Request
	if attempt.Context != ctx || r.Operation != op || r.PublicModel != publicModel ||
		r.ProviderType != prov.Type || r.APIKey != prov.APIKey || r.CopilotToken != prov.CopilotToken ||
		r.BaseURL != prov.BaseURL || r.UpstreamModel != model.UpstreamName || r.ModelProtocol != model.Protocol ||
		r.Client != clients[target] || r.Version != "reasoning-test" ||
		r.UserAgent != prov.UserAgent || r.ForwardUserAgent != prov.ForwardUserAgent ||
		!reflect.DeepEqual(r.ForwardHeaders, prov.ForwardHeaders) || r.ForwardAuthorization != prov.ForwardAuthorization {
		t.Fatalf("attempt changed target identity, transport, or context: %+v", r)
	}
	deadline, ok := ctx.Deadline()
	gotDeadline, gotOK := attempt.Context.Deadline()
	if ok != gotOK || !deadline.Equal(gotDeadline) {
		t.Fatal("attempt changed the admitted deadline")
	}
	if !reflect.DeepEqual(r.Inbound.Header.Values("X-Fixture"), []string{"inbound", "second"}) {
		t.Fatal("attempt changed inbound header values")
	}
}

func TestScopedEncryptedReasoningDirect(t *testing.T) {
	for _, op := range []provider.Operation{provider.OpChatCompletions, provider.OpResponses} {
		for _, tc := range []struct {
			name, root, provider, model, alias string
			statuses                           []int
			stripped                           []bool
		}{
			{"absent", "", "", "", "", []int{400}, []bool{false}},
			{"alias-only", "", "", "", "strip", []int{400}, []bool{false}},
			{"root-strip", "strip", "", "", "", []int{400}, []bool{true}},
			{"provider-strip", "retry", "strip", "", "", []int{400}, []bool{true}},
			{"model-strip", "retry", "retry", "strip", "", []int{400}, []bool{true}},
			{"strip-never-recovers", "", "", "strip-retry", "", []int{400}, []bool{true}},
			{"root-retry", "retry", "", "", "", []int{400, 200}, []bool{false, true}},
			{"provider-replaces-root-strip", "strip", "retry", "", "", []int{400, 200}, []bool{false, true}},
			{"model-replaces-provider-strip", "strip", "strip", "retry", "", []int{400, 200}, []bool{false, true}},
			{"provider-reset", "strip", "reset", "", "", []int{400}, []bool{false}},
			{"model-reset", "retry", "strip", "reset", "", []int{400}, []bool{false}},
			{"direct-retry-no-failover", "", "retry", "", "retry", []int{400, 503}, []bool{false, true}},
			{"direct-one-recovery", "", "", "retry", "", []int{400, 400}, []bool{false, true}},
		} {
			t.Run(op.String()+"/"+tc.name, func(t *testing.T) {
				rt := scopedReasoningRuntime(tc.root, tc.alias, []string{tc.provider}, []string{tc.model})
				probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
					if call >= len(tc.statuses) {
						t.Errorf("unexpected inference attempt %d", call+1)
						return scopedReasoningResult(200), nil
					}
					return scopedReasoningResult(tc.statuses[call]), nil
				}}
				h, recorder, clients := scopedReasoningHandler(t, rt, probe)
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				original, stripped := scopedReasoningBodies(op, "p1/m")
				w := scopedReasoningPost(h, ctx, op, original)
				if w.Code != tc.statuses[len(tc.statuses)-1] {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				attempts := probe.snapshot()
				if len(attempts) != len(tc.statuses) {
					t.Fatalf("attempts=%d want=%d", len(attempts), len(tc.statuses))
				}
				for i, attempt := range attempts {
					assertScopedReasoningAttempt(t, attempt, rt, clients, ctx, op, "p1/m", "p1")
					assertScopedReasoningBody(t, attempt.Request.Body, original, stripped, tc.stripped[i])
					if attempt.Closes.Load() != 1 {
						t.Fatalf("attempt %d finalized %d times", i+1, attempt.Closes.Load())
					}
					if i > 0 && attempt.Request.Inbound == attempts[i-1].Request.Inbound {
						t.Fatal("recovery reused an exhausted inbound body")
					}
				}
				final := attempts[len(attempts)-1].Result
				if !bytes.Equal(w.Body.Bytes(), final.Body) || !reflect.DeepEqual(w.Header().Values("X-Upstream-Marker"), final.Header.Values("X-Upstream-Marker")) {
					t.Fatal("terminal response bytes or header values changed")
				}
				events := recorder.Events()
				if len(events) != 1 || events[0].Provider != "p1" || events[0].UpstreamModel != "m" ||
					events[0].TotalTokens != 10 || events[0].ReasoningEffort != "high" {
					t.Fatalf("recovery duplicated or changed inbound accounting: %+v", events)
				}
				metrics := collectMetrics(t, h)
				if strings.Contains(metrics, `aiproxy_alias_retries_total{alias="a"`) {
					t.Fatal("direct recovery wrote alias retry state")
				}
				for _, p := range rt.Catalog.Providers() {
					_, model, _ := rt.Catalog.Model(p.Name, "m")
					if _, ok := h.current().Resolver.Cooldowns().Remaining(modelresolver.CooldownFingerprintFor("a", p, model)); ok {
						t.Fatal("direct recovery wrote an alias cooldown")
					}
				}
			})
		}
	}
}

func TestScopedEncryptedReasoningAliasSequences(t *testing.T) {
	for _, op := range []provider.Operation{provider.OpChatCompletions, provider.OpResponses} {
		for _, tc := range []struct {
			name, root, alias string
			providers, models []string
			retry400          bool
			statuses          []int
			targets           []string
			stripped          []bool
		}{
			{"explicit-sticky", "", "retry", []string{"strip", "reset"}, nil, false, []int{400, 503, 200}, []string{"p1", "p1", "p2"}, []bool{false, true, true}},
			{"target-local", "", "", []string{"retry", "retry"}, nil, false, []int{400, 503, 200}, []string{"p1", "p1", "p2"}, []bool{false, true, false}},
			{"exhausted-still-prestrips", "", "", []string{"retry", "strip"}, nil, false, []int{400, 503, 200}, []string{"p1", "p1", "p2"}, []bool{false, true, true}},
			{"explicit-exhausted", "", "retry", nil, nil, false, []int{400, 503, 400}, []string{"p1", "p1", "p2"}, []bool{false, true, true}},
			{"target-local-exhausted", "", "", []string{"retry", "retry"}, nil, false, []int{400, 503, 400}, []string{"p1", "p1", "p2"}, []bool{false, true, false}},
			{"explicit-prestrip", "", "strip", nil, nil, false, []int{503, 200}, []string{"p1", "p2"}, []bool{true, true}},
			{"prestrip-then-reset", "retry", "", []string{"strip", "reset"}, nil, false, []int{503, 400}, []string{"p1", "p2"}, []bool{true, false}},
			{"prestrip-then-recovery", "", "", []string{"strip", "retry"}, nil, false, []int{503, 400, 200}, []string{"p1", "p2", "p2"}, []bool{true, false, true}},
			{"400-failover-then-recovery", "", "", []string{"reset", "retry"}, nil, true, []int{400, 400, 200}, []string{"p1", "p2", "p2"}, []bool{false, false, true}},
			{"400-recovery-before-failover", "", "", []string{"retry", "reset"}, nil, true, []int{400, 400, 200}, []string{"p1", "p1", "p2"}, []bool{false, true, false}},
			{"exhausted-400-normal-failover", "", "", []string{"retry", "retry", "retry"}, nil, true, []int{400, 503, 400, 400}, []string{"p1", "p1", "p2", "p3"}, []bool{false, true, false, false}},
			{"explicit-exhausted-400-failover", "", "retry", []string{"strip", "strip", "strip"}, nil, true, []int{400, 503, 400, 400}, []string{"p1", "p1", "p2", "p3"}, []bool{false, true, true, true}},
			{"explicit-reset-overrides-all", "strip", "reset", []string{"strip", "strip"}, []string{"strip", "strip"}, false, []int{503, 400}, []string{"p1", "p2"}, []bool{false, false}},
			{"selected-model-policy", "strip", "", []string{"strip", "reset"}, []string{"reset", "retry"}, true, []int{400, 400, 200}, []string{"p1", "p2", "p2"}, []bool{false, false, true}},
			{"root-fallback-per-target", "retry", "", nil, []string{"reset", ""}, true, []int{400, 400, 200}, []string{"p1", "p2", "p2"}, []bool{false, false, true}},
			{"recovery-transport-failover", "", "", []string{"retry", "reset"}, nil, false, []int{400, -1, 200}, []string{"p1", "p1", "p2"}, []bool{false, true, false}},
		} {
			t.Run(op.String()+"/"+tc.name, func(t *testing.T) {
				rt := scopedReasoningRuntime(tc.root, tc.alias, tc.providers, tc.models)
				if tc.retry400 {
					aliases := rt.Catalog.Aliases()
					aliases[0].RetryStatusCodes = append(aliases[0].RetryStatusCodes, 400)
					rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
				}
				probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
					if call >= len(tc.statuses) {
						t.Errorf("unexpected inference attempt %d", call+1)
						return scopedReasoningResult(200), nil
					}
					if tc.statuses[call] == -1 {
						return scopedReasoningResult(502), io.ErrUnexpectedEOF
					}
					return scopedReasoningResult(tc.statuses[call]), nil
				}}
				h, recorder, clients := scopedReasoningHandler(t, rt, probe)
				ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
				defer cancel()
				original, stripped := scopedReasoningBodies(op, "alias/a")
				w := scopedReasoningPost(h, ctx, op, original)
				if w.Code != tc.statuses[len(tc.statuses)-1] {
					t.Fatalf("status=%d body=%s", w.Code, w.Body.String())
				}
				attempts := probe.snapshot()
				if len(attempts) != len(tc.statuses) || len(attempts) > len(rt.Catalog.Providers())+1 {
					t.Fatalf("attempts=%d want=%d; request-wide recovery bound exceeded", len(attempts), len(tc.statuses))
				}
				for i, attempt := range attempts {
					assertScopedReasoningAttempt(t, attempt, rt, clients, ctx, op, "alias/a", tc.targets[i])
					assertScopedReasoningBody(t, attempt.Request.Body, original, stripped, tc.stripped[i])
					if attempt.Closes.Load() != 1 {
						t.Fatalf("attempt %d finalized %d times", i+1, attempt.Closes.Load())
					}
					if i > 0 && attempt.Request.Inbound == attempts[i-1].Request.Inbound {
						t.Fatal("attempts shared an exhausted inbound body")
					}
				}
				final := attempts[len(attempts)-1].Result
				if !bytes.Equal(w.Body.Bytes(), final.Body) {
					t.Fatal("terminal response body changed")
				}
				events := recorder.Events()
				if len(events) != 1 || events[0].Provider != tc.targets[len(tc.targets)-1] || events[0].TotalTokens != 10 {
					t.Fatalf("recovery duplicated or changed inbound accounting: %+v", events)
				}
				metrics := collectMetrics(t, h)
				for _, name := range tc.targets {
					route02GaugeZero(t, metrics, fmt.Sprintf(`aiproxy_alias_inflight_requests{alias="a",model="m",provider=%q}`, name))
				}
			})
		}
	}
}
