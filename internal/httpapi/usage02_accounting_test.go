package httpapi

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

func TestUsage02ResponsesCountsReachAccountingOnce(t *testing.T) {
	const upstreamBody = `{"id":"resp_1","object":"response","model":"gpt-4o-mini","output":[],"status":"completed","usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, upstreamBody)
	}))
	defer upstream.Close()

	rt := newRT()
	replaceProviders(rt, []config.Provider{{
		Type:    config.ProviderTypeOpenAI,
		Name:    "openai",
		BaseURL: upstream.URL,
		APIKey:  "sk",
		Models:  []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}},
	}})
	recorder := &accounting.MemoryRecorder{}
	h := NewHandler(Dependencies{
		Resolver:   modelresolver.New(rt),
		Adapter:    provider.New(),
		Auth:       auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Authorizer: auth.NewAuthorizer(config.Auth{Mode: config.AuthModeNone}),
		Catalog:    rt.Catalog,
		Metrics:    observability.NewMetrics(),
		Accounting: recorder,
	})
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/responses", bytes.NewReader([]byte(`{"model":"openai/gpt-4o-mini","input":"hi"}`)))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if w.Body.String() != upstreamBody {
		t.Fatalf("body not preserved: got %q want %q", w.Body.String(), upstreamBody)
	}
	events := recorder.Events()
	if len(events) != 1 {
		t.Fatalf("events = %+v, want exactly one", events)
	}
	e := events[0]
	if e.PromptTokens != 4 || e.CompletionTokens != 6 || e.TotalTokens != 10 {
		t.Fatalf("accounting event = %+v, want 4/6/10", e)
	}
	if e.Operation != "responses" {
		t.Fatalf("operation = %q", e.Operation)
	}
}
