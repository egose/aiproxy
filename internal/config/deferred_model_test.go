package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

const deferredModelTestAuth = `listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
`

func deferredModelTestSource(t *testing.T, format string, providers map[string]any, root bool) []byte {
	t.Helper()
	object := map[string]any{
		"listener": map[string]any{"http": map[string]any{"public": map[string]any{"address": ":8080"}}},
		"auth":     map[string]any{"main": map[string]any{"mode": "none"}},
		"provider": providers,
	}
	if root {
		object["deferred"] = map[string]any{}
		object["auth"] = map[string]any{"main": map[string]any{
			"mode":   "bearer_static",
			"client": map[string]any{"peer": map[string]any{"token": "peer-token"}},
		}}
	}
	source, err := json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	if format == FormatJSON {
		return source
	}
	source, err = decodeJSONConfig(source, "test.json")
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func deferredModelPeer(models map[string]any, fields map[string]any) map[string]any {
	peer := map[string]any{
		"base_url": "https://peer.example/mount/v1",
		"api_key":  "peer-key",
		"model":    models,
	}
	for name, value := range fields {
		peer[name] = value
	}
	return peer
}

func deferredModelRawSource(format, body string) []byte {
	if format == FormatHCL {
		return []byte(deferredModelTestAuth + `provider "openai-compatible" "peer" {
  base_url = "https://peer.example/mount/v1"
  api_key = "peer-key"
  model "chat" {
    ` + body + `
  }
  model "ordinary" {}
}
`)
	}
	return []byte(`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai-compatible":{"peer":{"base_url":"https://peer.example/mount/v1","api_key":"peer-key","model":{"chat":{` + body + `},"ordinary":{}}}}}}`)
}

func deferredModelRejected(t *testing.T, source []byte, format, want string) {
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
			if err := check.run(); err == nil || (want != "" && !strings.Contains(err.Error(), want)) {
				t.Fatalf("invalid model declaration error = %v, want %q", err, want)
			}
		})
	}
}

func TestLoadDeferredModelOptInsAndPrecedence(t *testing.T) {
	providerOptions := DeferredTransport{
		ControlTimeout: 20 * time.Second, MaxWait: time.Hour, PollInterval: time.Second,
		MaxControlCalls: 2048, MaxControlRetries: 20,
	}
	providerBlock := map[string]any{
		"control_timeout": "20s", "max_wait": "1h", "poll_interval": "1s",
		"max_control_calls": 2048, "max_control_retries": 20,
	}
	partial := DefaultDeferredTransport()
	partial.MaxWait = 20 * time.Minute
	partial.MaxControlRetries = 0
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, compatibilityDefault := range []bool{false, true} {
			for _, local := range []struct {
				name    string
				options any
				want    *DeferredTransport
			}{
				{"omitted", nil, nil},
				{"empty", map[string]any{}, deferredModelOptions(DefaultDeferredTransport())},
				{"partial", map[string]any{"max_wait": "20m", "max_control_retries": 0}, &partial},
				{"full", providerBlock, &providerOptions},
			} {
				t.Run(fmt.Sprintf("%s/default=%t/%s", format, compatibilityDefault, local.name), func(t *testing.T) {
					chat := map[string]any{"upstream_name": "executor/chat"}
					if local.options != nil {
						chat["deferred_transport"] = local.options
					}
					fields := map[string]any{}
					if compatibilityDefault {
						fields["deferred_transport"] = providerBlock
					}
					source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{
						"peer": deferredModelPeer(map[string]any{
							"chat": chat, "ordinary": map[string]any{},
							"embedding": map[string]any{"capabilities": []any{"embeddings"}},
						}, fields),
					}}, false)
					rt, err := Load(source, "test."+format)
					if err != nil {
						t.Fatal(err)
					}
					p := testProvider(t, rt, "peer")
					if rt.Deferred.Enabled || (p.DeferredTransport != nil) != compatibilityDefault {
						t.Fatal("caller-only root or compatibility-default presence changed")
					}
					for _, model := range p.Models {
						wantLocal := (*DeferredTransport)(nil)
						if model.Name == "chat" {
							wantLocal = local.want
						}
						if !reflect.DeepEqual(model.DeferredTransport, wantLocal) || !reflect.DeepEqual(p.ModelByName[model.Name].DeferredTransport, wantLocal) {
							t.Fatalf("model %s local options disagree: slice=%+v index=%+v want=%+v", model.Name, model.DeferredTransport, p.ModelByName[model.Name].DeferredTransport, wantLocal)
						}
						wantEffective := wantLocal
						if wantEffective == nil && compatibilityDefault {
							wantEffective = &providerOptions
						}
						if got := EffectiveDeferredTransport(p, model); !reflect.DeepEqual(got, wantEffective) {
							t.Fatalf("model %s effective options = %+v, want %+v", model.Name, got, wantEffective)
						}
					}
					if HasCallerDeferredTransport(p) != (compatibilityDefault || local.want != nil) {
						t.Fatal("caller-presence lookup disagrees with local declarations")
					}
					if err := ValidateDeferredConfig(source, "test."+format); err != nil {
						t.Fatalf("standalone validation disagrees: %v", err)
					}
				})
			}
		}
	}
}

func deferredModelOptions(options DeferredTransport) *DeferredTransport {
	return &options
}

