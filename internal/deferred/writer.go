package deferred

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"
)

var ErrOutputSealed = fmt.Errorf("deferred output is sealed")

type Work func(context.Context, *Writer) Outcome
type Finalizer func(context.Context) error

type job struct {
	store                  *Store
	record                 *record
	limits                 Limits
	headers                http.Header
	body                   []byte
	inputBytes             int64
	outputBytes            int64
	workBytes              int64
	inputReaders           int
	budget                 *WorkBudget
	budgetDrained          bool
	runningHeld            bool
	creditHeld             bool
	released               bool
	state                  State
	head                   *ResponseHead
	outcome                *Outcome
	slabs                  []*[ResultSlabBytes]byte
	tail                   uint64
	hasher                 hash.Hash
	bodyHash               string
	executionDeadlineMS    int64
	retainUntilMS          int64
	finalizationDeadlineMS int64
	ctx                    context.Context
	cancel                 context.CancelFunc
	source                 *ownedSource
	sourceDone             bool
	sourceDoneCh           chan struct{}
	ownershipDoneCh        chan struct{}
	ownershipDone          bool
	finalizer              Finalizer
	runtimeRelease         func()
	finalized              bool
	finishQueued           bool
	finishNext             *job
}

type ownedSource struct {
	closer io.Closer
	once   sync.Once
	err    error
}

func (s *ownedSource) close() error {
	if s == nil {
		return nil
	}
	s.once.Do(func() { s.err = callCompletion(func() error { return s.closer.Close() }) })
	return s.err
}

type Writer struct{ job *job }

func newJob(s *Store, rec *record, headers http.Header, body []byte, limits Limits, input, output, workBytes int64) *job {
	j := &job{
		store: s, record: rec, limits: limits, headers: headers, body: make([]byte, len(body)),
		inputBytes: input, outputBytes: output, workBytes: workBytes, runningHeld: true, creditHeld: true,
		slabs: make([]*[ResultSlabBytes]byte, 0, int(output/ResultSlabBytes)), hasher: sha256.New(),
		sourceDoneCh: make(chan struct{}), ownershipDoneCh: make(chan struct{}),
	}
	copy(j.body, body)
	j.budget = newWorkBudget(workBytes, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if rec.preparing {
			s.rejectLocked(rec, protocolError(400, "deferred_limit_exceeded", "deferred preparation work allowance exceeded", AcceptanceNotAccepted))
		} else if rec.accepted {
			s.sealLocked(j, failureOutcome(OutcomeLimitExceeded, "deferred_limit_exceeded", "deferred work allowance exceeded"))
		}
	}, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		j.budgetDrained = true
		s.finishOwnershipLocked(j)
		s.releaseJobLocked(j)
		s.finishShutdownLocked()
	})
	return j
}

