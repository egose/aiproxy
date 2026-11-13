package upstreamhttp

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/tlsprofile"
	utls "github.com/refraction-networking/utls"
)

const (
	tcpDialTimeout         = 30 * time.Second
	tcpKeepAlive           = 30 * time.Second
	tlsHandshakeTimeout    = 10 * time.Second
	proxyConnectTimeout    = time.Minute
	maxConnectHeaderBytes  = 10 << 20
	maxProfiledProxyRoutes = 8
)

type Transport interface {
	http.RoundTripper
	CloseIdleConnections()
}

type Options struct {
	Profile               tlsprofile.Profile
	ResponseHeaderTimeout time.Duration
}

type transportDependencies struct {
	roots       *x509.CertPool
	proxy       func(*http.Request) (*url.URL, error)
	dialContext func(context.Context, string, string) (net.Conn, error)
}

func NewTransport(options Options) (Transport, error) {
	return newTransport(options, transportDependencies{})
}

func newTransport(options Options, deps transportDependencies) (Transport, error) {
	customDial := deps.dialContext != nil
	if deps.proxy == nil {
		deps.proxy = http.ProxyFromEnvironment
	}
	if deps.dialContext == nil {
		dialer := &net.Dialer{Timeout: tcpDialTimeout, KeepAlive: tcpKeepAlive}
		deps.dialContext = dialer.DialContext
	}
	native := http.DefaultTransport.(*http.Transport).Clone()
	native.Proxy = deps.proxy
	native.ResponseHeaderTimeout = options.ResponseHeaderTimeout
	if customDial {
		native.DialContext = deps.dialContext
	}
	if deps.roots != nil {
		if native.TLSClientConfig == nil {
			native.TLSClientConfig = &tls.Config{}
		}
		native.TLSClientConfig.RootCAs = deps.roots
	}
	if options.Profile.IsNative() {
		return native, nil
	}
	if _, err := options.Profile.Materialize(); err != nil {
		return nil, fmt.Errorf("upstream TLS profile: %w", err)
	}
	profiled := &profileTransport{
		options: options,
		deps:    deps,
		native:  native,
		routes:  make(map[proxyRoute]*http.Transport),
	}
	profiled.routes[proxyRoute{}] = profiled.newRouteTransport(proxyRoute{})
	return profiled, nil
}

type proxyRoute struct {
	scheme        string
	address       string
	serverName    string
	authorization string
}

func freezeProxyRoute(proxy *url.URL) (proxyRoute, error) {
	if proxy == nil {
		return proxyRoute{}, nil
	}
	if proxy.Scheme != "http" && proxy.Scheme != "https" {
		return proxyRoute{}, errors.New("upstream TLS profile supports only HTTP/1.1 CONNECT through http or https proxies; SOCKS and HTTP/2 CONNECT are unsupported")
	}
	host := proxy.Hostname()
	if host == "" {
		return proxyRoute{}, errors.New("upstream TLS proxy has no hostname")
	}
	port := proxy.Port()
	if port == "" {
		port = "80"
		if proxy.Scheme == "https" {
			port = "443"
		}
	}
	route := proxyRoute{scheme: proxy.Scheme, address: net.JoinHostPort(host, port), serverName: host}
	if proxy.User != nil {
		password, _ := proxy.User.Password()
		route.authorization = "Basic " + base64.StdEncoding.EncodeToString([]byte(proxy.User.Username()+":"+password))
	}
	return route, nil
}

type profileTransport struct {
	options Options
	deps    transportDependencies
	native  *http.Transport
	mu      sync.Mutex
	routes  map[proxyRoute]*http.Transport
}

func (t *profileTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.URL.Scheme != "https" {
		return t.native.RoundTrip(req)
	}
	proxy, err := t.deps.proxy(req)
	if err != nil {
		closeRequestBody(req)
		return nil, err
	}
	route, err := freezeProxyRoute(proxy)
	if err != nil {
		closeRequestBody(req)
		return nil, err
	}
	t.mu.Lock()
	inner := t.routes[route]
	if inner == nil {
		if len(t.routes) >= maxProfiledProxyRoutes {
			t.mu.Unlock()
			closeRequestBody(req)
			return nil, errors.New("upstream TLS proxy route limit exceeded")
		}
		inner = t.newRouteTransport(route)
		t.routes[route] = inner
	}
	t.mu.Unlock()
	return inner.RoundTrip(req)
}

