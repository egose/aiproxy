package config

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func reasoningRawSource(format, scope, declaration string) []byte {
	parts := map[string]string{"root": "", "provider": "", "model": "", "alias": ""}
	parts[scope] = declaration
	if format == FormatHCL {
		return []byte(deferredModelTestAuth + parts["root"] + `
provider "openai" "p1" {
  api_key = "sk-test"
  ` + parts["provider"] + `
  model "m" {
    ` + parts["model"] + `
  }
}
alias "a" {
  algorithm = "round_robin"
  providers = ["p1"]
  model = "m"
  ` + parts["alias"] + `
}
`)
	}
	for key, part := range parts {
		if part != "" {
			parts[key] = "," + part
		}
	}
	return []byte(`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}}` + parts["root"] +
		`,"provider":{"openai":{"p1":{"api_key":"sk-test"` + parts["provider"] + `,"model":{"m":{` +
		strings.TrimPrefix(parts["model"], ",") + `}}}}},"alias":{"a":{"algorithm":"round_robin","providers":["p1"],"model":"m"` + parts["alias"] + `}}}`)
}

func reasoningScopeOwner(scope string) string {
	switch scope {
	case "root":
		return "root"
	case "provider":
		return `provider "p1"`
	case "model":
		return `provider "p1": model "m"`
	case "alias":
		return `alias "a"`
	default:
		panic(scope)
	}
}

func reasoningRequireRejected(t *testing.T, source []byte, format, owner, field string) {
	t.Helper()
	for _, check := range []struct {
		name string
		run  func() error
	}{
		{"load", func() error { _, err := Load(source, "test."+format); return err }},
		{"standalone", func() error { return ValidateDeferredConfig(source, "test."+format) }},
		{"convert", func() error { _, _, _, err := Convert(source, "test."+format, false); return err }},
	} {
		t.Run(check.name, func(t *testing.T) {
			err := check.run()
			if err == nil || !strings.Contains(err.Error(), owner) || !strings.Contains(err.Error(), field) {
				t.Fatalf("error = %v, want owner %q and field %q", err, owner, field)
			}
		})
	}
}

func TestEncryptedReasoningInvalidStaticDeclarations(t *testing.T) {
	for _, scope := range []string{"root", "provider", "model", "alias"} {
		for _, declaration := range []string{
			`encrypted_reasoning "named" {}`,
			"encrypted_reasoning {}\nencrypted_reasoning {}",
			"encrypted_reasoning = {}",
			"encrypted_reasoning = null",
			"encrypted_reasoning { unknown = 1 }",
			"encrypted_reasoning {\n nested {}\n}",
			"encrypted_reasoning { passthrough = null }",
			`encrypted_reasoning { passthrough = "false" }`,
			"encrypted_reasoning { passthrough = 0 }",
			"encrypted_reasoning { on_caller_mismatch = null }",
			"encrypted_reasoning { on_caller_mismatch = false }",
			"encrypted_reasoning { match_messages = null }",
			`encrypted_reasoning { match_messages = "custom marker" }`,
			"encrypted_reasoning { match_messages = [false] }",
			"encrypted_reasoning { match_messages = [null] }",
			"encrypted_reasoning { match_messages = {} }",
			"encrypted_reasoning { passthrough = unknown_value }",
			"encrypted_reasoning { Passthrough = false }",
		} {
			t.Run(scope+"/hcl/"+declaration, func(t *testing.T) {
				reasoningRequireRejected(t, reasoningRawSource(FormatHCL, scope, declaration), FormatHCL, reasoningScopeOwner(scope), "encrypted_reasoning")
			})
		}
		for _, value := range []string{
			"null", "false", "12", `""`, "[]", "[null]", "[false]", "[{},{}]",
			`{"unknown":1}`, `{"nested":{}}`, `{"passthrough":null}`, `{"passthrough":"false"}`,
			`{"passthrough":0}`, `{"on_caller_mismatch":null}`, `{"on_caller_mismatch":false}`,
			`{"match_messages":null}`, `{"match_messages":"custom marker"}`, `{"match_messages":[false]}`,
			`{"match_messages":[null]}`, `{"match_messages":{}}`, `{"Passthrough":false}`,
		} {
			t.Run(scope+"/json/"+value, func(t *testing.T) {
				reasoningRequireRejected(t, reasoningRawSource(FormatJSON, scope, `"encrypted_reasoning":`+value), FormatJSON, reasoningScopeOwner(scope), "encrypted_reasoning")
			})
		}
	}
}

