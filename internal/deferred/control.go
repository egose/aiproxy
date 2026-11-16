package deferred

import (
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"
)

type tokenBucket struct {
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newTokenBucket(rate, burst float64, now time.Time) tokenBucket {
	return tokenBucket{rate: rate, burst: burst, tokens: burst, last: now}
}

func (b *tokenBucket) refill(now time.Time) {
	if now.After(b.last) {
		b.tokens = min(b.burst, b.tokens+now.Sub(b.last).Seconds()*b.rate)
		b.last = now
	}
}

type ownerBucket struct {
	owner          Owner
	control        tokenBucket
	deletion       tokenBucket
	jobs           int
	running        int
	controls       int
	deletes        int
	lastSeenMS     int64
	previous, next *ownerBucket
}

type Control struct {
	store    *Store
	owner    Owner
	deleting bool
	active   bool
	released bool
	refs     int
	snapshot bool
	encoded  bool
	record   *record
	scratch  []byte
	once     sync.Once
}

type Ingress struct {
	*Control
	scratchBytes int64
	extraBytes   int64
	wireLimit    int64
	preparation  *Preparation
	read         bool
	begun        bool
	released     bool
	ioRefs       int
	once         sync.Once
}

func (s *Store) AcquireControl(owner Owner, deleting bool) (*Control, error) {
	if ValidateOwner(owner) != nil {
		return nil, protocolError(401, "auth_failed", "deferred controls require an inference identity", AcceptanceNotAccepted)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.acquireControlLocked(owner, deleting)
}

func (s *Store) acquireControlLocked(owner Owner, deleting bool) (*Control, error) {
	if s.shuttingDown {
		return nil, protocolError(503, "deferred_saturated", "deferred store is shutting down", AcceptanceNotAccepted)
	}
	now := s.clock.Now()
	s.cleanupOwnersLocked(now.UnixMilli(), 128)
	bucket := s.owners[owner]
	if bucket == nil {
		if len(s.owners) >= s.options.MaxEntries {
			return nil, protocolError(429, "deferred_control_limited", "deferred owner control table is full", AcceptanceNotAccepted)
		}
		owner = cloneOwner(owner)
		bucket = &ownerBucket{
			owner: owner, control: newTokenBucket(20, 40, now), deletion: newTokenBucket(2, 4, now),
			lastSeenMS: now.UnixMilli(), next: s.ownerHead,
		}
		if bucket.next != nil {
			bucket.next.previous = bucket
		}
		s.ownerHead = bucket
		s.owners[owner] = bucket
	}
	bucket.lastSeenMS = now.UnixMilli()
	global, personal := &s.global, &bucket.control
	full := s.stats.Controls >= s.options.MaxControlInflight-2 || bucket.controls >= 4
	if deleting {
		global, personal = &s.globalDelete, &bucket.deletion
		full = s.stats.Deletes >= 2
	}
	global.refill(now)
	personal.refill(now)
	if full || global.tokens < 1 || personal.tokens < 1 {
		failure := protocolError(429, "deferred_control_limited", "deferred control capacity is exhausted", AcceptanceNotAccepted)
		failure.Wire.PollAfterMS = PollAfterMS
		return nil, failure
	}
	global.tokens--
	personal.tokens--
	control := &Control{store: s, owner: bucket.owner, deleting: deleting, active: true, refs: 1}
	if deleting {
		s.stats.Deletes++
		bucket.deletes++
	} else {
		s.stats.Controls++
		bucket.controls++
	}
	return control, nil
}

func (s *Store) AcquireIngress(owner Owner, contentLength int64) (*Ingress, error) {
	if ValidateOwner(owner) != nil {
		return nil, protocolError(401, "auth_failed", "deferred submissions require an inference identity", AcceptanceNotAccepted)
	}
	hardWire, _ := SubmitWireBytes(HardRequestBytes)
	if contentLength > hardWire || contentLength < -1 {
		return nil, protocolError(413, "request_too_large", "deferred submit body exceeds hard limit", AcceptanceNotAccepted)
	}
	wireLimit := contentLength
	if wireLimit <= 0 {
		wireLimit = hardWire
	}
	decoded := min(HardRequestBytes, (wireLimit+3)/4*3)
	scratch := wireLimit + decoded
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stats.Admissions >= s.options.MaxAdmissions {
		return nil, protocolError(503, "deferred_saturated", "deferred submit readers are exhausted", AcceptanceNotAccepted)
	}
	base, _ := SubmitScratchBytes(s.options.MaxRequestBytes)
	extra := max(int64(0), scratch-base)
	if extra > s.options.MaxMemoryBytes-s.reservedLocked() {
		return nil, protocolError(503, "deferred_saturated", "deferred submit memory capacity is exhausted", AcceptanceNotAccepted)
	}
	control, err := s.acquireControlLocked(owner, false)
	if err != nil {
		return nil, err
	}
	ingress := &Ingress{Control: control, scratchBytes: scratch, extraBytes: extra, wireLimit: wireLimit}
	s.ingresses[ingress] = struct{}{}
	s.stats.Admissions++
	s.stats.IngressExtraBytes += extra
	return ingress, nil
}

func (i *Ingress) WireLimit() int64 { return i.wireLimit }

func (i *Ingress) ReadSubmit(reader io.Reader) (SubmitRequest, error) {
	i.store.mu.Lock()
	if !i.active || i.released || i.read {
		i.store.mu.Unlock()
		return SubmitRequest{}, fmt.Errorf("deferred ingress has been used or released")
	}
	i.read = true
	i.ioRefs++
	i.store.mu.Unlock()
	defer func() {
		i.finishIO()
	}()
	data := make([]byte, int(i.wireLimit)+1)
	n, err := io.ReadFull(reader, data)
	if n == len(data) {
		return SubmitRequest{}, protocolError(413, "request_too_large", "deferred submit body exceeds ingress limit", AcceptanceNotAccepted)
	}
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return SubmitRequest{}, protocolError(400, "invalid_body", "cannot read deferred submit body", AcceptanceNotAccepted)
	}
	return DecodeSubmit(data[:n])
}

func (i *Ingress) finishIO() {
	i.store.mu.Lock()
	i.ioRefs--
	release := i.released && i.ioRefs == 0
	if release {
		i.releaseLocked()
	}
	i.store.mu.Unlock()
	if release {
		i.Control.Release()
	}
}

func (i *Ingress) Release() {
	if i == nil {
		return
	}
	i.once.Do(func() {
		s := i.store
		s.mu.Lock()
		i.released = true
		release := i.ioRefs == 0
		if release {
			i.releaseLocked()
		}
		s.mu.Unlock()
		if release {
			i.Control.Release()
		}
	})
}

func (i *Ingress) releaseLocked() {
	s := i.store
	if i.preparation != nil && i.preparation.record.preparing {
		s.rejectLocked(i.preparation.record, protocolError(400, "invalid_request", "deferred preparation was interrupted", AcceptanceNotAccepted))
	}
	delete(s.ingresses, i)
	s.stats.Admissions--
	s.stats.IngressExtraBytes -= i.extraBytes
	i.preparation = nil
}

func (c *Control) Release() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		s := c.store
		s.mu.Lock()
		defer s.mu.Unlock()
		c.released = true
		c.releaseRefLocked()
	})
}

