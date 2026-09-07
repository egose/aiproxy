package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureProviderNonInteractiveCreatesOpenCodeZenDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "opencode-zen",
		"--name", "zen",
		"--api-key-env", "OPENCODE_ZEN_API_KEY",
		"--model", "glm-5.3",
		"--model-protocol", "glm-5.3=chat",
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
		`provider "opencode-zen" "zen" {`,
		`api_key = env("OPENCODE_ZEN_API_KEY")`,
		`model "glm-5.3" {`,
		`protocol = "chat"`,
		`capabilities = ["chat"]`,
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("config output missing %q:\n%s", check, configText)
		}
	}
	if strings.Contains(configText, "base_url") {
		t.Fatalf("zen provider should omit base_url by default:\n%s", configText)
	}
	if !strings.Contains(stdout, `updated provider "zen"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestConfigureProviderNonInteractiveCreatesOpenCodeGoWithOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "opencode-go",
		"--name", "go",
		"--base-url", "http://127.0.0.1:18081",
		"--api-key-env", "OPENCODE_GO_API_KEY",
		"--model", "minimax-m3",
		"--model-protocol", "minimax-m3=messages",
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
		`provider "opencode-go" "go" {`,
		`base_url = "http://127.0.0.1:18081"`,
		`api_key = env("OPENCODE_GO_API_KEY")`,
		`model "minimax-m3" {`,
		`protocol = "messages"`,
		`capabilities = ["chat", "responses"]`,
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("config output missing %q:\n%s", check, configText)
		}
	}
	if !strings.Contains(stdout, `updated provider "go"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestConfigureProviderNonInteractiveOpenCodeRequiresProtocol(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "opencode-zen",
		"--name", "zen",
		"--api-key-env", "OPENCODE_ZEN_API_KEY",
		"--model", "glm-5.3",
	)
	if err == nil || !strings.Contains(err.Error(), "--model-protocol") {
		t.Fatalf("error = %v, want missing-protocol error", err)
	}
}

func TestConfigureProviderNonInteractiveRejectsGeminiProtocolOnGo(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "opencode-go",
		"--name", "go",
		"--api-key-env", "OPENCODE_GO_API_KEY",
		"--model", "gemini-3.8-flash",
		"--model-protocol", "gemini-3.8-flash=gemini",
	)
	if err == nil || !strings.Contains(err.Error(), "not supported by provider type") {
		t.Fatalf("error = %v, want gemini-on-go rejection", err)
	}
}

func TestConfigureProviderNonInteractiveRejectsCapabilityOutsideProtocol(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "opencode-zen",
		"--name", "zen",
		"--api-key-env", "OPENCODE_ZEN_API_KEY",
		"--model", "glm-5.3",
		"--model-protocol", "glm-5.3=chat",
		"--model-capabilities", "glm-5.3=chat,responses",
	)
	if err == nil || !strings.Contains(err.Error(), "not served by protocol") {
		t.Fatalf("error = %v, want capability/protocol mismatch rejection", err)
	}
}

func TestConfigureProviderNonInteractiveOpenCodeEditPreservesProtocolAndOverride(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	seed := strings.TrimSpace(`provider "opencode-go" "go" {
  display_name = "Go"
  base_url = "http://127.0.0.1:18081"
  api_key = env("OPENCODE_GO_API_KEY")

  model "minimax-m3" {
    protocol = "messages"
    capabilities = ["chat", "responses"]
  }
}`) + "\n"
	if err := os.WriteFile(configPath, []byte(seed), 0o600); err != nil {
		t.Fatalf("WriteFile(seed): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--name", "go",
		"--display-name", "Go updated",
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
		`provider "opencode-go" "go" {`,
		`display_name = "Go updated"`,
		`base_url = "http://127.0.0.1:18081"`,
		`protocol = "messages"`,
		`capabilities = ["chat", "responses"]`,
		`api_key = env("OPENCODE_GO_API_KEY")`,
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("edited config missing %q:\n%s", check, configText)
		}
	}
	if strings.Count(configText, `provider "`) != 1 {
		t.Fatalf("expected one provider block after update:\n%s", configText)
	}
	if !strings.Contains(stdout, `updated provider "go"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestConfigureProviderInteractiveCreatesOpenCodeZen(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	input := strings.Join([]string{
		configPath,
		"5",
		"zen",
		"",
		"",
		"",
		"",
		"",
		"2",
		"",
		"glm-5.3",
		"",
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
		`provider "opencode-zen" "zen" {`,
		`api_key = env("OPENCODE_ZEN_API_KEY")`,
		`model "glm-5.3" {`,
		`protocol = "chat"`,
	}
	for _, check := range checks {
		if !strings.Contains(configText, check) {
			t.Fatalf("config output missing %q:\n%s", check, configText)
		}
	}
	if strings.Contains(configText, "base_url") {
		t.Fatalf("zen provider should omit base_url by default:\n%s", configText)
	}
	if !strings.Contains(stdout, `updated provider "zen"`) {
		t.Fatalf("stdout missing provider summary:\n%s", stdout)
	}
}

func TestDefaultProviderEnvExpressionOpenCode(t *testing.T) {
	if got := defaultProviderEnvExpression("opencode-zen"); got != `env("OPENCODE_ZEN_API_KEY")` {
		t.Fatalf("zen env expression = %q", got)
	}
	if got := defaultProviderEnvExpression("opencode-go"); got != `env("OPENCODE_GO_API_KEY")` {
		t.Fatalf("go env expression = %q", got)
	}
}

func TestSupportedCapabilitiesOpenCode(t *testing.T) {
	for _, providerType := range []string{"opencode-zen", "opencode-go"} {
		got := supportedCapabilities(providerType)
		if len(got) != 2 || got[0] != "chat" || got[1] != "responses" {
			t.Fatalf("supportedCapabilities(%q) = %v", providerType, got)
		}
		if got := defaultCapabilities(providerType); len(got) != 2 {
			t.Fatalf("defaultCapabilities(%q) = %v", providerType, got)
		}
	}
}

func TestSupportedProtocolsOpenCode(t *testing.T) {
	zen := supportedProtocols("opencode-zen")
	if len(zen) != 4 || zen[3] != "gemini" {
		t.Fatalf("supportedProtocols(zen) = %v", zen)
	}
	goProtocols := supportedProtocols("opencode-go")
	if len(goProtocols) != 3 || goProtocols[0] != "chat" || goProtocols[2] != "messages" {
		t.Fatalf("supportedProtocols(go) = %v", goProtocols)
	}
	if supportedProtocols("openai") != nil {
		t.Fatalf("supportedProtocols(openai) should be nil")
	}
}
