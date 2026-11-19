package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/store"
)

type deferredSinkCall struct {
	deadline time.Time
	initial  error
}

func deferredSinkContext(ctx context.Context) deferredSinkCall {
	deadline, _ := ctx.Deadline()
	return deferredSinkCall{deadline: deadline, initial: ctx.Err()}
}

type deferredHealthBackend struct {
	mu      sync.Mutex
	calls   []deferredSinkCall
	closes  atomic.Int32
	persist func(context.Context) error
}

func (b *deferredHealthBackend) record(ctx context.Context) error {
	b.mu.Lock()
	b.calls = append(b.calls, deferredSinkContext(ctx))
	b.mu.Unlock()
	if b.persist != nil {
		return b.persist(ctx)
	}
	return ctx.Err()
}

func (b *deferredHealthBackend) MarkSuccess(ctx context.Context, _ string) error {
	return b.record(ctx)
}

func (b *deferredHealthBackend) MarkFailure(ctx context.Context, _ string, _ time.Duration) error {
	return b.record(ctx)
}

func (*deferredHealthBackend) IsHealthy(ctx context.Context, _ string) (bool, error) {
	return true, ctx.Err()
}

func (*deferredHealthBackend) Snapshot(ctx context.Context, names []string) (map[string]bool, error) {
	out := make(map[string]bool, len(names))
	for _, name := range names {
		out[name] = true
	}
	return out, ctx.Err()
}

func (b *deferredHealthBackend) Close() error {
	b.closes.Add(1)
	return nil
}

func (b *deferredHealthBackend) snapshot() []deferredSinkCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]deferredSinkCall(nil), b.calls...)
}

type deferredContextPayload struct {
	mu       sync.Mutex
	calls    []deferredSinkCall
	entries  []payloadlog.Entry
	ordinary atomic.Int32
	record   func(context.Context) error
}

func (*deferredContextPayload) MaxBodyBytes() int { return 64 }

func (p *deferredContextPayload) Record(payloadlog.Entry) error {
	p.ordinary.Add(1)
	return errors.New("context-free remote payload call")
}

func (p *deferredContextPayload) RecordContext(ctx context.Context, entry payloadlog.Entry) error {
	p.mu.Lock()
	p.calls = append(p.calls, deferredSinkContext(ctx))
	p.entries = append(p.entries, entry)
	p.mu.Unlock()
	if p.record != nil {
		return p.record(ctx)
	}
	return ctx.Err()
}

func (p *deferredContextPayload) snapshot() []deferredSinkCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]deferredSinkCall(nil), p.calls...)
}

