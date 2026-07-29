package providerhealth

import (
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

const defaultCooldown = 30 * time.Second

type Tracker struct {
	mu       sync.Mutex
	states   map[string]time.Time
	cooldown time.Duration
	now      func() time.Time
	metrics  *observability.Metrics
}

func New(metrics *observability.Metrics) *Tracker {
	return &Tracker{
		states:   make(map[string]time.Time),
		cooldown: defaultCooldown,
		now:      time.Now,
		metrics:  metrics,
	}
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
		if _, ok := t.states[name]; !ok {
			t.states[name] = time.Time{}
		}
		if t.metrics != nil {
			t.metrics.SetProviderHealthy(name, true)
		}
	}
	for name := range t.states {
		if !known[name] {
			delete(t.states, name)
			if t.metrics != nil {
				t.metrics.RemoveProviderHealthy(name)
			}
		}
	}
}

func (t *Tracker) MarkSuccess(name string) {
	if t == nil || name == "" {
		return
	}
	t.mu.Lock()
	t.states[name] = time.Time{}
	t.mu.Unlock()
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, true)
	}
}

func (t *Tracker) MarkFailure(name string) {
	if t == nil || name == "" {
		return
	}
	t.mu.Lock()
	t.states[name] = t.now().Add(t.cooldown)
	t.mu.Unlock()
	if t.metrics != nil {
		t.metrics.SetProviderHealthy(name, false)
	}
}

func (t *Tracker) IsHealthy(name string) bool {
	if t == nil || name == "" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	until, ok := t.states[name]
	if !ok || until.IsZero() {
		return true
	}
	if !t.now().Before(until) {
		t.states[name] = time.Time{}
		if t.metrics != nil {
			t.metrics.SetProviderHealthy(name, true)
		}
		return true
	}
	return false
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
