package dbmerge

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func storedReasoningJSON(mode string) json.RawMessage {
	switch mode {
	case "absent":
		return nil
	case "null":
		return json.RawMessage(`null`)
	case "reset":
		return json.RawMessage(`{}`)
	case "strip":
		return json.RawMessage(`{"passthrough":false}`)
	case "retry":
		return json.RawMessage(`{"on_caller_mismatch":"strip_and_retry"}`)
	case "custom":
		return json.RawMessage(`{"passthrough":true,"on_caller_mismatch":"strip_and_retry","match_messages":[" Vendor Marker "]}`)
	default:
		panic(mode)
	}
}

func mergeReasoningPolicy(t *testing.T, mode string) *config.EncryptedReasoning {
	t.Helper()
	policy, err := config.DecodeEncryptedReasoningJSON(storedReasoningJSON(mode), "fixture")
	if err != nil {
		t.Fatal(err)
	}
	return policy
}

func reasoningBaseFixture(t *testing.T, providerMode, modelMode string) config.Provider {
	t.Helper()
	model := config.Model{
		Name: "m", DisplayName: "Model M", UpstreamName: "upstream/m",
		Capabilities:       []config.Capability{config.CapabilityChat, config.CapabilityResponses},
		Pricing:            &config.ModelPricing{InputPerMillion: 2},
		EncryptedReasoning: mergeReasoningPolicy(t, modelMode),
	}
	unconfigured := config.Model{Name: "unconfigured", UpstreamName: "upstream/unconfigured"}
	indexed := config.Model{Name: "indexed", UpstreamName: "upstream/indexed"}
	return config.Provider{
		Name: "base", Type: config.ProviderTypeOpenAI, Enabled: true,
		BaseURL: "https://upstream.example/v1", APIKey: "base-only-key",
		DisplayName: "Base", EncryptedReasoning: mergeReasoningPolicy(t, providerMode),
		Models: []config.Model{model, unconfigured},
		ModelByName: map[string]config.Model{
			"m": model, "unconfigured": unconfigured, "indexed": indexed,
		},
	}
}

func assertMergedReasoning(t *testing.T, got config.EncryptedReasoningResolution, mode string, source config.EncryptedReasoningSource, inherited bool) {
	t.Helper()
	want := config.DefaultEncryptedReasoning()
	if policy := mergeReasoningPolicy(t, mode); policy != nil {
		want = *config.NormalizeEncryptedReasoning(policy)
		want.Declaration = nil
	}
	if got.Source != source || got.Inherited != inherited || !reflect.DeepEqual(got.Policy, want) {
		t.Fatalf("resolution = %+v, want mode=%s source=%s inherited=%t", got, mode, source, inherited)
	}
}

func assertMaterializedModelCopies(t *testing.T, p config.Provider) {
	t.Helper()
	if len(p.Models) != len(p.ModelByName) {
		t.Fatal("served slice/index inventory differs")
	}
	for _, model := range p.Models {
		indexed, ok := p.ModelByName[model.Name]
		if !ok || !reflect.DeepEqual(model, indexed) {
			t.Fatalf("served slice/index definition or policy differs for %s", model.Name)
		}
		if model.EncryptedReasoning != nil && (model.EncryptedReasoning == indexed.EncryptedReasoning || model.EncryptedReasoning.Declaration == indexed.EncryptedReasoning.Declaration) {
			t.Fatalf("served slice/index declaration is shared for %s", model.Name)
		}
		if model.Pricing != nil && model.Pricing == indexed.Pricing {
			t.Fatalf("served slice/index pricing is shared for %s", model.Name)
		}
	}
}

