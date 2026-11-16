//go:build integration && linux

package integration

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
)

func binaryDeferredMixedExecutorConfig(addr, upstream string, policy binaryDeferredPolicy) string {
	return strings.NewReplacer(
		`allowed_models = ["p/m"]`, `allowed_models = ["p/model-1", "p/model-2"]`,
		`"other-token"`, `"next-token"`,
		`  model "m" { upstream_name = "actual-model" }`, `
  model "model-1" { upstream_name = "actual-model-1" }
  model "model-2" { upstream_name = "actual-model-2" }`,
	).Replace(binaryDeferredExecutorConfig(addr, upstream, "peer-token", "ultimate-key", "actual-model", policy))
}

func binaryDeferredMixedOptions() *config.DeferredTransport {
	options := config.DefaultDeferredTransport()
	options.ControlTimeout, options.MaxWait, options.PollInterval = time.Second, 5*time.Second, 50*time.Millisecond
	options.MaxControlCalls = 256
	return &options
}

func binaryDeferredMixedBlock(options *config.DeferredTransport) string {
	if options == nil {
		return ""
	}
	return fmt.Sprintf(`
    deferred_transport {
      control_timeout = %q
      max_wait = %q
      poll_interval = %q
      max_control_calls = %d
      max_control_retries = %d
    }`, options.ControlTimeout.String(), options.MaxWait.String(), options.PollInterval.String(), options.MaxControlCalls, options.MaxControlRetries)
}

func binaryDeferredMixedCallerConfig(addr, peer, token string, first, second *config.DeferredTransport, deferredFirst bool) string {
	base := binaryDeferredCallerConfig(addr, peer, token, false, time.Second, 5*time.Second, "")
	models := fmt.Sprintf(`  model "model-1" {
    upstream_name = "p/model-1"
    %s
  }
  model "model-2" {
    upstream_name = "p/model-2"
    %s
  }`, binaryDeferredMixedBlock(first), binaryDeferredMixedBlock(second))
	base = strings.NewReplacer(
		`provider "openai-compatible" "peer"`, `provider "openai-compatible" "a"`,
		`  model "m" {
    upstream_name = "p/m"
  }`, models,
	).Replace(base)
	targets := []string{"model-1", "model-2"}
	if deferredFirst {
		targets[0], targets[1] = targets[1], targets[0]
	}
	return base + fmt.Sprintf(`
alias "mixed" {
  algorithm = "round_robin"
  retry_status_codes = ["400", "429", "500", "502", "503", "504"]
  target {
    provider = "a"
    model = %q
  }
  target {
    provider = "a"
    model = %q
  }
}
alias "first" {
  algorithm = "round_robin"
  target {
    provider = "a"
    model = "model-1"
  }
}
alias "second" {
  algorithm = "round_robin"
  target {
    provider = "a"
    model = "model-2"
  }
}
`, targets[0], targets[1])
}

func binaryDeferredMixedResponse(model, path string, stream bool) []byte {
	if stream {
		return []byte(": original\r\nid: " + model + "\r\nevent: vendor\r\ndata: {\"path\":\"" + path + "\",\"vendor\":\"kept\"}\r\n\r\n" +
			"data: {\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":3,\"total_tokens\":5}}\n\ndata: [DONE]\n\n")
	}
	field := "choices"
	if path == "/v1/responses" {
		field = "output"
	} else if path == "/v1/embeddings" {
		field = "data"
	}
	return []byte(" \n" + fmt.Sprintf(`{"model":%q,"path":%q,%q:[],"vendor":"kept","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`, model, path, field) + "\n")
}

func binaryDeferredMixedInput(model, path string, stream bool) []byte {
	if path == "/v1/chat/completions" {
		return binaryDeferredChatBody(model, stream)
	}
	if path == "/v1/embeddings" {
		return []byte(fmt.Sprintf(`{"model":%q,"input":"test","vendor":{"unchanged":true}}`, model))
	}
	return []byte(fmt.Sprintf(`{"model":%q,"stream":%t,"input":"test","vendor":{"unchanged":true}}`, model, stream))
}

func privateBinaryDeferredMixedCalls(peer *binaryDeferredPeer) int {
	count := 0
	for _, call := range peer.Calls("") {
		if strings.HasPrefix(call.path, "/mount"+deferred.Prefix) {
			count++
		}
	}
	return count
}

func setBinaryDeferredMixedPeerHooks(t *testing.T, peer *binaryDeferredPeer, token string, modify func(*http.Response) error) {
	t.Helper()
	peer.hooks(func(w http.ResponseWriter, r *http.Request) bool {
		if r.Header.Get("Authorization") != "Bearer "+token || r.Header.Get("Cookie") != "" {
			t.Errorf("actual binary peer lost its configured credential or forwarded a cookie: %s", r.URL.Path)
		}
		if strings.HasPrefix(r.URL.Path, "/mount"+deferred.Prefix) {
			if r.Header.Get("X-Aiproxy-Deferred-Version") != "1" || r.Header.Get("Accept") != deferred.MediaType {
				t.Errorf("actual binary control lost media/version markers: %s", r.URL.Path)
			}
		} else {
			for name := range r.Header {
				if config.IsDeferredHeader(name) {
					t.Errorf("ordinary binary sibling forwarded a private header: %s", name)
				}
			}
		}
		return false
	}, modify)
}

func replaceBinaryDeferredMixedControl(response *http.Response, status int, wire []byte) {
	_ = response.Body.Close()
	response.StatusCode, response.ContentLength = status, int64(len(wire))
	response.Header.Set("Content-Length", strconv.Itoa(len(wire)))
	response.Body = io.NopCloser(bytes.NewReader(wire))
}
