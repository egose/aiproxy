package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestLoadForwardAuthorization(t *testing.T) {
	for _, kind := range []ProviderType{ProviderTypeOpenAI, ProviderTypeOpenAICompatible, ProviderTypeOpenCodeZen, ProviderTypeOpenCodeGo, ProviderTypeZenMux, ProviderTypeOpenRouter} {
		t.Run(string(kind), func(t *testing.T) {
			protocol := ""
			if IsOpenCodeProviderType(kind) {
				protocol = `protocol = "chat"`
			}
			cfg := fmt.Sprintf(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider %q "test" {
  base_url = "https://example.com/v1"
  forward_authorization = true
  model "m" { %s }
}`, kind, protocol)
			rt, err := Load([]byte(cfg), "test.hcl")
			if err != nil {
				t.Fatal(err)
			}
			p, ok := rt.Catalog.Provider("test")
			if !ok || !p.ForwardAuthorization || p.APIKey != "" || p.APIKeyRef != nil {
				t.Fatalf("unexpected provider: present=%t forwarding=%t key=%t ref=%t", ok, p.ForwardAuthorization, p.APIKey != "", p.APIKeyRef != nil)
			}
			if err := ValidateDynamicProvider(p); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestLoadForwardAuthorizationRejectsInvalidConfig(t *testing.T) {
	for _, tc := range []struct {
		name, kind, extra, want string
	}{
		{"key", "openai", `api_key = "configured"`, "cannot be combined"},
		{"reference", "openai", `api_key_ref { key = "key" }`, "cannot be combined"},
		{"anthropic", "anthropic", "", "not supported"},
		{"gemini", "gemini", "", "not supported"},
		{"copilot", "github-copilot", "", "not supported"},
		{"disabled-conflict", "openai", "enabled = false\napi_key = \"configured\"", "cannot be combined"},
		{"generic-headers", "openai", `forward_headers = ["Authorization"]`, "managed by the proxy"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := fmt.Sprintf(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider %q "test" {
  forward_authorization = true
  %s
  model "m" {}
}`, tc.kind, tc.extra)
			_, err := Load([]byte(cfg), "test.hcl")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
	_, err := Load([]byte(`forward_authorization = true
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "test" { api_key = "key"
model "m" {} }`), "test.hcl")
	if err == nil {
		t.Fatal("root forward_authorization must be rejected")
	}
}

func TestLoadDerivedForwardAuthorizationIsLocal(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "base" {
  forward_authorization = true
  model "m" {}
}
provider "openai" "forward" {
  extends = "base"
  forward_authorization = true
}
provider "openai" "fixed" {
  extends = "base"
  api_key = "local-key"
}`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	forward, _ := rt.Catalog.Provider("forward")
	fixed, _ := rt.Catalog.Provider("fixed")
	if !forward.ForwardAuthorization || fixed.ForwardAuthorization || fixed.APIKey != "local-key" {
		t.Fatal("derived provider authorization mode must be explicitly local")
	}
}
