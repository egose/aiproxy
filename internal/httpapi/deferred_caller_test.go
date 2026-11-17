package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
)

func callerDispatchRuntime(peer, backup string) *config.Runtime {
	options := config.DefaultDeferredTransport()
	options.ControlTimeout, options.MaxWait, options.PollInterval = 200*time.Millisecond, 3*time.Second, 50*time.Millisecond
	return &config.Runtime{Catalog: config.NewCatalog([]config.Provider{
		{Type: config.ProviderTypeOpenAICompatible, Name: "p1", BaseURL: peer + "/v1", APIKey: "peer-token",
			Models: []config.Model{{Name: "m", UpstreamName: "p/m", DeferredTransport: &options}}},
		{Type: config.ProviderTypeOpenAICompatible, Name: "p2", BaseURL: backup + "/v1", APIKey: "backup-token",
			Models: []config.Model{{Name: "m", UpstreamName: "backup-model"}}},
	}, nil, []config.Alias{{
		Name: "a", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{400, 429, 500, 502, 503, 504},
		Targets:            []config.AliasTarget{{Provider: "p1", Model: "m"}, {Provider: "p2", Model: "m"}},
		EncryptedReasoning: &config.EncryptedReasoning{Passthrough: true, OnCallerMismatch: config.EncryptedReasoningStripAndRetry},
	}})}
}

