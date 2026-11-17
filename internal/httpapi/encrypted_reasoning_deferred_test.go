package httpapi

import (
	"bytes"
	"context"
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

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
)

type scopedReasoningBudget struct {
	mu     sync.Mutex
	limit  int64
	used   int64
	closed bool
	calls  []int64
}

func (b *scopedReasoningBudget) Reserve(n int64) (func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, n)
	if b.closed {
		return nil, deferred.ErrBudgetClosed
	}
	if n < 0 || n > b.limit-b.used {
		return nil, deferred.ErrBudgetExceeded
	}
	b.used += n
	return sync.OnceFunc(func() {
		b.mu.Lock()
		b.used -= n
		b.mu.Unlock()
	}), nil
}

func TestScopedEncryptedReasoningDeferredRecoveryBudgets(t *testing.T) {
	for _, target := range []string{"p1/m", "alias/a"} {
		for _, tc := range []struct {
			name, mode string
			calls      int
			failure    error
			limitError bool
		}{
			{"success", "retry", 2, nil, false},
			{"prestrip-work", "strip", 0, deferred.ErrBudgetExceeded, false},
			{"prestrip-depth", "strip", 0, nil, true},
			{"prestrip-memory", "strip", 0, deferred.ErrBudgetExceeded, false},
			{"inspection-work", "retry", 1, deferred.ErrBudgetExceeded, false},
			{"rewrite-depth", "retry", 1, nil, true},
			{"rewrite-size", "retry", 1, nil, true},
			{"rewrite-memory", "retry", 1, deferred.ErrBudgetExceeded, false},
			{"closed-on-discard", "retry", 1, deferred.ErrBudgetClosed, false},
			{"origin-control", "retry", 1, nil, false},
			{"typed-control", "retry", 1, nil, false},
		} {
			t.Run(target+"/"+tc.name, func(t *testing.T) {
				work := &scopedReasoningBudget{limit: 16 << 20}
				memory := &scopedReasoningBudget{limit: 64 << 20}
				execution := &deferredExecution{
					limits: &provider.ExecutionLimits{
						Work: work, RequestBytes: 64 << 10, ResponseBytes: 128 << 10,
						JSONDepth: deferred.JSONDepth, JSONTokens: deferred.JSONTokens,
					},
					memory: &admissionMemory{budget: memory},
				}
				defer execution.memory.close()
				switch tc.name {
				case "prestrip-work":
					work.limit = 1
				case "inspection-work":
					work.limit = 4 << 20
				case "prestrip-depth", "rewrite-depth":
					execution.limits.JSONDepth = 1
				case "rewrite-size":
					execution.limits.RequestBytes = 1
				case "prestrip-memory", "rewrite-memory":
					memory.limit = 1
				case "origin-control", "typed-control":
					work.limit = 0
				}
				rt := scopedReasoningRuntime(tc.mode, "", nil, nil)
				probe := &scopedReasoningProbe{do: func(call int, _ context.Context, request provider.Request) (*provider.Result, error) {
					if request.ExecutionLimits != execution.limits || request.ObserveBody == nil {
						t.Error("recovery lost the frozen execution budget or source observer")
					}
					result := scopedReasoningResult(400)
					if call == 1 {
						result = scopedReasoningResult(200)
					}
					if tc.name == "closed-on-discard" {
						result.OnClose = func() {
							work.mu.Lock()
							work.closed = true
							work.mu.Unlock()
						}
					}
					if tc.name == "origin-control" {
						result.Origin = provider.OriginPeerControl
					} else if tc.name == "typed-control" {
						return result, &provider.DeferredControlError{Code: "deferred_request_lost", Acceptance: deferred.AcceptanceAccepted, Err: io.ErrUnexpectedEOF}
					}
					return result, nil
				}}
				h, _, _ := scopedReasoningHandler(t, rt, probe)
				deps := h.current()
				ctx := context.WithValue(context.Background(), deferredExecutionKey{}, execution)
				body, stripped := scopedReasoningBodies(provider.OpChatCompletions, target)
				inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(body)).WithContext(ctx)
				resolved, err := deps.Resolver.Resolve(target)
				if err != nil {
					t.Fatal(err)
				}
				var result *provider.Result
				if resolved.Kind == modelresolver.KindDirect {
					result, err = h.dispatchDirect(deps, ctx, provider.OpChatCompletions, resolved, inbound, body, deps.Logger)
				} else {
					result, err = h.dispatchAlias(deps, ctx, provider.OpChatCompletions, resolved, inbound, body, deps.Logger)
				}
				closeResult(result)
				if tc.failure != nil && !errors.Is(err, tc.failure) {
					t.Fatalf("error=%v want=%v", err, tc.failure)
				}
				var executionLimit provider.ErrExecutionLimit
				if tc.limitError && !errors.As(err, &executionLimit) {
					t.Fatalf("error=%v, want an execution shape/rewrite limit", err)
				}
				if tc.name == "typed-control" {
					var control *provider.DeferredControlError
					if !errors.As(err, &control) {
						t.Fatalf("typed peer control lost its classification: %v", err)
					}
				} else if tc.failure == nil && !tc.limitError && err != nil {
					t.Fatal(err)
				}
				attempts := probe.snapshot()
				if len(attempts) != tc.calls {
					t.Fatalf("budget/control failure authorized fresh inference: calls=%d want=%d error=%v", len(attempts), tc.calls, err)
				}
				for i, attempt := range attempts {
					if attempt.Closes.Load() != 1 || attempt.Context != ctx {
						t.Fatal("budget failure duplicated close or replaced the admitted execution context")
					}
					assertScopedReasoningBody(t, attempt.Request.Body, body, stripped, i == 1)
				}
				execution.memory.close()
				if work.used != 0 || memory.used != 0 {
					t.Fatalf("recovery leaked execution reservations: work=%d memory=%d", work.used, memory.used)
				}
				metrics := collectMetrics(t, h)
				route02GaugeZero(t, metrics, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`)
			})
		}
	}
}

func TestScopedEncryptedReasoningDeferredAliasPrestripReservations(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(fmt.Sprintf("explicit=%t", explicit), func(t *testing.T) {
			work, memory := &scopedReasoningBudget{limit: 16 << 20}, &scopedReasoningBudget{limit: 64 << 20}
			execution := &deferredExecution{
				limits: &provider.ExecutionLimits{
					Work: work, RequestBytes: 64 << 10, ResponseBytes: 128 << 10,
					JSONDepth: deferred.JSONDepth, JSONTokens: deferred.JSONTokens,
				},
				memory: &admissionMemory{budget: memory},
			}
			defer execution.memory.close()
			aliasPolicy := ""
			if explicit {
				aliasPolicy = "strip"
			}
			rt := scopedReasoningRuntime("strip", aliasPolicy, nil, nil)
			probe := &scopedReasoningProbe{do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
				status := 200
				if call == 0 {
					status = 503
				}
				return scopedReasoningResult(status), nil
			}}
			h, _, _ := scopedReasoningHandler(t, rt, probe)
			ctx := context.WithValue(context.Background(), deferredExecutionKey{}, execution)
			body, stripped := scopedReasoningBodies(provider.OpChatCompletions, "alias/a")
			w := scopedReasoningPost(h, ctx, provider.OpChatCompletions, body)
			attempts := probe.snapshot()
			expectedReservations := 2
			if explicit {
				expectedReservations = 1
			}
			if w.Code != 200 || len(attempts) != 2 || len(work.calls) != expectedReservations || len(memory.calls) != expectedReservations {
				t.Fatalf("prestrip propagated/reallocated incorrectly: status=%d attempts=%d work/memory reservations=%d/%d want=%d",
					w.Code, len(attempts), len(work.calls), len(memory.calls), expectedReservations)
			}
			for _, attempt := range attempts {
				assertScopedReasoningBody(t, attempt.Request.Body, body, stripped, true)
				if attempt.Closes.Load() != 1 {
					t.Fatal("prestrip failover duplicated result closure")
				}
			}
			execution.memory.close()
			if work.used != 0 || memory.used != 0 {
				t.Fatal("prestrip failover leaked execution reservations")
			}
		})
	}
}

func TestScopedEncryptedReasoningDeferredJobRecoverySnapshotAccountingAndReplay(t *testing.T) {
	for _, target := range []string{"p/m", "alias/a"} {
		t.Run(target, func(t *testing.T) {
			entered, gate := make(chan struct{}), make(chan struct{})
			release := sync.OnceFunc(func() { close(gate) })
			probe := &scopedReasoningProbe{t: t, do: func(call int, ctx context.Context, request provider.Request) (*provider.Result, error) {
				if request.ExecutionLimits == nil || request.ExecutionLimits.Work == nil || request.ObserveBody == nil ||
					request.APIKey != "model-secret" || request.UpstreamModel != "actual" {
					t.Error("deferred recovery replaced the admitted target or execution budget")
				}
				if call == 0 {
					close(entered)
					select {
					case <-gate:
						result := scopedReasoningResult(400)
						result.StreamBody = io.NopCloser(bytes.NewReader(nil))
						return result, nil
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				if call != 1 {
					t.Errorf("unexpected inference attempt %d", call+1)
				}
				result := scopedReasoningResult(200)
				result.StreamBody = io.NopCloser(bytes.NewReader(nil))
				return result, nil
			}}
			options := executorTestOptions()
			options.MaxWorkBytes, options.MaxMemoryBytes = 16<<20, 128<<20
			f := newDeferredHTTPFixture(t, probe, options)
			defer release()
			f.rt.EncryptedReasoning = scopedReasoningPolicy("retry")
			providers := f.rt.Catalog.Providers()
			backup := providers[0]
			backup.Name, backup.APIKey = "backup", "backup-secret"
			f.rt.Catalog = config.NewCatalog(append(providers, backup), nil, []config.Alias{{
				Name: "a", Algorithm: config.AlgorithmRoundRobin, RetryStatusCodes: []int{400, 503},
				Targets: []config.AliasTarget{{Provider: "p", Model: "m"}, {Provider: "backup", Model: "m"}},
			}})
			deps := f.h.current()
			deps.Catalog, deps.Resolver = f.rt.Catalog, modelresolver.New(f.rt)
			deps.MetricsToken = metricsTestToken
			f.h.UpdateDependencies(deps)
			client, quota := dynamicDeferredClient(), &deferredQuotaStore{}
			configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, time.Now)
			body, stripped := scopedReasoningBodies(provider.OpChatCompletions, target)
			request := deferredSubmitRequest(t, f.capability(t, target), body)
			receipt := f.submit(t, request)
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("deferred inference did not start")
			}
			oldResolver := f.h.current().Resolver
			f.rt.EncryptedReasoning = scopedReasoningPolicy("reset")
			deps = f.h.current()
			deps.Resolver = modelresolver.NewWithPrevious(f.rt, oldResolver)
			f.h.UpdateDependencies(deps)
			release()
			manifest := f.finish(t, receipt)
			attempts := probe.snapshot()
			if manifest.Outcome.Kind != deferred.OutcomeResponse || manifest.Response.StatusCode != 200 ||
				len(attempts) != 2 || f.rate.calls.Load() != 1 || quota.reads.Load() != 2 || quota.inserts.Load() != 1 ||
				len(f.accounting.Events()) != 1 || f.accounting.Events()[0].TotalTokens != 10 || f.store.Stats().WorkBytes != 0 {
				t.Fatalf("deferred recovery lost snapshot/budget/once-only completion: calls=%d outcome=%+v stats=%+v", len(attempts), manifest.Outcome, f.store.Stats())
			}
			for i, attempt := range attempts {
				assertScopedReasoningBody(t, attempt.Request.Body, body, stripped, i == 1)
				if attempt.Closes.Load() != 1 || attempt.BodyCloses.Load() != 1 || attempt.Context != attempts[0].Context ||
					attempt.Request.ExecutionLimits != attempts[0].Request.ExecutionLimits {
					t.Fatal("deferred recovery changed identity/context/budget or duplicated result closure")
				}
			}
			deadline, ok := attempts[0].Context.Deadline()
			if !ok || deadline.UnixMilli() != receipt.ExecutionDeadlineMS {
				t.Fatal("deferred recovery renewed the execution deadline")
			}
			for range 3 {
				replayed := f.submit(t, request)
				frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
				if !replayed.Replayed || replayed.ID != receipt.ID || !frame.Done || frame.Response == nil || frame.Response.StatusCode != 200 {
					t.Fatal("completed inference replay lost the original acceptance/result")
				}
			}
			if len(probe.snapshot()) != 2 || f.rate.calls.Load() != 1 || quota.inserts.Load() != 1 || len(f.accounting.Events()) != 1 {
				t.Fatal("peer replay/control paths authorized fresh reasoning inference or duplicate accounting")
			}
			w := f.call(http.MethodDelete, deferred.Prefix+"/jobs/"+receipt.ID, "peer-token", nil)
			if w.Code != http.StatusNoContent || len(probe.snapshot()) != 2 {
				t.Fatalf("peer deletion/finalization changed control behavior or inference count: status=%d calls=%d", w.Code, len(probe.snapshot()))
			}
			metrics := collectMetrics(t, f.h)
			route02GaugeZero(t, metrics, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p"}`)
		})
	}
}

