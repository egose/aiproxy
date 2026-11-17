package httpapi

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
)

type deferredDeliveryProbe struct {
	provider.Adapter
	calls      atomic.Int32
	finalizers atomic.Int32
	closes     atomic.Int32
	result     atomic.Pointer[provider.Result]
}

func (p *deferredDeliveryProbe) Do(ctx context.Context, request provider.Request) (*provider.Result, error) {
	p.calls.Add(1)
	result, err := p.Adapter.Do(ctx, request)
	if result != nil {
		previous := result.OnClose
		result.OnClose = func() {
			if previous != nil {
				previous()
			}
			p.finalizers.Add(1)
		}
		if result.StreamBody != nil {
			result.StreamBody = &deferredDeliveryBody{ReadCloser: result.StreamBody, closes: &p.closes}
		}
		p.result.Store(result)
	}
	return result, err
}

type deferredDeliveryBody struct {
	io.ReadCloser
	closes *atomic.Int32
}

func (b *deferredDeliveryBody) Close() error {
	b.closes.Add(1)
	return b.ReadCloser.Close()
}

func deferredDeliveryResources(t *testing.T, h *Handler, probe *deferredDeliveryProbe, recorder *accounting.MemoryRecorder, payload *deferredPayloadRecorder) (*RuntimeResources, *atomic.Int32) {
	t.Helper()
	var closes atomic.Int32
	owner := NewResourceOwner(func() error {
		closes.Add(1)
		if probe.finalizers.Load() != 1 || probe.closes.Load() != 1 || len(recorder.Events()) != 1 || len(payload.snapshot()) != 1 {
			t.Error("runtime released before body close, finalizers, accounting, and payload completion")
		}
		return nil
	}, nil)
	resources, err := NewRuntimeResources(owner)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(resources.Retire)
	deps := h.current()
	deps.RuntimeResources, deps.PayloadLog = resources, payload
	h.UpdateDependencies(deps)
	return resources, &closes
}

func retireDeferredDeliveryResources(h *Handler, resources *RuntimeResources) {
	deps := h.current()
	deps.RuntimeResources = nil
	h.UpdateDependencies(deps)
	resources.Retire()
}

