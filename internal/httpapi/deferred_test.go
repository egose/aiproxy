package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
)

type deferredAdapterFunc func(context.Context, provider.Request) (*provider.Result, error)

func (f deferredAdapterFunc) Do(ctx context.Context, r provider.Request) (*provider.Result, error) {
	return f(ctx, r)
}

type deferredRateLimiter struct {
	calls   atomic.Int32
	allowed atomic.Bool
}

func (l *deferredRateLimiter) Allow(string) (bool, time.Duration) {
	l.calls.Add(1)
	return l.allowed.Load(), time.Second
}

type deferredPayloadRecorder struct {
	mu      sync.Mutex
	max     int
	entries []payloadlog.Entry
}

func (r *deferredPayloadRecorder) MaxBodyBytes() int { return r.max }
func (r *deferredPayloadRecorder) Record(e payloadlog.Entry) error {
	r.mu.Lock()
	r.entries = append(r.entries, e)
	r.mu.Unlock()
	return nil
}

func (r *deferredPayloadRecorder) snapshot() []payloadlog.Entry {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]payloadlog.Entry(nil), r.entries...)
}

type deferredHTTPFixture struct {
	h          *Handler
	store      *deferred.Store
	rt         *config.Runtime
	metrics    *observability.Metrics
	accounting *accounting.MemoryRecorder
	rate       *deferredRateLimiter
}

func executorTestOptions() deferred.Options {
	o := deferred.DefaultOptions()
	o.Enabled = true
	o.MaxJobs, o.MaxRunning, o.MaxAdmissions, o.MaxEntries = 8, 4, 4, 32
	o.PerOwnerJobs, o.PerOwnerRunning = 8, 4
	o.MaxRequestBytes, o.MaxResultBytes, o.MaxWorkBytes, o.MaxMemoryBytes = 64<<10, 128<<10, 8<<20, 64<<20
	o.ExecutionTimeout, o.Retention = 2*time.Second, 10*time.Second
	o.ControlTimeout, o.FinalizationTimeout = 200*time.Millisecond, 200*time.Millisecond
	return o
}

func newDeferredHTTPFixture(t *testing.T, adapter provider.Adapter, options ...deferred.Options) *deferredHTTPFixture {
	t.Helper()
	o := executorTestOptions()
	if len(options) > 0 {
		o = options[0]
	}
	s, err := deferred.New(context.Background(), o)
	if err != nil {
		t.Fatal(err)
	}
	f := &deferredHTTPFixture{
		store: s, metrics: observability.NewMetrics(), accounting: &accounting.MemoryRecorder{}, rate: &deferredRateLimiter{},
		rt: &config.Runtime{
			Auth: config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{
				"peer": {Token: "peer-token", Tenant: "tenant"}, "other": {Token: "other-token", Tenant: "tenant"},
			}},
			Catalog: config.NewCatalog([]config.Provider{{
				Type: config.ProviderTypeOpenAICompatible, Name: "p", BaseURL: "http://127.0.0.1:1/v1", APIKey: "model-secret",
				DeferredReceiver: &config.DeferredReceiver{}, Models: []config.Model{{Name: "m", UpstreamName: "actual"}},
			}}, nil, nil),
		},
	}
	f.rate.allowed.Store(true)
	f.h = NewHandler(Dependencies{
		Auth: auth.NewAuthenticator(f.rt.Auth), Authorizer: auth.NewAuthorizer(f.rt.Auth),
		Catalog: f.rt.Catalog, Resolver: modelresolver.New(f.rt), Adapter: adapter,
		DeferredStore: s, RateLimiter: f.rate, Metrics: f.metrics, Accounting: f.accounting,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), HasAccessLog: true,
		Health: providerhealth.New(f.metrics, config.ProviderHealth{}),
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := s.Close(ctx); err != nil {
			t.Errorf("deferred close: %v, stats=%+v", err, s.Stats())
		}
		_ = f.h.current().Health.Close()
	})
	return f
}

func (f *deferredHTTPFixture) request(method, path, token string, body []byte) *http.Request {
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+token)
	r.Header.Set("X-Aiproxy-Deferred-Version", "1")
	if strings.Contains(path, "/jobs/") {
		r.Header.Set("X-Aiproxy-Deferred-Generation", f.store.Generation())
	}
	if method == http.MethodPost {
		r.Header.Set("Content-Type", deferred.MediaType)
	}
	return r
}

