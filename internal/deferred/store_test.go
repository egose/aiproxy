package deferred

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type testClock struct {
	mu  sync.Mutex
	now time.Time
}

type testTimer struct{ c chan time.Time }

func (t testTimer) C() <-chan time.Time { return t.c }
func (testTimer) Stop() bool            { return true }

func (c *testClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *testClock) NewTimer(time.Duration) Timer { return testTimer{make(chan time.Time)} }

func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

func smallOptions() Options {
	o := DefaultOptions()
	o.Enabled = true
	o.MaxJobs, o.MaxRunning, o.MaxAdmissions, o.MaxEntries = 4, 4, 4, 16
	o.MaxRequestBytes, o.MaxResultBytes, o.MaxWorkBytes, o.MaxMemoryBytes = 1024, 1024, 4<<20, 32<<20
	o.PerOwnerJobs, o.PerOwnerRunning = 4, 4
	o.ExecutionTimeout, o.Retention = 10*time.Second, 10*time.Second
	o.ControlTimeout, o.FinalizationTimeout = 100*time.Millisecond, 100*time.Millisecond
	return o
}

func testStore(t *testing.T, options Options) (*Store, *testClock) {
	t.Helper()
	clock := &testClock{now: time.Now().Truncate(time.Millisecond)}
	s, err := New(context.Background(), options, Hooks{Clock: clock})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Errorf("store close: %v; stats=%+v", err, s.Stats())
		}
	})
	return s, clock
}

func testOwner(name string) Owner { return Owner{Kind: "static", ID: name} }

func testCapability(t *testing.T, s *Store, owner Owner) Capability {
	t.Helper()
	control, err := s.AcquireControl(owner, false)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Release()
	capability, err := s.Capability(control, "local/chat", ReceiverLimits{})
	if err != nil {
		t.Fatal(err)
	}
	return capability
}

func testRequest(t *testing.T, capability Capability) SubmitRequest {
	t.Helper()
	key, err := NewAttemptKey()
	if err != nil {
		t.Fatal(err)
	}
	return SubmitRequest{
		Envelope: NewEnvelope(), Generation: capability.Generation, SubmitToken: capability.SubmitToken,
		AttemptKey: key, Headers: http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}},
		Body: []byte(`{"model":"local/chat","messages":[]}`),
	}
}

func beginRequest(t *testing.T, s *Store, owner Owner, request SubmitRequest) (*Ingress, *Preparation, *Receipt, error) {
	t.Helper()
	wire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	ingress, err := s.AcquireIngress(owner, int64(len(wire)))
	if err != nil {
		return nil, nil, nil, err
	}
	decoded, err := ingress.ReadSubmit(bytes.NewReader(wire))
	if err != nil {
		ingress.Release()
		return nil, nil, nil, err
	}
	preparation, receipt, err := s.Begin(ingress, "local/chat", decoded)
	return ingress, preparation, receipt, err
}

func prepareRequest(t *testing.T, s *Store, owner Owner, request SubmitRequest) (*Ingress, *Preparation) {
	t.Helper()
	ingress, preparation, replay, err := beginRequest(t, s, owner, request)
	if err != nil {
		if ingress != nil {
			ingress.Release()
		}
		t.Fatal(err)
	}
	if replay != nil || preparation == nil {
		t.Fatal("expected a new preparation")
	}
	return ingress, preparation
}

func assertProtocol(t *testing.T, err error, status int, code string) {
	t.Helper()
	var failure *ProtocolError
	if !errors.As(err, &failure) || failure.Status != status || failure.Wire.Error.Type != code {
		t.Fatalf("error=%v, want %d %s", err, status, code)
	}
	if failure.Wire.Envelope != NewEnvelope() {
		t.Fatal("protocol error omitted recognition envelope")
	}
}

func await(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-deadline.C:
			t.Fatal("timed out waiting for deferred lifecycle")
		case <-ticker.C:
		}
	}
}

func responseWork(body []byte) Work {
	return func(_ context.Context, writer *Writer) Outcome {
		if err := writer.SetResponse(ResponseHead{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}}); err != nil {
			return failureOutcome(OutcomeUpstreamError, "upstream_error", err.Error())
		}
		if _, err := writer.Write(body); err != nil {
			return failureOutcome(OutcomeLimitExceeded, "deferred_limit_exceeded", err.Error())
		}
		return Outcome{Kind: OutcomeResponse}
	}
}

