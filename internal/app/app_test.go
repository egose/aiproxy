package app

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
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

func TestBuildDoesNotReplaceDefaultLogger(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	original := slog.Default()
	if _, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"}); err != nil {
		t.Fatalf("build app: %v", err)
	}
	if slog.Default() != original {
		t.Fatal("Build should not replace slog.Default")
	}
}

func TestBuildInstancesUseIsolatedLoggers(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var firstLogs bytes.Buffer
	var secondLogs bytes.Buffer
	first, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "one", LogOutput: &firstLogs})
	if err != nil {
		t.Fatalf("build first app: %v", err)
	}
	second, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "two", LogOutput: &secondLogs})
	if err != nil {
		t.Fatalf("build second app: %v", err)
	}

	first.Server.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if firstLogs.Len() == 0 {
		t.Fatal("first app should write to first logger")
	}
	if secondLogs.Len() != 0 {
		t.Fatalf("first app wrote to second logger:\n%s", secondLogs.String())
	}

	second.Server.Handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/v1/models", nil))
	if secondLogs.Len() == 0 {
		t.Fatal("second app should write to second logger")
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

func TestReloadPreservesProviderHealthStateWhenConfigIsUnchanged(t *testing.T) {
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
	a.health.MarkFailure("openai")

	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if a.health.IsHealthy("openai") {
		t.Fatal("provider health should be preserved across reload")
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("readyz status = %d, want 503", w.Code)
	}
}

func TestReloadPreservesRateLimiterWhenConfigIsUnchanged(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" {
  mode = "none"
  rate_limit {
    requests_per_minute = 60
    burst = 1
  }
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	if allowed, _ := a.rateLimiter.Allow("client-a"); !allowed {
		t.Fatal("first request should pass")
	}
	if allowed, _ := a.rateLimiter.Allow("client-a"); allowed {
		t.Fatal("second request should exhaust bucket")
	}

	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":0" }
auth "main" {
  mode = "none"
  rate_limit {
    requests_per_minute = 60
    burst = 1
  }
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if allowed, _ := a.rateLimiter.Allow("client-a"); allowed {
		t.Fatal("unchanged rate limit config should preserve exhausted bucket")
	}
}

func TestReloadResetsRateLimiterWhenConfigChanges(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" {
  mode = "none"
  rate_limit {
    requests_per_minute = 60
    burst = 1
  }
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	if allowed, _ := a.rateLimiter.Allow("client-a"); !allowed {
		t.Fatal("first request should pass")
	}

	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":0" }
auth "main" {
  mode = "none"
  rate_limit {
    requests_per_minute = 120
    burst = 2
  }
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload app: %v", err)
	}
	if allowed, _ := a.rateLimiter.Allow("client-a"); !allowed {
		t.Fatal("changed rate limit config should create a fresh bucket")
	}
}

func TestReloadRejectsLogLevelChange(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
logging { level = "info" }
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
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
logging { level = "debug" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "logging level changes require restart") {
		t.Fatalf("reload error = %v", err)
	}
}

func TestReloadRejectsDashboardEnablement(t *testing.T) {
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
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard { token = "sekret" }
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "enabling dashboard requires restart") {
		t.Fatalf("reload error = %v", err)
	}
}

func TestReloadUpdatesDashboardSnapshotEndpoint(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {
  token = "sekret"
}
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
	r := httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil)
	r.Header.Set("Authorization", "Bearer sekret")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot status = %d, body=%s", w.Code, w.Body.String())
	}

	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {
  token = "sekret"
}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
provider "anthropic" "anthropic" {
  api_key = "sk-test"
  model "claude-3-5-sonnet" {}
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload app: %v", err)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil)
	r.Header.Set("Authorization", "Bearer sekret")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("post-reload snapshot status = %d, body=%s", w.Code, w.Body.String())
	}
	var snap dashboardSnapshotJSON
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if len(snap.Providers) != 2 {
		t.Fatalf("expected 2 providers in reloaded snapshot, got %+v", snap.Providers)
	}
	if snap.Providers[1].Name != "anthropic" {
		t.Fatalf("expected anthropic as second provider, got %+v", snap.Providers[1])
	}
}

