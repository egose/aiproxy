//go:build integration && linux

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
)

func TestBinaryDeferredMixedModelsUseDistinctTransports(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", target, stream), func(t *testing.T) {
				trust := newBinaryTLSCertificate(t)
				var firstCalls, secondCalls atomic.Int32
				upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					input, _ := io.ReadAll(r.Body)
					var request struct {
						Model  string
						Stream bool
					}
					if json.Unmarshal(input, &request) != nil {
						t.Error("binary mixed-model inference input is not JSON")
					}
					switch request.Model {
					case "actual-model-1":
						firstCalls.Add(1)
					case "actual-model-2":
						secondCalls.Add(1)
					default:
						t.Errorf("binary mixed-model rewrite changed: %s", input)
					}
					if r.Header.Get("Authorization") != "Bearer ultimate-key" || r.UserAgent() != "binary-deferred-client" ||
						r.Header.Get("X-Inference") != "binary-inference" || !bytes.Contains(input, []byte(`"vendor":{"unchanged":true}`)) {
						t.Errorf("binary mixed-model inference lost credentials/metadata: %s %#v", input, r.Header)
					}
					for name := range r.Header {
						if config.IsDeferredHeader(name) {
							t.Errorf("private binary control header reached ultimate inference: %s", name)
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
					_, _ = w.Write(binaryDeferredMixedResponse(request.Model, r.URL.Path, request.Stream))
				}))
				executorAddr := freeAddr(t)
				executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredMixedExecutorConfig(executorAddr, upstream.URL, defaultBinaryDeferredPolicy())), executorAddr)
				peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 300*time.Millisecond)
				setBinaryDeferredMixedPeerHooks(t, peer, "peer-token", nil)
				callerAddr := freeAddr(t)
				caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredMixedCallerConfig(callerAddr, peer.URL(), "peer-token", nil, binaryDeferredMixedOptions(), false)), callerAddr)
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				for i, name := range []string{"model-1", "model-2"} {
					model := "a/" + name
					if target == "alias" {
						model = "alias/mixed"
					}
					reply := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "end-client-token", "", binaryDeferredMixedInput(model, "/v1/chat/completions", stream), false)
					requireNoBinaryDeferredEnvelope(t, reply)
					contentType := "application/json"
					if stream {
						contentType = "text/event-stream"
					}
					if reply.status != 202 || reply.err != nil || !bytes.Equal(reply.body, binaryDeferredMixedResponse(fmt.Sprintf("actual-model-%d", i+1), "/v1/chat/completions", stream)) ||
						reply.header.Get("Content-Type") != contentType || reply.header.Get("X-Vendor") != "kept" {
						t.Fatalf("binary mixed chat delivery changed: status=%d body=%q err=%v", reply.status, reply.body, reply.err)
					}
					if i == 0 && (privateBinaryDeferredMixedCalls(peer) != 0 || firstCalls.Load() != 1 || secondCalls.Load() != 0) {
						t.Fatal("ordinary binary sibling used private endpoints or a different inference")
					}
				}
				requireBinaryDeferredReleased(t, executor)
				if firstCalls.Load() != 1 || secondCalls.Load() != 1 || len(peer.Calls("/v1/chat/completions")) != 1 ||
					len(peer.Calls("/capabilities")) != 1 || len(peer.Calls("/submit")) != 1 ||
					len(peer.Calls("/read")) == 0 || len(peer.Calls("/result")) != 1 {
					t.Fatal("binary deferred sibling duplicated inference or used synchronous fallback")
				}
				query, err := url.ParseQuery(peer.Calls("/capabilities")[0].query)
				if err != nil || query.Get("model") != "p/model-2" {
					t.Fatal("binary caller negotiated the wrong executor public model")
				}
				prepared, err := deferred.DecodeSubmit(peer.Calls("/submit")[0].body)
				if err != nil || !bytes.Contains(prepared.Body, []byte(`"model":"p/model-2"`)) ||
					!bytes.Contains(prepared.Body, []byte(`"vendor":{"unchanged":true}`)) || prepared.Headers.Get("Authorization") != "" ||
					prepared.Headers.Get("User-Agent") != "binary-deferred-client" || prepared.Headers.Get("X-Inference") != "binary-inference" {
					t.Fatalf("binary submission lost prepared model/metadata or exposed credentials: %v", err)
				}
				for name := range prepared.Headers {
					if config.IsDeferredHeader(name) {
						t.Fatalf("private header entered prepared binary inference: %s", name)
					}
				}
				requireBinaryDeferredPeerProtocol(t, peer, "h2", "/capabilities", "/submit", "/read", "/result", "/jobs/"+submittedBinaryDeferredID(t, peer))
				privateBefore := privateBinaryDeferredMixedCalls(peer)
				for _, operation := range []struct {
					path          string
					stream        bool
					ordinaryCalls int
				}{{"/v1/responses", false, 1}, {"/v1/responses", true, 2}, {"/v1/embeddings", false, 1}} {
					model := "a/model-2"
					if target == "alias" {
						model = "alias/second"
					}
					reply := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", operation.path, "end-client-token", "", binaryDeferredMixedInput(model, operation.path, operation.stream), false)
					requireNoBinaryDeferredEnvelope(t, reply)
					if reply.status != 202 || reply.err != nil || !bytes.Equal(reply.body, binaryDeferredMixedResponse("actual-model-2", operation.path, operation.stream)) ||
						privateBinaryDeferredMixedCalls(peer) != privateBefore || len(peer.Calls(operation.path)) != operation.ordinaryCalls {
						t.Fatalf("binary opted model deferred or changed ordinary %s delivery: %d %q %v", operation.path, reply.status, reply.body, reply.err)
					}
				}
				if firstCalls.Load() != 1 || secondCalls.Load() != 4 || peer.cuts.Load() != 0 {
					t.Fatal("binary mixed/non-chat attempts duplicated inference or exceeded a finite peer transfer")
				}
				requireBinaryDeferredAccounting(t, caller, "", 5, 25)
				requireBinaryDeferredAccounting(t, executor, "peer-token", 5, 25)
			})
		}
	}
}

