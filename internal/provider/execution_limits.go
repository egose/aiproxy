package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

type ExecutionLimits struct {
	RequestBytes  int64
	ResponseBytes int64
	SnapshotBytes int64
	SSELineBytes  int
	SSEEventBytes int
	SSEDataLines  int
	JSONDepth     int
	JSONTokens    int
	Work          deferred.ByteBudget
}

type ErrExecutionLimit struct {
	Kind  string
	Limit int64
}

func (e ErrExecutionLimit) Error() string {
	return fmt.Sprintf("deferred %s limit exceeded (%d)", e.Kind, e.Limit)
}

func (e ErrExecutionLimit) Unwrap() error { return deferred.ErrBudgetExceeded }

type executionState struct {
	limits   ExecutionLimits
	mu       sync.Mutex
	refs     int
	releases []func()
}

func newExecutionState(limits ExecutionLimits) (*executionState, error) {
	if limits.RequestBytes < 1 || limits.RequestBytes > deferred.HardRequestBytes ||
		limits.ResponseBytes < 1 || limits.ResponseBytes > maxUpstreamBodyBytes ||
		limits.SnapshotBytes < 0 || limits.SnapshotBytes > deferred.HardRequestBytes || limits.Work == nil {
		return nil, ErrInvalidRequest{Message: "invalid deferred execution byte budget"}
	}
	for _, setting := range []struct {
		value *int
		limit int
	}{
		{&limits.SSELineBytes, deferred.SSELineBytes}, {&limits.SSEEventBytes, deferred.SSEEventBytes},
		{&limits.SSEDataLines, deferred.SSEDataLines}, {&limits.JSONDepth, deferred.JSONDepth},
		{&limits.JSONTokens, deferred.JSONTokens},
	} {
		if *setting.value == 0 {
			*setting.value = setting.limit
		}
		if *setting.value < 1 || *setting.value > setting.limit {
			return nil, ErrInvalidRequest{Message: "invalid deferred execution structural limits"}
		}
	}
	state := &executionState{limits: limits, refs: 1}
	if err := state.hold(deferred.JobMetadataBytes); err != nil {
		return nil, err
	}
	return state, nil
}

func (s *executionState) reserve(n int64) (func(), error) {
	release, err := s.limits.Work.Reserve(n)
	if err != nil {
		return nil, fmt.Errorf("deferred allocation: %w", err)
	}
	return release, nil
}

func (s *executionState) hold(n int64) error {
	release, err := s.reserve(n)
	if err != nil {
		return err
	}
	s.holdRelease(release)
	return nil
}

func (s *executionState) holdRelease(release func()) {
	s.mu.Lock()
	s.releases = append(s.releases, release)
	s.mu.Unlock()
}

func (s *executionState) retain() (func(), bool) {
	s.mu.Lock()
	if s.refs == 0 {
		s.mu.Unlock()
		return nil, false
	}
	s.refs++
	s.mu.Unlock()
	return sync.OnceFunc(s.release), true
}

func (s *executionState) release() {
	s.mu.Lock()
	s.refs--
	if s.refs != 0 {
		s.mu.Unlock()
		return
	}
	releases := s.releases
	s.releases = nil
	s.mu.Unlock()
	for _, release := range releases {
		release()
	}
}

