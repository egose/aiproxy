package main

import (
	"bytes"
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
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/tlsprofile"
	"github.com/egose/aiproxy/internal/upstreamhttp"
	utls "github.com/refraction-networking/utls"
)

type modelTLSHello struct {
	suites []uint16
	alpn   []string
	name   string
}

type modelTLSLog struct {
	mu     sync.Mutex
	hellos []modelTLSHello
	closed atomic.Int32
}

func (l *modelTLSLog) snapshot() []modelTLSHello {
	l.mu.Lock()
	defer l.mu.Unlock()
	return append([]modelTLSHello(nil), l.hellos...)
}

func modelTLSCertificate(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "discovery synthetic root"},
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

func modelTLSServer(t *testing.T, cert tls.Certificate, version uint16, alpn string, handler http.Handler) (*httptest.Server, *modelTLSLog) {
	t.Helper()
	observed := &modelTLSLog{}
	server := httptest.NewUnstartedServer(handler)
	server.EnableHTTP2 = alpn == "h2"
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Config.ConnState = func(_ net.Conn, state http.ConnState) {
		if state == http.StateClosed {
			observed.closed.Add(1)
		}
	}
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: version, MaxVersion: version, NextProtos: []string{alpn, "http/1.1"}, GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		observed.mu.Lock()
		observed.hellos = append(observed.hellos, modelTLSHello{suites: append([]uint16(nil), hello.CipherSuites...), alpn: append([]string(nil), hello.SupportedProtos...), name: hello.ServerName})
		observed.mu.Unlock()
		return nil, nil
	}}
	server.StartTLS()
	t.Cleanup(server.Close)
	server.URL = strings.Replace(server.URL, "127.0.0.1", "localhost", 1)
	return server, observed
}

func modelCustomTLSProfile(t *testing.T, h2 bool) tlsprofile.Profile {
	t.Helper()
	data, err := os.ReadFile("../../internal/tlsprofile/testdata/bun-1.4.4-fetch.json")
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

func requireDiscoveryTLSProfile(t *testing.T, profile tlsprofile.Profile, hello modelTLSHello) {
	t.Helper()
	if hello.name != "localhost" || len(hello.suites) == 0 {
		t.Fatal("discovery did not emit a target ClientHello/SNI")
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
		t.Fatalf("discovery used wrong profile %s", profile.CanonicalName())
	}
	for _, extension := range spec.Extensions {
		if alpn, ok := extension.(*utls.ALPNExtension); ok && !slices.Equal(hello.alpn, alpn.AlpnProtocols) {
			t.Fatal("discovery changed profile ALPN")
		}
	}
}

func waitModelTLSClosed(t *testing.T, observed *modelTLSLog) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if observed.closed.Load() >= int32(len(observed.snapshot())) && len(observed.snapshot()) > 0 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("discovery retained an operation-owned TLS connection")
}

