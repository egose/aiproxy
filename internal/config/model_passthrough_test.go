package config

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/copilotlogin"
)

const passthroughSampleLiteral = `
  model "qwen/qwen3-32b" {
    upstream_name = "pinned-chat"
    capabilities  = ["chat"]
  }
`

const passthroughSample = `
listener "http" "public" {
  address = "127.0.0.1:8080"
}

provider "openai-compatible" "router" {
  base_url = "https://router.example/v1"
  api_key  = env("ROUTER_API_KEY")
` + passthroughSampleLiteral + `
  model_passthrough "qwen3-chat" {
    match        = "^qwen/qwen3-[0-9]+b(-[a-z0-9-]+)?$"
    capabilities = ["chat"]
  }

  model_passthrough "embeddings" {
    match        = "^openai/text-embedding-3-[a-z0-9-]+$"
    capabilities = ["embeddings"]
  }
}

auth "main" {
  mode = "bearer_static"
  client "chat-app" {
    token                      = env("CHAT_APP_TOKEN")
    allowed_models             = ["router/qwen/qwen3-32b"]
    allowed_model_passthroughs = ["router/qwen3-chat"]
  }
}
`

func assertPassthroughSample(t *testing.T, rt *Runtime, literal bool) {
	t.Helper()
	p, ok := rt.Catalog.Provider("router")
	if !ok || len(p.ModelPassthroughs) != 2 {
		t.Fatal("missing router rules")
	}
	rule, result := p.MatchModelPassthrough("qwen/qwen3-72b")
	if result != ModelPassthroughMatched || rule.Name() != "qwen3-chat" || !slices.Equal(rule.Capabilities(), []Capability{CapabilityChat}) {
		t.Fatalf("chat rule = %q, %v, %v", rule.Name(), rule.Capabilities(), result)
	}
	rule, result = p.MatchModelPassthrough("openai/text-embedding-3-small")
	if result != ModelPassthroughMatched || rule.Name() != "embeddings" || !slices.Equal(rule.Capabilities(), []Capability{CapabilityEmbeddings}) {
		t.Fatalf("embedding rule = %q, %v, %v", rule.Name(), rule.Capabilities(), result)
	}
	c := rt.Auth.Clients["chat-app"]
	if !slices.Equal(c.AllowedModels, []string{"router/qwen/qwen3-32b"}) ||
		!slices.Equal(c.AllowedModelPassthroughs, []string{"router/qwen3-chat"}) {
		t.Fatal("literal allowlist and separate rule grants were not preserved")
	}
	_, model, ok := rt.Catalog.Model(p.Name, "qwen/qwen3-32b")
	if ok != literal || (literal && model.UpstreamName != "pinned-chat") || len(p.Models) != len(p.ModelByName) || len(p.Models) > 1 {
		t.Fatal("literal catalog was not preserved")
	}
	for _, name := range []string{"qwen3-chat", "embeddings", "qwen/qwen3-72b", "openai/text-embedding-3-small"} {
		if _, _, exists := rt.Catalog.Model(p.Name, name); exists {
			t.Fatalf("nonliteral %q entered the model index", name)
		}
	}
}

