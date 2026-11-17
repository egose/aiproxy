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
	"strconv"
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

func callerMixedModelRuntime(peer string) *config.Runtime {
	options := config.DefaultDeferredTransport()
	options.ControlTimeout, options.MaxWait, options.PollInterval = 200*time.Millisecond, 3*time.Second, 50*time.Millisecond
	return &config.Runtime{Catalog: config.NewCatalog([]config.Provider{{
		Type: config.ProviderTypeOpenAICompatible, Name: "a", BaseURL: peer + "/v1", APIKey: "peer-token",
		Models: []config.Model{
			{Name: "model-1", UpstreamName: "p/model-1"},
			{Name: "model-2", UpstreamName: "p/model-2", DeferredTransport: &options},
		},
	}}, nil, []config.Alias{{
		Name: "mixed", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{429},
		Targets: []config.AliasTarget{{Provider: "a", Model: "model-1"}, {Provider: "a", Model: "model-2"}},
	}})}
}

func setDeferredMixedModelUpstream(f *deferredHTTPFixture, upstream string) {
	p, _ := f.rt.Catalog.Provider("p")
	p.BaseURL = upstream + "/v1"
	p.Models = []config.Model{
		{Name: "model-1", UpstreamName: "actual-1"},
		{Name: "model-2", UpstreamName: "actual-2"},
	}
	p.ModelByName = nil
	f.rt.Catalog = config.NewCatalog([]config.Provider{p}, nil, nil)
	deps := f.h.current()
	deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
	f.h.UpdateDependencies(deps)
}

type deferredModelPeerCounts struct {
	capabilities atomic.Int32
	submits      atomic.Int32
	reads        atomic.Int32
	manifests    atomic.Int32
	deletes      atomic.Int32
	ordinary     atomic.Int32
}

func newDeferredModelPeer(t *testing.T, f *deferredHTTPFixture) (*httptest.Server, *deferredModelPeerCounts) {
	t.Helper()
	counts := &deferredModelPeerCounts{}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer peer-token" {
			t.Errorf("peer request lost its configured credential: %s", r.URL.Path)
		}
		route, _, _ := deferredRoute(r.URL.Path, r.Method)
		switch route {
		case "capabilities":
			counts.capabilities.Add(1)
		case "submit":
			counts.submits.Add(1)
		case "read":
			counts.reads.Add(1)
		case "result":
			counts.manifests.Add(1)
		case "delete":
			counts.deletes.Add(1)
		default:
			if r.URL.Path != "/v1/chat/completions" {
				t.Errorf("unexpected peer endpoint: %s", r.URL.Path)
			}
			counts.ordinary.Add(1)
		}
		f.h.ServeHTTP(w, r)
	}))
	t.Cleanup(peer.Close)
	return peer, counts
}

func deferredModelResponse(model string, streaming bool) string {
	if streaming {
		return ": original\r\nid: " + model + "\r\nevent: vendor\r\ndata: {\"vendor\":\"kept\"}\r\n\r\n" +
			"data: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\ndata: [DONE]\n\n"
	}
	return " \n" + `{"model":"` + model + `","choices":[],"vendor":"kept","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}` + "\n"
}

