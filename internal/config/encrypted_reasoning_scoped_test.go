package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func reasoningBool(value bool) *bool { return &value }

func reasoningString(value string) *string { return &value }

func reasoningMessages(values ...string) *[]string { return &values }

func reasoningTestBlock(name string) any {
	switch name {
	case "absent":
		return nil
	case "reset":
		return map[string]any{}
	case "strip":
		return map[string]any{"passthrough": false}
	case "retry":
		return map[string]any{"on_caller_mismatch": EncryptedReasoningStripAndRetry}
	case "custom":
		return map[string]any{
			"passthrough": false, "on_caller_mismatch": EncryptedReasoningStripAndRetry,
			"match_messages": []string{"  Custom Caller Marker  "},
		}
	default:
		panic(name)
	}
}

func reasoningSetBlock(owner map[string]any, name string) {
	if block := reasoningTestBlock(name); block != nil {
		owner["encrypted_reasoning"] = block
	}
}

func reasoningConfigObject(root, provider, model, alias string) map[string]any {
	m := map[string]any{}
	reasoningSetBlock(m, model)
	p := map[string]any{"api_key": "sk-test", "model": map[string]any{"m": m}}
	reasoningSetBlock(p, provider)
	a := map[string]any{"algorithm": "round_robin", "providers": []string{"p1"}, "model": "m"}
	reasoningSetBlock(a, alias)
	object := map[string]any{
		"listener": map[string]any{"http": map[string]any{"public": map[string]any{"address": ":8080"}}},
		"auth":     map[string]any{"main": map[string]any{"mode": "none"}},
		"provider": map[string]any{"openai": map[string]any{"p1": p}},
		"alias":    map[string]any{"a": a},
	}
	reasoningSetBlock(object, root)
	return object
}

func reasoningConfigSource(t *testing.T, format string, object map[string]any) []byte {
	t.Helper()
	source, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if format == FormatHCL {
		source, err = decodeJSONConfig(source, "test.json")
		if err != nil {
			t.Fatal(err)
		}
	}
	return source
}

func assertReasoningResolution(t *testing.T, got EncryptedReasoningResolution, winner string, source EncryptedReasoningSource, inherited bool) {
	t.Helper()
	want := DefaultEncryptedReasoning()
	switch winner {
	case "strip":
		want.Passthrough = false
	case "retry":
		want.OnCallerMismatch = EncryptedReasoningStripAndRetry
	case "custom":
		want.Passthrough = false
		want.OnCallerMismatch = EncryptedReasoningStripAndRetry
		want.MatchMessages = []string{"custom caller marker"}
	}
	if got.Source != source || got.Inherited != inherited || !reflect.DeepEqual(got.Policy, want) {
		t.Fatalf("resolved = %+v / %+v, want source=%s inherited=%t policy=%+v", got, got.Policy, source, inherited, want)
	}
}

func TestEncryptedReasoningScopedPrecedence(t *testing.T) {
	states := []string{"absent", "reset", "strip", "retry"}
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, root := range states {
			for _, provider := range states {
				for _, model := range states {
					for _, alias := range states {
						t.Run(strings.Join([]string{format, root, provider, model, alias}, "/"), func(t *testing.T) {
							source := reasoningConfigSource(t, format, reasoningConfigObject(root, provider, model, alias))
							rt, err := Load(source, "test."+format)
							if err != nil {
								t.Fatal(err)
							}
							p, m, ok := rt.Catalog.Model("p1", "m")
							if !ok {
								t.Fatal("model not found")
							}
							a := testAlias(t, rt, "a")
							for _, layer := range []struct {
								name   string
								policy *EncryptedReasoning
							}{{root, rt.EncryptedReasoning}, {provider, p.EncryptedReasoning}, {model, m.EncryptedReasoning}, {alias, a.EncryptedReasoning}} {
								if (layer.policy == nil) != (layer.name == "absent") {
									t.Fatalf("declaration %s was flattened or lost: %+v", layer.name, layer.policy)
								}
								if layer.policy != nil && layer.policy.Declaration == nil {
									t.Fatal("sparse declaration was lost")
								}
							}
							winner, scope := "reset", EncryptedReasoningSourceBuiltin
							if root != "absent" {
								winner, scope = root, EncryptedReasoningSourceRoot
							}
							if provider != "absent" {
								winner, scope = provider, EncryptedReasoningSourceProvider
							}
							assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, Model{}, nil), winner, scope, false)
							if model != "absent" {
								winner, scope = model, EncryptedReasoningSourceModel
							}
							assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, m, nil), winner, scope, false)
							if alias != "absent" {
								winner, scope = alias, EncryptedReasoningSourceAlias
							}
							assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, m, &a), winner, scope, false)
							if !reflect.DeepEqual(p.Models[0].EncryptedReasoning, p.ModelByName["m"].EncryptedReasoning) {
								t.Fatal("slice and index policy declarations disagree")
							}
						})
					}
				}
			}
		}
	}
}