func (c *Control) releaseRefLocked() {
	c.refs--
	if c.refs != 0 {
		return
	}
	s := c.store
	c.active, c.scratch = false, nil
	if c.record != nil && c.record.control == c {
		c.record.control = nil
	}
	bucket := s.owners[c.owner]
	bucket.lastSeenMS = s.clock.Now().UnixMilli()
	if c.deleting {
		s.stats.Deletes--
		bucket.deletes--
	} else {
		s.stats.Controls--
		bucket.controls--
	}
	s.finishShutdownLocked()
}

func (c *Control) Owner() Owner { return c.owner }

func (c *Control) Encode(writer io.Writer, value any) error {
	c.store.mu.Lock()
	if !c.active || c.released || c.encoded {
		c.store.mu.Unlock()
		return fmt.Errorf("deferred control reservation has been used or released")
	}
	c.encoded = true
	c.refs++
	c.store.mu.Unlock()
	defer func() {
		c.store.mu.Lock()
		defer c.store.mu.Unlock()
		c.releaseRefLocked()
	}()
	if err := validateControlValue(value); err != nil {
		return err
	}
	if _, err := JSONEncodedSize(value, MaxControlBodyBytes-1, ControlJSONDepth, ControlJSONTokens); err != nil {
		return err
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if len(data)+1 > MaxControlBodyBytes {
		return fmt.Errorf("deferred control response exceeds limit")
	}
	data = append(data, '\n')
	n, err := writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	return err
}

func (s *Store) controlLocked(control *Control, deleting bool) error {
	if control == nil || control.store != s || !control.active || control.released || control.deleting != deleting {
		return protocolError(429, "deferred_control_limited", "a matching control reservation is required", AcceptanceNotAccepted)
	}
	return nil
}

func (s *Store) lookupLocked(control *Control, generation, id string) (*record, error) {
	if ValidateID(generation) != nil || ValidateID(id) != nil {
		return nil, protocolError(400, "invalid_request", "invalid deferred job identifier", AcceptanceAccepted)
	}
	if generation != s.generation {
		return nil, protocolError(410, "generation_mismatch", "deferred execution generation changed", AcceptanceAccepted)
	}
	rec := s.byID[id]
	if rec == nil || rec.owner != control.owner {
		return nil, protocolError(404, "job_unavailable", "deferred job is unavailable", AcceptanceAccepted)
	}
	if !control.deleting {
		if control.record != nil && control.record != rec || rec.control != nil && rec.control != control {
			return nil, protocolError(429, "deferred_control_limited", "deferred job already has an active control", AcceptanceAccepted)
		}
		control.record, rec.control = rec, control
	}
	now := s.clock.Now().UnixMilli()
	if rec.job != nil && !rec.job.state.Terminal() && now >= rec.job.executionDeadlineMS {
		s.sealLocked(rec.job, failureOutcome(OutcomeExecutionTimeout, "deferred_execution_timeout", "deferred execution deadline exceeded"))
	}
	s.expireRecordLocked(rec, now)
	if rec.protectedMS <= now && rec.job == nil {
		s.removeRecordLocked(rec)
		return nil, protocolError(404, "job_unavailable", "deferred job is unavailable", AcceptanceAccepted)
	}
	return rec, nil
}

func (s *Store) Model(control *Control, generation, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if control == nil || control.store != s || !control.active || control.released {
		return "", protocolError(429, "deferred_control_limited", "a control reservation is required", AcceptanceNotAccepted)
	}
	rec, err := s.lookupLocked(control, generation, id)
	if err != nil {
		return "", err
	}
	return rec.receipt.PublicModel, nil
}

func snapshotControl(control *Control) error {
	if control.snapshot {
		return protocolError(429, "deferred_control_limited", "control snapshot has already been taken", AcceptanceAccepted)
	}
	control.snapshot = true
	return nil
}

func tombstoneError(code string) *ProtocolError {
	message := "deferred job has been released"
	if code == "job_expired" {
		message = "deferred job has expired"
	}
	return protocolError(410, code, message, AcceptanceAccepted)
}

func (s *Store) Status(control *Control, generation, id string) (JobInfo, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.controlLocked(control, false); err != nil {
		return JobInfo{}, err
	}
	rec, err := s.lookupLocked(control, generation, id)
	if err != nil {
		return JobInfo{}, err
	}
	if rec.tombstone != "" {
		return JobInfo{}, tombstoneError(rec.tombstone)
	}
	if err := snapshotControl(control); err != nil {
		return JobInfo{}, err
	}
	return s.infoLocked(rec.job), nil
}

func (s *Store) Result(control *Control, generation, id string) (ResultManifest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.controlLocked(control, false); err != nil {
		return ResultManifest{}, false, err
	}
	rec, err := s.lookupLocked(control, generation, id)
	if err != nil {
		return ResultManifest{}, false, err
	}
	if rec.tombstone != "" {
		return ResultManifest{}, false, tombstoneError(rec.tombstone)
	}
	if err := snapshotControl(control); err != nil {
		return ResultManifest{}, false, err
	}
	j := rec.job
	return ResultManifest{JobInfo: s.infoLocked(j), BodySHA256: j.bodyHash}, j.state.Terminal(), nil
}

func (s *Store) Read(control *Control, generation, id string, cursor uint64, limit int) (ReadFrame, error) {
	if limit == 0 {
		limit = MaxReadBytes
	}
	if limit < 1 || limit > MaxReadBytes {
		return ReadFrame{}, protocolError(400, "invalid_cursor", "invalid deferred read limit", AcceptanceAccepted)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.controlLocked(control, false); err != nil {
		return ReadFrame{}, err
	}
	rec, err := s.lookupLocked(control, generation, id)
	if err != nil {
		return ReadFrame{}, err
	}
	if rec.tombstone != "" {
		return ReadFrame{}, tombstoneError(rec.tombstone)
	}
	if err := snapshotControl(control); err != nil {
		return ReadFrame{}, err
	}
	j := rec.job
	if cursor > j.tail {
		return ReadFrame{}, protocolError(400, "invalid_cursor", "deferred cursor exceeds available bytes", AcceptanceAccepted)
	}
	n := min(uint64(limit), j.tail-cursor)
	if control.scratch == nil {
		control.scratch = make([]byte, int(n))
	}
	data := control.scratch[:int(n):int(n)]
	for copied := uint64(0); copied < n; {
		at := cursor + copied
		slab, offset := at/ResultSlabBytes, at%ResultSlabBytes
		size := min(n-copied, ResultSlabBytes-offset)
		copy(data[int(copied):int(copied+size)], j.slabs[int(slab)][int(offset):int(offset+size)])
		copied += size
	}
	next := cursor + n
	return ReadFrame{
		JobInfo: s.infoLocked(j), Cursor: cursor, NextCursor: next,
		Data: data, Done: j.state.Terminal() && next == j.tail,
	}, nil
}

func validateControlValue(value any) error {
	switch v := value.(type) {
	case Capability:
		return validateCapability(v)
	case *Capability:
		if v != nil {
			return validateCapability(*v)
		}
	case Receipt:
		return ValidateReceipt(v)
	case *Receipt:
		if v != nil {
			return ValidateReceipt(*v)
		}
	case JobInfo:
		return ValidateJobInfo(v)
	case *JobInfo:
		if v != nil {
			return ValidateJobInfo(*v)
		}
	case ResultManifest:
		if err := ValidateJobInfo(v.JobInfo); err != nil {
			return err
		}
		if v.State.Terminal() && validateHash(v.BodySHA256) == nil {
			return nil
		}
	case *ResultManifest:
		if v != nil {
			return validateControlValue(*v)
		}
	case ReadFrame:
		return ValidateReadFrame(v)
	case *ReadFrame:
		if v != nil {
			return ValidateReadFrame(*v)
		}
	case ControlError:
		if v.Envelope == NewEnvelope() && ValidateErrorDetail(v.Error) == nil &&
			(v.Acceptance == AcceptanceNotAccepted || v.Acceptance == AcceptanceAccepted || v.Acceptance == AcceptanceIndeterminate) &&
			v.PollAfterMS >= 0 && v.PollAfterMS <= 2000 {
			return nil
		}
	case *ControlError:
		if v != nil {
			return validateControlValue(*v)
		}
	}
	return fmt.Errorf("invalid deferred control response")
}

func (s *Store) Delete(control *Control, generation, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.controlLocked(control, true); err != nil {
		return err
	}
	rec, err := s.lookupLocked(control, generation, id)
	if err != nil {
		return err
	}
	if rec.tombstone == "job_released" {
		return nil
	}
	if rec.tombstone != "" {
		return tombstoneError(rec.tombstone)
	}
	s.releaseRecordLocked(rec, "job_released")
	return nil
}

func (s *Store) releaseRecordLocked(rec *record, code string) {
	if rec.tombstone != "" {
		return
	}
	rec.tombstone = code
	if code == "job_expired" {
		s.stats.Expirations++
	}
	j := rec.job
	if j == nil {
		return
	}
	if !j.state.Terminal() {
		s.sealLocked(j, failureOutcome(OutcomeCancelled, "deferred_cancelled", "deferred execution cancelled"))
	}
	j.released = true
	s.releaseOutputLocked(j)
	s.releaseJobLocked(j)
}

func (s *Store) cleanupOwnersLocked(now int64, limit int) {
	if s.ownerCursor == nil {
		s.ownerCursor = s.ownerHead
	}
	for i := 0; i < limit && s.ownerCursor != nil; i++ {
		bucket := s.ownerCursor
		s.ownerCursor = bucket.next
		if bucket.jobs != 0 || bucket.controls != 0 || bucket.deletes != 0 || now-bucket.lastSeenMS < time.Minute.Milliseconds() {
			continue
		}
		delete(s.owners, bucket.owner)
		if bucket.previous == nil {
			s.ownerHead = bucket.next
		} else {
			bucket.previous.next = bucket.next
		}
		if bucket.next != nil {
			bucket.next.previous = bucket.previous
		}
		bucket.next, bucket.previous = nil, nil
	}
}

func metadataLimit(value any) error {
	if _, err := JSONEncodedSize(value, MaxMetadataBytes, ControlJSONDepth, ControlJSONTokens); err != nil {
		return fmt.Errorf("deferred metadata exceeds limit")
	}
	return nil
}
