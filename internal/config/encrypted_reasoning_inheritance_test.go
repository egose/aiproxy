package config

import (
	"fmt"
	"reflect"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2/gohcl"
)

func reasoningDerivedObject(root, basePolicy, baseModelPolicy, childPolicy, childModelPolicy string) map[string]any {
	object := reasoningConfigObject(root, "absent", "absent", "absent")
	model := map[string]any{
		"display_name": "Model M", "upstream_name": "upstream/m",
		"capabilities": []string{"chat", "responses"}, "pricing": map[string]any{"input_per_million": 2},
	}
	reasoningSetBlock(model, baseModelPolicy)
	base := map[string]any{
		"api_key": "base-key", "base_url": "https://upstream.example/v1",
		"model": map[string]any{
			"m": model, "sibling": map[string]any{},
			"embedding": map[string]any{"capabilities": []string{"embeddings"}},
		},
	}
	reasoningSetBlock(base, basePolicy)
	child := map[string]any{"extends": "base", "api_key": "child-key"}
	reasoningSetBlock(child, childPolicy)
	if childModelPolicy != "absent" {
		child["model"] = map[string]any{"m": map[string]any{"encrypted_reasoning": reasoningTestBlock(childModelPolicy)}}
	}
	object["provider"] = map[string]any{"openai-compatible": map[string]any{
		"base": base, "child": child, "other": map[string]any{"extends": "base", "api_key": "other-key"},
	}}
	object["alias"] = map[string]any{"a": map[string]any{
		"algorithm": "round_robin", "providers": []string{"child", "base"}, "model": "m",
	}}
	return object
}

func reasoningDerivedProviderObject(object map[string]any, name string) map[string]any {
	return object["provider"].(map[string]any)["openai-compatible"].(map[string]any)[name].(map[string]any)
}

func TestEncryptedReasoningStaticInheritedLayersAndResets(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range []struct {
			root, base, model, child, overlay string
			winner                            string
			source                            EncryptedReasoningSource
			inherited                         bool
		}{
			{"retry", "retry", "strip", "reset", "absent", "strip", EncryptedReasoningSourceModel, true},
			{"retry", "retry", "strip", "strip", "reset", "reset", EncryptedReasoningSourceModel, false},
			{"retry", "retry", "strip", "reset", "absent", "strip", EncryptedReasoningSourceModel, true},
			{"retry", "retry", "absent", "strip", "absent", "strip", EncryptedReasoningSourceProvider, false},
			{"strip", "retry", "absent", "absent", "absent", "retry", EncryptedReasoningSourceProvider, true},
			{"strip", "strip", "reset", "retry", "absent", "reset", EncryptedReasoningSourceModel, true},
			{"retry", "absent", "absent", "absent", "absent", "retry", EncryptedReasoningSourceRoot, false},
			{"absent", "absent", "absent", "absent", "absent", "reset", EncryptedReasoningSourceBuiltin, false},
			{"strip", "custom", "absent", "retry", "absent", "retry", EncryptedReasoningSourceProvider, false},
			{"retry", "strip", "custom", "reset", "retry", "retry", EncryptedReasoningSourceModel, false},
			{"strip", "reset", "absent", "absent", "absent", "reset", EncryptedReasoningSourceProvider, true},
		} {
			t.Run(fmt.Sprintf("%s/%s/%s/%s/%s/%s", format, tc.root, tc.base, tc.model, tc.child, tc.overlay), func(t *testing.T) {
				rt, err := Load(reasoningConfigSource(t, format, reasoningDerivedObject(tc.root, tc.base, tc.model, tc.child, tc.overlay)), "test."+format)
				if err != nil {
					t.Fatal(err)
				}
				base := testProvider(t, rt, "base")
				child := testProvider(t, rt, "child")
				model := child.ModelByName["m"]
				if base.Extends != "" || child.Extends != "base" || child.APIKey != "child-key" || child.BaseURL != base.BaseURL || len(child.Models) != len(base.Models) {
					t.Fatal("reasoning overrides changed inheritance provenance, credentials, or inventory")
				}
				if model.DisplayName != "Model M" || model.UpstreamName != "upstream/m" || !reflect.DeepEqual(model.Capabilities, []Capability{CapabilityChat, CapabilityResponses}) || model.Pricing == nil || model.Pricing.InputPerMillion != 2 {
					t.Fatalf("reasoning overlay changed model definition: %+v", model)
				}
				if child.EncryptedReasoningInherited != (tc.child == "absent" && tc.base != "absent") || model.EncryptedReasoningInherited != (tc.overlay == "absent" && tc.model != "absent") {
					t.Fatal("layer-local versus inherited declaration provenance was lost")
				}
				if child.EncryptedReasoning == nil && tc.base != "absent" || model.EncryptedReasoning == nil && tc.model != "absent" {
					t.Fatal("declared base policy was not inherited")
				}
				assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, child, model, nil), tc.winner, tc.source, tc.inherited)
				for _, m := range child.Models {
					indexed := child.ModelByName[m.Name]
					if !reflect.DeepEqual(m.EncryptedReasoning, indexed.EncryptedReasoning) || m.EncryptedReasoningInherited != indexed.EncryptedReasoningInherited {
						t.Fatal("inherited slice/index policies or provenance disagree")
					}
				}
			})
		}
	}
}

