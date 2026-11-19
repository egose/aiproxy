package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func loadDocumentedModelPassthrough(t *testing.T, stem, format string) *Runtime {
	t.Helper()
	doc, err := os.ReadFile(filepath.Join("..", "..", "website", "docs", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	name := stem + "." + format
	fence := "```" + format + " title=\"" + name + "\"\n"
	if strings.Count(string(doc), fence) != 1 {
		t.Fatalf("expected one documented %s configuration", name)
	}
	_, rest, _ := strings.Cut(string(doc), fence)
	source, _, ok := strings.Cut(rest, "\n```")
	if !ok {
		t.Fatalf("unterminated documented %s configuration", name)
	}
	rt, err := Load([]byte(source+"\n"), name)
	if err != nil {
		t.Fatalf("documented %s does not validate: %v", name, err)
	}
	return rt
}

func TestDocumentedModelPassthroughPublishedExamples(t *testing.T) {
	t.Setenv("ROUTER_API_KEY", "documentation-router-key")
	t.Setenv("CHAT_APP_TOKEN", "documentation-chat-token")
	for _, format := range []string{"hcl", "json"} {
		t.Run(format, func(t *testing.T) {
			rt := loadDocumentedModelPassthrough(t, "model-id-passthrough", format)
			assertPassthroughSample(t, rt, true)
			p, _ := rt.Catalog.Provider("router")
			if rt.RequiresDB() || rt.Auth.Mode != AuthModeBearerStatic ||
				p.BaseURL != "https://router.example/v1" || p.APIKey != "documentation-router-key" ||
				rt.Auth.Clients["chat-app"].Token != "documentation-chat-token" {
				t.Fatal("documented deployment settings or credentials drifted")
			}
			for _, suffix := range []string{"qwen3-chat", "embeddings", "qwen/qwen3-72b:free", "prefix/qwen/qwen3-72b"} {
				if _, match := p.MatchModelPassthrough(suffix); match != ModelPassthroughNoMatch {
					t.Errorf("documented rules unexpectedly match %q", suffix)
				}
			}
		})
	}
}

func TestDocumentedModelPassthroughProtocolExamples(t *testing.T) {
	t.Setenv("OPENCODE_ZEN_API_KEY", "documentation-zen-key")
	t.Setenv("GEMINI_API_KEY", "documentation-gemini-key")
	t.Setenv("CHAT_APP_TOKEN", "documentation-chat-token")
	for _, format := range []string{"hcl", "json"} {
		t.Run(format, func(t *testing.T) {
			rt := loadDocumentedModelPassthrough(t, "model-id-passthrough-protocols", format)
			client := rt.Auth.Clients["chat-app"]
			if rt.RequiresDB() || rt.Auth.Mode != AuthModeBearerStatic ||
				client.Token != "documentation-chat-token" || len(client.AllowedModels) != 0 ||
				!slices.Equal(client.AllowedModelPassthroughs, []string{"zen/claude-family", "google/embeddings"}) {
				t.Fatalf("documented auth or rule grants drifted: %v", client.AllowedModelPassthroughs)
			}
			zen := testProvider(t, rt, "zen")
			google := testProvider(t, rt, "google")
			if zen.Type != ProviderTypeOpenCodeZen || zen.APIKey != "documentation-zen-key" ||
				google.Type != ProviderTypeGemini || google.APIKey != "documentation-gemini-key" {
				t.Fatal("documented provider types or credentials drifted")
			}
			if len(zen.ModelPassthroughs) != 2 || len(google.ModelPassthroughs) != 1 ||
				len(zen.Models)+len(google.Models)+len(zen.ModelByName)+len(google.ModelByName) != 0 ||
				len(rt.Catalog.Aliases()) != 0 {
				t.Fatal("documented rule-only inventory drifted")
			}
			for _, tc := range []struct {
				provider, suffix, name string
				protocol               ModelProtocol
				capabilities           []Capability
			}{
				{"zen", "claude-sonnet-4-6", "claude-family", ModelProtocolMessages, []Capability{CapabilityChat, CapabilityResponses, CapabilityMessages}},
				{"zen", "gemini-2.5-flash", "gemini-family", ModelProtocolGemini, []Capability{CapabilityChat, CapabilityResponses}},
				{"google", "gemini-embedding-001", "embeddings", "", []Capability{CapabilityEmbeddings}},
			} {
				_, rule, match := rt.Catalog.MatchModelPassthrough(tc.provider, tc.suffix, nil)
				if match != ModelPassthroughMatched || rule.Name() != tc.name || rule.Protocol() != tc.protocol ||
					!slices.Equal(rule.Capabilities(), tc.capabilities) {
					t.Errorf("%s/%s = rule %q, protocol %q, capabilities %v, match %v", tc.provider, tc.suffix, rule.Name(), rule.Protocol(), rule.Capabilities(), match)
				}
			}
			for _, tc := range []struct{ provider, suffix string }{
				{"zen", "claude-family"},
				{"zen", "gemini-family"},
				{"zen", "models/gemini-2.5-flash"},
				{"google", "models/gemini-embedding-001"},
				{"google", "gemini-2.5-flash"},
			} {
				if _, _, match := rt.Catalog.MatchModelPassthrough(tc.provider, tc.suffix, nil); match != ModelPassthroughNoMatch {
					t.Errorf("documented rules unexpectedly match %s/%s", tc.provider, tc.suffix)
				}
			}
		})
	}
}
