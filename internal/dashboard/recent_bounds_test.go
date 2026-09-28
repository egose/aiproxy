package dashboard

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/app"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
)

func TestRejectedLargeModelHTTPRecentBounds(t *testing.T) {
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
	client := srv.Client()
	for i := 0; i < 205; i++ {
		size := 32 << 10
		if i == 204 {
			size = 1 << 20
		}
		public := "missing/" + strings.Repeat("界", size/3) + fmt.Sprint(i)
		body, err := json.Marshal(map[string]any{"model": public, "messages": []any{}})
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), "POST", srv.URL+"/v1/chat/completions", strings.NewReader(string(body)))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-Request-Id", fmt.Sprintf("bounded-%d", i))
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("large model response %d", resp.StatusCode)
		}
	}
	req, err := http.NewRequestWithContext(t.Context(), "GET", srv.URL+dashrpc.SnapshotPath, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer dashboard-secret-fixture")
	resp, err := client.Do(req)
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
	if wire.PayloadEnabled || len(wire.Logs) != 0 {
		t.Fatalf("logging fixture not isolated: payload=%v logs=%d", wire.PayloadEnabled, len(wire.Logs))
	}
	if len(wire.Recent) != 200 || wire.Recent[0].RequestID != "bounded-5" || wire.Recent[199].RequestID != "bounded-204" {
		t.Fatal("HTTP completion count/eviction lost")
	}
	for _, e := range wire.Recent {
		if e.Truncated != (accounting.RecentTruncation{PublicModel: true}) || len(e.PublicModel) > accounting.RecentModelBytes || e.Model != "_unresolved_model" || e.Provider != "" || e.UpstreamModel != "" || e.StatusCode != 404 || e.Duration <= 0 {
			t.Fatalf("invalid rejected diagnostic: %+v", e)
		}
	}
	if len(wire.Usage) != 1 || wire.Usage[0].Count != 205 || wire.Usage[0].Model != "_unresolved_model" || wire.Usage[0].TotalTokens != 0 {
		t.Fatalf("rejected billing aggregation changed: %+v", wire.Usage)
	}
	data, err := json.Marshal(wire.Recent)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 200*(accounting.RecentEntryJSONBytes+1)+2 {
		t.Fatalf("recent wire bytes=%d", len(data))
	}
	t.Logf("205 rejected HTTP requests (204 x ~32KiB, 1 x ~1MiB); logs=0 payload=false; retained=200; recent JSON=%d bytes; one billing sentinel count=205", len(data))
	m := InitialModel(SnapshotFromTransport(wire)).(*model)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	layoutApplyKey(m, "5")
	if !strings.Contains(m.View().Content, "[truncated]") {
		t.Fatal("truncation not visible in compact list")
	}
	layoutApplyKey(m, "enter")
	_, lines := m.metadataLines()
	if !strings.Contains(strings.Join(lines, "\n"), "Truncated fields (retained prefixes): PublicModel") {
		t.Fatal("detail truncation guidance lost through HTTP/RPC")
	}
}

