package accounting

import (
	"testing"
	"time"
)

func TestAggregatorAggregatesCachedTokens(t *testing.T) {
	a := NewAggregator()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	a.Record(Event{Timestamp: now, Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200, Provider: "openai", UpstreamModel: "gpt-4o-mini", PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30})
	a.Record(Event{Timestamp: now, Model: "openai/gpt-4o-mini", Operation: "chat_completions", StatusCode: 200, Provider: "openai", UpstreamModel: "gpt-4o-mini", PromptTokens: 50, CompletionTokens: 10, TotalTokens: 60, CachedTokens: 5})

	summaries := a.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	s := summaries[0]
	if s.PromptTokens != 150 || s.CompletionTokens != 30 || s.TotalTokens != 180 || s.CachedTokens != 35 {
		t.Fatalf("summary = %+v, want 150/30/180 cached 35", s)
	}

	stats := a.ProviderSummaries()
	if len(stats) != 1 || stats[0].CachedTokens != 35 || stats[0].PromptTokens != 150 || stats[0].CompletionTokens != 30 {
		t.Fatalf("provider summaries = %+v", stats)
	}

	up := a.UpstreamSummaries()
	if len(up) != 1 || up[0].CachedTokens != 35 {
		t.Fatalf("upstream summaries = %+v", up)
	}
}

func TestAggregatorSplitsCacheCreationAndRead(t *testing.T) {
	a := NewAggregator()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	a.now = func() time.Time { return now }
	a.Record(Event{Timestamp: now, Model: "anthropic/claude", Operation: "chat_completions", StatusCode: 200, Provider: "anthropic", UpstreamModel: "claude", PromptTokens: 100, CompletionTokens: 20, TotalTokens: 120, CachedTokens: 30, CacheCreationTokens: 10, CacheReadTokens: 20})

	summaries := a.Summaries()
	if len(summaries) != 1 {
		t.Fatalf("summaries = %+v", summaries)
	}
	s := summaries[0]
	if s.CacheCreationTokens != 10 || s.CacheReadTokens != 20 {
		t.Fatalf("summary = %+v, want creation 10 read 20", s)
	}
	stats := a.ProviderSummaries()
	if len(stats) != 1 || stats[0].CacheCreationTokens != 10 || stats[0].CacheReadTokens != 20 {
		t.Fatalf("provider summaries = %+v", stats)
	}
	up := a.UpstreamSummaries()
	if len(up) != 1 || up[0].CacheCreationTokens != 10 || up[0].CacheReadTokens != 20 {
		t.Fatalf("upstream summaries = %+v", up)
	}
}
