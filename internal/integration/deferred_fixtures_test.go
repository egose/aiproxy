//go:build integration && linux

package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/deferred"
)

type binaryDeferredPolicy struct {
	enabled   bool
	jobs      int
	running   int
	request   int64
	result    int64
	execution time.Duration
	control   time.Duration
}

func defaultBinaryDeferredPolicy() binaryDeferredPolicy {
	return binaryDeferredPolicy{
		enabled: true, jobs: 4, running: 2, request: 64 << 10, result: 256 << 10,
		execution: 3 * time.Second, control: time.Second,
	}
}

func binaryDeferredExecutorConfig(addr, upstream, token, key, model string, policy binaryDeferredPolicy) string {
	root, receiver := "", ""
	if policy.enabled {
		root = fmt.Sprintf(`
deferred {
  max_jobs = %d
  max_running = %d
  max_admissions = 2
  max_entries = 16
  max_memory_bytes = 33554432
  max_request_bytes = %d
  max_result_bytes = %d
  max_work_bytes = 4194304
  per_owner_jobs = %d
  per_owner_running = %d
  max_control_inflight = 4
  execution_timeout = %q
  retention = "10s"
  control_timeout = %q
  finalization_timeout = "500ms"
}
`, policy.jobs, policy.running, policy.request, policy.result, policy.jobs, policy.running, policy.execution.String(), policy.control.String())
		receiver = "  deferred_receiver {}\n"
	}
	return fmt.Sprintf(`
listener "http" "public" { address = %q }
auth "main" {
  mode = "bearer_static"
  client "peer" {
    token = %q
    allowed_models = ["p/m"]
  }
  client "other" {
    token = "other-token"
    allowed_models = ["p/m"]
  }
}
metrics { token = "metrics-token" }
%sprovider "openai-compatible" "p" {
  base_url = %q
  api_key = %q
  upstream_header_timeout = "15s"
  forward_user_agent = true
  forward_headers = ["x-inference"]
%s  model "m" { upstream_name = %q }
}
`, addr, token, root, upstream+"/v1", key, receiver, model)
}

func binaryDeferredCallerConfig(addr, peer, token string, optIn bool, control, wait time.Duration, backup string) string {
	block, extra := "", ""
	if optIn {
		block = fmt.Sprintf(`
    deferred_transport {
      control_timeout = %q
      max_wait = %q
      poll_interval = "50ms"
      max_control_calls = 256
      max_control_retries = 8
    }
`, control.String(), wait.String())
	}
	if backup != "" {
		extra = fmt.Sprintf(`
provider "openai-compatible" "backup" {
  base_url = %q
  api_key = "backup-key"
  model "m" { upstream_name = "backup-model" }
}
alias "mixed" {
  algorithm = "round_robin"
  retry_status_codes = ["400", "429", "500", "502", "503", "504"]
  target {
    provider = "peer"
    model = "m"
  }
  target {
    provider = "backup"
    model = "m"
  }
}
`, backup+"/v1")
	}
	return fmt.Sprintf(`
listener "http" "public" { address = %q }
auth "main" { mode = "none" }
metrics { token = "metrics-token" }
provider "openai-compatible" "peer" {
  base_url = %q
  api_key = %q
  forward_user_agent = true
  forward_headers = ["x-inference"]
  model "m" {
    upstream_name = "p/m"
%s  }
}
%s`, addr, peer+"/mount/v1", token, block, extra)
}

func startBinaryDeferred(t *testing.T, trust binaryTLSCertificate, path, addr string) *binaryServer {
	t.Helper()
	return startBinaryTLSServer(t, path, addr, t.TempDir(), binaryTLSChildEnv(t, trust, "", nil))
}

type binaryDeferredIdleConn struct {
	net.Conn
	window time.Duration
	cuts   *atomic.Int32
}

func (c *binaryDeferredIdleConn) Read(p []byte) (int, error) {
	if err := c.Conn.SetReadDeadline(time.Now().Add(c.window)); err != nil {
		return 0, err
	}
	n, err := c.Conn.Read(p)
	if timeout, ok := err.(net.Error); ok && timeout.Timeout() {
		c.cuts.Add(1)
	}
	return n, err
}

