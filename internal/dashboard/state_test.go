package dashboard

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
)

func stateSnapshot(names ...string) *RuntimeSnapshot {
	s := &RuntimeSnapshot{Version: "state", StartTime: time.Unix(100, 0), SnapshotAt: time.Unix(200, 0)}
	u := &remoteUsage{providerStatsAvailable: true}
	for i, name := range names {
		s.Providers = append(s.Providers, config.Provider{Name: name})
		s.Aliases = append(s.Aliases, config.Alias{Name: name})
		u.summaries = append(u.summaries, accounting.Summary{Model: name, Count: int64(len(names) - i), StatusCode: 200})
	}
	s.Usage = u
	return s
}

func stateModel(names ...string) *model {
	m := InitialModel(stateSnapshot(names...)).(*model)
	m.now = time.Unix(200, 0)
	m.width, m.height, m.statsHeight, m.bottomHeight = 140, 30, 10, 7
	return m
}

func stateKey(m *model, key string) tea.Cmd {
	_, cmd := m.Update(tea.KeyPressMsg(tea.Key{Text: key}))
	return cmd
}

func TestRefreshIdentityAcrossListClasses(t *testing.T) {
	for _, tc := range []struct {
		name, top, selected string
		names               []string
	}{
		{"repeat", "b", "c", []string{"a", "b", "c", "d", "e"}},
		{"prepend", "b", "c", []string{"x", "a", "b", "c", "d", "e"}},
		{"reorder", "b", "c", []string{"e", "d", "a", "b", "c"}},
		{"delete-selected", "b", "d", []string{"a", "b", "d", "e"}},
		{"delete-top", "c", "c", []string{"a", "c", "d", "e"}},
		{"shrink", "a", "a", []string{"a"}},
		{"empty", "", "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := stateModel("a", "b", "c", "d", "e")
			m.providerScroll, m.usageScroll, m.aliasOffset, m.aliasCursor = 1, 1, 1, 2
			m.providerCursor = 1
			m.payloadOffset, m.payloadCursor, m.blockOffset, m.blockCursor = 1, 2, 1, 2
			for _, name := range []string{"a", "b", "c", "d", "e"} {
				m.payloads = append(m.payloads, PayloadSummary{RequestID: name})
				m.blocks = append(m.blocks, BlockSummary{BlockID: name})
			}
			m.Update(snapshotMsg{snapshot: stateSnapshot(tc.names...)})
			p, b := payloadListMsg{enabled: true}, blockListMsg{enabled: true}
			for _, name := range tc.names {
				p.entries = append(p.entries, PayloadSummary{RequestID: name})
				b.entries = append(b.entries, BlockSummary{BlockID: name})
			}
			m.Update(p)
			m.Update(b)
			if len(tc.names) == 0 {
				if m.providerScroll != 0 || m.usageScroll != 0 || m.aliasCursor != 0 || m.aliasOffset != 0 || m.payloadCursor != 0 || m.payloadOffset != 0 || m.blockCursor != 0 || m.blockOffset != 0 {
					t.Fatal("empty lists must reset all anchors")
				}
				return
			}
			for name, got := range map[string]string{
				"providers": m.providerList()[m.providerScroll].Name,
				"usage":     m.filteredSummaries()[m.usageScroll].Model,
				"aliases":   m.aliasList()[m.aliasOffset].Name,
				"payloads":  m.payloadAt(m.payloadOffset).RequestID,
				"blocks":    m.blockAt(m.blockOffset).BlockID,
			} {
				if got != tc.top {
					t.Errorf("%s top = %q, want %q", name, got, tc.top)
				}
			}
			for name, got := range map[string]string{"aliases": m.aliasList()[m.aliasCursor].Name, "payloads": m.payloadAt(m.payloadCursor).RequestID, "blocks": m.blockAt(m.blockCursor).BlockID} {
				if got != tc.selected {
					t.Errorf("%s selected = %q, want %q", name, got, tc.selected)
				}
			}
		})
	}
}

func TestRefreshSelectionWinsConflictingTopAndDisabledProviderMove(t *testing.T) {
	m := stateModel("a", "b", "c", "d", "e")
	m.providerScroll, m.aliasOffset, m.aliasCursor = 1, 1, 2
	m.providerCursor = 1
	s := stateSnapshot("c", "a", "d", "e", "b")
	s.DisabledProviders, s.Providers = s.Providers[4:], s.Providers[:4]
	m.Update(snapshotMsg{snapshot: s})
	if m.providerList()[m.providerScroll].Name != "b" || m.aliasCursor != 0 || m.aliasOffset != 0 {
		t.Fatal("provider identity must survive enabled group change; selection must stay visible")
	}
}

