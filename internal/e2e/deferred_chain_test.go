package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/app"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
)

type responseInactivityConn struct {
	net.Conn
	window time.Duration
	cuts   *atomic.Int32
}

func (c *responseInactivityConn) Read(p []byte) (int, error) {
	_ = c.Conn.SetReadDeadline(time.Now().Add(c.window))
	n, err := c.Conn.Read(p)
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		c.cuts.Add(1)
	}
	return n, err
}

func newResponseInactivityCutter(t *testing.T, target string, window time.Duration, modify func(*http.Response) error) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	origin, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	cuts := &atomic.Int32{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	transport.DisableKeepAlives = true
	transport.ResponseHeaderTimeout = 0
	dialer := &net.Dialer{Timeout: time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &responseInactivityConn{Conn: conn, window: window, cuts: cuts}, nil
	}
	proxy := &httputil.ReverseProxy{
		Transport: transport,
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(origin)
			r.Out.URL.Path = strings.TrimPrefix(r.In.URL.Path, "/mount")
			r.Out.URL.RawPath = ""
		},
		ModifyResponse: func(response *http.Response) error {
			if location := response.Header.Get("Location"); location != "" {
				response.Header.Set("Location", "/mount"+location)
			}
			if modify != nil {
				return modify(response)
			}
			return nil
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "response inactivity cutoff", http.StatusGatewayTimeout)
		},
	}
	server := httptest.NewServer(proxy)
	t.Cleanup(func() {
		server.Close()
		transport.CloseIdleConnections()
	})
	return server, cuts
}

