package dashboard

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
)

func searchKey(m *model, key string) tea.Cmd {
	codes := map[string]rune{"left": tea.KeyLeft, "right": tea.KeyRight, "backspace": tea.KeyBackspace, "delete": tea.KeyDelete}
	if code, ok := codes[key]; ok {
		_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: code}))
		return cmd
	}
	if strings.HasPrefix(key, "ctrl+") {
		_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Code: rune(key[len(key)-1]), Mod: tea.ModCtrl}))
		return cmd
	}
	return layoutKey(m, key)
}

func applySearch(m *model, text string) {
	searchKey(m, "/")
	searchKey(m, "ctrl+u")
	m.Update(tea.KeyPressMsg(tea.Key{Text: text}))
	searchKey(m, "enter")
}

func requestModel() *model {
	var events []accounting.Event
	var usage []accounting.Summary
	for i := 0; i < 40; i++ {
		events = append(events, accounting.Event{RequestID: fmt.Sprintf("req-%02d", i), Timestamp: time.Unix(int64(i), 0), Client: fmt.Sprintf("client-%d", i%2), Tenant: fmt.Sprintf("tenant-%d", i%3), Model: "alias/chat", Provider: "origin", UpstreamModel: "model", Operation: "chat_completions", StatusCode: 200 + i%2*300, Duration: time.Second, TotalTokens: 10})
	}
	for _, e := range events[:3] {
		usage = append(usage, accounting.Summary{Model: e.Model, Client: e.Client, Tenant: e.Tenant, Operation: e.Operation, StatusCode: 200, Count: 1})
	}
	m := InitialModel(&RuntimeSnapshot{Usage: &remoteUsage{recent: events, summaries: usage}, Logs: &remoteLogs{entries: []observability.LogEntry{
		{Seq: 1, RequestID: "req-39", Level: slog.LevelInfo, Message: "one"},
		{Seq: 2, RequestID: "req-390", Level: slog.LevelError, Message: "other"},
		{Seq: 3, RequestID: "req-39", Level: slog.LevelWarn, Message: "two"},
		{Seq: 4, Level: slog.LevelError, Attrs: "request_id=req-39", Message: "legacy cannot correlate"},
	}}}).(*model)
	m.connection = ConnectionStatus{LastSuccess: time.Now()}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	layoutApplyKey(m, "5")
	return m
}

func TestSearchEditorOwnershipBoundAndCancel(t *testing.T) {
	m := requestModel()
	applySearch(m, "client:client-1")
	if len(m.recentRequests()) != 20 {
		t.Fatal("client search not applied")
	}
	searchKey(m, "/")
	searchKey(m, "ctrl+u")
	for _, key := range []string{"q", "p", "h", "?", "1", "5", "[", "]"} {
		searchKey(m, key)
	}
	if string(m.search.text) != "qph?15[]" || m.quit || m.paused || m.showHelp || m.bottomTab != bottomTabRequests {
		t.Fatalf("editor leaked keys: %+v", m.search)
	}
	m.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyF1}))
	if string(m.search.text) != "qph?15[]" {
		t.Fatal("non-text key inserted into editor")
	}
	searchKey(m, "left")
	searchKey(m, "backspace")
	searchKey(m, "delete")
	if string(m.search.text) != "qph?15" {
		t.Fatalf("editing: %q", m.search.text)
	}
	searchKey(m, "home")
	searchKey(m, "right")
	m.Update(tea.KeyPressMsg(tea.Key{Text: "界"}))
	if string(m.search.text) != "q界ph?15" {
		t.Fatal(string(m.search.text))
	}
	searchKey(m, "end")
	m.Update(tea.KeyPressMsg(tea.Key{Text: strings.Repeat("界", 1000) + "\x1b\n"}))
	if len(m.search.text) != searchLimit {
		t.Fatalf("editor unbounded: %d", len(m.search.text))
	}
	view := m.View().Content
	if len(strings.Split(view, "\n")) != 12 {
		t.Fatal("search overflows compact viewport")
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) != 80 {
			t.Fatal("search width overflow")
		}
	}
	for _, want := range []string{"SEARCH", "│", "[enter] apply", "[esc] cancel", "^U clear", "Ctrl+C quit", "last OK"} {
		if !strings.Contains(view, want) {
			t.Fatalf("missing %s: %s", want, view)
		}
	}
	searchKey(m, "esc")
	if m.queries[bottomTabRequests] != "client:client-1" || m.search.active {
		t.Fatal("cancel did not retain applied filter")
	}
	searchKey(m, "ctrl+u")
	if len(m.recentRequests()) != 40 || m.queries[bottomTabRequests] != "" {
		t.Fatal("clear failed")
	}
	searchKey(m, "/")
	retried := false
	m.retry = func() { retried = true }
	searchKey(m, "ctrl+r")
	if !retried || !m.search.active {
		t.Fatal("retry not reachable in editor")
	}
	cmd := searchKey(m, "ctrl+c")
	if cmd == nil {
		t.Fatal("missing quit")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatal("ctrl+c not quit")
	}
}