func (f *deferredHTTPFixture) call(method, path, token string, body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, f.request(method, path, token, body))
	return w
}

func assertDeferredHeaders(t *testing.T, w *httptest.ResponseRecorder) {
	t.Helper()
	for name, value := range map[string]string{
		"Content-Type": deferred.MediaType, "X-Aiproxy-Deferred-Version": "1",
		"Cache-Control": "private, no-store", "Pragma": "no-cache", "Vary": "Authorization",
	} {
		if w.Header().Get(name) != value {
			t.Fatalf("%s = %q, want %q; status=%d body=%s", name, w.Header().Get(name), value, w.Code, w.Body.String())
		}
	}
}

func controlErrorType(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	assertDeferredHeaders(t, w)
	var failure deferred.ControlError
	if err := deferred.DecodeControl(w.Body.Bytes(), &failure); err != nil {
		t.Fatalf("control error: %v; %s", err, w.Body.String())
	}
	return failure.Error.Type
}

func (f *deferredHTTPFixture) capability(t *testing.T, model string) deferred.Capability {
	t.Helper()
	w := f.call(http.MethodGet, deferred.Prefix+"/capabilities?model="+url.QueryEscape(model), "peer-token", nil)
	assertDeferredHeaders(t, w)
	var capability deferred.Capability
	if w.Code != 200 || deferred.DecodeControl(w.Body.Bytes(), &capability) != nil {
		t.Fatalf("capability: status=%d body=%s", w.Code, w.Body.String())
	}
	return capability
}

func deferredSubmitRequest(t *testing.T, capability deferred.Capability, body []byte) deferred.SubmitRequest {
	t.Helper()
	key, err := deferred.NewAttemptKey()
	if err != nil {
		t.Fatal(err)
	}
	return deferred.SubmitRequest{
		Envelope: deferred.NewEnvelope(), Generation: capability.Generation, SubmitToken: capability.SubmitToken,
		AttemptKey: key, Body: body, Headers: http.Header{"Content-Type": {"application/json"}, "Accept": {"application/json"}, "User-Agent": {"peer-inference"}},
	}
}

func submitWire(t *testing.T, request deferred.SubmitRequest) []byte {
	t.Helper()
	wire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	return wire
}

func (f *deferredHTTPFixture) submit(t *testing.T, request deferred.SubmitRequest) deferred.Receipt {
	t.Helper()
	w := f.call(http.MethodPost, deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
	assertDeferredHeaders(t, w)
	var receipt deferred.Receipt
	if w.Code != 202 || deferred.DecodeControl(w.Body.Bytes(), &receipt) != nil {
		t.Fatalf("submit: status=%d body=%s", w.Code, w.Body.String())
	}
	if w.Header().Get("Location") != deferred.Prefix+"/jobs/"+receipt.ID {
		t.Fatal("submit location does not match the locally derived job path")
	}
	return receipt
}

func awaitDeferredHTTP(t *testing.T, label string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", label)
		}
		time.Sleep(time.Millisecond)
	}
}

func (f *deferredHTTPFixture) finish(t *testing.T, receipt deferred.Receipt) deferred.ResultManifest {
	t.Helper()
	awaitDeferredHTTP(t, "executor finalization", func() bool { return f.store.Stats().Running == 0 && f.store.Stats().FinalizationCredits == 0 })
	w := f.call(http.MethodGet, deferred.Prefix+"/jobs/"+receipt.ID+"/result", "peer-token", nil)
	var manifest deferred.ResultManifest
	if w.Code != 200 || deferred.DecodeControl(w.Body.Bytes(), &manifest) != nil {
		t.Fatalf("manifest: status=%d body=%s", w.Code, w.Body.String())
	}
	return manifest
}

func (f *deferredHTTPFixture) read(t *testing.T, receipt deferred.Receipt, cursor uint64, limit int) deferred.ReadFrame {
	t.Helper()
	path := fmt.Sprintf("%s/jobs/%s/read?cursor=%d&limit=%d", deferred.Prefix, receipt.ID, cursor, limit)
	w := f.call(http.MethodGet, path, "peer-token", nil)
	var frame deferred.ReadFrame
	if w.Code != 200 || deferred.DecodeControl(w.Body.Bytes(), &frame) != nil {
		t.Fatalf("read: status=%d body=%s", w.Code, w.Body.String())
	}
	return frame
}

