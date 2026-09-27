package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/egose/aiproxy/internal/dashrpc"
)

type findingFetcher struct {
	*stubBlockFetcher
	calls []dashrpc.BlockDecisionRequest
	ids   []string
	err   error
	ack   *dashrpc.BlockDecisionResponse
}

func (f *findingFetcher) DecideBlock(_ context.Context, id, action string, shas []string) (dashrpc.BlockDecisionResponse, error) {
	f.calls = append(f.calls, dashrpc.BlockDecisionRequest{Action: action, FindingSHAs: append([]string(nil), shas...)})
	f.ids = append(f.ids, id)
	if f.ack != nil {
		return *f.ack, nil
	}
	return dashrpc.BlockDecisionResponse{Ok: true, Action: action, Count: len(shas)}, f.err
}

func findingModel(t *testing.T) (*model, *findingFetcher) {
	t.Helper()
	f := &findingFetcher{stubBlockFetcher: blockTestFetcher()}
	id := f.list.Blocks[0].BlockID
	c := f.detail[id]
	c.Findings = nil
	for i := range 3 {
		c.Findings = append(c.Findings, dashrpc.BlockFinding{
			RuleID: fmt.Sprintf("rule-%d", i), SecretSHA: strings.Repeat(fmt.Sprint(i+1), 64), Secret: fmt.Sprintf("secret-%d", i),
		})
	}
	f.detail[id] = c
	m := layoutModel()
	m.blockFetcher, m.blockKnown, m.blocks = f, false, nil
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	layoutApplyKey(m, "4")
	view := assertViewport(t, m)
	if !strings.Contains(view, "Enter consumes take-once") || !strings.Contains(view, "re-open unavailable") {
		t.Fatalf("missing pre-open consumption notice: %s", view)
	}
	layoutApplyKey(m, "enter")
	return m, f
}

func TestSelectedFindingExactHashesAndDuplicateActions(t *testing.T) {
	m, f := findingModel(t)
	for i, action := range []string{"allow", "redact", "deny"} {
		key := []string{"a", "s", "d"}[i]
		cmd := layoutKey(m, key)
		if cmd == nil {
			t.Fatal("missing selected decision")
		}
		for _, pendingKey := range []string{key, "a", "s", "d", "n", "N", "ctrl+r"} {
			if extra := layoutKey(m, pendingKey); extra != nil || m.blockFindingCursor != i {
				t.Fatalf("pending %s changed decision/selection", pendingKey)
			}
		}
		if view := assertViewport(t, m); !strings.Contains(view, "finding locked") || !strings.Contains(view, "SELECTED hash only") {
			t.Fatalf("pending scope invisible: %s", view)
		}
		result := cmd()
		m.Update(result)
		if len(f.calls) != i+1 || !reflect.DeepEqual(f.calls[i], dashrpc.BlockDecisionRequest{Action: action, FindingSHAs: []string{strings.Repeat(fmt.Sprint(i+1), 64)}}) || f.ids[i] != m.blockDetailID {
			t.Fatalf("wrong exact mutation: %+v %v", f.calls, f.ids)
		}
		if !strings.Contains(assertViewport(t, m), action+" recorded for selected hash") {
			t.Fatal("success not visible")
		}
		if layoutKey(m, key) != nil {
			t.Fatal("successful repeated action replayed")
		}
		duplicate := result.(blockDecisionMsg)
		duplicate.err = "late failure"
		m.Update(duplicate)
		if m.blockDecisionErr != "" {
			t.Fatal("duplicate result replaced success")
		}
		layoutApplyKey(m, "n")
	}
	if m.blockFindingCursor != 0 || layoutKey(m, "a") != nil || len(f.calls) != 3 {
		t.Fatal("returning to a finding lost successful action suppression")
	}
	layoutApplyKey(m, "N")
	if m.blockFindingCursor != 2 || layoutKey(m, "d") != nil {
		t.Fatal("reverse finding selection lost identity/status")
	}
}

