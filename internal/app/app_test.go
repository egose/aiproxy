package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/accounting"
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

func rewriteConfigFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
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

func TestReloadSwapsRuntimeWithoutRestart(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("initial readyz status = %d", w.Code)
	}

	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = ""
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload app: %v", err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/readyz", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("reloaded readyz status = %d, want 503", w.Code)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("models status = %d", w.Code)
	}
	if strings.Contains(w.Body.String(), "openai/gpt-4o-mini") {
		t.Fatalf("models should not include disabled provider after reload: %s", w.Body.String())
	}
}

func TestReloadRejectsListenerShapeChange(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":12345" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err == nil {
		t.Fatal("expected reload to reject listener change")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("readyz status after failed reload = %d, want 200", w.Code)
	}
}

func TestBuildWiresUsageAggregator(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[]}`))
	}))
	defer upstream.Close()

	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" {
  mode = "bearer_static"
  client "ci" {
    token = "tok"
    tenant = "team-a"
    allowed_models = ["openai/gpt-4o-mini"]
  }
}
provider "openai" "openai" {
  base_url = "`+upstream.URL+`"
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`))
	r.Header.Set("Authorization", "Bearer tok")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	summaries := a.usage.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	if summaries[0] != (accounting.Summary{Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200, Count: 1}) {
		t.Fatalf("summary = %+v", summaries[0])
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/v1/billing/usage", nil)
	r.Header.Set("Authorization", "Bearer tok")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("billing usage status = %d, body=%s", w.Code, w.Body.String())
	}
}
