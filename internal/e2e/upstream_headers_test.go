package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/payloadlog"
)

func TestScopedUpstreamHeadersStayLocalAcrossDeferredPeer(t *testing.T) {
	const senderValue = "synthetic-sender-config-value"
	const receiverValue = "synthetic-receiver-fixed-value"
	const callerValue = "permitted-caller-value"
	const upstreamResponse = `{"choices":[]}`
	var upstreamCalls, controlCalls, submitCalls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalls.Add(1)
		for name, want := range map[string]string{
			"X-Choice": receiverValue, "X-Receiver-Root": "root-value", "X-Receiver-Provider": "provider-value",
			"X-Receiver-Model": "model-value", "X-Preserved": "forwarded", "Authorization": "Bearer mock-ultimate",
		} {
			if !slices.Equal(r.Header.Values(name), []string{want}) {
				t.Errorf("ultimate provider missing the single expected %s value", name)
			}
		}
		for name := range r.Header {
			if strings.HasPrefix(strings.ToLower(name), "x-sender-") {
				t.Errorf("sender configuration reached ultimate provider: %s", name)
			}
		}
		var body struct{ Model string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Model != "actual-model" {
			t.Errorf("receiver did not route to its model: %q, %v", body.Model, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, upstreamResponse)
	}))
	t.Cleanup(upstream.Close)
	dir := t.TempDir()
	receiverConfig := fmt.Sprintf(`
upstream_headers = { X-Choice = "receiver-root-choice", X-Receiver-Root = "root-value" }
listener "http" "public" { address = ":0" }
auth "main" {
  mode = "bearer_static"
  client "peer" { token = "mock-peer-token" }
}
logging {
  payload_log {
    enabled = true
    dir = %q
    max_body_bytes = 4096
  }
}
deferred {
  execution_timeout = "2s"
  control_timeout = "1s"
}
provider "openai-compatible" "ultimate" {
  base_url = %q
  api_key = "mock-ultimate"
  deferred_receiver {}
  forward_headers = ["X-Choice", "X-Preserved", "X-Sender-Root", "X-Sender-Provider", "X-Sender-Model", "X-Sender-Alias"]
  upstream_headers = { X-Receiver-Provider = "provider-value" }
  model "m" {
    upstream_name = "actual-model"
    upstream_headers = { X-Receiver-Model = "model-value" }
  }
}
alias "local" {
  algorithm = "round_robin"
  providers = ["ultimate"]
  model = "m"
  upstream_headers = { x-choice = %q }
}
`, dir, upstream.URL, receiverValue)
	receiver, _ := newDeferredChainApp(t, writeConfig(t, receiverConfig))
	checkNoSenderHeaders := func(headers http.Header) {
		for name := range headers {
			if strings.HasPrefix(strings.ToLower(name), "x-sender-") {
				t.Errorf("sender configuration reached a peer request: %s", name)
			}
		}
	}
	peer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		controlCalls.Add(1)
		if !strings.HasPrefix(r.URL.Path, deferred.Prefix+"/") || r.Header.Get("X-Choice") != "" {
			t.Errorf("unexpected peer control path or headers: %s", r.URL.Path)
		}
		checkNoSenderHeaders(r.Header)
		if r.URL.Path == deferred.Prefix+"/submit" {
			submitCalls.Add(1)
			wire, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read peer submit: %v", err)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			_ = r.Body.Close()
			r.Body = io.NopCloser(bytes.NewReader(wire))
			var submit deferred.SubmitRequest
			if err := json.Unmarshal(wire, &submit); err != nil {
				t.Errorf("decode peer submit: %v", err)
			}
			checkNoSenderHeaders(submit.Headers)
			if bytes.Contains(wire, []byte(senderValue)) || !slices.Equal(submit.Headers.Values("X-Choice"), []string{callerValue}) ||
				!slices.Equal(submit.Headers.Values("X-Preserved"), []string{"forwarded"}) {
				t.Error("peer submit included a fixed value or lost caller metadata")
			}
		}
		receiver.Server.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(peer.Close)
	senderConfig := fmt.Sprintf(`
upstream_headers = { X-Sender-Root = %[1]q, X-Choice = %[1]q }
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
provider "openai-compatible" "peer" {
  base_url = %[2]q
  api_key = "mock-peer-token"
  forward_headers = ["X-Choice", "X-Preserved"]
  upstream_headers = { X-Sender-Provider = %[1]q }
  model "m" {
    upstream_name = "alias/local"
    upstream_headers = { X-Sender-Model = %[1]q }
    deferred_transport {
      control_timeout = "1s"
      max_wait = "5s"
      poll_interval = "50ms"
    }
  }
}
alias "send" {
  algorithm = "round_robin"
  providers = ["peer"]
  model = "m"
  upstream_headers = { X-Sender-Alias = %[1]q }
}
`, senderValue, peer.URL)
	_, sender := newDeferredChainApp(t, writeConfig(t, senderConfig))
	r, err := http.NewRequestWithContext(t.Context(), http.MethodPost, sender.URL+"/v1/chat/completions",
		strings.NewReader(`{"model":"alias/send","messages":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-Choice", callerValue)
	r.Header.Set("X-Preserved", "forwarded")
	resp, err := sender.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK || string(raw) != upstreamResponse ||
		controlCalls.Load() < 3 || submitCalls.Load() != 1 || upstreamCalls.Load() != 1 {
		t.Fatalf("deferred response=%d %q, controls=%d, submits=%d, upstreams=%d, err=%v",
			resp.StatusCode, raw, controlCalls.Load(), submitCalls.Load(), upstreamCalls.Load(), err)
	}
	awaitChain(t, "receiver payload finalization", func() bool {
		stats := receiver.DeferredStore().Stats()
		return stats.Running == 0 && stats.FinalizationCredits == 0
	})
	files, err := filepath.Glob(filepath.Join(dir, "payload-*.jsonl"))
	if err != nil || len(files) != 1 {
		t.Fatalf("receiver payload files: %v, %v", files, err)
	}
	log, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var entry payloadlog.Entry
	if err := json.Unmarshal(log, &entry); err != nil {
		t.Fatalf("parse receiver payload capture: %v", err)
	}
	for _, name := range []string{"X-Choice", "X-Receiver-Root", "X-Receiver-Provider", "X-Receiver-Model"} {
		if !slices.Equal(entry.UpstreamRequest.Headers[name], []string{"[REDACTED]"}) {
			t.Errorf("receiver upstream capture did not mask %s", name)
		}
	}
	if bytes.Contains(log, []byte(receiverValue)) || bytes.Contains(log, []byte(senderValue)) ||
		!slices.Equal(entry.Request.Headers["X-Choice"], []string{callerValue}) ||
		!slices.Equal(entry.UpstreamRequest.Headers["X-Preserved"], []string{"forwarded"}) {
		t.Error("receiver capture leaked a fixed value or changed caller data")
	}
}
