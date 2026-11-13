package app

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/tlsprofile"
	utls "github.com/refraction-networking/utls"
)

type appTLSHello struct {
	suites []uint16
	alpn   []string
	remote string
}

type appTLSRequest struct {
	hello appTLSHello
	key   string
	proto int
}

type appTLSObservations struct {
	mu       sync.Mutex
	hellos   map[string]appTLSHello
	requests []appTLSRequest
	closed   map[string]bool
}

func (o *appTLSObservations) snapshot() []appTLSRequest {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]appTLSRequest(nil), o.requests...)
}

func (o *appTLSObservations) connectionClosed(remote string) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.closed[remote]
}

func tlsAppProvider(baseURL, name, key, selector, source string) string {
	fields := ""
	if selector != "" {
		fields += fmt.Sprintf("  tls_client_hello = %q\n", selector)
	}
	if source != "" {
		fields += fmt.Sprintf("  tls_client_hello_spec_file = %q\n", source)
	}
	return fmt.Sprintf("provider \"openai-compatible\" %q {\n  base_url = %q\n  api_key = %q\n%s  model \"m\" {}\n}\n", name, baseURL, key, fields)
}

func tlsAppConfig(baseURL, selector, source string) string {
	return "listener \"http\" \"public\" { address = \"127.0.0.1:0\" }\nauth \"main\" { mode = \"none\" }\n" + tlsAppProvider(baseURL, "local", "synthetic-local", selector, source)
}

func tlsAppTestCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "aiproxy runtime synthetic root"},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
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
	rootFile := filepath.Join(t.TempDir(), "root.pem")
	if err := os.WriteFile(rootFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", rootFile)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func tlsAppServer(t *testing.T, cert tls.Certificate, handler http.Handler) (*httptest.Server, *appTLSObservations) {
	t.Helper()
	observations := &appTLSObservations{hellos: make(map[string]appTLSHello), closed: make(map[string]bool)}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		observations.mu.Lock()
		observations.requests = append(observations.requests, appTLSRequest{hello: observations.hellos[req.RemoteAddr], key: req.Header.Get("Authorization"), proto: req.ProtoMajor})
		observations.mu.Unlock()
		handler.ServeHTTP(w, req)
	}))
	server.EnableHTTP2 = true
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Config.ConnState = func(conn net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			observations.mu.Lock()
			observations.closed[conn.RemoteAddr().String()] = true
			observations.mu.Unlock()
		}
	}
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12, NextProtos: []string{"h2", "http/1.1"}, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		remote := hello.Conn.RemoteAddr().String()
		observations.mu.Lock()
		observations.hellos[remote] = appTLSHello{suites: append([]uint16(nil), hello.CipherSuites...), alpn: append([]string(nil), hello.SupportedProtos...), remote: remote}
		observations.mu.Unlock()
		return nil, nil
	}}
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, observations
}

func requireAppTLSProfile(t *testing.T, profile tlsprofile.Profile, observed appTLSRequest) {
	t.Helper()
	if len(observed.hello.suites) == 0 {
		t.Fatal("missing on-wire inference ClientHello")
	}
	if profile.IsNative() {
		return
	}
	spec, err := profile.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	suites := append([]uint16(nil), observed.hello.suites...)
	for i, value := range suites {
		if value&0x0f0f == 0x0a0a && byte(value>>8) == byte(value) {
			suites[i] = utls.GREASE_PLACEHOLDER
		}
	}
	if !slices.Equal(suites, spec.CipherSuites) {
		t.Fatalf("inference used wrong profile %s: %x != %x", profile.CanonicalName(), suites, spec.CipherSuites)
	}
	for _, extension := range spec.Extensions {
		if alpn, ok := extension.(*utls.ALPNExtension); ok && !slices.Equal(observed.hello.alpn, alpn.AlpnProtocols) {
			t.Fatalf("inference profile ALPN changed: %v != %v", observed.hello.alpn, alpn.AlpnProtocols)
		}
	}
}

func tlsAppBuild(t *testing.T, path string) *App {
	t.Helper()
	app, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = app.Close() })
	return app
}

func TestTLSProfileRuntimeInferenceAndReload(t *testing.T) {
	if os.Getenv("AIPROXY_TLS_APP_TEST_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestTLSProfileRuntimeInferenceAndReload$", "-test.timeout=45s")
		for _, value := range os.Environ() {
			if !strings.HasPrefix(value, "AIPROXY_TEST_DATABASE_URL=") && !strings.HasPrefix(value, "AIPROXY_TLS_APP_TEST_CHILD=") {
				cmd.Env = append(cmd.Env, value)
			}
		}
		cmd.Env = append(cmd.Env, "AIPROXY_TLS_APP_TEST_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("production system-root TLS runtime child: %v\n%s", err, output)
		}
		return
	}
	cert := tlsAppTestCertificate(t)
	t.Run("same-timeout-distinct-providers", func(t *testing.T) { tlsAppDistinctProviders(t, cert) })
	t.Run("same-path-custom-success-failure", func(t *testing.T) { tlsAppCustomReload(t, cert) })
	t.Run("alias-retry-after-reload", func(t *testing.T) { tlsAppAliasAcrossReload(t, cert) })
	t.Run("active-stream-through-repeated-reloads", func(t *testing.T) { tlsAppStreamAcrossReload(t, cert) })
}

