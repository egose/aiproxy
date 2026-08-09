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
		tt, release := s.Acquire(nil)
		seen[tt.Provider]++
		release()
	}
	if seen["a"] != 3 || seen["b"] != 3 || seen["c"] != 3 {
		t.Errorf("round robin distribution: %+v", seen)
	}
}

func TestRoundRobinSingleTarget(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmRoundRobin, Targets: targets("only")}
	s := NewSelector(a)
	for i := 0; i < 5; i++ {
		got, release := s.Acquire(nil)
		if got.Provider != "only" {
			t.Errorf("got %q, want only", got.Provider)
		}
		release()
	}
}

func TestLeastConnectionsPrefersIdle(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmLeastConnections, Targets: targets("a", "b", "c")}
	s := NewSelector(a)

	first, releaseFirst := s.Acquire(nil)
	defer releaseFirst()
	second, releaseSecond := s.Acquire(nil)
	defer releaseSecond()
	if first.Provider == second.Provider {
		t.Errorf("second least-conn pick reused busy target %q", first.Provider)
	}
	third, releaseThird := s.Acquire(nil)
	defer releaseThird()
	if third.Provider == first.Provider || third.Provider == second.Provider {
		t.Errorf("third least-conn pick reused a busy target: %+v", third)
	}
}

func TestLeastConnectionsReleaseReuses(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmLeastConnections, Targets: targets("a", "b")}
	s := NewSelector(a)

	first, release := s.Acquire(nil)
	release()
	release()
	// After release, counts are zero again; subsequent picks may reuse first.
	reused := false
	for i := 0; i < 10; i++ {
		got, release := s.Acquire(nil)
		if got.Provider == first.Provider {
			reused = true
		}
		release()
	}
	if !reused {
		t.Errorf("released target was never reused")
	}
}

func TestEmptyPoolReturnsZeroTarget(t *testing.T) {
	s := NewSelector(config.Alias{Algorithm: config.AlgorithmRoundRobin})
	if got, release := s.Acquire(nil); got != (Target{}) || release != nil {
		t.Errorf("expected zero Target, got %+v", got)
	}
}

func TestAcquireExcludesTargets(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmLeastConnections, Targets: targets("a", "b", "c")}
	s := NewSelector(a)

	got, release := s.Acquire(map[Target]bool{{Provider: "a", Model: "m0"}: true, {Provider: "b", Model: "m1"}: true})
	defer release()
	if got.Provider != "c" {
		t.Fatalf("got %+v, want provider c", got)
	}

	got, release = s.Acquire(map[Target]bool{{Provider: "a", Model: "m0"}: true, {Provider: "b", Model: "m1"}: true, {Provider: "c", Model: "m2"}: true})
	if got != (Target{}) || release != nil {
		t.Fatalf("got %+v release nil=%v, want no target", got, release == nil)
	}
}
