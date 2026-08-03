package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeDashboardConfig(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestRunDashboardErrorsWhenBlockAbsent(t *testing.T) {
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runDashboard(context.Background(), cfg, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when dashboard block is absent")
	}
	if !strings.Contains(stderr.String(), "not configured") {
		t.Fatalf("stderr should mention 'not configured', got: %s", stderr.String())
	}
}

func TestRunDashboardErrorsWhenConfigInvalid(t *testing.T) {
	cfg := writeDashboardConfig(t, `invalid hcl >>>`)
	var stdout, stderr bytes.Buffer
	err := runDashboard(context.Background(), cfg, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
	if !strings.Contains(err.Error(), "config") {
		t.Fatalf("err should mention config, got: %v", err)
	}
}

func TestRunDashboardErrorsWhenNoServerRunning(t *testing.T) {
	cfg := writeDashboardConfig(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {
  token = "sekret"
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var stdout, stderr bytes.Buffer
	err := runDashboard(context.Background(), cfg, &stdout, &stderr)
	if err == nil {
		t.Fatal("expected error when no server is running")
	}
	if !strings.Contains(stderr.String(), "no server running") {
		t.Fatalf("stderr should say 'no server running', got: %s", stderr.String())
	}
}
