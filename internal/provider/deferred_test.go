package provider_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

type deferredPeerFixture struct {
	handler *httpapi.Handler
	store   *deferred.Store
}

func newCallerPeerFixture(t *testing.T, upstream string, hooks ...deferred.Hooks) *deferredPeerFixture {
	t.Helper()
	o := deferred.DefaultOptions()
	o.Enabled = true
	o.MaxJobs, o.MaxRunning, o.MaxAdmissions, o.MaxEntries = 4, 2, 2, 16
	o.PerOwnerJobs, o.PerOwnerRunning = 4, 2
	o.MaxRequestBytes, o.MaxResultBytes, o.MaxWorkBytes, o.MaxMemoryBytes = 64<<10, 256<<10, 8<<20, 64<<20
	o.ExecutionTimeout, o.Retention, o.ControlTimeout, o.FinalizationTimeout = 2*time.Second, 10*time.Second, 200*time.Millisecond, 200*time.Millisecond
	store, err := deferred.New(context.Background(), o, hooks...)
	if err != nil {
		t.Fatal(err)
	}
	rt := &config.Runtime{
		Auth: config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"peer": {Token: "peer-token"}}},
		Catalog: config.NewCatalog([]config.Provider{{
			Type: config.ProviderTypeOpenAICompatible, Name: "p", BaseURL: upstream + "/v1", APIKey: "ultimate-secret",
			ForwardUserAgent: true, ForwardHeaders: []string{"X-Inference", "X-Aiproxy-Deferred-Version"},
			DeferredReceiver: &config.DeferredReceiver{}, Models: []config.Model{{Name: "m", UpstreamName: "actual-model"}},
		}}, nil, nil),
	}
	f := &deferredPeerFixture{store: store}
	f.handler = httpapi.NewHandler(httpapi.Dependencies{
		Auth: auth.NewAuthenticator(rt.Auth), Authorizer: auth.NewAuthorizer(rt.Auth), Catalog: rt.Catalog,
		Resolver: modelresolver.New(rt), DeferredStore: store, HasAccessLog: true,
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := store.Close(ctx); err != nil {
			t.Errorf("peer store close: %v", err)
		}
	})
	return f
}

func callerOptions() *config.DeferredTransport {
	o := config.DefaultDeferredTransport()
	o.ControlTimeout, o.MaxWait, o.PollInterval = 200*time.Millisecond, 3*time.Second, 50*time.Millisecond
	return &o
}

func callerRequest(peer string, options *config.DeferredTransport, stream bool) provider.Request {
	body := []byte(fmt.Sprintf(`{"model":"local/m","stream":%t,"messages":[{"role":"user","content":"hi"}],"vendor":{"keep":true}}`, stream))
	inbound := httptest.NewRequest("POST", "/v1/chat/completions", bytes.NewReader(body))
	inbound.Header.Set("Accept", "application/json")
	inbound.Header.Set("User-Agent", "inference-client")
	inbound.Header.Set("X-Inference", "keep-me")
	inbound.Header.Set("Cookie", "never-forward")
	inbound.Header.Set("Connection", "X-Hop")
	inbound.Header.Set("X-Hop", "never-forward")
	inbound.Header.Set("X-Aiproxy-Deferred-Version", "99")
	return provider.Request{
		Operation: provider.OpChatCompletions, ProviderType: config.ProviderTypeOpenAICompatible,
		PublicModel: "local/m", BaseURL: peer + "/v1", APIKey: "peer-token", UpstreamModel: "p/m",
		Body: body, Inbound: inbound, DeferredTransport: options, Version: "test", ForwardUserAgent: true,
		ForwardHeaders: []string{"X-Inference", "Cookie", "Authorization", "Connection", "X-Hop", "X-Aiproxy-Deferred-Version"},
	}
}

func closeCallerResult(result *provider.Result) {
	if result == nil {
		return
	}
	if result.StreamBody != nil {
		_ = result.StreamBody.Close()
	}
	if result.OnClose != nil {
		result.OnClose()
	}
}

func assertCallerControlError(t *testing.T, result *provider.Result, err error, code string, acceptance deferred.Acceptance) {
	t.Helper()
	closeCallerResult(result)
	var failure *provider.DeferredControlError
	if !errors.As(err, &failure) || failure.Code != code || failure.Acceptance != acceptance ||
		result != nil && result.Origin != provider.OriginPeerControl {
		t.Fatalf("control result: origin=%v error=%v, want %s/%s", result, err, code, acceptance)
	}
	if delay, ok := provider.CooldownDelayFromError(err); ok || delay != 0 {
		t.Fatal("control error became model retry advice")
	}
}

