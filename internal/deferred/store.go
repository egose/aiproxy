package deferred

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

var ErrCleanupIncomplete = errors.New("deferred cleanup incomplete")

type Clock interface {
	Now() time.Time
	NewTimer(time.Duration) Timer
}

type Timer interface {
	C() <-chan time.Time
	Stop() bool
}

type systemClock struct{}
type systemTimer struct{ *time.Timer }

func (systemClock) Now() time.Time { return time.Now() }
func (systemClock) NewTimer(d time.Duration) Timer {
	return systemTimer{time.NewTimer(d)}
}
func (t systemTimer) C() <-chan time.Time { return t.Timer.C }

type Hooks struct {
	Clock  Clock
	Random io.Reader
}

type Stats struct {
	Enabled              bool
	Jobs                 int
	Running              int
	Admissions           int
	Entries              int
	Owners               int
	Controls             int
	Deletes              int
	FinalizationCredits  int
	Finalizing           int
	FixedBytes           int64
	InputBytes           int64
	OutputBytes          int64
	WorkBytes            int64
	IngressExtraBytes    int64
	ReservedBytes        int64
	FinalizationFailures uint64
	Cancellations        uint64
	Expirations          uint64
	RunningJobs          int
	HeadersReadyJobs     int
	CompletedJobs        int
	FailedJobs           int
	CancelledJobs        int
	AvailableBytes       int64
}

type Store struct {
	mu           sync.Mutex
	options      Options
	fixedBytes   int64
	generation   string
	secret       [32]byte
	random       io.Reader
	clock        Clock
	ctx          context.Context
	cancel       context.CancelFunc
	shuttingDown bool
	stopped      bool
	stop         chan struct{}
	done         chan struct{}
	background   int
	finishWake   chan struct{}
	finishHead   *job
	finishTail   *job
	entries      map[string]*record
	byID         map[string]*record
	entryHead    *record
	entryCursor  *record
	owners       map[Owner]*ownerBucket
	ownerHead    *ownerBucket
	ownerCursor  *ownerBucket
	ingresses    map[*Ingress]struct{}
	jobs         map[*job]struct{}
	global       tokenBucket
	globalDelete tokenBucket
	stats        Stats
}

type record struct {
	key            string
	owner          Owner
	fingerprint    string
	submitUntilMS  int64
	protectedMS    int64
	preparing      bool
	accepted       bool
	rejection      *ProtocolError
	receipt        Receipt
	tombstone      string
	job            *job
	control        *Control
	previous, next *record
}

type Input struct {
	PublicModel string
	Owner       Owner
	Headers     map[string][]string
	Body        []byte
	Limits      Limits
}

type Preparation struct {
	store   *Store
	record  *record
	ingress *Ingress
	budget  *WorkBudget
}

type Update struct {
	store   *Store
	options Options
	used    bool
	mu      sync.Mutex
}

func New(ctx context.Context, options Options, hooks ...Hooks) (*Store, error) {
	if err := options.Validate(); err != nil {
		return nil, err
	}
	var hook Hooks
	if len(hooks) > 1 {
		return nil, fmt.Errorf("at most one deferred hook set is supported")
	}
	if len(hooks) == 1 {
		hook = hooks[0]
	}
	if hook.Clock == nil {
		hook.Clock = systemClock{}
	}
	if hook.Random == nil {
		hook.Random = rand.Reader
	}
	generation, err := randomID(hook.Random)
	if err != nil {
		return nil, fmt.Errorf("deferred generation: %w", err)
	}
	fixed, err := options.FixedMemoryBytes()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		ctx = context.Background()
	}
	appCtx, cancel := context.WithCancel(ctx)
	s := &Store{
		options: options, fixedBytes: fixed, generation: generation, random: hook.Random, clock: hook.Clock,
		ctx: appCtx, cancel: cancel, stop: make(chan struct{}), done: make(chan struct{}), background: FinalizationWorkers + 1,
		finishWake: make(chan struct{}, FinalizationWorkers), entries: make(map[string]*record),
		byID: make(map[string]*record), owners: make(map[Owner]*ownerBucket),
		ingresses: make(map[*Ingress]struct{}), jobs: make(map[*job]struct{}),
		global: newTokenBucket(100, 200, hook.Clock.Now()), globalDelete: newTokenBucket(8, 16, hook.Clock.Now()),
	}
	if _, err := io.ReadFull(hook.Random, s.secret[:]); err != nil {
		cancel()
		return nil, fmt.Errorf("deferred signing key: %w", err)
	}
	for range FinalizationWorkers {
		go s.completionWorker()
	}
	go s.maintenance()
	return s, nil
}

