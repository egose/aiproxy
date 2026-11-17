package config

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const encryptedReasoningConvertJSON = `{
  "listener": {"http": {"public": {"address": ":8080"}}},
  "auth": {"main": {"mode": "none"}},
  "provider": {"openai-compatible": {"base": {
    "base_url": "https://upstream.example/v1",
    "api_key": "base-key",
    "model": {"m": {"upstream_name": "upstream/m"}}
  }}},
  "alias": {
    "chat": {"algorithm": "round_robin", "target": {"provider": "base", "model": "m"}},
    "shorthand": {"algorithm": "round_robin", "providers": ["base"], "model": "m"}
  }
}`

func encryptedReasoningConvertHCL(scope, policy string) string {
	blocks := map[string]string{scope: policy}
	return fmt.Sprintf(`listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
%s
provider "openai-compatible" "base" {
  base_url = "https://upstream.example/v1"
  api_key = "base-key"
  %s
  model "m" {
    upstream_name = "upstream/m"
    %s
  }
}
alias "chat" {
  algorithm = "round_robin"
  %s
  target {
    provider = "base"
    model = "m"
  }
}
alias "shorthand" {
  algorithm = "round_robin"
  providers = ["base"]
  model = "m"
  %s
}
`, blocks["root"], blocks["provider"], blocks["model"], blocks["alias"], blocks["alias"])
}

func encryptedReasoningConvertObject(t *testing.T, scope, policy string) map[string]interface{} {
	t.Helper()
	var root map[string]interface{}
	if err := json.Unmarshal([]byte(encryptedReasoningConvertJSON), &root); err != nil {
		t.Fatal(err)
	}
	if policy == "" {
		return root
	}
	var value interface{}
	if err := json.Unmarshal([]byte(policy), &value); err != nil {
		t.Fatal(err)
	}
	provider := root["provider"].(map[string]interface{})["openai-compatible"].(map[string]interface{})["base"].(map[string]interface{})
	switch scope {
	case "root":
		root["encrypted_reasoning"] = value
	case "provider":
		provider["encrypted_reasoning"] = value
	case "model":
		provider["model"].(map[string]interface{})["m"].(map[string]interface{})["encrypted_reasoning"] = value
	case "alias":
		for _, alias := range root["alias"].(map[string]interface{}) {
			alias.(map[string]interface{})["encrypted_reasoning"] = value
		}
	default:
		t.Fatalf("unknown scope %q", scope)
	}
	return root
}

type encryptedReasoningConvertState struct {
	Policy    *EncryptedReasoning
	Inherited bool
	Upstream  string
	Deferred  *DeferredTransport
}

func encryptedReasoningConvertStates(rt *Runtime) map[string]encryptedReasoningConvertState {
	out := map[string]encryptedReasoningConvertState{"root": {Policy: CloneEncryptedReasoning(rt.EncryptedReasoning)}}
	for _, p := range rt.Catalog.Providers() {
		out["provider/"+p.Name] = encryptedReasoningConvertState{Policy: CloneEncryptedReasoning(p.EncryptedReasoning), Inherited: p.EncryptedReasoningInherited, Deferred: p.DeferredTransport}
		for _, m := range p.Models {
			out["model/"+p.Name+"/"+m.Name] = encryptedReasoningConvertState{Policy: CloneEncryptedReasoning(m.EncryptedReasoning), Inherited: m.EncryptedReasoningInherited, Upstream: m.UpstreamName, Deferred: m.DeferredTransport}
		}
	}
	for _, a := range rt.Catalog.Aliases() {
		out["alias/"+a.Name] = encryptedReasoningConvertState{Policy: CloneEncryptedReasoning(a.EncryptedReasoning)}
	}
	return out
}

func encryptedReasoningCheckConversions(t *testing.T, hclSource string, jsonObject map[string]interface{}) {
	t.Helper()
	jsonSource, err := json.Marshal(jsonObject)
	if err != nil {
		t.Fatal(err)
	}
	want, err := Load([]byte(hclSource), "policy.hcl")
	if err != nil {
		t.Fatal(err)
	}
	fromJSON, err := Load(jsonSource, "policy.json")
	if err != nil || !reflect.DeepEqual(encryptedReasoningConvertStates(fromJSON), encryptedReasoningConvertStates(want)) {
		t.Fatalf("initial HCL/JSON declarations differ: %v", err)
	}
	for _, compact := range []bool{false, true} {
		for _, start := range []struct {
			format string
			source []byte
		}{{FormatHCL, []byte(hclSource)}, {FormatJSON, jsonSource}} {
			t.Run(fmt.Sprintf("%s/compact=%t", start.format, compact), func(t *testing.T) {
				source, format := start.source, start.format
				for step := 0; step < 3; step++ {
					out, from, to, err := Convert(source, "policy."+format, compact)
					if err != nil {
						t.Fatalf("step %d: %v", step, err)
					}
					if from != format || to == from {
						t.Fatalf("incorrect formats: %s -> %s, source %s", from, to, format)
					}
					rt, err := Load(out, "policy."+to)
					if err != nil || !reflect.DeepEqual(encryptedReasoningConvertStates(rt), encryptedReasoningConvertStates(want)) {
						t.Fatalf("step %d lost declaration/inheritance intent: %v\n%s", step, err, out)
					}
					if to == FormatJSON {
						var object map[string]interface{}
						if err := json.Unmarshal(out, &object); err != nil {
							t.Fatal(err)
						}
						if !reflect.DeepEqual(object, jsonObject) {
							t.Fatalf("JSON local members/blocks changed:\ngot %s\nwant %s", out, jsonSource)
						}
					}
					source, format = out, to
				}
			})
		}
	}
}

