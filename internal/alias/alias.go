package alias

import (
	"sync"
	"sync/atomic"

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
	Select() Target
	Release(t Target)
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

func (r *roundRobin) Select() Target {
	if len(r.targets) == 0 {
		return Target{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	i := r.cursor % uint64(len(r.targets))
	r.cursor++
	return r.targets[i]
}

func (r *roundRobin) Release(Target) {}

type leastConnections struct {
	mu      sync.Mutex
	targets []Target
	counts  []int64
}

func (l *leastConnections) Select() Target {
	if len(l.targets) == 0 {
		return Target{}
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts == nil {
		l.counts = make([]int64, len(l.targets))
	}
	idx := 0
	for i := 1; i < len(l.targets); i++ {
		if atomic.LoadInt64(&l.counts[i]) < atomic.LoadInt64(&l.counts[idx]) {
			idx = i
		}
	}
	atomic.AddInt64(&l.counts[idx], 1)
	return l.targets[idx]
}

func (l *leastConnections) Release(t Target) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts == nil {
		return
	}
	for i, tgt := range l.targets {
		if tgt == t {
			if c := atomic.AddInt64(&l.counts[i], -1); c < 0 {
				atomic.StoreInt64(&l.counts[i], 0)
			}
			return
		}
	}
}