func TestMaterializeEncryptedReasoningInheritedLayersAndOverlays(t *testing.T) {
	for i, tc := range []struct {
		root, base, model, child, overlay, winner string
		source                                    config.EncryptedReasoningSource
		inherited                                 bool
	}{
		{"retry", "retry", "strip", "reset", "absent", "strip", config.EncryptedReasoningSourceModel, true},
		{"retry", "retry", "strip", "strip", "reset", "reset", config.EncryptedReasoningSourceModel, false},
		{"retry", "retry", "strip", "reset", "null", "strip", config.EncryptedReasoningSourceModel, true},
		{"retry", "retry", "absent", "strip", "absent", "strip", config.EncryptedReasoningSourceProvider, false},
		{"strip", "retry", "absent", "absent", "absent", "retry", config.EncryptedReasoningSourceProvider, true},
		{"strip", "strip", "reset", "retry", "absent", "reset", config.EncryptedReasoningSourceModel, true},
		{"retry", "absent", "absent", "absent", "absent", "retry", config.EncryptedReasoningSourceRoot, false},
		{"absent", "absent", "absent", "absent", "absent", "reset", config.EncryptedReasoningSourceBuiltin, false},
		{"strip", "custom", "absent", "retry", "absent", "retry", config.EncryptedReasoningSourceProvider, false},
		{"retry", "strip", "custom", "reset", "retry", "retry", config.EncryptedReasoningSourceModel, false},
		{"strip", "reset", "absent", "absent", "absent", "reset", config.EncryptedReasoningSourceProvider, true},
	} {
		t.Run(fmt.Sprintf("%d/%s/%s/%s/%s", i, tc.base, tc.model, tc.child, tc.overlay), func(t *testing.T) {
			base := reasoningBaseFixture(t, tc.base, tc.model)
			row := store.DBProvider{Name: "child", Type: string(base.Type), Extends: base.Name, Enabled: i%2 == 0, APIKeyRefKey: "local-key", EncryptedReasoning: storedReasoningJSON(tc.child)}
			models := []store.DBProviderModel{
				{Name: "m", EncryptedReasoning: storedReasoningJSON(tc.overlay)},
				{Name: "legacy-ghost", UpstreamName: "ignored", Capabilities: []string{"invalid"}, Pricing: json.RawMessage(`12`)},
			}
			p, err := BuildProvider(row, models, map[string]config.Provider{base.Name: base})
			if err != nil {
				t.Fatal(err)
			}
			if err := config.ValidateDynamicProvider(p); err != nil {
				t.Fatal(err)
			}
			if p.Extends != base.Name || p.Name != row.Name || p.Enabled != row.Enabled || p.BaseURL != base.BaseURL || p.APIKey != "" || p.APIKeyRef == nil || p.APIKeyRef.Key != row.APIKeyRefKey {
				t.Fatal("inheritance provenance, enabled state, or credential isolation changed")
			}
			if len(p.Models) != 3 || p.ModelByName["legacy-ghost"].Name != "" {
				t.Fatal("materialization flattened local rows instead of the complete inherited inventory")
			}
			model := p.ModelByName["m"]
			if model.DisplayName != "Model M" || model.UpstreamName != "upstream/m" || !reflect.DeepEqual(model.Capabilities, base.Models[0].Capabilities) || model.Pricing == nil || model.Pricing.InputPerMillion != 2 {
				t.Fatal("policy-only overlay changed inherited model definitions")
			}
			if p.EncryptedReasoningInherited != (tc.child == "absent" && tc.base != "absent") || model.EncryptedReasoningInherited != ((tc.overlay == "absent" || tc.overlay == "null") && tc.model != "absent") {
				t.Fatal("owner-local versus inherited declaration provenance changed")
			}
			assertMaterializedModelCopies(t, p)
			rt := &config.Runtime{EncryptedReasoning: mergeReasoningPolicy(t, tc.root)}
			p = rt.ApplyProviderDefaults(p)
			assertMergedReasoning(t, config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil), tc.winner, tc.source, tc.inherited)
			if tc.base == "absent" && tc.child == "absent" && p.EncryptedReasoning != nil {
				t.Fatal("root defaults were written into a provider declaration")
			}
		})
	}
}