func TestRequestFiltersSelectionPauseAndLayouts(t *testing.T) {
	m := requestModel()
	applySearch(m, "client:CLIENT-1 tenant:tenant-0 model:alias provider:origin status:500 op:chat resolved:model")
	rows := m.recentRequests()
	if len(rows) != 7 || rows[0].RequestID != "req-39" {
		t.Fatalf("AND metadata: %+v", rows)
	}
	layoutApplyKey(m, "e")
	layoutApplyKey(m, "down")
	selected := m.recentRequests()[m.requestCursor]
	s := *m.snapshot
	u := *s.Usage.(*remoteUsage)
	e := selected
	e.RequestID = "new"
	u.recent = append(append([]accounting.Event(nil), u.recent...), e)
	s.Usage = &u
	m.Update(snapshotMsg{snapshot: &s})
	if m.recentRequests()[m.requestCursor] != selected {
		t.Fatal("refresh moved selected request")
	}
	layoutApplyKey(m, "p")
	frozen := m.View().Content
	oldNow := m.now
	u2 := u
	u2.recent = nil
	s2 := s
	s2.Usage = &u2
	m.Update(snapshotMsg{snapshot: &s2})
	m.Update(tickMsg(time.Now().Add(time.Hour)))
	if m.View().Content != frozen || m.now != oldNow {
		t.Fatal("paused list or clock changed")
	}
	layoutApplyKey(m, "enter")
	if m.requestDetail == nil || *m.requestDetail != selected {
		t.Fatal("paused metadata unavailable")
	}
	for _, size := range [][2]int{{80, 12}, {80, 24}, {120, 30}, {160, 48}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		assertViewport(t, m)
		layoutApplyKey(m, "end")
		assertViewport(t, m)
		layoutApplyKey(m, "?")
		assertViewport(t, m)
		layoutApplyKey(m, "esc")
	}
	layoutApplyKey(m, "esc")
	layoutApplyKey(m, "p")
	if len(m.recentRequests()) != 0 {
		t.Fatal("resume did not apply latest")
	}
	if !strings.Contains(m.View().Content, "no matches") {
		t.Fatal("missing empty-filter explanation")
	}
	searchKey(m, "ctrl+u")
	if m.requestCursor != 0 || m.requestOffset != 0 {
		t.Fatal("empty selection not clamped")
	}
}

func TestUsageIdentityDetailDistinguishesEveryGroup(t *testing.T) {
	m := requestModel()
	layoutApplyKey(m, "shift+tab")
	if m.focus != focusUsage {
		t.Fatal("expected usage")
	}
	layoutApplyKey(m, "enter")
	seen := map[string]bool{}
	for i := 0; i < 3; i++ {
		_, lines := m.metadataLines()
		seen[strings.Join(lines, "\n")] = true
		assertViewport(t, m)
		layoutApplyKey(m, "n")
	}
	if len(seen) != 3 {
		t.Fatal("same model/status groups indistinguishable")
	}
	layoutApplyKey(m, "N")
	if m.usageDetail.Tenant != "tenant-2" {
		t.Fatal("reverse group navigation")
	}
}

