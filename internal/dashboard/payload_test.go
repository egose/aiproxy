package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/dashrpc"
)

type stubPayloadFetcher struct {
	list    dashrpc.PayloadList
	listErr error
	detail  map[string]string
}

func (f *stubPayloadFetcher) ListPayloads(ctx context.Context, limit int, errorsOnly bool) (dashrpc.PayloadList, error) {
	if f.listErr != nil {
		return dashrpc.PayloadList{}, f.listErr
	}
	out := dashrpc.PayloadList{Enabled: f.list.Enabled}
	for _, s := range f.list.Payloads {
		if errorsOnly && !s.IsError() {
			continue
		}
		out.Payloads = append(out.Payloads, s)
		if len(out.Payloads) >= limit {
			break
		}
	}
	return out, nil
}

func (f *stubPayloadFetcher) GetPayload(ctx context.Context, requestID string) (string, error) {
	if s, ok := f.detail[requestID]; ok {
		return s, nil
	}
	return "", errors.New("not found")
}

func payloadTestSnapshot() *RuntimeSnapshot {
	snap := newSnapshot()
	snap.PayloadEnabled = true
	return snap
}

func payloadTestFetcher() *stubPayloadFetcher {
	return &stubPayloadFetcher{
		list: dashrpc.PayloadList{Enabled: true, Payloads: []dashrpc.PayloadSummary{
			{RequestID: "req-new", Method: "POST", Path: "/v1/chat/completions", PublicModel: "openai/gpt-4o", Status: 500, DurationMs: 340, Timestamp: "2026-09-13T10:01:00Z"},
			{RequestID: "req-old", Method: "POST", Path: "/v1/embeddings", PublicModel: "openai/text-emb", Status: 200, DurationMs: 40, Timestamp: "2026-09-13T10:00:00Z"},
		}},
		detail: map[string]string{
			"req-new": "{\n  \"request_id\": \"req-new\"\n}",
		},
	}
}

func TestPayloadTabSwitchTriggersFetch(t *testing.T) {
	snap := payloadTestSnapshot()
	mm := InitialModelWithPayloadFetcher(snap, payloadTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "3"}))
	mod := mm.(*model)
	if mod.bottomTab != bottomTabPayload {
		t.Fatalf("bottomTab = %v, want payload", mod.bottomTab)
	}
	if cmd == nil {
		t.Fatal("expected fetch cmd after switching to payload tab")
	}
	msg := cmd()
	list, ok := msg.(payloadListMsg)
	if !ok {
		t.Fatalf("cmd msg = %T, want payloadListMsg", msg)
	}
	mm, _ = mm.Update(list)
	mod = mm.(*model)
	if !mod.payloadKnown || len(mod.payloads) != 2 {
		t.Fatalf("payloads = %+v, want 2 known entries", mod.payloads)
	}
	if got := mm.View().Content; !strings.Contains(got, "PAYLOADS") || !strings.Contains(got, "gpt-4o") {
		t.Errorf("payload list not rendered:\n%s", got)
	}
}

func TestPayloadEnterOpensPrettyDetail(t *testing.T) {
	snap := payloadTestSnapshot()
	mm := InitialModelWithPayloadFetcher(snap, payloadTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "3"}))
	mm, _ = mm.Update(cmd())
	mm, cmd = mm.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	if cmd == nil {
		t.Fatal("expected detail fetch cmd on enter")
	}
	mm, _ = mm.Update(cmd())
	mod := mm.(*model)
	if mod.payloadDetailID != "req-new" {
		t.Fatalf("detail id = %q, want req-new", mod.payloadDetailID)
	}
	if got := mm.View().Content; !strings.Contains(got, `"request_id": "req-new"`) {
		t.Errorf("pretty detail not rendered:\n%s", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Code: tea.KeyEscape}))
	if mod := mm.(*model); mod.payloadDetailOpen() {
		t.Fatal("esc should close payload detail")
	}
}

func TestPayloadStatusFilter(t *testing.T) {
	snap := payloadTestSnapshot()
	mm := InitialModelWithPayloadFetcher(snap, payloadTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "3"}))
	mm, _ = mm.Update(cmd())
	mm, cmd = mm.Update(tea.KeyPressMsg(tea.Key{Text: "s"}))
	if cmd == nil {
		t.Fatal("expected refetch cmd on filter toggle")
	}
	mm, _ = mm.Update(cmd())
	mod := mm.(*model)
	if !mod.payloadErrorsOnly || len(mod.payloads) != 1 || mod.payloads[0].RequestID != "req-new" {
		t.Fatalf("filtered payloads = %+v", mod.payloads)
	}
}