func TestFindingFailuresRetryMissingSHAAndInvalidAcknowledgments(t *testing.T) {
	m, f := findingModel(t)
	f.err = errors.New("offline")
	layoutApplyKey(m, "s")
	if view := assertViewport(t, m); !strings.Contains(view, "decision failed; outcome unknown") || !strings.Contains(view, "deliberately retries") {
		t.Fatalf("failure/retry status hidden: %s", view)
	}
	f.err = nil
	layoutApplyKey(m, "s")
	if len(f.calls) != 2 || !reflect.DeepEqual(f.calls[0], f.calls[1]) || m.blockDecisionErr != "" {
		t.Fatal("retry changed hash/action or retained error")
	}
	for _, sha := range []string{"", "not-a-hash", strings.Repeat("A", 64), strings.Repeat("x", 64)} {
		m.blockDetail.Findings[0].SecretSHA = sha
		m.selectedDecisionStatus()
		m.dirty = true
		for _, key := range []string{"a", "s", "d"} {
			if layoutKey(m, key) != nil {
				t.Fatalf("invalid SHA %q issued decision", sha)
			}
		}
		if !strings.Contains(assertViewport(t, m), "decisions unavailable") {
			t.Fatal("invalid SHA not explained")
		}
	}
	for _, ack := range []dashrpc.BlockDecisionResponse{{}, {Ok: true, Action: "allow", Count: 2}, {Ok: true, Action: "deny", Count: 1}} {
		m, f = findingModel(t)
		f.ack = &ack
		layoutApplyKey(m, "a")
		if m.blockDecisionErr == "" || m.blockDecisionMsg != "" {
			t.Fatalf("unexpected acknowledgment reported success: %+v", ack)
		}
	}
	m, _ = findingModel(t)
	m.blockDetail.Findings = nil
	m.dirty = true
	if layoutKey(m, "a") != nil || layoutKey(m, "n") != nil || !strings.Contains(assertViewport(t, m), "no findings; decisions unavailable") {
		t.Fatal("empty capture enabled a decision")
	}
}

func TestFindingHashEquivalentSelectionSharesRecordedStatus(t *testing.T) {
	m, f := findingModel(t)
	m.blockDetail.Findings[1].SecretSHA = m.blockDetail.Findings[0].SecretSHA
	layoutApplyKey(m, "a")
	layoutApplyKey(m, "n")
	if layoutKey(m, "a") != nil || !strings.Contains(m.blockDecisionMsg, "allow recorded") {
		t.Fatal("same-hash finding lost recorded status or repeated successful action")
	}
	layoutApplyKey(m, "d")
	if len(f.calls) != 2 || f.calls[1].Action != "deny" || !reflect.DeepEqual(f.calls[1].FindingSHAs, f.calls[0].FindingSHAs) {
		t.Fatal("deliberate replacement changed hash scope")
	}
}

func TestFindingNavigationHelpAndNumbersNeverMutate(t *testing.T) {
	for _, key := range []string{"1", "2", "3", "4", "5", "[", "]", "tab", "shift+tab", "enter", "esc"} {
		m, f := findingModel(t)
		layoutApplyKey(m, key)
		if len(f.calls) != 0 || m.hasDetail() {
			t.Fatalf("navigation %s mutated or failed to close", key)
		}
	}
	m, f := findingModel(t)
	for _, key := range []string{"n", "N", "j", "k", "pgdown", "pgup", "end", "home"} {
		layoutApplyKey(m, key)
		assertViewport(t, m)
	}
	layoutApplyKey(m, "?")
	for _, key := range []string{"a", "s", "d", "n", "N", "1", "2", "3", "4", "5"} {
		layoutApplyKey(m, key)
	}
	if len(f.calls) != 0 || m.blockFindingCursor != 0 || !m.showHelp {
		t.Fatal("navigation/help changed decisions or selection")
	}
}

