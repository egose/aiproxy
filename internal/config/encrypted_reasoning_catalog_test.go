package config

import (
	"reflect"
	"testing"
)

func mutateReasoningPolicy(policy *EncryptedReasoning) {
	policy.Passthrough = true
	policy.OnCallerMismatch = EncryptedReasoningFail
	policy.MatchMessages[0] = "changed caller"
	*policy.Declaration.Passthrough = true
	*policy.Declaration.OnCallerMismatch = EncryptedReasoningFail
	(*policy.Declaration.MatchMessages)[0] = "changed caller"
}

func TestEncryptedReasoningCatalogCopiesAllPolicyViews(t *testing.T) {
	policy := NewEncryptedReasoning(&EncryptedReasoningDeclaration{
		Passthrough: reasoningBool(false), OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry),
		MatchMessages: reasoningMessages("  Custom Caller Marker  "),
	})
	want := CloneEncryptedReasoning(policy)
	model := Model{Name: "m", UpstreamName: "m", EncryptedReasoning: policy, EncryptedReasoningInherited: true}
	sibling := model
	sibling.Name, sibling.UpstreamName = "sibling", "sibling"
	indexed := model
	indexed.Name, indexed.UpstreamName = "indexed", "indexed"
	provider := Provider{
		Name: "child", Extends: "base", Type: ProviderTypeOpenAI, Enabled: true,
		EncryptedReasoning: policy, EncryptedReasoningInherited: true,
		Models: []Model{model, sibling}, ModelByName: map[string]Model{"m": model, "sibling": sibling, "indexed": indexed},
	}
	disabled := provider
	disabled.Name, disabled.Enabled = "disabled", false
	a := Alias{Name: "a", EncryptedReasoning: policy}
	catalog := NewCatalog([]Provider{provider}, []Provider{disabled}, []Alias{a})
	mutateReasoningPolicy(policy)

	checkPolicy := func(got *EncryptedReasoning) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("catalog policy shared mutable values: got=%+v declaration=%+v", got, DeclaredEncryptedReasoning(got))
		}
	}
	checkProvider := func(got Provider) {
		t.Helper()
		if got.Extends != "base" || !got.EncryptedReasoningInherited || len(got.Models) != 3 {
			t.Fatal("catalog lost relationship metadata or indexed-only model")
		}
		checkPolicy(got.EncryptedReasoning)
		for _, m := range got.Models {
			checkPolicy(m.EncryptedReasoning)
			checkPolicy(got.ModelByName[m.Name].EncryptedReasoning)
			if !m.EncryptedReasoningInherited || !got.ModelByName[m.Name].EncryptedReasoningInherited {
				t.Fatal("catalog lost model policy provenance")
			}
		}
	}
	for _, got := range append(catalog.Providers(), catalog.DisabledProviders()...) {
		checkProvider(got)
		mutateReasoningPolicy(got.Models[0].EncryptedReasoning)
		checkPolicy(got.ModelByName["m"].EncryptedReasoning)
		checkPolicy(got.Models[1].EncryptedReasoning)
		checkPolicy(got.EncryptedReasoning)
		mutateReasoningPolicy(got.ModelByName["sibling"].EncryptedReasoning)
		checkPolicy(got.Models[1].EncryptedReasoning)
		mutateReasoningPolicy(got.ModelByName["indexed"].EncryptedReasoning)
		checkPolicy(got.Models[2].EncryptedReasoning)
	}
	got, resolved, ok := catalog.Model("child", "m")
	if !ok {
		t.Fatal("catalog model disappeared")
	}
	checkProvider(got)
	mutateReasoningPolicy(resolved.EncryptedReasoning)
	checkPolicy(got.Models[0].EncryptedReasoning)
	checkPolicy(got.ModelByName["m"].EncryptedReasoning)
	effective := ResolveEncryptedReasoning(nil, got, got.Models[0], nil)
	effective.Policy.MatchMessages[0] = "changed caller"
	checkPolicy(got.Models[0].EncryptedReasoning)
	effective = ResolveEncryptedReasoning(nil, got, Model{}, nil)
	effective.Policy.MatchMessages[0] = "changed caller"
	checkPolicy(got.EncryptedReasoning)
	for _, gotAlias := range catalog.Aliases() {
		checkPolicy(gotAlias.EncryptedReasoning)
		mutateReasoningPolicy(gotAlias.EncryptedReasoning)
	}
	gotAlias, _ := catalog.Alias("a")
	checkPolicy(gotAlias.EncryptedReasoning)
	effective = ResolveEncryptedReasoning(nil, got, got.Models[0], &gotAlias)
	effective.Policy.MatchMessages[0] = "changed caller"
	checkPolicy(gotAlias.EncryptedReasoning)
	mutateReasoningPolicy(gotAlias.EncryptedReasoning)
	again, _ := catalog.Provider("child")
	checkProvider(again)
	checkProvider(catalog.DisabledProviders()[0])
	againAlias, _ := catalog.Alias("a")
	checkPolicy(againAlias.EncryptedReasoning)
}

