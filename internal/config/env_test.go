package config

import (
	"os"
	"path/filepath"
	"testing"
)

const envTestConfig = `
listener "http" "public" {
  address = ":8080"
}

auth "main" {
  mode = "none"
}

provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`

func TestEnvConfigContentUnset(t *testing.T) {
	t.Setenv(ConfigEnvVar, "")
	if _, ok := EnvConfigContent(); ok {
		t.Fatal("empty env should be treated as unset")
	}
}

func TestLoadEnv(t *testing.T) {
	t.Setenv(ConfigEnvVar, envTestConfig)
	rt, err := LoadEnv()
	if err != nil {
		t.Fatalf("LoadEnv: %v", err)
	}
	if rt.Listener.Address != ":8080" {
		t.Fatalf("address = %q", rt.Listener.Address)
	}
}

func TestLoadEnvUnsetErrors(t *testing.T) {
	if err := os.Unsetenv(ConfigEnvVar); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}
	if _, err := LoadEnv(); err == nil {
		t.Fatal("expected error when env is unset")
	}
}

func TestLoadFileOrEnvPrefersEnvWhenNotExplicit(t *testing.T) {
	t.Setenv(ConfigEnvVar, envTestConfig)
	filePath := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(filePath, []byte("invalid hcl >>>"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	rt, err := LoadFileOrEnv(filePath, false)
	if err != nil {
		t.Fatalf("LoadFileOrEnv should prefer env, got: %v", err)
	}
	if rt.Listener.Address != ":8080" {
		t.Fatalf("address = %q", rt.Listener.Address)
	}
}

func TestLoadFileOrEnvUsesFileWhenExplicit(t *testing.T) {
	t.Setenv(ConfigEnvVar, "invalid hcl >>>")
	filePath := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(filePath, []byte(envTestConfig), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	rt, err := LoadFileOrEnv(filePath, true)
	if err != nil {
		t.Fatalf("LoadFileOrEnv should use file when explicit, got: %v", err)
	}
	if rt.Listener.Address != ":8080" {
		t.Fatalf("address = %q", rt.Listener.Address)
	}
}

func TestLoadFileOrEnvFallsBackToFile(t *testing.T) {
	if err := os.Unsetenv(ConfigEnvVar); err != nil {
		t.Fatalf("unsetenv: %v", err)
	}
	filePath := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(filePath, []byte(envTestConfig), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := LoadFileOrEnv(filePath, false); err != nil {
		t.Fatalf("LoadFileOrEnv fallback: %v", err)
	}
}