func newDeferredChainApp(t *testing.T, path string) (*app.App, *httptest.Server) {
	t.Helper()
	a, err := app.Build(context.Background(), app.BuildOptions{ConfigPath: path, Version: "test", LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(a.Server.Handler)
	t.Cleanup(func() {
		server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if err := a.CloseContext(ctx); err != nil {
			t.Errorf("chain app close: %v", err)
		}
	})
	return a, server
}

func deferredExecutorConfig(upstream string) string {
	return `
listener "http" "public" { address = ":0" }
auth "main" {
  mode = "bearer_static"
  client "peer" { token = "peer-token" }
}
metrics { token = "metrics-token" }
deferred {
  max_jobs = 4
  max_running = 2
  max_admissions = 2
  max_entries = 16
  max_memory_bytes = 33554432
  max_request_bytes = 65536
  max_result_bytes = 131072
  max_work_bytes = 4194304
  per_owner_jobs = 4
  per_owner_running = 2
  max_control_inflight = 4
  execution_timeout = "2s"
  retention = "10s"
  control_timeout = "200ms"
  finalization_timeout = "200ms"
}
provider "openai-compatible" "p" {
  base_url = "` + upstream + `/v1"
  api_key = "ultimate-secret"
  forward_user_agent = true
  forward_headers = ["x-inference"]
  deferred_receiver {}
  model "m" { upstream_name = "actual-model" }
}
`
}

func deferredCallerConfig(peer, credential string, optIn bool) string {
	block := ""
	if optIn {
		block = `
  deferred_transport {
    control_timeout = "200ms"
    max_wait = "3s"
    poll_interval = "50ms"
    max_control_calls = 128
    max_control_retries = 8
  }`
	}
	return `
listener "http" "public" { address = ":0" }
auth "main" { mode = "none" }
metrics { token = "metrics-token" }
provider "openai-compatible" "peer" {
  base_url = "` + peer + `/mount/v1"
  api_key = "` + credential + `"
  forward_user_agent = true
  forward_headers = ["x-inference"]
` + block + `
  model "m" { upstream_name = "p/m" }
}
`
}

func chainChat(t *testing.T, server *httptest.Server, stream bool) *http.Response {
	t.Helper()
	request, err := http.NewRequest("POST", server.URL+"/v1/chat/completions",
		strings.NewReader(fmt.Sprintf(`{"model":"peer/m","stream":%t,"messages":[{"role":"user","content":"test"}],"vendor":{"unchanged":true}}`, stream)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "chain-client")
	request.Header.Set("X-Inference", "chain-inference")
	request.Header.Set("X-Aiproxy-Deferred-Version", "99")
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func awaitChain(t *testing.T, label string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", label)
		}
		time.Sleep(time.Millisecond)
	}
}

func assertChainAccounting(t *testing.T, server *httptest.Server, token string, total int64) {
	t.Helper()
	request, _ := http.NewRequest("GET", server.URL+"/v1/billing/usage", nil)
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var usage struct {
		Data []accounting.Summary `json:"data"`
	}
	if json.NewDecoder(response.Body).Decode(&usage) != nil || len(usage.Data) != 1 || usage.Data[0].Count != 1 || usage.Data[0].TotalTokens != total {
		t.Fatalf("chain completion/accounting duplicated or lost: %+v status=%d", usage, response.StatusCode)
	}
}

func TestDeferredPeerChainSurvivesResponseInactivityCutter(t *testing.T) {
	const idle = 100 * time.Millisecond
	const silence = 350 * time.Millisecond
	unary := []byte(" \n" + `{"choices":[],"vendor":{"exact":true},"usage":{"prompt_tokens":3,"completion_tokens":2,"total_tokens":5}}` + "\n")
	first := "id: original-3\r\nevent: vendor\r\ndata: {\"choices\":[{\"delta\":{\"content\":\"first"
	last := "\"}}]}\r\n\r\n: native-comment\n\ndata: {\"usage\":{\"prompt_tokens\":3,\"completion_tokens\":2,\"total_tokens\":5}}\n\ndata: [DONE]\n\n"
	for _, shape := range []string{"unary-initial", "sse-initial", "sse-later"} {
		for _, optIn := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/deferred=%t", shape, optIn), func(t *testing.T) {
				var calls atomic.Int32
				upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					input, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(input), `"model":"actual-model"`) || !strings.Contains(string(input), `"vendor":{"unchanged":true}`) ||
						r.Header.Get("Authorization") != "Bearer ultimate-secret" || r.Header.Get("X-Inference") != "chain-inference" {
						t.Errorf("ordinary inference changed across two apps: %s %#v", input, r.Header)
					}
					for name := range r.Header {
						if config.IsDeferredHeader(name) {
							t.Errorf("private protocol header leaked to ultimate provider: %s", name)
						}
					}
					if shape != "sse-later" {
						select {
						case <-time.After(silence):
						case <-r.Context().Done():
							return
						}
					}
					if shape == "unary-initial" {
						w.Header().Set("Content-Type", "application/json")
						_, _ = w.Write(unary)
						return
					}
					w.Header().Set("Content-Type", "text/event-stream")
					w.WriteHeader(200)
					w.(http.Flusher).Flush()
					_, _ = io.WriteString(w, first)
					w.(http.Flusher).Flush()
					select {
					case <-time.After(silence):
					case <-r.Context().Done():
						return
					}
					for _, fragment := range []string{last[:7], last[7:36], last[36:]} {
						_, _ = io.WriteString(w, fragment)
						w.(http.Flusher).Flush()
					}
				}))
				defer upstream.Close()
				executor, executorServer := newDeferredChainApp(t, writeConfig(t, deferredExecutorConfig(upstream.URL)))
				cutter, cuts := newResponseInactivityCutter(t, executorServer.URL, idle, nil)
				_, callerServer := newDeferredChainApp(t, writeConfig(t, deferredCallerConfig(cutter.URL, "peer-token", optIn)))
				before := time.Now()
				response := chainChat(t, callerServer, shape != "unary-initial")
				body, readErr := io.ReadAll(response.Body)
				_ = response.Body.Close()
				if optIn {
					expected := unary
					if shape != "unary-initial" {
						expected = []byte(first + last)
					}
					if response.StatusCode != 200 || readErr != nil || !bytes.Equal(body, expected) || cuts.Load() != 0 ||
						time.Since(before) < silence || response.Header.Get("X-Aiproxy-Deferred-Version") != "" {
						t.Fatalf("deferred chain failed inactivity fixture: status=%d err=%v bytes=%q cuts=%d elapsed=%s", response.StatusCode, readErr, body, cuts.Load(), time.Since(before))
					}
					awaitChain(t, "released executor ownership", func() bool {
						s := executor.DeferredStore().Stats()
						return s.Jobs == 0 && s.Running == 0 && s.FinalizationCredits == 0 && s.Controls == 0 && s.Deletes == 0
					})
					assertChainAccounting(t, callerServer, "", 5)
					assertChainAccounting(t, executorServer, "peer-token", 5)
				} else if shape == "sse-later" {
					if response.StatusCode != 200 || string(body) != first || cuts.Load() == 0 {
						t.Fatalf("ordinary baseline did not fail on later response silence: status=%d err=%v bytes=%q cuts=%d", response.StatusCode, readErr, body, cuts.Load())
					}
				} else if response.StatusCode < 400 || cuts.Load() == 0 || readErr != nil {
					t.Fatalf("ordinary baseline did not fail before headers: status=%d err=%v cuts=%d body=%s", response.StatusCode, readErr, cuts.Load(), body)
				}
				if calls.Load() != 1 {
					t.Fatalf("two-proxy path duplicated inference: %d calls", calls.Load())
				}
				t.Logf("response inactivity=%s initial/later silence=%s baseline/deferred=%t cuts=%d", idle, silence, optIn, cuts.Load())
			})
		}
	}
}

