//go:build integration && linux

package integration

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/tlsprofile"
	utls "github.com/refraction-networking/utls"
)

const binaryTLSProxyHost = "upstream.integration.test"

type binaryTLSCertificate struct {
	certificate tls.Certificate
	roots       *x509.CertPool
	rootFile    string
	rootDir     string
}

func newBinaryTLSCertificate(t *testing.T, names ...string) binaryTLSCertificate {
	t.Helper()
	if len(names) == 0 {
		names = []string{"localhost", binaryTLSProxyHost}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "aiproxy binary integration synthetic root"},
		DNSNames:              names,
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(leaf)
	file := filepath.Join(t.TempDir(), "synthetic-root.pem")
	writeBinaryTLSFile(t, file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	return binaryTLSCertificate{
		certificate: tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf},
		roots:       roots,
		rootFile:    file,
		rootDir:     t.TempDir(),
	}
}

func binaryTLSChildEnv(t *testing.T, trust binaryTLSCertificate, proxyURL string, overrides map[string]string) []string {
	t.Helper()
	removed := map[string]bool{
		"AIPROXY_CONFIG": true, "AIPROXY_TEST_DATABASE_URL": true,
		"HOME": true, "XDG_CONFIG_HOME": true, "SSL_CERT_FILE": true, "SSL_CERT_DIR": true,
		"HTTP_PROXY": true, "HTTPS_PROXY": true, "http_proxy": true, "https_proxy": true,
		"ALL_PROXY": true, "all_proxy": true, "NO_PROXY": true, "no_proxy": true,
	}
	values := make(map[string]string)
	for _, entry := range os.Environ() {
		name, value, ok := strings.Cut(entry, "=")
		if ok && !removed[name] {
			values[name] = value
		}
	}
	values["HOME"] = t.TempDir()
	values["XDG_CONFIG_HOME"] = t.TempDir()
	values["SSL_CERT_FILE"] = trust.rootFile
	values["SSL_CERT_DIR"] = trust.rootDir
	for _, name := range []string{"HTTP_PROXY", "HTTPS_PROXY", "http_proxy", "https_proxy"} {
		values[name] = proxyURL
	}
	values["NO_PROXY"] = "localhost,127.0.0.1,::1"
	values["no_proxy"] = values["NO_PROXY"]
	for name, value := range overrides {
		values[name] = value
	}
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	env := make([]string, 0, len(names))
	for _, name := range names {
		env = append(env, name+"="+values[name])
	}
	return env
}

func binaryTLSExecutable(t *testing.T) string {
	t.Helper()
	binary, err := filepath.Abs(aiproxyBinary(t))
	if err != nil {
		t.Fatal(err)
	}
	return binary
}

func runBinaryTLSCLI(t *testing.T, dir string, env []string, args ...string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binaryTLSExecutable(t), args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("aiproxy %s exceeded its deadline: %v\n%s", args[0], ctx.Err(), out)
	}
	return string(out), err
}

func startBinaryTLSServer(t *testing.T, configPath, addr, dir string, env []string) *binaryServer {
	t.Helper()
	cmd := exec.Command(binaryTLSExecutable(t), "serve", "--config", configPath)
	cmd.Dir, cmd.Env = dir, env
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		_ = stdout.Close()
		t.Fatal(err)
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	srv := &binaryServer{
		cmd: cmd, done: make(chan error, 1), logs: make(chan string, 200),
		baseURL: "http://" + addr, client: &http.Client{Transport: transport, Timeout: 10 * time.Second},
	}
	if err := cmd.Start(); err != nil {
		_ = stdout.Close()
		_ = stderr.Close()
		t.Fatal(err)
	}
	go scanLines(stdout, srv.logs)
	go scanLines(stderr, srv.logs)
	go func() { srv.done <- cmd.Wait() }()
	t.Cleanup(func() {
		srv.client.CloseIdleConnections()
		srv.Stop(t)
	})
	srv.WaitReady(t)
	return srv
}

func reloadBinaryTLS(t *testing.T, srv *binaryServer, success bool) {
	t.Helper()
	if err := srv.cmd.Process.Signal(syscall.SIGHUP); err != nil {
		t.Fatal(err)
	}
	message := "config reload failed"
	if success {
		message = "config reloaded"
	}
	srv.WaitLogContains(t, message)
}

