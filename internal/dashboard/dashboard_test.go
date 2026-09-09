package dashboard

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
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
	health.SetProviders(config.NewCatalog([]config.Provider{{Name: "openai"}, {Name: "backup"}}, nil, nil))
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
	mm3, cmd3 := mm3.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
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
	row := providerRow("openai", true, true, ps, 2, 1_200*time.Millisecond, 5, false, 18, 5, 4, 10)
	if !strings.Contains(row, "4") || !strings.Contains(row, "25.0%") || !strings.Contains(row, "1.2s") {
		t.Errorf("provider row missing err/p95: %q", row)
	}
	if !strings.Contains(row, "✓") {
		t.Errorf("missing health check: %q", row)
	}
	if !strings.Contains(row, "2") {
		t.Errorf("missing throttled 429 count: %q", row)
	}
}

func TestProviderPaneAttributesAliasTraffic(t *testing.T) {
	usage := accounting.NewAggregator()
	usage.Record(accounting.Event{
		Model: "alias/chat", Operation: "chat_completions", StatusCode: 200,
		Provider: "zen", UpstreamModel: "spark", TotalTokens: 10,
		Duration: 100 * time.Millisecond,
	})
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(config.NewCatalog([]config.Provider{{Name: "zen"}}, nil, nil))
	snap := &RuntimeSnapshot{
		Version: "test", Address: ":8080", AuthMode: "none",
		StartTime: time.Now(), Providers: []config.Provider{{Name: "zen"}},
		Aliases: []config.Alias{{Name: "chat"}},
		Usage:   usage,
		Health:  health,
	}
	m := &model{snapshot: snap, health: map[string]bool{"zen": true}, now: time.Now(), dirty: true, lastRefresh: time.Now()}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	got := mm.View().Content
	if !strings.Contains(got, "zen") {
		t.Fatalf("missing provider row:\n%s", got)
	}
	if !strings.Contains(got, "100ms") {
		t.Errorf("provider P95 missing alias-served latency:\n%s", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "u"}))
	if got := mm.View().Content; !strings.Contains(got, "zen/spark") {
		t.Errorf("upstream view missing resolved model:\n%s", got)
	}
}

func TestProviderRowUnknownHealthAndSparseLatency(t *testing.T) {
	ps := accounting.ProviderSummary{Provider: "openai", Requests: 1}
	row := providerRow("openai", false, false, ps, 0, 0, 0, false, 18, 5, 4, 10)
	if !strings.Contains(row, "?") {
		t.Errorf("expected unknown health mark: %q", row)
	}
	if !strings.Contains(row, "n/a") {
		t.Errorf("expected n/a latency: %q", row)
	}
}

func TestTokensCellStreamingMarker(t *testing.T) {
	if got := tokensCell(accounting.Summary{TotalTokens: 0}, 8); !strings.Contains(got, "~") {
		t.Errorf("expected ~ marker, got %q", got)
	}
	if got := tokensCell(accounting.Summary{TotalTokens: 42}, 8); strings.Contains(got, "~") || !strings.Contains(got, "42") {
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

func TestRenderLogsShowsBufferedEntries(t *testing.T) {
	buf := observability.NewLogBuffer(10)
	buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "server up", Attrs: "addr=:8080"})
	buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelError, Message: "boom", Attrs: "err=dial"})
	snap := newSnapshot()
	snap.Logs = buf
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusUsage, bottomHeight: 10}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	got := mm.View().Content
	if !strings.Contains(got, "server up") {
		t.Errorf("missing info log in:\n%s", got)
	}
	if !strings.Contains(got, "boom") {
		t.Errorf("missing error log in:\n%s", got)
	}
}

func TestRenderLogsEmptyState(t *testing.T) {
	snap := newSnapshot() // Logs is nil
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, bottomHeight: 10}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	got := mm.View().Content
	if !strings.Contains(got, "no logs captured") {
		t.Errorf("expected empty state in:\n%s", got)
	}
}