func TestDeferredCallerMixedModelsSelectDistinctTransports(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", target, streaming), func(t *testing.T) {
				var firstCalls, secondCalls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					input, _ := io.ReadAll(r.Body)
					model := extractModel(input)
					switch model {
					case "actual-1":
						firstCalls.Add(1)
					case "actual-2":
						secondCalls.Add(1)
					default:
						t.Errorf("wrong ultimate model rewrite: %s", input)
						w.WriteHeader(400)
						return
					}
					if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer model-secret" ||
						!bytes.Contains(input, []byte(`"vendor":{"keep":true}`)) {
						t.Errorf("ordinary inference metadata changed: %s %#v", input, r.Header)
					}
					for name := range r.Header {
						if config.IsDeferredHeader(name) {
							t.Errorf("private control header reached inference: %s", name)
						}
					}
					contentType := "application/json"
					if streaming {
						contentType = "text/event-stream"
					}
					w.Header().Set("Content-Type", contentType)
					w.Header().Set("X-Vendor", "kept")
					w.WriteHeader(202)
					_, _ = io.WriteString(w, deferredModelResponse(model, streaming))
				}))
				t.Cleanup(upstream.Close)
				executor := newDeferredHTTPFixture(t, provider.New())
				setDeferredMixedModelUpstream(executor, upstream.URL)
				peer, counts := newDeferredModelPeer(t, executor)
				h, recorder, health := callerDispatchHandler(callerMixedModelRuntime(peer.URL), provider.New())
				t.Cleanup(func() { _ = health.Close() })
				for i, model := range []string{"model-1", "model-2"} {
					publicModel := "a/" + model
					if target == "alias" {
						publicModel = "alias/mixed"
					}
					request := httptest.NewRequest("POST", "/v1/chat/completions",
						strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[],"vendor":{"keep":true}}`, publicModel, streaming)))
					request.Header.Set("Authorization", "Bearer inbound-client-token")
					request.Header.Set("X-Aiproxy-Deferred-Submit-Token", "never-forward")
					w := httptest.NewRecorder()
					h.ServeHTTP(w, request)
					expectedType := "application/json"
					if streaming {
						expectedType = "text/event-stream"
					}
					if w.Code != 202 || w.Header().Get("Content-Type") != expectedType || w.Header().Get("X-Vendor") != "kept" ||
						w.Header().Get("X-Aiproxy-Deferred-Version") != "" || w.Body.String() != deferredModelResponse(fmt.Sprintf("actual-%d", i+1), streaming) {
						t.Fatalf("%s inference changed: status=%d headers=%v body=%q", model, w.Code, w.Header(), w.Body.String())
					}
					if i == 0 {
						if firstCalls.Load() != 1 || secondCalls.Load() != 0 || counts.ordinary.Load() != 1 ||
							counts.capabilities.Load() != 0 || counts.submits.Load() != 0 || counts.reads.Load() != 0 || counts.deletes.Load() != 0 {
							t.Fatal("ordinary sibling used private transport or selected another model")
						}
					} else if firstCalls.Load() != 1 || secondCalls.Load() != 1 || counts.ordinary.Load() != 1 ||
						counts.capabilities.Load() != 1 || counts.submits.Load() != 1 || counts.reads.Load() == 0 || counts.manifests.Load() != 1 || counts.deletes.Load() != 1 {
						t.Fatalf("model opt-in did not complete one peer inference without fallback: ordinary=%d capabilities=%d submits=%d reads=%d manifests=%d deletes=%d",
							counts.ordinary.Load(), counts.capabilities.Load(), counts.submits.Load(), counts.reads.Load(), counts.manifests.Load(), counts.deletes.Load())
					}
				}
				awaitDeferredHTTP(t, "mixed-model executor finalization", func() bool { return executor.store.Stats().FinalizationCredits == 0 })
				if len(recorder.Events()) != 2 || len(executor.accounting.Events()) != 2 || executor.rate.calls.Load() != 2 || !health.IsHealthy("a") {
					t.Fatal("peer controls duplicated logical inference or changed shared provider health")
				}
				for _, event := range recorder.Events() {
					if event.TotalTokens != 5 {
						t.Fatal("mixed-model delivery lost inference usage")
					}
				}
			})
		}
	}
}

func TestDeferredCallerMixedModelAliasFailoverAndCooldownStayPerTarget(t *testing.T) {
	for _, deferredFirst := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("deferred-first=%t/stream=%t", deferredFirst, streaming), func(t *testing.T) {
				failedModel, successModel := "actual-1", "actual-2"
				failedName, successName := "model-1", "model-2"
				if deferredFirst {
					failedModel, successModel = successModel, failedModel
					failedName, successName = successName, failedName
				}
				var failedCalls, successCalls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					input, _ := io.ReadAll(r.Body)
					model := extractModel(input)
					if r.Header.Get("Authorization") != "Bearer model-secret" {
						t.Error("alias inference lost configured credential")
					}
					if model == failedModel {
						failedCalls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						w.Header().Set("Retry-After", "60")
						w.WriteHeader(429)
						_, _ = io.WriteString(w, `{"error":{"type":"rate_limited","message":"inference quota"}}`)
						return
					}
					if model != successModel {
						t.Errorf("alias rewrote an unexpected model: %s", model)
					}
					successCalls.Add(1)
					contentType := "application/json"
					if streaming {
						contentType = "text/event-stream"
					}
					w.Header().Set("Content-Type", contentType)
					_, _ = io.WriteString(w, deferredModelResponse(model, streaming))
				}))
				t.Cleanup(upstream.Close)
				executor := newDeferredHTTPFixture(t, provider.New())
				setDeferredMixedModelUpstream(executor, upstream.URL)
				peer, counts := newDeferredModelPeer(t, executor)
				rt := callerMixedModelRuntime(peer.URL)
				aliases := rt.Catalog.Aliases()
				aliases[0].Targets = []config.AliasTarget{{Provider: "a", Model: failedName}, {Provider: "a", Model: successName}}
				rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
				h, _, health := callerDispatchHandler(rt, provider.New())
				t.Cleanup(func() { _ = health.Close() })
				call := func(model string) *httptest.ResponseRecorder {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions",
						strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[]}`, model, streaming))))
					return w
				}
				for range 2 {
					w := call("alias/mixed")
					if w.Code != 200 || w.Body.String() != deferredModelResponse(successModel, streaming) || !health.IsHealthy("a") {
						t.Fatalf("same-provider mixed alias failed over incorrectly: %d %q", w.Code, w.Body.String())
					}
				}
				if failedCalls.Load() != 1 || successCalls.Load() != 2 {
					t.Fatalf("cooldown was provider-wide or retried a cooling target: failed=%d success=%d", failedCalls.Load(), successCalls.Load())
				}
				for _, name := range []string{failedName, successName} {
					p, model, _ := rt.Catalog.Model("a", name)
					delay, cooling := h.current().Resolver.Cooldowns().Remaining(modelresolver.CooldownFingerprintFor("mixed", p, model))
					if name == failedName && (!cooling || delay < 50*time.Second) || name == successName && cooling {
						t.Fatalf("cooldown crossed model identity for %s: %s %t", name, delay, cooling)
					}
				}
				expectedDeferred, expectedOrdinary := int32(2), int32(1)
				if deferredFirst {
					expectedDeferred, expectedOrdinary = 1, 2
				}
				if counts.capabilities.Load() != expectedDeferred || counts.submits.Load() != expectedDeferred ||
					counts.deletes.Load() != expectedDeferred || counts.ordinary.Load() != expectedOrdinary || counts.reads.Load() == 0 {
					t.Fatal("alias attempts did not select each model's transport")
				}
				w := call("a/" + failedName)
				if w.Code != 429 || failedCalls.Load() != 2 || successCalls.Load() != 2 || !health.IsHealthy("a") {
					t.Fatal("direct model consulted alias cooldown, failed over, or changed provider health on retryable 4xx")
				}
			})
		}
	}
}

