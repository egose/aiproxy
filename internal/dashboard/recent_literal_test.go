package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/app"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
)

const hostileMetadata = "bad\nFORGED\r\t\x1b[38;2;13;17;19mRED\x1b[0m\x1b[2J\x1b]52;c;ZGF0YQ==\a\x00\x7f\u009b31m界👩‍💻é\\n\\x1b[31m"

var ownedSGR = regexp.MustCompile(`^\x1b\[[0-9;:]*m`)

func metadataOwnedCodes(t *testing.T, m *model) map[string]bool {
	t.Helper()
	codes := map[string]bool{}
	width, height := m.width, m.height
	for _, size := range [][2]int{{80, 12}, {120, 30}, {160, 48}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, view := range []string{m.View().Content, renderLogEntry(20, 20, observability.LogEntry{Message: "owned fixture"})} {
			for _, code := range regexp.MustCompile(`\x1b\[[0-9;:]*m`).FindAllString(view, -1) {
				codes[code] = true
			}
		}
	}
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	if len(codes) == 0 {
		t.Fatal("fixture must capture legitimate UI-owned styling")
	}
	return codes
}

func assertLiteralTerminal(t *testing.T, raw string, codes map[string]bool) {
	t.Helper()
	for len(raw) > 0 {
		if raw[0] == '\x1b' {
			code := ownedSGR.FindString(raw)
			if code == "" || !codes[code] {
				t.Fatalf("data-owned terminal escape reached raw output: %q", raw)
			}
			raw = raw[len(code):]
			continue
		}
		r, n := utf8.DecodeRuneInString(raw)
		if (r == utf8.RuneError && n == 1) || (unicode.IsControl(r) && r != '\n') || r == '\u2028' || r == '\u2029' {
			t.Fatalf("data-owned control/invalid byte reached terminal: %q", raw)
		}
		raw = raw[n:]
	}
}

func assertLiteralRequestSelection(t *testing.T, m *model, codes map[string]bool) accounting.Event {
	t.Helper()
	assertLiteralTerminal(t, m.View().Content, codes)
	screen := assertViewport(t, m)
	rows := m.recentRequests()
	selected := rows[m.requestCursor]
	selectedLine := "▸ "
	if selected.Truncated != (accounting.RecentTruncation{}) {
		selectedLine += "[truncated] "
	}
	selectedLine += selected.Timestamp.Format("15:04:05") + fmt.Sprintf(" %d %s", selected.StatusCode, selected.RequestID)
	if strings.Count(screen, "│▸ ") != 1 || !strings.Contains(screen, "│"+selectedLine) {
		t.Fatalf("selected completion not visibly marked: %q\n%s", selectedLine, screen)
	}
	for i := m.requestOffset; i < min(len(rows), m.requestOffset+m.bottomVisibleRows()); i++ {
		if !strings.Contains(screen, rows[i].RequestID) {
			t.Fatalf("logical event %s displaced by injected rows\n%s", rows[i].RequestID, screen)
		}
	}
	layoutApplyKey(m, "enter")
	if m.requestDetail == nil || *m.requestDetail != selected {
		t.Fatal("Enter did not open the exact visibly selected completion")
	}
	assertLiteralTerminal(t, m.View().Content, codes)
	assertViewport(t, m)
	layoutApplyKey(m, "esc")
	return selected
}

func TestMetadataLiteralEncodingAndBounds(t *testing.T) {
	for _, tc := range []struct{ raw, want string }{
		{"ordinary/界👩‍💻é", "ordinary/界👩‍💻é"},
		{"\n\r\t\x1b\a\b\v\f\x00\x7f\u0085\u009b\u2028\u2029", `\n\r\t\x1b\a\b\v\f\x00\x7f\u0085\u009b\u2028\u2029`},
		{`\n\r\t\x1b`, `\\n\\r\\t\\x1b`},
		{"\xff\xfe", `\xff\xfe`},
		{"\"quoted\"", `\"quoted\"`},
		{strings.Repeat("a", 512), strings.Repeat("a", 512)},
		{strings.Repeat("a", 513), strings.Repeat("a", 512) + metadataDisplayClipped},
		{strings.Repeat("a", 510) + "界", strings.Repeat("a", 510) + metadataDisplayClipped},
		{strings.Repeat("a", 509) + "界", strings.Repeat("a", 509) + "界"},
		{strings.Repeat("\x00", 1<<20), strings.Repeat(`\x00`, 512) + metadataDisplayClipped},
	} {
		if got := metadataText(tc.raw); got != tc.want {
			t.Fatalf("literal encoding: got %q want %q", got, tc.want)
		}
	}
	for r := rune(0); r <= 0x9f; r++ {
		if unicode.IsControl(r) {
			got := metadataText(string(r))
			if strings.ContainsRune(got, r) || strings.ContainsAny(got, "\r\n\t\x1b") {
				t.Fatalf("control U+%04X was not literal: %q", r, got)
			}
		}
	}
}

