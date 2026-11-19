//go:build integration && linux

package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestBinaryModelPassthroughSIGHUPUpdateAndRollback(t *testing.T) {
	const (
		token       = "fixture-client"
		viewerToken = "fixture-viewer"
		beforeMatch = "^vendor/old-[a-z0-9-]+$"
		keptMatch   = "^vendor/old-(known|pinned)$"
		nextMatch   = "^vendor/new-[a-z0-9-]+$"
		known       = "vendor/old-known"
		late        = "vendor/old-late"
		updated     = "vendor/new-chat"
		pinned      = "router/vendor/old-pinned"
		alias       = "alias/pinned"
		unavailable = `{"error":{"type":"upstream_unknown_model","message":"ID not yet offered by fixture"}}`
	)
	reply := func(model, generation string) string {
		return fmt.Sprintf(`{"model":%q,"fixture":%q,"choices":[]}`, model, generation)
	}
	event := func(model, text string) string {
		return fmt.Sprintf("data: {\"model\":%q,\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", model, text)
	}
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	var lateAvailable atomic.Bool
	upstream := func(generation string) (*httptest.Server, *atomic.Int32) {
		var calls atomic.Int32
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			var input struct {
				Model  string `json:"model"`
				Stream bool   `json:"stream"`
			}
			if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" ||
				r.Header.Get("Authorization") != "Bearer fixture-"+generation ||
				json.NewDecoder(r.Body).Decode(&input) != nil {
				t.Errorf("unexpected %s upstream request: %s %s", generation, r.Method, r.URL.Path)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if input.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, event(input.Model, generation+"-start"))
				w.(http.Flusher).Flush()
				select {
				case <-gate:
					_, _ = io.WriteString(w, event(input.Model, generation+"-finish")+"data: [DONE]\n\n")
				case <-r.Context().Done():
				}
				return
			}
			w.Header().Set("Content-Type", "application/json")
			if input.Model == late && !lateAvailable.Load() {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, unavailable)
				return
			}
			_, _ = io.WriteString(w, reply(input.Model, generation))
		}))
		t.Cleanup(s.Close)
		return s, &calls
	}
	old, oldCalls := upstream("before")
	active, activeCalls := upstream("after")
	rejected := newUpstreamStub(t)
	addr := freeAddr(t)
	source := func(base, generation, before, after, grant string) string {
		return fmt.Sprintf(`listener "http" "public" { address = %q }
auth "main" {
  mode = "bearer_static"
  client "app" {
    token = %q
    allowed_models = [%q, %q]
    allowed_model_passthroughs = [%q]
  }
  client "catalog-viewer" {
    token = %q
    allowed_model_passthroughs = [%q]
  }
}
provider "openai-compatible" "router" {
  base_url = %q
  api_key = %q
  model "vendor/old-pinned" {
    upstream_name = "pinned-chat"
    capabilities = ["chat"]
  }
  model_passthrough "before" {
    match = %q
    capabilities = ["chat"]
  }
  model_passthrough "after" {
    match = %q
    capabilities = ["chat"]
  }
}
alias "pinned" {
  algorithm = "round_robin"
  providers = ["router"]
  model = "vendor/old-pinned"
}
`, addr, token, pinned, alias, "router/"+grant, viewerToken, "router/"+grant, base+"/v1", "fixture-"+generation, before, after)
	}
	path := writeConfig(t, source(old.URL, "before", beforeMatch, "^vendor/(pending|old-overlap)$", "before"))
	srv := startBinaryServer(t, path, addr)
	requestBody := func(model string, stream bool) string {
		return fmt.Sprintf(`{"model":%q,"messages":[],"stream":%t}`, model, stream)
	}
	expectChat := func(model string, wantStatus int, wantBody string) {
		t.Helper()
		status, body := postChat(t, srv, token, requestBody(model, false))
		if status != wantStatus || body != wantBody {
			t.Fatalf("%s = %d %q, want %d %q", model, status, body, wantStatus, wantBody)
		}
	}
	expectDenied := func(model string) {
		t.Helper()
		status, body := postChat(t, srv, token, requestBody(model, false))
		if status != http.StatusForbidden || !strings.Contains(body, `"type":"forbidden"`) {
			t.Fatalf("%s = %d %q, want literal allowlist miss", model, status, body)
		}
	}
	expectCatalog := func() {
		t.Helper()
		var list struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		status, body := httpGet(t, srv, "/v1/models", viewerToken)
		if status != http.StatusOK || json.Unmarshal([]byte(body), &list) != nil {
			t.Fatalf("model list = %d %s", status, body)
		}
		var ids []string
		for _, m := range list.Data {
			ids = append(ids, m.ID)
		}
		slices.Sort(ids)
		if !slices.Equal(ids, []string{alias, pinned}) {
			t.Fatalf("list exposed more than the declared model and alias: %v", ids)
		}
	}
	expectCalls := func(wantOld, wantActive int32) {
		t.Helper()
		if before, after, invalid := oldCalls.Load(), activeCalls.Load(), len(rejected.Calls()); before != wantOld || after != wantActive || invalid != 0 {
			t.Fatalf("upstream requests before/after/rejected = %d/%d/%d, want %d/%d/0", before, after, invalid, wantOld, wantActive)
		}
	}

	expectChat(pinned, http.StatusOK, reply("pinned-chat", "before"))
	expectChat(alias, http.StatusOK, reply("pinned-chat", "before"))
	expectChat("router/"+known, http.StatusOK, reply(known, "before"))
	expectChat("router/"+late, http.StatusNotFound, unavailable)
	lateAvailable.Store(true)
	expectChat("router/"+late, http.StatusOK, reply(late, "before"))
	expectDenied("router/" + updated)
	expectDenied("router/vendor/old-overlap")
	expectCatalog()
	expectCalls(5, 0)

	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/v1/chat/completions", strings.NewReader(requestBody("router/"+known, true)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	streamClient := &http.Client{Transport: srv.client.Transport}
	resp, err := streamClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("initial SSE = %d %v", resp.StatusCode, resp.Header)
	}
	wantStart := event(known, "before-start")
	start := make([]byte, len(wantStart))
	if _, err := io.ReadFull(resp.Body, start); err != nil || string(start) != wantStart {
		t.Fatalf("initial SSE = %q, err %v", start, err)
	}
	expectCalls(6, 0)

	writeConfigPath(t, path, source(active.URL, "after", keptMatch, nextMatch, "after"))
	reloadBinaryTLS(t, srv, true)
	expectDenied("router/" + known)
	expectDenied("router/vendor/pending")
	expectChat("router/"+updated, http.StatusOK, reply(updated, "after"))
	expectChat(pinned, http.StatusOK, reply("pinned-chat", "after"))
	expectChat(alias, http.StatusOK, reply("pinned-chat", "after"))
	expectCalls(6, 3)

	writeConfigPath(t, path, source(rejected.URL(), "rejected", "*", "^vendor/rejected-chat$", "before"))
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	srv.WaitLogContains(t, "invalid match regexp")
	expectDenied("router/" + known)
	expectDenied("router/vendor/rejected-chat")
	expectChat("router/"+updated, http.StatusOK, reply(updated, "after"))
	expectCatalog()
	expectCalls(6, 4)

	release()
	tail, err := io.ReadAll(resp.Body)
	if err != nil || string(tail) != event(known, "before-finish")+"data: [DONE]\n\n" {
		t.Fatalf("old SSE did not finish through both reloads: %q, err %v", tail, err)
	}
	expectCalls(6, 4)
}
