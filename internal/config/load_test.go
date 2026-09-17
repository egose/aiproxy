package config

import (
	"os"
	"path/filepath"
	"strconv"
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

func testProvider(t *testing.T, rt *Runtime, name string) Provider {
	t.Helper()
	provider, ok := rt.Catalog.Provider(name)
	if !ok {
		t.Fatalf("provider %q not found", name)
	}
	return provider
}

func testAlias(t *testing.T, rt *Runtime, name string) Alias {
	t.Helper()
	alias, ok := rt.Catalog.Alias(name)
	if !ok {
		t.Fatalf("alias %q not found", name)
	}
	return alias
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
	if rt.Logging.Level != LogLevelInfo || !rt.Logging.AccessLog {
		t.Fatalf("logging = %+v", rt.Logging)
	}
	providers := rt.Catalog.Providers()
	if len(providers) != 1 {
		t.Fatalf("expected 1 provider, got %d", len(providers))
	}
	p := providers[0]
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
	if p.UpstreamHeaderTimeout != DefaultUpstreamHeaderTimeout {
		t.Errorf("upstream header timeout = %v", p.UpstreamHeaderTimeout)
	}
}

func TestLoadUpstreamHeaderTimeoutPrecedence(t *testing.T) {
	cfg := `
upstream_header_timeout = "120s"

listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "primary" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
provider "openai" "slow" {
  upstream_header_timeout = "180s"
  enabled = false
  api_key = ""
  model "gpt-4o" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if rt.UpstreamHeaderTimeout != 120*time.Second {
		t.Fatalf("root upstream header timeout = %v", rt.UpstreamHeaderTimeout)
	}
	if got := testProvider(t, rt, "primary").UpstreamHeaderTimeout; got != 120*time.Second {
		t.Fatalf("primary timeout = %v", got)
	}
	disabledProviders := rt.Catalog.DisabledProviders()
	if len(disabledProviders) != 1 {
		t.Fatalf("disabled providers = %d", len(disabledProviders))
	}
	if got := disabledProviders[0].UpstreamHeaderTimeout; got != 180*time.Second {
		t.Fatalf("disabled provider timeout = %v", got)
	}
}

func TestLoadRejectsInvalidUpstreamHeaderTimeouts(t *testing.T) {
	for _, tc := range []struct {
		name string
		cfg  string
		want string
	}{
		{name: "root malformed", cfg: `upstream_header_timeout = "later"`, want: "invalid upstream_header_timeout"},
		{name: "root zero", cfg: `upstream_header_timeout = "0s"`, want: "invalid upstream_header_timeout"},
		{name: "root negative", cfg: `upstream_header_timeout = "-1s"`, want: "invalid upstream_header_timeout"},
		{name: "provider malformed", cfg: `provider "openai" "openai" { upstream_header_timeout = "later" }`, want: `provider "openai": invalid upstream_header_timeout`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
` + tc.cfg + `
provider "openai" "fallback" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
			_, err := Load([]byte(cfg), "test.hcl")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadLoggingConfig(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
logging {
  level = "warn"
  access_log = false
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
	if rt.Logging.Level != LogLevelWarn || rt.Logging.AccessLog {
		t.Fatalf("logging = %+v", rt.Logging)
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
	p := rt.Catalog.Providers()[0]
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
	got := rt.Catalog.Providers()[0].ModelByName["text-embedding-3-large"].Capabilities
	if len(got) != 1 || got[0] != CapabilityEmbeddings {
		t.Fatalf("capabilities = %+v", got)
	}
}

func TestLoadAllowsProviderModelNamesWithSlash(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "nvidia" {
  base_url = "https://integrate.api.nvidia.com/v1"
  api_key = "k"
  model "z-ai/glm-5.2" {
    capabilities = ["chat"]
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	providers := rt.Catalog.Providers()
	if _, ok := providers[0].ModelByName["z-ai/glm-5.2"]; !ok {
		t.Fatalf("model with slash not loaded: %+v", providers[0].ModelByName)
	}
	if providers[0].Models[0].UpstreamName != "z-ai/glm-5.2" {
		t.Fatalf("upstream_name = %q", providers[0].Models[0].UpstreamName)
	}
}

func TestLoadDerivedProviderInheritsBaseAndUsesLocalCredential(t *testing.T) {
	secretsPath := writeTempFile(t, "keys.json", `{"derived":"sk-derived"}`)
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }

provider "openai-compatible" "derived" {
  extends      = "base"
  display_name = "Derived"
  api_key_ref {
    path = "` + secretsPath + `"
    key  = "derived"
  }
}

provider "openai-compatible" "base" {
  display_name = "Base"
  base_url = "https://integrate.api.nvidia.com/v1"
  upstream_header_timeout = "30s"
  api_key = "sk-base"
  model "z-ai/glm-5.2" {
    display_name = "GLM 5.2"
    upstream_name = "upstream-glm"
    capabilities = ["chat", "responses"]
  }
}

alias "chat" {
  algorithm = "round_robin"
  target {
    provider = "derived"
    model    = "z-ai/glm-5.2"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	derived := testProvider(t, rt, "derived")
	if derived.Type != ProviderTypeOpenAICompatible || derived.BaseURL != "https://integrate.api.nvidia.com/v1" || derived.UpstreamHeaderTimeout != 30*time.Second {
		t.Fatalf("derived inherited fields = %+v", derived)
	}
	if derived.DisplayName != "Derived" || derived.APIKey != "sk-derived" {
		t.Fatalf("derived local fields = %+v", derived)
	}
	model := derived.ModelByName["z-ai/glm-5.2"]
	if model.UpstreamName != "upstream-glm" || len(model.Capabilities) != 2 || model.Capabilities[1] != CapabilityResponses {
		t.Fatalf("derived model = %+v", model)
	}
	derived.Models[0].Capabilities[0] = CapabilityEmbeddings
	if testProvider(t, rt, "base").Models[0].Capabilities[0] != CapabilityChat {
		t.Fatalf("derived model capabilities shared with base")
	}
}

func TestLoadDerivedProviderAcceptsInlineLocalCredential(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "base" {
  api_key = "sk-base"
  model "gpt-4o-mini" {}
}
provider "openai" "derived" {
  extends = "base"
  api_key = "sk-derived"
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	derived := testProvider(t, rt, "derived")
	if derived.APIKey != "sk-derived" {
		t.Fatalf("derived api key = %q", derived.APIKey)
	}
	if _, ok := derived.ModelByName["gpt-4o-mini"]; !ok {
		t.Fatalf("derived models = %+v", derived.Models)
	}
}

func TestLoadRejectsInvalidDerivedProviders(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{name: "missing base", body: `provider "openai" "child" {
  extends = "missing"
  api_key = "k"
}`, want: `provider "child": extends "missing" is not defined`},
		{name: "self", body: `provider "openai" "child" {
  extends = "child"
  api_key = "k"
}`, want: `provider "child": extends "child" references itself`},
		{name: "chain", body: `provider "openai" "base" {
  extends = "root"
  api_key = "k"
}
provider "openai" "child" {
  extends = "base"
  api_key = "k"
}
provider "openai" "root" {
  api_key = "k"
  model "gpt-4o-mini" {}
}`, want: `provider "child": extends "base" references derived provider "base"`},
		{name: "type mismatch", body: `provider "anthropic" "child" {
  extends = "base"
  api_key = "k"
}
provider "openai" "base" {
  api_key = "k"
  model "gpt-4o-mini" {}
}`, want: `provider "child": type "anthropic" must match base provider "base" type "openai"`},
		{name: "disabled base", body: `provider "openai" "child" {
  extends = "base"
  api_key = "k"
}
provider "openai" "base" {
  enabled = false
  api_key = ""
  model "gpt-4o-mini" {}
}`, want: `provider "child": extends "base" references a disabled provider`},
		{name: "forbidden empty base_url", body: `provider "openai" "child" {
  extends = "base"
  base_url = ""
  api_key = "k"
}
provider "openai" "base" {
  api_key = "k"
  model "gpt-4o-mini" {}
}`, want: `provider "child": derived provider cannot declare base_url`},
		{name: "forbidden model", body: `provider "openai" "child" {
  extends = "base"
  api_key = "k"
  model "gpt-4o-mini" {}
}
provider "openai" "base" {
  api_key = "k"
  model "gpt-4o-mini" {}
}`, want: `provider "child": derived provider cannot declare model blocks`},
		{name: "no credential", body: `provider "openai" "child" { extends = "base" }
provider "openai" "base" {
  api_key = "k"
  model "gpt-4o-mini" {}
}`, want: `provider "child": derived provider requires exactly one local credential`},
		{name: "both credentials", body: `provider "openai" "child" {
  extends = "base"
  api_key = "k"
  api_key_ref { key = "child" }
}
provider "openai" "base" {
  api_key = "k"
  model "gpt-4o-mini" {}
}`, want: `provider "child": derived provider requires exactly one local credential`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
` + tc.body + `
`
			_, err := Load([]byte(cfg), "test.hcl")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestLoadRejectsProviderModelNamesWithEmptySlashSegment(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  model "z-ai/" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "each '/'-separated segment") {
		t.Fatalf("expected slash-segment validation error, got %v", err)
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
  cache_ttl = "60s"
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
	if rt.ProviderHealth.RedisURL != "redis://127.0.0.1:6379" || rt.ProviderHealth.KeyPrefix != "aiproxy:test" || rt.ProviderHealth.Cooldown != 45*time.Second || rt.ProviderHealth.CacheTTL != 60*time.Second {
		t.Fatalf("provider_health = %+v", rt.ProviderHealth)
	}
}

func TestLoadRejectsNonPositiveProviderHealthCacheTTL(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider_health {
  redis_url = "redis://127.0.0.1:6379"
  cache_ttl = "0s"
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err == nil || !strings.Contains(err.Error(), "provider_health.cache_ttl must be positive") {
		t.Fatalf("expected positive cache_ttl error, got %v", err)
	}
}

func TestLoadRejectsNegativeProviderHealthCooldownWithoutRedis(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider_health {
  cooldown = "-1s"
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "cooldown must not be negative") {
		t.Fatalf("expected negative cooldown error, got %v", err)
	}
}

func TestLoadRejectsMalformedProviderHealthRedisURL(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider_health {
  redis_url = "localhost:6379"
  key_prefix = "aiproxy:test"
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "redis_url must be a valid Redis URL") {
		t.Fatalf("expected malformed redis_url error, got %v", err)
	}
}

func TestLoadInvalidLoggingLevel(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
logging {
  level = "verbose"
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "logging: invalid level") {
		t.Fatalf("expected invalid logging level error, got %v", err)
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

func TestLoadRejectsMalformedProviderBaseURL(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  base_url = "not-a-url"
  api_key = "k"
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "base_url must be an absolute") {
		t.Fatalf("expected malformed base_url error, got %v", err)
	}
}

func TestLoadRejectsRemoteHTTPProviderBaseURL(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  base_url = "http://example.com/v1"
  api_key = "k"
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "non-HTTPS base_url") {
		t.Fatalf("expected remote http base_url error, got %v", err)
	}
}

func TestLoadAllowsLoopbackHTTPProviderBaseURL(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  base_url = "http://127.0.0.1:11434/v1"
  api_key = "k"
  model "m" {}
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err != nil {
		t.Fatalf("load: %v", err)
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

func TestLoadRejectsURLShapedListenerAddress(t *testing.T) {
	for _, address := range []string{"http://127.0.0.1:8080", "https://dashboard.example.com:8443"} {
		t.Run(address, func(t *testing.T) {
			cfg := `
listener "http" "public" { address = "` + address + `" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
			_, err := Load([]byte(cfg), "test.hcl")
			if err == nil || !strings.Contains(err.Error(), "not a URL") {
				t.Fatalf("expected URL-shaped listener address error, got %v", err)
			}
		})
	}
}

func TestLoadRejectsInvalidListenerAddress(t *testing.T) {
	cfg := `
listener "http" "public" { address = "127.0.0.1" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "host:port") {
		t.Fatalf("expected host:port listener address error, got %v", err)
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
	providers := rt.Catalog.Providers()
	if providers[0].APIKey != "sk-from-file" {
		t.Errorf("resolved key = %q", providers[0].APIKey)
	}
	if !providers[0].APIKeyRef.Resolved {
		t.Errorf("Resolved flag not set")
	}
}

func TestLoadSkipsProviderWithEmptyAPIKey(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  enabled = false
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
	providers := rt.Catalog.Providers()
	if len(providers) != 1 || providers[0].Name != "backup" {
		t.Fatalf("active providers = %+v", providers)
	}
	disabledProviders := rt.Catalog.DisabledProviders()
	if len(disabledProviders) != 1 || disabledProviders[0].Name != "openai" {
		t.Fatalf("disabled providers = %+v", disabledProviders)
	}
	if _, ok := rt.Catalog.Provider("openai"); ok {
		t.Fatalf("disabled provider unexpectedly present in active catalog")
	}
}

func TestLoadSkipsProviderWithMissingCredential(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  enabled = false
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
	disabledProviders := rt.Catalog.DisabledProviders()
	if len(disabledProviders) != 1 || disabledProviders[0].Name != "openai" {
		t.Fatalf("disabled providers = %+v", disabledProviders)
	}
}

func TestLoadRejectsInvalidDisabledProviderStructure(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "bogus" "bad" {
  enabled = false
  api_key = ""
  model "m" {}
}
provider "openai" "backup" {
  api_key = "sk-backup"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "unsupported type") {
		t.Fatalf("expected disabled provider structure error, got %v", err)
	}
}

func TestLoadRejectsEnabledProviderWithMissingCredential(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = ""
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "enabled providers require a non-empty api_key") {
		t.Fatalf("expected enabled provider credential error, got %v", err)
	}
}

func TestLoadRejectsEnabledProviderWithMissingAPIKeyRef(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "enabled providers require a non-empty api_key") {
		t.Fatalf("expected enabled provider credential error, got %v", err)
	}
}

func TestLoadAcceptsKeylessEnabledZenProvider(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  model "mimo-v2.5-free" {
    protocol = "chat"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("keyless zen provider should load: %v", err)
	}
	p, ok := rt.Catalog.Provider("zen")
	if !ok {
		t.Fatal("zen provider missing from catalog")
	}
	if p.APIKey != "" {
		t.Fatalf("zen APIKey = %q, want empty", p.APIKey)
	}
}

func TestLoadUserAgentOverride(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  api_key = "k"
  user_agent = "opencode/local"
  model "m" {
    protocol = "chat"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("user_agent override should load: %v", err)
	}
	p, ok := rt.Catalog.Provider("zen")
	if !ok {
		t.Fatal("zen provider missing from catalog")
	}
	if p.UserAgent != "opencode/local" {
		t.Fatalf("zen UserAgent = %q, want override", p.UserAgent)
	}
}

func TestLoadUserAgentOverrideOnNonOpenCodeProvider(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  user_agent = "opencode/local"
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("user_agent override should load on non-OpenCode providers: %v", err)
	}
	p, ok := rt.Catalog.Provider("openai")
	if !ok {
		t.Fatal("openai provider missing from catalog")
	}
	if p.UserAgent != "opencode/local" {
		t.Fatalf("openai UserAgent = %q, want override", p.UserAgent)
	}
}

func TestLoadForwardUserAgent(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  forward_user_agent = true
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("forward_user_agent should load: %v", err)
	}
	p, ok := rt.Catalog.Provider("openai")
	if !ok {
		t.Fatal("openai provider missing from catalog")
	}
	if !p.ForwardUserAgent {
		t.Fatal("openai ForwardUserAgent = false, want true")
	}
}

func TestLoadRootUserAgentInheritedWhenProviderUnset(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
user_agent = "root-agent/1.0"
forward_user_agent = true
provider "openai" "plain" {
  api_key = "k"
  model "m" {}
}
provider "openai" "custom" {
  api_key = "k"
  user_agent = "provider-agent/2.0"
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("root user_agent should load: %v", err)
	}
	if rt.UserAgent != "root-agent/1.0" {
		t.Fatalf("root UserAgent = %q, want root-agent/1.0", rt.UserAgent)
	}
	if !rt.ForwardUserAgent {
		t.Fatal("root ForwardUserAgent = false, want true")
	}
	plain, ok := rt.Catalog.Provider("plain")
	if !ok {
		t.Fatal("plain provider missing from catalog")
	}
	if plain.UserAgent != "root-agent/1.0" {
		t.Fatalf("plain UserAgent = %q, want inherited root-agent/1.0", plain.UserAgent)
	}
	if !plain.ForwardUserAgent {
		t.Fatal("plain ForwardUserAgent = false, want inherited true")
	}
	custom, ok := rt.Catalog.Provider("custom")
	if !ok {
		t.Fatal("custom provider missing from catalog")
	}
	if custom.UserAgent != "provider-agent/2.0" {
		t.Fatalf("custom UserAgent = %q, want provider override", custom.UserAgent)
	}
}

func TestLoadRootUserAgentRejectsInvalidValue(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
user_agent = "bad\nagent"
provider "openai" "plain" {
  api_key = "k"
  model "m" {}
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err == nil {
		t.Fatal("invalid root user_agent should fail validation")
	}
}

func TestLoadDerivedProviderRejectsForwardUserAgent(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "base" {
  api_key = "k"
  model "m" {}
}
provider "openai" "derived" {
  extends = "base"
  api_key = "k2"
  forward_user_agent = true
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err == nil {
		t.Fatal("derived provider declaring forward_user_agent should fail validation")
	}
}

func TestLoadRejectsUserAgentWithNewline(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "opencode-zen" "zen" {
  api_key = "k"
  user_agent = "bad\nagent"
  model "m" {
    protocol = "chat"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "user_agent must be") {
		t.Fatalf("expected user_agent charset error, got %v", err)
	}
}

func TestLoadRejectsKeylessEnabledGoProvider(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "opencode-go" "go" {
  model "minimax-m3" {
    protocol = "messages"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "enabled providers require a non-empty api_key") {
		t.Fatalf("expected enabled provider credential error, got %v", err)
	}
}

func TestLoadAcceptsDisabledProviderWithoutCredential(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  enabled = false
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
	disabledProviders := rt.Catalog.DisabledProviders()
	if len(disabledProviders) != 1 || disabledProviders[0].Name != "openai" {
		t.Fatalf("disabled providers = %+v", disabledProviders)
	}
	if disabledProviders[0].Enabled {
		t.Fatalf("disabled provider should preserve Enabled=false")
	}
}

func TestLoadDisabledProviderPreservesEnabledFalse(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  enabled = false
  api_key = "sk-still"
  model "gpt-4o-mini" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	disabledProviders := rt.Catalog.DisabledProviders()
	if len(disabledProviders) != 1 {
		t.Fatalf("disabled providers = %+v", disabledProviders)
	}
	if disabledProviders[0].Enabled {
		t.Fatalf("Enabled should be false on disabled provider")
	}
	if disabledProviders[0].APIKey != "sk-still" {
		t.Fatalf("APIKey should be preserved, got %q", disabledProviders[0].APIKey)
	}
}

func TestLoadAliasPrunesDisabledProviderTargets(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "primary" {
  api_key = "sk-primary"
  model "gpt-4o-mini" {}
}
provider "openai" "disabled-a" {
  enabled = false
  model "gpt-4o-mini" {}
}
provider "openai" "backup" {
  api_key = "sk-backup"
  model "gpt-4o-mini" {}
}
provider "openai" "disabled-b" {
  enabled = false
  api_key = ""
  model "gpt-4o-mini" {}
}
alias "chat" {
  algorithm = "round_robin"
  target {
    provider = "disabled-a"
    model    = "gpt-4o-mini"
  }
  target {
    provider = "backup"
    model    = "gpt-4o-mini"
  }
  target {
    provider = "disabled-b"
    model    = "gpt-4o-mini"
  }
  target {
    provider = "primary"
    model    = "gpt-4o-mini"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "chat")
	want := []AliasTarget{{Provider: "backup", Model: "gpt-4o-mini"}, {Provider: "primary", Model: "gpt-4o-mini"}}
	if len(a.Targets) != len(want) {
		t.Fatalf("targets = %+v, want %+v", a.Targets, want)
	}
	for i := range want {
		if a.Targets[i] != want[i] {
			t.Fatalf("targets = %+v, want %+v", a.Targets, want)
		}
	}
}

func TestLoadRejectsAliasWithOnlyDisabledProviderTargets(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "primary" {
  api_key = "sk-primary"
  model "gpt-4o-mini" {}
}
provider "openai" "disabled-a" {
  enabled = false
  model "gpt-4o-mini" {}
}
provider "openai" "disabled-b" {
  enabled = false
  api_key = ""
  model "gpt-4o-mini" {}
}
alias "chat" {
  algorithm = "round_robin"
  target {
    provider = "disabled-a"
    model    = "gpt-4o-mini"
  }
  target {
    provider = "disabled-b"
    model    = "gpt-4o-mini"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	want := `alias "chat": at least one target is required`
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestLoadRejectsReservedAliasProviderName(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "alias" {
  api_key = "k"
  model "m" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "reserved for alias model routing") {
		t.Fatalf("expected reserved alias provider error, got %v", err)
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
	if got := rt.Catalog.Providers()[0].APIKey; got != "sk-from-env" {
		t.Errorf("api_key = %q", got)
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
	if got := rt.Catalog.Providers()[0].APIKey; got != "sk-\"quoted\"\\value" {
		t.Errorf("api_key = %q", got)
	}
}

func TestLoadAliasRetryStatusCodes(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
alias "a" {
  algorithm          = "round_robin"
  retry_status_codes = ["429", "503"]
  target {
    provider = "p"
    model    = "m"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "a")
	if len(a.RetryStatusCodes) != 2 {
		t.Fatalf("retry_status_codes = %v, want 2 entries", a.RetryStatusCodes)
	}
	if a.RetryStatusCodes[0] != 429 || a.RetryStatusCodes[1] != 503 {
		t.Errorf("retry_status_codes = %v, want [429 503]", a.RetryStatusCodes)
	}
}

func TestLoadAliasRetryStatusCodesDefault(t *testing.T) {
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
    model    = "m"
  }
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	a := testAlias(t, rt, "a")
	want := []int{500, 502, 503, 504}
	if len(a.RetryStatusCodes) != len(want) {
		t.Fatalf("retry_status_codes = %v, want %v", a.RetryStatusCodes, want)
	}
	for i, c := range a.RetryStatusCodes {
		if c != want[i] {
			t.Errorf("retry_status_codes[%d] = %d, want %d", i, c, want[i])
		}
	}
}

func TestLoadAliasRetryStatusCodesInvalid(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
alias "a" {
  algorithm          = "round_robin"
  retry_status_codes = ["429", "abc"]
  target {
    provider = "p"
    model    = "m"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "invalid retry status code") {
		t.Fatalf("expected invalid retry status code error, got %v", err)
	}
}

func TestLoadAliasRetryStatusCodesOutOfRange(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
alias "a" {
  algorithm          = "round_robin"
  retry_status_codes = ["429", "999"]
  target {
    provider = "p"
    model    = "m"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "must be between 400 and 599") {
		t.Fatalf("expected out-of-range error, got %v", err)
	}
}

func TestLoadAliasRetryStatusCodesRejectsSuccessfulStatus(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "p" {
  api_key = "k"
  model "m" {}
}
alias "a" {
  algorithm          = "round_robin"
  retry_status_codes = ["200"]
  target {
    provider = "p"
    model    = "m"
  }
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "must be between 400 and 599") {
		t.Fatalf("expected successful retry status error, got %v", err)
	}
}

func TestLoadRejectsMetricsBlockWithoutToken(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
metrics {}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "metrics: token is required") {
		t.Fatalf("expected metrics token required error, got %v", err)
	}
}

func TestLoadAcceptsMetricsBlockWithToken(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
metrics {
  token = "scrape-secret"
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
	if !rt.Metrics.Enabled {
		t.Fatalf("Metrics.Enabled = false, want true")
	}
	if rt.Metrics.Token != "scrape-secret" {
		t.Fatalf("Metrics.Token = %q, want scrape-secret", rt.Metrics.Token)
	}
}

func TestLoadRejectsMultipleMetricsBlocks(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
metrics { token = "a" }
metrics { token = "b" }
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "only one metrics block is supported") {
		t.Fatalf("expected single metrics block error, got %v", err)
	}
}

func TestLoadRejectsDashboardInsecureRemoteWithMintedToken(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
dashboard {
  allow_insecure_remote = true
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "allow_insecure_remote is unsupported") {
		t.Fatalf("expected insecure remote token error, got %v", err)
	}
}

func TestLoadRejectsDashboardInsecureRemoteWithWeakToken(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
dashboard {
  token = "short"
  allow_insecure_remote = true
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "allow_insecure_remote is unsupported") {
		t.Fatalf("expected unsupported allow_insecure_remote error, got %v", err)
	}
}

func TestLoadRejectsDashboardInsecureRemoteWithStrongToken(t *testing.T) {
	token := strings.Repeat("a", 40)
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
dashboard {
  token = "` + token + `"
  allow_insecure_remote = true
}
provider "openai" "openai" {
  api_key = "k"
  model "gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "allow_insecure_remote is unsupported") {
		t.Fatalf("expected unsupported allow_insecure_remote error, got %v", err)
	}
}

func TestLoadForwardHeaders(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  forward_headers = ["X-Session-Id", "X-Session-Affinity"]
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("forward_headers should load: %v", err)
	}
	p, ok := rt.Catalog.Provider("openai")
	if !ok {
		t.Fatal("openai provider missing from catalog")
	}
	if len(p.ForwardHeaders) != 2 || p.ForwardHeaders[0] != "X-Session-Id" || p.ForwardHeaders[1] != "X-Session-Affinity" {
		t.Fatalf("ForwardHeaders = %v", p.ForwardHeaders)
	}
}

func TestLoadForwardHeadersRejectsManaged(t *testing.T) {
	for _, name := range []string{"Authorization", "X-Api-Key", "X-Goog-Api-Key", "Cookie", "Content-Type", "Accept", "User-Agent", "Content-Length", "Connection", "Transfer-Encoding", "Host", "Upgrade", "not a header", ""} {
		t.Run(name, func(t *testing.T) {
			cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "k"
  forward_headers = [` + strconv.Quote(name) + `]
  model "m" {}
}
`
			if _, err := Load([]byte(cfg), "test.hcl"); err == nil {
				t.Fatalf("forward_headers entry %q should fail validation", name)
			}
		})
	}
}

func TestLoadRootForwardHeadersUnion(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
forward_headers = ["X-Session-Id", "X-Root-Only"]
provider "openai" "plain" {
  api_key = "k"
  model "m" {}
}
provider "openai" "custom" {
  api_key = "k"
  forward_headers = ["x-session-id", "X-Provider-Only"]
  model "m" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("root forward_headers should load: %v", err)
	}
	plain, ok := rt.Catalog.Provider("plain")
	if !ok {
		t.Fatal("plain provider missing from catalog")
	}
	if len(plain.ForwardHeaders) != 2 || plain.ForwardHeaders[0] != "X-Session-Id" || plain.ForwardHeaders[1] != "X-Root-Only" {
		t.Fatalf("plain ForwardHeaders = %v", plain.ForwardHeaders)
	}
	custom, ok := rt.Catalog.Provider("custom")
	if !ok {
		t.Fatal("custom provider missing from catalog")
	}
	if len(custom.ForwardHeaders) != 3 || custom.ForwardHeaders[0] != "X-Session-Id" || custom.ForwardHeaders[1] != "X-Root-Only" || custom.ForwardHeaders[2] != "X-Provider-Only" {
		t.Fatalf("custom ForwardHeaders = %v, want root-first deduped union", custom.ForwardHeaders)
	}
}

func TestLoadRootForwardHeadersRejectsInvalid(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
forward_headers = ["Authorization"]
provider "openai" "plain" {
  api_key = "k"
  model "m" {}
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err == nil {
		t.Fatal("invalid root forward_headers should fail validation")
	}
}

func TestLoadDerivedProviderRejectsForwardHeaders(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "base" {
  api_key = "k"
  forward_headers = ["X-Session-Id"]
  model "m" {}
}
provider "openai" "derived" {
  extends = "base"
  api_key = "k2"
  forward_headers = ["X-Other"]
}
`
	if _, err := Load([]byte(cfg), "test.hcl"); err == nil {
		t.Fatal("derived provider declaring forward_headers should fail validation")
	}
}

func TestLoadDerivedProviderInheritsForwardHeaders(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "base" {
  api_key = "k"
  forward_headers = ["X-Session-Id"]
  model "m" {}
}
provider "openai" "derived" {
  extends = "base"
  api_key = "k2"
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("derived forward_headers inheritance should load: %v", err)
	}
	derived, ok := rt.Catalog.Provider("derived")
	if !ok {
		t.Fatal("derived provider missing from catalog")
	}
	if len(derived.ForwardHeaders) != 1 || derived.ForwardHeaders[0] != "X-Session-Id" {
		t.Fatalf("derived ForwardHeaders = %v", derived.ForwardHeaders)
	}
}