func TestScopedEncryptedReasoningDeferredFinalizationErrorCannotAuthorizeInference(t *testing.T) {
	probe := &scopedReasoningProbe{t: t, do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
		if call != 0 {
			t.Error("finalization failure triggered fresh inference")
		}
		return scopedReasoningResult(200), nil
	}}
	f := newDeferredHTTPFixture(t, probe)
	f.rt.EncryptedReasoning = scopedReasoningPolicy("retry")
	deps := f.h.current()
	deps.Resolver = modelresolver.New(f.rt)
	f.h.UpdateDependencies(deps)
	client := dynamicDeferredClient()
	quota := &deferredQuotaStore{insert: func(context.Context, *store.SpendEntry) error {
		return fmt.Errorf("encrypted_content was not issued to this caller")
	}}
	configureDynamicDeferred(f, []auth.DynamicClient{client}, quota, time.Now)
	body, _ := scopedReasoningBodies(provider.OpChatCompletions, "p/m")
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), body)
	receipt := f.submit(t, request)
	manifest := f.finish(t, receipt)
	for range 2 {
		_ = f.read(t, receipt, 0, deferred.MaxReadBytes)
		_ = f.submit(t, request)
	}
	if manifest.Outcome.Kind != deferred.OutcomeResponse || manifest.Response.StatusCode != 200 ||
		len(probe.snapshot()) != 1 || f.rate.calls.Load() != 1 || quota.inserts.Load() != 1 ||
		len(f.accounting.Events()) != 1 || f.store.Stats().FinalizationFailures != 1 {
		t.Fatal("finalization error altered definite inference delivery or authorized duplicate inference/completion")
	}
}

