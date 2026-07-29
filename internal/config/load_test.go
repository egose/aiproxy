package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestLoadMinimalConfig(t *testing.T) {
	cfg := `
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.Listener.Address != ":8080" {
		t.Errorf("address = %q, want :8080", rt.Listener.Address)
	}
	if rt.Auth.Mode != AuthModeNone {
		t.Errorf("auth mode = %q", rt.Auth.Mode)
	}
	if len(rt.Providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(rt.Providers))
	}
	p := rt.Providers[0]
	if p.Type != ProviderTypeOpenAI {
		t.Errorf("provider type = %q", p.Type)
	}
	if p.APIKey != "sk-test" {
		t.Errorf("api_key = %q", p.APIKey)
	}
	if len(p.Models) != 1 || p.Models[0].Name != "gpt-4o-mini" {
		t.Errorf("models = %+v", p.Models)
	}
	if p.ModelByName["gpt-4o-mini"].UpstreamName != "gpt-4o-mini" {
		t.Errorf("upstream name default mismatch")
	}
}

func TestLoadUpstreamNameDefaults(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "anthropic" "anthropic" {
  api_key = "k"
  model "claude" { upstream_name = "claude-sonnet-4-20250514" }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p := rt.Providers[0]
	m := p.ModelByName["claude"]
	if m.UpstreamName != "claude-sonnet-4-20250514" {
		t.Errorf("upstream = %q", m.UpstreamName)
	}
}

func TestLoadModelCapabilities(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  model "text-embedding-3-large" {
    capabilities = ["embeddings"]
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	got := rt.Providers[0].ModelByName["text-embedding-3-large"].Capabilities
	if len(got) != 1 || got[0] != CapabilityEmbeddings {
		t.Fatalf("capabilities = %+v", got)
	}
}

func TestLoadClientTenantAndAllowedModels(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "bearer_static"
  client "ci" {
    token = "tok"
    tenant = "team-a"
    allowed_models = ["openai/gpt-4o-mini", "alias/chat_default"]
  }
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	client := rt.Auth.Clients["ci"]
	if client.Tenant != "team-a" {
		t.Fatalf("tenant = %q", client.Tenant)
	}
	if len(client.AllowedModels) != 2 || client.AllowedModels[1] != "alias/chat_default" {
		t.Fatalf("allowed_models = %+v", client.AllowedModels)
	}
}

func TestLoadProviderHealthConfig(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider_health {
  redis_url = "redis://127.0.0.1:6379"
  key_prefix = "aiproxy:test"
  cooldown = "45s"
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.ProviderHealth.RedisURL != "redis://127.0.0.1:6379" || rt.ProviderHealth.KeyPrefix != "aiproxy:test" || rt.ProviderHealth.Cooldown != 45*time.Second {
		t.Fatalf("provider_health = %+v", rt.ProviderHealth)
	}
}

func TestLoadOpenAICompatibleRequiresBaseURL(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  api_key = "k"
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "base_url is required") {
		t.Fatalf("expected base_url error, got %v", err)
	}
}

func TestLoadRejectsNegativeListenerTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name    string
		field   string
		message string
	}{
		{name: "read_header", field: `read_header = "-1s"`, message: "invalid read_header timeout"},
		{name: "idle", field: `idle = "-1s"`, message: "invalid idle timeout"},
		{name: "write", field: `write = "-1s"`, message: "invalid write timeout"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := `
listener "http" "public" {
  address = ":8080"
  timeouts {
    ` + tc.field + `
  }
}
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
			_, err := Load([]byte(cfg), "test.hcl")
			if err == nil || !strings.Contains(err.Error(), tc.message) {
				t.Fatalf("expected %q error, got %v", tc.message, err)
			}
		})
	}
}

func TestLoadRejectsBothAPIKeyAndRef(t *testing.T) {
	keyFile := writeTempFile(t, "keys.json", `{"k":"v"}`)
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "inline"
  api_key_ref {
    path = "` + keyFile + `"
    key  = "k"
  }
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "only one of api_key") {
		t.Fatalf("expected only-one error, got %v", err)
	}
}

