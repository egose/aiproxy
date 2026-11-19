package upstreamhttp

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/upstreampolicy"
)

func TestPublicTransportVerifiedTLSAndSameOriginRedirects(t *testing.T) {
	for _, selector := range []string{"native", "chrome", "safari"} {
		for _, alpn := range []string{"http/1.1", "h2"} {
			t.Run(selector+"/"+alpn, func(t *testing.T) {
				cert, roots := transportTestCertificate(t, "api.example.com")
				var calls, lookups, dials, proxyCalls atomic.Int32
				server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls.Add(1)
					if r.Host != "api.example.com" || r.TLS == nil || r.TLS.ServerName != "api.example.com" ||
						r.Header.Get("Authorization") != "Bearer own-synthetic-secret" {
						t.Error("original hostname/SNI or own credential was not retained")
					}
					switch r.URL.Path {
					case "/redirect":
						http.Redirect(w, r, "/v1/models", http.StatusTemporaryRedirect)
					case "/foreign":
						http.Redirect(w, r, "https://probe.example.com/ready", http.StatusTemporaryRedirect)
					default:
						_, _ = io.WriteString(w, "ok")
					}
				}))
				t.Setenv("HTTPS_PROXY", "socks5://127.0.0.1:1")
				t.Setenv("HTTP_PROXY", "http://127.0.0.1:1")
				policy, err := upstreampolicy.New([]string{"https://api.example.com", "https://probe.example.com"})
				if err != nil {
					t.Fatal(err)
				}
				transport, err := newTransport(Options{Profile: transportTestProfile(t, selector), OutboundPolicy: policy}, transportDependencies{
					roots: roots,
					proxy: func(*http.Request) (*url.URL, error) {
						proxyCalls.Add(1)
						return url.Parse("http://127.0.0.1:1")
					},
					lookupNetIP: func(ctx context.Context, network, host string) ([]netip.Addr, error) {
						lookups.Add(1)
						if network != "ip" || host != "api.example.com" {
							t.Errorf("unexpected resolver target %s %s", network, host)
						}
						return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
					},
					dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
						dials.Add(1)
						if address != "8.8.8.8:443" {
							t.Errorf("dial did not use the validated numeric IP: %s", address)
						}
						return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
				call := func(path string) (*http.Response, error) {
					req, _ := http.NewRequest(http.MethodGet, "https://api.example.com"+path, nil)
					req.Header.Set("Authorization", "Bearer own-synthetic-secret")
					return Do(client, req)
				}
				resp, err := call("/redirect")
				if err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, resp.Body)
				_ = resp.Body.Close()
				if calls.Load() != 2 || lookups.Load() != 1 || dials.Load() != 1 || proxyCalls.Load() != 0 ||
					resp.TLS == nil || len(resp.TLS.VerifiedChains) == 0 || (alpn == "h2" && resp.ProtoMajor != 2) {
					t.Fatalf("verified redirect route: calls=%d lookup=%d dial=%d proxy=%d TLS=%+v", calls.Load(), lookups.Load(), dials.Load(), proxyCalls.Load(), resp.TLS)
				}
				resp, err = call("/foreign")
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
				if !errors.Is(err, ErrRedirectOrigin) || calls.Load() != 3 || lookups.Load() != 1 || dials.Load() != 1 {
					t.Fatalf("cross-origin redirect escaped even though both origins were approved: %v", err)
				}
				beforeCalls, beforeDials, beforeLookups := calls.Load(), dials.Load(), lookups.Load()
				for _, target := range []string{"http://api.example.com/v1", "https://unapproved.example.com/v1", "https://api.example.com:8443/v1", "https://127.0.0.1/v1"} {
					req, _ := http.NewRequest(http.MethodPost, target, nil)
					resp, err := client.Do(req)
					if resp != nil {
						_ = resp.Body.Close()
					}
					if !errors.Is(err, upstreampolicy.ErrDestination) {
						t.Fatalf("unsafe request %s = %v", target, err)
					}
				}
				req, _ := http.NewRequest(http.MethodGet, "https://api.example.com/v1", nil)
				req.Host = "unapproved.example.com"
				if _, err := client.Do(req); !errors.Is(err, upstreampolicy.ErrDestination) {
					t.Fatalf("Host override was not rejected: %v", err)
				}
				if calls.Load() != beforeCalls || dials.Load() != beforeDials || lookups.Load() != beforeLookups {
					t.Fatal("forbidden URL made a resolver, dial or upstream call")
				}
			})
		}
	}
}

