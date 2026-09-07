//go:build integration && linux

package integration

import (
	"bufio"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type upstreamCall struct {
	Path          string
	Authorization string
	Body          string
}

type upstreamResponse struct {
	Status      int
	ContentType string
	Body        string
	Headers     map[string]string
	Hold        <-chan struct{}
}

type upstreamStub struct {
	server    *httptest.Server
	responses []upstreamResponse

	mu    sync.Mutex
	calls []upstreamCall
}

func newUpstreamStub(t *testing.T, responses ...upstreamResponse) *upstreamStub {
	t.Helper()
	s := &upstreamStub{responses: responses}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read upstream body: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		s.mu.Lock()
		s.calls = append(s.calls, upstreamCall{Path: r.URL.Path, Authorization: r.Header.Get("Authorization"), Body: string(body)})
		idx := len(s.calls) - 1
		s.mu.Unlock()

		resp := upstreamResponse{Status: http.StatusOK, ContentType: "application/json", Body: `{"id":"chatcmpl_default","object":"chat.completion","choices":[]}`}
		if len(s.responses) > 0 {
			if idx >= len(s.responses) {
				idx = len(s.responses) - 1
			}
			resp = s.responses[idx]
		}
		if resp.Status == 0 {
			resp.Status = http.StatusOK
		}
		if resp.ContentType == "" {
			resp.ContentType = "application/json"
		}
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", resp.ContentType)
		for key, value := range resp.Headers {
			w.Header().Set(key, value)
		}
		w.WriteHeader(resp.Status)
		_, _ = w.Write([]byte(resp.Body))
		if flusher, ok := w.(http.Flusher); ok {
			flusher.Flush()
		}
		if resp.Hold != nil {
			<-resp.Hold
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *upstreamStub) URL() string {
	return s.server.URL
}

func (s *upstreamStub) Calls() []upstreamCall {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]upstreamCall, len(s.calls))
	copy(out, s.calls)
	return out
}

func (s *upstreamStub) WaitCalls(t *testing.T, want int) []upstreamCall {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		calls := s.Calls()
		if len(calls) >= want {
			return calls
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("upstream calls = %d, want at least %d", len(s.Calls()), want)
	return nil
}

type binaryServer struct {
	cmd     *exec.Cmd
	done    chan error
	logs    chan string
	baseURL string
	client  *http.Client
	stop    sync.Once
}

func startBinaryServer(t *testing.T, configPath, addr string) *binaryServer {
	t.Helper()
	cmd := exec.Command(aiproxyBinary(t), "serve", "--config", configPath)
	cmd.Env = append(os.Environ(), "XDG_CONFIG_HOME="+t.TempDir())
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		t.Fatalf("stderr pipe: %v", err)
	}
	s := &binaryServer{
		cmd:     cmd,
		done:    make(chan error, 1),
		logs:    make(chan string, 200),
		baseURL: "http://" + addr,
		client:  &http.Client{Timeout: 5 * time.Second},
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start aiproxy: %v", err)
	}
	go scanLines(stdout, s.logs)
	go scanLines(stderr, s.logs)
	go func() { s.done <- cmd.Wait() }()
	t.Cleanup(func() { s.Stop(t) })
	s.WaitReady(t)
	return s
}

func scanLines(r io.Reader, ch chan<- string) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		select {
		case ch <- s.Text():
		default:
		}
	}
}

func (s *binaryServer) WaitReady(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-s.done:
			t.Fatalf("aiproxy exited before ready: %v\nlogs:\n%s", err, s.DrainLogs())
		default:
		}
		resp, err := s.client.Get(s.baseURL + "/readyz")
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("aiproxy not ready: %v", lastErr)
}

func (s *binaryServer) WaitLogContains(t *testing.T, text string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case line := <-s.logs:
			if strings.Contains(line, text) {
				return
			}
		case err := <-s.done:
			t.Fatalf("aiproxy exited while waiting for log %q: %v\nlogs:\n%s", text, err, s.DrainLogs())
		case <-timer.C:
			t.Fatalf("timed out waiting for log %q", text)
		}
	}
}

func (s *binaryServer) DrainLogs() string {
	var b strings.Builder
	for {
		select {
		case line := <-s.logs:
			b.WriteString(line)
			b.WriteByte('\n')
		default:
			return b.String()
		}
	}
}

