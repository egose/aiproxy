package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestExecuteUpstreamSnapshotsSentRequest(t *testing.T) {
	var gotBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		gotBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"p/m","messages":[]}`))
	res, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeOpenAI,
		PublicModel:   "p/m",
		BaseURL:       upstream.URL,
		APIKey:        "provider-secret",
		UpstreamModel: "up-m",
		Version:       "1.2.3-test",
		Body:          []byte(`{"model":"p/m","messages":[]}`),
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if string(res.UpstreamRequestBody) != string(gotBody) {
		t.Fatalf("logged body %q != received body %q", res.UpstreamRequestBody, gotBody)
	}
	if !strings.Contains(string(res.UpstreamRequestBody), `"model":"up-m"`) {
		t.Fatalf("snapshot should show rewritten model: %q", res.UpstreamRequestBody)
	}
	if got := res.UpstreamRequestHeaders.Get("Authorization"); got != "Bearer provider-secret" {
		t.Fatalf("snapshot authorization = %q", got)
	}
	if got := res.UpstreamRequestHeaders.Get("User-Agent"); got != "aiproxy/1.2.3-test" {
		t.Fatalf("snapshot user-agent = %q", got)
	}
	if got := res.UpstreamRequestHeaders.Get("Content-Type"); got != "application/json" {
		t.Fatalf("snapshot content-type = %q", got)
	}
}

func TestExecuteUpstreamSnapshotsErrorResponses(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":"bad"}`)
	}))
	defer upstream.Close()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"p/m","messages":[]}`))
	res, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeOpenAI,
		PublicModel:   "p/m",
		BaseURL:       upstream.URL,
		APIKey:        "provider-secret",
		UpstreamModel: "up-m",
		Version:       "1.2.3-test",
		Body:          []byte(`{"model":"p/m","messages":[]}`),
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res == nil {
		t.Fatalf("expected result for upstream error status")
	}
	if res.UpstreamRequestHeaders == nil || len(res.UpstreamRequestBody) == 0 {
		t.Fatalf("snapshot missing on error path: %+v", res)
	}
}

func TestExecuteUpstreamForwardsAllowlistedHeaders(t *testing.T) {
	var gotHeaders http.Header
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeaders = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"p/m","messages":[]}`))
	inbound.Header.Set("X-Session-Id", "sess-123")
	inbound.Header.Add("X-Multi", "one")
	inbound.Header.Add("X-Multi", "two")
	inbound.Header.Set("Authorization", "Bearer client-secret")
	inbound.Header.Set("X-Bad", "has\nnewline")
	inbound.Header.Set("X-Long", strings.Repeat("a", maxForwardedHeaderValueBytes+1))
	_, err := New().Do(context.Background(), Request{
		Operation:      OpChatCompletions,
		ProviderType:   config.ProviderTypeOpenAI,
		PublicModel:    "p/m",
		BaseURL:        upstream.URL,
		APIKey:         "provider-secret",
		UpstreamModel:  "up-m",
		Version:        "1.2.3-test",
		Body:           []byte(`{"model":"p/m","messages":[]}`),
		Inbound:        inbound,
		Client:         upstream.Client(),
		ForwardHeaders: []string{"X-Session-Id", "X-Session-Affinity", "X-Multi", "Authorization", "X-Bad", "X-Long"},
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if got := gotHeaders.Get("X-Session-Id"); got != "sess-123" {
		t.Fatalf("X-Session-Id = %q, want forwarded", got)
	}
	if got := gotHeaders.Values("X-Multi"); len(got) != 2 || got[0] != "one" || got[1] != "two" {
		t.Fatalf("X-Multi = %v, want both values forwarded", got)
	}
	if _, present := gotHeaders["X-Session-Affinity"]; present {
		t.Fatalf("absent header should be skipped, got %v", gotHeaders["X-Session-Affinity"])
	}
	if got := gotHeaders.Get("Authorization"); got != "Bearer provider-secret" {
		t.Fatalf("Authorization = %q, want proxy credential to win", got)
	}
	if _, present := gotHeaders["X-Bad"]; present {
		t.Fatalf("invalid value should be dropped, got %v", gotHeaders["X-Bad"])
	}
	if _, present := gotHeaders["X-Long"]; present {
		t.Fatalf("over-long value should be dropped")
	}
}

func TestApplyForwardedHeadersNilSafe(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "http://example.com", nil)
	applyForwardedHeaders(Request{ForwardHeaders: []string{"X-Session-Id"}}, req)
	applyForwardedHeaders(Request{Inbound: httptest.NewRequest(http.MethodPost, "http://example.com", nil)}, req)
}
