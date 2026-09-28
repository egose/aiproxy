package dashboard

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
)

func layoutModel() *model {
	names := make([]string, 40)
	for i := range names {
		names[i] = fmt.Sprintf("row-%02d界", i)
	}
	snap := stateSnapshot(names...)
	snap.Version = "layout"
	snap.Logs = &remoteLogs{}
	pf, bf := payloadTestFetcher(), blockTestFetcher()
	m := InitialModelWithBlockFetcher(snap, pf, bf).(*model)
	for i, name := range names {
		snap.Logs.(*remoteLogs).entries = append(snap.Logs.(*remoteLogs).entries, observability.LogEntry{
			Seq: uint64(i + 1), Level: slog.LevelInfo, Message: name, Attrs: strings.Repeat("界👩‍💻é", 50),
		})
		m.payloads = append(m.payloads, PayloadSummary{RequestID: name, PublicModel: name, Path: strings.Repeat("界", 90)})
		pf.detail[name] = strings.Repeat("界👩‍💻é long payload\n", 40)
		m.blocks = append(m.blocks, BlockSummary{BlockID: name, PublicModel: name, RuleIDs: []string{"test"}, FindingCount: 2})
		bf.detail[name] = BlockCapture{BlockID: name, PublicModel: name, Findings: []dashrpc.BlockFinding{
			{RuleID: "one", SecretSHA: "hash-one", Secret: strings.Repeat("界", 90)},
			{RuleID: "two", SecretSHA: "hash-two", Secret: strings.Repeat("é👩‍💻", 90)},
		}}
	}
	m.payloadKnown, m.payloadEnabled, m.blockKnown, m.blockEnabled = true, true, true, true
	m.connection = ConnectionStatus{LastSuccess: time.Unix(200, 0)}
	return m
}

func assertViewport(t *testing.T, m *model) string {
	t.Helper()
	view := m.View().Content
	lines := strings.Split(view, "\n")
	if len(lines) != m.height {
		t.Fatalf("%dx%d mode=%v focus=%v: got %d rows\n%s", m.width, m.height, m.inputMode(), m.focus, len(lines), view)
	}
	for i, line := range lines {
		if !utf8.ValidString(line) || ansi.StringWidth(line) != m.width {
			t.Fatalf("%dx%d row %d: width %d, invalid or unbounded: %q", m.width, m.height, i, ansi.StringWidth(line), line)
		}
	}
	for _, hint := range []string{"[?] help", "[esc] back", "[q/Ctrl+C] quit"} {
		if !strings.Contains(ansi.Strip(lines[len(lines)-2]), hint) {
			t.Fatalf("footer missing %q: %s", hint, view)
		}
	}
	if !strings.Contains(lines[len(lines)-1], "last OK") {
		t.Fatalf("live connection status hidden: %s", view)
	}
	return ansi.Strip(view)
}

func layoutKey(m *model, key string) tea.Cmd {
	code := map[string]rune{"tab": tea.KeyTab, "enter": tea.KeyEnter, "esc": tea.KeyEscape, "up": tea.KeyUp, "down": tea.KeyDown, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown, "home": tea.KeyHome, "end": tea.KeyEnd}
	k := tea.Key{Text: key}
	if c, ok := code[key]; ok {
		k = tea.Key{Code: c}
	} else if key == "shift+tab" {
		k = tea.Key{Code: tea.KeyTab, Mod: tea.ModShift}
	} else if key == "ctrl+c" {
		k = tea.Key{Code: 'c', Mod: tea.ModCtrl}
	} else if key == "ctrl+r" {
		k = tea.Key{Code: 'r', Mod: tea.ModCtrl}
	}
	_, cmd := m.Update(tea.KeyPressMsg(k))
	return cmd
}

func layoutApplyKey(m *model, key string) {
	if cmd := layoutKey(m, key); cmd != nil {
		m.Update(cmd())
	}
}

