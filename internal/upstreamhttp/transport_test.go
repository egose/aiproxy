package upstreamhttp

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/tlsprofile"
	utls "github.com/refraction-networking/utls"
)

type testHello struct {
	suites     []uint16
	alpn       []string
	versions   []uint16
	groups     []tls.CurveID
	signatures []tls.SignatureScheme
	raw        []byte
}

type helloLog struct {
	mu     sync.Mutex
	hellos []testHello
}

func (l *helloLog) snapshot() []testHello {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]testHello(nil), l.hellos...)
}

type helloListener struct{ net.Listener }

func (l helloListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return &helloConn{Conn: conn}, nil
}

type helloConn struct {
	net.Conn
	hello []byte
	done  bool
}

func (c *helloConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if !c.done && n > 0 {
		c.hello = append(c.hello, p[:min(n, tlsprofile.MaxClientHelloBytes+5-len(c.hello))]...)
		if len(c.hello) >= 5 && len(c.hello) >= 5+int(binary.BigEndian.Uint16(c.hello[3:5])) {
			c.hello = c.hello[:5+int(binary.BigEndian.Uint16(c.hello[3:5]))]
			c.done = true
		}
	}
	return n, err
}

func transportTestCertificate(t *testing.T, names ...string) (tls.Certificate, *x509.CertPool) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "aiproxy transport synthetic root"},
		DNSNames:              []string{"localhost", "example.test"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	if len(names) > 0 {
		template.DNSNames = names
		template.IPAddresses = nil
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
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key, Leaf: leaf}, roots
}

func transportTestServer(t *testing.T, cert tls.Certificate, version uint16, alpn string, handler http.Handler) (*httptest.Server, *helloLog, *atomic.Int32, *atomic.Int32) {
	t.Helper()
	hellos := &helloLog{}
	var accepts, closes atomic.Int32
	server := httptest.NewUnstartedServer(handler)
	server.Listener = helloListener{Listener: server.Listener}
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateNew {
			accepts.Add(1)
		}
		if state == http.StateClosed {
			closes.Add(1)
		}
	}
	server.EnableHTTP2 = alpn == "h2"
	cfg := &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: version, MaxVersion: version}
	if alpn != "" {
		cfg.NextProtos = []string{alpn, "http/1.1"}
	}
	cfg.GetConfigForClient = func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		captured := testHello{
			suites:     append([]uint16(nil), hello.CipherSuites...),
			alpn:       append([]string(nil), hello.SupportedProtos...),
			versions:   append([]uint16(nil), hello.SupportedVersions...),
			groups:     append([]tls.CurveID(nil), hello.SupportedCurves...),
			signatures: append([]tls.SignatureScheme(nil), hello.SignatureSchemes...),
		}
		if conn, ok := hello.Conn.(*helloConn); ok {
			captured.raw = append([]byte(nil), conn.hello...)
		}
		hellos.mu.Lock()
		hellos.hellos = append(hellos.hellos, captured)
		hellos.mu.Unlock()
		if alpn == "" {
			noALPN := cfg.Clone()
			noALPN.NextProtos = nil
			noALPN.GetConfigForClient = nil
			return noALPN, nil
		}
		return nil, nil
	}
	server.TLS = cfg
	server.StartTLS()
	t.Cleanup(server.Close)
	return server, hellos, &accepts, &closes
}

func transportTestProxy(t *testing.T, cert tls.Certificate, secure bool, strictHeaders ...bool) (*url.URL, *helloLog, *atomic.Int32) {
	t.Helper()
	strict := len(strictHeaders) == 0 || strictHeaders[0]
	return newTransportTestProxy(t, cert, secure, strict, "")
}