func TestPayloadDisabledState(t *testing.T) {
	snap := newSnapshot()
	snap.PayloadEnabled = false
	disabled := &stubPayloadFetcher{list: dashrpc.PayloadList{Enabled: false}}
	mm := InitialModelWithPayloadFetcher(snap, disabled)
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "3"}))
	if cmd != nil {
		mm, _ = mm.Update(cmd())
	}
	if got := mm.View().Content; !strings.Contains(got, "payload log disabled") {
		t.Errorf("expected disabled notice:\n%s", got)
	}
}

func TestPayloadFetchError(t *testing.T) {
	snap := payloadTestSnapshot()
	fetcher := payloadTestFetcher()
	fetcher.listErr = errors.New("disk gone")
	mm := InitialModelWithPayloadFetcher(snap, fetcher)
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "3"}))
	mm, _ = mm.Update(cmd())
	if got := mm.View().Content; !strings.Contains(got, "fetch failed") {
		t.Errorf("expected fetch error:\n%s", got)
	}
}

func TestPayloadTabStripAndCycle(t *testing.T) {
	snap := payloadTestSnapshot()
	mm := InitialModelWithPayloadFetcher(snap, payloadTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	if got := mm.View().Content; !strings.Contains(got, "3:Payloads") {
		t.Fatalf("tab strip missing payloads tab:\n%s", got)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "["}))
	if mod := mm.(*model); mod.bottomTab != bottomTabAliases {
		t.Fatalf("[ from logs should reach aliases, got %v", mod.bottomTab)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "["}))
	if mod := mm.(*model); mod.bottomTab != bottomTabPayload {
		t.Fatalf("[ from aliases should reach payload, got %v", mod.bottomTab)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "]"}))
	if mod := mm.(*model); mod.bottomTab != bottomTabBlocks {
		t.Fatalf("] from payload should reach blocks, got %v", mod.bottomTab)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "]"}))
	if mod := mm.(*model); mod.bottomTab != bottomTabLogs {
		t.Fatalf("] from blocks should wrap to logs, got %v", mod.bottomTab)
	}
}

func TestSnapshotFromTransportCarriesPayloadFlag(t *testing.T) {
	snap := SnapshotFromTransport(dashrpc.Snapshot{PayloadEnabled: true})
	if !snap.PayloadEnabled {
		t.Fatal("PayloadEnabled not carried")
	}
	if SnapshotFromTransport(dashrpc.Snapshot{}).PayloadEnabled {
		t.Fatal("PayloadEnabled should default false")
	}
}

func TestPayloadOrderToggle(t *testing.T) {
	snap := payloadTestSnapshot()
	mm := InitialModelWithPayloadFetcher(snap, payloadTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "3"}))
	mm, _ = mm.Update(cmd())
	got := mm.View().Content
	if strings.Index(got, "gpt-4o") < 0 {
		t.Fatalf("missing payload rows:\n%s", got)
	}
	if strings.Index(got, "gpt-4o") > strings.Index(got, "text-emb") {
		t.Fatalf("default should be newest-first (req-new first):\n%s", got)
	}
	if !strings.Contains(got, "newest-first") {
		t.Fatalf("default should be newest-first:\n%s", got)
	}
	mod := mm.(*model)
	if mod.payloadAt(0).RequestID != "req-new" {
		t.Fatalf("newest-first first = %q, want req-new", mod.payloadAt(0).RequestID)
	}
	mm, _ = mm.Update(tea.KeyPressMsg(tea.Key{Text: "o"}))
	mod = mm.(*model)
	if !mod.payloadOldestFirst {
		t.Fatal("o should toggle payloads to oldest-first")
	}
	if mod.payloadAt(0).RequestID != "req-old" {
		t.Fatalf("oldest-first first = %q, want req-old", mod.payloadAt(0).RequestID)
	}
	mod.dirty = true
	if got := mm.View().Content; !strings.Contains(got, "oldest-first") {
		t.Fatalf("title should show order:\n%s", got)
	}
	mm, cmd = mm.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	if cmd == nil {
		t.Fatal("expected detail fetch cmd on enter in oldest-first")
	}
	mm, _ = mm.Update(cmd())
	if mod := mm.(*model); mod.payloadDetailID != "req-old" {
		t.Fatalf("detail id = %q, want req-old", mod.payloadDetailID)
	}
}

func TestPayloadCursorClamp(t *testing.T) {
	snap := payloadTestSnapshot()
	mm := InitialModelWithPayloadFetcher(snap, payloadTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "3"}))
	mm, _ = mm.Update(cmd())
	mod := mm.(*model)
	mod.payloadCursor = 99
	mod.clampPayloadCursor()
	if mod.payloadCursor != 1 {
		t.Fatalf("cursor = %d, want 1", mod.payloadCursor)
	}
	_ = time.Now
}