func TestFindingAsyncHashPauseAndReopenedGeneration(t *testing.T) {
	m, f := findingModel(t)
	old := layoutKey(m, "a")()
	wrongHash := old.(blockDecisionMsg)
	wrongHash.sha = m.blockDetail.Findings[1].SecretSHA
	m.Update(wrongHash)
	if m.blockDecisionPending != "allow" || m.blockDecisionMsg != "" {
		t.Fatal("wrong hash accepted for current generation")
	}
	layoutApplyKey(m, "p")
	m.Update(old)
	if m.pausedResults.blockDecision == nil || m.blockDecisionPending != "allow" {
		t.Fatal("paused result did not wait")
	}
	layoutApplyKey(m, "esc")
	if m.pausedResults.blockDecision != nil || m.blockDecisionResults != nil || m.blockDecisionSHA != "" {
		t.Fatal("close retained decision state")
	}
	layoutApplyKey(m, "p")
	layoutApplyKey(m, "enter")
	layoutApplyKey(m, "n")
	fresh := layoutKey(m, "d")()
	m.Update(old)
	if m.blockDecisionPending != "deny" || m.blockDecisionMsg != "" {
		t.Fatal("old same-capture result overwrote newer finding")
	}
	layoutApplyKey(m, "p")
	m.Update(fresh)
	if layoutKey(m, "d") != nil || m.blockDecisionMsg != "" {
		t.Fatal("paused decision repeated or changed visible status")
	}
	layoutApplyKey(m, "p")
	if m.blockDecisionPending != "" || !strings.Contains(m.blockDecisionMsg, "deny recorded") || len(f.calls) != 2 {
		t.Fatal("resume lost acknowledgment or replayed decision")
	}
	m.Update(old)
	if !strings.Contains(m.blockDecisionMsg, "deny recorded") {
		t.Fatal("late prior capture result replaced resumed result")
	}
}

func TestInspectionWrapPreservesEveryWithinCapCharacter(t *testing.T) {
	text := strings.Repeat("界👩‍💻é<0123456789>", 3000)
	text = text[:strings.LastIndex(text[:payloadDetailMaxOut-20], ">")+1] + "END-OF-CONTENT"
	if len(text) > payloadDetailMaxOut || !utf8.ValidString(text) {
		t.Fatal("invalid within-cap fixture")
	}
	for _, size := range [][2]int{{80, 7}, {80, 19}, {120, 25}, {160, 43}} {
		for _, headerCount := range []int{2, 3} {
			scroll, consumed := 0, 0
			var got strings.Builder
			headers := []string{"ID", "SELECTED hash only", "future matches; no replay"}[:headerCount]
			for {
				view := ansi.Strip(renderInspection(size[0], size[1], headers, text, "cap notice", &scroll))
				lines := strings.Split(view, "\n")
				if len(lines) != size[1] {
					t.Fatalf("height overflow: %s", view)
				}
				for _, line := range lines {
					if ansi.StringWidth(line) != size[0] || !utf8.ValidString(line) {
						t.Fatalf("invalid cells: %q", line)
					}
				}
				visible := size[1] - 3 - headerCount
				for i, line := range lines[headerCount+1 : len(lines)-2] {
					if scroll+i < consumed {
						continue
					}
					got.WriteString(strings.TrimRight(strings.TrimSuffix(strings.TrimPrefix(line, "│"), "│"), " "))
				}
				consumed = scroll + visible
				if strings.HasSuffix(got.String(), "END-OF-CONTENT") {
					break
				}
				previous := scroll
				scroll += visible
				if consumed > len(text) || (previous > 0 && got.Len() == 0) {
					t.Fatal("inspection did not make progress")
				}
			}
			if got.String() != text {
				t.Fatalf("%v headers=%d: content changed: got %d bytes, want %d", size, headerCount, got.Len(), len(text))
			}
		}
	}
}

