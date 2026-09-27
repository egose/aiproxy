package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/dashrpc"
)

type takeOnceBlockFixture struct {
	list      []dashrpc.BlockSummary
	captures  map[string]dashrpc.BlockCapture
	listErr   error
	listCalls int
	getCalls  int
	gotIDs    []string
}

func (f *takeOnceBlockFixture) ListBlocks(ctx context.Context) (dashrpc.BlockList, error) {
	f.listCalls++
	if f.listErr != nil {
		return dashrpc.BlockList{}, f.listErr
	}
	return dashrpc.BlockList{Enabled: true, Blocks: append([]dashrpc.BlockSummary(nil), f.list...)}, nil
}

func (f *takeOnceBlockFixture) GetBlock(ctx context.Context, blockID string) (dashrpc.BlockCapture, error) {
	f.getCalls++
	f.gotIDs = append(f.gotIDs, blockID)
	c, ok := f.captures[blockID]
	if !ok {
		return dashrpc.BlockCapture{}, errors.New("block not found (expired or already consumed)")
	}
	delete(f.captures, blockID)
	return c, nil
}

func (f *takeOnceBlockFixture) DecideBlock(ctx context.Context, blockID, action string, shas []string) (dashrpc.BlockDecisionResponse, error) {
	if _, ok := f.captures[blockID]; !ok {
		if blockID == "" {
			return dashrpc.BlockDecisionResponse{}, errors.New("missing block")
		}
	}
	return dashrpc.BlockDecisionResponse{Ok: true, Action: action, Count: len(shas)}, nil
}

func newTakeOnceFixture() *takeOnceBlockFixture {
	mk := func(id string) (dashrpc.BlockSummary, dashrpc.BlockCapture) {
		sum := dashrpc.BlockSummary{BlockID: id, Timestamp: "2026-09-14T10:01:00Z", Operation: "responses", PublicModel: "alias/x", RuleIDs: []string{"generic-api-key"}, FindingCount: 1}
		cap := dashrpc.BlockCapture{BlockID: id, Operation: "responses", PublicModel: "alias/x", RuleIDs: []string{"generic-api-key"}, Findings: []dashrpc.BlockFinding{{RuleID: "generic-api-key", SecretSHA: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}}}
		return sum, cap
	}
	sA, cA := mk("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	sB, cB := mk("blk_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	return &takeOnceBlockFixture{
		list:     []dashrpc.BlockSummary{sA, sB},
		captures: map[string]dashrpc.BlockCapture{sA.BlockID: cA, sB.BlockID: cB},
	}
}

func mountBlocks(t *testing.T, f *takeOnceBlockFixture) (*model, tea.Cmd) {
	t.Helper()
	snap := newSnapshot()
	mm := InitialModelWithBlockFetcher(snap, nil, f)
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "4"}))
	if cmd == nil {
		t.Fatal("expected blocks fetch cmd on tab switch")
	}
	mm, _ = mm.Update(cmd())
	mod := mm.(*model)
	if !mod.blockKnown || len(mod.blocks) != 2 {
		t.Fatalf("initial list not visible: known=%v n=%d err=%q", mod.blockKnown, len(mod.blocks), mod.blockErr)
	}
	return mod, nil
}

func press(m *model, key tea.Key) (*model, tea.Cmd) {
	mm, cmd := m.Update(tea.KeyPressMsg(key))
	return mm.(*model), cmd
}

func TestHiddenBlockListIgnoresTakeOnceActions(t *testing.T) {
	f := newTakeOnceFixture()
	m, _ := mountBlocks(t, f)
	if f.listCalls != 1 {
		t.Fatalf("listCalls = %d, want 1", f.listCalls)
	}
	m, _ = press(m, tea.Key{Text: "j"})
	if m.blockCursor != 1 {
		t.Fatalf("cursor = %d, want 1 after visible j", m.blockCursor)
	}
	selected := m.blocks[m.blockCursor].BlockID

	_, refreshCmd := press(m, tea.Key{Text: "r"})
	if refreshCmd == nil {
		t.Fatal("expected manual refresh cmd")
	}
	if m.blockListVisible() {
		t.Fatal("list must be hidden while manual refresh is held")
	}
	heldCursor := m.blockCursor
	for _, k := range []tea.Key{{Text: "j"}, {Text: "k"}, {Code: tea.KeyDown}, {Code: tea.KeyUp}, {Text: "G"}, {Text: "g"}, {Text: "enter"}} {
		var cmd tea.Cmd
		m, cmd = press(m, k)
		if cmd != nil {
			t.Fatalf("hidden key %q issued a cmd", k.Text)
		}
	}
	if f.getCalls != 0 {
		t.Fatalf("getCalls = %d while hidden, want 0", f.getCalls)
	}
	if len(f.captures) != 2 {
		t.Fatalf("captures consumed while hidden: %d left, want 2", len(f.captures))
	}
	if m.blockCursor != heldCursor || m.blockPendingID != "" || m.blockDetailID != "" {
		t.Fatalf("hidden navigation changed selection: cursor=%d pending=%q detail=%q", m.blockCursor, m.blockPendingID, m.blockDetailID)
	}
	if got := m.View().Content; !strings.Contains(got, "[r] retry") {
		t.Fatalf("hidden loading view missing retry cue:\n%s", got)
	}
	if footer := m.renderContextFooter(); strings.Contains(footer, "[enter]") {
		t.Fatalf("hidden loading footer falsely offers enter: %q", footer)
	}

	m.Update(refreshCmd())
	if !m.blockListVisible() {
		t.Fatal("list must be visible after recovery")
	}
	if m.blockCursor != heldCursor || m.blocks[m.blockCursor].BlockID != selected {
		t.Fatalf("recovery lost selection: cursor=%d id=%q want %q", m.blockCursor, m.blocks[m.blockCursor].BlockID, selected)
	}
	var detailCmd tea.Cmd
	m, detailCmd = press(m, tea.Key{Text: "enter"})
	if detailCmd == nil {
		t.Fatal("expected detail cmd after recovery")
	}
	m.Update(detailCmd())
	if f.getCalls != 1 {
		t.Fatalf("getCalls = %d after recovery, want 1", f.getCalls)
	}
	if len(f.gotIDs) != 1 || f.gotIDs[0] != selected {
		t.Fatalf("detail opened %v, want exactly [%s]", f.gotIDs, selected)
	}
	if m.blockDetailID != selected {
		t.Fatalf("detail id = %q, want %q", m.blockDetailID, selected)
	}
	if len(f.captures) != 1 {
		t.Fatalf("captures left = %d, want 1 after single take", len(f.captures))
	}
}

