package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/guardrails"
)

func guardrailTestUpstream(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[]}`))
	}))
}

func guardrailTestConfig(upstreamURL, extra string) string {
	return `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
` + extra + `
provider "openai" "openai" {
  base_url = "` + upstreamURL + `"
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`
}

func TestBuildWithoutGuardrailsLeavesScannerNil(t *testing.T) {
	upstream := guardrailTestUpstream(t)
	defer upstream.Close()
	configPath := writeConfigFile(t, guardrailTestConfig(upstream.URL, ""))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	if a.guardrails != nil {
		t.Fatalf("scanner should be nil when guardrails are not configured")
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`))
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with guardrails disabled", w.Code)
	}
}

func TestBuildWithGuardrailsCompilesPolicy(t *testing.T) {
	upstream := guardrailTestUpstream(t)
	defer upstream.Close()
	configPath := writeConfigFile(t, guardrailTestConfig(upstream.URL, `
ingress_guardrails {
  enabled = true
  mode = "audit"
  max_text_bytes = 4096
  max_strings = 64
}
`))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	if a.guardrails == nil {
		t.Fatalf("scanner should be compiled when guardrails are enabled")
	}
	policy := a.guardrails.Policy()
	if policy.Mode != guardrails.ModeAudit || policy.MaxTextBytes != 4096 || policy.MaxStrings != 64 {
		t.Fatalf("policy = %+v", policy)
	}
}

func TestReloadSwapsGuardrailPolicy(t *testing.T) {
	upstream := guardrailTestUpstream(t)
	defer upstream.Close()
	configPath := writeConfigFile(t, guardrailTestConfig(upstream.URL, ""))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	if a.guardrails != nil {
		t.Fatalf("scanner should start nil")
	}
	if err := os.WriteFile(configPath, []byte(guardrailTestConfig(upstream.URL, `
ingress_guardrails {
  enabled = true
  mode = "block"
}
`)), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.guardrails == nil {
		t.Fatalf("scanner should be active after reload")
	}
	if mode := a.guardrails.Policy().Mode; mode != guardrails.ModeBlock {
		t.Fatalf("mode = %q, want block", mode)
	}
	if err := os.WriteFile(configPath, []byte(guardrailTestConfig(upstream.URL, "")), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.guardrails != nil {
		t.Fatalf("scanner should be nil after disabling guardrails")
	}
}

func TestFailedReloadPreservesGuardrailPolicy(t *testing.T) {
	upstream := guardrailTestUpstream(t)
	defer upstream.Close()
	configPath := writeConfigFile(t, guardrailTestConfig(upstream.URL, `
ingress_guardrails {
  enabled = true
  mode = "audit"
}
`))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	active := a.guardrails
	if active == nil {
		t.Fatalf("scanner should be active")
	}
	if err := os.WriteFile(configPath, []byte(guardrailTestConfig(upstream.URL, `
ingress_guardrails {
  enabled = true
  mode = "watch"
}
`)), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := a.Reload(); err == nil {
		t.Fatalf("expected reload to fail for invalid guardrail mode")
	}
	if a.guardrails != active {
		t.Fatalf("failed reload must preserve the active scanner")
	}
	if mode := a.guardrails.Policy().Mode; mode != guardrails.ModeAudit {
		t.Fatalf("mode = %q, want preserved audit policy", mode)
	}
}

func TestBuildWithQuarantineCreatesStore(t *testing.T) {
	upstream := guardrailTestUpstream(t)
	defer upstream.Close()
	configPath := writeConfigFile(t, guardrailTestConfig(upstream.URL, `
ingress_guardrails {
  enabled = true
  quarantine {
    enabled = true
    max_entries = 16
    ttl = "5m"
  }
}
`))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	if a.quarantine == nil {
		t.Fatalf("quarantine should be created when enabled")
	}
	policy := a.quarantine.Policy()
	if policy.MaxEntries != 16 {
		t.Fatalf("max entries = %d, want 16", policy.MaxEntries)
	}
}

func TestReloadPreservesQuarantineEntries(t *testing.T) {
	upstream := guardrailTestUpstream(t)
	defer upstream.Close()
	configPath := writeConfigFile(t, guardrailTestConfig(upstream.URL, `
ingress_guardrails {
  enabled = true
  quarantine {
    enabled = true
  }
}
`))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	if a.quarantine == nil {
		t.Fatalf("quarantine should be active")
	}
	a.quarantine.Store("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", guardrails.Capture{RuleIDs: []string{"x"}})
	before := a.quarantine
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.quarantine != before {
		t.Fatalf("unchanged quarantine config should preserve the store")
	}
	if _, ok := a.quarantine.Take("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); !ok {
		t.Fatalf("reload dropped quarantined entry")
	}
}

func TestReloadRebuildsQuarantineOnConfigChange(t *testing.T) {
	upstream := guardrailTestUpstream(t)
	defer upstream.Close()
	configPath := writeConfigFile(t, guardrailTestConfig(upstream.URL, `
ingress_guardrails {
  enabled = true
  quarantine {
    enabled = true
    max_entries = 16
  }
}
`))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	defer func() { _ = a.Close() }()
	before := a.quarantine
	if err := os.WriteFile(configPath, []byte(guardrailTestConfig(upstream.URL, `
ingress_guardrails {
  enabled = true
  quarantine {
    enabled = true
    max_entries = 32
  }
}
`)), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.quarantine == before {
		t.Fatalf("changed quarantine config should rebuild the store")
	}
	if got := a.quarantine.Policy().MaxEntries; got != 32 {
		t.Fatalf("max entries = %d, want 32", got)
	}
}
