package accounting

import (
	"testing"
	"time"
)

func TestAggregatorSummaries(t *testing.T) {
	a := NewAggregator()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
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
