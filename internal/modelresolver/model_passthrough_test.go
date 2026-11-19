package modelresolver

import (
	"errors"
	"strings"
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

func TestResolveModelPassthroughOpenCodeProtocolSnapshot(t *testing.T) {
	const source = `
listener "http" "fixture" { address = ":8080" }
auth "fixture" { mode = "none" }
provider "opencode-zen" "zen" {
  model_passthrough "family" {
    match = "^org/.+$"
    protocol = "chat"
    capabilities = ["chat"]
  }
}
`
	load := func(src string) *config.Runtime {
		t.Helper()
		rt, err := config.Load([]byte(src), "passthrough.hcl")
		if err != nil {
			t.Fatal(err)
		}
		return rt
	}
	resolver := New(load(source))
	admitted, match := resolver.ResolveModelPassthrough("zen/org/new", nil)
	if match != config.ModelPassthroughMatched || admitted.Kind != KindModelPassthrough || admitted.PassthroughRule.Protocol() != config.ModelProtocolChat {
		t.Fatalf("matched rule protocol = %q (%v)", admitted.PassthroughRule.Protocol(), match)
	}
	clear(admitted.Provider.ModelPassthroughs)
	updated := NewWithPrevious(load(strings.Replace(source, `protocol = "chat"`, `protocol = "messages"`, 1)), resolver)
	next, match := updated.ResolveModelPassthrough("zen/org/new", nil)
	old, oldMatch := resolver.ResolveModelPassthrough("zen/org/new", nil)
	if match != config.ModelPassthroughMatched || next.PassthroughRule.Protocol() != config.ModelProtocolMessages ||
		oldMatch != config.ModelPassthroughMatched || old.PassthroughRule.Protocol() != config.ModelProtocolChat ||
		admitted.PassthroughRule.Protocol() != config.ModelProtocolChat || next.Model.Protocol != "" || next.Model.Name != "" {
		t.Fatal("a new catalog or a returned provider changed the admitted rule, or rule resolution fabricated a model")
	}
}

func TestResolveModelPassthroughGeminiPathEligibility(t *testing.T) {
	rt, err := config.Load([]byte(`
listener "http" "fixture" { address = ":8080" }
auth "fixture" { mode = "none" }
provider "gemini" "gemini" {
  api_key = "test-key"
  model_passthrough "path" {
    match = ".*"
    capabilities = ["chat", "responses", "embeddings"]
  }
}
provider "opencode-zen" "zen" {
  model "org/exact" { protocol = "chat" }
  model_passthrough "path" {
    match = "gemini-.+|models/.+|org/(both|exact)"
    protocol = "gemini"
    capabilities = ["responses"]
  }
  model_passthrough "body" {
    match = "gemini-both|org/(both|exact|new)"
    protocol = "chat"
    capabilities = ["chat"]
  }
}
`), "gemini.hcl")
	if err != nil {
		t.Fatal(err)
	}
	resolver := New(rt)
	for _, tc := range []struct {
		provider, suffix, rule string
		protocol               config.ModelProtocol
		match                  config.ModelPassthroughMatch
	}{
		{"gemini", "gemini-next_2.0", "path", "", config.ModelPassthroughMatched},
		{"gemini", "models/gemini-next_2.0", "", "", config.ModelPassthroughNoMatch},
		{"gemini", strings.Repeat("a", config.MaxModelPassthroughSuffixBytes), "path", "", config.ModelPassthroughMatched},
		{"gemini", strings.Repeat("a", config.MaxModelPassthroughSuffixBytes+1), "", "", config.ModelPassthroughNoMatch},
		{"zen", "gemini-next_2.0", "path", config.ModelProtocolGemini, config.ModelPassthroughMatched},
		{"zen", "models/gemini-next_2.0", "", "", config.ModelPassthroughNoMatch},
		{"zen", "org/both", "", "", config.ModelPassthroughAmbiguous},
		{"zen", "gemini-both", "", "", config.ModelPassthroughAmbiguous},
		{"zen", "org/exact", "", "", config.ModelPassthroughNoMatch},
		{"zen", "org/new", "body", config.ModelProtocolChat, config.ModelPassthroughMatched},
	} {
		public := tc.provider + "/" + tc.suffix
		got, match := resolver.ResolveModelPassthrough(public, nil)
		if match != tc.match || got.PassthroughRule.Name() != tc.rule || got.PassthroughRule.Protocol() != tc.protocol {
			t.Errorf("%q: rule %q, protocol %q, match %v; want %q, %q, %v", public, got.PassthroughRule.Name(), got.PassthroughRule.Protocol(), match, tc.rule, tc.protocol, tc.match)
		}
		if match == config.ModelPassthroughMatched && (got.Kind != KindModelPassthrough || got.PassthroughSuffix != tc.suffix || got.Model.Name != "") {
			t.Errorf("%q changed identity or entered the literal inventory", public)
		}
	}
}
