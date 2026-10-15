package httpapi

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func TestHandlerForwardAuthorization(t *testing.T) {
	for _, model := range []string{"test/m", "alias/a"} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", model, streaming), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Header.Get("Authorization") != "Bearer client-secret" {
						t.Error("client authorization not forwarded")
					}
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = w.Write([]byte("data: [DONE]\n\n"))
					} else {
						_, _ = w.Write([]byte(`{"choices":[]}`))
					}
				}))
				defer upstream.Close()
				rt, err := config.Load([]byte(fmt.Sprintf(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "test" {
  base_url = %q
  forward_authorization = true
  model "m" {}
}

alias "a" {
  algorithm = "round_robin"
  target {
    provider = "test"
    model = "m"
  }
}`, upstream.URL)), "test.hcl")
				if err != nil {
					t.Fatal(err)
				}
				h := newHandler(t, rt, provider.New())
				for _, token := range []string{"", "Bearer client-secret"} {
					r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[],"stream":%t}`, model, streaming)))
					if token != "" {
						r.Header.Set("Authorization", token)
					}
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					want := http.StatusOK
					if token == "" {
						want = http.StatusBadRequest
						if calls.Load() != 0 {
							t.Fatal("missing header reached upstream")
						}
					}
					if w.Code != want {
						t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
					}
				}
				if calls.Load() != 1 {
					t.Fatalf("upstream calls=%d, want 1", calls.Load())
				}
			})
		}
	}
}

func TestForwardAuthorizationDoesNotBypassProxyAuth(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Header.Get("Authorization") != "Bearer proxy-token" {
			t.Error("proxy token must be forwarded unchanged")
		}
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer upstream.Close()
	rt := &config.Runtime{Catalog: config.NewCatalog([]config.Provider{{
		Type: config.ProviderTypeOpenAI, Name: "test", BaseURL: upstream.URL,
		ForwardAuthorization: true, Models: []config.Model{{Name: "m", UpstreamName: "m"}},
	}}, nil, nil)}
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt), Catalog: rt.Catalog, Adapter: provider.New(),
		Auth: auth.NewAuthenticator(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"client": {Name: "client", Token: "proxy-token"}}}),
	})
	for _, token := range []string{"upstream-token", "proxy-token"} {
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test/m","messages":[]}`))
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		want := http.StatusOK
		if token == "upstream-token" {
			want = http.StatusUnauthorized
			if calls.Load() != 0 {
				t.Fatal("unauthenticated client reached upstream")
			}
		}
		if w.Code != want {
			t.Fatalf("status=%d want=%d body=%s", w.Code, want, w.Body.String())
		}
	}
}