func TestDeferredRemoteFinalizationSharesTerminalDeadlineAndPublishesLocalUsageOnce(t *testing.T) {
	for _, blocked := range []string{"health", "quota", "payload"} {
		t.Run(blocked, func(t *testing.T) {
			var calls atomic.Int32
			f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
				calls.Add(1)
				return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`),
					Usage: provider.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}}, nil
			}))
			client, quota := dynamicDeferredClient(), &deferredQuotaStore{}
			configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, time.Now)
			entered := make(chan deferredSinkCall, 1)
			stall := func(ctx context.Context) error {
				entered <- deferredSinkContext(ctx)
				<-ctx.Done()
				return ctx.Err()
			}
			health, payload := &deferredHealthBackend{}, &deferredContextPayload{}
			switch blocked {
			case "health":
				health.persist = stall
			case "quota":
				quota.insert = func(ctx context.Context, _ *store.SpendEntry) error { return stall(ctx) }
			case "payload":
				payload.record = stall
			}
			deps := f.h.current()
			_ = deps.Health.Close()
			deps.Health = providerhealth.NewWithBackend(f.metrics, config.ProviderHealth{}, health)
			deps.PayloadLog = payload
			f.h.UpdateDependencies(deps)
			request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`))
			receipt := f.submit(t, request)
			var observed deferredSinkCall
			select {
			case observed = <-entered:
			case <-time.After(time.Second):
				t.Fatal("remote sink was not invoked by a bounded completion worker")
			}
			status := f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
			var info deferred.JobInfo
			if status.Code != 200 || deferred.DecodeControl(status.Body.Bytes(), &info) != nil || info.State != deferred.StateCompleted {
				t.Fatalf("remote persistence withheld terminal publication: %d %s", status.Code, status.Body.String())
			}
			fixedDeadline := time.UnixMilli(info.RetainUntilMS).Add(-f.store.Options().Retention + f.store.Options().FinalizationTimeout)
			if observed.initial != nil || observed.deadline.IsZero() ||
				observed.deadline.Before(fixedDeadline.Add(-10*time.Millisecond)) || observed.deadline.After(fixedDeadline.Add(10*time.Millisecond)) {
				t.Fatalf("remote completion reused execution/submit context or renewed its timeout: observed=%+v terminal=%v", observed, fixedDeadline)
			}
			events := f.accounting.Events()
			workspaceID, _ := client.Workspace.ID()
			tokens, _ := deps.Quota.windowTokens(workspaceID, quotaScope{typ: quotaScopeUser, id: client.OwnerUserID}, "p/m")
			if len(events) != 1 || events[0].TotalTokens != 5 || tokens != 5 || f.store.Stats().WorkBytes == 0 || f.store.Stats().FinalizationCredits != 1 {
				t.Fatal("remote wait delayed/duplicated local accounting or freed completion ownership")
			}
			f.submit(t, request)
			f.read(t, receipt, 0, deferred.MaxReadBytes)
			manifest := f.finish(t, receipt)
			if manifest.Outcome.Usage.TotalTokens != 5 || len(f.accounting.Events()) != 1 || calls.Load() != 1 ||
				f.rate.calls.Load() != 1 || f.store.Stats().FinalizationFailures != 1 || f.store.Stats().WorkBytes != 0 ||
				f.store.Stats().InputBytes != 0 || payload.ordinary.Load() != 0 {
				t.Fatal("remote timeout duplicated inference/accounting or leaked execution ownership")
			}
			expectedQuota, expectedPayload := int32(1), 1
			if blocked == "health" {
				expectedQuota, expectedPayload = 0, 0
			} else if blocked == "quota" {
				expectedPayload = 0
			}
			if quota.inserts.Load() != expectedQuota || len(payload.snapshot()) != expectedPayload || len(health.snapshot()) != 1 {
				t.Fatal("expired remote finalization invoked later sinks or retried failed persistence")
			}
			for _, call := range append(health.snapshot(), payload.snapshot()...) {
				if call.initial != nil || !call.deadline.Equal(observed.deadline) {
					t.Fatal("remote sinks did not receive the same remaining completion deadline")
				}
			}
		})
	}
}

type deferredLocalPayload struct {
	gate    <-chan struct{}
	entered chan payloadlog.Entry
	calls   atomic.Int32
}

func (*deferredLocalPayload) MaxBodyBytes() int { return 64 }

func (p *deferredLocalPayload) Record(entry payloadlog.Entry) error {
	p.calls.Add(1)
	p.entered <- entry
	<-p.gate
	return nil
}