func (p *Preparation) Commit(work Work, releaseRuntime func()) (Receipt, error) {
	s := p.store
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := p.record
	if !rec.preparing || rec.job == nil {
		if rec.rejection != nil {
			return Receipt{}, cloneProtocolError(rec.rejection)
		}
		return Receipt{}, protocolError(409, "submission_in_progress", "deferred preparation already resolved", AcceptanceNotAccepted)
	}
	reject := func(failure *ProtocolError) (Receipt, error) {
		s.rejectLocked(rec, failure)
		return Receipt{}, failure
	}
	if work == nil {
		return reject(protocolError(400, "invalid_request", "deferred execution work is required", AcceptanceNotAccepted))
	}
	now := s.clock.Now().UnixMilli()
	if now >= rec.submitUntilMS {
		return reject(protocolError(410, "submit_scope_expired", "deferred submission scope expired", AcceptanceNotAccepted))
	}
	if !s.options.Enabled || s.shuttingDown {
		return reject(protocolError(404, "deferred_disabled", "deferred admission is disabled", AcceptanceNotAccepted))
	}
	j := rec.job
	if j.inputBytes > s.options.MaxRequestBytes || j.workBytes > s.options.MaxWorkBytes {
		return reject(protocolError(503, "deferred_saturated", "deferred admission policy changed during preparation", AcceptanceNotAccepted))
	}
	j.limits.RequestBytes = min(j.limits.RequestBytes, s.options.MaxRequestBytes)
	j.limits.ResultBytes = min(j.limits.ResultBytes, s.options.MaxResultBytes)
	j.limits.ExecutionTimeoutMS = min(j.limits.ExecutionTimeoutMS, s.options.ExecutionTimeout.Milliseconds())
	j.limits.RetentionMS = min(j.limits.RetentionMS, s.options.Retention.Milliseconds())
	j.limits.ControlTimeoutMS = min(j.limits.ControlTimeoutMS, s.options.ControlTimeout.Milliseconds())
	j.limits.FinalizationTimeoutMS = min(j.limits.FinalizationTimeoutMS, s.options.FinalizationTimeout.Milliseconds())
	output, _ := RoundedResultBytes(j.limits.ResultBytes)
	s.stats.OutputBytes -= j.outputBytes - output
	j.outputBytes = output
	deadline, err := addMilliseconds(now, j.limits.ExecutionTimeoutMS)
	if err != nil {
		return reject(protocolError(400, "invalid_request", "invalid deferred execution deadline", AcceptanceNotAccepted))
	}
	j.executionDeadlineMS = deadline
	j.ctx, j.cancel = context.WithDeadline(s.ctx, time.Now().Add(time.Duration(j.limits.ExecutionTimeoutMS)*time.Millisecond))
	j.runtimeRelease = releaseRuntime
	j.state = StateRunning
	rec.preparing, rec.accepted = false, true
	rec.protectedMS = rec.receipt.DedupUntilMS
	rec.receipt.ExecutionDeadlineMS = deadline
	s.byID[rec.receipt.ID] = rec
	go s.run(j, work)
	return rec.receipt, nil
}

func (j *job) inputLocked() Input {
	return Input{PublicModel: j.record.receipt.PublicModel, Owner: j.record.owner, Headers: j.headers, Body: j.body, Limits: j.limits}
}

func (w *Writer) Input() Input {
	s := w.job.store
	s.mu.Lock()
	defer s.mu.Unlock()
	return w.job.inputLocked()
}

func (w *Writer) WorkBudget() *WorkBudget { return w.job.budget }

func (w *Writer) Receipt() Receipt {
	s := w.job.store
	s.mu.Lock()
	defer s.mu.Unlock()
	return w.job.record.receipt
}

func (w *Writer) SetFinalizer(finalizer Finalizer) error {
	if finalizer == nil {
		return fmt.Errorf("deferred finalizer must not be nil")
	}
	j, s := w.job, w.job.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.finalized || j.sourceDone || j.finalizer != nil {
		return fmt.Errorf("deferred finalizer already set or execution finished")
	}
	j.finalizer = finalizer
	return nil
}

func (w *Writer) SetSource(source io.Closer) error {
	if source == nil {
		return fmt.Errorf("deferred source must not be nil")
	}
	j, s := w.job, w.job.store
	s.mu.Lock()
	if j.source != nil || j.sourceDone || j.finalized {
		s.mu.Unlock()
		return fmt.Errorf("deferred source already set or execution finished")
	}
	j.source = &ownedSource{closer: source}
	sealed, owned := j.state.Terminal() || j.released, j.source
	s.mu.Unlock()
	if sealed {
		return owned.close()
	}
	return nil
}

