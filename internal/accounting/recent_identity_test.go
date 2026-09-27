package accounting

import (
	"fmt"
	"testing"
	"time"
)

func TestRecentIdentityCapDoesNotExpandAggregations(t *testing.T) {
	now := time.Now()
	a := NewAggregatorWithClock(func() time.Time { return now })
	for i := 0; i < 10000; i++ {
		a.Record(Event{RequestID: fmt.Sprint(i), PublicModel: fmt.Sprintf("missing/%d", i), Timestamp: now,
			Model: "_model_not_found", Client: "ci", Tenant: "team", Operation: "chat_completions", StatusCode: 404})
	}
	rows := a.Recent(10000)
	if len(rows) != 200 || rows[0].RequestID != "9800" || rows[199].RequestID != "9999" || rows[199].PublicModel != "missing/9999" {
		t.Fatalf("ring metadata/cap: %d %+v", len(rows), rows[0])
	}
	rows[0].RequestID = "mutated"
	if a.Recent(200)[0].RequestID != "9800" {
		t.Fatal("returned events alias retained state")
	}
	if rows := a.Summaries(); len(rows) != 1 || rows[0].Count != 10000 {
		t.Fatalf("completion IDs expanded billing identities: %+v", rows)
	}
	if len(a.buckets) != 1 {
		t.Fatal("completion metadata expanded buckets")
	}
}