func TestPublicTransportRejectsRebindingAtEachNewConnection(t *testing.T) {
	for _, selector := range []string{"native", "chrome", "safari"} {
		t.Run(selector, func(t *testing.T) {
			cert, roots := transportTestCertificate(t, "api.example.com")
			var calls, lookups, dials atomic.Int32
			var private atomic.Bool
			server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, "http/1.1", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				_, _ = io.WriteString(w, "ok")
			}))
			policy, _ := upstreampolicy.New([]string{"https://api.example.com"})
			transport, err := newTransport(Options{Profile: transportTestProfile(t, selector), OutboundPolicy: policy}, transportDependencies{
				roots: roots,
				lookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
					lookups.Add(1)
					ip := "8.8.8.8"
					if private.Load() {
						ip = "169.254.169.254"
					}
					return []netip.Addr{netip.MustParseAddr(ip)}, nil
				},
				dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
					dials.Add(1)
					if address != "8.8.8.8:443" {
						t.Errorf("dialed an unvalidated IP: %s", address)
					}
					return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			defer transport.CloseIdleConnections()
			client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
			resp, err := client.Get("https://api.example.com/v1/models")
			if err != nil {
				t.Fatal(err)
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			private.Store(true)
			transport.CloseIdleConnections()
			if _, err := client.Get("https://api.example.com/v1/models"); !errors.Is(err, ErrPublicAddress) {
				t.Fatalf("rebound DNS was not rejected at connection time: %v", err)
			}
			if calls.Load() != 1 || dials.Load() != 1 || lookups.Load() != 2 {
				t.Fatalf("rebound request connected: calls=%d dial=%d lookup=%d", calls.Load(), dials.Load(), lookups.Load())
			}
		})
	}
}

func TestPublicTransportRejectsEveryNonPublicDNSAnswerBeforeDial(t *testing.T) {
	policy, _ := upstreampolicy.New([]string{"https://api.example.com"})
	for _, selector := range []string{"native", "chrome", "safari"} {
		for _, raw := range []string{
			"0.0.0.0", "10.0.0.1", "100.100.100.200", "127.0.0.1", "169.254.169.254", "172.16.0.1",
			"192.0.0.1", "192.0.2.1", "192.88.99.1", "192.168.0.1", "198.18.0.1", "198.51.100.1",
			"203.0.113.1", "224.0.0.1", "240.0.0.1", "168.63.129.16", "::", "::1", "::ffff:127.0.0.1",
			"64:ff9b::7f00:1", "2001::1", "2001:db8::1", "2002:7f00:1::1", "3fff::1", "fc00::1",
			"fd00:ec2::254", "fe80::1", "fe80::1%eth0", "ff02::1", "4000::1",
		} {
			t.Run(selector+"/"+raw, func(t *testing.T) {
				var dials atomic.Int32
				transport, err := newTransport(Options{Profile: transportTestProfile(t, selector), OutboundPolicy: policy}, transportDependencies{
					lookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
						return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr(raw)}, nil
					},
					dialContext: func(context.Context, string, string) (net.Conn, error) {
						dials.Add(1)
						return nil, errors.New("must never dial")
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: time.Second}
				if _, err := client.Get("https://api.example.com/v1/chat/completions"); !errors.Is(err, ErrPublicAddress) || dials.Load() != 0 {
					t.Fatalf("mixed public/non-public answer caused a dial: err=%v dials=%d", err, dials.Load())
				}
			})
		}
	}
}