func TestDeferredCallerOptedInModelNonChatRequestsRemainOrdinary(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, tc := range []struct {
			name, path string
			streaming  bool
		}{
			{"responses-json", "/v1/responses", false},
			{"responses-sse", "/v1/responses", true},
			{"embeddings", "/v1/embeddings", false},
		} {
			t.Run(target+"/"+tc.name, func(t *testing.T) {
				var ordinary, private atomic.Int32
				peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if isDeferredPath(r.URL.Path) {
						private.Add(1)
						w.WriteHeader(400)
						return
					}
					ordinary.Add(1)
					input, _ := io.ReadAll(r.Body)
					if r.URL.Path != tc.path || extractModel(input) != "p/model-2" || r.Header.Get("Authorization") != "Bearer peer-token" {
						t.Errorf("non-chat dispatch changed: %s %s %#v", r.URL.Path, input, r.Header)
					}
					contentType := "application/json"
					if tc.streaming {
						contentType = "text/event-stream"
					}
					w.Header().Set("Content-Type", contentType)
					w.WriteHeader(202)
					_, _ = io.WriteString(w, deferredModelResponse("ordinary", tc.streaming))
				}))
				t.Cleanup(peer.Close)
				rt := callerMixedModelRuntime(peer.URL)
				aliases := rt.Catalog.Aliases()
				aliases[0].Targets[0], aliases[0].Targets[1] = aliases[0].Targets[1], aliases[0].Targets[0]
				rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
				adapter := deferredAdapterFunc(func(ctx context.Context, request provider.Request) (*provider.Result, error) {
					if request.DeferredTransport == nil {
						t.Error("test did not dispatch the opted-in model")
					}
					return provider.New().Do(ctx, request)
				})
				h, _, health := callerDispatchHandler(rt, adapter)
				t.Cleanup(func() { _ = health.Close() })
				model := "a/model-2"
				if target == "alias" {
					model = "alias/mixed"
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("POST", tc.path,
					strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":%t,"input":"hello"}`, model, tc.streaming))))
				if w.Code != 202 || w.Body.String() != deferredModelResponse("ordinary", tc.streaming) || ordinary.Load() != 1 || private.Load() != 0 {
					t.Fatalf("non-chat model opt-in used private transport: status=%d ordinary=%d private=%d body=%q", w.Code, ordinary.Load(), private.Load(), w.Body.String())
				}
			})
		}
	}
}

