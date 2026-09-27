package dashrpc

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
)

func TestRecentTransportBoundsFlagsAndOrdinaryCompatibility(t *testing.T) {
	now := time.Now().UTC()
	a := accounting.NewAggregatorWithClock(func() time.Time { return now })
	source := strings.Repeat("\x00", 4096)
	e := accounting.Event{Timestamp: now, RequestID: source, PublicModel: source, Tenant: source, Client: source,
		Model: source, Provider: source, UpstreamModel: source, Operation: source, StatusCode: 429,
		Duration: time.Second, PromptTokens: 7, CompletionTokens: 3, TotalTokens: 10}
	for i := 0; i < 201; i++ {
		a.Record(e)
	}
	snap := Build("test", ":0", "none", now, config.Catalog{}, a, nil, nil, 1000)
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var wire Snapshot
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(wire.Recent, snap.Recent) {
		t.Fatal("recent wire round trip changed values")
	}
	if len(wire.Recent) != 200 || len(wire.Recent[0].Truncated.Fields()) != 8 {
		t.Fatal("recent cap or flags lost")
	}
	if wire.Recent[0].ProviderID != wire.ProviderStats[0].ProviderID || wire.ProviderStats[0].Provider != source || wire.ProviderStats[0].Requests != 201 {
		t.Fatal("exact provider identity/counters lost")
	}
	if wire.Usage[0].Model != source || wire.Usage[0].Tenant != source || wire.Billing.Upstream[0].Model != source || wire.Usage[0].Count != 201 {
		t.Fatal("recent bounds changed full aggregate identity")
	}
	recentData, err := json.Marshal(wire.Recent)
	if err != nil {
		t.Fatal(err)
	}
	if len(recentData) > 2+200*(accounting.RecentEntryJSONBytes+1) {
		t.Fatal("serialized recent component exceeded budget")
	}
	t.Logf("all-field escaped RPC recent=%d bytes; retained=200; aggregation remains full", len(recentData))
	ordinary := accounting.Event{Timestamp: now, RequestID: "ordinary-id", PublicModel: "alias/chat", Model: "alias/chat",
		Provider: "origin", UpstreamModel: "configured", Tenant: "team", Client: "cli", Operation: "chat_completions",
		StatusCode: 200, Duration: time.Second, TotalTokens: 17}
	a.Record(ordinary)
	got := a.Recent(1)[0]
	got.ProviderID, got.RecentSequence = 0, 0
	if got != ordinary {
		t.Fatal("ordinary event values changed")
	}
	data, err = json.Marshal(a.Recent(1)[0])
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Truncated") {
		t.Fatal("ordinary event falsely marked truncated")
	}
	var old struct {
		RequestID, PublicModel, Model, Provider string
		TotalTokens                             int64
	}
	if err := json.Unmarshal(data, &old); err != nil {
		t.Fatal(err)
	}
	if old.RequestID != ordinary.RequestID || old.Model != ordinary.Model || old.TotalTokens != 17 {
		t.Fatal("additive fields broke old consumer")
	}
}
