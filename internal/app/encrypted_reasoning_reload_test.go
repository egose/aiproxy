package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/httpapi"
)

const appReasoningMismatch = `{"error":{"message":"reasoning encrypted_content was not issued to this caller"}}`

type appReasoningConfig struct {
	root, p1, m1, p2, m2, alias, generation string
}

func appReasoningBlock(mode string) string {
	switch mode {
	case "":
		return ""
	case "strip":
		return `encrypted_reasoning { passthrough = false }`
	case "retry":
		return `encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }`
	case "reset":
		return `encrypted_reasoning {}`
	case "invalid":
		return `encrypted_reasoning { on_caller_mismatch = "invalid" }`
	default:
		panic("unknown reasoning app fixture: " + mode)
	}
}

func appReasoningContents(upstream string, policy appReasoningConfig) string {
	return fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
logging { access_log = false }
%s
provider "openai-compatible" "p1" {
  base_url = %q
  api_key = %q
  %s
  model "m" {
    upstream_name = %q
    %s
  }
}
provider "openai-compatible" "p2" {
  base_url = %q
  api_key = %q
  %s
  model "m" {
    upstream_name = %q
    %s
  }
}
alias "a" {
  algorithm = "round_robin"
  %s
  target {
    provider = "p1"
    model = "m"
  }
  target {
    provider = "p2"
    model = "m"
  }
}
`, appReasoningBlock(policy.root), upstream, "credential-"+policy.generation+"-p1", appReasoningBlock(policy.p1),
		"upstream-"+policy.generation+"-p1", appReasoningBlock(policy.m1), upstream, "credential-"+policy.generation+"-p2",
		appReasoningBlock(policy.p2), "upstream-"+policy.generation+"-p2", appReasoningBlock(policy.m2), appReasoningBlock(policy.alias))
}

func appReasoningBodies(operation, model string) ([]byte, []byte) {
	if operation == "responses" {
		return []byte(`{ "model":"` + model + `","input":[{"type":"reasoning","encrypted_content":"blob","summary":[]},{"type":"message","role":"user","content":"hi"}]}`),
			[]byte(`{"model":"` + model + `","input":[{"type":"message","role":"user","content":"hi"}]}`)
	}
	return []byte(`{ "model":"` + model + `","messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"blob"},{"type":"text","text":"hi"}]}]}`),
		[]byte(`{"model":"` + model + `","messages":[{"role":"assistant","content":[{"type":"text","text":"hi"}]}]}`)
}

type appReasoningRequest struct {
	body []byte
	key  string
	path string
}

type appReasoningRequests struct {
	mu      sync.Mutex
	entries []appReasoningRequest
}

func (r *appReasoningRequests) record(t *testing.T, request *http.Request) appReasoningRequest {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	if err != nil {
		t.Error(err)
	}
	entry := appReasoningRequest{body: body, key: request.Header.Get("Authorization"), path: request.URL.Path}
	r.mu.Lock()
	r.entries = append(r.entries, entry)
	r.mu.Unlock()
	return entry
}

func (r *appReasoningRequests) snapshot() []appReasoningRequest {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]appReasoningRequest(nil), r.entries...)
}

func appReasoningBuild(t *testing.T, path string) *App {
	t.Helper()
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, Version: "reasoning-test", LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	return a
}

func appReasoningPost(h http.Handler, ctx context.Context, operation string, body []byte) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/v1/"+operation, bytes.NewReader(body)).WithContext(ctx)
	request.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, request)
	return w
}

