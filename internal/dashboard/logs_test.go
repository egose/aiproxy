package dashboard

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

func logOrderModel(buf *observability.LogBuffer) *model {
	snap := newSnapshot()
	snap.Logs = buf
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusBottom, bottomTab: bottomTabLogs, logFollow: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return mm.(*model)
}

func TestLogOrderStableWithRemoteSnapshot(t *testing.T) {
	entries := []observability.LogEntry{
		{Time: time.Now(), Level: slog.LevelInfo, Message: "alpha", Seq: 1},
		{Time: time.Now(), Level: slog.LevelInfo, Message: "beta", Seq: 2},
		{Time: time.Now(), Level: slog.LevelInfo, Message: "gamma", Seq: 3},
	}
	snap := newSnapshot()
	snap.Logs = &remoteLogs{entries: entries}
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusBottom, bottomTab: bottomTabLogs, logFollow: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = mm.(*model)
	orderOf := func(view string) (int, int, int) {
		return strings.Index(view, "alpha"), strings.Index(view, "beta"), strings.Index(view, "gamma")
	}
	for i := 0; i < 5; i++ {
		m.dirty = true
		ja, jb, jc := orderOf(m.View().Content)
		if !(jc >= 0 && jc < jb && jb < ja) {
			t.Fatalf("render %d broke newest-first order: gamma %d beta %d alpha %d", i, jc, jb, ja)
		}
		mm, _ := m.Update(tickMsg(time.Now()))
		m = mm.(*model)
	}
	m.toggleLogOrder()
	m.dirty = true
	ja, jb, jc := orderOf(m.View().Content)
	if !(ja >= 0 && ja < jb && jb < jc) {
		t.Fatalf("oldest-first toggle broke order: alpha %d beta %d gamma %d", ja, jb, jc)
	}
}

func TestLogTailFollowsChronological(t *testing.T) {
	buf := observability.NewLogBuffer(100)
	for i := 0; i < 10; i++ {
		buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "old"})
	}
	m := logOrderModel(buf)
	mm, _ := m.Update(tickMsg(time.Now()))
	m = mm.(*model)
	if m.logCursor != 0 {
		t.Fatalf("expected cursor at newest index 0, got %d", m.logCursor)
	}
	buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "newcomer"})
	mm, _ = m.Update(tickMsg(time.Now()))
	m = mm.(*model)
	if m.logCursor != 0 {
		t.Fatalf("tail should follow newest, cursor = %d", m.logCursor)
	}
	got := m.View().Content
	if !strings.Contains(got, "newcomer") {
		t.Fatalf("newest entry missing from tail view:\n%s", got)
	}
	firstNew, firstOld := strings.Index(got, "newcomer"), strings.Index(got, "old")
	if firstNew < 0 || firstOld < 0 || firstNew > firstOld {
		t.Fatalf("newest should be at top:\n%s", got)
	}
}

func TestLogPinnedSelectionSurvivesArrival(t *testing.T) {
	buf := observability.NewLogBuffer(100)
	for i := 0; i < 10; i++ {
		buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "old"})
	}
	m := logOrderModel(buf)
	mm, _ := m.Update(tickMsg(time.Now()))
	m = mm.(*model)
	for i := 0; i < 3; i++ {
		mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
		m = mm.(*model)
	}
	pinned := m.logCursorSeq
	buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "newcomer"})
	mm, _ = m.Update(tickMsg(time.Now()))
	m = mm.(*model)
	if m.logCursorSeq != pinned {
		t.Fatalf("pinned selection moved: was %d now %d", pinned, m.logCursorSeq)
	}
	if m.logFollow {
		t.Fatal("scrolled-up view should not follow tail")
	}
}

func TestArrowKeysMoveRemoteCursor(t *testing.T) {
	for _, seqs := range []bool{true, false} {
		var entries []observability.LogEntry
		for i := 0; i < 30; i++ {
			e := observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "m"}
			if seqs {
				e.Seq = uint64(i + 1)
			}
			entries = append(entries, e)
		}
		snap := newSnapshot()
		snap.Logs = &remoteLogs{entries: entries}
		m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusBottom, bottomTab: bottomTabLogs, logFollow: true}
		mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
		m = mm.(*model)
		mm, _ = m.Update(tickMsg(time.Now()))
		m = mm.(*model)
		start := m.logCursor
		if start != 0 {
			t.Fatalf("seqs=%v: newest-first should pin cursor at 0, got %d", seqs, start)
		}
		mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		m = mm.(*model)
		if m.logCursor != start+1 {
			t.Fatalf("seqs=%v: down arrow: cursor %d -> %d, want %d", seqs, start, m.logCursor, start+1)
		}
		mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyDown}))
		m = mm.(*model)
		if m.logCursor != start+2 {
			t.Fatalf("seqs=%v: second down: cursor = %d, want %d", seqs, m.logCursor, start+2)
		}
		mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyUp}))
		m = mm.(*model)
		if m.logCursor != start+1 {
			t.Fatalf("seqs=%v: up arrow: cursor = %d, want %d", seqs, m.logCursor, start+1)
		}
		mm, _ = m.Update(tickMsg(time.Now()))
		m = mm.(*model)
		if m.logCursor != start+1 {
			t.Fatalf("seqs=%v: tick after arrows reset cursor to %d, want pinned %d", seqs, m.logCursor, start+1)
		}
		if got := m.View().Content; !strings.Contains(got, "(j/k move)") && !strings.Contains(got, "[j/k] move") {
			t.Fatalf("seqs=%v: missing payload-style move hint:\n%s", seqs, got)
		}
	}
}