func TestDeferredModelDeclarationsRejected(t *testing.T) {
	for _, declaration := range []string{
		`deferred_transport "named" {}`,
		"deferred_transport {}\n    deferred_transport {}",
		"deferred_transport = {}",
		"deferred_transport = null",
		"deferred_transport { enabled = false }",
		"deferred_transport { unknown = 1 }",
		"deferred_transport {\n      limits {}\n    }",
		"deferred_transport { max_wait = null }",
		"deferred_transport { max_control_calls = null }",
		"deferred_transport { max_control_retries = [] }",
		"deferred_transport { poll_interval = unknown_value }",
		"deferred_receiver {}",
		"deferred_receiver = null",
	} {
		t.Run("hcl/"+declaration, func(t *testing.T) {
			deferredModelRejected(t, deferredModelRawSource(FormatHCL, declaration), FormatHCL, "deferred")
		})
	}
	for _, value := range []string{
		"null", "false", "12", `""`, `"5s"`, "[]", "[null]", "[false]", "[{},{}]",
		`{"enabled":false}`, `{"unknown":1}`, `{"named":{}}`,
		`{"max_wait":null}`, `{"max_control_calls":null}`, `{"max_control_retries":[]}`,
		`{"max_wait":{}}`, `{"poll_interval":true}`,
	} {
		t.Run("json/"+value, func(t *testing.T) {
			deferredModelRejected(t, deferredModelRawSource(FormatJSON, `"deferred_transport":`+value), FormatJSON, "deferred")
		})
	}
	for _, declaration := range []string{
		`"deferred_transport":{},"deferred_transport":{}`,
		`"deferred_transport":{"max_wait":"1s","max_wait":"2h"}`,
		`"deferred_receiver":{}`, `"deferred_receiver":[{}]`, `"deferred_receiver":null`,
	} {
		t.Run("json/"+declaration, func(t *testing.T) {
			deferredModelRejected(t, deferredModelRawSource(FormatJSON, declaration), FormatJSON, "")
		})
	}
	for _, format := range []string{FormatHCL, FormatJSON} {
		t.Run(format+"/wrong case", func(t *testing.T) {
			body := "Deferred_Transport {}"
			if format == FormatJSON {
				body = `"Deferred_Transport":{}`
			}
			deferredModelRejected(t, deferredModelRawSource(format, body), format, "Deferred_Transport")
		})
	}
}

func TestDeferredModelTransportBounds(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, field := range []struct {
			name string
			good []string
			bad  []string
		}{
			{"control_timeout", []string{`"100ms"`, `"30s"`}, []string{`"99ms"`, `"30s1ns"`, `""`, `"0s"`, `"-1s"`, `"invalid"`}},
			{"max_wait", []string{`"1s"`, `"2h"`}, []string{`"999ms"`, `"2h1ns"`, `""`, `"0s"`, `"9223372036854775808ns"`}},
			{"poll_interval", []string{`"50ms"`, `"5s"`}, []string{`"49ms"`, `"5s1ns"`, `""`, `"-1s"`}},
			{"max_control_calls", []string{"4", "65536"}, []string{"3", "65537", "0", "-1", "1.25", "9223372036854775808"}},
			{"max_control_retries", []string{"0", "32"}, []string{"-1", "33", "1.25", "9223372036854775808"}},
		} {
			for _, values := range []struct {
				list []string
				bad  bool
			}{{field.good, false}, {field.bad, true}} {
				for _, value := range values.list {
					t.Run(format+"/"+field.name+"/"+value, func(t *testing.T) {
						body := "deferred_transport {\n      " + field.name + " = " + value + "\n    }"
						if format == FormatJSON {
							body = `"deferred_transport":{"` + field.name + `":` + value + `}`
						}
						source := deferredModelRawSource(format, body)
						if values.bad {
							want := field.name
							if value == "1.25" || value == "9223372036854775808" {
								want = "whole number"
							}
							deferredModelRejected(t, source, format, want)
							return
						}
						if _, err := Load(source, "test."+format); err != nil {
							t.Fatalf("inclusive bound rejected: %v", err)
						}
						if err := ValidateDeferredConfig(source, "test."+format); err != nil {
							t.Fatalf("standalone rejected inclusive bound: %v", err)
						}
						if _, _, _, err := Convert(source, "test."+format, false); err != nil {
							t.Fatalf("conversion rejected inclusive bound: %v", err)
						}
					})
				}
			}
		}
	}
}

func TestDeferredModelConversionRejectsFractionalLimitsBeforeRounding(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for name, value := range map[string]string{
			"max_control_calls":   "4096.0000000000000000000000001",
			"max_control_retries": "8.0000000000000000000000001",
		} {
			t.Run(format+"/"+name, func(t *testing.T) {
				body := "deferred_transport { " + name + " = " + value + " }"
				if format == FormatJSON {
					body = `"deferred_transport":{"` + name + `":` + value + `}`
				}
				deferredModelRejected(t, deferredModelRawSource(format, body), format, "whole number")
			})
		}
	}
}

func TestDeferredModelProviderEligibility(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range []struct {
			name   string
			fields map[string]any
			models map[string]any
			root   bool
			want   string
		}{
			{"disabled missing credential", map[string]any{"enabled": false, "api_key": ""}, nil, false, "configured api_key"},
			{"disabled missing reference key", map[string]any{"enabled": false, "api_key": "", "api_key_ref": map[string]any{"key": ""}}, nil, false, "api_key_ref.key"},
			{"disabled conflicting credentials", map[string]any{"enabled": false, "api_key_ref": map[string]any{"key": "peer"}}, nil, false, "only one"},
			{"forwarded credential", map[string]any{"forward_authorization": true, "api_key": ""}, nil, false, "forward_authorization"},
			{"direct embedding", nil, map[string]any{"chat": map[string]any{}, "embedding": map[string]any{
				"capabilities": []any{"embeddings"}, "deferred_transport": map[string]any{},
			}}, false, "chat-capable"},
			{"direct responses", nil, map[string]any{"chat": map[string]any{}, "responses": map[string]any{
				"capabilities": []any{"responses"}, "deferred_transport": map[string]any{},
			}}, false, "chat-capable"},
			{"receiver and model caller", map[string]any{"deferred_receiver": map[string]any{}}, nil, true, "cannot be combined"},
			{"disabled receiver and model caller", map[string]any{"enabled": false, "deferred_receiver": map[string]any{}}, nil, true, "cannot be combined"},
			{"default without chat", map[string]any{"deferred_transport": map[string]any{}}, map[string]any{
				"embedding": map[string]any{"capabilities": []any{"embeddings"}},
			}, false, "chat-capable"},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				models := tc.models
				if models == nil {
					models = map[string]any{"chat": map[string]any{"deferred_transport": map[string]any{}}}
				}
				source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{
					"peer": deferredModelPeer(models, tc.fields),
				}}, tc.root)
				deferredModelRejected(t, source, format, tc.want)
			})
		}
		for _, providerType := range []ProviderType{
			ProviderTypeOpenAI, ProviderTypeAnthropic, ProviderTypeGemini, ProviderTypeOpenCodeZen,
			ProviderTypeOpenCodeGo, ProviderTypeGitHubCopilot, ProviderTypeZenMux, ProviderTypeOpenRouter,
		} {
			t.Run(format+"/wrong type/"+string(providerType), func(t *testing.T) {
				model := map[string]any{"deferred_transport": map[string]any{}}
				if IsOpenCodeProviderType(providerType) {
					model["protocol"] = "chat"
				}
				fields := map[string]any{"enabled": false}
				if providerType == ProviderTypeGitHubCopilot {
					fields["api_key"] = ""
					fields["credential_ref"] = map[string]any{"name": "peer", "path": "/not-opened/keys.json"}
				}
				source := deferredModelTestSource(t, format, map[string]any{string(providerType): map[string]any{
					"peer": deferredModelPeer(map[string]any{"chat": model}, fields),
				}}, false)
				deferredModelRejected(t, source, format, "static openai-compatible")
			})
		}
	}
}