func copyCallerResponse(w http.ResponseWriter, response *httptest.ResponseRecorder, interrupt bool) {
	for name, values := range response.Header() {
		w.Header()[name] = values
	}
	data := response.Body.Bytes()
	if interrupt {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)+1))
	}
	w.WriteHeader(response.Code)
	if interrupt {
		_, _ = w.Write(data[:max(1, len(data)/2)])
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		return
	}
	_, _ = w.Write(data)
}

func TestDeferredCallerLostAckDuplicateSubmitAndIncompleteReadKeepOneInference(t *testing.T) {
	var calls atomic.Int32
	body := []byte(" \n" + `{"vendor":"` + strings.Repeat("x", 100<<10) + `","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}` + "\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		input, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(input), `"model":"actual-model"`) || !strings.Contains(string(input), `"vendor":{"keep":true}`) ||
			r.Header.Get("Authorization") != "Bearer ultimate-secret" || r.Header.Get("X-Inference") != "keep-me" ||
			r.Header.Get("User-Agent") != "inference-client" || r.Header.Get("Accept") != "application/json" ||
			r.Header.Get("Cookie") != "" || r.Header.Get("X-Hop") != "" {
			t.Errorf("ordinary inference metadata changed: %s %#v", input, r.Header)
		}
		for name := range r.Header {
			if config.IsDeferredHeader(name) {
				t.Errorf("private header reached ultimate provider: %s", name)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Vendor", "retained")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var submits, reads, deletes atomic.Int32
	var mu sync.Mutex
	var wires [][]byte
	var cursors []string
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/mount/") {
			t.Error("caller escaped the configured mount")
		}
		r.URL.Path = strings.TrimPrefix(r.URL.Path, "/mount")
		if r.Method == "DELETE" {
			deletes.Add(1)
		}
		response := httptest.NewRecorder()
		submit := r.URL.Path == deferred.Prefix+"/submit"
		if submit {
			data, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(data))
			mu.Lock()
			wires = append(wires, data)
			mu.Unlock()
		}
		read := strings.HasSuffix(r.URL.Path, "/read")
		if read {
			mu.Lock()
			cursors = append(cursors, r.URL.Query().Get("cursor"))
			mu.Unlock()
		}
		f.handler.ServeHTTP(response, r)
		if location := response.Header().Get("Location"); location != "" {
			response.Header().Set("Location", "/mount"+location)
		}
		interrupt := submit && submits.Add(1) == 1 || read && reads.Add(1) == 1
		copyCallerResponse(w, response, interrupt)
	}))
	defer peer.Close()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL+"/mount", callerOptions(), false))
	if err != nil {
		closeCallerResult(result)
		t.Fatal(err)
	}
	if result.StatusCode != 202 || result.Origin != provider.OriginInference || result.Streaming || !bytes.Equal(result.Body, body) ||
		result.Header.Get("X-Vendor") != "retained" || result.Usage.TotalTokens != 5 ||
		result.UpstreamRequestHeaders.Get("User-Agent") != "inference-client" ||
		!strings.Contains(string(result.UpstreamRequestBody), `"model":"p/m"`) {
		t.Fatalf("ordinary unary outcome changed: status=%d origin=%s streaming=%v usage=%+v bytes=%d", result.StatusCode, result.Origin, result.Streaming, result.Usage, len(result.Body))
	}
	closeCallerResult(result)
	mu.Lock()
	defer mu.Unlock()
	if calls.Load() != 1 || submits.Load() != 2 || deletes.Load() != 1 || len(wires) != 2 || !bytes.Equal(wires[0], wires[1]) ||
		len(cursors) < 2 || cursors[0] != "0" || cursors[1] != "0" {
		t.Fatalf("unsafe retry: inference=%d submits=%d deletes=%d cursors=%v", calls.Load(), submits.Load(), deletes.Load(), cursors)
	}
	submitted, decodeErr := deferred.DecodeSubmit(wires[0])
	if decodeErr != nil {
		t.Fatalf("invalid submit: %v", decodeErr)
	}
	for name := range submitted.Headers {
		if config.IsDeferredHeader(name) || strings.EqualFold(name, "Authorization") || strings.EqualFold(name, "Cookie") ||
			strings.EqualFold(name, "Connection") || strings.EqualFold(name, "X-Hop") {
			t.Fatalf("forbidden inference bag header: %s", name)
		}
	}
	if !strings.Contains(string(result.UpstreamRequestBody), `"model":"p/m"`) || bytes.Contains(result.UpstreamRequestBody, []byte("submit_token")) {
		t.Fatal("payload snapshot contains a private envelope instead of inference")
	}
}

