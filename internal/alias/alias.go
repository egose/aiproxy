package alias

import (
	"sync"

	"github.com/egose/aiproxy/internal/config"
)

// Target is a concrete provider/model pair selected by a Balancer.
type Target struct {
	Provider string
	Model    string
}

// Selector chooses one target from an alias's pool, possibly tracking state
// across calls. Selectors are safe for concurrent use.
type Selector interface {
	Acquire(exclude map[Target]bool) (Target, func())
}

// NewSelector returns a stateful selector for the given algorithm. Unknown
// algorithms fall back to round_robin to keep the proxy working if config
// validation is bypassed (tests, future algorithms).
func NewSelector(a config.Alias) Selector {
	targets := make([]Target, 0, len(a.Targets))
	for _, t := range a.Targets {
		targets = append(targets, Target{Provider: t.Provider, Model: t.Model})
	}
	switch a.Algorithm {
	case config.AlgorithmLeastConnections:
		return &leastConnections{targets: targets}
	default:
		return &roundRobin{targets: targets}
	}
}

type roundRobin struct {
	mu      sync.Mutex
	targets []Target
	cursor  uint64
}

func (r *roundRobin) Acquire(exclude map[Target]bool) (Target, func()) {
	if len(r.targets) == 0 {
		return Target{}, nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for i := 0; i < len(r.targets); i++ {
		idx := (r.cursor + uint64(i)) % uint64(len(r.targets))
		t := r.targets[idx]
		if exclude[t] {
			continue
		}
		r.cursor = idx + 1
		return t, func() {}
	}
	return Target{}, nil
}

type leastConnections struct {
	mu      sync.Mutex
	targets []Target
	counts  []int
}

func (l *leastConnections) Acquire(exclude map[Target]bool) (Target, func()) {
	if len(l.targets) == 0 {
		return Target{}, nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts == nil {
		l.counts = make([]int, len(l.targets))
	}
	idx := -1
	for i, t := range l.targets {
		if exclude[t] {
			continue
		}
		if idx == -1 || l.counts[i] < l.counts[idx] {
			idx = i
		}
	}
	if idx == -1 {
		return Target{}, nil
	}
	l.counts[idx]++
	var once sync.Once
	return l.targets[idx], func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			if l.counts == nil || l.counts[idx] == 0 {
				return
			}
			l.counts[idx]--
		})
	}
}
