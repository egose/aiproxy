package httpapi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/provider"
)

func TestRuntimeResourcesShareOwnersAcrossGenerationsUntilLastLease(t *testing.T) {
	var health, payload atomic.Int32
	healthOwner := NewResourceOwner(func() error { health.Add(1); return nil }, nil)
	payloadOwner := NewResourceOwner(func() error { payload.Add(1); return nil }, nil)
	first, err := NewRuntimeResources(healthOwner, payloadOwner)
	if err != nil {
		t.Fatal(err)
	}
	release, ok := first.Acquire()
	if !ok {
		t.Fatal("initial runtime could not be acquired")
	}
	second, err := NewRuntimeResources(healthOwner, payloadOwner)
	if err != nil {
		t.Fatal(err)
	}
	first.Retire()
	second.Retire()
	if health.Load() != 0 || payload.Load() != 0 {
		t.Fatal("shared resource closed while old-generation lease remained")
	}
	if _, ok := first.Acquire(); ok {
		t.Fatal("retired runtime admitted another lease")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if err := first.Wait(ctx); !errors.Is(err, ErrResourcesIncomplete) {
		t.Fatal("live runtime lease was falsely reported closed")
	}
	cancel()
	release()
	release()
	if health.Load() != 1 || payload.Load() != 1 {
		t.Fatal("shared ownership did not close exactly once")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := healthOwner.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := NewRuntimeResources(healthOwner); err == nil {
		t.Fatal("closed shared resource was resurrected")
	}
}

func TestRuntimeStopAdmissionKeepsGenerationOwnerUntilRetire(t *testing.T) {
	var closes atomic.Int32
	owner := NewResourceOwner(func() error { closes.Add(1); return nil }, nil)
	resources, err := NewRuntimeResources(owner)
	if err != nil {
		t.Fatal(err)
	}
	resources.StopAdmission()
	if _, ok := resources.Acquire(); ok || closes.Load() != 0 {
		t.Fatal("shutdown admission freeze dropped a resource still owned by probes")
	}
	resources.Retire()
	if closes.Load() != 1 {
		t.Fatal("frozen generation could not retire")
	}
}

func TestDependencySnapshotAcquiresLeaseUnderHandlerLock(t *testing.T) {
	var closes atomic.Int32
	old, _ := NewRuntimeResources(NewResourceOwner(func() error { closes.Add(1); return nil }, nil))
	next, _ := NewRuntimeResources()
	h := NewHandler(Dependencies{RuntimeResources: old})
	old.mu.Lock()
	snapshot := make(chan func(), 1)
	go func() {
		deps, release, ok := h.SnapshotDependenciesWithLease()
		if !ok || deps.RuntimeResources != old {
			t.Error("snapshot lost the pre-activation generation")
		}
		snapshot <- release
	}()
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()
	for {
		if !h.mu.TryLock() {
			break
		}
		h.mu.Unlock()
		select {
		case <-deadline.C:
			old.mu.Unlock()
			t.Fatal("snapshot did not acquire handler dependency lock")
		default:
			time.Sleep(time.Millisecond)
		}
	}
	activated := make(chan error, 1)
	go func() {
		activated <- h.UpdateDependenciesWithActivation(Dependencies{RuntimeResources: next}, nil)
	}()
	select {
	case err := <-activated:
		old.mu.Unlock()
		t.Fatalf("dependency activation raced ahead of lease acquisition: %v", err)
	case <-time.After(10 * time.Millisecond):
	}
	old.mu.Unlock()
	release := <-snapshot
	if err := <-activated; err != nil {
		t.Fatal(err)
	}
	old.Retire()
	if closes.Load() != 0 {
		t.Fatal("retirement closed an acquired old snapshot")
	}
	release()
	if closes.Load() != 1 {
		t.Fatal("last snapshot did not release its resource")
	}
	next.Retire()
}

func TestFailedDependencyActivationPreservesCurrentResources(t *testing.T) {
	old, _ := NewRuntimeResources()
	next, _ := NewRuntimeResources()
	h := NewHandler(Dependencies{RuntimeResources: old})
	failure := errors.New("activation failed")
	if err := h.UpdateDependenciesWithActivation(Dependencies{RuntimeResources: next}, func() error { return failure }); !errors.Is(err, failure) {
		t.Fatal("activation failure was lost")
	}
	deps, release, ok := h.SnapshotDependenciesWithLease()
	if !ok || deps.RuntimeResources != old {
		t.Fatal("failed activation changed dependency snapshot")
	}
	release()
	old.Retire()
	next.Retire()
}

type resourceTestRecorder struct {
	closed     *atomic.Bool
	records    atomic.Int32
	afterClose atomic.Bool
}

func (r *resourceTestRecorder) Record(payloadlog.Entry) error {
	if r.closed.Load() {
		r.afterClose.Store(true)
	}
	r.records.Add(1)
	return nil
}

func (*resourceTestRecorder) MaxBodyBytes() int { return 1024 }

type resourceTestAccounting struct {
	closed     *atomic.Bool
	records    atomic.Int32
	afterClose atomic.Bool
}

func (r *resourceTestAccounting) Record(accounting.Event) {
	if r.closed.Load() {
		r.afterClose.Store(true)
	}
	r.records.Add(1)
}

type resourceStreamAdapter struct {
	body *resourceStreamBody
}

func (a resourceStreamAdapter) Do(context.Context, provider.Request) (*provider.Result, error) {
	return &provider.Result{
		StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
		Streaming: true, StreamBody: a.body,
	}, nil
}

type resourceStreamBody struct {
	entered chan struct{}
	gate    chan struct{}
	closed  atomic.Bool
	once    sync.Once
}

func (b *resourceStreamBody) Read([]byte) (int, error) {
	b.once.Do(func() { close(b.entered) })
	<-b.gate
	return 0, io.EOF
}

func (b *resourceStreamBody) Close() error {
	b.closed.Store(true)
	return nil
}

func TestOrdinaryHandlerLeaseSurvivesBodyCloseAccountingAndPayloadCompletion(t *testing.T) {
	var closed atomic.Bool
	var bodyClosedAtRetirement atomic.Bool
	body := &resourceStreamBody{entered: make(chan struct{}), gate: make(chan struct{})}
	payload := &resourceTestRecorder{closed: &closed}
	accountingRecorder := &resourceTestAccounting{closed: &closed}
	owner := NewResourceOwner(func() error {
		bodyClosedAtRetirement.Store(body.closed.Load())
		closed.Store(true)
		return nil
	}, nil)
	resources, _ := NewRuntimeResources(owner)
	rt := newRT()
	deps := Dependencies{
		Resolver: modelresolver.New(rt), Catalog: rt.Catalog, Adapter: resourceStreamAdapter{body},
		Auth: auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}), RuntimeResources: resources,
		PayloadLog: payload, Accounting: accountingRecorder, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	h := NewHandler(deps)
	done := make(chan struct{})
	go func() {
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"openai/gpt-4o-mini","stream":true}`)))
		close(done)
	}()
	<-body.entered
	next, _ := NewRuntimeResources()
	deps.RuntimeResources = next
	h.UpdateDependencies(deps)
	resources.Retire()
	if closed.Load() {
		t.Fatal("ordinary stream lost completion resources on reload")
	}
	close(body.gate)
	<-done
	if !closed.Load() || !bodyClosedAtRetirement.Load() || payload.records.Load() != 1 || accountingRecorder.records.Load() != 1 ||
		payload.afterClose.Load() || accountingRecorder.afterClose.Load() {
		t.Fatal("runtime lease ended before source close and once-only completion")
	}
	next.Retire()
}
