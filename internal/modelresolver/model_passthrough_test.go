package modelresolver

import (
	"errors"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/google/uuid"
)

func TestResolveModelPassthroughKeepsLiteralLookupAndScopesBeforeMatching(t *testing.T) {
	rt, err := config.Load([]byte(`
listener "http" "fixture" {
  address = ":8080"
}
auth "fixture" {
  mode = "none"
}
provider "openai" "p" {
  api_key = "test-key"
  model "org/exact" {
    upstream_name = "pinned"
  }
  model_passthrough "family" {
    match = "^org/.+$"
    capabilities = ["chat"]
  }
  model_passthrough "overlap" {
    match = "^org/(exact|ambiguous)$"
    capabilities = ["embeddings"]
  }
}
`), "passthrough.hcl")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := rt.Catalog.Provider("p")
	p.Workspace = identity.InWorkspace(uuid.New())
	rt.Catalog = config.NewCatalog([]config.Provider{p}, nil, nil)
	r := New(rt)
	inScope, outOfScope, unscoped := p.Workspace, identity.InWorkspace(uuid.New()), identity.WorkspaceScope{}

	got, err := r.Resolve("p/org/exact")
	if err != nil || got.Kind != KindDirect || got.Model.UpstreamName != "pinned" {
		t.Fatalf("literal resolution = %+v, %v", got, err)
	}
	if _, match := r.ResolveModelPassthrough("p/org/exact", &inScope); match != config.ModelPassthroughNoMatch {
		t.Fatalf("exact ID participated in rule matching: %v", match)
	}
	if _, match := r.ResolveModelPassthrough("p/org/ambiguous", &inScope); match != config.ModelPassthroughAmbiguous {
		t.Fatalf("same workspace did not see overlapping rules: %v", match)
	}
	for _, scope := range []*identity.WorkspaceScope{&outOfScope, &unscoped} {
		if _, match := r.ResolveModelPassthrough("p/org/ambiguous", scope); match != config.ModelPassthroughNoMatch {
			t.Fatalf("out-of-scope provider participated in matching: %v", match)
		}
	}
	got, match := r.ResolveModelPassthrough("p/org/new", &inScope)
	if match != config.ModelPassthroughMatched || got.Kind != KindModelPassthrough || got.PassthroughSuffix != "org/new" ||
		got.PassthroughRule.Name() != "family" || got.Workspace != p.Workspace || got.Provider.APIKey != p.APIKey || got.Model.Name != "" {
		t.Fatalf("passthrough didn't preserve separate target, rule and workspace: %+v, %v", got, match)
	}
	for _, public := range []string{"p/org/new", "p/family"} {
		_, err := r.Resolve(public)
		var unknown ErrUnknownModel
		if !errors.As(err, &unknown) {
			t.Fatalf("ordinary literal resolver registered %q: %v", public, err)
		}
		if _, _, ok := r.Model("p", public[len("p/"):]); ok || r.ModelVisibleInWorkspace(public, p.Workspace) {
			t.Fatalf("literal catalog registered %q", public)
		}
	}
}