func TestMaterializeEncryptedReasoningConcreteLocalDeclarations(t *testing.T) {
	for _, tc := range []struct {
		provider, model, winner string
		source                  config.EncryptedReasoningSource
	}{
		{"absent", "absent", "retry", config.EncryptedReasoningSourceRoot},
		{"reset", "absent", "reset", config.EncryptedReasoningSourceProvider},
		{"strip", "reset", "reset", config.EncryptedReasoningSourceModel},
		{"retry", "strip", "strip", config.EncryptedReasoningSourceModel},
		{"null", "custom", "custom", config.EncryptedReasoningSourceModel},
	} {
		t.Run(tc.provider+"/"+tc.model, func(t *testing.T) {
			row := store.DBProvider{Name: "concrete", Type: "openai", Enabled: true, APIKeyRefKey: "local-key", EncryptedReasoning: storedReasoningJSON(tc.provider)}
			models := []store.DBProviderModel{{Name: "m", DisplayName: "Local M", Capabilities: []string{"chat"}, EncryptedReasoning: storedReasoningJSON(tc.model)}}
			beforeRow, beforeModels := row, append([]store.DBProviderModel(nil), models...)
			p, err := BuildProvider(row, models, nil)
			if err != nil {
				t.Fatal(err)
			}
			rt := &config.Runtime{EncryptedReasoning: mergeReasoningPolicy(t, "retry")}
			p = rt.ApplyProviderDefaults(p)
			if err := config.ValidateDynamicProvider(p); err != nil {
				t.Fatal(err)
			}
			if p.Extends != "" || p.EncryptedReasoningInherited || p.ModelByName["m"].EncryptedReasoningInherited || p.ModelByName["m"].UpstreamName != "m" {
				t.Fatal("concrete definition defaults or local provenance changed")
			}
			assertMergedReasoning(t, config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil), tc.winner, tc.source, false)
			assertMaterializedModelCopies(t, p)
			if !reflect.DeepEqual(row, beforeRow) || !reflect.DeepEqual(models, beforeModels) {
				t.Fatal("materialization changed stored local intent")
			}
		})
	}
}

func TestMaterializeEncryptedReasoningCopyIsolation(t *testing.T) {
	base := reasoningBaseFixture(t, "custom", "custom")
	beforeProvider := config.CloneEncryptedReasoning(base.EncryptedReasoning)
	beforeModel := config.CloneEncryptedReasoning(base.Models[0].EncryptedReasoning)
	build := func(name string) config.Provider {
		t.Helper()
		p, err := MaterializeProvider(store.DBProvider{Name: name, Type: string(base.Type), Extends: base.Name, Enabled: true}, nil, map[string]config.Provider{base.Name: base})
		if err != nil {
			t.Fatal(err)
		}
		return p
	}
	child, sibling := build("child"), build("sibling")
	child.EncryptedReasoning.MatchMessages[0] = "changed effective provider"
	(*child.EncryptedReasoning.Declaration.MatchMessages)[0] = "changed local provider"
	*child.EncryptedReasoning.Declaration.Passthrough = false
	child.Models[0].EncryptedReasoning.MatchMessages[0] = "changed effective model"
	(*child.Models[0].EncryptedReasoning.Declaration.MatchMessages)[0] = "changed local model"
	*child.Models[0].EncryptedReasoning.Declaration.Passthrough = false
	child.Models[0].Capabilities[0] = config.CapabilityEmbeddings
	child.Models[0].Pricing.InputPerMillion = 9
	for _, policy := range []*config.EncryptedReasoning{base.EncryptedReasoning, sibling.EncryptedReasoning} {
		if !reflect.DeepEqual(policy, beforeProvider) {
			t.Fatal("child provider policy mutated its base or sibling")
		}
	}
	for _, policy := range []*config.EncryptedReasoning{base.Models[0].EncryptedReasoning, sibling.Models[0].EncryptedReasoning, child.ModelByName["m"].EncryptedReasoning} {
		if !reflect.DeepEqual(policy, beforeModel) {
			t.Fatal("child model policy mutated its base, sibling, or independent index")
		}
	}
	if child.ModelByName["m"].Capabilities[0] != config.CapabilityChat || child.ModelByName["m"].Pricing.InputPerMillion != 2 || base.Models[0].Pricing.InputPerMillion != 2 {
		t.Fatal("child model definitions share mutable slice/index/base values")
	}
	indexed := child.ModelByName["m"]
	(*indexed.EncryptedReasoning.Declaration.MatchMessages)[0] = "changed indexed model"
	if (*child.Models[0].EncryptedReasoning.Declaration.MatchMessages)[0] != "changed local model" {
		t.Fatal("indexed declarations share the model slice")
	}
}

