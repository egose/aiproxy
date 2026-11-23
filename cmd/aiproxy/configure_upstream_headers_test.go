package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestConfigurePreservesScopedUpstreamHeaderExpressions(t *testing.T) {
	t.Setenv("AIPROXY_CLI_FIXED_HEADER", "expanded-fixture-secret")
	const root = `(
  { X-Root = env("AIPROXY_CLI_FIXED_HEADER") # root expression comment with }
  }
)`
	const provider = `{ X-Provider = "provider-fixture-secret" }`
	const model = `{
      # keep this comment and env() spelling
      X-Model = env("AIPROXY_CLI_FIXED_HEADER")
    }`
	const overlay = `{ X-Child-Model = env("AIPROXY_CLI_FIXED_HEADER") }`
	const alias = `{ X-Alias = "alias-fixture-secret" }`
	source := `upstream_headers = ` + root + `
listener "http" "public" { address = "127.0.0.1:8080" }
auth "main" { mode = "none" }
provider "openai" "base" {
  api_key = "fixture-base"
  upstream_headers = ` + provider + `
  model "m" { upstream_headers = ` + model + ` }
  model "n" { upstream_headers = {} }
}
provider "openai" "child" {
  extends = "base"
  api_key = "fixture-child"
  upstream_headers = { X-Child = "child-fixture-secret" }
  model "m" { upstream_headers = ` + overlay + ` }
  model "n" {
    upstream_headers = {}
    encrypted_reasoning {}
  }
}
alias "a" {
  algorithm = "round_robin"
  upstream_headers = ` + alias + `
  providers = ["base", "child"]
  model = "m"
}
`
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, edit := range []struct {
		name string
		args []string
	}{
		{"root", []string{"upstream", "--user-agent", "Edited/1.0"}},
		{"provider", []string{"provider", "--name", "base", "--display-name", "Edited"}},
		{"retained-model", []string{"provider", "--name", "base", "--model", "m=upstream/replacement", "--model", "n"}},
		{"derived-provider", []string{"provider", "--name", "child", "--display-name", "Edited derivative"}},
		{"derived-overlay-reasoning-removal", []string{"provider", "--name", "child", "--display-name", "Move to inherited reasoning", "--no-model-reasoning", "n"}},
		{"alias", []string{"alias", "--name", "a", "--algorithm", "least_connections"}},
	} {
		t.Run(edit.name, func(t *testing.T) {
			command := append(append([]string{"configure"}, edit.args...), "--config", path, "--non-interactive")
			stdout, stderr, err := executeRootCommand("", command...)
			if err != nil {
				t.Fatalf("configure %v: %v\n%s\n%s", edit.args, err, stdout, stderr)
			}
			if strings.Contains(stdout+stderr, "-fixture-secret") || strings.Contains(stdout+stderr, "AIPROXY_CLI_FIXED_HEADER") {
				t.Fatal("configure exposed a fixed header expression or its value")
			}
			saved := mustReadFile(t, path)
			for _, expr := range []string{root, provider, model, overlay, alias} {
				if !strings.Contains(saved, "upstream_headers = "+expr) {
					t.Fatal("configure changed a raw map expression")
				}
			}
			if strings.Contains(saved, "expanded-fixture-secret") || strings.Count(saved, "upstream_headers = {}") != 2 {
				t.Fatal("configure expanded env() or lost an explicit empty map")
			}
			loaded, err := config.LoadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			base, _ := loaded.Catalog.Provider("base")
			child, _ := loaded.Catalog.Provider("child")
			a, _ := loaded.Catalog.Alias("a")
			if loaded.UpstreamHeaders.Values()["X-Root"] != "expanded-fixture-secret" || base.UpstreamHeaders.Values()["X-Provider"] != "provider-fixture-secret" ||
				base.ModelByName["m"].UpstreamHeaders.Values()["X-Model"] != "expanded-fixture-secret" || base.ModelByName["n"].UpstreamHeaders.Values() == nil ||
				child.UpstreamHeaders.Values()["X-Child"] != "child-fixture-secret" || child.ModelByName["m"].UpstreamHeaders.Values()["X-Child-Model"] != "expanded-fixture-secret" ||
				child.ModelByName["n"].UpstreamHeaders.Values() == nil || a.UpstreamHeaders.Values()["X-Alias"] != "alias-fixture-secret" {
				t.Fatal("configure changed where a map was declared or how it loads")
			}
		})
	}
	loaded, err := config.LoadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, retained, _ := loaded.Catalog.Model("base", "m")
	_, child, _ := loaded.Catalog.Model("child", "n")
	aliasConfig, _ := loaded.Catalog.Alias("a")
	if loaded.UserAgent != "Edited/1.0" || retained.UpstreamName != "upstream/replacement" || child.EncryptedReasoning != nil || aliasConfig.Algorithm != config.AlgorithmLeastConnections {
		t.Fatal("requested unrelated edits did not take effect")
	}
}

func TestConfigureAliasCannotDropInvalidUpstreamHeaderBlock(t *testing.T) {
	source := reasoningConfigureHeader + `
provider "openai" "base" {
  api_key = "fixture-base"
  model "m" {}
}
alias "a" {
  algorithm = "round_robin"
  providers = ["base"]
  model = "m"
  upstream_headers { X-Fixture = "fixture-not-for-diagnostics" }
}
`
	path := filepath.Join(t.TempDir(), "config.hcl")
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, err := executeRootCommand("", "configure", "alias", "--non-interactive", "--config", path, "--name", "a", "--algorithm", "least_connections")
	if err == nil || !strings.Contains(err.Error(), "upstream_headers must be a map attribute") || strings.Contains(stdout+stderr+err.Error(), "fixture-not-for-diagnostics") {
		t.Fatalf("expected a safe validation error, got %v", err)
	}
	if mustReadFile(t, path) != source {
		t.Fatal("configure replaced the file despite an invalid map")
	}
}