func TestDeferredRoutingVersionsMethodsDisabledAndWebUI(t *testing.T) {
	f := newDeferredHTTPFixture(t, nil)
	deps := f.h.current()
	deps.WebUI = config.WebUI{Enabled: true}
	f.h.UpdateDependencies(deps)
	id, _ := deferred.NewAttemptKey()
	for _, tc := range []struct {
		method, path, version, errorType, allow string
		status                                  int
	}{
		{"GET", deferred.Prefix + "/capabilities?model=p/m", "", "unsupported_deferred_version", "", 426},
		{"GET", deferred.Prefix + "/capabilities?model=p/m", "2", "unsupported_deferred_version", "", 426},
		{"GET", deferredPath + "/v2/capabilities?model=p/m", "1", "unsupported_deferred_version", "", 426},
		{"POST", deferred.Prefix + "/capabilities", "1", "method_not_allowed", "GET", 405},
		{"GET", deferred.Prefix + "/submit", "1", "method_not_allowed", "POST", 405},
		{"PATCH", deferred.Prefix + "/jobs/" + id, "1", "method_not_allowed", "GET, DELETE", 405},
		{"DELETE", deferred.Prefix + "/jobs/" + id + "/read", "1", "method_not_allowed", "GET", 405},
		{"GET", deferred.Prefix + "/unknown", "1", "not_found", "", 404},
		{"GET", deferred.Prefix + "/jobs/" + id + "/unknown", "1", "not_found", "", 404},
		{"GET", deferredPath, "1", "unsupported_deferred_version", "", 426},
	} {
		r := f.request(tc.method, tc.path, "peer-token", nil)
		r.Header.Set("Accept", "text/html")
		r.Header.Set("X-Aiproxy-Deferred-Version", tc.version)
		w := httptest.NewRecorder()
		f.h.ServeHTTP(w, r)
		if w.Code != tc.status || controlErrorType(t, w) != tc.errorType || w.Header().Get("Allow") != tc.allow {
			t.Fatalf("%s %s: status=%d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}
	o := f.store.Options()
	o.Enabled = false
	if err := f.store.Reconfigure(o); err != nil {
		t.Fatal(err)
	}
	w := f.call("GET", deferred.Prefix+"/capabilities?model=p/m", "peer-token", nil)
	if w.Code != 404 || controlErrorType(t, w) != "deferred_disabled" {
		t.Fatalf("disabled capability: %d %s", w.Code, w.Body.String())
	}
	deps = f.h.current()
	deps.DeferredStore = nil
	f.h.UpdateDependencies(deps)
	w = f.call("POST", deferred.Prefix+"/submit", "peer-token", []byte("{}"))
	if w.Code != 404 || controlErrorType(t, w) != "deferred_disabled" {
		t.Fatalf("absent store submit fell through to UI: %d %s", w.Code, w.Body.String())
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "text/html")
	w = httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "text/html") {
		t.Fatal("ordinary UI root stopped working alongside private routes")
	}
	if len(f.accounting.Events()) != 0 || f.rate.calls.Load() != 0 {
		t.Fatal("private negotiation/control traffic was charged as inference")
	}
}

func TestDeferredAdmissionStructuralAndHeaderRejectionsHaveZeroInference(t *testing.T) {
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return nil, fmt.Errorf("unexpected inference")
	}))
	capability := f.capability(t, "p/m")
	for _, body := range []string{
		`[]`, `{}`, `{"model":""}`, `{"model":null}`, `{"model":"p/m","model":"p/m"}`,
		`{"model":"p/m","mo\u0064el":"p/m"}`, `{"model":"p/m","stream":1}`,
		`{"model":"p/m","stream":true,"stream":false}`, `{"model":"p/m"} trailing`,
		`{"model":"p/m","x":` + strings.Repeat("[", deferred.JSONDepth) + `0` + strings.Repeat("]", deferred.JSONDepth) + `}`,
	} {
		request := deferredSubmitRequest(t, capability, []byte(body))
		w := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
		if w.Code != 400 {
			t.Fatalf("accepted invalid chat body %q: %d %s", body, w.Code, w.Body.String())
		}
	}
	for _, name := range []string{"Authorization", "Cookie", "Connection", "X-Aiproxy-Deferred-Version", "x-aiproxy-deferred-secret"} {
		request := deferredSubmitRequest(t, capability, []byte(`{"model":"p/m","messages":[]}`))
		request.Headers[name] = []string{"private"}
		w := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
		if w.Code != 400 || controlErrorType(t, w) != "invalid_body" {
			t.Fatalf("accepted forbidden header %s: %d %s", name, w.Code, w.Body.String())
		}
	}
	request := deferredSubmitRequest(t, capability, []byte(`{"model":"p/m","x":"`+strings.Repeat("x", int(f.store.Options().MaxRequestBytes))+`"}`))
	w := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
	if w.Code != 413 || controlErrorType(t, w) != "request_too_large" {
		t.Fatalf("oversized decoded input: %d %s", w.Code, w.Body.String())
	}
	wire := submitWire(t, deferredSubmitRequest(t, capability, []byte(`{"model":"p/m"}`)))
	for _, malformed := range [][]byte{
		append(append([]byte(nil), wire[:len(wire)-1]...), []byte(`,"unknown":1}`)...),
		append(append([]byte(nil), wire[:len(wire)-1]...), []byte(`,"version":1}`)...),
		bytes.Replace(wire, []byte(`"version":1`), []byte(`"version":2`), 1),
	} {
		w := f.call("POST", deferred.Prefix+"/submit", "peer-token", malformed)
		if w.Code != 400 && w.Code != 426 {
			t.Fatalf("malformed envelope: %d %s", w.Code, w.Body.String())
		}
	}
	if calls.Load() != 0 || f.rate.calls.Load() != 0 || len(f.accounting.Events()) != 0 || f.store.Stats().Jobs != 0 {
		t.Fatal("pre-admission validation dispatched or consumed inference policy")
	}
}

