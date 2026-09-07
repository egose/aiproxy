package configedit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertBlockPreservesDocumentShape(t *testing.T) {
	source := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

provider "openai" "primary" {
  api_key = env("OPENAI_API_KEY")

  model "gpt-4o-mini" {}
}`) + "\n"

	block := RenderProviderBlock(ProviderInput{
		ProviderType: "openai-compatible",
		Name:         "primary",
		BaseURL:      "https://llm.internal/v1",
		Credential:   ProviderCredentialInput{Mode: "inline", APIKeyValue: `env("LOCALAI_API_KEY")`},
		Models:       []ProviderModelInput{{Name: "qwen3-32b", Capabilities: []string{"chat"}}},
	}, "/unused/keys.json")

	updated, err := UpsertBlock(source, block, func(candidate TopLevelBlock) bool {
		return candidate.Type == "provider" && len(candidate.Labels) >= 2 && candidate.Labels[1] == "primary"
	})
	if err != nil {
		t.Fatalf("UpsertBlock(): %v", err)
	}
	if !strings.Contains(updated, `listener "http" "public"`) {
		t.Fatalf("listener block not preserved:\n%s", updated)
	}
	if strings.Count(updated, `provider "`) != 1 {
		t.Fatalf("expected exactly one provider block:\n%s", updated)
	}
	if !strings.Contains(updated, `api_key = env("LOCALAI_API_KEY")`) {
		t.Fatalf("env expression was not preserved:\n%s", updated)
	}
	if strings.Contains(updated, "enabled = false") {
		t.Fatalf("enabled = false should not render for enabled provider:\n%s", updated)
	}
	if err := ValidateGeneratedConfig([]byte(updated), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
	}
}

func TestRenderProviderBlockOpenCodeProtocol(t *testing.T) {
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "opencode-go",
		Name:         "go",
		BaseURL:      "http://127.0.0.1:18081",
		Credential:   ProviderCredentialInput{Mode: "inline", APIKeyValue: `env("OPENCODE_GO_API_KEY")`},
		Models: []ProviderModelInput{
			{Name: "minimax-m3", Protocol: "messages", Capabilities: []string{"chat", "responses"}},
			{Name: "glm-5.3", UpstreamName: "glm-5.3", Protocol: "chat", Capabilities: []string{"chat"}},
		},
	}, "/unused/keys.json")
	for _, want := range []string{
		`provider "opencode-go" "go" {`,
		`base_url = "http://127.0.0.1:18081"`,
		`api_key = env("OPENCODE_GO_API_KEY")`,
		`model "minimax-m3" {`,
		`protocol = "messages"`,
		`capabilities = ["chat", "responses"]`,
		`model "glm-5.3" {`,
		`protocol = "chat"`,
		`capabilities = ["chat"]`,
	} {
		if !strings.Contains(block, want) {
			t.Fatalf("rendered block missing %q:\n%s", want, block)
		}
	}
	if err := ValidateGeneratedConfig([]byte(block), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
	}
	updated, err := UpsertBlock("", block, func(candidate TopLevelBlock) bool {
		return candidate.Type == "provider" && len(candidate.Labels) >= 2 && candidate.Labels[1] == "go"
	})
	if err != nil {
		t.Fatalf("UpsertBlock(): %v", err)
	}
	if !strings.Contains(updated, `protocol = "messages"`) || !strings.Contains(updated, `base_url = "http://127.0.0.1:18081"`) {
		t.Fatalf("round trip dropped protocol or override:\n%s", updated)
	}
}

func TestRenderProviderBlockDisabled(t *testing.T) {
	disabled := false
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "openai",
		Name:         "backup",
		Enabled:      &disabled,
		Credential:   ProviderCredentialInput{Mode: "disabled"},
	}, "/unused/keys.json")
	if !strings.Contains(block, "enabled = false") {
		t.Fatalf("expected enabled = false marker:\n%s", block)
	}
	if strings.Contains(block, "api_key") {
		t.Fatalf("disabled provider must not render credential block:\n%s", block)
	}
	if strings.Contains(block, "model ") {
		t.Fatalf("disabled provider should not render models:\n%s", block)
	}
	if err := ValidateGeneratedConfig([]byte(block), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
	}
}

func TestRenderProviderBlockDerivedStaysCompact(t *testing.T) {
	block := RenderProviderBlock(ProviderInput{
		ProviderType: "openai-compatible",
		Name:         "nvidia-2",
		Extends:      "nvidia-1",
		DisplayName:  "Nvidia - corean",
		BaseURL:      "https://should-not-render.example/v1",
		Credential:   ProviderCredentialInput{Mode: "secrets_file", SecretsKey: "nvidia-2"},
		Models:       []ProviderModelInput{{Name: "should-not-render"}},
	}, "/home/user/.config/aiproxy/keys.json")
	for _, want := range []string{`extends = "nvidia-1"`, `display_name = "Nvidia - corean"`, `key  = "nvidia-2"`} {
		if !strings.Contains(block, want) {
			t.Fatalf("derived provider missing %q:\n%s", want, block)
		}
	}
	for _, forbidden := range []string{"base_url", "model ", "upstream_header_timeout", "enabled"} {
		if strings.Contains(block, forbidden) {
			t.Fatalf("derived provider rendered %q:\n%s", forbidden, block)
		}
	}
	if err := ValidateGeneratedConfig([]byte(block), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
	}
}

func TestAvailableProviderModelsIncludesDerivedProviderModels(t *testing.T) {
	blocks, err := ParseTopLevelBlocks(`
provider "openai-compatible" "nvidia-2" {
  extends = "nvidia-1"
  api_key = "k"
}

provider "openai-compatible" "nvidia-1" {
  base_url = "https://integrate.api.nvidia.com/v1"
  api_key = "k"
  model "z-ai/glm-5.2" {}
}
`)
	if err != nil {
		t.Fatalf("ParseTopLevelBlocks(): %v", err)
	}
	models := AvailableProviderModels(blocks)
	if len(models) != 2 || models[0] != "nvidia-1/z-ai/glm-5.2" || models[1] != "nvidia-2/z-ai/glm-5.2" {
		t.Fatalf("models = %#v", models)
	}
}

func TestWriteProviderFilesUpdatesConfigAndSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")
	if err := os.WriteFile(secretsPath, []byte("{\n  \"old\": \"value\"\n}\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	source := RenderProviderBlock(ProviderInput{
		ProviderType: "openai",
		Name:         "primary",
		Credential:   ProviderCredentialInput{Mode: "secrets_file", SecretsPath: secretsPath, SecretsKey: "primary"},
		Models:       []ProviderModelInput{{Name: "gpt-4o-mini", Capabilities: []string{"chat", "responses"}}},
	}, filepath.Join(dir, "default-keys.json"))

	if err := WriteProviderFiles(configPath, source, SecretsUpdate{Path: secretsPath, Key: "primary", Value: "sk-test"}); err != nil {
		t.Fatalf("WriteProviderFiles(): %v", err)
	}
	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	if !strings.Contains(string(configData), `provider "openai" "primary"`) {
		t.Fatalf("config missing provider block:\n%s", string(configData))
	}
	secretsData, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("ReadFile(secrets): %v", err)
	}
	if !strings.Contains(string(secretsData), `"old": "value"`) || !strings.Contains(string(secretsData), `"primary": "sk-test"`) {
		t.Fatalf("secrets update did not preserve and add keys:\n%s", string(secretsData))
	}
}
