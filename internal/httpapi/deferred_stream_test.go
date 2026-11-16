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
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func setDeferredUpstream(f *deferredHTTPFixture, baseURL string) {
	p := f.rt.Catalog.Providers()[0]
	p.BaseURL = baseURL + "/v1"
	f.rt.Catalog = config.NewCatalog([]config.Provider{p}, nil, nil)
	deps := f.h.current()
	deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
	f.h.UpdateDependencies(deps)
}

func TestDeferredUnaryInitialAndHeaderOnlySilenceRetryObservationAndExactBytes(t *testing.T) {
	beforeHeaders, afterHeaders := make(chan struct{}), make(chan struct{})
	releaseHeaders := sync.OnceFunc(func() { close(beforeHeaders) })
	releaseBody := sync.OnceFunc(func() { close(afterHeaders) })
	started, headersAt := make(chan struct{}), make(chan time.Time, 1)
	var calls atomic.Int32
	body := []byte(" {\n \"choices\":[], \"vendor\":{\"preserved\":true}, \"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		input, _ := io.ReadAll(r.Body)
		if extractModel(input) != "actual" || r.Header.Get("Authorization") != "Bearer model-secret" {
			w.WriteHeader(400)
			return
		}
		close(started)
		select {
		case <-beforeHeaders:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Retry-After-Ms", "1500")
		w.Header().Set("Connection", "X-Hop")
		w.Header().Set("X-Hop", "not-retained")
		w.Header().Set("X-Aiproxy-Deferred-Private", "not-retained")
		headersAt <- time.Now()
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		select {
		case <-afterHeaders:
		case <-r.Context().Done():
			return
		}
		_, _ = w.Write(body)
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(releaseBody)
	t.Cleanup(releaseHeaders)
	adapter := provider.New()
	observed := make(chan struct{}, 1)
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(ctx context.Context, request provider.Request) (*provider.Result, error) {
		track := request.ObserveBody
		request.ObserveBody = func(body io.ReadCloser) io.ReadCloser {
			observed <- struct{}{}
			return track(body)
		}
		return adapter.Do(ctx, request)
	}))
	setDeferredUpstream(f, upstream.URL)
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","stream":false,"messages":[],"vendor":17}`)))
	<-started
	begin := time.Now()
	frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if time.Since(begin) >= f.store.Options().ControlTimeout || len(frame.Data) != 0 || frame.Done || frame.Response != nil {
		t.Fatal("pending unary read waited for generation or fabricated a response")
	}
	releaseHeaders()
	headTime := <-headersAt
	select {
	case <-observed:
	case <-time.After(time.Second):
		t.Fatal("upstream headers were not observed before the silent body wait")
	}
	w := f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
	var info deferred.JobInfo
	if w.Code != 200 || deferred.DecodeControl(w.Body.Bytes(), &info) != nil || info.Response != nil || info.State != deferred.StateRunning {
		t.Fatal("unary body buffering published a response head before its adapter result")
	}
	time.Sleep(80 * time.Millisecond)
	bodyReleased := time.Now()
	releaseBody()
	manifest := f.finish(t, receipt)
	frame = f.read(t, receipt, 0, deferred.MaxReadBytes)
	if !bytes.Equal(frame.Data, body) {
		t.Fatalf("unary inference bytes changed: got %q, want %q", frame.Data, body)
	}
	if !frame.Done || manifest.State != deferred.StateCompleted || manifest.Response == nil || manifest.Response.Streaming {
		t.Fatalf("unary completion changed: done=%t state=%s response=%+v", frame.Done, manifest.State, manifest.Response)
	}
	if manifest.Outcome == nil || manifest.Outcome.Usage.TotalTokens != 5 {
		t.Fatalf("unary usage changed: outcome=%+v", manifest.Outcome)
	}
	if manifest.Response.Headers.Get("X-Hop") != "" || manifest.Response.Headers.Get("X-Aiproxy-Deferred-Private") != "" {
		t.Fatalf("unary retained forbidden response headers: %+v", manifest.Response.Headers)
	}
	retry := manifest.Response.Retry
	if retry == nil || retry.ObservedAtMS > bodyReleased.UnixMilli()-40 || retry.ObservedAtMS < headTime.UnixMilli()-20 ||
		retry.DeadlineMS-retry.ObservedAtMS != 1500 || retry.RemainingMS >= 1470 {
		t.Fatalf("retry advice was observed after unary buffering: %+v", retry)
	}
	time.Sleep(20 * time.Millisecond)
	again := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if again.Response.Retry.DeadlineMS != retry.DeadlineMS || again.Response.Retry.ObservedAtMS != retry.ObservedAtMS ||
		again.Response.Retry.RemainingMS >= retry.RemainingMS || calls.Load() != 1 || f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 1 {
		t.Fatal("repeated reads restarted retry advice or logical inference accounting")
	}
}