func statusJob(t *testing.T, s *Store, owner Owner, receipt Receipt) (JobInfo, error) {
	t.Helper()
	control, err := s.AcquireControl(owner, false)
	if err != nil {
		return JobInfo{}, err
	}
	defer control.Release()
	return s.Status(control, receipt.Generation, receipt.ID)
}

func deleteJob(t *testing.T, s *Store, owner Owner, receipt Receipt) error {
	t.Helper()
	control, err := s.AcquireControl(owner, true)
	if err != nil {
		return err
	}
	defer control.Release()
	return s.Delete(control, receipt.Generation, receipt.ID)
}

func TestCapabilityCompactSignedScopeAndRandomIdentities(t *testing.T) {
	s, clock := testStore(t, smallOptions())
	owner := Owner{Kind: "static", ID: strings.Repeat("i", 256), Tenant: strings.Repeat("t", 256)}
	control, err := s.AcquireControl(owner, false)
	if err != nil {
		t.Fatal(err)
	}
	defer control.Release()
	model := strings.Repeat("m", 512)
	capability, err := s.Capability(control, model, ReceiverLimits{ExecutionTimeout: time.Second, MaxResultBytes: 1024})
	if err != nil {
		t.Fatal(err)
	}
	if len(capability.SubmitToken) > MaxSubmitTokenBytes || len(capability.SubmitToken) != 252 {
		t.Fatalf("oversized/noncompact ticket: %d", len(capability.SubmitToken))
	}
	if capability.SubmitUntilMS != clock.Now().Add(SubmitWindow).UnixMilli() || capability.Limits.ExecutionTimeoutMS != 1000 {
		t.Fatal("scope expiry or narrowed policy differs")
	}
	if ValidateID(capability.Generation) != nil {
		t.Fatal("generation is not a canonical random identifier")
	}
	scope, err := s.verifyTicket(owner, model, capability.SubmitToken)
	if err != nil || scope.limits != capability.Limits || scope.untilMS != capability.SubmitUntilMS {
		t.Fatalf("signed limits did not round trip: %+v %v", scope, err)
	}
	for _, forged := range []struct {
		owner Owner
		model string
		token string
	}{
		{testOwner("other"), model, capability.SubmitToken},
		{owner, "another/model", capability.SubmitToken},
		{owner, model, "B" + capability.SubmitToken[1:]},
		{owner, model, strings.Repeat("A", 512)},
		{owner, model, capability.SubmitToken + "="},
	} {
		if _, err := s.verifyTicket(forged.owner, forged.model, forged.token); err == nil {
			t.Fatal("forged submission scope was accepted")
		}
	}
	other, _ := testStore(t, smallOptions())
	if s.Generation() == other.Generation() || s.secret == other.secret {
		t.Fatal("restart reused process identity or signing secret")
	}
}

