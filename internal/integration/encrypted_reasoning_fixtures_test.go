//go:build integration && linux

package integration

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

const binaryReasoningMismatch = `{"error":{"message":"reasoning encrypted_content was not issued to this caller"}}`

const (
	binaryReasoningReset = "encrypted_reasoning {}"
	binaryReasoningStrip = "encrypted_reasoning { passthrough = false }"
	binaryReasoningRetry = `encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }`
)

type binaryReasoningOperation struct {
	name     string
	path     string
	intact   string
	stripped string
	success  string
}

var binaryReasoningOperations = []binaryReasoningOperation{
	{
		name: "chat", path: "/v1/chat/completions",
		intact:   `{"model":%q,"messages":[{"role":"assistant","content":[{"type":"reasoning","encrypted_content":"opaque-history"},{"type":"thinking","thinking":"unsigned-history"},{"type":"text","text":"ordinary text"}],"tool_calls":[{"id":"call-1","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call-1","content":"tool result"}]}`,
		stripped: `{"model":%q,"messages":[{"role":"assistant","content":[{"type":"text","text":"ordinary text"}],"tool_calls":[{"id":"call-1","type":"function","function":{"name":"f","arguments":"{}"}}]},{"role":"tool","tool_call_id":"call-1","content":"tool result"}]}`,
		success:  `{"id":"chatcmpl-reasoning","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"ok"}}]}`,
	},
	{
		name: "responses", path: "/v1/responses",
		intact:   `{"model":%q,"input":[{"type":"reasoning","encrypted_content":"opaque-history","summary":[]},{"type":"thinking","thinking":"unsigned-history"},{"type":"message","role":"user","content":"ordinary text"},{"type":"function_call","call_id":"call-1","name":"f","arguments":"{}"},{"type":"function_call_output","call_id":"call-1","output":"tool result"}]}`,
		stripped: `{"model":%q,"input":[{"type":"message","role":"user","content":"ordinary text"},{"type":"function_call","call_id":"call-1","name":"f","arguments":"{}"},{"type":"function_call_output","call_id":"call-1","output":"tool result"}]}`,
		success:  `{"id":"resp-reasoning","object":"response","output":[]}`,
	},
}

type binaryReasoningUpstream struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []upstreamCall
}

func newBinaryReasoningUpstream(t *testing.T, op binaryReasoningOperation, statuses ...int) *binaryReasoningUpstream {
	t.Helper()
	stub := &binaryReasoningUpstream{}
	stub.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read reasoning upstream request: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		stub.mu.Lock()
		index := len(stub.calls)
		stub.calls = append(stub.calls, upstreamCall{Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Body: string(body)})
		stub.mu.Unlock()
		if r.Method != http.MethodPost || r.URL.Path != op.path {
			http.NotFound(w, r)
			return
		}
		status, response := http.StatusOK, op.success
		if len(statuses) > 0 {
			status = statuses[min(index, len(statuses)-1)]
		}
		switch status {
		case http.StatusBadRequest:
			response = binaryReasoningMismatch
		case http.StatusServiceUnavailable:
			response = `{"error":{"message":"ordinary failover"}}`
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Reasoning-Fixture", op.name)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, response)
	}))
	t.Cleanup(stub.server.Close)
	return stub
}

func (s *binaryReasoningUpstream) assertCalls(t *testing.T, op binaryReasoningOperation, key, model string, stripped ...bool) {
	t.Helper()
	s.mu.Lock()
	calls := append([]upstreamCall(nil), s.calls...)
	s.mu.Unlock()
	if len(calls) != len(stripped) {
		t.Fatalf("upstream %s calls = %d, want %d: %+v", key, len(calls), len(stripped), calls)
	}
	for i, call := range calls {
		if call.Path != op.path || call.Authorization != "Bearer "+key {
			t.Fatalf("call %d identity = %+v, want path %s and local credential %s", i+1, call, op.path, key)
		}
		template := op.intact
		if stripped[i] {
			template = op.stripped
		}
		var got, want any
		if err := json.Unmarshal([]byte(call.Body), &got); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(fmt.Sprintf(template, model)), &want); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("call %d body = %s, want %s", i+1, call.Body, fmt.Sprintf(template, model))
		}
	}
}

func binaryReasoningProvider(name, upstreamURL, key, upstreamModel, policy, modelPolicy string) string {
	return fmt.Sprintf(`provider "openai-compatible" %q {
  base_url = %q
  api_key = %q
  %s
  model "m" {
    upstream_name = %q
    %s
  }
}
`, name, upstreamURL+"/v1", key, policy, upstreamModel, modelPolicy)
}

func binaryReasoningAlias(policy, retryCodes string) string {
	return fmt.Sprintf(`alias "route" {
  algorithm = "round_robin"
  retry_status_codes = %s
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
`, retryCodes, policy)
}

func binaryReasoningConfig(addr, root, providers, aliases string) string {
	return fmt.Sprintf(`listener "http" "public" { address = %q }
auth "main" {
  mode = "bearer_static"
  client "binary" { token = "inbound-reasoning-token" }
}
%s
%s
%s
`, addr, root, providers, aliases)
}

func startBinaryReasoningServer(t *testing.T, addr, source string) (*binaryServer, string) {
	t.Helper()
	path := writeConfig(t, source)
	env := binaryTLSChildEnv(t, binaryTLSCertificate{}, "", nil)
	return startBinaryTLSServer(t, path, addr, filepath.Dir(path), env), path
}

func postBinaryReasoning(t *testing.T, srv *binaryServer, op binaryReasoningOperation, model string, wantStatus int) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.baseURL+op.path, strings.NewReader(fmt.Sprintf(op.intact, model)))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer inbound-reasoning-token")
	resp, err := srv.client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	wantBody := op.success
	if wantStatus == http.StatusBadRequest {
		wantBody = binaryReasoningMismatch
	}
	if resp.StatusCode != wantStatus || string(data) != wantBody || resp.Header.Get("X-Reasoning-Fixture") != op.name {
		t.Fatalf("inference = %d %s, header %q; want %d %s with original fixture header", resp.StatusCode, data, resp.Header.Get("X-Reasoning-Fixture"), wantStatus, wantBody)
	}
}
