package providerhealth

import (
	"context"
	"errors"
	"sync/atomic"
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
	snapshot    func(context.Context, []string) (map[string]bool, error)
	close       func() error
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

func (b stubBackend) Snapshot(ctx context.Context, names []string) (map[string]bool, error) {
	if b.snapshot != nil {
		return b.snapshot(ctx, names)
	}
	out := make(map[string]bool, len(names))
	for _, name := range names {
		healthy, err := b.IsHealthy(ctx, name)
		if err != nil {
			return nil, err
		}
		out[name] = healthy
	}
	return out, nil
}

func (b stubBackend) Close() error {
	if b.close != nil {
		return b.close()
	}
	return nil
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
	backend, err := newRedisBackend("redis://"+server.Addr(), "test")
	if err != nil {
		t.Fatalf("redis backend: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := backend.MarkFailure(ctx, "openai", 30*time.Second); !errors.Is(err, context.Canceled) {
		t.Fatalf("mark failure error = %v", err)
	}
	if healthy, err := backend.IsHealthy(context.Background(), "openai"); err != nil || !healthy {
		t.Fatalf("healthy=%v err=%v", healthy, err)
	}
}

func TestRedisBackendRejectsMalformedURL(t *testing.T) {
	if _, err := newRedisBackend("localhost:6379", "test"); err == nil {
		t.Fatal("expected malformed Redis URL to be rejected")
	}
}

func TestRedisBackendCloseClosesClient(t *testing.T) {
	server, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	defer server.Close()
	backend, err := newRedisBackend("redis://"+server.Addr(), "test")
	if err != nil {
		t.Fatalf("redis backend: %v", err)
	}
	if err := backend.Close(); err != nil {
		t.Fatalf("close backend: %v", err)
	}
	if err := backend.MarkSuccess(context.Background(), "openai"); err == nil {
		t.Fatal("expected operation after close to fail")
	}
}

func TestTrackerCloseClosesBackend(t *testing.T) {
	closed := false
	tracker := &Tracker{backend: stubBackend{close: func() error {
		closed = true
		return nil
	}}}
	if err := tracker.Close(); err != nil {
		t.Fatalf("close tracker: %v", err)
	}
	if !closed {
		t.Fatal("backend was not closed")
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

func TestTrackerSnapshotDoesNotHoldProviderLockDuringBackendRead(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	tracker := &Tracker{known: map[string]bool{"openai": true}, backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
		close(started)
		<-release
		return map[string]bool{"openai": true}, nil
	}}}
	done := make(chan struct{})
	go func() {
		_ = tracker.Snapshot()
		close(done)
	}()
	<-started
	setDone := make(chan struct{})
	go func() {
		tracker.SetProviders(map[string]config.Provider{"gemini": {Name: "gemini"}})
		close(setDone)
	}()
	select {
	case <-setDone:
	case <-time.After(200 * time.Millisecond):
		close(release)
		t.Fatal("SetProviders blocked on snapshot backend read")
	}
	close(release)
	<-done
}

func TestTrackerSnapshotContextCancellationFailsOpen(t *testing.T) {
	tracker := &Tracker{known: map[string]bool{"openai": true, "gemini": true}, backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	snapshot := tracker.SnapshotContext(ctx)
	if len(snapshot) != 2 || !snapshot["openai"] || !snapshot["gemini"] {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestTrackerAnyHealthyUsesOneBackendSnapshot(t *testing.T) {
	var calls atomic.Int32
	tracker := &Tracker{backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
		calls.Add(1)
		return map[string]bool{"openai": false, "gemini": true}, nil
	}}}
	providers := map[string]config.Provider{"openai": {Name: "openai"}, "gemini": {Name: "gemini"}}
	if !tracker.AnyHealthyContext(context.Background(), providers) {
		t.Fatal("expected one healthy provider")
	}
	if calls.Load() != 1 {
		t.Fatalf("backend snapshot calls = %d, want 1", calls.Load())
	}
}

func TestTrackerIsHealthyFailsOpenWithoutCache(t *testing.T) {
	var backendErr atomic.Int32
	tracker := &Tracker{backend: stubBackend{isHealthy: func(ctx context.Context, name string) (bool, error) {
		backendErr.Add(1)
		return false, context.Canceled
	}}}
	if !tracker.IsHealthyContext(context.Background(), "openai") {
		t.Fatal("should fail open when no cache exists")
	}
	if backendErr.Load() == 0 {
		t.Fatal("backend should have been called")
	}
}

func TestTrackerIsHealthyUsesCachedValueOnBackendError(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := &Tracker{cacheTTL: defaultCacheTTL, cache: map[string]cachedHealth{"openai": {healthy: false, expiresAt: clock.Add(time.Minute)}}, now: func() time.Time { return clock }}
	calls := 0
	tracker.backend = stubBackend{isHealthy: func(ctx context.Context, name string) (bool, error) {
		calls++
		return true, context.Canceled
	}}
	if tracker.IsHealthyContext(context.Background(), "openai") {
		t.Fatal("cached unhealthy value should be returned on backend error")
	}
	if calls != 1 {
		t.Fatalf("backend calls = %d, want 1", calls)
	}
}

func TestTrackerIsHealthyIgnoresExpiredCache(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := &Tracker{cache: map[string]cachedHealth{"openai": {healthy: false, expiresAt: clock.Add(-time.Second)}}, now: func() time.Time { return clock }}
	tracker.backend = stubBackend{isHealthy: func(ctx context.Context, name string) (bool, error) {
		return true, context.Canceled
	}}
	if !tracker.IsHealthyContext(context.Background(), "openai") {
		t.Fatal("expired cache entry should not influence fallback")
	}
}

func TestTrackerSnapshotFailsOpenWithoutCache(t *testing.T) {
	tracker := &Tracker{known: map[string]bool{"openai": true}, backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
		return nil, context.Canceled
	}}}
	snapshot := tracker.Snapshot()
	if len(snapshot) != 1 || !snapshot["openai"] {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestTrackerSnapshotUsesCachedValueOnBackendError(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := &Tracker{
		cacheTTL: defaultCacheTTL,
		known:    map[string]bool{"openai": true, "backup": true},
		cache: map[string]cachedHealth{
			"openai": {healthy: true, expiresAt: clock.Add(time.Minute)},
			"backup": {healthy: false, expiresAt: clock.Add(time.Minute)},
		},
		now: func() time.Time { return clock },
		backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
			return nil, context.Canceled
		}},
	}
	snapshot := tracker.Snapshot()
	if len(snapshot) != 2 || !snapshot["openai"] || snapshot["backup"] {
		t.Fatalf("snapshot = %+v", snapshot)
	}
}

func TestTrackerSnapshotDoesNotCacheFailOpenValues(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	invocations := 0
	tracker := &Tracker{known: map[string]bool{"openai": true}, now: func() time.Time { return clock }, backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
		invocations++
		if invocations == 1 {
			return nil, context.Canceled
		}
		return map[string]bool{"openai": false}, nil
	}}}
	if got := tracker.Snapshot(); !got["openai"] {
		t.Fatalf("first call should fail open when no cache; got %+v", got)
	}
	if got := tracker.Snapshot(); got["openai"] {
		t.Fatalf("second call should reflect unhealthy state from backend; got %+v", got)
	}
}

