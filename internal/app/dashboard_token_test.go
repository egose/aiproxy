package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
)

func TestBuildMintsDashboardTokenWithoutPersisting(t *testing.T) {
	tokenPath := filepath.Join(t.TempDir(), "aiproxy", "dashboard.token")
	t.Setenv("XDG_CONFIG_HOME", filepath.Dir(filepath.Dir(tokenPath)))
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
	if _, err := os.Stat(tokenPath); !os.IsNotExist(err) {
		t.Fatalf("Build should not persist token; stat err = %v", err)
	}
	if err := a.persistDashboardTokenIfNeeded(); err != nil {
		t.Fatalf("persist token: %v", err)
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
	_ = os.Remove(dashrpc.TokenFilePath())
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
	if a.Config.Dashboard.TokenFromConfig {
		t.Fatal("post-second-reload TokenFromConfig should be false after the token is removed from config")
	}

	// The explicit-to-omitted transition must publish the carried-over token
	// so a tokenless CLI discovery keeps working.
	data, err := os.ReadFile(dashrpc.TokenFilePath())
	if err != nil {
		t.Fatalf("token file should be published on explicit-to-omitted transition: %v", err)
	}
	if strings.TrimSpace(string(data)) != "tok-B" {
		t.Fatal("published token file does not match the carried-over runtime token")
	}
	if fi, err := os.Stat(dashrpc.TokenFilePath()); err != nil || fi.Mode().Perm() != 0o600 {
		t.Fatalf("published token file must retain mode 0600: stat=%v mode=%v", err, fi.Mode())
	}
}

func TestEnsureDashboardTokenNoopsWhenBlockAbsent(t *testing.T) {
	rt := &config.Runtime{}
	if minted, published, err := ensureDashboardToken(rt, config.Dashboard{Token: "irrelevant", TokenFromConfig: true}, false); err != nil || minted || published {
		t.Fatalf("ensureDashboardToken err = %v minted = %v published = %v", err, minted, published)
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
	if minted, published, err := ensureDashboardToken(rt, config.Dashboard{}, true); err != nil || !minted || !published {
		t.Fatalf("ensureDashboardToken err = %v minted = %v published = %v", err, minted, published)
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
	if minted, published, err := ensureDashboardToken(rt, config.Dashboard{Token: "previously-minted"}, true); err != nil || minted || published {
		t.Fatalf("ensureDashboardToken err = %v minted = %v published = %v", err, minted, published)
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
	if minted, published, err := ensureDashboardToken(rt, config.Dashboard{Token: "previously-minted"}, true); err != nil || minted || published {
		t.Fatalf("ensureDashboardToken err = %v minted = %v published = %v", err, minted, published)
	}
	if rt.Dashboard.Token != "from-config" {
		t.Fatalf("Token = %q, want from-config (config beats existing)", rt.Dashboard.Token)
	}
	if _, err := os.Stat(dashrpc.TokenFilePath()); !os.IsNotExist(err) {
		t.Fatalf("token file should NOT be persisted when config provided one; err = %v", err)
	}
}

func TestEnsureDashboardTokenPublishesExplicitToOmittedTransition(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	rt := &config.Runtime{Dashboard: config.Dashboard{Enabled: true}}
	current := config.Dashboard{Token: "carried-over", TokenFromConfig: true}
	if minted, published, err := ensureDashboardToken(rt, current, true); err != nil || minted || !published {
		t.Fatalf("ensureDashboardToken err = %v minted = %v published = %v", err, minted, published)
	}
	if rt.Dashboard.Token != "carried-over" {
		t.Fatalf("Token = %q, want carried-over", rt.Dashboard.Token)
	}
	if rt.Dashboard.TokenFromConfig {
		t.Fatal("reused runtime must keep omitted provenance (TokenFromConfig false)")
	}
	data, err := os.ReadFile(dashrpc.TokenFilePath())
	if err != nil {
		t.Fatalf("token file should be published: %v", err)
	}
	if strings.TrimSpace(string(data)) != "carried-over" {
		t.Fatal("published token file does not match the carried-over token")
	}
	fi, err := os.Stat(dashrpc.TokenFilePath())
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("published token file mode = %o, want 600", fi.Mode().Perm())
	}
}

func TestEnsureDashboardTokenPublishFailureClearsCandidate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	old := persistDashboardToken
	persistDashboardToken = func(string) error { return errors.New("injected persistence failure") }
	t.Cleanup(func() { persistDashboardToken = old })

	rt := &config.Runtime{Dashboard: config.Dashboard{Enabled: true}}
	current := config.Dashboard{Token: "carried-over", TokenFromConfig: true}
	_, _, err := ensureDashboardToken(rt, current, true)
	if err == nil {
		t.Fatal("expected persistence failure")
	}
	if strings.Contains(err.Error(), "carried-over") {
		t.Fatalf("error must not contain token contents: %v", err)
	}
	if rt.Dashboard.Token != "" {
		t.Fatal("failed publication must not leave an unpersisted token on the candidate runtime")
	}
	if _, err := os.Stat(dashrpc.TokenFilePath()); !os.IsNotExist(err) {
		t.Fatalf("failed publication must not create a token file; err = %v", err)
	}
}
