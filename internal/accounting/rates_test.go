package accounting

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestRatesIndependentOfRecentCapAndIdle(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 500000000, time.UTC)
	a := NewAggregatorWithClock(func() time.Time { return now })
	end := now.Truncate(time.Second)
	for minute := 0; minute < 15; minute++ {
		for n := 0; n < 1000; n++ {
			status := 200
			if n < 100 {
				status = 500
			} else if n < 150 {
				status = 429
			}
			a.Record(Event{Timestamp: end.Add(-time.Duration(minute)*time.Minute - time.Second),
				Model: "p/m", StatusCode: status, TotalTokens: 100})
		}
	}
	r := a.RateSnapshot()
	want := RateCounts{Requests: 1000, Errors: 100, Throttled: 50, Tokens: 100000}
	if len(a.Recent(20000)) != 200 || r.Minute != want || r.FiveMinutes.Requests != 5000 {
		t.Fatalf("rates/cap: %+v recent=%d", r, len(a.Recent(20000)))
	}
	for i, counts := range r.Minutes {
		if counts != want {
			t.Fatalf("minute %d: %+v", i, counts)
		}
	}
	now = now.Add(2 * time.Minute)
	if r := a.RateSnapshot(); r.Minute != (RateCounts{}) || r.FiveMinutes.Requests != 3000 {
		t.Fatalf("idle minute/5m: %+v", r)
	}
	now = now.Add(15 * time.Minute)
	r = a.RateSnapshot()
	if r.Minute != (RateCounts{}) || r.FiveMinutes != (RateCounts{}) || r.Minutes != ([15]RateCounts{}) {
		t.Fatalf("idle horizon: %+v", r)
	}
	if a.ProviderSummaries()[0].Requests != 15000 || len(a.Summaries()) == 0 {
		t.Fatal("rate expiry changed lifetime or retained billing counters")
	}
}

func TestRateHalfOpenBoundariesAndTimestampNormalization(t *testing.T) {
	end := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name           string
		ts             time.Time
		one, five, all int64
	}{
		{"lower minute", end.Add(-time.Minute), 1, 1, 1},
		{"before minute", end.Add(-time.Minute - time.Nanosecond), 0, 1, 1},
		{"lower five", end.Add(-5 * time.Minute), 0, 1, 1},
		{"before five", end.Add(-5*time.Minute - time.Nanosecond), 0, 0, 1},
		{"lower horizon", end.Add(-15 * time.Minute), 0, 0, 1},
		{"expired", end.Add(-15*time.Minute - time.Nanosecond), 0, 0, 0},
		{"last nanosecond", end.Add(-time.Nanosecond), 1, 1, 1},
		{"current second", end, 0, 0, 0},
		{"zero", time.Time{}, 0, 0, 0},
		{"future", end.Add(48 * time.Hour), 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := end.Add(500 * time.Millisecond)
			a := NewAggregatorWithClock(func() time.Time { return now })
			a.Record(Event{Timestamp: tc.ts, StatusCode: 0, TotalTokens: 17})
			r := a.RateSnapshot()
			var all int64
			for _, c := range r.Minutes {
				all += c.Requests
			}
			if !r.WindowEnd.Equal(end) || r.BucketSeconds != 1 || r.Minute.Requests != tc.one || r.FiveMinutes.Requests != tc.five || all != tc.all {
				t.Fatalf("boundary: %+v total=%d", r, all)
			}
			if tc.name == "future" || tc.name == "zero" || tc.name == "current second" {
				now = end.Add(time.Second)
				if got := a.RateSnapshot().Minute; got != (RateCounts{Requests: 1, Errors: 1, Tokens: 17}) {
					t.Fatalf("normalized into next complete second: %+v", got)
				}
			}
		})
	}
}

func TestRateLateEventsCannotOverwriteLiveBucketsAndStateIsFixed(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	a := NewAggregatorWithClock(func() time.Time { return now })
	for i := 0; i < 4*rateSeconds; i++ {
		now = now.Add(time.Second)
		a.Record(Event{Model: "p/m", Tenant: fmt.Sprint(i), TotalTokens: 1})
		a.Record(Event{Timestamp: now.Add(-901 * time.Second), Model: "p/m", TotalTokens: 10000})
	}
	a.Record(Event{Timestamp: now.Add(-60 * time.Second), Model: "p/m", StatusCode: 429, TotalTokens: 7})
	r := a.RateSnapshot()
	if len(a.rates) != 901 || r.Minute.Requests != 61 || r.Minute.Tokens != 67 || r.Minute.Throttled != 1 || r.FiveMinutes.Requests != 301 {
		t.Fatalf("late event/ring reuse: %+v buckets=%d", r, len(a.rates))
	}
	now = now.Add(time.Hour)
	a.RateSnapshot()
	for _, b := range a.rates {
		if b != (rateBucket{}) {
			t.Fatalf("idle bucket not cleared: %+v", b)
		}
	}
}

func TestRateSnapshotsConcurrentAndDetached(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	a := NewAggregatorWithClock(func() time.Time { return now })
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 500; i++ {
				a.Record(Event{Timestamp: now.Add(-time.Second), StatusCode: 400, TotalTokens: 3})
				r := a.RateSnapshot()
				if r.Minute.Errors != r.Minute.Requests || r.Minute.Tokens != 3*r.Minute.Requests {
					t.Errorf("torn rate counts: %+v", r.Minute)
				}
				r.Minute.Requests = -1
				r.Minutes[14].Requests = -1
			}
		}()
	}
	wg.Wait()
	if r := a.RateSnapshot(); r.Minute.Requests != 2000 || r.Minutes[14].Requests != 2000 {
		t.Fatalf("final rate snapshot: %+v", r)
	}
}
