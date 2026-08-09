package providerhealth

import (
	"context"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

const defaultCooldown = 30 * time.Second

type backend interface {
	MarkSuccess(ctx context.Context, name string) error
	MarkFailure(ctx context.Context, name string, cooldown time.Duration) error
	IsHealthy(ctx context.Context, name string) (bool, error)
	Snapshot(ctx context.Context, names []string) (map[string]bool, error)
	Close() error
}

type Tracker struct {
	mu       sync.Mutex
	known    map[string]bool
	cooldown time.Duration
	backend  backend
	metrics  *observability.Metrics
}

func New(metrics *observability.Metrics, cfg config.ProviderHealth) *Tracker {
	cooldown := cfg.Cooldown
	if cooldown <= 0 {
		cooldown = defaultCooldown
	}
	t := &Tracker{
		known:    make(map[string]bool),
		cooldown: cooldown,
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

func (t *Tracker) SetProviders(providers map[string]config.Provider) {
	if t == nil {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	known := make(map[string]bool, len(providers))
	for name := range providers {
		known[name] = true
		if t.metrics != nil {
			t.metrics.SetProviderHealthy(name, true)
		}
	}
	for name := range t.known {
		if !known[name] && t.metrics != nil {
			t.metrics.RemoveProviderHealthy(name)
		}
	}
	t.known = known
}

func (t *Tracker) Close() error {
	if t == nil || t.backend == nil {
		return nil
	}
	return t.backend.Close()
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
		out = make(map[string]bool, len(names))
		for _, name := range names {
			out[name] = true
		}
	}
	t.recordHealth(out)
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
		return true
	}
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, healthy)
	}
	return healthy
}

func (t *Tracker) AnyHealthy(providers map[string]config.Provider) bool {
	return t.AnyHealthyContext(context.Background(), providers)
}

func (t *Tracker) AnyHealthyContext(ctx context.Context, providers map[string]config.Provider) bool {
	if len(providers) == 0 {
		return false
	}
	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	health, err := t.backend.Snapshot(ctx, names)
	if err != nil {
		t.recordBackendError("any_healthy")
		return true
	}
	t.recordHealth(health)
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