func (s *Store) Generation() string { return s.generation }

func (s *Store) Options() Options {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.options
}

func (s *Store) Stats() Stats {
	s.mu.Lock()
	defer s.mu.Unlock()
	stats := s.stats
	stats.Enabled = s.options.Enabled && !s.shuttingDown
	stats.FixedBytes = s.fixedBytes
	stats.Entries, stats.Owners = len(s.entries), len(s.owners)
	stats.ReservedBytes = s.reservedLocked()
	for j := range s.jobs {
		if j.released || !j.record.accepted {
			continue
		}
		stats.AvailableBytes += int64(j.tail)
		switch j.state {
		case StateRunning:
			stats.RunningJobs++
		case StateHeadersReady:
			stats.HeadersReadyJobs++
		case StateCompleted:
			stats.CompletedJobs++
		case StateFailed:
			stats.FailedJobs++
		case StateCancelled:
			stats.CancelledJobs++
		}
	}
	return stats
}

func (s *Store) reservedLocked() int64 {
	return s.fixedBytes + s.stats.InputBytes + s.stats.OutputBytes + s.stats.WorkBytes + s.stats.IngressExtraBytes
}

func (s *Store) PrepareUpdate(options Options) (*Update, error) {
	if options.Enabled {
		if err := options.Validate(); err != nil {
			return nil, err
		}
	}
	return &Update{store: s, options: options}, nil
}

func (u *Update) Cancel() {
	u.mu.Lock()
	u.used = true
	u.mu.Unlock()
}

func (u *Update) Commit(activate func() error) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.used {
		return fmt.Errorf("deferred update already consumed")
	}
	u.used = true
	s := u.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return fmt.Errorf("deferred store is shutting down")
	}
	options := u.options
	if !options.Enabled {
		options = s.options
		options.Enabled = false
	}
	fixed, err := options.FixedMemoryBytes()
	if err != nil {
		return err
	}
	extra, err := s.validateUpdateLocked(options, fixed)
	if err != nil {
		return err
	}
	if activate != nil {
		if err := activate(); err != nil {
			return err
		}
	}
	s.options, s.fixedBytes, s.stats.IngressExtraBytes = options, fixed, extra
	base, _ := SubmitScratchBytes(options.MaxRequestBytes)
	for ingress := range s.ingresses {
		ingress.extraBytes = max(int64(0), ingress.scratchBytes-base)
	}
	return nil
}

func (s *Store) Reconfigure(options Options) error {
	update, err := s.PrepareUpdate(options)
	if err != nil {
		return err
	}
	return update.Commit(nil)
}

func (s *Store) validateUpdateLocked(options Options, fixed int64) (int64, error) {
	if options.MaxJobs < s.stats.Jobs || options.MaxRunning < s.stats.Running || options.MaxAdmissions < s.stats.Admissions ||
		options.MaxEntries < len(s.entries) || options.MaxEntries < len(s.owners) ||
		options.MaxControlInflight-2 < s.stats.Controls || s.stats.Deletes > 2 {
		return 0, fmt.Errorf("deferred capacity reduction is below live reservations")
	}
	for _, owner := range s.owners {
		if owner.jobs > options.PerOwnerJobs || owner.running > options.PerOwnerRunning {
			return 0, fmt.Errorf("deferred owner capacity reduction is below live reservations")
		}
	}
	base, _ := SubmitScratchBytes(options.MaxRequestBytes)
	var extra int64
	for ingress := range s.ingresses {
		extra += max(int64(0), ingress.scratchBytes-base)
	}
	dynamic := s.stats.InputBytes + s.stats.OutputBytes + s.stats.WorkBytes + extra
	if fixed > options.MaxMemoryBytes || dynamic > options.MaxMemoryBytes-fixed {
		return 0, fmt.Errorf("deferred memory reduction is below live reservations")
	}
	return extra, nil
}

