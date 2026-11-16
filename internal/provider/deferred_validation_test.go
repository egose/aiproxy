package provider

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
)

func callerValidationState(t *testing.T) (*deferredCaller, deferred.JobInfo) {
	t.Helper()
	generation, _ := deferred.NewAttemptKey()
	id, _ := deferred.NewAttemptKey()
	key, _ := deferred.NewAttemptKey()
	now := time.Now()
	receipt := deferred.Receipt{
		Envelope: deferred.NewEnvelope(), Generation: generation, ID: id, PublicModel: "p/m",
		AttemptKey: key, ScopeDigest: strings.Repeat("a", 64), Fingerprint: strings.Repeat("b", 64),
		State: deferred.StateRunning, ExecutionDeadlineMS: now.Add(time.Second).UnixMilli(),
		DedupUntilMS: now.Add(time.Minute).UnixMilli(), PollAfterMS: 250,
	}
	prefix, _ := url.Parse("http://peer" + deferred.Prefix)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	t.Cleanup(cancel)
	s := &deferredCaller{
		ctx: ctx, requestCtx: ctx, cancel: cancel, prefix: prefix, receipt: receipt,
		options: config.DefaultDeferredTransport(), acceptance: deferred.AcceptanceAccepted,
		capability: deferred.Capability{Limits: deferred.Limits{ResultBytes: 64 << 10, ControlTimeoutMS: 100}},
	}
	info := deferred.JobInfo{Receipt: receipt, Response: &deferred.ResponseHead{StatusCode: 429, Headers: http.Header{"Content-Type": {"application/json"}, "Retry-After": {"3600"}}}}
	info.State = deferred.StateHeadersReady
	return s, info
}

func TestDeferredCallerRetryObservationFixesEarliestLocalDeadlineFromCallStart(t *testing.T) {
	s, info := callerValidationState(t)
	observed := time.Now().Add(-time.Second).Truncate(time.Millisecond)
	info.Response.Retry = &deferred.RetryAdvice{ObservedAtMS: observed.UnixMilli(), DeadlineMS: observed.Add(2 * time.Second).UnixMilli(), RemainingMS: 500}
	started := time.Now().Add(-100 * time.Millisecond)
	if err := s.observeInfo(info, started); err != nil {
		t.Fatal(err)
	}
	first := s.result(OriginInference, s.head)
	if !first.RetryDeadline.Equal(started.Add(500*time.Millisecond)) || !first.RetryObservedAt.Equal(observed) ||
		first.RetryDelay <= 0 || first.RetryDelay > 400*time.Millisecond {
		t.Fatalf("retry duration restarted after transfer: %+v", first)
	}
	nextHead := *info.Response
	nextRetry := *nextHead.Retry
	nextRetry.RemainingMS = 450
	nextHead.Retry, info.Response = &nextRetry, &nextHead
	if err := s.observeInfo(info, time.Now()); err != nil {
		t.Fatal(err)
	}
	if !s.retryDeadline.Equal(first.RetryDeadline) || !s.retryObserved.Equal(observed) {
		t.Fatal("repeated retrieval extended the original cooldown observation")
	}
	nextRetry.RemainingMS = 0
	lastStarted := time.Now().Add(-time.Millisecond)
	if err := s.observeInfo(info, lastStarted); err != nil {
		t.Fatal(err)
	}
	if !s.retryDeadline.Equal(lastStarted) || s.result(OriginInference, s.head).RetryDelay != 0 {
		t.Fatal("expired peer advice fell back to original Retry-After headers")
	}
}

