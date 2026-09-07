package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func writeModelsConfig(t *testing.T, contents string) string {
	t.Helper()
	return writeDashboardConfig(t, contents)
}

func TestRunModelsListsProviderModels(t *testing.T) {
	cfg := writeModelsConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4.1" {
    display_name = "GPT-4.1"
    capabilities = ["chat", "responses"]
  }
}
provider "anthropic" "anthropic" {
  api_key = "sk-test"
  model "claude-sonnet" {
    display_name = "Claude Sonnet"
    upstream_name = "claude-sonnet-4-20250514"
    capabilities = ["chat", "responses"]
  }
}
`)
	var stdout, stderr bytes.Buffer
	if err := runModels(cfg, "anthropic", &stdout, &stderr); err != nil {
		t.Fatalf("runModels: %v", err)
	}
	out := stdout.String()
	for _, want := range []string{`Provider "anthropic"`, "anthropic/claude-sonnet", "Claude Sonnet", "claude-sonnet-4-20250514", "chat, responses"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "openai/gpt-4.1") {
		t.Fatalf("output should only list the selected provider:\n%s", out)
	}
}

func TestRunModelsErrorsOnUnknownProvider(t *testing.T) {
	cfg := writeModelsConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runModels(cfg, "nope", &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
	if !strings.Contains(stderr.String(), "unknown provider") {
		t.Fatalf("stderr should mention 'unknown provider', got: %s", stderr.String())
	}
}

func TestRunModelsRequiresTTYWithoutProvider(t *testing.T) {
	cfg := writeModelsConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runModels(cfg, "", &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when no provider given without a terminal")
	}
	if !strings.Contains(err.Error(), "--provider") {
		t.Fatalf("err should suggest --provider, got: %v", err)
	}
}

func TestModelsCommandWired(t *testing.T) {
	cmd := newRootCommand()
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stdout)
	cmd.SetArgs([]string{"models", "--help"})
	if err := cmd.ExecuteContext(context.Background()); err != nil {
		t.Fatalf("models --help: %v", err)
	}
	if !strings.Contains(stdout.String(), "List models for a provider") {
		t.Fatalf("models --help missing short description:\n%s", stdout.String())
	}
}