func TestDocumentedModelPassthrough(t *testing.T) {
	t.Setenv("ROUTER_API_KEY", "test-router-key")
	t.Setenv("CHAT_APP_TOKEN", "test-chat-token")
	for _, literal := range []bool{true, false} {
		t.Run(fmt.Sprintf("literal-%t", literal), func(t *testing.T) {
			source := passthroughSample
			if !literal {
				source = strings.Replace(source, passthroughSampleLiteral, "", 1)
			}
			rt, err := Load([]byte(source), "sample.hcl")
			if err != nil {
				t.Fatal(err)
			}
			assertPassthroughSample(t, rt, literal)
			t.Setenv(ConfigEnvVar, source)
			rt, err = LoadEnv()
			if err != nil {
				t.Fatal(err)
			}
			assertPassthroughSample(t, rt, literal)

			converted, from, to, err := Convert([]byte(source), "sample.hcl", false)
			if err != nil || from != FormatHCL || to != FormatJSON {
				t.Fatalf("convert %q -> %q: %v", from, to, err)
			}
			converted = []byte(strings.Replace(string(converted), `"test-chat-token"`, `env("CHAT_APP_TOKEN")`, 1))
			t.Setenv(ConfigEnvVar, string(converted))
			rt, err = LoadEnv()
			if err != nil {
				t.Fatal(err)
			}
			assertPassthroughSample(t, rt, literal)
			roundTrip, from, to, err := Convert(converted, EnvConfigFilename, false)
			if err != nil || from != FormatJSON || to != FormatHCL {
				t.Fatalf("convert %q -> %q: %v", from, to, err)
			}
			rt, err = Load(roundTrip, "round-trip.hcl")
			if err != nil {
				t.Fatal(err)
			}
			assertPassthroughSample(t, rt, literal)
		})
	}
}

func TestModelPassthroughFullMatchAndSnapshot(t *testing.T) {
	t.Setenv("FULL_MATCH_PATTERN", `(?im)^FOO|FOOBAR$|other/\d+`)
	t.Setenv("FULL_MATCH_GRANT", "router/family")
	src := `{
  "listener":{"http":{"public":{"address":":8080"}}},
  "auth":{"main":{"mode":"bearer_static","client":{"test":{
    "token":"test-token","allowed_model_passthroughs":[env("FULL_MATCH_GRANT")]
  }}}},
  "provider":{"openai":{"router":{
    "api_key":"router-key",
    "model_passthrough":[{"family":{"match":env("FULL_MATCH_PATTERN"),"capabilities":["chat","responses"]}},
      {"overlap":[{"match":"^(?i:dup)/id$|^other/7$","capabilities":["embeddings"]}]}]
  }}}
}`
	back, _, _, err := Convert([]byte(src), "family.json", false)
	if err != nil {
		t.Fatal(err)
	}
	for _, source := range [][]byte{[]byte(src), back} {
		rt, err := Load(source, "family")
		if err != nil {
			t.Fatal(err)
		}
		p, _ := rt.Catalog.Provider("router")
		if !slices.Equal(rt.Auth.Clients["test"].AllowedModelPassthroughs, []string{"router/family"}) {
			t.Fatal("grant changed")
		}
		for _, suffix := range []string{"foo", "foobar", "other/42"} {
			rule, result := p.MatchModelPassthrough(suffix)
			if result != ModelPassthroughMatched || rule.Name() != "family" {
				t.Errorf("valid full match %q = %q (%v)", suffix, rule.Name(), result)
			}
		}
		for _, suffix := range []string{"fooz", "xfoobar", "prefix/foobar", "other/42/extra", "other//42", "other/42:free", "FOO", "foo\nbar", " foo", ""} {
			if rule, result := p.MatchModelPassthrough(suffix); result != ModelPassthroughNoMatch {
				t.Errorf("unexpected match for %q = %q (%v)", suffix, rule.Name(), result)
			}
		}
		if rule, result := p.MatchModelPassthrough("other/7"); result != ModelPassthroughAmbiguous || rule.Name() != "" {
			t.Fatalf("overlap selected a rule: %q (%v)", rule.Name(), result)
		}
		clone := NewCatalog(rt.Catalog.Providers(), nil, nil)
		rule, _ := p.MatchModelPassthrough("foo")
		rule.Capabilities()[0] = CapabilityMessages
		clear(p.ModelPassthroughs)
		for _, catalog := range []Catalog{rt.Catalog, clone} {
			q, _ := catalog.Provider("router")
			got, result := q.MatchModelPassthrough("foo")
			if result != ModelPassthroughMatched || got.pattern != rule.pattern || !slices.Equal(got.Capabilities(), []Capability{CapabilityChat, CapabilityResponses}) {
				t.Fatal("snapshot reused mutable declarations, recompiled, or lost the rule")
			}
		}
		if err := Validate(rt); err != nil {
			t.Fatal(err)
		}
	}
}