func writeBinaryTLSFile(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func binaryTLSRecipe(t *testing.T, h2 bool) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "examples", "tls", "bun-1.4.4-fetch.json"))
	if err != nil {
		t.Fatal(err)
	}
	if h2 {
		data = bytes.Replace(data, []byte(`"http/1.1"`), []byte(`"h2", "http/1.1"`), 1)
	}
	if _, err := tlsprofile.ParseCustom(data); err != nil {
		t.Fatalf("synthetic recipe: %v", err)
	}
	return data
}

func binaryTLSProvider(name, baseURL, key, selector, source, extra string) string {
	fields := ""
	if selector != "" {
		fields += fmt.Sprintf("  tls_client_hello = %q\n", selector)
	}
	if source != "" {
		fields += fmt.Sprintf("  tls_client_hello_spec_file = %q\n", source)
	}
	return fmt.Sprintf(`provider "openai-compatible" %q {
  base_url = %q
  api_key = %q
  upstream_header_timeout = "2s"
%s%s  model "chat" {
    upstream_name = "upstream-chat"
  }
}
`, name, baseURL+"/v1", key, fields, extra)
}

func binaryTLSConfig(addr, providers string) string {
	return fmt.Sprintf("listener \"http\" \"public\" { address = %q }\nauth \"main\" { mode = \"none\" }\n%s", addr, providers)
}

func binaryTLSValidationConfig(t *testing.T, format, addr, baseURL string, fields, derived map[string]any) string {
	t.Helper()
	provider := map[string]any{
		"base_url": baseURL + "/v1", "api_key": "synthetic-provider",
		"model": map[string]any{"chat": map[string]any{}},
	}
	for name, value := range fields {
		provider[name] = value
	}
	providers := map[string]any{"local": provider}
	if derived != nil {
		child := map[string]any{"extends": "local", "api_key": "synthetic-derived"}
		for name, value := range derived {
			child[name] = value
		}
		providers["derived"] = child
	}
	if format == "json" {
		data, err := json.Marshal(map[string]any{
			"listener": map[string]any{"http": map[string]any{"public": map[string]any{"address": addr}}},
			"auth":     map[string]any{"main": map[string]any{"mode": "none"}},
			"provider": map[string]any{"openai-compatible": providers},
		})
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	var out strings.Builder
	out.WriteString(binaryTLSConfig(addr, ""))
	for _, name := range []string{"local", "derived"} {
		entry, ok := providers[name]
		if !ok {
			continue
		}
		fmt.Fprintf(&out, "provider \"openai-compatible\" %q {\n", name)
		values := entry.(map[string]any)
		keys := make([]string, 0, len(values))
		for key := range values {
			keys = append(keys, key)
		}
		slices.Sort(keys)
		for _, key := range keys {
			if key == "model" {
				out.WriteString("  model \"chat\" {}\n")
				continue
			}
			value, err := json.Marshal(values[key])
			if err != nil {
				t.Fatal(err)
			}
			fmt.Fprintf(&out, "  %s = %s\n", key, value)
		}
		out.WriteString("}\n")
	}
	return out.String()
}

type binaryTLSHello struct {
	remote     string
	raw        []byte
	summary    tlsprofile.ClientHelloSummary
	serverName string
	versions   []uint16
}

type binaryTLSCall struct {
	remote, method, path, authorization, proxyAuthorization, userAgent, body, alpn string
	protoMajor                                                                     int
	version                                                                        uint16
}

type binaryTLSObservations struct {
	mu     sync.Mutex
	hellos map[string]binaryTLSHello
	calls  []binaryTLSCall
	closed map[string]bool
}

func newBinaryTLSObservations() *binaryTLSObservations {
	return &binaryTLSObservations{hellos: make(map[string]binaryTLSHello), closed: make(map[string]bool)}
}

func (o *binaryTLSObservations) Hello(t *testing.T, remote string) binaryTLSHello {
	t.Helper()
	o.mu.Lock()
	defer o.mu.Unlock()
	hello, ok := o.hellos[remote]
	if !ok {
		t.Fatalf("missing initial ClientHello for %s", remote)
	}
	hello.raw = bytes.Clone(hello.raw)
	hello.versions = slices.Clone(hello.versions)
	return hello
}

func (o *binaryTLSObservations) Calls(path string) []binaryTLSCall {
	o.mu.Lock()
	defer o.mu.Unlock()
	var calls []binaryTLSCall
	for _, call := range o.calls {
		if path == "" || path == call.path {
			calls = append(calls, call)
		}
	}
	return calls
}

func (o *binaryTLSObservations) WaitCalls(t *testing.T, path string, count int) []binaryTLSCall {
	t.Helper()
	waitBinaryTLSCondition(t, fmt.Sprintf("%d upstream HTTP calls to %s", count, path), func() bool { return len(o.Calls(path)) >= count })
	return o.Calls(path)
}

func (o *binaryTLSObservations) Closed(remote string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.closed[remote]
}

func waitBinaryTLSCondition(t *testing.T, name string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", name)
}

type binaryTLSCaptureListener struct {
	net.Listener
	observed *binaryTLSObservations
}

func (l binaryTLSCaptureListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &binaryTLSCaptureConn{Conn: conn, observed: l.observed}, nil
}

type binaryTLSCaptureConn struct {
	net.Conn
	observed *binaryTLSObservations
	captured bool
	replay   *bytes.Reader
}

func (c *binaryTLSCaptureConn) Read(p []byte) (int, error) {
	if !c.captured {
		c.captured = true
		if err := c.Conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
			return 0, err
		}
		var consumed bytes.Buffer
		raw, err := tlsprofile.ReadClientHello(io.TeeReader(c.Conn, &consumed))
		if err != nil {
			return 0, err
		}
		if err := c.Conn.SetReadDeadline(time.Time{}); err != nil {
			return 0, err
		}
		summary, err := tlsprofile.SummarizeClientHello(raw)
		if err != nil {
			return 0, err
		}
		remote := c.RemoteAddr().String()
		c.observed.mu.Lock()
		c.observed.hellos[remote] = binaryTLSHello{remote: remote, raw: raw, summary: summary}
		c.observed.mu.Unlock()
		c.replay = bytes.NewReader(consumed.Bytes())
	}
	if c.replay.Len() > 0 {
		return c.replay.Read(p)
	}
	return c.Conn.Read(p)
}