func TestEncryptedReasoningWholeBlockReplacement(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range []struct {
			root, provider, model, alias string
			winner                       string
			source                       EncryptedReasoningSource
		}{
			{"custom", "retry", "absent", "absent", "retry", EncryptedReasoningSourceProvider},
			{"custom", "strip", "reset", "absent", "reset", EncryptedReasoningSourceModel},
			{"custom", "retry", "strip", "reset", "reset", EncryptedReasoningSourceAlias},
			{"custom", "absent", "absent", "absent", "custom", EncryptedReasoningSourceRoot},
			{"reset", "custom", "absent", "absent", "custom", EncryptedReasoningSourceProvider},
			{"reset", "reset", "custom", "absent", "custom", EncryptedReasoningSourceModel},
			{"reset", "reset", "reset", "custom", "custom", EncryptedReasoningSourceAlias},
		} {
			t.Run(fmt.Sprintf("%s/%s/%s/%s/%s", format, tc.root, tc.provider, tc.model, tc.alias), func(t *testing.T) {
				rt, err := Load(reasoningConfigSource(t, format, reasoningConfigObject(tc.root, tc.provider, tc.model, tc.alias)), "test."+format)
				if err != nil {
					t.Fatal(err)
				}
				p, m, _ := rt.Catalog.Model("p1", "m")
				a := testAlias(t, rt, "a")
				assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, m, &a), tc.winner, tc.source, false)
			})
		}
	}
}

func reasoningPolicyOwner(object map[string]any, scope string) map[string]any {
	p := object["provider"].(map[string]any)["openai"].(map[string]any)["p1"].(map[string]any)
	switch scope {
	case "root":
		return object
	case "provider":
		return p
	case "model":
		return p["model"].(map[string]any)["m"].(map[string]any)
	case "alias":
		return object["alias"].(map[string]any)["a"].(map[string]any)
	default:
		panic(scope)
	}
}

func reasoningPolicyAt(t *testing.T, rt *Runtime, scope string) *EncryptedReasoning {
	t.Helper()
	switch scope {
	case "root":
		return rt.EncryptedReasoning
	case "provider":
		return testProvider(t, rt, "p1").EncryptedReasoning
	case "model":
		return testProvider(t, rt, "p1").ModelByName["m"].EncryptedReasoning
	case "alias":
		return testAlias(t, rt, "a").EncryptedReasoning
	default:
		panic(scope)
	}
}

func TestEncryptedReasoningSparseDeclarationsAndFormatParity(t *testing.T) {
	for _, scope := range []string{"root", "provider", "model", "alias"} {
		for _, declaration := range []map[string]any{
			{},
			{"passthrough": false},
			{"passthrough": true},
			{"on_caller_mismatch": ""},
			{"on_caller_mismatch": EncryptedReasoningStripAndRetry},
			{"match_messages": []string{}},
			{"match_messages": []string{"  Custom Caller Marker  "}},
			{"passthrough": false, "on_caller_mismatch": EncryptedReasoningStripAndRetry, "match_messages": []string{}},
		} {
			declaredJSON, err := json.Marshal(declaration)
			if err != nil {
				t.Fatal(err)
			}
			t.Run(scope+"/"+string(declaredJSON), func(t *testing.T) {
				var previous *EncryptedReasoning
				for _, format := range []string{FormatHCL, FormatJSON} {
					object := reasoningConfigObject("absent", "absent", "absent", "absent")
					reasoningPolicyOwner(object, scope)["encrypted_reasoning"] = declaration
					rt, err := Load(reasoningConfigSource(t, format, object), "test."+format)
					if err != nil {
						t.Fatal(err)
					}
					policy := reasoningPolicyAt(t, rt, scope)
					gotJSON, err := json.Marshal(DeclaredEncryptedReasoning(policy))
					var declaredObject any
					if err == nil {
						err = json.Unmarshal(gotJSON, &declaredObject)
					}
					if err == nil {
						gotJSON, err = json.Marshal(declaredObject)
					}
					if err != nil || string(gotJSON) != string(declaredJSON) {
						t.Fatalf("%s declared JSON = %s, want %s (err=%v)", format, gotJSON, declaredJSON, err)
					}
					wantPassthrough := declaration["passthrough"] != false
					wantMismatch := EncryptedReasoningFail
					if declaration["on_caller_mismatch"] == EncryptedReasoningStripAndRetry {
						wantMismatch = EncryptedReasoningStripAndRetry
					}
					wantMessages := DefaultEncryptedReasoning().MatchMessages
					if messages, ok := declaration["match_messages"].([]string); ok && len(messages) != 0 {
						wantMessages = []string{"custom caller marker"}
					}
					if !reflect.DeepEqual(policy.MatchMessages, wantMessages) || policy.Passthrough != wantPassthrough || policy.OnCallerMismatch != wantMismatch {
						t.Fatalf("%s normalized policy = %+v", format, policy)
					}
					if !reflect.DeepEqual(NormalizeEncryptedReasoning(policy), policy) {
						t.Fatal("normalization is not idempotent or sparse presence was lost")
					}
					if previous != nil && !reflect.DeepEqual(policy, previous) {
						t.Fatalf("HCL/JSON policies differ: %+v versus %+v", previous, policy)
					}
					previous = policy
				}
			})
		}
	}
}