const passthroughTestPrefix = `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
`

func TestModelPassthroughInvalidDeclarations(t *testing.T) {
	for _, tc := range []struct {
		name, hclRule, jsonRule, want string
	}{
		{"bad-regex", `match = "*"`, `"match":"*"`, "invalid match regexp"},
		{"bad-regex-cannot-escape-wrapping", `match = "x)|(?:y"`, `"match":"x)|(?:y"`, "invalid match regexp"},
		{"empty-match", `match = ""`, `"match":""`, "match must be a nonempty string"},
		{"null-match", `match = null`, `"match":null`, "match must be a nonempty string"},
		{"number-match", `match = 123`, `"match":123`, "match must be a nonempty string"},
		{"missing-match", "", "", "required"},
		{"oversize-match", `match = "` + strings.Repeat("é", 257) + `"`, `"match":"` + strings.Repeat("é", 257) + `"`, "at most 512 bytes"},
		{"empty-capabilities", `capabilities = []`, `"capabilities":[]`, "capabilities must not be empty"},
		{"null-capabilities", `capabilities = null`, `"capabilities":null`, "capabilities must be a list"},
		{"missing-capabilities", "", "", "required"},
		{"null-capability", `capabilities = [null]`, `"capabilities":[null]`, "non-null strings"},
		{"unknown-capability", `capabilities = ["completions"]`, `"capabilities":["completions"]`, "invalid capability"},
		{"unsupported-capability", `capabilities = ["messages"]`, `"capabilities":["messages"]`, "not supported by provider type"},
		{"duplicate-capability", `capabilities = ["chat", "chat"]`, `"capabilities":["chat","chat"]`, "duplicate capability"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			hcl, json := tc.hclRule, tc.jsonRule
			if strings.Contains(tc.name, "capabilit") {
				hcl = `match = "x"` + "\n" + hcl
				json = strings.TrimSuffix(`"match":"x",`+json, ",")
			} else {
				hcl += "\n" + `capabilities = ["chat"]`
				json = strings.TrimPrefix(json+`,"capabilities":["chat"]`, ",")
			}
			sources := []string{
				passthroughTestPrefix + `provider "openai" "router" {
  api_key = "test-key"
  model "literal" {}
  model_passthrough "family" {
` + hcl + `
  }
}`,
				`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},
 "provider":{"openai":{"router":{"api_key":"test-key","model":{"literal":{}},"model_passthrough":{"family":{` + json + `}}}}}}`,
			}
			for _, source := range sources {
				if _, err := Load([]byte(source), "invalid"); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("load error = %v, want %q", err, tc.want)
				}
				if _, _, _, err := Convert([]byte(source), "invalid", false); err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("convert error = %v, want %q", err, tc.want)
				}
			}
		})
	}
}