func TestInitialModelSeedsHealth(t *testing.T) {
	snap := newSnapshot()
	mm := InitialModel(snap)
	mod, ok := mm.(*model)
	if !ok {
		t.Fatal("InitialModel did not return *model")
	}
	if !mod.health["openai"] {
		t.Errorf("expected seeded healthy openai: %+v", mod.health)
	}
	if mod.health["backup"] {
		t.Errorf("expected seeded unhealthy backup: %+v", mod.health)
	}
	mm, _ = mod.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if got := mm.View().Content; !strings.Contains(got, "✓") {
		t.Errorf("expected healthy mark on first paint:\n%s", got)
	}
}

func TestStaleIndicatorOnPollError(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{"openai": true}, now: time.Now(), dirty: true, lastRefresh: time.Now()}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mm, _ = mm.Update(pollErrorMsg{err: "connection refused", at: time.Now()})
	mod := mm.(*model)
	mod.SetNowForTest(time.Now().Add(5 * time.Second))
	mod.dirty = true
	if got := mod.View().Content; !strings.Contains(got, "STALE") {
		t.Errorf("expected STALE header:\n%s", got)
	}
}

func TestTenantFilterCycles(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mod := mm.(*model)
	if got := len(mod.filteredSummaries()); got != 3 {
		t.Fatalf("unfiltered summaries = %d, want 3", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "t"}))
	mod = mm.(*model)
	if got := len(mod.filteredSummaries()); got != 3 {
		t.Fatalf("team-a filtered summaries = %d, want 3", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "e"}))
	if got := len(mm.(*model).filteredSummaries()); got == 3 {
		t.Fatal("errors-only should hide 200 rows")
	}
}

func TestAliasCooldownRendered(t *testing.T) {
	snap := newSnapshot()
	snap.Aliases = []config.Alias{{Name: "chat_default", Algorithm: config.AlgorithmRoundRobin,
		RetryStatusCodes: []int{500}, Targets: []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}}}}
	snap.Cooldowns = []CooldownEntry{{Alias: "chat_default", Provider: "openai", Model: "gpt-4o-mini", RemainingMs: 12000}}
	m := &model{snapshot: snap, health: map[string]bool{"openai": true}, now: time.Now(), dirty: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if got := mm.View().Content; !strings.Contains(got, "cool") {
		t.Errorf("expected cooldown state in:\n%s", got)
	}
}

func TestPercentileCeilsSmallSamples(t *testing.T) {
	got := percentile([]time.Duration{100 * time.Millisecond, 200 * time.Millisecond}, 0.95)
	if got != 200*time.Millisecond {
		t.Errorf("p95 of 2 samples = %v, want 200ms", got)
	}
}

func TestBottomTabsSwitchContent(t *testing.T) {
	snap := newSnapshot()
	buf := observability.NewLogBuffer(10)
	buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "tabbed-log-marker"})
	snap.Logs = buf
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	if got := mm.View().Content; !strings.Contains(got, "tabbed-log-marker") {
		t.Fatalf("default bottom tab should be logs:\n%s", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "1"}))
	mod := mm.(*model)
	if mod.bottomTab != bottomTabAliases {
		t.Fatalf("bottomTab = %v, want aliases", mod.bottomTab)
	}
	if got := mod.View().Content; !strings.Contains(got, "ALIASES") || strings.Contains(got, "tabbed-log-marker") {
		t.Errorf("aliases tab should replace logs pane:\n%s", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "2"}))
	if mod := mm.(*model); mod.bottomTab != bottomTabLogs {
		t.Fatalf("bottomTab = %v, want logs", mod.bottomTab)
	}
	if got := mm.View().Content; !strings.Contains(got, "tabbed-log-marker") {
		t.Errorf("logs tab should restore logs pane:\n%s", got)
	}
}