func TestInspectionResizeScopeCapsAndControlBytes(t *testing.T) {
	m, _ := findingModel(t)
	text := strings.Repeat("界👩‍💻éabcdefgh", 30) + "FINAL-CONTENT"
	m.blockDetail.Findings[0].Secret = text
	m.blockDetail.Findings[0].Line = text
	for _, size := range [][2]int{{80, 12}, {80, 24}, {120, 30}, {160, 48}, {80, 12}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		layoutApplyKey(m, "end")
		view := assertViewport(t, m)
		for _, want := range []string{"Finding 1/3", "SELECTED hash only", "future matches", "no replay", "take-once consumed", "truncation unknown", "FINAL-CONTENT", "[a/s/d] selected"} {
			if !strings.Contains(view, want) {
				t.Fatalf("resize/scroll hides %q: %s", want, view)
			}
		}
		if m.blockFindingCursor != 0 {
			t.Fatal("resize changed finding")
		}
		layoutApplyKey(m, "home")
	}
	layoutApplyKey(m, "3")
	m.payloadDetailID, m.payloadPendingID = "payload-id", ""
	for _, body := range []string{
		`{"request":{"body":{"truncated":true}}}`,
		`{"upstream_request":{"body":{"truncated":true}}}`,
		`{"response":{"body":{"truncated":true}}}`,
	} {
		m.payloadDetail, m.dirty = body, true
		if !strings.Contains(assertViewport(t, m), "TRUNCATED capture") {
			t.Fatal("server capture truncation hidden")
		}
	}
	m.payloadDetail = prettyPayload([]byte(`{"text":"` + strings.Repeat("界", payloadDetailMaxOut) + `"}`))
	layoutApplyKey(m, "end")
	if !strings.Contains(assertViewport(t, m), "TRUNCATED pretty output (64 KiB)") {
		t.Fatal("pretty output cap hidden")
	}
	m.payloadDetail = "A\x1b[2JB\rC\tD\x00E" + string([]byte{0xff}) + "F"
	layoutApplyKey(m, "end")
	if view := assertViewport(t, m); !strings.Contains(view, `A\x1b[2JB\rC\tD\x00E\xffF`) {
		t.Fatalf("control/invalid bytes not safely inspectable: %s", view)
	}
}

func TestPayloadWithinCapJSONWrapPagingAndResize(t *testing.T) {
	m := layoutModel()
	layoutApplyKey(m, "3")
	layoutApplyKey(m, "enter")
	raw, err := json.Marshal(map[string]string{"text": strings.Repeat("界👩‍💻éJSON", 2000), "zz_end": "FINAL-CONTENT"})
	if err != nil {
		t.Fatal(err)
	}
	m.payloadDetail = prettyPayload(raw)
	if len(m.payloadDetail) >= payloadDetailMaxOut {
		t.Fatal("fixture must be entirely within cap")
	}
	id := m.payloadDetailID
	for _, size := range [][2]int{{80, 12}, {80, 24}, {120, 30}, {160, 48}, {80, 12}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		layoutApplyKey(m, "home")
		if view := assertViewport(t, m); !strings.Contains(view, "PAYLOAD "+id) || strings.Contains(view, "TRUNCATED") {
			t.Fatalf("within-cap metadata/truncation incorrect: %s", view)
		}
		layoutApplyKey(m, "pgdown")
		assertViewport(t, m)
		layoutApplyKey(m, "pgup")
		if m.payloadDetailScroll != 0 {
			t.Fatal("paging not reversible")
		}
		layoutApplyKey(m, "end")
		if view := assertViewport(t, m); !strings.Contains(view, "FINAL-CONTENT") || !strings.Contains(view, "PAYLOAD "+id) {
			t.Fatalf("long JSON end/metadata unavailable after resize: %s", view)
		}
	}
}
