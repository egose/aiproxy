package app

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestFailedRuntimeBundleDoesNotRegisterOrCloseUnadoptedCandidateResources(t *testing.T) {
	manager := newRuntimeResourceManager()
	entered, gate := make(chan struct{}), make(chan struct{})
	var oldCloses, candidateCloses atomic.Int32
	oldResource := ownedRuntimeResource{key: "old", close: func() error {
		oldCloses.Add(1)
		close(entered)
		<-gate
		return nil
	}}
	old, _, err := manager.prepare([]ownedRuntimeResource{oldResource})
	if err != nil {
		t.Fatal(err)
	}
	old.Retire()
	<-entered
	candidate := ownedRuntimeResource{key: "candidate", close: func() error { candidateCloses.Add(1); return nil }}
	if _, _, err := manager.prepare([]ownedRuntimeResource{candidate, oldResource}); err == nil {
		t.Fatal("retiring owner was resurrected")
	}
	manager.mu.Lock()
	registered := manager.owners[candidate.key] != nil
	manager.mu.Unlock()
	if registered || candidateCloses.Load() != 0 {
		t.Fatal("failed bundle adopted or asynchronously closed a caller-owned candidate")
	}
	if err := candidate.close(); err != nil {
		t.Fatal(err)
	}
	close(gate)
	manager.beginShutdown()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := manager.wait(ctx); err != nil {
		t.Fatal(err)
	}
	if oldCloses.Load() != 1 || candidateCloses.Load() != 1 {
		t.Fatal("failed candidate cleanup duplicated resource closes")
	}
}