func TestDeferredExecutorTwoWorkersRetainLocalSinkOwnershipAndExpireQueuedRemoteWork(t *testing.T) {
	o := executorTestOptions()
	o.MaxJobs, o.MaxRunning, o.PerOwnerJobs, o.PerOwnerRunning = 3, 3, 3, 3
	var calls, closes atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`),
			Usage: provider.Usage{PromptTokens: 3, CompletionTokens: 4, TotalTokens: 7}}, nil
	}), o)
	client, quota := dynamicDeferredClient(), &deferredQuotaStore{}
	configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, time.Now)
	gate := make(chan struct{})
	releaseSink := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(releaseSink)
	payload := &deferredLocalPayload{gate: gate, entered: make(chan payloadlog.Entry, 3)}
	resources, err := NewRuntimeResources(NewResourceOwner(func() error { closes.Add(1); return nil }, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resources.Retire)
	deps := f.h.current()
	deps.PayloadLog, deps.RuntimeResources = payload, resources
	f.h.UpdateDependencies(deps)
	capability := f.capability(t, "p/m")
	requests, receipts := make([]deferred.SubmitRequest, 0, 3), make([]deferred.Receipt, 0, 3)
	for index := range 3 {
		request := deferredSubmitRequest(t, capability, []byte(`{"model":"p/m"}`))
		receipt := f.submit(t, request)
		requests, receipts = append(requests, request), append(receipts, receipt)
		if index < 2 {
			select {
			case <-payload.entered:
			case <-time.After(time.Second):
				t.Fatal("bounded local completion worker did not enter its sink")
			}
		}
	}
	awaitDeferredHTTP(t, "three published logical completions", func() bool { return f.store.Stats().Running == 0 && len(f.accounting.Events()) == 3 })
	next, _ := NewRuntimeResources()
	t.Cleanup(next.Retire)
	deps = f.h.current()
	deps.RuntimeResources = next
	f.h.UpdateDependencies(deps)
	resources.Retire()
	stats := f.store.Stats()
	workspaceID, _ := client.Workspace.ID()
	tokens, _ := deps.Quota.windowTokens(workspaceID, quotaScope{typ: quotaScopeUser, id: client.OwnerUserID}, "p/m")
	if stats.Finalizing != 2 || stats.FinalizationCredits != 3 || stats.WorkBytes != 3*o.MaxWorkBytes ||
		stats.InputBytes == 0 || closes.Load() != 0 || quota.inserts.Load() != 2 || payload.calls.Load() != 2 || tokens != 21 {
		t.Fatalf("blocked local sinks lost bounded workers, memory, or runtime leases: %+v", stats)
	}
	saturated := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, deferredSubmitRequest(t, capability, []byte(`{"model":"p/m"}`))))
	if saturated.Code != 429 || controlErrorType(t, saturated) != "deferred_owner_limit" || f.rate.calls.Load() != 3 {
		t.Fatal("blocked completion owners did not apply admission backpressure before inference policy")
	}
	status := f.call("GET", deferred.Prefix+"/jobs/"+receipts[2].ID, "peer-token", nil)
	var queued deferred.JobInfo
	if deferred.DecodeControl(status.Body.Bytes(), &queued) != nil {
		t.Fatal(status.Body.String())
	}
	for index, receipt := range receipts {
		start := time.Now()
		deleted := f.call("DELETE", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
		if deleted.Code != 204 || time.Since(start) >= o.ControlTimeout {
			t.Fatal("delete waited for a blocked local sink")
		}
		replay := f.submit(t, requests[index])
		if replay.ID != receipt.ID || !replay.Replayed {
			t.Fatal("release lost accepted submission evidence")
		}
	}
	if f.store.Stats().OutputBytes != 0 || f.store.Stats().Jobs != 3 || f.store.Stats().WorkBytes != 3*o.MaxWorkBytes || closes.Load() != 0 {
		t.Fatal("delete falsely freed ownership still held by local completion workers")
	}
	deadline := time.UnixMilli(queued.RetainUntilMS).Add(-o.Retention + o.FinalizationTimeout + 50*time.Millisecond)
	if wait := time.Until(deadline); wait > 0 {
		timer := time.NewTimer(wait)
		<-timer.C
	}
	releaseSink()
	awaitDeferredHTTP(t, "local workers and expired queue drain", func() bool { return f.store.Stats().FinalizationCredits == 0 && closes.Load() == 1 })
	if quota.inserts.Load() != 2 || payload.calls.Load() != 2 || calls.Load() != 3 || f.rate.calls.Load() != 3 ||
		len(f.accounting.Events()) != 3 || f.store.Stats().Jobs != 0 || f.store.Stats().WorkBytes != 0 ||
		f.store.Stats().InputBytes != 0 || f.store.Stats().FinalizationFailures != 3 {
		t.Fatalf("queued work renewed its deadline, duplicated accounting, or leaked ownership: %+v", f.store.Stats())
	}
}

type deferredCleanupBody struct {
	prefix   *bytes.Reader
	waiting  chan struct{}
	closed   chan struct{}
	cleanup  <-chan struct{}
	entered  chan struct{}
	readOnce sync.Once
	once     sync.Once
	closes   atomic.Int32
}

func (b *deferredCleanupBody) Read(data []byte) (int, error) {
	if b.prefix.Len() > 0 {
		return b.prefix.Read(data)
	}
	b.readOnce.Do(func() { close(b.waiting) })
	<-b.closed
	return 0, io.ErrClosedPipe
}

func (b *deferredCleanupBody) Close() error {
	b.once.Do(func() {
		b.closes.Add(1)
		close(b.closed)
		close(b.entered)
		<-b.cleanup
	})
	return nil
}

type deferredSubmitContextKey struct{}

func TestDeferredConcurrentPollReplayDeleteDetachesSubmitAndKeepsSourceCleanupOwnership(t *testing.T) {
	prefix := []byte("id: upstream\n: preserved\ndata: {\"choices\":[]}\n\n")
	cleanup := make(chan struct{})
	releaseCleanup := sync.OnceFunc(func() { close(cleanup) })
	body := &deferredCleanupBody{
		prefix: bytes.NewReader(prefix), waiting: make(chan struct{}), closed: make(chan struct{}),
		entered: make(chan struct{}), cleanup: cleanup,
	}
	var calls, closes atomic.Int32
	inference := make(chan provider.Request, 1)
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(ctx context.Context, request provider.Request) (*provider.Result, error) {
		calls.Add(1)
		if ctx.Value(deferredSubmitContextKey{}) != nil {
			return nil, fmt.Errorf("retained submit context")
		}
		inference <- request
		stream := provider.NewStreamCompletion()
		stream.SetUsage(provider.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5})
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
			Streaming: true, StreamBody: body, Stream: stream}, nil
	}))
	t.Cleanup(releaseCleanup)
	client, quota := dynamicDeferredClient(), &deferredQuotaStore{}
	configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, time.Now)
	payload := &deferredPayloadRecorder{max: 64}
	resources, _ := NewRuntimeResources(NewResourceOwner(func() error { closes.Add(1); return nil }, nil))
	t.Cleanup(resources.Retire)
	deps := f.h.current()
	deps.RuntimeResources, deps.PayloadLog = resources, payload
	f.h.UpdateDependencies(deps)
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","stream":true}`))
	wire := submitWire(t, request)
	ctx, cancelSubmit := context.WithCancel(context.WithValue(context.Background(), deferredSubmitContextKey{}, "not-owned"))
	defer cancelSubmit()
	submit := f.request("POST", deferred.Prefix+"/submit", "peer-token", wire).WithContext(ctx)
	ack := httptest.NewRecorder()
	f.h.ServeHTTP(ack, submit)
	var receipt deferred.Receipt
	if ack.Code != 202 || deferred.DecodeControl(ack.Body.Bytes(), &receipt) != nil {
		t.Fatalf("submit: %d %s", ack.Code, ack.Body.String())
	}
	cancelSubmit()
	workerRequest := <-inference
	workerDeadline, ok := workerRequest.Inbound.Context().Deadline()
	if !ok || workerRequest.Inbound.Context().Value(deferredSubmitContextKey{}) != nil ||
		workerDeadline.UnixMilli() != receipt.ExecutionDeadlineMS || workerRequest.Inbound.Context().Err() != nil ||
		workerRequest.Inbound == submit || workerRequest.Inbound.URL.Path != "/v1/chat/completions" ||
		workerRequest.Inbound.Header.Get("Authorization") != "" || workerRequest.Inbound.Header.Get("X-Aiproxy-Deferred-Version") != "" {
		t.Fatal("worker retained caller transport/context or lost its fixed execution context")
	}
	select {
	case <-body.waiting:
	case <-time.After(time.Second):
		t.Fatal("detached source did not survive submit cancellation")
	}
	first := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if !bytes.Equal(first.Data, prefix) || first.Done {
		t.Fatal("pending source prefix was not retained exactly")
	}
	next, _ := NewRuntimeResources()
	t.Cleanup(next.Retire)
	deps = f.h.current()
	deps.RuntimeResources = next
	f.h.UpdateDependencies(deps)
	resources.Retire()
	type reply struct {
		method, route string
		response      *httptest.ResponseRecorder
	}
	start, replies := make(chan struct{}), make(chan reply, 18)
	var group sync.WaitGroup
	for index := range 18 {
		method, route, input := "GET", deferred.Prefix+"/jobs/"+receipt.ID, []byte(nil)
		switch {
		case index < 4:
			route += "/read?cursor=0"
		case index < 8:
			route += "/result"
		case index < 12:
		case index < 16:
			method, route, input = "POST", deferred.Prefix+"/submit", wire
		default:
			method = "DELETE"
		}
		group.Add(1)
		go func() {
			defer group.Done()
			<-start
			replies <- reply{method: method, route: route, response: f.call(method, route, "peer-token", input)}
		}()
	}
	close(start)
	group.Wait()
	close(replies)
	for reply := range replies {
		w := reply.response
		assertDeferredHeaders(t, w)
		if reply.method == "DELETE" {
			if w.Code != 204 {
				t.Fatalf("reserved delete was blocked by polling pressure: %d %s", w.Code, w.Body.String())
			}
			continue
		}
		if w.Code == 429 {
			if controlErrorType(t, w) != "deferred_control_limited" {
				t.Fatal("control pressure became inference pressure")
			}
			continue
		}
		if reply.method == "POST" {
			var replay deferred.Receipt
			if w.Code != 202 || deferred.DecodeControl(w.Body.Bytes(), &replay) != nil || replay.ID != receipt.ID || !replay.Replayed {
				t.Fatal("concurrent qualified replay resubmitted or lost accepted work")
			}
		} else if w.Code != 200 && w.Code != 202 && (w.Code != 410 || controlErrorType(t, w) != "job_released") {
			t.Fatalf("poll/delete snapshot race returned invalid outcome: %d %s", w.Code, w.Body.String())
		}
	}
	select {
	case <-body.entered:
	case <-time.After(time.Second):
		t.Fatal("delete did not close the local source")
	}
	awaitDeferredHTTP(t, "known cancelled completion", func() bool { return len(f.accounting.Events()) == 1 })
	stats := f.store.Stats()
	if body.closes.Load() != 1 || closes.Load() != 0 || stats.FinalizationCredits != 1 ||
		stats.WorkBytes == 0 || stats.InputBytes == 0 || stats.OutputBytes != 0 || stats.Jobs != 1 ||
		stats.Cancellations != 1 || calls.Load() != 1 || f.rate.calls.Load() != 1 {
		t.Fatalf("delete duplicated work or falsely freed blocked source cleanup: %+v", stats)
	}
	releaseCleanup()
	awaitDeferredHTTP(t, "source cleanup and runtime release", func() bool { return f.store.Stats().Jobs == 0 && closes.Load() == 1 })
	events := f.accounting.Events()
	workspaceID, _ := client.Workspace.ID()
	tokens, _ := deps.Quota.windowTokens(workspaceID, quotaScope{typ: quotaScopeUser, id: client.OwnerUserID}, "p/m")
	if len(events) != 1 || events[0].StatusCode != 502 || events[0].TotalTokens != 5 || tokens != 5 ||
		quota.inserts.Load() != 1 || quota.reads.Load() != 2 || len(payload.snapshot()) != 1 || body.closes.Load() != 1 || f.store.Stats().WorkBytes != 0 {
		t.Fatal("poll/replay/delete/source finalization duplicated or lost known usage")
	}
}

