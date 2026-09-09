package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const convertTestSource = `listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`

func runConvertCommand(t *testing.T, env map[string]string, args ...string) (string, error) {
	t.Helper()
	for k, v := range env {
		t.Setenv(k, v)
	}
	cmd := newRootCommand()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetArgs(append([]string{"convert"}, args...))
	err := cmd.Execute()
	return buf.String(), err
}

func TestConvertFileToJSON(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.hcl")
	if err := os.WriteFile(src, []byte(convertTestSource), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	dst := filepath.Join(dir, "out.json")
	out, err := runConvertCommand(t, nil, dst, "--config", src)
	if err != nil {
		t.Fatalf("convert: %v (out=%s)", err, out)
	}
	if !strings.Contains(out, "(hcl) -> ") || !strings.Contains(out, "(json)") {
		t.Fatalf("confirmation = %q", out)
	}
	raw, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read target: %v", err)
	}
	if !strings.Contains(string(raw), `"address": ":8080"`) {
		t.Fatalf("target =\n%s", raw)
	}
}

func TestConvertEnvSourceDefaultsToCWD(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	out, err := runConvertCommand(t, map[string]string{"AIPROXY_CONFIG": convertTestSource})
	if err != nil {
		t.Fatalf("convert: %v (out=%s)", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatalf("default target missing: %v (out=%s)", err, out)
	}
	if !strings.Contains(string(raw), `"mode": "none"`) {
		t.Fatalf("target =\n%s", raw)
	}
}

func TestConvertRefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.hcl")
	if err := os.WriteFile(src, []byte(convertTestSource), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	dst := filepath.Join(dir, "out.json")
	if err := os.WriteFile(dst, []byte("sentinel"), 0o600); err != nil {
		t.Fatalf("write target: %v", err)
	}
	_, err := runConvertCommand(t, nil, dst, "--config", src)
	if err == nil || !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error = %v, want overwrite refusal", err)
	}
	raw, _ := os.ReadFile(dst)
	if string(raw) != "sentinel" {
		t.Fatal("target was modified without --force")
	}
	out, err := runConvertCommand(t, nil, dst, "--config", src, "--force")
	if err != nil {
		t.Fatalf("convert --force: %v (out=%s)", err, out)
	}
	raw, _ = os.ReadFile(dst)
	if strings.Contains(string(raw), "sentinel") {
		t.Fatal("target was not overwritten with --force")
	}
}

func TestConvertStdoutDash(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "input.hcl")
	if err := os.WriteFile(src, []byte(convertTestSource), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out, err := runConvertCommand(t, nil, "--compact", "-", "--config", src)
	if err != nil {
		t.Fatalf("convert: %v (out=%s)", err, out)
	}
	if strings.Count(out, "\n") != 1 || !strings.HasPrefix(out, "{") {
		t.Fatalf("stdout = %q", out)
	}
}

func TestConvertJSONToHCLDefaultName(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	src := filepath.Join(dir, "remote.json")
	jsonCfg := `{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"openai":{"api_key":"k","model":{"m":{}}}}}}`
	if err := os.WriteFile(src, []byte(jsonCfg), 0o600); err != nil {
		t.Fatalf("write source: %v", err)
	}
	out, err := runConvertCommand(t, nil, "--config", src)
	if err != nil {
		t.Fatalf("convert: %v (out=%s)", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "remote.hcl"))
	if err != nil {
		t.Fatalf("default target missing: %v (out=%s)", err, out)
	}
	if !strings.Contains(string(raw), `listener "http" "public"`) {
		t.Fatalf("target =\n%s", raw)
	}
}

func TestConvertMissingSourceErrors(t *testing.T) {
	_ = os.Unsetenv("AIPROXY_CONFIG")
	missing := filepath.Join(t.TempDir(), "missing.hcl")
	_, err := runConvertCommand(t, nil, "--config", missing)
	if err == nil {
		t.Fatal("expected error for missing source")
	}
}
