package httpapi

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var ErrResourcesIncomplete = errors.New("runtime resource cleanup incomplete")

type ResourceOwner struct {
	mu       sync.Mutex
	refs     int
	closing  bool
	close    func() error
	schedule func(func())
	done     chan struct{}
	err      error
}

func NewResourceOwner(close func() error, schedule func(func())) *ResourceOwner {
	if close == nil {
		close = func() error { return nil }
	}
	if schedule == nil {
		schedule = func(call func()) { call() }
	}
	return &ResourceOwner{close: close, schedule: schedule, done: make(chan struct{})}
}

func (o *ResourceOwner) retain() bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.closing {
		return false
	}
	o.refs++
	return true
}

func (o *ResourceOwner) release() {
	o.mu.Lock()
	o.refs--
	if o.refs != 0 {
		o.mu.Unlock()
		return
	}
	o.closing = true
	schedule := o.schedule
	o.mu.Unlock()
	schedule(func() {
		err := func() (err error) {
			defer func() {
				if recover() != nil {
					err = fmt.Errorf("runtime resource close failed")
				}
			}()
			return o.close()
		}()
		o.mu.Lock()
		o.err = err
		o.close, o.schedule = nil, nil
		close(o.done)
		o.mu.Unlock()
	})
}

func (o *ResourceOwner) Wait(ctx context.Context) error {
	select {
	case <-o.done:
		o.mu.Lock()
		defer o.mu.Unlock()
		return o.err
	default:
	}
	select {
	case <-o.done:
		o.mu.Lock()
		defer o.mu.Unlock()
		return o.err
	case <-ctx.Done():
		return errors.Join(ErrResourcesIncomplete, ctx.Err())
	}
}

type RuntimeResources struct {
	mu       sync.Mutex
	refs     int
	retired  bool
	released bool
	owners   []*ResourceOwner
	done     chan struct{}
}

func NewRuntimeResources(owners ...*ResourceOwner) (*RuntimeResources, error) {
	resources := &RuntimeResources{refs: 1, done: make(chan struct{})}
	seen := make(map[*ResourceOwner]bool, len(owners))
	for _, owner := range owners {
		if owner == nil || seen[owner] {
			continue
		}
		if !owner.retain() {
			for _, held := range resources.owners {
				held.release()
			}
			return nil, fmt.Errorf("runtime resource owner is already retiring")
		}
		seen[owner] = true
		resources.owners = append(resources.owners, owner)
	}
	return resources, nil
}

func (r *RuntimeResources) Acquire() (release func(), ok bool) {
	if r == nil {
		return func() {}, true
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.retired {
		return nil, false
	}
	r.refs++
	return sync.OnceFunc(r.release), true
}

func (r *RuntimeResources) release() {
	r.mu.Lock()
	r.refs--
	if r.refs != 0 {
		r.mu.Unlock()
		return
	}
	owners := r.owners
	r.owners = nil
	r.mu.Unlock()
	for _, owner := range owners {
		owner.release()
	}
	close(r.done)
}

func (r *RuntimeResources) Retire() {
	if r == nil {
		return
	}
	r.mu.Lock()
	if r.released {
		r.mu.Unlock()
		return
	}
	r.retired = true
	r.released = true
	r.mu.Unlock()
	r.release()
}

func (r *RuntimeResources) StopAdmission() {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.retired = true
	r.mu.Unlock()
}

func (h *Handler) StopAdmission() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deps.RuntimeResources.StopAdmission()
}

func (r *RuntimeResources) Wait(ctx context.Context) error {
	if r == nil {
		return nil
	}
	select {
	case <-r.done:
		return nil
	default:
	}
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
		return errors.Join(ErrResourcesIncomplete, ctx.Err())
	}
}

func (h *Handler) SnapshotDependenciesWithLease() (Dependencies, func(), bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	deps := h.deps
	release, ok := deps.RuntimeResources.Acquire()
	return deps, release, ok
}

func (h *Handler) UpdateDependenciesWithActivation(deps Dependencies, activate func() error) error {
	return h.ActivateDependencies(func() (Dependencies, error) {
		if activate != nil {
			if err := activate(); err != nil {
				return Dependencies{}, err
			}
		}
		return deps, nil
	})
}

func (h *Handler) ActivateDependencies(activate func() (Dependencies, error)) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	deps, err := activate()
	if err != nil {
		return err
	}
	deps = normalizeDependencies(deps)
	deps.revision = h.deps.revision + 1
	h.deps = deps
	return nil
}