func newTransportTestProxy(t *testing.T, cert tls.Certificate, secure, strict bool, target string) (*url.URL, *helloLog, *atomic.Int32) {
	t.Helper()
	var tunnels atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodConnect {
			http.Error(w, "CONNECT only", http.StatusBadRequest)
			return
		}
		for name := range req.Header {
			if name != "Proxy-Authorization" && (strict || name != "User-Agent") {
				t.Errorf("unexpected CONNECT header %s", name)
			}
		}
		if req.Header.Get("Proxy-Authorization") != "Basic dXNlcjpwYXNz" {
			t.Error("CONNECT omitted proxy URL credentials")
		}
		host, _, err := net.SplitHostPort(req.Host)
		if err != nil || (host != "127.0.0.1" && host != "localhost" && (host != "example.test" || target == "")) {
			http.Error(w, "loopback only", http.StatusBadRequest)
			return
		}
		address := req.Host
		if host == "example.test" {
			address = target
		}
		upstream, err := net.DialTimeout("tcp", address, time.Second)
		if err != nil {
			http.Error(w, "dial failed", http.StatusBadGateway)
			return
		}
		client, buffered, err := w.(http.Hijacker).Hijack()
		if err != nil {
			_ = upstream.Close()
			return
		}
		tunnels.Add(1)
		_, err = buffered.WriteString("HTTP/1.1 200 Connection Established\r\n\r\n")
		if err == nil {
			err = buffered.Flush()
		}
		if err != nil {
			_ = upstream.Close()
			_ = client.Close()
			return
		}
		go func() {
			defer upstream.Close()
			defer client.Close()
			_, _ = io.Copy(upstream, buffered)
		}()
		go func() {
			defer upstream.Close()
			defer client.Close()
			_, _ = io.Copy(client, upstream)
		}()
	})
	var server *httptest.Server
	var hellos *helloLog
	if secure {
		server, hellos, _, _ = transportTestServer(t, cert, tls.VersionTLS13, "http/1.1", handler)
	} else {
		server = httptest.NewServer(handler)
		hellos = &helloLog{}
		t.Cleanup(server.Close)
	}
	proxy, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	proxy.User = url.UserPassword("user", "pass")
	return proxy, hellos, &tunnels
}