func TestScopedEncryptedReasoningDeferredCallerRecoversOnlyDefiniteTerminalMismatch(t *testing.T) {
	for _, target := range []string{"p1/m", "alias/a"} {
		t.Run(target, func(t *testing.T) {
			executorProbe := &scopedReasoningProbe{t: t, do: func(call int, _ context.Context, _ provider.Request) (*provider.Result, error) {
				if call == 0 {
					return scopedReasoningResult(400), nil
				}
				if call != 1 {
					t.Errorf("private replay/control duplicated inference: call=%d", call+1)
				}
				return scopedReasoningResult(200), nil
			}}
			executor := newDeferredHTTPFixture(t, executorProbe)
			var mu sync.Mutex
			var wires [][]byte
			var submits, capabilities, deletes, ordinary, backupCalls atomic.Int32
			peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
				copyDeferredModelReply(w, response, interrupt)
			}))
			defer peer.Close()
			backup := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { backupCalls.Add(1) }))
			defer backup.Close()
			rt := callerDispatchRuntime(peer.URL, backup.URL)
			rt.EncryptedReasoning = scopedReasoningPolicy("retry")
			aliases := rt.Catalog.Aliases()
			aliases[0].EncryptedReasoning = nil
			rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, aliases)
			adapter := provider.New()
			callerProbe := &scopedReasoningProbe{t: t, do: func(_ int, ctx context.Context, request provider.Request) (*provider.Result, error) {
				return adapter.Do(ctx, request)
			}}
			h, recorder, health := callerDispatchHandler(rt, callerProbe)
			defer health.Close()
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			body, stripped := scopedReasoningBodies(provider.OpChatCompletions, target)
			w := scopedReasoningPost(h, ctx, provider.OpChatCompletions, body)
			callerAttempts, executorAttempts := callerProbe.snapshot(), executorProbe.snapshot()
			if w.Code != 200 || len(callerAttempts) != 2 || len(executorAttempts) != 2 ||
				submits.Load() != 3 || capabilities.Load() != 2 || ordinary.Load() != 0 || backupCalls.Load() != 0 ||
				len(recorder.Events()) != 1 || recorder.Events()[0].TotalTokens != 10 {
				t.Fatalf("terminal reasoning recovery confused inference and controls: status=%d caller=%d executor=%d submit/capabilities=%d/%d backup=%d",
					w.Code, len(callerAttempts), len(executorAttempts), submits.Load(), capabilities.Load(), backupCalls.Load())
			}
			awaitDeferredHTTP(t, "both recovered peer jobs finalized/deleted", func() bool {
				stats := executor.store.Stats()
				return stats.Jobs == 0 && stats.Running == 0 && stats.FinalizationCredits == 0 && deletes.Load() == 2
			})
			if executor.rate.calls.Load() != 2 || len(executor.accounting.Events()) != 2 {
				t.Fatal("private submit replay created a third inbound execution/accounting event")
			}
			for i, attempt := range callerAttempts {
				assertScopedReasoningBody(t, attempt.Request.Body, body, stripped, i == 1)
				if attempt.Context != ctx || attempt.Request.APIKey != "peer-token" ||
					attempt.Request.BaseURL != peer.URL+"/v1" || attempt.Request.UpstreamModel != "p/m" ||
					attempt.Request.DeferredTransport == nil || attempt.Closes.Load() != 1 ||
					attempt.Result.Origin != provider.OriginInference || executorAttempts[i].Closes.Load() != 1 {
					t.Fatal("recovery changed the admitted peer identity/context or lost once-only result ownership")
				}
			}
			mu.Lock()
			defer mu.Unlock()
			if len(wires) != 3 || !bytes.Equal(wires[0], wires[1]) {
				t.Fatal("lost acknowledgment refreshed the original inference attempt")
			}
			first, err := deferred.DecodeSubmit(wires[0])
			if err != nil {
				t.Fatal(err)
			}
			recovery, err := deferred.DecodeSubmit(wires[2])
			if err != nil {
				t.Fatal(err)
			}
			if first.AttemptKey == recovery.AttemptKey || !bytes.Contains(first.Body, []byte(`"encrypted_content"`)) ||
				bytes.Contains(recovery.Body, []byte(`"encrypted_content"`)) || !bytes.Contains(recovery.Body, []byte(`"hi"`)) {
				t.Fatal("same-target reasoning recovery was mistaken for control replay or lost its bounded rewrite")
			}
			metrics := collectMetrics(t, h)
			if strings.Contains(metrics, `aiproxy_alias_retries_total{alias="a"`) {
				t.Fatal("same-peer reasoning recovery was recorded as normal alias failover")
			}
		})
	}
}