func TestPanesShareFullWidth(t *testing.T) {
	stripANSI := func(s string) string {
		var b strings.Builder
		inEsc := false
		for _, r := range s {
			if inEsc {
				if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
					inEsc = false
				}
				continue
			}
			if r == '\x1b' {
				inEsc = true
				continue
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	for _, width := range []int{80, 100, 120, 160} {
		snap := newSnapshot()
		buf := observability.NewLogBuffer(10)
		buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "w", Attrs: "a=b"})
		snap.Logs = buf
		m := &model{snapshot: snap, health: map[string]bool{"openai": true}, now: time.Now(), dirty: true, lastRefresh: time.Now()}
		mm, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: 30})
		for i, line := range strings.Split(mm.View().Content, "\n") {
			if got := len([]rune(stripANSI(line))); got != width {
				t.Fatalf("width=%d line %d = %d runes, want %d: %q", width, i, got, width, line)
			}
		}
	}
}

func TestUsageColumnsExpandToContentOnWideTerminal(t *testing.T) {
	usage := accounting.NewAggregator()
	usage.Record(accounting.Event{
		Model: "alias/muse-spark-1.3-contributor-free", Operation: "responses",
		StatusCode: 200, TotalTokens: 1191785, Duration: 100 * time.Millisecond,
	})
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(config.NewCatalog([]config.Provider{{Name: "zen"}}, nil, nil))
	snap := &RuntimeSnapshot{
		Version: "test", Address: ":8080", AuthMode: "none",
		StartTime: time.Now(), Providers: []config.Provider{{Name: "zen"}},
		Usage:  usage,
		Health: health,
	}
	m := &model{snapshot: snap, health: map[string]bool{"zen": true}, now: time.Now(), dirty: true, lastRefresh: time.Now()}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 250, Height: 30})
	got := mm.View().Content
	if !strings.Contains(got, "alias/muse-spark-1.3-contributor-free") {
		t.Errorf("long model truncated despite space:\n%s", got)
	}
	if !strings.Contains(got, "1,191,785") {
		t.Errorf("token count missing thousands separators:\n%s", got)
	}
}

func TestUsageColumnsShrinkOnNarrowTerminal(t *testing.T) {
	summaries := []accounting.Summary{
		{Model: "alias/muse-spark-1.3-contributor-free", Operation: "responses", StatusCode: 200, Count: 1615, TotalTokens: 1191785},
	}
	modelW, opW, countW, tokW := usageColWidths(summaries, 58)
	if total := modelW + opW + 6 + countW + tokW + 4; total > 58 {
		t.Fatalf("columns overflow narrow pane: %d > 58", total)
	}
	if modelW < 10 || opW < 8 || countW < 4 || tokW < 6 {
		t.Fatalf("columns shrank below floors: %d %d %d %d", modelW, opW, countW, tokW)
	}
}

func TestProviderColumnsSizeToContent(t *testing.T) {
	nameW, reqW, t429W, tokW := providerColWidths([]string{"zen", "render-coreanesque"}, []int64{0, 1615}, []int64{0, 280}, []int64{0, 1234567}, 125)
	if nameW != len("render-coreanesque") {
		t.Fatalf("nameW = %d, want %d", nameW, len("render-coreanesque"))
	}
	if reqW != len("1,615") {
		t.Fatalf("reqW = %d, want %d", reqW, len("1,615"))
	}
	if t429W != len("280") {
		t.Fatalf("t429W = %d, want %d", t429W, len("280"))
	}
	if tokW != len("1,234,567") {
		t.Fatalf("tokW = %d, want %d", tokW, len("1,234,567"))
	}
}

