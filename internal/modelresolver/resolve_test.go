package modelresolver

import (
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func buildRT() *config.Runtime {
	rt := &config.Runtime{
		ProviderByName: map[string]config.Provider{
			"openai": {
				Name:    "openai",
				BaseURL: "https://api.openai.com/v1",
				APIKey:  "sk",
				ModelByName: map[string]config.Model{
					"gpt-4o-mini": {Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"},
				},
			},
		},
		AliasByName: map[string]config.Alias{
			"chat_default": {
				Name:      "chat_default",
				Algorithm: config.AlgorithmRoundRobin,
				Targets: []config.AliasTarget{
					{Provider: "openai", Model: "gpt-4o-mini"},
				},
			},
		},
	}
	return rt
}

func TestResolveDirectProviderModel(t *testing.T) {
	r := New(buildRT())
	res, err := r.Resolve("openai/gpt-4o-mini")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Kind != KindDirect {
		t.Errorf("kind = %v, want direct", res.Kind)
	}
	if res.Provider.Name != "openai" {
		t.Errorf("provider = %q", res.Provider.Name)
	}
	if res.Model.Name != "gpt-4o-mini" {
		t.Errorf("model = %q", res.Model.Name)
	}
}

func TestResolveAlias(t *testing.T) {
	r := New(buildRT())
	res, err := r.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if res.Kind != KindAlias {
		t.Errorf("kind = %v, want alias", res.Kind)
	}
	if res.Alias.Name != "chat_default" {
		t.Errorf("alias = %q", res.Alias.Name)
	}
	if res.Selector == nil {
		t.Errorf("selector is nil")
	}
}

func TestResolveUnknownProvider(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve("azure/foo"); err == nil {
		t.Errorf("expected error for unknown provider")
	}
}

func TestResolveUnknownModel(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve("openai/unknown-model"); err == nil {
		t.Errorf("expected error for unknown model")
	}
}

func TestResolveUnknownAlias(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve("alias/missing"); err == nil {
		t.Errorf("expected error for unknown alias")
	}
}

func TestResolveEmptyModel(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve(""); err == nil {
		t.Errorf("expected error for empty model")
	}
}

func TestResolveUnqualifiedModelFails(t *testing.T) {
	r := New(buildRT())
	if _, err := r.Resolve("gpt-4o-mini"); err == nil {
		t.Errorf("expected error for unqualified model")
	}
}
