package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

type countingAdapter struct {
	mu     sync.Mutex
	calls  int
	bodies [][]byte
}

func (a *countingAdapter) Do(ctx context.Context, r provider.Request) (*provider.Result, error) {
	a.mu.Lock()
	a.calls++
	body := append([]byte(nil), r.Body...)
	a.bodies = append(a.bodies, body)
	a.mu.Unlock()
	return &provider.Result{
		StatusCode: http.StatusOK,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       []byte(`{"id":"chatcmpl-stub"}`),
	}, nil
}

func (a *countingAdapter) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func (a *countingAdapter) lastBody() []byte {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.bodies) == 0 {
		return nil
	}
	return a.bodies[len(a.bodies)-1]
}

func guardrailTestKey(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("rand: %v", err)
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ234567"
	out := make([]byte, 16)
	for i := range out {
		out[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return "AKIA" + string(out)
}

func guardrailTestScanner(t *testing.T, mode guardrails.Mode, maxTextBytes, maxStrings int) *guardrails.Scanner {
	t.Helper()
	s, err := guardrails.New(guardrails.Policy{Enabled: true, Mode: mode, MaxTextBytes: maxTextBytes, MaxStrings: maxStrings})
	if err != nil {
		t.Fatalf("new scanner: %v", err)
	}
	return s
}

func guardrailTestHandler(t *testing.T, rt *config.Runtime, adapter provider.Adapter, scanner *guardrails.Scanner) http.Handler {
	t.Helper()
	deps := Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    adapter,
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		Guardrails: scanner,
	}
	return NewHandler(deps)
}

func guardrailChatBody(model, content string) string {
	msg, _ := json.Marshal(content)
	return `{"model":"` + model + `","messages":[{"role":"user","content":` + string(msg) + `}]}`
}

func guardrailErrorType(t *testing.T, body string) string {
	t.Helper()
	var e struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &e); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	return e.Error.Type
}