func TestDeferredRejectedReplayDoesNotRerunRateAdmission(t *testing.T) {
	f := newDeferredHTTPFixture(t, nil)
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","messages":[]}`))
	f.rate.allowed.Store(false)
	first := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
	var ordinary apiError
	if first.Code != 429 || json.Unmarshal(first.Body.Bytes(), &ordinary) != nil || ordinary.Error.Type != "rate_limited" {
		t.Fatalf("rate rejection: %d %s", first.Code, first.Body.String())
	}
	if bytes.Contains(first.Body.Bytes(), []byte(`"protocol"`)) || first.Header().Get("Retry-After") != "1" {
		t.Fatal("ordinary admission rejection was changed into a protocol/provider outcome")
	}
	f.rate.allowed.Store(true)
	second := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
	if second.Code != first.Code || second.Body.String() != first.Body.String() || second.Header().Get("Retry-After") != "1" ||
		f.rate.calls.Load() != 1 || f.store.Stats().Jobs != 0 || len(f.accounting.Events()) != 0 {
		t.Fatal("cached policy rejection was recharged or rerun")
	}
}

func TestDeferredCapabilitiesValidateWholeAliasPoolAndMinimumLimits(t *testing.T) {
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return nil, fmt.Errorf("unexpected inference")
	}))
	p1 := f.rt.Catalog.Providers()[0]
	p1.DeferredReceiver = &config.DeferredReceiver{ExecutionTimeout: time.Second, MaxResultBytes: 2048}
	p2 := p1
	p2.Name = "p2"
	p2.DeferredReceiver = &config.DeferredReceiver{ExecutionTimeout: 2 * time.Second, MaxResultBytes: 4096}
	alias := config.Alias{Name: "a", Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "p", Model: "m"}, {Provider: "p2", Model: "m"}}}
	f.rt.Catalog = config.NewCatalog([]config.Provider{p1, p2}, nil, []config.Alias{alias})
	deps := f.h.current()
	deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
	f.h.UpdateDependencies(deps)
	capability := f.capability(t, "alias/a")
	if capability.Limits.ResultBytes != 2048 || capability.Limits.ExecutionTimeoutMS != 1000 {
		t.Fatalf("alias advertised wider than its pool: %+v", capability.Limits)
	}
	for _, mode := range []string{"missing_receiver", "recursive", "unsupported_chat"} {
		bad := p2
		switch mode {
		case "missing_receiver":
			bad.DeferredReceiver = nil
		case "recursive":
			bad.DeferredTransport = &config.DeferredTransport{}
		case "unsupported_chat":
			bad.Models = []config.Model{{Name: "m", UpstreamName: "actual", Capabilities: []config.Capability{config.CapabilityEmbeddings}}}
			bad.ModelByName = nil
		}
		f.rt.Catalog = config.NewCatalog([]config.Provider{p1, bad}, nil, []config.Alias{alias})
		deps = f.h.current()
		deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
		f.h.UpdateDependencies(deps)
		w := f.call("GET", deferred.Prefix+"/capabilities?model=alias/a", "peer-token", nil)
		if w.Code != 400 {
			t.Fatalf("%s alias capability: %d %s", mode, w.Code, w.Body.String())
		}
		request := deferredSubmitRequest(t, capability, []byte(`{"model":"alias/a","messages":[]}`))
		w = f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
		if w.Code != 400 || calls.Load() != 0 {
			t.Fatalf("%s alias submitted by filtering its pool: %d %s", mode, w.Code, w.Body.String())
		}
	}
}

