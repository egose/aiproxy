package healthcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/providerhealth"
)

type probeTransportFunc func(*http.Request) (*http.Response, error)

func (f probeTransportFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type heldFailureBackend struct {
	started chan struct{}
	release chan struct{}
	healthy atomic.Bool
}

func (b *heldFailureBackend) MarkSuccess(context.Context, string) error {
	b.healthy.Store(true)
	return nil
}

func (b *heldFailureBackend) MarkFailure(context.Context, string, time.Duration) error {
	close(b.started)
	<-b.release
	b.healthy.Store(false)
	return nil
}

func (b *heldFailureBackend) IsHealthy(context.Context, string) (bool, error) {
	return b.healthy.Load(), nil
}

func (b *heldFailureBackend) Snapshot(context.Context, []string) (map[string]bool, error) {
	return map[string]bool{"local": b.healthy.Load()}, nil
}

func (b *heldFailureBackend) Close() error { return nil }

func TestProbePublicationCompletesBeforeReplacement(t *testing.T) {
	b := &heldFailureBackend{started: make(chan struct{}), release: make(chan struct{})}
	tracker := providerhealth.NewWithBackend(nil, config.ProviderHealth{}, b)
	defer tracker.Close()
	m := New(tracker, nil, "test")
	defer m.Close()
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(b.release) })
	m.client = &http.Client{Transport: probeTransportFunc(func(r *http.Request) (*http.Response, error) {
		code := http.StatusOK
		if r.URL.Path == "/old" {
			code = http.StatusServiceUnavailable
		}
		return &http.Response{StatusCode: code, Body: http.NoBody, Header: make(http.Header)}, nil
	})}
	old := testProvider("local", "http://probe.test", fastCheck("/old"))
	old.Healthcheck.Interval = time.Hour
	m.SetProviders(config.NewCatalog([]config.Provider{old}, nil, nil))
	select {
	case <-b.started:
	case <-time.After(3 * time.Second):
		t.Fatal("old health publication did not start")
	}
	next := testProvider("local", "http://probe.test", fastCheck("/new"))
	next.Healthcheck.Interval = time.Hour
	replaced := make(chan struct{})
	go func() {
		m.SetProviders(config.NewCatalog([]config.Provider{next}, nil, nil))
		close(replaced)
	}()
	select {
	case <-replaced:
		t.Error("replacement activated while old health publication was outstanding")
	case <-time.After(100 * time.Millisecond):
	}
	releaseOnce.Do(func() { close(b.release) })
	select {
	case <-replaced:
	case <-time.After(3 * time.Second):
		t.Fatal("replacement did not activate after publication completed")
	}
	waitForCondition(t, "replacement health publication", func() bool {
		st, ok := m.StatusFor("local")
		return ok && st.Path == "/new" && st.Checked && st.Healthy && tracker.IsHealthy("local")
	})
}

func TestReplacedProbeCannotOverwriteActiveHealth(t *testing.T) {
	tracker := providerhealth.New(nil, config.ProviderHealth{})
	defer tracker.Close()
	m := New(tracker, nil, "test")
	defer m.Close()
	started, release := make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	defer releaseOnce.Do(func() { close(release) })
	m.client = &http.Client{Transport: probeTransportFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/old" {
			close(started)
			<-r.Context().Done()
			<-release
			return nil, context.Canceled
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Header: make(http.Header)}, nil
	})}
	old := testProvider("local", "http://probe.test", fastCheck("/old"))
	old.Healthcheck.Interval = time.Hour
	m.SetProviders(config.NewCatalog([]config.Provider{old}, nil, nil))
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("old probe did not start")
	}
	m.mu.Lock()
	retired := m.probes["local"]
	m.mu.Unlock()
	next := testProvider("local", "http://probe.test", fastCheck("/new"))
	next.Healthcheck.Interval = time.Hour
	m.SetProviders(config.NewCatalog([]config.Provider{next}, nil, nil))
	waitForCondition(t, "new probe success", func() bool {
		st, ok := m.StatusFor("local")
		return ok && st.Path == "/new" && st.Checked && st.Healthy
	})
	releaseOnce.Do(func() { close(release) })
	select {
	case <-retired.done:
	case <-time.After(3 * time.Second):
		t.Fatal("retired probe did not stop")
	}
	st, ok := m.StatusFor("local")
	if !ok || st.Path != "/new" || !st.Healthy || !tracker.IsHealthy("local") {
		t.Fatalf("retired probe overwrote active health: %+v", st)
	}
}

