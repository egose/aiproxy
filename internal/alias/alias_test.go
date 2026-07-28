package alias

import (
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func targets(n ...string) []config.AliasTarget {
	out := make([]config.AliasTarget, 0, len(n))
	for i, name := range n {
		out = append(out, config.AliasTarget{Provider: name, Model: "m" + string(rune('0'+i))})
	}
	return out
}

func TestRoundRobinRotates(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmRoundRobin, Targets: targets("a", "b", "c")}
	s := NewSelector(a)
	seen := map[string]int{}
	for i := 0; i < 9; i++ {
		tt := s.Select()
		seen[tt.Provider]++
	}
	if seen["a"] != 3 || seen["b"] != 3 || seen["c"] != 3 {
		t.Errorf("round robin distribution: %+v", seen)
	}
}

func TestRoundRobinSingleTarget(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmRoundRobin, Targets: targets("only")}
	s := NewSelector(a)
	for i := 0; i < 5; i++ {
		if got := s.Select(); got.Provider != "only" {
			t.Errorf("got %q, want only", got.Provider)
		}
	}
}

func TestLeastConnectionsPrefersIdle(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmLeastConnections, Targets: targets("a", "b", "c")}
	s := NewSelector(a)

	first := s.Select()
	// Without release, first target stays busy; second pick should be different.
	second := s.Select()
	if first.Provider == second.Provider {
		t.Errorf("second least-conn pick reused busy target %q", first.Provider)
	}
	third := s.Select()
	if third.Provider == first.Provider || third.Provider == second.Provider {
		t.Errorf("third least-conn pick reused a busy target: %+v", third)
	}
}

func TestLeastConnectionsReleaseReuses(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmLeastConnections, Targets: targets("a", "b")}
	s := NewSelector(a)

	first := s.Select()
	s.Release(first)
	// After release, counts are zero again; subsequent picks may reuse first.
	reused := false
	for i := 0; i < 10; i++ {
		if s.Select().Provider == first.Provider {
			reused = true
		}
	}
	if !reused {
		t.Errorf("released target was never reused")
	}
}

func TestEmptyPoolReturnsZeroTarget(t *testing.T) {
	s := NewSelector(config.Alias{Algorithm: config.AlgorithmRoundRobin})
	if got := s.Select(); got != (Target{}) {
		t.Errorf("expected zero Target, got %+v", got)
	}
}