func closeRequestBody(req *http.Request) {
	if req.Body != nil {
		_ = req.Body.Close()
	}
}

func (t *profileTransport) newRouteTransport(route proxyRoute) *http.Transport {
	inner := http.DefaultTransport.(*http.Transport).Clone()
	inner.Proxy = nil
	inner.ForceAttemptHTTP2 = true
	inner.ResponseHeaderTimeout = t.options.ResponseHeaderTimeout
	inner.DialTLSContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		return dialProfile(ctx, network, address, t.options.Profile, route, t.deps)
	}
	return inner
}

func (t *profileTransport) CloseIdleConnections() {
	t.native.CloseIdleConnections()
	t.mu.Lock()
	routes := make([]*http.Transport, 0, len(t.routes))
	for _, inner := range t.routes {
		routes = append(routes, inner)
	}
	t.mu.Unlock()
	for _, inner := range routes {
		inner.CloseIdleConnections()
	}
}

func dialProfile(ctx context.Context, network, address string, profile tlsprofile.Profile, route proxyRoute, deps transportDependencies) (_ net.Conn, err error) {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}
	spec, err := profile.Materialize()
	if err != nil {
		return nil, fmt.Errorf("upstream TLS profile: %w", err)
	}
	dialAddress := address
	if route.scheme != "" {
		dialAddress = route.address
	}
	conn, err := deps.dialContext(ctx, network, dialAddress)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err != nil {
			_ = conn.Close()
		}
	}()
	if route.scheme == "https" {
		proxyTLS := tls.Client(conn, &tls.Config{
			RootCAs:    deps.roots,
			ServerName: route.serverName,
			MinVersion: tls.VersionTLS12,
			NextProtos: []string{"http/1.1"},
		})
		if err = connectionPhase(ctx, tlsHandshakeTimeout, conn, proxyTLS.HandshakeContext); err != nil {
			return nil, fmt.Errorf("HTTPS proxy requires verified TLS and HTTP/1.1 CONNECT: %w", err)
		}
		if protocol := proxyTLS.ConnectionState().NegotiatedProtocol; protocol != "" && protocol != "http/1.1" {
			return nil, errors.New("HTTP/2 CONNECT is unsupported by upstream TLS profiles")
		}
		conn = proxyTLS
	}
	if route.scheme != "" {
		var tunnel net.Conn
		tunnel, err = connectTunnel(ctx, conn, route, address)
		if err != nil {
			return nil, err
		}
		conn = tunnel
	}
	guarded := &negotiationGuardConn{Conn: conn}
	cfg := &utls.Config{
		ServerName:       host,
		RootCAs:          deps.roots,
		MinVersion:       utls.VersionTLS12,
		MaxVersion:       utls.VersionTLS13,
		VerifyConnection: verifyProfileConnection,
	}
	uconn := utls.UClient(guarded, cfg, utls.HelloCustom)
	if err = uconn.ApplyPreset(spec); err != nil {
		return nil, fmt.Errorf("apply upstream TLS profile: %w", err)
	}
	cfg.MinVersion = max(cfg.MinVersion, utls.VersionTLS12)
	if cfg.MaxVersion == 0 || cfg.MaxVersion > utls.VersionTLS13 {
		cfg.MaxVersion = utls.VersionTLS13
	}
	if err = connectionPhase(ctx, tlsHandshakeTimeout, conn, uconn.HandshakeContext); err != nil {
		return nil, err
	}
	if err = guarded.guard.finish(); err != nil {
		return nil, err
	}
	cfg.Renegotiation = utls.RenegotiateNever
	return &stateConn{UConn: uconn}, nil
}

