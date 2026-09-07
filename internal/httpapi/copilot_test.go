package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
)

func copilotTestRT() *config.Runtime {
	return &config.Runtime{
		Catalog: config.NewCatalog([]config.Provider{
			{
				Type:         config.ProviderTypeGitHubCopilot,
				Name:         "copilot",
				CopilotToken: "gho_test-token",
				Models:       []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-2024-08-06"}},
			},
		}, nil, nil),
	}
}

func errorTypeOf(t *testing.T, body []byte) string {
	t.Helper()
	var resp struct {
		Error struct {
			Type string `json:"type"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal error response: %v", err)
	}
	return resp.Error.Type
}

func TestHandlerRejectsUnsupportedOpsForCopilot(t *testing.T) {
	for _, tc := range []struct {
		path string
		body string
	}{
		{"/v1/embeddings", `{"model":"copilot/gpt-4o-mini","input":"hello"}`},
		{"/v1/responses", `{"model":"copilot/gpt-4o-mini","input":"hello"}`},
		{"/v1/images/generations", `{"model":"copilot/gpt-4o-mini","prompt":"a cat"}`},
		{"/v1/audio/transcriptions", `{"model":"copilot/gpt-4o-mini"}`},
		{"/v1/audio/speech", `{"model":"copilot/gpt-4o-mini","input":"hi","voice":"alloy"}`},
	} {
		stub := &stubAdapter{}
		h := newHandler(t, copilotTestRT(), stub)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, tc.path, bytes.NewReader([]byte(tc.body)))
		h.ServeHTTP(w, r)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", tc.path, w.Code)
		}
		if got := errorTypeOf(t, w.Body.Bytes()); got != "unsupported_operation" {
			t.Fatalf("%s: error type = %q", tc.path, got)
		}
		if stub.got.PublicModel != "" {
			t.Fatalf("%s: adapter should not have been called, got %+v", tc.path, stub.got)
		}
	}
}

func TestHandlerPlumbsCopilotTokenToAdapter(t *testing.T) {
	stub := &stubAdapter{}
	h := newHandler(t, copilotTestRT(), stub)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"copilot/gpt-4o-mini","messages":[]}`)))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.CopilotToken != "gho_test-token" {
		t.Fatalf("copilot token = %q", stub.got.CopilotToken)
	}
	if stub.got.APIKey != "" {
		t.Fatalf("APIKey must stay empty for copilot, got %q", stub.got.APIKey)
	}
}

func TestHandlerCopilotDirectAuthFailureVerbatim(t *testing.T) {
	var calls int
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if got := r.Header.Get("Authorization"); got != "Bearer gho_revoked" {
			t.Errorf("auth = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"message":"invalid token"}}`)
	}))
	defer upstream.Close()

	rt := &config.Runtime{
		Catalog: config.NewCatalog([]config.Provider{
			{
				Type:         config.ProviderTypeGitHubCopilot,
				Name:         "copilot",
				BaseURL:      upstream.URL,
				CopilotToken: "gho_revoked",
				Models:       []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-2024-08-06"}},
			},
		}, nil, nil),
	}
	h := newHandler(t, rt, provider.New())
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"copilot/gpt-4o-mini","messages":[]}`)))
	h.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", w.Code)
	}
	if !strings.Contains(w.Body.String(), "invalid token") {
		t.Fatalf("body not verbatim: %s", w.Body.String())
	}
	if calls != 1 {
		t.Fatalf("calls = %d, want exactly 1 (no auto-replay)", calls)
	}
}

func TestHandlerCopilotAliasRetrySemantics(t *testing.T) {
	setup := func(p1Status int, p1Body string) (*httptest.Server, *httptest.Server, *int, *int) {
		var c1, c2 int
		s1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c1++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(p1Status)
			_, _ = io.WriteString(w, p1Body)
		}))
		s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			c2++
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"id":"chatcmpl-p2","object":"chat.completion","choices":[]}`)
		}))
		return s1, s2, &c1, &c2
	}
	newAliasRT := func(s1, s2 *httptest.Server) *config.Runtime {
		return &config.Runtime{
			Catalog: config.NewCatalog([]config.Provider{
				{
					Type:         config.ProviderTypeGitHubCopilot,
					Name:         "copilot",
					BaseURL:      s1.URL,
					CopilotToken: "gho_test-token",
					Models:       []config.Model{{Name: "m", UpstreamName: "m"}},
				},
				{
					Type:    config.ProviderTypeOpenAI,
					Name:    "openai",
					BaseURL: s2.URL,
					APIKey:  "sk-test",
					Models:  []config.Model{{Name: "m", UpstreamName: "m"}},
				},
			}, nil, []config.Alias{{
				Name:             "a",
				Algorithm:        config.AlgorithmRoundRobin,
				RetryStatusCodes: []int{500, 502, 503, 504},
				Targets:          []config.AliasTarget{{Provider: "copilot", Model: "m"}, {Provider: "openai", Model: "m"}},
			}}),
		}
	}

	s1, s2, c1, c2 := setup(http.StatusBadGateway, `{"error":"upstream down"}`)
	defer s1.Close()
	defer s2.Close()
	h := newHandler(t, newAliasRT(s1, s2), provider.New())
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"alias/a","messages":[]}`))))
	if w.Code != http.StatusOK {
		t.Fatalf("retryable 502: status = %d, want 200 (failover to openai)", w.Code)
	}
	if *c1 != 1 || *c2 != 1 {
		t.Fatalf("calls = p1:%d p2:%d, want 1/1", *c1, *c2)
	}

	s1, s2, c1, c2 = setup(http.StatusUnauthorized, `{"error":"invalid token"}`)
	defer s1.Close()
	defer s2.Close()
	h = newHandler(t, newAliasRT(s1, s2), provider.New())
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"alias/a","messages":[]}`))))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("401: status = %d, want 401 verbatim", w.Code)
	}
	if *c1 != 1 || *c2 != 0 {
		t.Fatalf("401 must not retry: calls = p1:%d p2:%d, want 1/0", *c1, *c2)
	}
}