func TestGuardrailBlockDirectZeroUpstream(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
	key := guardrailTestKey(t)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key)))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "secret_blocked" {
		t.Fatalf("error type = %q, want secret_blocked", got)
	}
	if strings.Contains(w.Body.String(), key) {
		t.Fatalf("error body leaks secret text")
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

func TestGuardrailBlockAliasZeroUpstream(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
	key := guardrailTestKey(t)
	body := `{"model":"alias/chat_default","messages":[{"role":"user","content":"deploy with ` + key + `"}]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "secret_blocked" {
		t.Fatalf("error type = %q, want secret_blocked", got)
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

func TestGuardrailBlockStreamingRequest(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
	key := guardrailTestKey(t)
	body := `{"model":"openai/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"key ` + key + `"}]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for SSE-requested secret", w.Code)
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

func TestGuardrailBlockResponsesOp(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
	key := guardrailTestKey(t)
	body := `{"model":"openai/gpt-4o-mini","instructions":"deploy with ` + key + `","input":"hello"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s, want 400", w.Code, w.Body.String())
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "secret_blocked" {
		t.Fatalf("error type = %q, want secret_blocked", got)
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

func TestGuardrailAuditForwardsOriginal(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	deps := Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    adapter,
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		Guardrails: guardrailTestScanner(t, guardrails.ModeAudit, 0, 0),
	}
	h := NewHandler(deps)
	key := guardrailTestKey(t)
	body := guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200 in audit mode", w.Code, w.Body.String())
	}
	if n := adapter.count(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
	if got := string(adapter.lastBody()); got != body {
		t.Fatalf("forwarded body was rewritten: %q", got)
	}
}

func TestGuardrailCleanForwardsInBlockMode(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
	body := guardrailChatBody("openai/gpt-4o-mini", "explain photosynthesis")
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200", w.Code, w.Body.String())
	}
	if n := adapter.count(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

func TestGuardrailIncompleteOversize(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 1024, 0))
	body := guardrailChatBody("openai/gpt-4o-mini", strings.Repeat("hello world photosynthesis ", 200))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for oversize scan", w.Code)
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "scan_incomplete" {
		t.Fatalf("error type = %q, want scan_incomplete", got)
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

func TestGuardrailIncompleteAuditForwards(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeAudit, 1024, 0))
	body := guardrailChatBody("openai/gpt-4o-mini", strings.Repeat("hello world photosynthesis ", 200))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 in audit mode", w.Code)
	}
	if n := adapter.count(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

func TestGuardrailExtractionCoverage(t *testing.T) {
	key := guardrailTestKey(t)
	token := "ghp_" + guardrailGHToken(t)
	escaped := strings.ReplaceAll("deploy "+key, "AKIA", `\u0041KIA`)
	nestedArgs, _ := json.Marshal(map[string]string{"api_key": token})
	cases := []struct {
		name string
		body string
	}{
		{"history", `{"model":"openai/gpt-4o-mini","messages":[{"role":"system","content":"be brief"},{"role":"user","content":"hi"},{"role":"assistant","content":"hello"},{"role":"user","content":"use ` + key + `"}]}`},
		{"array parts", `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":[{"type":"text","text":"key is ` + key + `"}]}]}`},
		{"input_text part", `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":[{"type":"input_text","input_text":"` + key + `"}]}]}`},
		{"tool arguments plain", `{"model":"openai/gpt-4o-mini","messages":[{"role":"assistant","tool_calls":[{"id":"1","type":"function","function":{"name":"deploy","arguments":"token ` + key + `"}}]}]}`},
		{"tool arguments nested json", `{"model":"openai/gpt-4o-mini","messages":[{"role":"assistant","tool_calls":[{"id":"1","type":"function","function":{"name":"deploy","arguments":` + strconv.Quote(string(nestedArgs)) + `}}]}]}`},
		{"tool result", `{"model":"openai/gpt-4o-mini","messages":[{"role":"tool","tool_call_id":"1","content":"result ` + key + `"}]}`},
		{"unicode escapes", `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"` + escaped + `"}]}`},
		{"allow marker", `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"` + key + ` gitleaks:allow"}]}`},
		{"responses input items", `{"model":"openai/gpt-4o-mini","instructions":"help","input":[{"role":"user","content":[{"type":"input_text","text":"` + key + `"}]}]}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newRT()
			adapter := &countingAdapter{}
			h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
			path := "/v1/chat/completions"
			if strings.HasPrefix(tc.name, "responses") {
				path = "/v1/responses"
			}
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(w, r)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("status = %d body=%s, want 400", w.Code, w.Body.String())
			}
			if n := adapter.count(); n != 0 {
				t.Fatalf("upstream calls = %d, want 0", n)
			}
		})
	}
}

func guardrailGHToken(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 36)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("rand: %v", err)
	}
	const alphabet = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	out := make([]byte, 36)
	for i := range out {
		out[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return string(out)
}

func TestGuardrailBenignSamplesForward(t *testing.T) {
	benign := []string{
		"What is an API key and how do I use one?",
		"tokenize this sentence for NLP",
		"my password is hunter2 just kidding, explain entropy",
		"ghp_ is a prefix used by GitHub tokens",
		"AKIAIOSFODNN7EXAMPLE is a documentation example",
	}
	for i, content := range benign {
		rt := newRT()
		adapter := &countingAdapter{}
		h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(guardrailChatBody("openai/gpt-4o-mini", content)))
		r.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("benign[%d] status = %d body=%s, want 200", i, w.Code, w.Body.String())
		}
	}
}

func TestGuardrailMalformedKeepsPrecedence(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":`))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "invalid_model" {
		t.Fatalf("error type = %q, want invalid_model precedence", got)
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

func TestGuardrailOutOfScopeEmbeddingsBypass(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	deps := Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    adapter,
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		Guardrails: guardrailTestScanner(t, guardrails.ModeBlock, 0, 0),
	}
	h := NewHandler(deps)
	key := guardrailTestKey(t)
	body := `{"model":"openai/gpt-4o-mini","input":"embed ` + key + `"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/embeddings", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s, want 200 (embeddings out of scope)", w.Code, w.Body.String())
	}
	if n := adapter.count(); n != 1 {
		t.Fatalf("upstream calls = %d, want 1", n)
	}
}

func TestGuardrailMetricsRecorded(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	metrics := observability.NewMetrics()
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    adapter,
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    metrics,
		Guardrails: guardrailTestScanner(t, guardrails.ModeBlock, 0, 0),
	})
	key := guardrailTestKey(t)
	serve := func(body string) {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		h.ServeHTTP(w, r)
	}
	serve(guardrailChatBody("openai/gpt-4o-mini", "deploy "+key))
	serve(guardrailChatBody("openai/gpt-4o-mini", "hello"))
	out := collectMetricsBody(t, metrics)
	blocked := countMetricLines(out, "aiproxy_guardrail_scans_total", `outcome="flagged"`)
	clean := countMetricLines(out, "aiproxy_guardrail_scans_total", `outcome="clean"`)
	if blocked != 1 || clean != 1 {
		t.Fatalf("flagged=%v clean=%v, want 1 each\n%s", blocked, clean, out)
	}
	if strings.Contains(out, key) {
		t.Fatalf("metrics exposition leaks secret text")
	}
}

func collectMetricsBody(t *testing.T, m *observability.Metrics) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	m.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", rec.Code)
	}
	return rec.Body.String()
}

func countMetricLines(out, name, labelSub string) float64 {
	var total float64
	for _, line := range strings.Split(out, "\n") {
		if !strings.HasPrefix(line, name+"{") || !strings.Contains(line, labelSub) {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		v, err := strconv.ParseFloat(fields[1], 64)
		if err != nil {
			continue
		}
		total += v
	}
	return total
}

func TestGuardrailPayloadLogOmitsCoveredBodies(t *testing.T) {
	key := guardrailTestKey(t)
	secretBody := guardrailChatBody("openai/gpt-4o-mini", "deploy with "+key)
	cleanBody := guardrailChatBody("openai/gpt-4o-mini", "explain photosynthesis")
	earlyRejectBody := `{"messages":[]}`
	cases := []struct {
		name       string
		body       string
		wantStatus int
	}{
		{"clean", cleanBody, http.StatusOK},
		{"blocked", secretBody, http.StatusBadRequest},
		{"early rejected", earlyRejectBody, http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rt := newRT()
			pl, dir := payloadTestLogger(t, 1<<20)
			adapter := &countingAdapter{}
			h := NewHandler(Dependencies{
				Resolver:   modelresolver.New(rt),
				Adapter:    adapter,
				Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
				Catalog:    rt.Catalog,
				Metrics:    observability.NewMetrics(),
				PayloadLog: pl,
				Guardrails: guardrailTestScanner(t, guardrails.ModeBlock, 0, 0),
			})
			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(tc.body))
			r.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(w, r)
			if w.Code != tc.wantStatus {
				t.Fatalf("status = %d body=%s, want %d", w.Code, w.Body.String(), tc.wantStatus)
			}
			entries := readPayloadEntries(t, dir)
			if len(entries) != 1 {
				t.Fatalf("entries = %d, want 1", len(entries))
			}
			if entries[0].Request.Body.Data != "" || entries[0].Request.Body.Bytes != 0 {
				t.Fatalf("request body persisted: %+v", entries[0].Request.Body)
			}
			raw, _ := json.Marshal(entries[0])
			if strings.Contains(string(raw), key) {
				t.Fatalf("payload entry leaks secret text")
			}
			if strings.Contains(w.Body.String(), key) {
				t.Fatalf("error response leaks secret text")
			}
		})
	}
}

func TestGuardrailPayloadLogDisabledKeepsBodies(t *testing.T) {
	rt := newRT()
	pl, dir := payloadTestLogger(t, 1<<20)
	adapter := &countingAdapter{}
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    adapter,
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		PayloadLog: pl,
	})
	body := guardrailChatBody("openai/gpt-4o-mini", "hello")
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	entries := readPayloadEntries(t, dir)
	if len(entries) != 1 || entries[0].Request.Body.Data != body {
		t.Fatalf("disabled guardrails must preserve raw bodies: %+v", entries)
	}
}

func TestGuardrailMultipartCoveredOpIsIncomplete(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 0))
	var sb strings.Builder
	writer := multipart.NewWriter(&sb)
	part, err := writer.CreateFormField("model")
	if err != nil {
		t.Fatalf("form field: %v", err)
	}
	_, _ = io.WriteString(part, "openai/gpt-4o-mini")
	_ = writer.Close()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(sb.String()))
	r.Header.Set("Content-Type", writer.FormDataContentType())
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "scan_incomplete" {
		t.Fatalf("error type = %q, want scan_incomplete for multipart chat", got)
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}

func TestGuardrailTooManyStrings(t *testing.T) {
	rt := newRT()
	adapter := &countingAdapter{}
	h := guardrailTestHandler(t, rt, adapter, guardrailTestScanner(t, guardrails.ModeBlock, 0, 2))
	var msgs []string
	for i := 0; i < 5; i++ {
		msgs = append(msgs, fmt.Sprintf(`{"role":"user","content":"message %d"}`, i))
	}
	body := `{"model":"openai/gpt-4o-mini","messages":[` + strings.Join(msgs, ",") + `]}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", w.Code)
	}
	if got := guardrailErrorType(t, w.Body.String()); got != "scan_incomplete" {
		t.Fatalf("error type = %q, want scan_incomplete", got)
	}
	if n := adapter.count(); n != 0 {
		t.Fatalf("upstream calls = %d, want 0", n)
	}
}