func TestDeferredDefiniteInferenceTransportFailureStillPersistsModelHealth(t *testing.T) {
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(ctx context.Context, _ provider.Request) (*provider.Result, error) {
		if ctx.Err() != nil {
			t.Error("inference context was cancelled before the transport failure")
		}
		return nil, errors.New("definite upstream transport failure")
	}))
	health := &deferredHealthBackend{}
	deps := f.h.current()
	_ = deps.Health.Close()
	deps.Health = providerhealth.NewWithBackend(f.metrics, config.ProviderHealth{}, health)
	f.h.UpdateDependencies(deps)
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`)))
	manifest := f.finish(t, receipt)
	if manifest.Outcome.Kind != deferred.OutcomeUpstreamError || len(health.snapshot()) != 1 ||
		f.accounting.Events()[0].StatusCode != 502 {
		t.Fatal("executor cutoff handling suppressed a definite inference transport failure")
	}
}

func TestDeferredAcceptedHTTPReplayIgnoresSmallerNewInputLimits(t *testing.T) {
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	capability := f.capability(t, "p/m")
	body := append([]byte(`{"model":"p/m","vendor":"`), bytes.Repeat([]byte("x"), 1500)...)
	body = append(body, []byte(`"}`)...)
	request := deferredSubmitRequest(t, capability, body)
	receipt := f.submit(t, request)
	manifest := f.finish(t, receipt)
	options := f.store.Options()
	options.MaxRequestBytes = 1024
	if err := f.store.Reconfigure(options); err != nil {
		t.Fatal(err)
	}
	replay := f.submit(t, request)
	if replay.ID != receipt.ID || !replay.Replayed || replay.ExecutionDeadlineMS != receipt.ExecutionDeadlineMS ||
		calls.Load() != 1 || f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 1 {
		t.Fatal("smaller new input policy rejected or recharged an accepted qualified replay")
	}
	fresh := deferredSubmitRequest(t, capability, body)
	w := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, fresh))
	if w.Code != 413 || controlErrorType(t, w) != "request_too_large" || calls.Load() != 1 || f.rate.calls.Load() != 1 {
		t.Fatal("new admission ignored the reduced input policy")
	}
	retained := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if !retained.Done || retained.RetainUntilMS != manifest.RetainUntilMS || !bytes.Equal(retained.Data, []byte(`{}`)) {
		t.Fatal("replay/reload changed retained output or renewed terminal retention")
	}
}