func TestFailedBlockListIgnoresTakeOnceActions(t *testing.T) {
	f := newTakeOnceFixture()
	m, _ := mountBlocks(t, f)
	m, _ = press(m, tea.Key{Text: "j"})
	selected := m.blocks[m.blockCursor].BlockID
	heldCursor := m.blockCursor

	f.listErr = errors.New("unreachable")
	var errCmd tea.Cmd
	m, errCmd = press(m, tea.Key{Text: "r"})
	if errCmd == nil {
		t.Fatal("expected refresh cmd for error path")
	}
	m.Update(errCmd())
	if m.blockErr == "" {
		t.Fatal("expected block error")
	}
	if m.blockListVisible() {
		t.Fatal("list must be hidden while fetch has failed")
	}
	for _, k := range []tea.Key{{Text: "j"}, {Text: "k"}, {Code: tea.KeyDown}, {Text: "G"}, {Text: "enter"}} {
		var cmd tea.Cmd
		m, cmd = press(m, k)
		if cmd != nil {
			t.Fatalf("error-hidden key %q issued a cmd", k.Text)
		}
	}
	if f.getCalls != 0 {
		t.Fatalf("getCalls = %d while error-hidden, want 0", f.getCalls)
	}
	if len(f.captures) != 2 {
		t.Fatalf("captures consumed during error-hidden actions: %d left", len(f.captures))
	}
	if m.blockCursor != heldCursor || m.blockPendingID != "" {
		t.Fatalf("error-hidden navigation changed selection: cursor=%d pending=%q", m.blockCursor, m.blockPendingID)
	}
	if got := m.View().Content; !strings.Contains(got, "[r] retry") {
		t.Fatalf("error view missing retry cue:\n%s", got)
	}
	if footer := m.renderContextFooter(); strings.Contains(footer, "[enter]") {
		t.Fatalf("error footer falsely offers enter: %q", footer)
	}

	f.listErr = nil
	var retryCmd tea.Cmd
	m, retryCmd = press(m, tea.Key{Text: "r"})
	if retryCmd == nil {
		t.Fatal("expected retry cmd")
	}
	m.Update(retryCmd())
	if !m.blockListVisible() || m.blockErr != "" {
		t.Fatalf("recovery failed: known=%v err=%q", m.blockKnown, m.blockErr)
	}
	if m.blocks[m.blockCursor].BlockID != selected {
		t.Fatalf("recovery selection = %q, want %q", m.blocks[m.blockCursor].BlockID, selected)
	}
	var detailCmd tea.Cmd
	m, detailCmd = press(m, tea.Key{Text: "enter"})
	if detailCmd == nil {
		t.Fatal("expected detail cmd after error recovery")
	}
	m.Update(detailCmd())
	if f.getCalls != 1 || len(f.gotIDs) != 1 || f.gotIDs[0] != selected {
		t.Fatalf("recovery opened %v, want exactly [%s]", f.gotIDs, selected)
	}
}

func TestBackgroundBlockRefreshStaysActionable(t *testing.T) {
	f := newTakeOnceFixture()
	m, _ := mountBlocks(t, f)
	cmd := m.requestBlocks()
	if cmd == nil {
		t.Fatal("expected background refresh cmd while rows visible")
	}
	if !m.blockListVisible() {
		t.Fatal("background refresh must keep rows visible")
	}
	m, detailCmd := press(m, tea.Key{Text: "enter"})
	if detailCmd == nil {
		t.Fatal("background refresh must keep Enter actionable")
	}
	m.Update(detailCmd())
	if f.getCalls != 1 {
		t.Fatalf("getCalls = %d, want 1 during visible background load", f.getCalls)
	}
	m.Update(cmd())
	if m.blockErr != "" || !m.blockListVisible() {
		t.Fatalf("background list result broke visibility: err=%q", m.blockErr)
	}
}
