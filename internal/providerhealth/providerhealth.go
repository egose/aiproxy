package providerhealth

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

const (
	defaultCooldown = 30 * time.Second
	defaultCacheTTL = 30 * time.Second
)

type backend interface {
	MarkSuccess(ctx context.Context, name string) error
	MarkFailure(ctx context.Context, name string, cooldown time.Duration) error
	IsHealthy(ctx context.Context, name string) (bool, error)
	Snapshot(ctx context.Context, names []string) (map[string]bool, error)
	Close() error
}

type cachedHealth struct {
	healthy   bool
	expiresAt time.Time
}

type Tracker struct {
	closeOnce sync.Once
	closeErr  error
	mu        sync.Mutex
	known     map[string]bool
	cache     map[string]cachedHealth
	cooldown  time.Duration
	cacheTTL  time.Duration
	now       func() time.Time
	logger    *slog.Logger
	backend   backend
	metrics   *observability.Metrics
}

type Backend = backend

func New(metrics *observability.Metrics, cfg config.ProviderHealth) *Tracker {
	return newTracker(metrics, cfg, nil)
}

func NewWithBackend(metrics *observability.Metrics, cfg config.ProviderHealth, backend Backend) *Tracker {
	t := newTracker(metrics, cfg, nil)
	if backend != nil {
		t.backend = backend
	}
	return t
}

func newTracker(metrics *observability.Metrics, cfg config.ProviderHealth, logger *slog.Logger) *Tracker {
	cooldown := cfg.Cooldown
	if cooldown <= 0 {
		cooldown = defaultCooldown
	}
	cacheTTL := cfg.CacheTTL
	if cacheTTL <= 0 {
		cacheTTL = defaultCacheTTL
	}
	t := &Tracker{
		known:    make(map[string]bool),
		cache:    make(map[string]cachedHealth),
		cooldown: cooldown,
		cacheTTL: cacheTTL,
		now:      time.Now,
		logger:   logger,
		metrics:  metrics,
	}
	if cfg.RedisURL != "" {
		backend, err := newRedisBackend(cfg.RedisURL, cfg.KeyPrefix)
		if err != nil {
			t.backend = newMemoryBackend()
			t.recordBackendError("configure")
		} else {
			t.backend = backend
		}
	} else {
		t.backend = newMemoryBackend()
	}
	return t
}

func (t *Tracker) SetProviders(catalog config.Catalog) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	names := catalog.ProviderNames()
	known := make(map[string]bool, len(names))
	for _, name := range names {
		known[name] = true
		if t.metrics != nil {
			t.metrics.SetProviderHealthy(name, true)
		}
	}
	for name := range t.known {
		if !known[name] {
			if t.metrics != nil {
				t.metrics.RemoveProviderHealthy(name)
			}
			delete(t.cache, name)
		}
	}
	t.known = known
}

func (t *Tracker) Close() error {
	if t == nil || t.backend == nil {
		return nil
	}
	t.closeOnce.Do(func() {
		t.closeErr = t.backend.Close()
	})
	return t.closeErr
}

func (t *Tracker) Snapshot() map[string]bool {
	return t.SnapshotContext(context.Background())
}

func (t *Tracker) SnapshotContext(ctx context.Context) map[string]bool {
	if t == nil {
		return nil
	}
	names := t.providerNames()
	out, err := t.backend.Snapshot(ctx, names)
	if err != nil {
		t.recordBackendError("snapshot")
		return t.fallbackSnapshot(names, "snapshot")
	}
	t.recordHealth(out)
	t.updateCache(out)
	return out
}

func (t *Tracker) providerNames() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	names := make([]string, 0, len(t.known))
	for name := range t.known {
		names = append(names, name)
	}
	return names
}

func (t *Tracker) MarkSuccess(name string) {
	t.MarkSuccessContext(context.Background(), name)
}

func (t *Tracker) MarkSuccessContext(ctx context.Context, name string) {
	if t == nil || name == "" {
		return
	}
	_ = t.backend.MarkSuccess(ctx, name)
	t.writeCache(name, true)
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, true)
	}
}

func (t *Tracker) MarkFailure(name string) {
	t.MarkFailureContext(context.Background(), name)
}

func (t *Tracker) MarkFailureContext(ctx context.Context, name string) {
	if t == nil || name == "" {
		return
	}
	_ = t.backend.MarkFailure(ctx, name, t.cooldown)
	t.writeCache(name, false)
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, false)
	}
}

func (t *Tracker) IsHealthy(name string) bool {
	return t.IsHealthyContext(context.Background(), name)
}

