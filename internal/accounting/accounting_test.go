package accounting

import (
	"testing"
	"time"
)

func TestAggregatorSummaries(t *testing.T) {
	a := NewAggregator()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	a.Record(Event{Timestamp: now, Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200})
	a.Record(Event{Timestamp: now, Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200})
	a.Record(Event{Timestamp: now, Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini", Operation: "responses", StatusCode: 200})

	summaries := a.Summaries()
	if len(summaries) != 2 {
		t.Fatalf("summaries = %+v", summaries)
	}
	if summaries[0].Operation != "chat_completions" || summaries[0].Count != 2 {
		t.Fatalf("first summary = %+v", summaries[0])
	}
	if summaries[1].Operation != "responses" || summaries[1].Count != 1 {
		t.Fatalf("second summary = %+v", summaries[1])
	}
}

func TestMultiRecorderFanout(t *testing.T) {
	memory := &MemoryRecorder{}
	agg := NewAggregator()
	recorder := NewMulti(memory, agg)
	recorder.Record(Event{Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200})
	if len(memory.Events()) != 1 {
		t.Fatalf("memory events = %+v", memory.Events())
	}
	summaries := agg.Summaries()
	if len(summaries) != 1 || summaries[0].Count != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
}

func TestAggregatorPrunesExpiredEntries(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	a := NewAggregator()
	a.retention = time.Hour
	a.now = func() time.Time { return now }
	a.Record(Event{Timestamp: now.Add(-2 * time.Hour), Tenant: "team-a", Client: "ci", Model: "openai/old", Operation: "chat_completions", StatusCode: 200})
	a.Record(Event{Timestamp: now.Add(-30 * time.Minute), Tenant: "team-a", Client: "ci", Model: "openai/new", Operation: "chat_completions", StatusCode: 200})

	summaries := a.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	if summaries[0].Model != "openai/new" {
		t.Fatalf("summary model = %q", summaries[0].Model)
	}
}

func TestAggregatorKeepsRecentEntriesWithoutTimestamp(t *testing.T) {
	now := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	a := NewAggregator()
	a.retention = time.Hour
	a.now = func() time.Time { return now }
	a.Record(Event{Tenant: "team-a", Client: "ci", Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200})

	now = now.Add(30 * time.Minute)
	summaries := a.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	now = now.Add(31 * time.Minute)
	if summaries := a.Summaries(); len(summaries) != 0 {
		t.Fatalf("summaries after retention = %+v", summaries)
	}
}

func TestAggregatorRecordsTokens(t *testing.T) {
	a := NewAggregator()
	a.Record(Event{
		Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200,
		PromptTokens: 12, CompletionTokens: 8, TotalTokens: 20,
	})
	a.Record(Event{
		Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200,
		PromptTokens: 4, CompletionTokens: 1, TotalTokens: 5,
	})
	summaries := a.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	s := summaries[0]
	if s.Count != 2 || s.PromptTokens != 16 || s.CompletionTokens != 9 || s.TotalTokens != 25 {
		t.Fatalf("summary = %+v", s)
	}
}

func TestAggregatorRecent(t *testing.T) {
	a := NewAggregator()
	a.Record(Event{Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200})
	a.Record(Event{Model: "openai/gpt-4.1", Operation: "responses", StatusCode: 500})
	a.Record(Event{Model: "alias/chat", Operation: "chat_completions", StatusCode: 200})

	recent := a.Recent(2)
	if len(recent) != 2 {
		t.Fatalf("recent = %+v", recent)
	}
	if recent[0].Model != "openai/gpt-4.1" || recent[1].Model != "alias/chat" {
		t.Fatalf("recent order = %+v", recent)
	}
	if got := a.Recent(10); len(got) != 3 {
		t.Fatalf("recent(10) = %+v", got)
	}
	if got := a.Recent(0); got != nil {
		t.Fatalf("recent(0) = %+v", got)
	}
}

func TestAggregatorRecentRingBuffer(t *testing.T) {
	a := NewAggregator()
	// Shrink ring buffer to test wraparound: record 5 events into capacity-3 buffer.
	a.recent = newRingBuffer(3)
	for i := 0; i < 5; i++ {
		a.Record(Event{Model: "openai/m", Operation: "chat_completions", StatusCode: 200 + i})
	}
	recent := a.Recent(10)
	if len(recent) != 3 {
		t.Fatalf("recent = %+v", recent)
	}
	if recent[0].StatusCode != 202 || recent[2].StatusCode != 204 {
		t.Fatalf("recent order = %+v", recent)
	}
}

func TestByProvider(t *testing.T) {
	summaries := []Summary{
		{Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200, Count: 10, TotalTokens: 100},
		{Model: "openai/gpt-4.1", Operation: "chat_completions", StatusCode: 500, Count: 2, TotalTokens: 20},
		{Model: "backup/qwen3", Operation: "chat_completions", StatusCode: 200, Count: 5, TotalTokens: 50},
		{Model: "_unresolved_model", Operation: "chat_completions", StatusCode: 404, Count: 3},
	}
	got := ByProvider(summaries)
	if len(got) != 3 {
		t.Fatalf("byProvider = %+v", got)
	}
	// Sorted by request count desc: openai(12) > backup(5) > aiproxy(3)
	if got[0].Provider != "openai" || got[0].Requests != 12 || got[0].Errors != 2 || got[0].TotalTokens != 120 {
		t.Fatalf("got[0] = %+v", got[0])
	}
	if got[1].Provider != "backup" || got[1].Requests != 5 || got[1].Errors != 0 {
		t.Fatalf("got[1] = %+v", got[1])
	}
	if got[2].Provider != "aiproxy" || got[2].Requests != 3 || got[2].Errors != 3 {
		t.Fatalf("got[2] = %+v", got[2])
	}
}

func TestMemoryRecorderRecent(t *testing.T) {
	m := &MemoryRecorder{}
	m.Record(Event{Model: "openai/gpt-4o-mini", StatusCode: 200})
	m.Record(Event{Model: "openai/gpt-4.1", StatusCode: 200})
	recent := m.Recent(1)
	if len(recent) != 1 || recent[0].Model != "openai/gpt-4.1" {
		t.Fatalf("recent = %+v", recent)
	}
}