type binaryDeferredPeerCall struct {
	method, path, query string
	body                []byte
}

type binaryDeferredPeer struct {
	tls       *binaryTLSFixture
	client    *http.Client
	transport *http.Transport
	cuts      atomic.Int32
	mu        sync.Mutex
	calls     []binaryDeferredPeerCall
	before    func(http.ResponseWriter, *http.Request) bool
	modify    func(*http.Response) error
}

func newBinaryDeferredPeer(t *testing.T, trust binaryTLSCertificate, target, alpn string, idle time.Duration) *binaryDeferredPeer {
	t.Helper()
	origin, err := url.Parse(target)
	if err != nil {
		t.Fatal(err)
	}
	peer := &binaryDeferredPeer{}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy, transport.DisableKeepAlives, transport.ResponseHeaderTimeout = nil, true, 0
	dialer := &net.Dialer{Timeout: time.Second}
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address != origin.Host {
			return nil, fmt.Errorf("fixture refused non-local target %q", address)
		}
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		return &binaryDeferredIdleConn{Conn: conn, window: idle, cuts: &peer.cuts}, nil
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
			peer.mu.Lock()
			modify := peer.modify
			peer.mu.Unlock()
			if modify != nil {
				return modify(response)
			}
			return nil
		},
		FlushInterval: -1,
		ErrorLog:      log.New(io.Discard, "", 0),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, "response inactivity cutoff", http.StatusGatewayTimeout)
		},
	}
	t.Cleanup(transport.CloseIdleConnections)
	peer.tls = newBinaryTLSFixture(t, trust, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		deadline := time.Now().Add(3 * time.Second)
		controller := http.NewResponseController(w)
		if err := controller.SetReadDeadline(deadline); err != nil {
			t.Errorf("peer read deadline: %v", err)
			return
		}
		if err := controller.SetWriteDeadline(deadline); err != nil {
			t.Errorf("peer write deadline: %v", err)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		peer.mu.Lock()
		peer.calls = append(peer.calls, binaryDeferredPeerCall{r.Method, r.URL.Path, r.URL.RawQuery, bytes.Clone(body)})
		before := peer.before
		peer.mu.Unlock()
		if before == nil || !before(w, r) {
			proxy.ServeHTTP(w, r)
		}
	}))
	clientTransport := http.DefaultTransport.(*http.Transport).Clone()
	clientTransport.Proxy = nil
	clientTransport.ForceAttemptHTTP2 = true
	clientTransport.TLSClientConfig = &tls.Config{RootCAs: trust.roots}
	peer.client, peer.transport = &http.Client{Transport: clientTransport, Timeout: 3 * time.Second}, transport
	t.Cleanup(peer.client.CloseIdleConnections)
	t.Cleanup(peer.tls.server.CloseClientConnections)
	return peer
}

func (p *binaryDeferredPeer) URL() string { return p.tls.URL() }

func (p *binaryDeferredPeer) hooks(before func(http.ResponseWriter, *http.Request) bool, modify func(*http.Response) error) {
	p.mu.Lock()
	p.before, p.modify = before, modify
	p.mu.Unlock()
}

func (p *binaryDeferredPeer) Calls(suffix string) []binaryDeferredPeerCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []binaryDeferredPeerCall
	for _, call := range p.calls {
		if suffix == "" || strings.HasSuffix(call.path, suffix) {
			call.body = bytes.Clone(call.body)
			out = append(out, call)
		}
	}
	return out
}

func requireBinaryDeferredPeerProtocol(t *testing.T, peer *binaryDeferredPeer, alpn string, routes ...string) {
	t.Helper()
	for _, route := range routes {
		found := false
		for _, call := range peer.tls.observed.Calls("") {
			if !strings.Contains(call.path, "/mount"+deferred.Prefix) || !strings.HasSuffix(call.path, route) {
				continue
			}
			found = true
			major := 1
			if alpn == "h2" {
				major = 2
			}
			if call.protoMajor != major || call.alpn != alpn || call.version != tls.VersionTLS13 ||
				call.authorization != "Bearer peer-token" || call.proxyAuthorization != "" {
				t.Fatalf("peer control did not use verified TLS/%s and scoped bearer: %+v", alpn, call)
			}
		}
		if !found {
			t.Fatalf("missing actual TLS/%s peer control %s", alpn, route)
		}
	}
}