func (a *adapter) doWithExecutionLimits(ctx context.Context, r Request, desc providerDescriptor) (result *Result, err error) {
	if r.Operation != OpChatCompletions {
		return nil, ErrUnsupportedOperation{ProviderType: r.ProviderType, Operation: r.Operation}
	}
	state, err := newExecutionState(*r.ExecutionLimits)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil || result == nil {
			if result != nil {
				if result.StreamBody != nil {
					_ = result.StreamBody.Close()
				}
				if result.OnClose != nil {
					result.OnClose()
				}
			}
			result = nil
			state.release()
			err = boundedExecutionError(err)
			return
		}
		previous := result.OnClose
		result.OnClose = sync.OnceFunc(func() {
			if result.StreamBody != nil {
				_ = result.StreamBody.Close()
			}
			if previous != nil {
				previous()
			}
			result.Body, result.StreamBody, result.Header, result.UpstreamRequestBody, result.UpstreamRequestHeaders = nil, nil, nil, nil, nil
			state.release()
		})
	}()
	r.execution, r.ExecutionLimits = state, &state.limits
	if deferred.ValidatePublicModel(r.PublicModel) != nil {
		return nil, ErrInvalidRequest{Message: "invalid deferred public model"}
	}
	if r.Inbound == nil {
		r.Inbound = &http.Request{Header: make(http.Header)}
	}
	if len(r.APIKey) > 4096-7 || len(r.CopilotToken) > 4096-7 || len(r.UserAgent) > 256 || len(r.Version) > 256 ||
		int64(len(r.UpstreamModel)) > state.limits.RequestBytes || len(r.BaseURL) > deferred.MaxMetadataBytes {
		return nil, ErrExecutionLimit{Kind: "upstream request metadata", Limit: deferred.MaxMetadataBytes}
	}
	if err := deferred.ValidateHeaderBag(r.Inbound.Header); err != nil {
		return nil, ErrExecutionLimit{Kind: "input headers", Limit: deferred.MaxMetadataBytes}
	}
	if r.Body == nil && r.Inbound.Body != nil {
		body, release, err := state.readBody(r.Inbound.Body, state.limits.RequestBytes, r.Inbound.ContentLength, "request bytes")
		if err != nil {
			return nil, err
		}
		state.holdRelease(release)
		r.Body = body
	}
	if int64(len(r.Body)) > state.limits.RequestBytes {
		return nil, ErrExecutionLimit{Kind: "request bytes", Limit: state.limits.RequestBytes}
	}
	shape, _, err := deferred.InspectChatInput(r.Body, state.limits.RequestBytes, state.limits.JSONDepth, state.limits.JSONTokens)
	if err != nil {
		var limit deferred.JSONLimitError
		if errors.As(err, &limit) {
			return nil, ErrExecutionLimit{Kind: "JSON " + limit.Kind, Limit: int64(limit.Limit)}
		}
		return nil, ErrInvalidRequest{Message: err.Error()}
	}
	release, err := state.reserve(jsonDecodeWorkBytes(len(r.Body), shape.Tokens))
	if err != nil {
		return nil, err
	}
	defer release()
	targetWork := max(int64(0), int64(len(r.BaseURL)+len(r.UpstreamModel))*4-8<<10)
	if targetWork > 0 {
		if err := state.hold(targetWork); err != nil {
			return nil, err
		}
	}
	return desc.do(a, ctx, r)
}

func jsonDecodeWorkBytes(n, tokens int) int64 {
	return int64(n)*8 + int64(tokens)*256 + 16<<10
}

func (s *executionState) readBody(reader io.Reader, limit, contentLength int64, kind string) ([]byte, func(), error) {
	if contentLength > limit {
		return nil, nil, ErrExecutionLimit{Kind: kind, Limit: limit}
	}
	capacity := min(limit, int64(32<<10))
	if contentLength > 0 {
		capacity = contentLength
	}
	release, err := s.reserve(capacity)
	if err != nil {
		return nil, nil, err
	}
	data := make([]byte, int(capacity))
	size, emptyReads := 0, 0
	fail := func(err error) ([]byte, func(), error) {
		release()
		return nil, nil, err
	}
	for {
		if size == len(data) {
			var probe [1]byte
			n, err := reader.Read(probe[:])
			if n > 0 {
				if int64(size) == limit {
					return fail(ErrExecutionLimit{Kind: kind, Limit: limit})
				}
				next := min(limit, max(int64(32<<10), int64(len(data))*2))
				nextRelease, reserveErr := s.reserve(next)
				if reserveErr != nil {
					return fail(reserveErr)
				}
				grown := make([]byte, int(next))
				copy(grown, data[:size])
				grown[size] = probe[0]
				data, size = grown, size+1
				release()
				release = nextRelease
			}
			if err == io.EOF {
				return data[:size], release, nil
			}
			if err != nil {
				return fail(err)
			}
			if n == 0 {
				emptyReads++
				if emptyReads >= 100 {
					return fail(io.ErrNoProgress)
				}
			} else {
				emptyReads = 0
			}
			continue
		}
		n, err := reader.Read(data[size:])
		size += n
		if err == io.EOF {
			return data[:size], release, nil
		}
		if err != nil {
			return fail(err)
		}
		if n == 0 {
			emptyReads++
			if emptyReads >= 100 {
				return fail(io.ErrNoProgress)
			}
		} else {
			emptyReads = 0
		}
	}
}

func (s *executionState) marshal(value any, limit int64, retained bool) ([]byte, func(), error) {
	size, err := deferred.JSONEncodedSize(value, limit, s.limits.JSONDepth, s.limits.JSONTokens)
	if err != nil {
		return nil, nil, ErrExecutionLimit{Kind: "encoded JSON bytes or structure", Limit: limit}
	}
	owned := max(int64(64), size*2)
	releaseOwned, err := s.reserve(owned)
	if err != nil {
		return nil, nil, err
	}
	releaseTemp, err := s.reserve(max(int64(512), size*2) + int64(s.limits.JSONTokens)*64 + 16<<10)
	if err != nil {
		releaseOwned()
		return nil, nil, err
	}
	data, err := json.Marshal(value)
	releaseTemp()
	if err != nil || int64(cap(data)) > owned || int64(len(data)) > limit {
		releaseOwned()
		if err == nil {
			err = ErrExecutionLimit{Kind: "encoded JSON capacity", Limit: limit}
		}
		return nil, nil, err
	}
	if retained {
		s.holdRelease(releaseOwned)
		return data, func() {}, nil
	}
	return data, releaseOwned, nil
}