func TestSharedLogAndPayloadSearchAndCorrelationReturn(t *testing.T) {
	m := requestModel()
	f := payloadTestFetcher()
	f.list.Payloads = append([]PayloadSummary{{RequestID: "req-39", Status: 200, PublicModel: "alias/chat", Provider: "origin"}, {RequestID: "req-390", Status: 200}}, f.list.Payloads...)
	f.detail["req-39"] = "captured fixture"
	m.payloadFetcher = f
	m.payloadEnabled = true
	layoutApplyKey(m, "3")
	applySearch(m, "model:openai status:500")
	if rows := m.orderedPayloads(); len(rows) != 1 || rows[0].RequestID != "req-new" {
		t.Fatalf("payload metadata filter: %+v", rows)
	}
	layoutApplyKey(m, "s")
	if len(m.orderedPayloads()) != 1 {
		t.Fatal("combined payload error/search filter")
	}
	layoutApplyKey(m, "2")
	applySearch(m, "id:req-39 level:warn")
	if rows := m.filteredLogs(500); len(rows) != 1 || rows[0].Seq != 3 {
		t.Fatalf("log filter: %+v", rows)
	}
	layoutApplyKey(m, "l")
	if len(m.filteredLogs(500)) != 1 {
		t.Fatal("combined log level/search")
	}
	layoutApplyKey(m, "5")
	applySearch(m, "id:req-39")
	layoutApplyKey(m, "z")
	layoutApplyKey(m, "enter")
	m.metadataScroll = 2
	layoutApplyKey(m, "l")
	if rows := m.filteredLogs(500); len(rows) != 2 {
		t.Fatalf("exact correlation matched prefix/flattened attrs or retained target filters: %+v", rows)
	}
	if !strings.Contains(m.View().Content, "ID-only") {
		t.Fatal("correlation falsely labels bypassed filters")
	}
	layoutApplyKey(m, "enter")
	if !m.logDetailOpen {
		t.Fatal("correlated log not inspectable")
	}
	layoutApplyKey(m, "esc")
	layoutApplyKey(m, "esc")
	if m.requestDetail == nil || m.requestDetail.RequestID != "req-39" || m.metadataScroll != 2 || !m.zoomed || m.queries[bottomTabLogs] != "id:req-39 level:warn" {
		t.Fatal("request/log return context lost")
	}
	layoutApplyKey(m, "v")
	if rows := m.orderedPayloads(); len(rows) != 1 || rows[0].RequestID != "req-39" {
		t.Fatalf("payload correlation: %+v", rows)
	}
	if !strings.Contains(m.View().Content, "ID-only") || strings.Contains(m.View().Content, "errs-only") {
		t.Fatal("payload correlation falsely labels bypassed filter")
	}
	layoutApplyKey(m, "enter")
	if m.payloadDetail != "captured fixture" {
		t.Fatal("payload detail failed")
	}
	layoutApplyKey(m, "esc")
	layoutApplyKey(m, "esc")
	if m.requestDetail == nil || !m.payloadErrorsOnly || m.queries[bottomTabPayload] != "model:openai status:500" {
		t.Fatal("payload return filters lost")
	}
	layoutApplyKey(m, "esc")
	layoutApplyKey(m, "3")
	if rows := m.orderedPayloads(); len(rows) != 1 || rows[m.payloadCursor].RequestID != "req-new" {
		t.Fatal("target selection not restored")
	}
}

func TestCorrelationMissingDisabledExpiredPauseAndLateResults(t *testing.T) {
	m := requestModel()
	layoutApplyKey(m, "enter")
	m.requestDetail.RequestID = "absent"
	layoutApplyKey(m, "l")
	if !strings.Contains(m.View().Content, "No matching logs") {
		t.Fatal(m.View().Content)
	}
	layoutApplyKey(m, "esc")
	layoutApplyKey(m, "v")
	if !strings.Contains(m.View().Content, "unavailable") {
		t.Fatal("missing fetcher not explained")
	}
	layoutApplyKey(m, "esc")
	f := payloadTestFetcher()
	f.list.Enabled = false
	m.payloadFetcher = f
	layoutApplyKey(m, "v")
	if !strings.Contains(m.View().Content, "disabled") {
		t.Fatal("disabled payload not explained")
	}
	layoutApplyKey(m, "esc")
	f.list.Enabled = true
	layoutApplyKey(m, "v")
	if !strings.Contains(m.View().Content, "No matching payload") {
		t.Fatal("expiry/cap not explained")
	}
	layoutApplyKey(m, "esc")
	m.requestDetail.RequestID = "req-new"
	layoutApplyKey(m, "p")
	if cmd := layoutKey(m, "v"); cmd != nil {
		t.Fatal("paused correlation issued read")
	}
	if cmd := layoutKey(m, "enter"); cmd != nil {
		t.Fatal("paused payload detail issued read")
	}
	layoutApplyKey(m, "p")
	cmd := layoutKey(m, "enter")
	if cmd == nil {
		t.Fatal("expected detail fetch")
	}
	late := cmd()
	layoutApplyKey(m, "esc")
	layoutApplyKey(m, "esc")
	m.Update(late)
	if m.payloadDetailOpen() || m.requestDetail == nil {
		t.Fatal("late correlation detail overwrote return")
	}
	layoutApplyKey(m, "v")
	layoutApplyKey(m, "5")
	if m.correlation != nil || m.hasDetail() {
		t.Fatal("numbered navigation retained hidden return context")
	}
	m.snapshot.Usage = &remoteUsage{recent: []accounting.Event{{Model: "legacy/model", StatusCode: 200}}}
	layoutApplyKey(m, "enter")
	layoutApplyKey(m, "l")
	if m.correlation != nil || !strings.Contains(m.correlationNotice, "no request ID") {
		t.Fatal("old snapshot invented correlation")
	}
}

