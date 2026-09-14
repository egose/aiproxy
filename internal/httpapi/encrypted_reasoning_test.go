package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

func TestStripChatKeepsTextDropsThinking(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"private","signature":"sig"},{"type":"text","text":"hello"}]}]}`)
	out, changed := stripOpaqueBlocks(provider.OpChatCompletions, body)
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if strings.Contains(string(out), "private") || strings.Contains(string(out), "\"signature\"") {
		t.Fatalf("stripped body still contains thinking: %s", out)
	}
	if !strings.Contains(string(out), "hello") {
		t.Fatalf("stripped body lost text: %s", out)
	}
}

func TestStripChatStringContentUntouched(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"user","content":"hi"}]}`)
	out, changed := stripOpaqueBlocks(provider.OpChatCompletions, body)
	if changed {
		t.Fatalf("changed = true, want false for string content: %s", out)
	}
}

func TestStripChatEncryptedContentPart(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"blob"},{"type":"text","text":"done"}]}]}`)
	out, changed := stripOpaqueBlocks(provider.OpChatCompletions, body)
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if strings.Contains(string(out), "blob") {
		t.Fatalf("stripped body still contains encrypted blob: %s", out)
	}
	if !strings.Contains(string(out), "done") {
		t.Fatalf("stripped body lost text: %s", out)
	}
}

func TestStripChatToolHistoryKept(t *testing.T) {
	body := []byte(`{"model":"m","messages":[{"role":"assistant","content":[{"type":"text","text":"call"}],"tool_calls":[{"id":"1","type":"function","function":{"name":"f"}}]}]}`)
	out, changed := stripOpaqueBlocks(provider.OpChatCompletions, body)
	if changed {
		t.Fatalf("changed = true, want false when no opaque blocks: %s", out)
	}
	if !strings.Contains(string(out), "tool_calls") {
		t.Fatalf("body lost tool_calls: %s", out)
	}
}

func TestStripResponsesDropsReasoningKeepsMessage(t *testing.T) {
	body := []byte(`{"model":"m","input":[{"type":"reasoning","id":"rs_1","encrypted_content":"blob","summary":[]},{"type":"message","role":"user","content":"hi"}]}`)
	out, changed := stripOpaqueBlocks(provider.OpResponses, body)
	if !changed {
		t.Fatal("changed = false, want true")
	}
	if strings.Contains(string(out), "rs_1") || strings.Contains(string(out), "blob") {
		t.Fatalf("stripped input still contains reasoning: %s", out)
	}
	if !strings.Contains(string(out), `"hi"`) {
		t.Fatalf("stripped input lost message: %s", out)
	}
}

func TestStripInvalidJSONPassthrough(t *testing.T) {
	body := []byte(`{invalid`)
	out, changed := stripOpaqueBlocks(provider.OpChatCompletions, body)
	if changed || string(out) != string(body) {
		t.Fatalf("invalid JSON must pass through unchanged, got %s changed=%v", out, changed)
	}
}

func TestIsCallerMismatchGating(t *testing.T) {
	patterns := []string{"not issued to this caller"}
	mismatch := []byte(`{"error":{"type":"invalid_request_error","message":"reasoning encrypted_content was not issued to this caller"}}`)
	if !isCallerMismatch(400, mismatch, patterns) {
		t.Fatal("matching 400 body not detected")
	}
	if isCallerMismatch(400, []byte(strings.ToUpper(string(mismatch))), patterns) == false {
		t.Fatal("match must be case-insensitive")
	}
	if isCallerMismatch(500, mismatch, patterns) {
		t.Fatal("non-400 status must not match")
	}
	if isCallerMismatch(400, []byte(`{"error":"bad max_tokens"}`), patterns) {
		t.Fatal("unrelated 400 must not match")
	}
	if isCallerMismatch(400, mismatch, nil) {
		t.Fatal("empty patterns must not match")
	}
}

type reasoningStubAdapter struct {
	mu      sync.Mutex
	bodies  [][]byte
	keys    []string
	results []*provider.Result
}

func (a *reasoningStubAdapter) Do(_ context.Context, r provider.Request) (*provider.Result, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.bodies = append(a.bodies, append([]byte(nil), r.Body...))
	a.keys = append(a.keys, r.APIKey)
	if len(a.results) == 0 {
		return &provider.Result{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"id":"ok"}`)}, nil
	}
	res := a.results[0]
	a.results = a.results[1:]
	return res, nil
}

func (a *reasoningStubAdapter) recorded() ([][]byte, []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([][]byte(nil), a.bodies...), append([]string(nil), a.keys...)
}

