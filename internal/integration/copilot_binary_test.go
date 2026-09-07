//go:build integration && linux

package integration

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
)

type copilotCall struct {
	Path          string
	Authorization string
	UserAgent     string
	Intent        string
	APIVersion    string
	Initiator     string
	Vision        string
	Cookie        string
	APIKey        string
	Body          string
}

type copilotStub struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []copilotCall
}

func newCopilotStub(t *testing.T) *copilotStub {
	t.Helper()
	s := &copilotStub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read copilot stub body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		s.mu.Lock()
		s.calls = append(s.calls, copilotCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			UserAgent:     r.Header.Get("User-Agent"),
			Intent:        r.Header.Get("Openai-Intent"),
			APIVersion:    r.Header.Get("X-GitHub-Api-Version"),
			Initiator:     r.Header.Get("x-initiator"),
			Vision:        r.Header.Get("Copilot-Vision-Request"),
			Cookie:        r.Header.Get("Cookie"),
			APIKey:        r.Header.Get("x-api-key"),
			Body:          string(raw),
		})
		s.mu.Unlock()
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if strings.Contains(string(raw), `"stream":true`) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"from-copilot-stream\"}}]}\n\ndata: [DONE]\n\n")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl_copilot","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-copilot"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`)
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *copilotStub) Calls() []copilotCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]copilotCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func writeCopilotSidecar(t *testing.T, secretsPath, name, token string) {
	t.Helper()
	cred, err := copilotlogin.NewCredential("test-client-id", token, time.Now())
	if err != nil {
		t.Fatalf("new copilot credential: %v", err)
	}
	if err := copilotlogin.Save(secretsPath, name, cred); err != nil {
		t.Fatalf("save copilot credential: %v", err)
	}
}

func copilotServeConfig(addr, baseURL, secretsPath string) string {
	return fmt.Sprintf(`
listener "http" "public" { address = %q }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  base_url = %q
  credential_ref {
    path = %q
    name = "binarytest"
  }
  model "test-chat" {
    display_name = "Binary Test Chat"
  }
}
alias "copilot_chat" {
  algorithm = "round_robin"
  target {
    provider = "copilot"
    model = "test-chat"
  }
}
`, addr, baseURL, secretsPath)
}

func TestBinaryGitHubCopilotChatServe(t *testing.T) {
	stub := newCopilotStub(t)
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(secretsPath, []byte("{}"), 0o600); err != nil {
		t.Fatalf("write keys file: %v", err)
	}
	writeCopilotSidecar(t, secretsPath, "binarytest", "test-copilot-token")
	addr := freeAddr(t)
	srv := startBinaryServer(t, writeConfig(t, copilotServeConfig(addr, stub.server.URL, secretsPath)), addr)

	status, body := postChat(t, srv, "", `{"model":"copilot/test-chat","messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK || !strings.Contains(body, "from-copilot") {
		t.Fatalf("copilot JSON chat = %d %q, want proxied response", status, body)
	}
	calls := stub.Calls()
	if len(calls) != 1 {
		t.Fatalf("copilot calls = %d, want 1", len(calls))
	}
	got := calls[0]
	if got.Path != "/chat/completions" {
		t.Fatalf("copilot path = %q, want /chat/completions", got.Path)
	}
	if got.Authorization != "Bearer test-copilot-token" {
		t.Fatalf("copilot authorization = %q", got.Authorization)
	}
	if !strings.HasPrefix(got.UserAgent, "aiproxy/") {
		t.Fatalf("copilot user-agent = %q, want aiproxy/ prefix", got.UserAgent)
	}
	if got.Intent == "" || got.APIVersion == "" || got.Initiator != "user" {
		t.Fatalf("copilot headers intent=%q version=%q initiator=%q", got.Intent, got.APIVersion, got.Initiator)
	}
	if got.Vision != "" {
		t.Fatalf("copilot vision header = %q, want empty for text-only body", got.Vision)
	}
	if got.Cookie != "" || got.APIKey != "" {
		t.Fatalf("copilot leaked inbound headers cookie=%q apikey=%q", got.Cookie, got.APIKey)
	}
	if !strings.Contains(got.Body, `"model":"test-chat"`) {
		t.Fatalf("copilot upstream body missing model rewrite: %s", got.Body)
	}

	status, body = postChat(t, srv, "", `{"model":"copilot/test-chat","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK || !strings.Contains(body, "from-copilot-stream") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("copilot SSE chat = %d %q, want streamed response", status, body)
	}

	before := len(stub.Calls())
	req, err := http.NewRequest(http.MethodPost, srv.baseURL+"/v1/responses", strings.NewReader(`{"model":"copilot/test-chat","input":"hi"}`))
	if err != nil {
		t.Fatalf("new responses request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.client.Do(req)
	if err != nil {
		t.Fatalf("post responses: %v", err)
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		t.Fatalf("read responses body: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest || !strings.Contains(string(respBody), "unsupported_operation") {
		t.Fatalf("copilot responses = %d %q, want 400 unsupported_operation", resp.StatusCode, respBody)
	}
	if got := len(stub.Calls()); got != before {
		t.Fatalf("rejected responses made upstream calls: before=%d after=%d", before, got)
	}

	if status, body := httpGet(t, srv, "/v1/models", ""); status != http.StatusOK || !strings.Contains(body, "copilot/test-chat") {
		t.Fatalf("GET /v1/models = %d %q, want copilot/test-chat", status, body)
	}
	if got := len(stub.Calls()); got != before {
		t.Fatalf("models endpoint made upstream calls: before=%d after=%d", before, got)
	}
	srv.Stop(t)
}

func TestBinaryGitHubCopilotLoginHasNoEndpointOverrides(t *testing.T) {
	binary := aiproxyBinary(t)
	help := exec.Command(binary, "login", "github-copilot", "--help")
	out, err := help.CombinedOutput()
	if err != nil {
		t.Fatalf("login --help: %v\n%s", err, out)
	}
	text := string(out)
	for _, want := range []string{"--client-id", "--credential"} {
		if !strings.Contains(text, want) {
			t.Fatalf("login --help missing %q:\n%s", want, text)
		}
	}
	for _, forbidden := range []string{"device-code-url", "token-url", "endpoint", "base-url", "client-secret"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("login --help must not expose %q:\n%s", forbidden, text)
		}
	}

	missing := exec.Command(binary, "login", "github-copilot", "--credential", "binarytest")
	missing.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir())
	out, err = missing.CombinedOutput()
	if err == nil {
		t.Fatalf("login without --client-id unexpectedly succeeded: %s", out)
	}
	if !strings.Contains(string(out), "--client-id") {
		t.Fatalf("login without --client-id error missing flag hint: %s", out)
	}
}
