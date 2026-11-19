package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
)

func TestScopedEncryptedReasoningImmediateStreamingErrorAndEstablishedSSE(t *testing.T) {
	for _, op := range []provider.Operation{provider.OpChatCompletions, provider.OpResponses} {
		for _, target := range []string{"p1/m", "alias/a"} {
			for _, mode := range []string{"immediate-error", "established-error", "prestrip"} {
				t.Run(op.String()+"/"+target+"/"+mode, func(t *testing.T) {
					prefix := "data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n"
					if op == provider.OpResponses {
						prefix = "event: response.completed\ndata: {\"type\":\"response.completed\",\"response\":{\"usage\":{\"input_tokens\":4,\"output_tokens\":6,\"total_tokens\":10}}}\n\n"
					}
					tail := "data: [DONE]\n\n"
					if mode == "established-error" {
						tail = "event: error\ndata: {\"error\":{\"message\":\"encrypted_content was not issued to this caller\"}}\n\n"
					}
					var upstreamCalls atomic.Int32
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						call := upstreamCalls.Add(1)
						_, _ = io.Copy(io.Discard, r.Body)
						w.Header().Set("X-Upstream-Marker", "stream")
						if mode == "immediate-error" && call == 1 {
							w.Header().Set("Content-Type", "application/json")
							w.WriteHeader(400)
							_, _ = io.WriteString(w, scopedReasoningMismatch)
							return
						}
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, prefix)
						w.(http.Flusher).Flush()
						_, _ = io.WriteString(w, tail)
					}))
					defer upstream.Close()
					root := "retry"
					if mode == "prestrip" {
						root = "strip"
					}
					rt := scopedReasoningRuntime(root, "", nil, nil)
					providers := rt.Catalog.Providers()
					providers[0].BaseURL = upstream.URL
					rt.Catalog = config.NewCatalog(providers, nil, rt.Catalog.Aliases())
					adapter := provider.New()
					probe := &scopedReasoningProbe{do: func(_ int, ctx context.Context, request provider.Request) (*provider.Result, error) {
						return adapter.Do(ctx, request)
					}}
					h, recorder, clients := scopedReasoningHandler(t, rt, probe)
					for _, client := range clients {
						defer client.CloseIdleConnections()
					}
					original, stripped := scopedReasoningBodies(op, target)
					original = bytes.Replace(original, []byte(`{ "model"`), []byte(`{ "stream":true,"model"`), 1)
					stripped = bytes.Replace(stripped, []byte(`{"model"`), []byte(`{"stream":true,"model"`), 1)
					ctx := context.Background()
					w := scopedReasoningPost(h, ctx, op, original)
					attempts := probe.snapshot()
					expectedCalls := 1
					if mode == "immediate-error" {
						expectedCalls = 2
					}
					if w.Code != 200 || w.Body.String() != prefix+tail || len(attempts) != expectedCalls || int(upstreamCalls.Load()) != expectedCalls {
						t.Fatalf("streaming changed delivery or inference count: status=%d calls=%d body=%q", w.Code, len(attempts), w.Body.String())
					}
					for i, attempt := range attempts {
						assertScopedReasoningAttempt(t, attempt, rt, clients, ctx, op, target, "p1")
						assertScopedReasoningBody(t, attempt.Request.Body, original, stripped, mode == "prestrip" || i == 1)
						if attempt.Closes.Load() != 1 {
							t.Fatalf("streaming attempt %d finalized %d times", i+1, attempt.Closes.Load())
						}
						if mode == "immediate-error" && i == 0 {
							if attempt.Result.Streaming || attempt.BodyCloses.Load() != 0 {
								t.Fatal("immediate streaming request error was treated as an established stream")
							}
						} else if !attempt.Result.Streaming || attempt.BodyCloses.Load() != 1 {
							t.Fatal("established stream lost once-only body close/finalization")
						}
					}
					events := recorder.Events()
					if len(events) != 1 || events[0].TotalTokens != 10 || events[0].Provider != "p1" {
						t.Fatalf("stream recovery duplicated/lost inbound usage: %+v", events)
					}
					metrics := collectMetrics(t, h)
					if mode == "immediate-error" &&
						metricValue(t, metrics, fmt.Sprintf(`aiproxy_upstream_requests_total{operation=%q,outcome="http_400",provider="p1"}`, op.String())) != 1 {
						t.Fatal("immediate mismatch did not retain its per-attempt metric")
					}
					finalOutcome := "success"
					if mode == "established-error" {
						finalOutcome = "transport_error"
					}
					if metricValue(t, metrics, fmt.Sprintf(`aiproxy_upstream_requests_total{operation=%q,outcome=%q,provider="p1"}`, op.String(), finalOutcome)) != 1 ||
						metricValue(t, metrics, fmt.Sprintf(`aiproxy_upstream_response_body_bytes_count{operation=%q,outcome=%q,provider="p1"}`, op.String(), finalOutcome)) != 1 {
						t.Fatal("stream finalizers lost or duplicated upstream telemetry")
					}
					route02GaugeZero(t, metrics, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`)
				})
			}
		}
	}
}

func TestScopedEncryptedReasoningAccountingQuotaAndAttemptTelemetry(t *testing.T) {
	for _, op := range []provider.Operation{provider.OpChatCompletions, provider.OpResponses} {
		for _, target := range []string{"p1/m", "alias/a"} {
			t.Run(op.String()+"/"+target, func(t *testing.T) {
				rt := scopedReasoningRuntime("retry", "", nil, nil)
				statuses := []int{400, 200}
				targets := []string{"p1", "p1"}
				if target == "alias/a" {
					statuses, targets = []int{400, 503, 200}, []string{"p1", "p1", "p2"}
				}
				probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
					if call >= len(statuses) {
						t.Errorf("unexpected inference attempt %d", call+1)
						return scopedReasoningResult(200), nil
					}
					result := scopedReasoningResult(statuses[call])
					if call == 0 {
						result.Usage = provider.Usage{PromptTokens: 100, TotalTokens: 100}
					}
					result.StreamBody = io.NopCloser(strings.NewReader("discarded body resource"))
					return result, nil
				}}
				h, recorder, _ := scopedReasoningHandler(t, rt, probe)
				deps := h.current()
				client := dynamicDeferredClient()
				quota := &deferredQuotaStore{}
				rate := &deferredRateLimiter{}
				rate.allowed.Store(true)
				configuration := config.Auth{Mode: config.AuthModeBearerStatic}
				deps.Auth, deps.Authorizer = auth.NewAuthenticatorWithClients(configuration, []auth.DynamicClient{client}), auth.NewAuthorizerWithClients(configuration, []auth.DynamicClient{client})
				deps.MultiTenancy.Enabled, deps.Quota, deps.RateLimiter = true, NewQuotaTrackerWithStore(quota), rate
				rt.Catalog = withTestWorkspace(rt.Catalog, client.Workspace)
				deps.Catalog, deps.Resolver = rt.Catalog, modelresolver.New(rt)
				var logs bytes.Buffer
				deps.AccessLog = true
				deps.Logger = slog.New(slog.NewJSONHandler(&logs, nil))
				h.UpdateDependencies(deps)
				body, _ := scopedReasoningBodies(op, target)
				path := "/v1/chat/completions"
				if op == provider.OpResponses {
					path = "/v1/responses"
				}
				request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
				request.Header.Set("Content-Type", "application/json")
				request.Header.Set("Authorization", "Bearer peer-token")
				w := httptest.NewRecorder()
				h.ServeHTTP(w, request)
				attempts := probe.snapshot()
				if w.Code != 200 || len(attempts) != len(statuses) || rate.calls.Load() != 1 || quota.reads.Load() != 2 || quota.inserts.Load() != 1 {
					t.Fatalf("recovery reran admission/quota: status=%d attempts=%d rate=%d quota reads/inserts=%d/%d", w.Code, len(attempts), rate.calls.Load(), quota.reads.Load(), quota.inserts.Load())
				}
				events := recorder.Events()
				finalProvider := targets[len(targets)-1]
				if len(events) != 1 || events[0].Client != client.Name || events[0].Tenant != client.Tenant ||
					events[0].Provider != finalProvider || events[0].TotalTokens != 10 || events[0].ReasoningEffort != "high" {
					t.Fatalf("recovery charged intermediate usage or changed identity: %+v", events)
				}
				quota.mu.Lock()
				if len(quota.entries) != 1 || quota.entries[0].KeyID != client.KeyID ||
					quota.entries[0].Model != target || quota.entries[0].Tokens != 10 {
					t.Error("recovery duplicated or changed persistent usage")
				}
				quota.mu.Unlock()
				workspaceID, _ := client.Workspace.ID()
				if tokens, _ := deps.Quota.windowTokens(workspaceID, quotaScope{typ: quotaScopeUser, id: client.OwnerUserID}, target); tokens != 10 {
					t.Fatalf("local usage charged %d tokens, want only the final 10", tokens)
				}
				if strings.Count(logs.String(), `"msg":"upstream request started"`) != len(statuses) ||
					strings.Count(logs.String(), `"msg":"upstream request finished"`) != len(statuses) {
					t.Fatalf("recovery lost per-attempt access logs: %s", logs.String())
				}
				metrics := collectMetrics(t, h)
				for i, attempt := range attempts {
					if attempt.Closes.Load() != 1 || attempt.BodyCloses.Load() != 1 {
						t.Fatalf("attempt %d closed/finalized %d/%d times", i+1, attempt.BodyCloses.Load(), attempt.Closes.Load())
					}
					outcome := fmt.Sprintf("http_%d", statuses[i])
					if statuses[i] >= 500 {
						outcome = "http_5xx"
					} else if statuses[i] < 400 {
						outcome = "success"
					}
					if metricValue(t, metrics, fmt.Sprintf(`aiproxy_upstream_requests_total{operation=%q,outcome=%q,provider=%q}`, op.String(), outcome, targets[i])) != 1 ||
						metricValue(t, metrics, fmt.Sprintf(`aiproxy_upstream_response_body_bytes_count{operation=%q,outcome=%q,provider=%q}`, op.String(), outcome, targets[i])) != 1 {
						t.Fatalf("attempt %d lost or duplicated upstream telemetry", i+1)
					}
				}
			})
		}
	}
}

func TestScopedEncryptedReasoningDirectCooldownIsolation(t *testing.T) {
	for _, existing := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", existing), func(t *testing.T) {
			rt := scopedReasoningRuntime("retry", "", nil, nil)
			probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
				status := 200
				if call == 0 {
					status = 400
				}
				result := scopedReasoningResult(status)
				result.HasRetryDelay, result.RetryDelay = true, 2*time.Hour
				return result, nil
			}}
			h, _, _ := scopedReasoningHandler(t, rt, probe)
			prov, model, _ := rt.Catalog.Model("p1", "m")
			fp := modelresolver.CooldownFingerprintFor("a", prov, model)
			var before time.Duration
			if existing {
				h.current().Resolver.Cooldowns().Observe(fp, time.Hour)
				before, _ = h.current().Resolver.Cooldowns().Remaining(fp)
			}
			original, _ := scopedReasoningBodies(provider.OpChatCompletions, "p1/m")
			w := scopedReasoningPost(h, context.Background(), provider.OpChatCompletions, original)
			if w.Code != 200 || len(probe.snapshot()) != 2 {
				t.Fatal("direct recovery consulted alias cooldown exclusions")
			}
			after, cooling := h.current().Resolver.Cooldowns().Remaining(fp)
			if cooling != existing || existing && (after > before || after < before-time.Second) {
				t.Fatalf("direct inference/recovery populated or extended cooldowns: before=%s after=%s cooling=%t", before, after, cooling)
			}
		})
	}
}

func TestScopedEncryptedReasoningAliasRecoveryRetainsSelectedLeaseDespiteCooldownAndHealth(t *testing.T) {
	rt := scopedReasoningRuntime("retry", "", nil, nil)
	aliases := rt.Catalog.Aliases()
	aliases[0].RetryStatusCodes = []int{400, 503}
	rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
	var health *providerhealth.Tracker
	probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
		result := scopedReasoningResult(200)
		if call == 0 {
			result = scopedReasoningResult(400)
			result.HasRetryDelay, result.RetryDelay = true, time.Hour
			result.OnClose = func() { health.MarkFailure("p1") }
		}
		return result, nil
	}}
	h, _, clients := scopedReasoningHandler(t, rt, probe)
	deps := h.current()
	health = providerhealth.New(deps.Metrics, config.ProviderHealth{})
	defer health.Close()
	health.SetProviders(rt.Catalog)
	deps.Health = health
	h.UpdateDependencies(deps)
	p2, m2, _ := rt.Catalog.Model("p2", "m")
	deps.Resolver.Cooldowns().Observe(modelresolver.CooldownFingerprintFor("a", p2, m2), time.Hour)
	ctx := context.Background()
	original, stripped := scopedReasoningBodies(provider.OpChatCompletions, "alias/a")
	w := scopedReasoningPost(h, ctx, provider.OpChatCompletions, original)
	attempts := probe.snapshot()
	if w.Code != 200 || len(attempts) != 2 {
		t.Fatal("same-target recovery reselected around admitted cooldown/health state")
	}
	for i, attempt := range attempts {
		assertScopedReasoningAttempt(t, attempt, rt, clients, ctx, provider.OpChatCompletions, "alias/a", "p1")
		assertScopedReasoningBody(t, attempt.Request.Body, original, stripped, i == 1)
	}
	w = scopedReasoningPost(h, ctx, provider.OpChatCompletions, original)
	if w.Code != 429 || len(probe.snapshot()) != 2 || !strings.Contains(w.Body.String(), "upstream_rate_limited") {
		t.Fatal("future admissions ignored the newly recorded all-cooling state")
	}
	p1, m1, _ := rt.Catalog.Model("p1", "m")
	if _, cooling := deps.Resolver.Cooldowns().Remaining(modelresolver.CooldownFingerprintFor("a", p1, m1)); !cooling {
		t.Fatal("recognized inference mismatch lost normal retry advice")
	}
}

func TestScopedEncryptedReasoningNoRecoveryPreservesStreamingResult(t *testing.T) {
	for _, target := range []string{"p1/m", "alias/a"} {
		t.Run(target, func(t *testing.T) {
			rt := scopedReasoningRuntime("retry", "", nil, nil)
			aliases := rt.Catalog.Aliases()
			aliases[0].RetryStatusCodes = []int{400}
			rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
			probe := &scopedReasoningProbe{do: func(int, context.Context, provider.Request) (*provider.Result, error) {
				result := scopedReasoningResult(400)
				result.Streaming = true
				result.Stream = provider.NewStreamCompletion()
				result.StreamBody = io.NopCloser(strings.NewReader(scopedReasoningMismatch))
				return result, nil
			}}
			h, _, _ := scopedReasoningHandler(t, rt, probe)
			body, _ := scopedReasoningBodies(provider.OpChatCompletions, target)
			w := scopedReasoningPost(h, context.Background(), provider.OpChatCompletions, body)
			attempts := probe.snapshot()
			if w.Code != 400 || w.Body.String() != scopedReasoningMismatch || len(attempts) != 1 ||
				attempts[0].Closes.Load() != 1 || attempts[0].BodyCloses.Load() != 1 ||
				!reflect.DeepEqual(w.Header().Values("X-Upstream-Marker"), []string{"original", "second"}) {
				t.Fatal("established streaming error was recovered/replayed or finalized incorrectly")
			}
		})
	}
}

func TestScopedEncryptedReasoningRecoveryKeepsLeastConnectionsLeaseUntilStreamClose(t *testing.T) {
	rt := scopedReasoningRuntime("retry", "", nil, nil)
	aliases := rt.Catalog.Aliases()
	aliases[0].Algorithm = config.AlgorithmLeastConnections
	rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
	prefixWritten, gate := make(chan struct{}), make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	defer release()
	const prefix, tail = "data: admitted-prefix\n\n", "data: [DONE]\n\n"
	probe := &scopedReasoningProbe{do: func(call int, _ context.Context, request provider.Request) (*provider.Result, error) {
		switch call {
		case 0:
			return scopedReasoningResult(400), nil
		case 1:
			if request.APIKey != "credential-p1" {
				t.Error("reasoning recovery reselected a least-connections target")
			}
			result := scopedReasoningResult(200)
			result.Header.Set("Content-Type", "text/event-stream")
			result.Streaming, result.StreamBody, result.Stream = true, reader, provider.NewStreamCompletion()
			result.Stream.SetUsage(provider.Usage{PromptTokens: 4, CompletionTokens: 6, TotalTokens: 10})
			go func() {
				_, _ = io.WriteString(writer, prefix)
				close(prefixWritten)
				<-gate
				_, _ = io.WriteString(writer, tail)
				_ = writer.Close()
			}()
			return result, nil
		case 2:
			if request.APIKey != "credential-p2" {
				t.Error("discarding the mismatch released the active stream's selected lease")
			}
		case 3:
			if request.APIKey != "credential-p1" {
				t.Error("stream finalization leaked the least-connections lease")
			}
		default:
			t.Errorf("unexpected inference attempt %d", call+1)
		}
		return scopedReasoningResult(200), nil
	}}
	h, recorder, _ := scopedReasoningHandler(t, rt, probe)
	body, _ := scopedReasoningBodies(provider.OpChatCompletions, "alias/a")
	streamBody := bytes.Replace(body, []byte(`{ "model"`), []byte(`{ "stream":true,"model"`), 1)
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		finished <- scopedReasoningPost(h, context.Background(), provider.OpChatCompletions, streamBody)
	}()
	select {
	case <-prefixWritten:
	case <-time.After(3 * time.Second):
		t.Fatal("recovery stream did not start")
	}
	attempts := probe.snapshot()
	if len(attempts) != 2 || attempts[0].Closes.Load() != 1 || attempts[1].Closes.Load() != 0 {
		t.Fatal("mismatch/active stream finalization crossed ownership boundaries")
	}
	metrics := collectMetrics(t, h)
	if metricValue(t, metrics, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`) != 1 {
		t.Fatal("recovery did not retain one selected alias lease")
	}
	if w := scopedReasoningPost(h, context.Background(), provider.OpChatCompletions, body); w.Code != 200 {
		t.Fatal("concurrent request could not use the unleased target")
	}
	release()
	select {
	case w := <-finished:
		if w.Code != 200 || w.Body.String() != prefix+tail {
			t.Fatalf("recovery stream changed delivery: %d %q", w.Code, w.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("recovery stream did not finish")
	}
	attempts = probe.snapshot()
	if attempts[1].Closes.Load() != 1 || attempts[1].BodyCloses.Load() != 1 || len(recorder.Events()) != 2 {
		t.Fatal("recovery stream duplicated finalizers, body close, or inbound accounting")
	}
	metrics = collectMetrics(t, h)
	route02GaugeZero(t, metrics, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`)
	if w := scopedReasoningPost(h, context.Background(), provider.OpChatCompletions, body); w.Code != 200 || len(probe.snapshot()) != 4 {
		t.Fatal("completed recovery stream did not release its selected target")
	}
}