func callerDispatchHandler(rt *config.Runtime, adapter provider.Adapter) (*Handler, *accounting.MemoryRecorder, *providerhealth.Tracker) {
	metrics := observability.NewMetrics()
	health := providerhealth.New(metrics, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	accounting := &accounting.MemoryRecorder{}
	h := NewHandler(Dependencies{
		Resolver: modelresolver.New(rt), Catalog: rt.Catalog, Adapter: adapter,
		Auth: auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}), Accounting: accounting,
		Health: health, Metrics: metrics, MetricsToken: metricsTestToken,
		HasAccessLog: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	return h, accounting, health
}

func assertNoCallerCooldown(t *testing.T, h *Handler) {
	t.Helper()
	for _, name := range []string{"p1", "p2"} {
		prov, model, _ := h.current().Catalog.Model(name, "m")
		fp := modelresolver.CooldownFingerprintFor("a", prov, model)
		if remaining, ok := h.current().Resolver.Cooldowns().Remaining(fp); ok || remaining != 0 {
			t.Fatalf("private control created model cooldown for %s: %s", name, remaining)
		}
	}
}

func TestDeferredCallerTypedAndOriginControlOutcomesBypassAliasReasoningHealthAndCooldown(t *testing.T) {
	for _, direct := range []bool{false, true} {
		for _, code := range []string{
			"deferred_protocol_error", "deferred_submit_indeterminate", "deferred_request_lost",
			"deferred_control_error", "deferred_wait_timeout", "deferred_execution_timeout", "deferred_limit_exceeded", "admission",
		} {
			t.Run(code+"/direct="+map[bool]string{false: "false", true: "true"}[direct], func(t *testing.T) {
				var calls, closes atomic.Int32
				rt := callerDispatchRuntime("http://peer", "http://backup")
				rt.EncryptedReasoning = scopedReasoningPolicy("retry")
				adapter := deferredAdapterFunc(func(ctx context.Context, request provider.Request) (*provider.Result, error) {
					calls.Add(1)
					if request.DeferredTransport == nil || request.APIKey != "peer-token" {
						t.Error("caller configuration not wired or alias selected a fresh target")
					}
					result := &provider.Result{
						StatusCode: 400, Origin: provider.OriginPeerControl, Header: http.Header{"Content-Type": {"application/json"}, "Retry-After": {"3600"}},
						Body:       []byte(`{"error":{"type":"invalid_request","message":"encrypted_content was not issued to this caller","block_id":"rule-1"}}`),
						RetryDelay: time.Hour, HasRetryDelay: true, Usage: provider.Usage{PromptTokens: 2, TotalTokens: 2},
						OnClose: func() { closes.Add(1) },
					}
					if code == "admission" {
						return result, nil
					}
					control := &provider.DeferredControlError{Code: code, Acceptance: deferred.AcceptanceAccepted, Err: io.ErrUnexpectedEOF}
					return result, &provider.CooldownError{Err: control, Delay: time.Hour, OK: true}
				})
				h, recorder, health := callerDispatchHandler(rt, adapter)
				defer health.Close()
				model := "alias/a"
				if direct {
					model = "p1/m"
				}
				w := httptest.NewRecorder()
				h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions",
					strings.NewReader(`{"model":"`+model+`","messages":[{"role":"assistant","content":[{"type":"redacted_thinking","encrypted_content":"opaque"}]}]}`)))
				expected := 502
				if code == "deferred_wait_timeout" || code == "deferred_execution_timeout" {
					expected = 504
				} else if code == "admission" {
					expected = 400
				}
				if w.Code != expected || calls.Load() != 1 || closes.Load() != 1 || !health.IsHealthy("p1") || !health.IsHealthy("p2") ||
					len(recorder.Events()) != 1 || recorder.Events()[0].TotalTokens != 2 {
					t.Fatalf("control retried or finalized incorrectly: status=%d calls=%d closes=%d events=%+v health=%t", w.Code, calls.Load(), closes.Load(), recorder.Events(), health.IsHealthy("p1"))
				}
				if code == "admission" && !strings.Contains(w.Body.String(), `"block_id":"rule-1"`) {
					t.Fatal("ordinary admission error lost its block ID")
				}
				assertNoCallerCooldown(t, h)
				metrics := collectMetrics(t, h)
				if strings.Contains(metrics, `aiproxy_upstream_requests_total{operation="chat_completions"`) ||
					strings.Contains(metrics, `aiproxy_alias_retries_total{alias="a"`) {
					t.Fatal("peer control counted as a provider result or alias retry")
				}
				if !direct && metricValue(t, metrics, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`) != 0 {
					t.Fatal("control outcome leaked an alias lease")
				}
			})
		}
	}
}

func TestDeferredCallerDefiniteInferenceOutcomesRetainAliasFailover(t *testing.T) {
	for _, outcome := range []string{"http-error", "transport-error", "limit", "control-loss"} {
		t.Run(outcome, func(t *testing.T) {
			var executorCalls, backupCalls atomic.Int32
			adapter := deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
				executorCalls.Add(1)
				switch outcome {
				case "http-error":
					return &provider.Result{StatusCode: 503, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{"error":{"type":"overloaded","message":"ordinary inference"}}`)}, nil
				case "transport-error":
					return nil, io.ErrUnexpectedEOF
				case "limit":
					return nil, provider.ErrExecutionLimit{Kind: "result", Limit: 128 << 10}
				default:
					return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
				}
			})
			executor := newDeferredHTTPFixture(t, adapter)
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if outcome == "control-loss" && strings.HasSuffix(r.URL.Path, "/read") {
					deferredResponseHeaders(w)
					w.WriteHeader(410)
					_, _ = io.WriteString(w, `{"protocol":"aiproxy-deferred","version":1,"error":{"type":"job_expired","message":"lost"},"acceptance":"not_accepted"}`)
					return
				}
				executor.h.ServeHTTP(w, r)
			}))
			defer peer.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				backupCalls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"backup":true,"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}`)
			}))
			defer backup.Close()
			rt := callerDispatchRuntime(peer.URL, backup.URL)
			h, recorder, health := callerDispatchHandler(rt, provider.New())
			defer health.Close()
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"alias/a","messages":[]}`)))
			retry := outcome == "http-error" || outcome == "transport-error"
			if retry {
				if w.Code != 200 || backupCalls.Load() != 1 || health.IsHealthy("p1") || !strings.Contains(w.Body.String(), `"backup":true`) {
					t.Fatalf("definite inference did not retain ordinary failover/health: status=%d backup=%d healthy=%t body=%s", w.Code, backupCalls.Load(), health.IsHealthy("p1"), w.Body.String())
				}
			} else if w.Code != 502 || backupCalls.Load() != 0 || !health.IsHealthy("p1") {
				t.Fatalf("accepted limit/loss retried or changed health: status=%d backup=%d healthy=%t", w.Code, backupCalls.Load(), health.IsHealthy("p1"))
			}
			if executorCalls.Load() > 1 || len(recorder.Events()) != 1 || executor.rate.calls.Load() != 1 {
				t.Fatalf("polls duplicated inference/accounting: executor=%d rate=%d events=%d", executorCalls.Load(), executor.rate.calls.Load(), len(recorder.Events()))
			}
			assertNoCallerCooldown(t, h)
		})
	}
}

func TestDeferredCallerTransportLossAfterHeadAbortsWireWithoutFailoverOrHealthMutation(t *testing.T) {
	var backupCalls atomic.Int32
	ultimate := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: original-prefix\n\n")
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer ultimate.Close()
	executor := newDeferredHTTPFixture(t, provider.New())
	deps := executor.h.current()
	prov, _ := deps.Catalog.Provider("p")
	prov.BaseURL = ultimate.URL + "/v1"
	deps.Catalog = config.NewCatalog([]config.Provider{prov}, nil, nil)
	deps.Resolver = modelresolver.New(&config.Runtime{Catalog: deps.Catalog})
	executor.h.UpdateDependencies(deps)
	var seenData atomic.Bool
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/read") {
			if seenData.Load() {
				deferredResponseHeaders(w)
				w.WriteHeader(410)
				_, _ = io.WriteString(w, `{"protocol":"aiproxy-deferred","version":1,"error":{"type":"job_unavailable","message":"lost"},"acceptance":"accepted"}`)
				return
			}
			response := httptest.NewRecorder()
			executor.h.ServeHTTP(response, r)
			var frame deferred.ReadFrame
			if deferred.DecodeControl(response.Body.Bytes(), &frame) == nil && len(frame.Data) > 0 {
				seenData.Store(true)
			}
			for name, values := range response.Header() {
				w.Header()[name] = values
			}
			w.WriteHeader(response.Code)
			_, _ = w.Write(response.Body.Bytes())
			return
		}
		executor.h.ServeHTTP(w, r)
	}))
	defer peer.Close()
	backup := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { backupCalls.Add(1) }))
	defer backup.Close()
	h, recorder, health := callerDispatchHandler(callerDispatchRuntime(peer.URL, backup.URL), provider.New())
	defer health.Close()
	server := httptest.NewServer(h)
	defer server.Close()
	response, err := server.Client().Post(server.URL+"/v1/chat/completions", "application/json",
		strings.NewReader(`{"model":"alias/a","stream":true,"messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 200 || !errors.Is(err, io.ErrUnexpectedEOF) || string(data) != "data: original-prefix\n\n" || backupCalls.Load() != 0 || !health.IsHealthy("p1") {
		t.Fatalf("committed retrieval loss became success/replacement: status=%d err=%v data=%q backup=%d healthy=%t", response.StatusCode, err, data, backupCalls.Load(), health.IsHealthy("p1"))
	}
	awaitDeferredHTTP(t, "caller stream completion", func() bool { return len(recorder.Events()) == 1 })
	assertNoCallerCooldown(t, h)
	metrics := collectMetrics(t, h)
	if strings.Contains(metrics, `aiproxy_upstream_requests_total{operation="chat_completions"`) {
		t.Fatal("retrieval failure was reported as a definite provider success/error")
	}
}