func (t *Tracker) IsHealthyContext(ctx context.Context, name string) bool {
	if t == nil || name == "" {
		return true
	}
	healthy, err := t.backend.IsHealthy(ctx, name)
	if err != nil {
		t.recordBackendError("is_healthy")
		fallback := t.fallbackHealth(name, "is_healthy")
		if t.metrics != nil {
			t.metrics.SetProviderHealthy(name, fallback)
		}
		return fallback
	}
	t.writeCache(name, healthy)
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, healthy)
	}
	return healthy
}

func (t *Tracker) AnyHealthy(catalog config.Catalog) bool {
	return t.AnyHealthyCatalogContext(context.Background(), catalog)
}

func (t *Tracker) AnyHealthyCatalogContext(ctx context.Context, catalog config.Catalog) bool {
	names := catalog.ProviderNames()
	if len(names) == 0 {
		return false
	}
	health, err := t.backend.Snapshot(ctx, names)
	if err != nil {
		t.recordBackendError("any_healthy")
		health = t.fallbackSnapshot(names, "any_healthy")
		for _, healthy := range health {
			if healthy {
				return true
			}
		}
		return false
	}
	t.recordHealth(health)
	t.updateCache(health)
	for _, healthy := range health {
		if healthy {
			return true
		}
	}
	return false
}

func (t *Tracker) recordHealth(health map[string]bool) {
	if t.metrics == nil {
		return
	}
	for name, healthy := range health {
		t.metrics.SetProviderHealthy(name, healthy)
	}
}

func (t *Tracker) recordBackendError(operation string) {
	if t.metrics != nil {
		t.metrics.RecordProviderHealthBackendError(operation)
	}
}

func (t *Tracker) recordFallback(operation, reason string) {
	if t.metrics != nil {
		t.metrics.RecordProviderHealthFallback(operation, reason)
	}
	if t.logger != nil {
		t.logger.Debug("provider health fallback",
			"operation", operation,
			"reason", reason)
	}
}

func (t *Tracker) fallbackHealth(name, operation string) bool {
	cached, ok := t.readCache(name)
	if !ok {
		t.recordFallback(operation, "open_no_cache")
		return true
	}
	t.recordFallback(operation, "cached")
	return cached
}

func (t *Tracker) fallbackSnapshot(names []string, operation string) map[string]bool {
	out := make(map[string]bool, len(names))
	uncached := make([]string, 0, len(names))
	for _, name := range names {
		cached, ok := t.readCache(name)
		if !ok {
			uncached = append(uncached, name)
			out[name] = true
			continue
		}
		out[name] = cached
	}
	if len(uncached) > 0 {
		t.recordFallback(operation, "open_no_cache")
	} else {
		t.recordFallback(operation, "cached")
	}
	t.recordHealth(out)
	return out
}

func (t *Tracker) readCache(name string) (bool, bool) {
	if t == nil {
		return false, false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		return false, false
	}
	state, ok := t.cache[name]
	if !ok {
		return false, false
	}
	if t.curNow().After(state.expiresAt) {
		return false, false
	}
	return state.healthy, true
}

func (t *Tracker) writeCache(name string, healthy bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		t.cache = make(map[string]cachedHealth)
	}
	t.cache[name] = cachedHealth{healthy: healthy, expiresAt: t.curNow().Add(t.cacheTTL)}
}

func (t *Tracker) updateCache(health map[string]bool) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cache == nil {
		t.cache = make(map[string]cachedHealth, len(health))
	}
	expiry := t.curNow().Add(t.cacheTTL)
	for name, healthy := range health {
		t.cache[name] = cachedHealth{healthy: healthy, expiresAt: expiry}
	}
}

func (t *Tracker) curNow() time.Time {
	if t.now != nil {
		return t.now()
	}
	return time.Now()
}

type memoryBackend struct {
	mu     sync.Mutex
	states map[string]time.Time
	now    func() time.Time
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{states: make(map[string]time.Time), now: time.Now}
}

func (b *memoryBackend) MarkSuccess(_ context.Context, name string) error {
	b.mu.Lock()
	b.states[name] = time.Time{}
	b.mu.Unlock()
	return nil
}

func (b *memoryBackend) MarkFailure(_ context.Context, name string, cooldown time.Duration) error {
	b.mu.Lock()
	b.states[name] = b.now().Add(cooldown)
	b.mu.Unlock()
	return nil
}

func (b *memoryBackend) IsHealthy(_ context.Context, name string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	until, ok := b.states[name]
	if !ok || until.IsZero() {
		return true, nil
	}
	if !b.now().Before(until) {
		b.states[name] = time.Time{}
		return true, nil
	}
	return false, nil
}

func (b *memoryBackend) Snapshot(ctx context.Context, names []string) (map[string]bool, error) {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		healthy, err := b.IsHealthy(ctx, name)
		if err != nil {
			return nil, err
		}
		out[name] = healthy
	}
	return out, nil
}

func (b *memoryBackend) Close() error {
	return nil
}
