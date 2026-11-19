package dbmerge

import (
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func TestManagedModelPassthroughInheritanceIsLiteralOnly(t *testing.T) {
	const source = `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "base" {
  api_key = "static-key"
  model "literal" { upstream_name = "renamed" }
  model_passthrough "family" {
    match = "vendor/.+"
    capabilities = ["chat"]
  }
}`
	rt, err := config.Load([]byte(source), "base.hcl")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := rt.Catalog.Provider("base")
	bases := map[string]config.Provider{base.Name: base}
	row := store.DBProvider{
		Name: "child", Extends: "base", Type: "openai", Enabled: true, WorkspaceID: store.SystemWorkspaceID,
		APIKeyRefKey: "managed-local",
	}
	child, err := BuildProvider(row, nil, bases)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateDynamicProvider(child); err != nil {
		t.Fatal(err)
	}
	if !child.DatabaseManaged || len(child.ModelPassthroughs) != 0 ||
		len(child.Models) != 1 || len(child.ModelByName) != 1 || child.ModelByName["literal"].UpstreamName != "renamed" ||
		child.APIKey != "" || child.APIKeyRef == nil || child.APIKeyRef.Key != "managed-local" {
		t.Fatal("managed derivative copied a static rule/credential or lost a literal model")
	}
	if _, status := base.MatchModelPassthrough("vendor/new"); status != config.ModelPassthroughMatched {
		t.Fatal("managed inheritance changed the base")
	}
	child.ModelPassthroughs = base.ModelPassthroughs
	if err := config.ValidateDynamicProvider(child); err == nil || !strings.Contains(err.Error(), "static-config-only") {
		t.Fatalf("managed validation did not reject injected rules: %v", err)
	}
	row.APIKeyRefKey = ""
	child, err = BuildProvider(row, nil, bases)
	if err != nil {
		t.Fatal(err)
	}
	if err := config.ValidateDynamicProvider(child); err == nil || !strings.Contains(err.Error(), "require a non-empty api_key") {
		t.Fatalf("managed derivative lost local credential requirement: %v", err)
	}
	base.Models, base.ModelByName = nil, nil
	bases[base.Name] = base
	for _, enabled := range []bool{true, false} {
		row.Enabled = enabled
		if _, err := MaterializeProvider(row, nil, bases); err == nil || !strings.Contains(err.Error(), "at least one literal model") {
			t.Fatalf("managed derivative accepted a rule-only base: %v", err)
		}
	}
}