func TestViewportMatrixWithPopulatedPanesAndDetails(t *testing.T) {
	for _, size := range [][2]int{{80, 12}, {80, 24}, {120, 30}, {160, 48}, {250, 60}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			m := layoutModel()
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			for _, focus := range []focusArea{focusUsage, focusProviders, focusBottom} {
				m.focus, m.dirty = focus, true
				for _, zoom := range []bool{false, true} {
					m.zoomed, m.dirty = zoom, true
					got := assertViewport(t, m)
					if !strings.Contains(got, "row-") || !strings.Contains(got, "1:Aliases") || !strings.Contains(got, "4:Blocks") {
						t.Fatalf("unusable pane/tab layout: %s", got)
					}
					if m.focusedLayout() && focus != focusBottom && strings.Contains(got, "LOGS newest") {
						t.Fatal("focused layout rendered hidden pane")
					}
					layoutApplyKey(m, "end")
					assertViewport(t, m)
				}
			}
			for _, tab := range []string{"1", "2", "3", "4"} {
				layoutApplyKey(m, tab)
				layoutApplyKey(m, "home")
				assertViewport(t, m)
				layoutApplyKey(m, "down")
				layoutApplyKey(m, "enter")
				if !m.hasDetail() {
					t.Fatalf("tab %s did not open selected detail", tab)
				}
				assertViewport(t, m)
				layoutApplyKey(m, "end")
				got := assertViewport(t, m)
				if tab == "4" {
					lines := strings.Split(got, "\n")
					if !strings.Contains(lines[len(lines)-2], "[a/s/d] selected") {
						t.Fatal("scrolled block detail hid action scope")
					}
				}
				layoutApplyKey(m, "?")
				assertViewport(t, m)
				layoutApplyKey(m, "end")
				if got := assertViewport(t, m); !strings.Contains(got, "End of help") {
					t.Fatal("help cannot scroll to final line")
				}
				layoutApplyKey(m, "esc")
				if !m.hasDetail() {
					t.Fatal("help close discarded detail")
				}
				layoutApplyKey(m, "esc")
				assertViewport(t, m)
			}
		})
	}
}

func TestLongErrorsAndUnicodeStayInsideFrames(t *testing.T) {
	long := strings.Repeat("界👩‍💻é\tlong ", 70) + "\n" + strings.Repeat("another line\n", 70)
	for _, size := range [][2]int{{80, 12}, {80, 24}, {120, 30}, {160, 48}} {
		for _, mode := range []string{"header", "payload-list", "block-list", "payload-detail", "block-detail", "decision"} {
			t.Run(fmt.Sprint(size)+mode, func(t *testing.T) {
				m := layoutModel()
				m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
				m.connection.Reconnecting, m.connection.Message = true, long
				switch mode {
				case "header":
					m.snapshot.Version, m.snapshot.Address, m.snapshot.AuthMode = long, long, long
				case "payload-list":
					layoutApplyKey(m, "3")
					m.payloadErr = long
				case "block-list":
					layoutApplyKey(m, "4")
					m.blockErr = long
				case "payload-detail":
					layoutApplyKey(m, "3")
					layoutApplyKey(m, "enter")
					m.payloadDetailErr, m.payloadDetailID = long, long
				case "block-detail", "decision":
					layoutApplyKey(m, "4")
					layoutApplyKey(m, "enter")
					if mode == "decision" {
						m.blockDecisionErr = long
					} else {
						m.blockDetailErr, m.blockDetailID = long, long
					}
				}
				m.dirty = true
				assertViewport(t, m)
			})
		}
	}
	for _, text := range []string{"界界界", "ééé", "👩‍💻👩‍💻👩‍💻", "\x1b[31m界界界\x1b[0m"} {
		if got := truncate(text, 4); ansi.StringWidth(got) > 4 || !utf8.ValidString(got) {
			t.Fatalf("invalid cell truncation: %q", got)
		}
		for _, line := range wrapText(text, 4) {
			if ansi.StringWidth(line) > 4 {
				t.Fatalf("invalid cell wrap: %q", line)
			}
		}
	}
}

