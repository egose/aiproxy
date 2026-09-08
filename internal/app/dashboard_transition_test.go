package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/dashrpc"
)

const transitionExplicitConfig = `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard { token = "transition-explicit" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`

const transitionOmittedConfig = `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`

func snapshotStatus(t *testing.T, h http.Handler, bearer string) int {
	t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, dashrpc.SnapshotPath, nil)
	r.Header.Set("Authorization", "Bearer "+bearer)
	h.ServeHTTP(w, r)
	return w.Code
}

func TestReloadExplicitToOmittedPublishesAbsentTokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configPath := writeConfigFile(t, transitionExplicitConfig)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	if _, err := os.Stat(dashrpc.TokenFilePath()); !os.IsNotExist(err) {
		t.Fatalf("explicit startup must not write a token file; err = %v", err)
	}

	rewriteConfigFile(t, configPath, transitionOmittedConfig)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.Config.Dashboard.TokenFromConfig {
		t.Fatal("reloaded runtime must record omitted token provenance")
	}

	discovered, err := dashrpc.LoadToken()
	if err != nil {
		t.Fatalf("CLI discovery must read the published token: %v", err)
	}
	if discovered != a.Config.Dashboard.Token {
		t.Fatal("discovered token does not match the active runtime token")
	}
	fi, err := os.Stat(dashrpc.TokenFilePath())
	if err != nil {
		t.Fatalf("stat token file: %v", err)
	}
	if fi.Mode().Perm() != 0o600 {
		t.Fatalf("published token file mode = %o, want 600", fi.Mode().Perm())
	}

	srv := httptest.NewServer(a.Server.Handler)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+dashrpc.SnapshotPath, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+discovered)
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatalf("snapshot fetch with discovered token: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("snapshot with discovered token status = %d, want 200", resp.StatusCode)
	}
}

func TestReloadExplicitToOmittedOverwritesStaleTokenFile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configPath := writeConfigFile(t, transitionExplicitConfig)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	if err := dashrpc.PersistToken("stale-file-token"); err != nil {
		t.Fatalf("seed stale token file: %v", err)
	}

	rewriteConfigFile(t, configPath, transitionOmittedConfig)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}

	discovered, err := dashrpc.LoadToken()
	if err != nil {
		t.Fatalf("load token: %v", err)
	}
	if discovered != a.Config.Dashboard.Token {
		t.Fatal("stale file was not replaced by the carried-over runtime token")
	}
	if got := snapshotStatus(t, a.Server.Handler, discovered); got != http.StatusOK {
		t.Fatalf("snapshot with discovered token status = %d, want 200", got)
	}
	if got := snapshotStatus(t, a.Server.Handler, "stale-file-token"); got != http.StatusUnauthorized {
		t.Fatalf("snapshot with stale token status = %d, want 401", got)
	}
}

func TestReloadExplicitToOmittedPersistenceFailureKeepsRuntime(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	configPath := writeConfigFile(t, transitionExplicitConfig)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	activeToken := a.Config.Dashboard.Token
	if err := dashrpc.PersistToken("stale-file-token"); err != nil {
		t.Fatalf("seed stale token file: %v", err)
	}
	staleBefore, err := os.ReadFile(dashrpc.TokenFilePath())
	if err != nil {
		t.Fatalf("read stale token file: %v", err)
	}

	old := persistDashboardToken
	persistDashboardToken = func(string) error { return errors.New("injected persistence failure") }
	t.Cleanup(func() { persistDashboardToken = old })

	rewriteConfigFile(t, configPath, transitionOmittedConfig)
	reloadErr := a.Reload()
	if reloadErr == nil {
		t.Fatal("expected reload to fail on persistence failure")
	}
	if strings.Contains(reloadErr.Error(), activeToken) || strings.Contains(reloadErr.Error(), "stale-file-token") {
		t.Fatalf("reload error must not contain token contents: %v", reloadErr)
	}

	if a.Config.Dashboard.Token != activeToken || !a.Config.Dashboard.TokenFromConfig {
		t.Fatal("failed reload must leave the active runtime unchanged")
	}
	if got := snapshotStatus(t, a.Server.Handler, activeToken); got != http.StatusOK {
		t.Fatalf("snapshot with active token status = %d, want 200", got)
	}
	staleAfter, err := os.ReadFile(dashrpc.TokenFilePath())
	if err != nil {
		t.Fatalf("read token file after failed reload: %v", err)
	}
	if string(staleAfter) != string(staleBefore) {
		t.Fatal("failed reload must leave the existing token file untouched")
	}

	persistDashboardToken = old
	if err := a.Reload(); err != nil {
		t.Fatalf("retry reload after restoring persistence: %v", err)
	}
	discovered, err := dashrpc.LoadToken()
	if err != nil {
		t.Fatalf("load token after retry: %v", err)
	}
	if discovered != a.Config.Dashboard.Token {
		t.Fatal("retry must publish the carried-over runtime token")
	}
}
