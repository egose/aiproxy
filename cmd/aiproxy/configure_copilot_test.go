package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestConfigureProviderLineOrientedCreatesCopilot(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	input := strings.Join([]string{
		"github-copilot",
		"copilot",
		"",
		"",
		"",
		"",
		"main",
		secretsPath,
		"gpt-4o-mini",
		"",
		"",
		"",
		"n",
	}, "\n") + "\n"

	stdout, stderr, err := executeRootCommand(input, "configure", "provider", "--config", configPath)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}

	configData, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("ReadFile(config): %v", err)
	}
	configText := string(configData)
	for _, want := range []string{
		`provider "github-copilot" "copilot" {`,
		"credential_ref {",
		`name = "main"`,
		`model "gpt-4o-mini" {`,
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("config output missing %q:\n%s", want, configText)
		}
	}
	if strings.Contains(configText, "api_key") {
		t.Fatalf("copilot config must not contain api_key:\n%s", configText)
	}
	if strings.Contains(configText, "gho_") || strings.Contains(stdout, "gho_") {
		t.Fatalf("token must never appear in config or output")
	}
	if !strings.Contains(configText, secretsPath) {
		t.Fatalf("config should record credential path %q:\n%s", secretsPath, configText)
	}
	if !strings.Contains(stdout, `updated provider "copilot"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestConfigureProviderNonInteractiveCreatesCopilot(t *testing.T) {
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
		"--type", "github-copilot",
		"--name", "copilot",
		"--credential", "main",
		"--credential-path", secretsPath,
		"--model", "gpt-4o-mini",
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
		`provider "github-copilot" "copilot" {`,
		"credential_ref {",
		`name = "main"`,
		`model "gpt-4o-mini" {`,
	} {
		if !strings.Contains(configText, want) {
			t.Fatalf("config output missing %q:\n%s", want, configText)
		}
	}
	if strings.Contains(configText, "api_key") {
		t.Fatalf("copilot config must not contain api_key:\n%s", configText)
	}
	if _, err := config.LoadFile(configPath); err == nil {
		t.Fatalf("expected load to fail without a saved login (offline validation)")
	} else if !strings.Contains(err.Error(), "credential_ref") {
		t.Fatalf("offline validation error should mention credential_ref, got: %v", err)
	}
	_ = stdout
}

func TestConfigureProviderNonInteractiveCopilotEquivalentRef(t *testing.T) {
	dir := t.TempDir()
	linePath := filepath.Join(dir, "line.hcl")
	scriptedPath := filepath.Join(dir, "scripted.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	lineInput := strings.Join([]string{
		"github-copilot",
		"copilot",
		"",
		"",
		"",
		"",
		"main",
		secretsPath,
		"gpt-4o-mini",
		"",
		"",
		"",
		"n",
	}, "\n") + "\n"
	if _, _, err := executeRootCommand(lineInput, "configure", "provider", "--config", linePath); err != nil {
		t.Fatalf("line-oriented: %v", err)
	}
	if _, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", scriptedPath,
		"--non-interactive",
		"--type", "github-copilot",
		"--name", "copilot",
		"--credential", "main",
		"--credential-path", secretsPath,
		"--model", "gpt-4o-mini",
	); err != nil {
		t.Fatalf("scripted: %v", err)
	}

	lineData, _ := os.ReadFile(linePath)
	scriptedData, _ := os.ReadFile(scriptedPath)
	for _, path := range []string{linePath, scriptedPath} {
		data, _ := os.ReadFile(path)
		text := string(data)
		if !strings.Contains(text, "credential_ref {") || !strings.Contains(text, `name = "main"`) {
			t.Fatalf("%s missing equivalent credential_ref:\n%s", path, text)
		}
		if strings.Contains(text, "api_key") {
			t.Fatalf("%s must not contain api_key:\n%s", path, text)
		}
	}
	_ = lineData
	_ = scriptedData
}

func TestConfigureProviderNonInteractiveCopilotRejectsAPIKeyFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "github-copilot",
		"--name", "copilot",
		"--api-key", "sk-test",
		"--model", "gpt-4o-mini",
	)
	if err == nil || !strings.Contains(err.Error(), "--credential") {
		t.Fatalf("expected --credential guidance, got %v", err)
	}
}

func TestConfigureProviderNonInteractiveCopilotRequiresCredential(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "github-copilot",
		"--name", "copilot",
		"--model", "gpt-4o-mini",
	)
	if err == nil || !strings.Contains(err.Error(), "--credential") {
		t.Fatalf("expected missing-credential error, got %v", err)
	}
}

func TestConfigureProviderNonInteractiveCopilotPreservesRefOnEdit(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")
	seed := `provider "github-copilot" "copilot" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "main"
  }
  model "gpt-4o-mini" {}
}
`
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--name", "copilot",
		"--display-name", "Copilot",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	configData, _ := os.ReadFile(configPath)
	text := string(configData)
	if !strings.Contains(text, "credential_ref {") || !strings.Contains(text, `name = "main"`) {
		t.Fatalf("edit dropped credential_ref:\n%s", text)
	}
	if !strings.Contains(text, `display_name = "Copilot"`) {
		t.Fatalf("edit missing display_name:\n%s", text)
	}
	if strings.Contains(text, "api_key") {
		t.Fatalf("edit must not introduce api_key:\n%s", text)
	}
}

func TestConfigureProviderNonInteractiveCopilotDerivedRequiresLocalCredential(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")
	seed := `provider "github-copilot" "base" {
  credential_ref {
    path = "` + secretsPath + `"
    name = "base"
  }
  model "gpt-4o-mini" {}
}
`
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "github-copilot",
		"--name", "derived",
		"--extends", "base",
	)
	if err == nil || !strings.Contains(err.Error(), "credential") {
		t.Fatalf("expected local-credential error, got %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "github-copilot",
		"--name", "derived",
		"--extends", "base",
		"--credential", "team",
		"--credential-path", secretsPath,
	)
	if err != nil {
		t.Fatalf("derived create: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	configData, _ := os.ReadFile(configPath)
	text := string(configData)
	if !strings.Contains(text, `provider "github-copilot" "derived"`) || !strings.Contains(text, `extends = "base"`) || !strings.Contains(text, `name = "team"`) {
		t.Fatalf("derived config missing expected ref:\n%s", text)
	}
	derivedStart := strings.Index(text, `provider "github-copilot" "derived"`)
	if strings.Contains(text[derivedStart:], "base_url") || strings.Contains(text[derivedStart:], "model ") {
		t.Fatalf("derived provider rendered inherited fields:\n%s", text[derivedStart:])
	}
}