func TestEncryptedReasoningCatalogCanonicalizesDeclarationPresence(t *testing.T) {
	retry := NewEncryptedReasoning(&EncryptedReasoningDeclaration{
		OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry),
	})
	reset := NewEncryptedReasoning(&EncryptedReasoningDeclaration{})
	strip := NewEncryptedReasoning(&EncryptedReasoningDeclaration{Passthrough: reasoningBool(false)})
	for _, tc := range []struct {
		name             string
		ordered, indexed *EncryptedReasoning
		orderedInherited bool
		indexedInherited bool
		want             *EncryptedReasoning
		inherited        bool
	}{
		{"absent", nil, nil, false, false, nil, false},
		{"indexed declaration", nil, retry, false, true, retry, true},
		{"indexed reset", nil, reset, false, true, reset, true},
		{"ordered reset replaces inherited retry", reset, retry, false, true, reset, false},
		{"ordered false replaces indexed reset", strip, reset, true, false, strip, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ordered := Model{
				Name: "m", UpstreamName: "updated", Capabilities: []Capability{CapabilityChat},
				EncryptedReasoning: tc.ordered, EncryptedReasoningInherited: tc.orderedInherited,
			}
			indexed := Model{
				Name: "m", UpstreamName: "stale",
				EncryptedReasoning: tc.indexed, EncryptedReasoningInherited: tc.indexedInherited,
			}
			input := Provider{Name: "p", Models: []Model{ordered}, ModelByName: map[string]Model{"m": indexed}}
			catalog := NewCatalog([]Provider{input}, nil, nil)
			got, model, ok := catalog.Model("p", "m")
			if !ok {
				t.Fatal("catalog model disappeared")
			}
			for _, m := range []Model{got.Models[0], got.ModelByName["m"], model} {
				if m.UpstreamName != "updated" || !reflect.DeepEqual(m.Capabilities, ordered.Capabilities) ||
					!reflect.DeepEqual(m.EncryptedReasoning, tc.want) || m.EncryptedReasoningInherited != tc.inherited {
					t.Fatal("catalog lost a declared reset/false, merged blocks, or retained stale model definitions")
				}
			}
			if tc.want != nil {
				model.EncryptedReasoning.MatchMessages[0] = "changed caller"
				if !reflect.DeepEqual(got.Models[0].EncryptedReasoning, tc.want) || !reflect.DeepEqual(got.ModelByName["m"].EncryptedReasoning, tc.want) {
					t.Fatal("catalog canonicalization shares policy storage across views")
				}
				*model.EncryptedReasoning.Declaration = EncryptedReasoningDeclaration{}
				if !reflect.DeepEqual(DeclaredEncryptedReasoning(got.Models[0].EncryptedReasoning), DeclaredEncryptedReasoning(tc.want)) {
					t.Fatal("catalog canonicalization shares declaration storage")
				}
			}
			if !reflect.DeepEqual(input.Models[0], ordered) || !reflect.DeepEqual(input.ModelByName["m"], indexed) {
				t.Fatal("catalog canonicalization changed its input")
			}
		})
	}
}
