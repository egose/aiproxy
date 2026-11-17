package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

const configureFeatureBase = `listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
`

func writeFeatureFixture(t *testing.T, extra string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(configureFeatureBase+extra), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFeatureFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func loadFeatureFixture(t *testing.T, path string) *config.Runtime {
	t.Helper()
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestConfigureDashboardUICombinations(t *testing.T) {
	for _, tc := range []struct {
		name, seed, input, advice string
		args                      []string
		dashboard, webUI          bool
	}{
		{
			name: "both via menu", input: configureDashboardMenu + "\ny\ny\n\ny\n",
			advice: "require a restart", dashboard: true, webUI: true,
		},
		{
			name: "TUI only", args: []string{"dashboard", "--non-interactive", "--dashboard"},
			advice: "require a restart", dashboard: true,
		},
		{
			name: "shell only", args: []string{"dashboard", "--non-interactive", "--web-ui"},
			advice: "Only the Web UI shell", webUI: true,
		},
		{
			name: "management only", args: []string{"dashboard", "--non-interactive", "--web-ui"},
			seed:   "multi_tenancy {}\ndatabase { url = \"postgres://localhost/workspaces\" }\n",
			advice: "workspace management is available", webUI: true,
		},
		{
			name: "disable dashboard", args: []string{"dashboard", "--non-interactive", "--dashboard=false"},
			seed: "dashboard {}\nweb_ui {}\n", advice: "SIGHUP", webUI: true,
		},
		{
			name: "disable web UI", args: []string{"dashboard", "--non-interactive", "--web-ui=false"},
			seed: "dashboard {}\nweb_ui {}\n", advice: "SIGHUP", dashboard: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFeatureFixture(t, tc.seed)
			args := append([]string{"configure", "--config", path}, tc.args...)
			stdout, stderr, err := executeRootCommand(tc.input, args...)
			if err != nil {
				t.Fatalf("configure: %v\n%s\n%s", err, stdout, stderr)
			}
			if !strings.Contains(stdout, tc.advice) || !strings.Contains(stdout, path) {
				t.Fatalf("missing guidance %q or config path in %s", tc.advice, stdout)
			}
			rt := loadFeatureFixture(t, path)
			if rt.Dashboard.Enabled != tc.dashboard || rt.WebUI.Enabled != tc.webUI || rt.Dashboard.Token != "" {
				t.Fatalf("unexpected feature state: dashboard=%t web_ui=%t explicit dashboard token=%t", rt.Dashboard.Enabled, rt.WebUI.Enabled, rt.Dashboard.Token != "")
			}
		})
	}
}

func TestConfigureDashboardTokenPreserveReplaceAndClear(t *testing.T) {
	const token = "fixture-dashboard-secret"
	const dashboard = "dashboard {\n  token = \"" + token + "\" # keep this comment\n  allow_insecure_remote = false\n}\n"
	path := writeFeatureFixture(t, dashboard)
	args := []string{"configure", "dashboard", "--config", path}
	run := func(input string, flags ...string) string {
		t.Helper()
		stdout, stderr, err := executeRootCommand(input, append(append([]string(nil), args...), flags...)...)
		if err != nil {
			t.Fatalf("configure: %v\n%s\n%s", err, stdout, stderr)
		}
		if strings.Contains(stdout+stderr, token) {
			t.Fatal("configure printed the existing dashboard token")
		}
		return stdout
	}
	run("\ny\n\ny\n")
	if got := readFeatureFixture(t, path); !strings.Contains(got, dashboard) {
		t.Fatalf("existing dashboard block changed:\n%s", got)
	}

	t.Setenv("TEST_DASHBOARD_TOKEN", "env-fixture-secret")
	out := run("", "--non-interactive", "--token-env", "TEST_DASHBOARD_TOKEN")
	if strings.Contains(out, "env-fixture-secret") {
		t.Fatal("configure printed the environment value")
	}
	if got := readFeatureFixture(t, path); !strings.Contains(got, `token = env("TEST_DASHBOARD_TOKEN") # keep this comment`) {
		t.Fatalf("token was not replaced in place:\n%s", got)
	}
	if rt := loadFeatureFixture(t, path); rt.Dashboard.Token != "env-fixture-secret" {
		t.Fatal("token environment variable did not resolve")
	}

	run("", "--non-interactive", "--token-env=")
	if got := readFeatureFixture(t, path); strings.Contains(got, "token =") || !strings.Contains(got, "allow_insecure_remote = false") || !strings.Contains(got, "# keep this comment") {
		t.Fatalf("clearing the token changed other settings:\n%s", got)
	}
	if rt := loadFeatureFixture(t, path); !rt.Dashboard.Enabled || rt.Dashboard.Token != "" {
		t.Fatal("cleared token did not restore server-generated mode")
	}
}

func TestConfigureMultiTenancyFirstSetup(t *testing.T) {
	t.Setenv("AIPROXY_DB_URL", "")
	path := writeFeatureFixture(t, "")
	stdout, stderr, err := executeRootCommand(configureMultiTenancyMenu+"\ny\n\n\ny\n", "configure", "--config", path)
	if err != nil {
		t.Fatalf("configure: %v\n%s\n%s", err, stdout, stderr)
	}
	for _, text := range []string{"AIPROXY_DB_URL", "AIPROXY_JWT_SECRET", "AIPROXY_ADMIN_EMAIL", "AIPROXY_ADMIN_PASSWORD", "aiproxy migrate up", "require a restart"} {
		if !strings.Contains(stdout, text) {
			t.Errorf("missing %q in setup guidance:\n%s", text, stdout)
		}
	}
	got := readFeatureFixture(t, path)
	if !strings.Contains(got, `url = env("AIPROXY_DB_URL")`) || !strings.Contains(got, "allow_public_registration = false") {
		t.Fatalf("missing database reference or safe registration default:\n%s", got)
	}
	t.Setenv("AIPROXY_DB_URL", "postgres://localhost/workspaces")
	rt := loadFeatureFixture(t, path)
	if !rt.MultiTenancy.Enabled || rt.MultiTenancy.AllowPublicRegistration || rt.Database.URL != "postgres://localhost/workspaces" || rt.Dashboard.Enabled || rt.WebUI.Enabled {
		t.Fatal("unexpected runtime for newly enabled multi-tenancy")
	}
}

func TestConfigureMultiTenancyPreservesSettings(t *testing.T) {
	const url = "postgres://test:fixture-db-secret@localhost/workspaces"
	const database = "database { url = \"" + url + "\" } # database stays\n"
	for _, tc := range []struct {
		name, block  string
		registration bool
	}{
		{"explicit open", "multi_tenancy {\n  enabled = true\n  allow_public_registration = true\n}\n", true},
		{"implicit open", "multi_tenancy {}\n", true},
		{"explicit closed", "multi_tenancy {\n  enabled = true\n  allow_public_registration = false\n}\n", false},
		{"disabled with no existing registration setting", "multi_tenancy { enabled = false }\n", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			seed := database + tc.block
			path := writeFeatureFixture(t, seed)
			stdout, stderr, err := executeRootCommand("y\n\n\ny\n", "configure", "multi-tenancy", "--config", path)
			if err != nil {
				t.Fatalf("configure: %v\n%s\n%s", err, stdout, stderr)
			}
			if strings.Contains(stdout+stderr, "fixture-db-secret") {
				t.Fatal("configure printed the PostgreSQL URL")
			}
			got := readFeatureFixture(t, path)
			if !strings.Contains(got, database) {
				t.Fatal("configure changed the database block")
			}
			rt := loadFeatureFixture(t, path)
			if !rt.MultiTenancy.Enabled || rt.MultiTenancy.AllowPublicRegistration != tc.registration {
				t.Fatalf("unexpected multi-tenancy options: %#v", rt.MultiTenancy)
			}
		})
	}
}

