package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

const cliModelPassthroughRule = `model_passthrough "family" {
    match = env("CLI_PASSTHROUGH_PATTERN")
    capabilities = ["chat"]
  }`

const cliModelPassthroughGrant = `[
      env("CLI_PASSTHROUGH_GRANT"), # preserve the operator's expression
    ]`

func cliModelPassthroughFixture(t *testing.T, mixed bool) string {
	t.Helper()
	t.Setenv("CLI_PASSTHROUGH_PATTERN", `(?i)^vendor/FOO|vendor/FOOBAR$`)
	t.Setenv("CLI_PASSTHROUGH_GRANT", "router/family")
	source := `listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "bearer_static"
  client "app" {
    token = "app-token"
    allowed_models = ["router/cached"]
    allowed_model_passthroughs = ` + cliModelPassthroughGrant + `
  }
}
provider "openai" "router" {
  api_key = "router-key"
  ` + cliModelPassthroughRule + `
  model_passthrough "another" {
    match = "bar"
    capabilities = ["embeddings"]
  }
`
	if mixed {
		source += "  model \"cached\" {}\n"
	}
	source += `}
provider "openai" "base" {
  api_key = "base-key"
  model "literal" {}
}
`
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.LoadFile(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestConfigureModelPassthroughPreservesRuleAndGrantExpressions(t *testing.T) {
	for _, mixed := range []bool{false, true} {
		name := "rule-only"
		if mixed {
			name = "mixed"
		}
		t.Run(name, func(t *testing.T) {
			path := cliModelPassthroughFixture(t, mixed)
			provider := []string{"configure", "provider", "--config", path, "--non-interactive", "--name", "router"}
			auth := []string{"configure", "auth", "--config", path, "--non-interactive"}
			edits := [][]string{
				append(slices.Clone(provider), "--display-name", "Edited"),
				append(slices.Clone(auth), "--rate-limit-rpm", "80"),
				append(slices.Clone(auth), "--client-token", "app=rotated-token", "--client-allowed-models", "app=router/cached"),
				append(slices.Clone(provider), "--enabled=false"),
			}
			for _, args := range edits {
				if stdout, stderr, err := executeRootCommand("", args...); err != nil {
					t.Fatalf("configure: %v\n%s\n%s", err, stdout, stderr)
				}
				source := mustReadFile(t, path)
				for _, want := range []string{cliModelPassthroughRule, cliModelPassthroughGrant, `model_passthrough "another"`} {
					if !strings.Contains(source, want) {
						t.Fatalf("edit discarded %q", want)
					}
				}
			}
			disabled, err := config.LoadFile(path)
			if err != nil || disabled.Catalog.DisabledProviderCount() != 1 {
				t.Fatalf("provider could not be disabled with existing grants: %v", err)
			}
			if _, _, err := executeRootCommand("", append(provider, "--enabled=true", "--api-key", "new-router-key")...); err != nil {
				t.Fatal(err)
			}
			t.Setenv("CLI_PASSTHROUGH_PATTERN", `(?i)^vendor/BAZ$`)
			t.Setenv("CLI_PASSTHROUGH_GRANT", "router/another")
			rt, err := config.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			p, found := rt.Catalog.Provider("router")
			m, status := p.MatchModelPassthrough("vendor/baz")
			c := rt.Auth.Clients["app"]
			if !found || p.DisplayName != "Edited" || (len(p.Models) == 1) != mixed ||
				status != config.ModelPassthroughMatched || m.Name() != "family" ||
				c.Token != "rotated-token" || !slices.Equal(c.AllowedModelPassthroughs, []string{"router/another"}) ||
				!slices.Equal(c.AllowedModels, []string{"router/cached"}) {
				t.Fatal("reloaded config lost the provider, literal inventory, rule behavior, or grants")
			}
		})
	}
}

func TestConfigureModelPassthroughUnsafeEditsLeaveConfigUntouched(t *testing.T) {
	path := cliModelPassthroughFixture(t, true)
	original := mustReadFile(t, path)
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"provider", "--name", "router", "--type", "anthropic"}, "not supported by provider type"},
		{[]string{"provider", "--name", "router", "--extends", "base"}, "derived provider cannot declare model_passthrough"},
		{[]string{"provider", "--name", "router", "--delete"}, "not a declared static model_passthrough rule"},
		{[]string{"auth", "--client-token", "replacement=replacement-token"}, "cannot preserve allowed_model_passthroughs"},
		{[]string{"auth", "--mode", "none"}, "cannot preserve allowed_model_passthroughs"},
	} {
		args := append([]string{"configure", "--config", path}, tc.args...)
		args = append(args, "--non-interactive")
		if _, _, err := executeRootCommand("", args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("edit %v: got %v, want %q", tc.args, err, tc.want)
		}
		if mustReadFile(t, path) != original {
			t.Fatalf("rejected edit rewrote config: %v", tc.args)
		}
	}
}

func TestConfigureModelPassthroughInteractiveRuleOnlyProvider(t *testing.T) {
	path := cliModelPassthroughFixture(t, false)
	answers := []string{"", "", "", "Edited interactively", "", "", "", "", "", "", "", "", "n", "", ""}
	stdout, stderr, err := executeRootCommand(strings.Join(answers, "\n")+"\n", "configure", "provider", "--config", path, "--name", "router")
	if err != nil {
		t.Fatalf("configure: %v\n%s\n%s", err, stdout, stderr)
	}
	rt, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	p, ok := rt.Catalog.Provider("router")
	rule, status := p.MatchModelPassthrough("vendor/foobar")
	if !ok || p.DisplayName != "Edited interactively" || len(p.Models) != 0 ||
		status != config.ModelPassthroughMatched || rule.Name() != "family" ||
		!strings.Contains(mustReadFile(t, path), cliModelPassthroughRule) {
		t.Fatal("interactive edit did not preserve rule-only provider")
	}
}