func TestUsageIdentityIncludesEveryGroupingDimension(t *testing.T) {
	base := accounting.Summary{Model: "p/m", Tenant: "t", Client: "c", Operation: "chat", StatusCode: 200, Count: 20}
	for _, dimension := range []string{"tenant", "client", "model", "operation", "status"} {
		for _, upstream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/upstream=%t", dimension, upstream), func(t *testing.T) {
				other := base
				switch dimension {
				case "tenant":
					other.Tenant = "other"
				case "client":
					other.Client = "other"
				case "model":
					other.Model = "p/other"
				case "operation":
					other.Operation = "responses"
				case "status":
					other.StatusCode = 500
				}
				other.Count = 10
				makeSnapshot := func(rows ...accounting.Summary) *RuntimeSnapshot {
					s := stateSnapshot()
					u := s.Usage.(*remoteUsage)
					u.summaries = rows
					u.billing = &accounting.BillingSnapshot{}
					for _, row := range rows {
						parts := strings.SplitN(row.Model, "/", 2)
						u.billing.Upstream = append(u.billing.Upstream, accounting.UpstreamSummary{Tenant: row.Tenant, Client: row.Client, Provider: parts[0], Model: parts[1], Operation: row.Operation, StatusCode: row.StatusCode, Count: row.Count})
					}
					return s
				}
				m := stateModel()
				m.snapshot, m.usageUpstream, m.usageScroll = makeSnapshot(base, other), upstream, 1
				other.Count, other.TotalTokens = 100, 123
				m.Update(snapshotMsg{snapshot: makeSnapshot(other, base)})
				if m.usageScroll != 0 || usageIdentity(m.filteredSummaries()[0]) != usageIdentity(other) {
					t.Fatal("usage anchor conflated identities or included mutable counters")
				}
			})
		}
	}
}

func TestTenantRefreshKeepsNameOrFallsBackToAll(t *testing.T) {
	snapshot := func(names ...string) *RuntimeSnapshot {
		s := stateSnapshot(names...)
		for i := range s.Usage.(*remoteUsage).summaries {
			s.Usage.(*remoteUsage).summaries[i].Tenant = names[i]
		}
		return s
	}
	m := stateModel()
	m.snapshot = snapshot("b", "c")
	stateKey(m, "t")
	for _, s := range []*RuntimeSnapshot{snapshot("a", "b", "c"), snapshot("c", "b", "a"), snapshot("b", "c")} {
		m.Update(snapshotMsg{snapshot: s})
		if m.activeTenant() != "b" || len(m.filteredSummaries()) != 1 || m.filteredSummaries()[0].Tenant != "b" {
			t.Fatal("tenant identity moved with list index")
		}
	}
	m.Update(snapshotMsg{snapshot: snapshot("a", "c")})
	if m.activeTenant() != "" || m.tenantIndex != 0 || len(m.filteredSummaries()) != 2 {
		t.Fatal("removed tenant must select all, not a different tenant")
	}
	m.Update(snapshotMsg{snapshot: snapshot()})
	stateKey(m, "t")
	if m.activeTenant() != "" {
		t.Fatal("empty tenant list must stay all")
	}
}

func TestPayloadOldestFirstRefreshAndOrderKeepSelection(t *testing.T) {
	m := stateModel()
	m.payloads = []PayloadSummary{{RequestID: "d"}, {RequestID: "c"}, {RequestID: "b"}, {RequestID: "a"}}
	m.payloadOldestFirst, m.payloadCursor, m.payloadOffset = true, 2, 1
	m.Update(payloadListMsg{enabled: true, entries: []PayloadSummary{{RequestID: "e"}, {RequestID: "d"}, {RequestID: "c"}, {RequestID: "b"}, {RequestID: "a"}}})
	if m.payloadAt(m.payloadCursor).RequestID != "c" || m.payloadAt(m.payloadOffset).RequestID != "b" {
		t.Fatal("oldest-first refresh moved anchor")
	}
	m.togglePayloadOrder()
	if m.payloadAt(m.payloadCursor).RequestID != "c" {
		t.Fatal("explicit order change moved selection")
	}
}

