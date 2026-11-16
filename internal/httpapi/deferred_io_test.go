package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/provider"
)

type deferredSocketWrite struct {
	size     int
	deadline time.Time
	done     chan error
}

type deferredSocketConn struct {
	net.Conn
	mu            sync.Mutex
	readDeadline  time.Time
	writeDeadline time.Time
	writes        chan deferredSocketWrite
	readErrors    chan error
}

func (c *deferredSocketConn) SetReadDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.readDeadline = deadline
	c.mu.Unlock()
	return c.Conn.SetReadDeadline(deadline)
}

func (c *deferredSocketConn) SetWriteDeadline(deadline time.Time) error {
	c.mu.Lock()
	c.writeDeadline = deadline
	c.mu.Unlock()
	return c.Conn.SetWriteDeadline(deadline)
}

func (c *deferredSocketConn) Read(data []byte) (int, error) {
	n, err := c.Conn.Read(data)
	if err != nil {
		select {
		case c.readErrors <- err:
		default:
		}
	}
	return n, err
}

func (c *deferredSocketConn) Write(data []byte) (int, error) {
	c.mu.Lock()
	deadline := c.writeDeadline
	c.mu.Unlock()
	event := deferredSocketWrite{size: len(data), deadline: deadline, done: make(chan error, 1)}
	select {
	case c.writes <- event:
	default:
	}
	n, err := c.Conn.Write(data)
	event.done <- err
	return n, err
}

type deferredSocketListener struct {
	net.Listener
	connections chan *deferredSocketConn
}

func (l *deferredSocketListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err != nil {
		return nil, err
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		if err := tcp.SetWriteBuffer(1024); err != nil {
			_ = conn.Close()
			return nil, err
		}
	}
	wrapped := &deferredSocketConn{Conn: conn, writes: make(chan deferredSocketWrite, 16), readErrors: make(chan error, 8)}
	l.connections <- wrapped
	return wrapped, nil
}

func deferredSocketServer(t *testing.T, h http.Handler) (*httptest.Server, *deferredSocketListener) {
	t.Helper()
	server := httptest.NewUnstartedServer(h)
	listener := &deferredSocketListener{Listener: server.Listener, connections: make(chan *deferredSocketConn, 16)}
	server.Listener = listener
	server.Config.ReadTimeout, server.Config.WriteTimeout = 5*time.Second, 5*time.Second
	server.Start()
	t.Cleanup(server.Close)
	return server, listener
}

func deferredSocketRequest(t *testing.T, server *httptest.Server, f *deferredHTTPFixture, method, path string) (*http.Response, []byte) {
	t.Helper()
	request := f.request(method, path, "peer-token", nil)
	request.URL.Scheme, request.URL.Host, request.RequestURI = "http", server.Listener.Addr().String(), ""
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(response.Body)
	_ = response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	return response, body
}

func deferredSlowSocket(t *testing.T, server *httptest.Server, listener *deferredSocketListener, path, generation string) (*net.TCPConn, *deferredSocketConn) {
	t.Helper()
	conn, err := net.DialTCP("tcp", nil, server.Listener.Addr().(*net.TCPAddr))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	if err := conn.SetReadBuffer(1024); err != nil {
		t.Fatal(err)
	}
	_, err = fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: peer\r\nAuthorization: Bearer peer-token\r\nX-Aiproxy-Deferred-Version: 1\r\nX-Aiproxy-Deferred-Generation: %s\r\n\r\n", path, generation)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case accepted := <-listener.connections:
		return conn, accepted
	case <-time.After(time.Second):
		t.Fatal("server did not accept deferred socket")
		return nil, nil
	}
}

func awaitDeferredSocketWrite(t *testing.T, conn *deferredSocketConn, minimum int) deferredSocketWrite {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for {
		select {
		case event := <-conn.writes:
			if event.size >= minimum {
				return event
			}
		case <-timer.C:
			t.Fatal("deferred response did not reach the real socket")
			return deferredSocketWrite{}
		}
	}
}