func TestBinaryDeferredMixedModelFailuresDoNotStartFreshInference(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, stream := range []bool{false, true} {
			for _, fault := range []string{"protocol", "admission", "read-loss"} {
				t.Run(fmt.Sprintf("%s/stream=%t/%s", target, stream, fault), func(t *testing.T) {
					trust := newBinaryTLSCertificate(t)
					var calls atomic.Int32
					started := make(chan struct{})
					observe := sync.OnceFunc(func() { close(started) })
					upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						observe()
						input, _ := io.ReadAll(r.Body)
						if !bytes.Contains(input, []byte(`"model":"actual-model-2"`)) || r.Header.Get("Authorization") != "Bearer ultimate-key" {
							t.Errorf("binary failed attempt changed model/credential: %s", input)
						}
						contentType := "application/json"
						if stream {
							contentType = "text/event-stream"
						}
						w.Header().Set("Content-Type", contentType)
						_, _ = w.Write(binaryDeferredMixedResponse("actual-model-2", r.URL.Path, stream))
					}))
					executorAddr := freeAddr(t)
					cfg := binaryDeferredMixedExecutorConfig(executorAddr, upstream.URL, defaultBinaryDeferredPolicy())
					if fault == "admission" {
						cfg += "\ningress_guardrails {\n enabled = true\n mode = \"block\"\n}\n"
					}
					executor := startBinaryDeferred(t, trust, writeConfig(t, cfg), executorAddr)
					peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 300*time.Millisecond)
					setBinaryDeferredMixedPeerHooks(t, peer, "peer-token", func(response *http.Response) error {
						if fault == "protocol" && strings.HasSuffix(response.Request.URL.Path, "/capabilities") {
							response.Header.Set("X-Aiproxy-Deferred-Version", "2")
						}
						if fault != "read-loss" || !strings.HasSuffix(response.Request.URL.Path, "/read") {
							return nil
						}
						if _, err := binaryDeferredResponseBody(response); err != nil {
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
						replaceBinaryDeferredMixedControl(response, 410, wire)
						return nil
					})
					options := binaryDeferredMixedOptions()
					options.MaxControlRetries = 0
					callerAddr := freeAddr(t)
					caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredMixedCallerConfig(callerAddr, peer.URL(), "peer-token", nil, options, true)), callerAddr)
					model := "a/model-2"
					if target == "alias" {
						model = "alias/mixed"
					}
					input := binaryDeferredMixedInput(model, "/v1/chat/completions", stream)
					secret := ""
					if fault == "admission" {
						secret = guardrailBinaryKey(t)
						input = bytes.Replace(input, []byte(`"content":"test"`), []byte(`"content":"deploy with `+secret+` now"`), 1)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
					defer cancel()
					reply := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "end-client-token", "", input, false)
					requireNoBinaryDeferredEnvelope(t, reply)
					status, code, submissions, inferences := 502, "deferred_protocol_error", 0, int32(0)
					if fault == "admission" {
						status, code, submissions = 400, "secret_blocked", 1
					} else if fault == "read-loss" {
						code, submissions, inferences = "deferred_request_lost", 1, 1
					}
					if reply.status != status || reply.err != nil || !bytes.Contains(reply.body, []byte(code)) ||
						secret != "" && bytes.Contains(reply.body, []byte(secret)) || calls.Load() != inferences ||
						len(peer.Calls("/capabilities")) != 1 || len(peer.Calls("/submit")) != submissions ||
						len(peer.Calls("/v1/chat/completions")) != 0 {
						t.Fatalf("binary %s failure started fresh/synchronous/alias inference: status=%d body=%s err=%v inference=%d", fault, reply.status, reply.body, reply.err, calls.Load())
					}
					requireBinaryDeferredReleased(t, executor)
					requireBinaryDeferredAccounting(t, caller, "", 1, 0)
				})
			}
		}
	}
}

