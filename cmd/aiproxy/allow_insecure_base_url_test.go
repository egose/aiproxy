package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestConfigureProviderNonInteractiveAllowInsecureBaseURL(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")
	seed := "listener \"http\" \"public\" { address = \":8080\" }\nauth \"main\" { mode = \"none\" }\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openai-compatible",
		"--name", "incluster",
		"--base-url", "http://llm.internal.svc.cluster.local/v1",
		"--allow-insecure-base-url",
		"--secrets-path", secretsPath,
		"--secrets-key", "localai",
		"--api-key", "secret-value",
		"--model", "m",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	for _, want := range []string{
		`base_url = "http://llm.internal.svc.cluster.local/v1"`,
		"allow_insecure_base_url = true",
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("config output missing %q:\n%s", want, configText)
		}
	}
	if _, err := config.LoadFile(configPath); err != nil {
		t.Fatalf("generated config does not validate: %v", err)
	}
}

func TestConfigureProviderEditPreservesAllowInsecureBaseURL(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")
	seed := strings.TrimSpace(`listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai-compatible" "incluster" {
  base_url = "http://llm.internal.svc.cluster.local/v1"
  allow_insecure_base_url = true
  api_key_ref {
    path = "`+secretsPath+`"
    key  = "localai"
  }

  model "m" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}
	if err := os.WriteFile(secretsPath, []byte(`{"localai": "secret-value"}`), 0o600); err != nil {
		t.Fatalf("WriteFile(secrets): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openai-compatible",
		"--name", "incluster",
		"--display-name", "In-cluster LLM",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	for _, want := range []string{
		`base_url = "http://llm.internal.svc.cluster.local/v1"`,
		"allow_insecure_base_url = true",
		"display_name = \"In-cluster LLM\"",
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("updated config missing %q:\n%s", want, configText)
		}
	}
	if _, err := config.LoadFile(configPath); err != nil {
		t.Fatalf("updated config does not validate: %v", err)
	}
}

func TestConfigureProviderNonInteractiveRejectsFlagOnDerived(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openrouter" "base" {
  api_key = "sk-base"
  model "openai/gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openrouter",
		"--name", "derived",
		"--extends", "base",
		"--allow-insecure-base-url",
		"--api-key", "sk-derived",
	)
	if err == nil || !strings.Contains(err.Error(), "cannot be set on a derived provider") {
		t.Fatalf("expected derived-provider rejection, got %v", err)
	}
}