func TestModelPassthroughLabelsAndBounds(t *testing.T) {
	rule := func(label, pattern string) string {
		return fmt.Sprintf("\n  model_passthrough %q {\n    match = %q\n    capabilities = [\"chat\"]\n  }\n", label, pattern)
	}
	start := passthroughTestPrefix + `provider "openai" "router" {
  api_key = "test-key"
`
	for _, tc := range []struct{ source, want string }{
		{start + rule("Bad/label", "x") + "}", "name must be lowercase"},
		{start + rule("same", "x") + rule("same", "y") + "}", "duplicate"},
		{`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"router":{
			"api_key":"test-key","model_passthrough":{"same":{"match":"x","capabilities":["chat"]},"same":{"match":"y","capabilities":["chat"]}}
		}}}}`, "distinct named"},
		{`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"router":{
			"api_key":"test-key","model_passthrough":{"same":[{"match":"x","capabilities":["chat"]},{"match":"y","capabilities":["chat"]}]}
		}}}}`, "duplicate"},
		{`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"router":{
			"api_key":"test-key","model":{"literal":{}},"model_passthrough":{"missing":null}
		}}}}`, "non-null block"},
		{`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},"provider":{"openai":{"router":{
			"api_key":"test-key","model":{"literal":{}},"model_passthrough":{"missing":[]}
		}}}}`, "non-null block"},
	} {
		if _, err := Load([]byte(tc.source), "invalid"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("load error = %v, want %q", err, tc.want)
		}
		if _, _, _, err := Convert([]byte(tc.source), "invalid", false); err == nil {
			t.Fatal("conversion dropped an invalid declaration")
		}
	}

	var source strings.Builder
	source.WriteString(start)
	source.WriteString(rule("x_1.0", "a/.+|"+strings.Repeat("z", MaxModelPassthroughPatternBytes-5)))
	for i := 1; i < MaxModelPassthroughRules; i++ {
		source.WriteString(rule("other-"+strconv.Itoa(i), "id-"+strconv.Itoa(i)))
	}
	rt, err := Load([]byte(source.String()+"}"), "bounds.hcl")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := rt.Catalog.Provider("router")
	for _, n := range []int{MaxModelPassthroughSuffixBytes, MaxModelPassthroughSuffixBytes + 1} {
		_, result := p.MatchModelPassthrough("a/" + strings.Repeat("x", n-2))
		if (result == ModelPassthroughMatched) != (n == MaxModelPassthroughSuffixBytes) {
			t.Fatalf("suffix length %d gave %v", n, result)
		}
	}
	source.WriteString(rule("over-limit", "extra"))
	if _, err := Load([]byte(source.String()+"}"), "bounds.hcl"); err == nil || !strings.Contains(err.Error(), "at most 32 model_passthrough rules") {
		t.Fatalf("too many rules: %v", err)
	}
}