func TestDeferredCallerDispatchPreservesProviderDefaultAndWholeModelOverrides(t *testing.T) {
	for _, tc := range []struct {
		name, block string
		retries     int
	}{
		{"empty", "deferred_transport {}", 8},
		{"partial", "deferred_transport { max_control_retries = 2 }", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rt, err := config.Load([]byte(fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "openai-compatible" "a" {
  base_url = "http://127.0.0.1:1/v1"
  api_key = "peer-token"
  deferred_transport {
    control_timeout = "200ms"
    max_wait = "3s"
    poll_interval = "50ms"
    max_control_calls = 4
    max_control_retries = 0
  }
  model "model-1" { upstream_name = "p/model-1" }
  model "model-2" {
    upstream_name = "p/model-2"
    %s
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
`, tc.block)), "caller.hcl")
			if err != nil {
				t.Fatal(err)
			}
			compatibility := config.DeferredTransport{
				ControlTimeout: 200 * time.Millisecond, MaxWait: 3 * time.Second, PollInterval: 50 * time.Millisecond,
				MaxControlCalls: 4, MaxControlRetries: 0,
			}
			override := config.DefaultDeferredTransport()
			override.MaxControlRetries = tc.retries
			var calls atomic.Int32
			adapter := deferredAdapterFunc(func(_ context.Context, request provider.Request) (*provider.Result, error) {
				calls.Add(1)
				expected := compatibility
				if request.UpstreamModel == "p/model-2" {
					expected = override
				}
				if request.APIKey != "peer-token" || request.DeferredTransport == nil || *request.DeferredTransport != expected {
					t.Errorf("effective options were ignored or merged: model=%s options=%+v want=%+v", request.UpstreamModel, request.DeferredTransport, expected)
				}
				if request.DeferredTransport != nil {
					*request.DeferredTransport = config.DeferredTransport{}
				}
				return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
			})
			h, _, health := callerDispatchHandler(rt, adapter)
			t.Cleanup(func() { _ = health.Close() })
			for range 2 {
				for _, model := range []string{"a/model-1", "a/model-2", "alias/first", "alias/second"} {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`)))
					if w.Code != 200 {
						t.Fatalf("%s: %d %s", model, w.Code, w.Body.String())
					}
				}
			}
			p, first, _ := rt.Catalog.Model("a", "model-1")
			_, second, _ := rt.Catalog.Model("a", "model-2")
			if calls.Load() != 8 || *config.EffectiveDeferredTransport(p, first) != compatibility || *config.EffectiveDeferredTransport(p, second) != override {
				t.Fatal("dispatch mutated shared provider/model policy")
			}
		})
	}
}

func copyDeferredModelReply(w http.ResponseWriter, response *httptest.ResponseRecorder, interrupt bool) {
	for name, values := range response.Header() {
		w.Header()[name] = values
	}
	data := response.Body.Bytes()
	if interrupt {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)+1))
		data = data[:max(1, len(data)/2)]
	}
	w.WriteHeader(response.Code)
	_, _ = w.Write(data)
}

