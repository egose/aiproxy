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
	if err := ValidateGeneratedConfig([]byte(updated), "config.hcl"); err != nil {
		t.Fatalf("ValidateGeneratedConfig(): %v", err)
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