func TestDeferredModelCallerBaseURLRules(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range []struct {
			base     string
			insecure bool
			valid    bool
		}{
			{"https://peer.example/mount/v1", false, true},
			{"https://peer.example/a%20b/v1", false, true},
			{"http://127.0.0.1:8080/mount/v1", false, true},
			{"http://peer.example/mount/v1", true, true},
			{"http://peer.example/mount/v1", false, false},
			{"", false, false},
			{"/v1", false, false},
			{"ftp://peer.example/v1", false, false},
			{"https://user:pass@peer.example/v1", false, false},
			{"https://peer.example/v1?", false, false},
			{"https://peer.example/v1?query=value", false, false},
			{"https://peer.example/v1#", false, false},
			{"https://peer.example/mount/../v1", false, false},
			{"https://peer.example/mount/%252e%252e/v1", false, false},
			{"https://peer.example/mount%2fv1", false, false},
			{"https://peer.example/mount%252fv1", false, false},
			{"https://peer.example/mount%5cv1", false, false},
			{"https://peer.example/mount\\name/v1", false, false},
		} {
			t.Run(format+"/"+tc.base+fmt.Sprint(tc.insecure), func(t *testing.T) {
				source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{
					"peer": deferredModelPeer(map[string]any{"chat": map[string]any{"deferred_transport": map[string]any{}}}, map[string]any{
						"base_url": tc.base, "allow_insecure_base_url": tc.insecure, "enabled": false,
					}),
				}}, false)
				if !tc.valid {
					deferredModelRejected(t, source, format, "base_url")
					return
				}
				if _, err := Load(source, "test."+format); err != nil {
					t.Fatal(err)
				}
				if err := ValidateDeferredConfig(source, "test."+format); err != nil {
					t.Fatal(err)
				}
			})
		}
	}
}

func TestDeferredModelDisabledCredentialReferenceAndStandaloneNoIO(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, enabled := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/enabled=%t", format, enabled), func(t *testing.T) {
				peer := deferredModelPeer(map[string]any{"chat": map[string]any{"deferred_transport": map[string]any{}}}, map[string]any{
					"enabled": enabled, "api_key_ref": map[string]any{"path": "/not-opened/keys.json", "key": "peer"},
				})
				delete(peer, "api_key")
				source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{"peer": peer}}, false)
				if err := ValidateDeferredConfig(source, "test."+format); err != nil {
					t.Fatalf("standalone validation must not read credentials: %v", err)
				}
				rt, err := Load(source, "test."+format)
				if enabled {
					if err == nil || !strings.Contains(err.Error(), "read api_key_ref") {
						t.Fatalf("enabled loading must still resolve credentials: %v", err)
					}
					return
				}
				if err != nil {
					t.Fatal(err)
				}
				p := rt.Catalog.DisabledProviders()[0]
				if p.APIKeyRef == nil || p.APIKeyRef.Resolved || p.APIKey != "" || p.Models[0].DeferredTransport == nil || p.ModelByName["chat"].DeferredTransport == nil {
					t.Fatal("disabled caller credential or both model opt-in views were lost")
				}
				if _, _, _, err := Convert(source, "test."+format, false); err != nil {
					t.Fatalf("disabled unresolved credentials must survive conversion: %v", err)
				}
			})
		}
	}
}