func TestConcurrentDedupPreparingReplayConflictAndProtectedRelease(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	owner := testOwner("peer")
	request := testRequest(t, testCapability(t, s, owner))
	ingress, preparation := prepareRequest(t, s, owner, request)
	var wg sync.WaitGroup
	var preparing atomic.Int32
	for range 3 {
		wg.Go(func() {
			i, p, r, err := beginRequest(t, s, owner, request)
			if i != nil {
				defer i.Release()
			}
			var failure *ProtocolError
			if errors.As(err, &failure) && failure.Status == 409 && failure.Wire.Error.Type == "submission_in_progress" && p == nil && r == nil {
				preparing.Add(1)
			} else {
				t.Errorf("duplicate preparation: %v", err)
			}
		})
	}
	wg.Wait()
	if preparing.Load() != 3 {
		t.Fatal("preparing dedup did not serialize admission")
	}
	entered, finish := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	receipt, err := preparation.Commit(func(ctx context.Context, w *Writer) Outcome {
		calls.Add(1)
		close(entered)
		select {
		case <-finish:
			return responseWork([]byte("{}"))(ctx, w)
		case <-ctx.Done():
			return failureOutcome(OutcomeCancelled, "deferred_cancelled", "cancelled")
		}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ingress.Release()
	<-entered
	var replays atomic.Int32
	for range 3 {
		wg.Go(func() {
			i, p, r, err := beginRequest(t, s, owner, request)
			if i != nil {
				defer i.Release()
			}
			if err == nil && p == nil && r != nil && r.Replayed && r.ID == receipt.ID {
				replays.Add(1)
			} else {
				t.Errorf("accepted replay: %v", err)
			}
		})
	}
	wg.Wait()
	if replays.Load() != 3 || calls.Load() != 1 {
		t.Fatal("accepted replay dispatched more inference")
	}
	conflict := request
	conflict.Body = append([]byte(nil), request.Body...)
	conflict.Body = append(conflict.Body, ' ')
	i, _, _, err := beginRequest(t, s, owner, conflict)
	assertProtocol(t, err, 409, "attempt_conflict")
	i.Release()
	if err := deleteJob(t, s, owner, receipt); err != nil {
		t.Fatal(err)
	}
	close(finish)
	await(t, func() bool { return s.Stats().Jobs == 0 })
	if err := deleteJob(t, s, owner, receipt); err != nil {
		t.Fatalf("idempotent delete: %v", err)
	}
	_, err = statusJob(t, s, owner, receipt)
	assertProtocol(t, err, 410, "job_released")
	i, p, replay, err := beginRequest(t, s, owner, request)
	if err != nil || p != nil || replay == nil || replay.ID != receipt.ID || !replay.Replayed {
		t.Fatalf("release erased dedup protection: %v", err)
	}
	i.Release()
	if s.Stats().Entries != 1 || calls.Load() != 1 {
		t.Fatal("protected tombstone was evicted or redispatched")
	}
}

func TestCachedAdmissionRejectionAndAbsentExpiredScope(t *testing.T) {
	s, clock := testStore(t, smallOptions())
	owner := testOwner("peer")
	capability := testCapability(t, s, owner)
	request := testRequest(t, capability)
	i, p := prepareRequest(t, s, owner, request)
	rejection := NewProtocolError(403, ErrorDetail{Type: "forbidden", Message: "denied", BlockID: "block-id"}, AcceptanceNotAccepted)
	if err := p.Reject(rejection); err != nil {
		t.Fatal(err)
	}
	i.Release()
	i, p, replay, err := beginRequest(t, s, owner, request)
	assertProtocol(t, err, 403, "forbidden")
	if p != nil || replay != nil {
		t.Fatal("rejection replay entered preparation")
	}
	var failure *ProtocolError
	_ = errors.As(err, &failure)
	if failure.Wire.Error.BlockID != "block-id" {
		t.Fatal("cached ordinary rejection lost block identity")
	}
	i.Release()
	clock.advance(SubmitWindow)
	s.Sweep()
	i, _, _, err = beginRequest(t, s, owner, request)
	assertProtocol(t, err, 410, "submit_scope_expired")
	i.Release()
	if stats := s.Stats(); stats.Jobs != 0 || stats.Entries != 0 || stats.Running != 0 {
		t.Fatalf("expired rejected scope retained execution ownership: %+v", stats)
	}
}

func TestAcceptedReplayAfterScopeExpiryAndRestartFailsClosed(t *testing.T) {
	s, clock := testStore(t, smallOptions())
	owner := testOwner("peer")
	request := testRequest(t, testCapability(t, s, owner))
	i, p := prepareRequest(t, s, owner, request)
	receipt, err := p.Commit(responseWork([]byte("{}")), nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
	if receipt.DedupUntilMS != clock.Now().Add(SubmitWindow+10*time.Second+10*time.Second+100*time.Millisecond).UnixMilli() {
		t.Fatal("dedup horizon is not fixed from the signed submission window")
	}
	clock.advance(SubmitWindow + time.Millisecond)
	s.Sweep()
	i, p, replay, err := beginRequest(t, s, owner, request)
	if err != nil || p != nil || replay == nil || replay.ID != receipt.ID {
		t.Fatalf("expired ticket did not replay accepted scope: %v", err)
	}
	i.Release()
	other, _ := testStore(t, smallOptions())
	i, _, _, err = beginRequest(t, other, owner, request)
	assertProtocol(t, err, 410, "generation_mismatch")
	i.Release()
	control, err := other.AcquireControl(owner, false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = other.Status(control, receipt.Generation, receipt.ID)
	assertProtocol(t, err, 410, "generation_mismatch")
	control.Release()
}

func TestAdmissionReservesRunningOwnerMemoryAndFinalizationBeforeCommit(t *testing.T) {
	options := smallOptions()
	options.MaxJobs, options.MaxRunning, options.PerOwnerJobs, options.PerOwnerRunning = 2, 1, 1, 1
	s, _ := testStore(t, options)
	owner := testOwner("first")
	capability := testCapability(t, s, owner)
	i, p := prepareRequest(t, s, owner, testRequest(t, capability))
	stats := s.Stats()
	if stats.Jobs != 1 || stats.Running != 1 || stats.FinalizationCredits != 1 ||
		stats.OutputBytes != ResultSlabBytes || stats.WorkBytes != options.MaxWorkBytes || stats.InputBytes == 0 ||
		stats.ReservedBytes > options.MaxMemoryBytes {
		t.Fatalf("preparation did not reserve full ownership: %+v", stats)
	}
	j, _, _, err := beginRequest(t, s, owner, testRequest(t, capability))
	assertProtocol(t, err, 429, "deferred_owner_limit")
	j.Release()
	second := testOwner("second")
	request := testRequest(t, testCapability(t, s, second))
	j, _, _, err = beginRequest(t, s, second, request)
	assertProtocol(t, err, 503, "deferred_saturated")
	j.Release()
	p.Abort()
	i.Release()
	if stats := s.Stats(); stats.Jobs != 0 || stats.Running != 0 || stats.WorkBytes != 0 || stats.OutputBytes != 0 {
		t.Fatalf("aborted reservation leaked: %+v", stats)
	}
	j, _, _, err = beginRequest(t, s, second, request)
	assertProtocol(t, err, 503, "deferred_saturated")
	j.Release()
}

func TestOutputSlabsFiniteReadsHashAndDeletedReaderOwnership(t *testing.T) {
	options := smallOptions()
	options.MaxResultBytes = 3*ResultSlabBytes + 17
	s, _ := testStore(t, options)
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	body := bytes.Repeat([]byte{0xff, 0, '\n'}, 30000)
	receipt, err := p.Commit(responseWork(body), nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
	if s.Stats().OutputBytes != 4*ResultSlabBytes {
		t.Fatal("result capacity was not slab rounded")
	}
	control, _ := s.AcquireControl(owner, false)
	manifest, terminal, err := s.Result(control, receipt.Generation, receipt.ID)
	control.Release()
	sum := sha256.Sum256(body)
	if err != nil || !terminal || manifest.BodySHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("terminal body hash differs: %+v %v", manifest, err)
	}
	control, _ = s.AcquireControl(owner, false)
	frame, err := s.Read(control, receipt.Generation, receipt.ID, 0, 0)
	if err != nil || frame.NextCursor != MaxReadBytes || frame.Done || cap(frame.Data) != MaxReadBytes || !bytes.Equal(frame.Data, body[:MaxReadBytes]) {
		t.Fatalf("finite first read: %+v %v", frame.JobInfo, err)
	}
	if err := ValidateReadFrame(frame); err != nil {
		t.Fatal(err)
	}
	if err := deleteJob(t, s, owner, receipt); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(frame.Data, body[:MaxReadBytes]) {
		t.Fatal("delete corrupted an admitted reader snapshot")
	}
	stats := s.Stats()
	if stats.OutputBytes != 0 || stats.Controls != 1 || stats.ReservedBytes < ControlScratchBytes {
		t.Fatalf("deleted read scratch was not retained/accounted: %+v", stats)
	}
	var wire bytes.Buffer
	if err := control.Encode(&wire, frame); err != nil {
		t.Fatal(err)
	}
	var decoded ReadFrame
	if err := DecodeControl(wire.Bytes(), &decoded); err != nil || !bytes.Equal(decoded.Data, frame.Data) {
		t.Fatalf("binary finite frame did not round trip: %v", err)
	}
	control.Release()
}

func TestGrowthAndWorkFailureSealCancelWithoutTruncatedSuccess(t *testing.T) {
	for _, growth := range []string{"result", "work", "metadata"} {
		t.Run(growth, func(t *testing.T) {
			s, _ := testStore(t, smallOptions())
			owner := testOwner("peer")
			i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
			closed := make(chan struct{})
			var finalizations atomic.Int32
			receipt, err := p.Commit(func(ctx context.Context, w *Writer) Outcome {
				_ = w.SetSource(closerFunc(func() error { close(closed); return nil }))
				_ = w.SetFinalizer(func(context.Context) error { finalizations.Add(1); return nil })
				head := ResponseHead{StatusCode: 200, Headers: http.Header{"Content-Type": {"application/json"}}}
				if growth == "metadata" {
					head.Headers["X-Large"] = []string{strings.Repeat("x", 4097)}
				}
				_ = w.SetResponse(head)
				if growth == "result" {
					_, _ = w.Write([]byte("prefix"))
					if _, err := w.Write(make([]byte, 1024)); err == nil {
						t.Error("oversized append succeeded")
					}
				}
				if growth == "work" {
					if _, err := w.WorkBudget().Reserve(w.WorkBudget().Limit() + 1); !errors.Is(err, ErrBudgetExceeded) {
						t.Errorf("work budget failure: %v", err)
					}
				}
				<-ctx.Done()
				return Outcome{Kind: OutcomeResponse}
			}, nil)
			if err != nil {
				t.Fatal(err)
			}
			i.Release()
			<-closed
			await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
			info, err := statusJob(t, s, owner, receipt)
			if err != nil || info.State != StateFailed || info.Outcome.Kind != OutcomeLimitExceeded || finalizations.Load() != 1 {
				t.Fatalf("growth became a success or duplicated finalization: %+v %v", info, err)
			}
		})
	}
}

type closerFunc func() error

func (f closerFunc) Close() error { return f() }

func TestDeadlineRetentionAbandonedJobAndPollsNeverRenew(t *testing.T) {
	s, clock := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	entered := make(chan struct{})
	receipt, err := p.Commit(func(ctx context.Context, w *Writer) Outcome {
		close(entered)
		<-ctx.Done()
		return failureOutcome(OutcomeCancelled, "deferred_cancelled", "cancelled")
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	<-entered
	control, _ := s.AcquireControl(owner, false)
	frame, err := s.Read(control, receipt.Generation, receipt.ID, 0, 1)
	control.Release()
	if err != nil || frame.Done || frame.Data == nil || len(frame.Data) != 0 || frame.State != StateRunning {
		t.Fatal("empty active read claimed completion")
	}
	clock.advance(10 * time.Second)
	s.Sweep()
	await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
	info, err := statusJob(t, s, owner, receipt)
	if err != nil || info.State != StateFailed || info.Outcome.Kind != OutcomeExecutionTimeout {
		t.Fatalf("fixed deadline was not sealed: %+v %v", info, err)
	}
	until := info.RetainUntilMS
	clock.advance(9 * time.Second)
	info, err = statusJob(t, s, owner, receipt)
	if err != nil || info.RetainUntilMS != until {
		t.Fatal("poll renewed retention")
	}
	clock.advance(time.Second)
	s.Sweep()
	_, err = statusJob(t, s, owner, receipt)
	assertProtocol(t, err, 410, "job_expired")
	if s.Stats().Jobs != 0 || s.Stats().WorkBytes != 0 {
		t.Fatal("abandoned job ownership was not reclaimed")
	}
	clock.advance(time.Minute)
	s.Sweep()
	_, err = statusJob(t, s, owner, receipt)
	assertProtocol(t, err, 404, "job_unavailable")
}

func TestAbandonedPreparingScopeAndBorrowedInputRemainAccounted(t *testing.T) {
	s, clock := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	input, release, err := p.Input()
	if err != nil {
		t.Fatal(err)
	}
	clock.advance(SubmitWindow)
	s.Sweep()
	if !bytes.Equal(input.Body, []byte(`{"model":"local/chat","messages":[]}`)) || s.Stats().InputBytes == 0 || s.Stats().Jobs != 1 {
		t.Fatal("scope expiry freed borrowed preparation memory")
	}
	if _, err := p.Commit(responseWork([]byte("{}")), nil); err == nil {
		t.Fatal("expired preparing scope dispatched")
	}
	release()
	i.Release()
	if s.Stats().Jobs != 0 || s.Stats().InputBytes != 0 {
		t.Fatal("released input borrower retained actual reservation")
	}
}

func TestFinalizationWorkersCreditsQueueDeadlineAndBackpressure(t *testing.T) {
	options := smallOptions()
	options.MaxJobs, options.MaxRunning, options.PerOwnerJobs, options.PerOwnerRunning = 3, 3, 3, 3
	s, clock := testStore(t, options)
	owner := testOwner("peer")
	capability := testCapability(t, s, owner)
	gate := make(chan struct{})
	entered := make(chan int, 3)
	expired := make(chan bool, 1)
	var releases, finalizations atomic.Int32
	receipts := make([]Receipt, 0, 3)
	for index := 0; index < 3; index++ {
		i, p := prepareRequest(t, s, owner, testRequest(t, capability))
		index := index
		receipt, err := p.Commit(func(ctx context.Context, w *Writer) Outcome {
			_ = w.SetFinalizer(func(ctx context.Context) error {
				finalizations.Add(1)
				entered <- index
				if index < 2 {
					<-gate
				} else {
					expired <- errors.Is(ctx.Err(), context.DeadlineExceeded)
				}
				return nil
			})
			return responseWork([]byte("{}"))(ctx, w)
		}, func() { releases.Add(1) })
		if err != nil {
			t.Fatal(err)
		}
		i.Release()
		receipts = append(receipts, receipt)
		if index < 2 {
			if got := <-entered; got != index {
				t.Fatalf("unexpected worker order: %d", got)
			}
		}
	}
	await(t, func() bool { return s.Stats().Running == 0 })
	if stats := s.Stats(); stats.Finalizing != 2 || stats.FinalizationCredits != 3 || stats.Jobs != 3 || stats.WorkBytes != 3*options.MaxWorkBytes {
		t.Fatalf("unbounded or unreserved completion ownership: %+v", stats)
	}
	i, _, _, err := beginRequest(t, s, owner, testRequest(t, capability))
	assertProtocol(t, err, 429, "deferred_owner_limit")
	i.Release()
	for _, receipt := range receipts[:2] {
		if err := deleteJob(t, s, owner, receipt); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if err := s.Close(ctx); !errors.Is(err, ErrCleanupIncomplete) {
		t.Fatalf("blocked finalizers were falsely freed: %v", err)
	}
	cancel()
	if releases.Load() != 0 || s.Stats().WorkBytes == 0 {
		t.Fatal("shutdown released live callback reservations")
	}
	clock.advance(time.Second)
	close(gate)
	if !<-expired {
		t.Fatal("queued callback received a renewed finalization timeout")
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil {
		t.Fatal(err)
	}
	if finalizations.Load() != 3 || releases.Load() != 3 || s.Stats().ReservedBytes != 0 || s.Stats().FinalizationFailures == 0 {
		t.Fatal("completion credits, leases, or failure reporting differ")
	}
}

func TestOutstandingWorkReservationSurvivesFinalizerAndDelete(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	releaseWork, err := p.WorkBudget().Reserve(4096)
	if err != nil {
		t.Fatal(err)
	}
	var releasedRuntime atomic.Bool
	receipt, err := p.Commit(responseWork([]byte("{}")), func() { releasedRuntime.Store(true) })
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool {
		p.budget.mu.Lock()
		defer p.budget.mu.Unlock()
		return p.budget.closed
	})
	if err := deleteJob(t, s, owner, receipt); err != nil {
		t.Fatal(err)
	}
	if s.Stats().Jobs != 1 || s.Stats().WorkBytes == 0 || releasedRuntime.Load() || s.Stats().FinalizationCredits != 1 {
		t.Fatal("outstanding work ownership was freed with the entry")
	}
	releaseWork()
	releaseWork()
	await(t, func() bool { return s.Stats().Jobs == 0 })
	if s.Stats().WorkBytes != 0 || !releasedRuntime.Load() || s.Stats().FinalizationCredits != 0 {
		t.Fatal("work reservation release was not idempotent")
	}
	if _, err := p.WorkBudget().Reserve(1); !errors.Is(err, ErrBudgetClosed) {
		t.Fatal("finalized budget permitted new allocations")
	}
}

func TestBorrowedInputIsBoundedAndKeepsRuntimeUntilFinalization(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	input, releaseInput, err := p.Input()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := p.Input(); err == nil {
		t.Fatal("preparation allowed unbounded input borrowers")
	}
	var released atomic.Bool
	receipt, err := p.Commit(responseWork([]byte("{}")), func() { released.Store(true) })
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool {
		p.budget.mu.Lock()
		defer p.budget.mu.Unlock()
		return p.budget.closed
	})
	if released.Load() || s.Stats().InputBytes == 0 || len(input.Body) == 0 {
		t.Fatal("finalization freed borrowed input or its runtime")
	}
	if err := deleteJob(t, s, owner, receipt); err != nil {
		t.Fatal(err)
	}
	releaseInput()
	releaseInput()
	await(t, func() bool { return s.Stats().Jobs == 0 })
	if !released.Load() || s.Stats().InputBytes != 0 {
		t.Fatal("last input borrower did not finish resource cleanup")
	}
}

func TestSourceAttachedAfterSealingIsClosedWithoutWaitingForWorkReturn(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	closed, attach := make(chan struct{}), make(chan struct{})
	var closes atomic.Int32
	_, err := p.Commit(func(ctx context.Context, w *Writer) Outcome {
		w.Complete(failureOutcome(OutcomeUpstreamError, "upstream_error", "sealed"))
		<-attach
		if err := w.SetSource(closerFunc(func() error {
			closes.Add(1)
			close(closed)
			return nil
		})); err != nil {
			t.Error(err)
		}
		<-closed
		return Outcome{Kind: OutcomeResponse}
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool { return s.Stats().Finalizing == 1 })
	close(attach)
	await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
	if closes.Load() != 1 {
		t.Fatal("late source cleanup did not run exactly once")
	}
}

func TestShutdownClosesSourcesBeforeWaitAndReportsBlockedLocalClose(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	entered, closeEntered, unblock := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var released atomic.Bool
	_, err := p.Commit(func(ctx context.Context, w *Writer) Outcome {
		_ = w.SetSource(closerFunc(func() error { close(closeEntered); <-unblock; return nil }))
		close(entered)
		<-ctx.Done()
		return failureOutcome(OutcomeCancelled, "deferred_cancelled", "cancelled")
	}, func() { released.Store(true) })
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	<-entered
	s.BeginShutdown()
	<-closeEntered
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = s.Close(ctx)
	cancel()
	if !errors.Is(err, ErrCleanupIncomplete) || released.Load() || s.Stats().WorkBytes == 0 || s.Stats().Jobs != 1 {
		t.Fatalf("blocked source close lost live ownership: %v %+v", err, s.Stats())
	}
	close(unblock)
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil || !released.Load() || s.Stats().ReservedBytes != 0 {
		t.Fatalf("source release did not finish cleanup: %v", err)
	}
}

func TestReconfigureTransactionFailureDrainAndAcceptedReplayWithSmallerInput(t *testing.T) {
	options := smallOptions()
	options.MaxRequestBytes = 2048
	s, _ := testStore(t, options)
	owner := testOwner("peer")
	request := testRequest(t, testCapability(t, s, owner))
	request.Body = bytes.Repeat([]byte(" "), 1500)
	i, p := prepareRequest(t, s, owner, request)
	failed := options
	failed.MaxJobs, failed.MaxRunning, failed.PerOwnerJobs, failed.PerOwnerRunning = 1, 1, 1, 1
	failed.MaxMemoryBytes = 16 << 20
	activationErr := errors.New("dependency activation failed")
	update, err := s.PrepareUpdate(failed)
	if err != nil {
		t.Fatal(err)
	}
	if err := update.Commit(func() error { return activationErr }); !errors.Is(err, activationErr) {
		t.Fatalf("activation failure did not propagate: %v", err)
	}
	if s.Options() != options {
		t.Fatal("failed activation changed store policy")
	}
	receipt, err := p.Commit(responseWork([]byte("{}")), nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
	smaller := options
	smaller.MaxRequestBytes = 1024
	if err := s.Reconfigure(smaller); err != nil {
		t.Fatal(err)
	}
	i, p, replay, err := beginRequest(t, s, owner, request)
	if err != nil || p != nil || replay == nil || replay.ID != receipt.ID {
		t.Fatalf("smaller policy misclassified accepted replay: %v", err)
	}
	i.Release()
	if err := s.Reconfigure(Options{}); err != nil {
		t.Fatal(err)
	}
	wantDrain := smaller
	wantDrain.Enabled = false
	if s.Options() != wantDrain {
		t.Fatal("root removal discarded draining resource policy")
	}
	i, _, replay, err = beginRequest(t, s, owner, request)
	if err != nil || replay == nil {
		t.Fatalf("disablement rejected protected replay: %v", err)
	}
	i.Release()
	control, _ := s.AcquireControl(owner, false)
	_, err = s.Capability(control, "local/chat", ReceiverLimits{})
	assertProtocol(t, err, 404, "deferred_disabled")
	control.Release()
}

func TestCapacityReductionIncludesLiveJobsOwnersReadersAndIngressScratch(t *testing.T) {
	options := smallOptions()
	options.MaxMemoryBytes = 64 << 20
	s, _ := testStore(t, options)
	owner := testOwner("peer")
	capability := testCapability(t, s, owner)
	var ingresses []*Ingress
	for range 2 {
		i, _ := prepareRequest(t, s, owner, testRequest(t, capability))
		ingresses = append(ingresses, i)
	}
	defer func() {
		for _, i := range ingresses {
			i.Release()
		}
	}()
	for _, changed := range []Options{
		func() Options {
			o := options
			o.MaxJobs, o.MaxRunning, o.PerOwnerJobs, o.PerOwnerRunning = 1, 1, 1, 1
			return o
		}(),
		func() Options { o := options; o.PerOwnerJobs = 1; return o }(),
		func() Options { o := options; o.PerOwnerRunning = 1; return o }(),
		func() Options { o := options; o.MaxAdmissions = 1; return o }(),
		func() Options { o := options; o.MaxMemoryBytes = 16 << 20; return o }(),
	} {
		if err := s.Reconfigure(changed); err == nil || !reflect.DeepEqual(s.Options(), options) {
			t.Fatalf("capacity reduction changed live policy: %+v %v", changed, err)
		}
	}
	before := s.Stats()
	hard, err := s.AcquireIngress(testOwner("other"), -1)
	if err != nil {
		t.Fatal(err)
	}
	if s.Stats().IngressExtraBytes == 0 || s.Stats().ReservedBytes <= before.ReservedBytes {
		t.Fatal("unknown-length base64 ingress did not reserve its full hard ceiling")
	}
	reduced := options
	reduced.MaxMemoryBytes = 24 << 20
	if err := s.Reconfigure(reduced); err == nil {
		t.Fatal("memory update ignored live ingress scratch")
	}
	hard.Release()
}

func TestFingerprintLengthPrefixesExactBodyAndCanonicalHeaders(t *testing.T) {
	a, err := Fingerprint("local/chat", []byte(`{"x":1}`), http.Header{"Content-Type": {"application/json"}, "X-Test": {"a", "bc"}})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Fingerprint("local/chat", []byte(`{"x":1}`), http.Header{"content-type": {"application/json"}, "x-test": {"a", "bc"}})
	if err != nil || a != b {
		t.Fatal("header case changed fingerprint")
	}
	for _, pair := range []struct {
		body    []byte
		headers http.Header
	}{
		{[]byte(`{"x":1} `), http.Header{"Content-Type": {"application/json"}, "X-Test": {"a", "bc"}}},
		{[]byte(`{"x":1}`), http.Header{"Content-Type": {"application/json"}, "X-Test": {"ab", "c"}}},
		{[]byte(`{"x":1}`), http.Header{"Content-Type": {"application/json"}, "X-Test": {"bc", "a"}}},
	} {
		hash, err := Fingerprint("local/chat", pair.body, pair.headers)
		if err != nil || hash == a {
			t.Fatal("fingerprint omitted exact bytes, field lengths, or value order")
		}
	}
	if len(a) != 64 || a != strings.ToLower(a) {
		t.Fatal("fingerprint is not lowercase SHA-256")
	}
}

func TestOptionsFixedAllowancesMatchConfigurationMinimum(t *testing.T) {
	for _, options := range []Options{DefaultOptions(), smallOptions()} {
		fixed, err := options.FixedMemoryBytes()
		if err != nil {
			t.Fatal(err)
		}
		output, _ := RoundedResultBytes(options.MaxResultBytes)
		minimum := fixed + options.MaxRequestBytes + output + options.MaxWorkBytes
		options.MaxMemoryBytes = max(16<<20, minimum)
		if err := options.Validate(); err != nil {
			t.Fatalf("store/config fixed reservations disagree: %v", err)
		}
		if minimum >= 16<<20 {
			options.MaxMemoryBytes--
			if options.Validate() == nil {
				t.Fatal("config accepted less than store fixed reservation minimum")
			}
		}
	}
	if _, err := RoundedResultBytes(int64(^uint64(0) >> 1)); err == nil {
		t.Fatal("slab capacity arithmetic overflowed")
	}
	if _, err := New(context.Background(), smallOptions(), Hooks{Random: bytes.NewReader(nil)}); err == nil {
		t.Fatal("random generation failure was ignored")
	}
}
