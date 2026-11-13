package upstreamhttp

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type proxySelectionBody struct {
	io.Reader
	closed atomic.Bool
}

func (b *proxySelectionBody) Close() error { b.closed.Store(true); return nil }

func TestProfileProxySelectionErrorsAndUnsupportedSchemesBeforeDial(t *testing.T) {
	selectionError := errors.New("proxy selection failed")
	for _, scheme := range []string{"socks5", "socks5h", "h2", "h2c", "unsupported", "selection-error"} {
		t.Run(scheme, func(t *testing.T) {
			var dials atomic.Int32
			transport, err := newTransport(Options{Profile: transportTestProfile(t, "chrome")}, transportDependencies{
				proxy: func(*http.Request) (*url.URL, error) {
					if scheme == "selection-error" {
						return nil, selectionError
					}
					return &url.URL{Scheme: scheme, Host: "127.0.0.1:1", User: url.UserPassword("private-user", "private-password")}, nil
				},
				dialContext: func(context.Context, string, string) (net.Conn, error) {
					dials.Add(1)
					return nil, errors.New("unexpected dial")
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer transport.CloseIdleConnections()
			body := &proxySelectionBody{Reader: strings.NewReader("request")}
			req, _ := http.NewRequest(http.MethodPost, "https://example.test/inference", body)
			_, err = transport.RoundTrip(req)
			if err == nil || dials.Load() != 0 || !body.closed.Load() || strings.Contains(err.Error(), "private-") {
				t.Fatalf("unsupported route dialed or leaked credentials: %v dials=%d closed=%t", err, dials.Load(), body.closed.Load())
			}
			if scheme == "selection-error" && !errors.Is(err, selectionError) {
				t.Fatal("proxy selection error lost")
			}
		})
	}
}

func TestProfileHTTPSProxyHTTP2ConnectRejected(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	proxy := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("HTTP/1 CONNECT reached an HTTP/2-only proxy")
	}))
	var protocols http.Protocols
	protocols.SetHTTP2(true)
	proxy.Config.Protocols = &protocols
	proxy.EnableHTTP2 = true
	proxy.TLS = &tls.Config{Certificates: []tls.Certificate{cert}, NextProtos: []string{"h2"}}
	proxy.StartTLS()
	defer proxy.Close()
	proxyURL, _ := url.Parse(proxy.URL)
	client, _ := transportTestClient(t, transportTestProfile(t, "chrome"), roots, proxyURL, time.Second)
	_, err := client.Get("https://localhost:1/")
	if err == nil || !strings.Contains(err.Error(), "HTTP/1.1 CONNECT") {
		t.Fatalf("HTTP/2-only proxy not explicitly rejected: %v", err)
	}
}

func TestConnectBufferedBytesAndHeaderBound(t *testing.T) {
	for _, scenario := range []string{"buffered", "oversized", "non-200", "h2", "malformed"} {
		t.Run(scenario, func(t *testing.T) {
			client, server := net.Pipe()
			defer client.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				defer server.Close()
				_, err := http.ReadRequest(bufio.NewReader(server))
				if err != nil {
					return
				}
				switch scenario {
				case "buffered":
					_, _ = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\n\r\nfirst")
					_, _ = server.Write(bytes.Repeat([]byte{'x'}, maxConnectHeaderBytes+1))
				case "oversized":
					_, _ = io.WriteString(server, "HTTP/1.1 200 Connection Established\r\nX-Large: ")
					_, _ = server.Write(bytes.Repeat([]byte{'x'}, maxConnectHeaderBytes))
					_, _ = io.WriteString(server, "\r\n\r\n")
				case "non-200":
					_, _ = io.WriteString(server, "HTTP/1.1 407 private-proxy-message\r\n\r\n")
				case "h2":
					_, _ = io.WriteString(server, "HTTP/2.0 200 OK\r\n\r\n")
				case "malformed":
					_, _ = io.WriteString(server, "invalid response\r\n\r\n")
				}
			}()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			conn, err := connectTunnel(ctx, client, proxyRoute{scheme: "http"}, "localhost:443")
			if scenario == "buffered" {
				if err != nil {
					t.Fatal(err)
				}
				body, err := io.ReadAll(conn)
				if err != nil || len(body) != len("first")+maxConnectHeaderBytes+1 || !bytes.HasPrefix(body, []byte("first")) {
					t.Fatalf("CONNECT lost buffered bytes or limited tunnel body: %d %v", len(body), err)
				}
			} else if err == nil || strings.Contains(err.Error(), "private-proxy-message") {
				t.Fatalf("invalid CONNECT accepted or echoed private status: %v", err)
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("CONNECT did not close failing socket")
			}
		})
	}
}