func TestPublicDialBoundsNumericPinningAndCancellation(t *testing.T) {
	policy, _ := upstreampolicy.New([]string{"https://api.example.com", "https://8.8.8.8:8443"})
	for _, scenario := range []string{"empty", "invalid", "lookup-error", "too-many", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			dials := 0
			dial := publicDialContext(policy, func(context.Context, string, string) ([]netip.Addr, error) {
				switch scenario {
				case "invalid":
					return []netip.Addr{{}}, nil
				case "lookup-error":
					return nil, errors.New("resolver unavailable")
				case "too-many":
					return slices.Repeat([]netip.Addr{netip.MustParseAddr("8.8.8.8")}, maxDNSAnswers+1), nil
				case "cancelled":
					cancel()
					return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
				default:
					return nil, nil
				}
			}, func(context.Context, string, string) (net.Conn, error) {
				dials++
				return nil, errors.New("must never dial")
			})
			if _, err := dial(ctx, "tcp", "api.example.com:443"); err == nil || dials != 0 {
				t.Fatalf("invalid resolver result made a dial: %v, %d", err, dials)
			}
		})
	}
	lookups, dials := 0, []string{}
	dial := publicDialContext(policy, func(context.Context, string, string) ([]netip.Addr, error) {
		lookups++
		return []netip.Addr{netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("1.1.1.1")}, nil
	}, func(ctx context.Context, network, address string) (net.Conn, error) {
		dials = append(dials, address)
		if address == "8.8.8.8:443" {
			return nil, errors.New("synthetic first-IP failure")
		}
		a, b := net.Pipe()
		_ = b.Close()
		return a, nil
	})
	conn, err := dial(context.Background(), "tcp", "api.example.com:443")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if lookups != 1 || !slices.Equal(dials, []string{"8.8.8.8:443", "1.1.1.1:443"}) {
		t.Fatalf("validated addresses were not pinned/deduplicated: %d, %v", lookups, dials)
	}
	conn, err = dial(context.Background(), "tcp", "8.8.8.8:8443")
	if err != nil {
		t.Fatal(err)
	}
	_ = conn.Close()
	if lookups != 1 || dials[len(dials)-1] != "8.8.8.8:8443" {
		t.Fatal("approved IP literal was resolved again or lost its exact port")
	}
	before := len(dials)
	if _, err := dial(context.Background(), "tcp", "api.example.com:8443"); err == nil || len(dials) != before || lookups != 1 {
		t.Fatal("unapproved connection origin reached DNS or dial")
	}
}

func TestPublicTransportKeepsCertificateAndHostnameVerification(t *testing.T) {
	policy, _ := upstreampolicy.New([]string{"https://api.example.com"})
	for _, selector := range []string{"native", "chrome", "safari"} {
		for _, scenario := range []string{"untrusted", "hostname"} {
			t.Run(selector+"/"+scenario, func(t *testing.T) {
				name := "api.example.com"
				if scenario == "hostname" {
					name = "wrong.example.com"
				}
				cert, roots := transportTestCertificate(t, name)
				if scenario == "untrusted" {
					roots = x509.NewCertPool()
				}
				var calls atomic.Int32
				server, _, _, _ := transportTestServer(t, cert, tls.VersionTLS13, "http/1.1", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					calls.Add(1)
				}))
				transport, err := newTransport(Options{Profile: transportTestProfile(t, selector), OutboundPolicy: policy}, transportDependencies{
					roots: roots,
					lookupNetIP: func(context.Context, string, string) ([]netip.Addr, error) {
						return []netip.Addr{netip.MustParseAddr("8.8.8.8")}, nil
					},
					dialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
						return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
					},
				})
				if err != nil {
					t.Fatal(err)
				}
				defer transport.CloseIdleConnections()
				client := &http.Client{Transport: transport, Timeout: time.Second}
				if _, err := client.Get("https://api.example.com/v1/models"); err == nil || calls.Load() != 0 {
					t.Fatalf("certificate/hostname verification was bypassed: %v calls=%d", err, calls.Load())
				}
			})
		}
	}
}
