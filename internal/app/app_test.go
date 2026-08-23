package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
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

type countingHealthBackend struct {
	closes   atomic.Int32
	closeErr error
}

func (b *countingHealthBackend) MarkSuccess(context.Context, string) error { return nil }

func (b *countingHealthBackend) MarkFailure(context.Context, string, time.Duration) error { return nil }

func (b *countingHealthBackend) IsHealthy(context.Context, string) (bool, error) { return true, nil }

func (b *countingHealthBackend) Snapshot(context.Context, []string) (map[string]bool, error) {
	return nil, nil
}

func (b *countingHealthBackend) Close() error {
	b.closes.Add(1)
	return b.closeErr
}

type failingListener struct {
	closeCalls atomic.Int32
	acceptErr  error
}

func (l *failingListener) Accept() (net.Conn, error) { return nil, l.acceptErr }

func (l *failingListener) Close() error {
	l.closeCalls.Add(1)
	return nil
}

func (l *failingListener) Addr() net.Addr { return &net.TCPAddr{IP: net.ParseIP("127.0.0.1")} }

func newLifecycleTestApp(server *http.Server, backend *countingHealthBackend) *App {
	if server == nil {
		server = &http.Server{Addr: "127.0.0.1:0", Handler: http.NewServeMux()}
	}
	if server.Addr == "" {
		server.Addr = "127.0.0.1:0"
	}
	return &App{
		Config:  &config.Runtime{},
		Server:  server,
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		clients: newUpstreamClientPool(),
		health:  providerhealth.NewWithBackend(nil, config.ProviderHealth{}, backend),
	}
}

func rewriteConfigFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("rewrite config: %v", err)
	}
}

func TestStartupSummaryIncludesEnabledSkippedAndAliases(t *testing.T) {
	rt := &config.Runtime{
		Catalog: config.NewCatalog([]config.Provider{{
			Type:        config.ProviderTypeOpenAI,
			Name:        "openai",
			DisplayName: "OpenAI",
			Models:      []config.Model{{Name: "gpt-4o-mini", Capabilities: []config.Capability{config.CapabilityChat}}},
		}}, []config.Provider{{
			Type:        config.ProviderTypeOpenAICompatible,
			Name:        "localai",
			DisplayName: "LocalAI",
			Models:      []config.Model{{Name: "qwen3-32b"}},
		}}, []config.Alias{{
			Name:      "chat_default",
			Algorithm: config.AlgorithmRoundRobin,
			Targets:   []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}},
		}}),
	}

	summary := observability.StartupSummary(rt)
	for _, want := range []string{
		"enabled providers: 1",
		"openai (openai)",
		"skipped providers: 1",
		"localai (openai-compatible)",
		"reason=\"disabled\"",
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
  enabled = false
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
  enabled = false
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

func TestReloadAddsChangesRemovesDerivedProviderAndRollsBackInvalidCandidate(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		mu.Lock()
		calls = append(calls, r.Header.Get("Authorization")+" "+string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[]}`))
	}))
	defer upstream.Close()
	baseConfig := func(child string) string {
		return `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai-compatible" "base" {
  base_url = "` + upstream.URL + `/v1"
  api_key = "sk-base"
  model "glm" {
    upstream_name = "upstream-glm"
  }
}
` + child
	}
	childConfig := func(key string) string {
		return `provider "openai-compatible" "derived" {
  extends = "base"
  api_key = "` + key + `"
}
`
	}
	var a *App
	postDerived := func(wantStatus int, wantKey string) {
		t.Helper()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"derived/glm","messages":[]}`))
		a.Server.Handler.ServeHTTP(w, r)
		if w.Code != wantStatus {
			t.Fatalf("derived status = %d, want %d, body=%s", w.Code, wantStatus, w.Body.String())
		}
		if wantKey == "" {
			return
		}
		mu.Lock()
		last := calls[len(calls)-1]
		mu.Unlock()
		if !strings.Contains(last, "Bearer "+wantKey) || !strings.Contains(last, `"model":"upstream-glm"`) {
			t.Fatalf("last derived request = %s", last)
		}
	}

	configPath := writeConfigFile(t, baseConfig(""))
	var err error
	a, err = Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	rewriteConfigFile(t, configPath, baseConfig(childConfig("sk-derived-a")))
	if err := a.Reload(); err != nil {
		t.Fatalf("reload add derived: %v", err)
	}
	postDerived(http.StatusOK, "sk-derived-a")

	rewriteConfigFile(t, configPath, baseConfig(childConfig("sk-derived-b")))
	if err := a.Reload(); err != nil {
		t.Fatalf("reload change derived: %v", err)
	}
	postDerived(http.StatusOK, "sk-derived-b")

	rewriteConfigFile(t, configPath, baseConfig(""))
	if err := a.Reload(); err != nil {
		t.Fatalf("reload remove derived: %v", err)
	}
	postDerived(http.StatusNotFound, "")

	rewriteConfigFile(t, configPath, baseConfig(childConfig("sk-derived-c")))
	if err := a.Reload(); err != nil {
		t.Fatalf("reload re-add derived: %v", err)
	}
	postDerived(http.StatusOK, "sk-derived-c")

	rewriteConfigFile(t, configPath, baseConfig(`provider "openai-compatible" "derived" {
  extends = "base"
  base_url = "https://invalid.example/v1"
  api_key = "sk-invalid"
}
`))
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "derived provider cannot declare base_url") {
		t.Fatalf("invalid reload error = %v", err)
	}
	postDerived(http.StatusOK, "sk-derived-c")
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