func TestLogJKMoveHintMatchesPayload(t *testing.T) {
	snap := newSnapshot()
	var entries []observability.LogEntry
	for i := 0; i < 30; i++ {
		entries = append(entries, observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "m", Seq: uint64(i + 1)})
	}
	snap.Logs = &remoteLogs{entries: entries}
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusBottom, bottomTab: bottomTabLogs, logFollow: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = mm.(*model)
	mm, _ = m.Update(tickMsg(time.Now()))
	m = mm.(*model)
	before := m.logCursor
	mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	m = mm.(*model)
	if m.logCursor != before+1 {
		t.Fatalf("j moved cursor to %d, want %d", m.logCursor, before+1)
	}
	mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "k"}))
	m = mm.(*model)
	if m.logCursor != before {
		t.Fatalf("k moved cursor to %d, want %d", m.logCursor, before)
	}
}

func TestAliasEnterOpensDetail(t *testing.T) {
	snap := newSnapshot()
	snap.Aliases = []config.Alias{
		{Name: "chat", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{500},
			Targets: []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}, {Provider: "backup", Model: "gpt-4o-mini"}}},
		{Name: "other", Targets: []config.AliasTarget{{Provider: "backup", Model: "x"}}},
	}
	snap.Cooldowns = []CooldownEntry{{Alias: "chat", Provider: "openai", Model: "gpt-4o-mini", RemainingMs: 12000}}
	m := &model{snapshot: snap, health: map[string]bool{"openai": true, "backup": true}, now: time.Now(), dirty: true, focus: focusBottom, bottomTab: bottomTabAliases}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = mm.(*model)
	mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "j"}))
	m = mm.(*model)
	if m.aliasCursor != 1 {
		t.Fatalf("j moved alias cursor to %d, want 1", m.aliasCursor)
	}
	mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	m = mm.(*model)
	if !m.aliasDetailOpen() || m.aliasDetailName != "other" {
		t.Fatalf("alias detail = %q open=%v, want other", m.aliasDetailName, m.aliasDetailOpen())
	}
	got := m.View().Content
	if !strings.Contains(got, "ALIAS other") {
		t.Fatalf("missing alias detail title:\n%s", got)
	}
	if !strings.Contains(got, "backup/x") {
		t.Fatalf("missing target row in detail:\n%s", got)
	}
	mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	m = mm.(*model)
	if m.aliasDetailOpen() {
		t.Fatal("esc should close alias detail")
	}
}

func TestAliasDetailShowsCooldownAndHealth(t *testing.T) {
	snap := newSnapshot()
	snap.Aliases = []config.Alias{
		{Name: "chat", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{500},
			Targets: []config.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}}},
	}
	snap.Cooldowns = []CooldownEntry{{Alias: "chat", Provider: "openai", Model: "gpt-4o-mini", RemainingMs: 12000}}
	m := &model{snapshot: snap, health: map[string]bool{"openai": true}, now: time.Now(), dirty: true, focus: focusBottom, bottomTab: bottomTabAliases}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = mm.(*model)
	mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	m = mm.(*model)
	got := m.View().Content
	if !strings.Contains(got, "openai/gpt-4o-mini") {
		t.Fatalf("missing target in detail:\n%s", got)
	}
	if !strings.Contains(got, "cool") {
		t.Fatalf("missing cooldown state in detail:\n%s", got)
	}
	if !strings.Contains(got, "retry: 500") && !strings.Contains(got, "retry:500") {
		t.Fatalf("missing retry codes in detail:\n%s", got)
	}
}

func TestBottomEnterOpensDetailNotZoom(t *testing.T) {
	snap := newSnapshot()
	buf := observability.NewLogBuffer(10)
	buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "enter-marker"})
	snap.Logs = buf
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusBottom, bottomTab: bottomTabLogs, logFollow: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	m = mm.(*model)
	mm, _ = m.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	m = mm.(*model)
	if m.zoomed {
		t.Fatal("enter in bottom tab should open detail, not zoom")
	}
	if !m.logDetailOpen {
		t.Fatal("enter should open log detail")
	}
}

func TestLogOrderToggleKey(t *testing.T) {
	buf := observability.NewLogBuffer(10)
	buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "logAAA"})
	buf.Add(observability.LogEntry{Time: time.Now(), Level: slog.LevelInfo, Message: "logZZZ"})
	snap := newSnapshot()
	snap.Logs = buf
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusBottom, bottomTab: bottomTabLogs, logFollow: true}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mm, _ = mm.Update(tickMsg(time.Now()))
	got := mm.View().Content
	if strings.Index(got, "logZZZ") > strings.Index(got, "logAAA") {
		t.Fatalf("default should be newest-first:\n%s", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "o"}))
	mod := mm.(*model)
	if !mod.logOldestFirst {
		t.Fatal("o should toggle logs to oldest-first")
	}
	mod.dirty = true
	got = mm.View().Content
	if strings.Index(got, "logAAA") > strings.Index(got, "logZZZ") {
		t.Fatalf("toggled should be oldest-first:\n%s", got)
	}
	if !strings.Contains(got, "oldest-first") {
		t.Fatalf("title should show order:\n%s", got)
	}
}

func TestZStillZooms(t *testing.T) {
	snap := newSnapshot()
	m := &model{snapshot: snap, health: map[string]bool{}, now: time.Now(), dirty: true, focus: focusUsage}
	mm, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "z"}))
	if mod := mm.(*model); !mod.zoomed {
		t.Fatal("z should zoom focused pane")
	}
}