func reasoningTestRT(er *config.EncryptedReasoning) *config.Runtime {
	return &config.Runtime{Catalog: config.NewCatalog([]config.Provider{
		{Type: config.ProviderTypeOpenAI, Name: "p1", APIKey: "key1", BaseURL: "https://x", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
		{Type: config.ProviderTypeOpenAI, Name: "p2", APIKey: "key2", BaseURL: "https://x", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
	}, nil, []config.Alias{{
		Name:               "a",
		Algorithm:          config.AlgorithmRoundRobin,
		RetryStatusCodes:   []int{500},
		EncryptedReasoning: er,
		Targets:            []config.AliasTarget{{Provider: "p1", Model: "m"}, {Provider: "p2", Model: "m"}},
	}})}
}

func reasoningTestHandler(t *testing.T, rt *config.Runtime, adapter *reasoningStubAdapter) http.Handler {
	t.Helper()
	return NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  adapter,
		Auth:     auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:  rt.Catalog,
		Metrics:  observability.NewMetrics(),
	})
}

func TestAliasUnconditionalStrip(t *testing.T) {
	adapter := &reasoningStubAdapter{}
	h := reasoningTestHandler(t, reasoningTestRT(&config.EncryptedReasoning{Passthrough: false, OnCallerMismatch: config.EncryptedReasoningFail}), adapter)
	reqBody := `{"model":"alias/a","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"secret","signature":"sig"},{"type":"text","text":"hi"}]}]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	bodies, _ := adapter.recorded()
	if len(bodies) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(bodies))
	}
	if strings.Contains(string(bodies[0]), "secret") {
		t.Fatalf("upstream body still contains thinking: %s", bodies[0])
	}
	if !strings.Contains(string(bodies[0]), `"hi"`) {
		t.Fatalf("upstream body lost text: %s", bodies[0])
	}
}

func TestAliasStripAndRetryOnCallerMismatch(t *testing.T) {
	mismatch := &provider.Result{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"error":{"type":"invalid_request_error","message":"reasoning encrypted_content was not issued to this caller"}}`)}
	adapter := &reasoningStubAdapter{results: []*provider.Result{mismatch, {StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"id":"ok"}`)}}}
	h := reasoningTestHandler(t, reasoningTestRT(&config.EncryptedReasoning{Passthrough: true, OnCallerMismatch: config.EncryptedReasoningStripAndRetry}), adapter)
	reqBody := `{"model":"alias/a","messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"blob"},{"type":"text","text":"hi"}]}]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want stripped retry success", w.Code, w.Body.String())
	}
	bodies, keys := adapter.recorded()
	if len(bodies) != 2 {
		t.Fatalf("upstream calls = %d, want 2", len(bodies))
	}
	if keys[0] != keys[1] {
		t.Fatalf("retry went to %q, want same target %q", keys[1], keys[0])
	}
	if !strings.Contains(string(bodies[0]), "blob") {
		t.Fatalf("first attempt must carry original reasoning: %s", bodies[0])
	}
	if strings.Contains(string(bodies[1]), "blob") {
		t.Fatalf("retry must be stripped: %s", bodies[1])
	}
}

func TestAliasNoRetryOnUnrelated400(t *testing.T) {
	other := &provider.Result{StatusCode: http.StatusBadRequest, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: []byte(`{"error":{"type":"invalid_request_error","message":"bad max_tokens"}}`)}
	adapter := &reasoningStubAdapter{results: []*provider.Result{other}}
	h := reasoningTestHandler(t, reasoningTestRT(&config.EncryptedReasoning{Passthrough: true, OnCallerMismatch: config.EncryptedReasoningStripAndRetry}), adapter)
	reqBody := `{"model":"alias/a","messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"blob"},{"type":"text","text":"hi"}]}]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want verbatim 400", w.Code)
	}
	bodies, _ := adapter.recorded()
	if len(bodies) != 1 {
		t.Fatalf("upstream calls = %d, want 1 without retry", len(bodies))
	}
}

func TestDirectRequestNeverStripped(t *testing.T) {
	adapter := &reasoningStubAdapter{}
	h := reasoningTestHandler(t, reasoningTestRT(&config.EncryptedReasoning{Passthrough: false, OnCallerMismatch: config.EncryptedReasoningFail}), adapter)
	reqBody := `{"model":"p1/m","messages":[{"role":"assistant","content":[{"type":"thinking","thinking":"secret","signature":"sig"},{"type":"text","text":"hi"}]}]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(reqBody))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	bodies, _ := adapter.recorded()
	if len(bodies) != 1 {
		t.Fatalf("upstream calls = %d, want 1", len(bodies))
	}
	if !strings.Contains(string(bodies[0]), "secret") {
		t.Fatalf("direct request must stay verbatim: %s", bodies[0])
	}
}
