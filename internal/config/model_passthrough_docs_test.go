package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocumentedModelPassthroughPublishedExamples(t *testing.T) {
	t.Setenv("ROUTER_API_KEY", "documentation-router-key")
	t.Setenv("CHAT_APP_TOKEN", "documentation-chat-token")
	doc, err := os.ReadFile(filepath.Join("..", "..", "website", "docs", "configuration.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []string{"hcl", "json"} {
		t.Run(format, func(t *testing.T) {
			name := "model-id-passthrough." + format
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
