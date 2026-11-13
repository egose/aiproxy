package upstreamhttp

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func awaitTransportCondition(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("transport condition timed out")
}

func TestRetiringTransportPreservesActiveStreamsAndClosesIdleIncludingH2(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, name := range []string{"native", "chrome", "custom-h2"} {
		for _, alpn := range []string{"http/1.1", "h2"} {
			t.Run(name+"/"+alpn, func(t *testing.T) {
				first, second := make(chan struct{}), make(chan struct{})
				releaseFirst := sync.OnceFunc(func() { close(first) })
				releaseSecond := sync.OnceFunc(func() { close(second) })
				defer releaseFirst()
				defer releaseSecond()
				server, _, accepts, closes := transportTestServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					_, _ = io.WriteString(w, "x")
					w.(http.Flusher).Flush()
					var release <-chan struct{}
					switch req.URL.Path {
					case "/first":
						release = first
					case "/second":
						release = second
					default:
						return
					}
					select {
					case <-release:
						_, _ = io.WriteString(w, "finished")
					case <-req.Context().Done():
					}
				}))
				idle, _, _, idleCloses := transportTestServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					_, _ = io.WriteString(w, "idle")
				}))
				_, inner := transportTestClient(t, transportTestProfile(t, name), roots, nil, time.Second)
				retiring := NewRetiringTransport(inner)
				defer retiring.Retire()
				client := &http.Client{Transport: retiring}
				get := func(target string) *http.Response {
					t.Helper()
					resp, err := client.Get(target)
					if err != nil {
						t.Fatal(err)
					}
					return resp
				}
				idleResponse := get(idle.URL)
				_, _ = io.Copy(io.Discard, idleResponse.Body)
				_ = idleResponse.Body.Close()
				one, two := get(server.URL+"/first"), get(server.URL+"/second")
				defer one.Body.Close()
				defer two.Body.Close()
				for _, resp := range []*http.Response{one, two} {
					if _, err := io.ReadFull(resp.Body, make([]byte, 1)); err != nil {
						t.Fatal(err)
					}
				}
				connections := accepts.Load()
				if alpn == "h2" && connections != 1 {
					t.Fatal("HTTP/2 did not multiplex active streams")
				}
				retiring.Retire()
				awaitTransportCondition(t, func() bool { return idleCloses.Load() == 1 })
				if closes.Load() != 0 {
					t.Fatal("retirement interrupted active connections")
				}
				releaseFirst()
				body, err := io.ReadAll(one.Body)
				if err != nil || string(body) != "finished" {
					t.Fatalf("first active stream failed: %s %v", body, err)
				}
				if alpn == "h2" && closes.Load() != 0 {
					t.Fatal("first EOF closed a multiplexed active stream")
				}
				releaseSecond()
				body, err = io.ReadAll(two.Body)
				if err != nil || string(body) != "finished" {
					t.Fatalf("second active stream failed: %s %v", body, err)
				}
				awaitTransportCondition(t, func() bool { return closes.Load() == connections })
				later := get(server.URL + "/admitted-retry")
				if _, err := io.ReadAll(later.Body); err != nil {
					t.Fatal(err)
				}
				awaitTransportCondition(t, func() bool { return closes.Load() == connections+1 })
				_ = later.Body.Close()
				retiring.mu.Lock()
				if retiring.active != 0 || len(retiring.conns) != 0 || retiring.cleanup != nil {
					t.Error("retired transport retained connection history")
				}
				retiring.mu.Unlock()
			})
		}
	}
}

func TestRetiringTransportCancellationDoesNotKillOtherH2Stream(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	release := make(chan struct{})
	releaseOnce := sync.OnceFunc(func() { close(release) })
	defer releaseOnce()
	server, _, _, closes := transportTestServer(t, cert, tls.VersionTLS13, "h2", http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		_, _ = io.WriteString(w, "x")
		w.(http.Flusher).Flush()
		select {
		case <-release:
			_, _ = io.WriteString(w, "finished")
		case <-req.Context().Done():
		}
	}))
	_, inner := transportTestClient(t, transportTestProfile(t, "chrome"), roots, nil, 0)
	retiring := NewRetiringTransport(inner)
	defer retiring.Retire()
	client := &http.Client{Transport: retiring}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	one, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer one.Body.Close()
	two, err := client.Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer two.Body.Close()
	for _, resp := range []*http.Response{one, two} {
		if _, err := io.ReadFull(resp.Body, make([]byte, 1)); err != nil {
			t.Fatal(err)
		}
	}
	retiring.Retire()
	cancel()
	if _, err := io.ReadAll(one.Body); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled body: %v", err)
	}
	releaseOnce()
	body, err := io.ReadAll(two.Body)
	if err != nil || string(body) != "finished" {
		t.Fatalf("canceling one retired stream interrupted another: %s %v", body, err)
	}
	awaitTransportCondition(t, func() bool { return closes.Load() == 1 })
}