func TestDeferredCallerModelAckAndReadRecoveryPinsAttempt(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", target, streaming), func(t *testing.T) {
				body := deferredModelResponse("actual-2", streaming)
				if streaming {
					body = ": " + strings.Repeat("x", 80<<10) + "\n\n" + body
				} else {
					body = strings.Replace(body, `"vendor":"kept"`, `"vendor":"`+strings.Repeat("x", 80<<10)+`"`, 1)
				}
				var inferences, capabilities, submits, deletes, ordinary atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					inferences.Add(1)
					input, _ := io.ReadAll(r.Body)
					if extractModel(input) != "actual-2" || r.Header.Get("Authorization") != "Bearer model-secret" {
						t.Error("recovery changed inference identity")
					}
					contentType := "application/json"
					if streaming {
						contentType = "text/event-stream"
					}
					w.Header().Set("Content-Type", contentType)
					_, _ = io.WriteString(w, body)
				}))
				t.Cleanup(upstream.Close)
				executor := newDeferredHTTPFixture(t, provider.New())
				setDeferredMixedModelUpstream(executor, upstream.URL)
				var mu sync.Mutex
				var wires [][]byte
				var interrupted, recovered string
				droppedRead := false
				peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer peer-token" {
						t.Error("recovery lost the pinned credential")
					}
					route, _, _ := deferredRoute(r.URL.Path, r.Method)
					switch route {
					case "capabilities":
						capabilities.Add(1)
					case "submit":
						wire, _ := io.ReadAll(r.Body)
						r.Body = io.NopCloser(bytes.NewReader(wire))
						mu.Lock()
						wires = append(wires, wire)
						mu.Unlock()
					case "delete":
						deletes.Add(1)
					default:
						if !isDeferredPath(r.URL.Path) {
							ordinary.Add(1)
						}
					}
					response := httptest.NewRecorder()
					executor.h.ServeHTTP(response, r)
					interrupt := route == "submit" && submits.Add(1) == 1
					if route == "read" && response.Code == 200 {
						var frame deferred.ReadFrame
						if deferred.DecodeControl(response.Body.Bytes(), &frame) == nil && len(frame.Data) > 0 {
							mu.Lock()
							if !droppedRead {
								droppedRead, interrupt, interrupted = true, true, r.URL.Query().Get("cursor")
							} else if recovered == "" {
								recovered = r.URL.Query().Get("cursor")
							}
							mu.Unlock()
						}
					}
					copyDeferredModelReply(w, response, interrupt)
				}))
				t.Cleanup(peer.Close)
				rt := callerMixedModelRuntime(peer.URL)
				aliases := rt.Catalog.Aliases()
				aliases[0].Targets[0], aliases[0].Targets[1] = aliases[0].Targets[1], aliases[0].Targets[0]
				rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
				h, recorder, health := callerDispatchHandler(rt, provider.New())
				t.Cleanup(func() { _ = health.Close() })
				model := "a/model-2"
				if target == "alias" {
					model = "alias/mixed"
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions",
					strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[]}`, model, streaming))))
				if w.Code != 200 || w.Body.String() != body || inferences.Load() != 1 || capabilities.Load() != 1 ||
					submits.Load() != 2 || deletes.Load() != 1 || ordinary.Load() != 0 || !health.IsHealthy("a") ||
					len(recorder.Events()) != 1 || recorder.Events()[0].TotalTokens != 5 {
					t.Fatalf("model recovery duplicated or replaced inference: status=%d inference=%d capabilities=%d submits=%d deletes=%d ordinary=%d",
						w.Code, inferences.Load(), capabilities.Load(), submits.Load(), deletes.Load(), ordinary.Load())
				}
				mu.Lock()
				defer mu.Unlock()
				if len(wires) != 2 || !bytes.Equal(wires[0], wires[1]) || !droppedRead || interrupted != recovered {
					t.Fatalf("recovery refreshed the attempt or advanced an incomplete cursor: wires=%d cursor=%q -> %q", len(wires), interrupted, recovered)
				}
			})
		}
	}
}

func TestDeferredCallerModelPeerFailuresAndControlBudgetsFailClosed(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, tc := range []struct {
			name, code           string
			retries, calls       int
			capabilities, submit int32
		}{
			{"capability-protocol", "deferred_protocol_error", 8, 4096, 1, 0},
			{"submit-indeterminate", "deferred_submit_indeterminate", 0, 4096, 1, 1},
			{"accepted-read-loss", "deferred_request_lost", 8, 4096, 1, 1},
			{"zero-retries", "deferred_control_error", 0, 4096, 1, 0},
			{"retry-budget", "deferred_control_error", 2, 4096, 3, 0},
			{"call-budget", "deferred_control_error", 8, 4, 4, 0},
		} {
			t.Run(target+"/"+tc.name, func(t *testing.T) {
				var inferences, capabilities, submits, ordinary atomic.Int32
				executor := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
					inferences.Add(1)
					return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
				}))
				setDeferredMixedModelUpstream(executor, "http://127.0.0.1:1")
				peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					route, _, _ := deferredRoute(r.URL.Path, r.Method)
					if route == "capabilities" {
						capabilities.Add(1)
						if tc.name == "capability-protocol" {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(202)
							_, _ = io.WriteString(w, `{"ordinary":true}`)
							return
						}
						if strings.HasSuffix(tc.name, "retries") || strings.HasSuffix(tc.name, "budget") {
							deferredResponseHeaders(w)
							w.WriteHeader(503)
							_ = json.NewEncoder(w).Encode(deferred.ControlError{
								Envelope: deferred.NewEnvelope(), Acceptance: deferred.AcceptanceNotAccepted,
								Error: deferred.ErrorDetail{Type: "deferred_saturated", Message: "control pressure"},
							})
							return
						}
					}
					if route == "submit" {
						submits.Add(1)
					}
					if route == "read" && tc.name == "accepted-read-loss" {
						deferredResponseHeaders(w)
						w.WriteHeader(410)
						_ = json.NewEncoder(w).Encode(deferred.ControlError{
							Envelope: deferred.NewEnvelope(), Acceptance: deferred.AcceptanceNotAccepted,
							Error: deferred.ErrorDetail{Type: "job_unavailable", Message: "accepted result lost"},
						})
						return
					}
					if !isDeferredPath(r.URL.Path) {
						ordinary.Add(1)
					}
					response := httptest.NewRecorder()
					executor.h.ServeHTTP(response, r)
					copyDeferredModelReply(w, response, route == "submit" && tc.name == "submit-indeterminate")
				}))
				t.Cleanup(peer.Close)
				rt := callerMixedModelRuntime(peer.URL)
				providers, aliases := rt.Catalog.Providers(), rt.Catalog.Aliases()
				rt.EncryptedReasoning = scopedReasoningPolicy("retry")
				providers[0].Models[1].DeferredTransport.MaxControlRetries = tc.retries
				providers[0].Models[1].DeferredTransport.MaxControlCalls = tc.calls
				providers[0].ModelByName = nil
				aliases[0].Targets[0], aliases[0].Targets[1] = aliases[0].Targets[1], aliases[0].Targets[0]
				rt.Catalog = config.NewCatalog(providers, nil, aliases)
				h, recorder, health := callerDispatchHandler(rt, provider.New())
				t.Cleanup(func() { _ = health.Close() })
				model := "a/model-2"
				if target == "alias" {
					model = "alias/mixed"
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"opaque"}]}]}`)))
				if w.Code != 502 || !strings.Contains(w.Body.String(), `"type":"`+tc.code+`"`) || capabilities.Load() != tc.capabilities ||
					submits.Load() != tc.submit || ordinary.Load() != 0 || inferences.Load() > tc.submit || !health.IsHealthy("a") || len(recorder.Events()) != 1 {
					t.Fatalf("model control failure dispatched fresh inference or ignored budgets: status=%d body=%s capabilities=%d submits=%d inference=%d ordinary=%d",
						w.Code, w.Body.String(), capabilities.Load(), submits.Load(), inferences.Load(), ordinary.Load())
				}
				for _, name := range []string{"model-1", "model-2"} {
					p, model, _ := rt.Catalog.Model("a", name)
					if _, cooling := h.current().Resolver.Cooldowns().Remaining(modelresolver.CooldownFingerprintFor("mixed", p, model)); cooling {
						t.Fatal("private control failure populated model cooldown")
					}
				}
			})
		}
	}
}