func TestMaterializeProviderDoesNotDecryptOrDiscloseCredentials(t *testing.T) {
	base := reasoningBaseFixture(t, "retry", "strip")
	base.CopilotToken = "private-base-token"
	base.APIKeyRef = &config.APIKeyRef{Key: "private-base-ref"}
	base.CopilotCredentialRef = &config.CopilotCredentialRef{Name: "private-base-credential"}
	for _, extends := range []string{"", base.Name} {
		row := store.DBProvider{Name: "metadata", Type: string(base.Type), Extends: extends, Enabled: true, APIKeyEncrypted: []byte("private-invalid-ciphertext"), EncryptedReasoning: storedReasoningJSON("reset")}
		models := []store.DBProviderModel{{Name: "m", EncryptedReasoning: storedReasoningJSON("reset")}}
		p, err := MaterializeProvider(row, models, map[string]config.Provider{base.Name: base})
		if err != nil || p.APIKey != "" || p.APIKeyRef != nil || p.CopilotToken != "" || p.CopilotCredentialRef != nil {
			t.Fatalf("metadata materialization decrypted or disclosed credentials: %v", err)
		}
		if p.EncryptedReasoning == nil || len(p.Models) == 0 || p.ModelByName["m"].EncryptedReasoning == nil {
			t.Fatal("metadata materialization omitted served policies/inventory")
		}
		if _, err := BuildProvider(row, models, map[string]config.Provider{base.Name: base}); err == nil || !strings.Contains(err.Error(), "decrypt") || strings.Contains(err.Error(), "private") {
			t.Fatalf("serving build bypassed local credential validation or leaked ciphertext: %v", err)
		}
	}
}

func TestMaterializeEncryptedReasoningRejectsCorruptPolicies(t *testing.T) {
	base := reasoningBaseFixture(t, "retry", "strip")
	for _, scope := range []string{"provider", "concrete-model", "derived-model"} {
		for _, raw := range []string{`[]`, `{"unknown":true}`, `{"passthrough":null}`, `{"passthrough":"false"}`, `{"Passthrough":false}`, `{"passthrough":false,"passthrough":true}`, `{"match_messages":["MARKER"," marker "]}`} {
			t.Run(scope+"/"+raw, func(t *testing.T) {
				for _, enabled := range []bool{false, true} {
					row := store.DBProvider{Name: "bad", Type: string(base.Type), Enabled: enabled, APIKeyEncrypted: []byte("invalid-ciphertext")}
					models := []store.DBProviderModel{{Name: "m"}}
					owner := `provider "bad"`
					if scope == "provider" {
						row.EncryptedReasoning = json.RawMessage(raw)
					} else {
						models[0].EncryptedReasoning = json.RawMessage(raw)
						owner += `: model "m"`
					}
					if scope == "derived-model" {
						row.Extends = base.Name
					}
					_, err := BuildProvider(row, models, map[string]config.Provider{base.Name: base})
					if err == nil || !strings.Contains(err.Error(), owner) || !strings.Contains(err.Error(), "encrypted_reasoning") || strings.Contains(err.Error(), "decrypt") {
						t.Fatalf("corrupt policy bypassed candidate validation: %v", err)
					}
				}
			})
		}
	}
}

