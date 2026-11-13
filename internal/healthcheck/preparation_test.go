package healthcheck

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

type preparedRequest struct {
	profile string
	agent   string
}

type preparedTransport struct {
	profile string
	mu      sync.Mutex
	seen    []preparedRequest
	closed  atomic.Int32
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (t *preparedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	t.seen = append(t.seen, preparedRequest{profile: t.profile, agent: req.UserAgent()})
	t.mu.Unlock()
	if t.started != nil {
		t.once.Do(func() { close(t.started) })
		select {
		case <-t.release:
		case <-req.Context().Done():
			return nil, req.Context().Err()
		}
	}
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ready"))}, nil
}

func (t *preparedTransport) CloseIdleConnections() { t.closed.Add(1) }

func (t *preparedTransport) requests() []preparedRequest {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]preparedRequest(nil), t.seen...)
}

func TestPrepareIsSideEffectFreeAndFailurePreservesActiveProbes(t *testing.T) {
	tracker := providerhealth.New(nil, config.ProviderHealth{})
	defer tracker.Close()
	m := New(tracker, nil, "test")
	defer m.Close()
	var transports []*preparedTransport
	var fail atomic.Bool
	failure := errors.New("synthetic health preparation failure")
	factory := func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		if options.ResponseHeaderTimeout != 0 {
			t.Error("health acquired an inference header timeout")
		}
		if fail.Load() && options.Profile.CanonicalName() == "safari-16.0" {
			return nil, failure
		}
		transport := &preparedTransport{profile: options.Profile.CanonicalName()}
		transports = append(transports, transport)
		return transport, nil
	}
	old := testProvider("local", "http://not-contacted.test", fastCheck("/health"))
	old.Healthcheck.Interval = time.Hour
	old.TLSClientHello = "chrome"
	catalog := config.NewCatalog([]config.Provider{old}, nil, nil)
	tracker.SetProviders(catalog)
	prepared, err := m.Prepare(catalog, factory)
	if err != nil {
		t.Fatal(err)
	}
	if len(transports[0].requests()) != 0 || len(m.Snapshot()) != 0 {
		t.Fatal("preparation started probes or mutated status")
	}
	if prepared.entries["local"].client.Timeout != 0 {
		t.Fatal("health request-context deadline was replaced by a client timeout")
	}
	if err := prepared.Activate(tracker); err != nil {
		t.Fatal(err)
	}
	prepared.Close()
	status := waitForStatus(t, m, "local")
	if !status.Healthy || transports[0].closed.Load() != 0 {
		t.Fatal("activated candidate was retired by its cleanup")
	}
	m.mu.Lock()
	active := m.probes["local"]
	m.mu.Unlock()
	other := old
	other.Name = "other"
	next := old
	next.TLSClientHello = "safari"
	fail.Store(true)
	if _, err := m.Prepare(config.NewCatalog([]config.Provider{other, next}, nil, nil), factory); !errors.Is(err, failure) {
		t.Fatalf("preparation failure: %v", err)
	}
	m.mu.Lock()
	if m.probes["local"] != active || len(m.probes) != 1 {
		t.Error("failed candidate replaced active probes")
	}
	m.mu.Unlock()
	if transports[0].closed.Load() != 0 || len(transports) != 2 || transports[1].closed.Load() == 0 || len(transports[1].requests()) != 0 {
		t.Fatal("failed candidate leaked or retired active clients")
	}
	current, _ := m.StatusFor("local")
	if current != status || !tracker.IsHealthy("local") {
		t.Fatal("failed preparation mutated current status/tracker")
	}
	fail.Store(false)
	prepared, err = m.Prepare(catalog, factory)
	if err != nil {
		t.Fatal("failed preparation retained its update lock")
	}
	prepared.Close()
	prepared.Close()
}

func TestUnchangedProbeRetainsOwnedClientAndRefreshesSnapshot(t *testing.T) {
	m := New(nil, nil, "test")
	defer m.Close()
	var transports []*preparedTransport
	factory := func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		transport := &preparedTransport{profile: options.Profile.CanonicalName()}
		transports = append(transports, transport)
		return transport, nil
	}
	provider := testProvider("local", "http://not-contacted.test", fastCheck("/health"))
	provider.TLSClientHello = "chrome"
	provider.UserAgent = "health-old"
	prepared, err := m.Prepare(config.NewCatalog([]config.Provider{provider}, nil, nil), factory)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Activate(nil); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, m, "local")
	m.mu.Lock()
	old := m.probes["local"]
	m.mu.Unlock()
	provider.TLSClientHello = "chrome-133"
	provider.UserAgent = "health-new"
	provider.UpstreamHeaderTimeout = time.Millisecond
	for range 5 {
		prepared, err = m.Prepare(config.NewCatalog([]config.Provider{provider}, nil, nil), factory)
		if err != nil {
			t.Fatal(err)
		}
		if err := prepared.Activate(nil); err != nil {
			t.Fatal(err)
		}
		m.mu.Lock()
		if m.probes["local"] != old || len(m.probes) != 1 {
			t.Error("unchanged effective profile replaced its probe/client")
		}
		m.mu.Unlock()
	}
	waitForCondition(t, "snapshot refresh on retained client", func() bool {
		seen := transports[0].requests()
		return len(seen) > 1 && seen[len(seen)-1].agent == "health-new"
	})
	if transports[0].closed.Load() != 0 {
		t.Fatal("unchanged probe was tied to a retired candidate generation")
	}
	for _, unused := range transports[1:] {
		if unused.closed.Load() == 0 || len(unused.requests()) != 0 {
			t.Fatal("unused candidate transport retained resources")
		}
	}
}