func TestDeferredCallerModelPolicyUpdateKeepsAcceptedPeerAndOptions(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", target, streaming), func(t *testing.T) {
				gate := make(chan struct{})
				release := sync.OnceFunc(func() { close(gate) })
				observed := make(chan struct{})
				observe := sync.OnceFunc(func() { close(observed) })
				prefix := "data: original-prefix\n\n"
				expectedBody := deferredModelResponse("actual-2", streaming)
				if streaming {
					expectedBody = prefix + expectedBody
				}
				var inferences, capabilities, submits, deletes, ordinary, resourceCloses atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					inferences.Add(1)
					input, _ := io.ReadAll(r.Body)
					if extractModel(input) != "actual-2" || r.Header.Get("Authorization") != "Bearer model-secret" {
						t.Error("admitted execution lost its original model or credential")
					}
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, prefix)
						w.(http.Flusher).Flush()
					}
					select {
					case <-gate:
					case <-r.Context().Done():
						return
					}
					if !streaming {
						w.Header().Set("Content-Type", "application/json")
					}
					_, _ = io.WriteString(w, deferredModelResponse("actual-2", streaming))
				}))
				t.Cleanup(upstream.Close)
				executor := newDeferredHTTPFixture(t, provider.New())
				setDeferredMixedModelUpstream(executor, upstream.URL)
				var updated atomic.Bool
				var mu sync.Mutex
				var interrupted, recovered string
				dropped := false
				oldPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer peer-token" || resourceCloses.Load() != 0 {
						t.Error("admitted controls changed credential or lost their owned resources")
					}
					route, _, _ := deferredRoute(r.URL.Path, r.Method)
					switch route {
					case "capabilities":
						capabilities.Add(1)
					case "submit":
						submits.Add(1)
					case "delete":
						deletes.Add(1)
					case "read":
						if !streaming {
							observe()
						}
					default:
						if !isDeferredPath(r.URL.Path) {
							ordinary.Add(1)
						}
					}
					response := httptest.NewRecorder()
					executor.h.ServeHTTP(response, r)
					interrupt := false
					if route == "read" && updated.Load() && response.Code == 200 {
						var frame deferred.ReadFrame
						if deferred.DecodeControl(response.Body.Bytes(), &frame) == nil && len(frame.Data) > 0 {
							mu.Lock()
							if !dropped {
								dropped, interrupt, interrupted = true, true, r.URL.Query().Get("cursor")
							} else if recovered == "" {
								recovered = r.URL.Query().Get("cursor")
							}
							mu.Unlock()
						}
					}
					copyDeferredModelReply(w, response, interrupt)
				}))
				t.Cleanup(oldPeer.Close)
				var nextOrdinary, nextCapabilities, nextSubmits atomic.Int32
				nextPeer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Header.Get("Authorization") != "Bearer next-token" {
						t.Error("new model policy lost its updated peer credential")
					}
					switch r.URL.Path {
					case "/v1/chat/completions":
						nextOrdinary.Add(1)
						input, _ := io.ReadAll(r.Body)
						if extractModel(input) != "next/model-2" {
							t.Error("new ordinary model did not use the updated model rewrite")
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, `{"source":"next"}`)
					case deferred.Prefix + "/capabilities":
						nextCapabilities.Add(1)
						if r.URL.Query().Get("model") != "next/model-1" {
							t.Error("new model opt-in did not negotiate its updated model")
						}
						deferredResponseHeaders(w)
						w.WriteHeader(503)
						_ = json.NewEncoder(w).Encode(deferred.ControlError{
							Envelope: deferred.NewEnvelope(), Acceptance: deferred.AcceptanceNotAccepted,
							Error: deferred.ErrorDetail{Type: "deferred_saturated", Message: "new control pressure"},
						})
					default:
						nextSubmits.Add(1)
						w.WriteHeader(400)
					}
				}))
				t.Cleanup(nextPeer.Close)
				rt := callerMixedModelRuntime(oldPeer.URL)
				aliases := []config.Alias{
					{Name: "first", Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "a", Model: "model-1"}}},
					{Name: "second", Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "a", Model: "model-2"}}},
				}
				rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
				h, recorder, health := callerDispatchHandler(rt, provider.New())
				t.Cleanup(func() { _ = health.Close() })
				oldResources, err := NewRuntimeResources(NewResourceOwner(func() error {
					resourceCloses.Add(1)
					return nil
				}, nil))
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(oldResources.Retire)
				deps := h.current()
				oldClient := oldPeer.Client()
				deps.RuntimeResources = oldResources
				deps.ClientForProvider = func(config.Provider) *http.Client { return oldClient }
				h.UpdateDependencies(deps)
				server := httptest.NewServer(h)
				t.Cleanup(server.Close)
				t.Cleanup(release)
				publicModel := "a/model-2"
				if target == "alias" {
					publicModel = "alias/second"
				}
				type delivery struct {
					status int
					body   []byte
					err    error
				}
				delivered := make(chan delivery, 1)
				go func() {
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					request, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/v1/chat/completions",
						strings.NewReader(fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[]}`, publicModel, streaming)))
					if err != nil {
						delivered <- delivery{err: err}
						return
					}
					request.Header.Set("Content-Type", "application/json")
					response, err := server.Client().Do(request)
					if err != nil {
						delivered <- delivery{err: err}
						return
					}
					defer response.Body.Close()
					var data []byte
					if streaming {
						data = make([]byte, len(prefix))
						_, err = io.ReadFull(response.Body, data)
						if err == nil && string(data) == prefix {
							observe()
						}
					}
					tail, readErr := io.ReadAll(response.Body)
					delivered <- delivery{response.StatusCode, append(data, tail...), errors.Join(err, readErr)}
				}()
				select {
				case <-observed:
				case <-time.After(3 * time.Second):
					t.Fatal("original model did not admit its job or commit its stream")
				}
				providers := rt.Catalog.Providers()
				p := &providers[0]
				p.BaseURL, p.APIKey = nextPeer.URL+"/v1", "next-token"
				options := *p.Models[1].DeferredTransport
				options.ControlTimeout, options.MaxWait, options.MaxControlRetries, options.MaxControlCalls = 100*time.Millisecond, time.Second, 0, 4
				p.Models[0].UpstreamName, p.Models[0].DeferredTransport = "next/model-1", &options
				p.Models[1].UpstreamName, p.Models[1].DeferredTransport = "next/model-2", nil
				p.ModelByName = nil
				next := &config.Runtime{Catalog: config.NewCatalog(providers, nil, aliases)}
				nextResources, err := NewRuntimeResources()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(nextResources.Retire)
				deps = h.current()
				deps.Catalog, deps.Resolver, deps.RuntimeResources = next.Catalog, modelresolver.NewWithPrevious(next, deps.Resolver), nextResources
				nextClient := nextPeer.Client()
				deps.ClientForProvider = func(config.Provider) *http.Client { return nextClient }
				h.UpdateDependencies(deps)
				updated.Store(true)
				oldResources.Retire()
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
				err = oldResources.Wait(ctx)
				cancel()
				if !errors.Is(err, ErrResourcesIncomplete) || resourceCloses.Load() != 0 {
					t.Fatal("model policy update released an admitted execution's resources")
				}
				first, second := "a/model-1", "a/model-2"
				if target == "alias" {
					first, second = "alias/first", "alias/second"
				}
				call := func(model string) *httptest.ResponseRecorder {
					w := httptest.NewRecorder()
					h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`)))
					return w
				}
				if w := call(second); w.Code != 200 || w.Body.String() != `{"source":"next"}` {
					t.Fatalf("new request retained removed model deferral: %d %s", w.Code, w.Body.String())
				}
				if w := call(first); w.Code != 502 || !strings.Contains(w.Body.String(), `"type":"deferred_control_error"`) {
					t.Fatalf("new request ignored model opt-in or its zero retry budget: %d %s", w.Code, w.Body.String())
				}
				if nextOrdinary.Load() != 1 || nextCapabilities.Load() != 1 || nextSubmits.Load() != 0 || resourceCloses.Load() != 0 {
					t.Fatal("new model policy fell back, ignored budgets, or closed admitted resources")
				}
				release()
				select {
				case result := <-delivered:
					if result.err != nil || result.status != 200 || string(result.body) != expectedBody {
						t.Fatalf("admitted delivery crossed model policy generations: status=%d err=%v body=%q", result.status, result.err, result.body)
					}
				case <-time.After(4 * time.Second):
					t.Fatal("original model did not finish through its pinned peer")
				}
				ctx, cancel = context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				if err := oldResources.Wait(ctx); err != nil {
					t.Fatal(err)
				}
				if inferences.Load() != 1 || capabilities.Load() != 1 || submits.Load() != 1 || deletes.Load() != 1 ||
					ordinary.Load() != 0 || resourceCloses.Load() != 1 || len(recorder.Events()) != 3 {
					t.Fatal("admitted model duplicated inference or lost once-only resource ownership")
				}
				mu.Lock()
				defer mu.Unlock()
				if !dropped || interrupted != recovered {
					t.Fatalf("admitted model adopted the new zero retry budget or advanced its incomplete cursor: %q -> %q", interrupted, recovered)
				}
			})
		}
	}
}