func TestDeferredModelDerivativeOptInsAreLocal(t *testing.T) {
	baseOptions := map[string]any{"max_wait": "1h", "control_timeout": "20s", "max_control_retries": 20}
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, baseDefault := range []bool{false, true} {
			for _, local := range []struct {
				name        string
				fields      map[string]any
				model       any
				wantModel   *DeferredTransport
				wantDefault *DeferredTransport
				receiver    bool
			}{
				{name: "ordinary"},
				{name: "empty overlay", model: map[string]any{}, wantModel: deferredModelOptions(DefaultDeferredTransport())},
				{name: "partial overlay", model: map[string]any{"max_wait": "20m"}, wantModel: deferredModelOptions(DeferredTransport{
					ControlTimeout: 5 * time.Second, MaxWait: 20 * time.Minute, PollInterval: 250 * time.Millisecond,
					MaxControlCalls: 4096, MaxControlRetries: 8,
				})},
				{name: "local default", fields: map[string]any{"deferred_transport": map[string]any{}}, wantDefault: deferredModelOptions(DefaultDeferredTransport())},
				{name: "default and overlay", fields: map[string]any{"deferred_transport": baseOptions}, model: map[string]any{},
					wantModel: deferredModelOptions(DefaultDeferredTransport()), wantDefault: deferredModelOptions(DeferredTransport{
						ControlTimeout: 20 * time.Second, MaxWait: time.Hour, PollInterval: 250 * time.Millisecond,
						MaxControlCalls: 4096, MaxControlRetries: 20,
					})},
				{name: "local receiver", fields: map[string]any{"deferred_receiver": map[string]any{}}, receiver: true},
			} {
				t.Run(fmt.Sprintf("%s/base-default=%t/%s", format, baseDefault, local.name), func(t *testing.T) {
					baseFields := map[string]any{}
					if baseDefault {
						baseFields["deferred_transport"] = baseOptions
					}
					child := map[string]any{"extends": "base", "api_key": "child-key"}
					for name, value := range local.fields {
						child[name] = value
					}
					if local.model != nil {
						child["model"] = map[string]any{"chat": map[string]any{"deferred_transport": local.model}}
					}
					source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{
						"base": deferredModelPeer(map[string]any{
							"chat": map[string]any{
								"display_name": "Chat", "upstream_name": "executor/chat",
								"capabilities": []any{"chat", "responses"}, "deferred_transport": baseOptions,
								"pricing": map[string]any{"input_per_million": 2},
							},
							"sibling":   map[string]any{"deferred_transport": baseOptions},
							"embedding": map[string]any{"capabilities": []any{"embeddings"}},
						}, baseFields),
						"child": child,
						"other": map[string]any{"extends": "base", "api_key": "other-key"},
					}}, local.receiver)
					rt, err := Load(source, "test."+format)
					if err != nil {
						t.Fatal(err)
					}
					p := testProvider(t, rt, "child")
					if p.APIKey != "child-key" || p.BaseURL != "https://peer.example/mount/v1" || !reflect.DeepEqual(p.DeferredTransport, local.wantDefault) || (p.DeferredReceiver != nil) != local.receiver {
						t.Fatalf("derived provider local options changed: %+v", p)
					}
					for _, model := range p.Models {
						want := (*DeferredTransport)(nil)
						if model.Name == "chat" {
							want = local.wantModel
							if model.DisplayName != "Chat" || model.UpstreamName != "executor/chat" || !reflect.DeepEqual(model.Capabilities, []Capability{CapabilityChat, CapabilityResponses}) || model.Pricing == nil || model.Pricing.InputPerMillion != 2 {
								t.Fatalf("overlay changed inherited model definition: %+v", model)
							}
						}
						if !reflect.DeepEqual(model.DeferredTransport, want) || !reflect.DeepEqual(p.ModelByName[model.Name].DeferredTransport, want) {
							t.Fatalf("inherited opt-in or inconsistent overlay on %s", model.Name)
						}
					}
					other := testProvider(t, rt, "other")
					if HasCallerDeferredTransport(other) || other.DeferredReceiver != nil {
						t.Fatal("base or sibling derivative caller options leaked")
					}
					if err := ValidateDeferredConfig(source, "test."+format); err != nil {
						t.Fatalf("standalone derivative validation disagrees: %v", err)
					}
					converted, _, _, err := Convert(source, "test."+format, false)
					if err != nil {
						t.Fatal(err)
					}
					got, err := Load(converted, "converted")
					if err != nil || !reflect.DeepEqual(testProvider(t, got, "child"), p) {
						t.Fatalf("derived conversion changed local options: %v", err)
					}
				})
			}
		}
	}
}

func TestDeferredModelDerivativeOverlayRejections(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, tc := range []struct {
			name   string
			models map[string]any
			fields map[string]any
			root   bool
			want   string
		}{
			{"empty no deferred anywhere", map[string]any{"chat": map[string]any{}}, nil, false, "deferred_transport"},
			{"new model", map[string]any{"new": map[string]any{"deferred_transport": map[string]any{}}}, nil, false, "new"},
			{"duplicate overlay", map[string]any{"chat": []any{
				map[string]any{"deferred_transport": map[string]any{}},
				map[string]any{"deferred_transport": map[string]any{}},
			}}, nil, false, "duplicate"},
			{"nonchat overlay", map[string]any{"embedding": map[string]any{"deferred_transport": map[string]any{}}}, nil, false, "chat-capable"},
			{"receiver conflict", map[string]any{"chat": map[string]any{"deferred_transport": map[string]any{}}}, map[string]any{"deferred_receiver": map[string]any{}}, true, "cannot be combined"},
			{"forwarding conflict", map[string]any{"chat": map[string]any{"deferred_transport": map[string]any{}}}, map[string]any{"api_key": "", "forward_authorization": true}, false, "forward_authorization"},
		} {
			t.Run(format+"/"+tc.name, func(t *testing.T) {
				child := map[string]any{"extends": "base", "api_key": "child-key", "model": tc.models}
				for name, value := range tc.fields {
					child[name] = value
				}
				source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{
					"base": deferredModelPeer(map[string]any{
						"chat": map[string]any{}, "embedding": map[string]any{"capabilities": []any{"embeddings"}},
					}, nil),
					"child": child,
				}}, tc.root)
				deferredModelRejected(t, source, format, tc.want)
			})
		}
		for _, field := range []struct {
			name   string
			values []any
		}{
			{"display_name", []any{"", "Replacement", nil}},
			{"upstream_name", []any{"", "replacement", nil}},
			{"protocol", []any{"", "chat", nil}},
			{"capabilities", []any{[]any{}, []any{"chat"}, nil}},
			{"pricing", []any{map[string]any{}, map[string]any{"input_per_million": 1}, []any{}, nil}},
		} {
			for _, value := range field.values {
				if _, array := value.([]any); format == FormatHCL && field.name == "pricing" && array {
					continue
				}
				for _, opted := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/%s/%v/opted=%t", format, field.name, value, opted), func(t *testing.T) {
						model := map[string]any{field.name: value}
						if opted {
							model["deferred_transport"] = map[string]any{}
						}
						source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{
							"base":  deferredModelPeer(map[string]any{"chat": map[string]any{}}, nil),
							"child": map[string]any{"extends": "base", "api_key": "child-key", "model": map[string]any{"chat": model}},
						}}, false)
						deferredModelRejected(t, source, format, field.name)
					})
				}
			}
		}
	}
}

