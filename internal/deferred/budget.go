package deferred

import (
	"errors"
	"sync"
)

var ErrBudgetExceeded = errors.New("deferred work budget exceeded")
var ErrBudgetClosed = errors.New("deferred work budget closed")

type ByteBudget interface {
	Reserve(int64) (release func(), err error)
}

type WorkBudget struct {
	mu        sync.Mutex
	limit     int64
	used      int64
	closed    bool
	onFailure func()
	onDrained func()
}

func newWorkBudget(limit int64, failure, drained func()) *WorkBudget {
	return &WorkBudget{limit: limit, onFailure: failure, onDrained: drained}
}

func (b *WorkBudget) Reserve(n int64) (func(), error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrBudgetClosed
	}
	if n < 0 || n > b.limit-b.used {
		failure := b.onFailure
		b.mu.Unlock()
		if failure != nil {
			failure()
		}
		return nil, ErrBudgetExceeded
	}
	b.used += n
	b.mu.Unlock()
	return sync.OnceFunc(func() {
		b.mu.Lock()
		b.used -= n
		var drained func()
		if b.closed && b.used == 0 {
			drained = b.onDrained
			b.onDrained = nil
		}
		b.mu.Unlock()
		if drained != nil {
			drained()
		}
	}), nil
}

func (b *WorkBudget) Used() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.used
}

func (b *WorkBudget) Limit() int64 {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.limit
}

func (b *WorkBudget) close() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	b.onFailure = nil
	if b.used == 0 {
		b.onDrained = nil
		return true
	}
	return false
}