func assertAppReasoningRequest(t *testing.T, request appReasoningRequest, operation, target, generation string, expected []byte) {
	t.Helper()
	upstreamModel := "upstream-" + generation + "-" + target
	if request.key != "Bearer credential-"+generation+"-"+target || request.path != "/v1/"+operation {
		t.Fatalf("request crossed target/config generation: %+v", request)
	}
	var actual, want map[string]any
	if err := json.Unmarshal(request.body, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(expected, &want); err != nil {
		t.Fatal(err)
	}
	want["model"] = upstreamModel
	if !reflect.DeepEqual(actual, want) {
		t.Fatalf("request body/model crossed policy generation:\ngot  %s\nwant %#v", request.body, want)
	}
}

func TestEncryptedReasoningStartupReloadAndInvalidRollback(t *testing.T) {
	for _, operation := range []string{"chat/completions", "responses"} {
		for _, model := range []string{"p1/m", "alias/a"} {
			t.Run(operation+"/"+model, func(t *testing.T) {
				observations := &appReasoningRequests{}
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					entry := observations.record(t, request)
					w.Header().Set("Content-Type", "application/json")
					w.Header().Set("X-Upstream-Marker", "original")
					if bytes.Contains(entry.body, []byte(`"encrypted_content"`)) {
						w.WriteHeader(400)
						_, _ = io.WriteString(w, appReasoningMismatch)
						return
					}
					_, _ = io.WriteString(w, `{"choices":[],"output":[]}`)
				}))
				t.Cleanup(upstream.Close)
				states := []struct {
					name   string
					config appReasoningConfig
					status int
					bodies []bool
				}{
					{"startup-root-strip", appReasoningConfig{root: "strip", generation: "old"}, 200, []bool{true}},
					{"provider-reset", appReasoningConfig{root: "strip", p1: "reset", p2: "reset", generation: "next"}, 400, []bool{false}},
					{"model-recovery", appReasoningConfig{root: "strip", p1: "reset", p2: "reset", m1: "retry", m2: "retry", generation: "model"}, 200, []bool{false, true}},
					{"alias-strip-direct-model-retry", appReasoningConfig{root: "retry", m1: "retry", m2: "retry", alias: "strip", generation: "alias"}, 200, []bool{false, true}},
					{"root-recovery", appReasoningConfig{root: "retry", generation: "root"}, 200, []bool{false, true}},
					{"removed-root", appReasoningConfig{generation: "removed"}, 400, []bool{false}},
					{"alias-reset-over-root-strip", appReasoningConfig{root: "strip", alias: "reset", generation: "reset"}, 200, []bool{true}},
				}
				path := writeConfigFile(t, appReasoningContents(upstream.URL, states[0].config))
				a := appReasoningBuild(t, path)
				body, stripped := appReasoningBodies(operation, model)
				for i, state := range states {
					if i > 0 {
						rewriteConfigFile(t, path, appReasoningContents(upstream.URL, state.config))
						if err := a.Reload(); err != nil {
							t.Fatal(err)
						}
					}
					before := len(observations.snapshot())
					w := appReasoningPost(a.Server.Handler, context.Background(), operation, body)
					status, expectedBodies := state.status, state.bodies
					if model == "alias/a" && state.config.alias == "strip" {
						expectedBodies = []bool{true}
					} else if model == "alias/a" && state.config.alias == "reset" {
						status, expectedBodies = 400, []bool{false}
					}
					requests := observations.snapshot()[before:]
					if w.Code != status || len(requests) != len(expectedBodies) {
						t.Fatalf("%s: status=%d calls=%d want=%d/%d body=%s", state.name, w.Code, len(requests), status, len(expectedBodies), w.Body.String())
					}
					target := "p1"
					if strings.HasSuffix(requests[0].key, "-p2") {
						target = "p2"
					}
					for i, request := range requests {
						expected := body
						if expectedBodies[i] {
							expected = stripped
						}
						assertAppReasoningRequest(t, request, operation, target, state.config.generation, expected)
					}
					if status == 400 && (w.Body.String() != appReasoningMismatch || w.Header().Get("X-Upstream-Marker") != "original") {
						t.Fatal("non-recovered upstream 400 bytes/headers changed")
					}
				}
				oldConfig := a.Config
				active := a.handler.SnapshotDependencies()
				p1, _ := active.Catalog.Provider("p1")
				oldClient := active.ClientForProvider(p1)
				rewriteConfigFile(t, path, appReasoningContents(upstream.URL, appReasoningConfig{root: "invalid", generation: "invalid"}))
				err := a.Reload()
				current := a.handler.SnapshotDependencies()
				if err == nil || !strings.Contains(err.Error(), "root") || a.Config != oldConfig ||
					current.Resolver != active.Resolver || current.RuntimeResources != active.RuntimeResources ||
					current.ClientForProvider(p1) != oldClient {
					t.Fatalf("invalid policy changed the admitted runtime/resources: %v", err)
				}
				resolved, err := current.Resolver.Resolve("p1/m")
				if err != nil {
					t.Fatal(err)
				}
				resolution := current.Resolver.EncryptedReasoning(resolved.Provider, resolved.Model, nil)
				if resolution.Source != config.EncryptedReasoningSourceRoot || resolution.Policy.Passthrough {
					t.Fatal("failed reload changed the effective root snapshot")
				}
				before := len(observations.snapshot())
				w := appReasoningPost(a.Server.Handler, context.Background(), operation, body)
				expectedStatus := 200
				if model == "alias/a" {
					expectedStatus = 400
				}
				if w.Code != expectedStatus || len(observations.snapshot()) != before+1 {
					t.Fatal("invalid reload did not preserve serving policy/alias reset")
				}
			})
		}
	}
}

