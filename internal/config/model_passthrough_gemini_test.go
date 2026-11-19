package config

import (
	"fmt"
	"strings"
	"testing"
)

func TestModelPassthroughGeminiBaseURL(t *testing.T) {
	const geminiRuleHCL = `
  model_passthrough "path" {
    match = ".*"
    capabilities = ["chat", "responses", "embeddings"]
  }`
	const geminiRuleJSON = `,"model_passthrough":{"path":{"match":".*","capabilities":["chat","responses","embeddings"]}}`
	const zenLiteralHCL = `model "pinned" { protocol = "gemini" }`
	const zenLiteralJSON = `,"model":{"pinned":{"protocol":"gemini"}}`
	const zenBodyHCL = `
  model_passthrough "body" {
    match = "^org/.+$"
    protocol = "chat"
    capabilities = ["chat"]
  }`
	const zenBodyJSON = `"body":{"match":"^org/.+$","protocol":"chat","capabilities":["chat"]}`
	const zenPathHCL = `
  model_passthrough "path" {
    match = "^gemini-.+$"
    protocol = "gemini"
    capabilities = ["chat", "responses"]
  }`
	const zenPathJSON = `"path":{"match":"^gemini-.+$","protocol":"gemini","capabilities":["chat","responses"]}`
	for _, tc := range []struct {
		name      string
		typ       ProviderType
		hcl, json string
		pathRule  bool
		enabled   bool
	}{
		{"gemini-rule-only", ProviderTypeGemini, geminiRuleHCL, geminiRuleJSON, true, true},
		{"gemini-disabled", ProviderTypeGemini, geminiRuleHCL, geminiRuleJSON, true, false},
		{"gemini-literal-only", ProviderTypeGemini, `model "pinned" {}`, `,"model":{"pinned":{}}`, false, true},
		{"zen-mixed-rules", ProviderTypeOpenCodeZen, zenBodyHCL + zenPathHCL, `,"model_passthrough":{` + zenBodyJSON + `,` + zenPathJSON + `}`, true, true},
		{"zen-disabled", ProviderTypeOpenCodeZen, zenPathHCL, `,"model_passthrough":{` + zenPathJSON + `}`, true, false},
		{"zen-literal-only", ProviderTypeOpenCodeZen, zenLiteralHCL, zenLiteralJSON, false, true},
		{"zen-body-rule-only-with-gemini-literal", ProviderTypeOpenCodeZen, zenLiteralHCL + zenBodyHCL, zenLiteralJSON + `,"model_passthrough":{` + zenBodyJSON + `}`, false, true},
	} {
		for _, suffix := range []string{"", "%3Fpart%23tag/", "?key=fixture", "?", "#tag", "#"} {
			t.Run(tc.name+"/"+suffix, func(t *testing.T) {
				base := "https://fixture.example/prefix/" + suffix
				for _, file := range []struct{ name, source string }{
					{"base-url.hcl", passthroughTestPrefix + fmt.Sprintf(`
provider %q "fixture" {
  api_key = "test-key"
  base_url = %q
  enabled = %t
  %s
}`, tc.typ, base, tc.enabled, tc.hcl)},
					{"base-url.json", fmt.Sprintf(`{"listener":{"http":{"public":{"address":":8080"}}},"auth":{"main":{"mode":"none"}},
"provider":{%q:{"fixture":{"api_key":"test-key","base_url":%q,"enabled":%t%s}}}}`, tc.typ, base, tc.enabled, tc.json)},
				} {
					src := []byte(file.source)
					_, loadErr := Load(src, file.name)
					editErr := ValidateDeferredConfig(src, file.name)
					converted, _, _, convertErr := Convert(src, file.name, false)
					for _, check := range []struct {
						name string
						err  error
					}{{"load", loadErr}, {"edit validation", editErr}, {"convert", convertErr}} {
						if tc.pathRule && strings.ContainsAny(suffix, "?#") {
							if check.err == nil || !strings.Contains(check.err.Error(), "base_url must not include a query or fragment for path-based model_passthrough rules") {
								t.Errorf("%s %s accepted or misdiagnosed a path rule at %q: %v", file.name, check.name, base, check.err)
							}
						} else if check.err != nil {
							t.Errorf("%s %s rejected %q: %v", file.name, check.name, base, check.err)
						}
					}
					if convertErr == nil {
						rt, err := Load(converted, "converted")
						if err != nil {
							t.Fatal(err)
						}
						if tc.enabled {
							p := testProvider(t, rt, "fixture")
							if p.BaseURL != base {
								t.Fatalf("conversion changed base_url: %q, want %q", p.BaseURL, base)
							}
						}
					}
				}
			})
		}
	}
}

func TestModelPassthroughGeminiUnsupportedCapabilities(t *testing.T) {
	for _, tc := range []struct {
		typ         ProviderType
		protocol    string
		unsupported []Capability
	}{
		{ProviderTypeGemini, "", []Capability{CapabilityMessages, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech}},
		{ProviderTypeOpenCodeZen, `protocol = "gemini"`, []Capability{CapabilityMessages, CapabilityEmbeddings, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech}},
	} {
		for _, capability := range tc.unsupported {
			t.Run(string(tc.typ)+"/"+string(capability), func(t *testing.T) {
				src := passthroughTestPrefix + fmt.Sprintf(`
provider %q "fixture" {
  enabled = false
  model_passthrough "path" {
    match = ".*"
    %s
    capabilities = ["chat", %q]
  }
}`, tc.typ, tc.protocol, capability)
				if _, err := Load([]byte(src), "capabilities.hcl"); err == nil ||
					!strings.Contains(err.Error(), fmt.Sprintf(`model_passthrough "path": capability %q is not`, capability)) {
					t.Fatalf("unexpected validation for unsupported capability: %v", err)
				}
			})
		}
	}
}
