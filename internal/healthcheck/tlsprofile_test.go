package healthcheck

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
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
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/tlsprofile"
	"github.com/egose/aiproxy/internal/upstreamhttp"
	utls "github.com/refraction-networking/utls"
)

type healthHello struct {
	suites []uint16
	alpn   []string
	name   string
}

type healthHelloLog struct {
	mu     sync.Mutex
	hellos []healthHello
}

func (l *healthHelloLog) snapshot() []healthHello {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]healthHello(nil), l.hellos...)
}

func healthTLSCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "health synthetic root"},
		DNSNames: []string{"localhost"}, IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "root.pem")
	if err := os.WriteFile(file, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SSL_CERT_FILE", file)
	t.Setenv("SSL_CERT_DIR", t.TempDir())
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func healthTLSServer(t *testing.T, cert tls.Certificate, version uint16, alpn string, handler http.Handler) (*httptest.Server, *healthHelloLog, *atomic.Int32) {
	t.Helper()
	hellos := &healthHelloLog{}
	var closed atomic.Int32
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = alpn == "h2"
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			closed.Add(1)
		}
	}
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: version, MaxVersion: version, NextProtos: []string{alpn, "http/1.1"}, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		hellos.mu.Lock()
		hellos.hellos = append(hellos.hellos, healthHello{suites: append([]uint16(nil), hello.CipherSuites...), alpn: append([]string(nil), hello.SupportedProtos...), name: hello.ServerName})
		hellos.mu.Unlock()
		return nil, nil
	}}
	server.StartTLS()
	t.Cleanup(server.Close)
	server.URL = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	return server, hellos, &closed
}

func healthCustomProfile(t *testing.T, h2 bool) tlsprofile.Profile {
	t.Helper()
	data, err := os.ReadFile("../tlsprofile/testdata/bun-1.4.4-fetch.json")
	if err != nil {
		t.Fatal(err)
	}
	if h2 {
		data = []byte(strings.Replace(string(data), `["http/1.1"]`, `["h2","http/1.1"]`, 1))
	}
	profile, err := tlsprofile.ParseCustom(data)
	if err != nil {
		t.Fatal(err)
	}
	return profile
}

func requireHealthProfile(t *testing.T, profile tlsprofile.Profile, hello healthHello) {
	t.Helper()
	if hello.name != "localhost" || len(hello.suites) == 0 {
		t.Fatal("missing real target SNI/ClientHello")
	}
	if profile.IsNative() {
		return
	}
	spec, err := profile.Materialize()
	if err != nil {
		t.Fatal(err)
	}
	suites := append([]uint16(nil), hello.suites...)
	for i, value := range suites {
		if value&0x0f0f == 0x0a0a && byte(value>>8) == byte(value) {
			suites[i] = utls.GREASE_PLACEHOLDER
		}
	}
	if !slices.Equal(suites, spec.CipherSuites) {
		t.Fatalf("health used wrong profile %s", profile.CanonicalName())
	}
	for _, extension := range spec.Extensions {
		if alpn, ok := extension.(*utls.ALPNExtension); ok && !slices.Equal(hello.alpn, alpn.AlpnProtocols) {
			t.Fatal("health changed profile ALPN")
		}
	}
}