func TestModelDiscoveryTLSProfilesAndLifecycle(t *testing.T) {
	if os.Getenv("AIPROXY_MODELS_TLS_CHILD") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestModelDiscoveryTLSProfilesAndLifecycle$", "-test.timeout=60s")
		for _, variable := range os.Environ() {
			if !strings.HasPrefix(variable, "AIPROXY_TEST_DATABASE_URL=") && !strings.HasPrefix(variable, "AIPROXY_MODELS_TLS_CHILD=") {
				cmd.Env = append(cmd.Env, variable)
			}
		}
		cmd.Env = append(cmd.Env, "AIPROXY_MODELS_TLS_CHILD=1")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("discovery TLS child: %v\n%s", err, output)
		}
		return
	}
	cert := modelTLSCertificate(t)
	for _, selector := range []string{"native", "chrome", "safari", "custom-h1", "custom-h2"} {
		profile, _ := tlsprofile.ResolveBuiltin(selector)
		declared := selector
		if strings.HasPrefix(selector, "custom") {
			declared = "custom"
			profile = modelCustomTLSProfile(t, selector == "custom-h2")
		}
		for _, version := range []uint16{tls.VersionTLS12, tls.VersionTLS13} {
			for _, alpn := range []string{"http/1.1", "h2"} {
				for _, kind := range []config.ProviderType{config.ProviderTypeOpenAICompatible, config.ProviderTypeAnthropic, config.ProviderTypeGemini, config.ProviderTypeOpenCodeZen, config.ProviderTypeGitHubCopilot} {
					t.Run(fmt.Sprintf("%s/%x/%s/%s", selector, version, alpn, kind), func(t *testing.T) {
						var calls atomic.Int32
						server, observed := modelTLSServer(t, cert, version, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
							page := calls.Add(1)
							if req.Method != "GET" || req.UserAgent() != "models-test/1" || req.Header.Get("Cookie") != "" || req.Header.Get("Proxy-Authorization") != "" {
								t.Error("discovery request/header contract changed")
							}
							credentialHeader, credential := "Authorization", "Bearer synthetic"
							if kind == config.ProviderTypeAnthropic {
								credentialHeader, credential = "x-api-key", "synthetic"
								if req.Header.Get("anthropic-version") != "2023-06-01" {
									t.Error("Anthropic version lost on a page")
								}
							} else if kind == config.ProviderTypeGemini {
								credentialHeader, credential = "x-goog-api-key", "synthetic"
								if req.URL.Path != "/v1beta/models" {
									t.Error("Gemini path changed")
								}
							} else if kind == config.ProviderTypeOpenCodeZen && !strings.HasPrefix(req.Header.Get("x-opencode-session"), "ses_") {
								t.Error("OpenCode discovery session omitted")
							} else if kind == config.ProviderTypeGitHubCopilot && (req.URL.Path != "/models" || req.Header.Get(copilotlogin.APIVersionHeader) != copilotlogin.APIVersion) {
								t.Error("Copilot discovery header/path changed")
							}
							if req.Header.Get(credentialHeader) != credential || ((kind == config.ProviderTypeAnthropic || kind == config.ProviderTypeGemini) && req.Header.Get("Authorization") != "") {
								t.Error("discovery changed provider credential allowlist")
							}
							wantProtocol := 1
							if alpn == "h2" && selector != "custom-h1" {
								wantProtocol = 2
							}
							if req.ProtoMajor != wantProtocol {
								t.Error("discovery did not follow negotiated ALPN")
							}
							next := ""
							if (kind == config.ProviderTypeAnthropic || kind == config.ProviderTypeGemini) && page < 3 {
								next = fmt.Sprintf("next-%d", page)
							}
							_, _ = io.WriteString(w, discoveryPage(kind, 1, next))
						}))
						provider := discoveryProvider(kind, server.URL)
						provider.UserAgent = "models-test/1"
						provider.TLSClientHello, provider.TLSProfile = declared, profile
						if declared == "custom" {
							provider.TLSClientHelloSpecFile = "already-removed-custom.json"
						}
						models, err := listUpstreamModels(context.Background(), provider)
						pages := 1
						if kind == config.ProviderTypeAnthropic || kind == config.ProviderTypeGemini {
							pages = 3
						}
						if err != nil || len(models) != pages || int(calls.Load()) != pages {
							t.Fatalf("discovery TLS/pagination: models=%d calls=%d err=%v", len(models), calls.Load(), err)
						}
						hellos := observed.snapshot()
						if len(hellos) != 1 {
							t.Fatal("discovery pages did not share one operation-owned connection")
						}
						requireDiscoveryTLSProfile(t, profile, hellos[0])
						waitModelTLSClosed(t, observed)
					})
				}
			}
		}
	}
	for _, alpn := range []string{"http/1.1", "h2"} {
		for _, stage := range []string{"status", "decode", "truncated-body", "headers-timeout", "body-timeout", "cancellation"} {
			t.Run(alpn+"/"+stage, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				server, observed := modelTLSServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					switch stage {
					case "status":
						http.Error(w, "unavailable", 503)
					case "decode":
						_, _ = io.WriteString(w, "invalid JSON")
					case "truncated-body":
						w.Header().Set("Content-Length", "100")
						_, _ = io.WriteString(w, `{"data":[]}`)
					case "headers-timeout":
						<-req.Context().Done()
					default:
						_, _ = io.WriteString(w, `{"data":[`)
						w.(http.Flusher).Flush()
						if stage == "cancellation" {
							cancel()
						}
						<-req.Context().Done()
					}
				}))
				provider := discoveryProvider(config.ProviderTypeOpenAICompatible, server.URL)
				provider.TLSClientHello = "chrome"
				provider.UpstreamHeaderTimeout = 80 * time.Millisecond
				models, err := listUpstreamModels(ctx, provider)
				if err == nil || models != nil {
					t.Fatal("incomplete discovery returned models")
				}
				if strings.Contains(stage, "timeout") && !errors.Is(err, context.DeadlineExceeded) {
					t.Fatalf("whole-client timeout disappeared: %v", err)
				}
				if stage == "cancellation" && !errors.Is(err, context.Canceled) {
					t.Fatalf("discovery cancellation lost: %v", err)
				}
				waitModelTLSClosed(t, observed)
			})
		}
	}
	t.Run("configured-custom-and-cached-replay", func(t *testing.T) {
		server, observed := modelTLSServer(t, cert, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, _ = io.WriteString(w, `{"data":[{"id":"m"}]}`)
		}))
		profile := modelCustomTLSProfile(t, true)
		source := filepath.Join(t.TempDir(), "custom.json")
		if err := os.WriteFile(source, profile.CanonicalJSON(), 0o600); err != nil {
			t.Fatal(err)
		}
		path := writeModelsConfig(t, fmt.Sprintf(`
listener "http" "public" { address = "127.0.0.1:0" }
auth "main" { mode = "none" }
provider "openai-compatible" "local" {
  base_url = %q
  api_key = "synthetic"
  tls_client_hello = "custom"
  tls_client_hello_spec_file = %q
  model "m" {}
}
`, server.URL, source))
		var stdout, stderr bytes.Buffer
		if err := runModels(context.Background(), path, true, "local", true, &stdout, &stderr); err != nil || !strings.Contains(stdout.String(), "configured as local/m") {
			t.Fatalf("configured custom discovery: %v %s", err, stderr.String())
		}
		runtime, err := config.LoadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		provider, _ := runtime.Catalog.Provider("local")
		if err := os.Remove(source); err != nil {
			t.Fatal(err)
		}
		if models, err := listUpstreamModels(context.Background(), provider); err != nil || len(models) != 1 {
			t.Fatalf("discovery reopened its cached custom source: %v", err)
		}
		for _, hello := range observed.snapshot() {
			requireDiscoveryTLSProfile(t, profile, hello)
		}
		waitModelTLSClosed(t, observed)
	})
	for _, selector := range []string{"native", "chrome"} {
		t.Run("certificate-rejection/"+selector, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("unverified discovery reached application")
			}))
			defer server.Close()
			provider := discoveryProvider(config.ProviderTypeOpenAICompatible, server.URL)
			provider.TLSClientHello = selector
			if models, err := listUpstreamModels(context.Background(), provider); err == nil || models != nil {
				t.Fatal("untrusted discovery certificate was accepted")
			}
		})
	}
}

