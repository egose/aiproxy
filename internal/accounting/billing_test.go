package accounting

import (
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestBillingSnapshotRetentionAndLifetimeCounters(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	a := NewAggregatorWithClock(func() time.Time { return now })
	event := Event{Model: "alias/a", Provider: "p", UpstreamModel: "m", Tenant: "t", Client: "c", Operation: "responses", StatusCode: 200,
		PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30, CacheCreationTokens: 10, CacheReadTokens: 20}
	a.Record(event)
	now = now.Add(time.Hour)
	a.Record(event)
	providers, lifetime := a.ProviderSummaries(), a.UpstreamSummaries()
	now = now.Add(23 * time.Hour)
	summaries, upstream := a.BillingSummaries()
	if len(summaries) != 1 || summaries[0].Count != 2 || len(upstream) != 1 || upstream[0].Count != 2 {
		t.Fatalf("inclusive cutoff: %+v %+v", summaries, upstream)
	}
	now = now.Add(time.Nanosecond)
	summaries, upstream = a.BillingSummaries()
	if len(summaries) != 1 || summaries[0].Count != 1 || len(upstream) != 1 || upstream[0].Count != 1 {
		t.Fatalf("expired first bucket: %+v %+v", summaries, upstream)
	}
	s, u := summaries[0], upstream[0]
	if u.PublicModel != event.Model || u.Model != event.UpstreamModel || u.Provider != event.Provider ||
		u.Tenant != s.Tenant || u.Client != s.Client || u.Operation != s.Operation || u.StatusCode != s.StatusCode ||
		u.PromptTokens != 100 || u.CompletionTokens != 20 || u.TotalTokens != 120 || u.CachedTokens != 30 || u.CacheCreationTokens != 10 || u.CacheReadTokens != 20 {
		t.Fatalf("retained attribution: %+v", u)
	}
	now = now.Add(time.Hour)
	summaries, upstream = a.BillingSummaries()
	if len(summaries) != 0 || len(upstream) != 0 || len(a.buckets) != 0 || len(a.bucketOrder) != 0 {
		t.Fatalf("expired state retained: %+v %+v buckets=%d order=%d", summaries, upstream, len(a.buckets), len(a.bucketOrder))
	}
	if !reflect.DeepEqual(providers, a.ProviderSummaries()) || !reflect.DeepEqual(lifetime, a.UpstreamSummaries()) || len(a.Recent(10)) != 2 {
		t.Fatal("billing expiry changed lifetime counters or recent ring")
	}
}

func TestBillingBucketsBoundOutOfOrderAndFutureTimestamps(t *testing.T) {
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	a := NewAggregatorWithClock(func() time.Time { return now })
	for i := 0; i < 2000; i++ {
		a.Record(Event{Timestamp: now.Add(time.Duration(i) * time.Minute), Model: "alias/a", Provider: "p", UpstreamModel: "m"})
		a.Record(Event{Timestamp: now.Add(-48 * time.Hour), Model: "alias/" + fmt.Sprint(i), Provider: "p", UpstreamModel: "m"})
	}
	if len(a.buckets) != 1 || len(a.bucketOrder) != 1 {
		t.Fatalf("future/expired bucket count = %d/%d", len(a.buckets), len(a.bucketOrder))
	}
	for _, bucket := range a.buckets {
		if len(bucket) != 1 {
			t.Fatalf("expired key cardinality retained: %d", len(bucket))
		}
	}
	for i := 0; i < 3*24*60; i++ {
		now = now.Add(time.Minute)
		a.Record(Event{Model: "alias/" + fmt.Sprint(i), Provider: "p", UpstreamModel: "m"})
		if len(a.buckets) > 1441 || len(a.bucketOrder) > 1441 {
			t.Fatalf("rolling bucket bound exceeded: %d/%d", len(a.buckets), len(a.bucketOrder))
		}
	}
	now = now.Add(25 * time.Hour)
	_, upstream := a.BillingSummaries()
	if len(upstream) != 0 || len(a.buckets) != 0 {
		t.Fatal("idle rolling keys survived expiry")
	}
}

func TestBillingSnapshotIsAtomicAndDetached(t *testing.T) {
	a := NewAggregator()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 2000; i++ {
			a.Record(Event{Model: "alias/a", Provider: "p", UpstreamModel: "m", TotalTokens: 1})
		}
	}()
	for i := 0; i < 200; i++ {
		s, u := a.BillingSummaries()
		if len(s) != len(u) || (len(s) == 1 && (s[0].Count != u[0].Count || s[0].TotalTokens != u[0].TotalTokens)) {
			t.Fatalf("torn billing snapshot: %+v %+v", s, u)
		}
		if len(u) > 0 {
			u[0].TotalTokens = -1
		}
	}
	wg.Wait()
	s, u := a.BillingSummaries()
	if s[0].Count != 2000 || u[0].TotalTokens != 2000 {
		t.Fatalf("final snapshot: %+v %+v", s, u)
	}
}

func TestMemoryBillingSnapshotAttributesPublicModels(t *testing.T) {
	m := &MemoryRecorder{}
	for _, model := range []string{"alias/a", "alias/b", "p/m"} {
		m.Record(Event{Model: model, Provider: "p", UpstreamModel: "m", PromptTokens: 10})
	}
	s, u := m.BillingSummaries()
	if len(s) != 3 || len(u) != 3 {
		t.Fatalf("snapshot: %+v %+v", s, u)
	}
	for i := range s {
		if u[i].PublicModel != s[i].Model || u[i].PromptTokens != 10 || u[i].Count != 1 {
			t.Fatalf("attribution: %+v %+v", s[i], u[i])
		}
	}
}

func TestBillingClientScopeDoesNotCrossTenants(t *testing.T) {
	summaries := []Summary{{Client: "same"}, {Tenant: "a", Client: "same"}, {Tenant: "b", Client: "same"}, {Client: "other"}}
	if got := FilterSummaries(summaries, "", "same"); len(got) != 1 || got[0].Tenant != "" {
		t.Fatalf("tenantless client scope: %+v", got)
	}
	if got := FilterSummaries(summaries, "a", "same"); len(got) != 1 || got[0].Tenant != "a" {
		t.Fatalf("tenant scope: %+v", got)
	}
}
