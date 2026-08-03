package providerhealth

import (
	"context"
	"errors"
	"testing"
	"time"

	miniredis "github.com/alicebob/miniredis/v2"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

type stubBackend struct {
	markSuccess func(context.Context, string) error
	markFailure func(context.Context, string, time.Duration) error
	isHealthy   func(context.Context, string) (bool, error)
}

func (b stubBackend) MarkSuccess(ctx context.Context, name string) error {
	if b.markSuccess != nil {
		return b.markSuccess(ctx, name)
	}
	return nil
}

func (b stubBackend) MarkFailure(ctx context.Context, name string, cooldown time.Duration) error {
	if b.markFailure != nil {
		return b.markFailure(ctx, name, cooldown)
	}
	return nil
}

func (b stubBackend) IsHealthy(ctx context.Context, name string) (bool, error) {
	if b.isHealthy != nil {
		return b.isHealthy(ctx, name)
	}
	return true, nil
}

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

func TestRedisBackendRespectsCancelledContext(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer server.Close()
	backend := newRedisBackend("redis://"+server.Addr(), "test")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := backend.MarkFailure(ctx, "openai", 30*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("mark failure error = %v", err)
	}
	if healthy, err := backend.IsHealthy(context.Background(), "openai"); err != nil || !healthy {
		t.Fatalf("healthy=%v err=%v", healthy, err)
	}
}

func TestTrackerUsesContextAwareHealthMethods(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	tracker := &Tracker{backend: stubBackend{isHealthy: func(got context.Context, name string) (bool, error) {
		called = true
		if got.Err() == nil {
			t.Fatalf("expected cancelled context")
		}
		if name != "openai" {
			t.Fatalf("name = %q", name)
		}
		return true, got.Err()
	}}}
	if !tracker.IsHealthyContext(ctx, "openai") {
		t.Fatal("tracker should fail open on backend error")
	}
	if !called {
		t.Fatal("backend was not called")
	}
}

func TestTrackerSnapshotReportsKnownProviders(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := New(nil, config.ProviderHealth{})
	backend := tracker.backend.(*memoryBackend)
	backend.now = func() time.Time { return clock }
	tracker.SetProviders(map[string]config.Provider{
		"openai": {Name: "openai"},
		"backup": {Name: "backup"},
		"gemini": {Name: "gemini"},
	})
	tracker.MarkFailure("backup")
	if snapshot := tracker.Snapshot(); len(snapshot) != 3 ||
		!snapshot["openai"] || snapshot["backup"] || !snapshot["gemini"] {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	clock = clock.Add(31 * time.Second)
	if snapshot := tracker.Snapshot(); !snapshot["backup"] {
		t.Fatalf("snapshot after cooldown = %+v", snapshot)
	}
}

func TestTrackerSnapshotNilSafe(t *testing.T) {
	var tracker *Tracker
	if snap := tracker.Snapshot(); snap != nil {
		t.Fatalf("nil tracker snapshot = %+v", snap)
	}
}