func TestDeferredCallerStreamingFragmentedBytesRecoverSameCursor(t *testing.T) {
	pieces := []string{
		": original\r\nid: native-7\r\nevent: vendor\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"",
		"one\"}}]}\r\n\r\n",
		"data: {\"vendor\":\"kept\"}\ndata: second-line\n\n",
		"data: {\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n",
		"data: [DONE]\n\n",
	}
	firstSent, later := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		_, _ = io.WriteString(w, pieces[0])
		w.(http.Flusher).Flush()
		close(firstSent)
		select {
		case <-later:
		case <-r.Context().Done():
			return
		}
		for _, piece := range pieces[1:] {
			_, _ = io.WriteString(w, piece)
			w.(http.Flusher).Flush()
		}
	}))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var dropped atomic.Bool
	var mu sync.Mutex
	var interrupted, retried string
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, r)
		interrupt := false
		if strings.HasSuffix(r.URL.Path, "/read") && response.Code == 200 {
			var frame deferred.ReadFrame
			if json.Unmarshal(response.Body.Bytes(), &frame) == nil && len(frame.Data) > 0 {
				mu.Lock()
				if dropped.CompareAndSwap(false, true) {
					interrupted, interrupt = r.URL.Query().Get("cursor"), true
				} else if retried == "" {
					retried = r.URL.Query().Get("cursor")
				}
				mu.Unlock()
			}
		}
		copyCallerResponse(w, response, interrupt)
	}))
	defer peer.Close()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), true))
	if err != nil {
		t.Fatal(err)
	}
	defer closeCallerResult(result)
	if !result.Streaming || result.StreamBody == nil || result.StatusCode != 200 || result.Origin != provider.OriginInference {
		t.Fatal("caller did not return an ordinary incremental stream after the head")
	}
	<-firstSent
	var delivered bytes.Buffer
	scratch := make([]byte, 7)
	for delivered.Len() < len(pieces[0]) {
		n, err := result.StreamBody.Read(scratch)
		if err != nil {
			t.Fatalf("read first fragmented prefix: %v", err)
		}
		delivered.Write(scratch[:n])
	}
	if string(delivered.Bytes()) != pieces[0] {
		t.Fatal("native partial event was buffered, synthesized, or changed")
	}
	close(later)
	remaining, err := io.ReadAll(result.StreamBody)
	if err != nil {
		t.Fatal(err)
	}
	delivered.Write(remaining)
	if delivered.String() != strings.Join(pieces, "") || result.Stream.Wait().Usage.TotalTokens != 6 || calls.Load() != 1 {
		t.Fatalf("stream changed or accounting duplicated: bytes=%q usage=%+v calls=%d", delivered.String(), result.Stream.Wait(), calls.Load())
	}
	mu.Lock()
	defer mu.Unlock()
	if !dropped.Load() || interrupted != retried {
		t.Fatalf("partial frame advanced cursor: %q -> %q", interrupted, retried)
	}
}

func deferredMarker(w http.ResponseWriter) {
	w.Header().Set("Content-Type", deferred.MediaType)
	w.Header().Set("X-Aiproxy-Deferred-Version", "1")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Vary", "Authorization")
}

func TestDeferredCallerRequiresPositiveCapabilityAndRejectsOrdinary202(t *testing.T) {
	id, _ := deferred.NewAttemptKey()
	valid := deferred.Capability{
		Envelope: deferred.NewEnvelope(), Generation: id, PublicModel: "p/m", SubmitToken: "bounded-signed-scope",
		SubmitUntilMS: time.Now().Add(deferred.SubmitWindow).UnixMilli(), PollAfterMS: 250,
		Limits: deferred.Limits{RequestBytes: 64 << 10, ResultBytes: 64 << 10, ExecutionTimeoutMS: 1000, RetentionMS: 10000, ControlTimeoutMS: 100, FinalizationTimeoutMS: 100, ReadBytes: deferred.MaxReadBytes},
	}
	for _, variant := range []string{"ordinary-202", "old-peer", "wrong-marker", "wrong-envelope", "wrong-model", "wrong-limits", "duplicate", "unknown"} {
		t.Run(variant, func(t *testing.T) {
			var submits, ordinary atomic.Int32
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == "POST" {
					submits.Add(1)
				}
				if r.URL.Path == "/v1/chat/completions" {
					ordinary.Add(1)
				}
				capability := valid
				deferredMarker(w)
				switch variant {
				case "ordinary-202":
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(202)
					_, _ = io.WriteString(w, `{"id":"ordinary-provider-202"}`)
					return
				case "old-peer":
					w.Header().Del("X-Aiproxy-Deferred-Version")
				case "wrong-marker":
					w.Header().Set("X-Aiproxy-Deferred-Version", "2")
				case "wrong-envelope":
					capability.Version = 2
				case "wrong-model":
					capability.PublicModel = "other/m"
				case "wrong-limits":
					capability.Limits.ReadBytes++
				case "duplicate":
					wire, _ := json.Marshal(capability)
					_, _ = w.Write(append([]byte(`{"protocol":"aiproxy-deferred",`), wire[1:]...))
					return
				case "unknown":
					wire, _ := json.Marshal(capability)
					_, _ = w.Write(append([]byte(`{"unknown":1,`), wire[1:]...))
					return
				}
				_ = json.NewEncoder(w).Encode(capability)
			}))
			defer peer.Close()
			result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), false))
			assertCallerControlError(t, result, err, "deferred_protocol_error", deferred.AcceptanceNotAccepted)
			if submits.Load() != 0 || ordinary.Load() != 0 {
				t.Fatal("unrecognized capability caused inference or unsafe submit")
			}
		})
	}
}

