package modelresolver

import (
	"reflect"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func assertResolverReasoning(t *testing.T, got config.EncryptedReasoningResolution, passthrough bool, mismatch string, source config.EncryptedReasoningSource, inherited bool) {
	t.Helper()
	want := config.DefaultEncryptedReasoning()
	want.Passthrough, want.OnCallerMismatch = passthrough, mismatch
	if !reflect.DeepEqual(got.Policy, want) || got.Source != source || got.Inherited != inherited {
		t.Fatalf("policy=%+v source=%s inherited=%t, want policy=%+v source=%s inherited=%t", got.Policy, got.Source, got.Inherited, want, source, inherited)
	}
}

func TestResolverEncryptedReasoningUsesEachSelectedTarget(t *testing.T) {
	passthrough := false
	mismatch := config.EncryptedReasoningStripAndRetry
	strip := config.NewEncryptedReasoning(&config.EncryptedReasoningDeclaration{Passthrough: &passthrough})
	retry := config.NewEncryptedReasoning(&config.EncryptedReasoningDeclaration{OnCallerMismatch: &mismatch})
	reset := config.NewEncryptedReasoning(&config.EncryptedReasoningDeclaration{})
	providers := []config.Provider{
		{Name: "a", EncryptedReasoning: reset, Models: []config.Model{{Name: "m", UpstreamName: "m", EncryptedReasoning: strip}}},
		{Name: "b", EncryptedReasoning: retry, Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
		{Name: "c", Models: []config.Model{{Name: "m", UpstreamName: "m"}}},
	}
	targets := []config.AliasTarget{{Provider: "a", Model: "m"}, {Provider: "b", Model: "m"}, {Provider: "c", Model: "m"}}
	rt := &config.Runtime{
		EncryptedReasoning: strip,
		Catalog: config.NewCatalog(providers, nil, []config.Alias{
			{Name: "target-policy", Algorithm: config.AlgorithmRoundRobin, Targets: targets},
			{Name: "reset", Algorithm: config.AlgorithmRoundRobin, Targets: targets, EncryptedReasoning: reset},
			{Name: "retry", Algorithm: config.AlgorithmRoundRobin, Targets: targets, EncryptedReasoning: retry},
		}),
	}
	resolver := New(rt)
	for _, tc := range []struct {
		name        string
		passthrough bool
		mismatch    string
		source      config.EncryptedReasoningSource
	}{
		{"a", false, config.EncryptedReasoningFail, config.EncryptedReasoningSourceModel},
		{"b", true, config.EncryptedReasoningStripAndRetry, config.EncryptedReasoningSourceProvider},
		{"c", false, config.EncryptedReasoningFail, config.EncryptedReasoningSourceRoot},
	} {
		t.Run("direct/"+tc.name, func(t *testing.T) {
			resolved, err := resolver.Resolve(tc.name + "/m")
			if err != nil {
				t.Fatal(err)
			}
			assertResolverReasoning(t, resolver.EncryptedReasoning(resolved.Provider, resolved.Model, nil), tc.passthrough, tc.mismatch, tc.source, false)
		})
	}
	for _, aliasName := range []string{"target-policy", "reset", "retry"} {
		t.Run("alias/"+aliasName, func(t *testing.T) {
			resolved, err := resolver.Resolve("alias/" + aliasName)
			if err != nil {
				t.Fatal(err)
			}
			for _, wantTarget := range targets {
				target, release := resolved.Selector.Acquire(nil)
				release()
				if target.Provider != wantTarget.Provider || target.Model != wantTarget.Model {
					t.Fatal("policy resolution changed target selection")
				}
				provider, model, ok := resolver.Model(target.Provider, target.Model)
				if !ok {
					t.Fatal("selected target disappeared")
				}
				got := resolver.EncryptedReasoning(provider, model, &resolved.Alias)
				switch aliasName {
				case "reset":
					assertResolverReasoning(t, got, true, config.EncryptedReasoningFail, config.EncryptedReasoningSourceAlias, false)
				case "retry":
					assertResolverReasoning(t, got, true, config.EncryptedReasoningStripAndRetry, config.EncryptedReasoningSourceAlias, false)
				default:
					switch target.Provider {
					case "a":
						assertResolverReasoning(t, got, false, config.EncryptedReasoningFail, config.EncryptedReasoningSourceModel, false)
					case "b":
						assertResolverReasoning(t, got, true, config.EncryptedReasoningStripAndRetry, config.EncryptedReasoningSourceProvider, false)
					case "c":
						assertResolverReasoning(t, got, false, config.EncryptedReasoningFail, config.EncryptedReasoningSourceRoot, false)
					}
				}
			}
		})
	}
}

func TestResolverEncryptedReasoningRootSnapshotAndDynamicFallback(t *testing.T) {
	passthrough := false
	mismatch := config.EncryptedReasoningStripAndRetry
	patterns := []string{"  Original Caller Marker  "}
	rt := &config.Runtime{EncryptedReasoning: config.NewEncryptedReasoning(&config.EncryptedReasoningDeclaration{
		Passthrough: &passthrough, OnCallerMismatch: &mismatch, MatchMessages: &patterns,
	})}
	provider := rt.ApplyProviderDefaults(config.Provider{
		Name: "dynamic", Models: []config.Model{{Name: "m", UpstreamName: "m"}},
	})
	if provider.EncryptedReasoning != nil || provider.Models[0].EncryptedReasoning != nil {
		t.Fatal("dynamic target persisted root policy as a local declaration")
	}
	rt.Catalog = config.NewCatalog([]config.Provider{provider}, nil, nil)
	original := New(rt)
	checkOriginal := func() {
		t.Helper()
		p, m, ok := original.Model("dynamic", "m")
		if !ok {
			t.Fatal("dynamic model disappeared")
		}
		got := original.EncryptedReasoning(p, m, nil)
		if got.Policy.Passthrough || got.Policy.OnCallerMismatch != config.EncryptedReasoningStripAndRetry ||
			!reflect.DeepEqual(got.Policy.MatchMessages, []string{"original caller marker"}) || got.Policy.Declaration != nil ||
			got.Source != config.EncryptedReasoningSourceRoot || got.Inherited {
			t.Fatalf("admitted root snapshot changed: %+v", got)
		}
		got.Policy.MatchMessages[0] = "changed returned caller"
	}
	checkOriginal()
	*rt.EncryptedReasoning.Declaration.Passthrough = true
	*rt.EncryptedReasoning.Declaration.OnCallerMismatch = config.EncryptedReasoningFail
	(*rt.EncryptedReasoning.Declaration.MatchMessages)[0] = "changed declared caller"
	rt.EncryptedReasoning.MatchMessages[0] = "changed normalized caller"
	checkOriginal()
	rt.EncryptedReasoning = config.NewEncryptedReasoning(&config.EncryptedReasoningDeclaration{})
	reloaded := NewWithPrevious(rt, original)
	p, m, _ := reloaded.Model("dynamic", "m")
	assertResolverReasoning(t, reloaded.EncryptedReasoning(p, m, nil), true, config.EncryptedReasoningFail, config.EncryptedReasoningSourceRoot, false)
	checkOriginal()
	rt.EncryptedReasoning = nil
	removed := NewWithPrevious(rt, reloaded)
	p, m, _ = removed.Model("dynamic", "m")
	assertResolverReasoning(t, removed.EncryptedReasoning(p, m, nil), true, config.EncryptedReasoningFail, config.EncryptedReasoningSourceBuiltin, false)
	p, m, _ = reloaded.Model("dynamic", "m")
	assertResolverReasoning(t, reloaded.EncryptedReasoning(p, m, nil), true, config.EncryptedReasoningFail, config.EncryptedReasoningSourceRoot, false)
	checkOriginal()
}

func TestResolverEncryptedReasoningInheritedCatalogSnapshot(t *testing.T) {
	rt, err := config.Load([]byte(`
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
encrypted_reasoning { passthrough = false }
provider "openai" "base" {
  api_key = "base-key"
  encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }
  model "m" {
    encrypted_reasoning { passthrough = false }
  }
  model "provider-layer" {}
}
provider "openai" "child" {
  extends = "base"
  api_key = "child-key"
}
alias "a" {
  algorithm = "round_robin"
  providers = ["child"]
  model = "m"
  encrypted_reasoning {}
}
`), "test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	original := New(rt)
	admitted, err := original.Resolve("child/m")
	if err != nil {
		t.Fatal(err)
	}
	if admitted.Provider.Extends != "base" {
		t.Fatal("resolver discarded inheritance provenance")
	}
	assertResolverReasoning(t, original.EncryptedReasoning(admitted.Provider, admitted.Model, nil), false, config.EncryptedReasoningFail, config.EncryptedReasoningSourceModel, true)
	p, m, _ := original.Model("child", "provider-layer")
	assertResolverReasoning(t, original.EncryptedReasoning(p, m, nil), true, config.EncryptedReasoningStripAndRetry, config.EncryptedReasoningSourceProvider, true)
	*admitted.Model.EncryptedReasoning.Declaration.Passthrough = true
	admitted.Model.EncryptedReasoning.MatchMessages[0] = "changed caller"
	*admitted.Provider.EncryptedReasoning.Declaration.OnCallerMismatch = config.EncryptedReasoningFail
	p, m, _ = original.Model("child", "m")
	assertResolverReasoning(t, original.EncryptedReasoning(p, m, nil), false, config.EncryptedReasoningFail, config.EncryptedReasoningSourceModel, true)
	p, m, _ = original.Model("child", "provider-layer")
	assertResolverReasoning(t, original.EncryptedReasoning(p, m, nil), true, config.EncryptedReasoningStripAndRetry, config.EncryptedReasoningSourceProvider, true)
	a, _ := original.Alias("a")
	assertResolverReasoning(t, original.EncryptedReasoning(p, m, &a), true, config.EncryptedReasoningFail, config.EncryptedReasoningSourceAlias, false)
	a.EncryptedReasoning.Declaration.OnCallerMismatch = new(string)
	*a.EncryptedReasoning.Declaration.OnCallerMismatch = config.EncryptedReasoningStripAndRetry
	a.EncryptedReasoning.MatchMessages[0] = "changed caller"
	a, _ = original.Alias("a")
	assertResolverReasoning(t, original.EncryptedReasoning(p, m, &a), true, config.EncryptedReasoningFail, config.EncryptedReasoningSourceAlias, false)
	providers := rt.Catalog.Providers()
	for i := range providers {
		providers[i].EncryptedReasoning = config.NewEncryptedReasoning(&config.EncryptedReasoningDeclaration{})
		providers[i].EncryptedReasoningInherited = false
	}
	rt.Catalog = config.NewCatalog(providers, nil, rt.Catalog.Aliases())
	reloaded := NewWithPrevious(rt, original)
	p, m, _ = reloaded.Model("child", "provider-layer")
	assertResolverReasoning(t, reloaded.EncryptedReasoning(p, m, nil), true, config.EncryptedReasoningFail, config.EncryptedReasoningSourceProvider, false)
	p, m, _ = original.Model("child", "provider-layer")
	assertResolverReasoning(t, original.EncryptedReasoning(p, m, nil), true, config.EncryptedReasoningStripAndRetry, config.EncryptedReasoningSourceProvider, true)
}
