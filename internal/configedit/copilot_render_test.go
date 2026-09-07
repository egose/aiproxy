package configedit

import (
	"strings"
	"testing"
)

func TestRenderProviderBlockCopilotCredentialRef(t *testing.T) {
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "github-copilot",
		Name:         "copilot",
		Credential:   ProviderCredentialInput{Mode: "credential_ref", CopilotPath: "/tmp/keys.json", CopilotName: "main"},
		Models:       []ProviderModelInput{{Name: "gpt-4o-mini", Capabilities: []string{"chat"}}},
	}, "/tmp/keys.json")
	for _, want := range []string{
		`provider "github-copilot" "copilot" {`,
		"credential_ref {",
		`name = "main"`,
		`model "gpt-4o-mini" {`,
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("rendered block missing %q:\n%s", want, block)
		}
	}
	if strings.Contains(block, "api_key") {
		t.Fatalf("copilot block must not render api_key:\n%s", block)
	}
	if strings.Contains(block, "path =") {
		t.Fatalf("default path must be omitted:\n%s", block)
	}
	if err := ValidateGeneratedConfig([]byte(block), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
	}
}

func TestRenderProviderBlockCopilotCustomPath(t *testing.T) {
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "github-copilot",
		Name:         "copilot",
		Credential:   ProviderCredentialInput{Mode: "credential_ref", CopilotPath: "/etc/aiproxy/keys.json", CopilotName: "team"},
		Models:       []ProviderModelInput{{Name: "gpt-4o-mini", Capabilities: []string{"chat"}}},
	}, "/home/user/.config/aiproxy/keys.json")
	if !strings.Contains(block, `path = "/etc/aiproxy/keys.json"`) {
		t.Fatalf("custom path missing:\n%s", block)
	}
	if !strings.Contains(block, `name = "team"`) {
		t.Fatalf("credential name missing:\n%s", block)
	}
	if err := ValidateGeneratedConfig([]byte(block), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
	}
}

func TestRenderProviderBlockCopilotDerivedStaysCompact(t *testing.T) {
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "github-copilot",
		Name:         "derived",
		Extends:      "base",
		DisplayName:  "Team",
		BaseURL:      "https://should-not-render.example",
		Credential:   ProviderCredentialInput{Mode: "credential_ref", CopilotPath: "/tmp/keys.json", CopilotName: "team"},
		Models:       []ProviderModelInput{{Name: "should-not-render"}},
	}, "/tmp/keys.json")
	for _, want := range []string{`extends = "base"`, `display_name = "Team"`, `name = "team"`} {
		if !strings.Contains(block, want) {
			t.Fatalf("derived copilot missing %q:\n%s", want, block)
		}
	}
	for _, forbidden := range []string{"base_url", "model ", "upstream_header_timeout", "enabled"} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("derived copilot rendered %q:\n%s", forbidden, block)
		}
	}
	if err := ValidateGeneratedConfig([]byte(block), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
	}
}
