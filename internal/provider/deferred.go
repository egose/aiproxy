package provider

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"hash"
	"io"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

type DeferredControlError struct {
	Code       string
	Acceptance deferred.Acceptance
	Err        error
}

func (e *DeferredControlError) Error() string {
	if e == nil {
		return "deferred control failed"
	}
	if e.Err == nil {
		return e.Code
	}
	return e.Code + ": " + e.Err.Error()
}

func (e *DeferredControlError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsPeerControlOutcome(result *Result, err error) bool {
	if result != nil && result.Origin == OriginPeerControl {
		return true
	}
	if err == nil {
		return false
	}
	var control *DeferredControlError
	return errors.As(err, &control)
}

type deferredCaller struct {
	ctx           context.Context
	requestCtx    context.Context
	cancel        context.CancelFunc
	options       config.DeferredTransport
	client        http.Client
	prefix        *url.URL
	authorization string
	userAgent     string
	headers       http.Header
	body          []byte
	capability    deferred.Capability
	submit        deferred.SubmitRequest
	fingerprint   string
	scopeDigest   string
	dedupUntil    int64
	receipt       deferred.Receipt
	acceptance    deferred.Acceptance
	info          deferred.JobInfo
	head          *deferred.ResponseHead
	retryObserved time.Time
	retryDeadline time.Time
	calls         int
	retries       int
	cleanupOnce   sync.Once
}

type deferredReply struct {
	status  int
	header  http.Header
	body    []byte
	started time.Time
}

type deferredAdmissionError struct {
	status int
	detail deferred.ErrorDetail
}

func (e *deferredAdmissionError) Error() string { return "peer rejected inference admission" }

func (a *adapter) doDeferred(ctx context.Context, r Request, inference *http.Request, body []byte) (*Result, error) {
	options := *r.DeferredTransport
	waitCtx, cancel := context.WithTimeout(ctx, options.MaxWait)
	s := &deferredCaller{
		ctx: waitCtx, requestCtx: ctx, cancel: cancel, options: options, client: *clientFor(r),
		authorization: "Bearer " + r.APIKey, userAgent: "aiproxy/" + defaultVersion(r.Version),
		acceptance: deferred.AcceptanceNotAccepted,
	}
	s.client.Jar = nil
	fail := func(err error) (*Result, error) {
		s.finish()
		result := s.result(OriginPeerControl, nil)
		return result, err
	}
	if !validDeferredCallerOptions(options) || r.APIKey == "" {
		return fail(s.failure("deferred_protocol_error", errors.New("invalid deferred caller configuration")))
	}
	prefix, err := deferredControlPrefix(r.BaseURL)
	if err != nil {
		return fail(s.failure("deferred_protocol_error", err))
	}
	s.prefix = prefix
	if int64(len(body)) > deferred.HardRequestBytes {
		return fail(s.failure("deferred_limit_exceeded", errors.New("deferred inference input exceeds limit")))
	}
	model, err := deferred.ChatInputModel(body)
	if err != nil || model != r.UpstreamModel {
		cancel()
		return nil, ErrInvalidRequest{Message: "invalid deferred chat input"}
	}
	inference.Header.Set("User-Agent", upstreamUserAgent(r))
	applyForwardedHeaders(r, inference)
	headers, err := deferredInferenceHeaders(inference.Header, r.Inbound)
	if err != nil {
		return fail(s.failure("deferred_protocol_error", errors.New("invalid deferred inference metadata")))
	}
	s.headers, s.body = headers, body
	query := url.Values{"model": {model}}
	reply, err := s.call("capabilities", http.MethodGet, "/capabilities", query, nil)
	if err != nil {
		return fail(err)
	}
	if reply.status != http.StatusOK || deferred.DecodeControl(reply.body, &s.capability) != nil || s.capability.PublicModel != model {
		return fail(s.failure("deferred_protocol_error", errors.New("invalid deferred capability")))
	}
	protection := s.capability.Limits.ExecutionTimeoutMS + s.capability.Limits.RetentionMS + s.capability.Limits.FinalizationTimeoutMS
	if s.capability.SubmitUntilMS > math.MaxInt64-protection {
		return fail(s.failure("deferred_protocol_error", errors.New("deferred capability lifetime overflows")))
	}
	s.dedupUntil = s.capability.SubmitUntilMS + protection
	if int64(len(body)) > s.capability.Limits.RequestBytes {
		return fail(s.failure("deferred_limit_exceeded", errors.New("deferred inference input exceeds peer limit")))
	}
	key, err := deferred.NewAttemptKey()
	if err != nil {
		return fail(s.failure("deferred_control_error", errors.New("cannot create deferred attempt")))
	}
	s.submit = deferred.SubmitRequest{
		Envelope: deferred.NewEnvelope(), Generation: s.capability.Generation, SubmitToken: s.capability.SubmitToken,
		AttemptKey: key, Headers: headers, Body: body,
	}
	s.fingerprint, err = deferred.Fingerprint(model, body, headers)
	if err != nil {
		return fail(s.failure("deferred_protocol_error", errors.New("invalid deferred inference fingerprint")))
	}
	digest := sha256.Sum256([]byte(s.capability.SubmitToken))
	s.scopeDigest = hex.EncodeToString(digest[:])
	metadata := s.submit
	metadata.Body = []byte{}
	if _, err := deferred.JSONEncodedSize(metadata, deferred.MaxMetadataBytes, deferred.ControlJSONDepth, deferred.ControlJSONTokens); err != nil {
		return fail(s.failure("deferred_limit_exceeded", errors.New("deferred submit metadata exceeds limit")))
	}
	wireLimit, _ := deferred.SubmitWireBytes(s.capability.Limits.RequestBytes)
	if _, err := deferred.JSONEncodedSize(s.submit, wireLimit, deferred.ControlJSONDepth, deferred.ControlJSONTokens); err != nil {
		return fail(s.failure("deferred_limit_exceeded", errors.New("deferred submit metadata exceeds limit")))
	}
	wire, err := json.Marshal(s.submit)
	if err != nil {
		return fail(s.failure("deferred_protocol_error", errors.New("cannot encode deferred submit")))
	}
	reply, err = s.call("submit", http.MethodPost, "/submit", nil, wire)
	if err != nil {
		var admission *deferredAdmissionError
		if errors.As(err, &admission) {
			s.cancel()
			result := s.result(OriginPeerControl, nil)
			result.StatusCode = admission.status
			result.Header = http.Header{"Content-Type": {"application/json"}}
			result.Body = deferredOrdinaryError(admission.detail)
			result.OnClose = s.finish
			return result, nil
		}
		return fail(err)
	}
	var receipt deferred.Receipt
	if reply.status != http.StatusAccepted || deferred.DecodeControl(reply.body, &receipt) != nil ||
		receipt.Generation != s.capability.Generation || receipt.PublicModel != model ||
		receipt.AttemptKey != key || receipt.ScopeDigest != s.scopeDigest || receipt.Fingerprint != s.fingerprint ||
		receipt.DedupUntilMS != s.dedupUntil || receipt.ExecutionDeadlineMS > s.capability.SubmitUntilMS+s.capability.Limits.ExecutionTimeoutMS {
		s.promote(deferred.AcceptanceIndeterminate)
		return fail(s.failure("deferred_protocol_error", errors.New("invalid deferred receipt")))
	}
	s.receipt = receipt
	s.promote(deferred.AcceptanceAccepted)
	if !s.validLocation(reply.header.Values("Location")) {
		return fail(s.failure("deferred_protocol_error", errors.New("invalid deferred job location")))
	}
	s.submit, wire = deferred.SubmitRequest{}, nil
	stopCancellation := context.AfterFunc(s.ctx, s.finish)
	reader := &deferredReadCloser{caller: s, hasher: sha256.New(), stream: NewStreamCompletion()}
	var unary []byte
	var cursor uint64
	for {
		frame, err := s.read(cursor)
		if err != nil {
			return fail(err)
		}
		if s.head != nil && s.head.Streaming && (!frame.State.Terminal() || frame.Outcome.Kind == deferred.OutcomeResponse) {
			if frame.Done {
				_, _ = reader.hasher.Write(frame.Data)
				manifest, err := s.manifest(frame.NextCursor, reader.hasher)
				if err != nil {
					return fail(err)
				}
				if err := s.inferenceOutcome(manifest.Outcome); err != nil {
					return fail(s.failure("deferred_protocol_error", errors.New("deferred terminal outcome changed")))
				}
				reader.verified = true
			}
			reader.setFrame(frame)
			result := s.result(OriginInference, s.head)
			reader.stream.SetUsage(result.Usage)
			result.Streaming, result.StreamBody, result.Stream = true, reader, reader.stream
			result.AbortOnStreamError = true
			result.OnClose = func() { _ = reader.Close() }
			return result, nil
		}
		_, _ = reader.hasher.Write(frame.Data)
		cursor = frame.NextCursor
		if s.head == nil || !s.head.Streaming {
			unary = appendDeferredBytes(unary, frame.Data, int(s.capability.Limits.ResultBytes))
		}
		if frame.Done {
			manifest, err := s.manifest(cursor, reader.hasher)
			if err != nil {
				return fail(err)
			}
			if err := s.inferenceOutcome(manifest.Outcome); err != nil {
				if IsPeerControlOutcome(nil, err) {
					return fail(err)
				}
				s.finish()
				result := s.result(OriginInference, s.head)
				result.Body = unary
				return result, s.withRetryError(err)
			}
			result := s.result(OriginInference, s.head)
			result.Body = unary
			stopCancellation()
			s.cancel()
			result.OnClose = s.finish
			return result, nil
		}
		if len(frame.Data) == 0 {
			if err := s.pause(s.pollDelay(frame.PollAfterMS)); err != nil {
				return fail(err)
			}
		}
	}
}

func validDeferredCallerOptions(o config.DeferredTransport) bool {
	return o.ControlTimeout >= 100*time.Millisecond && o.ControlTimeout <= 30*time.Second &&
		o.MaxWait >= time.Second && o.MaxWait <= 2*time.Hour && o.PollInterval >= 50*time.Millisecond && o.PollInterval <= 5*time.Second &&
		o.MaxControlCalls >= 4 && o.MaxControlCalls <= 65536 && o.MaxControlRetries >= 0 && o.MaxControlRetries <= 32
}

func deferredControlPrefix(base string) (*url.URL, error) {
	u, err := url.Parse(base)
	if err != nil || u.Scheme != "http" && u.Scheme != "https" || u.Host == "" || u.User != nil || u.Opaque != "" ||
		u.RawQuery != "" || u.ForceQuery || strings.Contains(base, "#") {
		return nil, errors.New("invalid deferred peer base URL")
	}
	path := u.EscapedPath()
	for {
		lower := strings.ToLower(path)
		if strings.Contains(path, `\`) || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") {
			return nil, errors.New("invalid deferred peer base path")
		}
		for segment := range strings.SplitSeq(path, "/") {
			if segment == "." || segment == ".." {
				return nil, errors.New("invalid deferred peer base path")
			}
		}
		next, err := url.PathUnescape(path)
		if err != nil {
			return nil, errors.New("invalid deferred peer base path")
		}
		if next == path {
			break
		}
		path = next
	}
	escaped := strings.TrimRight(u.EscapedPath(), "/")
	if slash := strings.LastIndex(escaped, "/"); slash >= 0 {
		segment, _ := url.PathUnescape(escaped[slash+1:])
		if segment == "v1" {
			escaped = escaped[:slash]
		}
	}
	escaped += deferred.Prefix
	u.Path, err = url.PathUnescape(escaped)
	if err != nil {
		return nil, errors.New("invalid deferred peer base path")
	}
	u.RawPath = escaped
	return u, nil
}

func deferredInferenceHeaders(headers http.Header, inbound *http.Request) (http.Header, error) {
	out := make(http.Header)
	for name, values := range headers {
		lower := strings.ToLower(name)
		switch lower {
		case "authorization", "cookie", "set-cookie", "x-api-key", "x-goog-api-key", "host",
			"connection", "keep-alive", "proxy-authenticate", "proxy-authorization", "te", "trailer", "transfer-encoding", "upgrade", "content-length":
			continue
		}
		if config.IsDeferredHeader(name) {
			continue
		}
		nominated := false
		if inbound != nil {
			for _, value := range inbound.Header.Values("Connection") {
				for token := range strings.SplitSeq(value, ",") {
					nominated = nominated || strings.EqualFold(strings.TrimSpace(token), name)
				}
			}
		}
		if !nominated {
			out[name] = values
		}
	}
	return deferred.CanonicalSubmitHeaders(out)
}

func (s *deferredCaller) controlURL(suffix string, query url.Values) *url.URL {
	u := *s.prefix
	u.Path += suffix
	if u.RawPath != "" {
		u.RawPath += suffix
	}
	u.RawQuery = query.Encode()
	return &u
}

func (s *deferredCaller) validLocation(values []string) bool {
	if len(values) != 1 {
		return false
	}
	expected := s.controlURL("/jobs/"+s.receipt.ID, nil)
	return values[0] == expected.String() || values[0] == expected.EscapedPath()
}

func (s *deferredCaller) timeout() time.Duration {
	timeout := s.options.ControlTimeout
	if s.capability.Limits.ControlTimeoutMS > 0 {
		timeout = min(timeout, time.Duration(s.capability.Limits.ControlTimeoutMS)*time.Millisecond)
	}
	return timeout
}

func (s *deferredCaller) once(ctx context.Context, method, suffix string, query url.Values, body []byte) (deferredReply, error) {
	started := time.Now()
	callCtx, cancel := context.WithTimeout(upstreamhttp.ControlContext(ctx), s.timeout())
	defer cancel()
	var input io.Reader
	if body != nil {
		input = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(callCtx, method, s.controlURL(suffix, query).String(), input)
	if err != nil {
		return deferredReply{}, err
	}
	req.GetBody = nil
	req.Header.Set("Authorization", s.authorization)
	req.Header.Set("User-Agent", s.userAgent)
	req.Header.Set("Accept", deferred.MediaType)
	req.Header.Set("Accept-Encoding", "identity")
	req.Header.Set("X-Aiproxy-Deferred-Version", "1")
	if strings.HasPrefix(suffix, "/jobs/") {
		req.Header.Set("X-Aiproxy-Deferred-Generation", s.receipt.Generation)
	}
	if body != nil {
		req.Header.Set("Content-Type", deferred.MediaType)
	}
	resp, err := upstreamhttp.Do(&s.client, req)
	if err != nil {
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		return deferredReply{}, err
	}
	defer resp.Body.Close()
	if resp.ContentLength > deferred.MaxControlBodyBytes {
		return deferredReply{}, s.failure("deferred_protocol_error", errors.New("deferred control body exceeds limit"))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, deferred.MaxControlBodyBytes+1))
	if err != nil {
		return deferredReply{}, err
	}
	if callCtx.Err() != nil {
		return deferredReply{}, callCtx.Err()
	}
	if len(data) > deferred.MaxControlBodyBytes {
		return deferredReply{}, s.failure("deferred_protocol_error", errors.New("deferred control body exceeds limit"))
	}
	return deferredReply{status: resp.StatusCode, header: resp.Header, body: data, started: started}, nil
}

func recognizedDeferredReply(header http.Header) bool {
	return len(header.Values("Content-Type")) == 1 && header.Get("Content-Type") == deferred.MediaType &&
		len(header.Values("X-Aiproxy-Deferred-Version")) == 1 && header.Get("X-Aiproxy-Deferred-Version") == "1"
}

func (s *deferredCaller) call(route, method, suffix string, query url.Values, body []byte) (deferredReply, error) {
	for {
		if err := s.ctx.Err(); err != nil {
			return deferredReply{}, s.contextFailure(err)
		}
		if s.calls >= s.options.MaxControlCalls {
			return deferredReply{}, s.recoveryFailure(errors.New("deferred control call budget exhausted"))
		}
		s.calls++
		reply, err := s.once(s.ctx, method, suffix, query, body)
		advice := int64(0)
		if err != nil {
			if route == "submit" {
				s.promote(deferred.AcceptanceIndeterminate)
			}
			if errors.Is(err, upstreamhttp.ErrControlRedirect) || IsPeerControlOutcome(nil, err) {
				return deferredReply{}, s.failure("deferred_protocol_error", errors.New("invalid deferred control transfer"))
			}
		} else {
			if !recognizedDeferredReply(reply.header) {
				code := "deferred_protocol_error"
				if route == "submit" {
					s.promote(deferred.AcceptanceIndeterminate)
					code = "deferred_submit_indeterminate"
				}
				return deferredReply{}, s.failure(code, errors.New("unrecognized deferred peer response"))
			}
			if reply.status < 400 {
				return reply, nil
			}
			var failure deferred.ControlError
			if deferred.DecodeControl(reply.body, &failure) != nil {
				if route == "submit" {
					if detail, ok := decodeDeferredAdmission(reply.body); ok {
						return deferredReply{}, &deferredAdmissionError{status: reply.status, detail: detail}
					}
					s.promote(deferred.AcceptanceIndeterminate)
				}
				return deferredReply{}, s.failure("deferred_protocol_error", errors.New("invalid deferred control error"))
			}
			s.promote(failure.Acceptance)
			if reply.status != 429 && reply.status != 503 {
				code := "deferred_control_error"
				if failure.Error.Type == "unsupported_deferred_version" {
					code = "deferred_protocol_error"
				} else if reply.status == 404 || reply.status == 410 {
					if s.receipt.ID == "" && s.acceptance != deferred.AcceptanceNotAccepted {
						code = "deferred_submit_indeterminate"
					} else if s.acceptance == deferred.AcceptanceAccepted {
						code = "deferred_request_lost"
					} else if s.acceptance == deferred.AcceptanceIndeterminate {
						code = "deferred_submit_indeterminate"
					}
				}
				return deferredReply{}, s.failure(code, errors.New("peer rejected deferred control"))
			}
			advice = failure.PollAfterMS
			if advice == 0 {
				advice = deferredControlRetryAdvice(reply.header, reply.started)
			}
			err = errors.New("peer deferred control pressure")
		}
		if err := s.ctx.Err(); err != nil {
			return deferredReply{}, s.contextFailure(err)
		}
		if s.retries >= s.options.MaxControlRetries || s.calls >= s.options.MaxControlCalls {
			return deferredReply{}, s.recoveryFailure(err)
		}
		backoff := min(2*time.Second, 100*time.Millisecond*time.Duration(1<<min(s.retries, 5)))
		s.retries++
		delay := time.Duration(rand.Int64N(int64(backoff) + 1))
		if advice > 0 {
			delay = max(delay, time.Duration(max(int64(50), min(int64(2000), advice)))*time.Millisecond)
		}
		if err := s.pause(delay); err != nil {
			return deferredReply{}, err
		}
	}
}

func deferredControlRetryAdvice(header http.Header, started time.Time) int64 {
	for _, value := range header.Values("Retry-After") {
		trimmed := strings.TrimSpace(value)
		if n, err := strconv.ParseUint(trimmed, 10, 64); err == nil {
			if n > 0 {
				return int64(min(n, uint64(2))) * 1000
			}
		} else if date, err := http.ParseTime(trimmed); err == nil && date.After(started) {
			return min(int64(2000), max(int64(50), date.Sub(started).Milliseconds()))
		}
	}
	return 0
}

func decodeDeferredAdmission(body []byte) (deferred.ErrorDetail, bool) {
	var ordinary struct {
		Error *deferred.ErrorDetail `json:"error"`
	}
	if len(body) > deferred.MaxMetadataBytes || deferred.ValidateJSON(body, deferred.ControlJSONDepth, deferred.ControlJSONTokens) != nil {
		return deferred.ErrorDetail{}, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&ordinary) != nil || ordinary.Error == nil || deferred.ValidateErrorDetail(*ordinary.Error) != nil {
		return deferred.ErrorDetail{}, false
	}
	return *ordinary.Error, true
}

func deferredOrdinaryError(detail deferred.ErrorDetail) []byte {
	body, _ := json.Marshal(struct {
		Error deferred.ErrorDetail `json:"error"`
	}{detail})
	return append(body, '\n')
}

func (s *deferredCaller) promote(acceptance deferred.Acceptance) {
	if s.acceptance == deferred.AcceptanceAccepted {
		return
	}
	if acceptance == deferred.AcceptanceAccepted || acceptance == deferred.AcceptanceIndeterminate && s.acceptance != deferred.AcceptanceAccepted {
		s.acceptance = acceptance
	}
}

func (s *deferredCaller) failure(code string, err error) error {
	return &DeferredControlError{Code: code, Acceptance: s.acceptance, Err: err}
}

func (s *deferredCaller) contextFailure(err error) error {
	code := "deferred_control_error"
	if errors.Is(err, context.DeadlineExceeded) {
		code = "deferred_wait_timeout"
	}
	return s.failure(code, err)
}

func (s *deferredCaller) recoveryFailure(err error) error {
	code := "deferred_control_error"
	if s.receipt.ID == "" && s.acceptance != deferred.AcceptanceNotAccepted {
		code = "deferred_submit_indeterminate"
	}
	return s.failure(code, err)
}

func (s *deferredCaller) pause(delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return s.contextFailure(s.ctx.Err())
	case <-timer.C:
		return nil
	}
}

func (s *deferredCaller) pollDelay(advice int64) time.Duration {
	return max(s.options.PollInterval, time.Duration(max(int64(50), min(int64(2000), advice)))*time.Millisecond)
}

func (s *deferredCaller) read(cursor uint64) (deferred.ReadFrame, error) {
	query := url.Values{"cursor": {strconv.FormatUint(cursor, 10)}, "limit": {strconv.Itoa(deferred.MaxReadBytes)}}
	reply, err := s.call("read", http.MethodGet, "/jobs/"+s.receipt.ID+"/read", query, nil)
	if err != nil {
		return deferred.ReadFrame{}, err
	}
	var frame deferred.ReadFrame
	if reply.status != 200 || deferred.DecodeControl(reply.body, &frame) != nil || frame.Cursor != cursor {
		return deferred.ReadFrame{}, s.failure("deferred_protocol_error", errors.New("invalid deferred read frame"))
	}
	if err := s.observeInfo(frame.JobInfo, reply.started); err != nil {
		return deferred.ReadFrame{}, err
	}
	return frame, nil
}

func (s *deferredCaller) observeInfo(info deferred.JobInfo, started time.Time) error {
	expected, got := s.receipt, info.Receipt
	got.State, got.PollAfterMS, got.Replayed = expected.State, expected.PollAfterMS, expected.Replayed
	if got != expected || info.AvailableBytes > uint64(s.capability.Limits.ResultBytes) ||
		info.AvailableBytes < s.info.AvailableBytes || info.State == deferred.StateRunning && (info.Response != nil || info.AvailableBytes != 0) ||
		info.AvailableBytes > 0 && info.Response == nil || s.info.State == deferred.StateHeadersReady && info.State == deferred.StateRunning ||
		s.info.State.Terminal() && (info.State != s.info.State || info.AvailableBytes != s.info.AvailableBytes ||
			info.RetainUntilMS != s.info.RetainUntilMS || info.Outcome.Kind != s.info.Outcome.Kind || !reflect.DeepEqual(info.Outcome.Error, s.info.Outcome.Error)) {
		return s.failure("deferred_protocol_error", errors.New("deferred job identity or state changed"))
	}
	var head *deferred.ResponseHead
	if info.Response != nil {
		filtered, err := deferred.FilterResponseHeaders(info.Response.Headers)
		if err != nil || len(filtered) != len(info.Response.Headers) {
			return s.failure("deferred_protocol_error", errors.New("invalid deferred inference head"))
		}
		copy := *info.Response
		copy.Headers = filtered
		head = &copy
		if s.head != nil && (head.StatusCode != s.head.StatusCode || head.Streaming != s.head.Streaming ||
			!reflect.DeepEqual(head.Headers, s.head.Headers) || !sameDeferredRetry(head.Retry, s.head.Retry)) {
			return s.failure("deferred_protocol_error", errors.New("deferred inference head changed"))
		}
		if head.Retry != nil && head.Retry.RemainingMS > math.MaxInt64/int64(time.Millisecond) {
			return s.failure("deferred_protocol_error", errors.New("deferred retry advice overflows"))
		}
	} else if s.head != nil {
		return s.failure("deferred_protocol_error", errors.New("deferred inference head disappeared"))
	}
	if head != nil && head.Retry != nil {
		observed := time.UnixMilli(head.Retry.ObservedAtMS)
		deadline := started.Add(time.Duration(head.Retry.RemainingMS) * time.Millisecond)
		if s.retryObserved.IsZero() || observed.Before(s.retryObserved) {
			s.retryObserved = observed
		}
		if s.retryDeadline.IsZero() || deadline.Before(s.retryDeadline) {
			s.retryDeadline = deadline
		}
	}
	s.head, s.info = head, info
	return nil
}

func sameDeferredRetry(a, b *deferred.RetryAdvice) bool {
	return a == nil && b == nil || a != nil && b != nil && a.ObservedAtMS == b.ObservedAtMS && a.DeadlineMS == b.DeadlineMS
}

func (s *deferredCaller) manifest(cursor uint64, hasher hash.Hash) (deferred.ResultManifest, error) {
	reply, err := s.call("result", http.MethodGet, "/jobs/"+s.receipt.ID+"/result", nil, nil)
	if err != nil {
		return deferred.ResultManifest{}, err
	}
	var manifest deferred.ResultManifest
	if reply.status != 200 || deferred.DecodeControl(reply.body, &manifest) != nil || manifest.AvailableBytes != cursor ||
		manifest.BodySHA256 != hex.EncodeToString(hasher.Sum(nil)) {
		return deferred.ResultManifest{}, s.failure("deferred_protocol_error", errors.New("invalid deferred terminal manifest"))
	}
	if err := s.observeInfo(manifest.JobInfo, reply.started); err != nil {
		return deferred.ResultManifest{}, err
	}
	return manifest, nil
}

func (s *deferredCaller) inferenceOutcome(outcome *deferred.Outcome) error {
	switch outcome.Kind {
	case deferred.OutcomeResponse:
		return nil
	case deferred.OutcomeUpstreamError:
		return errors.New("upstream request failed")
	case deferred.OutcomeInvalidRequest:
		if outcome.Error != nil && outcome.Error.Type == "unsupported_operation" {
			return ErrUnsupportedOperation{ProviderType: config.ProviderTypeOpenAICompatible, Operation: OpChatCompletions}
		}
		return ErrInvalidRequest{Message: "upstream adapter rejected the request"}
	case deferred.OutcomeExecutionTimeout:
		return s.failure("deferred_execution_timeout", errors.New("deferred execution deadline exceeded"))
	case deferred.OutcomeLimitExceeded:
		return s.failure("deferred_limit_exceeded", errors.New("deferred execution allowance exceeded"))
	default:
		return s.failure("deferred_cancelled", errors.New("deferred execution cancelled"))
	}
}

func (s *deferredCaller) withRetryError(err error) error {
	if delay := time.Until(s.retryDeadline); !s.retryDeadline.IsZero() && delay > 0 {
		return &CooldownError{Err: err, Delay: delay, OK: true, Deadline: s.retryDeadline}
	}
	return err
}

func (s *deferredCaller) result(origin ResultOrigin, head *deferred.ResponseHead) *Result {
	result := &Result{
		Origin: origin, Header: http.Header{"Content-Type": {"application/json"}},
		UpstreamRequestHeaders: s.headers, UpstreamRequestBody: s.body,
	}
	if head != nil {
		result.StatusCode, result.Header = head.StatusCode, head.Headers
	}
	if s.info.Outcome != nil {
		u := s.info.Outcome.Usage
		result.Usage = Usage{PromptTokens: u.PromptTokens, CompletionTokens: u.CompletionTokens, TotalTokens: u.TotalTokens,
			CachedTokens: u.CachedTokens, CacheCreationTokens: u.CacheCreationTokens, CacheReadTokens: u.CacheReadTokens}
	}
	if !s.retryDeadline.IsZero() && origin == OriginInference {
		result.HasRetryDelay = true
		result.RetryObservedAt, result.RetryDeadline = s.retryObserved, s.retryDeadline
		result.RetryDelay = max(time.Duration(0), time.Until(s.retryDeadline))
	}
	return result
}

func (s *deferredCaller) finish() {
	s.cancel()
	s.cleanupOnce.Do(func() {
		if s.receipt.ID == "" || s.prefix == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), s.timeout())
		defer cancel()
		_, _ = s.once(ctx, http.MethodDelete, "/jobs/"+s.receipt.ID, nil, nil)
	})
}

func appendDeferredBytes(body, data []byte, ceiling int) []byte {
	length := len(body) + len(data)
	if length > cap(body) {
		capacity := min(ceiling, max(length, max(1024, cap(body)*2)))
		next := make([]byte, len(body), capacity)
		copy(next, body)
		body = next
	}
	return append(body, data...)
}

type deferredReadCloser struct {
	caller    *deferredCaller
	mu        sync.Mutex
	closed    atomic.Bool
	closeOnce sync.Once
	stream    *StreamCompletion
	hasher    hash.Hash
	cursor    uint64
	data      []byte
	offset    int
	next      uint64
	done      bool
	verified  bool
	finalErr  error
	finished  bool
}

func (r *deferredReadCloser) setFrame(frame deferred.ReadFrame) {
	r.data, r.offset, r.next, r.done = frame.Data, 0, frame.NextCursor, frame.Done
}

func (r *deferredReadCloser) Read(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if r.closed.Load() {
			return 0, io.ErrClosedPipe
		}
		if r.offset < len(r.data) {
			n := copy(p, r.data[r.offset:])
			r.offset += n
			return n, nil
		}
		if r.data != nil {
			if !r.verified {
				_, _ = r.hasher.Write(r.data)
			}
			r.cursor, r.data = r.next, nil
		}
		if r.done || r.finished {
			r.finished = true
			r.stream.Complete(r.finalErr, r.caller.requestCtx.Err() != nil)
			if r.finalErr != nil {
				return 0, r.finalErr
			}
			return 0, io.EOF
		}
		frame, err := r.caller.read(r.cursor)
		if err == nil && frame.Done {
			_, _ = r.hasher.Write(frame.Data)
			var manifest deferred.ResultManifest
			manifest, err = r.caller.manifest(frame.NextCursor, r.hasher)
			if err == nil {
				r.verified = true
				r.stream.SetUsage(r.caller.result(OriginInference, r.caller.head).Usage)
				r.finalErr = r.caller.inferenceOutcome(manifest.Outcome)
			}
		}
		if err != nil {
			r.finished, r.finalErr = true, err
			if r.caller.info.Outcome != nil {
				r.stream.SetUsage(r.caller.result(OriginInference, r.caller.head).Usage)
			}
			r.stream.Complete(err, r.caller.requestCtx.Err() != nil)
			r.caller.finish()
			return 0, err
		}
		r.setFrame(frame)
		if len(frame.Data) == 0 && !frame.Done {
			if err := r.caller.pause(r.caller.pollDelay(frame.PollAfterMS)); err != nil {
				r.finished, r.finalErr = true, err
				r.stream.Complete(err, r.caller.requestCtx.Err() != nil)
				r.caller.finish()
				return 0, err
			}
		}
	}
}

func (r *deferredReadCloser) Close() error {
	r.closeOnce.Do(func() {
		r.closed.Store(true)
		r.stream.Complete(io.ErrClosedPipe, true)
		r.caller.finish()
	})
	return nil
}