func TestEncryptedReasoningProgrammaticCompatibilityAndCloning(t *testing.T) {
	for _, tc := range []struct {
		name        string
		policy      *EncryptedReasoning
		passthrough bool
		present     bool
	}{
		{"nil", nil, true, false},
		{"nil declaration", NewEncryptedReasoning(nil), true, false},
		{"legacy zero bool", &EncryptedReasoning{}, false, true},
		{"legacy false bool", &EncryptedReasoning{Passthrough: false}, false, true},
		{"legacy true bool", &EncryptedReasoning{Passthrough: true}, true, true},
		{"sparse reset", NewEncryptedReasoning(&EncryptedReasoningDeclaration{}), true, true},
		{"sparse false", NewEncryptedReasoning(&EncryptedReasoningDeclaration{Passthrough: reasoningBool(false)}), false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := NormalizeAlias(Alias{EncryptedReasoning: tc.policy})
			if (a.EncryptedReasoning != nil) != tc.present {
				t.Fatal("normalization changed block presence")
			}
			got := ResolveEncryptedReasoning(nil, Provider{}, Model{}, &a)
			if got.Policy.Passthrough != tc.passthrough || got.Policy.OnCallerMismatch != EncryptedReasoningFail {
				t.Fatalf("legacy boolean/default compatibility changed: %+v", got.Policy)
			}
			if tc.present && !reflect.DeepEqual(NewEncryptedReasoning(DeclaredEncryptedReasoning(a.EncryptedReasoning)).MatchMessages, got.Policy.MatchMessages) {
				t.Fatal("declaration accessor changed normalized patterns")
			}
			if err := ValidateEncryptedReasoning(tc.policy, "test"); err != nil {
				t.Fatal(err)
			}
		})
	}
	declaration := &EncryptedReasoningDeclaration{
		Passthrough: reasoningBool(false), OnCallerMismatch: reasoningString(""),
		MatchMessages: reasoningMessages("  Custom Caller Marker  "),
	}
	policy := NewEncryptedReasoning(declaration)
	if policy.Declaration == declaration || policy.Declaration.Passthrough == declaration.Passthrough || policy.Declaration.OnCallerMismatch == declaration.OnCallerMismatch || policy.Declaration.MatchMessages == declaration.MatchMessages {
		t.Fatal("constructor retained mutable declaration pointers")
	}
	*declaration.Passthrough = true
	*declaration.OnCallerMismatch = EncryptedReasoningStripAndRetry
	(*declaration.MatchMessages)[0] = "changed caller"
	if policy.Passthrough || *policy.Declaration.Passthrough || policy.OnCallerMismatch != EncryptedReasoningFail || *policy.Declaration.OnCallerMismatch != "" || policy.MatchMessages[0] != "custom caller marker" || (*policy.Declaration.MatchMessages)[0] != "  Custom Caller Marker  " {
		t.Fatal("declared/raw and normalized values were not independently copied")
	}
	for _, clone := range []*EncryptedReasoning{CloneEncryptedReasoning(policy), NormalizeEncryptedReasoning(policy)} {
		*clone.Declaration.Passthrough = true
		(*clone.Declaration.MatchMessages)[0] = "changed caller"
		clone.MatchMessages[0] = "changed caller"
	}
	declared := DeclaredEncryptedReasoning(policy)
	*declared.Passthrough = true
	(*declared.MatchMessages)[0] = "changed caller"
	if *policy.Declaration.Passthrough || (*policy.Declaration.MatchMessages)[0] != "  Custom Caller Marker  " || policy.MatchMessages[0] != "custom caller marker" {
		t.Fatal("clone/normalization/declaration accessor exposed mutable state")
	}
	first, second := DefaultEncryptedReasoning(), DefaultEncryptedReasoning()
	first.MatchMessages[0] = "changed caller"
	if second.MatchMessages[0] != DefaultEncryptedReasoningMatchMessages[0] {
		t.Fatal("default policy exposes shared patterns")
	}
}

