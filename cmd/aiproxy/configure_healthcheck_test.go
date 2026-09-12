package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureProviderNonInteractiveHealthcheckFlags(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openai-compatible",
		"--name", "local",
		"--base-url", "http://127.0.0.1:11434/v1",
		"--secrets-path", secretsPath,
		"--secrets-key", "localai",
		"--api-key", "secret-value",
		"--model", "m",
		"--healthcheck-path", "/health",
		"--healthcheck-method", "GET",
		"--healthcheck-expected-status", "200",
		"--healthcheck-expected-body", "ok",
		"--healthcheck-interval", "15s",
		"--healthcheck-timeout", "3s",
		"--healthcheck-failure-threshold", "3",
		"--healthcheck-success-threshold", "2",
		"--healthcheck-send-authorization",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	configText := readFileForTest(t, configPath)
	for _, check := range []string{
		"healthcheck {",
		`path = "/health"`,
		`method = "GET"`,
		"expected_status = 200",
		`expected_body = "ok"`,
		`interval = "15s"`,
		`timeout = "3s"`,
		"failure_threshold = 3",
		"success_threshold = 2",
		"send_authorization = true",
	} {
		if !strings.Contains(configText, check) {
			t.Fatalf("config output missing %q:\n%s", check, configText)
		}
	}
}

func TestConfigureProviderNonInteractivePreservesHealthcheck(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	if _, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openai-compatible",
		"--name", "local",
		"--base-url", "http://127.0.0.1:11434/v1",
		"--secrets-path", secretsPath,
		"--secrets-key", "localai",
		"--api-key", "secret-value",
		"--model", "m",
		"--healthcheck-path", "/health",
	); err != nil {
		t.Fatalf("setup Execute(): %v", err)
	}

	stdout, stderr, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--name", "local",
		"--display-name", "Renamed",
		"--model", "m",
	)
	if err != nil {
		t.Fatalf("Execute(): %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	configText := readFileForTest(t, configPath)
	if !strings.Contains(configText, "healthcheck {") || !strings.Contains(configText, `path = "/health"`) {
		t.Fatalf("healthcheck block was dropped on update:\n%s", configText)
	}
	if !strings.Contains(configText, `display_name = "Renamed"`) {
		t.Fatalf("display name update missing:\n%s", configText)
	}
}

func TestConfigureProviderNonInteractiveRemovesHealthcheck(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")
	secretsPath := filepath.Join(dir, "keys.json")

	if _, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openai-compatible",
		"--name", "local",
		"--base-url", "http://127.0.0.1:11434/v1",
		"--secrets-path", secretsPath,
		"--secrets-key", "localai",
		"--api-key", "secret-value",
		"--model", "m",
		"--healthcheck-path", "/health",
	); err != nil {
		t.Fatalf("setup Execute(): %v", err)
	}

	if _, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--name", "local",
		"--model", "m",
		"--no-healthcheck",
	); err != nil {
		t.Fatalf("Execute(): %v", err)
	}
	if configText := readFileForTest(t, configPath); strings.Contains(configText, "healthcheck") {
		t.Fatalf("healthcheck block should be removed:\n%s", configText)
	}
}

func TestConfigureProviderNonInteractiveRejectsExtendsWithHealthcheck(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "openai-compatible",
		"--name", "derived",
		"--extends", "base",
		"--api-key", "secret-value",
		"--model", "m",
		"--healthcheck-path", "/health",
	)
	if err == nil || !strings.Contains(err.Error(), "--extends cannot be combined") {
		t.Fatalf("expected extends conflict error, got %v", err)
	}
}

func TestConfigureProviderNonInteractiveRejectsCopilotHealthcheck(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.hcl")

	_, _, err := executeRootCommand(
		"",
		"configure", "provider",
		"--config", configPath,
		"--non-interactive",
		"--type", "github-copilot",
		"--name", "copilot",
		"--model", "gpt-5-mini",
		"--healthcheck-path", "/health",
	)
	if err == nil || !strings.Contains(err.Error(), "healthcheck is not supported by github-copilot") {
		t.Fatalf("expected copilot healthcheck error, got %v", err)
	}
}

func readFileForTest(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", path, err)
	}
	return string(data)
}