func (s *binaryServer) Stop(t *testing.T) {
	t.Helper()
	s.stop.Do(func() {
		select {
		case err := <-s.done:
			if err != nil {
				t.Fatalf("aiproxy exited unexpectedly: %v", err)
			}
			return
		default:
		}
		if err := s.cmd.Process.Signal(os.Interrupt); err != nil {
			t.Fatalf("signal aiproxy: %v", err)
		}
		select {
		case err := <-s.done:
			if err != nil {
				t.Fatalf("aiproxy shutdown: %v", err)
			}
		case <-time.After(5 * time.Second):
			_ = s.cmd.Process.Kill()
			t.Fatal("aiproxy did not shut down after interrupt")
		}
	})
}

func TestBinaryLifecycleReloadAuthMetricsStreamingDerived(t *testing.T) {
	upstream := newUpstreamStub(t,
		upstreamResponse{ContentType: "text/event-stream", Body: "data: {\"choices\":[{\"delta\":{\"content\":\"from-derived-stream\"}}]}\n\ndata: [DONE]\n\n"},
		upstreamResponse{Body: `{"id":"chatcmpl_reloaded","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"from-reloaded"}}]}`},
		upstreamResponse{Body: `{"id":"chatcmpl_after_failed_reload","object":"chat.completion","choices":[{"message":{"role":"assistant","content":"rollback-ok"}}]}`},
	)
	addr := freeAddr(t)
	configPath := writeConfig(t, lifecycleConfig(addr, upstream.URL(), "sk-derived", false))
	srv := startBinaryServer(t, configPath, addr)

	if status, body := httpGet(t, srv, "/healthz", ""); status != http.StatusOK || strings.TrimSpace(body) != "ok" {
		t.Fatalf("GET /healthz = %d %q, want 200 ok", status, body)
	}
	if status, _ := postChat(t, srv, "", `{"model":"alias/stream","stream":true,"messages":[{"role":"user","content":"hi"}]}`); status != http.StatusUnauthorized {
		t.Fatalf("unauthenticated chat status = %d, want 401", status)
	}
	if status, _ := httpGet(t, srv, "/metrics", "wrong-token"); status != http.StatusUnauthorized {
		t.Fatalf("GET /metrics with wrong token = %d, want 401", status)
	}
	if status, body := httpGet(t, srv, "/metrics", "metrics-token"); status != http.StatusOK || !strings.Contains(body, "aiproxy_build_info") {
		t.Fatalf("GET /metrics with correct token = %d, body missing build info", status)
	}

	status, body := postChat(t, srv, "api-token", `{"model":"alias/stream","stream":true,"messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK || !strings.Contains(body, "from-derived-stream") || !strings.Contains(body, "[DONE]") {
		t.Fatalf("streaming chat = %d %q, want streamed response", status, body)
	}
	calls := upstream.WaitCalls(t, 1)
	if calls[0].Authorization != "Bearer sk-derived" || !strings.Contains(calls[0].Body, `"model":"upstream-chat"`) {
		t.Fatalf("derived upstream call = %+v, want local key and inherited upstream model", calls[0])
	}

	writeConfigPath(t, configPath, lifecycleConfig(addr, upstream.URL(), "sk-derived-two", true))
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("signal reload: %v", err)
	}
	srv.WaitLogContains(t, "config reloaded")
	if status, body := httpGet(t, srv, "/v1/models", "api-token"); status != http.StatusOK || !strings.Contains(body, "derivedtwo/chat") {
		t.Fatalf("models after reload = %d %q, want derivedtwo/chat", status, body)
	}
	status, _ = postChat(t, srv, "api-token", `{"model":"derivedtwo/chat","messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("derivedtwo chat status = %d, want 200", status)
	}
	calls = upstream.WaitCalls(t, 2)
	if calls[1].Authorization != "Bearer sk-derived-two" {
		t.Fatalf("reloaded derived auth = %q, want Bearer sk-derived-two", calls[1].Authorization)
	}

	writeConfigPath(t, configPath, failedReloadConfig(addr, upstream.URL()))
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatalf("signal failed reload: %v", err)
	}
	srv.WaitLogContains(t, "config reload failed")
	if status, body := httpGet(t, srv, "/v1/models", "api-token"); status != http.StatusOK || !strings.Contains(body, "derivedtwo/chat") || strings.Contains(body, "candidate/chat") {
		t.Fatalf("models after failed reload = %d %q, want previous catalog only", status, body)
	}
	status, _ = postChat(t, srv, "api-token", `{"model":"derivedtwo/chat","messages":[{"role":"user","content":"hi"}]}`)
	if status != http.StatusOK {
		t.Fatalf("derivedtwo chat after failed reload status = %d, want 200", status)
	}
	srv.Stop(t)
}

