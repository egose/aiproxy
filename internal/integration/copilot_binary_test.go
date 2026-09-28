//go:build integration && linux

package integration

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/copilotlogin"
)

type copilotCall struct {
	Method        string
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
	server        *httptest.Server
	mu            sync.Mutex
	calls         []copilotCall
	revokedStatus atomic.Int32
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
			Method:        r.Method,
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
		if status := s.revokedStatus.Load(); status != 0 && r.Header.Get("Authorization") == "Bearer synthetic-original" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(int(status))
			_, _ = io.WriteString(w, `{"error":"synthetic-revoked"}`)
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/models" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"data":[{"id":"upstream-chat"},{"id":"discovered-only"}]}`)
			return
		}
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

func TestBinaryGitHubCopilotDiscoveryAndRecovery(t *testing.T) {
	guard := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("binary attempted unexpected non-loopback HTTP request: %s %s", r.Method, r.Host)
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(guard.Close)
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(key, guard.URL)
	}
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		t.Setenv(key, "127.0.0.1,::1")
	}
	t.Setenv("AIPROXY_CONFIG", "")
	for _, failure := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(fmt.Sprint(failure), func(t *testing.T) {
			stub := newCopilotStub(t)
			dir := t.TempDir()
			secrets := filepath.Join(dir, "keys.json")
			writeCopilotSidecar(t, secrets, "binarytest", "synthetic-original")
			addr := freeAddr(t)
			text := strings.Replace(copilotServeConfig(addr, stub.server.URL, secrets), `display_name = "Binary Test Chat"`, `upstream_name = "upstream-chat"`, 1)
			configPath := writeConfig(t, text)
			srv := startBinaryServer(t, configPath, addr)
			checkCall := func(before int, method, path, token string) {
				t.Helper()
				calls := stub.Calls()
				if len(calls) != before+1 {
					t.Fatalf("calls=%d want=%d (no silent retries)", len(calls), before+1)
				}
				call := calls[before]
				if call.Method != method || call.Path != path || call.Authorization != "Bearer "+token || !strings.HasPrefix(call.UserAgent, "aiproxy/") || call.APIVersion != copilotlogin.APIVersion {
					t.Fatal("incorrect method/path or shared credential headers")
				}
				if method == http.MethodPost && !strings.Contains(call.Body, `"model":"upstream-chat"`) {
					t.Fatal("incorrect model rewrite")
				}
			}
			models := func(token string, wantStatus int) {
				t.Helper()
				before := len(stub.Calls())
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, aiproxyBinary(t), "models", "--config", configPath, "--provider", "copilot", "--upstream")
				cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+dir)
				out, err := cmd.CombinedOutput()
				if wantStatus == 200 {
					if err != nil || !strings.Contains(string(out), "upstream-chat (configured as copilot/test-chat)") || !strings.Contains(string(out), "discovered-only (not in config)") {
						t.Fatalf("models: %v %s", err, out)
					}
				} else if err == nil || !strings.Contains(string(out), fmt.Sprint(wantStatus)) || !strings.Contains(string(out), "login") {
					t.Fatalf("missing model discovery re-login guidance: %v %s", err, out)
				}
				if strings.Contains(string(out), token) {
					t.Fatal("models exposed synthetic credential")
				}
				checkCall(before, http.MethodGet, "/models", token)
			}
			chat := func(token string, wantStatus int) {
				t.Helper()
				for _, stream := range []bool{false, true} {
					before := len(stub.Calls())
					status, body := postChat(t, srv, "", fmt.Sprintf(`{"model":"copilot/test-chat","stream":%t,"messages":[{"role":"user","content":"hi"}]}`, stream))
					if status != wantStatus {
						t.Fatalf("chat stream=%t: %d %s", stream, status, body)
					}
					if wantStatus == 200 {
						if !strings.Contains(body, "from-copilot") || (stream && !strings.Contains(body, "[DONE]")) {
							t.Fatal("incomplete JSON/SSE response")
						}
					} else if body != `{"error":"synthetic-revoked"}` {
						t.Fatal("auth error not passed through verbatim")
					}
					checkCall(before, http.MethodPost, "/chat/completions", token)
				}
			}
			models("synthetic-original", 200)
			chat("synthetic-original", 200)
			stub.revokedStatus.Store(int32(failure))
			models("synthetic-original", failure)
			chat("synthetic-original", failure)
			writeCopilotSidecar(t, secrets, "binarytest", "synthetic-rotated")
			models("synthetic-rotated", 200)
			chat("synthetic-original", failure)
			sidecar, err := copilotlogin.SidecarPath(secrets, "binarytest")
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(sidecar, []byte(`{invalid`), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
				t.Fatal(err)
			}
			srv.WaitLogContains(t, "config reload failed")
			chat("synthetic-original", failure)
			writeCopilotSidecar(t, secrets, "binarytest", "synthetic-rotated")
			if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
				t.Fatal(err)
			}
			srv.WaitLogContains(t, "config reloaded")
			chat("synthetic-rotated", 200)
			models("synthetic-rotated", 200)
			before := len(stub.Calls())
			if status, body := httpGet(t, srv, "/v1/models", ""); status != 200 || !strings.Contains(body, "copilot/test-chat") || strings.Contains(body, "discovered-only") {
				t.Fatalf("static inventory changed: %d %s", status, body)
			}
			if len(stub.Calls()) != before {
				t.Fatal("proxy model listing queried upstream")
			}
			srv.Stop(t)
		})
	}
}
