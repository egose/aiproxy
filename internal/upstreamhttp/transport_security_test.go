package upstreamhttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

func TestProfileCertificateHostnameAndWeakNegotiationRejection(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, scenario := range []string{"untrusted", "hostname", "tls10", "tls11", "cbc"} {
		for _, route := range []string{"direct", "http-connect", "https-connect"} {
			t.Run(scenario+"/"+route, func(t *testing.T) {
				serverCert, clientRoots := cert, roots
				version := uint16(tls.VersionTLS13)
				switch scenario {
				case "untrusted":
					clientRoots = x509.NewCertPool()
				case "hostname":
					serverCert, clientRoots = transportTestCertificate(t, "wrong.test")
				case "tls10":
					version = tls.VersionTLS10
				case "tls11":
					version = tls.VersionTLS11
				case "cbc":
					version = tls.VersionTLS12
				}
				server, _, _, _ := transportTestServer(t, serverCert, version, "http/1.1", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					t.Error("unsafe TLS reached inference")
				}))
				if scenario == "cbc" {
					server.TLS.CipherSuites = []uint16{tls.TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA}
				}
				var proxy *url.URL
				if route != "direct" {
					proxy, _, _ = transportTestProxy(t, cert, route == "https-connect")
					if route == "https-connect" && scenario == "hostname" {
						clientRoots.AddCert(cert.Leaf)
					}
				}
				client, _ := transportTestClient(t, transportTestProfile(t, "safari"), clientRoots, proxy, time.Second)
				resp, err := client.Get(server.URL)
				if err == nil {
					_ = resp.Body.Close()
					t.Fatal("unsafe TLS accepted")
				}
				if scenario == "hostname" {
					var hostname x509.HostnameError
					if !errors.As(err, &hostname) {
						t.Fatalf("hostname verification did not reject: %v", err)
					}
				}
			})
		}
	}
}

func TestProfileHTTPSProxyCertificateVerification(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, "http/1.1", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("unverified proxy reached inference")
	}))
	for _, scenario := range []string{"untrusted", "hostname"} {
		t.Run(scenario, func(t *testing.T) {
			var proxyCert tls.Certificate
			var proxyRoots *x509.CertPool
			if scenario == "hostname" {
				proxyCert, proxyRoots = transportTestCertificate(t, "wrong.test")
				proxyRoots.AddCert(cert.Leaf)
			} else {
				proxyCert, _ = transportTestCertificate(t)
				proxyRoots = roots
			}
			proxy, _, tunnels := transportTestProxy(t, proxyCert, true)
			client, _ := transportTestClient(t, transportTestProfile(t, "chrome"), proxyRoots, proxy, time.Second)
			resp, err := client.Get(server.URL)
			if err == nil {
				_ = resp.Body.Close()
				t.Fatal("unverified HTTPS proxy accepted")
			}
			if tunnels.Load() != 0 {
				t.Fatal("CONNECT sent before proxy verification")
			}
		})
	}
}

func TestProfileSHA1CounterexampleRequiresReadOnlyGuard(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	var accepted atomic.Int32
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		accepted.Add(1)
		_, _ = io.WriteString(w, "ok")
	}))
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.Listener = utls.NewListener(server.Listener, &utls.Config{
		Certificates: []utls.Certificate{{Certificate: cert.Certificate, PrivateKey: cert.PrivateKey, SupportedSignatureAlgorithms: []utls.SignatureScheme{utls.ECDSAWithSHA1}}},
		MinVersion:   utls.VersionTLS12,
		MaxVersion:   utls.VersionTLS12,
		CipherSuites: []uint16{utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256},
		NextProtos:   []string{"http/1.1"},
	})
	server.Start()
	defer server.Close()
	profile := transportTestProfile(t, "safari")
	dialer := &net.Dialer{Timeout: time.Second}
	unsafe := http.DefaultTransport.(*http.Transport).Clone()
	unsafe.Proxy = nil
	unsafe.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dialer.DialContext(ctx, network, address)
		if err != nil {
			return nil, err
		}
		host, _, _ := net.SplitHostPort(address)
		cfg := &utls.Config{ServerName: host, RootCAs: roots, VerifyConnection: verifyProfileConnection}
		uconn := utls.UClient(conn, cfg, utls.HelloCustom)
		spec, err := profile.Materialize()
		if err == nil {
			err = uconn.ApplyPreset(spec)
		}
		cfg.MinVersion = utls.VersionTLS12
		if err == nil {
			err = connectionPhase(ctx, time.Second, conn, uconn.HandshakeContext)
		}
		if err != nil {
			_ = conn.Close()
			return nil, err
		}
		return &stateConn{UConn: uconn}, nil
	}
	unsafeClient := &http.Client{Transport: unsafe, Timeout: time.Second}
	defer unsafeClient.CloseIdleConnections()
	resp, err := unsafeClient.Get("https://" + server.Listener.Addr().String())
	if err != nil {
		t.Fatalf("trusted SHA-1 counterexample failed to reproduce: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	client, _ := transportTestClient(t, profile, roots, nil, time.Second)
	resp, err = client.Get("https://" + server.Listener.Addr().String())
	if err == nil {
		_ = resp.Body.Close()
		t.Fatal("production guard accepted trusted SHA-1")
	}
	if !strings.Contains(err.Error(), "signature") || accepted.Load() != 1 {
		t.Fatalf("SHA-1 was not rejected by negotiation guard: %v accepted=%d", err, accepted.Load())
	}
}

func TestProfileSelectedALPSResumptionAndALPNRejection(t *testing.T) {
	valid := utls.ConnectionState{Version: utls.VersionTLS13, CipherSuite: utls.TLS_AES_128_GCM_SHA256}
	if err := verifyProfileConnection(valid); err != nil {
		t.Fatal(err)
	}
	for _, payload := range [][]byte{{}, {1}} {
		state := valid
		state.PeerApplicationSettings = payload
		if err := verifyProfileConnection(state); err == nil || !strings.Contains(err.Error(), "ALPS") {
			t.Fatal("present ALPS settings were ignored")
		}
	}
	state := valid
	state.DidResume = true
	if err := verifyProfileConnection(state); err == nil {
		t.Fatal("resumed handshake accepted")
	}
	state = valid
	state.NegotiatedProtocol = "h3"
	if err := verifyProfileConnection(state); err == nil {
		t.Fatal("unsupported ALPN accepted")
	}
}

func TestProfileTLS13HelloRetryRequestGroups(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, name := range []string{"chrome", "safari", "custom-h1"} {
		for _, group := range []tls.CurveID{tls.X25519, tls.CurveP256, tls.CurveP384} {
			t.Run(name+"/"+tls.CipherSuiteName(uint16(group)), func(t *testing.T) {
				server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, "http/1.1", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					_, _ = io.WriteString(w, "ok")
				}))
				server.TLS.CurvePreferences = []tls.CurveID{group}
				client, _ := transportTestClient(t, transportTestProfile(t, name), roots, nil, time.Second)
				resp, err := client.Get(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
			})
		}
	}
}
