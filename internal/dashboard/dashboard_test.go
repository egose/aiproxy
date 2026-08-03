package dashboard

import (
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/providerhealth"
)

func newSnapshot() *RuntimeSnapshot {
	usage := accounting.NewAggregator()
	usage.Record(accounting.Event{
		Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini",
		Operation: "chat_completions", StatusCode: 200,
		PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20,
		Duration: 350 * time.Millisecond,
	})
	usage.Record(accounting.Event{
		Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini",
		Operation: "chat_completions", StatusCode: 500,
		Duration: 1_200 * time.Millisecond,
	})
	usage.Record(accounting.Event{
		Tenant: "team-a", Client: "ops", Model: "alias/chat_default",
		Operation: "chat_completions", StatusCode: 200,
		PromptTokens: 4, CompletionTokens: 1, TotalTokens: 5,
		Duration: 80 * time.Millisecond,
	})
	// Streaming-style entry: zero tokens (not recorded by adapter for streams).
	usage.Record(accounting.Event{
		Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini",
		Operation: "chat_completions", StatusCode: 200,
		Duration: 90 * time.Millisecond,
	})
	// Sentinel-bucketed error (should not appear in the usage table).
	usage.Record(accounting.Event{
		Tenant: "team-a", Client: "ci", Model: "_unresolved_model",
		Operation: "chat_completions", StatusCode: 404,
	})
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(map[string]config.Provider{
		"openai": {Name: "openai"},
		"backup": {Name: "backup"},
	})
	health.MarkFailure("backup")
	return &RuntimeSnapshot{
		Version:   "test",
		Address:   ":8080",
		AuthMode:  "bearer_static",
		StartTime: time.Now().Add(-2 * time.Minute),
		Providers: []config.Provider{
			{Name: "openai"},
			{Name: "backup"},
		},
		DisabledProviders: []config.Provider{
			{Name: "localai"},
		},
		Aliases: []config.Alias{{Name: "chat_default"}},
		Usage:   usage,
		Health:  health,
	}
}

func TestModelRenderNormal(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{"openai": true, "backup": false}, now: time.Now(), dirty: true}
	// Force a window size by routing through Update.
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	got := mm.View().Content
	if !strings.Contains(got, "aiproxy test") {
		t.Errorf("missing header in:\n%s", got)
	}
	if !strings.Contains(got, "openai") || !strings.Contains(got, "backup") {
		t.Errorf("missing providers in:\n%s", got)
	}
	if !strings.Contains(got, "chat_completions") {
		t.Errorf("missing usage rows in:\n%s", got)
	}
	// Disabled provider appears dimmed (no assertion on style; just presence).
	if !strings.Contains(got, "localai") {
		t.Errorf("missing disabled provider in:\n%s", got)
	}
	// Sentinel "_unresolved_model" should not surface anywhere.
	if strings.Contains(got, "_unresolved_model") {
		t.Errorf("sentinel model leaked into view:\n%s", got)
	}
}

func TestModelRenderTooSmall(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now()}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 40, Height: 8})
	got := mm.View().Content
	if !strings.Contains(got, "Terminal too small") {
		t.Errorf("expected too-small, got:\n%s", got)
	}
}

func TestModelTickUpdatesHealthAndTimestamp(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now().Add(-time.Hour)}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if mm, _ = mm.Update(tickMsg(time.Now())); mm == nil {
		t.Fatal("nil model after tick")
	}
	mod := mm.(*model)
	if len(mod.health) != 2 {
		t.Fatalf("health not updated: %+v", mod.health)
	}
	// backup is marked failed in the snapshot.
	if mod.health["backup"] {
		t.Errorf("backup should be unhealthy: %+v", mod.health)
	}
}

func TestModelQuitOnKey(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "q"}))
	if cmd == nil {
		t.Fatalf("expected quit cmd for q")
	}
	mod := mm.(*model)
	if !mod.quit {
		t.Errorf("expected quit=true for q")
	}

	m2 := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true}
	mm2, _ := m2.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mm2, cmd2 := mm2.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if cmd2 == nil {
		t.Fatalf("expected quit cmd for Esc")
	}

	m3 := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true}
	mm3, _ := m3.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mm3, cmd3 := mm3.Update(tea.KeyPressMsg(tea.Key{Text: "Tab"}))
	if cmd3 != nil {
		t.Fatalf("expected no-quit for non-quit key, got cmd")
	}
}

func TestModelViewDirtyCaching(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{"openai": true, "backup": false}, now: time.Now(), dirty: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mod := mm.(*model)
	firstView := mod.View().Content
	if firstView == "" {
		t.Fatal("first view empty")
	}
	if mod.dirty {
		t.Errorf("dirty should be cleared after View()")
	}
	mod.SetNowForTest(time.Now().Add(time.Hour))
	secondView := mod.View().Content
	if secondView != firstView {
		t.Errorf("second view should equal cached first when not dirty")
	}
}

func TestByProviderInSnapshot(t *testing.T) {
	snap := newSnapshot()
	summaries := snap.Usage.Summaries()
	got := accounting.ByProvider(summaries)
	if len(got) != 3 {
		t.Fatalf("byProvider = %+v", got)
	}
	if got[0].Provider != "openai" || got[0].Requests != 3 || got[0].Errors != 1 {
		t.Fatalf("expected openai first with reqs=3 errs=1, got %+v", got[0])
	}
}

func TestProviderRowFormatsErrorAndP95(t *testing.T) {
	ps := accounting.ProviderSummary{Provider: "openai", Requests: 4, Errors: 1, TotalTokens: 25}
	row := providerRow("openai", true, ps, 1_200*time.Millisecond, false)
	if !strings.Contains(row, "4") || !strings.Contains(row, "25.0%") || !strings.Contains(row, "1.2s") {
		t.Errorf("provider row missing err/p95: %q", row)
	}
	if !strings.Contains(row, "✓") {
		t.Errorf("missing health check: %q", row)
	}
}

func TestTokensCellStreamingMarker(t *testing.T) {
	if got := tokensCell(accounting.Summary{TotalTokens: 0}); !strings.Contains(got, "~") {
		t.Errorf("expected ~ marker, got %q", got)
	}
	if got := tokensCell(accounting.Summary{TotalTokens: 42}); strings.Contains(got, "~") || !strings.Contains(got, "42") {
		t.Errorf("expected numeric tokens, got %q", got)
	}
}

func TestP95LatencyByProvider(t *testing.T) {
	events := []accounting.Event{
		{Model: "openai/a", Duration: 100 * time.Millisecond},
		{Model: "openai/a", Duration: 200 * time.Millisecond},
		{Model: "openai/a", Duration: 300 * time.Millisecond},
		{Model: "openai/a", Duration: 400 * time.Millisecond},
		{Model: "openai/a", Duration: 500 * time.Millisecond},
	}
	got := p95LatencyByProvider(events)
	if p95, ok := got["openai"]; !ok || p95 < 400*time.Millisecond || p95 > 500*time.Millisecond {
		t.Errorf("p95 = %v (ok=%v)", p95, ok)
	}
}

func TestSnapshotRefreshUpdatesProviders(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	updated := newSnapshot()
	updated.Providers = []config.Provider{{Name: "claude"}}
	mm, _ = mm.Update(snapshotMsg{snapshot: updated})
	mod := mm.(*model)
	if len(mod.snapshot.Providers) != 1 || mod.snapshot.Providers[0].Name != "claude" {
		t.Fatalf("snapshot not refreshed: %+v", mod.snapshot.Providers)
	}
}