func TestModelPassthroughProviderScope(t *testing.T) {
	const openAICapabilities = `["chat","responses","embeddings","images","audio_transcriptions","audio_speech"]`
	enabledTypes := map[ProviderType]string{
		ProviderTypeOpenAI:           openAICapabilities,
		ProviderTypeOpenAICompatible: openAICapabilities,
		ProviderTypeOpenRouter:       openAICapabilities,
		ProviderTypeZenMux:           openAICapabilities,
		ProviderTypeAnthropic:        `["chat","responses","messages"]`,
		ProviderTypeGitHubCopilot:    `["chat"]`,
		ProviderTypeOpenCodeZen:      `["chat","responses","messages"]`,
		ProviderTypeOpenCodeGo:       `["chat","responses","messages"]`,
		ProviderTypeGemini:           `["chat","responses","embeddings"]`,
	}
	for _, providerType := range ProviderTypes() {
		t.Run(string(providerType), func(t *testing.T) {
			caps, supported := enabledTypes[providerType]
			if !supported {
				caps = `["chat"]`
			}
			protocol := ""
			if IsOpenCodeProviderType(providerType) {
				protocol = `protocol = "messages"`
			}
			src := passthroughTestPrefix + fmt.Sprintf(`
provider %q "router" {
  base_url = "https://router.example/v1"
  enabled = false
  model_passthrough "family" {
    match = "^.+$"
    %s
    capabilities = %s
  }
}`, providerType, protocol, caps)
			rt, err := Load([]byte(src), "providers.hcl")
			if !supported {
				if err == nil || !strings.Contains(err.Error(), "model_passthrough is not supported") {
					t.Fatalf("unsupported provider: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if rt.Catalog.ProviderCount() != 0 || rt.Catalog.DisabledProviderCount() != 1 {
				t.Fatal("valid disabled provider was not retained only as disabled")
			}
			p := rt.Catalog.DisabledProviders()[0]
			if _, match := p.MatchModelPassthrough("org/fixture"); len(p.ModelPassthroughs) != 1 || match != ModelPassthroughNoMatch {
				t.Fatalf("disabled provider did not retain one inactive rule: %v, %v", len(p.ModelPassthroughs), match)
			}
		})
	}
}

func TestModelPassthroughAnthropicCopilotInvalidCapabilities(t *testing.T) {
	for _, tc := range []struct {
		providerType ProviderType
		unsupported  []Capability
	}{
		{ProviderTypeAnthropic, []Capability{CapabilityEmbeddings, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech}},
		{ProviderTypeGitHubCopilot, []Capability{CapabilityResponses, CapabilityMessages, CapabilityEmbeddings, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech}},
	} {
		for _, capability := range tc.unsupported {
			t.Run(string(tc.providerType)+"/"+string(capability), func(t *testing.T) {
				src := passthroughTestPrefix + fmt.Sprintf(`
provider %q "router" {
  enabled = false
  model_passthrough "family" {
    match = ".+"
    capabilities = ["chat", %q]
  }
}`, tc.providerType, capability)
				if _, err := Load([]byte(src), "providers.hcl"); err == nil || !strings.Contains(err.Error(), "model_passthrough \"family\": capability") ||
					!strings.Contains(err.Error(), "not supported by provider type") {
					t.Fatalf("unsupported rule capability: %v", err)
				}
			})
		}
	}
}

func TestModelPassthroughCopilotCredentialValidation(t *testing.T) {
	secretsPath := writeCopilotCredential(t, "main", "fixture-copilot-token")
	sidecar, err := copilotlogin.SidecarPath(secretsPath, "malformed")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sidecar, []byte(`{"access_token":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	ref := func(name string) string {
		return fmt.Sprintf("\n  credential_ref {\n    name = %q\n    path = %q\n  }\n", name, secretsPath)
	}
	for _, tc := range []struct{ name, settings, want string }{
		{"missing-reference", "", "require a resolvable credential_ref"},
		{"missing-sidecar", ref("absent"), `credential "absent" not found`},
		{"invalid-sidecar", ref("malformed"), `invalid credential "malformed"`},
		{"invalid-reference", ref("bad/name"), "invalid credential name"},
		{"api-key", `api_key = "fixture-key"`, "api_key and api_key_ref are not supported"},
		{"caller-authorization", "forward_authorization = true\n" + ref("main"), "forward_authorization is not supported"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := passthroughTestPrefix + fmt.Sprintf(`
provider "github-copilot" "copilot" {
  %s
  model_passthrough "family" {
    match = ".+"
    capabilities = ["chat"]
  }
}`, tc.settings)
			if _, err := Load([]byte(src), "copilot.hcl"); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("credential validation = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestModelPassthroughGrantsAndInheritance(t *testing.T) {
	t.Setenv("ROUTER_API_KEY", "test-router-key")
	t.Setenv("CHAT_APP_TOKEN", "test-chat-token")
	child := `
provider "openai-compatible" "child" {
  extends = "router"
  api_key = "test-child-key"
}
`
	rt, err := Load([]byte(passthroughSample+child), "inherit.hcl")
	if err != nil {
		t.Fatal(err)
	}
	p, model, ok := rt.Catalog.Model("child", "qwen/qwen3-32b")
	if !ok || model.UpstreamName != "pinned-chat" || p.APIKey != "test-child-key" || len(p.ModelPassthroughs) != 0 {
		t.Fatal("static derivative did not retain just literal inventory and local credentials")
	}
	disabled := strings.Replace(passthroughSample, `api_key  = env("ROUTER_API_KEY")`, `enabled = false`, 1)
	rt, err = Load([]byte(disabled), "disabled.hcl")
	if err != nil {
		t.Fatal(err)
	}
	if len(rt.Auth.Clients["chat-app"].AllowedModelPassthroughs) != 1 || rt.Catalog.ProviderCount() != 0 {
		t.Fatal("grant to disabled provider changed declaration or activated provider")
	}
	p = rt.Catalog.DisabledProviders()[0]
	if len(p.ModelPassthroughs) != 2 {
		t.Fatal("disabled provider was not validated or retained")
	}
	if _, match := p.MatchModelPassthrough("qwen/qwen3-72b"); match != ModelPassthroughNoMatch {
		t.Fatal("disabled provider rule was activated")
	}
	for _, tc := range []struct{ source, want string }{
		{strings.Replace(disabled, `^qwen/qwen3-[0-9]+b(-[a-z0-9-]+)?$`, "*", 1), "invalid match regexp"},
		{strings.Replace(passthroughSample, passthroughSampleLiteral, "", 1) + child, "at least one literal model"},
		{passthroughSample + strings.Replace(child, `api_key = "test-child-key"`, "", 1), "local credential"},
		{passthroughSample + strings.Replace(child, `api_key = "test-child-key"`, `api_key = "test-child-key"
  model_passthrough "family" {
    match = "x"
    capabilities = ["chat"]
  }`, 1), "derived provider cannot declare model_passthrough"},
		{passthroughSample + strings.Replace(child, `api_key = "test-child-key"`, `api_key = "test-child-key"
  model "qwen3-chat" {
    encrypted_reasoning {}
  }`, 1), "existing inherited model"},
	} {
		if _, err := Load([]byte(tc.source), "invalid.hcl"); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("load error = %v, want %q", err, tc.want)
		}
	}
	for _, grant := range []string{`[""]`, `["router"]`, `["router/qwen/qwen3-32b"]`, `["router/missing"]`, `["db/qwen3-chat"]`,
		`["child/qwen3-chat"]`, `["Router/qwen3-chat"]`, `["router/qwen3-chat "]`, `["alias/qwen3-chat"]`, `null`, `[null]`, `[true]`, `"router/qwen3-chat"`} {
		src := strings.Replace(passthroughSample, `["router/qwen3-chat"]`, grant, 1) + child
		if _, err := Load([]byte(src), "grant.hcl"); err == nil || !strings.Contains(err.Error(), "allowed_model_passthroughs") {
			t.Fatalf("invalid grant %s: %v", grant, err)
		}
		if _, _, _, err := Convert([]byte(src), "grant.hcl", false); err == nil {
			t.Fatalf("converted invalid grant %s", grant)
		}
	}
	for _, target := range []string{"qwen/qwen3-72b", "qwen3-chat"} {
		src := passthroughSample + fmt.Sprintf(`
alias "routing" {
  algorithm = "round_robin"
  providers = ["router"]
  model = %q
}`, target)
		if _, err := Load([]byte(src), "alias.hcl"); err == nil || !strings.Contains(err.Error(), "target model") {
			t.Fatalf("nonliteral alias target %q: %v", target, err)
		}
	}
}

func TestModelPassthroughDoesNotSatisfyDeferredLiteralChatRequirement(t *testing.T) {
	t.Setenv("ROUTER_API_KEY", "test-router-key")
	t.Setenv("CHAT_APP_TOKEN", "test-chat-token")
	for _, block := range []string{"deferred_transport {}", "deferred_receiver {}"} {
		t.Run(block, func(t *testing.T) {
			src := "deferred {}\n" + strings.Replace(passthroughSample, passthroughSampleLiteral, passthroughSampleLiteral+"  "+block+"\n", 1)
			if _, err := Load([]byte(src), "mixed.hcl"); err != nil {
				t.Fatal(err)
			}
			if _, err := Load([]byte(strings.Replace(src, passthroughSampleLiteral, "", 1)), "only-rules.hcl"); err == nil || !strings.Contains(err.Error(), "chat-capable model") {
				t.Fatalf("rule satisfied deferred literal eligibility: %v", err)
			}
		})
	}
}
