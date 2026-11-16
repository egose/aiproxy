//go:build integration && linux

package integration

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
)

func TestBinaryDeferredPeerResponseInactivity(t *testing.T) {
	const idle, silence = 150 * time.Millisecond, 450 * time.Millisecond
	unary := []byte(" \n" + `{"choices":[],"vendor":{"exact":true},"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}` + "\n")
	first := "id: original-3\r\nevent: vendor\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"first"
	last := "\"}}]}\r\n\r\n: native-comment\n\ndata: {\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\ndata: [DONE]\n\n"
	for _, alpn := range []string{"http/1.1", "h2"} {
		for _, shape := range []string{"json-initial", "json-later", "sse-initial", "sse-later"} {
			for _, optIn := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/deferred=%t", alpn, shape, optIn), func(t *testing.T) {
					trust := newBinaryTLSCertificate(t)
					var calls atomic.Int32
					upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						calls.Add(1)
						input, _ := io.ReadAll(r.Body)
						if r.URL.Path != "/v1/chat/completions" || !bytes.Contains(input, []byte(`"model":"actual-model"`)) ||
							!bytes.Contains(input, []byte(`"vendor":{"unchanged":true}`)) || r.Header.Get("Authorization") != "Bearer ultimate-key" ||
							r.Header.Get("X-Inference") != "binary-inference" || r.UserAgent() != "binary-deferred-client" {
							t.Errorf("binary chain changed ordinary inference: %s %#v", input, r.Header)
						}
						for name := range r.Header {
							if config.IsDeferredHeader(name) {
								t.Errorf("private request header reached model: %s", name)
							}
						}
						if strings.HasSuffix(shape, "-initial") && !binaryDeferredPause(r.Context(), silence) {
							return
						}
						w.Header().Set("X-Vendor", "original-head")
						w.Header().Set("X-Aiproxy-Deferred-Upstream", "must-not-escape")
						if strings.HasPrefix(shape, "json") {
							w.Header().Set("Content-Type", "application/json")
							if shape == "json-later" {
								_, _ = w.Write(unary[:7])
								w.(http.Flusher).Flush()
								if !binaryDeferredPause(r.Context(), silence) {
									return
								}
								_, _ = w.Write(unary[7:])
							} else {
								_, _ = w.Write(unary)
							}
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, first)
						w.(http.Flusher).Flush()
						if !binaryDeferredPause(r.Context(), silence) {
							return
						}
						for _, fragment := range []string{last[:7], last[7:36], last[36:]} {
							_, _ = io.WriteString(w, fragment)
							w.(http.Flusher).Flush()
						}
					}))
					executorAddr := freeAddr(t)
					policy := defaultBinaryDeferredPolicy()
					executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(executorAddr, upstream.URL, "peer-token", "ultimate-key", "actual-model", policy)), executorAddr)
					peer := newBinaryDeferredPeer(t, trust, executor.baseURL, alpn, idle)
					callerAddr := freeAddr(t)
					caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", optIn, time.Second, 5*time.Second, "")), callerAddr)
					ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
					defer cancel()
					start := time.Now()
					reply := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredChatBody("peer/m", strings.HasPrefix(shape, "sse")), false)
					requireNoBinaryDeferredEnvelope(t, reply)
					if optIn {
						expected := unary
						if strings.HasPrefix(shape, "sse") {
							expected = []byte(first + last)
						}
						if reply.status != 200 || reply.err != nil || !bytes.Equal(reply.body, expected) || peer.cuts.Load() != 0 ||
							time.Since(start) < silence || reply.header.Get("X-Vendor") != "original-head" {
							t.Fatalf("opted binary chain failed: status=%d err=%v bytes=%q cuts=%d elapsed=%s", reply.status, reply.err, reply.body, peer.cuts.Load(), time.Since(start))
						}
						requireBinaryDeferredReleased(t, executor)
						requireBinaryDeferredAccounting(t, caller, "", 1, 5)
						requireBinaryDeferredAccounting(t, executor, "peer-token", 1, 5)
						requireBinaryDeferredPeerProtocol(t, peer, alpn, "/capabilities", "/submit", "/read", "/result", "/jobs/"+submittedBinaryDeferredID(t, peer))
						if len(peer.Calls("/v1/chat/completions")) != 0 || len(peer.Calls("/submit")) != 1 {
							t.Fatal("opted-in chat silently used ordinary inference or submitted twice")
						}
					} else if shape == "sse-later" {
						if reply.status != 200 || !bytes.Equal(reply.body, []byte(first)) || peer.cuts.Load() == 0 {
							t.Fatalf("ordinary later-gap baseline did not fail: status=%d err=%v bytes=%q cuts=%d", reply.status, reply.err, reply.body, peer.cuts.Load())
						}
					} else if reply.status < 400 || reply.err != nil || peer.cuts.Load() == 0 {
						t.Fatalf("ordinary initial/buffered-gap baseline did not fail: status=%d err=%v cuts=%d body=%s", reply.status, reply.err, peer.cuts.Load(), reply.body)
					}
					if calls.Load() != 1 {
						t.Fatalf("binary chain duplicated inference: %d", calls.Load())
					}
					t.Logf("two binaries, verified TLS/%s, response inactivity=%s, initial/later model gap=%s, deferred=%t, cuts=%d", alpn, idle, silence, optIn, peer.cuts.Load())
				})
			}
		}
	}
}

func submittedBinaryDeferredID(t *testing.T, peer *binaryDeferredPeer) string {
	t.Helper()
	calls := peer.Calls("")
	for _, call := range calls {
		if call.method == "DELETE" && strings.Contains(call.path, "/jobs/") {
			return call.path[strings.LastIndex(call.path, "/")+1:]
		}
	}
	t.Fatal("successful caller did not issue its single best-effort delete")
	return ""
}