func TestEncryptedReasoningStaticOverlayRemovalRestoresBaseModel(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		object := reasoningDerivedObject("strip", "retry", "retry", "strip", "reset")
		first, err := Load(reasoningConfigSource(t, format, object), "first."+format)
		if err != nil {
			t.Fatal(err)
		}
		child := testProvider(t, first, "child")
		assertReasoningResolution(t, ResolveEncryptedReasoning(first.EncryptedReasoning, child, child.ModelByName["m"], nil), "reset", EncryptedReasoningSourceModel, false)
		delete(reasoningDerivedProviderObject(object, "child"), "model")
		second, err := Load(reasoningConfigSource(t, format, object), "second."+format)
		if err != nil {
			t.Fatal(err)
		}
		child = testProvider(t, second, "child")
		assertReasoningResolution(t, ResolveEncryptedReasoning(second.EncryptedReasoning, child, child.ModelByName["m"], nil), "retry", EncryptedReasoningSourceModel, true)
	}
}

func TestEncryptedReasoningStaticDerivedDeclarationCopies(t *testing.T) {
	object := reasoningDerivedObject("retry", "custom", "custom", "absent", "absent")
	source := reasoningConfigSource(t, FormatHCL, object)
	file, err := parseConfig(source, "test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	var raw rawFile
	if diags := gohcl.DecodeBody(file.Body, nil, &raw); diags.HasErrors() {
		t.Fatal(diags)
	}
	annotateProviderSyntax(file.Body, &raw)
	byName := make(map[string]rawProvider)
	bySyntax := make(map[string]rawProviderSyntax)
	for i, provider := range raw.Providers {
		byName[provider.Name], bySyntax[provider.Name] = provider, raw.providerSyntax[i]
	}
	base, err := buildProvider(byName["base"], bySyntax["base"], "test.hcl", DefaultUpstreamHeaderTimeout, "", false, nil, DefaultDeferred())
	if err != nil {
		t.Fatal(err)
	}
	built := map[string]Provider{"base": base}
	child, err := buildDerivedProvider(byName["child"], byName, bySyntax, built, DefaultDeferred())
	if err != nil {
		t.Fatal(err)
	}
	other, err := buildDerivedProvider(byName["other"], byName, bySyntax, built, DefaultDeferred())
	if err != nil {
		t.Fatal(err)
	}
	modelIndex := -1
	for i := range child.Models {
		if child.Models[i].Name == "m" {
			modelIndex = i
		}
	}
	if modelIndex == -1 {
		t.Fatal("inherited model disappeared")
	}
	for _, policy := range []*EncryptedReasoning{child.EncryptedReasoning, child.Models[modelIndex].EncryptedReasoning} {
		policy.MatchMessages[0] = "changed caller"
		(*policy.Declaration.MatchMessages)[0] = "changed caller"
		*policy.Declaration.Passthrough = true
	}
	if child.ModelByName["m"].EncryptedReasoning.MatchMessages[0] != "custom caller marker" || (*child.ModelByName["m"].EncryptedReasoning.Declaration.MatchMessages)[0] != "  Custom Caller Marker  " {
		t.Fatal("derived model slice/index declarations share pointers")
	}
	for _, provider := range []Provider{base, other} {
		for _, policy := range []*EncryptedReasoning{provider.EncryptedReasoning, provider.ModelByName["m"].EncryptedReasoning} {
			if policy.MatchMessages[0] != "custom caller marker" || *policy.Declaration.Passthrough || (*policy.Declaration.MatchMessages)[0] != "  Custom Caller Marker  " {
				t.Fatal("mutating one derivative changed a base or sibling declaration")
			}
		}
	}
	rawBase := byName["base"]
	(*rawBase.EncryptedReasoning.MatchMessages)[0] = "changed raw caller"
	if (*base.EncryptedReasoning.Declaration.MatchMessages)[0] != "  Custom Caller Marker  " {
		t.Fatal("provider build retained raw declaration storage")
	}
}

func TestEncryptedReasoningStaticDerivedModelOverlayRejections(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range []struct {
			name   string
			models map[string]any
			want   string
		}{
			{"bare empty", map[string]any{"m": map[string]any{}}, "encrypted_reasoning or deferred_transport"},
			{"new model", map[string]any{"new": map[string]any{"encrypted_reasoning": map[string]any{}}}, "new model"},
			{"duplicate names", map[string]any{"m": []any{
				map[string]any{"encrypted_reasoning": map[string]any{}},
				map[string]any{"encrypted_reasoning": map[string]any{"passthrough": false}},
			}}, "duplicate model"},
			{"duplicate policy blocks", map[string]any{"m": map[string]any{"encrypted_reasoning": []any{map[string]any{}, map[string]any{}}}}, "encrypted_reasoning"},
			{"duplicate transport blocks", map[string]any{"m": map[string]any{
				"encrypted_reasoning": map[string]any{}, "deferred_transport": []any{map[string]any{}, map[string]any{}},
			}}, "deferred_transport"},
			{"model receiver", map[string]any{"m": map[string]any{"encrypted_reasoning": map[string]any{}, "deferred_receiver": map[string]any{}}}, "deferred_receiver"},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				object := reasoningDerivedObject("retry", "retry", "strip", "reset", "absent")
				reasoningDerivedProviderObject(object, "child")["model"] = tc.models
				reasoningRequireRejected(t, reasoningConfigSource(t, format, object), format, `provider "child"`, tc.want)
			})
		}
		for _, field := range []struct {
			name   string
			values []any
		}{
			{"display_name", []any{"", "new name", nil}},
			{"upstream_name", []any{"", "new-upstream", nil}},
			{"protocol", []any{"", "chat", nil}},
			{"capabilities", []any{[]any{}, []any{"chat"}, nil}},
			{"pricing", []any{map[string]any{}, map[string]any{"input_per_million": 1}, nil}},
			{"unknown", []any{"", nil}},
		} {
			for _, value := range field.values {
				t.Run(fmt.Sprintf("%s/%s/%v", format, field.name, value), func(t *testing.T) {
					object := reasoningDerivedObject("retry", "retry", "strip", "reset", "absent")
					reasoningDerivedProviderObject(object, "child")["model"] = map[string]any{
						"m": map[string]any{"encrypted_reasoning": map[string]any{}, field.name: value},
					}
					reasoningRequireRejected(t, reasoningConfigSource(t, format, object), format, `provider "child"`, field.name)
				})
			}
		}
	}
}

