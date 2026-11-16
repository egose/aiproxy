package e2e

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
)

func deferredMixedChainExecutorConfig(upstream string) string {
	return strings.NewReplacer(
		`  client "peer" { token = "peer-token" }`, `
  client "peer" {
    token = "peer-token"
    allowed_models = ["p/model-1", "p/model-2"]
  }
  client "next" {
    token = "next-token"
    allowed_models = ["p/model-1", "p/model-2"]
  }`,
		`execution_timeout = "2s"`, `execution_timeout = "5s"`,
		`  model "m" { upstream_name = "actual-model" }`, `
  model "model-1" { upstream_name = "actual-model-1" }
  model "model-2" { upstream_name = "actual-model-2" }`,
	).Replace(deferredExecutorConfig(upstream))
}

func deferredMixedChainOptions() *config.DeferredTransport {
	options := config.DefaultDeferredTransport()
	options.ControlTimeout, options.MaxWait, options.PollInterval = 200*time.Millisecond, 3*time.Second, 50*time.Millisecond
	options.MaxControlCalls = 128
	return &options
}

func deferredMixedChainBlock(options *config.DeferredTransport) string {
	if options == nil {
		return ""
	}
	return fmt.Sprintf(`
    deferred_transport {
      control_timeout = %q
      max_wait = %q
      poll_interval = %q
      max_control_calls = %d
      max_control_retries = %d
    }`, options.ControlTimeout.String(), options.MaxWait.String(), options.PollInterval.String(), options.MaxControlCalls, options.MaxControlRetries)
}

func deferredMixedChainCallerConfig(peer, token string, first, second *config.DeferredTransport, deferredFirst bool) string {
	targets := []string{"model-1", "model-2"}
	if deferredFirst {
		targets[0], targets[1] = targets[1], targets[0]
	}
	return fmt.Sprintf(`
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
metrics { token = "metrics-token" }
provider "openai-compatible" "a" {
  base_url = %q
  api_key = %q
  forward_user_agent = true
  forward_headers = ["x-inference"]
  model "model-1" {
    upstream_name = "p/model-1"
    %s
  }
  model "model-2" {
    upstream_name = "p/model-2"
    %s
  }
}
alias "mixed" {
  algorithm = "round_robin"
  retry_status_codes = ["400", "429", "500", "502", "503", "504"]
  target {
    provider = "a"
    model = %q
  }
  target {
    provider = "a"
    model = %q
  }
}
alias "first" {
  algorithm = "round_robin"
  target {
    provider = "a"
    model = "model-1"
  }
}
alias "second" {
  algorithm = "round_robin"
  target {
    provider = "a"
    model = "model-2"
  }
}
`, peer+"/v1", token, deferredMixedChainBlock(first), deferredMixedChainBlock(second), targets[0], targets[1])
}

type deferredMixedChainCall struct {
	method, path, query string
	body                []byte
}

type deferredMixedChainPeer struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []deferredMixedChainCall
}

func newDeferredMixedChainPeer(t *testing.T, target, token string, modify func(*http.Response) error) *deferredMixedChainPeer {
	t.Helper()
	origin, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	proxy := httputil.NewSingleHostReverseProxy(origin)
	proxy.Transport, proxy.FlushInterval, proxy.ModifyResponse = transport, -1, modify
	proxy.ErrorLog = log.New(io.Discard, "", 0)
	peer := &deferredMixedChainPeer{}
	peer.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		peer.mu.Lock()
		peer.calls = append(peer.calls, deferredMixedChainCall{r.Method, r.URL.Path, r.URL.RawQuery, bytes.Clone(body)})
		peer.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("actual peer lost its configured credential on %s", r.URL.Path)
		}
		if strings.HasPrefix(r.URL.Path, deferred.Prefix) {
			if r.Header.Get("X-Aiproxy-Deferred-Version") != "1" || r.Header.Get("Accept") != deferred.MediaType {
				t.Errorf("actual private control lost version/media markers: %s", r.URL.Path)
			}
		} else {
			for name := range r.Header {
				if config.IsDeferredHeader(name) {
					t.Errorf("ordinary sibling forwarded a private header: %s", name)
				}
			}
		}
		proxy.ServeHTTP(w, r)
	}))
	t.Cleanup(func() {
		peer.server.Close()
		transport.CloseIdleConnections()
	})
	return peer
}