func TestBinaryAliasRoutingAndRetrySemantics(t *testing.T) {
	t.Run("round_robin", func(t *testing.T) {
		a := newUpstreamStub(t, upstreamResponse{Body: `{"id":"a","object":"chat.completion","choices":[]}`})
		b := newUpstreamStub(t, upstreamResponse{Body: `{"id":"b","object":"chat.completion","choices":[]}`})
		srv := startRoutingServer(t, routingConfig(t, "round_robin", nil, a.URL(), b.URL()))
		for i := 0; i < 3; i++ {
			if status, body := postChat(t, srv, "", `{"model":"alias/route","messages":[]}`); status != http.StatusOK {
				t.Fatalf("request %d status = %d body=%s", i+1, status, body)
			}
		}
		if got := len(a.Calls()); got != 2 {
			t.Fatalf("provider a calls = %d, want 2", got)
		}
		if got := len(b.Calls()); got != 1 {
			t.Fatalf("provider b calls = %d, want 1", got)
		}
	})

	t.Run("least_connections", func(t *testing.T) {
		release := make(chan struct{})
		a := newUpstreamStub(t, upstreamResponse{ContentType: "text/event-stream", Body: "data: {\"choices\":[]}\n\n", Hold: release})
		b := newUpstreamStub(t, upstreamResponse{Body: `{"id":"b","object":"chat.completion","choices":[]}`})
		srv := startRoutingServer(t, routingConfig(t, "least_connections", nil, a.URL(), b.URL()))
		resp := postChatStreaming(t, srv, `{"model":"alias/route","stream":true,"messages":[]}`)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("stream status = %d", resp.StatusCode)
		}
		a.WaitCalls(t, 1)
		if status, body := postChat(t, srv, "", `{"model":"alias/route","messages":[]}`); status != http.StatusOK {
			t.Fatalf("least-connections second request status = %d body=%s", status, body)
		}
		b.WaitCalls(t, 1)
		close(release)
		_, _ = io.Copy(io.Discard, resp.Body)
	})

	t.Run("transient_5xx_retry", func(t *testing.T) {
		a := newUpstreamStub(t, upstreamResponse{Status: http.StatusServiceUnavailable, Body: `{"error":"busy"}`})
		b := newUpstreamStub(t, upstreamResponse{Body: `{"id":"b","object":"chat.completion","choices":[]}`})
		srv := startRoutingServer(t, routingConfig(t, "round_robin", nil, a.URL(), b.URL()))
		if status, body := postChat(t, srv, "", `{"model":"alias/route","messages":[]}`); status != http.StatusOK {
			t.Fatalf("5xx retry status = %d body=%s", status, body)
		}
		a.WaitCalls(t, 1)
		b.WaitCalls(t, 1)
	})

	t.Run("configured_retryable_4xx", func(t *testing.T) {
		a := newUpstreamStub(t, upstreamResponse{Status: http.StatusTooManyRequests, Body: `{"error":"limited"}`})
		b := newUpstreamStub(t, upstreamResponse{Body: `{"id":"b","object":"chat.completion","choices":[]}`})
		srv := startRoutingServer(t, routingConfig(t, "round_robin", []int{429, 503}, a.URL(), b.URL()))
		if status, body := postChat(t, srv, "", `{"model":"alias/route","messages":[]}`); status != http.StatusOK {
			t.Fatalf("429 retry status = %d body=%s", status, body)
		}
		a.WaitCalls(t, 1)
		b.WaitCalls(t, 1)
	})

	t.Run("ordinary_4xx_not_retried", func(t *testing.T) {
		a := newUpstreamStub(t, upstreamResponse{Status: http.StatusBadRequest, Body: `{"error":"bad request"}`})
		b := newUpstreamStub(t, upstreamResponse{Body: `{"id":"b","object":"chat.completion","choices":[]}`})
		srv := startRoutingServer(t, routingConfig(t, "round_robin", []int{429, 503}, a.URL(), b.URL()))
		if status, _ := postChat(t, srv, "", `{"model":"alias/route","messages":[]}`); status != http.StatusBadRequest {
			t.Fatalf("ordinary 400 status = %d, want 400", status)
		}
		a.WaitCalls(t, 1)
		if got := len(b.Calls()); got != 0 {
			t.Fatalf("provider b calls = %d, want no retry", got)
		}
	})
}

