package providerhealth

import (
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

func TestTrackerFailureCooldownAndRecovery(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := New(nil)
	tracker.cooldown = 30 * time.Second
	tracker.now = func() time.Time { return clock }
	tracker.SetProviders(map[string]config.Provider{"openai": {Name: "openai"}})
	if !tracker.IsHealthy("openai") {
		t.Fatal("provider should start healthy")
	}
	tracker.MarkFailure("openai")
	if tracker.IsHealthy("openai") {
		t.Fatal("provider should be unhealthy during cooldown")
	}
	clock = clock.Add(31 * time.Second)
	if !tracker.IsHealthy("openai") {
		t.Fatal("provider should recover after cooldown")
	}
}

func TestTrackerSetProvidersRemovesMissingMetrics(t *testing.T) {
	metrics := observability.NewMetrics()
	tracker := New(metrics)
	tracker.SetProviders(map[string]config.Provider{"openai": {Name: "openai"}})
	tracker.MarkFailure("openai")
	tracker.SetProviders(map[string]config.Provider{"gemini": {Name: "gemini"}})
	if tracker.IsHealthy("gemini") != true {
		t.Fatal("new provider should be healthy")
	}
}
