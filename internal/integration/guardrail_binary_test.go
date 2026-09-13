//go:build integration && linux

package integration

import (
	"crypto/rand"
	"fmt"
	"net/http"
	"strings"
	"syscall"
	"testing"
)

func guardrailBinaryConfig(addr, upstreamURL, mode string) string {
	block := ""
	if mode != "" {
		block = fmt.Sprintf("\ningress_guardrails {\n  enabled = true\n  mode = %q\n}\n", mode)
	}
	return fmt.Sprintf(`
listener "http" "public" { address = %q }
auth "main" { mode = "none" }
%sprovider "openai-compatible" "a" {
  base_url = %q
  api_key  = "sk-a"
  model "chat" {
    upstream_name = "a-chat"
  }
}
`, addr, block, upstreamURL+"/v1")
}

func guardrailBinaryKey(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("rand: %v", err)
	}
	const alphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ234567"
	out := make([]byte, 16)
	for i := range out {
		out[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return "AKIA" + string(out)
}

func TestBinaryIngressGuardrailBlockAuditReload(t *testing.T) {
	upstream := newUpstreamStub(t)
	addr := freeAddr(t)
	configPath := writeConfig(t, guardrailBinaryConfig(addr, upstream.URL(), "block"))
	srv := startBinaryServer(t, configPath, addr)

	key := guardrailBinaryKey(t)
	secretBody := `{"model":"a/chat","messages":[{"role":"user","content":"deploy with ` + key + ` now"}]}`
	cleanBody := `{"model":"a/chat","messages":[{"role":"user","content":"explain photosynthesis"}]}`

	if status, body := postChat(t, srv, "", cleanBody); status != http.StatusOK || !strings.Contains(body, "chatcmpl_default") {
		t.Fatalf("clean chat = %d %q, want 200 forwarded", status, body)
	}
	if got := len(upstream.Calls()); got != 1 {
		t.Fatalf("upstream calls after clean = %d, want 1", got)
	}

	status, body := postChat(t, srv, "", secretBody)
	if status != http.StatusBadRequest || !strings.Contains(body, `"secret_blocked"`) {
		t.Fatalf("secret chat = %d %q, want 400 secret_blocked", status, body)
	}
	if strings.Contains(body, key) {
		t.Fatalf("blocked response leaks secret text")
	}
	if got := len(upstream.Calls()); got != 1 {
		t.Fatalf("upstream calls after block = %d, want still 1 (zero upstream I/O)", got)
	}

	writeConfigPath(t, configPath, guardrailBinaryConfig(addr, upstream.URL(), "audit"))
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("signal reload: %v", err)
	}
	srv.WaitLogContains(t, "config reloaded")
	if status, body := postChat(t, srv, "", secretBody); status != http.StatusOK {
		t.Fatalf("audit secret chat = %d %q, want 200 forwarded", status, body)
	}
	if got := len(upstream.Calls()); got != 2 {
		t.Fatalf("upstream calls after audit forward = %d, want 2", got)
	}

	writeConfigPath(t, configPath, guardrailBinaryConfig(addr, upstream.URL(), "watch"))
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("signal failed reload: %v", err)
	}
	srv.WaitLogContains(t, "config reload failed")
	if status, body := postChat(t, srv, "", secretBody); status != http.StatusOK {
		t.Fatalf("secret chat after failed reload = %d %q, want 200 (audit policy preserved)", status, body)
	}
	if got := len(upstream.Calls()); got != 3 {
		t.Fatalf("upstream calls after failed reload = %d, want 3", got)
	}
	srv.Stop(t)
}