type deferredHTTPTestClock struct{ at atomic.Int64 }
type deferredHTTPTestTimer struct{ timer *time.Timer }

func (c *deferredHTTPTestClock) Now() time.Time { return time.UnixMilli(c.at.Load()) }
func (*deferredHTTPTestClock) NewTimer(delay time.Duration) deferred.Timer {
	return deferredHTTPTestTimer{timer: time.NewTimer(delay)}
}
func (t deferredHTTPTestTimer) C() <-chan time.Time { return t.timer.C }
func (t deferredHTTPTestTimer) Stop() bool          { return t.timer.Stop() }

func TestDeferredHTTPTerminalReadsDoNotRenewRetentionAndExpiredScopesCannotDispatch(t *testing.T) {
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	if err := f.store.Close(ctx); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	clock := &deferredHTTPTestClock{}
	clock.at.Store(time.Now().UnixMilli())
	s, err := deferred.New(context.Background(), executorTestOptions(), deferred.Hooks{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	f.store = s
	deps := f.h.current()
	deps.DeferredStore = s
	f.h.UpdateDependencies(deps)
	capability := f.capability(t, "p/m")
	request := deferredSubmitRequest(t, capability, []byte(`{"model":"p/m"}`))
	receipt := f.submit(t, request)
	manifest := f.finish(t, receipt)
	clock.at.Add(9 * time.Second.Milliseconds())
	for range 3 {
		frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
		if !frame.Done || frame.RetainUntilMS != manifest.RetainUntilMS || !bytes.Equal(frame.Data, []byte(`{}`)) {
			t.Fatal("terminal HTTP polling changed output or renewed retention")
		}
	}
	clock.at.Add(time.Second.Milliseconds())
	s.Sweep()
	base := deferred.Prefix + "/jobs/" + receipt.ID
	for _, endpoint := range []struct{ method, path string }{
		{"GET", base}, {"GET", base + "/result"}, {"GET", base + "/read?cursor=0"}, {"DELETE", base},
	} {
		w := f.call(endpoint.method, endpoint.path, "peer-token", nil)
		if w.Code != 410 || controlErrorType(t, w) != "job_expired" || bytes.Contains(w.Body.Bytes(), []byte(receipt.ID)) {
			t.Fatal("expired terminal HTTP result disclosed data or invented a released classification")
		}
	}
	replay := f.submit(t, request)
	if replay.ID != receipt.ID || !replay.Replayed || s.Stats().Jobs != 0 || s.Stats().Expirations != 1 {
		t.Fatal("terminal expiry erased protected acceptance evidence")
	}
	clock.at.Add(21 * time.Second.Milliseconds())
	fresh := deferredSubmitRequest(t, capability, request.Body)
	w := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, fresh))
	if w.Code != 410 || controlErrorType(t, w) != "submit_scope_expired" || calls.Load() != 1 ||
		f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 1 {
		t.Fatal("an absent expired HTTP submission scope dispatched fresh work")
	}
	replay = f.submit(t, request)
	if replay.ID != receipt.ID || !replay.Replayed {
		t.Fatal("ticket expiry prematurely removed accepted generation protection")
	}
}