func (w *Writer) SetResponse(head ResponseHead) error {
	headers, err := FilterResponseHeaders(head.Headers)
	if err != nil || head.StatusCode < 200 || head.StatusCode > 599 {
		w.failLimit("deferred response metadata exceeds limits")
		return fmt.Errorf("invalid deferred response metadata")
	}
	head.Headers = headers
	if head.Retry != nil {
		advice := *head.Retry
		if advice.ObservedAtMS <= 0 || advice.DeadlineMS < advice.ObservedAtMS || advice.RemainingMS < 0 || advice.RemainingMS > advice.DeadlineMS-advice.ObservedAtMS {
			w.failLimit("invalid deferred retry metadata")
			return fmt.Errorf("invalid deferred retry metadata")
		}
		head.Retry = &advice
	}
	j, s := w.job, w.job.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.state.Terminal() || j.released {
		return ErrOutputSealed
	}
	if j.head != nil {
		return fmt.Errorf("deferred response head already published")
	}
	worstHead := head
	if head.Retry != nil {
		advice := *head.Retry
		advice.RemainingMS = math.MaxInt64
		worstHead.Retry = &advice
	}
	worst := JobInfo{
		Receipt: j.record.receipt, Response: &worstHead, AvailableBytes: math.MaxUint64, RetainUntilMS: math.MaxInt64,
		Outcome: &Outcome{
			Kind:  OutcomeExecutionTimeout,
			Usage: Usage{math.MaxInt64, math.MaxInt64, math.MaxInt64, math.MaxInt64, math.MaxInt64, math.MaxInt64},
			Error: &ErrorDetail{Type: "\x00", Message: "\x00", BlockID: strings.Repeat("\x00", MaxErrorBytes-2)},
		},
	}
	worst.State = StateFailed
	if err := metadataLimit(ResultManifest{JobInfo: worst, BodySHA256: strings.Repeat("f", 64)}); err != nil ||
		metadataLimit(ReadFrame{JobInfo: worst, Cursor: math.MaxUint64, NextCursor: math.MaxUint64, Data: []byte{}, Done: true}) != nil {
		s.sealLocked(j, failureOutcome(OutcomeLimitExceeded, "deferred_limit_exceeded", "deferred response metadata exceeds limits"))
		return fmt.Errorf("deferred response metadata exceeds limits")
	}
	j.head, j.state = &head, StateHeadersReady
	return nil
}

func (w *Writer) Write(data []byte) (int, error) {
	j, s := w.job, w.job.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if j.state.Terminal() || j.released {
		return 0, ErrOutputSealed
	}
	if j.head == nil {
		s.sealLocked(j, failureOutcome(OutcomeUpstreamError, "upstream_error", "deferred response head is missing"))
		return 0, fmt.Errorf("deferred response head must precede output")
	}
	if uint64(len(data)) > uint64(j.limits.ResultBytes)-j.tail {
		s.sealLocked(j, failureOutcome(OutcomeLimitExceeded, "deferred_limit_exceeded", "deferred result allowance exceeded"))
		return 0, ErrBudgetExceeded
	}
	for copied := 0; copied < len(data); {
		slab, offset := int(j.tail/ResultSlabBytes), int(j.tail%ResultSlabBytes)
		if slab == len(j.slabs) {
			j.slabs = append(j.slabs, new([ResultSlabBytes]byte))
		}
		n := min(len(data)-copied, ResultSlabBytes-offset)
		copy(j.slabs[slab][offset:offset+n], data[copied:copied+n])
		_, _ = j.hasher.Write(data[copied : copied+n])
		j.tail += uint64(n)
		copied += n
	}
	return len(data), nil
}

func (w *Writer) Complete(outcome Outcome) {
	s := w.job.store
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealLocked(w.job, outcome)
}

func (w *Writer) Finish(outcome Outcome) Outcome {
	s, j := w.job.store, w.job
	s.mu.Lock()
	defer s.mu.Unlock()
	if !j.state.Terminal() {
		if s.shuttingDown || s.ctx.Err() != nil {
			outcome.Kind = OutcomeCancelled
			outcome.Error = &ErrorDetail{Type: "deferred_cancelled", Message: "deferred execution cancelled"}
		} else if s.clock.Now().UnixMilli() >= j.executionDeadlineMS || j.ctx.Err() != nil {
			outcome.Kind = OutcomeExecutionTimeout
			outcome.Error = &ErrorDetail{Type: "deferred_execution_timeout", Message: "deferred execution deadline exceeded"}
		}
		s.sealLocked(j, outcome)
	} else if validateOutcome(outcome, j.head != nil) == nil {
		j.outcome.Usage = outcome.Usage
	}
	final := *j.outcome
	if final.Error != nil {
		detail := *final.Error
		final.Error = &detail
	}
	return final
}

