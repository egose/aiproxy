package providerhealth

import (
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

const defaultCooldown = 30 * time.Second

type backend interface {
	MarkSuccess(name string) error
	MarkFailure(name string, cooldown time.Duration) error
	IsHealthy(name string) (bool, error)
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
		t.backend = newRedisBackend(cfg.RedisURL, cfg.KeyPrefix)
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

func (t *Tracker) MarkSuccess(name string) {
	if t == nil || name == "" {
		return
	}
	_ = t.backend.MarkSuccess(name)
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, true)
	}
}

func (t *Tracker) MarkFailure(name string) {
	if t == nil || name == "" {
		return
	}
	_ = t.backend.MarkFailure(name, t.cooldown)
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, false)
	}
}

func (t *Tracker) IsHealthy(name string) bool {
	if t == nil || name == "" {
		return true
	}
	healthy, err := t.backend.IsHealthy(name)
	if err != nil {
		return true
	}
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, healthy)
	}
	return healthy
}

func (t *Tracker) AnyHealthy(providers map[string]config.Provider) bool {
	if len(providers) == 0 {
		return false
	}
	for name := range providers {
		if t.IsHealthy(name) {
			return true
		}
	}
	return false
}

type memoryBackend struct {
	mu     sync.Mutex
	states map[string]time.Time
	now    func() time.Time
}

func newMemoryBackend() *memoryBackend {
	return &memoryBackend{states: make(map[string]time.Time), now: time.Now}
}

func (b *memoryBackend) MarkSuccess(name string) error {
	b.mu.Lock()
	b.states[name] = time.Time{}
	b.mu.Unlock()
	return nil
}

func (b *memoryBackend) MarkFailure(name string, cooldown time.Duration) error {
	b.mu.Lock()
	b.states[name] = b.now().Add(cooldown)
	b.mu.Unlock()
	return nil
}

func (b *memoryBackend) IsHealthy(name string) (bool, error) {
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