func TestEncryptedReasoningInflightRecoveryAndFailoverRetainSnapshot(t *testing.T) {
	for _, operation := range []string{"chat/completions", "responses"} {
		for _, scenario := range []struct{ model, alias string }{{"p1/m", ""}, {"alias/a", ""}, {"alias/a", "retry"}} {
			model := scenario.model
			t.Run(operation+"/"+model+"/explicit="+scenario.alias, func(t *testing.T) {
				entered, gate := make(chan struct{}), make(chan struct{})
				enter := sync.OnceFunc(func() { close(entered) })
				release := sync.OnceFunc(func() { close(gate) })
				defer release()
				oldRequests, newRequests := &appReasoningRequests{}, &appReasoningRequests{}
				oldUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					entry := oldRequests.record(t, request)
					w.Header().Set("Content-Type", "application/json")
					if strings.HasSuffix(entry.key, "-p1") && bytes.Contains(entry.body, []byte(`"encrypted_content"`)) {
						enter()
						select {
						case <-gate:
							w.WriteHeader(400)
							_, _ = io.WriteString(w, appReasoningMismatch)
						case <-request.Context().Done():
						}
						return
					}
					if model == "alias/a" && strings.HasSuffix(entry.key, "-p1") {
						w.WriteHeader(503)
					}
					_, _ = io.WriteString(w, `{"choices":[],"output":[]}`)
				}))
				t.Cleanup(oldUpstream.Close)
				newUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					_ = newRequests.record(t, request)
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"choices":[],"output":[]}`)
				}))
				t.Cleanup(newUpstream.Close)
				path := writeConfigFile(t, appReasoningContents(oldUpstream.URL, appReasoningConfig{root: "retry", alias: scenario.alias, generation: "old"}))
				a := appReasoningBuild(t, path)
				old := a.handler.SnapshotDependencies()
				body, stripped := appReasoningBodies(operation, model)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				finished := make(chan *httptest.ResponseRecorder, 1)
				go func() { finished <- appReasoningPost(a.Server.Handler, ctx, operation, body) }()
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("old inference did not start")
				}
				rewriteConfigFile(t, path, appReasoningContents(newUpstream.URL, appReasoningConfig{root: "invalid", generation: "invalid"}))
				if err := a.Reload(); err == nil || a.handler.SnapshotDependencies().Resolver != old.Resolver {
					t.Fatal("invalid policy candidate replaced an admitted request's snapshot")
				}
				rewriteConfigFile(t, path, appReasoningContents(newUpstream.URL, appReasoningConfig{root: "strip", generation: "new"}))
				if err := a.Reload(); err != nil {
					t.Fatal(err)
				}
				current := a.handler.SnapshotDependencies()
				if current.Resolver == old.Resolver || current.RuntimeResources == old.RuntimeResources {
					t.Fatal("successful reload did not activate an independent snapshot")
				}
				wait, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
				err := old.RuntimeResources.Wait(wait)
				stop()
				if !errors.Is(err, httpapi.ErrResourcesIncomplete) {
					t.Fatalf("reload released a snapshot during admitted inference: %v", err)
				}
				newBody, newStripped := appReasoningBodies(operation, "p1/m")
				if w := appReasoningPost(a.Server.Handler, ctx, operation, newBody); w.Code != 200 {
					t.Fatal("new admissions did not use the new policy")
				}
				newObserved := newRequests.snapshot()
				if len(newObserved) != 1 {
					t.Fatal("new admission unexpectedly recovered or used the old endpoint")
				}
				assertAppReasoningRequest(t, newObserved[0], operation, "p1", "new", newStripped)
				release()
				select {
				case w := <-finished:
					if w.Code != 200 {
						t.Fatalf("old recovery/failover did not finish: %d %s", w.Code, w.Body.String())
					}
				case <-time.After(3 * time.Second):
					t.Fatal("old recovery/failover did not finish")
				}
				oldObserved := oldRequests.snapshot()
				expectedCalls := 2
				if model == "alias/a" {
					expectedCalls = 3
				}
				if len(oldObserved) != expectedCalls || len(newRequests.snapshot()) != 1 {
					t.Fatalf("recovery/failover crossed snapshot endpoints: old=%d new=%d", len(oldObserved), len(newRequests.snapshot()))
				}
				assertAppReasoningRequest(t, oldObserved[0], operation, "p1", "old", body)
				assertAppReasoningRequest(t, oldObserved[1], operation, "p1", "old", stripped)
				if model == "alias/a" {
					expected := body
					if scenario.alias != "" {
						expected = stripped
					}
					assertAppReasoningRequest(t, oldObserved[2], operation, "p2", "old", expected)
				}
				wait, stop = context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				if err := old.RuntimeResources.Wait(wait); err != nil {
					t.Fatalf("completed recovery/failover retained its old resource lease: %v", err)
				}
				resolved, _ := old.Resolver.Resolve("p1/m")
				oldPolicy := old.Resolver.EncryptedReasoning(resolved.Provider, resolved.Model, nil)
				if !oldPolicy.Policy.Passthrough || oldPolicy.Policy.OnCallerMismatch != config.EncryptedReasoningStripAndRetry {
					t.Fatal("successful reload mutated the old root policy snapshot")
				}
			})
		}
	}
}

func TestEncryptedReasoningEstablishedSSESurvivesPolicyReloads(t *testing.T) {
	for _, operation := range []string{"chat/completions", "responses"} {
		for _, model := range []string{"p1/m", "alias/a"} {
			t.Run(operation+"/"+model, func(t *testing.T) {
				gate := make(chan struct{})
				release := sync.OnceFunc(func() { close(gate) })
				defer release()
				oldRequests, newRequests := &appReasoningRequests{}, &appReasoningRequests{}
				var canceled atomic.Bool
				const prefix = "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n"
				const tail = "event: error\ndata: {\"error\":{\"message\":\"encrypted_content was not issued to this caller\"}}\n\n"
				oldUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					_ = oldRequests.record(t, request)
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, prefix)
					w.(http.Flusher).Flush()
					select {
					case <-gate:
						_, _ = io.WriteString(w, tail)
					case <-request.Context().Done():
						canceled.Store(true)
					}
				}))
				t.Cleanup(oldUpstream.Close)
				newUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
					entry := newRequests.record(t, request)
					w.Header().Set("Content-Type", "application/json")
					if bytes.Contains(entry.body, []byte(`"encrypted_content"`)) {
						w.WriteHeader(400)
						_, _ = io.WriteString(w, appReasoningMismatch)
						return
					}
					_, _ = io.WriteString(w, `{"choices":[],"output":[]}`)
				}))
				t.Cleanup(newUpstream.Close)
				path := writeConfigFile(t, appReasoningContents(oldUpstream.URL, appReasoningConfig{root: "retry", generation: "old"}))
				a := appReasoningBuild(t, path)
				old := a.handler.SnapshotDependencies()
				downstream := httptest.NewServer(a.Server.Handler)
				t.Cleanup(downstream.Close)
				body, _ := appReasoningBodies(operation, model)
				streamBody := bytes.Replace(body, []byte(`{ "model"`), []byte(`{ "stream":true,"model"`), 1)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				request, err := http.NewRequestWithContext(ctx, http.MethodPost, downstream.URL+"/v1/"+operation, bytes.NewReader(streamBody))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Content-Type", "application/json")
				response, err := downstream.Client().Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				received := make([]byte, len(prefix))
				if _, err := io.ReadFull(response.Body, received); err != nil || string(received) != prefix || response.StatusCode != 200 {
					t.Fatalf("stream did not establish its original prefix: status=%d body=%q err=%v", response.StatusCode, received, err)
				}
				for i, mode := range []string{"strip", "reset", "retry"} {
					generation := fmt.Sprintf("next%d", i)
					rewriteConfigFile(t, path, appReasoningContents(newUpstream.URL, appReasoningConfig{root: mode, generation: generation}))
					if err := a.Reload(); err != nil {
						t.Fatal(err)
					}
					current := a.handler.SnapshotDependencies()
					rewriteConfigFile(t, path, appReasoningContents(newUpstream.URL, appReasoningConfig{root: "invalid", generation: "invalid"}))
					if err := a.Reload(); err == nil || a.handler.SnapshotDependencies().Resolver != current.Resolver {
						t.Fatal("invalid stream-era policy reload changed the active snapshot")
					}
					newBody, _ := appReasoningBodies(operation, "p1/m")
					before := len(newRequests.snapshot())
					w := appReasoningPost(a.Server.Handler, ctx, operation, newBody)
					expectedStatus, expectedCalls := 200, 1
					if mode == "reset" {
						expectedStatus = 400
					} else if mode == "retry" {
						expectedCalls = 2
					}
					if w.Code != expectedStatus || len(newRequests.snapshot()) != before+expectedCalls || canceled.Load() {
						t.Fatal("policy reload canceled admitted SSE or did not affect new admissions")
					}
					wait, stop := context.WithTimeout(context.Background(), 10*time.Millisecond)
					err := old.RuntimeResources.Wait(wait)
					stop()
					if !errors.Is(err, httpapi.ErrResourcesIncomplete) {
						t.Fatalf("reload released the established stream's runtime: %v", err)
					}
				}
				release()
				data, err := io.ReadAll(response.Body)
				if err != nil || string(data) != tail || canceled.Load() {
					t.Fatalf("policy reload/recovery changed established SSE bytes: body=%q err=%v canceled=%t", data, err, canceled.Load())
				}
				_ = response.Body.Close()
				wait, stop := context.WithTimeout(context.Background(), 3*time.Second)
				defer stop()
				if err := old.RuntimeResources.Wait(wait); err != nil {
					t.Fatal(err)
				}
				oldObserved := oldRequests.snapshot()
				if len(oldObserved) != 1 || len(newRequests.snapshot()) != 4 {
					t.Fatal("established error SSE authorized fresh inference or crossed endpoints")
				}
				assertAppReasoningRequest(t, oldObserved[0], operation, "p1", "old", streamBody)
			})
		}
	}
}