func TestDeferredCallerRejectsIdentityStateHeadAndCursorChangesBeforeBytes(t *testing.T) {
	for _, change := range []string{"id", "generation", "fingerprint", "deadline", "running-head", "missing-head", "private-head", "oversize-tail", "head-changed", "cursor", "done", "base64", "unknown-field"} {
		t.Run(change, func(t *testing.T) {
			s, info := callerValidationState(t)
			info.Response.StatusCode = 200
			if err := s.observeInfo(info, time.Now()); err != nil {
				t.Fatal(err)
			}
			frame := deferred.ReadFrame{JobInfo: info, Cursor: 0, NextCursor: 1, Data: []byte("x")}
			frame.AvailableBytes = 1
			head := *frame.Response
			head.Headers = head.Headers.Clone()
			frame.Response = &head
			switch change {
			case "id":
				frame.ID, _ = deferred.NewAttemptKey()
			case "generation":
				frame.Generation, _ = deferred.NewAttemptKey()
			case "fingerprint":
				frame.Fingerprint = strings.Repeat("c", 64)
			case "deadline":
				frame.ExecutionDeadlineMS++
			case "running-head":
				frame.State = deferred.StateRunning
			case "missing-head":
				frame.Response = nil
			case "private-head":
				frame.Response.Headers.Set("X-Aiproxy-Deferred-Private", "never-expose")
			case "oversize-tail":
				frame.AvailableBytes = 64<<10 + 1
			case "head-changed":
				frame.Response.StatusCode = 201
			case "cursor":
				frame.Cursor, frame.NextCursor, frame.AvailableBytes = 1, 2, 2
			case "done":
				frame.Done = true
			}
			wire, err := json.Marshal(frame)
			if err != nil {
				t.Fatal(err)
			}
			if change == "base64" {
				wire = []byte(strings.Replace(string(wire), `"data":"eA=="`, `"data":"eB=="`, 1))
			} else if change == "unknown-field" {
				wire = append([]byte(`{"unsafe":1,`), wire[1:]...)
			}
			s.client.Transport = callerValidationTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {deferred.MediaType}, "X-Aiproxy-Deferred-Version": {"1"}}, Body: io.NopCloser(strings.NewReader(string(wire)))}, nil
			})
			_, err = s.read(0)
			var control *DeferredControlError
			if !errors.As(err, &control) || control.Code != "deferred_protocol_error" || control.Acceptance != deferred.AcceptanceAccepted ||
				s.info.AvailableBytes != 0 {
				t.Fatalf("invalid bounded frame exposed/advanced bytes: %s error=%v tail=%d", change, err, s.info.AvailableBytes)
			}
		})
	}
}

type callerValidationTransport func(*http.Request) (*http.Response, error)

func (f callerValidationTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestDeferredCallerTerminalManifestRequiresHashAndExactTail(t *testing.T) {
	for _, invalid := range []string{"hash", "tail", "active"} {
		t.Run(invalid, func(t *testing.T) {
			s, info := callerValidationState(t)
			info.State, info.AvailableBytes, info.RetainUntilMS = deferred.StateCompleted, 1, time.Now().Add(time.Minute).UnixMilli()
			info.Outcome = &deferred.Outcome{Kind: deferred.OutcomeResponse}
			if err := s.observeInfo(info, time.Now()); err != nil {
				t.Fatal(err)
			}
			h := sha256.New()
			_, _ = h.Write([]byte("x"))
			manifest := deferred.ResultManifest{JobInfo: info, BodySHA256: hex.EncodeToString(h.Sum(nil))}
			switch invalid {
			case "hash":
				manifest.BodySHA256 = strings.Repeat("c", 64)
			case "tail":
				manifest.AvailableBytes++
			case "active":
				manifest.State, manifest.Outcome, manifest.RetainUntilMS = deferred.StateHeadersReady, nil, 0
			}
			wire, _ := json.Marshal(manifest)
			s.client.Transport = callerValidationTransport(func(request *http.Request) (*http.Response, error) {
				return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {deferred.MediaType}, "X-Aiproxy-Deferred-Version": {"1"}}, Body: io.NopCloser(strings.NewReader(string(wire)))}, nil
			})
			_, err := s.manifest(1, h)
			var control *DeferredControlError
			if !errors.As(err, &control) || control.Code != "deferred_protocol_error" {
				t.Fatalf("terminal %s accepted without exact completion proof: %v", invalid, err)
			}
		})
	}
}

func TestDeferredCallerControlPrefixKeepsSafeMountAndRemovesOnlyTerminalV1(t *testing.T) {
	for base, expected := range map[string]string{
		"https://peer/mount/v1":     "https://peer/mount" + deferred.Prefix,
		"https://peer/mount/v1/":    "https://peer/mount" + deferred.Prefix,
		"https://peer/mount":        "https://peer/mount" + deferred.Prefix,
		"https://peer/v1/mount":     "https://peer/v1/mount" + deferred.Prefix,
		"https://peer/a%20mount/v1": "https://peer/a%20mount" + deferred.Prefix,
		"https://peer/mount/%76%31": "https://peer/mount" + deferred.Prefix,
		"https://peer/":             "https://peer" + deferred.Prefix,
	} {
		prefix, err := deferredControlPrefix(base)
		if err != nil || prefix.String() != expected {
			t.Fatalf("prefix(%s)=%v error=%v want=%s", base, prefix, err, expected)
		}
	}
}