func TestConfigureMultiTenancyEditsAndDisable(t *testing.T) {
	const database = "database { url = env(\"CURRENT_PG_URL\") } # preserve this\n"
	path := writeFeatureFixture(t, database+"multi_tenancy {}\n")
	t.Setenv("CURRENT_PG_URL", "")
	run := func(flags ...string) string {
		t.Helper()
		args := []string{"configure", "multi-tenancy", "--config", path, "--non-interactive"}
		stdout, stderr, err := executeRootCommand("", append(args, flags...)...)
		if err != nil {
			t.Fatalf("configure: %v\n%s\n%s", err, stdout, stderr)
		}
		return stdout
	}
	if out := run("--allow-public-registration=false"); !strings.Contains(out, "SIGHUP") {
		t.Fatalf("expected reload guidance:\n%s", out)
	}
	if got := readFeatureFixture(t, path); !strings.Contains(got, database) || !strings.Contains(got, "allow_public_registration = false") {
		t.Fatalf("registration edit did not preserve the unresolved URL reference:\n%s", got)
	}
	if out := run("--database-url-env", "NEW_PG_URL"); !strings.Contains(out, "require a restart") || !strings.Contains(out, "aiproxy migrate up") {
		t.Fatalf("expected restart and migration guidance:\n%s", out)
	}
	if out := run("--enabled=false"); !strings.Contains(out, "require a restart") {
		t.Fatalf("expected restart guidance:\n%s", out)
	}
	if got := readFeatureFixture(t, path); !strings.Contains(got, strings.ReplaceAll(database, "CURRENT_PG_URL", "NEW_PG_URL")) {
		t.Fatal("disabling multi-tenancy changed its database reference")
	}
	if rt := loadFeatureFixture(t, path); rt.MultiTenancy.Enabled || rt.MultiTenancy.AllowPublicRegistration {
		t.Fatal("disabling multi-tenancy changed its saved registration setting or left it enabled")
	}
}