func TestEncryptedReasoningBoundsAndMemberDefaults(t *testing.T) {
	sixteen := make([]string, 16)
	for i := range sixteen {
		sixteen[i] = fmt.Sprintf("custom-marker-%02d", i)
	}
	seventeen := append(append([]string(nil), sixteen...), "custom-marker-16")
	for _, scope := range []string{"root", "provider", "model", "alias"} {
		for _, format := range []string{FormatHCL, FormatJSON} {
			for _, tc := range []struct {
				field string
				value any
				valid bool
			}{
				{"on_caller_mismatch", EncryptedReasoningFail, true},
				{"on_caller_mismatch", EncryptedReasoningStripAndRetry, true},
				{"on_caller_mismatch", "", true},
				{"on_caller_mismatch", "FAIL", false},
				{"on_caller_mismatch", " fail ", false},
				{"on_caller_mismatch", "sometimes", false},
				{"match_messages", []string{}, true},
				{"match_messages", []string{" abcd "}, true},
				{"match_messages", []string{"abc"}, false},
				{"match_messages", []string{strings.Repeat("x", 256)}, true},
				{"match_messages", []string{strings.Repeat("x", 257)}, false},
				{"match_messages", []string{"éé"}, true},
				{"match_messages", []string{strings.Repeat("é", 128)}, true},
				{"match_messages", []string{strings.Repeat("é", 129)}, false},
				{"match_messages", sixteen, true},
				{"match_messages", seventeen, false},
				{"match_messages", []string{"    "}, false},
				{"match_messages", []string{""}, false},
				{"match_messages", []string{"  Custom Marker", "CUSTOM MARKER  "}, false},
			} {
				value, err := json.Marshal(tc.value)
				if err != nil {
					t.Fatal(err)
				}
				t.Run(fmt.Sprintf("%s/%s/%s/%s", scope, format, tc.field, value), func(t *testing.T) {
					declaration := "encrypted_reasoning {\n" + tc.field + " = " + string(value) + "\n}"
					if format == FormatJSON {
						declaration = `"encrypted_reasoning":{"` + tc.field + `":` + string(value) + `}`
					}
					source := reasoningRawSource(format, scope, declaration)
					if !tc.valid {
						reasoningRequireRejected(t, source, format, reasoningScopeOwner(scope), tc.field)
						return
					}
					if _, err := Load(source, "test."+format); err != nil {
						t.Fatalf("inclusive bound/default rejected: %v", err)
					}
					if err := ValidateDeferredConfig(source, "test."+format); err != nil {
						t.Fatalf("standalone validation disagrees: %v", err)
					}
					if _, _, _, err := Convert(source, "test."+format, false); err != nil {
						t.Fatalf("conversion validation disagrees: %v", err)
					}
				})
			}
		}
	}
}

func TestEncryptedReasoningJSONSingletonAndDuplicateDeclarations(t *testing.T) {
	for _, scope := range []string{"root", "provider", "model", "alias"} {
		t.Run(scope+"/singleton", func(t *testing.T) {
			rt, err := Load(reasoningRawSource(FormatJSON, scope, `"encrypted_reasoning":[{}]`), "test.json")
			if err != nil {
				t.Fatal(err)
			}
			policy := reasoningPolicyAt(t, rt, scope)
			if !policy.Passthrough || policy.Declaration == nil || policy.Declaration.Passthrough != nil {
				t.Fatalf("singleton reset was reinterpreted: %+v", policy)
			}
		})
		for _, declaration := range []string{
			`"encrypted_reasoning":{},"encrypted_reasoning":{"passthrough":false}`,
			`"encrypted_reasoning":{"passthrough":true,"passthrough":false}`,
			`"encrypted_reasoning":{"on_caller_mismatch":"fail","on_caller_mismatch":"strip_and_retry"}`,
			`"encrypted_reasoning":{"match_messages":[],"match_messages":["custom marker"]}`,
			`"encrypted_reasoning":{"passthrough":false,"Passthrough":true}`,
		} {
			t.Run(scope+"/duplicate/"+declaration, func(t *testing.T) {
				reasoningRequireRejected(t, reasoningRawSource(FormatJSON, scope, declaration), FormatJSON, reasoningScopeOwner(scope), "encrypted_reasoning")
			})
		}
		for _, field := range []string{"passthrough = true\npassthrough = false", `on_caller_mismatch = "fail"` + "\n" + `on_caller_mismatch = "strip_and_retry"`, "match_messages = []\nmatch_messages = []"} {
			t.Run(scope+"/hcl-duplicate/"+field, func(t *testing.T) {
				reasoningRequireRejected(t, reasoningRawSource(FormatHCL, scope, "encrypted_reasoning {\n"+field+"\n}"), FormatHCL, reasoningScopeOwner(scope), "encrypted_reasoning")
			})
		}
	}
}