func TestPayloadBoundAndSearchNoMatches(t *testing.T) {
	m := requestModel()
	m.payloadFetcher = payloadTestFetcher()
	m.payloadEnabled = true
	m.bottomTab = bottomTabPayload
	rows := make([]dashrpc.PayloadSummary, 200)
	for i := range rows {
		rows[i] = PayloadSummary{RequestID: fmt.Sprint(i), Status: 200}
	}
	m.applyPayloadList(payloadListMsg{entries: rows, enabled: true})
	if len(m.payloads) != payloadFetchLimit {
		t.Fatal("payload response cap not enforced")
	}
	applySearch(m, "id:missing")
	if !strings.Contains(m.View().Content, "no matches") {
		t.Fatal(m.View().Content)
	}
	searchKey(m, "ctrl+u")
	if len(m.orderedPayloads()) != payloadFetchLimit {
		t.Fatal("clear did not restore bounded list")
	}
}

func TestFilteredListsAnchorDuringEditorAndPause(t *testing.T) {
	m := requestModel()
	f := payloadTestFetcher()
	m.payloadFetcher = f
	m.payloadEnabled = true
	layoutApplyKey(m, "3")
	applySearch(m, "model:openai")
	layoutApplyKey(m, "down")
	id := m.payloadAt(m.payloadCursor).RequestID
	searchKey(m, "/")
	m.applyPayloadList(payloadListMsg{generation: m.payloadListRequest.generation, enabled: true, entries: append([]PayloadSummary{{RequestID: "prepend", PublicModel: "openai/new", Status: 200}}, f.list.Payloads...)})
	if m.payloadAt(m.payloadCursor).RequestID != id || !m.search.active {
		t.Fatal("arrival disrupted editing/selection")
	}
	searchKey(m, "esc")
	layoutApplyKey(m, "p")
	applySearch(m, "status:500")
	if rows := m.orderedPayloads(); len(rows) != 1 || rows[0].RequestID != "req-new" {
		t.Fatal("cached payload search must work while paused")
	}
	m.applyPayloadList(payloadListMsg{generation: m.payloadListRequest.generation, enabled: true})
	if len(m.orderedPayloads()) != 1 {
		t.Fatal("paused arrival changed filtered payloads")
	}
	layoutApplyKey(m, "p")
	if len(m.orderedPayloads()) != 0 {
		t.Fatal("resume failed to apply latest payload list")
	}
	layoutApplyKey(m, "2")
	applySearch(m, "id:req-39")
	layoutApplyKey(m, "down")
	wanted := m.filteredLogs(500)[m.logCursor].Seq
	s := *m.snapshot
	l := *s.Logs.(*remoteLogs)
	l.entries = append(append([]observability.LogEntry(nil), l.entries...), observability.LogEntry{Seq: 5, RequestID: "req-39", Level: slog.LevelWarn})
	s.Logs = &l
	m.Update(snapshotMsg{snapshot: &s})
	if m.filteredLogs(500)[m.logCursor].Seq != wanted {
		t.Fatal("filtered log arrival changed pinned selection")
	}
	applySearch(m, "id:missing")
	if !strings.Contains(m.View().Content, "no matches") {
		t.Fatal("log no-match state hidden")
	}
}