func TestDeferredModelJSONDerivativeOverlayObjects(t *testing.T) {
	for _, value := range []any{nil, false, 12, "", []any{}, []any{nil}, []any{false}} {
		t.Run(fmt.Sprint(value), func(t *testing.T) {
			source := deferredModelTestSource(t, FormatJSON, map[string]any{"openai-compatible": map[string]any{
				"base": deferredModelPeer(map[string]any{"chat": map[string]any{}}, nil),
				"child": map[string]any{
					"extends": "base", "api_key": "child-key", "model": map[string]any{"chat": value},
				},
			}}, false)
			deferredModelRejected(t, source, FormatJSON, "model")
		})
	}
}

func TestDeferredModelDerivativeMultipleLocalOverlaysAndReceiverBase(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		t.Run(format, func(t *testing.T) {
			source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{
				"base": deferredModelPeer(map[string]any{
					"model-1": map[string]any{"upstream_name": "executor/model-1"},
					"model-2": map[string]any{"upstream_name": "executor/model-2"},
					"model-3": map[string]any{"upstream_name": "executor/model-3"},
				}, map[string]any{"deferred_receiver": map[string]any{}}),
				"child": map[string]any{
					"extends": "base", "api_key": "child-key",
					"model": map[string]any{
						"model-2": map[string]any{"deferred_transport": map[string]any{"max_wait": "20m"}},
						"model-1": map[string]any{"deferred_transport": map[string]any{}},
					},
				},
			}}, true)
			rt, err := Load(source, "test."+format)
			if err != nil {
				t.Fatal(err)
			}
			base, child := testProvider(t, rt, "base"), testProvider(t, rt, "child")
			if base.DeferredReceiver == nil || child.DeferredReceiver != nil || child.DeferredTransport != nil {
				t.Fatal("receiver eligibility was inherited or caller placement changed")
			}
			for _, model := range child.Models {
				if model.UpstreamName != "executor/"+model.Name || !reflect.DeepEqual(model.DeferredTransport, child.ModelByName[model.Name].DeferredTransport) {
					t.Fatal("multiple overlays changed inherited definitions or model-view consistency")
				}
				switch model.Name {
				case "model-1":
					if model.DeferredTransport == nil || *model.DeferredTransport != DefaultDeferredTransport() {
						t.Fatal("empty local overlay did not use standard defaults")
					}
				case "model-2":
					if model.DeferredTransport == nil || model.DeferredTransport.MaxWait != 20*time.Minute {
						t.Fatal("second local overlay did not apply to its named model")
					}
				case "model-3":
					if EffectiveDeferredTransport(child, model) != nil {
						t.Fatal("ordinary inherited sibling became deferred")
					}
				}
			}
			if err := ValidateDeferredConfig(source, "test."+format); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDeferredModelStandaloneDerivativeOverlaysDoNotReadCredentials(t *testing.T) {
	for _, format := range []string{FormatHCL, FormatJSON} {
		for _, valid := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/valid=%t", format, valid), func(t *testing.T) {
				base := deferredModelPeer(map[string]any{"chat": map[string]any{}}, map[string]any{
					"api_key_ref": map[string]any{"path": "/not-opened/base-keys.json", "key": "base"},
				})
				delete(base, "api_key")
				model := map[string]any{"deferred_transport": map[string]any{}}
				if !valid {
					model["capabilities"] = []any{}
				}
				source := deferredModelTestSource(t, format, map[string]any{"openai-compatible": map[string]any{
					"base": base,
					"child": map[string]any{
						"extends": "base", "api_key_ref": map[string]any{"path": "/not-opened/child-keys.json", "key": "child"},
						"model": map[string]any{"chat": model},
					},
				}}, false)
				err := ValidateDeferredConfig(source, "test."+format)
				if valid {
					if err != nil {
						t.Fatalf("standalone derivative validation performed credential I/O: %v", err)
					}
				} else if err == nil || !strings.Contains(err.Error(), "capabilities") || strings.Contains(err.Error(), "read api_key_ref") {
					t.Fatalf("invalid overlay was not rejected by declaration presence without credential I/O: %v", err)
				}
			})
		}
	}
}

