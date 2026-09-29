package httpapi

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func openRouterAliasRT() *config.Runtime {
	return &config.Runtime{Catalog: config.NewCatalog([]config.Provider{
		{Type: config.ProviderTypeOpenRouter, Name: "router", APIKey: "sk-or", BaseURL: "https://openrouter.ai/api/v1", Models: []config.Model{{Name: "gpt-4o-mini", UpstreamName: "openai/gpt-4o-mini"}}},
	}, nil, []config.Alias{{
		Name:      "or_default",
		Algorithm: config.AlgorithmRoundRobin,
		Targets:   []config.AliasTarget{{Provider: "router", Model: "gpt-4o-mini"}},
	}})}
}

func TestHandlerAliasRouteToOpenRouter(t *testing.T) {
	stub := &stubAdapter{}
	h := newHandler(t, openRouterAliasRT(), stub)

	body := []byte(`{"model":"alias/or_default","messages":[]}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if stub.got.ProviderType != config.ProviderTypeOpenRouter {
		t.Fatalf("provider type = %q, want openrouter", stub.got.ProviderType)
	}
	if stub.got.APIKey != "sk-or" {
		t.Fatalf("alias target credentials not forwarded: %q", stub.got.APIKey)
	}
	if stub.got.UpstreamModel != "openai/gpt-4o-mini" {
		t.Fatalf("upstream model = %q, want openai/gpt-4o-mini", stub.got.UpstreamModel)
	}
	if stub.got.PublicModel != "alias/or_default" {
		t.Fatalf("public model = %q, want alias/or_default", stub.got.PublicModel)
	}
}

func TestHandlerAliasRetriesOn5xxAcrossOpenRouterTargets(t *testing.T) {
	rt := &config.Runtime{Catalog: config.NewCatalog([]config.Provider{
		{Type: config.ProviderTypeOpenRouter, Name: "p1", APIKey: "key1", BaseURL: "https://openrouter.ai/api/v1", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
		{Type: config.ProviderTypeOpenRouter, Name: "p2", APIKey: "key2", BaseURL: "https://openrouter.ai/api/v1", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
	}, nil, []config.Alias{{
		Name:             "a",
		Algorithm:        config.AlgorithmRoundRobin,
		RetryStatusCodes: []int{500, 502, 503, 504},
		Targets:          []config.AliasTarget{{Provider: "p1", Model: "m"}, {Provider: "p2", Model: "m"}},
	}})}
	adapter := &statusByAPIKeyAdapter{rules: map[string]int{"key1": http.StatusBadGateway}}
	h := newHandler(t, rt, adapter)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(`{"model":"alias/a","messages":[]}`)))
	h.ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (retried to p2)", w.Code)
	}
	if calls := adapter.Calls(); len(calls) != 2 || calls[0] != "key1" || calls[1] != "key2" {
		t.Fatalf("calls = %v, want [key1 key2]", calls)
	}
}