func TestRequestMetadataAllFieldsLiteralAndInspectable(t *testing.T) {
	fields := []struct {
		name string
		set  func(*accounting.Event, string)
	}{
		{"request ID", func(e *accounting.Event, s string) { e.RequestID = s }},
		{"tenant", func(e *accounting.Event, s string) { e.Tenant = s }},
		{"client", func(e *accounting.Event, s string) { e.Client = s }},
		{"public model", func(e *accounting.Event, s string) { e.PublicModel = s }},
		{"legacy model", func(e *accounting.Event, s string) { e.Model = s }},
		{"provider", func(e *accounting.Event, s string) { e.Provider = s }},
		{"resolved model", func(e *accounting.Event, s string) { e.UpstreamModel = s }},
		{"operation", func(e *accounting.Event, s string) { e.Operation = s }},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			m := requestModel()
			codes := metadataOwnedCodes(t, m)
			e := accounting.Event{RequestID: "identity", Model: "p/m", Timestamp: time.Unix(1, 0), StatusCode: 404}
			field.set(&e, hostileMetadata)
			m.snapshot.Usage = &remoteUsage{recent: []accounting.Event{e}}
			m.dirty = true
			assertLiteralTerminal(t, m.View().Content, codes)
			assertViewport(t, m)
			pane := ansi.Strip(renderRequests(m, 2000, 5))
			if len(strings.Split(pane, "\n")) != 5 {
				t.Fatal("metadata added a physical request row")
			}
			if field.name == "request ID" || field.name == "client" || field.name == "tenant" || field.name == "public model" || field.name == "legacy model" {
				if !strings.Contains(pane, metadataText(hostileMetadata)) {
					t.Fatal("list lost escaped metadata or printable Unicode")
				}
			}
			layoutApplyKey(m, "enter")
			_, lines := m.metadataLines()
			if !strings.Contains(strings.Join(lines, "\n"), metadataText(hostileMetadata)) {
				t.Fatal("detail lost literal metadata")
			}
			for _, line := range lines {
				if strings.ContainsAny(line, "\n\r\t\x1b") {
					t.Fatalf("metadata composed before escaping: %q", line)
				}
			}
			for i := 0; i < 40; i++ {
				assertLiteralTerminal(t, m.View().Content, codes)
				assertViewport(t, m)
				layoutApplyKey(m, "down")
			}
			if *m.requestDetail != e || m.snapshot.Usage.Recent(1)[0] != e {
				t.Fatal("rendering changed stored/selected identity")
			}
		})
	}
	m := requestModel()
	codes := metadataOwnedCodes(t, m)
	a := accounting.NewAggregator()
	a.Record(accounting.Event{RequestID: "long-model", PublicModel: strings.Repeat("\x00", 506) + "END界" + strings.Repeat("z", 200), StatusCode: 404})
	m.snapshot.Usage = &remoteUsage{recent: a.Recent(1)}
	m.dirty = true
	if !strings.Contains(m.View().Content, "[truncated]") {
		t.Fatal("row retention marker lost")
	}
	assertLiteralTerminal(t, m.View().Content, codes)
	layoutApplyKey(m, "enter")
	_, lines := m.metadataLines()
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, metadataDisplayClipped) || !strings.Contains(joined, "END界") || !strings.Contains(joined, "Truncated fields (retained prefixes): PublicModel") {
		t.Fatal("detail must preserve every retained byte and EDGE-01 guidance")
	}
	seenEnd := false
	for i := 0; i < 80; i++ {
		assertLiteralTerminal(t, m.View().Content, codes)
		screen := assertViewport(t, m)
		seenEnd = seenEnd || strings.Contains(screen, "END界")
		layoutApplyKey(m, "down")
	}
	if !seenEnd {
		t.Fatal("escaped retained tail not reachable by detail scrolling")
	}
	m.requestDetail = &accounting.Event{RequestID: strings.Repeat("\x00", 10000), PublicModel: strings.Repeat("界", 10000)}
	_, lines = m.metadataLines()
	if strings.Count(strings.Join(lines, "\n"), metadataDisplayClipped) != 2 {
		t.Fatal("legacy detail must explicitly disclose presentation clipping")
	}
}