func TestProfileProxyRoutesFreezeCredentialsAndReuse(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	server, _, accepts, _ := transportTestServer(t, cert, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	proxy, _, tunnels := transportTestProxy(t, cert, false)
	var selectionMu sync.Mutex
	selected := proxy
	dialer := &net.Dialer{Timeout: time.Second}
	transport, err := newTransport(Options{Profile: transportTestProfile(t, "safari")}, transportDependencies{
		roots: roots,
		proxy: func(*http.Request) (*url.URL, error) {
			selectionMu.Lock()
			defer selectionMu.Unlock()
			return selected, nil
		},
		dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
			if address != server.Listener.Addr().String() && address != proxy.Host {
				return nil, errors.New("synthetic unavailable route")
			}
			return dialer.DialContext(ctx, network, address)
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport}
	get := func() {
		t.Helper()
		resp, err := client.Get(server.URL)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}
	get()
	selectionMu.Lock()
	selected = nil
	selectionMu.Unlock()
	get()
	selectionMu.Lock()
	selected = proxy
	selectionMu.Unlock()
	get()
	if accepts.Load() != 2 || tunnels.Load() != 1 {
		t.Fatalf("proxy/direct route pools were mixed: upstream=%d tunnels=%d", accepts.Load(), tunnels.Load())
	}
	profiled := transport.(*profileTransport)
	if len(profiled.routes) != 2 {
		t.Fatal("routes were not isolated")
	}
	selectionMu.Lock()
	proxy.User = url.UserPassword("changed-user", "changed-password")
	selectionMu.Unlock()
	for route := range profiled.routes {
		if strings.Contains(route.authorization, "changed") || (route.scheme != "" && route.authorization != "Basic dXNlcjpwYXNz") {
			t.Fatal("stored proxy route retained mutable URL credentials")
		}
	}
	for i := 0; i < maxProfiledProxyRoutes; i++ {
		selectionMu.Lock()
		selected = &url.URL{Scheme: "http", Host: "127.0.0.1:1", User: url.UserPassword("user", strings.Repeat("x", i+1))}
		selectionMu.Unlock()
		_, _ = client.Get(server.URL)
	}
	if len(profiled.routes) != maxProfiledProxyRoutes {
		t.Fatal("route registry was not bounded")
	}
	_, err = client.Get(server.URL)
	if err == nil || !strings.Contains(err.Error(), "route limit") {
		t.Fatalf("route limit did not fail closed: %v", err)
	}
}

func TestProfileProxyFromEnvironment(t *testing.T) {
	if scenario := os.Getenv("AIPROXY_TLS_PROXY_TEST"); scenario != "" {
		profileProxyEnvironmentChild(t, scenario)
		return
	}
	for _, scenario := range []string{"https-proxy", "no-proxy", "cgi-http", "socks"} {
		t.Run(scenario, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run=^TestProfileProxyFromEnvironment$", "-test.timeout=15s")
			for _, value := range os.Environ() {
				if !strings.HasPrefix(value, "AIPROXY_TEST_DATABASE_URL=") && !strings.HasPrefix(value, "AIPROXY_TLS_PROXY_TEST=") {
					cmd.Env = append(cmd.Env, value)
				}
			}
			cmd.Env = append(cmd.Env, "AIPROXY_TLS_PROXY_TEST="+scenario)
			output, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("environment proxy child: %v\n%s", err, output)
			}
		})
	}
}

func profileProxyEnvironmentChild(t *testing.T, scenario string) {
	cert, roots := transportTestCertificate(t)
	server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, "ok")
	}))
	proxy, _, tunnels := newTransportTestProxy(t, cert, true, true, server.Listener.Addr().String())
	for _, name := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy", "NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(name, "")
	}
	t.Setenv("HTTPS_PROXY", proxy.String())
	t.Setenv("HTTP_PROXY", proxy.String())
	if scenario == "no-proxy" {
		t.Setenv("NO_PROXY", "example.test")
	}
	if scenario == "cgi-http" {
		t.Setenv("REQUEST_METHOD", "GET")
	}
	if scenario == "socks" {
		t.Setenv("HTTPS_PROXY", "socks5://private-user:private-password@127.0.0.1:1")
	}
	upstreamURL, _ := url.Parse(server.URL)
	upstreamURL.Host = net.JoinHostPort("example.test", upstreamURL.Port())
	if scenario == "cgi-http" {
		upstreamURL.Scheme = "http"
	}
	var dials atomic.Int32
	dialer := &net.Dialer{Timeout: time.Second}
	transport, err := newTransport(Options{Profile: transportTestProfile(t, "chrome")}, transportDependencies{roots: roots, dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		dials.Add(1)
		if address == proxy.Host {
			return dialer.DialContext(ctx, network, address)
		}
		return dialer.DialContext(ctx, network, server.Listener.Addr().String())
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	resp, err := client.Get(upstreamURL.String())
	if scenario == "cgi-http" || scenario == "socks" {
		if err == nil || dials.Load() != 0 {
			t.Fatalf("proxy environment restriction did not fail before dialing: %v dials=%d", err, dials.Load())
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if scenario == "no-proxy" && tunnels.Load() != 0 {
		t.Fatal("NO_PROXY was ignored")
	}
	if scenario == "https-proxy" && tunnels.Load() != 1 {
		t.Fatal("HTTPS_PROXY was ignored")
	}
}