func TestBinaryAliasCooldownExclusionAndSynthetic(t *testing.T) {
	a := newUpstreamStub(t,
		upstreamResponse{Status: http.StatusTooManyRequests, Body: `{"error":{"type":"upstream_rate_limited","message":"slow down"}}`, Headers: map[string]string{"retry-after-ms": "120000"}},
	)
	b := newUpstreamStub(t,
		upstreamResponse{Body: `{"id":"b-first","object":"chat.completion","choices":[]}`},
		upstreamResponse{Body: `{"id":"b-second","object":"chat.completion","choices":[]}`, Headers: map[string]string{"retry-after-ms": "120000"}},
	)
	srv := startRoutingServer(t, routingConfig(t, "round_robin", []int{429}, a.URL(), b.URL()))

	status, body := postChat(t, srv, "", `{"model":"alias/route","messages":[]}`)
	if status != http.StatusOK || !strings.Contains(body, "b-first") {
		t.Fatalf("request 1 status = %d body=%s, want 200 b-first", status, body)
	}
	if got := len(a.Calls()); got != 1 {
		t.Fatalf("provider a calls after request 1 = %d, want 1", got)
	}
	if got := len(b.Calls()); got != 1 {
		t.Fatalf("provider b calls after request 1 = %d, want 1", got)
	}

	status, body = postChat(t, srv, "", `{"model":"alias/route","messages":[]}`)
	if status != http.StatusOK || !strings.Contains(body, "b-second") {
		t.Fatalf("request 2 status = %d body=%s, want 200 b-second", status, body)
	}
	if got := len(a.Calls()); got != 1 {
		t.Fatalf("provider a calls after request 2 = %d, want still 1 (cooling exclusion)", got)
	}
	if got := len(b.Calls()); got != 2 {
		t.Fatalf("provider b calls after request 2 = %d, want 2", got)
	}

	aBefore, bBefore := len(a.Calls()), len(b.Calls())
	status, header, respBody := postChatFull(t, srv, "", `{"model":"alias/route","messages":[]}`)
	if status != http.StatusTooManyRequests {
		t.Fatalf("request 3 status = %d body=%s, want 429", status, respBody)
	}
	if got := len(a.Calls()); got != aBefore {
		t.Fatalf("provider a calls after request 3 = %d, want still %d (zero upstream calls)", got, aBefore)
	}
	if got := len(b.Calls()); got != bBefore {
		t.Fatalf("provider b calls after request 3 = %d, want still %d (zero upstream calls)", got, bBefore)
	}
	if got := header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	msText := header.Get("Retry-After-Ms")
	if msText == "" {
		t.Fatalf("missing retry-after-ms header")
	}
	secText := header.Get("Retry-After")
	if secText == "" {
		t.Fatalf("missing Retry-After header")
	}
	var msValue int64
	if _, err := fmt.Sscanf(msText, "%d", &msValue); err != nil || msValue < 1 {
		t.Fatalf("retry-after-ms = %q, want positive integer", msText)
	}
	var secValue int64
	if _, err := fmt.Sscanf(secText, "%d", &secValue); err != nil || secValue < 1 {
		t.Fatalf("Retry-After = %q, want positive integer", secText)
	}
	wantSec := (msValue + 999) / 1000
	if secValue != wantSec {
		t.Fatalf("Retry-After = %d, want ceil(%dms/1000) = %d", secValue, msValue, wantSec)
	}
	if !strings.Contains(respBody, `"type":"upstream_rate_limited"`) {
		t.Fatalf("body = %s, want upstream_rate_limited type", respBody)
	}
	if want := "retry after " + msText + "ms"; !strings.Contains(respBody, want) {
		t.Fatalf("body = %s, want %q", respBody, want)
	}
}

func startRoutingServer(t *testing.T, config string) *binaryServer {
	t.Helper()
	addr := freeAddr(t)
	config = strings.ReplaceAll(config, "${LISTENER}", addr)
	return startBinaryServer(t, writeConfig(t, config), addr)
}