func TestDeferredPeerChainDroppedAckAndReadRetainOneJob(t *testing.T) {
	var calls, submits, reads atomic.Int32
	body := []byte(`{"vendor":"` + strings.Repeat("x", 80<<10) + `","usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	defer upstream.Close()
	executor, executorServer := newDeferredChainApp(t, writeConfig(t, deferredExecutorConfig(upstream.URL)))
	cutter, cuts := newResponseInactivityCutter(t, executorServer.URL, 200*time.Millisecond, func(response *http.Response) error {
		drop := response.Request.URL.Path == deferred.Prefix+"/submit" && submits.Add(1) == 1 ||
			strings.HasSuffix(response.Request.URL.Path, "/read") && reads.Add(1) == 1
		if !drop {
			return nil
		}
		wire, err := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if err != nil {
			return err
		}
		response.Body = io.NopCloser(bytes.NewReader(wire[:len(wire)/2]))
		return nil
	})
	_, callerServer := newDeferredChainApp(t, writeConfig(t, deferredCallerConfig(cutter.URL, "peer-token", true)))
	response := chainChat(t, callerServer, false)
	delivered, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if response.StatusCode != 200 || err != nil || !bytes.Equal(delivered, body) || calls.Load() != 1 || submits.Load() != 2 || reads.Load() < 2 || cuts.Load() != 0 {
		t.Fatalf("chain retried inference instead of qualified submit/read: status=%d err=%v inference=%d submits=%d reads=%d cuts=%d", response.StatusCode, err, calls.Load(), submits.Load(), reads.Load(), cuts.Load())
	}
	awaitChain(t, "replayed chain finalization", func() bool { return executor.DeferredStore().Stats().Jobs == 0 })
	assertChainAccounting(t, callerServer, "", 5)
	assertChainAccounting(t, executorServer, "peer-token", 5)
}

func TestDeferredPeerChainCallerReloadKeepsFrozenCredentialAndTransport(t *testing.T) {
	var calls atomic.Int32
	later := make(chan struct{})
	prefix := "data: {\"choices\":[{\"delta\":{\"content\":\"before\"}}]}\n\n"
	suffix := "data: {\"choices\":[{\"delta\":{\"content\":\"after\"}}]}\n\ndata: [DONE]\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, prefix)
		w.(http.Flusher).Flush()
		select {
		case <-later:
			_, _ = io.WriteString(w, suffix)
		case <-r.Context().Done():
		}
	}))
	defer upstream.Close()
	executor, executorServer := newDeferredChainApp(t, writeConfig(t, deferredExecutorConfig(upstream.URL)))
	cutter, _ := newResponseInactivityCutter(t, executorServer.URL, 200*time.Millisecond, nil)
	callerPath := writeConfig(t, deferredCallerConfig(cutter.URL, "peer-token", true))
	caller, callerServer := newDeferredChainApp(t, callerPath)
	response := chainChat(t, callerServer, true)
	initial := make([]byte, len(prefix))
	if _, err := io.ReadFull(response.Body, initial); err != nil || string(initial) != prefix {
		t.Fatalf("stream did not reach pre-reload commitment: %v %q", err, initial)
	}
	if err := os.WriteFile(callerPath, []byte(deferredCallerConfig(cutter.URL, "rotated-wrong-token", true)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := caller.Reload(); err != nil {
		t.Fatal(err)
	}
	close(later)
	remaining, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil || string(remaining) != suffix || calls.Load() != 1 {
		t.Fatalf("in-flight job adopted reloaded transport/credential: %v %q calls=%d", err, remaining, calls.Load())
	}
	failed := chainChat(t, callerServer, false)
	failureBody, _ := io.ReadAll(failed.Body)
	_ = failed.Body.Close()
	if failed.StatusCode != 502 || !bytes.Contains(failureBody, []byte("deferred_control_error")) || calls.Load() != 1 {
		t.Fatalf("new request did not use rotated credential or fell back: status=%d body=%s calls=%d", failed.StatusCode, failureBody, calls.Load())
	}
	awaitChain(t, "frozen chain release", func() bool { return executor.DeferredStore().Stats().Jobs == 0 })
}