func TestEncryptedReasoningStaticInheritanceRestrictions(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, scope := range []string{"provider", "model", "both"} {
			for _, tc := range []struct {
				name   string
				mutate func(map[string]any)
				want   string
			}{
				{"missing base", func(o map[string]any) { reasoningDerivedProviderObject(o, "child")["extends"] = "missing" }, "extends"},
				{"self", func(o map[string]any) { reasoningDerivedProviderObject(o, "child")["extends"] = "child" }, "extends"},
				{"chain", func(o map[string]any) { reasoningDerivedProviderObject(o, "child")["extends"] = "other" }, "inheritance chains"},
				{"type mismatch", func(o map[string]any) {
					providers := o["provider"].(map[string]any)
					original := providers["openai-compatible"].(map[string]any)
					providers["openai"] = map[string]any{"child": original["child"]}
					delete(original, "child")
				}, "must match base provider"},
				{"disabled base", func(o map[string]any) { reasoningDerivedProviderObject(o, "base")["enabled"] = false }, "disabled"},
				{"base URL override", func(o map[string]any) { reasoningDerivedProviderObject(o, "child")["base_url"] = "" }, "base_url"},
				{"enabled override", func(o map[string]any) { reasoningDerivedProviderObject(o, "child")["enabled"] = false }, "enabled"},
				{"missing credential", func(o map[string]any) { delete(reasoningDerivedProviderObject(o, "child"), "api_key") }, "local credential"},
				{"conflicting credentials", func(o map[string]any) {
					reasoningDerivedProviderObject(o, "child")["api_key_ref"] = map[string]any{"key": "child"}
				}, "local credential"},
			} {
				t.Run(format+"/"+scope+"/"+tc.name, func(t *testing.T) {
					object := reasoningDerivedObject("strip", "retry", "strip", "reset", "reset")
					child := reasoningDerivedProviderObject(object, "child")
					if scope == "provider" {
						delete(child, "model")
					}
					if scope == "model" {
						delete(child, "encrypted_reasoning")
					}
					tc.mutate(object)
					reasoningRequireRejected(t, reasoningConfigSource(t, format, object), format, `provider "child"`, tc.want)
				})
			}
		}
	}
}