func TestBinaryDeferredMixedModelSIGHUPPinsAdmittedPolicy(t *testing.T) {
	for _, target := range []string{"direct", "alias"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", target, stream), func(t *testing.T) {
				trust := newBinaryTLSCertificate(t)
				var calls atomic.Int32
				gate, observed := make(chan struct{}), make(chan struct{})
				release := sync.OnceFunc(func() { close(gate) })
				observe := sync.OnceFunc(func() { close(observed) })
				prefix := []byte("id: old\ndata: {\"choices\":[]}\n\n")
				oldBody := binaryDeferredMixedResponse("old-admitted", "/v1/chat/completions", stream)
				newBody := binaryDeferredMixedResponse("new-ordinary", "/v1/chat/completions", false)
				upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					input, _ := io.ReadAll(r.Body)
					if !bytes.Contains(input, []byte(`"model":"actual-model-2"`)) || r.Header.Get("Authorization") != "Bearer ultimate-key" {
						t.Errorf("SIGHUP changed ultimate admitted model/credential: %s", input)
					}
					for name := range r.Header {
						if config.IsDeferredHeader(name) {
							t.Errorf("SIGHUP leaked a private header to ultimate inference: %s", name)
						}
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
				t.Cleanup(release)
				executorAddr := freeAddr(t)
				executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredMixedExecutorConfig(executorAddr, upstream.URL, defaultBinaryDeferredPolicy())), executorAddr)
				oldPeer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 300*time.Millisecond)
				var updated, dropped atomic.Bool
				var droppedQuery atomic.Value
				setBinaryDeferredMixedPeerHooks(t, oldPeer, "peer-token", func(response *http.Response) error {
					if !strings.HasSuffix(response.Request.URL.Path, "/read") {
						return nil
					}
					if !stream {
						observe()
					}
					if !updated.Load() {
						return nil
					}
					wire, err := binaryDeferredResponseBody(response)
					if err != nil {
						return err
					}
					var frame deferred.ReadFrame
					if deferred.DecodeControl(wire, &frame) == nil && len(frame.Data) > 0 && dropped.CompareAndSwap(false, true) {
						droppedQuery.Store(response.Request.URL.RawQuery)
						truncateBinaryDeferredResponse(response, wire)
					}
					return nil
				})
				nextPeer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 300*time.Millisecond)
				setBinaryDeferredMixedPeerHooks(t, nextPeer, "next-token", func(response *http.Response) error {
					if !strings.HasSuffix(response.Request.URL.Path, "/capabilities") {
						return nil
					}
					if response.Request.URL.Query().Get("model") != "p/model-1" {
						t.Error("new binary model policy negotiated the wrong public model")
					}
					if _, err := binaryDeferredResponseBody(response); err != nil {
						return err
					}
					wire, err := json.Marshal(deferred.ControlError{
						Envelope: deferred.NewEnvelope(), Acceptance: deferred.AcceptanceNotAccepted,
						Error: deferred.ErrorDetail{Type: "deferred_saturated", Message: "fixture control pressure"},
					})
					if err != nil {
						return err
					}
					replaceBinaryDeferredMixedControl(response, 503, wire)
					return nil
				})
				callerAddr := freeAddr(t)
				path := writeConfig(t, binaryDeferredMixedCallerConfig(callerAddr, oldPeer.URL(), "peer-token", nil, binaryDeferredMixedOptions(), false))
				caller := startBinaryDeferred(t, trust, path, callerAddr)
				model := "a/model-2"
				if target == "alias" {
					model = "alias/second"
				}
				replies := make(chan binaryDeferredReply, 1)
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				go func() {
					request, err := http.NewRequestWithContext(ctx, "POST", caller.baseURL+"/v1/chat/completions", bytes.NewReader(binaryDeferredMixedInput(model, "/v1/chat/completions", stream)))
					if err != nil {
						replies <- binaryDeferredReply{err: err}
						return
					}
					request.Header.Set("Content-Type", "application/json")
					response, err := caller.client.Do(request)
					if err != nil {
						replies <- binaryDeferredReply{err: err}
						return
					}
					defer response.Body.Close()
					var body []byte
					if stream {
						body = make([]byte, len(prefix))
						if _, err = io.ReadFull(response.Body, body); err != nil || !bytes.Equal(body, prefix) {
							replies <- binaryDeferredReply{err: fmt.Errorf("pre-SIGHUP prefix: %q %v", body, err)}
							return
						}
						observe()
					}
					tail, err := io.ReadAll(response.Body)
					replies <- binaryDeferredReply{response.StatusCode, response.Header.Clone(), append(body, tail...), err}
				}()
				awaitBinaryDeferredSignal(t, "admitted mixed-model request before SIGHUP", observed)
				awaitBinaryDeferred(t, "admitted mixed-model upstream inference", time.Second, func() bool { return calls.Load() == 1 })
				next := binaryDeferredMixedOptions()
				next.ControlTimeout, next.MaxWait, next.MaxControlCalls, next.MaxControlRetries = 100*time.Millisecond, time.Second, 4, 0
				writeConfigPath(t, path, binaryDeferredMixedCallerConfig(callerAddr, nextPeer.URL(), "next-token", next, nil, false))
				reloadBinaryTLS(t, caller, true)
				updated.Store(true)
				first, second := "a/model-1", "a/model-2"
				if target == "alias" {
					first, second = "alias/first", "alias/second"
				}
				ordinary := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredMixedInput(second, "/v1/chat/completions", false), false)
				if ordinary.status != 200 || ordinary.err != nil || !bytes.Equal(ordinary.body, newBody) {
					t.Fatalf("new binary request retained removed model deferral: %d %q %v", ordinary.status, ordinary.body, ordinary.err)
				}
				failure := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredMixedInput(first, "/v1/chat/completions", false), false)
				if failure.status != 502 || failure.err != nil || !bytes.Contains(failure.body, []byte("deferred_control_error")) ||
					len(nextPeer.Calls("/capabilities")) != 1 || len(nextPeer.Calls("/submit")) != 0 || len(nextPeer.Calls("/v1/chat/completions")) != 1 {
					t.Fatal("new binary model policy ignored zero retries or used synchronous fallback")
				}
				if !binaryDeferredPause(ctx, 1100*time.Millisecond) {
					t.Fatal("fixture did not keep admitted work alive beyond the new model's max_wait")
				}
				release()
				select {
				case reply := <-replies:
					want := oldBody
					if stream {
						want = append(bytes.Clone(prefix), oldBody...)
					}
					requireNoBinaryDeferredEnvelope(t, reply)
					if reply.status != 200 || reply.err != nil || !bytes.Equal(reply.body, want) {
						t.Fatalf("admitted binary delivery adopted new peer/credential/options: %d %q %v", reply.status, reply.body, reply.err)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("admitted binary request did not finish through its original peer")
				}
				requireNoBinaryDeferredEnvelope(t, ordinary)
				requireNoBinaryDeferredEnvelope(t, failure)
				retried := 0
				for _, read := range oldPeer.Calls("/read") {
					if read.query == droppedQuery.Load() {
						retried++
					}
				}
				if !dropped.Load() || retried < 2 || calls.Load() != 2 ||
					len(oldPeer.Calls("/capabilities")) != 1 || len(oldPeer.Calls("/submit")) != 1 || len(oldPeer.Calls("/v1/chat/completions")) != 0 {
					t.Fatal("admitted binary request lost old retry options/cursor or started another inference")
				}
				requireBinaryDeferredReleased(t, executor)
				requireBinaryDeferredAccounting(t, caller, "", 3, 10)
				requireBinaryDeferredAccounting(t, executor, "peer-token", 1, 5)
				requireBinaryDeferredAccounting(t, executor, "next-token", 1, 5)
			})
		}
	}
}