func TestPauseBuffersLatestDataAndClockAtomically(t *testing.T) {
	m := stateModel("a", "b", "c")
	m.payloadFetcher, m.blockFetcher = payloadTestFetcher(), blockTestFetcher()
	m.payloads, m.blocks = []PayloadSummary{{RequestID: "old-p"}}, []BlockSummary{{BlockID: "old-b"}}
	m.payloadKnown, m.blockKnown = true, true
	p, b := m.requestPayloads(), m.requestBlocks()
	stateKey(m, "p")
	frozen, clock, rendered := m.snapshot, m.now, m.View().Content
	for i := 1; i <= 20; i++ {
		s := stateSnapshot("b", "a", "c")
		s.Version = fmt.Sprint(i)
		s.Usage.(*remoteUsage).rates = &accounting.RateSnapshot{WindowEnd: clock.Add(time.Duration(i) * time.Second), BucketSeconds: 1}
		m.Update(snapshotMsg{snapshot: s})
		m.Update(tickMsg(clock.Add(time.Duration(i) * time.Minute)))
	}
	m.Update(p())
	m.Update(b())
	if m.snapshot != frozen || m.now != clock || m.payloadAt(0).RequestID != "old-p" || m.blockAt(0).BlockID != "old-b" || m.View().Content != rendered {
		t.Fatal("paused arrival changed displayed data or clock")
	}
	if m.pending.Version != "20" || m.pausedResults.payloadList == nil || m.pausedResults.blockList == nil {
		t.Fatal("latest bounded results missing")
	}
	m.Update(ConnectionStatus{LastSuccess: clock, Reconnecting: true})
	if !strings.Contains(m.View().Content, "RECONNECTING") || m.now != clock {
		t.Fatal("connection status must remain live independently")
	}
	stateKey(m, "p")
	if m.snapshot.Version != "20" || m.now != clock.Add(20*time.Minute) || m.snapshot.Usage.(*remoteUsage).rates.WindowEnd != clock.Add(20*time.Second) || m.payloadAt(0).RequestID != "req-new" || m.blockAt(0).BlockID != blockTestFetcher().list.Blocks[0].BlockID {
		t.Fatal("resume did not apply latest data and measurement window together")
	}
	if m.pending != nil || m.hasPending || m.pauseSource != nil || !reflect.DeepEqual(m.pausedResults, pausedResults{}) || m.payloadLoading || m.blockLoading {
		t.Fatal("resume retained pending state")
	}
	snapshot := m.snapshot
	stateKey(m, "p")
	stateKey(m, "p")
	if m.snapshot != snapshot {
		t.Fatal("second resume reapplied stale snapshot")
	}
}

func TestPauseDetachesMutableViewersAndKeepsLocalNavigation(t *testing.T) {
	m := stateModel("a", "b", "c")
	u := accounting.NewAggregator()
	u.Record(accounting.Event{Model: "p/m", StatusCode: 200})
	logs := observability.NewLogBuffer(10)
	logs.Add(observability.LogEntry{Message: "before"})
	m.snapshot.Usage, m.snapshot.Logs = u, logs
	m.snapshot.Health = &remoteHealth{states: map[string]bool{"a": true}}
	m.clampScroll()
	stateKey(m, "p")
	before := rateLine(m.snapshot.Usage)
	u.Record(accounting.Event{Model: "p/m", StatusCode: 200})
	logs.Add(observability.LogEntry{Message: "after"})
	m.pauseSource.Health.(*remoteHealth).states["a"] = false
	m.Update(tickMsg(m.now.Add(time.Hour)))
	if m.snapshot.Usage.Summaries()[0].Count != 1 || len(m.filteredLogs(100)) != 1 || rateLine(m.snapshot.Usage) != before {
		t.Fatal("mutable viewers leaked through pause")
	}
	m.focus, m.bottomTab = focusBottom, bottomTabAliases
	stateKey(m, "down")
	if m.aliasCursor != 1 {
		t.Fatal("pause blocked local navigation")
	}
	stateKey(m, "p")
	if m.snapshot.Usage.Summaries()[0].Count != 2 || len(m.filteredLogs(100)) != 2 || m.health["a"] {
		t.Fatal("mutable viewer resume did not catch up")
	}
}

