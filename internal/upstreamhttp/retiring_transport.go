package upstreamhttp

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
)

type RetiringTransport struct {
	inner   Transport
	mu      sync.Mutex
	retired bool
	active  int
	conns   map[net.Conn]int
	dials   map[*retiringConn]struct{}
	cleanup chan struct{}
}

func NewRetiringTransport(inner Transport) *RetiringTransport {
	transport := &RetiringTransport{inner: inner, conns: make(map[net.Conn]int), dials: make(map[*retiringConn]struct{})}
	switch inner := inner.(type) {
	case *http.Transport:
		transport.trackNativeDials(inner)
	case *profileTransport:
		transport.trackNativeDials(inner.native)
		inner.deps.dialContext = transport.trackDial(inner.deps.dialContext)
	}
	return transport
}

func (t *RetiringTransport) trackNativeDials(transport *http.Transport) {
	if transport.DialContext != nil {
		transport.DialContext = t.trackDial(transport.DialContext)
	}
}

func (t *RetiringTransport) trackDial(dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		conn, err := dial(ctx, network, address)
		if err != nil {
			return nil, err
		}
		tracked := &retiringConn{Conn: conn, owner: t}
		t.mu.Lock()
		if t.retired && t.active == 0 {
			t.mu.Unlock()
			_ = conn.Close()
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			return nil, context.Canceled
		}
		t.dials[tracked] = struct{}{}
		t.mu.Unlock()
		return tracked, nil
	}
}

func (t *RetiringTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	for {
		t.mu.Lock()
		if cleanup := t.cleanup; cleanup != nil {
			t.mu.Unlock()
			select {
			case <-cleanup:
				continue
			case <-req.Context().Done():
				closeRequestBody(req)
				return nil, req.Context().Err()
			}
		}
		t.active++
		t.mu.Unlock()
		break
	}
	conns := make(map[net.Conn]struct{})
	trace := &httptrace.ClientTrace{GotConn: func(info httptrace.GotConnInfo) {
		t.mu.Lock()
		if _, seen := conns[info.Conn]; !seen {
			conns[info.Conn] = struct{}{}
			t.conns[info.Conn]++
		}
		t.mu.Unlock()
	}}
	var once sync.Once
	finish := func() { once.Do(func() { t.finish(conns) }) }
	resp, err := t.inner.RoundTrip(req.WithContext(httptrace.WithClientTrace(req.Context(), trace)))
	if err != nil || resp == nil || resp.Body == nil {
		finish()
		return resp, err
	}
	resp.Body = &retiringBody{ReadCloser: resp.Body, finish: finish}
	return resp, nil
}

func (t *RetiringTransport) finish(conns map[net.Conn]struct{}) {
	t.mu.Lock()
	t.active--
	for conn := range conns {
		t.conns[conn]--
		if !t.retired && t.conns[conn] == 0 {
			delete(t.conns, conn)
		}
	}
	retired := t.retired
	toClose, cleanup := t.prepareCleanupLocked()
	t.mu.Unlock()
	if retired {
		t.closeRetired(toClose, cleanup)
	}
}

func (t *RetiringTransport) Retire() {
	t.mu.Lock()
	if t.retired {
		t.mu.Unlock()
		return
	}
	t.retired = true
	toClose, cleanup := t.prepareCleanupLocked()
	t.mu.Unlock()
	t.closeRetired(toClose, cleanup)
}

func (t *RetiringTransport) prepareCleanupLocked() ([]net.Conn, chan struct{}) {
	if !t.retired || t.active != 0 || t.cleanup != nil {
		return nil, nil
	}
	conns := make([]net.Conn, 0, len(t.dials)+len(t.conns))
	for conn := range t.dials {
		conns = append(conns, conn)
	}
	for conn := range t.conns {
		conns = append(conns, conn)
	}
	clear(t.conns)
	clear(t.dials)
	t.cleanup = make(chan struct{})
	return conns, t.cleanup
}

func (t *RetiringTransport) closeRetired(conns []net.Conn, cleanup chan struct{}) {
	t.inner.CloseIdleConnections()
	for _, conn := range conns {
		_ = conn.Close()
	}
	if cleanup != nil {
		t.mu.Lock()
		t.cleanup = nil
		close(cleanup)
		t.mu.Unlock()
	}
}

func (t *RetiringTransport) CloseIdleConnections() {
	t.inner.CloseIdleConnections()
}

type retiringConn struct {
	net.Conn
	owner    *RetiringTransport
	once     sync.Once
	closeErr error
}

func (c *retiringConn) Close() error {
	c.once.Do(func() {
		c.closeErr = c.Conn.Close()
		c.owner.mu.Lock()
		delete(c.owner.dials, c)
		c.owner.mu.Unlock()
	})
	return c.closeErr
}

type retiringBody struct {
	io.ReadCloser
	once     sync.Once
	finish   func()
	closeErr error
}

func (b *retiringBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	if err != nil {
		_ = b.Close()
	}
	return n, err
}

func (b *retiringBody) Close() error {
	b.once.Do(func() {
		b.closeErr = b.ReadCloser.Close()
		b.finish()
	})
	return b.closeErr
}
