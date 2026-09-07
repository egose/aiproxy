package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
)

type openCodeHTTPUpstream struct {
	server  *httptest.Server
	calls   atomic.Int32
	path    string
	auth    string
	ua      string
	session string
	muBody  string
	mu      chan string
}

func newOpenCodeHTTPUpstream(t *testing.T, status int, payload string) *openCodeHTTPUpstream {
	t.Helper()
	u := &openCodeHTTPUpstream{mu: make(chan string, 16)}
	u.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u.calls.Add(1)
		u.path = r.URL.Path
		u.auth = r.Header.Get("Authorization")
		u.ua = r.Header.Get("User-Agent")
		u.session = r.Header.Get("x-opencode-session")
		body, _ := io.ReadAll(r.Body)
		u.mu <- string(body)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
	}))
	t.Cleanup(u.server.Close)
	return u
}

func openCodeHTTPHandler(t *testing.T, rt *config.Runtime, version string) http.Handler {
	t.Helper()
	return NewHandler(Dependencies{
		Resolver: modelresolver.New(rt),
		Adapter:  provider.New(),
		Auth:     auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}),
		Catalog:  rt.Catalog,
		Metrics:  observability.NewMetrics(),
		Version:  version,
	})
}

func openCodeHTTPRuntime(t *testing.T, cfg string) *config.Runtime {
	t.Helper()
	rt, err := config.Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	return rt
}

func TestOpenCodeDirectDispatchRoutesByServiceAndProtocol(t *testing.T) {
	zen := newOpenCodeHTTPUpstream(t, http.StatusOK, `{"id":"chatcmpl-1","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	goUp := newOpenCodeHTTPUpstream(t, http.StatusOK, `{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	rt := openCodeHTTPRuntime(t, `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.server.URL+`"
  api_key = "sk-zen"
  model "minimax-m3" {
    protocol = "chat"
  }
}
provider "opencode-go" "go" {
  base_url = "`+goUp.server.URL+`"
  api_key = "sk-go"
  model "minimax-m3" {
    protocol = "messages"
  }
}
`)
	h := openCodeHTTPHandler(t, rt, "9.9.9")

	zenBody := `{"model":"zen/minimax-m3","messages":[{"role":"user","content":"hi"}]}`
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(zenBody)))
	if w.Code != http.StatusOK {
		t.Fatalf("zen status = %d body=%s", w.Code, w.Body.String())
	}
	if zen.calls.Load() != 1 || goUp.calls.Load() != 0 {
		t.Fatalf("calls zen=%d go=%d", zen.calls.Load(), goUp.calls.Load())
	}
	if zen.path != "/v1/chat/completions" {
		t.Fatalf("zen path = %q", zen.path)
	}
	if zen.auth != "Bearer sk-zen" || zen.ua != "aiproxy/9.9.9" || zen.session != "" {
		t.Fatalf("zen headers auth=%q ua=%q session=%q", zen.auth, zen.ua, zen.session)
	}

	goBody := `{"model":"go/minimax-m3","messages":[{"role":"user","content":"hi"}]}`
	w = httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(goBody))
	r.Header.Set("x-opencode-session", "client-session_1")
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("go status = %d body=%s", w.Code, w.Body.String())
	}
	if goUp.calls.Load() != 1 || zen.calls.Load() != 1 {
		t.Fatalf("calls zen=%d go=%d", zen.calls.Load(), goUp.calls.Load())
	}
	if goUp.path != "/messages" {
		t.Fatalf("go path = %q", goUp.path)
	}
	if goUp.auth != "Bearer sk-go" || goUp.session != "client-session_1" {
		t.Fatalf("go headers auth=%q session=%q", goUp.auth, goUp.session)
	}
	if !strings.Contains(w.Body.String(), `"content":"hi"`) {
		t.Fatalf("go response not translated: %s", w.Body.String())
	}
}

func TestOpenCodeDirectUnsupportedMakesZeroUpstreamCalls(t *testing.T) {
	zen := newOpenCodeHTTPUpstream(t, http.StatusOK, `{}`)
	rt := openCodeHTTPRuntime(t, `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.server.URL+`"
  api_key = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
  }
}
`)
	h := openCodeHTTPHandler(t, rt, "itest")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/responses", strings.NewReader(`{"model":"zen/glm-5.3","input":"hi"}`)))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "unsupported_operation") {
		t.Fatalf("body = %s", w.Body.String())
	}
	if zen.calls.Load() != 0 {
		t.Fatalf("upstream calls = %d", zen.calls.Load())
	}
}

func TestOpenCodeAliasFailoverAcrossServices(t *testing.T) {
	first := newOpenCodeHTTPUpstream(t, http.StatusBadGateway, `{"error":{"message":"overloaded"}}`)
	second := newOpenCodeHTTPUpstream(t, http.StatusOK, `{"id":"chatcmpl-2","choices":[],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	rt := openCodeHTTPRuntime(t, `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+first.server.URL+`"
  api_key = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
  }
}
provider "opencode-go" "go" {
  base_url = "`+second.server.URL+`"
  api_key = "sk-go"
  model "glm-5.3" {
    protocol = "chat"
  }
}
alias "fallback" {
  algorithm = "round_robin"
  target {
    provider = "zen"
    model = "glm-5.3"
  }
  target {
    provider = "go"
    model = "glm-5.3"
  }
}
`)
	h := openCodeHTTPHandler(t, rt, "itest")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"alias/fallback","messages":[]}`)))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", w.Code, w.Body.String())
	}
	if first.calls.Load() != 1 || second.calls.Load() != 1 {
		t.Fatalf("calls first=%d second=%d", first.calls.Load(), second.calls.Load())
	}
	if second.session == "" {
		t.Fatal("go failover target missing session header")
	}
}

func TestOpenCodeModelsStayProxyOwned(t *testing.T) {
	zen := newOpenCodeHTTPUpstream(t, http.StatusOK, `{}`)
	rt := openCodeHTTPRuntime(t, `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  base_url = "`+zen.server.URL+`"
  api_key = "sk-zen"
  model "glm-5.3" {
    protocol = "chat"
  }
}
provider "opencode-go" "go" {
  base_url = "`+zen.server.URL+`"
  api_key = "sk-go"
  model "minimax-m3" {
    protocol = "messages"
  }
}
`)
	h := openCodeHTTPHandler(t, rt, "itest")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"id":"zen/glm-5.3"`) || !strings.Contains(body, `"id":"go/minimax-m3"`) {
		t.Fatalf("models = %s", body)
	}
	if zen.calls.Load() != 0 {
		t.Fatalf("models endpoint called upstream %d times", zen.calls.Load())
	}
}