func TestTrackerAnyHealthyUsesCachedFallback(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := &Tracker{
		cacheTTL: defaultCacheTTL,
		cache:    map[string]cachedHealth{"openai": {healthy: false, expiresAt: clock.Add(time.Minute)}},
		now:      func() time.Time { return clock },
		backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
			return nil, context.Canceled
		}},
	}
	providers := map[string]config.Provider{"openai": {Name: "openai"}}
	if tracker.AnyHealthyContext(context.Background(), providers) {
		t.Fatal("cached unhealthy should suppress healthy fallback")
	}

	tracker2 := &Tracker{
		cacheTTL: defaultCacheTTL,
		cache:    map[string]cachedHealth{"openai": {healthy: true, expiresAt: clock.Add(time.Minute)}},
		now:      func() time.Time { return clock },
		backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
			return nil, context.Canceled
		}},
	}
	if !tracker2.AnyHealthyContext(context.Background(), providers) {
		t.Fatal("cached healthy should not be inverted on fallback")
	}
}

func TestTrackerAnyHealthyFailsOpenWithoutCache(t *testing.T) {
	tracker := &Tracker{backend: stubBackend{snapshot: func(ctx context.Context, names []string) (map[string]bool, error) {
		return nil, context.Canceled
	}}}
	providers := map[string]config.Provider{"openai": {Name: "openai"}}
	if !tracker.AnyHealthyContext(context.Background(), providers) {
		t.Fatal("should fail open when no cache exists")
	}
}

func TestTrackerMarkSuccessPopulatesCache(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := &Tracker{cacheTTL: defaultCacheTTL, now: func() time.Time { return clock }, backend: stubBackend{}}
	tracker.MarkSuccess("openai")
	clock = clock.Add(time.Second)
	if healthy, ok := tracker.readCache("openai"); !ok || !healthy {
		t.Fatalf("cache = %+v ok=%v", healthy, ok)
	}
	clock = clock.Add(defaultCacheTTL)
	if _, ok := tracker.readCache("openai"); ok {
		t.Fatal("cache entry should expire after TTL")
	}
}

func TestTrackerMarkFailurePopulatesCache(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tracker := &Tracker{cacheTTL: defaultCacheTTL, now: func() time.Time { return clock }, backend: stubBackend{}}
	tracker.MarkFailure("openai")
	if healthy, ok := tracker.readCache("openai"); !ok || healthy {
		t.Fatalf("cache = %+v ok=%v", healthy, ok)
	}

	clock = clock.Add(time.Second)
	tracker.backend = stubBackend{isHealthy: func(ctx context.Context, name string) (bool, error) {
		return true, context.Canceled
	}}
	if tracker.IsHealthyContext(context.Background(), "openai") {
		t.Fatal("MarkFailure should populate cache with unhealthy and survive backend error")
	}
}
