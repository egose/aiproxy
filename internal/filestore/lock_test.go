package filestore

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestLockSerializesHolders(t *testing.T) {
	dir := t.TempDir()
	target := dir + "/keys.json"
	var current atomic.Int32
	var maxSeen atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := Lock(target)
			if err != nil {
				t.Errorf("Lock(): %v", err)
				return
			}
			defer unlock()
			n := current.Add(1)
			for {
				old := maxSeen.Load()
				if n <= old || maxSeen.CompareAndSwap(old, n) {
					break
				}
			}
			current.Add(-1)
		}()
	}
	wg.Wait()
	if got := maxSeen.Load(); got != 1 {
		t.Fatalf("max concurrent holders = %d, want 1", got)
	}
	unlock, err := Lock(target)
	if err != nil {
		t.Fatalf("Lock(): %v", err)
	}
	unlock()
}
