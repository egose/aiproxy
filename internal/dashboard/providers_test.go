package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/app"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/httpapi"
)

func diagnosticHTTP(t *testing.T, handler http.Handler) (*model, dashrpc.Snapshot, string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, dashrpc.SnapshotPath, nil)
	req.Header.Set("Authorization", "Bearer dashboard-secret-fixture")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("snapshot status %d: %s", w.Code, w.Body.String())
	}
	var wire dashrpc.Snapshot
	if err := json.Unmarshal(w.Body.Bytes(), &wire); err != nil {
		t.Fatal(err)
	}
	m := InitialModel(SnapshotFromTransport(wire)).(*model)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m.focus = focusProviders
	m.connection = ConnectionStatus{LastSuccess: wire.Now}
	return m, wire, w.Body.String()
}

func TestProviderStoredProbeThroughAppHTTPAndTUI(t *testing.T) {
	for _, code := range []int{200, 503} {
		t.Run(fmt.Sprint(code), func(t *testing.T) {
			started, release := make(chan struct{}), make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				close(started)
				select {
				case <-release:
				case <-r.Context().Done():
					return
				}
				w.WriteHeader(code)
			}))
			defer upstream.Close()
			path := filepath.Join(t.TempDir(), "config.hcl")
			text := fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
dashboard { token = "dashboard-secret-fixture" }
provider "openai-compatible" "standalone" {
  display_name = "Standalone diagnostic"
  base_url = %q
  api_key = "upstream-secret-fixture"
  model "public" {
    upstream_name = "actual-model"
    capabilities = ["chat", "responses"]
  }
  healthcheck {
    path = "/health?token=query-secret-fixture"
    interval = "1h"
    timeout = "5s"
    failure_threshold = 1
    success_threshold = 1
  }
}
`, upstream.URL)
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			a, err := app.Build(t.Context(), app.BuildOptions{ConfigPath: path, LogOutput: io.Discard})
			if err != nil {
				t.Fatal(err)
			}
			defer a.Close()
			select {
			case <-started:
			case <-time.After(3 * time.Second):
				t.Fatal("probe did not start")
			}
			pending, _, _ := diagnosticHTTP(t, a.Server.Handler)
			layoutKey(pending, "enter")
			if got := strings.Join(pending.providerDetailLines(), "\n"); !strings.Contains(got, "pending (no completed check)") || !strings.Contains(got, "last checked: unknown") {
				t.Fatal(got)
			}
			before := time.Now()
			close(release)
			var m *model
			var wire dashrpc.Snapshot
			var raw string
			deadline := time.Now().Add(3 * time.Second)
			for {
				m, wire, raw = diagnosticHTTP(t, a.Server.Handler)
				if len(wire.Healthchecks) == 1 && wire.Healthchecks[0].Checked {
					break
				}
				if time.Now().After(deadline) {
					t.Fatal("probe did not complete")
				}
				time.Sleep(10 * time.Millisecond)
			}
			h := wire.Healthchecks[0]
			if h.LastChecked.Before(before) || h.LastChecked.After(wire.Now) || !m.snapshot.Healthchecks[0].LastChecked.Equal(h.LastChecked) {
				t.Fatalf("stored timestamp lost: %+v", h)
			}
			_, second, _ := diagnosticHTTP(t, a.Server.Handler)
			if !second.Healthchecks[0].LastChecked.Equal(h.LastChecked) {
				t.Fatal("snapshot fabricated new check time")
			}
			if len(wire.Aliases) != 0 {
				t.Fatal("standalone fixture has aliases")
			}
			m.now = h.LastChecked.Add(37 * time.Second)
			layoutKey(m, "enter")
			got := strings.Join(m.providerDetailLines(), "\n")
			for _, want := range []string{"Standalone diagnostic", "openai-compatible", "standalone/public", "actual-model", "chat, responses", "probe: GET /health", "last checked: 37s ago", fmt.Sprintf("last probe HTTP: %d", code)} {
				if !strings.Contains(got, want) {
					t.Fatalf("missing %q: %s", want, got)
				}
			}
			want := "reason: ok"
			if code == 503 {
				want = "reason: unexpected status 503 (want 200)"
			}
			if !strings.Contains(got, want) {
				t.Fatal(got)
			}
			for _, secret := range []string{"dashboard-secret-fixture", "upstream-secret-fixture", "query-secret-fixture"} {
				if strings.Contains(raw+got, secret) {
					t.Fatalf("secret escaped diagnostic boundary: %s", secret)
				}
			}
		})
	}
}

func TestProviderCatalogDiagnosticsSecretsAndAffinityThroughHTTP(t *testing.T) {
	p := config.Provider{Type: config.ProviderTypeOpenCodeZen, Name: "zen", DisplayName: "Zen", APIKey: "api-secret-fixture", CopilotToken: "copilot-secret-fixture",
		APIKeyRef:            &config.APIKeyRef{Path: "key-path-secret-fixture", Key: "key-name-secret-fixture"},
		CopilotCredentialRef: &config.CopilotCredentialRef{Path: "oauth-path-secret-fixture", Name: "oauth-name-secret-fixture"},
		BaseURL:              "https://user-secret-fixture:password-secret-fixture@example.test/v1/path-secret-fixture?key=query-secret-fixture#fragment-secret-fixture",
		Models:               []config.Model{{Name: "public", UpstreamName: "upstream", Protocol: config.ModelProtocol("messages"), Capabilities: []config.Capability{"chat", "responses"}}},
		Healthcheck:          &config.ProviderHealthcheck{Path: "/health?token=probe-secret-fixture", ExpectedBody: "body-secret-fixture", SendAuthorization: true}}
	aliases := []config.Alias{
		{Name: "custom", SessionAffinity: &config.SessionAffinity{Headers: []string{"x-custom-session"}}, Targets: []config.AliasTarget{{Provider: "zen", Model: "public"}, {Provider: "zen", Model: "other"}}},
		{Name: "defaults", SessionAffinity: &config.SessionAffinity{}}, {Name: "off"},
	}
	disabled := config.Provider{Name: "disabled", Type: config.ProviderTypeOpenAI, Models: []config.Model{{Name: "m"}}}
	noProbe := config.Provider{Name: "no-probe", Type: config.ProviderTypeOpenAI}
	catalog := config.NewCatalog([]config.Provider{p, noProbe}, []config.Provider{disabled}, aliases)
	u := accounting.NewAggregator()
	u.Record(accounting.Event{Provider: "zen", Model: "zen/public", StatusCode: 200, TotalTokens: 99})
	source := dashrpc.NewRuntimeSource(config.Dashboard{Enabled: true, Token: "dashboard-secret-fixture"}, "test", ":8080", "none", time.Now(), catalog, u, nil, nil)
	source.SetHealthcheckSource(func() []dashrpc.HealthcheckStatus {
		return []dashrpc.HealthcheckStatus{{Provider: "zen", Configured: true, Checked: true, Message: "Get https://user:reason-secret-fixture@example.test/?token=more-secret-fixture: connection refused", Path: p.Healthcheck.Path}}
	})
	handler := httpapi.NewHandler(httpapi.Dependencies{Dashboard: source, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	m, wire, raw := diagnosticHTTP(t, handler)
	layoutKey(m, "enter")
	got := strings.Join(m.providerDetailLines(), "\n")
	for _, want := range []string{"https://example.test/v1/", "upstream: upstream", "protocol: messages", "capabilities: chat, responses", "reason: connection refused"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q: %s", want, got)
		}
	}
	for _, secret := range []string{"api-secret", "copilot-secret", "key-path-secret", "key-name-secret", "oauth-path-secret", "oauth-name-secret", "user-secret", "password-secret", "path-secret", "query-secret", "fragment-secret", "probe-secret", "body-secret", "reason-secret", "more-secret", "dashboard-secret", "send_authorization", "api_key", "credential_ref"} {
		if strings.Contains(raw+got, secret) {
			t.Fatalf("secret escaped: %s", secret)
		}
	}
	for _, tc := range []struct{ name, want string }{{"no-probe", "probe: not configured"}, {"disabled", "probe status: inactive (provider disabled)"}} {
		m.providerDetailName = tc.name
		if got := strings.Join(m.providerDetailLines(), "\n"); !strings.Contains(got, tc.want) {
			t.Fatal(got)
		}
	}
	for i, a := range wire.Aliases {
		if a.SessionAffinity == nil {
			t.Fatal("missing affinity metadata")
		}
		if a.SessionAffinity.Enabled != (i != 2) {
			t.Fatalf("affinity %s: %+v", a.Name, a.SessionAffinity)
		}
		m.aliasDetailName = a.Name
		got := renderAliasDetail(m, 160, 45)
		want := "disabled"
		if i == 0 {
			want = "x-custom-session"
		} else if i == 1 {
			want = "x-opencode-session"
		}
		if !strings.Contains(got, "session_affinity: "+want) {
			t.Fatal(got)
		}
		if i == 0 && (!strings.Contains(got, "NOT target/alias counts") || strings.Count(got, "provider-wide lifetime reqs:1") != 2) {
			t.Fatal(got)
		}
	}
}

func TestProviderOldMetadataRemainsUnknown(t *testing.T) {
	var wire dashrpc.Snapshot
	if err := json.Unmarshal([]byte(`{"providers":[{"name":"old","type":"openai","models":[{"name":"m"}]}],"aliases":[{"name":"a","targets":[]}],"healthchecks":[{"provider":"old","configured":true,"checked":true,"healthy":true,"status_code":200}]}`), &wire); err != nil {
		t.Fatal(err)
	}
	m := InitialModel(SnapshotFromTransport(wire)).(*model)
	m.providerDetailName = "old"
	got := strings.Join(m.providerDetailLines(), "\n")
	for _, want := range []string{"endpoint (sanitized): unknown", "settings/probe configuration: unknown", "upstream/protocol/capabilities: unknown", "last checked: unknown", "routing health: unknown"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q: %s", want, got)
		}
	}
	m.aliasDetailName = "a"
	if got := renderAliasDetail(m, 120, 30); !strings.Contains(got, "session_affinity: unknown") {
		t.Fatal(got)
	}
	wire.Healthchecks = nil
	m = InitialModel(SnapshotFromTransport(wire)).(*model)
	m.providerDetailName = "old"
	if got := strings.Join(m.providerDetailLines(), "\n"); !strings.Contains(got, "probe status/last checked: unknown") || strings.Contains(got, "probe: not configured") {
		t.Fatal(got)
	}
}

func TestProviderCursorWinsConflictingTopAndSurvivesDisabledMovement(t *testing.T) {
	m := stateModel("a", "b", "c", "d", "e")
	m.providerScroll, m.providerCursor = 1, 2
	s := stateSnapshot("c", "a", "d", "e", "b")
	m.Update(snapshotMsg{snapshot: s})
	if m.providerCursor != 0 || m.providerScroll != 0 {
		t.Fatal("selected c must win over conflicting top b")
	}
	s = stateSnapshot("a", "d", "e", "b", "c")
	s.DisabledProviders, s.Providers = s.Providers[4:], s.Providers[:4]
	m.Update(snapshotMsg{snapshot: s})
	if m.providerList()[m.providerCursor].Name != "c" {
		t.Fatal("enabled/disabled movement lost selection")
	}
	m.providerDetailName = "c"
	if got := strings.Join(m.providerDetailLines(), "\n"); !strings.Contains(got, "enabled: false") {
		t.Fatal(got)
	}
}

func TestProviderSelectionDetailPauseAndCompact(t *testing.T) {
	m := layoutModel()
	m.focus = focusProviders
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	layoutKey(m, "down")
	wanted := m.providerList()[m.providerCursor].Name
	m.snapshot.Healthchecks = []HealthcheckEntry{{Provider: wanted, Configured: true, Checked: true, Healthy: true, LastChecked: m.now.Add(-30 * time.Second)}}
	if !strings.Contains(assertViewport(t, m), "> "+wanted) {
		t.Fatal("selected row not visible")
	}
	layoutKey(m, "z")
	layoutKey(m, "enter")
	if !m.zoomed || m.providerDetailName != wanted {
		t.Fatal("Enter did not drill down; z must retain zoom")
	}
	for _, size := range [][2]int{{80, 12}, {80, 24}, {120, 30}, {160, 48}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		assertViewport(t, m)
		layoutKey(m, "end")
		assertViewport(t, m)
		layoutKey(m, "home")
	}
	layoutKey(m, "p")
	before := strings.Join(m.providerDetailLines(), "\n")
	m.Update(snapshotMsg{snapshot: stateSnapshot("new", wanted, "other")})
	m.Update(tickMsg(time.Now().Add(time.Hour)))
	if strings.Join(m.providerDetailLines(), "\n") != before {
		t.Fatal("paused provider detail changed")
	}
	layoutKey(m, "?")
	layoutKey(m, "j")
	layoutKey(m, "esc")
	if m.providerDetailName != wanted {
		t.Fatal("help lost detail")
	}
	layoutKey(m, "p")
	if m.providerList()[m.providerCursor].Name != wanted {
		t.Fatal("resume lost selected identity")
	}
	layoutKey(m, "esc")
	if !m.zoomed || m.hasDetail() {
		t.Fatal("detail back lost zoom")
	}
	m.Update(snapshotMsg{snapshot: stateSnapshot("other", "new", wanted)})
	if m.providerList()[m.providerCursor].Name != wanted {
		t.Fatal("reorder lost selection")
	}
	layoutKey(m, "enter")
	m.Update(snapshotMsg{snapshot: stateSnapshot("other", "new")})
	if !strings.Contains(strings.Join(m.providerDetailLines(), "\n"), "removed") || m.providerCursor != 1 {
		t.Fatal("removed provider silently switched detail")
	}
	layoutKey(m, "tab")
	if m.hasDetail() {
		t.Fatal("navigation did not close provider detail")
	}
	m.Update(snapshotMsg{snapshot: stateSnapshot()})
	if m.providerCursor != 0 || m.providerScroll != 0 {
		t.Fatal("empty list anchors not reset")
	}
}

func TestSlowDNSCannotBlockProviderViewOrQuit(t *testing.T) {
	old := net.DefaultResolver
	var calls atomic.Int32
	net.DefaultResolver = &net.Resolver{PreferGo: true, Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
		calls.Add(1)
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	defer func() { net.DefaultResolver = old }()
	m := layoutModel()
	m.focus = focusProviders
	for i := range m.snapshot.Providers {
		m.snapshot.Providers[i].BaseURL = fmt.Sprintf("https://slow-%d.invalid/v1", i)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	start := time.Now()
	assertViewport(t, m)
	layoutKey(m, "down")
	layoutKey(m, "enter")
	assertViewport(t, m)
	cmd := layoutKey(m, "q")
	if cmd == nil {
		t.Fatal("quit unavailable")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("quit did not complete")
	}
	if calls.Load() != 0 || time.Since(start) > time.Second {
		t.Fatalf("UI performed slow DNS: calls=%d elapsed=%s", calls.Load(), time.Since(start))
	}
}