func TestDeferredCallerInvalidBasePathsHaveZeroNetworkIO(t *testing.T) {
	for _, base := range []string{
		"http://peer/mount/v1?x=1", "http://peer/mount/v1?", "http://peer/mount/v1#",
		"http://peer/a/../v1", "http://peer/a/%2e%2e/v1", "http://peer/a/%252e%252e/v1",
		"http://peer/a%2fb/v1", "http://peer/a%252fb/v1", "http://peer/a%5cb/v1",
		"http://peer/a\\b/v1", "http://user:secret@peer/v1", "/relative/v1",
	} {
		t.Run(base, func(t *testing.T) {
			var calls atomic.Int32
			request := callerRequest("http://peer", callerOptions(), false)
			request.BaseURL = base
			request.Client = &http.Client{Transport: callerTestTransport(func(*http.Request) (*http.Response, error) {
				calls.Add(1)
				return nil, errors.New("unexpected network I/O")
			})}
			result, err := provider.New().Do(context.Background(), request)
			assertCallerControlError(t, result, err, "deferred_protocol_error", deferred.AcceptanceNotAccepted)
			if calls.Load() != 0 {
				t.Fatal("invalid base URL sent a credential-bearing request")
			}
		})
	}
}

func TestDeferredCallerInvalidLocationPinsAcceptedJobWithoutCredentialLeak(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	var escaped atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { escaped.Add(1) }))
	defer destination.Close()
	for _, variant := range []string{"cross-origin", "wrong-mount", "other-job", "query", "fragment", "encoded", "multiple"} {
		t.Run(variant, func(t *testing.T) {
			f := newCallerPeerFixture(t, upstream.URL)
			var submits, reads, deletes atomic.Int32
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				r.URL.Path = strings.TrimPrefix(r.URL.Path, "/mount")
				response := httptest.NewRecorder()
				f.handler.ServeHTTP(response, r)
				if r.URL.Path == deferred.Prefix+"/submit" {
					submits.Add(1)
					location := "/mount" + response.Header().Get("Location")
					switch variant {
					case "cross-origin":
						location = destination.URL + location
					case "wrong-mount":
						location = strings.TrimPrefix(location, "/mount")
					case "other-job":
						location += "other"
					case "query":
						location += "?credential=never"
					case "fragment":
						location += "#fragment"
					case "encoded":
						location = strings.Replace(location, "/jobs/", "%2fjobs%2f", 1)
					case "multiple":
						response.Header().Add("Location", location)
					}
					response.Header()["Location"] = append([]string{location}, response.Header().Values("Location")[1:]...)
				}
				if strings.HasSuffix(r.URL.Path, "/read") {
					reads.Add(1)
				}
				if r.Method == "DELETE" {
					deletes.Add(1)
				}
				copyCallerResponse(w, response, false)
			}))
			defer peer.Close()
			result, err := provider.New().Do(context.Background(), callerRequest(peer.URL+"/mount", callerOptions(), false))
			assertCallerControlError(t, result, err, "deferred_protocol_error", deferred.AcceptanceAccepted)
			if submits.Load() != 1 || reads.Load() != 0 || deletes.Load() != 1 || escaped.Load() != 0 {
				t.Fatalf("invalid Location followed or resubmitted: submits=%d reads=%d deletes=%d escapes=%d", submits.Load(), reads.Load(), deletes.Load(), escaped.Load())
			}
		})
	}
}