func TestEncryptedReasoningDisabledProviderValidation(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, scope := range []string{"provider", "model"} {
			for _, valid := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/valid=%t", format, scope, valid), func(t *testing.T) {
					object := reasoningConfigObject("strip", "reset", "retry", "absent")
					owner := reasoningPolicyOwner(object, scope)
					if !valid {
						owner["encrypted_reasoning"] = map[string]any{"match_messages": []string{"abc"}}
					}
					p := reasoningPolicyOwner(object, "provider")
					p["enabled"], p["api_key"] = false, ""
					delete(object, "alias")
					source := reasoningConfigSource(t, format, object)
					if !valid {
						reasoningRequireRejected(t, source, format, reasoningScopeOwner(scope), "match_messages")
						return
					}
					rt, err := Load(source, "test."+format)
					if err != nil {
						t.Fatal(err)
					}
					if rt.Catalog.ProviderCount() != 0 || rt.Catalog.DisabledProviderCount() != 1 {
						t.Fatal("disabled provider was activated")
					}
					disabled := rt.Catalog.DisabledProviders()[0]
					assertReasoningResolution(t, ResolveEncryptedReasoning(rt.EncryptedReasoning, disabled, disabled.Models[0], nil), "retry", EncryptedReasoningSourceModel, false)
				})
			}
		}
	}
}

func TestEncryptedReasoningDynamicPolicyValidation(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, location := range []string{"provider", "model", "index"} {
			for _, policy := range []*EncryptedReasoning{
				{Passthrough: false, OnCallerMismatch: "sometimes"},
				NewEncryptedReasoning(&EncryptedReasoningDeclaration{MatchMessages: reasoningMessages("abcd", " ABCD ")}),
				NewEncryptedReasoning(&EncryptedReasoningDeclaration{MatchMessages: reasoningMessages(strings.Repeat("x", 257))}),
			} {
				t.Run(fmt.Sprintf("enabled=%t/%s/%+v", enabled, location, policy), func(t *testing.T) {
					p := validDynamicProvider()
					p.Enabled = enabled
					owner := fmt.Sprintf("provider %q", p.Name)
					switch location {
					case "provider":
						p.EncryptedReasoning = policy
					case "model":
						p.Models[0].EncryptedReasoning = policy
						owner += fmt.Sprintf(": model %q", p.Models[0].Name)
					case "index":
						model := p.Models[0]
						model.EncryptedReasoning = policy
						p.ModelByName = map[string]Model{model.Name: model}
						owner += fmt.Sprintf(": model %q", model.Name)
					}
					if err := ValidateDynamicProvider(p); err == nil || !strings.Contains(err.Error(), owner) || !strings.Contains(err.Error(), "encrypted_reasoning") {
						t.Fatalf("invalid dynamic policy was accepted or lost its owner: %v", err)
					}
				})
			}
		}
	}
	p := validDynamicProvider()
	p.EncryptedReasoning = &EncryptedReasoning{Passthrough: false}
	p.Models[0].EncryptedReasoning = NewEncryptedReasoning(&EncryptedReasoningDeclaration{})
	if err := ValidateDynamicProvider(p); err != nil {
		t.Fatalf("defaulted legacy and sparse dynamic policies rejected: %v", err)
	}
	rt, err := Load(reasoningRawSource(FormatHCL, "root", "encrypted_reasoning {}"), "test.hcl")
	if err != nil {
		t.Fatal(err)
	}
	rt.EncryptedReasoning = &EncryptedReasoning{OnCallerMismatch: "sometimes"}
	if err := Validate(rt); err == nil || !strings.Contains(err.Error(), "root: encrypted_reasoning.on_caller_mismatch") {
		t.Fatalf("invalid programmatic root policy was accepted: %v", err)
	}
}
