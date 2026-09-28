package httpapi

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
)

func TestPublicEndpointsKeyExpirationWithoutReload(t *testing.T) {
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if body.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"id\":\"fixture\"}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fixture","usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(upstream.Close)
	deadline := time.Date(2030, 1, 2, 3, 4, 5, 123456000, time.UTC)
	var clock atomic.Int64
	clock.Store(deadline.Add(-time.Nanosecond).UnixNano())
	rt := newRT()
	p := testProvider(t, rt, "openai")
	p.BaseURL = upstream.URL
	replaceProviders(rt, []config.Provider{p})
	cfg := config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"static": {Token: "static"}}}
	dyn := []auth.DynamicClient{
		{Name: "expiring", TokenHash: store.TokenHash("expiring"), ExpiresAt: deadline},
		{Name: "permanent", TokenHash: store.TokenHash("permanent")},
	}
	agg := accounting.NewAggregator()
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt), Catalog: rt.Catalog, Adapter: provider.New(),
		Auth:       auth.NewAuthenticatorWithClientsAndClock(cfg, dyn, func() time.Time { return time.Unix(0, clock.Load()) }),
		Authorizer: auth.NewAuthorizerWithClients(cfg, dyn),
		Metrics:    observability.NewMetrics(), Accounting: agg, Usage: agg,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	routes := []struct {
		name, method, path, body string
		stream                   bool
	}{
		{name: "models", method: http.MethodGet, path: "/v1/models"},
		{name: "billing", method: http.MethodGet, path: "/v1/billing/usage"},
		{name: "chat-json", method: http.MethodPost, path: "/v1/chat/completions", body: `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`},
		{name: "chat-sse", method: http.MethodPost, path: "/v1/chat/completions", body: `{"model":"alias/chat_default","messages":[{"role":"user","content":"hi"}],"stream":true}`, stream: true},
		{name: "responses-json", method: http.MethodPost, path: "/v1/responses", body: `{"model":"openai/gpt-4o-mini","input":"hi"}`},
		{name: "responses-sse", method: http.MethodPost, path: "/v1/responses", body: `{"model":"openai/gpt-4o-mini","input":"hi","stream":true}`, stream: true},
	}
	for _, offset := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		clock.Store(deadline.Add(offset).UnixNano())
		for _, route := range routes {
			t.Run(offset.String()+"/"+route.name, func(t *testing.T) {
				for _, token := range []string{"expiring", "permanent", "static", "unknown"} {
					before := calls.Load()
					r := httptest.NewRequest(route.method, route.path, strings.NewReader(route.body))
					r.Header.Set("Authorization", "Bearer "+token)
					r.Header.Set("Content-Type", "application/json")
					if route.stream {
						r.Header.Set("Accept", "text/event-stream")
					}
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					rejected := token == "unknown" || token == "expiring" && offset >= 0
					if rejected {
						if w.Code != http.StatusUnauthorized || w.Header().Get("Content-Type") != "application/json" {
							t.Errorf("%s: response = %d %s %s", token, w.Code, w.Header().Get("Content-Type"), w.Body.String())
						}
						var body struct {
							Error struct {
								Type    string `json:"type"`
								Message string `json:"message"`
							} `json:"error"`
						}
						if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body.Error.Type != "auth_failed" || body.Error.Message != auth.ErrInvalidToken.Error() {
							t.Errorf("%s: expected standard invalid-token error, got %s (decode: %v)", token, w.Body.String(), err)
						}
					} else {
						if w.Code != http.StatusOK {
							t.Errorf("%s: response = %d %s", token, w.Code, w.Body.String())
						}
						if route.stream && (!strings.HasPrefix(w.Header().Get("Content-Type"), "text/event-stream") || !strings.Contains(w.Body.String(), "data: [DONE]")) {
							t.Errorf("%s: expected SSE, got %s", token, w.Body.String())
						}
					}
					wantCalls := int64(0)
					if !rejected && route.method == http.MethodPost {
						wantCalls = 1
					}
					if got := calls.Load() - before; got != wantCalls {
						t.Errorf("%s: upstream calls = %d, want %d", token, got, wantCalls)
					}
				}
			})
		}
	}
}