func TestBinaryDeferredPeerDroppedTransfersKeepOneInference(t *testing.T) {
	for _, alpn := range []string{"http/1.1", "h2"} {
		for _, stream := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/stream=%t", alpn, stream), func(t *testing.T) {
				trust := newBinaryTLSCertificate(t)
				body := []byte(`{"vendor":"` + strings.Repeat("x", 100<<10) + `","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
				contentType := "application/json"
				if stream {
					body = []byte(": original-" + strings.Repeat("x", 100<<10) + "\r\n\r\nid: original\nevent: vendor\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\ndata: [DONE]\n\n")
					contentType = "text/event-stream"
				}
				var calls atomic.Int32
				upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					_, _ = io.Copy(io.Discard, r.Body)
					w.Header().Set("Content-Type", contentType)
					_, _ = w.Write(body)
				}))
				addr := freeAddr(t)
				executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", defaultBinaryDeferredPolicy())), addr)
				peer := newBinaryDeferredPeer(t, trust, executor.baseURL, alpn, 200*time.Millisecond)
				var submits, manifests atomic.Int32
				var droppedFirst, droppedLater atomic.Bool
				receipts := make(chan deferred.Receipt, 2)
				peer.hooks(nil, func(response *http.Response) error {
					path := response.Request.URL.Path
					if path != deferred.Prefix+"/submit" && !strings.HasSuffix(path, "/read") && !strings.HasSuffix(path, "/result") {
						return nil
					}
					wire, err := binaryDeferredResponseBody(response)
					if err != nil {
						return err
					}
					drop := false
					if path == deferred.Prefix+"/submit" {
						var receipt deferred.Receipt
						if deferred.DecodeControl(wire, &receipt) != nil {
							t.Error("fixture could not inspect the real binary receipt")
						} else {
							receipts <- receipt
						}
						drop = submits.Add(1) == 1
					} else if strings.HasSuffix(path, "/read") {
						var frame deferred.ReadFrame
						if deferred.DecodeControl(wire, &frame) != nil {
							return nil
						}
						drop = len(frame.Data) > 0 && (frame.Cursor == 0 && droppedFirst.CompareAndSwap(false, true) ||
							frame.Cursor > 0 && droppedLater.CompareAndSwap(false, true))
					} else {
						drop = response.StatusCode == 200 && manifests.Add(1) == 1
					}
					if drop {
						truncateBinaryDeferredResponse(response, wire)
					}
					return nil
				})
				callerAddr := freeAddr(t)
				caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", true, time.Second, 5*time.Second, "")), callerAddr)
				ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
				defer cancel()
				reply := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredChatBody("peer/m", stream), false)
				if reply.status != 200 || reply.err != nil || !bytes.Equal(reply.body, body) || calls.Load() != 1 ||
					submits.Load() != 2 || !droppedFirst.Load() || !droppedLater.Load() || manifests.Load() != 2 || peer.cuts.Load() != 0 {
					t.Fatalf("lost ack/initial/later read/manifest duplicated or lost inference: status=%d err=%v calls=%d submits=%d reads=%t/%t manifests=%d cuts=%d", reply.status, reply.err, calls.Load(), submits.Load(), droppedFirst.Load(), droppedLater.Load(), manifests.Load(), peer.cuts.Load())
				}
				requireNoBinaryDeferredEnvelope(t, reply)
				a, b := <-receipts, <-receipts
				if a.ID != b.ID || a.Replayed || !b.Replayed || a.Fingerprint != b.Fingerprint {
					t.Fatal("lost acknowledgment created a different binary job")
				}
				submitCalls := peer.Calls("/submit")
				if len(submitCalls) != 2 || !bytes.Equal(submitCalls[0].body, submitCalls[1].body) {
					t.Fatal("submit retry changed generation/ticket/key/prepared bytes")
				}
				for _, cursor := range []string{"cursor=0", "cursor=65536"} {
					count := 0
					for _, read := range peer.Calls("/read") {
						if strings.Contains(read.query, cursor+"&") {
							count++
						}
					}
					if count < 2 {
						t.Fatalf("incomplete read did not retry unchanged %s", cursor)
					}
				}
				requireBinaryDeferredReleased(t, executor)
				requireBinaryDeferredAccounting(t, executor, "peer-token", 1, 5)
				requireBinaryDeferredAccounting(t, caller, "", 1, 5)
				metrics := binaryDeferredMetrics(t, executor)
				if metrics[`aiproxy_deferred_admissions_total{outcome="accepted"}`] != 1 ||
					metrics[`aiproxy_deferred_admissions_total{outcome="replayed"}`] != 1 ||
					metrics[`aiproxy_deferred_ownership{kind="entries"}`] != 1 {
					t.Fatal("ack/retrieval retries duplicated admission or erased its tombstone")
				}
				requireBinaryDeferredPeerProtocol(t, peer, alpn, "/capabilities", "/submit", "/read", "/result", "/jobs/"+a.ID)
			})
		}
	}
}

func TestBinaryDeferredCrashGenerationFailsClosed(t *testing.T) {
	for _, lostAck := range []bool{true, false} {
		t.Run(fmt.Sprintf("lost-ack=%t", lostAck), func(t *testing.T) {
			trust := newBinaryTLSCertificate(t)
			var calls, backupCalls atomic.Int32
			started, closed := make(chan struct{}), make(chan struct{})
			upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				close(started)
				<-r.Context().Done()
				close(closed)
			}))
			backup := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { backupCalls.Add(1) }))
			addr := freeAddr(t)
			policy := defaultBinaryDeferredPolicy()
			policy.execution, policy.control = 10*time.Second, 2*time.Second
			path := writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", policy))
			executor := startBinaryDeferred(t, trust, path, addr)
			peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
			gate, intercepted := make(chan struct{}), make(chan deferred.Receipt, 1)
			release := sync.OnceFunc(func() { close(gate) })
			t.Cleanup(release)
			var once atomic.Bool
			peer.hooks(nil, func(response *http.Response) error {
				target := response.Request.URL.Path == deferred.Prefix+"/submit"
				if !lostAck {
					target = strings.HasSuffix(response.Request.URL.Path, "/read")
				}
				if !target || !once.CompareAndSwap(false, true) {
					return nil
				}
				wire, err := binaryDeferredResponseBody(response)
				if err != nil {
					return err
				}
				var receipt deferred.Receipt
				if lostAck {
					err = deferred.DecodeControl(wire, &receipt)
				} else {
					var frame deferred.ReadFrame
					err = deferred.DecodeControl(wire, &frame)
					receipt = frame.Receipt
				}
				if err != nil {
					return err
				}
				intercepted <- receipt
				select {
				case <-gate:
				case <-response.Request.Context().Done():
					return response.Request.Context().Err()
				}
				if lostAck {
					truncateBinaryDeferredResponse(response, wire)
				}
				return nil
			})
			callerAddr := freeAddr(t)
			caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", true, 2*time.Second, 6*time.Second, backup.URL)), callerAddr)
			ctx, cancel := context.WithTimeout(context.Background(), 7*time.Second)
			t.Cleanup(cancel)
			replies := make(chan binaryDeferredReply, 1)
			go func() {
				replies <- binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredChatBody("alias/mixed", false), false)
			}()
			var admitted deferred.Receipt
			select {
			case admitted = <-intercepted:
			case <-time.After(3 * time.Second):
				t.Fatal("binary job was not admitted before the crash")
			}
			awaitBinaryDeferredSignal(t, "first process inference", started)
			killBinaryDeferred(t, executor)
			awaitBinaryDeferredSignal(t, "crashed process upstream socket closure", closed)
			restarted := startBinaryDeferred(t, trust, path, addr)
			release()
			var reply binaryDeferredReply
			select {
			case reply = <-replies:
			case <-time.After(4 * time.Second):
				t.Fatal("caller did not resolve generation loss within its recovery budget")
			}
			code, submits := "deferred_request_lost", 1
			if lostAck {
				code, submits = "deferred_submit_indeterminate", 2
			}
			if reply.status != 502 || reply.err != nil || !bytes.Contains(reply.body, []byte(code)) ||
				calls.Load() != 1 || backupCalls.Load() != 0 || len(peer.Calls("/submit")) != submits || len(peer.Calls("/capabilities")) != 1 {
				t.Fatalf("crash refreshed scope, retried alias, or invented outcome: %d %s %v calls=%d backup=%d", reply.status, reply.body, reply.err, calls.Load(), backupCalls.Load())
			}
			requireNoBinaryDeferredEnvelope(t, reply)
			if lostAck {
				s := peer.Calls("/submit")
				if !bytes.Equal(s[0].body, s[1].body) {
					t.Fatal("crash recovery changed the qualified submission")
				}
			}
			current := binaryDeferredCapability(t, peer, "peer-token")
			if current.Generation == admitted.Generation {
				t.Fatal("actual process restart retained the previous generation")
			}
			for _, endpoint := range []struct{ method, suffix string }{
				{"GET", ""}, {"GET", "/result"}, {"GET", "/read?cursor=0"}, {"DELETE", ""},
			} {
				old := callBinaryDeferred(t, peer, endpoint.method, "/jobs/"+admitted.ID+endpoint.suffix, "peer-token", admitted.Generation, nil)
				requireBinaryDeferredError(t, old, 410, "generation_mismatch")
			}
			unknown := callBinaryDeferred(t, peer, "GET", "/jobs/"+admitted.ID, "peer-token", current.Generation, nil)
			requireBinaryDeferredError(t, unknown, 404, "job_unavailable")
			if bytes.Contains(unknown.body, []byte(admitted.ID)) || bytes.Contains(unknown.body, []byte("job_expired")) ||
				binaryDeferredMetrics(t, restarted)[`aiproxy_deferred_ownership{kind="entries"}`] != 0 {
				t.Fatal("restart fabricated retained expiry/deletion or admitted an old-generation attempt")
			}
			requireBinaryDeferredAccounting(t, caller, "", 1, 0)
			requireBinaryDeferredReleased(t, restarted)
		})
	}
}

func TestBinaryDeferredHTTP2SlowReadTransferDeadline(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	body := []byte(`{"vendor":"` + strings.Repeat("x", 100<<10) + `","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
	var calls atomic.Int32
	upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	addr := freeAddr(t)
	policy := defaultBinaryDeferredPolicy()
	policy.control = 200 * time.Millisecond
	executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", policy)), addr)
	peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 300*time.Millisecond)
	var delayed atomic.Bool
	released := make(chan bool, 1)
	peer.hooks(nil, func(response *http.Response) error {
		if !strings.HasSuffix(response.Request.URL.Path, "/read") || delayed.Load() {
			return nil
		}
		wire, err := binaryDeferredResponseBody(response)
		if err != nil {
			return err
		}
		var frame deferred.ReadFrame
		if deferred.DecodeControl(wire, &frame) != nil || len(frame.Data) == 0 || !delayed.CompareAndSwap(false, true) {
			return nil
		}
		response.Body = &binaryDeferredGappedBody{
			first: bytes.NewReader(wire[:len(wire)/2]), last: bytes.NewReader(wire[len(wire)/2:]),
			ctx: response.Request.Context(), gap: 750 * time.Millisecond, done: released,
		}
		return nil
	})
	callerAddr := freeAddr(t)
	caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", true, policy.control, 3*time.Second, "")), callerAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	reply := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredChatBody("peer/m", false), false)
	if reply.status != 200 || reply.err != nil || !bytes.Equal(reply.body, body) || calls.Load() != 1 || !delayed.Load() || peer.cuts.Load() != 0 {
		t.Fatalf("slow HTTP/2 finite transfer exceeded deadline or duplicated bytes: %d %v inference=%d delayed=%t", reply.status, reply.err, calls.Load(), delayed.Load())
	}
	select {
	case deadlineCanceled := <-released:
		if !deadlineCanceled {
			t.Fatal("HTTP/2 caller waited out a response gap exceeding its whole-control deadline")
		}
	case <-time.After(time.Second):
		t.Fatal("timed-out HTTP/2 transfer retained its fixture reader")
	}
	reads := peer.Calls("/read")
	if len(reads) < 3 || reads[0].query != reads[1].query {
		t.Fatal("slow partial HTTP/2 frame was consumed instead of retrying its unchanged cursor")
	}
	requireNoBinaryDeferredEnvelope(t, reply)
	requireBinaryDeferredReleased(t, executor)
	requireBinaryDeferredAccounting(t, caller, "", 1, 5)
	requireBinaryDeferredAccounting(t, executor, "peer-token", 1, 5)
	requireBinaryDeferredPeerProtocol(t, peer, "h2", "/capabilities", "/submit", "/read", "/result")
}

func TestBinaryDeferredReloadDrainAndFrozenExecution(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	var oldCalls, newCalls atomic.Int32
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	started := make(chan struct{}, 2)
	body := []byte(" \n" + `{"source":"old","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}` + "\n")
	old := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		oldCalls.Add(1)
		input, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer old-key" || !bytes.Contains(input, []byte(`"model":"old-model"`)) {
			t.Error("accepted binary execution adopted candidate credentials/model")
		}
		started <- struct{}{}
		select {
		case <-gate:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write(body)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(release)
	next := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		newCalls.Add(1)
		input, _ := io.ReadAll(r.Body)
		if r.Header.Get("Authorization") != "Bearer new-key" || !bytes.Contains(input, []byte(`"model":"new-model"`)) {
			t.Error("new binary execution did not use activated credentials/model")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"source":"new","usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
	}))
	addr := freeAddr(t)
	policy := defaultBinaryDeferredPolicy()
	policy.execution = 10 * time.Second
	initial := binaryDeferredExecutorConfig(addr, old.URL, "peer-token", "old-key", "old-model", policy)
	path := writeConfig(t, initial)
	executor := startBinaryDeferred(t, trust, path, addr)
	peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
	capability := binaryDeferredCapability(t, peer, "peer-token")
	var requests []deferred.SubmitRequest
	var receipts []deferred.Receipt
	for range 2 {
		request := binaryDeferredSubmitRequest(t, capability, []byte(`{"model":"p/m","messages":[],"vendor":"`+strings.Repeat("x", 2048)+`"}`))
		requests = append(requests, request)
		receipts = append(receipts, submitBinaryDeferred(t, peer, "peer-token", request))
		select {
		case <-started:
		case <-time.After(3 * time.Second):
			t.Fatal("accepted binary job was not independently scheduled")
		}
	}
	before := binaryDeferredMetrics(t, executor)
	narrow := policy
	narrow.jobs, narrow.running = 1, 1
	writeConfigPath(t, path, binaryDeferredExecutorConfig(addr, next.URL, "rotated-token", "new-key", "new-model", narrow))
	reloadBinaryTLS(t, executor, false)
	after := binaryDeferredMetrics(t, executor)
	for _, name := range []string{"aiproxy_deferred_enabled", `aiproxy_deferred_ownership{kind="jobs"}`, `aiproxy_deferred_ownership{kind="running"}`, `aiproxy_deferred_bytes{kind="work"}`} {
		if before[name] != after[name] {
			t.Fatal("failed capacity SIGHUP changed live ownership/policy")
		}
	}
	if current := binaryDeferredCapability(t, peer, "peer-token"); current.Generation != capability.Generation || current.Limits != capability.Limits {
		t.Fatal("failed SIGHUP changed advertised admission generation/limits")
	}
	updated := policy
	updated.execution, updated.request, updated.result = time.Second, 1024, 1024
	enabled := binaryDeferredExecutorConfig(addr, next.URL, "rotated-token", "new-key", "new-model", updated)
	writeConfigPath(t, path, enabled)
	reloadBinaryTLS(t, executor, true)
	current := binaryDeferredCapability(t, peer, "rotated-token")
	if current.Generation != capability.Generation || current.Limits.ExecutionTimeoutMS != 1000 || current.Limits.RequestBytes != 1024 {
		t.Fatal("successful SIGHUP failed to publish new admission limits while retaining generation")
	}
	for _, receipt := range receipts {
		oldToken := callBinaryDeferred(t, peer, "GET", "/jobs/"+receipt.ID, "peer-token", receipt.Generation, nil)
		requireBinaryDeferredError(t, oldToken, 401, "auth_failed")
		other := callBinaryDeferred(t, peer, "GET", "/jobs/"+receipt.ID, "other-token", receipt.Generation, nil)
		requireBinaryDeferredError(t, other, 404, "job_unavailable")
	}
	draining := updated
	draining.enabled = false
	removed := binaryDeferredExecutorConfig(addr, next.URL, "rotated-token", "new-key", "new-model", draining)
	writeConfigPath(t, path, removed)
	reloadBinaryTLS(t, executor, true)
	requireBinaryDeferredError(t, callBinaryDeferred(t, peer, "GET", "/capabilities?model=p/m", "rotated-token", "", nil), 404, "deferred_disabled")
	fresh := binaryDeferredSubmitRequest(t, capability, []byte(`{"model":"p/m","messages":[]}`))
	requireBinaryDeferredError(t, callBinaryDeferred(t, peer, "POST", "/submit", "rotated-token", "", binaryDeferredWire(t, fresh)), 404, "deferred_disabled")
	for index, receipt := range receipts {
		replayed := submitBinaryDeferred(t, peer, "rotated-token", requests[index])
		if !replayed.Replayed || replayed.ID != receipt.ID || replayed.ExecutionDeadlineMS != receipt.ExecutionDeadlineMS {
			t.Fatal("removal/smaller input policy changed accepted replay or deadline")
		}
		reply := callBinaryDeferred(t, peer, "GET", "/jobs/"+receipt.ID, "rotated-token", receipt.Generation, nil)
		var info deferred.JobInfo
		if reply.status != 200 || deferred.DecodeControl(reply.body, &info) != nil || info.State != deferred.StateRunning ||
			info.ExecutionDeadlineMS != receipt.ExecutionDeadlineMS {
			t.Fatal("draining status lost the frozen binary execution")
		}
		read := readBinaryDeferred(t, peer, "rotated-token", receipt, 0)
		if read.Done || len(read.Data) != 0 {
			t.Fatal("initial silence became a terminal result during drain")
		}
	}
	writeConfigPath(t, path, binaryDeferredExecutorConfig(addr, next.URL, "rotated-token", "new-key", "new-model", narrow))
	reloadBinaryTLS(t, executor, false)
	if binaryDeferredMetrics(t, executor)["aiproxy_deferred_enabled"] != 0 {
		t.Fatal("failed reenable partially changed draining policy")
	}
	release()
	for _, receipt := range receipts {
		manifest := finishBinaryDeferred(t, peer, "rotated-token", receipt)
		requireBinaryDeferredManifest(t, manifest, body, deferred.OutcomeResponse)
		if manifest.ExecutionDeadlineMS != receipt.ExecutionDeadlineMS || manifest.Outcome.Usage.TotalTokens != 5 {
			t.Fatal("accepted execution/completion crossed SIGHUP snapshot")
		}
		frame := readBinaryDeferred(t, peer, "rotated-token", receipt, 0)
		if !frame.Done || !bytes.Equal(frame.Data, body) {
			t.Fatal("draining read changed exact retained binary output")
		}
		reply := callBinaryDeferred(t, peer, "DELETE", "/jobs/"+receipt.ID, "rotated-token", receipt.Generation, nil)
		if reply.status != 204 || len(reply.body) != 0 {
			t.Fatal("draining delete failed to release output")
		}
	}
	requireBinaryDeferredReleased(t, executor)
	writeConfigPath(t, path, enabled)
	reloadBinaryTLS(t, executor, true)
	capability = binaryDeferredCapability(t, peer, "rotated-token")
	newReceipt := submitBinaryDeferred(t, peer, "rotated-token", binaryDeferredSubmitRequest(t, capability, []byte(`{"model":"p/m","messages":[]}`)))
	manifest := finishBinaryDeferred(t, peer, "rotated-token", newReceipt)
	if manifest.State != deferred.StateCompleted || oldCalls.Load() != 2 || newCalls.Load() != 1 {
		t.Fatal("safe reenable failed to use new catalog or drain duplicated old calls")
	}
	callBinaryDeferred(t, peer, "DELETE", "/jobs/"+newReceipt.ID, "rotated-token", newReceipt.Generation, nil)
	requireBinaryDeferredReleased(t, executor)
	requireBinaryDeferredAccounting(t, executor, "rotated-token", 3, 15)
}