func TestDeferredFragmentedSSELateSilenceIncrementalReadsKeepNativeFraming(t *testing.T) {
	firstGate, laterGate := make(chan struct{}), make(chan struct{})
	releaseFirst := sync.OnceFunc(func() { close(firstGate) })
	releaseLater := sync.OnceFunc(func() { close(laterGate) })
	started := make(chan struct{})
	prefix := []byte("id: native-7\r\n: upstream note\r\nevent: vendor.chunk\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"fi")
	suffix := []byte("rst\"}}]}\r\n\r\nevent: vendor.chunk\ndata: {\"choices\":[{\"delta\":{\"content\":\" second\"}}]}\n\n" +
		"data: {\"choices\":[],\ndata: \"usage\":{\"prompt_tokens\":3,\"completion_tokens\":4,\"total_tokens\":7}}\n\n" +
		": native tail\ndata: [DONE]\n\n")
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		input, _ := io.ReadAll(r.Body)
		if !providerStreamFlag(input) || extractModel(input) != "actual" {
			w.WriteHeader(400)
			return
		}
		close(started)
		select {
		case <-firstGate:
		case <-r.Context().Done():
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		for _, fragment := range [][]byte{prefix[:13], prefix[13:41], prefix[41:]} {
			_, _ = w.Write(fragment)
			w.(http.Flusher).Flush()
		}
		select {
		case <-laterGate:
		case <-r.Context().Done():
			return
		}
		for _, fragment := range [][]byte{suffix[:4], suffix[4:35], suffix[35:]} {
			_, _ = w.Write(fragment)
			w.(http.Flusher).Flush()
		}
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(releaseFirst)
	t.Cleanup(releaseLater)
	f := newDeferredHTTPFixture(t, provider.New())
	setDeferredUpstream(f, upstream.URL)
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","stream":true,"messages":[]}`)))
	<-started
	pending := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if len(pending.Data) != 0 || pending.Done {
		t.Fatal("initial SSE silence was replaced with synthetic traffic")
	}
	releaseFirst()
	awaitDeferredHTTP(t, "fragmented native prefix", func() bool { return f.store.Stats().AvailableBytes == int64(len(prefix)) })
	begin := time.Now()
	first := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if time.Since(begin) >= f.store.Options().ControlTimeout || first.State != deferred.StateHeadersReady || !first.Response.Streaming ||
		first.Done || !bytes.Equal(first.Data, prefix) {
		t.Fatalf("fragmented SSE prefix was rewritten or withheld: %+v", first)
	}
	again := f.read(t, receipt, 0, deferred.MaxReadBytes)
	silent := f.read(t, receipt, first.NextCursor, deferred.MaxReadBytes)
	if !bytes.Equal(first.Data, again.Data) || len(silent.Data) != 0 || silent.NextCursor != first.NextCursor || silent.Done {
		t.Fatal("later silent reads consumed prefix bytes or fabricated SSE/completion")
	}
	releaseLater()
	manifest := f.finish(t, receipt)
	last := f.read(t, receipt, first.NextCursor, deferred.MaxReadBytes)
	if !last.Done || !bytes.Equal(append(first.Data, last.Data...), append(prefix, suffix...)) ||
		manifest.Outcome.Kind != deferred.OutcomeResponse || manifest.Outcome.Usage.TotalTokens != 7 ||
		f.accounting.Events()[0].TotalTokens != 7 || calls.Load() != 1 {
		t.Fatal("SSE resume/read bytes, native IDs/extensions, or terminal usage changed")
	}
}

func providerStreamFlag(body []byte) bool {
	var request struct{ Stream bool }
	_ = json.Unmarshal(body, &request)
	return request.Stream
}

func TestDeferredNativeStreamErrorSealsPrefixAndClosesLaterSilence(t *testing.T) {
	closed := make(chan struct{})
	prefix := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"total_tokens\":3}}\n\n" +
		"data: {\"error\":{\"type\":\"provider_error\",\"message\":\"native failure\"}}\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, prefix)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	t.Cleanup(upstream.Close)
	f := newDeferredHTTPFixture(t, provider.New())
	setDeferredUpstream(f, upstream.URL)
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","stream":true}`)))
	manifest := f.finish(t, receipt)
	frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("native stream failure left the silent model source open")
	}
	if manifest.State != deferred.StateFailed || manifest.Outcome.Kind != deferred.OutcomeUpstreamError ||
		manifest.Outcome.Usage.TotalTokens != 3 || !frame.Done || string(frame.Data) != prefix ||
		f.accounting.Events()[0].StatusCode != 502 || f.accounting.Events()[0].TotalTokens != 3 ||
		f.h.current().Health.IsHealthy("p") {
		t.Fatalf("stream prefix/error/known usage was classified as success: %+v", manifest)
	}
}

func TestDeferredProviderHTTPAndAdapterFailuresRemainDistinct(t *testing.T) {
	for _, tc := range []struct {
		name, contentType, body string
		status                  int
		err                     error
		kind                    deferred.OutcomeKind
	}{
		{"json_provider_error", "application/json", `{"error":{"type":"provider_auth","message":"denied"},"vendor":1}`, 401, nil, deferred.OutcomeResponse},
		{"ordinary_provider_202", "application/json", `{"vendor":"upstream accepted response"}`, 202, nil, deferred.OutcomeResponse},
		{"plain_provider_error", "text/html", "  provider temporarily unavailable \n", 503, nil, deferred.OutcomeResponse},
		{"adapter_invalid", "", "", 0, provider.ErrInvalidRequest{Message: "unsupported input"}, deferred.OutcomeInvalidRequest},
		{"adapter_unsupported", "", "", 0, provider.ErrUnsupportedOperation{Operation: provider.OpChatCompletions}, deferred.OutcomeInvalidRequest},
		{"transport", "", "", 0, errors.New("connection failed"), deferred.OutcomeUpstreamError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
				if tc.err != nil {
					return nil, tc.err
				}
				return &provider.Result{StatusCode: tc.status, Header: http.Header{"Content-Type": {tc.contentType}, "X-Aiproxy-Deferred-Private": {"secret"}}, Body: []byte(tc.body)}, nil
			}))
			receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`)))
			manifest := f.finish(t, receipt)
			if manifest.Outcome.Kind != tc.kind {
				t.Fatalf("provider/control outcome conflated: %+v", manifest)
			}
			if tc.err == nil {
				frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
				if manifest.State != deferred.StateCompleted || manifest.Response.StatusCode != tc.status || !frame.Done {
					t.Fatal("retained provider error was replaced by a control failure")
				}
				expected := []byte(tc.body)
				if tc.contentType != "application/json" {
					expected, _ = json.Marshal(upstreamErrorEnvelope(tc.status, []byte(tc.body)))
					expected = append(expected, '\n')
				}
				if !bytes.Equal(frame.Data, expected) || manifest.Response.Headers.Get("X-Aiproxy-Deferred-Private") != "" {
					t.Fatal("provider body normalization or private stripping diverged")
				}
			}
			expectedStatus := tc.status
			if tc.err != nil {
				expectedStatus = 502
				if tc.kind == deferred.OutcomeInvalidRequest {
					expectedStatus = 400
				}
			}
			if len(f.accounting.Events()) != 1 || f.accounting.Events()[0].StatusCode != expectedStatus {
				t.Fatal("control acknowledgment replaced or duplicated the terminal inference status")
			}
		})
	}
}

func TestDeferredAliasPublishesOnlyFinalSelectedHeadAndFinalizesAttemptsOnce(t *testing.T) {
	gate, started := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	var calls, closes atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(ctx context.Context, request provider.Request) (*provider.Result, error) {
		calls.Add(1)
		if request.APIKey == "model-secret" {
			return &provider.Result{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}, "X-Target": {"first"}},
				Body: []byte(`{"error":"first"}`), OnClose: func() { closes.Add(1) }}, nil
		}
		close(started)
		select {
		case <-gate:
			return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "X-Target": {"final"}},
				Body: []byte(`{"id":"final"}`), OnClose: func() { closes.Add(1) }}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	t.Cleanup(release)
	p1 := f.rt.Catalog.Providers()[0]
	p2 := p1
	p2.Name, p2.APIKey = "p2", "second-key"
	f.rt.Catalog = config.NewCatalog([]config.Provider{p1, p2}, nil, []config.Alias{{
		Name: "a", Algorithm: config.AlgorithmLeastConnections, RetryStatusCodes: []int{503},
		Targets: []config.AliasTarget{{Provider: "p", Model: "m"}, {Provider: "p2", Model: "m"}},
	}})
	deps := f.h.current()
	deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
	f.h.UpdateDependencies(deps)
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "alias/a"), []byte(`{"model":"alias/a","stream":false}`)))
	<-started
	frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if frame.Response != nil || len(frame.Data) != 0 || frame.State != deferred.StateRunning {
		t.Fatal("an intermediate retryable alias response head/body was published")
	}
	release()
	manifest := f.finish(t, receipt)
	if manifest.Response.Headers.Get("X-Target") != "final" || f.accounting.Events()[0].Provider != "p2" ||
		calls.Load() != 2 || closes.Load() != 2 || len(f.accounting.Events()) != 1 || f.rate.calls.Load() != 1 {
		t.Fatal("alias selected/finalized attempts or logical completion more than once")
	}
}

func TestDeferredStreamGrowthLimitNeverSealsTruncatedSuccess(t *testing.T) {
	o := executorTestOptions()
	o.MaxResultBytes = 2048
	closed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: "+strings.Repeat("x", 4096)+"\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	t.Cleanup(upstream.Close)
	f := newDeferredHTTPFixture(t, provider.New(), o)
	setDeferredUpstream(f, upstream.URL)
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","stream":true}`)))
	manifest := f.finish(t, receipt)
	frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("growing source was not cancelled/closed on the result ceiling")
	}
	if manifest.State != deferred.StateFailed || manifest.Outcome.Kind != deferred.OutcomeLimitExceeded ||
		len(frame.Data) > 2048 || !frame.Done || f.accounting.Events()[0].StatusCode != 502 || f.store.Stats().WorkBytes != 0 ||
		!f.h.current().Health.IsHealthy("p") {
		t.Fatalf("bounded prefix reported truncated success or leaked work: %+v", manifest)
	}
}