func TestDeferredModelRoundTripFileInlineAndEnv(t *testing.T) {
	t.Setenv("MODEL_DEFER_KEY", "peer-key")
	t.Setenv("MODEL_DEFER_TIMEOUT", "100ms")
	t.Setenv("MODEL_DEFER_WAIT", "20m")
	t.Setenv("MODEL_DEFER_POLL", "500ms")
	t.Setenv("MODEL_DEFER_CALLS", "64")
	t.Setenv("MODEL_DEFER_RETRIES", "0")
	source := deferredModelTestAuth + `provider "openai-compatible" "peer" {
  api_key = env("MODEL_DEFER_KEY")
  base_url = "https://peer.example/mount/v1"
  model "model-1" { upstream_name = "executor/model-1" }
  model "model-2" {
    upstream_name = "executor/model-2"
    deferred_transport {
      control_timeout = env("MODEL_DEFER_TIMEOUT")
      max_wait = env("MODEL_DEFER_WAIT")
      poll_interval = env("MODEL_DEFER_POLL")
      max_control_calls = env("MODEL_DEFER_CALLS")
      max_control_retries = env("MODEL_DEFER_RETRIES")
    }
  }
}
`
	want, err := LoadFile(writeTempFile(t, "config.hcl", source))
	if err != nil {
		t.Fatal(err)
	}
	p := testProvider(t, want, "peer")
	if EffectiveDeferredTransport(p, p.ModelByName["model-1"]) != nil || p.ModelByName["model-2"].DeferredTransport == nil {
		t.Fatal("planned mixed-model caller placement did not load")
	}
	if got := *p.ModelByName["model-2"].DeferredTransport; got != (DeferredTransport{
		ControlTimeout: 100 * time.Millisecond, MaxWait: 20 * time.Minute, PollInterval: 500 * time.Millisecond,
		MaxControlCalls: 64, MaxControlRetries: 0,
	}) {
		t.Fatalf("env-expanded model options = %+v", got)
	}
	for _, compact := range []bool{false, true} {
		jsonSource, from, to, err := Convert([]byte(source), "test.hcl", compact)
		if err != nil || from != FormatHCL || to != FormatJSON || strings.Contains(string(jsonSource), "env(") {
			t.Fatalf("HCL conversion did not resolve model expressions: %v", err)
		}
		hclSource, from, to, err := Convert(jsonSource, "test.json", false)
		if err != nil || from != FormatJSON || to != FormatHCL {
			t.Fatalf("JSON conversion failed: %v", err)
		}
		jsonEnv := strings.Replace(string(jsonSource), `"20m"`, `env("MODEL_DEFER_WAIT")`, 1)
		jsonEnv = strings.Replace(jsonEnv, `"max_control_calls":64`, `"max_control_calls":env("MODEL_DEFER_CALLS")`, 1)
		jsonEnv = strings.Replace(jsonEnv, `"max_control_calls": 64`, `"max_control_calls": env("MODEL_DEFER_CALLS")`, 1)
		for _, candidate := range []string{source, string(jsonSource), string(hclSource), jsonEnv} {
			t.Setenv(ConfigEnvVar, candidate)
			got, err := LoadFileOrEnv("/not-opened/config.hcl", false)
			if err != nil || !reflect.DeepEqual(testProvider(t, got, "peer"), p) {
				t.Fatalf("file/inline/env round-trip changed model placement or options: %v", err)
			}
			if err := ValidateDeferredConfig([]byte(candidate), EnvConfigFilename); err != nil {
				t.Fatalf("standalone inline/env validation failed: %v", err)
			}
		}
	}
	t.Setenv("MODEL_DEFER_CALLS", "64.0000000000000000000000001")
	t.Setenv(ConfigEnvVar, source)
	if _, err := LoadEnv(); err == nil || !strings.Contains(err.Error(), "whole number") {
		t.Fatalf("inline fractional env limit escaped exact validation: %v", err)
	}
	if _, _, _, err := Convert([]byte(source), EnvConfigFilename, false); err == nil || !strings.Contains(err.Error(), "whole number") {
		t.Fatalf("conversion rounded a fractional model env limit: %v", err)
	}
}