func tlsAppDistinctProviders(t *testing.T, cert tls.Certificate) {
	server, observations := tlsAppServer(t, cert, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	source := filepath.Join(t.TempDir(), "custom.json")
	custom := appTestCustomTLSProfile(t, false)
	if err := os.WriteFile(source, custom.CanonicalJSON(), 0o600); err != nil {
		t.Fatal(err)
	}
	contents := "listener \"http\" \"public\" { address = \"127.0.0.1:0\" }\nauth \"main\" { mode = \"none\" }\n"
	for _, name := range []string{"native", "chrome", "safari", "custom"} {
		profileSource := ""
		if name == "custom" {
			profileSource = source
		}
		contents += tlsAppProvider(server.URL, name, "synthetic-"+name, name, profileSource)
	}
	app := tlsAppBuild(t, writeConfigFile(t, contents))
	clients := make(map[*http.Client]bool)
	for _, provider := range app.Config.Catalog.Providers() {
		client := app.handler.SnapshotDependencies().ClientForProvider(provider)
		if clients[client] || client.Timeout != 0 {
			t.Fatal("same timeout merged different TLS profiles or acquired a request timeout")
		}
		clients[client] = true
		if response := resourceChat(app.Server.Handler, provider.Name+"/m"); response.Code != http.StatusOK {
			t.Fatalf("profile inference: %d %s", response.Code, response.Body.String())
		}
		requests := observations.snapshot()
		requireAppTLSProfile(t, provider.TLSProfile, requests[len(requests)-1])
	}
	requests := observations.snapshot()
	if slices.Equal(requests[0].hello.suites, requests[1].hello.suites) || slices.Equal(requests[1].hello.suites, requests[2].hello.suites) {
		t.Fatal("native/browser provider ClientHellos were not distinct")
	}
}

func tlsAppCustomReload(t *testing.T, cert tls.Certificate) {
	server, observations := tlsAppServer(t, cert, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	source := filepath.Join(t.TempDir(), "custom.json")
	one, two := appTestCustomTLSProfile(t, false), appTestCustomTLSProfile(t, true)
	if err := os.WriteFile(source, one.CanonicalJSON(), 0o600); err != nil {
		t.Fatal(err)
	}
	path := writeConfigFile(t, tlsAppConfig(server.URL, "custom", source))
	app := tlsAppBuild(t, path)
	if response := resourceChat(app.Server.Handler, "local/m"); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	old := observations.snapshot()[0]
	requireAppTLSProfile(t, one, old)
	oldPool := app.clients
	rewriteConfigFile(t, source, string(two.CanonicalJSON()))
	if err := app.Reload(); err != nil {
		t.Fatal(err)
	}
	provider, _ := app.Config.Catalog.Provider("local")
	if app.clients == oldPool || provider.TLSProfile.Identity() == one.Identity() {
		t.Fatal("same-path content change reused an incompatible generation")
	}
	if response := resourceChat(app.Server.Handler, "local/m"); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	requests := observations.snapshot()
	requireAppTLSProfile(t, two, requests[len(requests)-1])
	if requests[len(requests)-1].proto != 2 || old.proto != 1 {
		t.Fatal("same-path custom reload did not change wire ALPN/HTTP selection")
	}
	awaitResourceCondition(t, "retired custom idle connection", func() bool { return observations.connectionClosed(old.hello.remote) })
	activeConfig, activePool := app.Config, app.clients
	activeClient := app.handler.SnapshotDependencies().ClientForProvider(provider)
	rewriteConfigFile(t, source, "{invalid replacement")
	if err := app.Reload(); err == nil {
		t.Fatal("malformed replacement activated")
	}
	if app.Config != activeConfig || app.clients != activePool || app.handler.SnapshotDependencies().ClientForProvider(provider) != activeClient {
		t.Fatal("failed custom reload replaced original bytes or clients")
	}
	app.clients.CloseIdleConnections()
	if response := resourceChat(app.Server.Handler, "local/m"); response.Code != http.StatusOK {
		t.Fatalf("runtime reopened changed custom file: %s", response.Body.String())
	}
	requests = observations.snapshot()
	requireAppTLSProfile(t, two, requests[len(requests)-1])
	for i := 0; i < 6; i++ {
		profile := one
		if i%2 != 0 {
			profile = two
		}
		rewriteConfigFile(t, source, string(profile.CanonicalJSON()))
		if err := app.Reload(); err != nil {
			t.Fatal(err)
		}
		if len(app.clients.clients) != 2 {
			t.Fatal("reload retained a historical transport map")
		}
		if response := resourceChat(app.Server.Handler, "local/m"); response.Code != http.StatusOK {
			t.Fatal(response.Body.String())
		}
		requests = observations.snapshot()
		requireAppTLSProfile(t, profile, requests[len(requests)-1])
	}
}

func tlsAppAliasAcrossReload(t *testing.T, cert tls.Certificate) {
	entered, release := make(chan struct{}), make(chan struct{})
	enterOnce := sync.OnceFunc(func() { close(entered) })
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()
	server, observations := tlsAppServer(t, cert, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Header.Get("Authorization") == "Bearer synthetic-primary-old" {
			enterOnce()
			select {
			case <-release:
				w.WriteHeader(http.StatusServiceUnavailable)
			case <-req.Context().Done():
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	contents := func(next bool) string {
		primaryProfile, backupProfile, suffix := "chrome", "safari", "old"
		if next {
			primaryProfile, backupProfile, suffix = "native", "chrome", "new"
		}
		return "listener \"http\" \"public\" { address = \"127.0.0.1:0\" }\nauth \"main\" { mode = \"none\" }\n" +
			tlsAppProvider(server.URL, "primary", "synthetic-primary-"+suffix, primaryProfile, "") +
			tlsAppProvider(server.URL, "backup", "synthetic-backup-"+suffix, backupProfile, "") + `
alias "chat" {
  algorithm = "round_robin"
  target {
    provider = "primary"
    model = "m"
  }
  target {
    provider = "backup"
    model = "m"
  }
}
`
	}
	path := writeConfigFile(t, contents(false))
	app := tlsAppBuild(t, path)
	finished := make(chan *httptest.ResponseRecorder, 1)
	go func() { finished <- resourceChat(app.Server.Handler, "alias/chat") }()
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("first alias attempt did not start")
	}
	rewriteConfigFile(t, path, contents(true))
	if err := app.Reload(); err != nil {
		t.Fatal(err)
	}
	if response := resourceChat(app.Server.Handler, "backup/m"); response.Code != http.StatusOK {
		t.Fatal(response.Body.String())
	}
	releaseOnce()
	select {
	case response := <-finished:
		if response.Code != http.StatusOK {
			t.Fatalf("admitted alias retry failed after reload: %d %s", response.Code, response.Body.String())
		}
	case <-time.After(3 * time.Second):
		t.Fatal("alias retry did not finish")
	}
	requests := observations.snapshot()
	if len(requests) != 3 || requests[0].key != "Bearer synthetic-primary-old" || requests[1].key != "Bearer synthetic-backup-new" || requests[2].key != "Bearer synthetic-backup-old" {
		t.Fatalf("alias crossed runtime credentials/catalog generation: %+v", requests)
	}
	chrome, _ := tlsprofile.ResolveBuiltin("chrome")
	safari, _ := tlsprofile.ResolveBuiltin("safari")
	requireAppTLSProfile(t, chrome, requests[0])
	requireAppTLSProfile(t, chrome, requests[1])
	requireAppTLSProfile(t, safari, requests[2])
	awaitResourceCondition(t, "retired alias retry socket", func() bool { return observations.connectionClosed(requests[2].hello.remote) })
}

func tlsAppStreamAcrossReload(t *testing.T, cert tls.Certificate) {
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()
	server, observations := tlsAppServer(t, cert, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var body struct {
			Stream bool `json:"stream"`
		}
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if !body.Stream {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"choices":[]}`)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[]}\n\n")
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
		case <-req.Context().Done():
		}
	}))
	one, two := appTestCustomTLSProfile(t, false), appTestCustomTLSProfile(t, true)
	source := filepath.Join(t.TempDir(), "stream.json")
	rewriteConfigFile(t, source, string(one.CanonicalJSON()))
	app := tlsAppBuild(t, writeConfigFile(t, tlsAppConfig(server.URL, "custom", source)))
	downstream := httptest.NewServer(app.Server.Handler)
	defer downstream.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, downstream.URL+"/v1/chat/completions", strings.NewReader(`{"model":"local/m","messages":[],"stream":true}`))
	resp, err := downstream.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	reader := bufio.NewReader(resp.Body)
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	if _, err := reader.ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	old := observations.snapshot()[0]
	for i := 0; i < 6; i++ {
		profile := two
		if i%2 != 0 {
			profile = one
		}
		rewriteConfigFile(t, source, string(profile.CanonicalJSON()))
		if err := app.Reload(); err != nil {
			t.Fatal(err)
		}
		if response := resourceChat(app.Server.Handler, "local/m"); response.Code != http.StatusOK || len(app.clients.clients) != 2 {
			t.Fatalf("new work after stream reload: %d %s", response.Code, response.Body.String())
		}
		requests := observations.snapshot()
		requireAppTLSProfile(t, profile, requests[len(requests)-1])
		if observations.connectionClosed(old.hello.remote) {
			t.Fatal("reload terminated an admitted stream")
		}
	}
	releaseOnce()
	body, err := io.ReadAll(reader)
	if err != nil || !strings.Contains(string(body), "[DONE]") {
		t.Fatalf("admitted SSE failed across reloads: %s %v", body, err)
	}
	_ = resp.Body.Close()
	awaitResourceCondition(t, "retired stream socket after EOF", func() bool { return observations.connectionClosed(old.hello.remote) })
}