type deferredAdmissionGate struct {
	started chan struct{}
	gate    <-chan struct{}
	once    sync.Once
	calls   atomic.Int32
}

func (g *deferredAdmissionGate) Allow(identity.CredentialID) (bool, time.Duration) {
	g.calls.Add(1)
	g.once.Do(func() { close(g.started) })
	<-g.gate
	return true, 0
}

func TestDeferredExpiredAdmissionContextCannotCommitAndReplayDoesNotRerunPolicy(t *testing.T) {
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`))
	wire := submitWire(t, request)
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	t.Cleanup(release)
	admission := &deferredAdmissionGate{started: make(chan struct{}), gate: gate}
	deps := f.h.current()
	deps.RateLimiter = admission
	f.h.UpdateDependencies(deps)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	r := f.request("POST", deferred.Prefix+"/submit", "peer-token", wire).WithContext(ctx)
	w, done := httptest.NewRecorder(), make(chan struct{})
	go func() {
		defer close(done)
		f.h.ServeHTTP(w, r)
	}()
	select {
	case <-admission.started:
	case <-time.After(time.Second):
		t.Fatal("request did not enter shared bounded admission")
	}
	<-ctx.Done()
	release()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("expired admission did not release preparation")
	}
	if w.Code != 503 || controlErrorType(t, w) != "deferred_saturated" {
		t.Fatalf("expired admission context committed fresh execution: %d %s", w.Code, w.Body.String())
	}
	replay := f.call("POST", deferred.Prefix+"/submit", "peer-token", wire)
	if replay.Code != w.Code || !bytes.Equal(replay.Body.Bytes(), w.Body.Bytes()) || calls.Load() != 0 ||
		admission.calls.Load() != 1 || len(f.accounting.Events()) != 0 || f.store.Stats().Jobs != 0 ||
		f.store.Stats().Admissions != 0 || f.store.Stats().WorkBytes != 0 || f.store.Stats().FinalizationCredits != 0 {
		t.Fatal("expired preparation replay reran policy, dispatched, or leaked reservations")
	}
}

func TestDeferredAdmissionWorkLimitReturnsExactCachedRejectionWithoutDispatch(t *testing.T) {
	options := executorTestOptions()
	options.MaxRequestBytes, options.MaxWorkBytes, options.ControlTimeout = 1<<20, 4<<20, time.Second
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}), options)
	body := append([]byte(`{"model":"p/m","vendor":"`), bytes.Repeat([]byte("x"), 512<<10)...)
	body = append(body, []byte(`"}`)...)
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), body)
	wire := submitWire(t, request)
	first := f.call("POST", deferred.Prefix+"/submit", "peer-token", wire)
	if first.Code != 400 {
		t.Fatalf("admission did not reserve decode work before dispatch: %d %s", first.Code, first.Body.String())
	}
	var initial deferred.ControlError
	if json.Unmarshal(first.Body.Bytes(), &initial) != nil || initial.Error.Type != "deferred_limit_exceeded" {
		t.Fatal("admission work ceiling produced an unrelated policy rejection")
	}
	replay := f.call("POST", deferred.Prefix+"/submit", "peer-token", wire)
	if replay.Code != first.Code || !bytes.Equal(replay.Body.Bytes(), first.Body.Bytes()) {
		t.Fatalf("work-budget rejection changed during dedup replay: first=%s replay=%s", first.Body.String(), replay.Body.String())
	}
	if calls.Load() != 0 || f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 0 || f.store.Stats().Jobs != 0 ||
		f.store.Stats().Admissions != 0 || f.store.Stats().InputBytes != 0 || f.store.Stats().WorkBytes != 0 ||
		f.store.Stats().FinalizationCredits != 0 {
		t.Fatal("work-limit rejection replay dispatched, reran policy, or freed live reservations incorrectly")
	}
}