func TestHealthTLSProfilesAndLifecycle(t *testing.T) {
	if os.Getenv("AIPROXY_HEALTH_TLS_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestHealthTLSProfilesAndLifecycle$", "-test.timeout=40s")
		for _, variable := range os.Environ() {
			if !strings.HasPrefix(variable, "AIPROXY_TEST_DATABASE_URL=") && !strings.HasPrefix(variable, "AIPROXY_HEALTH_TLS_CHILD=") {
				cmd.Env = append(cmd.Env, variable)
			}
		}
		cmd.Env = append(cmd.Env, "AIPROXY_HEALTH_TLS_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("health TLS child: %v\n%s", err, output)
		}
		return
	}
	cert := healthTLSCertificate(t)
	for _, selector := range []string{"native", "chrome", "safari", "custom-h1", "custom-h2"} {
		profile, _ := tlsprofile.ResolveBuiltin(selector)
		declared := selector
		if strings.HasPrefix(selector, "custom") {
			profile = healthCustomProfile(t, selector == "custom-h2")
			declared = "custom"
		}
		for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
			for _, alpn := range []string{"http/1.1", "h2"} {
				for _, method := range []string{"GET", "HEAD"} {
					t.Run(fmt.Sprintf("%s/%x/%s/%s", selector, version, alpn, method), func(t *testing.T) {
						var protocol atomic.Int32
						server, hellos, closed := healthTLSServer(t, cert, version, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							protocol.Store(int32(req.ProtoMajor))
							if req.Method != method || req.URL.Path != "/absolute-health" || req.Header.Get("Authorization") != "Bearer synthetic-health" || req.Header.Get("User-Agent") != "health-test/1" || req.Header.Get("Accept") != "*/*" || req.Header.Get("Cookie") != "" || req.Header.Get("Proxy-Authorization") != "" {
								t.Error("health request/header contract changed")
							}
							_, _ = io.WriteString(w, "ready")
						}))
						hc := fastCheck(server.URL + "/absolute-health")
						hc.Interval = time.Hour
						hc.Method = method
						hc.SendAuthorization = true
						provider := testProvider("health", "http://127.0.0.1:1", hc)
						provider.APIKey, provider.UserAgent = "synthetic-health", "health-test/1"
						provider.TLSClientHello, provider.TLSProfile = declared, profile
						if declared == "custom" {
							provider.TLSClientHelloSpecFile = "already-removed-custom.json"
						}
						manager := New(nil, nil, "test")
						defer manager.Close()
						if err := manager.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil)); err != nil {
							t.Fatal(err)
						}
						if status := waitForStatus(t, manager, "health"); !status.Healthy {
							t.Fatalf("verified health TLS failed: %+v", status)
						}
						captured := hellos.snapshot()
						if len(captured) != 1 {
							t.Fatalf("unexpected health handshakes: %d", len(captured))
						}
						requireHealthProfile(t, profile, captured[0])
						want := int32(1)
						if alpn == "h2" && selector != "custom-h1" {
							want = 2
						}
						if protocol.Load() != want {
							t.Fatal("health did not follow negotiated ALPN")
						}
						manager.Close()
						waitForCondition(t, "health idle TLS retirement", func() bool { return closed.Load() == 1 })
					})
				}
			}
		}
	}
	for _, alpn := range []string{"http/1.1", "h2"} {
		for _, stage := range []string{"headers", "body", "close"} {
			t.Run(alpn+"/cancel-"+stage, func(t *testing.T) {
				entered, exited := make(chan struct{}), make(chan struct{})
				server, _, closed := healthTLSServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					if stage != "headers" {
						_, _ = io.WriteString(w, "partial")
						w.(http.Flusher).Flush()
					}
					close(entered)
					<-req.Context().Done()
					close(exited)
				}))
				hc := fastCheck("/health")
				hc.Interval = time.Hour
				hc.Timeout = 80 * time.Millisecond
				if stage == "close" {
					hc.Timeout = 2 * time.Second
				}
				provider := testProvider("health", server.URL, hc)
				provider.TLSClientHello = "chrome"
				manager := New(nil, nil, "test")
				defer manager.Close()
				if err := manager.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil)); err != nil {
					t.Fatal(err)
				}
				select {
				case <-entered:
				case <-time.After(3 * time.Second):
					t.Fatal("health request did not start")
				}
				if stage != "close" {
					if status := waitForStatus(t, manager, "health"); status.Healthy {
						t.Fatal("health operation timeout disappeared")
					}
				}
				start := time.Now()
				manager.Close()
				if time.Since(start) > time.Second {
					t.Fatal("health shutdown did not cancel/join its operation")
				}
				select {
				case <-exited:
				case <-time.After(time.Second):
					t.Fatal("health cancellation retained server work")
				}
				waitForCondition(t, "health canceled connection cleanup", func() bool { return closed.Load() == 1 })
			})
		}
	}
	for _, selector := range []string{"native", "chrome"} {
		t.Run("certificate-rejection/"+selector, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("unverified health request reached application")
			}))
			defer server.Close()
			provider := testProvider("health", server.URL, fastCheck("/health"))
			provider.Healthcheck.Interval = time.Hour
			provider.TLSClientHello = selector
			manager := New(nil, nil, "test")
			defer manager.Close()
			if err := manager.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil)); err != nil {
				t.Fatal(err)
			}
			if status := waitForStatus(t, manager, "health"); status.Healthy {
				t.Fatal("untrusted health certificate was accepted")
			}
		})
	}
}

func TestHealthProbeCanceledBeforeRequest(t *testing.T) {
	var calls atomic.Int32
	manager := New(nil, nil, "test")
	manager.factory = func(upstreamhttp.Options) (upstreamhttp.Transport, error) {
		calls.Add(1)
		return nil, errors.New("unexpected preparation")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := manager.probe(ctx, testProvider("health", "http://not-contacted.test", fastCheck("/health"))); !errors.Is(err, context.Canceled) || calls.Load() != 0 {
		t.Fatalf("canceled probe prepared or dispatched: %v calls=%d", err, calls.Load())
	}
}