func TestBinaryDeferredTerminalTTLAndDuplicateConflict(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	var calls atomic.Int32
	closed := make(chan struct{})
	prefix := []byte("id: original\ndata: {\"choices\":[],\"usage\":{\"prompt_tokens\":3,\"total_tokens\":3}}\n\n")
	upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		input, _ := io.ReadAll(r.Body)
		if bytes.Contains(input, []byte(`"stream":true`)) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write(prefix)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
			close(closed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"usage":{"prompt_tokens":3,"total_tokens":3}}`)
	}))
	addr := freeAddr(t)
	policy := defaultBinaryDeferredPolicy()
	policy.execution = time.Second
	executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", policy)), addr)
	peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
	capability := binaryDeferredCapability(t, peer, "peer-token")
	request := binaryDeferredSubmitRequest(t, capability, binaryDeferredChatBody("p/m", true))
	receipt := submitBinaryDeferred(t, peer, "peer-token", request)
	replayed := submitBinaryDeferred(t, peer, "peer-token", request)
	if receipt.ID != replayed.ID || !replayed.Replayed {
		t.Fatal("binary duplicate key scheduled a different job")
	}
	conflict := request
	conflict.Body = []byte(`{"model":"p/m","stream":true,"messages":[],"changed":true}`)
	requireBinaryDeferredError(t, callBinaryDeferred(t, peer, "POST", "/submit", "peer-token", "", binaryDeferredWire(t, conflict)), 409, "attempt_conflict")
	awaitBinaryDeferred(t, "native binary prefix", time.Second, func() bool {
		frame := readBinaryDeferred(t, peer, "peer-token", receipt, 0)
		return bytes.Equal(frame.Data, prefix) && !frame.Done
	})
	manifest := finishBinaryDeferred(t, peer, "peer-token", receipt)
	requireBinaryDeferredManifest(t, manifest, prefix, deferred.OutcomeExecutionTimeout)
	awaitBinaryDeferredSignal(t, "fixed execution deadline closed source", closed)
	if manifest.Outcome.Usage.TotalTokens != 3 || manifest.ExecutionDeadlineMS != receipt.ExecutionDeadlineMS ||
		manifest.RetainUntilMS < receipt.ExecutionDeadlineMS+10*time.Second.Milliseconds()-100 {
		t.Fatal("execution/terminal retention clock or observed usage changed")
	}
	requireBinaryDeferredAccounting(t, executor, "peer-token", 1, 3)
	for time.Now().UnixMilli() < manifest.RetainUntilMS-250 {
		frame := readBinaryDeferred(t, peer, "peer-token", receipt, 0)
		if frame.RetainUntilMS != manifest.RetainUntilMS || !frame.Done || !bytes.Equal(frame.Data, prefix) {
			t.Fatal("slow repeated terminal reads renewed retention or changed the failed prefix")
		}
		remaining := time.Until(time.UnixMilli(manifest.RetainUntilMS - 250))
		if remaining > 0 {
			binaryDeferredPause(context.Background(), min(time.Second, remaining))
		}
	}
	binaryDeferredPause(context.Background(), time.Until(time.UnixMilli(manifest.RetainUntilMS).Add(50*time.Millisecond)))
	for _, endpoint := range []struct{ method, suffix string }{
		{"GET", ""}, {"GET", "/result"}, {"GET", "/read?cursor=0"}, {"DELETE", ""},
	} {
		requireBinaryDeferredError(t, callBinaryDeferred(t, peer, endpoint.method, "/jobs/"+receipt.ID+endpoint.suffix, "peer-token", receipt.Generation, nil), 410, "job_expired")
	}
	replayed = submitBinaryDeferred(t, peer, "peer-token", request)
	if !replayed.Replayed || replayed.ID != receipt.ID || calls.Load() != 1 {
		t.Fatal("true binary retention expiry erased protected acceptance or dispatched again")
	}
	requireBinaryDeferredReleased(t, executor)
	metrics := binaryDeferredMetrics(t, executor)
	if metrics["aiproxy_deferred_expirations_total"] != 1 || metrics[`aiproxy_deferred_ownership{kind="entries"}`] != 1 {
		t.Fatal("binary expiry failed to retain only its bounded dedup tombstone")
	}
	requireBinaryDeferredPeerProtocol(t, peer, "h2", "/capabilities", "/submit", "/jobs/"+receipt.ID, "/read", "/result")
}

func TestBinaryDeferredConsumerDisappearsAndCancellationLoss(t *testing.T) {
	for _, loseDelete := range []bool{false, true} {
		t.Run(fmt.Sprintf("lose-delete=%t", loseDelete), func(t *testing.T) {
			trust := newBinaryTLSCertificate(t)
			var calls atomic.Int32
			closed := make(chan struct{})
			prefix := "id: original\ndata: {\"choices\":[]}\n\n"
			upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, prefix)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(closed)
			}))
			addr := freeAddr(t)
			policy := defaultBinaryDeferredPolicy()
			policy.execution = time.Second
			executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", policy)), addr)
			peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
			receipts := make(chan deferred.Receipt, 1)
			peer.hooks(func(w http.ResponseWriter, r *http.Request) bool {
				if loseDelete && r.Method == "DELETE" {
					http.Error(w, "delete request lost before executor", 503)
					return true
				}
				return false
			}, func(response *http.Response) error {
				if response.Request.URL.Path == deferred.Prefix+"/submit" {
					wire, err := binaryDeferredResponseBody(response)
					if err != nil {
						return err
					}
					var receipt deferred.Receipt
					if err := deferred.DecodeControl(wire, &receipt); err != nil {
						return err
					}
					receipts <- receipt
				}
				return nil
			})
			callerAddr := freeAddr(t)
			caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", true, time.Second, 3*time.Second, "")), callerAddr)
			response := openBinaryDeferredChat(t, caller, "peer/m")
			initial := make([]byte, len(prefix))
			if _, err := io.ReadFull(response.Body, initial); err != nil || string(initial) != prefix {
				t.Fatalf("consumer did not see exact initial prefix: %q %v", initial, err)
			}
			_ = response.Body.Close()
			var receipt deferred.Receipt
			select {
			case receipt = <-receipts:
			case <-time.After(3 * time.Second):
				t.Fatal("missing actual binary admission")
			}
			awaitBinaryDeferred(t, "one independent cancellation delete", time.Second, func() bool {
				count := 0
				for _, call := range peer.Calls("") {
					if call.method == "DELETE" {
						count++
					}
				}
				if count > 1 {
					t.Fatal("disappearing consumer spawned unbounded cancellation retries")
				}
				return count == 1
			})
			awaitBinaryDeferredSignal(t, "disappearing consumer source/deadline cleanup", closed)
			if calls.Load() != 1 {
				t.Fatal("consumer loss restarted inference")
			}
			peer.hooks(nil, nil)
			if loseDelete {
				manifest := finishBinaryDeferred(t, peer, "peer-token", receipt)
				requireBinaryDeferredManifest(t, manifest, []byte(prefix), deferred.OutcomeExecutionTimeout)
				frame := readBinaryDeferred(t, peer, "peer-token", receipt, 0)
				if !frame.Done || !bytes.Equal(frame.Data, []byte(prefix)) {
					t.Fatal("lost cancellation lacked the finite execution backstop")
				}
				callBinaryDeferred(t, peer, "DELETE", "/jobs/"+receipt.ID, "peer-token", receipt.Generation, nil)
			} else {
				requireBinaryDeferredError(t, callBinaryDeferred(t, peer, "GET", "/jobs/"+receipt.ID, "peer-token", receipt.Generation, nil), 410, "job_released")
			}
			requireBinaryDeferredReleased(t, executor)
			requireBinaryDeferredAccounting(t, caller, "", 1, 0)
			requireBinaryDeferredAccounting(t, executor, "peer-token", 1, 0)
		})
	}
}

func TestBinaryDeferredGrowthAndShutdown(t *testing.T) {
	t.Run("growth-after-client-commit", func(t *testing.T) {
		trust := newBinaryTLSCertificate(t)
		var calls atomic.Int32
		gate, closed := make(chan struct{}), make(chan struct{})
		release := sync.OnceFunc(func() { close(gate) })
		prefix := "id: original\ndata: {\"choices\":[]}\n\n"
		upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			_, _ = io.Copy(io.Discard, r.Body)
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, prefix)
			w.(http.Flusher).Flush()
			select {
			case <-gate:
				_, _ = io.WriteString(w, "data: "+strings.Repeat("x", 4096)+"\n\n")
				w.(http.Flusher).Flush()
			case <-r.Context().Done():
			}
			<-r.Context().Done()
			close(closed)
		}))
		t.Cleanup(release)
		addr := freeAddr(t)
		policy := defaultBinaryDeferredPolicy()
		policy.result = 1024
		executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", policy)), addr)
		peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
		sealed := make(chan deferred.ResultManifest, 1)
		peer.hooks(nil, func(response *http.Response) error {
			if !strings.HasSuffix(response.Request.URL.Path, "/result") || response.StatusCode != 200 {
				return nil
			}
			wire, err := binaryDeferredResponseBody(response)
			if err != nil {
				return err
			}
			var manifest deferred.ResultManifest
			if err := deferred.DecodeControl(wire, &manifest); err != nil {
				return err
			}
			sealed <- manifest
			return nil
		})
		callerAddr := freeAddr(t)
		caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", true, time.Second, 5*time.Second, "")), callerAddr)
		response := openBinaryDeferredChat(t, caller, "peer/m")
		initial := make([]byte, len(prefix))
		if _, err := io.ReadFull(response.Body, initial); err != nil || string(initial) != prefix {
			t.Fatalf("growth test did not reach client commitment: %q %v", initial, err)
		}
		before := binaryDeferredMetrics(t, executor)
		if before[`aiproxy_deferred_bytes{kind="output"}`] != deferred.ResultSlabBytes {
			t.Fatal("binary did not reserve bounded slab capacity before growth")
		}
		release()
		remaining, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		growth := []byte("data: " + strings.Repeat("x", 4096) + "\n\n")
		if response.StatusCode != 200 || err == nil || len(remaining) > int(policy.result)-len(prefix) ||
			!bytes.HasPrefix(growth, remaining) || calls.Load() != 1 {
			t.Fatalf("growth produced a truncated success or replacement: status=%d err=%v remaining=%q calls=%d", response.StatusCode, err, remaining, calls.Load())
		}
		select {
		case manifest := <-sealed:
			requireBinaryDeferredManifest(t, manifest, append([]byte(prefix), remaining...), deferred.OutcomeLimitExceeded)
		case <-time.After(3 * time.Second):
			t.Fatal("growth failure lacked its exact bounded terminal manifest")
		}
		awaitBinaryDeferredSignal(t, "growth failure closed its source", closed)
		requireBinaryDeferredReleased(t, executor)
		requireBinaryDeferredAccounting(t, caller, "", 1, 0)
		requireBinaryDeferredAccounting(t, executor, "peer-token", 1, 0)
	})
	t.Run("shutdown-cancels-admitted-source", func(t *testing.T) {
		trust := newBinaryTLSCertificate(t)
		started, closed := make(chan struct{}), make(chan struct{})
		upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			close(started)
			<-r.Context().Done()
			close(closed)
		}))
		addr := freeAddr(t)
		policy := defaultBinaryDeferredPolicy()
		policy.execution = 10 * time.Second
		executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", policy)), addr)
		peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
		capability := binaryDeferredCapability(t, peer, "peer-token")
		submitBinaryDeferred(t, peer, "peer-token", binaryDeferredSubmitRequest(t, capability, binaryDeferredChatBody("p/m", false)))
		awaitBinaryDeferredSignal(t, "live binary shutdown source", started)
		start := time.Now()
		executor.Stop(t)
		awaitBinaryDeferredSignal(t, "shutdown source cancellation", closed)
		if time.Since(start) >= 3*time.Second {
			t.Fatal("binary shutdown waited for the original long execution deadline")
		}
	})
}

func openBinaryDeferredChat(t *testing.T, caller *binaryServer, model string) *http.Response {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	t.Cleanup(cancel)
	request, err := http.NewRequestWithContext(ctx, "POST", caller.baseURL+"/v1/chat/completions", bytes.NewReader(binaryDeferredChatBody(model, true)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := caller.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = response.Body.Close() })
	return response
}

func TestBinaryDeferredCallerSIGHUPKeepsAdmittedPeer(t *testing.T) {
	trust := newBinaryTLSCertificate(t)
	var calls, wrongPeerCalls atomic.Int32
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	first := "id: original\ndata: {\"choices\":[]}\n\n"
	last := "data: {\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\ndata: [DONE]\n\n"
	upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, first)
		w.(http.Flusher).Flush()
		select {
		case <-gate:
			_, _ = io.WriteString(w, last)
		case <-r.Context().Done():
		}
	}))
	t.Cleanup(release)
	addr := freeAddr(t)
	executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", defaultBinaryDeferredPolicy())), addr)
	peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
	wrong := newBinaryTLSFixture(t, trust, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wrongPeerCalls.Add(1)
		if r.Header.Get("Authorization") != "Bearer rotated-wrong-token" {
			t.Error("new caller request did not use rotated credentials")
		}
		http.Error(w, "new peer rejects control", 401)
	}))
	callerAddr := freeAddr(t)
	path := writeConfig(t, binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", true, time.Second, 5*time.Second, ""))
	caller := startBinaryDeferred(t, trust, path, callerAddr)
	response := openBinaryDeferredChat(t, caller, "peer/m")
	initial := make([]byte, len(first))
	if _, err := io.ReadFull(response.Body, initial); err != nil || string(initial) != first {
		t.Fatal("binary caller did not reach pre-SIGHUP commitment")
	}
	writeConfigPath(t, path, binaryDeferredCallerConfig(callerAddr, wrong.URL(), "rotated-wrong-token", true, time.Second, 5*time.Second, ""))
	reloadBinaryTLS(t, caller, true)
	release()
	tail, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(tail) != last || calls.Load() != 1 || wrongPeerCalls.Load() != 0 {
		t.Fatalf("SIGHUP changed admitted peer endpoint/credential/transport: %q %v calls=%d wrong=%d", tail, err, calls.Load(), wrongPeerCalls.Load())
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reply := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredChatBody("peer/m", false), false)
	if reply.status != 502 || reply.err != nil || !bytes.Contains(reply.body, []byte("deferred_protocol_error")) || wrongPeerCalls.Load() != 1 || calls.Load() != 1 {
		t.Fatalf("new caller request did not use candidate peer or silently fell back: %d %s %v", reply.status, reply.body, reply.err)
	}
	requireNoBinaryDeferredEnvelope(t, reply)
	requireBinaryDeferredReleased(t, executor)
	requireBinaryDeferredAccounting(t, executor, "peer-token", 1, 5)
	requireBinaryDeferredAccounting(t, caller, "", 2, 5)
}

func TestBinaryDeferredGuardrailParityAndErrorNormalization(t *testing.T) {
	t.Run("guardrail-cached-rejection", func(t *testing.T) {
		trust := newBinaryTLSCertificate(t)
		var calls atomic.Int32
		upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
		addr := freeAddr(t)
		cfg := binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", defaultBinaryDeferredPolicy()) +
			"\ningress_guardrails {\n enabled = true\n mode = \"block\"\n}\n"
		executor := startBinaryDeferred(t, trust, writeConfig(t, cfg), addr)
		peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
		capability := binaryDeferredCapability(t, peer, "peer-token")
		secret := guardrailBinaryKey(t)
		body := []byte(`{"model":"p/m","messages":[{"role":"user","content":"deploy with ` + secret + ` now"}]}`)
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		ordinary := binaryDeferredHTTP(ctx, executor.client, executor.baseURL, "POST", "/v1/chat/completions", "peer-token", "", body, false)
		request := binaryDeferredSubmitRequest(t, capability, body)
		rejected := callBinaryDeferred(t, peer, "POST", "/submit", "peer-token", "", binaryDeferredWire(t, request))
		replay := callBinaryDeferred(t, peer, "POST", "/submit", "peer-token", "", binaryDeferredWire(t, request))
		if ordinary.status != 400 || rejected.status != ordinary.status || !bytes.Equal(ordinary.body, rejected.body) ||
			replay.status != rejected.status || !bytes.Equal(replay.body, rejected.body) || calls.Load() != 0 || bytes.Contains(rejected.body, []byte(secret)) {
			t.Fatal("binary deferred/ordinary guardrail rejection or cached replay diverged")
		}
		requireNoBinaryDeferredEnvelope(t, ordinary)
		callerAddr := freeAddr(t)
		caller := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", true, time.Second, 3*time.Second, "")), callerAddr)
		body = bytes.Replace(body, []byte(`"p/m"`), []byte(`"peer/m"`), 1)
		external := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", body, false)
		if external.status != 400 || external.err != nil || !bytes.Contains(external.body, []byte(`"secret_blocked"`)) || calls.Load() != 0 {
			t.Fatal("caller exposed an envelope or retried blocked inference")
		}
		requireNoBinaryDeferredEnvelope(t, external)
		requireBinaryDeferredReleased(t, executor)
	})
	for _, contentType := range []string{"application/json", "text/html"} {
		t.Run("error/"+contentType, func(t *testing.T) {
			trust := newBinaryTLSCertificate(t)
			var calls atomic.Int32
			body := []byte(`{"error":{"type":"vendor_error","message":"exact upstream failure"},"vendor":true}`)
			if contentType == "text/html" {
				body = []byte("<html>upstream rejected the request</html>")
			}
			upstream := newBinaryDeferredModel(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.Copy(io.Discard, r.Body)
				w.Header().Set("Content-Type", contentType)
				w.Header().Set("X-Vendor", "original")
				w.Header().Set("Retry-After", "2")
				w.Header().Set("Connection", "X-Hop")
				w.Header().Set("X-Hop", "must-not-escape")
				w.Header().Set("X-Aiproxy-Deferred-Upstream", "must-not-escape")
				w.WriteHeader(403)
				_, _ = w.Write(body)
			}))
			addr := freeAddr(t)
			executor := startBinaryDeferred(t, trust, writeConfig(t, binaryDeferredExecutorConfig(addr, upstream.URL, "peer-token", "ultimate-key", "actual-model", defaultBinaryDeferredPolicy())), addr)
			peer := newBinaryDeferredPeer(t, trust, executor.baseURL, "h2", 200*time.Millisecond)
			callerAddr := freeAddr(t)
			cfg := binaryDeferredCallerConfig(callerAddr, peer.URL(), "peer-token", true, time.Second, 3*time.Second, "") +
				"\nprovider \"openai-compatible\" \"ordinary\" {\n extends = \"peer\"\n api_key = \"peer-token\"\n}\n"
			caller := startBinaryDeferred(t, trust, writeConfig(t, cfg), callerAddr)
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			ordinary := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredChatBody("ordinary/m", false), false)
			opted := binaryDeferredHTTP(ctx, caller.client, caller.baseURL, "POST", "/v1/chat/completions", "", "", binaryDeferredChatBody("peer/m", false), false)
			if ordinary.status != 403 || opted.status != ordinary.status || ordinary.err != nil || opted.err != nil ||
				!bytes.Equal(ordinary.body, opted.body) || calls.Load() != 2 || len(peer.Calls("/v1/chat/completions")) != 1 {
				t.Fatalf("binary ordinary/deferred error normalization diverged: ordinary=%d %s %v opted=%d %s %v", ordinary.status, ordinary.body, ordinary.err, opted.status, opted.body, opted.err)
			}
			if contentType == "application/json" && !bytes.Equal(opted.body, body) {
				t.Fatal("deferred provider JSON error lost exact bytes")
			}
			for _, reply := range []binaryDeferredReply{ordinary, opted} {
				requireNoBinaryDeferredEnvelope(t, reply)
				expectedVendor, expectedRetry := "original", "2"
				if contentType != "application/json" {
					expectedVendor, expectedRetry = "", ""
				}
				if reply.header.Get("X-Vendor") != expectedVendor || reply.header.Get("X-Hop") != "" || reply.header.Get("Retry-After") != expectedRetry ||
					reply.header.Get("Content-Type") != "application/json" || !json.Valid(reply.body) {
					t.Fatal("original status/head filtering/non-JSON normalization changed")
				}
			}
			requireBinaryDeferredReleased(t, executor)
			requireBinaryDeferredAccounting(t, executor, "peer-token", 2, 0)
			requireBinaryDeferredAccounting(t, caller, "", 2, 0)
		})
	}
}
