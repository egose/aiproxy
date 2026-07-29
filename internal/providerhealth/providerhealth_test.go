package providerhealth

import (
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

func TestTrackerFailureCooldownAndRecovery(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := New(nil, config.ProviderHealth{})
	tracker.cooldown = 30 * time.Second
	backend := tracker.backend.(*memoryBackend)
	backend.now = func() time.Time { return clock }
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
	tracker := New(metrics, config.ProviderHealth{})
	tracker.SetProviders(map[string]config.Provider{"openai": {Name: "openai"}})
	tracker.MarkFailure("openai")
	tracker.SetProviders(map[string]config.Provider{"gemini": {Name: "gemini"}})
	if tracker.IsHealthy("gemini") != true {
		t.Fatal("new provider should be healthy")
	}
}

func TestRedisBackendSharesHealthAcrossTrackers(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer server.Close()
	left := New(nil, config.ProviderHealth{RedisURL: "redis://" + server.Addr(), KeyPrefix: "test", Cooldown: 30 * time.Second})
	right := New(nil, config.ProviderHealth{RedisURL: "redis://" + server.Addr(), KeyPrefix: "test", Cooldown: 30 * time.Second})
	left.MarkFailure("openai")
	if right.IsHealthy("openai") {
		t.Fatal("expected redis-backed health state to be shared")
	}
	right.MarkSuccess("openai")
	if !left.IsHealthy("openai") {
		t.Fatal("expected redis-backed recovery to be shared")
	}
}