func TestLoadAPIKeyRefWithDefaultPath(t *testing.T) {
	xdgRoot := t.TempDir()
	keyDir := filepath.Join(xdgRoot, "aiproxy")
	if err := os.MkdirAll(keyDir, 0o755); err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(keyDir, "keys.json")
	if err := os.WriteFile(keyFile, []byte(`{"openai":"sk-from-file","local":"lk"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgRoot)
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key_ref {
    key = "openai"
  }
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.Providers[0].APIKey != "sk-from-file" {
		t.Errorf("resolved key = %q", rt.Providers[0].APIKey)
	}
	if !rt.Providers[0].APIKeyRef.Resolved {
		t.Errorf("Resolved flag not set")
	}
}

func TestLoadSkipsProviderWithEmptyAPIKey(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = ""
  model "gpt-4o-mini" {}
}
provider "openai" "backup" {
  api_key = "sk-backup"
  model "gpt-4o-mini" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(rt.Providers) != 1 || rt.Providers[0].Name != "backup" {
		t.Fatalf("active providers = %+v", rt.Providers)
	}
	if len(rt.DisabledProviders) != 1 || rt.DisabledProviders[0].Name != "openai" {
		t.Fatalf("disabled providers = %+v", rt.DisabledProviders)
	}
	if _, ok := rt.ProviderByName["openai"]; ok {
		t.Fatalf("disabled provider unexpectedly present in ProviderByName")
	}
}

func TestLoadDuplicateProvider(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
provider "openai" "p" {
  api_key = "k2"
  model "m2" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "duplicate provider") {
		t.Fatalf("expected duplicate error, got %v", err)
	}
}

func TestLoadAuthBearerStaticRequiresClients(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "bearer_static" }
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "at least one client") {
		t.Fatalf("expected client-required error, got %v", err)
	}
}

func TestLoadAuthRateLimitDefaultsBurst(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "none"
  rate_limit {
    requests_per_minute = 120
  }
}
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.Auth.RateLimit == nil {
		t.Fatal("expected auth rate limit")
	}
	if rt.Auth.RateLimit.RequestsPerMinute != 120 || rt.Auth.RateLimit.Burst != 120 {
		t.Fatalf("rate limit = %+v", rt.Auth.RateLimit)
	}
}

func TestLoadRejectsInvalidAuthRateLimit(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "none"
  rate_limit {
    requests_per_minute = 0
  }
}
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "rate_limit.requests_per_minute") {
		t.Fatalf("expected rate limit validation error, got %v", err)
	}
}

func TestLoadAliasUnknownTarget(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
alias "a" {
  algorithm = "round_robin"
  target {
    provider = "p"
    model    = "missing"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "not defined on provider") {
		t.Fatalf("expected unknown-target error, got %v", err)
	}
}

func TestLoadInvalidAlgorithm(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
alias "a" {
  algorithm = "bogus"
  target {
    provider = "p"
    model    = "m"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "invalid algorithm") {
		t.Fatalf("expected invalid algorithm error, got %v", err)
	}
}

func TestLoadRejectsAliasWithoutSharedCapability(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4.1" {
    capabilities = ["responses"]
  }
}
provider "gemini" "gemini" {
  api_key = "k"
  model "gemini-2.5-pro" {
    capabilities = ["chat"]
  }
}
alias "mixed" {
  algorithm = "round_robin"
  target {
    provider = "openai"
    model    = "gpt-4.1"
  }
  target {
    provider = "gemini"
    model    = "gemini-2.5-pro"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "do not share any capabilities") {
		t.Fatalf("expected shared-capability error, got %v", err)
	}
}

func TestLoadRejectsUppercaseProviderName(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "OpenAI" {
  api_key = "k"
  model "m" {}
}
	`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "lowercase") {
		t.Fatalf("expected lowercase error, got %v", err)
	}
}

func TestLoadRejectsInvalidCapability(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "k"
  model "m" {
    capabilities = ["vision"]
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "invalid capability") {
		t.Fatalf("expected invalid capability error, got %v", err)
	}
}

func TestLoadRejectsUnsupportedCapabilityForProviderType(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "anthropic" "anthropic" {
  api_key = "k"
  model "claude-sonnet" {
    capabilities = ["embeddings"]
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "is not supported by provider type") {
		t.Fatalf("expected unsupported capability error, got %v", err)
	}
}
func TestEnvExpansion(t *testing.T) {
	t.Setenv("AIPROXY_TEST_KEY", "sk-from-env")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = env("AIPROXY_TEST_KEY")
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.Providers[0].APIKey != "sk-from-env" {
		t.Errorf("api_key = %q", rt.Providers[0].APIKey)
	}
}

func TestEnvExpansionEscapesQuotedSecrets(t *testing.T) {
	t.Setenv("AIPROXY_TEST_KEY", "sk-\"quoted\"\\value")
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = env("AIPROXY_TEST_KEY")
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.Providers[0].APIKey != "sk-\"quoted\"\\value" {
		t.Errorf("api_key = %q", rt.Providers[0].APIKey)
	}
}