func TestDeferredSmallControlAndDeleteFlushWithinRealSocketDeadline(t *testing.T) {
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: []byte(`{}`)}, nil
	}))
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`)))
	f.finish(t, receipt)
	server, listener := deferredSocketServer(t, f.h)
	for _, request := range []struct{ method, path string }{
		{"GET", deferred.Prefix + "/capabilities?model=p/m"},
		{"GET", deferred.Prefix + "/unknown"},
		{"DELETE", deferred.Prefix + "/jobs/" + receipt.ID},
	} {
		response, _ := deferredSocketRequest(t, server, f, request.method, request.path)
		var conn *deferredSocketConn
		select {
		case conn = <-listener.connections:
		default:
		}
		if conn == nil {
			t.Fatal("control socket was not observed")
		}
		event := awaitDeferredSocketWrite(t, conn, 1)
		if event.deadline.IsZero() || time.Until(event.deadline) > f.store.Options().ControlTimeout {
			t.Fatalf("%s control flushed without its private write deadline: deadline=%v status=%d", request.method, event.deadline, response.StatusCode)
		}
		server.Client().CloseIdleConnections()
	}
}

func TestDeferredSubmitRealBodyReadDeadlineReleasesIngress(t *testing.T) {
	var calls atomic.Int32
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return nil, errors.New("unexpected inference")
	}))
	server, listener := deferredSocketServer(t, f.h)
	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	start := time.Now()
	_, err = fmt.Fprintf(conn, "POST %s/submit HTTP/1.1\r\nHost: peer\r\nAuthorization: Bearer peer-token\r\nX-Aiproxy-Deferred-Version: 1\r\nContent-Type: %s\r\nContent-Length: 1000\r\n\r\n{", deferred.Prefix, deferred.MediaType)
	if err != nil {
		t.Fatal(err)
	}
	accepted := <-listener.connections
	awaitDeferredHTTP(t, "reserved submit reader", func() bool { return f.store.Stats().Admissions == 1 })
	var readErr error
	select {
	case readErr = <-accepted.readErrors:
	case <-time.After(time.Second):
		t.Fatal("private body read used the five-second listener deadline")
	}
	var networkError net.Error
	if !errors.As(readErr, &networkError) || !networkError.Timeout() || time.Since(start) > time.Second {
		t.Fatalf("real submit body read was not deadline-bounded: %v", readErr)
	}
	awaitDeferredHTTP(t, "timed-out ingress cleanup", func() bool {
		s := f.store.Stats()
		return s.Admissions == 0 && s.Controls == 0 && s.IngressExtraBytes == 0
	})
	if calls.Load() != 0 || f.rate.calls.Load() != 0 || len(f.accounting.Events()) != 0 || f.store.Stats().Jobs != 0 {
		t.Fatal("timed-out submit reader consumed inference policy or scheduled work")
	}
}

func TestDeferredRealReadWriteTimeoutPreservesResultForUnchangedCursor(t *testing.T) {
	var calls atomic.Int32
	body := []byte(`{"vendor":"` + strings.Repeat("x", 100<<10) + `"}`)
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
	}))
	receipt := f.submit(t, deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`)))
	f.finish(t, receipt)
	server, listener := deferredSocketServer(t, f.h)
	path := deferred.Prefix + "/jobs/" + receipt.ID + "/read?cursor=0"
	slow, accepted := deferredSlowSocket(t, server, listener, path, receipt.Generation)
	event := awaitDeferredSocketWrite(t, accepted, 32<<10)
	select {
	case err := <-event.done:
		var networkError net.Error
		if !errors.As(err, &networkError) || !networkError.Timeout() || event.deadline.IsZero() {
			t.Fatalf("real result write did not time out under private deadline: %v deadline=%v", err, event.deadline)
		}
	case <-time.After(time.Second):
		t.Fatal("finite result write used the five-second listener deadline")
	}
	_ = slow.Close()
	awaitDeferredHTTP(t, "failed transfer releases only its control", func() bool { return f.store.Stats().Controls == 0 })
	frame := f.read(t, receipt, 0, deferred.MaxReadBytes)
	if frame.Done || frame.Cursor != 0 || frame.NextCursor != deferred.MaxReadBytes || !bytes.Equal(frame.Data, body[:deferred.MaxReadBytes]) ||
		f.store.Stats().AvailableBytes != int64(len(body)) || calls.Load() != 1 || f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 1 {
		t.Fatal("failed result transfer consumed output, cursor, or inference accounting")
	}
	metrics := httptest.NewRecorder()
	f.metrics.Handler().ServeHTTP(metrics, httptest.NewRequest("GET", "/metrics", nil))
	if !strings.Contains(metrics.Body.String(), `aiproxy_deferred_delivery_errors_total{route="read"} 1`) ||
		strings.Contains(metrics.Body.String(), receipt.ID) {
		t.Fatalf("failed real transfer was not reported with bounded labels: %s", metrics.Body.String())
	}
	var replay deferred.ReadFrame
	response, wire := deferredSocketRequest(t, server, f, "GET", path)
	if response.StatusCode != 200 || json.Unmarshal(wire, &replay) != nil || !bytes.Equal(replay.Data, frame.Data) {
		t.Fatal("same cursor was not recoverable over a fresh real HTTP connection")
	}
}