func TestLiteralMetadataSearchAndExactCorrelation(t *testing.T) {
	m := requestModel()
	codes := metadataOwnedCodes(t, m)
	actualID := "key\n\x1b[38;2;13;17;19m"
	literalID := `key\n\x1b[38;2;13;17;19m`
	actual := accounting.Event{RequestID: actualID, PublicModel: "p/\n\x1b", StatusCode: 404}
	literal := accounting.Event{RequestID: literalID, PublicModel: `p/\n\x1b`, StatusCode: 404}
	m.snapshot.Usage = &remoteUsage{recent: []accounting.Event{actual, literal}}
	for _, query := range []string{`model:\n`, `model:\x1b`} {
		applySearch(m, query)
		rows := m.recentRequests()
		if len(rows) != 1 || rows[0] != literal {
			t.Fatal("search confused actual controls with literal backslash data")
		}
	}
	m.setSearch("model:\x1b")
	if rows := m.recentRequests(); len(rows) != 1 || rows[0] != actual {
		t.Fatal("matching mutated raw retained controls")
	}
	assertLiteralTerminal(t, m.View().Content, codes)
	searchKey(m, "ctrl+u")
	m.requestDetail = &actual
	m.snapshot.Logs = &remoteLogs{entries: []observability.LogEntry{
		{Seq: 1, RequestID: actualID, Message: "exact match"},
		{Seq: 2, RequestID: literalID, Message: "literal decoy"},
	}}
	f := payloadTestFetcher()
	f.list.Payloads = []PayloadSummary{
		{RequestID: actualID, PublicModel: hostileMetadata, Path: hostileMetadata, Method: "POST", Status: 404},
		{RequestID: literalID, PublicModel: "decoy", Status: 404},
	}
	f.detail[actualID] = "exact captured body\nsecond body line"
	m.payloadFetcher, m.payloadEnabled = f, true
	for _, tab := range []string{"l", "v"} {
		layoutApplyKey(m, tab)
		if m.correlation == nil || m.correlation.event != actual {
			t.Fatal("correlation mutated raw request identity")
		}
		if label := m.searchLabel(m.bottomTab); label != "ID="+metadataText(actualID)+" [esc] Requests" {
			t.Fatalf("unsafe correlation label: %q", label)
		}
		if tab == "l" {
			if rows := m.filteredLogs(200); len(rows) != 1 || rows[0].RequestID != actualID {
				t.Fatal("log correlation matched literal decoy")
			}
		} else if rows := m.orderedPayloads(); len(rows) != 1 || rows[0].RequestID != actualID {
			t.Fatal("payload correlation matched literal decoy")
		}
		assertLiteralTerminal(t, m.View().Content, codes)
		assertViewport(t, m)
		layoutApplyKey(m, "enter")
		assertLiteralTerminal(t, m.View().Content, codes)
		if tab == "v" && (m.payloadDetailID != actualID || m.payloadDetail != f.detail[actualID]) {
			t.Fatal("payload Enter did not fetch exact raw ID")
		}
		layoutApplyKey(m, "esc")
		layoutApplyKey(m, "esc")
		if m.requestDetail == nil || *m.requestDetail != actual {
			t.Fatal("correlation return lost original request")
		}
	}
}

