package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

func writeConfigFile(t *testing.T, contents string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.hcl")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return path
}

func TestStartupSummaryIncludesEnabledSkippedAndAliases(t *testing.T) {
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

	summary := observability.StartupSummary(rt)
	for _, want := range []string{
		"enabled providers: 1",
		"openai (openai)",
		"skipped providers: 1",
		"localai (openai-compatible)",
		"reason=\"empty api key\"",
		"aliases: 1",
		"chat_default",
		"openai/gpt-4o-mini",
	} {
		if !strings.Contains(summary, want) {
			t.Fatalf("startup summary missing %q\n%s", want, summary)
		}
	}
}

func TestBuildWiresReadyzToProviderAvailability(t *testing.T) {
	activePath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	inactivePath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = ""
  model "gpt-4o-mini" {}
}
`)

	activeApp, err := Build(context.Background(), BuildOptions{ConfigPath: activePath, Version: "test"})
	if err != nil {
		t.Fatalf("build active app: %v", err)
	}
	activeW := httptest.NewRecorder()
	activeR := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	activeApp.Server.Handler.ServeHTTP(activeW, activeR)
	if activeW.Code != http.StatusOK {
		t.Fatalf("active readyz status = %d, want 200", activeW.Code)
	}

	inactiveApp, err := Build(context.Background(), BuildOptions{ConfigPath: inactivePath, Version: "test"})
	if err != nil {
		t.Fatalf("build inactive app: %v", err)
	}
	inactiveW := httptest.NewRecorder()
	inactiveR := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	inactiveApp.Server.Handler.ServeHTTP(inactiveW, inactiveR)
	if inactiveW.Code != http.StatusServiceUnavailable {
		t.Fatalf("inactive readyz status = %d, want 503", inactiveW.Code)
	}
}