func TestDeferredRealBlockedPollAllowsDeleteWithoutPinningReleasedResult(t *testing.T) {
	var calls atomic.Int32
	body := []byte(`{"vendor":"` + strings.Repeat("x", 100<<10) + `"}`)
	f := newDeferredHTTPFixture(t, deferredAdapterFunc(func(context.Context, provider.Request) (*provider.Result, error) {
		calls.Add(1)
		return &provider.Result{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}}, Body: body}, nil
	}))
	request := deferredSubmitRequest(t, f.capability(t, "p/m"), []byte(`{"model":"p/m"}`))
	receipt := f.submit(t, request)
	f.finish(t, receipt)
	server, listener := deferredSocketServer(t, f.h)
	path := deferred.Prefix + "/jobs/" + receipt.ID
	slow, accepted := deferredSlowSocket(t, server, listener, path+"/read?cursor=0", receipt.Generation)
	event := awaitDeferredSocketWrite(t, accepted, 32<<10)
	before := f.store.Stats()
	pressure := f.call("GET", path, "peer-token", nil)
	if pressure.Code != 429 || controlErrorType(t, pressure) != "deferred_control_limited" || before.Controls != 1 {
		t.Fatal("a logical job admitted overlapping polls while a real transfer owned its scratch")
	}
	begin := time.Now()
	response, data := deferredSocketRequest(t, server, f, "DELETE", path)
	if response.StatusCode != 204 || len(data) != 0 || time.Since(begin) >= f.store.Options().ControlTimeout {
		t.Fatal("reserved delete waited for the blocked real poll transfer")
	}
	after := f.store.Stats()
	if after.Jobs != 0 || after.OutputBytes != 0 || after.AvailableBytes != 0 || after.Controls != 1 ||
		before.ReservedBytes-after.ReservedBytes != before.OutputBytes {
		t.Fatalf("blocked read pinned deleted output or falsely released control I/O ownership: before=%+v after=%+v", before, after)
	}
	select {
	case err := <-event.done:
		var networkError net.Error
		if !errors.As(err, &networkError) || !networkError.Timeout() {
			t.Fatalf("released result transfer did not retain its finite socket deadline: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("released result scratch remained in an unbounded socket write")
	}
	_ = slow.Close()
	awaitDeferredHTTP(t, "released poll scratch cleanup", func() bool { return f.store.Stats().Controls == 0 })
	replay := f.submit(t, request)
	if replay.ID != receipt.ID || !replay.Replayed || calls.Load() != 1 || f.rate.calls.Load() != 1 || len(f.accounting.Events()) != 1 {
		t.Fatal("poll/delete transfer race lost dedup protection or duplicated inference")
	}
	released := f.call("GET", path+"/read?cursor=0", "peer-token", nil)
	if released.Code != 410 || controlErrorType(t, released) != "job_released" {
		t.Fatal("released result became readable after its in-flight transfer ended")
	}
}
