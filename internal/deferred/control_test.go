package deferred

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"
)

func TestControlLanesOwnerConcurrencyAndReservedDeletes(t *testing.T) {
	options := smallOptions()
	options.MaxControlInflight = 6
	s, _ := testStore(t, options)
	owner := testOwner("peer")
	var polls []*Control
	for range 4 {
		control, err := s.AcquireControl(owner, false)
		if err != nil {
			t.Fatal(err)
		}
		polls = append(polls, control)
	}
	if _, err := s.AcquireControl(owner, false); err == nil {
		t.Fatal("owner poll concurrency exceeded four")
	}
	if _, err := s.AcquireControl(testOwner("other"), false); err == nil {
		t.Fatal("polls consumed deletion-reserved slots")
	}
	first, err := s.AcquireControl(owner, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.AcquireControl(owner, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AcquireControl(testOwner("other"), true); err == nil {
		t.Fatal("delete concurrency exceeded two")
	}
	first.Release()
	second.Release()
	for _, control := range polls {
		control.Release()
	}
	if s.Stats().Controls != 0 || s.Stats().Deletes != 0 {
		t.Fatal("control lane releases leaked")
	}
}

func TestControlRateBucketsDoNotResetWhenSlotsRelease(t *testing.T) {
	s, clock := testStore(t, smallOptions())
	owner := testOwner("peer")
	for range 40 {
		control, err := s.AcquireControl(owner, false)
		if err != nil {
			t.Fatal(err)
		}
		control.Release()
	}
	_, err := s.AcquireControl(owner, false)
	assertProtocol(t, err, 429, "deferred_control_limited")
	clock.advance(50 * time.Millisecond)
	control, err := s.AcquireControl(owner, false)
	if err != nil {
		t.Fatal("owner bucket did not refill at 20/s")
	}
	control.Release()
	for range 4 {
		control, err := s.AcquireControl(owner, true)
		if err != nil {
			t.Fatal("delete rate lane was charged for polls")
		}
		control.Release()
	}
	_, err = s.AcquireControl(owner, true)
	assertProtocol(t, err, 429, "deferred_control_limited")
	clock.advance(500 * time.Millisecond)
	control, err = s.AcquireControl(owner, true)
	if err != nil {
		t.Fatal("owner delete bucket did not refill at 2/s")
	}
	control.Release()
}

func TestGlobalControlAndDeleteRates(t *testing.T) {
	s, clock := testStore(t, smallOptions())
	for index := range 200 {
		owner := testOwner(fmt.Sprintf("owner-%d", index/20))
		control, err := s.AcquireControl(owner, false)
		if err != nil {
			t.Fatalf("global burst rejected call %d: %v", index, err)
		}
		control.Release()
	}
	_, err := s.AcquireControl(testOwner("fresh"), false)
	assertProtocol(t, err, 429, "deferred_control_limited")
	clock.advance(10 * time.Millisecond)
	control, err := s.AcquireControl(testOwner("fresh"), false)
	if err != nil {
		t.Fatal("global control bucket did not refill at 100/s")
	}
	control.Release()
	for index := range 16 {
		control, err := s.AcquireControl(testOwner(fmt.Sprintf("owner-%d", index/2)), true)
		if err != nil {
			t.Fatalf("global delete burst rejected call %d: %v", index, err)
		}
		control.Release()
	}
	_, err = s.AcquireControl(testOwner("fresh"), true)
	assertProtocol(t, err, 429, "deferred_control_limited")
	clock.advance(125 * time.Millisecond)
	control, err = s.AcquireControl(testOwner("fresh"), true)
	if err != nil {
		t.Fatal("global delete bucket did not refill at 8/s")
	}
	control.Release()
}

func TestOwnerTableBoundedAndOnlyIdleJobFreeOwnersReclaimed(t *testing.T) {
	options := smallOptions()
	options.MaxJobs, options.MaxRunning, options.MaxEntries = 1, 1, 4
	options.PerOwnerJobs, options.PerOwnerRunning, options.ExecutionTimeout = 1, 1, time.Hour
	s, clock := testStore(t, options)
	owner := testOwner("active")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	_, err := p.Commit(func(ctx context.Context, _ *Writer) Outcome {
		<-ctx.Done()
		return failureOutcome(OutcomeCancelled, "deferred_cancelled", "cancelled")
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	for index := range 3 {
		control, err := s.AcquireControl(testOwner(fmt.Sprintf("idle-%d", index)), false)
		if err != nil {
			t.Fatal(err)
		}
		control.Release()
	}
	if _, err := s.AcquireControl(testOwner("fresh"), false); err == nil {
		t.Fatal("owner table evicted a live rate bucket to admit a new identity")
	}
	clock.advance(59 * time.Second)
	if _, err := s.AcquireControl(testOwner("fresh"), false); err == nil {
		t.Fatal("idle owner reclaimed before one minute")
	}
	clock.advance(time.Second)
	s.Sweep()
	control, err := s.AcquireControl(testOwner("fresh"), false)
	if err != nil {
		t.Fatal(err)
	}
	control.Release()
	if stats := s.Stats(); stats.Owners != 2 || stats.Jobs != 1 {
		t.Fatalf("idle sweep reclaimed an active owner or retained idle owners: %+v", stats)
	}
}

type blockingWriter struct {
	entered chan struct{}
	gate    chan struct{}
	once    sync.Once
}

func (w *blockingWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.entered) })
	<-w.gate
	return len(data), nil
}

func TestControlScratchRemainsReservedThroughBlockedEncoding(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	control, err := s.AcquireControl(testOwner("peer"), false)
	if err != nil {
		t.Fatal(err)
	}
	writer := &blockingWriter{entered: make(chan struct{}), gate: make(chan struct{})}
	done := make(chan error, 1)
	go func() {
		done <- control.Encode(writer, protocolError(503, "deferred_saturated", "busy", AcceptanceNotAccepted).Wire)
	}()
	<-writer.entered
	control.Release()
	if s.Stats().Controls != 1 {
		t.Fatal("release freed scratch while the HTTP body still owned it")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = s.Close(ctx)
	cancel()
	if !errors.Is(err, ErrCleanupIncomplete) {
		t.Fatal("blocked control write was not reported")
	}
	close(writer.gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil || s.Stats().ReservedBytes != 0 {
		t.Fatal("encoding scratch was not released after the writer finished")
	}
}

func TestIngressReservedBeforeReadingAndHeldUntilBlockedReadReturns(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	ingress, err := s.AcquireIngress(testOwner("peer"), -1)
	if err != nil {
		t.Fatal(err)
	}
	reader, writer := io.Pipe()
	defer reader.Close()
	done := make(chan error, 1)
	go func() {
		_, err := ingress.ReadSubmit(reader)
		done <- err
	}()
	await(t, func() bool {
		s.mu.Lock()
		defer s.mu.Unlock()
		return ingress.ioRefs == 1
	})
	ingress.Release()
	if stats := s.Stats(); stats.Admissions != 1 || stats.IngressExtraBytes == 0 || stats.Controls != 1 {
		t.Fatalf("blocked submit buffer was falsely released: %+v", stats)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	err = s.Close(ctx)
	cancel()
	if !errors.Is(err, ErrCleanupIncomplete) {
		t.Fatal("blocked submit reader was not reported")
	}
	_ = writer.Close()
	<-done
	ctx, cancel = context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := s.Close(ctx); err != nil || s.Stats().ReservedBytes != 0 {
		t.Fatal("submit scratch did not release after the reader returned")
	}
}

func TestOneSnapshotPerControlAndOneNonDeleteControlPerJob(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	receipt, err := p.Commit(responseWork([]byte("{}")), nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	await(t, func() bool { return s.Stats().FinalizationCredits == 0 })
	first, _ := s.AcquireControl(owner, false)
	model, err := s.Model(first, receipt.Generation, receipt.ID)
	if err != nil || model != "local/chat" {
		t.Fatal("model authorization lookup did not bind admitted model")
	}
	if _, err := s.Read(first, receipt.Generation, receipt.ID, 0, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Read(first, receipt.Generation, receipt.ID, 1, 1); err == nil {
		t.Fatal("one control allocated multiple retained frames")
	}
	second, _ := s.AcquireControl(owner, false)
	_, err = s.Status(second, receipt.Generation, receipt.ID)
	assertProtocol(t, err, 429, "deferred_control_limited")
	second.Release()
	if err := deleteJob(t, s, owner, receipt); err != nil {
		t.Fatal("non-delete job lock blocked reserved delete lane")
	}
	first.Release()
}

func TestInvalidOwnersCrossOwnerAndCursorOverflow(t *testing.T) {
	s, _ := testStore(t, smallOptions())
	for _, owner := range []Owner{{}, {Kind: "static"}, {Kind: "dashboard", ID: "admin"}, {Kind: "key", ID: "key", Tenant: "tenant", Workspace: "workspace"}} {
		if _, err := s.AcquireControl(owner, false); err == nil {
			t.Fatal("invalid/anonymous/admin identity acquired deferred controls")
		}
	}
	owner := testOwner("peer")
	i, p := prepareRequest(t, s, owner, testRequest(t, testCapability(t, s, owner)))
	receipt, err := p.Commit(responseWork([]byte("{}")), nil)
	if err != nil {
		t.Fatal(err)
	}
	i.Release()
	control, _ := s.AcquireControl(testOwner("other"), false)
	_, err = s.Status(control, receipt.Generation, receipt.ID)
	assertProtocol(t, err, 404, "job_unavailable")
	control.Release()
	control, _ = s.AcquireControl(testOwner("other"), true)
	err = s.Delete(control, receipt.Generation, receipt.ID)
	assertProtocol(t, err, 404, "job_unavailable")
	control.Release()
	control, _ = s.AcquireControl(owner, false)
	_, err = s.Read(control, receipt.Generation, receipt.ID, ^uint64(0), MaxReadBytes)
	assertProtocol(t, err, 400, "invalid_cursor")
	control.Release()
}

func TestControlCapacityUpdateRejectsLiveResponseScratch(t *testing.T) {
	options := smallOptions()
	s, _ := testStore(t, options)
	var controls []*Control
	for index := range 3 {
		control, err := s.AcquireControl(testOwner(fmt.Sprintf("owner-%d", index)), false)
		if err != nil {
			t.Fatal(err)
		}
		controls = append(controls, control)
	}
	defer func() {
		for _, control := range controls {
			control.Release()
		}
	}()
	reduced := options
	reduced.MaxControlInflight = 4
	if err := s.Reconfigure(reduced); err == nil || s.Options() != options {
		t.Fatal("control reduction discarded active response slots")
	}
}
