package config

import (
	"strings"
	"testing"
)

func TestOpenRouterIsKnownProviderType(t *testing.T) {
	want := ProviderType("openrouter")
	for _, got := range ProviderTypes() {
		if got == want {
			return
		}
	}
	t.Fatalf("ProviderTypes() = %v, want it to contain %q", ProviderTypes(), want)
}

func TestOpenRouterLoadsWithoutBaseURL(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openrouter" "router" {
  api_key = "sk-test"
  model "openai/gpt-4o-mini" {}
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	p, ok := rt.Catalog.Provider("router")
	if !ok {
		t.Fatal("router provider missing from catalog")
	}
	if p.Type != ProviderTypeOpenRouter {
		t.Fatalf("provider type = %q, want openrouter", p.Type)
	}
}

func TestOpenRouterRejectsMissingCredential(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openrouter" "router" {
  model "openai/gpt-4o-mini" {}
}
`
	_, err := Load([]byte(cfg), "test.hcl")
	if err == nil || !strings.Contains(err.Error(), "enabled providers require a non-empty api_key") {
		t.Fatalf("expected enabled provider credential error, got %v", err)
	}
}

func TestOpenRouterDerivedProviderLoads(t *testing.T) {
	cfg := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openrouter" "base" {
  api_key = "sk-base"
  model "openai/gpt-4o-mini" {}
}
provider "openrouter" "derived" {
  extends = "base"
  api_key = "sk-derived"
}
`
	rt, err := Load([]byte(cfg), "test.hcl")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	derived, ok := rt.Catalog.Provider("derived")
	if !ok {
		t.Fatal("derived provider missing from catalog")
	}
	if derived.APIKey != "sk-derived" {
		t.Fatalf("derived api key = %q, want sk-derived", derived.APIKey)
	}
	if _, ok := derived.ModelByName["openai/gpt-4o-mini"]; !ok {
		t.Fatalf("derived models = %+v", derived.Models)
	}
}
