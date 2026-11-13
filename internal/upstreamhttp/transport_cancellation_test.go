package upstreamhttp

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProfileDialStageCancellation(t *testing.T) {
	cert, roots := transportTestCertificate(t)
	for _, stage := range []string{"upstream-tls", "http-connect", "https-proxy-tls", "upstream-after-http-connect", "upstream-after-https-connect"} {
		t.Run(stage, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			done := make(chan struct{})
			go func() {
				defer close(done)
				conn, err := listener.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
				if stage == "http-connect" {
					_, _ = http.ReadRequest(bufio.NewReader(conn))
				}
				_, _ = io.Copy(io.Discard, conn)
			}()
			var proxy *url.URL
			address := listener.Addr().String()
			switch stage {
			case "http-connect":
				proxy, _ = url.Parse("http://" + address)
				address = "localhost:1"
			case "https-proxy-tls":
				proxy, _ = url.Parse("https://" + address)
				address = "localhost:1"
			case "upstream-after-http-connect":
				proxy, _, _ = transportTestProxy(t, cert, false)
			case "upstream-after-https-connect":
				proxy, _, _ = transportTestProxy(t, cert, true)
			}
			route, err := freezeProxyRoute(proxy)
			if err != nil {
				t.Fatal(err)
			}
			dialer := &net.Dialer{Timeout: tcpDialTimeout, KeepAlive: tcpKeepAlive}
			ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
			defer cancel()
			start := time.Now()
			conn, err := dialProfile(ctx, "tcp", address, transportTestProfile(t, "chrome"), route, transportDependencies{roots: roots, dialContext: dialer.DialContext})
			if conn != nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second {
				t.Fatalf("dial stage cancellation: %v elapsed=%v", err, time.Since(start))
			}
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("canceled dial stage retained socket")
			}
		})
	}
}

type phaseTestConn struct {
	net.Conn
	mu        sync.Mutex
	deadlines []time.Time
	closes    atomic.Int32
	started   chan struct{}
	release   chan struct{}
	once      sync.Once
}

func (c *phaseTestConn) SetDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.deadlines = append(c.deadlines, deadline)
	c.mu.Unlock()
	return nil
}

func (c *phaseTestConn) Close() error {
	c.closes.Add(1)
	if c.started != nil {
		c.once.Do(func() { close(c.started) })
		<-c.release
	}
	return nil
}

func TestConnectionPhaseStopsCancellationBeforeReturningReusableSocket(t *testing.T) {
	for _, timeout := range []time.Duration{tlsHandshakeTimeout, proxyConnectTimeout} {
		ctx, cancel := context.WithCancel(context.Background())
		conn := &phaseTestConn{}
		start := time.Now()
		if err := connectionPhase(ctx, timeout, conn, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		cancel()
		time.Sleep(time.Millisecond)
		conn.mu.Lock()
		deadlines := append([]time.Time(nil), conn.deadlines...)
		conn.mu.Unlock()
		if conn.closes.Load() != 0 || len(deadlines) != 2 || !deadlines[1].IsZero() || deadlines[0].Before(start.Add(timeout-time.Second)) || deadlines[0].After(start.Add(timeout+time.Second)) {
			t.Fatalf("deadline/cancellation callback affected returned socket: %v closes=%d", deadlines, conn.closes.Load())
		}
	}
}

func TestConnectionPhaseJoinsAlreadyRunningCancellationCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn := &phaseTestConn{started: make(chan struct{}), release: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		result <- connectionPhase(ctx, tlsHandshakeTimeout, conn, func(context.Context) error {
			cancel()
			<-conn.started
			return nil
		})
	}()
	<-conn.started
	select {
	case err := <-result:
		t.Fatalf("returned before cancellation callback finished: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(conn.release)
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("racing successful phase lost cancellation: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("phase did not join canceled close")
	}
}

func TestConnectionPhaseDeadlineBoundsAndErrors(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	conn := &phaseTestConn{}
	start := time.Now()
	err := connectionPhase(ctx, proxyConnectTimeout, conn, func(phase context.Context) error {
		<-phase.Done()
		return errors.New("closed socket")
	})
	if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) > time.Second || conn.closes.Load() == 0 {
		t.Fatalf("parent deadline was not honored: %v", err)
	}
	phaseError := errors.New("handshake failed")
	if err := connectionPhase(context.Background(), tlsHandshakeTimeout, &phaseTestConn{}, func(context.Context) error { return phaseError }); !errors.Is(err, phaseError) {
		t.Fatal("phase lost handshake error")
	}
}