func TestDeferredCallerControlLossNeverDowngradesAcceptance(t *testing.T) {
	for _, code := range []string{"job_expired", "job_unavailable", "generation_mismatch", "job_released"} {
		t.Run(code, func(t *testing.T) {
			var calls, submits atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{}`)
			}))
			defer upstream.Close()
			f := newCallerPeerFixture(t, upstream.URL)
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/read") {
					deferredMarker(w)
					w.WriteHeader(410)
					_ = json.NewEncoder(w).Encode(deferred.ControlError{
						Envelope: deferred.NewEnvelope(), Acceptance: deferred.AcceptanceNotAccepted,
						Error: deferred.ErrorDetail{Type: code, Message: "retained job is unavailable"},
					})
					return
				}
				if r.URL.Path == deferred.Prefix+"/submit" {
					submits.Add(1)
				}
				f.handler.ServeHTTP(w, r)
			}))
			defer peer.Close()
			result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), false))
			assertCallerControlError(t, result, err, "deferred_request_lost", deferred.AcceptanceAccepted)
			if submits.Load() != 1 || calls.Load() > 1 {
				t.Fatal("accepted retrieval loss caused a fresh inference")
			}
		})
	}
}

func TestDeferredCallerOrdinaryAdmissionErrorHasNoProtocolBody(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("unexpected ultimate inference") }))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var submits atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == deferred.Prefix+"/submit" {
			submits.Add(1)
			deferredMarker(w)
			w.Header().Set("Retry-After", "3600")
			w.WriteHeader(429)
			_, _ = io.WriteString(w, `{"error":{"type":"quota_exceeded","message":"ordinary rejection","block_id":"rule-1"}}`)
			return
		}
		f.handler.ServeHTTP(w, r)
	}))
	defer peer.Close()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), false))
	defer closeCallerResult(result)
	if err != nil || result == nil || result.StatusCode != 429 || result.Origin != provider.OriginPeerControl || result.HasRetryDelay ||
		bytes.Contains(result.Body, []byte("protocol")) || !bytes.Contains(result.Body, []byte(`"block_id":"rule-1"`)) ||
		result.Header.Get("X-Aiproxy-Deferred-Version") != "" || result.Header.Get("Retry-After") != "" || submits.Load() != 1 {
		t.Fatalf("admission became provider success/retry: result=%+v err=%v", result, err)
	}
}

func TestDeferredCallerFailedCallRetryAndCallBudgetsIncludeDiscovery(t *testing.T) {
	for _, tc := range []struct {
		name     string
		retries  int
		calls    int
		expected int32
	}{
		{"zero-retries", 0, 4096, 1},
		{"global-retries", 2, 4096, 3},
		{"global-calls", 8, 4, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				deferredMarker(w)
				w.WriteHeader(503)
				_ = json.NewEncoder(w).Encode(deferred.ControlError{
					Envelope: deferred.NewEnvelope(), Acceptance: deferred.AcceptanceNotAccepted,
					Error: deferred.ErrorDetail{Type: "deferred_saturated", Message: "control pressure"},
				})
			}))
			defer peer.Close()
			options := callerOptions()
			options.MaxControlRetries, options.MaxControlCalls = tc.retries, tc.calls
			result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, options, false))
			assertCallerControlError(t, result, err, "deferred_control_error", deferred.AcceptanceNotAccepted)
			if calls.Load() != tc.expected {
				t.Fatalf("calls=%d want=%d", calls.Load(), tc.expected)
			}
		})
	}
}

func TestDeferredCallerLostAckWithoutRecoveryIsIndeterminate(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var submits atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, r)
		drop := r.URL.Path == deferred.Prefix+"/submit"
		if drop {
			submits.Add(1)
		}
		copyCallerResponse(w, response, drop)
	}))
	defer peer.Close()
	options := callerOptions()
	options.MaxControlRetries = 0
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, options, false))
	assertCallerControlError(t, result, err, "deferred_submit_indeterminate", deferred.AcceptanceIndeterminate)
	if submits.Load() != 1 {
		t.Fatal("zero retries was ignored for an ambiguous submit")
	}
}

func TestDeferredCallerWaitAndCancellationUseIndependentOneDelete(t *testing.T) {
	for _, timeout := range []bool{false, true} {
		t.Run(fmt.Sprintf("timeout=%t", timeout), func(t *testing.T) {
			started := make(chan struct{})
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				select {
				case <-r.Context().Done():
				case <-time.After(3 * time.Second):
					t.Error("ultimate provider cancellation did not arrive")
				}
			}))
			defer upstream.Close()
			f := newCallerPeerFixture(t, upstream.URL)
			var deletes atomic.Int32
			retrieval, retrieved := make(chan struct{}), &sync.Once{}
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/read") {
					retrieved.Do(func() { close(retrieval) })
				}
				if r.Method == "DELETE" {
					deletes.Add(1)
					if r.Context().Err() != nil || r.Header.Get("Authorization") != "Bearer peer-token" {
						t.Error("cleanup reused cancelled context or lost the pinned credential")
					}
				}
				f.handler.ServeHTTP(w, r)
			}))
			defer peer.Close()
			options := callerOptions()
			options.MaxWait = time.Second
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if !timeout {
				go func() {
					<-started
					<-retrieval
					cancel()
				}()
			}
			before := time.Now()
			result, err := provider.New().Do(ctx, callerRequest(peer.URL, options, false))
			code := "deferred_control_error"
			if timeout {
				code = "deferred_wait_timeout"
			}
			assertCallerControlError(t, result, err, code, deferred.AcceptanceAccepted)
			if deletes.Load() != 1 || time.Since(before) > 2*time.Second {
				t.Fatalf("cleanup was missing/unbounded: deletes=%d elapsed=%s", deletes.Load(), time.Since(before))
			}
		})
	}
}

func TestDeferredCallerSlowControlBodiesAreDeadlineBounded(t *testing.T) {
	var calls atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		deferredMarker(w)
		_, _ = io.WriteString(w, `{"protocol":`)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer peer.Close()
	options := callerOptions()
	options.ControlTimeout, options.MaxControlRetries = 100*time.Millisecond, 0
	before := time.Now()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, options, false))
	assertCallerControlError(t, result, err, "deferred_control_error", deferred.AcceptanceNotAccepted)
	if calls.Load() != 1 || time.Since(before) > time.Second {
		t.Fatal("control header timeout did not bound the whole slow body")
	}
}

func TestDeferredCallerNonChatAndNoOptInKeepOrdinaryTransport(t *testing.T) {
	for _, tc := range []struct {
		operation provider.Operation
		optIn     bool
		path      string
	}{
		{provider.OpChatCompletions, false, "/v1/chat/completions"},
		{provider.OpResponses, true, "/v1/responses"},
		{provider.OpEmbeddings, true, "/v1/embeddings"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			var calls atomic.Int32
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				if r.URL.Path != tc.path || r.Header.Get("Authorization") != "Bearer peer-token" {
					t.Errorf("ordinary request changed: %s %#v", r.URL.Path, r.Header)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(202)
				_, _ = io.WriteString(w, `{"ordinary":true}`)
			}))
			defer peer.Close()
			request := callerRequest(peer.URL, callerOptions(), false)
			request.Operation = tc.operation
			if !tc.optIn {
				request.DeferredTransport = nil
			}
			result, err := provider.New().Do(context.Background(), request)
			defer closeCallerResult(result)
			if err != nil || result.StatusCode != 202 || string(result.Body) != `{"ordinary":true}` || calls.Load() != 1 {
				t.Fatalf("ordinary202 mistaken for deferred ack: result=%+v err=%v", result, err)
			}
		})
	}
}

type callerOffsetClock struct{ offset atomic.Int64 }
type callerRealTimer struct{ timer *time.Timer }

func (c *callerOffsetClock) Now() time.Time { return time.Now().Add(time.Duration(c.offset.Load())) }
func (c *callerOffsetClock) NewTimer(d time.Duration) deferred.Timer {
	return callerRealTimer{time.NewTimer(d)}
}
func (t callerRealTimer) C() <-chan time.Time { return t.timer.C }
func (t callerRealTimer) Stop() bool          { return t.timer.Stop() }

func TestDeferredCallerExpiredSignedScopeHasZeroInferenceAndNoNewScope(t *testing.T) {
	var inference, capabilities, submits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { inference.Add(1) }))
	defer upstream.Close()
	clock := &callerOffsetClock{}
	f := newCallerPeerFixture(t, upstream.URL, deferred.Hooks{Clock: clock})
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, r)
		if strings.HasSuffix(r.URL.Path, "/capabilities") {
			capabilities.Add(1)
			clock.offset.Store(int64(deferred.SubmitWindow + time.Second))
		} else if strings.HasSuffix(r.URL.Path, "/submit") {
			submits.Add(1)
		}
		copyCallerResponse(w, response, false)
	}))
	defer peer.Close()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), false))
	assertCallerControlError(t, result, err, "deferred_control_error", deferred.AcceptanceNotAccepted)
	if inference.Load() != 0 || capabilities.Load() != 1 || submits.Load() != 1 {
		t.Fatalf("expired scope refreshed/resubmitted inference: inference=%d capabilities=%d submits=%d", inference.Load(), capabilities.Load(), submits.Load())
	}
}

func TestDeferredCallerRestartBetweenLostAckAndReplayFailsClosed(t *testing.T) {
	var inference, capabilities, submits atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inference.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	first := newCallerPeerFixture(t, upstream.URL)
	restarted := newCallerPeerFixture(t, upstream.URL)
	var active atomic.Pointer[httpapi.Handler]
	active.Store(first.handler)
	var mu sync.Mutex
	var wires [][]byte
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/capabilities") {
			capabilities.Add(1)
		}
		submit := strings.HasSuffix(r.URL.Path, "/submit")
		if submit {
			wire, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(wire))
			mu.Lock()
			wires = append(wires, wire)
			mu.Unlock()
		}
		response := httptest.NewRecorder()
		active.Load().ServeHTTP(response, r)
		drop := submit && submits.Add(1) == 1
		if drop {
			active.Store(restarted.handler)
		}
		copyCallerResponse(w, response, drop)
	}))
	defer peer.Close()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), false))
	assertCallerControlError(t, result, err, "deferred_submit_indeterminate", deferred.AcceptanceIndeterminate)
	mu.Lock()
	defer mu.Unlock()
	if capabilities.Load() != 1 || submits.Load() != 2 || inference.Load() > 1 || len(wires) != 2 || !bytes.Equal(wires[0], wires[1]) ||
		restarted.store.Stats().Jobs != 0 {
		t.Fatal("generation replacement admitted replay under a fresh scope/key")
	}
}

func TestDeferredCallerExpiredAcceptedResultIsLostNotNewInference(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	clock := &callerOffsetClock{}
	f := newCallerPeerFixture(t, upstream.URL, deferred.Hooks{Clock: clock})
	var submits atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/read") {
			deadline := time.Now().Add(time.Second)
			for f.store.Stats().Running != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			clock.offset.Store(int64(11 * time.Second))
			f.store.Sweep()
		} else if strings.HasSuffix(r.URL.Path, "/submit") {
			submits.Add(1)
		}
		f.handler.ServeHTTP(w, r)
	}))
	defer peer.Close()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), false))
	assertCallerControlError(t, result, err, "deferred_request_lost", deferred.AcceptanceAccepted)
	if calls.Load() != 1 || submits.Load() != 1 {
		t.Fatal("terminal retention expiry caused another inference/submit")
	}
}

func TestDeferredCallerOrdinarySubmit202AfterPositiveCapabilityIsIndeterminate(t *testing.T) {
	f := newCallerPeerFixture(t, "http://127.0.0.1:1")
	var submits, ordinary atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/submit") {
			submits.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(202)
			_, _ = io.WriteString(w, `{"id":"ordinary-202","status":"pending"}`)
			return
		}
		if r.Method == "POST" {
			ordinary.Add(1)
		}
		f.handler.ServeHTTP(w, r)
	}))
	defer peer.Close()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), false))
	assertCallerControlError(t, result, err, "deferred_submit_indeterminate", deferred.AcceptanceIndeterminate)
	if submits.Load() != 1 || ordinary.Load() != 0 || f.store.Stats().Jobs != 0 {
		t.Fatal("ordinary202 created a hidden synchronous fallback or new attempt")
	}
}

func TestDeferredCallerCancellationAfterHeadWithoutReadingStillDeletes(t *testing.T) {
	started := make(chan struct{})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(200)
		w.(http.Flusher).Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var deletes atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletes.Add(1)
		}
		f.handler.ServeHTTP(w, r)
	}))
	defer peer.Close()
	ctx, cancel := context.WithCancel(context.Background())
	result, err := provider.New().Do(ctx, callerRequest(peer.URL, callerOptions(), true))
	if err != nil || result == nil || !result.Streaming {
		cancel()
		closeCallerResult(result)
		t.Fatalf("stream head: %v", err)
	}
	<-started
	cancel()
	deadline := time.Now().Add(time.Second)
	for deletes.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	closeCallerResult(result)
	if deletes.Load() != 1 {
		t.Fatalf("cancelled idle consumer did not get one independent DELETE: %d", deletes.Load())
	}
}

type callerTestTransport func(*http.Request) (*http.Response, error)

func (f callerTestTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDeferredCallerLostAckFreezesTransportCredentialAndPreparedFingerprint(t *testing.T) {
	body := []byte(`{"vendor":"` + strings.Repeat("x", 80<<10) + `"}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var submits atomic.Int32
	var mu sync.Mutex
	var wires [][]byte
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer peer-token" {
			t.Error("pinned credential changed after submit")
		}
		submit := strings.HasSuffix(r.URL.Path, "/submit")
		if submit {
			wire, _ := io.ReadAll(r.Body)
			r.Body = io.NopCloser(bytes.NewReader(wire))
			mu.Lock()
			wires = append(wires, wire)
			mu.Unlock()
		}
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, r)
		copyCallerResponse(w, response, submit && submits.Add(1) == 1)
	}))
	defer peer.Close()
	options := callerOptions()
	request := callerRequest(peer.URL, options, false)
	client := peer.Client()
	inner := client.Transport
	var frozen bool
	client.Transport = callerTestTransport(func(r *http.Request) (*http.Response, error) {
		if r.Method == "POST" && !frozen {
			frozen = true
			options.MaxControlRetries, options.MaxControlCalls = 0, 4
			request.APIKey, request.BaseURL = "changed-token", "http://wrong-peer/v1"
			request.Inbound.Header.Set("User-Agent", "changed-agent")
			request.Inbound.Header.Set("X-Inference", "changed-inference")
			request.Body[0] = '['
			client.Transport = callerTestTransport(func(*http.Request) (*http.Response, error) {
				return nil, errors.New("reloaded transport must not be used")
			})
		}
		return inner.RoundTrip(r)
	})
	request.Client = client
	result, err := provider.New().Do(context.Background(), request)
	defer closeCallerResult(result)
	if err != nil || result == nil || !bytes.Equal(result.Body, body) || !frozen {
		t.Fatalf("qualified recovery adopted mutable caller state: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(wires) != 2 || !bytes.Equal(wires[0], wires[1]) || !strings.Contains(string(result.UpstreamRequestBody), `"model":"p/m"`) ||
		result.UpstreamRequestHeaders.Get("User-Agent") != "inference-client" || result.UpstreamRequestHeaders.Get("X-Inference") != "keep-me" {
		t.Fatal("lost acknowledgment refreshed its ticket/key/body/header fingerprint")
	}
}

func TestDeferredCallerCompletedSSEStillReturnsAfterOneBoundedFrame(t *testing.T) {
	body := []byte(": original " + strings.Repeat("x", 130<<10) + "\n\ndata: [DONE]\n\n")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var reads atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, r)
		if strings.HasSuffix(r.URL.Path, "/submit") {
			deadline := time.Now().Add(time.Second)
			for f.store.Stats().Running != 0 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
		} else if strings.HasSuffix(r.URL.Path, "/read") {
			reads.Add(1)
		}
		copyCallerResponse(w, response, false)
	}))
	defer peer.Close()
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, callerOptions(), true))
	defer closeCallerResult(result)
	if err != nil || result == nil || !result.Streaming || reads.Load() != 1 {
		t.Fatalf("completed SSE accumulated a whole unary replay buffer before returning: err=%v reads=%d", err, reads.Load())
	}
	delivered, err := io.ReadAll(result.StreamBody)
	if err != nil || !bytes.Equal(delivered, body) {
		t.Fatal("completed SSE lost exact incremental retained bytes")
	}
}