func (w *Writer) failLimit(message string) {
	s := w.job.store
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sealLocked(w.job, failureOutcome(OutcomeLimitExceeded, "deferred_limit_exceeded", message))
}

func failureOutcome(kind OutcomeKind, code, message string) Outcome {
	return Outcome{Kind: kind, Error: &ErrorDetail{Type: code, Message: message}}
}

func (s *Store) run(j *job, work Work) {
	outcome := failureOutcome(OutcomeUpstreamError, "upstream_error", "deferred execution did not complete")
	defer func() {
		if recovered := recover(); recovered != nil {
			outcome = failureOutcome(OutcomeUpstreamError, "upstream_error", "deferred execution failed")
		}
		s.mu.Lock()
		if !j.state.Terminal() {
			if s.shuttingDown || s.ctx.Err() != nil {
				outcome = failureOutcome(OutcomeCancelled, "deferred_cancelled", "deferred execution cancelled")
			} else if s.clock.Now().UnixMilli() >= j.executionDeadlineMS || j.ctx.Err() != nil {
				outcome = failureOutcome(OutcomeExecutionTimeout, "deferred_execution_timeout", "deferred execution deadline exceeded")
			}
			s.sealLocked(j, outcome)
		}
		j.sourceDone = true
		close(j.sourceDoneCh)
		s.releaseRunningLocked(j)
		s.releaseJobLocked(j)
		s.finishShutdownLocked()
		s.mu.Unlock()
	}()
	outcome = work(j.ctx, &Writer{job: j})
}

func (s *Store) sealLocked(j *job, outcome Outcome) {
	if j.state.Terminal() || !j.record.accepted {
		return
	}
	if err := validateOutcome(outcome, j.head != nil); err != nil {
		outcome = failureOutcome(OutcomeLimitExceeded, "deferred_limit_exceeded", "invalid deferred completion metadata")
	}
	copy := outcome
	if outcome.Error != nil {
		detail := *outcome.Error
		detail.Type, detail.Message, detail.BlockID = strings.Clone(detail.Type), strings.Clone(detail.Message), strings.Clone(detail.BlockID)
		copy.Error = &detail
	}
	j.outcome = &copy
	switch outcome.Kind {
	case OutcomeResponse:
		j.state = StateCompleted
	case OutcomeCancelled:
		j.state = StateCancelled
		s.stats.Cancellations++
	default:
		j.state = StateFailed
	}
	now := s.clock.Now().UnixMilli()
	j.retainUntilMS, _ = addMilliseconds(now, j.limits.RetentionMS)
	j.finalizationDeadlineMS, _ = addMilliseconds(now, j.limits.FinalizationTimeoutMS)
	j.bodyHash = hex.EncodeToString(j.hasher.Sum(nil))
	j.hasher = nil
	if j.cancel != nil {
		j.cancel()
	}
	s.queueFinishLocked(j)
}

func (s *Store) queueFinishLocked(j *job) {
	if j.finishQueued {
		return
	}
	j.finishQueued = true
	if s.finishTail == nil {
		s.finishHead = j
	} else {
		s.finishTail.finishNext = j
	}
	s.finishTail = j
	for range FinalizationWorkers {
		select {
		case s.finishWake <- struct{}{}:
		default:
		}
	}
}

func (s *Store) completionWorker() {
	defer s.backgroundDone()
	for {
		s.mu.Lock()
		j := s.finishHead
		if j != nil {
			s.finishHead = j.finishNext
			if s.finishHead == nil {
				s.finishTail = nil
			}
			j.finishNext = nil
			s.stats.Finalizing++
		}
		s.mu.Unlock()
		if j != nil {
			s.finalize(j)
			continue
		}
		select {
		case <-s.stop:
			return
		case <-s.finishWake:
		}
	}
}