func TestConvertEncryptedReasoningDeclarationsAllScopes(t *testing.T) {
	if arity, exists := jsonBlockLabels["encrypted_reasoning"]; !exists || arity != 0 {
		t.Fatal("encrypted_reasoning must remain an unlabeled conversion block")
	}
	for _, scope := range []string{"root", "provider", "model", "alias"} {
		for _, tc := range []struct {
			name string
			hcl  string
			json string
		}{
			{"absent", "", ""},
			{"reset", "encrypted_reasoning {}", `{}`},
			{"false", "encrypted_reasoning { passthrough = false }", `{"passthrough":false}`},
			{"true", "encrypted_reasoning { passthrough = true }", `{"passthrough":true}`},
			{"empty-mismatch", `encrypted_reasoning { on_caller_mismatch = "" }`, `{"on_caller_mismatch":""}`},
			{"empty-messages", "encrypted_reasoning { match_messages = [] }", `{"match_messages":[]}`},
			{"optional-empty-members", "encrypted_reasoning {\n passthrough = false\n on_caller_mismatch = \"\"\n match_messages = []\n}", `{"passthrough":false,"on_caller_mismatch":"","match_messages":[]}`},
			{"custom", "encrypted_reasoning {\n passthrough = true\n on_caller_mismatch = \"strip_and_retry\"\n match_messages = [\"  Caller Mismatch  \", \"OTHER marker\"]\n}", `{"passthrough":true,"on_caller_mismatch":"strip_and_retry","match_messages":["  Caller Mismatch  ","OTHER marker"]}`},
		} {
			t.Run(scope+"/"+tc.name, func(t *testing.T) {
				hclSource := encryptedReasoningConvertHCL(scope, tc.hcl)
				rt, err := Load([]byte(hclSource), "policy.hcl")
				if err != nil {
					t.Fatal(err)
				}
				key := map[string]string{"root": "root", "provider": "provider/base", "model": "model/base/m", "alias": "alias/chat"}[scope]
				want, err := DecodeEncryptedReasoningJSON([]byte(tc.json), scope)
				if err != nil || !reflect.DeepEqual(encryptedReasoningConvertStates(rt)[key].Policy, want) {
					t.Fatalf("fixture lost explicit presence: %v", err)
				}
				encryptedReasoningCheckConversions(t, hclSource, encryptedReasoningConvertObject(t, scope, tc.json))
			})
		}
	}
}

func TestConvertEncryptedReasoningStaticDerivedOverlays(t *testing.T) {
	hclSource := `listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "bearer_static"
  client "peer" { token = "peer-token" }
}
deferred {}
encrypted_reasoning { passthrough = false }
provider "openai-compatible" "base" {
  api_key = "base-key"
  base_url = "https://peer.example/v1"
  encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }
  model "m" {
    upstream_name = "inherited/m"
    capabilities = ["chat"]
    deferred_transport {}
    encrypted_reasoning { match_messages = ["Base Marker"] }
  }
  model "ordinary" {}
}
provider "openai-compatible" "reasoning" {
  extends = "base"
  api_key = "reasoning-key"
  encrypted_reasoning {}
  model "m" {
    encrypted_reasoning {}
  }
}
provider "openai-compatible" "deferred" {
  extends = "base"
  api_key = "deferred-key"
  model "m" {
    deferred_transport {}
  }
}
provider "openai-compatible" "both" {
  extends = "base"
  api_key = "both-key"
  model "m" {
    deferred_transport {}
    encrypted_reasoning {
      passthrough = false
      on_caller_mismatch = ""
      match_messages = []
    }
  }
}
provider "openai-compatible" "inherited" {
  extends = "base"
  api_key = "inherited-key"
}
`
	jsonSource := `{
 "listener":{"http":{"public":{"address":":8080"}}},
 "auth":{"main":{"mode":"bearer_static","client":{"peer":{"token":"peer-token"}}}},
 "deferred":{}, "encrypted_reasoning":{"passthrough":false},
 "provider":{"openai-compatible":{
  "base":{"api_key":"base-key","base_url":"https://peer.example/v1","encrypted_reasoning":{"on_caller_mismatch":"strip_and_retry"},"model":{
   "m":{"upstream_name":"inherited/m","capabilities":["chat"],"deferred_transport":{},"encrypted_reasoning":{"match_messages":["Base Marker"]}},
   "ordinary":{}
  }},
  "reasoning":{"extends":"base","api_key":"reasoning-key","encrypted_reasoning":{},"model":{"m":{"encrypted_reasoning":{}}}},
  "deferred":{"extends":"base","api_key":"deferred-key","model":{"m":{"deferred_transport":{}}}},
  "both":{"extends":"base","api_key":"both-key","model":{"m":{"deferred_transport":{},"encrypted_reasoning":{"passthrough":false,"on_caller_mismatch":"","match_messages":[]}}}},
  "inherited":{"extends":"base","api_key":"inherited-key"}
 }}
}`
	var object map[string]interface{}
	if err := json.Unmarshal([]byte(jsonSource), &object); err != nil {
		t.Fatal(err)
	}
	encryptedReasoningCheckConversions(t, hclSource, object)
	rt, err := Load([]byte(hclSource), "policy.hcl")
	if err != nil {
		t.Fatal(err)
	}
	states := encryptedReasoningConvertStates(rt)
	for _, name := range []string{"reasoning", "deferred", "both", "inherited"} {
		state := states["model/"+name+"/m"]
		if state.Upstream != "inherited/m" || state.Inherited != (name == "deferred" || name == "inherited") {
			t.Fatalf("%s lost inherited definition or local overlay: %+v", name, state)
		}
		if (state.Deferred != nil) != (name == "deferred" || name == "both") {
			t.Fatalf("%s copied a base opt-in or lost a local one", name)
		}
	}
}