func TestDeferredCallerCommittedInferenceFailureDelivery(t *testing.T) {
	for _, mode := range []string{"deferred-transport", "deferred-native-error", "ordinary-transport"} {
		for _, target := range []string{"direct", "alias"} {
			t.Run(mode+"/"+target, func(t *testing.T) {
				const prefix = "data: {\"choices\":[{\"delta\":{\"content\":\"original-prefix\"}}],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":2,\"total_tokens\":6}}\n\n"
				optedIn := mode != "ordinary-transport"
				suffix := ""
				if mode == "deferred-native-error" {
					suffix = "data: {\"error\":{\"type\":\"invalid_request_error\",\"message\":\"encrypted_content was not issued to this caller\"}}\n\n"
				}
				gate := make(chan struct{})
				release := sync.OnceFunc(func() { close(gate) })
				var upstreamCalls, backupCalls, deletes atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					upstreamCalls.Add(1)
					input, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(input), `"encrypted_content":"opaque"`) {
						t.Error("committed request unexpectedly stripped encrypted reasoning")
					}
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, prefix)
					w.(http.Flusher).Flush()
					select {
					case <-gate:
					case <-r.Context().Done():
						return
					}
					if suffix == "" {
						panic(http.ErrAbortHandler)
					}
					_, _ = io.WriteString(w, suffix)
				}))
				defer upstream.Close()
				defer release()
				executorProbe := &deferredDeliveryProbe{Adapter: provider.New()}
				executor := newDeferredHTTPFixture(t, executorProbe)
				setDeferredUpstream(executor, upstream.URL)
				executorPayload := &deferredPayloadRecorder{max: 4096}
				var executorResources *RuntimeResources
				var executorReleases *atomic.Int32
				if optedIn {
					executorResources, executorReleases = deferredDeliveryResources(t, executor.h, executorProbe, executor.accounting, executorPayload)
				}
				peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodDelete {
						deletes.Add(1)
					}
					executor.h.ServeHTTP(w, r)
				}))
				defer peer.Close()
				backup := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { backupCalls.Add(1) }))
				defer backup.Close()
				rt := callerDispatchRuntime(peer.URL, backup.URL)
				rt.EncryptedReasoning = scopedReasoningPolicy("retry")
				if !optedIn {
					providers := rt.Catalog.Providers()
					providers[0].DeferredTransport, providers[0].BaseURL = nil, upstream.URL+"/v1"
					providers[0].Models[0].DeferredTransport = nil
					providers[0].ModelByName = nil
					rt.Catalog = config.NewCatalog(providers, nil, rt.Catalog.Aliases())
				}
				callerProbe := &deferredDeliveryProbe{Adapter: provider.New()}
				h, recorder, health := callerDispatchHandler(rt, callerProbe)
				defer health.Close()
				callerPayload := &deferredPayloadRecorder{max: 4096}
				callerResources, callerReleases := deferredDeliveryResources(t, h, callerProbe, recorder, callerPayload)
				server := httptest.NewServer(h)
				defer server.Close()
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				model := "p1/m"
				if target == "alias" {
					model = "alias/a"
				}
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/v1/chat/completions",
					strings.NewReader(`{"model":"`+model+`","stream":true,"messages":[{"role":"assistant","content":[{"type":"redacted_thinking","encrypted_content":"opaque"}]}]}`))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				response, err := server.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				data := make([]byte, len(prefix))
				if _, err := io.ReadFull(response.Body, data); err != nil || string(data) != prefix || response.StatusCode != 200 || response.ProtoMajor != 1 {
					t.Fatalf("failed to observe committed HTTP/1 prefix: status=%d data=%q err=%v", response.StatusCode, data, err)
				}
				retireDeferredDeliveryResources(h, callerResources)
				if callerReleases.Load() != 0 {
					t.Fatal("caller runtime released during committed stream")
				}
				if optedIn {
					retireDeferredDeliveryResources(executor.h, executorResources)
					if executorReleases.Load() != 0 {
						t.Fatal("executor runtime released during inference")
					}
				}
				release()
				tail, readErr := io.ReadAll(response.Body)
				if string(tail) != suffix {
					t.Fatalf("failed delivery changed or invented SSE bytes: got %q, want %q", tail, suffix)
				}
				if optedIn && !errors.Is(readErr, io.ErrUnexpectedEOF) {
					t.Errorf("known failed deferred inference ended as clean HTTP success: status=%d err=%v", response.StatusCode, readErr)
				} else if !optedIn && readErr != nil {
					t.Errorf("ordinary non-opted streaming semantics changed: %v", readErr)
				}
				if err := callerResources.Wait(ctx); err != nil {
					t.Fatal(err)
				}
				if upstreamCalls.Load() != 1 || backupCalls.Load() != 0 || callerProbe.calls.Load() != 1 || callerReleases.Load() != 1 ||
					health.IsHealthy("p1") || !health.IsHealthy("p2") {
					t.Fatal("committed inference retried, leaked a runtime, or lost definite provider failure classification")
				}
				result := callerProbe.result.Load()
				outcome := result.Stream.Wait()
				if outcome.Err == nil || outcome.DownstreamCanceled || provider.IsPeerControlOutcome(result, outcome.Err) ||
					outcome.Usage.TotalTokens != 6 || result.AbortOnStreamError != optedIn {
					t.Fatalf("close masked definite stream failure or known usage: %+v", outcome)
				}
				events := recorder.Events()
				entries := callerPayload.snapshot()
				if len(events) != 1 || events[0].TotalTokens != 6 || events[0].PromptTokens != 4 || events[0].CompletionTokens != 2 ||
					len(entries) != 1 || entries[0].Response.Body.AsString() != prefix+suffix || entries[0].Response.Body.Truncated {
					t.Fatalf("caller completion lost once-only usage or exact inference payload: events=%+v entries=%+v", events, entries)
				}
				metrics := collectMetrics(t, h)
				if metricValue(t, metrics, `aiproxy_upstream_requests_total{operation="chat_completions",outcome="transport_error",provider="p1"}`) != 1 ||
					strings.Contains(metrics, `aiproxy_alias_retries_total{alias="a"`) {
					t.Fatal("definite stream failure was counted incorrectly or retried")
				}
				if target == "alias" && metricValue(t, metrics, `aiproxy_alias_inflight_requests{alias="a",model="m",provider="p1"}`) != 0 {
					t.Fatal("committed stream leaked its alias lease")
				}
				assertNoCallerCooldown(t, h)
				if optedIn {
					if err := executorResources.Wait(ctx); err != nil {
						t.Fatal(err)
					}
					awaitDeferredHTTP(t, "failed inference deletion and finalization", func() bool {
						stats := executor.store.Stats()
						return stats.Jobs == 0 && stats.Running == 0 && stats.Admissions == 0 && stats.Controls == 0 && stats.Deletes == 0 &&
							stats.FinalizationCredits == 0 && stats.Finalizing == 0 && stats.InputBytes == 0 && stats.OutputBytes == 0 && stats.WorkBytes == 0
					})
					events, entries = executor.accounting.Events(), executorPayload.snapshot()
					if executorProbe.calls.Load() != 1 || executorReleases.Load() != 1 || deletes.Load() != 1 || executor.rate.calls.Load() != 1 ||
						len(events) != 1 || events[0].StatusCode != 502 || events[0].TotalTokens != 6 || executor.h.current().Health.IsHealthy("p") ||
						len(entries) != 1 || entries[0].Error != "upstream_error" || entries[0].Response.Body.AsString() != prefix+suffix ||
						result.Origin != provider.OriginInference || executor.store.Stats().FinalizationFailures != 0 {
						t.Fatalf("executor completion lost classification, usage, payload, or once-only cleanup: events=%+v entries=%+v stats=%+v", events, entries, executor.store.Stats())
					}
				} else if executorProbe.calls.Load() != 0 || deletes.Load() != 0 {
					t.Fatal("ordinary inference unexpectedly used deferred transport")
				}
			})
		}
	}
}