func (s *Store) Capability(control *Control, publicModel string, receiver ReceiverLimits) (Capability, error) {
	if err := ValidatePublicModel(publicModel); err != nil {
		return Capability{}, protocolError(400, "invalid_model", "invalid deferred public model", AcceptanceNotAccepted)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.controlLocked(control, false); err != nil {
		return Capability{}, err
	}
	if !s.options.Enabled || s.shuttingDown {
		return Capability{}, protocolError(404, "deferred_disabled", "deferred admission is disabled", AcceptanceNotAccepted)
	}
	limits, err := s.options.limits(receiver)
	if err != nil {
		return Capability{}, protocolError(400, "invalid_request", "invalid deferred receiver limits", AcceptanceNotAccepted)
	}
	until, err := addMilliseconds(s.clock.Now().UnixMilli(), SubmitWindow.Milliseconds())
	if err != nil {
		return Capability{}, protocolError(400, "invalid_request", "invalid deferred submission time", AcceptanceNotAccepted)
	}
	token, err := s.issueTicket(control.owner, publicModel, limits, until)
	if err != nil {
		return Capability{}, protocolError(503, "deferred_saturated", "cannot issue deferred submission scope", AcceptanceNotAccepted)
	}
	return Capability{
		Envelope: NewEnvelope(), Generation: s.generation, PublicModel: publicModel,
		SubmitToken: token, SubmitUntilMS: until, Limits: limits, PollAfterMS: PollAfterMS,
	}, nil
}

func (s *Store) Begin(ingress *Ingress, publicModel string, request SubmitRequest) (*Preparation, *Receipt, error) {
	s.mu.Lock()
	if ingress == nil || ingress.store != s || !ingress.active || ingress.released {
		s.mu.Unlock()
		return nil, nil, protocolError(429, "deferred_control_limited", "submit ingress reservation is required", AcceptanceNotAccepted)
	}
	if ingress.preparation != nil || ingress.begun {
		s.mu.Unlock()
		return nil, nil, protocolError(409, "submission_in_progress", "submit ingress already has a preparation", AcceptanceNotAccepted)
	}
	ingress.begun = true
	ingress.ioRefs++
	s.mu.Unlock()
	defer ingress.finishIO()
	if request.Envelope != NewEnvelope() {
		return nil, nil, protocolError(426, "unsupported_deferred_version", "unsupported deferred protocol version", AcceptanceNotAccepted)
	}
	if ValidateID(request.Generation) != nil || ValidateAttemptKey(request.AttemptKey) != nil || ValidatePublicModel(publicModel) != nil || request.Body == nil {
		return nil, nil, protocolError(400, "invalid_body", "invalid deferred submit fields", AcceptanceNotAccepted)
	}
	if int64(len(request.Body)) > HardRequestBytes {
		return nil, nil, protocolError(413, "request_too_large", "deferred input exceeds hard limit", AcceptanceNotAccepted)
	}
	headers, err := CanonicalSubmitHeaders(request.Headers)
	if err != nil {
		return nil, nil, protocolError(400, "invalid_body", "invalid deferred submit headers", AcceptanceNotAccepted)
	}
	metadata := request
	metadata.Headers, metadata.Body = headers, []byte{}
	if err := metadataLimit(metadata); err != nil {
		return nil, nil, protocolError(400, "invalid_body", "deferred submit metadata exceeds limit", AcceptanceNotAccepted)
	}
	fingerprint, err := Fingerprint(publicModel, request.Body, headers)
	if err != nil {
		return nil, nil, protocolError(400, "invalid_body", "invalid deferred request fingerprint", AcceptanceNotAccepted)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if ingress == nil || ingress.store != s || !ingress.active || ingress.released {
		return nil, nil, protocolError(429, "deferred_control_limited", "submit ingress reservation is required", AcceptanceNotAccepted)
	}
	if request.Generation != s.generation {
		return nil, nil, protocolError(410, "generation_mismatch", "deferred execution generation changed", AcceptanceNotAccepted)
	}
	scope, err := s.verifyTicket(ingress.owner, publicModel, request.SubmitToken)
	if err != nil {
		return nil, nil, protocolError(400, "invalid_body", "invalid deferred submission ticket", AcceptanceNotAccepted)
	}
	ownerHash := ownerDigest(ingress.owner)
	key := scope.digest + string(ownerHash[:]) + request.AttemptKey
	now := s.clock.Now().UnixMilli()
	if existing := s.entries[key]; existing != nil {
		s.expireRecordLocked(existing, now)
		if existing.protectedMS <= now && existing.job == nil {
			s.removeRecordLocked(existing)
		} else {
			if existing.fingerprint != fingerprint {
				return nil, nil, protocolError(409, "attempt_conflict", "deferred attempt key conflicts with prior input", AcceptanceNotAccepted)
			}
			if existing.accepted {
				receipt := existing.receipt
				receipt.Replayed = true
				return nil, &receipt, nil
			}
			if existing.preparing {
				failure := protocolError(409, "submission_in_progress", "deferred admission is still preparing", AcceptanceNotAccepted)
				failure.Wire.PollAfterMS = PollAfterMS
				return nil, nil, failure
			}
			if existing.rejection != nil {
				return nil, nil, cloneProtocolError(existing.rejection)
			}
		}
	}
	if scope.untilMS <= now {
		return nil, nil, protocolError(410, "submit_scope_expired", "deferred submission scope expired", AcceptanceNotAccepted)
	}
	if !s.options.Enabled || s.shuttingDown {
		return nil, nil, protocolError(404, "deferred_disabled", "deferred admission is disabled", AcceptanceNotAccepted)
	}
	s.cleanupEntriesLocked(now, 128)
	if len(s.entries) >= s.options.MaxEntries {
		return nil, nil, protocolError(503, "deferred_saturated", "deferred dedup capacity is exhausted", AcceptanceNotAccepted)
	}
	rec := &record{
		key: key, owner: cloneOwner(ingress.owner), fingerprint: fingerprint,
		submitUntilMS: scope.untilMS, protectedMS: scope.untilMS,
	}
	s.addRecordLocked(rec)
	reject := func(failure *ProtocolError) (*Preparation, *Receipt, error) {
		rec.rejection = cloneProtocolError(failure)
		return nil, nil, failure
	}
	if int64(len(request.Body)) > s.options.MaxRequestBytes || int64(len(request.Body)) > scope.limits.RequestBytes {
		return reject(protocolError(413, "request_too_large", "deferred input exceeds admission policy", AcceptanceNotAccepted))
	}
	bucket := s.owners[ingress.owner]
	if bucket.jobs >= s.options.PerOwnerJobs || bucket.running >= s.options.PerOwnerRunning {
		return reject(protocolError(429, "deferred_owner_limit", "deferred owner job capacity is exhausted", AcceptanceNotAccepted))
	}
	if s.stats.Jobs >= s.options.MaxJobs || s.stats.Running >= s.options.MaxRunning {
		return reject(protocolError(503, "deferred_saturated", "deferred execution capacity is exhausted", AcceptanceNotAccepted))
	}
	limits := scope.limits
	limits.RequestBytes = min(limits.RequestBytes, s.options.MaxRequestBytes)
	limits.ResultBytes = min(limits.ResultBytes, s.options.MaxResultBytes)
	limits.ExecutionTimeoutMS = min(limits.ExecutionTimeoutMS, s.options.ExecutionTimeout.Milliseconds())
	limits.RetentionMS = min(limits.RetentionMS, s.options.Retention.Milliseconds())
	limits.ControlTimeoutMS = min(limits.ControlTimeoutMS, s.options.ControlTimeout.Milliseconds())
	limits.FinalizationTimeoutMS = min(limits.FinalizationTimeoutMS, s.options.FinalizationTimeout.Milliseconds())
	output, _ := RoundedResultBytes(limits.ResultBytes)
	input, workBytes := int64(len(request.Body)), s.options.MaxWorkBytes
	reservation := input + output + workBytes
	if reservation > s.options.MaxMemoryBytes-s.reservedLocked() {
		return reject(protocolError(503, "deferred_saturated", "deferred memory capacity is exhausted", AcceptanceNotAccepted))
	}
	id, err := randomID(s.random)
	if err != nil {
		return reject(protocolError(503, "deferred_saturated", "cannot allocate deferred job identity", AcceptanceNotAccepted))
	}
	if s.byID[id] != nil {
		return reject(protocolError(503, "deferred_saturated", "cannot allocate a unique deferred job identity", AcceptanceNotAccepted))
	}
	for reserved := range s.jobs {
		if reserved.record.receipt.ID == id {
			return reject(protocolError(503, "deferred_saturated", "cannot allocate a unique deferred job identity", AcceptanceNotAccepted))
		}
	}
	protected, err := addMilliseconds(scope.untilMS, scope.limits.ExecutionTimeoutMS, scope.limits.RetentionMS, scope.limits.FinalizationTimeoutMS)
	if err != nil {
		return reject(protocolError(400, "invalid_body", "deferred lifetime overflows", AcceptanceNotAccepted))
	}
	rec.preparing = true
	rec.receipt = Receipt{
		Envelope: NewEnvelope(), Generation: s.generation, ID: id, PublicModel: strings.Clone(publicModel),
		AttemptKey: strings.Clone(request.AttemptKey), ScopeDigest: scope.digest, Fingerprint: fingerprint,
		State: StateRunning, DedupUntilMS: protected, PollAfterMS: PollAfterMS,
	}
	j := newJob(s, rec, headers, request.Body, limits, input, output, workBytes)
	rec.job = j
	s.jobs[j] = struct{}{}
	s.stats.Jobs++
	s.stats.Running++
	s.stats.FinalizationCredits++
	s.stats.InputBytes += input
	s.stats.OutputBytes += output
	s.stats.WorkBytes += workBytes
	bucket.jobs++
	bucket.running++
	preparation := &Preparation{store: s, record: rec, ingress: ingress, budget: j.budget}
	ingress.preparation = preparation
	return preparation, nil, nil
}

func (p *Preparation) WorkBudget() *WorkBudget { return p.budget }

func (p *Preparation) ConstrainReceiver(receiver ReceiverLimits) error {
	s := p.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if !p.record.preparing || p.record.job == nil {
		if p.record.rejection != nil {
			return cloneProtocolError(p.record.rejection)
		}
		return protocolError(409, "submission_in_progress", "deferred preparation already resolved", AcceptanceNotAccepted)
	}
	limits, err := s.options.limits(receiver)
	if err != nil {
		return protocolError(400, "invalid_request", "invalid deferred receiver limits", AcceptanceNotAccepted)
	}
	j := p.record.job
	j.limits.ResultBytes = min(j.limits.ResultBytes, limits.ResultBytes)
	j.limits.ExecutionTimeoutMS = min(j.limits.ExecutionTimeoutMS, limits.ExecutionTimeoutMS)
	return nil
}

func (p *Preparation) Input() (Input, func(), error) {
	s := p.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if !p.record.preparing || p.record.job == nil {
		return Input{}, nil, protocolError(410, "submit_scope_expired", "deferred preparation is no longer active", AcceptanceNotAccepted)
	}
	j := p.record.job
	if j.inputReaders != 0 {
		return Input{}, nil, protocolError(429, "deferred_control_limited", "deferred preparation input is already borrowed", AcceptanceNotAccepted)
	}
	j.inputReaders++
	input := j.inputLocked()
	release := sync.OnceFunc(func() {
		s.mu.Lock()
		j.inputReaders--
		s.finishOwnershipLocked(j)
		s.releaseJobLocked(j)
		s.finishShutdownLocked()
		s.mu.Unlock()
	})
	return input, release, nil
}

func (p *Preparation) Reject(failure *ProtocolError) error {
	if failure == nil || failure.Status < 400 || failure.Status > 599 || ValidateErrorDetail(failure.Wire.Error) != nil || failure.Wire.Acceptance != AcceptanceNotAccepted {
		return fmt.Errorf("invalid deferred admission rejection")
	}
	s := p.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if !p.record.preparing {
		if p.record.rejection != nil {
			return cloneProtocolError(p.record.rejection)
		}
		return fmt.Errorf("deferred preparation already resolved")
	}
	s.rejectLocked(p.record, failure)
	return nil
}

func (p *Preparation) Abort() {
	if p == nil {
		return
	}
	s := p.store
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.record.preparing {
		s.rejectLocked(p.record, protocolError(400, "invalid_request", "deferred preparation was interrupted", AcceptanceNotAccepted))
	}
}

func (s *Store) rejectLocked(rec *record, failure *ProtocolError) {
	rec.preparing = false
	rec.rejection = cloneProtocolError(failure)
	j := rec.job
	if j != nil {
		j.sourceDone, j.finalized, j.released = true, true, true
		s.releaseRunningLocked(j)
		if j.creditHeld {
			j.creditHeld = false
			s.stats.FinalizationCredits--
		}
		j.budgetDrained = j.budget.close()
		s.releaseOutputLocked(j)
		s.releaseJobLocked(j)
	}
	s.finishShutdownLocked()
}

func cloneProtocolError(failure *ProtocolError) *ProtocolError {
	copy := *failure
	copy.Wire.Error.Type = strings.Clone(failure.Wire.Error.Type)
	copy.Wire.Error.Message = strings.Clone(failure.Wire.Error.Message)
	copy.Wire.Error.BlockID = strings.Clone(failure.Wire.Error.BlockID)
	return &copy
}

func (s *Store) addRecordLocked(rec *record) {
	s.entries[rec.key] = rec
	rec.next = s.entryHead
	if rec.next != nil {
		rec.next.previous = rec
	}
	s.entryHead = rec
}

func (s *Store) removeRecordLocked(rec *record) {
	delete(s.entries, rec.key)
	if rec.accepted {
		delete(s.byID, rec.receipt.ID)
	}
	if s.entryCursor == rec {
		s.entryCursor = rec.next
	}
	if rec.previous == nil {
		s.entryHead = rec.next
	} else {
		rec.previous.next = rec.next
	}
	if rec.next != nil {
		rec.next.previous = rec.previous
	}
	rec.previous, rec.next = nil, nil
}

func (s *Store) expireRecordLocked(rec *record, now int64) {
	if rec.preparing && now >= rec.submitUntilMS {
		s.rejectLocked(rec, protocolError(410, "submit_scope_expired", "deferred submission scope expired", AcceptanceNotAccepted))
	}
	if j := rec.job; rec.accepted && j != nil && j.state.Terminal() && !j.released && now >= j.retainUntilMS {
		s.releaseRecordLocked(rec, "job_expired")
	}
}

func (s *Store) cleanupEntriesLocked(now int64, limit int) {
	if s.entryCursor == nil {
		s.entryCursor = s.entryHead
	}
	for i := 0; i < limit && s.entryCursor != nil; i++ {
		rec := s.entryCursor
		s.entryCursor = rec.next
		s.expireRecordLocked(rec, now)
		if rec.protectedMS <= now && rec.job == nil {
			s.removeRecordLocked(rec)
		}
	}
}

func (s *Store) Sweep() {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.clock.Now().UnixMilli()
	for j := range s.jobs {
		if j.record.accepted && !j.state.Terminal() && (now >= j.executionDeadlineMS || j.ctx.Err() != nil) {
			outcome := failureOutcome(OutcomeExecutionTimeout, "deferred_execution_timeout", "deferred execution deadline exceeded")
			if s.shuttingDown || s.ctx.Err() != nil {
				outcome = failureOutcome(OutcomeCancelled, "deferred_cancelled", "deferred execution cancelled")
			}
			s.sealLocked(j, outcome)
		}
		if j.record.accepted {
			s.expireRecordLocked(j.record, now)
		}
	}
	s.cleanupEntriesLocked(now, 128)
	s.cleanupOwnersLocked(now, 128)
	s.finishShutdownLocked()
}

func (s *Store) maintenance() {
	defer s.backgroundDone()
	for {
		timer := s.clock.NewTimer(50 * time.Millisecond)
		select {
		case <-s.stop:
			timer.Stop()
			return
		case <-s.ctx.Done():
			timer.Stop()
			s.BeginShutdown()
			<-s.stop
			return
		case <-timer.C():
			s.Sweep()
		}
	}
}

func (s *Store) BeginShutdown() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.shuttingDown {
		return
	}
	s.shuttingDown = true
	s.cancel()
	for j := range s.jobs {
		if j.record.preparing {
			s.rejectLocked(j.record, protocolError(503, "deferred_saturated", "deferred store is shutting down", AcceptanceNotAccepted))
			continue
		}
		if !j.state.Terminal() {
			s.sealLocked(j, failureOutcome(OutcomeCancelled, "deferred_cancelled", "deferred execution cancelled"))
		}
		s.releaseRecordLocked(j.record, "job_released")
	}
	s.finishShutdownLocked()
}

func (s *Store) finishShutdownLocked() {
	if !s.shuttingDown || s.stopped || s.stats.Jobs != 0 || s.stats.Controls != 0 || s.stats.Deletes != 0 || s.stats.Admissions != 0 {
		return
	}
	s.stopped = true
	clear(s.entries)
	clear(s.byID)
	clear(s.owners)
	s.entryHead, s.entryCursor, s.ownerHead, s.ownerCursor = nil, nil, nil, nil
	s.fixedBytes = 0
	close(s.stop)
}

func (s *Store) backgroundDone() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.background--
	if s.background == 0 {
		close(s.done)
	}
}

func (s *Store) Close(ctx context.Context) error {
	if s == nil {
		return nil
	}
	s.BeginShutdown()
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-s.done:
		return nil
	default:
	}
	select {
	case <-s.done:
		return nil
	case <-ctx.Done():
		return errors.Join(ErrCleanupIncomplete, ctx.Err())
	}
}
