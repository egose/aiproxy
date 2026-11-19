package config

import (
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestModelPassthroughOpenCodeProtocolConversion(t *testing.T) {
	t.Setenv("FIXTURE_RULE_PROTOCOL", "messages")
	const source = `
listener "http" "public" { address = ":8080" }
auth "main" {
  mode = "bearer_static"
  client "test" {
    token = "test-token"
    allowed_model_passthroughs = ["zen/chat-family", "zen/responses-family", "zen/gemini-family", "go/messages-family"]
  }
}
provider "opencode-zen" "zen" {
  model_passthrough "chat-family" {
    match = "^org/chat-.*$"
    protocol = "chat"
    capabilities = ["chat"]
  }
  model_passthrough "responses-family" {
    match = "^org/resp-.*$"
    protocol = "responses"
    capabilities = ["responses"]
  }
  model_passthrough "gemini-family" {
    match = "^gemini-.+$"
    protocol = "gemini"
    capabilities = ["chat", "responses"]
  }
}
provider "opencode-go" "go" {
  api_key = "fixture-go-key"
  model_passthrough "messages-family" {
    match = "^org/claude-.*$"
    protocol = env("FIXTURE_RULE_PROTOCOL")
    capabilities = ["chat", "messages"]
  }
}
`
	converted, from, to, err := Convert([]byte(source), "opencode.hcl", false)
	if err != nil || from != FormatHCL || to != FormatJSON || !strings.Contains(string(converted), `"protocol": "messages"`) {
		t.Fatalf("HCL to JSON = %s -> %s, %v", from, to, err)
	}
	jsonWithEnv := strings.Replace(string(converted), `"protocol": "messages"`, `"protocol": env("FIXTURE_RULE_PROTOCOL")`, 1)
	roundTrip, from, to, err := Convert([]byte(jsonWithEnv), EnvConfigFilename, false)
	if err != nil || from != FormatJSON || to != FormatHCL {
		t.Fatalf("inline JSON to HCL = %s -> %s, %v", from, to, err)
	}
	assertRules := func(t *testing.T, rt *Runtime) {
		t.Helper()
		for _, tc := range []struct {
			provider, rule, suffix string
			protocol               ModelProtocol
			caps                   []Capability
		}{
			{"zen", "chat-family", "org/chat-next", ModelProtocolChat, []Capability{CapabilityChat}},
			{"zen", "responses-family", "org/resp-next", ModelProtocolResponses, []Capability{CapabilityResponses}},
			{"zen", "gemini-family", "gemini-next.1", ModelProtocolGemini, []Capability{CapabilityChat, CapabilityResponses}},
			{"go", "messages-family", "org/claude-next", ModelProtocolMessages, []Capability{CapabilityChat, CapabilityMessages}},
		} {
			p := testProvider(t, rt, tc.provider)
			rule, match := p.MatchModelPassthrough(tc.suffix)
			if match != ModelPassthroughMatched || rule.Name() != tc.rule || rule.Protocol() != tc.protocol ||
				!slices.Equal(rule.Capabilities(), tc.caps) {
				t.Fatalf("%s/%s: match=%v, rule=%q, protocol=%q, capabilities=%v", tc.provider, tc.suffix, match, rule.Name(), rule.Protocol(), rule.Capabilities())
			}
			if len(p.Models) != 0 || len(p.ModelByName) != 0 {
				t.Fatalf("rule registered a literal model for %s", p.Name)
			}
		}
		if got := rt.Auth.Clients["test"].AllowedModelPassthroughs; !slices.Equal(got, []string{"zen/chat-family", "zen/responses-family", "zen/gemini-family", "go/messages-family"}) {
			t.Fatalf("rule grants = %v", got)
		}
	}
	for _, tc := range []struct{ name, source string }{
		{"original.hcl", source},
		{"converted.json", string(converted)},
		{"with-env.json", jsonWithEnv},
		{"round-trip.hcl", string(roundTrip)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := Load([]byte(tc.source), tc.name)
			if err != nil {
				t.Fatal(err)
			}
			assertRules(t, rt)
			t.Setenv(ConfigEnvVar, tc.source)
			rt, err = LoadEnv()
			if err != nil {
				t.Fatal(err)
			}
			assertRules(t, rt)
		})
	}
}