func TestCommaSeparatesThousands(t *testing.T) {
	cases := map[int64]string{
		0: "0", 42: "42", 280: "280", 999: "999",
		1000: "1,000", 1615: "1,615", 1191785: "1,191,785",
	}
	for n, want := range cases {
		if got := comma(n); got != want {
			t.Errorf("comma(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestLogsAttrsExpandToContent(t *testing.T) {
	buf := observability.NewLogBuffer(10)
	buf.Add(observability.LogEntry{Time: time.Now(), Level: 8, Message: "boom", Attrs: "request_id=01JZ7Q8K9MND0P3Q4R5ST6UV7W public_model=alias/muse-spark-1.3-contributor-free provider=zen upstream_model=spark status=200"})
	snap := newSnapshot()
	snap.Logs = buf
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, lastRefresh: time.Now()}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 250, Height: 30})
	got := mm.View().Content
	if !strings.Contains(got, "upstream_model=spark status=200") {
		t.Errorf("attrs tail truncated despite space:\n%s", got)
	}
}

func TestProviderPaneScrollsManyProviders(t *testing.T) {
	usage := accounting.NewAggregator()
	var providers []config.Provider
	health := map[string]bool{}
	for i := 0; i < 20; i++ {
		name := "prov-" + strings.Repeat("x", i)
		providers = append(providers, config.Provider{Name: name})
		health[name] = true
	}
	snap := &RuntimeSnapshot{
		Version: "test", Address: ":8080", AuthMode: "none",
		StartTime: time.Now(), Providers: providers,
		Usage:  usage,
		Health: &remoteHealth{states: health},
	}
	m := &model{snapshot: snap, health: health, now: time.Now(), dirty: true, focus: focusUsage, lastRefresh: time.Now()}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 24})
	mod := mm.(*model)
	if got := mod.maxProviderScroll(); got <= 0 {
		t.Fatalf("maxProviderScroll = %d, want > 0 for 20 providers", got)
	}
	first := mm.View().Content
	if strings.Contains(first, "prov-"+strings.Repeat("x", 19)) {
		t.Fatalf("last provider visible without scrolling:\n%s", first)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if mod := mm.(*model); mod.focus != focusProviders {
		t.Fatalf("focus = %v, want providers", mod.focus)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "G"}))
	if got := mm.View().Content; !strings.Contains(got, "prov-"+strings.Repeat("x", 19)) {
		t.Errorf("last provider not reachable by scrolling:\n%s", got)
	}
}

func TestTabCyclesThreeFocusAreas(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusUsage}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mod := mm.(*model)
	if mod.focus != focusUsage {
		t.Fatalf("focus = %v, want usage", mod.focus)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyTab}))
	if mod := mm.(*model); mod.focus != focusProviders {
		t.Fatalf("focus after 2 tabs = %v, want providers", mod.focus)
	}
}

func TestSideWidthFollowsProviderContent(t *testing.T) {
	narrow := &model{snapshot: &RuntimeSnapshot{Providers: []config.Provider{{Name: "a"}, {Name: "b"}}}, width: 120}
	if got := narrow.sideWidth(); got >= 60 {
		t.Fatalf("sideWidth = %d, want < 60 for short names", got)
	}
	var many []config.Provider
	for i := 0; i < 5; i++ {
		many = append(many, config.Provider{Name: "very-long-provider-name-" + strings.Repeat("z", 10)})
	}
	wide := &model{snapshot: &RuntimeSnapshot{Providers: many}, width: 120}
	if got := wide.sideWidth(); got != 60 {
		t.Fatalf("sideWidth = %d, want capped at half (60)", got)
	}
}

func TestPauseBuffersSnapshot(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "p"}))
	updated := newSnapshot()
	updated.Providers = []config.Provider{{Name: "claude"}}
	mm, _ = mm.Update(snapshotMsg{snapshot: updated})
	if got := mm.(*model).snapshot.Providers[0].Name; got != "openai" {
		t.Fatalf("paused snapshot swapped early: %q", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "p"}))
	if got := mm.(*model).snapshot.Providers[0].Name; got != "claude" {
		t.Fatalf("unpause did not apply pending: %q", got)
	}
}
