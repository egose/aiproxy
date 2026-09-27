package dashrpc

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
)

func TestBuildCarriesFullRateHorizonAndDetachedBilling(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	a := accounting.NewAggregatorWithClock(func() time.Time { return now })
	for minute := 0; minute < 15; minute++ {
		for i := 0; i < 700; i++ {
			a.Record(accounting.Event{Timestamp: now.Add(-time.Duration(minute)*time.Minute - time.Second),
				Model: "alias/a", Provider: "p", UpstreamModel: "m", StatusCode: 429, TotalTokens: 2})
		}
	}
	snap := Build("test", ":8080", "none", now, config.Catalog{}, a, nil, nil, 1)
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	var wire Snapshot
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	if len(wire.Recent) != 1 || wire.Rates.Minute.Requests != 700 || wire.Rates.FiveMinutes.Requests != 3500 {
		t.Fatalf("recentN altered independent rate windows: %+v", wire.Rates)
	}
	for i, c := range wire.Rates.Minutes {
		if c != (accounting.RateCounts{Requests: 700, Throttled: 700, Tokens: 1400}) {
			t.Fatalf("minute %d: %+v", i, c)
		}
	}
	if wire.Usage[0] != wire.Billing.Usage[0] || wire.Billing.Upstream[0].PublicModel != "alias/a" || wire.Billing.Upstream[0].Count != 10500 || wire.Upstream[0].PublicModel != "" {
		t.Fatal("public/retained/lifetime boundaries lost in JSON")
	}
	snap.Rates.Minutes[14].Requests = -1
	snap.Billing.Usage[0].Count = -1
	snap.Billing.Upstream[0].Count = -1
	if a.RateSnapshot().Minute.Requests != 700 || a.BillingSnapshot().Usage[0].Count != 10500 {
		t.Fatal("transport snapshot mutated live accounting")
	}
	now = now.Add(25 * time.Hour)
	idle := Build("test", ":8080", "none", now, config.Catalog{}, a, nil, nil, 200)
	if idle.Rates.Minute.Requests != 0 || len(idle.Billing.Usage) != 0 || len(idle.Billing.Upstream) != 0 || idle.ProviderStats[0].Requests != 10500 {
		t.Fatal("idle rates/billing expiry changed lifetime provider stats")
	}
	absent := Build("test", ":8080", "none", now, config.Catalog{}, nil, nil, nil, 200)
	if absent.Rates != nil || absent.Billing != nil {
		t.Fatal("missing accounting represented as zero measurements")
	}
}