func (p *deferredMixedChainPeer) Calls(suffix string) []deferredMixedChainCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []deferredMixedChainCall
	for _, call := range p.calls {
		if suffix == "" || strings.HasSuffix(call.path, suffix) {
			call.body = bytes.Clone(call.body)
			out = append(out, call)
		}
	}
	return out
}

func (p *deferredMixedChainPeer) PrivateCalls() int {
	count := 0
	for _, call := range p.Calls("") {
		if strings.HasPrefix(call.path, deferred.Prefix) {
			count++
		}
	}
	return count
}

func deferredMixedChainResponse(model, path string, stream bool) []byte {
	if stream {
		return []byte(": original\r\nid: " + model + "\r\nevent: vendor\r\ndata: {\"path\":\"" + path + "\",\"vendor\":\"kept\"}\r\n\r\n" +
			"data: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\ndata: [DONE]\n\n")
	}
	field := "choices"
	if path == "/v1/responses" {
		field = "output"
	} else if path == "/v1/embeddings" {
		field = "data"
	}
	return []byte(" \n" + fmt.Sprintf(`{"model":%q,"path":%q,%q:[],"vendor":"kept","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`, model, path, field) + "\n")
}

func deferredMixedChainInput(model, path string, stream bool) []byte {
	if path == "/v1/chat/completions" {
		return []byte(fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[{"role":"user","content":"test"}],"vendor":{"unchanged":true}}`, model, stream))
	}
	if path == "/v1/embeddings" {
		return []byte(fmt.Sprintf(`{"model":%q,"input":"test","vendor":{"unchanged":true}}`, model))
	}
	return []byte(fmt.Sprintf(`{"model":%q,"stream":%t,"input":"test","vendor":{"unchanged":true}}`, model, stream))
}

type deferredMixedChainReply struct {
	status int
	header http.Header
	body   []byte
	err    error
}

func deferredMixedChainRequest(ctx context.Context, base, path string, body []byte) (*http.Request, error) {
	request, err := http.NewRequestWithContext(ctx, "POST", base+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer end-client-token")
	request.Header.Set("User-Agent", "chain-client")
	request.Header.Set("X-Inference", "chain-inference")
	request.Header.Set("X-Aiproxy-Deferred-Version", "99")
	return request, nil
}

func deferredMixedChainHTTP(ctx context.Context, server *httptest.Server, path string, body []byte) deferredMixedChainReply {
	request, err := deferredMixedChainRequest(ctx, server.URL, path, body)
	if err != nil {
		return deferredMixedChainReply{err: err}
	}
	response, err := server.Client().Do(request)
	if err != nil {
		return deferredMixedChainReply{err: err}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	return deferredMixedChainReply{response.StatusCode, response.Header.Clone(), data, err}
}

func requireNoDeferredMixedChainEnvelope(t *testing.T, reply deferredMixedChainReply) {
	t.Helper()
	for name := range reply.header {
		if config.IsDeferredHeader(name) {
			t.Fatalf("private response header escaped to the ordinary client: %s", name)
		}
	}
	for _, marker := range []string{deferred.Protocol, `"submit_token"`, `"scope_digest"`, `"next_cursor"`, `"body_sha256"`} {
		if bytes.Contains(reply.body, []byte(marker)) {
			t.Fatalf("private envelope escaped to the ordinary client: %s", marker)
		}
	}
}

func readDeferredMixedChainControl(response *http.Response) ([]byte, error) {
	wire, err := io.ReadAll(io.LimitReader(response.Body, deferred.MaxControlBodyBytes+1))
	_ = response.Body.Close()
	if err != nil {
		return nil, err
	}
	if len(wire) > deferred.MaxControlBodyBytes {
		return nil, fmt.Errorf("fixture control exceeded finite body limit")
	}
	response.Body = io.NopCloser(bytes.NewReader(wire))
	return wire, nil
}

func TestDeferredMixedModelPeerChainUsesDistinctTransports(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", target, stream), func(t *testing.T) {
				var firstCalls, secondCalls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					input, _ := io.ReadAll(r.Body)
					var request struct {
						Model  string
						Stream bool
					}
					if json.Unmarshal(input, &request) != nil {
						t.Error("two-app input is not JSON")
					}
					switch request.Model {
					case "actual-model-1":
						firstCalls.Add(1)
					case "actual-model-2":
						secondCalls.Add(1)
					default:
						t.Errorf("two-app model rewrite changed: %s", input)
					}
					if r.Header.Get("Authorization") != "Bearer ultimate-secret" || r.UserAgent() != "chain-client" ||
						r.Header.Get("X-Inference") != "chain-inference" || !bytes.Contains(input, []byte(`"vendor":{"unchanged":true}`)) {
						t.Errorf("two-app inference lost configured credentials or metadata: %s %#v", input, r.Header)
					}
					for name := range r.Header {
						if config.IsDeferredHeader(name) {
							t.Errorf("private control header reached ultimate inference: %s", name)
						}
					}
					contentType := "application/json"
					if request.Stream {
						contentType = "text/event-stream"
					}
					w.Header().Set("Content-Type", contentType)
					w.Header().Set("X-Vendor", "kept")
					w.Header().Set("X-Aiproxy-Deferred-Upstream", "must-not-escape")
					w.WriteHeader(202)
					_, _ = w.Write(deferredMixedChainResponse(request.Model, r.URL.Path, request.Stream))
				}))
				t.Cleanup(upstream.Close)
				executor, executorServer := newDeferredChainApp(t, writeConfig(t, deferredMixedChainExecutorConfig(upstream.URL)))
				peer := newDeferredMixedChainPeer(t, executorServer.URL, "peer-token", nil)
				caller, callerServer := newDeferredChainApp(t, writeConfig(t, deferredMixedChainCallerConfig(peer.server.URL, "peer-token", nil, deferredMixedChainOptions(), false)))
				if caller.DeferredStore() != nil || len(caller.Config.Catalog.Providers()) != 1 || len(executor.Config.Catalog.Providers()) != 1 {
					t.Fatal("mixed-model chain must use two actual apps with one provider each and no caller store")
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				for i, name := range []string{"model-1", "model-2"} {
					model := "a/" + name
					if target == "alias" {
						model = "alias/mixed"
					}
					reply := deferredMixedChainHTTP(ctx, callerServer, "/v1/chat/completions", deferredMixedChainInput(model, "/v1/chat/completions", stream))
					want := deferredMixedChainResponse(fmt.Sprintf("actual-model-%d", i+1), "/v1/chat/completions", stream)
					requireNoDeferredMixedChainEnvelope(t, reply)
					if reply.status != 202 || reply.err != nil || !bytes.Equal(reply.body, want) || reply.header.Get("X-Vendor") != "kept" {
						t.Fatalf("mixed-model chat bytes changed: status=%d err=%v body=%q", reply.status, reply.err, reply.body)
					}
					if i == 0 && (peer.PrivateCalls() != 0 || firstCalls.Load() != 1 || secondCalls.Load() != 0) {
						t.Fatal("ordinary sibling used private controls or a different inference")
					}
				}
				if firstCalls.Load() != 1 || secondCalls.Load() != 1 || len(peer.Calls("/v1/chat/completions")) != 1 ||
					len(peer.Calls("/capabilities")) != 1 || len(peer.Calls("/submit")) != 1 ||
					len(peer.Calls("/read")) == 0 || len(peer.Calls("/result")) != 1 {
					t.Fatal("deferred sibling duplicated inference, used synchronous fallback or lost private delivery")
				}
				submit := peer.Calls("/submit")[0]
				prepared, err := deferred.DecodeSubmit(submit.body)
				if err != nil || !bytes.Contains(prepared.Body, []byte(`"model":"p/model-2"`)) ||
					prepared.Headers.Get("Authorization") != "" || prepared.Headers.Get("X-Aiproxy-Deferred-Version") != "" {
					t.Fatalf("two-app submission lost model rewriting or leaked private credentials/headers: %v", err)
				}
				for name := range prepared.Headers {
					if config.IsDeferredHeader(name) {
						t.Fatalf("private header entered prepared two-app inference: %s", name)
					}
				}
				awaitChain(t, "mixed-model private cleanup", func() bool {
					for _, call := range peer.Calls("") {
						if call.method == http.MethodDelete {
							return executor.DeferredStore().Stats().Jobs == 0
						}
					}
					return false
				})
				privateBefore := peer.PrivateCalls()
				for _, operation := range []struct {
					path          string
					stream        bool
					ordinaryCalls int
				}{{"/v1/responses", false, 1}, {"/v1/responses", true, 2}, {"/v1/embeddings", false, 1}} {
					model := "a/model-2"
					if target == "alias" {
						model = "alias/second"
					}
					reply := deferredMixedChainHTTP(ctx, callerServer, operation.path, deferredMixedChainInput(model, operation.path, operation.stream))
					requireNoDeferredMixedChainEnvelope(t, reply)
					if reply.status != 202 || reply.err != nil || !bytes.Equal(reply.body, deferredMixedChainResponse("actual-model-2", operation.path, operation.stream)) ||
						peer.PrivateCalls() != privateBefore || len(peer.Calls(operation.path)) != operation.ordinaryCalls {
						t.Fatalf("opted model used private transport or changed ordinary %s output: %d %q %v", operation.path, reply.status, reply.body, reply.err)
					}
				}
				if firstCalls.Load() != 1 || secondCalls.Load() != 4 {
					t.Fatal("mixed/non-chat logical requests duplicated inference")
				}
				awaitChain(t, "mixed-model executor release", func() bool {
					s := executor.DeferredStore().Stats()
					return s.Jobs == 0 && s.Running == 0 && s.FinalizationCredits == 0 && s.Controls == 0
				})
			})
		}
	}
}

func TestDeferredMixedModelPeerChainFailuresStayPinned(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, fault := range []string{"protocol", "admission", "read-loss"} {
			t.Run(target+"/"+fault, func(t *testing.T) {
				var calls atomic.Int32
				started := make(chan struct{})
				observe := sync.OnceFunc(func() { close(started) })
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					observe()
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write(deferredMixedChainResponse("actual-model-2", r.URL.Path, false))
				}))
				t.Cleanup(upstream.Close)
				executorConfig := deferredMixedChainExecutorConfig(upstream.URL)
				if fault == "admission" {
					executorConfig += "\ningress_guardrails {\n enabled = true\n mode = \"block\"\n}\n"
				}
				executor, executorServer := newDeferredChainApp(t, writeConfig(t, executorConfig))
				peer := newDeferredMixedChainPeer(t, executorServer.URL, "peer-token", func(response *http.Response) error {
					if fault == "protocol" && strings.HasSuffix(response.Request.URL.Path, "/capabilities") {
						response.Header.Set("X-Aiproxy-Deferred-Version", "2")
					}
					if fault != "read-loss" || !strings.HasSuffix(response.Request.URL.Path, "/read") {
						return nil
					}
					if _, err := readDeferredMixedChainControl(response); err != nil {
						return err
					}
					select {
					case <-started:
					case <-response.Request.Context().Done():
						return response.Request.Context().Err()
					}
					wire, err := json.Marshal(deferred.ControlError{
						Envelope: deferred.NewEnvelope(), Acceptance: deferred.AcceptanceAccepted,
						Error: deferred.ErrorDetail{Type: "job_unavailable", Message: "fixture lost accepted result"},
					})
					if err != nil {
						return err
					}
					response.StatusCode, response.ContentLength = 410, int64(len(wire))
					response.Header.Set("Content-Length", strconv.Itoa(len(wire)))
					response.Body = io.NopCloser(bytes.NewReader(wire))
					return nil
				})
				options := deferredMixedChainOptions()
				options.MaxControlRetries = 0
				_, callerServer := newDeferredChainApp(t, writeConfig(t, deferredMixedChainCallerConfig(peer.server.URL, "peer-token", nil, options, true)))
				model := "a/model-2"
				if target == "alias" {
					model = "alias/mixed"
				}
				input := deferredMixedChainInput(model, "/v1/chat/completions", false)
				if fault == "admission" {
					secret := "AKIA" + rand.Text()[:16]
					input = bytes.Replace(input, []byte(`"content":"test"`), []byte(`"content":"deploy with `+secret+` now"`), 1)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
				defer cancel()
				reply := deferredMixedChainHTTP(ctx, callerServer, "/v1/chat/completions", input)
				requireNoDeferredMixedChainEnvelope(t, reply)
				status, code, submissions, inferences := 502, "deferred_protocol_error", 0, int32(0)
				if fault == "admission" {
					status, code, submissions = 400, "secret_blocked", 1
				} else if fault == "read-loss" {
					code, submissions, inferences = "deferred_request_lost", 1, 1
				}
				if reply.status != status || reply.err != nil || !bytes.Contains(reply.body, []byte(code)) ||
					calls.Load() != inferences || len(peer.Calls("/capabilities")) != 1 || len(peer.Calls("/submit")) != submissions ||
					len(peer.Calls("/v1/chat/completions")) != 0 {
					t.Fatalf("two-app %s failure started fresh/synchronous/alias inference: status=%d body=%s err=%v inference=%d", fault, reply.status, reply.body, reply.err, calls.Load())
				}
				awaitChain(t, "failed mixed-model executor release", func() bool {
					s := executor.DeferredStore().Stats()
					return s.Jobs == 0 && s.Running == 0 && s.FinalizationCredits == 0
				})
			})
		}
	}
}

func TestDeferredMixedModelPeerChainReloadPinsAdmittedOptions(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", target, stream), func(t *testing.T) {
				var calls atomic.Int32
				gate, observed := make(chan struct{}), make(chan struct{})
				release := sync.OnceFunc(func() { close(gate) })
				observe := sync.OnceFunc(func() { close(observed) })
				prefix := []byte("id: old\ndata: {\"choices\":[]}\n\n")
				oldBody := deferredMixedChainResponse("old-admitted", "/v1/chat/completions", stream)
				newBody := deferredMixedChainResponse("new-ordinary", "/v1/chat/completions", false)
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					input, _ := io.ReadAll(r.Body)
					if !bytes.Contains(input, []byte(`"model":"actual-model-2"`)) || r.Header.Get("Authorization") != "Bearer ultimate-secret" {
						t.Errorf("reloaded chain changed ultimate admitted model/credential: %s", input)
					}
					if calls.Add(1) != 1 {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write(newBody)
						return
					}
					if stream {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = w.Write(prefix)
						w.(http.Flusher).Flush()
					}
					select {
					case <-gate:
						if !stream {
							w.Header().Set("Content-Type", "application/json")
						}
						_, _ = w.Write(oldBody)
					case <-r.Context().Done():
					}
				}))
				t.Cleanup(upstream.Close)
				t.Cleanup(release)
				executor, executorServer := newDeferredChainApp(t, writeConfig(t, deferredMixedChainExecutorConfig(upstream.URL)))
				var updated, dropped atomic.Bool
				var droppedQuery atomic.Value
				oldPeer := newDeferredMixedChainPeer(t, executorServer.URL, "peer-token", func(response *http.Response) error {
					if !strings.HasSuffix(response.Request.URL.Path, "/read") {
						return nil
					}
					if !stream {
						observe()
					}
					if !updated.Load() {
						return nil
					}
					wire, err := readDeferredMixedChainControl(response)
					if err != nil {
						return err
					}
					var frame deferred.ReadFrame
					if deferred.DecodeControl(wire, &frame) == nil && len(frame.Data) > 0 && dropped.CompareAndSwap(false, true) {
						droppedQuery.Store(response.Request.URL.RawQuery)
						response.ContentLength = int64(len(wire))
						response.Header.Set("Content-Length", strconv.Itoa(len(wire)))
						response.Body = io.NopCloser(bytes.NewReader(wire[:len(wire)/2]))
					}
					return nil
				})
				nextPeer := newDeferredMixedChainPeer(t, executorServer.URL, "next-token", func(response *http.Response) error {
					if !strings.HasSuffix(response.Request.URL.Path, "/capabilities") {
						return nil
					}
					if response.Request.URL.Query().Get("model") != "p/model-1" {
						t.Error("new two-app model policy negotiated the wrong public model")
					}
					if _, err := readDeferredMixedChainControl(response); err != nil {
						return err
					}
					wire, err := json.Marshal(deferred.ControlError{
						Envelope: deferred.NewEnvelope(), Acceptance: deferred.AcceptanceNotAccepted,
						Error: deferred.ErrorDetail{Type: "deferred_saturated", Message: "fixture control pressure"},
					})
					if err != nil {
						return err
					}
					response.StatusCode, response.ContentLength = 503, int64(len(wire))
					response.Header.Set("Content-Length", strconv.Itoa(len(wire)))
					response.Body = io.NopCloser(bytes.NewReader(wire))
					return nil
				})
				path := writeConfig(t, deferredMixedChainCallerConfig(oldPeer.server.URL, "peer-token", nil, deferredMixedChainOptions(), false))
				caller, callerServer := newDeferredChainApp(t, path)
				model := "a/model-2"
				if target == "alias" {
					model = "alias/second"
				}
				replies := make(chan deferredMixedChainReply, 1)
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				go func() {
					request, err := deferredMixedChainRequest(ctx, callerServer.URL, "/v1/chat/completions", deferredMixedChainInput(model, "/v1/chat/completions", stream))
					if err != nil {
						replies <- deferredMixedChainReply{err: err}
						return
					}
					response, err := callerServer.Client().Do(request)
					if err != nil {
						replies <- deferredMixedChainReply{err: err}
						return
					}
					defer response.Body.Close()
					var body []byte
					if stream {
						body = make([]byte, len(prefix))
						if _, err = io.ReadFull(response.Body, body); err != nil || !bytes.Equal(body, prefix) {
							replies <- deferredMixedChainReply{err: fmt.Errorf("pre-reload prefix: %q %v", body, err)}
							return
						}
						observe()
					}
					tail, err := io.ReadAll(response.Body)
					replies <- deferredMixedChainReply{response.StatusCode, response.Header.Clone(), append(body, tail...), err}
				}()
				select {
				case <-observed:
				case <-time.After(3 * time.Second):
					t.Fatal("two-app request did not admit/commit before reload")
				}
				awaitChain(t, "admitted mixed-model upstream inference", func() bool { return calls.Load() == 1 })
				next := deferredMixedChainOptions()
				next.ControlTimeout, next.MaxWait, next.MaxControlCalls, next.MaxControlRetries = 100*time.Millisecond, time.Second, 4, 0
				if err := os.WriteFile(path, []byte(deferredMixedChainCallerConfig(nextPeer.server.URL, "next-token", next, nil, false)), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := caller.Reload(); err != nil {
					t.Fatal(err)
				}
				updated.Store(true)
				first, second := "a/model-1", "a/model-2"
				if target == "alias" {
					first, second = "alias/first", "alias/second"
				}
				ordinary := deferredMixedChainHTTP(ctx, callerServer, "/v1/chat/completions", deferredMixedChainInput(second, "/v1/chat/completions", false))
				if ordinary.status != 200 || ordinary.err != nil || !bytes.Equal(ordinary.body, newBody) {
					t.Fatalf("new two-app request retained removed model deferral: %d %q %v", ordinary.status, ordinary.body, ordinary.err)
				}
				failure := deferredMixedChainHTTP(ctx, callerServer, "/v1/chat/completions", deferredMixedChainInput(first, "/v1/chat/completions", false))
				if failure.status != 502 || failure.err != nil || !bytes.Contains(failure.body, []byte("deferred_control_error")) ||
					len(nextPeer.Calls("/capabilities")) != 1 || len(nextPeer.Calls("/submit")) != 0 || len(nextPeer.Calls("/v1/chat/completions")) != 1 {
					t.Fatal("new two-app model policy ignored zero retries or fell back")
				}
				time.Sleep(1100 * time.Millisecond)
				release()
				select {
				case reply := <-replies:
					want := oldBody
					if stream {
						want = append(bytes.Clone(prefix), oldBody...)
					}
					requireNoDeferredMixedChainEnvelope(t, reply)
					if reply.status != 200 || reply.err != nil || !bytes.Equal(reply.body, want) {
						t.Fatalf("admitted two-app delivery adopted new options/peer/credential: %d %q %v", reply.status, reply.body, reply.err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("admitted two-app delivery did not finish")
				}
				requireNoDeferredMixedChainEnvelope(t, ordinary)
				requireNoDeferredMixedChainEnvelope(t, failure)
				retried := 0
				for _, read := range oldPeer.Calls("/read") {
					if read.query == droppedQuery.Load() {
						retried++
					}
				}
				if !dropped.Load() || retried < 2 || calls.Load() != 2 ||
					len(oldPeer.Calls("/capabilities")) != 1 || len(oldPeer.Calls("/submit")) != 1 || len(oldPeer.Calls("/v1/chat/completions")) != 0 {
					t.Fatal("admitted two-app request lost old retry options/cursor or started another inference")
				}
				awaitChain(t, "reloaded mixed-model executor release", func() bool {
					s := executor.DeferredStore().Stats()
					return s.Jobs == 0 && s.Running == 0 && s.FinalizationCredits == 0
				})
			})
		}
	}
}
