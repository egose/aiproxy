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

func TestBinaryOpenCodePassthroughProtocolReload(t *testing.T) {
	const (
		appToken    = "fixture-app"
		steadyToken = "fixture-steady"
		upstreamKey = "fixture-zen"
		prefix      = "/fixture/zen/v1"
		chatPath    = prefix + "/chat/completions"
		modelPath   = prefix + "/models/"
		shared      = "family-shared"
		late        = "family-late"
		added       = "family-new"
		control     = "control"
		pinned      = "zen/pinned"
		alias       = "alias/pinned"
		unavailable = `{"error":{"type":"upstream_unknown_model","message":"ID not yet offered by fixture"}}`
		geminiReply = `{"candidates":[{"content":{"role":"model","parts":[{"text":%q}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":6,"totalTokenCount":10}}`
	)
	reply := func(model string) string {
		return fmt.Sprintf(`{"model":%q,"fixture":"native-chat","choices":[]}`, model)
	}
	event := func(text string) string {
		return fmt.Sprintf("data: {\"model\":%q,\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", shared, text)
	}
	gate := make(chan struct{})
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	var lateAvailable atomic.Bool
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			Model    string          `json:"model"`
			Stream   bool            `json:"stream"`
			Messages json.RawMessage `json:"messages"`
			Contents json.RawMessage `json:"contents"`
		}
		if r.Method != http.MethodPost || r.URL.RawQuery != "" ||
			r.Header.Get("Authorization") != "Bearer "+upstreamKey || r.Header.Get("X-OpenCode-Session") == "" ||
			json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Errorf("unexpected upstream request: %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		switch r.URL.Path {
		case chatPath:
			if len(input.Messages) == 0 || len(input.Contents) != 0 || input.Model == "" {
				t.Errorf("expected Chat model and messages, got %+v", input)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if input.Stream && input.Model == shared {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, event("old-chat-start"))
				w.(http.Flusher).Flush()
				select {
				case <-gate:
					_, _ = io.WriteString(w, event("old-chat-finish")+"data: [DONE]\n\n")
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
			_, _ = io.WriteString(w, reply(input.Model))
		case modelPath + shared + ":generateContent", modelPath + added + ":generateContent":
			if input.Model != "" || len(input.Messages) != 0 || !strings.Contains(string(input.Contents), `"text":"fixture-input"`) {
				t.Errorf("expected translated Gemini contents, got %+v", input)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			id := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, modelPath), ":generateContent")
			_, _ = fmt.Fprintf(w, geminiReply, "gemini-wire-"+id)
		default:
			t.Errorf("unexpected upstream path: %s", r.URL)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(upstream.Close)
	rejected := newUpstreamStub(t)
	t.Cleanup(func() {
		if got := rejected.Calls(); len(got) != 0 {
			t.Errorf("rejected destination or non-loopback proxy got %d requests", len(got))
		}
	})
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy", "ALL_PROXY", "all_proxy"} {
		t.Setenv(key, rejected.URL())
	}
	for _, key := range []string{"NO_PROXY", "no_proxy"} {
		t.Setenv(key, "127.0.0.1,::1")
	}

	addr := freeAddr(t)
	source := func(base, pattern, protocol, appGrant string) string {
		return fmt.Sprintf(`listener "http" "public" { address = %q }
auth "main" {
  mode = "bearer_static"
  client "app" {
    token = %q
    allowed_models = [%q]
    allowed_model_passthroughs = [%q]
  }
  client "steady" {
    token = %q
    allowed_model_passthroughs = ["zen/moving", "zen/control"]
  }
}
provider "opencode-zen" "zen" {
  base_url = %q
  api_key = %q
  model "pinned" {
    protocol = "chat"
    upstream_name = "pinned-chat"
    capabilities = ["chat"]
  }
  model_passthrough "moving" {
    match = %q
    protocol = %q
    capabilities = ["chat"]
  }
  model_passthrough "control" {
    match = "^control$"
    protocol = "chat"
    capabilities = ["chat"]
  }
}
alias "pinned" {
  algorithm = "round_robin"
  providers = ["zen"]
  model = "pinned"
}
`, addr, appToken, alias, "zen/"+appGrant, steadyToken, base+prefix, upstreamKey, pattern, protocol)
	}
	path := writeConfig(t, source(upstream.URL, "^(family-shared|family-late|pinned)$", "chat", "control"))
	srv := startBinaryServer(t, path, addr)
	requestBody := func(model string, stream bool) string {
		return fmt.Sprintf(`{"model":%q,"messages":[{"role":"user","content":"fixture-input"}],"stream":%t}`, model, stream)
	}
	checkChat := func(token, model string, wantStatus int, wantCalls int32) string {
		t.Helper()
		before := calls.Load()
		status, body := postChat(t, srv, token, requestBody(model, false))
		if got := calls.Load() - before; status != wantStatus || got != wantCalls {
			t.Fatalf("%s for %s = %d %s (%d upstream calls), want status %d and %d calls", model, token, status, body, got, wantStatus, wantCalls)
		}
		return body
	}
	expectNative := func(token, model, wireModel string) {
		t.Helper()
		if body := checkChat(token, model, http.StatusOK, 1); body != reply(wireModel) {
			t.Fatalf("%s did not use the native Chat path with %q: %s", model, wireModel, body)
		}
	}
	expectGemini := func(token, suffix string) {
		t.Helper()
		public := "zen/" + suffix
		body := checkChat(token, public, http.StatusOK, 1)
		if !strings.Contains(body, `"model":"`+public+`"`) || !strings.Contains(body, `"content":"gemini-wire-`+suffix+`"`) {
			t.Fatalf("%s did not use the Gemini path: %s", public, body)
		}
	}
	expectDenied := func(token, model string) {
		t.Helper()
		status, kind := http.StatusForbidden, "forbidden"
		if token == steadyToken {
			status, kind = http.StatusNotFound, "model_not_found"
		}
		if body := checkChat(token, model, status, 0); !strings.Contains(body, `"type":"`+kind+`"`) {
			t.Fatalf("%s did not retain the literal miss: %s", model, body)
		}
	}
	inventories := make(map[string]string)
	expectLiteralPolicy := func() {
		t.Helper()
		expectNative(steadyToken, pinned, "pinned-chat")
		expectNative(appToken, alias, "pinned-chat")
		expectDenied(appToken, pinned)

		before := calls.Load()
		for _, tc := range []struct {
			token string
			ids   []string
		}{
			{steadyToken, []string{alias, pinned}},
			{appToken, []string{alias}},
		} {
			status, body := httpGet(t, srv, "/v1/models", tc.token)
			var list struct {
				Data []struct{ ID string } `json:"data"`
			}
			if status != http.StatusOK || json.Unmarshal([]byte(body), &list) != nil {
				t.Fatalf("model list for %s = %d %s", tc.token, status, body)
			}
			var ids []string
			for _, card := range list.Data {
				ids = append(ids, card.ID)
			}
			slices.Sort(ids)
			if previous := inventories[tc.token]; !slices.Equal(ids, tc.ids) || (previous != "" && previous != body) {
				t.Fatalf("literal/alias inventory for %s changed: %s", tc.token, body)
			}
			inventories[tc.token] = body
		}
		if calls.Load() != before {
			t.Fatal("model listing contacted upstream")
		}
	}

	expectLiteralPolicy()
	expectNative(appToken, "zen/"+control, control)
	expectDenied(appToken, "zen/"+shared)
	expectNative(steadyToken, "zen/"+shared, shared)
	expectDenied(steadyToken, "zen/"+added)
	if body := checkChat(steadyToken, "zen/"+late, http.StatusNotFound, 1); body != unavailable {
		t.Fatalf("expected an upstream miss before enabling the ID: %s", body)
	}
	lateAvailable.Store(true)
	expectNative(steadyToken, "zen/"+late, late)
	expectLiteralPolicy()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, srv.baseURL+"/v1/chat/completions", strings.NewReader(requestBody("zen/"+shared, true)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+steadyToken)
	streamClient := &http.Client{Transport: srv.client.Transport}
	beforeStream := calls.Load()
	resp, err := streamClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("initial SSE = %d %v", resp.StatusCode, resp.Header)
	}
	start := make([]byte, len(event("old-chat-start")))
	if _, err := io.ReadFull(resp.Body, start); err != nil || string(start) != event("old-chat-start") || calls.Load() != beforeStream+1 {
		t.Fatalf("initial SSE = %q, err %v", start, err)
	}

	writeConfigPath(t, path, source(upstream.URL, "^(family-shared|family-new|pinned)$", "gemini", "moving"))
	reloadBinaryTLS(t, srv, true)
	expectGemini(steadyToken, shared)
	expectDenied(steadyToken, "zen/"+late)
	expectGemini(appToken, shared)
	expectGemini(appToken, added)
	expectDenied(appToken, "zen/"+control)
	expectNative(steadyToken, "zen/"+control, control)
	expectLiteralPolicy()

	writeConfigPath(t, path, source(rejected.URL(), "^(family-shared|family-rejected|pinned)$", "bogus", "control"))
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	srv.WaitLogContains(t, "has invalid protocol")
	expectGemini(appToken, shared)
	expectGemini(steadyToken, added)
	expectDenied(steadyToken, "zen/family-rejected")
	expectDenied(appToken, "zen/"+control)
	expectNative(steadyToken, "zen/"+control, control)
	expectLiteralPolicy()

	status, body := httpGet(t, srv, "/v1/billing/usage", appToken)
	var billing struct {
		Data []struct {
			Model       string
			StatusCode  int
			Count       int
			TotalTokens int
			Cost        json.RawMessage `json:"estimated_cost_usd"`
		} `json:"data"`
	}
	if status != http.StatusOK || json.Unmarshal([]byte(body), &billing) != nil {
		t.Fatalf("rule billing = %d %s", status, body)
	}
	if strings.Contains(body, "zen/"+shared) || strings.Contains(body, "zen/"+added) {
		t.Fatalf("wire IDs leaked into grouped billing: %s", body)
	}
	var groups int
	for _, row := range billing.Data {
		if row.Model == "zen/@passthrough/moving" {
			groups++
			if row.StatusCode != http.StatusOK || row.Count != 3 || row.TotalTokens != 30 || row.Cost != nil {
				t.Fatalf("Gemini rule IDs did not share an unpriced usage row: %+v", row)
			}
		}
	}
	if groups != 1 {
		t.Fatalf("wanted one Gemini rule usage group: %s", body)
	}

	beforeRelease := calls.Load()
	release()
	tail, err := io.ReadAll(resp.Body)
	if err != nil || string(tail) != event("old-chat-finish")+"data: [DONE]\n\n" || calls.Load() != beforeRelease {
		t.Fatalf("old Chat SSE did not finish through both reloads: %q, err %v", tail, err)
	}
}