func TestDeferredModelJSONSingletonBlock(t *testing.T) {
	for _, value := range []string{"{}", "[{}]"} {
		t.Run(value, func(t *testing.T) {
			source := deferredModelRawSource(FormatJSON, `"deferred_transport":`+value)
			rt, err := Load(source, "test.json")
			if err != nil {
				t.Fatal(err)
			}
			p := testProvider(t, rt, "peer")
			if got := p.ModelByName["chat"].DeferredTransport; got == nil || *got != DefaultDeferredTransport() || p.DeferredTransport != nil {
				t.Fatalf("model singleton block presence/defaults changed: %+v", got)
			}
			if _, _, _, err := Convert(source, "test.json", false); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestDeferredModelCatalogCopiesBothViewsAndSiblings(t *testing.T) {
	options := DefaultDeferredTransport()
	model := Model{Name: "chat", UpstreamName: "chat", DeferredTransport: &options}
	sibling := model
	sibling.Name, sibling.UpstreamName = "sibling", "sibling"
	indexed := model
	indexed.Name, indexed.UpstreamName = "indexed", "indexed"
	p := Provider{
		Name: "peer", Type: ProviderTypeOpenAICompatible, APIKey: "peer-key",
		BaseURL: "https://peer.example/v1", Enabled: true, DeferredTransport: &options,
		Models: []Model{model, sibling}, ModelByName: map[string]Model{"chat": model, "sibling": sibling, "indexed": indexed},
	}
	disabled := p
	disabled.Name, disabled.Enabled = "disabled", false
	catalog := NewCatalog([]Provider{p}, []Provider{disabled}, nil)
	options.MaxWait = time.Second
	check := func(p Provider) {
		t.Helper()
		if p.DeferredTransport.MaxWait != 15*time.Minute || len(p.Models) != 3 {
			t.Fatal("catalog retained input pointers or lost an indexed-only model")
		}
		for _, m := range p.Models {
			if m.DeferredTransport.MaxWait != 15*time.Minute || p.ModelByName[m.Name].DeferredTransport.MaxWait != 15*time.Minute {
				t.Fatalf("catalog policy mutated on %s", m.Name)
			}
		}
	}
	for _, p := range append(catalog.Providers(), catalog.DisabledProviders()...) {
		check(p)
		p.Models[0].DeferredTransport.MaxWait = time.Second
		if p.ModelByName["chat"].DeferredTransport.MaxWait != 15*time.Minute || p.Models[1].DeferredTransport.MaxWait != 15*time.Minute || p.DeferredTransport.MaxWait != 15*time.Minute {
			t.Fatal("returned slice model shares options with the index, siblings, or provider default")
		}
		p.ModelByName["sibling"].DeferredTransport.MaxWait = 2 * time.Second
		if p.Models[1].DeferredTransport.MaxWait != 15*time.Minute {
			t.Fatal("returned indexed model shares options with the slice")
		}
		p.ModelByName["indexed"].DeferredTransport.MaxWait = 3 * time.Second
		if p.Models[2].DeferredTransport.MaxWait != 15*time.Minute {
			t.Fatal("indexed-only model shares options across cloned views")
		}
	}
	got, model, ok := catalog.Model("peer", "chat")
	if !ok {
		t.Fatal("catalog model disappeared")
	}
	model.DeferredTransport.MaxWait = 4 * time.Second
	if got.ModelByName["chat"].DeferredTransport.MaxWait != 15*time.Minute || got.Models[0].DeferredTransport.MaxWait != 15*time.Minute {
		t.Fatal("Catalog.Model shares options with its returned provider")
	}
	effective := EffectiveDeferredTransport(got, got.Models[0])
	effective.MaxWait = 5 * time.Second
	if got.Models[0].DeferredTransport.MaxWait != 15*time.Minute {
		t.Fatal("effective model lookup exposed a mutable options pointer")
	}
	ordinary := Model{Name: "ordinary"}
	EffectiveDeferredTransport(got, ordinary).MaxWait = 6 * time.Second
	if got.DeferredTransport.MaxWait != 15*time.Minute {
		t.Fatal("effective provider-default lookup exposed a mutable options pointer")
	}
	again, _ := catalog.Provider("peer")
	check(again)
	check(catalog.DisabledProviders()[0])
}

func TestDeferredModelIndexedPresenceSurvivesCloningAndGuards(t *testing.T) {
	for _, indexed := range []bool{false, true} {
		t.Run(fmt.Sprint(indexed), func(t *testing.T) {
			model := Model{Name: "chat", UpstreamName: "chat"}
			p := Provider{
				Type: ProviderTypeOpenAICompatible, Name: "peer", BaseURL: "https://peer.example/v1", APIKey: "peer-key",
				Models: []Model{model}, ModelByName: map[string]Model{"chat": model},
				DeferredReceiver: &DeferredReceiver{ExecutionTimeout: time.Minute, MaxResultBytes: 4096},
			}
			if indexed {
				model.DeferredTransport = deferredModelOptions(DefaultDeferredTransport())
				p.ModelByName["chat"] = model
			} else {
				p.Models[0].DeferredTransport = deferredModelOptions(DefaultDeferredTransport())
			}
			if !HasCallerDeferredTransport(p) {
				t.Fatal("presence lookup ignored a model view")
			}
			root := DefaultDeferred()
			root.Enabled = true
			if err := validateProviderDeferred(p, root); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
				t.Fatalf("original-input receiver guard ignored caller presence in one view: %v", err)
			}
			catalog := NewCatalog([]Provider{p}, nil, nil)
			got, _ := catalog.Provider("peer")
			if !HasCallerDeferredTransport(got) {
				t.Fatal("catalog normalization discarded a conflicting indexed opt-in before guards")
			}
			if err := validateProviderDeferred(got, root); err == nil || !strings.Contains(err.Error(), "cannot be combined") {
				t.Fatalf("receiver guard ignored caller opt-in in one view: %v", err)
			}
			got.DeferredReceiver = nil
			if indexed {
				got.ModelByName["chat"].DeferredTransport.MaxControlCalls = 0
			} else {
				got.Models[0].DeferredTransport.MaxControlCalls = 0
			}
			if err := validateProviderDeferred(got, root); err == nil || !strings.Contains(err.Error(), "max_control_calls") {
				t.Fatalf("invalid model options in one view escaped validation: %v", err)
			}
		})
	}
}

func TestDeferredModelCatalogKeepsOrderedDefinitionsAndIndexedCallerOptions(t *testing.T) {
	ordered := Model{Name: "chat", UpstreamName: "updated", Capabilities: []Capability{CapabilityChat}}
	indexed := Model{Name: "chat", UpstreamName: "stale", DeferredTransport: deferredModelOptions(DefaultDeferredTransport())}
	p := Provider{
		Name: "peer", Models: []Model{ordered}, ModelByName: map[string]Model{"chat": indexed},
	}
	catalog := NewCatalog([]Provider{p}, nil, nil)
	got, model, ok := catalog.Model("peer", "chat")
	if !ok || model.UpstreamName != "updated" || !reflect.DeepEqual(model.Capabilities, ordered.Capabilities) {
		t.Fatal("stale indexed model definitions overrode the ordered model definition")
	}
	if !HasCallerDeferredTransport(got) {
		t.Fatal("rebuilding model definitions normalized away indexed caller presence")
	}
	want := DefaultDeferredTransport()
	for _, candidate := range []Model{got.Models[0], got.ModelByName["chat"], model} {
		if candidate.DeferredTransport == nil || *candidate.DeferredTransport != want || !reflect.DeepEqual(EffectiveDeferredTransport(got, candidate), &want) {
			t.Fatal("indexed-only caller policy was not canonicalized into every model view")
		}
	}
	if got.Models[0].DeferredTransport == got.ModelByName["chat"].DeferredTransport || model.DeferredTransport == got.Models[0].DeferredTransport || model.DeferredTransport == got.ModelByName["chat"].DeferredTransport {
		t.Fatal("canonicalized model views share a caller policy pointer")
	}
	if p.Models[0].DeferredTransport != nil {
		t.Fatal("catalog normalization mutated the original ordered model")
	}
	indexed.DeferredTransport.MaxWait = time.Second
	if *model.DeferredTransport != want || *got.Models[0].DeferredTransport != want || *got.ModelByName["chat"].DeferredTransport != want {
		t.Fatal("retained indexed options share the caller's mutable pointer")
	}
	got.Models[0].DeferredTransport.MaxWait = 2 * time.Second
	if *model.DeferredTransport != want || *got.ModelByName["chat"].DeferredTransport != want {
		t.Fatal("updating the returned slice policy mutated other model views")
	}
	again, resolved, ok := catalog.Model("peer", "chat")
	if !ok || *resolved.DeferredTransport != want || *again.Models[0].DeferredTransport != want {
		t.Fatal("updating returned caller policies mutated the catalog")
	}
}

func TestDeferredModelCatalogUsesOrderedCallerPolicyOverStaleIndex(t *testing.T) {
	updated := DeferredTransport{
		ControlTimeout: 2 * time.Second, MaxWait: 20 * time.Minute, PollInterval: 50 * time.Millisecond,
		MaxControlCalls: 4, MaxControlRetries: 0,
	}
	for _, tc := range []struct {
		name    string
		indexed *DeferredTransport
		ordered DeferredTransport
	}{
		{"stale nil index", nil, updated},
		{"stale nonnil index", deferredModelOptions(DefaultDeferredTransport()), updated},
		{"empty ordered pointer replaces indexed policy", deferredModelOptions(DefaultDeferredTransport()), DeferredTransport{}},
	} {
		for _, returned := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/returned=%t", tc.name, returned), func(t *testing.T) {
				stale := Model{Name: "chat", UpstreamName: "stale", DeferredTransport: cloneDeferredTransport(tc.indexed)}
				p := Provider{
					Name: "peer", Type: ProviderTypeOpenAICompatible, BaseURL: "https://peer.example/v1", APIKey: "peer-key", Enabled: true,
					DeferredTransport: deferredModelOptions(DefaultDeferredTransport()),
					Models:            []Model{stale}, ModelByName: map[string]Model{"chat": stale},
				}
				if returned {
					p, _ = NewCatalog([]Provider{p}, nil, nil).Provider("peer")
				}
				ordered := tc.ordered
				p.Models[0].DeferredTransport = &ordered
				p.Models[0].UpstreamName = "updated"
				p.Models[0].Capabilities = []Capability{CapabilityChat}
				catalog := NewCatalog([]Provider{p}, nil, nil)
				got, model, ok := catalog.Model("peer", "chat")
				if !ok {
					t.Fatal("resolved model disappeared")
				}
				for _, candidate := range []Model{got.Models[0], got.ModelByName["chat"], model} {
					if candidate.UpstreamName != "updated" || !reflect.DeepEqual(candidate.Capabilities, []Capability{CapabilityChat}) {
						t.Fatal("stale index overrode the ordered model definition")
					}
					if candidate.DeferredTransport == nil || *candidate.DeferredTransport != tc.ordered || !reflect.DeepEqual(EffectiveDeferredTransport(got, candidate), &tc.ordered) {
						t.Fatalf("stale index erased, replaced, or field-merged ordered caller policy: got=%+v want=%+v", candidate.DeferredTransport, tc.ordered)
					}
				}
				if got.Models[0].DeferredTransport == got.ModelByName["chat"].DeferredTransport || model.DeferredTransport == got.Models[0].DeferredTransport || model.DeferredTransport == got.ModelByName["chat"].DeferredTransport {
					t.Fatal("canonical policy pointers are shared across returned model views")
				}
				if !reflect.DeepEqual(p.ModelByName["chat"].DeferredTransport, tc.indexed) || *p.Models[0].DeferredTransport != tc.ordered {
					t.Fatal("catalog canonicalization mutated the input policy views")
				}
				p.Models[0].DeferredTransport.MaxWait = time.Second
				if p.ModelByName["chat"].DeferredTransport != nil {
					p.ModelByName["chat"].DeferredTransport.MaxWait = 2 * time.Second
				}
				if *got.Models[0].DeferredTransport != tc.ordered || *got.ModelByName["chat"].DeferredTransport != tc.ordered || *model.DeferredTransport != tc.ordered {
					t.Fatal("canonical policies retain mutable input pointers")
				}
				got.Models[0].DeferredTransport.MaxWait = 3 * time.Second
				if *got.ModelByName["chat"].DeferredTransport != tc.ordered || *model.DeferredTransport != tc.ordered {
					t.Fatal("returned slice policy shares a pointer with the index or resolved model")
				}
				got.ModelByName["chat"].DeferredTransport.MaxWait = 4 * time.Second
				if got.Models[0].DeferredTransport.MaxWait != 3*time.Second || *model.DeferredTransport != tc.ordered {
					t.Fatal("returned indexed policy shares a pointer with the slice or resolved model")
				}
				effective := EffectiveDeferredTransport(got, model)
				effective.MaxWait = 5 * time.Second
				if *model.DeferredTransport != tc.ordered {
					t.Fatal("effective resolved lookup exposed the model policy pointer")
				}
				again, resolved, ok := catalog.Model("peer", "chat")
				if !ok || *resolved.DeferredTransport != tc.ordered || *again.Models[0].DeferredTransport != tc.ordered || *again.ModelByName["chat"].DeferredTransport != tc.ordered {
					t.Fatal("mutating input or returned policies changed subsequent catalog lookups")
				}
			})
		}
	}
}