func TestModelPassthroughOpenCodeProtocolValidation(t *testing.T) {
	t.Setenv("FIXTURE_RULE_PROTOCOL", "")
	for _, typ := range ProviderTypes() {
		t.Run(string(typ), func(t *testing.T) {
			check := func(t *testing.T, protocol, capabilities, want string) {
				t.Helper()
				var hclAttrs, jsonAttrs string
				for _, attr := range []struct{ name, expr string }{{"protocol", protocol}, {"capabilities", capabilities}} {
					if attr.expr != "" {
						hclAttrs += fmt.Sprintf("\n    %s = %s", attr.name, attr.expr)
						jsonAttrs += fmt.Sprintf(",%q:%s", attr.name, attr.expr)
					}
				}
				var hclLiteral, jsonLiteral string
				if IsOpenCodeProviderType(typ) {
					hclLiteral = `model "pinned" { protocol = "chat" }`
					jsonLiteral = `"model":{"pinned":{"protocol":"chat"}},`
				}
				for _, src := range []struct{ file, body string }{
					{"protocol.hcl", passthroughTestPrefix + fmt.Sprintf(`
provider %q "upstream" {
  enabled = false
  base_url = "https://fixture.example/v1"
  %s
  model_passthrough "family" {
    match = ".+"%s
  }
}`, typ, hclLiteral, hclAttrs)},
					{"protocol.json", fmt.Sprintf(`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},
"provider":{%q:{"upstream":{"enabled":false,"base_url":"https://fixture.example/v1",%s"model_passthrough":{"family":{"match":".+"%s}}}}}}`, typ, jsonLiteral, jsonAttrs)},
				} {
					if _, err := Load([]byte(src.body), src.file); err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("%s load = %v, want %q", src.file, err, want)
					}
					if _, _, _, err := Convert([]byte(src.body), src.file, false); err == nil || !strings.Contains(err.Error(), want) {
						t.Fatalf("%s convert = %v, want %q", src.file, err, want)
					}
				}
			}
			if !IsOpenCodeProviderType(typ) {
				for _, protocol := range []string{`"chat"`, `""`, `null`} {
					t.Run(protocol, func(t *testing.T) {
						check(t, protocol, `["chat"]`, "protocol is only supported by opencode-zen and opencode-go")
					})
				}
				return
			}
			for _, tc := range []struct{ name, protocol, capabilities, want string }{
				{"omitted", "", `["chat"]`, "protocol is required"},
				{"empty", `""`, `["chat"]`, "protocol must be a nonempty string"},
				{"null", "null", `["chat"]`, "protocol must be a nonempty string"},
				{"numeric", "1", `["chat"]`, "protocol must be a nonempty string"},
				{"boolean", "true", `["chat"]`, "protocol must be a nonempty string"},
				{"array", `["chat"]`, `["chat"]`, "protocol must be a nonempty string"},
				{"object", `{"type":"chat"}`, `["chat"]`, "protocol must be a nonempty string"},
				{"empty-env", `env("FIXTURE_RULE_PROTOCOL")`, `["chat"]`, "protocol must be a nonempty string"},
				{"unknown", `"other"`, `["chat"]`, `invalid protocol "other"`},
				{"case", `"Chat"`, `["chat"]`, `invalid protocol "Chat"`},
				{"whitespace", `" chat "`, `["chat"]`, `invalid protocol " chat "`},
				{"chat-cannot-serve-responses", `"chat"`, `["responses"]`, `not served by protocol "chat"`},
				{"responses-cannot-serve-chat", `"responses"`, `["chat"]`, `not served by protocol "responses"`},
				{"responses-cannot-serve-messages", `"responses"`, `["messages"]`, `not served by protocol "responses"`},
				{"messages-cannot-serve-embeddings", `"messages"`, `["embeddings"]`, "not supported by provider type"},
				{"missing-capabilities", `"messages"`, "", "required"},
				{"empty-capabilities", `"messages"`, `[]`, "capabilities must not be empty"},
				{"null-capabilities", `"messages"`, `null`, "capabilities must be a list"},
			} {
				t.Run(tc.name, func(t *testing.T) { check(t, tc.protocol, tc.capabilities, tc.want) })
			}
			if typ == ProviderTypeOpenCodeGo {
				t.Run("gemini", func(t *testing.T) {
					check(t, `"gemini"`, `["chat"]`, `protocol "gemini" is not supported by provider type "opencode-go"`)
				})
			}
		})
	}
}