func TestEncryptedReasoningStaticOverlaysPreservePeerOptInBoundaries(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range []struct {
			name                 string
			baseCaller           bool
			baseReceiver         bool
			localTransport       bool
			localReceiver        bool
			forwardAuthorization bool
			omitModelReasoning   bool
		}{
			{name: "reasoning only over caller", baseCaller: true},
			{name: "reasoning and transport", baseCaller: true, localTransport: true},
			{name: "transport only retains model reasoning", baseCaller: true, localTransport: true, omitModelReasoning: true},
			{name: "reasoning only over receiver", baseReceiver: true},
			{name: "local receiver with reasoning", baseReceiver: true, localReceiver: true},
			{name: "forwarded credential with reasoning only", baseCaller: true, forwardAuthorization: true},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				object := reasoningDerivedObject("strip", "custom", "custom", "absent", "reset")
				base := reasoningDerivedProviderObject(object, "base")
				baseModels := base["model"].(map[string]any)
				baseModel := baseModels["m"].(map[string]any)
				child := reasoningDerivedProviderObject(object, "child")
				childModels := child["model"].(map[string]any)
				childModels["embedding"] = map[string]any{"encrypted_reasoning": map[string]any{"passthrough": false}}
				if tc.baseCaller {
					base["deferred_transport"] = map[string]any{"max_wait": "1h"}
					baseModel["deferred_transport"] = map[string]any{"max_wait": "1h"}
				}
				if tc.localTransport {
					childModels["m"].(map[string]any)["deferred_transport"] = map[string]any{}
				}
				if tc.omitModelReasoning {
					delete(childModels["m"].(map[string]any), "encrypted_reasoning")
				}
				if tc.baseReceiver || tc.localReceiver {
					object["deferred"] = map[string]any{}
					object["auth"] = map[string]any{"main": map[string]any{
						"mode": "bearer_static", "client": map[string]any{"peer": map[string]any{"token": "peer-token"}},
					}}
				}
				if tc.baseReceiver {
					base["deferred_receiver"] = map[string]any{}
				}
				if tc.localReceiver {
					child["deferred_receiver"] = map[string]any{}
				}
				if tc.forwardAuthorization {
					delete(child, "api_key")
					child["forward_authorization"] = true
				}
				source := reasoningConfigSource(t, format, object)
				rt, err := Load(source, "test."+format)
				if err != nil {
					t.Fatal(err)
				}
				got := testProvider(t, rt, "child")
				if got.Extends != "base" || got.DeferredTransport != nil || (got.DeferredReceiver != nil) != tc.localReceiver ||
					got.ForwardAuthorization != tc.forwardAuthorization {
					t.Fatal("reasoning overlay inherited peer opt-ins or changed local provider declarations")
				}
				if tc.forwardAuthorization && got.APIKey != "" || !tc.forwardAuthorization && got.APIKey != "child-key" {
					t.Fatal("reasoning overlay changed local credential isolation")
				}
				assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, got, Model{}, nil), "custom", EncryptedReasoningSourceProvider, true)
				winner := "reset"
				if tc.omitModelReasoning {
					winner = "custom"
				}
				assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, got, got.ModelByName["m"], nil), winner, EncryptedReasoningSourceModel, tc.omitModelReasoning)
				assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, got, got.ModelByName["embedding"], nil), "strip", EncryptedReasoningSourceModel, false)
				for _, m := range got.Models {
					want := (*DeferredTransport)(nil)
					if tc.localTransport && m.Name == "m" {
						options := DefaultDeferredTransport()
						want = &options
					}
					if !reflect.DeepEqual(m.DeferredTransport, want) || !reflect.DeepEqual(got.ModelByName[m.Name].DeferredTransport, want) {
						t.Fatal("reasoning overlays leaked base peer policies or changed local transport defaults")
					}
				}
				if tc.baseCaller {
					if testProvider(t, rt, "base").ModelByName["m"].DeferredTransport.MaxWait != time.Hour {
						t.Fatal("child policy altered base deferred transport")
					}
				}
				if err := ValidateDeferredConfig(source, "test."+format); err != nil {
					t.Fatalf("standalone validation rejected an approved overlay: %v", err)
				}
				converted, _, _, err := Convert(source, "test."+format, false)
				if err != nil {
					t.Fatal(err)
				}
				reloaded, err := Load(converted, "converted")
				if err != nil || !reflect.DeepEqual(testProvider(t, reloaded, "child"), got) {
					t.Fatalf("converted overlay changed declaration or peer behavior: %v", err)
				}
			})
		}
	}
}