func TestDeferredReceiverRejectsCallerModelPresenceAcrossRepresentations(t *testing.T) {
	for _, mode := range []string{"provider_default", "model_slice", "model_index", "sibling_slice", "sibling_index", "standalone_resolved"} {
		t.Run(mode, func(t *testing.T) {
			model := config.Model{Name: "m", UpstreamName: "actual"}
			p := config.Provider{
				Type: config.ProviderTypeOpenAICompatible, Name: "p",
				DeferredReceiver: &config.DeferredReceiver{ExecutionTimeout: time.Second, MaxResultBytes: 2048},
				Models:           []config.Model{model}, ModelByName: map[string]config.Model{"m": model},
			}
			resolved := modelresolver.ResolveResult{Kind: modelresolver.KindDirect, Provider: p, Model: model}
			limits, err := deferredReceiverLimits(Dependencies{}, resolved)
			if err != nil || limits.ExecutionTimeout != time.Second || limits.MaxResultBytes != 2048 {
				t.Fatalf("ordinary receiver baseline: %+v %v", limits, err)
			}
			switch mode {
			case "provider_default":
				p.DeferredTransport = &config.DeferredTransport{}
			case "model_slice":
				p.Models[0].DeferredTransport = &config.DeferredTransport{}
			case "model_index":
				indexed := p.ModelByName["m"]
				indexed.DeferredTransport = &config.DeferredTransport{}
				p.ModelByName["m"] = indexed
			case "sibling_slice":
				p.Models = append(p.Models, config.Model{Name: "caller", DeferredTransport: &config.DeferredTransport{}})
			case "sibling_index":
				p.ModelByName["caller"] = config.Model{Name: "caller", DeferredTransport: &config.DeferredTransport{}}
			case "standalone_resolved":
				resolved.Model.DeferredTransport = &config.DeferredTransport{}
			}
			resolved.Provider = p
			if _, err := deferredReceiverLimits(Dependencies{}, resolved); err == nil || !strings.Contains(err.Error(), "without caller deferral") {
				t.Fatalf("receiver accepted caller presence outside selected ordinary model: %v", err)
			}
		})
	}
}