func TestProfileChangeCancelsOldProbeAndNeverPairsNewSnapshotWithOldClient(t *testing.T) {
	m := New(nil, nil, "test")
	defer m.Close()
	oldTransport := &preparedTransport{profile: "chrome-133", started: make(chan struct{}), release: make(chan struct{})}
	factory := func(options upstreamhttp.Options) (upstreamhttp.Transport, error) {
		if options.Profile.CanonicalName() == "chrome-133" {
			return oldTransport, nil
		}
		return &preparedTransport{profile: options.Profile.CanonicalName()}, nil
	}
	provider := testProvider("local", "http://not-contacted.test", fastCheck("/health"))
	provider.TLSClientHello = "chrome"
	provider.UserAgent = "chrome-snapshot"
	prepared, err := m.Prepare(config.NewCatalog([]config.Provider{provider}, nil, nil), factory)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Activate(nil); err != nil {
		t.Fatal(err)
	}
	select {
	case <-oldTransport.started:
	case <-time.After(3 * time.Second):
		t.Fatal("old probe did not start")
	}
	m.mu.Lock()
	old := m.probes["local"]
	m.mu.Unlock()
	provider.TLSClientHello = "safari"
	provider.UserAgent = "safari-snapshot"
	prepared, err = m.Prepare(config.NewCatalog([]config.Provider{provider}, nil, nil), factory)
	if err != nil {
		t.Fatal(err)
	}
	if err := prepared.Activate(nil); err != nil {
		t.Fatal(err)
	}
	waitForStatus(t, m, "local")
	select {
	case <-old.done:
	case <-time.After(3 * time.Second):
		t.Fatal("replaced probe did not finish after cancellation")
	}
	for _, seen := range oldTransport.requests() {
		if seen.agent != "chrome-snapshot" {
			t.Fatal("old client received a new provider snapshot")
		}
	}
	if oldTransport.closed.Load() == 0 {
		t.Fatal("old probe transport was not retired")
	}
}

func TestPreparedLifecycleAndInvalidSelectors(t *testing.T) {
	var absent *Manager
	if _, err := absent.Prepare(config.Catalog{}, nil); err == nil {
		t.Fatal("absent manager accepted preparation")
	}
	m := New(nil, nil, "test")
	defer m.Close()
	provider := testProvider("local", "http://not-contacted.test", fastCheck("/health"))
	provider.TLSClientHello = "unsupported"
	if err := m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil)); err == nil {
		t.Fatal("invalid selector fell back to native")
	}
	prepared, err := m.Prepare(config.Catalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	prepared.Close()
	if err := prepared.Activate(nil); err == nil {
		t.Fatal("discarded candidate activated")
	}
	var empty *Prepared
	empty.Close()
	if err := empty.Activate(nil); err == nil {
		t.Fatal("absent candidate activated")
	}
	partial := &preparedTransport{}
	failure := errors.New("partial factory failure")
	provider.TLSClientHello = "native"
	catalog := config.NewCatalog([]config.Provider{provider}, nil, nil)
	if _, err := m.Prepare(catalog, func(upstreamhttp.Options) (upstreamhttp.Transport, error) { return partial, failure }); !errors.Is(err, failure) || partial.closed.Load() != 1 {
		t.Fatal("partial factory failure leaked its transport")
	}
	if _, err := m.Prepare(catalog, func(upstreamhttp.Options) (upstreamhttp.Transport, error) { return nil, nil }); err == nil {
		t.Fatal("missing transport accepted")
	}
	m.Close()
	if _, err := m.Prepare(config.Catalog{}, nil); err == nil {
		t.Fatal("closed manager accepted a new generation")
	}
}

func TestProbeContextTimeoutAndBodyBounds(t *testing.T) {
	for _, method := range []string{"GET", "HEAD"} {
		m := New(nil, nil, "test")
		provider := testProvider("local", "http://not-contacted.test", fastCheck("/health"))
		provider.Healthcheck.Method = method
		body := strings.Repeat("x", maxBodyBytes+1) + "not-observable"
		m.factory = func(upstreamhttp.Options) (upstreamhttp.Transport, error) {
			return probeTransportFunc(func(req *http.Request) (*http.Response, error) {
				deadline, ok := req.Context().Deadline()
				if !ok || time.Until(deadline) > 5*time.Second {
					t.Error("probe request-context timeout disappeared")
				}
				return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body))}, nil
			}), nil
		}
		provider.Healthcheck.Timeout = 0
		code, got, err := m.probe(context.Background(), provider)
		want := maxBodyBytes
		if method == "HEAD" {
			want = 0
		}
		if err != nil || code != 200 || len(got) != want || strings.Contains(got, "not-observable") {
			t.Fatalf("probe body bound: %d %d %v", code, len(got), err)
		}
	}
}