func TestConfigureFeaturesDoNotSaveRejectedEdits(t *testing.T) {
	for _, tc := range []struct {
		name, seed, input, wantErr string
		args                       []string
	}{
		{
			name: "decline both multi-tenancy changes", input: "y\n\n\nn\n",
			args: []string{"multi-tenancy"},
		},
		{
			name: "missing URL", wantErr: "requires database.url",
			args: []string{"multi-tenancy", "--non-interactive", "--enabled"},
		},
		{
			name: "literal passed as DB environment variable", wantErr: "environment variable name",
			args: []string{"multi-tenancy", "--non-interactive", "--enabled", "--database-url-env", "postgres://example.test"},
		},
		{
			name: "token for disabled API", wantErr: "requires the Dashboard API",
			args: []string{"dashboard", "--non-interactive", "--token-env", "DASHBOARD_TOKEN"},
		},
		{
			name: "duplicates", seed: "dashboard {}\ndashboard {}\n", wantErr: "at most one",
			args: []string{"dashboard", "--non-interactive", "--web-ui"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := writeFeatureFixture(t, tc.seed)
			_, _, err := executeRootCommand(tc.input, append([]string{"configure", "--config", path}, tc.args...)...)
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("configure error = %v, expected %q", err, tc.wantErr)
			}
			if got := readFeatureFixture(t, path); got != configureFeatureBase+tc.seed {
				t.Fatalf("configure unexpectedly changed the file:\n%s", got)
			}
		})
	}
}

func TestConfigureFeaturesNonInteractiveDefaultPath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	for _, args := range [][]string{
		{"configure", "dashboard", "--non-interactive", "--dashboard", "--web-ui"},
		{"configure", "multi-tenancy", "--non-interactive", "--enabled", "--database-url-env", "PG_URL"},
	} {
		if stdout, stderr, err := executeRootCommand("", args...); err != nil {
			t.Fatalf("configure: %v\n%s\n%s", err, stdout, stderr)
		}
	}
	path := filepath.Join(dir, "aiproxy", "config.hcl")
	got := readFeatureFixture(t, path)
	for _, text := range []string{"dashboard {}", "web_ui {", "multi_tenancy {", `url = env("PG_URL")`, "allow_public_registration = false"} {
		if !strings.Contains(got, text) {
			t.Errorf("missing %q in %s", text, got)
		}
	}
}