func TestEncryptedReasoningCombinedOverlaysKeepDeferredRestrictions(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range []struct {
			name   string
			mutate func(map[string]any, map[string]any)
			want   string
		}{
			{"nonchat model", func(_ map[string]any, child map[string]any) {
				child["model"] = map[string]any{"embedding": map[string]any{
					"encrypted_reasoning": map[string]any{}, "deferred_transport": map[string]any{},
				}}
			}, "chat-capable"},
			{"forwarding conflict", func(_ map[string]any, child map[string]any) {
				delete(child, "api_key")
				child["forward_authorization"] = true
			}, "forward_authorization"},
			{"receiver conflict", func(object, child map[string]any) {
				object["deferred"] = map[string]any{}
				object["auth"] = map[string]any{"main": map[string]any{
					"mode": "bearer_static", "client": map[string]any{"peer": map[string]any{"token": "peer-token"}},
				}}
				child["deferred_receiver"] = map[string]any{}
			}, "cannot be combined"},
			{"provider type", func(object, _ map[string]any) {
				providers := object["provider"].(map[string]any)
				providers["openai"] = providers["openai-compatible"]
				delete(providers, "openai-compatible")
			}, "static openai-compatible"},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				object := reasoningDerivedObject("strip", "retry", "strip", "reset", "reset")
				child := reasoningDerivedProviderObject(object, "child")
				child["model"].(map[string]any)["m"].(map[string]any)["deferred_transport"] = map[string]any{}
				tc.mutate(object, child)
				reasoningRequireRejected(t, reasoningConfigSource(t, format, object), format, `provider "child"`, tc.want)
			})
		}
	}
}

func TestEncryptedReasoningStandaloneOverlaysDoNotReadCredentials(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		object := reasoningDerivedObject("retry", "custom", "custom", "reset", "reset")
		for _, name := range []string{"base", "child", "other"} {
			p := reasoningDerivedProviderObject(object, name)
			delete(p, "api_key")
			p["api_key_ref"] = map[string]any{"path": "unavailable-" + name + "-keys.json", "key": name}
		}
		source := reasoningConfigSource(t, format, object)
		if err := ValidateDeferredConfig(source, "test."+format); err != nil {
			t.Fatalf("standalone reasoning validation performed credential I/O: %v", err)
		}
	}
}
