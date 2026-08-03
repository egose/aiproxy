package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
)

func TestBuildMintsDashboardTokenWhenBlockDeclaredWithoutToken(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "aiproxy", "dashboard.token")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(filepath.Dir(tokenPath)))
	// Remove any pre-existing token file to truly test the mint path.
	_ = os.Remove(tokenPath)

	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	if !a.Config.Dashboard.Enabled {
		t.Fatal("Dashboard.Enabled should be true")
	}
	if a.Config.Dashboard.Token == "" {
		t.Fatal("Build should mint a non-empty dashboard token when config declares dashboard {} without a token")
	}
	if len(a.Config.Dashboard.Token) < 32 {
		t.Fatalf("minted token should be at least 32 hex chars, got %q", a.Config.Dashboard.Token)
	}
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		t.Fatalf("expected token persisted to %s: %v", tokenPath, err)
	}
	if strings.TrimSpace(string(data)) != a.Config.Dashboard.Token {
		t.Fatalf("persisted token %q != in-memory token %q", strings.TrimSpace(string(data)), a.Config.Dashboard.Token)
	}
}

func TestBuildPreservesExplicitDashboardTokenAndDoesNotTouchFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	tokenPath := filepath.Join(dir, "aiproxy", "dashboard.token")
	_ = os.Remove(tokenPath)

	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {
  token = "user-supplied-token"
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	if a.Config.Dashboard.Token != "user-supplied-token" {
		t.Fatalf("Token = %q, want user-supplied-token", a.Config.Dashboard.Token)
	}
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("Build should NOT persist token when config supplied one; stat err = %v", err)
	}
}

func TestReloadPreservesMintedDashboardTokenAcrossReloads(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	mintedToken := a.Config.Dashboard.Token
	if mintedToken == "" {
		t.Fatal("expected minted token after Build")
	}
	// Wipe the persistence file so we know Reload is not re-minting from disk.
	if err := os.Remove(dashrpc.TokenFilePath()); err != nil {
		t.Fatalf("remove token file: %v", err)
	}
	if err := a.Reload(); err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if a.Config.Dashboard.Token != mintedToken {
		t.Fatalf("Reload rotated the dashboard token without a config change: old=%q new=%q", mintedToken, a.Config.Dashboard.Token)
	}
	// Reload should not have re-persisted the token file either.
	if _, err := os.Stat(dashrpc.TokenFilePath()); !os.IsNotExist(err) {
		t.Fatalf("Reload should not re-persist the token file when there is no change; err = %v", err)
	}
}

func TestReloadReusesConfigSuppliedToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard { token = "tok-A" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	if a.Config.Dashboard.Token != "tok-A" {
		t.Fatalf("initial token = %q, want tok-A", a.Config.Dashboard.Token)
	}

	// Change the config to a different explicit token. Reload should pick it up.
	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard { token = "tok-B" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.Config.Dashboard.Token != "tok-B" {
		t.Fatalf("post-reload token = %q, want tok-B", a.Config.Dashboard.Token)
	}

	// Reload again with no token in config: the previously-resolved tok-B
	// must be preserved (not re-minted).
	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.Config.Dashboard.Token != "tok-B" {
		t.Fatalf("post-second-reload token = %q, want tok-B (preserved)", a.Config.Dashboard.Token)
	}

	// And the token file should NEVER have been written because the user
	// never relied on the auto-mint path.
	if _, err := os.Stat(dashrpc.TokenFilePath()); !os.IsNotExist(err) {
		t.Fatalf("token file should remain absent under explicitly-resolved path; err = %v", err)
	}
}

func TestEnsureDashboardTokenNoopsWhenBlockAbsent(t *testing.T) {
	rt := &config.Runtime{}
	if err := ensureDashboardToken(rt, "irrelevant"); err != nil {
		t.Fatalf("ensureDashboardToken err = %v", err)
	}
	if rt.Dashboard.Enabled {
		t.Fatal("Dashboard.Enabled should remain false")
	}
	if rt.Dashboard.Token != "" {
		t.Fatalf("Token = %q, want empty", rt.Dashboard.Token)
	}
}

func TestEnsureDashboardTokenMintsWhenEnabledWithEmptyTokenAndEmptyExisting(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	rt := &config.Runtime{Dashboard: config.Dashboard{Enabled: true}}
	if err := ensureDashboardToken(rt, ""); err != nil {
		t.Fatalf("ensureDashboardToken err = %v", err)
	}
	if rt.Dashboard.Token == "" {
		t.Fatal("expected minted token")
	}
	if _, err := os.Stat(dashrpc.TokenFilePath()); err != nil {
		t.Fatalf("token file should be persisted, err = %v", err)
	}
}

func TestEnsureDashboardTokenReusesExistingWhenEnabledWithEmptyToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	rt := &config.Runtime{Dashboard: config.Dashboard{Enabled: true}}
	if err := ensureDashboardToken(rt, "previously-minted"); err != nil {
		t.Fatalf("ensureDashboardToken err = %v", err)
	}
	if rt.Dashboard.Token != "previously-minted" {
		t.Fatalf("Token = %q, want previously-minted", rt.Dashboard.Token)
	}
	if _, err := os.Stat(dashrpc.TokenFilePath()); !os.IsNotExist(err) {
		t.Fatalf("token file should NOT be re-persisted when reusing existing; err = %v", err)
	}
}

func TestEnsureDashboardTokenUsesConfigToken(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	rt := &config.Runtime{Dashboard: config.Dashboard{Enabled: true, Token: "from-config"}}
	if err := ensureDashboardToken(rt, "previously-minted"); err != nil {
		t.Fatalf("ensureDashboardToken err = %v", err)
	}
	if rt.Dashboard.Token != "from-config" {
		t.Fatalf("Token = %q, want from-config (config beats existing)", rt.Dashboard.Token)
	}
	if _, err := os.Stat(dashrpc.TokenFilePath()); !os.IsNotExist(err) {
		t.Fatalf("token file should NOT be persisted when config provided one; err = %v", err)
	}
}
