package app

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/mongolog"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/store"
)

const maxRuntimeResourceOwners = 16384

type ownedRuntimeResource struct {
	key   any
	close func() error
}

type runtimeResourceManager struct {
	prepareMu sync.Mutex
	mu        sync.Mutex
	owners    map[any]*httpapi.ResourceOwner
	queue     []func()
	head      int
	running   bool
	closing   bool
	closeErr  error
	changed   chan struct{}
}

func newRuntimeResourceManager() *runtimeResourceManager {
	return &runtimeResourceManager{owners: make(map[any]*httpapi.ResourceOwner), changed: make(chan struct{})}
}

func runtimeResourceList(clients *upstreamClientPool, health *providerhealth.Tracker, disk *payloadlog.Logger, mongo *mongolog.Logger, admin *store.Store) []ownedRuntimeResource {
	var resources []ownedRuntimeResource
	if clients != nil {
		resources = append(resources, ownedRuntimeResource{clients, func() error { clients.Retire(); return nil }})
	}
	if health != nil {
		resources = append(resources, ownedRuntimeResource{health, health.Close})
	}
	if disk != nil {
		resources = append(resources, ownedRuntimeResource{disk, disk.Close})
	}
	if mongo != nil {
		resources = append(resources, ownedRuntimeResource{mongo, mongo.Close})
	}
	if admin != nil {
		resources = append(resources, ownedRuntimeResource{admin, admin.Close})
	}
	return resources
}

func (m *runtimeResourceManager) prepare(resources []ownedRuntimeResource) (*httpapi.RuntimeResources, []*httpapi.ResourceOwner, error) {
	m.prepareMu.Lock()
	defer m.prepareMu.Unlock()
	m.mu.Lock()
	if m.closing {
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("runtime resources are shutting down")
	}
	missing := 0
	for _, resource := range resources {
		if m.owners[resource.key] == nil {
			missing++
		}
	}
	if len(m.owners)+missing > maxRuntimeResourceOwners {
		m.mu.Unlock()
		return nil, nil, fmt.Errorf("runtime resource retirement capacity is exhausted")
	}
	owners := make([]*httpapi.ResourceOwner, 0, len(resources))
	fresh := make(map[any]*httpapi.ResourceOwner)
	var acquired []*httpapi.ResourceOwner
	for _, resource := range resources {
		owner := m.owners[resource.key]
		if owner == nil {
			if fresh[resource.key] != nil {
				continue
			}
			resource := resource
			owner = httpapi.NewResourceOwner(func() (err error) {
				defer func() {
					if recover() != nil {
						err = fmt.Errorf("runtime resource close failed")
					}
					m.mu.Lock()
					delete(m.owners, resource.key)
					if err != nil && m.closeErr == nil {
						m.closeErr = err
					}
					m.notifyLocked()
					m.mu.Unlock()
				}()
				return resource.close()
			}, m.schedule)
			fresh[resource.key] = owner
			acquired = append(acquired, owner)
		} else {
			owners = append(owners, owner)
		}
	}
	m.mu.Unlock()
	owners = append(owners, acquired...)
	bundle, err := httpapi.NewRuntimeResources(owners...)
	if err != nil {
		return nil, nil, err
	}
	m.mu.Lock()
	for key, owner := range fresh {
		m.owners[key] = owner
	}
	m.mu.Unlock()
	return bundle, acquired, nil
}

func (m *runtimeResourceManager) schedule(call func()) {
	m.mu.Lock()
	if m.head > 0 && len(m.queue) == cap(m.queue) {
		copy(m.queue, m.queue[m.head:])
		m.queue = m.queue[:len(m.queue)-m.head]
		m.head = 0
	}
	m.queue = append(m.queue, call)
	if !m.running {
		m.running = true
		go m.run()
	}
	m.mu.Unlock()
}

func (m *runtimeResourceManager) run() {
	for {
		m.mu.Lock()
		if m.head == len(m.queue) {
			m.queue, m.head, m.running = nil, 0, false
			m.notifyLocked()
			m.mu.Unlock()
			return
		}
		call := m.queue[m.head]
		m.queue[m.head] = nil
		m.head++
		m.mu.Unlock()
		call()
	}
}

func (m *runtimeResourceManager) notifyLocked() {
	close(m.changed)
	m.changed = make(chan struct{})
}

func (m *runtimeResourceManager) beginShutdown() {
	m.prepareMu.Lock()
	defer m.prepareMu.Unlock()
	m.mu.Lock()
	m.closing = true
	m.notifyLocked()
	m.mu.Unlock()
}

func (m *runtimeResourceManager) wait(ctx context.Context) error {
	for {
		m.mu.Lock()
		err, remaining, changed := m.closeErr, len(m.owners), m.changed
		m.mu.Unlock()
		if remaining == 0 {
			return err
		}
		select {
		case <-changed:
		case <-ctx.Done():
			return errors.Join(err, httpapi.ErrResourcesIncomplete, ctx.Err())
		}
	}
}

func waitCandidateOwners(ctx context.Context, owners []*httpapi.ResourceOwner) error {
	var err error
	for _, owner := range owners {
		err = errors.Join(err, owner.Wait(ctx))
	}
	return err
}