func TestDeferredModelDerivativeClearsBothInheritedViews(t *testing.T) {
	base := Provider{
		Type: ProviderTypeOpenAICompatible, Name: "base", BaseURL: "https://peer.example/v1", APIKey: "base-key", Enabled: true,
		DeferredTransport: deferredModelOptions(DefaultDeferredTransport()),
		Models:            []Model{{Name: "chat", UpstreamName: "chat"}},
		ModelByName: map[string]Model{
			"chat": {Name: "chat", UpstreamName: "chat", DeferredTransport: deferredModelOptions(DefaultDeferredTransport())},
		},
	}
	source := []byte(`provider "openai-compatible" "child" {
  extends = "base"
  api_key = "child-key"
}`)
	file, diags := hclsyntax.ParseConfig(source, "test.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatal(diags)
	}
	var raw rawFile
	if diags := gohcl.DecodeBody(file.Body, nil, &raw); diags.HasErrors() {
		t.Fatal(diags)
	}
	annotateProviderSyntax(file.Body, &raw)
	child, err := buildDerivedProvider(raw.Providers[0], map[string]rawProvider{
		"base": {Name: "base", Type: string(ProviderTypeOpenAICompatible), Models: []rawModel{{Name: "chat"}}},
	}, map[string]rawProviderSyntax{"child": raw.providerSyntax[0]}, map[string]Provider{"base": base}, DefaultDeferred())
	if err != nil || HasCallerDeferredTransport(child) {
		t.Fatalf("derivative retained caller options in one inherited view: %v", err)
	}
	if !HasCallerDeferredTransport(base) || base.ModelByName["chat"].DeferredTransport == nil {
		t.Fatal("clearing the child mutated the base provider")
	}
}
