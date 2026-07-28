package observability

import (
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestRequestIDUsesExistingOrGenerates(t *testing.T) {
	if got := RequestID("existing-id"); got != "existing-id" {
		t.Fatalf("RequestID(existing) = %q", got)
	}
	if got := RequestID(""); got == "" {
		t.Fatalf("RequestID(empty) returned empty string")
	}
}

func TestStartupSummaryIncludesKeySections(t *testing.T) {
	rt := &config.Runtime{
		Providers: []config.Provider{{
			Type:        config.ProviderTypeOpenAI,
			Name:        "openai",
			DisplayName: "OpenAI",
			Models:      []config.Model{{Name: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat}}},
			ModelByName: map[string]config.Model{"gpt-4o-mini": {Name: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat}}},
		}},
		DisabledProviders: []config.Provider{{
			Type:        config.ProviderTypeOpenAICompatible,
			Name:        "localai",
			DisplayName: "LocalAI",
			Models:      []config.Model{{Name: "qwen3-32b"}},
		}},
		Aliases: []config.Alias{{
			Name:      "chat_default",
			Algorithm: config.AlgorithmRoundRobin,
			Targets:   []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}},
		}},
		ProviderByName: map[string]config.Provider{
			"openai": {
				Type:        config.ProviderTypeOpenAI,
				Name:        "openai",
				DisplayName: "OpenAI",
				Models:      []config.Model{{Name: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat}}},
				ModelByName: map[string]config.Model{"gpt-4o-mini": {Name: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat}}},
			},
		},
	}

	summary := StartupSummary(rt)
	for _, want := range []string{
		"enabled providers: 1",
		"openai (openai)",
		"skipped providers: 1",
		"localai (openai-compatible)",
		"aliases: 1",
		"chat_default",
		"openai/gpt-4o-mini",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("summary missing %q\n%s", want, summary)
		}
	}
}