func testProvider(name, baseURL string, hc *config.ProviderHealthcheck) config.Provider {
	return config.Provider{
		Type:        config.ProviderTypeOpenAICompatible,
		Name:        name,
		BaseURL:     baseURL,
		Healthcheck: hc,
	}
}

func fastCheck(path string) *config.ProviderHealthcheck {
	return &config.ProviderHealthcheck{
		Path:             path,
		Method:           "GET",
		ExpectedStatus:   200,
		ExpectedBody:     "*",
		Interval:         20 * time.Millisecond,
		Timeout:          2 * time.Second,
		FailureThreshold: 1,
		SuccessThreshold: 1,
	}
}

func waitForStatus(t *testing.T, m *Manager, provider string) Status {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if st, ok := m.StatusFor(provider); ok && st.Checked {
			return st
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for healthcheck status for %q", provider)
	return Status{}
}

func waitForCondition(t *testing.T, what string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestManagerHealthyServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	provider := testProvider("local", srv.URL, fastCheck("/health"))
	tracker.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	m := New(tracker, nil, "test")
	defer m.Close()
	m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))

	st := waitForStatus(t, m, "local")
	if !st.Healthy {
		t.Fatalf("expected healthy, got %+v", st)
	}
	if !tracker.IsHealthy("local") {
		t.Fatalf("expected tracker healthy")
	}
	if len(m.Snapshot()) != 1 {
		t.Fatalf("expected 1 status, got %d", len(m.Snapshot()))
	}
}

func TestManagerUnhealthyServerMarksTracker(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	provider := testProvider("local", srv.URL, fastCheck("/health"))
	tracker.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	m := New(tracker, nil, "test")
	defer m.Close()
	m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))

	waitForCondition(t, "unhealthy status", func() bool {
		st, ok := m.StatusFor("local")
		return ok && st.Checked && !st.Healthy
	})
	if tracker.IsHealthy("local") {
		t.Fatalf("expected tracker unhealthy after failed healthcheck")
	}
}

func TestManagerFailureThresholdCountsConsecutively(t *testing.T) {
	var calls atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	hc := fastCheck("/health")
	hc.FailureThreshold = 3
	provider := testProvider("local", srv.URL, hc)
	tracker.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	m := New(tracker, nil, "test")
	defer m.Close()
	m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))

	waitForStatus(t, m, "local")
	if !tracker.IsHealthy("local") {
		t.Fatalf("single failure must not mark unhealthy with threshold 3")
	}
	waitForCondition(t, "unhealthy status", func() bool {
		st, ok := m.StatusFor("local")
		return ok && st.ConsecutiveFailures >= 3 && !st.Healthy
	})
	if tracker.IsHealthy("local") {
		t.Fatalf("expected tracker unhealthy after 3 consecutive failures")
	}
}

func TestManagerBodyMatching(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ready","version":"1.2.3"}`))
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	match := testProvider("match", srv.URL, fastCheck("/health"))
	match.Healthcheck.ExpectedBody = "ready"
	nomatch := testProvider("nomatch", srv.URL, fastCheck("/health"))
	nomatch.Healthcheck.ExpectedBody = "not-present"
	catalog := config.NewCatalog([]config.Provider{match, nomatch}, nil, nil)
	tracker.SetProviders(catalog)
	m := New(tracker, nil, "test")
	defer m.Close()
	m.SetProviders(catalog)

	waitForCondition(t, "nomatch unhealthy", func() bool {
		st, ok := m.StatusFor("nomatch")
		return ok && st.Checked && !st.Healthy
	})
	st, _ := m.StatusFor("match")
	if !st.Checked || !st.Healthy {
		t.Fatalf("expected substring match healthy, got %+v", st)
	}
}