func TestReloadPreservesUnchangedAliasLeastConnectionsLease(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "a" {
  api_key = "sk-test"
  model "m" {}
}
provider "openai" "b" {
  api_key = "sk-test"
  model "m" {}
}
alias "chat_default" {
  algorithm = "least_connections"
  target {
    provider = "a"
    model = "m"
  }
  target {
    provider = "b"
    model = "m"
  }
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	res, err := a.resolver.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve initial alias: %v", err)
	}
	first, releaseFirst := res.Selector.Acquire(nil)
	if first.Provider != "a" {
		t.Fatalf("first target = %+v, want provider a", first)
	}

	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "a" {
  api_key = "sk-test"
  model "m" {}
}
provider "openai" "b" {
  api_key = "sk-test"
  model "m" {}
}
alias "chat_default" {
  algorithm = "least_connections"
  target {
    provider = "a"
    model = "m"
  }
  target {
    provider = "b"
    model = "m"
  }
}
`)
	if err := a.Reload(); err != nil {
		t.Fatalf("reload app: %v", err)
	}
	res, err = a.resolver.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve reloaded alias: %v", err)
	}
	second, releaseSecond := res.Selector.Acquire(nil)
	if second.Provider != "b" {
		t.Fatalf("target while pre-reload lease is held = %+v, want provider b", second)
	}
	releaseFirst()
	third, releaseThird := res.Selector.Acquire(nil)
	defer releaseThird()
	if third.Provider != "a" {
		t.Fatalf("target after pre-reload lease release = %+v, want provider a", third)
	}
	releaseSecond()
}

func TestFailedReloadLeavesAliasSelectorAndCatalogUnchanged(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai" "a" {
  api_key = "sk-test"
  model "m" {}
}
provider "openai" "b" {
  api_key = "sk-test"
  model "m" {}
}
alias "chat_default" {
  algorithm = "round_robin"
  target {
    provider = "a"
    model = "m"
  }
  target {
    provider = "b"
    model = "m"
  }
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	res, err := a.resolver.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve initial alias: %v", err)
	}
	first, release := res.Selector.Acquire(nil)
	if first.Provider != "a" {
		t.Fatalf("first target = %+v, want provider a", first)
	}
	release()

	rewriteConfigFile(t, configPath, `
listener "http" "public" { address = ":1" }
auth "main" { mode = "none" }
provider "openai" "a" {
  api_key = "sk-test"
  model "m" {}
}
provider "openai" "b" {
  api_key = "sk-test"
  model "m" {}
}
provider "openai" "c" {
  api_key = "sk-test"
  model "m" {}
}
alias "chat_default" {
  algorithm = "round_robin"
  target {
    provider = "c"
    model = "m"
  }
  target {
    provider = "a"
    model = "m"
  }
}
`)
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "listener changes require restart") {
		t.Fatalf("reload error = %v", err)
	}
	if _, ok := a.Config.Catalog.Provider("c"); ok {
		t.Fatal("failed reload published candidate provider")
	}
	res, err = a.resolver.Resolve("alias/chat_default")
	if err != nil {
		t.Fatalf("resolve after failed reload: %v", err)
	}
	next, release := res.Selector.Acquire(nil)
	defer release()
	if next.Provider != "b" {
		t.Fatalf("selector state after failed reload = %+v, want provider b", next)
	}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("models status = %d, body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "c/m") {
		t.Fatalf("failed reload published candidate catalog: %s", w.Body.String())
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
	Health map[string]bool      `json:"health"`
	Usage  []accounting.Summary `json:"usage"`
}

func TestDerivedProviderIdentityAcrossRuntimeSurfaces(t *testing.T) {
	var mu sync.Mutex
	var authHeaders []string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		mu.Lock()
		authHeaders = append(authHeaders, r.Header.Get("Authorization")+" "+string(body))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl_test","object":"chat.completion","choices":[]}`))
	}))
	defer upstream.Close()

	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