func TestHelpOwnsInputOverBlockDetail(t *testing.T) {
	m := layoutModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	layoutApplyKey(m, "4")
	layoutApplyKey(m, "enter")
	id, capture := m.blockDetailID, m.blockDetail
	layoutApplyKey(m, "?")
	before := []any{m.focus, m.bottomTab, m.aliasCursor, m.payloadCursor, m.blockCursor, m.zoomed, m.paused, m.tenantIndex, m.errorsOnly, m.usageUpstream, m.logFilterOn, m.payloadErrorsOnly, m.blockDecisionRequest.generation}
	for _, key := range []string{"a", "s", "d", "1", "2", "3", "4", "[", "]", "tab", "shift+tab", "p", "r", "o", "e", "t", "u", "l", "z", "+", "J"} {
		if cmd := layoutKey(m, key); cmd != nil {
			t.Fatalf("help issued command for %s", key)
		}
		assertViewport(t, m)
	}
	after := []any{m.focus, m.bottomTab, m.aliasCursor, m.payloadCursor, m.blockCursor, m.zoomed, m.paused, m.tenantIndex, m.errorsOnly, m.usageUpstream, m.logFilterOn, m.payloadErrorsOnly, m.blockDecisionRequest.generation}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(capture, m.blockDetail) || m.blockDetailID != id {
		t.Fatal("help mutated hidden pane state")
	}
	for _, key := range []string{"pgdown", "down", "up", "pgup", "end", "home"} {
		layoutApplyKey(m, key)
		assertViewport(t, m)
	}
	if m.helpScroll != 0 {
		t.Fatal("help Home did not restore top")
	}
	for _, key := range []string{"esc", "?", "h", "enter"} {
		layoutApplyKey(m, key)
		if m.showHelp || m.quit || m.blockDetailID != id {
			t.Fatalf("%s did not close only help", key)
		}
		layoutApplyKey(m, "?")
	}
	retries := 0
	m.retry = func() { retries++ }
	layoutApplyKey(m, "ctrl+r")
	if retries != 1 || !m.showHelp {
		t.Fatal("reserved retry unavailable in help")
	}
}

func TestNavigationResizeAndZoomPreserveSelectedIdentity(t *testing.T) {
	for _, tab := range []string{"1", "2", "3", "4"} {
		m := layoutModel()
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
		layoutApplyKey(m, tab)
		for i := 0; i < 12; i++ {
			layoutApplyKey(m, "down")
		}
		selection := []int{m.aliasCursor, m.logCursor, m.payloadCursor, m.blockCursor}
		layoutApplyKey(m, "z")
		layoutApplyKey(m, "enter")
		for _, size := range [][2]int{{120, 30}, {80, 24}, {160, 48}, {80, 12}} {
			m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
			assertViewport(t, m)
			layoutApplyKey(m, "?")
			layoutApplyKey(m, "end")
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
			assertViewport(t, m)
			layoutApplyKey(m, "esc")
		}
		layoutApplyKey(m, "esc")
		if m.hasDetail() || !m.zoomed || m.quit {
			t.Fatal("Esc should close detail before zoom")
		}
		layoutApplyKey(m, "esc")
		if m.zoomed || m.quit {
			t.Fatal("Esc should unzoom before quitting")
		}
		layoutApplyKey(m, "tab")
		if m.focus != focusProviders {
			t.Fatal("Tab from bottom did not focus providers")
		}
		assertViewport(t, m)
		layoutApplyKey(m, "shift+tab")
		if m.focus != focusBottom {
			t.Fatal("Shift-Tab is not inverse focus navigation")
		}
		layoutApplyKey(m, "]")
		layoutApplyKey(m, "[")
		if got := []int{m.aliasCursor, m.logCursor, m.payloadCursor, m.blockCursor}; !reflect.DeepEqual(got, selection) {
			t.Fatalf("tab %s lost selection: %v != %v", tab, got, selection)
		}
		got := assertViewport(t, m)
		var marker string
		switch m.bottomTab {
		case bottomTabAliases:
			marker = m.aliasList()[m.aliasCursor].Name
		case bottomTabLogs:
			marker = m.filteredLogs(1 << 30)[m.logCursor].Message
		case bottomTabPayload:
			marker = m.payloadAt(m.payloadCursor).PublicModel
		case bottomTabBlocks:
			marker = m.blockAt(m.blockCursor).BlockID
		}
		selectedVisible := false
		for _, line := range strings.Split(got, "\n") {
			if strings.Contains(line, "▸ ") && strings.Contains(line, marker) {
				selectedVisible = true
			}
		}
		if !selectedVisible {
			t.Fatalf("tab %s selected row %s is off-screen: %s", tab, marker, got)
		}
	}
}