type binaryTLSFixture struct {
	server   *httptest.Server
	observed *binaryTLSObservations
}

func newBinaryTLSFixture(t *testing.T, cert binaryTLSCertificate, version uint16, alpn string, handler http.Handler) *binaryTLSFixture {
	t.Helper()
	observed := newBinaryTLSObservations()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(io.LimitReader(r.Body, 64<<10))
		if err != nil {
			t.Errorf("read synthetic request: %v", err)
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(body))
		call := binaryTLSCall{
			remote: r.RemoteAddr, method: r.Method, path: r.URL.Path, body: string(body),
			authorization: r.Header.Get("Authorization"), proxyAuthorization: r.Header.Get("Proxy-Authorization"),
			userAgent: r.UserAgent(), protoMajor: r.ProtoMajor,
		}
		if r.TLS != nil {
			call.version, call.alpn = r.TLS.Version, r.TLS.NegotiatedProtocol
		}
		observed.mu.Lock()
		observed.calls = append(observed.calls, call)
		observed.mu.Unlock()
		handler.ServeHTTP(w, r)
	}))
	server.Listener = binaryTLSCaptureListener{Listener: server.Listener, observed: observed}
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Config.ReadHeaderTimeout = 3 * time.Second
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			observed.mu.Lock()
			observed.closed[conn.RemoteAddr().String()] = true
			observed.mu.Unlock()
		}
	}
	server.EnableHTTP2 = alpn == "h2"
	nextProtos := []string{"http/1.1"}
	if alpn == "h2" {
		nextProtos = []string{"h2", "http/1.1"}
	}
	server.TLS = &tls.Config{
		Certificates: []tls.Certificate{cert.certificate}, MinVersion: version, MaxVersion: version,
		NextProtos: nextProtos,
		GetConfigForClient: func(info *tls.ClientHelloInfo) (*tls.Config, error) {
			remote := info.Conn.RemoteAddr().String()
			observed.mu.Lock()
			hello := observed.hellos[remote]
			hello.serverName, hello.versions = info.ServerName, slices.Clone(info.SupportedVersions)
			observed.hellos[remote] = hello
			observed.mu.Unlock()
			return nil, nil
		},
	}
	server.StartTLS()
	t.Cleanup(server.Close)
	return &binaryTLSFixture{server: server, observed: observed}
}

func (f *binaryTLSFixture) URL() string {
	return strings.Replace(f.server.URL, "127.0.0.1", "localhost", 1)
}

func (f *binaryTLSFixture) ProxyOrigin() string {
	return strings.Replace(f.server.URL, "127.0.0.1", binaryTLSProxyHost, 1)
}

