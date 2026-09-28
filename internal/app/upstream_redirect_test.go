package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/upstreamhttp"
)

func TestBuildUpstreamRedirectIsolation(t *testing.T) {
	for _, provider := range []struct {
		kind, header, credential, response string
	}{
		{"anthropic", "x-api-key", "synthetic-key", `{"id":"msg_test","type":"message","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`},
		{"gemini", "x-goog-api-key", "synthetic-key", `{"candidates":[{"content":{"parts":[{"text":"ok"}],"role":"model"},"finishReason":"STOP"}]}`},
		{"openai", "Authorization", "Bearer synthetic-key", `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`},
	} {
		for _, status := range []int{302, 307, 308} {
			for _, sameOrigin := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%d/same-origin=%t", provider.kind, status, sameOrigin), func(t *testing.T) {
					var destinations, starts atomic.Int32
					finish := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						destinations.Add(1)
						body, _ := io.ReadAll(r.Body)
						if !sameOrigin {
							t.Errorf("prohibited destination reached: credential=%t custom-header=%t body=%t", r.Header.Get(provider.header) == provider.credential, r.Header.Get("X-Custom-Secret") != "", strings.Contains(string(body), "synthetic-prompt"))
						} else {
							if r.Header.Get(provider.header) != provider.credential || r.Header.Get("X-Custom-Secret") != "synthetic-custom" {
								t.Error("same-origin redirect lost headers")
							}
							if status != 302 && (r.Method != http.MethodPost || !strings.Contains(string(body), "synthetic-prompt")) {
								t.Error("same-origin redirect lost POST body")
							}
							if status == 302 && (r.Method != http.MethodGet || len(body) != 0) {
								t.Error("302 redirect did not retain Go's GET conversion")
							}
						}
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, provider.response)
					})
					destination := httptest.NewServer(finish)
					defer destination.Close()
					upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/redirected" {
							finish.ServeHTTP(w, r)
							return
						}
						starts.Add(1)
						if r.Header.Get(provider.header) != provider.credential || r.Header.Get("X-Custom-Secret") != "synthetic-custom" {
							t.Error("initial request missing credentials")
						}
						target := destination.URL + "/redirected"
						if sameOrigin {
							target = "/redirected"
						}
						http.Redirect(w, r, target, status)
					}))
					defer upstream.Close()
					path := writeConfigFile(t, fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider %q "test" {
  base_url = %q
  api_key = "synthetic-key"
  forward_headers = ["X-Custom-Secret"]
  model "test" {}
}
`, provider.kind, upstream.URL))
					a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
					if err != nil {
						t.Fatal(err)
					}
					defer a.Close()
					r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test/test","messages":[{"role":"user","content":"synthetic-prompt"}]}`))
					r.Header.Set("Content-Type", "application/json")
					r.Header.Set("X-Custom-Secret", "synthetic-custom")
					w := httptest.NewRecorder()
					a.Server.Handler.ServeHTTP(w, r)
					if starts.Load() != 1 {
						t.Fatalf("initial requests = %d, want 1", starts.Load())
					}
					if sameOrigin {
						if destinations.Load() != 1 || w.Code != http.StatusOK {
							t.Fatalf("allowed redirect: calls=%d response=%d %s", destinations.Load(), w.Code, w.Body.String())
						}
					} else if destinations.Load() != 0 || w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), `"upstream_error"`) {
						t.Fatalf("blocked redirect: calls=%d response=%d %s", destinations.Load(), w.Code, w.Body.String())
					}
				})
			}
		}
	}
}

func TestBuildUpstreamRedirectLoopFailure(t *testing.T) {
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		http.Redirect(w, r, "/loop", http.StatusTemporaryRedirect)
	}))
	defer upstream.Close()
	path := writeConfigFile(t, fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "anthropic" "test" {
  base_url = %q
  api_key = "synthetic-key"
  model "test" {}
}
`, upstream.URL))
	a, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"test/test","messages":[{"role":"user","content":"synthetic-prompt"}]}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	a.Server.Handler.ServeHTTP(w, r)
	if calls.Load() != upstreamhttp.MaxRedirects+1 || w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), `"upstream_error"`) {
		t.Fatalf("redirect loop: calls=%d response=%d %s", calls.Load(), w.Code, w.Body.String())
	}
}
