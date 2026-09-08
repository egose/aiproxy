package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
)

// REVIEW-02 (STREAM-02 HTTP outcome): a mid-stream translated Anthropic error
// must not fabricate success. Headers are already committed (200), the partial
// content stays, no [DONE] terminal is synthesized, health records the failure,
// and accounting records exactly one event without fabricated usage.
func TestReview02TranslatedStreamErrorHTTPOutcome(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\n")
		_, _ = io.WriteString(w, "data: {\"message\":{\"id\":\"msg_1\"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		_, _ = io.WriteString(w, "event: error\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n")
	}))
	defer upstream.Close()

	rt := &config.Runtime{
		Catalog: config.NewCatalog([]config.Provider{{
			Type:    config.ProviderTypeAnthropic,
			Name:    "anthropic",
			BaseURL: upstream.URL,
			APIKey:  "sk-test",
			Models:  []config.Model{{Name: "claude-sonnet", UpstreamName: "claude-sonnet-4-20250514"}},
		}}, nil, nil),
	}
	metrics := observability.NewMetrics()
	health := providerhealth.New(metrics, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	recorder := &accounting.MemoryRecorder{}
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    provider.New(),
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Authorizer: auth.NewAuthorizer(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    metrics,
		Health:     health,
		Accounting: recorder,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		bytes.NewReader([]byte(`{"model":"anthropic/claude-sonnet","stream":true,"messages":[{"role":"user","content":"hi"}]}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%q (committed headers must not change to an error status)", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "Hello") {
		t.Fatalf("partial translated content lost: %q", body)
	}
	if strings.Contains(body, "data: [DONE]") {
		t.Fatalf("translated stream error fabricated [DONE]: %q", body)
	}
	if strings.Contains(body, "response.completed") {
		t.Fatalf("translated stream error fabricated completion: %q", body)
	}
	if health.IsHealthy("anthropic") {
		t.Fatalf("translated mid-stream error did not mark provider unhealthy")
	}
	events := recorder.Events()
	if len(events) != 1 {
		t.Fatalf("accounting events = %d, want exactly one", len(events))
	}
	if events[0].PromptTokens != 0 || events[0].CompletionTokens != 0 || events[0].TotalTokens != 0 {
		t.Fatalf("failed stream fabricated usage: %+v", events[0])
	}
}

// REVIEW-02 (USAGE-02 public summary): corrected native Responses usage must
// reach the public GET /v1/billing/usage summary, not just the recorder.
func TestReview02NativeResponsesUsageReachesBilling(t *testing.T) {
	const upstreamBody = `{"id":"resp_1","object":"response","model":"gpt-4o-mini","output":[],"status":"completed","usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, upstreamBody)
	}))
	defer upstream.Close()

	rt := &config.Runtime{
		Catalog: config.NewCatalog([]config.Provider{{
			Type:    config.ProviderTypeOpenAI,
			Name:    "openai",
			BaseURL: upstream.URL,
			APIKey:  "sk-test",
			Models:  []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}},
		}}, nil, nil),
	}
	agg := accounting.NewAggregator()
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    provider.New(),
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Authorizer: auth.NewAuthorizer(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		Accounting: agg,
		Usage:      agg,
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses",
		bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","input":"hi"}`)))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("responses status = %d, body=%s", w.Code, w.Body.String())
	}

	uw := httptest.NewRecorder()
	ur := httptest.NewRequest(http.MethodGet, "/v1/billing/usage", nil)
	h.ServeHTTP(uw, ur)
	if uw.Code != http.StatusOK {
		t.Fatalf("billing status = %d, body=%s", uw.Code, uw.Body.String())
	}
	var resp struct {
		Object string `json:"object"`
		Data   []struct {
			PromptTokens     int64 `json:"PromptTokens"`
			CompletionTokens int64 `json:"CompletionTokens"`
			TotalTokens      int64 `json:"TotalTokens"`
		} `json:"data"`
	}
	if err := json.Unmarshal(uw.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal billing: %v body=%s", err, uw.Body.String())
	}
	if len(resp.Data) != 1 {
		t.Fatalf("billing entries = %d, want 1: %s", len(resp.Data), uw.Body.String())
	}
	got := resp.Data[0]
	if got.PromptTokens != 4 || got.CompletionTokens != 6 || got.TotalTokens != 10 {
		t.Fatalf("billing usage = %+v, want 4/6/10", got)
	}
}