func TestDeferredAliasCompletionMetadataIsReservedBeforeScheduling(t *testing.T) {
	options := executorTestOptions()
	options.MaxWorkBytes, options.ControlTimeout = 4<<20, time.Second
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}), options)
	target := f.rt.Catalog.Providers()[0]
	target.Name = string(bytes.Repeat([]byte("p"), 512<<10))
	f.rt.Catalog = config.NewCatalog([]config.Provider{target}, nil, []config.Alias{{
		Name: "a", Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: target.Name, Model: "m"}},
	}})
	deps := f.h.current()
	deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
	f.h.UpdateDependencies(deps)
	request := deferredSubmitRequest(t, f.capability(t, "alias/a"), []byte(`{"model":"alias/a"}`))
	wire := submitWire(t, request)
	first := f.call("POST", deferred.Prefix+"/submit", "peer-token", wire)
	if first.Code != 400 || controlErrorType(t, first) != "deferred_limit_exceeded" {
		t.Fatal("alias target/completion copies escaped the admitted work reservation")
	}
	replay := f.call("POST", deferred.Prefix+"/submit", "peer-token", wire)
	if replay.Code != first.Code || !bytes.Equal(replay.Body.Bytes(), first.Body.Bytes()) || calls.Load() != 0 ||
		f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 0 || f.store.Stats().Jobs != 0 || f.store.Stats().WorkBytes != 0 {
		t.Fatal("metadata-limit rejection replay reran policy, dispatched, or leaked reserved ownership")
	}
}