func transportTestProfile(t *testing.T, name string) tlsprofile.Profile {
	t.Helper()
	if !strings.HasPrefix(name, "custom") {
		profile, err := tlsprofile.ResolveBuiltin(name)
		if err != nil {
			t.Fatal(err)
		}
		return profile
	}
	data, err := os.ReadFile("../tlsprofile/testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	var recipe map[string]json.RawMessage
	if err := json.Unmarshal(data, &recipe); err != nil {
		t.Fatal(err)
	}
	var extensions []map[string]json.RawMessage
	if err := json.Unmarshal(recipe["extensions"], &extensions); err != nil {
		t.Fatal(err)
	}
	for i := range extensions {
		if string(extensions[i]["name"]) != `"application_layer_protocol_negotiation"` {
			continue
		}
		switch name {
		case "custom-h2":
			extensions[i]["protocol_name_list"] = json.RawMessage(`["h2","http/1.1"]`)
		case "custom-none":
			extensions = append(extensions[:i], extensions[i+1:]...)
		}
		break
	}
	recipe["extensions"], err = json.Marshal(extensions)
	if err != nil {
		t.Fatal(err)
	}
	data, err = json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	profile, err := tlsprofile.ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func transportTestClient(t *testing.T, profile tlsprofile.Profile, roots *x509.CertPool, proxy *url.URL, timeout time.Duration) (*http.Client, Transport) {
	t.Helper()
	transport, err := newTransport(Options{Profile: profile, ResponseHeaderTimeout: timeout}, transportDependencies{roots: roots, proxy: http.ProxyURL(proxy)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(transport.CloseIdleConnections)
	return &http.Client{Transport: transport, CheckRedirect: CheckRedirect}, transport
}

func normalizeGrease(values []uint16) []uint16 {
	normalized := append([]uint16(nil), values...)
	for i, value := range normalized {
		if value&0x0f0f == 0x0a0a && byte(value>>8) == byte(value) {
			normalized[i] = utls.GREASE_PLACEHOLDER
		}
	}
	return normalized
}

func requireProfileHello(t *testing.T, profile tlsprofile.Profile, captured testHello) {
	t.Helper()
	if len(captured.raw) < 9 || captured.raw[0] != 22 || captured.raw[5] != 1 {
		t.Fatal("missing on-wire ClientHello record")
	}
	if profile.IsNative() {
		return
	}
	spec, err := profile.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(normalizeGrease(captured.suites), spec.CipherSuites) {
		t.Fatalf("bundled cipher advertisements changed: %x != %x", captured.suites, spec.CipherSuites)
	}
	for _, extension := range spec.Extensions {
		switch extension := extension.(type) {
		case *utls.ALPNExtension:
			if !slices.Equal(captured.alpn, extension.AlpnProtocols) {
				t.Fatalf("ALPN advertisements changed: %v", captured.alpn)
			}
		case *utls.SupportedVersionsExtension:
			if !slices.Equal(normalizeGrease(captured.versions), extension.Versions) {
				t.Fatalf("version advertisements changed: %x", captured.versions)
			}
		case *utls.SignatureAlgorithmsExtension:
			want := make([]tls.SignatureScheme, len(extension.SupportedSignatureAlgorithms))
			for i, scheme := range extension.SupportedSignatureAlgorithms {
				want[i] = tls.SignatureScheme(scheme)
			}
			if !slices.Equal(captured.signatures, want) {
				t.Fatalf("signature advertisements changed: %x", captured.signatures)
			}
		}
	}
}

func TestProfileTransportRouteMatrix(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, name := range []string{"native", "chrome-133", "safari-16.0", "custom-h1", "custom-h2", "custom-none"} {
		profile := transportTestProfile(t, name)
		for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
			for _, alpn := range []string{"http/1.1", "h2", ""} {
				for _, route := range []string{"direct", "http-connect", "https-connect"} {
					t.Run(fmt.Sprintf("%s/%x/%s/%s", name, version, alpn, route), func(t *testing.T) {
						server, hellos, accepts, _ := transportTestServer(t, cert, version, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							if req.Header.Get("Proxy-Authorization") != "" {
								t.Error("proxy credentials leaked upstream")
							}
							if req.Header.Get("Authorization") != "Bearer synthetic-provider" || req.Header.Get("Cookie") != "synthetic=cookie" {
								t.Error("upstream headers were changed")
							}
							_, _ = io.WriteString(w, req.Proto)
						}))
						var proxy *url.URL
						var proxyHellos *helloLog
						if route != "direct" {
							proxy, proxyHellos, _ = transportTestProxy(t, cert, route == "https-connect", !profile.IsNative())
						}
						client, transport := transportTestClient(t, profile, roots, proxy, time.Second)
						wantProtocol := 1
						if alpn == "h2" && name != "custom-h1" && name != "custom-none" {
							wantProtocol = 2
						}
						for range 2 {
							req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
							req.Header.Set("Authorization", "Bearer synthetic-provider")
							req.Header.Set("Cookie", "synthetic=cookie")
							resp, err := client.Do(req)
							if err != nil {
								t.Fatal(err)
							}
							body, err := io.ReadAll(resp.Body)
							_ = resp.Body.Close()
							if err != nil || resp.ProtoMajor != wantProtocol || len(body) == 0 {
								t.Fatalf("ALPN dispatch: %s %s %v", resp.Proto, body, err)
							}
							if resp.TLS == nil || resp.TLS.Version != version || !resp.TLS.HandshakeComplete || len(resp.TLS.VerifiedChains) == 0 || resp.TLS.DidResume {
								t.Fatalf("unverified or resumed TLS: %+v", resp.TLS)
							}
						}
						if accepts.Load() != 1 {
							t.Fatalf("connection reuse: accepted %d", accepts.Load())
						}
						requireProfileHello(t, profile, hellos.snapshot()[0])
						if route == "https-connect" {
							captured := proxyHellos.snapshot()[0]
							if !profile.IsNative() && !slices.Equal(captured.alpn, []string{"http/1.1"}) {
								t.Fatalf("HTTPS proxy TLS did not use native h1: %v", captured.alpn)
							}
							if !profile.IsNative() && slices.Equal(captured.suites, hellos.snapshot()[0].suites) {
								t.Fatal("provider profile applied to proxy TLS")
							}
						}
						transport.CloseIdleConnections()
						req, _ := http.NewRequest(http.MethodGet, server.URL, nil)
						req.Header.Set("Authorization", "Bearer synthetic-provider")
						req.Header.Set("Cookie", "synthetic=cookie")
						resp, err := client.Do(req)
						if err != nil {
							t.Fatal(err)
						}
						_, _ = io.Copy(io.Discard, resp.Body)
						_ = resp.Body.Close()
						if accepts.Load() != 2 || resp.TLS.DidResume {
							t.Fatal("idle cleanup or disabled resumption failed")
						}
						captured := hellos.snapshot()
						if bytes.Equal(captured[0].raw[11:43], captured[1].raw[11:43]) {
							t.Fatal("ClientHello randomness reused across connections")
						}
					})
				}
			}
		}
	}
}

func TestNativeTransportDefaultsAndExactHeaderTimeout(t *testing.T) {
	for _, timeout := range []time.Duration{0, 125 * time.Millisecond, 90 * time.Second} {
		transport, err := NewTransport(Options{ResponseHeaderTimeout: timeout})
		if err != nil {
			t.Fatal(err)
		}
		defer transport.CloseIdleConnections()
		native, ok := transport.(*http.Transport)
		if !ok {
			t.Fatalf("native factory returned %T", transport)
		}
		defaults := http.DefaultTransport.(*http.Transport)
		if native.ResponseHeaderTimeout != timeout || native.TLSHandshakeTimeout != 10*time.Second || native.IdleConnTimeout != defaults.IdleConnTimeout || native.MaxIdleConns != defaults.MaxIdleConns || native.MaxIdleConnsPerHost != defaults.MaxIdleConnsPerHost || native.ExpectContinueTimeout != defaults.ExpectContinueTimeout || (native.TLSClientConfig == nil) != (defaults.TLSClientConfig == nil) || native.DialTLSContext != nil || native.Proxy == nil || !native.ForceAttemptHTTP2 {
			t.Fatalf("native defaults changed: %+v", native)
		}
		if native.TLSClientConfig != nil && (native.TLSClientConfig.InsecureSkipVerify || native.TLSClientConfig.RootCAs != nil || !slices.Equal(native.TLSClientConfig.NextProtos, defaults.TLSClientConfig.NextProtos)) {
			t.Fatal("native TLS configuration changed")
		}
	}
}

func TestProfilePlainHTTPStaysNative(t *testing.T) {
	for _, name := range []string{"chrome", "safari", "custom-h1"} {
		t.Run(name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				if req.TLS != nil || req.ProtoMajor != 1 {
					t.Error("HTTP was TLS-wrapped")
				}
				_, _ = io.WriteString(w, "plain")
			}))
			defer server.Close()
			client, transport := transportTestClient(t, transportTestProfile(t, name), nil, nil, 0)
			resp, err := client.Get(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			if resp.TLS != nil || len(transport.(*profileTransport).routes) != 1 {
				t.Fatal("plain HTTP used a profile TLS route")
			}
		})
	}
}