func TestManagerSendsAuthorizationOnlyWhenEnabled(t *testing.T) {
	var gotAuth atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer secret" {
			gotAuth.Add(1)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	without := testProvider("without", srv.URL, fastCheck("/health"))
	without.APIKey = "secret"
	with := testProvider("with", srv.URL, fastCheck("/health"))
	with.APIKey = "secret"
	with.Healthcheck.SendAuthorization = true
	catalog := config.NewCatalog([]config.Provider{without, with}, nil, nil)
	tracker.SetProviders(catalog)
	m := New(tracker, nil, "test")
	defer m.Close()
	m.SetProviders(catalog)

	waitForStatus(t, m, "with")
	waitForStatus(t, m, "without")
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && gotAuth.Load() == 0 {
		time.Sleep(5 * time.Millisecond)
	}
	if gotAuth.Load() == 0 {
		t.Fatalf("expected Authorization header on send_authorization probe")
	}
}

func TestManagerHeadSkipsBodyCheck(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	hc := fastCheck("/health")
	hc.Method = "HEAD"
	hc.ExpectedBody = "anything"
	provider := testProvider("local", srv.URL, hc)
	tracker.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	m := New(tracker, nil, "test")
	defer m.Close()
	m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))

	st := waitForStatus(t, m, "local")
	if !st.Healthy {
		t.Fatalf("HEAD must skip body matching, got %+v", st)
	}
}

func TestManagerUserAgent(t *testing.T) {
	var gotUA atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA.Store(r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	provider := testProvider("local", srv.URL, fastCheck("/health"))
	tracker.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	m := New(tracker, nil, "9.9.9")
	defer m.Close()
	m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))

	waitForStatus(t, m, "local")
	if ua, _ := gotUA.Load().(string); ua != "aiproxy/9.9.9" {
		t.Fatalf("user agent = %q, want aiproxy/9.9.9", ua)
	}
}

func TestManagerUserAgentOverride(t *testing.T) {
	var gotUA atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA.Store(r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	provider := testProvider("local", srv.URL, fastCheck("/health"))
	provider.UserAgent = "custom/1.0"
	tracker.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	m := New(tracker, nil, "9.9.9")
	defer m.Close()
	m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))

	waitForStatus(t, m, "local")
	if ua, _ := gotUA.Load().(string); ua != "custom/1.0" {
		t.Fatalf("user agent = %q, want custom/1.0", ua)
	}
}

func TestManagerRemoveProviderStopsProbing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	tracker := providerhealth.New(nil, config.ProviderHealth{})
	provider := testProvider("local", srv.URL, fastCheck("/health"))
	tracker.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	m := New(tracker, nil, "test")
	defer m.Close()
	m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	waitForStatus(t, m, "local")

	m.SetProviders(config.NewCatalog(nil, nil, nil))
	waitForCondition(t, "status cleanup", func() bool {
		_, ok := m.StatusFor("local")
		return !ok
	})
	if len(m.Snapshot()) != 0 {
		t.Fatalf("expected empty snapshot, got %+v", m.Snapshot())
	}
}

func TestManagerUnconfiguredProviderHasNoProbe(t *testing.T) {
	tracker := providerhealth.New(nil, config.ProviderHealth{})
	provider := testProvider("local", "http://127.0.0.1:1", nil)
	tracker.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	m := New(tracker, nil, "test")
	defer m.Close()
	m.SetProviders(config.NewCatalog([]config.Provider{provider}, nil, nil))
	time.Sleep(50 * time.Millisecond)
	if _, ok := m.StatusFor("local"); ok {
		t.Fatalf("unconfigured provider must have no healthcheck status")
	}
}

func TestEvaluate(t *testing.T) {
	hc := &config.ProviderHealthcheck{ExpectedStatus: 200, ExpectedBody: "*"}
	if ok, _ := evaluate(hc, 200, "anything", nil); !ok {
		t.Errorf("expected ok")
	}
	if ok, _ := evaluate(hc, 500, "x", nil); ok {
		t.Errorf("expected status mismatch failure")
	}
	hc.ExpectedBody = "ready"
	if ok, _ := evaluate(hc, 200, `{"status":"ready"}`, nil); !ok {
		t.Errorf("expected substring match")
	}
	if ok, _ := evaluate(hc, 200, `{"status":"down"}`, nil); ok {
		t.Errorf("expected body mismatch failure")
	}
}
