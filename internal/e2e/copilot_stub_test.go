package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/app"
	"github.com/egose/aiproxy/internal/copilotlogin"
)

func buildTestApp(configPath string) (*app.App, error) {
	return app.Build(context.Background(), app.BuildOptions{ConfigPath: configPath, Version: "test"})
}

type copilotCall struct {
	Path          string
	Authorization string
	UserAgent     string
	APIVersion    string
	Intent        string
	Initiator     string
	Vision        string
	Cookie        string
	APIKey        string
	InteractionID string
	Body          string
}

type copilotStub struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []copilotCall
}

func newCopilotStub(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, body string)) *copilotStub {
	t.Helper()
	s := &copilotStub{}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read stub body: %v", err)
		}
		s.mu.Lock()
		s.calls = append(s.calls, copilotCall{
			Path:          r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			UserAgent:     r.Header.Get("User-Agent"),
			APIVersion:    r.Header.Get("X-GitHub-Api-Version"),
			Intent:        r.Header.Get("Openai-Intent"),
			Initiator:     r.Header.Get("x-initiator"),
			Vision:        r.Header.Get("Copilot-Vision-Request"),
			Cookie:        r.Header.Get("Cookie"),
			APIKey:        r.Header.Get("x-api-key"),
			InteractionID: r.Header.Get("X-Interaction-Id"),
			Body:          string(raw),
		})
		s.mu.Unlock()
		handler(w, r, string(raw))
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *copilotStub) URL() string { return s.server.URL }

func (s *copilotStub) Calls() []copilotCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]copilotCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func writeCopilotSecrets(t *testing.T, name, token string) string {
	t.Helper()
	dir := t.TempDir()
	secretsPath := filepath.Join(dir, "keys.json")
	cred, err := copilotlogin.NewCredential("Ov23testclient", token, time.Now())
	if err != nil {
		t.Fatalf("new credential: %v", err)
	}
	if err := copilotlogin.Save(secretsPath, name, cred); err != nil {
		t.Fatalf("save credential: %v", err)
	}
	return secretsPath
}

func copilotE2EConfig(upstreamURL, secretsPath, name string) string {
	return `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "github-copilot" "copilot" {
  base_url = "` + upstreamURL + `"
  credential_ref {
    path = ` + fmt.Sprintf("%q", secretsPath) + `
    name = ` + fmt.Sprintf("%q", name) + `
  }
  model "gpt-4o-mini" {
    upstream_name = "gpt-4o-2024-08-06"
  }
}
`
}

func TestEndToEndCopilotDirectChatJSON(t *testing.T) {
	upstream := newCopilotStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl_cp","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"from-copilot"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`)
	})
	secretsPath := writeCopilotSecrets(t, "main", "gho_e2e-token")
	server := newTestServer(t, writeConfig(t, copilotE2EConfig(upstream.URL(), secretsPath, "main")))

	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/chat/completions", strings.NewReader(`{"model":"copilot/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer inbound-caller")
	req.Header.Set("Cookie", "s=1")
	req.Header.Set("x-initiator", "agent")
	resp, err := server.Client().Do(req)
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	calls := upstream.Calls()
	if len(calls) != 1 {
		t.Fatalf("calls = %d", len(calls))
	}
	call := calls[0]
	if call.Path != "/chat/completions" {
		t.Fatalf("path = %q", call.Path)
	}
	if call.Authorization != "Bearer gho_e2e-token" {
		t.Fatalf("auth = %q", call.Authorization)
	}
	if call.UserAgent != "aiproxy/test" {
		t.Fatalf("user-agent = %q", call.UserAgent)
	}
	if call.Initiator != "user" {
		t.Fatalf("initiator = %q, want user", call.Initiator)
	}
	if call.Vision != "" {
		t.Fatalf("vision = %q, want empty", call.Vision)
	}
	if call.Cookie != "" || call.InteractionID != "" {
		t.Fatalf("inbound metadata leaked: cookie=%q interaction=%q", call.Cookie, call.InteractionID)
	}
	if !strings.Contains(call.Body, `"model":"gpt-4o-2024-08-06"`) {
		t.Fatalf("model not rewritten: %s", call.Body)
	}
}

func TestEndToEndCopilotDirectChatSSE(t *testing.T) {
	upstream := newCopilotStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":1,\"total_tokens\":3}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	secretsPath := writeCopilotSecrets(t, "main", "gho_e2e-token")
	server := newTestServer(t, writeConfig(t, copilotE2EConfig(upstream.URL(), secretsPath, "main")))

	resp, err := server.Client().Post(server.URL+"/v1/chat/completions", "application/json", strings.NewReader(`{"model":"copilot/gpt-4o-mini","stream":true,"messages":[{"role":"user","content":"hi"}]}`))
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read sse: %v", err)
	}
	if !strings.Contains(string(body), "Hello") || !strings.Contains(string(body), "[DONE]") {
		t.Fatalf("unexpected sse body: %q", string(body))
	}
	if len(upstream.Calls()) != 1 || upstream.Calls()[0].Path != "/chat/completions" {
		t.Fatalf("unexpected upstream calls: %+v", upstream.Calls())
	}
}

func TestEndToEndCopilotRejectsUnsupportedOps(t *testing.T) {
	upstream := newCopilotStub(t, func(w http.ResponseWriter, r *http.Request, _ string) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	})
	secretsPath := writeCopilotSecrets(t, "main", "gho_e2e-token")
	server := newTestServer(t, writeConfig(t, copilotE2EConfig(upstream.URL(), secretsPath, "main")))

	for _, tc := range []struct{ path, body string }{
		{"/v1/embeddings", `{"model":"copilot/gpt-4o-mini","input":"hi"}`},
		{"/v1/responses", `{"model":"copilot/gpt-4o-mini","input":"hi"}`},
		{"/v1/images/generations", `{"model":"copilot/gpt-4o-mini","prompt":"cat"}`},
	} {
		resp, err := server.Client().Post(server.URL+tc.path, "application/json", strings.NewReader(tc.body))
		if err != nil {
			t.Fatalf("post %s: %v", tc.path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s: status = %d, want 400", tc.path, resp.StatusCode)
		}
		if !strings.Contains(string(body), "unsupported_operation") {
			t.Fatalf("%s: body = %s", tc.path, string(body))
		}
	}
	if len(upstream.Calls()) != 0 {
		t.Fatalf("upstream calls = %d, want 0", len(upstream.Calls()))
	}
}

func TestEndToEndCopilotMissingCredentialFailsBuild(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "keys.json")
	configPath := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(configPath, []byte(copilotE2EConfig("http://127.0.0.1:1", missing, "absent")), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := buildTestApp(configPath)
	if err == nil || !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("expected credential_ref build error, got %v", err)
	}
}