func TestMaterializeEncryptedReasoningRejectsActiveOverlayRedefinitions(t *testing.T) {
	base := reasoningBaseFixture(t, "retry", "strip")
	for name, models := range map[string][]store.DBProviderModel{
		"new model":     {{Name: "ghost", EncryptedReasoning: storedReasoningJSON("reset")}},
		"duplicate":     {{Name: "m", EncryptedReasoning: storedReasoningJSON("reset")}, {Name: "m", EncryptedReasoning: storedReasoningJSON("strip")}},
		"display name":  {{Name: "m", DisplayName: "Changed", EncryptedReasoning: storedReasoningJSON("reset")}},
		"upstream":      {{Name: "m", UpstreamName: "changed", EncryptedReasoning: storedReasoningJSON("reset")}},
		"protocol":      {{Name: "m", Protocol: "chat", EncryptedReasoning: storedReasoningJSON("reset")}},
		"capabilities":  {{Name: "m", Capabilities: []string{"chat"}, EncryptedReasoning: storedReasoningJSON("reset")}},
		"pricing":       {{Name: "m", Pricing: json.RawMessage(`{"input_per_million":2}`), EncryptedReasoning: storedReasoningJSON("reset")}},
		"empty pricing": {{Name: "m", Pricing: json.RawMessage(`{}`), EncryptedReasoning: storedReasoningJSON("reset")}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := MaterializeProvider(store.DBProvider{Name: "child", Type: string(base.Type), Extends: base.Name}, models, map[string]config.Provider{base.Name: base})
			if err == nil || !strings.Contains(err.Error(), `provider "child": model "`) {
				t.Fatalf("invalid active overlay accepted: %v", err)
			}
		})
	}
	p, err := MaterializeProvider(store.DBProvider{Name: "child", Type: string(base.Type), Extends: base.Name}, []store.DBProviderModel{
		{Name: "ghost", DisplayName: "Legacy", UpstreamName: "ghost", Capabilities: []string{"unknown"}, Pricing: json.RawMessage(`12`)},
		{Name: "other-legacy", Protocol: "invalid", EncryptedReasoning: storedReasoningJSON("null")},
		{Name: "m", Capabilities: []string{}, Pricing: json.RawMessage(`null`), EncryptedReasoning: storedReasoningJSON("reset")},
	}, map[string]config.Provider{base.Name: base})
	if err != nil || len(p.Models) != 3 || p.ModelByName["ghost"].Name != "" || p.ModelByName["m"].EncryptedReasoningInherited {
		t.Fatalf("legacy NULL rows or empty stored overlay defaults changed: %v", err)
	}
	assertMaterializedModelCopies(t, p)
}

func TestMaterializeEncryptedReasoningBaseContract(t *testing.T) {
	base := reasoningBaseFixture(t, "retry", "strip")
	for _, enabled := range []bool{false, true} {
		for _, tc := range []struct {
			name string
			base config.Provider
			row  store.DBProvider
		}{
			{"disabled", func() config.Provider { p := base; p.Enabled = false; return p }(), store.DBProvider{Name: "child", Type: string(base.Type), Extends: base.Name, Enabled: enabled}},
			{"static-derived", func() config.Provider { p := base; p.Extends = "parent"; return p }(), store.DBProvider{Name: "child", Type: string(base.Type), Extends: base.Name, Enabled: enabled}},
			{"wrong type", base, store.DBProvider{Name: "child", Type: "anthropic", Extends: base.Name, Enabled: enabled}},
			{"self", base, store.DBProvider{Name: base.Name, Type: string(base.Type), Extends: base.Name, Enabled: enabled}},
			{"missing", base, store.DBProvider{Name: "child", Type: string(base.Type), Extends: "missing", Enabled: enabled}},
		} {
			t.Run(fmt.Sprintf("%s/%t", tc.name, enabled), func(t *testing.T) {
				if _, err := MaterializeProvider(tc.row, nil, map[string]config.Provider{base.Name: tc.base}); err == nil {
					t.Fatal("invalid base relationship accepted")
				}
			})
		}
	}
	middle, err := MaterializeProvider(store.DBProvider{Name: "middle", Type: string(base.Type), Extends: base.Name, Enabled: true}, nil, map[string]config.Provider{base.Name: base})
	if err != nil || middle.Extends != base.Name {
		t.Fatal("managed materialization lost extends provenance")
	}
	if _, err := MaterializeProvider(store.DBProvider{Name: "child", Type: string(base.Type), Extends: middle.Name, Enabled: true}, nil, map[string]config.Provider{middle.Name: middle}); err == nil || !strings.Contains(err.Error(), "inheritance chains") {
		t.Fatalf("materialized managed base allowed a chain: %v", err)
	}
}
