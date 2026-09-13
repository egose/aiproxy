package alias

import (
	"hash/fnv"
	"net/http"
	"strings"
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
	AcquireSpecific(want Target, exclude map[Target]bool) (Target, func(), bool)
}

// SessionKey returns the first non-empty session value from the request
// headers, following the alias's configured header precedence. The second
// return value is false when no affinity header is present.
func SessionKey(a config.Alias, header http.Header) (string, bool) {
	if a.SessionAffinity == nil || header == nil {
		return "", false
	}
	for _, name := range config.SessionAffinityHeaders(a) {
		if value := strings.TrimSpace(header.Get(name)); value != "" {
			return value, true
		}
	}
	return "", false
}

// PreferredTarget maps a session key to a stable pool position. Callers must
// fall back to Acquire when the preferred target is excluded, unhealthy, or
// cooling; affinity is a hint, never a guarantee.
func PreferredTarget(targets []Target, key string) (Target, bool) {
	if len(targets) == 0 || key == "" {
		return Target{}, false
	}
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return targets[h.Sum64()%uint64(len(targets))], true
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

func (r *roundRobin) AcquireSpecific(want Target, exclude map[Target]bool) (Target, func(), bool) {
	if exclude[want] {
		return Target{}, nil, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, t := range r.targets {
		if t == want {
			return t, func() {}, true
		}
	}
	return Target{}, nil, false
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
	return l.targets[idx], l.release(idx)
}

func (l *leastConnections) AcquireSpecific(want Target, exclude map[Target]bool) (Target, func(), bool) {
	if exclude[want] {
		return Target{}, nil, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.counts == nil {
		l.counts = make([]int, len(l.targets))
	}
	for i, t := range l.targets {
		if t == want {
			l.counts[i]++
			return t, l.release(i), true
		}
	}
	return Target{}, nil, false
}

func (l *leastConnections) release(idx int) func() {
	var once sync.Once
	return func() {
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