metrics { token = "scrape-secret" }
dashboard { token = "dash-secret" }
provider "openai-compatible" "base" {
  base_url = "`+upstream.URL+`/v1"
  api_key = "sk-base"
  model "glm" {
    upstream_name = "upstream-glm"
  }
}
provider "openai-compatible" "derived" {
  extends = "base"
  api_key = "sk-derived"
}
alias "chat" {
  algorithm = "round_robin"
  target {
    provider = "derived"
    model = "glm"
  }
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}

	for _, model := range []string{"derived/glm", "alias/chat"} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`))
		a.Server.Handler.ServeHTTP(w, r)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body=%s", model, w.Code, w.Body.String())
		}
	}
	mu.Lock()
	calls := append([]string(nil), authHeaders...)
	mu.Unlock()
	if len(calls) != 2 {
		t.Fatalf("upstream calls = %+v", calls)
	}
	for _, call := range calls {
		if !strings.Contains(call, "Bearer sk-derived") || !strings.Contains(call, `"model":"upstream-glm"`) {
			t.Fatalf("derived request did not use local credential and inherited model mapping: %s", call)
		}
	}
	a.health.MarkFailure("derived")

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.Header.Set("Authorization", "Bearer scrape-secret")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", w.Code)
	}
	metricsBody := w.Body.String()
	for _, want := range []string{
		`aiproxy_provider_selections_total{model="glm",operation="chat_completions",provider="derived",public_model="derived/glm"} 1`,
		`aiproxy_provider_selections_total{model="glm",operation="chat_completions",provider="derived",public_model="alias/chat"} 1`,
		`aiproxy_upstream_requests_total{operation="chat_completions",outcome="success",provider="derived"} 2`,
		`aiproxy_provider_healthy{name="base"} 1`,
		`aiproxy_provider_healthy{name="derived"} 0`,
	} {
		if !strings.Contains(metricsBody, want) {
			t.Fatalf("metrics missing %q\n%s", want, metricsBody)
		}
	}
	summaries := a.usage.Summaries()
	if !summaryExists(summaries, "derived/glm") || !summaryExists(summaries, "alias/chat") {
		t.Fatalf("usage summaries = %+v", summaries)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/_internal/dashboard/snapshot", nil)
	r.Header.Set("Authorization", "Bearer dash-secret")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard status = %d, body=%s", w.Code, w.Body.String())
	}
	var snap dashboardSnapshotJSON
	if err := json.Unmarshal(w.Body.Bytes(), &snap); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if !providerListed(snap.Providers, "derived") || snap.Health["base"] != true || snap.Health["derived"] != false || !summaryExists(snap.Usage, "derived/glm") {
		t.Fatalf("dashboard snapshot = %+v", snap)
	}
}

func summaryExists(summaries []accounting.Summary, model string) bool {
	for _, summary := range summaries {
		if summary.Model == model {
			return true
		}
	}
	return false
}

func providerListed(providers []struct {
	Name string `json:"name"`
}, name string) bool {
	for _, provider := range providers {
		if provider.Name == name {
			return true
		}
	}
	return false
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

func TestMetricsEndpointRequiresConfiguredToken(t *testing.T) {
	configPath := writeConfigFile(t, `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
metrics { token = "scrape-secret" }
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
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("metrics without token status = %d, want 401", w.Code)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.Header.Set("Authorization", "Bearer wrong")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("metrics with wrong token status = %d, want 401", w.Code)
	}

	w = httptest.NewRecorder()
	r = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	r.Header.Set("Authorization", "Bearer scrape-secret")
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics with correct token status = %d, want 200", w.Code)
	}
}

func TestMetricsEndpointDisabledWithoutMetricsBlock(t *testing.T) {
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
	r := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("metrics without metrics block status = %d, want 404", w.Code)
	}
}

func TestRunReadyClosesResourcesOnListenerFailure(t *testing.T) {
	backend := &countingHealthBackend{}
	a := newLifecycleTestApp(&http.Server{Addr: "127.0.0.1:not-a-port", Handler: http.NewServeMux()}, backend)
	if err := a.RunReady(context.Background(), nil); err == nil {
		t.Fatal("expected listener error")
	}
	assertHealthClosedOnce(t, backend)
	if err := a.Close(); err != nil {
		t.Fatalf("repeat close: %v", err)
	}
	assertHealthClosedOnce(t, backend)
}

