package alias

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func affinityAlias(headers ...string) config.Alias {
	a := config.Alias{Algorithm: config.AlgorithmRoundRobin, Targets: targets("a", "b", "c")}
	if headers != nil {
		a.SessionAffinity = &config.SessionAffinity{Headers: headers}
	}
	return a
}

func TestSessionKeyFirstHitWins(t *testing.T) {
	a := affinityAlias("x-opencode-session", "x-claude-code-session-id", "session-id")
	header := http.Header{
		"X-Unrelated":              []string{"wrong"},
		"X-Claude-Code-Session-Id": []string{"claude-1"},
		"Session-Id":               []string{"codex-1"},
	}
	key, ok := SessionKey(a, header)
	if !ok || key != "claude-1" {
		t.Fatalf("SessionKey = %q, %v; want %q, true", key, ok, "claude-1")
	}
}

func TestSessionKeyTrimsAndSkipsEmpty(t *testing.T) {
	a := affinityAlias("x-opencode-session", "session-id")
	header := http.Header{
		"X-Opencode-Session": []string{"   "},
		"Session-Id":         []string{"  s-9  "},
	}
	key, ok := SessionKey(a, header)
	if !ok || key != "s-9" {
		t.Fatalf("SessionKey = %q, %v; want %q, true", key, ok, "s-9")
	}
}

func TestSessionKeyAbsent(t *testing.T) {
	a := affinityAlias("x-opencode-session")
	if key, ok := SessionKey(a, http.Header{}); ok || key != "" {
		t.Fatalf("SessionKey = %q, %v; want empty, false", key, ok)
	}
	if key, ok := SessionKey(a, nil); ok || key != "" {
		t.Fatalf("SessionKey(nil) = %q, %v; want empty, false", key, ok)
	}
	plain := config.Alias{Algorithm: config.AlgorithmRoundRobin, Targets: targets("a")}
	header := http.Header{"X-Opencode-Session": []string{"s"}}
	if key, ok := SessionKey(plain, header); ok || key != "" {
		t.Fatalf("SessionKey(disabled) = %q, %v; want empty, false", key, ok)
	}
}

func TestPreferredTargetStable(t *testing.T) {
	pool := []Target{{Provider: "a", Model: "m0"}, {Provider: "b", Model: "m1"}}
	first, ok := PreferredTarget(pool, "session-1")
	if !ok {
		t.Fatal("PreferredTarget returned false")
	}
	for i := 0; i < 10; i++ {
		got, ok := PreferredTarget(pool, "session-1")
		if !ok || got != first {
			t.Fatalf("PreferredTarget = %+v, %v; want stable %+v", got, ok, first)
		}
	}
	if _, ok := PreferredTarget(pool, ""); ok {
		t.Fatal("PreferredTarget(empty key) = true, want false")
	}
	if _, ok := PreferredTarget(nil, "session-1"); ok {
		t.Fatal("PreferredTarget(empty pool) = true, want false")
	}
	seen := map[Target]bool{}
	for i := 0; i < 50; i++ {
		got, _ := PreferredTarget(pool, fmt.Sprintf("session-%d", i))
		seen[got] = true
	}
	if len(seen) != 2 {
		t.Fatalf("50 distinct keys mapped to %d targets, want both targets used", len(seen))
	}
}

func TestAcquireSpecificRoundRobinLeavesCursor(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmRoundRobin, Targets: targets("a", "b", "c")}
	s := NewSelector(a)
	want := Target{Provider: "c", Model: "m2"}
	got, release, ok := s.AcquireSpecific(want, nil)
	if !ok || got != want {
		t.Fatalf("AcquireSpecific = %+v, %v; want %+v, true", got, ok, want)
	}
	release()
	next, releaseNext := s.Acquire(nil)
	defer releaseNext()
	if next != (Target{Provider: "a", Model: "m0"}) {
		t.Fatalf("Acquire after affinity = %+v, want first target (cursor untouched)", next)
	}
}

func TestAcquireSpecificRespectsExcludeAndPool(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmRoundRobin, Targets: targets("a", "b")}
	s := NewSelector(a)
	want := Target{Provider: "a", Model: "m0"}
	if _, _, ok := s.AcquireSpecific(want, map[Target]bool{want: true}); ok {
		t.Fatal("AcquireSpecific(excluded) = true, want false")
	}
	if _, _, ok := s.AcquireSpecific(Target{Provider: "nope", Model: "x"}, nil); ok {
		t.Fatal("AcquireSpecific(unknown) = true, want false")
	}
}

func TestAcquireSpecificLeastConnectionsTracksInflight(t *testing.T) {
	a := config.Alias{Algorithm: config.AlgorithmLeastConnections, Targets: targets("a", "b")}
	s := NewSelector(a)
	want := Target{Provider: "a", Model: "m0"}
	got, release, ok := s.AcquireSpecific(want, nil)
	if !ok || got != want {
		t.Fatalf("AcquireSpecific = %+v, %v; want %+v, true", got, ok, want)
	}
	defer release()
	next, releaseNext := s.Acquire(nil)
	defer releaseNext()
	if next.Provider != "b" {
		t.Fatalf("Acquire during affinity lease = %+v, want idle provider b", next)
	}
}