func TestConvertEncryptedReasoningResolvesEnvironmentNormally(t *testing.T) {
	t.Setenv("CONVERT_ER_PASS", "no")
	t.Setenv("CONVERT_ER_MISMATCH", "strip_and_retry")
	t.Setenv("CONVERT_ER_MESSAGE", "  Env Caller Marker  ")
	hclPolicy := `encrypted_reasoning {
  passthrough = env("CONVERT_ER_PASS") == "yes"
  on_caller_mismatch = env("CONVERT_ER_MISMATCH")
  match_messages = [env("CONVERT_ER_MESSAGE")]
}`
	for _, scope := range []string{"root", "provider", "model", "alias"} {
		t.Run(scope, func(t *testing.T) {
			hclSource := encryptedReasoningConvertHCL(scope, hclPolicy)
			object := encryptedReasoningConvertObject(t, scope, `{"passthrough":false,"on_caller_mismatch":"strip_and_retry","match_messages":["  Env Caller Marker  "]}`)
			encryptedReasoningCheckConversions(t, hclSource, object)
			jsonSource, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			jsonSource = []byte(strings.ReplaceAll(string(jsonSource), `"strip_and_retry"`, `env("CONVERT_ER_MISMATCH")`))
			jsonSource = []byte(strings.ReplaceAll(string(jsonSource), `"  Env Caller Marker  "`, `env("CONVERT_ER_MESSAGE")`))
			for _, source := range [][]byte{[]byte(hclSource), jsonSource} {
				out, _, to, err := Convert(source, "policy.config", false)
				if err != nil || strings.Contains(string(out), "env(") {
					t.Fatalf("conversion did not resolve env expressions: %v\n%s", err, out)
				}
				rt, err := Load(out, "policy."+to)
				if err != nil {
					t.Fatal(err)
				}
				key := map[string]string{"root": "root", "provider": "provider/base", "model": "model/base/m", "alias": "alias/chat"}[scope]
				policy := encryptedReasoningConvertStates(rt)[key].Policy
				if policy == nil || policy.Passthrough || policy.OnCallerMismatch != EncryptedReasoningStripAndRetry || !reflect.DeepEqual(policy.MatchMessages, []string{"env caller marker"}) {
					t.Fatalf("resolved policy = %+v", policy)
				}
			}
		})
	}
}

func TestConvertEncryptedReasoningInvalidDeclarationsAreNotDropped(t *testing.T) {
	for _, scope := range []string{"root", "provider", "model", "alias"} {
		for _, policy := range []string{
			`null`, `[]`, `false`, `{"passthrough":null}`, `{"passthrough":"false"}`,
			`{"on_caller_mismatch":null}`, `{"on_caller_mismatch":"retry"}`,
			`{"match_messages":null}`, `{"match_messages":[null]}`, `{"match_messages":["same","SAME"]}`,
			`{"unknown":false}`,
		} {
			t.Run(scope+"/"+policy, func(t *testing.T) {
				source, err := json.Marshal(encryptedReasoningConvertObject(t, scope, policy))
				if err != nil {
					t.Fatal(err)
				}
				if out, _, _, err := Convert(source, "policy.json", false); err == nil || out != nil {
					t.Fatalf("invalid declaration was published or removed: %v\n%s", err, out)
				}
			})
		}
	}
}