func TestDeferredAcceptedReplayReadsCurrentOwnerAuthorizationAndRotation(t *testing.T) {
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(ctx context.Context, r provider.Request) (*provider.Result, error) {
		calls.Add(1)
		select {
		case <-gate:
			return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}, "X-Aiproxy-Deferred-Secret": {"never-public"}},
				Body: []byte(`{"choices":[],"vendor":{"kept":true}}`), Usage: provider.Usage{PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}}, nil
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}))
	t.Cleanup(release)
	payload := &deferredPayloadRecorder{max: 64}
	deps := f.h.current()
	deps.PayloadLog = payload
	f.h.UpdateDependencies(deps)
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m","messages":[],"vendor":{"keep":true}}`))
	receipt := f.submit(t, request)
	awaitDeferredHTTP(t, "one provider call", func() bool { return calls.Load() == 1 })
	pending := f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID+"/result", "peer-token", nil)
	var info deferred.JobInfo
	if pending.Code != 202 || deferred.DecodeControl(pending.Body.Bytes(), &info) != nil || info.State != deferred.StateRunning {
		t.Fatalf("pending result: %d %s", pending.Code, pending.Body.String())
	}
	frame := f.read(t, receipt, 0, 10)
	if frame.Data == nil || len(frame.Data) != 0 || frame.NextCursor != 0 || frame.Done {
		t.Fatal("pending immediate read invented output/completion")
	}
	replayed := f.submit(t, request)
	if !replayed.Replayed || replayed.ID != receipt.ID || f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 0 {
		t.Fatal("accepted replay reran inference admission or completion")
	}
	conflict := request
	conflict.Body = append(append([]byte(nil), request.Body...), '\n')
	w := f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, conflict))
	if w.Code != 409 || controlErrorType(t, w) != "attempt_conflict" {
		t.Fatal("conflicting original body fingerprint was accepted")
	}
	release()
	manifest := f.finish(t, receipt)
	if manifest.State != deferred.StateCompleted || manifest.Outcome.Kind != deferred.OutcomeResponse || manifest.Outcome.Usage.TotalTokens != 5 ||
		manifest.Response.Headers.Get("X-Aiproxy-Deferred-Secret") != "" {
		t.Fatalf("completed inference: %+v", manifest)
	}
	frame = f.read(t, receipt, 0, deferred.MaxReadBytes)
	repeated := f.read(t, receipt, 0, deferred.MaxReadBytes)
	sum := sha256.Sum256(frame.Data)
	if !frame.Done || !bytes.Equal(frame.Data, repeated.Data) || manifest.BodySHA256 != hex.EncodeToString(sum[:]) {
		t.Fatal("repeated finite reads consumed or changed the retained body")
	}
	for _, method := range []string{"GET", "DELETE"} {
		w := f.call(method, deferred.Prefix+"/jobs/"+receipt.ID, "other-token", nil)
		if w.Code != 404 || controlErrorType(t, w) != "job_unavailable" {
			t.Fatal("same-tenant different named owner could reveal/delete a job")
		}
	}
	deps = f.h.current()
	deps.Authorizer = auth.AuthorizerFunc(func(*auth.Principal, string) bool { return false })
	f.h.UpdateDependencies(deps)
	for _, suffix := range []string{"", "/result", "/read?cursor=0"} {
		w := f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID+suffix, "peer-token", nil)
		if w.Code != 403 || controlErrorType(t, w) != "forbidden" {
			t.Fatal("current model denial disclosed a retained result")
		}
	}
	w = f.call("DELETE", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
	if w.Code != 403 {
		t.Fatal("model-denied owner deleted a job")
	}
	w = f.call("POST", deferred.Prefix+"/submit", "peer-token", submitWire(t, request))
	if w.Code != 403 || controlErrorType(t, w) != "forbidden" {
		t.Fatal("accepted replay bypassed current model denial")
	}
	f.rt.Auth.Clients["peer"] = config.Client{Token: "rotated-token", Tenant: "tenant"}
	deps = f.h.current()
	deps.Auth, deps.Authorizer = auth.NewAuthenticator(f.rt.Auth), auth.NewAuthorizer(f.rt.Auth)
	f.h.UpdateDependencies(deps)
	w = f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
	if w.Code != 401 {
		t.Fatal("rotated-away token still controlled accepted work")
	}
	for range 2 {
		w = f.call("DELETE", deferred.Prefix+"/jobs/"+receipt.ID, "rotated-token", nil)
		assertDeferredHeaders(t, w)
		if w.Code != 204 || w.Body.Len() != 0 {
			t.Fatalf("known delete is not idempotent: %d %s", w.Code, w.Body.String())
		}
	}
	w = f.call("POST", deferred.Prefix+"/submit", "rotated-token", submitWire(t, request))
	if w.Code != 202 {
		t.Fatal("released tombstone erased accepted submission protection")
	}
	w = f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID+"/read?cursor=0", "rotated-token", nil)
	if w.Code != 410 || controlErrorType(t, w) != "job_released" {
		t.Fatal("released result remained readable")
	}
	if calls.Load() != 1 || f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 1 || len(payload.snapshot()) != 1 ||
		f.accounting.Events()[0].StatusCode != 200 || f.accounting.Events()[0].TotalTokens != 5 {
		t.Fatal("poll/replay/delete duplicated logical completion")
	}
	body := httptest.NewRecorder()
	f.metrics.Handler().ServeHTTP(body, httptest.NewRequest("GET", "/metrics", nil))
	if strings.Contains(body.Body.String(), receipt.ID) || strings.Contains(body.Body.String(), request.SubmitToken) ||
		!strings.Contains(body.Body.String(), `aiproxy_deferred_admissions_total{outcome="accepted"} 1`) ||
		!strings.Contains(body.Body.String(), `aiproxy_deferred_admissions_total{outcome="replayed"} 2`) {
		t.Fatalf("unbounded/private deferred metric labels: %s", body.Body.String())
	}
}

