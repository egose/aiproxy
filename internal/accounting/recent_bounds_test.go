package accounting

import (
	"encoding/json"
	"fmt"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
	"unsafe"
)

var recentBudgets = map[string]int{
	"RequestID": RecentIdentityBytes, "PublicModel": RecentModelBytes,
	"Tenant": RecentIdentityBytes, "Client": RecentIdentityBytes,
	"Model": RecentModelBytes, "Operation": RecentOperationBytes,
	"Provider": RecentIdentityBytes, "UpstreamModel": RecentModelBytes,
}

func recentWithStrings(s string) Event {
	var e Event
	v := reflect.ValueOf(&e).Elem()
	for name := range recentBudgets {
		v.FieldByName(name).SetString(s)
	}
	return e
}

func TestRecentFieldBudgetsUnicodeAndOwnership(t *testing.T) {
	stringsFound := 0
	typ := reflect.TypeOf(Event{})
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		if f.Type.Kind() == reflect.String {
			stringsFound++
			if _, ok := recentBudgets[f.Name]; !ok {
				t.Fatalf("unbudgeted diagnostic string %s", f.Name)
			}
		}
	}
	if stringsFound != len(recentBudgets) {
		t.Fatal("field budget coverage mismatch")
	}
	for name, budget := range recentBudgets {
		for _, tc := range []struct {
			name, input, want string
			truncated         bool
		}{
			{"empty", "", "", false}, {"ordinary", "ordinary/界🙂", "ordinary/界🙂", false},
			{"below", strings.Repeat("a", budget-1), strings.Repeat("a", budget-1), false},
			{"exact", strings.Repeat("a", budget), strings.Repeat("a", budget), false},
			{"above", strings.Repeat("a", budget+1), strings.Repeat("a", budget), true},
			{"unicode-cut", strings.Repeat("a", budget-1) + "界suffix", strings.Repeat("a", budget-1), true},
			{"unicode-exact", strings.Repeat("a", budget-4) + "🙂", strings.Repeat("a", budget-4) + "🙂", false},
			{"unicode-prefix", strings.Repeat("a", budget-4) + "🙂suffix", strings.Repeat("a", budget-4) + "🙂", true},
		} {
			t.Run(name+"/"+tc.name, func(t *testing.T) {
				parent := "padding" + tc.input + strings.Repeat("z", 64<<10)
				input := parent[7 : 7+len(tc.input)]
				var e Event
				reflect.ValueOf(&e).Elem().FieldByName(name).SetString(input)
				r := newRingBuffer(1)
				r.push(e)
				got := r.snapshot(1)[0]
				value := reflect.ValueOf(got).FieldByName(name).String()
				flag := reflect.ValueOf(got.Truncated).FieldByName(name).Bool()
				if value != tc.want || flag != tc.truncated || !utf8.ValidString(value) {
					t.Fatalf("value bytes=%d flag=%v, want bytes=%d flag=%v", len(value), flag, len(tc.want), tc.truncated)
				}
				if value != "" {
					start := uintptr(unsafe.Pointer(unsafe.StringData(parent)))
					retained := uintptr(unsafe.Pointer(unsafe.StringData(value)))
					if retained >= start && retained < start+uintptr(len(parent)) {
						t.Fatal("retained string still owns oversized source backing allocation")
					}
				}
				runtime.KeepAlive(parent)
			})
		}
	}
}

func TestRecentBoundedAllocationExperiment(t *testing.T) {
	for _, size := range []int{32 << 10, 1 << 20} {
		source := strings.Repeat("x", size)
		e := recentWithStrings(source)
		r := newRingBuffer(200)
		result := testing.Benchmark(func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				r.push(e)
			}
		})
		if got := result.AllocedBytesPerOp(); got > RecentEntryStringBytes+128 || got < RecentEntryStringBytes {
			t.Fatalf("retained copies allocated %d bytes/op for %d source bytes", got, size)
		}
		start := uintptr(unsafe.Pointer(unsafe.StringData(source)))
		control := source[:RecentModelBytes]
		if uintptr(unsafe.Pointer(unsafe.StringData(control))) != start {
			t.Fatal("slice-only control did not share backing allocation")
		}
		for _, retained := range r.snapshot(200) {
			for field := range recentBudgets {
				s := reflect.ValueOf(retained).FieldByName(field).String()
				p := uintptr(unsafe.Pointer(unsafe.StringData(s)))
				if p >= start && p < start+uintptr(size) {
					t.Fatal("ring retained source backing allocation")
				}
			}
		}
		t.Logf("source=%d bytes; slice-only control shares backing; detached ring=%d bytes/op, %d allocs/op, <=%d string bytes/200 entries", size, result.AllocedBytesPerOp(), result.AllocsPerOp(), 200*RecentEntryStringBytes)
		runtime.KeepAlive(source)
	}
}