func TestProfileTimeoutStreamingCancellation(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, alpn := range []string{"http/1.1", "h2"} {
		for _, route := range []string{"direct", "http-connect", "https-connect"} {
			t.Run(alpn+"/"+route, func(t *testing.T) {
				server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					switch req.URL.Path {
					case "/slow":
						time.Sleep(250 * time.Millisecond)
						_, _ = io.WriteString(w, "late")
					case "/sse":
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: first\n\n")
						w.(http.Flusher).Flush()
						time.Sleep(180 * time.Millisecond)
						_, _ = io.WriteString(w, "data: second\n\n")
					case "/cancel":
						_, _ = io.WriteString(w, "first")
						w.(http.Flusher).Flush()
						<-req.Context().Done()
					}
				}))
				var proxy *url.URL
				if route != "direct" {
					proxy, _, _ = transportTestProxy(t, cert, route == "https-connect")
				}
				client, _ := transportTestClient(t, transportTestProfile(t, "chrome"), roots, proxy, 60*time.Millisecond)
				start := time.Now()
				resp, err := client.Get(server.URL + "/slow")
				if err == nil {
					_ = resp.Body.Close()
					t.Fatal("missing response header timeout")
				}
				var timeout net.Error
				if !errors.As(err, &timeout) || !timeout.Timeout() || time.Since(start) > time.Second {
					t.Fatalf("header timeout: %v", err)
				}
				resp, err = client.Get(server.URL + "/sse")
				if err != nil {
					t.Fatal(err)
				}
				client.CloseIdleConnections()
				body, err := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if err != nil || !strings.Contains(string(body), "second") {
					t.Fatalf("SSE did not outlive header deadline/idle cleanup: %s %v", body, err)
				}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/cancel", nil)
				resp, err = client.Do(req)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := io.ReadFull(resp.Body, make([]byte, 5)); err != nil {
					t.Fatal(err)
				}
				cancel()
				_, err = io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if !errors.Is(err, context.Canceled) {
					t.Fatalf("stream cancellation: %v", err)
				}
			})
		}
	}
}

func TestProfileHeaderTimeoutZeroDisables(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, alpn := range []string{"http/1.1", "h2"} {
		server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			time.Sleep(110 * time.Millisecond)
			_, _ = io.WriteString(w, "ok")
		}))
		client, _ := transportTestClient(t, transportTestProfile(t, "safari"), roots, nil, 0)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
		resp, err := client.Do(req)
		if err != nil {
			cancel()
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		cancel()
	}
}