func TestRunReadyClosesResourcesOnDashboardTokenPersistenceFailure(t *testing.T) {
	xdgFile := filepath.Join(t.TempDir(), "xdg-file")
	if err := os.WriteFile(xdgFile, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write xdg file: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", xdgFile)
	configPath := writeConfigFile(t, `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
dashboard {}
provider "openai" "openai" {
  api_key = "sk-test"
  model "gpt-4o-mini" {}
}
`)
	a, err := Build(context.Background(), BuildOptions{ConfigPath: configPath, Version: "test"})
	if err != nil {
		t.Fatalf("build app: %v", err)
	}
	backend := &countingHealthBackend{}
	a.health = providerhealth.NewWithBackend(nil, config.ProviderHealth{}, backend)
	if err := a.RunReady(context.Background(), nil); err == nil || !strings.Contains(err.Error(), "dashboard token") {
		t.Fatalf("RunReady error = %v", err)
	}
	assertHealthClosedOnce(t, backend)
}

func TestRunReadyClosesResourcesOnReadinessCallbackFailure(t *testing.T) {
	readyErr := errors.New("ready failed")
	closeErr := errors.New("close failed")
	backend := &countingHealthBackend{closeErr: closeErr}
	a := newLifecycleTestApp(nil, backend)
	err := a.RunReady(context.Background(), func() error { return readyErr })
	if !errors.Is(err, readyErr) {
		t.Fatalf("RunReady error should include readiness error, got %v", err)
	}
	if !errors.Is(err, closeErr) {
		t.Fatalf("RunReady error should include close error, got %v", err)
	}
	assertHealthClosedOnce(t, backend)
}

func TestRunReadyClosesResourcesOnUnexpectedServeFailure(t *testing.T) {
	serveErr := errors.New("serve failed")
	backend := &countingHealthBackend{}
	listener := &failingListener{acceptErr: serveErr}
	a := newLifecycleTestApp(nil, backend)
	a.listen = func(network, address string) (net.Listener, error) { return listener, nil }
	err := a.RunReady(context.Background(), nil)
	if !errors.Is(err, serveErr) {
		t.Fatalf("RunReady error = %v, want serve failure", err)
	}
	assertHealthClosedOnce(t, backend)
}

func TestRunReadyClosesResourcesOnShutdownFailure(t *testing.T) {
	requestStarted := make(chan struct{})
	releaseRequest := make(chan struct{})
	var closeStarted sync.Once
	server := &http.Server{Addr: "127.0.0.1:0", Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		closeStarted.Do(func() { close(requestStarted) })
		<-releaseRequest
	})}
	backend := &countingHealthBackend{}
	a := newLifecycleTestApp(server, backend)
	a.shutdownTimeout = 20 * time.Millisecond
	addrCh := make(chan string, 1)
	a.listen = func(network, address string) (net.Listener, error) {
		listener, err := net.Listen(network, address)
		if err == nil {
			addrCh <- listener.Addr().String()
		}
		return listener, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- a.RunReady(ctx, nil) }()
	addr := <-addrCh
	reqDone := make(chan struct{})
	go func() {
		resp, _ := http.Get("http://" + addr)
		if resp != nil {
			_ = resp.Body.Close()
		}
		close(reqDone)
	}()
	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("request did not reach server")
	}
	cancel()
	err := <-errCh
	close(releaseRequest)
	<-reqDone
	if err == nil || !strings.Contains(err.Error(), "server shutdown") {
		t.Fatalf("RunReady error = %v, want shutdown failure", err)
	}
	assertHealthClosedOnce(t, backend)
}

func TestRunReadyClosesResourcesOnContextCancellation(t *testing.T) {
	backend := &countingHealthBackend{}
	var logs bytes.Buffer
	a := newLifecycleTestApp(nil, backend)
	a.logger = slog.New(slog.NewTextHandler(&logs, nil))
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	errCh := make(chan error, 1)
	go func() {
		errCh <- a.RunReady(ctx, func() error {
			close(ready)
			return nil
		})
	}()
	<-ready
	cancel()
	if err := <-errCh; err != nil {
		t.Fatalf("RunReady error = %v", err)
	}
	assertHealthClosedOnce(t, backend)
	if !strings.Contains(logs.String(), "shutting down server") || !strings.Contains(logs.String(), "server stopped") {
		t.Fatalf("shutdown logs missing, got:\n%s", logs.String())
	}
}

func assertHealthClosedOnce(t *testing.T, backend *countingHealthBackend) {
	t.Helper()
	if got := backend.closes.Load(); got != 1 {
		t.Fatalf("health backend closes = %d, want 1", got)
	}
}