func TestRecentRPCIdentityTruncationCorrelationAndP95(t *testing.T) {
	now := time.Now().UTC()
	a := accounting.NewAggregatorWithClock(func() time.Time { return now })
	for i := 0; i < 2; i++ {
		e := accounting.Event{Timestamp: now, RequestID: strings.Repeat("r", 300) + fmt.Sprint(i),
			PublicModel: strings.Repeat("m", 600) + fmt.Sprint(i), Model: "alias/a", Provider: strings.Repeat("p", 300) + fmt.Sprint(i),
			Duration: time.Duration(i+1) * time.Second, StatusCode: 200}
		a.Record(e)
	}
	transport := func() dashrpc.Snapshot {
		data, err := json.Marshal(dashrpc.Build("test", ":0", "none", now, config.Catalog{}, a, nil, nil, 200))
		if err != nil {
			t.Fatal(err)
		}
		var wire dashrpc.Snapshot
		if err := json.Unmarshal(data, &wire); err != nil {
			t.Fatal(err)
		}
		return wire
	}
	wire := transport()
	if wire.Recent[0].RequestID != wire.Recent[1].RequestID || requestIdentity(wire.Recent[0]) == requestIdentity(wire.Recent[1]) {
		t.Fatal("truncated completion identity collision")
	}
	latencies, samples := p95WithSamples(wire.Recent, wire.ProviderStats)
	for i := 0; i < 2; i++ {
		name := strings.Repeat("p", 300) + fmt.Sprint(i)
		if latencies[name] != time.Duration(i+1)*time.Second || samples[name] != 1 {
			t.Fatal("P95 provider groups changed after truncation")
		}
	}
	if len(latencies) != 2 {
		t.Fatal("P95 extra prefix group")
	}
	if unknown, _ := p95WithSamples(wire.Recent); len(unknown) != 0 {
		t.Fatal("truncated provider used as exact P95 key without identity mapping")
	}
	m := InitialModel(SnapshotFromTransport(wire)).(*model)
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	layoutApplyKey(m, "5")
	layoutApplyKey(m, "down")
	selected := m.recentRequests()[m.requestCursor]
	a.Record(accounting.Event{RequestID: "new", Model: "ordinary/model"})
	m.applySnapshot(SnapshotFromTransport(transport()))
	if got := m.recentRequests()[m.requestCursor]; got != selected {
		t.Fatal("truncated selected completion changed on refresh")
	}
	layoutApplyKey(m, "enter")
	m.payloadFetcher = payloadTestFetcher()
	for _, key := range []string{"l", "v"} {
		if cmd := layoutKey(m, key); cmd != nil {
			t.Fatal("truncated ID issued correlation command")
		}
		if m.correlation != nil || m.requestDetail == nil || *m.requestDetail != selected || !strings.Contains(m.correlationNotice, "not an exact key") {
			t.Fatal("truncated ID correlated or lost detail")
		}
	}
	title, lines := m.metadataLines()
	if !strings.Contains(title, "correlation unavailable") || !strings.Contains(strings.Join(lines, "\n"), "RequestID, PublicModel, Provider") {
		t.Fatal("per-field truncation guidance missing")
	}
	if strings.Contains(m.renderContextFooter(), "[l] logs") {
		t.Fatal("footer falsely advertises exact correlation")
	}
	var old dashrpc.Snapshot
	if err := json.Unmarshal([]byte(`{"recent":[{"RequestID":"old-id","Model":"old/model","Duration":100}]}`), &old); err != nil {
		t.Fatal(err)
	}
	m.applySnapshot(SnapshotFromTransport(old))
	m.requestDetail = &old.Recent[0]
	if requestPublicModel(*m.requestDetail) != "old/model" || m.requestDetail.Truncated != (accounting.RecentTruncation{}) {
		t.Fatal("legacy fallback changed")
	}
	layoutKey(m, "l")
	if m.correlation == nil || m.correlation.event.RequestID != "old-id" {
		t.Fatal("old exact IDs lost correlation")
	}
}

func TestRecentSequenceAnchorsOtherwiseIdenticalPrefixes(t *testing.T) {
	now := time.Now().UTC()
	a := accounting.NewAggregatorWithClock(func() time.Time { return now })
	base := accounting.Event{Timestamp: now, RequestID: "same-intact-id", PublicModel: strings.Repeat("m", 600), Model: "p/m", Duration: time.Second}
	for _, suffix := range []string{"one", "two"} {
		e := base
		e.PublicModel += suffix
		a.Record(e)
	}
	rows := a.Recent(200)
	first, second := rows[0], rows[1]
	first.RecentSequence, second.RecentSequence = 0, 0
	if first != second || requestIdentity(rows[0]) == requestIdentity(rows[1]) {
		t.Fatal("sequence did not distinguish identical retained prefixes")
	}
	m := InitialModel(SnapshotFromTransport(dashrpc.Build("test", ":0", "none", now, config.Catalog{}, a, nil, nil, 200))).(*model)
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	layoutApplyKey(m, "5")
	layoutApplyKey(m, "down")
	a.Record(base)
	m.applySnapshot(SnapshotFromTransport(dashrpc.Build("test", ":0", "none", now, config.Catalog{}, a, nil, nil, 200)))
	if m.recentRequests()[m.requestCursor].RecentSequence != 1 {
		t.Fatal("refresh selected another identical prefix")
	}
	layoutApplyKey(m, "enter")
	layoutKey(m, "l")
	if m.correlation == nil || m.correlation.event.RequestID != base.RequestID {
		t.Fatal("model-only truncation disabled intact ID correlation")
	}
}

func TestRecentP95DerivedProviderIdentitySurvivesModelTruncation(t *testing.T) {
	a := accounting.NewAggregator()
	for i := 0; i < 2; i++ {
		name := strings.Repeat("p", 600) + fmt.Sprint(i)
		a.Record(accounting.Event{Model: name + "/model", Duration: time.Duration(i+1) * time.Second})
	}
	recent := a.Recent(200)
	latency, samples := p95WithSamples(recent, a.ProviderSummaries())
	for i := 0; i < 2; i++ {
		name := strings.Repeat("p", 600) + fmt.Sprint(i)
		if latency[name] != time.Duration(i+1)*time.Second || samples[name] != 1 {
			t.Fatal("full derived provider grouping changed")
		}
	}
}