func binaryDeferredResponseBody(response *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(response.Body, deferred.MaxControlBodyBytes+1))
	_ = response.Body.Close()
	if err != nil {
		return nil, err
	}
	if len(data) > deferred.MaxControlBodyBytes {
		return nil, fmt.Errorf("fixture control response exceeded finite limit")
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return data, nil
}

func truncateBinaryDeferredResponse(response *http.Response, data []byte) {
	response.Body = io.NopCloser(bytes.NewReader(data[:len(data)/2]))
}

type binaryDeferredGappedBody struct {
	first, last *bytes.Reader
	ctx         context.Context
	gap         time.Duration
	waited      bool
	done        chan bool
	once        sync.Once
}

func (b *binaryDeferredGappedBody) Read(data []byte) (int, error) {
	if b.first.Len() > 0 {
		return b.first.Read(data)
	}
	if !b.waited {
		b.waited = true
		finished := binaryDeferredPause(b.ctx, b.gap)
		b.once.Do(func() { b.done <- !finished })
		if !finished {
			return 0, b.ctx.Err()
		}
	}
	return b.last.Read(data)
}

func (b *binaryDeferredGappedBody) Close() error { return nil }

type binaryDeferredReply struct {
	status int
	header http.Header
	body   []byte
	err    error
}

func binaryDeferredHTTP(ctx context.Context, client *http.Client, base, method, path, token, generation string, body []byte, control bool) binaryDeferredReply {
	request, err := http.NewRequestWithContext(ctx, method, base+path, bytes.NewReader(body))
	if err != nil {
		return binaryDeferredReply{err: err}
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "binary-deferred-client")
	request.Header.Set("X-Inference", "binary-inference")
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	if control {
		request.Header.Set("X-Aiproxy-Deferred-Version", "1")
		if method == http.MethodPost {
			request.Header.Set("Content-Type", deferred.MediaType)
		}
	} else {
		request.Header.Set("X-Aiproxy-Deferred-Version", "99")
	}
	if generation != "" {
		request.Header.Set("X-Aiproxy-Deferred-Generation", generation)
	}
	response, err := client.Do(request)
	if err != nil {
		return binaryDeferredReply{err: err}
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	return binaryDeferredReply{response.StatusCode, response.Header.Clone(), data, err}
}

func callBinaryDeferred(t *testing.T, peer *binaryDeferredPeer, method, path, token, generation string, body []byte) binaryDeferredReply {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	reply := binaryDeferredHTTP(ctx, peer.client, peer.URL()+"/mount", method, deferred.Prefix+path, token, generation, body, true)
	if reply.err != nil {
		t.Fatal(reply.err)
	}
	for name, expected := range map[string]string{
		"Content-Type": deferred.MediaType, "X-Aiproxy-Deferred-Version": "1",
		"Cache-Control": "private, no-store", "Pragma": "no-cache", "Vary": "Authorization",
	} {
		if reply.header.Get(name) != expected {
			t.Fatalf("%s: missing peer marker/cache header %s: %d %s", path, name, reply.status, reply.body)
		}
	}
	return reply
}

func binaryDeferredCapability(t *testing.T, peer *binaryDeferredPeer, token string) deferred.Capability {
	t.Helper()
	reply := callBinaryDeferred(t, peer, "GET", "/capabilities?model=p/m", token, "", nil)
	var capability deferred.Capability
	if reply.status != 200 || deferred.DecodeControl(reply.body, &capability) != nil {
		t.Fatalf("capability: %d %s", reply.status, reply.body)
	}
	return capability
}

func binaryDeferredSubmitRequest(t *testing.T, capability deferred.Capability, body []byte) deferred.SubmitRequest {
	t.Helper()
	key, err := deferred.NewAttemptKey()
	if err != nil {
		t.Fatal(err)
	}
	return deferred.SubmitRequest{
		Envelope: deferred.NewEnvelope(), Generation: capability.Generation, SubmitToken: capability.SubmitToken,
		AttemptKey: key, Headers: http.Header{"Content-Type": {"application/json"}, "User-Agent": {"binary-deferred-client"}, "X-Inference": {"binary-inference"}},
		Body: body,
	}
}

func binaryDeferredWire(t *testing.T, value any) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func submitBinaryDeferred(t *testing.T, peer *binaryDeferredPeer, token string, request deferred.SubmitRequest) deferred.Receipt {
	t.Helper()
	reply := callBinaryDeferred(t, peer, "POST", "/submit", token, "", binaryDeferredWire(t, request))
	var receipt deferred.Receipt
	if reply.status != 202 || deferred.DecodeControl(reply.body, &receipt) != nil {
		t.Fatalf("submit: %d %s", reply.status, reply.body)
	}
	return receipt
}

func awaitBinaryDeferred(t *testing.T, label string, timeout time.Duration, condition func() bool) {
	t.Helper()
	timer, tick := time.NewTimer(timeout), time.NewTicker(50*time.Millisecond)
	defer timer.Stop()
	defer tick.Stop()
	for {
		if condition() {
			return
		}
		select {
		case <-tick.C:
		case <-timer.C:
			t.Fatalf("timed out waiting for %s", label)
		}
	}
}

func awaitBinaryDeferredSignal(t *testing.T, label string, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", label)
	}
}

