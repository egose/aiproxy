package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const envSourceTestConfig = `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`

func TestValidatePrefersEnvWhenFlagUnset(t *testing.T) {
	t.Setenv("AIPROXY_CONFIG", envSourceTestConfig)
	missing := filepath.Join(t.TempDir(), "missing.hcl")

	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"validate", "--config", missing})
	if err := cmd.Execute(); err == nil {
		t.Fatal("explicit --config should use the file and fail for missing path")
	}

	cmd = newRootCommand()
	buf.Reset()
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"validate"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("validate with env should succeed, got: %v (out=%s)", err, buf.String())
	}
	if buf.String() != "config is valid\n" {
		t.Fatalf("output = %q", buf.String())
	}
}

func TestValidateExplicitFlagOverridesEnv(t *testing.T) {
	t.Setenv("AIPROXY_CONFIG", "invalid hcl >>>")
	configPath := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(configPath, []byte(envSourceTestConfig), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"validate", "--config", configPath})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("explicit file should win over invalid env, got: %v", err)
	}
}

func TestServeDaemonRejectsEnvConfig(t *testing.T) {
	t.Setenv("AIPROXY_CONFIG", envSourceTestConfig)
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"serve", "-d"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "daemon mode requires a config file") {
		t.Fatalf("expected daemon env error, got err=%v out=%s", err, buf.String())
	}
}

func TestPathsShowsEnvSource(t *testing.T) {
	t.Setenv("AIPROXY_CONFIG", envSourceTestConfig)
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs([]string{"paths"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("paths: %v", err)
	}
	if !strings.Contains(buf.String(), "env(AIPROXY_CONFIG)") {
		t.Fatalf("paths should mention env source, got: %q", buf.String())
	}
}