func TestDeferredSilentNativeStreamExecutionDeadlineSealsKnownPrefixWithoutModelHealthMutation(t *testing.T) {
	prefix := []byte("id: original\n: original silence boundary\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"total_tokens\":3}}\n\n")
	closed := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(prefix)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
		close(closed)
	}))
	t.Cleanup(upstream.Close)
	options := executorTestOptions()
	options.ExecutionTimeout = time.Second
	f := newDeferredHTTPFixture(t, provider.New(), options)
	setDeferredUpstream(f, upstream.URL)
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","stream":true}`)))
	awaitDeferredHTTP(t, "native prefix before execution deadline", func() bool { return f.store.Stats().AvailableBytes == int64(len(prefix)) })
	first := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if first.Done || !bytes.Equal(first.Data, prefix) {
		t.Fatal("initial native stream prefix was fabricated or withheld")
	}
	manifest := f.finish(t, receipt)
	frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("execution deadline did not close the silent real model source")
	}
	if manifest.State != deferred.StateFailed || manifest.Outcome.Kind != deferred.OutcomeExecutionTimeout ||
		manifest.Outcome.Usage.TotalTokens != 3 || !frame.Done || !bytes.Equal(frame.Data, prefix) ||
		len(f.accounting.Events()) != 1 || f.accounting.Events()[0].StatusCode != 504 || f.accounting.Events()[0].TotalTokens != 3 ||
		!f.h.current().Health.IsHealthy("p") || f.store.Stats().WorkBytes != 0 {
		t.Fatalf("native silence cutoff changed output, health, or known usage: %+v", manifest.Outcome)
	}
}