func routingConfig(t *testing.T, algorithm string, retryCodes []int, aURL, bURL string) string {
	t.Helper()
	retry := ""
	if retryCodes != nil {
		parts := make([]string, 0, len(retryCodes))
		for _, code := range retryCodes {
			parts = append(parts, fmt.Sprintf("\"%d\"", code))
		}
		retry = "  retry_status_codes = [" + strings.Join(parts, ", ") + "]\n"
	}
	return fmt.Sprintf(`
listener "http" "public" { address = "${LISTENER}" }
auth "main" { mode = "none" }
provider "openai-compatible" "a" {
  base_url = %q
  api_key  = "sk-a"
  model "chat" {
    upstream_name = "a-chat"
  }
}
provider "openai-compatible" "b" {
  base_url = %q
  api_key  = "sk-b"
  model "chat" {
    upstream_name = "b-chat"
  }
}
alias "route" {
  algorithm = %q
%s  target {
    provider = "a"
    model = "chat"
  }
  target {
    provider = "b"
    model = "chat"
  }
}
`, aURL+"/v1", bURL+"/v1", algorithm, retry)
}

func lifecycleConfig(addr, upstreamURL, derivedKey string, includeDerivedTwo bool) string {
	derivedTwo := ""
	if includeDerivedTwo {
		derivedTwo = fmt.Sprintf(`
provider "openai-compatible" "derivedtwo" {
  extends = "base"
  api_key = %q
}
`, derivedKey)
	}
	return fmt.Sprintf(`
listener "http" "public" { address = %q }
auth "main" {
  mode = "bearer_static"
  client "ci" {
    token = "api-token"
  }
}
metrics { token = "metrics-token" }
provider "openai-compatible" "base" {
  base_url = %q
  api_key  = "sk-base"
  model "chat" {
    upstream_name = "upstream-chat"
  }
}
provider "openai-compatible" "derived" {
  extends = "base"
  api_key = "sk-derived"
}
%salias "stream" {
  algorithm = "round_robin"
  target {
    provider = "derived"
    model = "chat"
  }
}
`, addr, upstreamURL+"/v1", derivedTwo)
}

func failedReloadConfig(addr, upstreamURL string) string {
	return fmt.Sprintf(`
listener "http" "public" { address = %q }
auth "main" {
  mode = "bearer_static"
  client "ci" {
    token = "api-token"
  }
}
metrics { token = "metrics-token" }
provider "openai-compatible" "base" {
  base_url = %q
  api_key  = "sk-base"
  model "chat" {
    upstream_name = "upstream-chat"
  }
}
provider "openai-compatible" "candidate" {
  extends = "base"
  api_key = "sk-candidate"
}
alias "stream" {
  algorithm = "round_robin"
  target {
    provider = "candidate"
    model = "chat"
  }
}
`, strings.Replace(addr, ":", ":1", 1), upstreamURL+"/v1")
}

func postChat(t *testing.T, srv *binaryServer, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.baseURL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new chat request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srv.client.Do(req)
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read chat response: %v", err)
	}
	return resp.StatusCode, string(out)
}

func postChatFull(t *testing.T, srv *binaryServer, token, body string) (int, http.Header, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.baseURL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new chat request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srv.client.Do(req)
	if err != nil {
		t.Fatalf("post chat: %v", err)
	}
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read chat response: %v", err)
	}
	return resp.StatusCode, resp.Header, string(out)
}

func postChatStreaming(t *testing.T, srv *binaryServer, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.baseURL+"/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new streaming chat request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := srv.client.Do(req)
	if err != nil {
		t.Fatalf("post streaming chat: %v", err)
	}
	return resp
}

func httpGet(t *testing.T, srv *binaryServer, path, token string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.baseURL+path, nil)
	if err != nil {
		t.Fatalf("new get request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := srv.client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read GET %s response: %v", path, err)
	}
	return resp.StatusCode, string(body)
}

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.hcl")
	writeConfigPath(t, path, content)
	return path
}

func writeConfigPath(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for free address: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close free address listener: %v", err)
	}
	return addr
}

func aiproxyBinary(t *testing.T) string {
	t.Helper()
	if path := os.Getenv("AIPROXY_BINARY"); path != "" {
		return path
	}
	path := filepath.Join("..", "..", "dist", "aiproxy")
	if _, err := os.Stat(path); err == nil {
		return path
	}
	t.Fatalf("AIPROXY_BINARY is not set and %s does not exist", path)
	return ""
}