// dashboardSnapshotJSON is the JSON-shape of dashrpc.Snapshot for test unmarshal.
type dashboardSnapshotJSON struct {
	Providers []struct {
		Name string `json:"name"`
	} `json:"providers"`
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

func TestBuildHonorsAccessLogSetting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[]}`))
	}))
	defer upstream.Close()

	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
logging {
  access_log = false
}
provider "openai" "openai" {
  base_url = "`+upstream.URL+`"
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var logs bytes.Buffer
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: &logs})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`))
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(logs.String(), "request received") || strings.Contains(logs.String(), "response sent") {
		t.Fatalf("access logs should be disabled, got logs:\n%s", logs.String())
	}
}

func TestBuildHonorsLogLevelSetting(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[]}`))
	}))
	defer upstream.Close()

	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
logging {
  level = "error"
  access_log = true
}
provider "openai" "openai" {
  base_url = "`+upstream.URL+`"
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	var logs bytes.Buffer
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test", LogOutput: &logs})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`))
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", w.Code, w.Body.String())
	}
	if logs.Len() != 0 {
		t.Fatalf("expected no info logs at error level, got:\n%s", logs.String())
	}
}

func TestNewHTTPClientDoesNotSetWholeRequestTimeout(t *testing.T) {
	client := newHTTPClient(config.DefaultUpstreamHeaderTimeout)
	if client.Timeout != 0 {
		t.Fatalf("timeout = %v", client.Timeout)
	}
	transport, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport type = %T", client.Transport)
	}
	if transport.ResponseHeaderTimeout != 90*time.Second {
		t.Fatalf("response header timeout = %v", transport.ResponseHeaderTimeout)
	}
}

func TestUpstreamClientPoolReusesByTimeout(t *testing.T) {
	pool := newUpstreamClientPool()
	c1 := pool.Client(30 * time.Second)
	c2 := pool.Client(30 * time.Second)
	c3 := pool.Client(45 * time.Second)
	if c1 != c2 {
		t.Fatal("expected same timeout to reuse client")
	}
	if c1 == c3 {
		t.Fatal("expected distinct timeout to use distinct client")
	}
	if c1.Timeout != 0 || c3.Timeout != 0 {
		t.Fatalf("whole request timeout set: %v %v", c1.Timeout, c3.Timeout)
	}
}

func TestReloadHealthTrackerReusesExistingTrackerWhenConfigMatches(t *testing.T) {
	tracker := providerhealth.New(nil, config.ProviderHealth{Cooldown: 15 * time.Second})
	current := &config.Runtime{ProviderHealth: config.ProviderHealth{Cooldown: 15 * time.Second}}
	next := &config.Runtime{ProviderHealth: config.ProviderHealth{Cooldown: 15 * time.Second}}
	if got := reloadHealthTracker(tracker, observability.NewMetrics(), current, next); got != tracker {
		t.Fatal("expected tracker reuse")
	}
	changed := &config.Runtime{ProviderHealth: config.ProviderHealth{Cooldown: 30 * time.Second}}
	if got := reloadHealthTracker(tracker, observability.NewMetrics(), current, changed); got == tracker {
		t.Fatal("expected new tracker when config changes")
	}
}

func TestDashboardEndpointAbsentWithoutDashboardBlock(t *testing.T) {
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
	r := httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("snapshot endpoint should be 404 when dashboard block is absent, got %d", w.Code)
	}
}

func TestDashboardEndpointRejectsMissingToken(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
dashboard {
  token = "sekret"
}
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
	r := httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("snapshot endpoint without Authorization should be 401, got %d", w.Code)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil)
	r.Header.Set("Authorization", "Bearer sekret")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot endpoint with correct token should be 200, got %d (body=%s)", w.Code, w.Body.String())
	}
}