func (s *Store) finalize(j *job) {
	s.mu.Lock()
	source := j.source
	s.mu.Unlock()
	closeErr := source.close()
	<-j.sourceDoneCh
	s.mu.Lock()
	source, finalizer, release := j.source, j.finalizer, j.runtimeRelease
	deadline := j.finalizationDeadlineMS
	s.mu.Unlock()
	if err := source.close(); closeErr == nil {
		closeErr = err
	}
	remaining := max(int64(0), deadline-s.clock.Now().UnixMilli())
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(remaining)*time.Millisecond)
	var finalErr error
	if finalizer != nil {
		finalErr = callCompletion(func() error { return finalizer(ctx) })
		if finalErr == nil {
			finalErr = ctx.Err()
		}
	}
	cancel()
	s.mu.Lock()
	j.budgetDrained = j.budget.close()
	s.finishOwnershipLocked(j)
	s.mu.Unlock()
	<-j.ownershipDoneCh
	var releaseErr error
	if release != nil {
		releaseErr = callCompletion(func() error { release(); return nil })
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if closeErr != nil || finalErr != nil || releaseErr != nil {
		s.stats.FinalizationFailures++
	}
	j.finalizer, j.runtimeRelease, j.source, j.ctx, j.cancel = nil, nil, nil, nil, nil
	j.finalized = true
	if j.creditHeld {
		j.creditHeld = false
		s.stats.FinalizationCredits--
	}
	s.stats.Finalizing--
	s.releaseJobLocked(j)
	s.finishShutdownLocked()
}

func callCompletion(call func() error) (err error) {
	defer func() {
		if recover() != nil {
			err = fmt.Errorf("deferred completion callback failed")
		}
	}()
	return call()
}

func (s *Store) releaseRunningLocked(j *job) {
	if !j.runningHeld {
		return
	}
	j.runningHeld = false
	s.stats.Running--
	if bucket := s.owners[j.record.owner]; bucket != nil {
		bucket.running--
	}
}

func (s *Store) releaseOutputLocked(j *job) {
	j.slabs = nil
	s.stats.OutputBytes -= j.outputBytes
	j.outputBytes = 0
}

func (s *Store) finishOwnershipLocked(j *job) {
	if !j.ownershipDone && j.budgetDrained && j.inputReaders == 0 {
		j.ownershipDone = true
		close(j.ownershipDoneCh)
	}
}

func (s *Store) releaseJobLocked(j *job) {
	if !j.sourceDone || !j.finalized || !j.budgetDrained || j.inputReaders != 0 {
		return
	}
	j.body, j.headers = nil, nil
	s.stats.InputBytes -= j.inputBytes
	s.stats.WorkBytes -= j.workBytes
	j.inputBytes, j.workBytes = 0, 0
	if !j.released {
		return
	}
	if _, ok := s.jobs[j]; !ok {
		return
	}
	delete(s.jobs, j)
	s.stats.Jobs--
	if bucket := s.owners[j.record.owner]; bucket != nil {
		bucket.jobs--
		bucket.lastSeenMS = s.clock.Now().UnixMilli()
	}
	j.head, j.outcome, j.hasher = nil, nil, nil
	j.record.job = nil
}

func (s *Store) infoLocked(j *job) JobInfo {
	receipt := j.record.receipt
	receipt.State = j.state
	info := JobInfo{Receipt: receipt, AvailableBytes: j.tail, RetainUntilMS: j.retainUntilMS}
	if j.head != nil {
		head := *j.head
		head.Headers = j.head.Headers.Clone()
		if j.head.Retry != nil {
			retry := *j.head.Retry
			retry.RemainingMS = min(retry.DeadlineMS-retry.ObservedAtMS, max(int64(0), retry.DeadlineMS-s.clock.Now().UnixMilli()))
			head.Retry = &retry
		}
		info.Response = &head
	}
	if j.outcome != nil {
		outcome := *j.outcome
		if j.outcome.Error != nil {
			detail := *j.outcome.Error
			outcome.Error = &detail
		}
		info.Outcome = &outcome
	}
	return info
}