func TestStalePayloadFilterResponsesRejected(t *testing.T) {
	for _, oldFirst := range []bool{true, false} {
		t.Run(fmt.Sprint(oldFirst), func(t *testing.T) {
			m := stateModel()
			m.payloadFetcher = payloadTestFetcher()
			m.focus, m.bottomTab = focusBottom, bottomTabPayload
			old := m.requestPayloads()()
			fresh := stateKey(m, "s")
			if fresh == nil {
				t.Fatal("filter must supersede in-flight list")
			}
			if oldFirst {
				m.Update(old)
				if !m.payloadLoading || m.payloadKnown {
					t.Fatal("stale completion changed loading state")
				}
			}
			m.Update(fresh())
			m.Update(old)
			if len(m.payloads) != 1 || m.payloadAt(0).RequestID != "req-new" || !m.payloadErrorsOnly || m.payloadErr != "" {
				t.Fatal("obsolete list overwrote filtered view")
			}
			staleError := old.(payloadListMsg)
			staleError.err = "obsolete failure"
			m.Update(staleError)
			if m.payloadErr != "" {
				t.Fatal("obsolete error overwrote newer success")
			}
		})
	}
}

type cancelListFetcher struct {
	stubPayloadFetcher
	started chan context.Context
}

func (f *cancelListFetcher) ListPayloads(ctx context.Context, _ int, _ bool) (dashrpc.PayloadList, error) {
	f.started <- ctx
	<-ctx.Done()
	return dashrpc.PayloadList{}, ctx.Err()
}

func TestSupersededFilterCancelsRequestAndParentLifetime(t *testing.T) {
	f := &cancelListFetcher{started: make(chan context.Context, 2)}
	m := stateModel()
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	m.ctx, m.payloadFetcher = parent, f
	m.focus, m.bottomTab = focusBottom, bottomTabPayload
	results := make(chan tea.Msg, 2)
	cmd := m.requestPayloads()
	go func() { results <- cmd() }()
	first := <-f.started
	next := stateKey(m, "s")
	if !errors.Is(first.Err(), context.Canceled) {
		t.Fatal("superseded request context not canceled")
	}
	m.Update(<-results)
	go func() { results <- next() }()
	second := <-f.started
	if _, ok := second.Deadline(); !ok {
		t.Fatal("lost TUI-01 bounded deadline")
	}
	cancel()
	m.Update(<-results)
	if !errors.Is(second.Err(), context.Canceled) {
		t.Fatal("request survived parent cancellation")
	}
}

func TestDetailGenerationRejectsCloseReopenSameID(t *testing.T) {
	for _, blocks := range []bool{false, true} {
		t.Run(fmt.Sprint(blocks), func(t *testing.T) {
			m := stateModel()
			m.payloadFetcher, m.blockFetcher = payloadTestFetcher(), blockTestFetcher()
			tab := "3"
			if blocks {
				tab = "4"
			}
			m.Update(stateKey(m, tab)())
			old := stateKey(m, "enter")()
			stateKey(m, "esc")
			fresh := stateKey(m, "enter")()
			m.Update(old)
			if m.payloadDetailOpen() || m.blockDetailOpen() {
				t.Fatal("stale same-ID detail reopened view")
			}
			stateKey(m, "p")
			m.Update(fresh)
			if m.payloadDetailOpen() || m.blockDetailOpen() {
				t.Fatal("paused detail arrival changed view")
			}
			stateKey(m, "p")
			if blocks && !m.blockDetailOpen() || !blocks && !m.payloadDetailOpen() {
				t.Fatal("resume lost accepted detail")
			}
			m.Update(old)
			if blocks && m.blockPendingID != "" || !blocks && m.payloadPendingID != "" {
				t.Fatal("late result changed detail state")
			}
		})
	}
}

func TestPausedDetailCloseDiscardsBufferAndNoNewRPC(t *testing.T) {
	m := stateModel()
	m.payloadFetcher, m.blockFetcher = payloadTestFetcher(), blockTestFetcher()
	m.Update(stateKey(m, "4")())
	detail := stateKey(m, "enter")()
	stateKey(m, "p")
	m.Update(detail)
	stateKey(m, "esc")
	if m.pausedResults.blockDetail != nil {
		t.Fatal("closed detail retained captured data")
	}
	for _, key := range []string{"enter", "r", "a", "3", "s", "e", "r", "enter"} {
		if cmd := stateKey(m, key); cmd != nil {
			t.Fatalf("paused %q issued RPC", key)
		}
	}
	if m.payloadErrorsOnly {
		t.Fatal("paused remote filter changed")
	}
	_, tick := m.Update(tickMsg(m.now.Add(time.Minute)))
	if tick == nil {
		t.Fatal("pause stopped clock scheduling")
	}
	stateKey(m, "p")
	if m.blockDetailOpen() || m.blockPendingID != "" {
		t.Fatal("resume reopened closed take-once detail")
	}
}