type modelOwnedTransport struct {
	closed atomic.Int32
	called atomic.Int32
}

func (t *modelOwnedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.called.Add(1)
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"data":[]}`))}, nil
}

func (t *modelOwnedTransport) CloseIdleConnections() { t.closed.Add(1) }

func TestUpstreamModelClientTimeoutAndPreparationFailures(t *testing.T) {
	for _, timeout := range []time.Duration{0, 45 * time.Second} {
		transport := &modelOwnedTransport{}
		client, retiring, err := upstreamHTTPClient(config.Provider{TLSClientHello: "chrome", UpstreamHeaderTimeout: timeout}, func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
			if options.Profile.CanonicalName() != "chrome-133" || options.ResponseHeaderTimeout != 0 {
				t.Error("discovery factory lost its profile or exact header timeout")
			}
			return transport, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		want := timeout
		if want == 0 {
			want = 30 * time.Second
		}
		if client.Timeout != want || client.CheckRedirect == nil {
			t.Fatal("discovery whole-client timeout/redirect policy changed")
		}
		retiring.Retire()
		if transport.closed.Load() == 0 {
			t.Fatal("operation-owned client was not retired")
		}
	}
	failure := errors.New("synthetic preparation failure")
	partial := &modelOwnedTransport{}
	if _, err := listUpstreamModelsWithTransportFactory(context.Background(), discoveryProvider(config.ProviderTypeOpenAI, "http://not-contacted.test"), func(upstreamhttp.Options) (upstreamhttp.Transport, error) { return partial, failure }); !errors.Is(err, failure) || partial.closed.Load() != 1 || partial.called.Load() != 0 {
		t.Fatal("partial discovery preparation failure leaked or dispatched")
	}
	if _, err := listUpstreamModelsWithTransportFactory(context.Background(), discoveryProvider(config.ProviderTypeOpenAI, "http://not-contacted.test"), func(upstreamhttp.Options) (upstreamhttp.Transport, error) { return nil, nil }); err == nil {
		t.Fatal("empty discovery transport was accepted")
	}
	var calls atomic.Int32
	provider := discoveryProvider(config.ProviderTypeOpenAI, "http://not-contacted.test")
	provider.TLSClientHello = "unsupported"
	if _, err := listUpstreamModelsWithTransportFactory(context.Background(), provider, func(upstreamhttp.Options) (upstreamhttp.Transport, error) { calls.Add(1); return nil, nil }); err == nil || calls.Load() != 0 {
		t.Fatal("invalid discovery selector fell back or dispatched")
	}
}