func TestRecentBoundsPreserveExactAggregationAndEviction(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	a := NewAggregatorWithClock(func() time.Time { return now })
	var full [2]Event
	for i := range full {
		full[i] = recentWithStrings(strings.Repeat("x", RecentModelBytes+16) + fmt.Sprint(i))
		full[i].Timestamp = now.Add(-time.Second)
		full[i].StatusCode = 200
		full[i].Duration = time.Duration(i+1) * time.Millisecond
		full[i].PromptTokens, full[i].CompletionTokens, full[i].TotalTokens = 7, 3, 10
		full[i].CachedTokens, full[i].CacheCreationTokens, full[i].CacheReadTokens = 2, 1, 2
	}
	for i := 0; i < 202; i++ {
		a.Record(full[i%2])
	}
	rows := a.Recent(1000)
	if len(rows) != 200 || rows[0].RecentSequence != 3 || rows[199].RecentSequence != 202 {
		t.Fatal("ring eviction/sequence changed")
	}
	if rows[0].RequestID != rows[1].RequestID || rows[0].ProviderID == rows[1].ProviderID {
		t.Fatal("prefix fixture or exact provider identity failed")
	}
	if len(rows[0].Truncated.Fields()) != 8 {
		t.Fatal("not all fields bounded")
	}
	rows[0].Truncated.RequestID = false
	if !a.Recent(200)[0].Truncated.RequestID {
		t.Fatal("mutable flag alias")
	}
	summaries, upstream := a.BillingSummaries()
	if len(summaries) != 2 || len(upstream) != 2 || len(a.upstream) != 2 || len(a.providers) != 2 {
		t.Fatal("full grouping dimensions merged")
	}
	for i, s := range summaries {
		e := full[i]
		if s.Tenant != e.Tenant || s.Client != e.Client || s.Model != e.Model || s.Operation != e.Operation || s.Count != 101 || s.PromptTokens != 707 || s.CompletionTokens != 303 || s.TotalTokens != 1010 || s.CachedTokens != 202 || s.CacheCreationTokens != 101 || s.CacheReadTokens != 202 {
			t.Fatal("billing identity or tokens changed")
		}
		if u := upstream[i]; u.Provider != e.Provider || u.Model != e.UpstreamModel || u.PublicModel != e.Model || u.Count != 101 {
			t.Fatal("attributed upstream identity changed")
		}
	}
	if rates := a.RateSnapshot(); rates.Minute != (RateCounts{Requests: 202, Tokens: 2020}) {
		t.Fatalf("rate semantics changed: %+v", rates.Minute)
	}
	for _, s := range a.ProviderSummaries() {
		if s.Requests != 101 || s.TotalTokens != 1010 || s.ProviderID == 0 {
			t.Fatal("lifetime counters changed")
		}
	}
}

func TestRecentWorstCaseJSONBudget(t *testing.T) {
	r := newRingBuffer(200)
	for i := 0; i < 200; i++ {
		r.push(recentWithStrings(strings.Repeat("\x00", 2048)))
	}
	data, err := json.Marshal(r.snapshot(200))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 2+200*(RecentEntryJSONBytes+1) {
		t.Fatalf("recent JSON %d exceeds conservative bound", len(data))
	}
	t.Logf("200 all-field escaped entries: %d JSON bytes (bound %d)", len(data), 2+200*(RecentEntryJSONBytes+1))
}