func TestSuccessfulRequestCancellationCannotCloseReusedProfileSocket(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, alpn := range []string{"http/1.1", "h2"} {
		server, _, accepts, _ := transportTestServer(t, cert, tls.VersionTLS13, alpn, http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			_, _ = io.WriteString(w, "ok")
		}))
		_, inner := transportTestClient(t, transportTestProfile(t, "safari"), roots, nil, 0)
		retiring := NewRetiringTransport(inner)
		defer retiring.Retire()
		client := &http.Client{Transport: retiring}
		for range 30 {
			ctx, cancel := context.WithCancel(context.Background())
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
			resp, err := client.Do(req)
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			_, err = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			cancel()
			if err != nil {
				t.Fatal(err)
			}
		}
		if accepts.Load() != 1 {
			t.Fatalf("completed request cancellation closed a reused socket: accepted=%d", accepts.Load())
		}
		retiring.mu.Lock()
		if len(retiring.conns) != 0 || retiring.active != 0 {
			t.Error("live transport retained past connection references")
		}
		retiring.mu.Unlock()
	}
}

type retirementFixtureTransport struct {
	conn      net.Conn
	body      io.ReadCloser
	err       error
	idleCalls atomic.Int32
}

func (t *retirementFixtureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if trace := httptrace.ContextClientTrace(req.Context()); trace != nil && trace.GotConn != nil {
		trace.GotConn(httptrace.GotConnInfo{Conn: t.conn})
	}
	if t.err != nil {
		return nil, t.err
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: t.body}, nil
}

func (t *retirementFixtureTransport) CloseIdleConnections() { t.idleCalls.Add(1) }

type retirementErrorBody struct{}

func (retirementErrorBody) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func (retirementErrorBody) Close() error             { return nil }

func TestRetiringTransportCleanupOnCloseReadErrorAndRoundTripError(t *testing.T) {
	for _, scenario := range []string{"close", "read-error", "roundtrip-error"} {
		t.Run(scenario, func(t *testing.T) {
			conn := &phaseTestConn{}
			fixture := &retirementFixtureTransport{conn: conn, body: io.NopCloser(strings.NewReader("body"))}
			if scenario == "read-error" {
				fixture.body = retirementErrorBody{}
			}
			if scenario == "roundtrip-error" {
				fixture.err = errors.New("roundtrip failed")
			}
			transport := NewRetiringTransport(fixture)
			transport.Retire()
			req, _ := http.NewRequest(http.MethodGet, "https://example.test", nil)
			resp, err := transport.RoundTrip(req)
			if scenario == "roundtrip-error" {
				if !errors.Is(err, fixture.err) {
					t.Fatal("roundtrip error lost")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if scenario == "read-error" {
					if _, err := io.ReadAll(resp.Body); !errors.Is(err, io.ErrUnexpectedEOF) {
						t.Fatal("body error lost")
					}
				} else if err := resp.Body.Close(); err != nil {
					t.Fatal(err)
				}
			}
			if conn.closes.Load() != 1 || fixture.idleCalls.Load() < 2 {
				t.Fatal("retirement cleanup missed response completion")
			}
		})
	}
}

func TestRetiringTransportWaitsForSocketCleanupBeforeAdmittedRetry(t *testing.T) {
	conn := &phaseTestConn{started: make(chan struct{}), release: make(chan struct{})}
	fixture := &retirementFixtureTransport{conn: conn, body: io.NopCloser(strings.NewReader("body"))}
	transport := NewRetiringTransport(fixture)
	req, _ := http.NewRequest(http.MethodGet, "https://example.test", nil)
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	transport.Retire()
	finished := make(chan struct{})
	go func() { _ = resp.Body.Close(); close(finished) }()
	<-conn.started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	retry, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://example.test", nil)
	if _, err := transport.RoundTrip(retry); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("retry entered a pool before cleanup finished: %v", err)
	}
	close(conn.release)
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not finish")
	}
}

func TestRetiringTransportClosesLateHTTP2DialAfterCancellation(t *testing.T) {
	for _, name := range []string{"native", "chrome"} {
		t.Run(name, func(t *testing.T) { retiringLateDialAfterCancellation(t, name) })
	}
}

func retiringLateDialAfterCancellation(t *testing.T, name string) {
	cert, roots := transportTestCertificate(t)
	server, _, _, closes := transportTestServer(t, cert, tls.VersionTLS13, "h2", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("canceled late dial reached inference")
	}))
	_, inner := transportTestClient(t, transportTestProfile(t, name), roots, nil, 0)
	ready, proceed := make(chan struct{}), make(chan struct{})
	continueOnce := sync.OnceFunc(func() { close(proceed) })
	defer continueOnce()
	late := func(dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
		return func(ctx context.Context, network, address string) (net.Conn, error) {
			conn, err := dial(ctx, network, address)
			if err != nil {
				return nil, err
			}
			close(ready)
			<-proceed
			return conn, nil
		}
	}
	if native, ok := inner.(*http.Transport); ok {
		native.DialContext = late(native.DialContext)
	} else {
		direct := inner.(*profileTransport).routes[proxyRoute{}]
		direct.DialTLSContext = late(direct.DialTLSContext)
	}
	retiring := NewRetiringTransport(inner)
	defer retiring.Retire()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	finished := make(chan error, 1)
	go func() { _, err := retiring.RoundTrip(req); finished <- err }()
	select {
	case <-ready:
	case <-time.After(3 * time.Second):
		t.Fatal("late dial did not connect")
	}
	retiring.Retire()
	cancel()
	if err := <-finished; !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled late dial: %v", err)
	}
	continueOnce()
	awaitTransportCondition(t, func() bool { return closes.Load() == 1 })
}
