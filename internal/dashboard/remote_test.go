package dashboard

import (
	"log/slog"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
)

func TestSnapshotFromTransportRoundTripsCoreFields(t *testing.T) {
	start := time.Now().Add(-1 * time.Hour)
	transport := dashrpc.Snapshot{
		Version:   "test",
		Address:   ":8080",
		AuthMode:  "bearer_static",
		StartTime: start,
		Providers: []dashrpc.Provider{
			{Type: "openai", Name: "openai", DisplayName: "OpenAI", Models: []string{"gpt-4o-mini"}},
		},
		DisabledProviders: []dashrpc.Provider{
			{Type: "openai-compatible", Name: "localai"},
		},
		Aliases: []dashrpc.Alias{
			{Name: "chat", Algorithm: "round_robin", Targets: []dashrpc.AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}}},
		},
		Health: map[string]bool{"openai": true},
		Usage: []accounting.Summary{
			{Model: "openai/gpt-4o-mini", Operation: "chat", StatusCode: 200, Count: 1},
		},
		Recent: []accounting.Event{
			{Model: "openai/gpt-4o-mini", Operation: "chat", StatusCode: 200, Duration: 5 * time.Millisecond},
		},
		Logs: []observability.LogEntry{
			{Seq: 1, Level: slog.LevelInfo, Message: "hello"},
		},
		LastSeq: 1,
	}

	snap := SnapshotFromTransport(transport)
	if snap.Version != "test" || snap.Address != ":8080" || snap.AuthMode != "bearer_static" {
		t.Fatalf("identity fields lost: %+v", snap)
	}
	if snap.StartTime != start {
		t.Fatalf("StartTime = %v, want %v", snap.StartTime, start)
	}
	if len(snap.Providers) != 1 || snap.Providers[0].Name != "openai" || snap.Providers[0].Type != config.ProviderTypeOpenAI {
		t.Fatalf("Providers mis-converted: %+v", snap.Providers)
	}
	if len(snap.Providers[0].Models) != 1 || snap.Providers[0].Models[0].Name != "gpt-4o-mini" {
		t.Fatalf("Provider Models lost: %+v", snap.Providers[0].Models)
	}
	if len(snap.DisabledProviders) != 1 || snap.DisabledProviders[0].Name != "localai" {
		t.Fatalf("DisabledProviders mis-converted: %+v", snap.DisabledProviders)
	}
	if len(snap.Aliases) != 1 || snap.Aliases[0].Algorithm != config.AlgorithmRoundRobin {
		t.Fatalf("Aliases mis-converted: %+v", snap.Aliases)
	}
	if !snap.Health.Snapshot()["openai"] {
		t.Fatalf("Health mis-converted: %+v", snap.Health.Snapshot())
	}
	if got := snap.Usage.Summaries(); len(got) != 1 || got[0].Model != "openai/gpt-4o-mini" {
		t.Fatalf("Usage mis-converted: %+v", got)
	}
	if got := snap.Usage.Recent(10); len(got) != 1 || got[0].Duration != 5*time.Millisecond {
		t.Fatalf("Recent mis-converted: %+v", got)
	}
	if got := snap.Logs.Since(10); len(got) != 1 || got[0].Message != "hello" {
		t.Fatalf("Logs mis-converted: %+v", got)
	}
}

func TestSnapshotFromTransportHandlesEmptyTransport(t *testing.T) {
	snap := SnapshotFromTransport(dashrpc.Snapshot{})
	if snap == nil {
		t.Fatal("expected non-nil snapshot")
	}
	if len(snap.Providers) != 0 || snap.Providers != nil {
		t.Fatalf("Providers should be empty/nil: %+v", snap.Providers)
	}
	if got := snap.Health.Snapshot(); len(got) != 0 {
		t.Fatalf("Health should be empty: %+v", got)
	}
	if got := snap.Usage.Summaries(); len(got) != 0 {
		t.Fatalf("Usage should be empty: %+v", got)
	}
	if got := snap.Logs.Since(10); got != nil {
		t.Fatalf("Logs should be nil: %+v", got)
	}
}
