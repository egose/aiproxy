package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureProviderCreatesConfigAndSecrets(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	input := strings.Join([]string{
		configPath,
		"",
		"primary",
		"",
		"",
		secretsPath,
		"",
		"sk-test-primary",
		"gpt-4o-mini",
		"",
		"",
		"",
		"n",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "provider")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"provider \"openai\" \"primary\" {",
		"api_key_ref {",
		"path = \"" + secretsPath + "\"",
		"key  = \"primary\"",
		"model \"gpt-4o-mini\" {",
		"capabilities = [\"chat\", \"responses\"]",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("config output missing %q:\n%s", check, configText)
		}
	}

	secretsData, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("ReadFile(secrets): %v", err)
	}
	if !strings.Contains(string(secretsData), `"primary": "sk-test-primary"`) {
		t.Fatalf("secrets output missing provider key:\n%s", string(secretsData))
	}
	if !strings.Contains(stdout, `updated provider "primary"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestConfigureProviderUpdatesExistingBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "primary" {
  api_key = "old-key"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	input := strings.Join([]string{
		configPath,
		"2",
		"1",
		"2",
		"primary",
		"Backup provider",
		"https://llm.internal/v1",
		"2",
		`env("LOCALAI_API_KEY")`,
		"y",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "provider")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"provider \"openai-compatible\" \"primary\" {",
		"display_name = \"Backup provider\"",
		"base_url = \"https://llm.internal/v1\"",
		"api_key = env(\"LOCALAI_API_KEY\")",
		"model \"gpt-4o-mini\" {",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("updated config missing %q:\n%s", check, configText)
		}
	}
	if strings.Count(configText, `provider "`) != 1 {
		t.Fatalf("expected one provider block after update:\n%s", configText)
	}
	if !strings.Contains(stdout, `updated provider "primary"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestConfigureProviderDeleteFlagRemovesBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand("", "configure", "provider", "--config", configPath, "--delete", "--name", "primary")
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	if strings.Contains(configText, `provider "openai" "primary"`) {
		t.Fatalf("provider block still present after delete:\n%s", configText)
	}
	if !strings.Contains(stdout, `deleted provider "primary"`) {
		t.Fatalf("stdout missing delete summary:\n%s", stdout)
	}
}

func TestConfigureProviderNonInteractiveFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openai-compatible",
		"--name", "backup",
		"--display-name", "Backup provider",
		"--base-url", "https://llm.internal/v1",
		"--secrets-path", secretsPath,
		"--secrets-key", "localai",
		"--api-key", "secret-value",
		"--model", "qwen3-32b=qwen/qwen3-32b",
		"--model-capabilities", "qwen3-32b=chat,responses",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"provider \"openai-compatible\" \"backup\" {",
		"display_name = \"Backup provider\"",
		"base_url = \"https://llm.internal/v1\"",
		"api_key_ref {",
		"path = \"" + secretsPath + "\"",
		"key  = \"localai\"",
		"model \"qwen3-32b\" {",
		"upstream_name = \"qwen/qwen3-32b\"",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("config output missing %q:\n%s", check, configText)
		}
	}
	secretsData, err := os.ReadFile(secretsPath)
	if err != nil {
		t.Fatalf("ReadFile(secrets): %v", err)
	}
	if !strings.Contains(string(secretsData), `"localai": "secret-value"`) {
		t.Fatalf("secrets output missing expected value:\n%s", string(secretsData))
	}
}

func TestConfigureAuthCreatesBearerStaticBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	input := strings.Join([]string{
		configPath,
		"main",
		"2",
		"y",
		"120",
		"120",
		"internal-app",
		`env("AIPROXY_CLIENT_TOKEN")`,
		"internal",
		"alias/chat_default,openai/gpt-4o-mini",
		"n",
	}, "\n") + "\n"

	_, stderr, err := executeRootCommand(input, "configure", "auth")
	if err != nil {
		t.Fatalf("Execute(): %v\nstderr:\n%s", err, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"auth \"main\" {",
		"mode = \"bearer_static\"",
		"rate_limit {",
		"requests_per_minute = 120",
		"client \"internal-app\" {",
		"token = env(\"AIPROXY_CLIENT_TOKEN\")",
		"allowed_models = [\"alias/chat_default\", \"openai/gpt-4o-mini\"]",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("auth config missing %q:\n%s", check, configText)
		}
	}
}

func TestConfigureAliasCreatesAliasBlock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	input := strings.Join([]string{
		configPath,
		"chat_default",
		"",
		"primary",
		"gpt-4o-mini",
		"n",
	}, "\n") + "\n"

	_, stderr, err := executeRootCommand(input, "configure", "alias")
	if err != nil {
		t.Fatalf("Execute(): %v\nstderr:\n%s", err, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"alias \"chat_default\" {",
		"algorithm = \"round_robin\"",
		"provider = \"primary\"",
		"model    = \"gpt-4o-mini\"",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("alias config missing %q:\n%s", check, configText)
		}
	}
}

func TestConfigureAliasNonInteractiveFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "alias",
		"--config", configPath,
		"--non-interactive",
		"--name", "chat_default",
		"--algorithm", "round_robin",
		"--target", "primary/gpt-4o-mini",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `alias "chat_default" {`) || !strings.Contains(configText, `provider = "primary"`) {
		t.Fatalf("alias config missing expected content:\n%s", configText)
	}
}

func TestConfigureAuthNonInteractiveFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`listener "http" "public" {
  address = ":8080"
}

provider "openai" "primary" {
  api_key = "sk-test"

  model "gpt-4o-mini" {}
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "auth",
		"--config", configPath,
		"--non-interactive",
		"--name", "main",
		"--mode", "bearer_static",
		"--rate-limit-rpm", "120",
		"--rate-limit-burst", "120",
		"--client", "internal-app",
		"--client-token-env", "internal-app=AIPROXY_CLIENT_TOKEN",
		"--client-tenant", "internal-app=internal",
		"--client-allowed-models", "internal-app=alias/chat_default,openai/gpt-4o-mini",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	checks := []string{
		"auth \"main\" {",
		"mode = \"bearer_static\"",
		"requests_per_minute = 120",
		"token = env(\"AIPROXY_CLIENT_TOKEN\")",
		"allowed_models = [\"alias/chat_default\", \"openai/gpt-4o-mini\"]",
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("auth config missing %q:\n%s", check, configText)
		}
	}
}

func TestConfigureRootCommandPromptsForBlockSelection(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	input := strings.Join([]string{
		"4",
		"public",
		":8081",
		"n",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "--config", configPath)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	if !strings.Contains(configText, `listener "http" "public" {`) || !strings.Contains(configText, `address = ":8081"`) {
		t.Fatalf("listener config missing expected content:\n%s", configText)
	}
	if !strings.Contains(stdout, `updated listener block`) {
		t.Fatalf("stdout missing listener summary:\n%s", stdout)
	}
}

func executeRootCommand(input string, args ...string) (string, string, error) {
	cmd := newRootCommand()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.SetIn(strings.NewReader(input))
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}