func TestDeferredCallerControlSuccessDoesNotRecoverUnhealthyProvider(t *testing.T) {
	rt := callerDispatchRuntime("http://peer", "http://backup")
	h, _, health := callerDispatchHandler(rt, nil)
	defer health.Close()
	health.MarkFailure("p1")
	h.recordProviderHealth(h.current(), context.Background(), "p1", &provider.Result{Origin: provider.OriginPeerControl, StatusCode: 200}, nil, false)
	if health.IsHealthy("p1") {
		t.Fatal("private control success mutated model provider health")
	}
}

func TestDeferredCallerCooldownUsesOnlyOriginalRemainingDeadline(t *testing.T) {
	rt := callerDispatchRuntime("http://peer", "http://backup")
	h, _, health := callerDispatchHandler(rt, nil)
	defer health.Close()
	prov, model, _ := rt.Catalog.Model("p1", "m")
	fp := modelresolver.CooldownFingerprintFor("a", prov, model)
	result := &provider.Result{
		Origin: provider.OriginInference, StatusCode: 429, Header: http.Header{"Retry-After": {"3600"}},
		HasRetryDelay: true, RetryDelay: time.Hour, RetryObservedAt: time.Now().Add(-500 * time.Millisecond), RetryDeadline: time.Now().Add(200 * time.Millisecond),
	}
	observeCooldownResult(h.current().Resolver.Cooldowns(), context.Background(), fp, result)
	if delay, ok := h.current().Resolver.Cooldowns().Remaining(fp); !ok || delay <= 0 || delay > 200*time.Millisecond {
		t.Fatalf("original retry headers restarted delayed inference advice: %s", delay)
	}
	expired := *result
	expired.RetryDeadline = time.Now().Add(-time.Second)
	store := modelresolver.NewCooldownStore()
	observeCooldownResult(store, context.Background(), fp, &expired)
	if _, ok := store.Remaining(fp); ok {
		t.Fatal("expired inference advice was reparsed/restarted")
	}
}