func TestDecisionResultsBoundToRequestAndPausedDetail(t *testing.T) {
	m := stateModel()
	m.blockFetcher = blockTestFetcher()
	m.Update(stateKey(m, "4")())
	m.Update(stateKey(m, "enter")())
	old := stateKey(m, "a")()
	stateKey(m, "esc")
	m.Update(stateKey(m, "enter")())
	fresh := stateKey(m, "d")()
	m.Update(old)
	if m.blockDecisionPending != "deny" || m.blockDecisionMsg != "" {
		t.Fatal("late acknowledgment affected another detail session")
	}
	stateKey(m, "p")
	m.Update(fresh)
	if m.blockDecisionPending != "deny" || m.blockDecisionMsg != "" {
		t.Fatal("paused acknowledgment changed display")
	}
	stateKey(m, "p")
	if m.blockDecisionPending != "" || !strings.Contains(m.blockDecisionMsg, "deny recorded") {
		t.Fatal("resume lost acknowledgment")
	}
	duplicate := fresh.(blockDecisionMsg)
	duplicate.err = "duplicate error"
	m.Update(duplicate)
	if m.blockDecisionErr != "" {
		t.Fatal("duplicate decision result was applied twice")
	}
}

func TestLogSnapshotAnchorsFollowAndPause(t *testing.T) {
	makeSnapshot := func(seqs ...uint64) *RuntimeSnapshot {
		s := stateSnapshot()
		logs := &remoteLogs{}
		for _, seq := range seqs {
			logs.entries = append(logs.entries, observability.LogEntry{Seq: seq, Message: fmt.Sprint(seq)})
		}
		s.Logs = logs
		return s
	}
	for _, oldest := range []bool{false, true} {
		m := stateModel()
		m.snapshot, m.logOldestFirst = makeSnapshot(1, 2, 3, 4, 5), oldest
		m.clampLogCursor()
		m.Update(snapshotMsg{snapshot: makeSnapshot(1, 2, 3, 4, 5, 6)})
		if m.logCursorSeq != 6 || !m.logFollow {
			t.Fatal("follow did not track newest")
		}
		m.logFollow, m.logCursorSeq = false, 3
		m.logOffset, m.logCursor = 2, 3
		m.clampLogCursor()
		top := m.filteredLogs(100)[m.logOffset].Seq
		stateKey(m, "p")
		m.Update(snapshotMsg{snapshot: makeSnapshot(1, 2, 3, 4, 5, 6, 7)})
		if m.logCursorSeq != 3 || len(m.filteredLogs(100)) != 6 {
			t.Fatal("paused logs changed")
		}
		stateKey(m, "p")
		if m.logCursorSeq != 3 || m.logFollow || m.filteredLogs(100)[m.logOffset].Seq != top {
			t.Fatal("resume lost pinned log/top")
		}
		m.Update(snapshotMsg{snapshot: makeSnapshot(4, 5, 6, 7)})
		if m.logCursorSeq == 3 || m.logFollow {
			t.Fatal("expired pinned log fallback must remain pinned")
		}
	}
}

func TestPausedListsFreezeErrorsAndDisabledState(t *testing.T) {
	for _, failure := range []string{"", "fetch failed"} {
		m := stateModel()
		m.payloadKnown, m.payloadEnabled, m.blockKnown, m.blockEnabled = true, true, true, true
		m.payloads, m.blocks = []PayloadSummary{{RequestID: "p"}}, []BlockSummary{{BlockID: "b"}}
		stateKey(m, "p")
		m.Update(payloadListMsg{err: failure})
		m.Update(blockListMsg{err: failure})
		if !m.payloadKnown || !m.blockKnown || !m.payloadEnabled || !m.blockEnabled || m.payloadErr != "" || m.blockErr != "" {
			t.Fatal("paused error/disabled result changed displayed state")
		}
		stateKey(m, "p")
		if failure != "" {
			if m.payloadErr != failure || m.blockErr != failure {
				t.Fatal("resume lost error")
			}
		} else if m.payloadEnabled || m.blockEnabled || len(m.payloads) != 0 || len(m.blocks) != 0 {
			t.Fatal("resume lost disabled result")
		}
	}
}