func binaryTLSNativeHello(t *testing.T, fixture *binaryTLSFixture, trust binaryTLSCertificate, host string) binaryTLSHello {
	t.Helper()
	conn, err := tls.DialWithDialer(&net.Dialer{Timeout: 3 * time.Second}, "tcp", fixture.server.Listener.Addr().String(), &tls.Config{
		RootCAs: trust.roots, ServerName: host, NextProtos: []string{"h2", "http/1.1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	remote := conn.LocalAddr().String()
	_ = conn.Close()
	return fixture.observed.Hello(t, remote)
}

func binaryTLSGrease(values []uint16) []uint16 {
	values = slices.Clone(values)
	for i, value := range values {
		if value&0x0f0f == 0x0a0a && byte(value>>8) == byte(value) {
			values[i] = utls.GREASE_PLACEHOLDER
		}
	}
	return values
}

func requireBinaryTLSProfile(t *testing.T, selector string, recipe []byte, hello binaryTLSHello, native *binaryTLSHello) {
	t.Helper()
	if len(hello.raw) < 9 || hello.raw[0] != 22 || hello.raw[5] != 1 || hello.serverName == "" {
		t.Fatal("missing actual initial ClientHello and runtime SNI")
	}
	if selector == "" || selector == "native" {
		if native != nil && (!reflect.DeepEqual(hello.summary, native.summary) || !slices.Equal(hello.versions, native.versions)) {
			t.Fatalf("native ClientHello differs from standard Go TLS:\n%+v\n%+v", hello.summary, native.summary)
		}
		return
	}
	var spec *utls.ClientHelloSpec
	var err error
	switch selector {
	case "chrome", "chrome-133":
		var preset utls.ClientHelloSpec
		preset, err = utls.UTLSIdToSpec(utls.HelloChrome_133)
		spec = &preset
	case "safari", "safari-16.0":
		var preset utls.ClientHelloSpec
		preset, err = utls.UTLSIdToSpec(utls.HelloSafari_16_0)
		spec = &preset
	case "custom":
		spec, err = tlsprofile.DecodeSpec(recipe)
	default:
		t.Fatalf("test has an unpinned selector %q", selector)
	}
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(binaryTLSGrease(hello.summary.CipherSuites), spec.CipherSuites) {
		t.Fatalf("%s cipher order differs from declared profile: %x != %x", selector, hello.summary.CipherSuites, spec.CipherSuites)
	}
	for _, extension := range spec.Extensions {
		switch ext := extension.(type) {
		case *utls.SupportedCurvesExtension:
			groups := make([]uint16, len(ext.Curves))
			for i, group := range ext.Curves {
				groups[i] = uint16(group)
			}
			if !slices.Equal(binaryTLSGrease(hello.summary.Groups), groups) {
				t.Fatalf("%s group order differs: %x != %x", selector, hello.summary.Groups, groups)
			}
		case *utls.SignatureAlgorithmsExtension:
			signatures := make([]uint16, len(ext.SupportedSignatureAlgorithms))
			for i, signature := range ext.SupportedSignatureAlgorithms {
				signatures[i] = uint16(signature)
			}
			if !slices.Equal(hello.summary.SignatureAlgorithms, signatures) {
				t.Fatalf("%s signature order differs", selector)
			}
		case *utls.ALPNExtension:
			if !slices.Equal(hello.summary.ALPN, ext.AlpnProtocols) {
				t.Fatalf("%s ALPN differs: %v != %v", selector, hello.summary.ALPN, ext.AlpnProtocols)
			}
		case *utls.SupportedVersionsExtension:
			if !slices.Equal(binaryTLSGrease(hello.versions), ext.Versions) {
				t.Fatalf("%s version advertisements differ: %x != %x", selector, hello.versions, ext.Versions)
			}
		}
	}
	if selector == "custom" {
		exported, err := tlsprofile.ExportClientHello(hello.raw)
		if err != nil {
			t.Fatal(err)
		}
		observed, err := tlsprofile.ParseCustom(exported)
		if err != nil {
			t.Fatal(err)
		}
		declared, err := tlsprofile.ParseCustom(recipe)
		if err != nil || !bytes.Equal(observed.CanonicalJSON(), declared.CanonicalJSON()) {
			t.Fatalf("custom ordered extension structure/payload differs: %v", err)
		}
		if !slices.Equal(hello.summary.ExtensionIDs, []uint16{0, 23, 65281, 10, 11, 35, 16, 5, 13, 18, 51, 45, 43}) ||
			!slices.Equal(hello.summary.KeyShares, []tlsprofile.ClientHelloKeyShare{{Group: 4588, Bytes: 1216}, {Group: 29, Bytes: 32}}) {
			t.Fatal("custom fixture extension/key-share order changed")
		}
		if hello.serverName == "localhost" && slices.Equal(hello.summary.ALPN, []string{"http/1.1"}) {
			fixture, err := os.ReadFile(filepath.Join("..", "tlsprofile", "testdata", "bun-1.4.4-fetch.clienthello.hex"))
			if err != nil {
				t.Fatal(err)
			}
			measured, err := hex.DecodeString(strings.Join(strings.Fields(string(fixture)), ""))
			if err != nil {
				t.Fatal(err)
			}
			normalized, err := tlsprofile.NormalizeClientHello(hello.raw)
			if err != nil || hello.summary.RecordBytes != 1500 || !bytes.Equal(normalized, measured) {
				t.Fatalf("binary custom first flight differs from measured Bun fixture after volatile-only normalization: %v", err)
			}
			if bytes.Equal(hello.raw, measured) {
				t.Fatal("binary replayed normalized fixture crypto instead of generating fresh values")
			}
		}
	}
}

type binaryTLSProxyCall struct {
	host    string
	headers http.Header
}

type binaryTLSProxy struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []binaryTLSProxyCall
	conns  map[net.Conn]bool
	closed bool
	copyWG sync.WaitGroup
}

func newBinaryTLSProxy(t *testing.T, targets map[string]string) *binaryTLSProxy {
	t.Helper()
	proxy := &binaryTLSProxy{conns: make(map[net.Conn]bool)}
	proxy.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		target, allowed := targets[r.Host]
		if r.Method != http.MethodConnect || !allowed {
			t.Errorf("unexpected proxy request (no external dial): %s %s", r.Method, r.Host)
			http.Error(w, "fixture target only", http.StatusForbidden)
			return
		}
		upstream, err := net.DialTimeout("tcp", target, time.Second)
		if err != nil {
			http.Error(w, "fixture dial failed", http.StatusBadGateway)
			return
		}
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		proxy.mu.Lock()
		if proxy.closed {
			proxy.mu.Unlock()
			_ = client.Close()
			_ = upstream.Close()
			return
		}
		proxy.calls = append(proxy.calls, binaryTLSProxyCall{host: r.Host, headers: r.Header.Clone()})
		proxy.conns[client], proxy.conns[upstream] = true, true
		proxy.copyWG.Add(2)
		proxy.mu.Unlock()
		finish := sync.OnceFunc(func() {
			_ = client.Close()
			_ = upstream.Close()
			proxy.mu.Lock()
			delete(proxy.conns, client)
			delete(proxy.conns, upstream)
			proxy.mu.Unlock()
		})
		_, err = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		if err == nil {
			err = buffered.Flush()
		}
		if err != nil {
			finish()
			proxy.copyWG.Done()
			proxy.copyWG.Done()
			return
		}
		go func() {
			defer proxy.copyWG.Done()
			defer finish()
			_, _ = io.Copy(upstream, buffered)
		}()
		go func() {
			defer proxy.copyWG.Done()
			defer finish()
			_, _ = io.Copy(client, upstream)
		}()
	}))
	t.Cleanup(func() {
		proxy.mu.Lock()
		proxy.closed = true
		for conn := range proxy.conns {
			_ = conn.Close()
		}
		proxy.mu.Unlock()
		proxy.server.Close()
		done := make(chan struct{})
		go func() { proxy.copyWG.Wait(); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("CONNECT fixture did not release its sockets/copy workers")
		}
	})
	return proxy
}

func (p *binaryTLSProxy) URL() string {
	u, _ := url.Parse(p.server.URL)
	u.User = url.UserPassword("synthetic-proxy", "synthetic-password")
	return u.String()
}

func (p *binaryTLSProxy) Calls() []binaryTLSProxyCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.calls)
}

func requireBinaryTLSConnect(t *testing.T, proxy *binaryTLSProxy, count int) {
	t.Helper()
	calls := proxy.Calls()
	if len(calls) != count {
		t.Fatalf("CONNECT count = %d, want %d", len(calls), count)
	}
	for _, call := range calls {
		if call.headers.Get("Proxy-Authorization") != "Basic c3ludGhldGljLXByb3h5OnN5bnRoZXRpYy1wYXNzd29yZA==" {
			t.Fatal("CONNECT did not use only synthetic proxy URL credentials")
		}
		for name := range call.headers {
			if name != "Proxy-Authorization" && name != "User-Agent" {
				t.Fatalf("provider/caller metadata leaked to CONNECT: %s", name)
			}
		}
	}
}
