package dashboard

import (
	"context"
	"errors"
	"strings"
	"testing"

	"charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/dashrpc"
)

type stubBlockFetcher struct {
	list    dashrpc.BlockList
	listErr error
	detail  map[string]dashrpc.BlockCapture
}

func (f *stubBlockFetcher) ListBlocks(ctx context.Context) (dashrpc.BlockList, error) {
	if f.listErr != nil {
		return dashrpc.BlockList{}, f.listErr
	}
	return f.list, nil
}

func (f *stubBlockFetcher) GetBlock(ctx context.Context, blockID string) (dashrpc.BlockCapture, error) {
	if c, ok := f.detail[blockID]; ok {
		return c, nil
	}
	return dashrpc.BlockCapture{}, errors.New("block not found (expired or already consumed)")
}

func blockTestFetcher() *stubBlockFetcher {
	return &stubBlockFetcher{
		list: dashrpc.BlockList{Enabled: true, Blocks: []dashrpc.BlockSummary{
			{BlockID: "blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Timestamp: "2026-09-14T10:01:00Z", Operation: "responses", PublicModel: "alias/x", RuleIDs: []string{"generic-api-key"}, FindingCount: 1},
		}},
		detail: map[string]dashrpc.BlockCapture{
			"blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": {
				BlockID: "blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Operation: "responses", PublicModel: "alias/x",
				RuleIDs: []string{"generic-api-key"},
				Findings: []dashrpc.BlockFinding{
					{RuleID: "generic-api-key", Secret: "sk-test-secret", Match: "sk-test-secret"},
				},
			},
		},
	}
}

func TestBlocksTabSwitchTriggersFetch(t *testing.T) {
	snap := newSnapshot()
	mm := InitialModelWithBlockFetcher(snap, nil, blockTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "4"}))
	mod := mm.(*model)
	if mod.bottomTab != bottomTabBlocks {
		t.Fatalf("bottomTab = %v, want blocks", mod.bottomTab)
	}
	if cmd == nil {
		t.Fatal("expected blocks fetch cmd on tab switch")
	}
	mm, _ = mm.Update(cmd())
	if got := mm.View().Content; !strings.Contains(got, "blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa") {
		t.Fatalf("missing block row:\n%s", got)
	}
}

func TestBlocksTabStripShowsCount(t *testing.T) {
	snap := newSnapshot()
	mm := InitialModelWithBlockFetcher(snap, nil, blockTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "4"}))
	mm, _ = mm.Update(cmd())
	if got := mm.View().Content; !strings.Contains(got, "4:Blocks(1)") {
		t.Fatalf("tab strip missing blocks count:\n%s", got)
	}
}

func TestBlockDetailShowsCapture(t *testing.T) {
	snap := newSnapshot()
	mm := InitialModelWithBlockFetcher(snap, nil, blockTestFetcher())
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "4"}))
	mm, _ = mm.Update(cmd())
	mm, cmd = mm.Update(tea.KeyPressMsg(tea.Key{Text: "enter"}))
	if cmd == nil {
		t.Fatal("expected detail fetch cmd on enter")
	}
	mm, _ = mm.Update(cmd())
	mod := mm.(*model)
	if mod.blockDetailID != "blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" {
		t.Fatalf("detail id = %q", mod.blockDetailID)
	}
	if got := mm.View().Content; !strings.Contains(got, "sk-test-secret") {
		t.Fatalf("detail missing captured secret:\n%s", got)
	}
	if got := mm.View().Content; !strings.Contains(got, "take-once") {
		t.Fatalf("detail missing take-once notice:\n%s", got)
	}
}

func TestBlocksDisabledNotice(t *testing.T) {
	snap := newSnapshot()
	disabled := &stubBlockFetcher{list: dashrpc.BlockList{Enabled: false}}
	mm := InitialModelWithBlockFetcher(snap, nil, disabled)
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "4"}))
	mm, _ = mm.Update(cmd())
	if got := mm.View().Content; !strings.Contains(got, "quarantine disabled") {
		t.Errorf("expected disabled notice:\n%s", got)
	}
}

func TestBlocksFetchError(t *testing.T) {
	snap := newSnapshot()
	fetcher := blockTestFetcher()
	fetcher.listErr = errors.New("unreachable")
	mm := InitialModelWithBlockFetcher(snap, nil, fetcher)
	mm, _ = mm.Update(tea.WindowSizeMsg{Width: 140, Height: 30})
	mm, cmd := mm.Update(tea.KeyPressMsg(tea.Key{Text: "4"}))
	mm, _ = mm.Update(cmd())
	if got := mm.View().Content; !strings.Contains(got, "fetch failed") {
		t.Errorf("expected fetch error:\n%s", got)
	}
}