func connectionPhase(ctx context.Context, timeout time.Duration, conn net.Conn, fn func(context.Context) error) error {
	phaseCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	deadline, _ := phaseCtx.Deadline()
	if err := conn.SetDeadline(deadline); err != nil {
		_ = conn.Close()
		return err
	}
	closed := make(chan struct{})
	stop := context.AfterFunc(phaseCtx, func() {
		defer close(closed)
		_ = conn.Close()
	})
	err := fn(phaseCtx)
	if !stop() {
		<-closed
	}
	if ctxErr := phaseCtx.Err(); ctxErr != nil {
		err = ctxErr
	} else if !time.Now().Before(deadline) {
		err = context.DeadlineExceeded
	}
	if err == nil {
		err = conn.SetDeadline(time.Time{})
		if ctxErr := phaseCtx.Err(); ctxErr != nil {
			err = ctxErr
		} else if !time.Now().Before(deadline) {
			err = context.DeadlineExceeded
		}
	}
	if err != nil {
		_ = conn.Close()
	}
	return err
}

type bufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *bufferedConn) Read(p []byte) (int, error) {
	if n := c.reader.Buffered(); n > 0 {
		return c.reader.Read(p[:min(len(p), n)])
	}
	return c.Conn.Read(p)
}

func connectTunnel(ctx context.Context, conn net.Conn, route proxyRoute, target string) (net.Conn, error) {
	var reader *bufio.Reader
	err := connectionPhase(ctx, proxyConnectTimeout, conn, func(context.Context) error {
		header := make(http.Header)
		header.Set("User-Agent", "")
		if route.authorization != "" {
			header.Set("Proxy-Authorization", route.authorization)
		}
		req := &http.Request{Method: http.MethodConnect, URL: &url.URL{Opaque: target}, Host: target, Header: header}
		if err := req.Write(conn); err != nil {
			return err
		}
		reader = bufio.NewReader(&io.LimitedReader{R: conn, N: maxConnectHeaderBytes})
		resp, err := http.ReadResponse(reader, req)
		if err != nil {
			return fmt.Errorf("read proxy HTTP/1.1 CONNECT response (HTTP/2 CONNECT is unsupported): %w", err)
		}
		if resp.ProtoMajor != 1 {
			return errors.New("HTTP/2 CONNECT is unsupported by upstream TLS profiles")
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("proxy CONNECT failed with status %d", resp.StatusCode)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &bufferedConn{Conn: conn, reader: reader}, nil
}

func verifyProfileConnection(state utls.ConnectionState) error {
	if err := verifyTLSSelection(state.Version, state.CipherSuite); err != nil {
		return err
	}
	if state.DidResume {
		return errors.New("resumed upstream TLS profile handshakes are unsupported")
	}
	if state.NegotiatedProtocol != "" && state.NegotiatedProtocol != "h2" && state.NegotiatedProtocol != "http/1.1" {
		return errors.New("unsupported upstream TLS negotiated ALPN")
	}
	if state.PeerApplicationSettings != nil {
		return errors.New("negotiated upstream TLS ALPS settings are unsupported")
	}
	return nil
}

func verifyTLSSelection(version, cipher uint16) error {
	switch version {
	case utls.VersionTLS12:
		switch cipher {
		case utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305_SHA256:
			return nil
		}
	case utls.VersionTLS13:
		switch cipher {
		case utls.TLS_AES_128_GCM_SHA256, utls.TLS_AES_256_GCM_SHA384, utls.TLS_CHACHA20_POLY1305_SHA256:
			return nil
		}
	default:
		return errors.New("unsafe selected upstream TLS version")
	}
	return fmt.Errorf("unsafe selected upstream TLS cipher 0x%04x", cipher)
}

type stateConn struct {
	*utls.UConn
}

func (c *stateConn) ConnectionState() tls.ConnectionState {
	state := c.UConn.ConnectionState()
	return tls.ConnectionState{
		Version:                     state.Version,
		HandshakeComplete:           state.HandshakeComplete,
		DidResume:                   state.DidResume,
		CipherSuite:                 state.CipherSuite,
		NegotiatedProtocol:          state.NegotiatedProtocol,
		NegotiatedProtocolIsMutual:  state.NegotiatedProtocolIsMutual,
		ServerName:                  state.ServerName,
		PeerCertificates:            state.PeerCertificates,
		VerifiedChains:              state.VerifiedChains,
		SignedCertificateTimestamps: state.SignedCertificateTimestamps,
		OCSPResponse:                state.OCSPResponse,
		TLSUnique:                   state.TLSUnique,
		ECHAccepted:                 state.ECHAccepted,
	}
}