func finishBinaryDeferred(t *testing.T, peer *binaryDeferredPeer, token string, receipt deferred.Receipt) deferred.ResultManifest {
	t.Helper()
	var manifest deferred.ResultManifest
	awaitBinaryDeferred(t, "terminal binary result", 4*time.Second, func() bool {
		reply := callBinaryDeferred(t, peer, "GET", "/jobs/"+receipt.ID+"/result", token, receipt.Generation, nil)
		if reply.status == 202 {
			return false
		}
		if reply.status != 200 || deferred.DecodeControl(reply.body, &manifest) != nil {
			t.Fatalf("result: %d %s", reply.status, reply.body)
		}
		return true
	})
	return manifest
}

func readBinaryDeferred(t *testing.T, peer *binaryDeferredPeer, token string, receipt deferred.Receipt, cursor uint64) deferred.ReadFrame {
	t.Helper()
	reply := callBinaryDeferred(t, peer, "GET", "/jobs/"+receipt.ID+"/read?cursor="+strconv.FormatUint(cursor, 10), token, receipt.Generation, nil)
	var frame deferred.ReadFrame
	if reply.status != 200 || deferred.DecodeControl(reply.body, &frame) != nil || frame.Cursor != cursor {
		t.Fatalf("read: %d %s", reply.status, reply.body)
	}
	return frame
}

func requireBinaryDeferredError(t *testing.T, reply binaryDeferredReply, status int, code string) {
	t.Helper()
	var failure deferred.ControlError
	if reply.err != nil || reply.status != status || deferred.DecodeControl(reply.body, &failure) != nil || failure.Error.Type != code {
		t.Fatalf("control error: %d %s %v, want %d %s", reply.status, reply.body, reply.err, status, code)
	}
}

func requireBinaryDeferredManifest(t *testing.T, manifest deferred.ResultManifest, body []byte, outcome deferred.OutcomeKind) {
	t.Helper()
	hash := sha256.Sum256(body)
	if !manifest.State.Terminal() || manifest.Outcome.Kind != outcome || manifest.AvailableBytes != uint64(len(body)) ||
		manifest.BodySHA256 != hex.EncodeToString(hash[:]) {
		t.Fatalf("terminal body/outcome/hash changed: %+v", manifest)
	}
}