func TestDeferredCancellationAndExecutionDeadlineDoNotPersistModelHealthFailure(t *testing.T) {
	for _, mode := range []string{"delete", "deadline"} {
		t.Run(mode, func(t *testing.T) {
			o := executorTestOptions()
			o.ExecutionTimeout = time.Second
			entered := make(chan struct{})
			f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(ctx context.Context, _ provider.Request) (*provider.Result, error) {
				close(entered)
				<-ctx.Done()
				return nil, fmt.Errorf("upstream cancelled: %w", ctx.Err())
			}), o)
			health := &deferredHealthBackend{}
			deps := f.h.current()
			_ = deps.Health.Close()
			deps.Health = providerhealth.NewWithBackend(f.metrics, config.ProviderHealth{}, health)
			f.h.UpdateDependencies(deps)
			receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`)))
			<-entered
			if mode == "delete" {
				w := f.call("DELETE", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
				if w.Code != 204 {
					t.Fatal(w.Body.String())
				}
				awaitDeferredHTTP(t, "cancelled source finalization", func() bool { return f.store.Stats().FinalizationCredits == 0 })
			} else {
				manifest := f.finish(t, receipt)
				if manifest.Outcome.Kind != deferred.OutcomeExecutionTimeout || manifest.State != deferred.StateFailed || f.accounting.Events()[0].StatusCode != 504 {
					t.Fatalf("execution deadline was not a retained executor timeout: %+v", manifest.Outcome)
				}
			}
			if len(health.snapshot()) != 0 || f.store.Stats().FinalizationFailures != 0 || len(f.accounting.Events()) != 1 {
				t.Fatal("executor cancellation/deadline mutated model health or duplicated completion")
			}
		})
	}
}