func marshalExecutionJSON(value any, state *executionState, request bool) ([]byte, error) {
	if state == nil {
		return json.Marshal(value)
	}
	limit := state.limits.ResponseBytes
	if request {
		limit = state.limits.RequestBytes
	}
	data, _, err := state.marshal(value, limit, true)
	return data, err
}

func executionArgument(states []*executionState) *executionState {
	if len(states) == 0 {
		return nil
	}
	return states[0]
}

func executionRequestError(r Request, action string, err error) error {
	if r.execution != nil && (errors.Is(err, deferred.ErrBudgetExceeded) || errors.Is(err, deferred.ErrBudgetClosed)) {
		return err
	}
	return ErrInvalidRequest{Message: fmt.Sprintf("%s: %v", action, err)}
}

func (s *executionState) joinStrings(parts []string, separator string, request bool) (string, error) {
	limit := s.limits.ResponseBytes
	if request {
		limit = s.limits.RequestBytes
	}
	var size int64
	for i, part := range parts {
		added := int64(len(part))
		if i > 0 {
			added += int64(len(separator))
		}
		if added > limit-size {
			return "", ErrExecutionLimit{Kind: "joined text bytes", Limit: limit}
		}
		size += added
	}
	return strings.Join(parts, separator), nil
}

func translatedErrorWithLimits(body []byte, state *executionState, anthropic bool) ([]byte, error) {
	if state == nil {
		if anthropic {
			return translateAnthropicError(body), nil
		}
		return translateGeminiError(body), nil
	}
	var envelope struct {
		Error struct {
			Type    string `json:"type"`
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(body, &envelope)
	kind, message := envelope.Error.Type, envelope.Error.Message
	if !anthropic {
		kind = strings.ToLower(envelope.Error.Status)
	}
	if message == "" {
		kind = "upstream_error"
		message = "gemini upstream error"
		if anthropic {
			message = "anthropic upstream error"
		}
	} else if !anthropic && kind == "" {
		kind = "upstream_error"
	}
	out := struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	out.Error.Type, out.Error.Message = kind, message
	return marshalExecutionJSON(out, state, false)
}

func boundedExecutionError(err error) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	if len(message) <= deferred.MaxErrorBytes {
		return err
	}
	var invalid ErrInvalidRequest
	if errors.As(err, &invalid) {
		return ErrInvalidRequest{Message: strings.Clone(message[:deferred.MaxErrorBytes])}
	}
	return fmt.Errorf("deferred upstream error: %s", message[:deferred.MaxErrorBytes-32])
}

func executeUpstreamLimited(r Request, req *http.Request, handlers upstreamResponseHandlers) (*Result, error) {
	s := r.execution
	if r.ForwardAuthorization {
		authorization, err := forwardedAuthorization(r)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", authorization)
	}
	req.Header.Set("User-Agent", upstreamUserAgent(r))
	applyForwardedHeaders(r, req)
	for name := range req.Header {
		if strings.HasPrefix(strings.ToLower(name), "x-aiproxy-deferred-") || strings.EqualFold(name, "X-Aiproxy-Deferred") {
			delete(req.Header, name)
		}
	}
	if req.ContentLength < 0 || req.ContentLength > s.limits.RequestBytes {
		return nil, ErrExecutionLimit{Kind: "upstream request bytes", Limit: s.limits.RequestBytes}
	}
	if err := deferred.ValidateHeaderBag(req.Header); err != nil {
		return nil, ErrExecutionLimit{Kind: "upstream request headers", Limit: deferred.MaxMetadataBytes}
	}
	var sentHeaders http.Header
	var sentBody []byte
	truncated := false
	if s.limits.SnapshotBytes > 0 {
		sentHeaders = req.Header.Clone()
		if req.Body != nil {
			if req.GetBody == nil {
				return nil, ErrInvalidRequest{Message: "deferred request snapshot requires a replayable body"}
			}
			size := min(s.limits.SnapshotBytes, req.ContentLength)
			if err := s.hold(size); err != nil {
				return nil, err
			}
			body, err := req.GetBody()
			if err != nil {
				return nil, err
			}
			sentBody = make([]byte, int(size))
			_, err = io.ReadFull(body, sentBody)
			_ = body.Close()
			if err != nil {
				return nil, fmt.Errorf("read deferred request snapshot: %w", err)
			}
			truncated = size < req.ContentLength
		}
	}
	attachSent := func(result *Result) *Result {
		if result != nil {
			result.UpstreamRequestHeaders, result.UpstreamRequestBody = sentHeaders, sentBody
			result.UpstreamRequestBodyTruncated = truncated
		}
		return result
	}
	if err := s.leaseRequestBody(req); err != nil {
		return nil, err
	}
	resp, err := upstreamhttp.Do(clientFor(r), req)
	if err != nil {
		if req.Body != nil {
			_ = req.Body.Close()
		}
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	observedAt := time.Now()
	delay, hasDelay := ParseRetryCooldown(resp.Header, observedAt)
	if r.ObserveBody != nil {
		resp.Body = r.ObserveBody(resp.Body)
	}
	transferred := false
	defer func() {
		if !transferred {
			_ = resp.Body.Close()
		}
	}()
	headers, err := deferred.FilterResponseHeaders(resp.Header)
	if err != nil {
		return nil, ErrExecutionLimit{Kind: "response headers", Limit: deferred.MaxMetadataBytes}
	}
	resp.Header = headers
	if resp.StatusCode < 400 && handlers.IsStreaming != nil && handlers.IsStreaming(resp) {
		result, err := handlers.OnStream(resp)
		if err == nil && result != nil {
			transferred = true
		}
		return attachCooldownObservation(attachSent(result), delay, hasDelay, observedAt), withCooldownObservationError(err, delay, hasDelay, observedAt)
	}
	body, releaseBody, err := s.readBody(resp.Body, s.limits.ResponseBytes, resp.ContentLength, "response bytes")
	if err != nil {
		return nil, withCooldownObservationError(fmt.Errorf("read upstream body: %w", err), delay, hasDelay, observedAt)
	}
	if handlers.RequireJSON {
		defer releaseBody()
		shape, err := deferred.InspectJSON(body, s.limits.JSONDepth, s.limits.JSONTokens)
		if err != nil {
			var limit deferred.JSONLimitError
			if errors.As(err, &limit) {
				return nil, ErrExecutionLimit{Kind: "JSON " + limit.Kind, Limit: int64(limit.Limit)}
			}
			if resp.StatusCode < 400 {
				return nil, fmt.Errorf("invalid translated upstream JSON: %w", err)
			}
			shape.Tokens = s.limits.JSONTokens
		}
		release, err := s.reserve(jsonDecodeWorkBytes(len(body), shape.Tokens))
		if err != nil {
			return nil, err
		}
		defer release()
	} else {
		s.holdRelease(releaseBody)
	}
	handler := handlers.OnSuccess
	if resp.StatusCode >= 400 {
		handler = handlers.OnError
	}
	var result *Result
	if handler == nil {
		result = &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}
	} else {
		result, err = handler(resp, body)
	}
	return attachCooldownObservation(attachSent(result), delay, hasDelay, observedAt), withCooldownObservationError(err, delay, hasDelay, observedAt)
}

type executionRequestBody struct {
	mu      sync.Mutex
	body    io.ReadCloser
	release func()
}

func (b *executionRequestBody) Read(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.body == nil {
		return 0, io.ErrClosedPipe
	}
	return b.body.Read(p)
}

func (b *executionRequestBody) Close() error {
	b.mu.Lock()
	body, release := b.body, b.release
	b.body, b.release = nil, nil
	b.mu.Unlock()
	if body == nil {
		return nil
	}
	err := body.Close()
	release()
	return err
}

func (s *executionState) leaseRequestBody(req *http.Request) error {
	if req.Body == nil {
		return nil
	}
	wrap := func(body io.ReadCloser) (io.ReadCloser, error) {
		release, ok := s.retain()
		if !ok {
			_ = body.Close()
			return nil, io.ErrClosedPipe
		}
		return &executionRequestBody{body: body, release: release}, nil
	}
	body, err := wrap(req.Body)
	if err != nil {
		return err
	}
	req.Body = body
	if getBody := req.GetBody; getBody != nil {
		req.GetBody = func() (io.ReadCloser, error) {
			body, err := getBody()
			if err != nil {
				return nil, err
			}
			return wrap(body)
		}
	}
	s.holdRelease(func() { req.GetBody = nil })
	return nil
}

func usageFromBodyWithLimits(body []byte, state *executionState) Usage {
	if state == nil {
		return usageFromBody(body)
	}
	shape, err := deferred.InspectJSON(body, state.limits.JSONDepth, state.limits.JSONTokens)
	if err != nil {
		return Usage{}
	}
	release, err := state.reserve(jsonDecodeWorkBytes(len(body), shape.Tokens))
	if err != nil {
		return Usage{}
	}
	defer release()
	return usageFromBody(body)
}