func TestDeferredCallerAdvertisedTimeoutBoundsWholeReadTransfer(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var readStarted atomic.Int64
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, r)
		if strings.HasSuffix(r.URL.Path, "/read") {
			readStarted.Store(time.Now().UnixNano())
			for name, values := range response.Header() {
				w.Header()[name] = values
			}
			w.WriteHeader(response.Code)
			_, _ = w.Write(response.Body.Bytes()[:1])
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			return
		}
		copyCallerResponse(w, response, false)
	}))
	defer peer.Close()
	options := callerOptions()
	options.ControlTimeout, options.MaxControlRetries = 500*time.Millisecond, 0
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, options, false))
	assertCallerControlError(t, result, err, "deferred_control_error", deferred.AcceptanceAccepted)
	elapsed := time.Since(time.Unix(0, readStarted.Load()))
	if readStarted.Load() == 0 || elapsed < 100*time.Millisecond || elapsed > 400*time.Millisecond {
		t.Fatalf("advertised 200ms control bound did not bound the whole body (configured 500ms): %s", elapsed)
	}
}

func TestDeferredCallerRetryBudgetIsSharedAcrossAckAndReadRecovery(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer upstream.Close()
	f := newCallerPeerFixture(t, upstream.URL)
	var submits, reads, deletes atomic.Int32
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := httptest.NewRecorder()
		f.handler.ServeHTTP(response, r)
		submit, read := strings.HasSuffix(r.URL.Path, "/submit"), strings.HasSuffix(r.URL.Path, "/read")
		if read {
			reads.Add(1)
		}
		if r.Method == "DELETE" {
			deletes.Add(1)
		}
		copyCallerResponse(w, response, submit && submits.Add(1) == 1 || read)
	}))
	defer peer.Close()
	options := callerOptions()
	options.MaxControlRetries = 1
	result, err := provider.New().Do(context.Background(), callerRequest(peer.URL, options, false))
	assertCallerControlError(t, result, err, "deferred_control_error", deferred.AcceptanceAccepted)
	if submits.Load() != 2 || reads.Load() != 1 || deletes.Load() != 1 {
		t.Fatalf("retry budgets restarted per phase/poll or cleanup consumed them: submits=%d reads=%d deletes=%d", submits.Load(), reads.Load(), deletes.Load())
	}
}