func binaryDeferredMetrics(t *testing.T, srv *binaryServer) map[string]float64 {
	t.Helper()
	status, body := httpGet(t, srv, "/metrics", "metrics-token")
	if status != 200 {
		t.Fatalf("metrics status = %d", status)
	}
	values := make(map[string]float64)
	for _, line := range strings.Split(body, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.HasPrefix(fields[0], "aiproxy_deferred_") {
			value, err := strconv.ParseFloat(fields[1], 64)
			if err != nil {
				t.Fatal(err)
			}
			values[fields[0]] = value
		}
	}
	for _, name := range []string{"aiproxy_deferred_enabled", `aiproxy_deferred_bytes{kind="reserved"}`,
		`aiproxy_deferred_ownership{kind="jobs"}`, `aiproxy_deferred_ownership{kind="controls"}`} {
		if _, exists := values[name]; !exists {
			t.Fatalf("binary did not expose its store ownership gauge %s", name)
		}
	}
	if reserved := values[`aiproxy_deferred_bytes{kind="reserved"}`]; reserved < 0 || reserved > 32<<20 {
		t.Fatalf("feature ownership exceeded configured memory budget: %v", reserved)
	}
	return values
}

func requireBinaryDeferredReleased(t *testing.T, srv *binaryServer) {
	t.Helper()
	awaitBinaryDeferred(t, "released binary job/source/finalization/control ownership", 3*time.Second, func() bool {
		values := binaryDeferredMetrics(t, srv)
		for _, kind := range []string{"jobs", "running", "admissions", "controls", "deletes", "finalization_credits", "finalizing"} {
			if values[`aiproxy_deferred_ownership{kind="`+kind+`"}`] != 0 {
				return false
			}
		}
		for _, kind := range []string{"input", "output", "work", "ingress", "retained"} {
			if values[`aiproxy_deferred_bytes{kind="`+kind+`"}`] != 0 {
				return false
			}
		}
		return true
	})
}

func requireBinaryDeferredAccounting(t *testing.T, srv *binaryServer, token string, count, total int64) {
	t.Helper()
	awaitBinaryDeferred(t, "exact binary completion accounting", 3*time.Second, func() bool {
		status, body := httpGet(t, srv, "/v1/billing/usage", token)
		var usage struct {
			Data []accounting.Summary `json:"data"`
		}
		if status != 200 || json.Unmarshal([]byte(body), &usage) != nil {
			t.Fatalf("usage: %d %s", status, body)
		}
		var gotCount, gotTotal int64
		for _, summary := range usage.Data {
			gotCount += summary.Count
			gotTotal += summary.TotalTokens
		}
		if gotCount > count || gotTotal > total {
			t.Fatalf("duplicate binary accounting: count=%d total=%d, want %d/%d", gotCount, gotTotal, count, total)
		}
		return gotCount == count && gotTotal == total
	})
}

func newBinaryDeferredModel(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(handler)
	server.Config.ReadHeaderTimeout = time.Second
	server.Config.ReadTimeout, server.Config.WriteTimeout = 15*time.Second, 15*time.Second
	server.Start()
	t.Cleanup(func() {
		server.CloseClientConnections()
		server.Close()
	})
	return server
}

func binaryDeferredPause(ctx context.Context, duration time.Duration) bool {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func binaryDeferredChatBody(model string, stream bool) []byte {
	return []byte(fmt.Sprintf(`{"model":%q,"stream":%t,"messages":[{"role":"user","content":"test"}],"vendor":{"unchanged":true}}`, model, stream))
}

func requireNoBinaryDeferredEnvelope(t *testing.T, reply binaryDeferredReply) {
	t.Helper()
	for name := range reply.header {
		lower := strings.ToLower(name)
		if lower == "x-aiproxy-deferred" || strings.HasPrefix(lower, "x-aiproxy-deferred-") {
			t.Fatalf("private response header leaked to ordinary client: %s", name)
		}
	}
	for _, marker := range []string{deferred.Protocol, `"submit_token"`, `"scope_digest"`, `"next_cursor"`, `"body_sha256"`} {
		if bytes.Contains(reply.body, []byte(marker)) {
			t.Fatalf("private envelope leaked to ordinary client: %s", marker)
		}
	}
}

func killBinaryDeferred(t *testing.T, srv *binaryServer) {
	t.Helper()
	srv.stop.Do(func() {
		if err := srv.cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-srv.done:
			if err == nil {
				t.Fatal("crashed binary unexpectedly exited successfully")
			}
		case <-time.After(5 * time.Second):
			t.Fatal("crashed binary was not reaped")
		}
	})
	srv.client.CloseIdleConnections()
}