func TestDeferredReadCursorAndGenerationValidation(t *testing.T) {
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`)))
	f.finish(t, receipt)
	for _, query := range []string{"", "?cursor=-1", "?cursor=00", "?cursor=18446744073709551616", "?cursor=3", "?cursor=0&limit=0", "?cursor=0&limit=65537", "?cursor=0&limit=", "?cursor=0&cursor=0", "?cursor=0&wait=1"} {
		w := f.call("GET", deferred.Prefix+"/jobs/"+receipt.ID+"/read"+query, "peer-token", nil)
		if w.Code != 400 || controlErrorType(t, w) != "invalid_cursor" {
			t.Fatalf("invalid cursor query %s: %d %s", query, w.Code, w.Body.String())
		}
	}
	r := f.request("GET", deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
	other, _ := deferred.NewAttemptKey()
	r.Header.Set("X-Aiproxy-Deferred-Generation", other)
	w := httptest.NewRecorder()
	f.h.ServeHTTP(w, r)
	if w.Code != 410 || controlErrorType(t, w) != "generation_mismatch" {
		t.Fatal("unknown process generation was treated as a current job")
	}
	for _, method := range []string{"GET", "DELETE"} {
		w := f.call(method, deferred.Prefix+"/jobs/"+other, "peer-token", nil)
		if w.Code != 404 || controlErrorType(t, w) != "job_unavailable" {
			t.Fatal("unknown job had a fabricated release/expiry classification")
		}
	}
	frame := f.read(t, receipt, 2, 1)
	if !frame.Done || frame.NextCursor != 2 || frame.Data == nil || len(frame.Data) != 0 {
		t.Fatal("terminal empty finite read was not canonical")
	}
	if deferredMetricsPath(deferred.Prefix+"/jobs/"+receipt.ID+"/read", "GET") != deferred.Prefix+"/jobs/{id}/read" ||
		deferredMetricsPath(deferred.Prefix+"/jobs/"+strings.Repeat("x/", 10000), "GET") != deferredPath+"/unknown" {
		t.Fatal("dynamic/private routes were not normalized")
	}
	if strconv.FormatUint(frame.AvailableBytes, 10) != "2" {
		t.Fatal("retained byte cursors changed")
	}
}