func TestDeferredUnaryResponseAndMetadataCeilingsBeforeBuffering(t *testing.T) {
	for _, mode := range []string{"body", "metadata"} {
		t.Run(mode, func(t *testing.T) {
			o := executorTestOptions()
			o.MaxResultBytes = 1024
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.ReadAll(r.Body)
				w.Header().Set("Content-Type", "application/json")
				if mode == "body" {
					w.Header().Set("Content-Length", "4096")
				} else {
					w.Header().Set("X-Oversized", strings.Repeat("x", 4097))
				}
				_, _ = io.WriteString(w, strings.Repeat("x", 4096))
			}))
			t.Cleanup(upstream.Close)
			f := newDeferredHTTPFixture(t, provider.New(), o)
			setDeferredUpstream(f, upstream.URL)
			receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`)))
			manifest := f.finish(t, receipt)
			if manifest.Outcome.Kind != deferred.OutcomeLimitExceeded || manifest.Response != nil || manifest.AvailableBytes != 0 {
				t.Fatalf("%s overflow published buffered success: %+v", mode, manifest)
			}
		})
	}
}

func TestDeferredUnlimitedPayloadSettingIsStillOwnedAndBounded(t *testing.T) {
	large := []byte(`{"choices":[],"vendor":"` + strings.Repeat("x", 70<<10) + `"}`)
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(_ context.Context, request provider.Request) (*provider.Result, error) {
		if request.ExecutionLimits.SnapshotBytes > request.ExecutionLimits.RequestBytes {
			return nil, fmt.Errorf("unbounded snapshot")
		}
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: large}, nil
	}))
	payload := &deferredPayloadRecorder{max: 0}
	deps := f.h.current()
	deps.PayloadLog = payload
	f.h.UpdateDependencies(deps)
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`)))
	manifest := f.finish(t, receipt)
	if manifest.State != deferred.StateCompleted || len(payload.snapshot()) != 1 ||
		len(payload.snapshot()[0].Response.Body.AsString()) > int(f.store.Options().MaxResultBytes) ||
		f.store.Stats().WorkBytes != 0 || f.store.Stats().InputBytes != 0 {
		t.Fatal("unlimited ordinary payload configuration escaped deferred ownership or its output ceiling")
	}
}