func TestDeferredCallerVerifiedFailedInferenceKeepsKnownUsageAndInferencePayload(t *testing.T) {
	for _, aliasRequest := range []bool{false, true} {
		t.Run("alias="+map[bool]string{false: "false", true: "true"}[aliasRequest], func(t *testing.T) {
			stream := provider.NewStreamCompletion()
			stream.SetUsage(provider.Usage{PromptTokens: 4, TotalTokens: 4})
			executor := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
				return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}},
					Streaming: true, StreamBody: &errorAfterReader{data: []byte("data: native-failed-prefix\n\n"), err: io.ErrUnexpectedEOF}, Stream: stream}, nil
			}))
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				response := httptest.NewRecorder()
				executor.h.ServeHTTP(response, r)
				if strings.HasSuffix(r.URL.Path, "/submit") {
					awaitDeferredHTTP(t, "known terminal transport failure", func() bool { return executor.store.Stats().Running == 0 })
				}
				for name, values := range response.Header() {
					w.Header()[name] = values
				}
				w.WriteHeader(response.Code)
				_, _ = w.Write(response.Body.Bytes())
			}))
			defer peer.Close()
			rt := callerDispatchRuntime(peer.URL, "http://127.0.0.1:1")
			aliases := rt.Catalog.Aliases()
			aliases[0].Targets = aliases[0].Targets[:1]
			rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
			h, recorder, health := callerDispatchHandler(rt, provider.New())
			defer health.Close()
			payload := &deferredPayloadRecorder{max: 4096}
			deps := h.current()
			deps.PayloadLog = payload
			h.UpdateDependencies(deps)
			model := "p1/m"
			if aliasRequest {
				model = "alias/a"
			}
			request := httptest.NewRequest("POST", "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","stream":true,"messages":[]}`))
			request.Header.Set("X-Aiproxy-Deferred-Submit-Token", "must-not-log")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, request)
			if w.Code != 502 || strings.Contains(w.Body.String(), "native-failed-prefix") || len(recorder.Events()) != 1 ||
				recorder.Events()[0].TotalTokens != 4 || health.IsHealthy("p1") {
				t.Fatalf("verified pre-commit inference failure lost usage or exposed a prefix: status=%d body=%s events=%+v", w.Code, w.Body.String(), recorder.Events())
			}
			entries := payload.snapshot()
			if len(entries) != 1 {
				t.Fatalf("private controls created inference payload entries: %d", len(entries))
			}
			wire, _ := json.Marshal(entries[0])
			if !strings.Contains(string(wire), `"model":"p/m"`) || strings.Contains(string(wire), "submit_token") ||
				strings.Contains(string(wire), "must-not-log") || strings.Contains(string(wire), "peer-token") {
				t.Fatal("payload did not preserve inference bodies separately from private envelopes/credentials")
			}
		})
	}
}
