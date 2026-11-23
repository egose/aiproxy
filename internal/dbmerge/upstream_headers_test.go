package dbmerge

import (
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func TestMaterializeProviderStripsStaticUpstreamHeaderDeclarations(t *testing.T) {
	src := []byte(`
upstream_headers = { X-Root = "root" }
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "base" {
  api_key = "fixture"
  upstream_headers = { X-Base = "base" }
  model "m" { upstream_headers = { X-Model = "model" } }
}
alias "a" {
  algorithm = "round_robin"
  upstream_headers = { X-Alias = "alias" }
  providers = ["base"]
  model = "m"
}
`)
	rt, err := config.Load(src, "fixture.hcl")
	if err != nil {
		t.Fatal(err)
	}
	base, _ := rt.Catalog.Provider("base")
	a, _ := rt.Catalog.Alias("a")
	indexed := base.ModelByName["m"]
	indexed.Name = "indexed"
	base.ModelByName["indexed"] = indexed
	for _, enabled := range []bool{true, false} {
		row := store.DBProvider{
			Name: "managed", Type: string(base.Type), Extends: base.Name, WorkspaceID: store.SystemWorkspaceID,
			Enabled: enabled, APIKeyRefKey: "fixture",
		}
		p, err := BuildProvider(row, nil, map[string]config.Provider{base.Name: base})
		if err != nil {
			t.Fatal(err)
		}
		p = rt.ApplyProviderDefaults(p)
		if err := config.ValidateDynamicProvider(p); err != nil {
			t.Fatal(err)
		}
		if !p.DatabaseManaged || p.UpstreamHeaders.Values() != nil || len(p.Models) != 2 || len(p.ModelByName) != 2 {
			t.Fatal("managed derivative retained headers or lost its inherited inventory")
		}
		for _, model := range p.Models {
			indexed := p.ModelByName[model.Name]
			if model.UpstreamHeaders.Values() != nil || indexed.UpstreamHeaders.Values() != nil {
				t.Fatalf("managed model %q retained static declarations", model.Name)
			}
			if got := config.ResolveUpstreamHeaders(rt.UpstreamHeaders, p, indexed, &a); got != nil {
				t.Fatalf("managed model %q resolved fixed headers from root or static alias", model.Name)
			}
		}
	}
	if base.UpstreamHeaders.Values()["X-Base"] != "base" || base.Models[0].UpstreamHeaders.Values()["X-Model"] != "model" || base.ModelByName["indexed"].UpstreamHeaders.Values()["X-Model"] != "model" {
		t.Fatal("managed materialization changed the static provider's own declarations")
	}
}