func TestPausedStaleListsCannotReplaceCurrentBuffer(t *testing.T) {
	m := stateModel()
	m.payloadFetcher, m.blockFetcher = payloadTestFetcher(), blockTestFetcher()
	p, b := m.requestPayloads()(), m.requestBlocks()()
	m.Update(p)
	m.Update(b)
	newP, newB := m.requestPayloads()(), m.requestBlocks()()
	stateKey(m, "p")
	m.Update(newP)
	m.Update(newB)
	oldP, oldB := p.(payloadListMsg), b.(blockListMsg)
	oldP.err, oldB.err = "obsolete", "obsolete"
	m.Update(oldP)
	m.Update(oldB)
	if m.pausedResults.payloadList.err != "" || m.pausedResults.blockList.err != "" {
		t.Fatal("stale list replaced current buffer")
	}
	stateKey(m, "p")
	if m.payloadErr != "" || m.blockErr != "" {
		t.Fatal("resume accepted stale list")
	}
}

func TestUnvisitedPausedPaneLoadsOnResumeButFailedPaneNeedsRefresh(t *testing.T) {
	for _, tab := range []string{"3", "4"} {
		m := stateModel()
		m.payloadFetcher, m.blockFetcher = payloadTestFetcher(), blockTestFetcher()
		stateKey(m, "p")
		if stateKey(m, tab) != nil {
			t.Fatal("paused tab fetched")
		}
		cmd := stateKey(m, "p")
		if cmd == nil {
			t.Fatal("unvisited pane did not start loading on resume")
		}
		m.Update(cmd())
		m.payloadKnown, m.blockKnown = false, false
		m.payloadErr, m.blockErr = "denied", "denied"
		stateKey(m, "p")
		if stateKey(m, "p") != nil {
			t.Fatal("resume retried failed list without explicit refresh")
		}
	}
}

type takeOnceFetcher struct {
	stubBlockFetcher
	reads int
}

func (f *takeOnceFetcher) GetBlock(ctx context.Context, id string) (dashrpc.BlockCapture, error) {
	f.reads++
	capture, err := f.stubBlockFetcher.GetBlock(ctx, id)
	delete(f.detail, id)
	return capture, err
}

func TestTakeOnceDetailPauseResumeNeverReplaysRead(t *testing.T) {
	f := &takeOnceFetcher{stubBlockFetcher: *blockTestFetcher()}
	m := stateModel()
	m.blockFetcher = f
	m.Update(stateKey(m, "4")())
	result := stateKey(m, "enter")()
	stateKey(m, "p")
	m.Update(result)
	if stateKey(m, "p") != nil || f.reads != 1 || !m.blockDetailOpen() || m.blockDetailErr != "" {
		t.Fatal("resume replayed/lost consumed capture")
	}
	stateKey(m, "p")
	stateKey(m, "p")
	if f.reads != 1 {
		t.Fatal("pause cycle replayed capture")
	}
	stateKey(m, "esc")
	m.Update(stateKey(m, "enter")())
	if f.reads != 2 || m.blockDetailErr == "" {
		t.Fatal("explicit reopen must report consumed capture unavailable")
	}
}

type heldDetailFetcher struct {
	stubPayloadFetcher
	stubBlockFetcher
	started chan context.Context
}

func (f *heldDetailFetcher) GetPayload(ctx context.Context, _ string) (string, error) {
	f.started <- ctx
	<-ctx.Done()
	return "", ctx.Err()
}

func (f *heldDetailFetcher) GetBlock(ctx context.Context, _ string) (dashrpc.BlockCapture, error) {
	f.started <- ctx
	<-ctx.Done()
	return dashrpc.BlockCapture{}, ctx.Err()
}

func TestCloseDetailCancelsInFlightRequest(t *testing.T) {
	for _, tab := range []string{"3", "4"} {
		f := &heldDetailFetcher{stubPayloadFetcher: *payloadTestFetcher(), stubBlockFetcher: *blockTestFetcher(), started: make(chan context.Context, 1)}
		m := stateModel()
		m.payloadFetcher, m.blockFetcher = f, f
		m.Update(stateKey(m, tab)())
		cmd := stateKey(m, "enter")
		result := make(chan tea.Msg, 1)
		go func() { result <- cmd() }()
		ctx := <-f.started
		stateKey(m, "esc")
		if !errors.Is(ctx.Err(), context.Canceled) {
			t.Fatal("closing detail did not cancel request")
		}
		m.Update(<-result)
		if m.payloadDetailOpen() || m.blockDetailOpen() || m.payloadDetailErr != "" || m.blockDetailErr != "" {
			t.Fatal("canceled result reopened detail")
		}
	}
}