func TestPendingDetailsResizeAndCloseInvalidateReads(t *testing.T) {
	for _, tab := range []string{"3", "4"} {
		for _, closeKey := range []string{"esc", "enter", "tab", "shift+tab", "1", "2", "3", "4", "[", "]"} {
			m := layoutModel()
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
			layoutApplyKey(m, tab)
			cmd := layoutKey(m, "enter")
			if cmd == nil {
				t.Fatal("detail request was not started")
			}
			assertViewport(t, m)
			layoutApplyKey(m, "?")
			m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
			assertViewport(t, m)
			layoutApplyKey(m, "esc")
			layoutApplyKey(m, closeKey)
			m.Update(cmd())
			if m.hasDetail() {
				t.Fatalf("%s %s late reply reopened closed detail", tab, closeKey)
			}
			assertViewport(t, m)
		}
	}
}

type layoutCountingFetcher struct {
	*stubBlockFetcher
	decisions int
}

func (f *layoutCountingFetcher) DecideBlock(ctx context.Context, id, action string, shas []string) (dashrpc.BlockDecisionResponse, error) {
	f.decisions++
	return f.stubBlockFetcher.DecideBlock(ctx, id, action, shas)
}

func TestNumberedTabsNeverDecideAndNavigationCancelsDetail(t *testing.T) {
	for _, key := range []string{"1", "2", "3", "4", "[", "]", "tab", "shift+tab"} {
		m := layoutModel()
		f := &layoutCountingFetcher{stubBlockFetcher: m.blockFetcher.(*stubBlockFetcher)}
		m.blockFetcher = f
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
		layoutApplyKey(m, "4")
		layoutApplyKey(m, "enter")
		generation := m.blockDetailRequest.generation
		m.pausedResults.blockDetail = &blockDetailMsg{}
		m.pausedResults.blockDecision = &blockDecisionMsg{}
		layoutApplyKey(m, key)
		if f.decisions != 0 || m.hasDetail() || m.blockDecisionPending != "" || m.blockDetailRequest.generation == generation || m.pausedResults.blockDetail != nil || m.pausedResults.blockDecision != nil {
			t.Fatalf("%s mutated or failed to close detail", key)
		}
		assertViewport(t, m)
	}
}

func TestHiddenPaneKeysAndUndersizedWarningCannotMutate(t *testing.T) {
	m := layoutModel()
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	layoutApplyKey(m, "4")
	for _, key := range []string{"t", "e", "u", "l", "o"} {
		if cmd := layoutKey(m, key); cmd != nil {
			t.Fatalf("hidden pane command for %s", key)
		}
	}
	if m.tenantIndex != 0 || m.errorsOnly || m.usageUpstream || m.logFilterOn || m.logOldestFirst {
		t.Fatal("hidden pane changed")
	}
	layoutApplyKey(m, "enter")
	m.Update(tea.WindowSizeMsg{Width: 70, Height: 10})
	for _, key := range []string{"a", "s", "d", "1", "p", "r", "tab"} {
		if cmd := layoutKey(m, key); cmd != nil {
			t.Fatalf("undersized view issued command for %s", key)
		}
	}
	if m.paused || m.blockDecisionPending != "" || !m.hasDetail() {
		t.Fatal("undersized warning allowed off-screen mutation")
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	assertViewport(t, m)
	for _, key := range []string{"tab", "tab"} {
		layoutApplyKey(m, key)
	}
	if m.focus != focusUsage {
		t.Fatal("expected usage focus")
	}
	layoutApplyKey(m, "enter")
	if m.zoomed || m.usageDetail == nil {
		t.Fatal("Usage Enter must inspect identity without zooming")
	}
}

func TestQuitKeysFromEveryInputMode(t *testing.T) {
	for _, mode := range []string{"browse", "zoom", "detail", "help", "small"} {
		for _, key := range []string{"q", "ctrl+c"} {
			m := layoutModel()
			m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
			switch mode {
			case "zoom":
				layoutApplyKey(m, "z")
			case "detail", "help":
				layoutApplyKey(m, "4")
				layoutApplyKey(m, "enter")
				if mode == "help" {
					layoutApplyKey(m, "?")
				}
			case "small":
				m.Update(tea.WindowSizeMsg{Width: 70, Height: 10})
			}
			if cmd := layoutKey(m, key); cmd == nil || !m.quit {
				t.Fatalf("%s in %s did not quit", key, mode)
			} else if _, ok := cmd().(tea.QuitMsg); !ok {
				t.Fatal("quit command did not terminate Program")
			}
		}
	}
}