func TestEncryptedReasoningDynamicDefaultsRetainRootFallback(t *testing.T) {
	rt := &Runtime{EncryptedReasoning: NewEncryptedReasoning(&EncryptedReasoningDeclaration{Passthrough: reasoningBool(false)})}
	p := validDynamicProvider()
	p = rt.ApplyProviderDefaults(p)
	if p.EncryptedReasoning != nil || p.Models[0].EncryptedReasoning != nil || p.ModelByName[p.Models[0].Name].EncryptedReasoning != nil {
		t.Fatal("root fallback was flattened into dynamic declarations")
	}
	assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.Models[0], nil), "strip", EncryptedReasoningSourceRoot, false)
	rt.EncryptedReasoning = NewEncryptedReasoning(&EncryptedReasoningDeclaration{OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry)})
	assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.Models[0], nil), "retry", EncryptedReasoningSourceRoot, false)
	p.EncryptedReasoning = NewEncryptedReasoning(&EncryptedReasoningDeclaration{})
	p.Models[0].EncryptedReasoning = NewEncryptedReasoning(&EncryptedReasoningDeclaration{OnCallerMismatch: reasoningString(EncryptedReasoningStripAndRetry)})
	got := rt.ApplyProviderDefaults(p)
	assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, got, Model{}, nil), "reset", EncryptedReasoningSourceProvider, false)
	assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, got, got.Models[0], nil), "retry", EncryptedReasoningSourceModel, false)
	*got.Models[0].EncryptedReasoning.Declaration.OnCallerMismatch = EncryptedReasoningFail
	if *p.Models[0].EncryptedReasoning.Declaration.OnCallerMismatch != EncryptedReasoningStripAndRetry || *got.ModelByName[got.Models[0].Name].EncryptedReasoning.Declaration.OnCallerMismatch != EncryptedReasoningStripAndRetry {
		t.Fatal("dynamic defaults share policy pointers with the input or rebuilt index")
	}
}

func TestEncryptedReasoningConversionPreservesSparseScopes(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, scopes := range [][4]string{
			{"custom", "reset", "strip", "retry"},
			{"reset", "retry", "custom", "strip"},
			{"retry", "strip", "reset", "reset"},
			{"absent", "absent", "absent", "absent"},
		} {
			for _, compact := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%v/compact=%t", format, scopes, compact), func(t *testing.T) {
					object := reasoningConfigObject(scopes[0], scopes[1], scopes[2], scopes[3])
					if scopes[1] == "retry" {
						reasoningPolicyOwner(object, "provider")["encrypted_reasoning"].(map[string]any)["match_messages"] = []string{}
					}
					source := reasoningConfigSource(t, format, object)
					want, err := Load(source, "test."+format)
					if err != nil {
						t.Fatal(err)
					}
					currentFormat := format
					for i := 0; i < 2; i++ {
						nextFormat := FormatHCL
						if currentFormat == FormatHCL {
							nextFormat = FormatJSON
						}
						converted, from, to, err := Convert(source, "test."+currentFormat, compact)
						if err != nil || from != currentFormat || to != nextFormat {
							t.Fatalf("conversion failed: from=%s to=%s err=%v", from, to, err)
						}
						got, err := Load(converted, "test."+nextFormat)
						if err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(got.EncryptedReasoning, want.EncryptedReasoning) ||
							!reflect.DeepEqual(got.Catalog.Providers(), want.Catalog.Providers()) ||
							!reflect.DeepEqual(got.Catalog.Aliases(), want.Catalog.Aliases()) {
							t.Fatal("conversion flattened, default-filled, or discarded a sparse declaration")
						}
						source, currentFormat = converted, nextFormat
					}
				})
			}
		}
	}
}