func TestLiteralSiblingMetadataPresentation(t *testing.T) {
	m := requestModel()
	codes := metadataOwnedCodes(t, m)
	m.requestDetail = nil
	m.usageDetail = &accounting.Summary{Tenant: hostileMetadata, Client: hostileMetadata, Model: hostileMetadata, Operation: hostileMetadata}
	_, lines := m.metadataLines()
	if strings.Count(strings.Join(lines, "\n"), metadataText(hostileMetadata)) != 4 {
		t.Fatal("shared usage identity detail failed literal rendering")
	}
	for i := 0; i < 30; i++ {
		m.dirty = true
		assertLiteralTerminal(t, m.View().Content, codes)
		assertViewport(t, m)
		layoutApplyKey(m, "down")
	}
	for _, public := range []string{"", hostileMetadata} {
		row := payloadRow(PayloadSummary{Timestamp: hostileMetadata, Method: hostileMetadata, PublicModel: public, Provider: hostileMetadata, UpstreamModel: hostileMetadata, Path: hostileMetadata}, 1000, 2000, 1000)
		assertLiteralTerminal(t, row, nil)
		if strings.Contains(row, "\n") || !strings.Contains(row, metadataText(hostileMetadata)) {
			t.Fatal("payload metadata/fallback added rows or lost literal Unicode")
		}
	}
}
func TestRejectedControlModelHTTPRenderSelection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.hcl")
	text := `
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
logging {
  level = "error"
  access_log = false
}
dashboard { token = "dashboard-secret-fixture" }
provider "openai-compatible" "origin" {
  base_url = "http://127.0.0.1:1"
  api_key = "fixture"
  model "public" {}
}
`
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	a, err := app.Build(t.Context(), app.BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	srv := httptest.NewServer(a.Server.Handler)
	defer srv.Close()
	post := func(i int) {
		t.Helper()
		body, err := json.Marshal(map[string]any{"model": "missing/" + hostileMetadata, "messages": []any{}})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), "POST", srv.URL+"/v1/chat/completions", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Request-Id", fmt.Sprintf("edge-%02d", i))
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("rejected status %d", resp.StatusCode)
		}
	}
	fetch := func() dashrpc.Snapshot {
		t.Helper()
		req, err := http.NewRequestWithContext(t.Context(), "GET", srv.URL+dashrpc.SnapshotPath, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer dashboard-secret-fixture")
		resp, err := srv.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var wire dashrpc.Snapshot
		if resp.StatusCode != 200 {
			t.Fatalf("snapshot status %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&wire); err != nil {
			t.Fatal(err)
		}
		return wire
	}
	for i := 0; i < 20; i++ {
		post(i)
	}
	wire := fetch()
	if wire.PayloadEnabled || len(wire.Logs) != 0 || len(wire.Recent) != 20 {
		t.Fatal("fixture must retain all completions without logs or payload capture")
	}
	for _, e := range wire.Recent {
		if e.PublicModel != "missing/"+hostileMetadata || e.Truncated != (accounting.RecentTruncation{}) || e.Model != "_unresolved_model" || e.Provider != "" || e.UpstreamModel != "" {
			t.Fatalf("actual control metadata changed before presentation: %+v", e)
		}
	}
	m := requestModel()
	codes := metadataOwnedCodes(t, m)
	m.Update(snapshotMsg{snapshot: SnapshotFromTransport(wire)})
	for _, size := range [][2]int{{80, 12}, {80, 24}, {120, 30}, {160, 48}, {80, 12}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		for _, key := range []string{"home", "down", "pgdown", "end", "up"} {
			layoutApplyKey(m, key)
			assertLiteralRequestSelection(t, m, codes)
		}
	}
	selected := m.recentRequests()[m.requestCursor]
	post(20)
	m.Update(snapshotMsg{snapshot: SnapshotFromTransport(fetch())})
	if got := assertLiteralRequestSelection(t, m, codes); got != selected {
		t.Fatal("HTTP refresh moved the selected raw completion identity")
	}
	applySearch(m, "model:FORGED status:404 op:chat")
	layoutApplyKey(m, "e")
	layoutApplyKey(m, "down")
	assertLiteralRequestSelection(t, m, codes)
	applySearch(m, `model:\n`)
	if len(m.recentRequests()) != 21 {
		t.Fatal("literal backslash search no longer matches retained literal text")
	}
	assertLiteralRequestSelection(t, m, codes)
	searchKey(m, "ctrl+u")
	assertLiteralRequestSelection(t, m, codes)
	t.Log("21 real rejected loopback HTTP requests; exact controls retained through authenticated RPC; compact/normal/resize/paging/refresh/search/filter Enter identities and raw terminal bytes verified")
}
