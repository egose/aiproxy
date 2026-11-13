package healthcheck

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/upstreamhttp"
)

const maxBodyBytes = 256 << 10

type Status struct {
	Provider             string
	Configured           bool
	Checked              bool
	Healthy              bool
	StatusCode           int
	Message              string
	Path                 string
	LastChecked          time.Time
	ConsecutiveFailures  int
	ConsecutiveSuccesses int
}

type Manager struct {
	updateMu     sync.Mutex
	mu           sync.Mutex
	tracker      *providerhealth.Tracker
	metrics      *observability.Metrics
	version      string
	factory      func(upstreamhttp.Options) (upstreamhttp.Transport, error)
	closed       bool
	probes       map[string]*probe
	fingerprints map[string]string
	providers    map[string]config.Provider
	status       map[string]Status
}

type probe struct {
	cancel    context.CancelFunc
	done      chan struct{}
	client    *http.Client
	transport *upstreamhttp.RetiringTransport
}

type Prepared struct {
	mu      sync.Mutex
	manager *Manager
	entries map[string]preparedProbe
}

type preparedProbe struct {
	provider    config.Provider
	fingerprint string
	client      *http.Client
	transport   *upstreamhttp.RetiringTransport
}

type probeStart struct {
	ctx      context.Context
	probe    *probe
	provider config.Provider
}

func New(tracker *providerhealth.Tracker, metrics *observability.Metrics, version string) *Manager {
	if version == "" {
		version = "dev"
	}
	return &Manager{
		tracker:      tracker,
		metrics:      metrics,
		version:      version,
		probes:       make(map[string]*probe),
		fingerprints: make(map[string]string),
		providers:    make(map[string]config.Provider),
		status:       make(map[string]Status),
	}
}

func (m *Manager) SetTracker(tracker *providerhealth.Tracker) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tracker = tracker
}

func (m *Manager) Prepare(catalog config.Catalog, factory func(upstreamhttp.Options) (upstreamhttp.Transport, error)) (_ *Prepared, err error) {
	if m == nil {
		return nil, errors.New("healthcheck manager is unavailable")
	}
	m.updateMu.Lock()
	prepared := &Prepared{manager: m, entries: make(map[string]preparedProbe)}
	defer func() {
		if err != nil {
			prepared.Close()
		}
	}()
	m.mu.Lock()
	closed := m.closed
	m.mu.Unlock()
	if closed {
		return nil, errors.New("healthcheck manager is closed")
	}
	if factory == nil {
		factory = m.factory
	}
	if factory == nil {
		factory = upstreamhttp.NewTransport
	}
	for _, provider := range catalog.Providers() {
		if provider.Healthcheck == nil {
			continue
		}
		entry, err := prepareProbe(provider, factory)
		if err != nil {
			return nil, err
		}
		prepared.entries[provider.Name] = entry
	}
	return prepared, nil
}

func prepareProbe(provider config.Provider, factory func(upstreamhttp.Options) (upstreamhttp.Transport, error)) (preparedProbe, error) {
	profile, err := upstreamhttp.ProfileForProvider(provider)
	if err != nil {
		return preparedProbe{}, err
	}
	provider.TLSProfile = profile
	transport, err := factory(upstreamhttp.Options{Profile: profile, ResponseHeaderTimeout: 0})
	if err != nil {
		if transport != nil {
			transport.CloseIdleConnections()
		}
		return preparedProbe{}, fmt.Errorf("provider %q: healthcheck transport: %w", provider.Name, err)
	}
	if transport == nil {
		return preparedProbe{}, fmt.Errorf("provider %q: healthcheck transport factory returned no transport", provider.Name)
	}
	retiring := upstreamhttp.NewRetiringTransport(transport)
	return preparedProbe{provider: provider, fingerprint: fingerprint(provider), client: &http.Client{Transport: retiring, CheckRedirect: upstreamhttp.CheckRedirect}, transport: retiring}, nil
}

func (p *Prepared) Close() {
	if p == nil {
		return
	}
	p.mu.Lock()
	manager, entries := p.manager, p.entries
	p.manager, p.entries = nil, nil
	p.mu.Unlock()
	if manager == nil {
		return
	}
	defer manager.updateMu.Unlock()
	for _, entry := range entries {
		entry.transport.Retire()
	}
}

func (p *Prepared) Activate(tracker *providerhealth.Tracker) error {
	if p == nil {
		return errors.New("healthcheck candidate is unavailable")
	}
	p.mu.Lock()
	m, want := p.manager, p.entries
	p.manager, p.entries = nil, nil
	p.mu.Unlock()
	if m == nil {
		return errors.New("healthcheck candidate has already been consumed")
	}
	defer m.updateMu.Unlock()
	m.mu.Lock()
	m.tracker = tracker
	var retired []*upstreamhttp.RetiringTransport
	var start []probeStart
	for name, entry := range want {
		provider, fp := entry.provider, entry.fingerprint
		if existing, ok := m.probes[name]; ok {
			if existingFingerprint, ok := m.fingerprints[name]; ok && existingFingerprint == fp {
				m.providers[name] = provider
				retired = append(retired, entry.transport)
				continue
			}
			m.stopLocked(name, existing)
			retired = append(retired, existing.transport)
		}
		ctx, cancel := context.WithCancel(context.Background())
		pr := &probe{cancel: cancel, done: make(chan struct{}), client: entry.client, transport: entry.transport}
		m.probes[name] = pr
		m.fingerprints[name] = fp
		m.providers[name] = provider
		if _, ok := m.status[name]; !ok {
			m.status[name] = Status{Provider: name, Configured: true, Path: provider.Healthcheck.Path}
		} else {
			st := m.status[name]
			st.Configured = true
			st.Path = provider.Healthcheck.Path
			m.status[name] = st
		}
		start = append(start, probeStart{ctx: ctx, probe: pr, provider: provider})
	}
	for name, pr := range m.probes {
		if _, ok := want[name]; !ok {
			m.stopLocked(name, pr)
			retired = append(retired, pr.transport)
			if m.metrics != nil {
				m.metrics.RemoveHealthcheck(name)
			}
		}
	}
	for name := range m.status {
		if _, ok := want[name]; !ok {
			delete(m.status, name)
		}
	}
	m.mu.Unlock()
	for _, transport := range retired {
		transport.Retire()
	}
	for _, next := range start {
		go m.loop(next.ctx, next.probe, next.provider)
	}
	return nil
}

func (m *Manager) SetProviders(catalog config.Catalog) error {
	prepared, err := m.Prepare(catalog, nil)
	if err != nil {
		return err
	}
	m.mu.Lock()
	tracker := m.tracker
	m.mu.Unlock()
	return prepared.Activate(tracker)
}

func (m *Manager) stopLocked(name string, pr *probe) {
	pr.cancel()
	delete(m.probes, name)
	delete(m.fingerprints, name)
	delete(m.providers, name)
}

func fingerprint(p config.Provider) string {
	hc := p.Healthcheck
	if hc == nil {
		return ""
	}
	return strings.Join([]string{
		string(p.Type),
		p.TLSProfile.Identity(),
		provider.EffectiveBaseURL(p.Type, p.BaseURL),
		hc.Path,
		strings.ToUpper(hc.Method),
		fmt.Sprintf("%d|%s|%s|%s|%d|%d|%t|%s",
			hc.ExpectedStatus, hc.ExpectedBody,
			hc.Interval.String(), hc.Timeout.String(),
			hc.FailureThreshold, hc.SuccessThreshold,
			hc.SendAuthorization, p.APIKey),
	}, "\x00")
}

func (m *Manager) loop(ctx context.Context, pr *probe, p config.Provider) {
	defer close(pr.done)
	defer pr.transport.Retire()
	hc := p.Healthcheck
	m.check(ctx, pr, p)
	ticker := time.NewTicker(hc.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			current, ok := m.providers[p.Name]
			active := m.probes[p.Name] == pr
			m.mu.Unlock()
			if !active || !ok || current.Healthcheck == nil {
				return
			}
			p = current
			m.check(ctx, pr, p)
		}
	}
}

func (m *Manager) check(ctx context.Context, pr *probe, p config.Provider) {
	hc := p.Healthcheck
	if hc == nil || ctx.Err() != nil {
		return
	}
	start := time.Now()
	code, body, err := m.probeWithClient(ctx, p, pr.client)
	latency := time.Since(start)
	healthy, message := evaluate(hc, code, body, err)
	m.mu.Lock()
	defer m.mu.Unlock()
	if ctx.Err() != nil || m.probes[p.Name] != pr {
		return
	}
	st := m.status[p.Name]
	st.Configured = true
	st.Provider = p.Name
	st.Path = hc.Path
	st.LastChecked = time.Now()
	if healthy {
		st.ConsecutiveSuccesses++
		st.ConsecutiveFailures = 0
	} else {
		st.ConsecutiveFailures++
		st.ConsecutiveSuccesses = 0
	}
	st.StatusCode = code
	st.Message = message
	transitioned := false
	if healthy && st.ConsecutiveSuccesses >= hc.SuccessThreshold {
		if !st.Checked || !st.Healthy {
			transitioned = true
		}
		st.Healthy = true
	} else if !healthy && st.ConsecutiveFailures >= hc.FailureThreshold {
		if !st.Checked || st.Healthy {
			transitioned = true
		}
		st.Healthy = false
	} else if !st.Checked {
		st.Healthy = true
	}
	st.Checked = true
	m.status[p.Name] = st
	tracker := m.tracker
	metrics := m.metrics
	if metrics != nil {
		metrics.RecordHealthcheck(p.Name, healthy, latency)
		if transitioned || (st.Checked && ((healthy && st.ConsecutiveSuccesses == hc.SuccessThreshold) || (!healthy && st.ConsecutiveFailures == hc.FailureThreshold))) {
			metrics.SetHealthcheckUp(p.Name, st.Healthy)
		}
	}
	if tracker == nil {
		return
	}
	if healthy && st.ConsecutiveSuccesses >= hc.SuccessThreshold {
		tracker.MarkSuccess(p.Name)
	} else if !healthy && st.ConsecutiveFailures >= hc.FailureThreshold {
		tracker.MarkFailure(p.Name)
	}
}

func evaluate(hc *config.ProviderHealthcheck, code int, body string, err error) (bool, string) {
	if err != nil {
		return false, err.Error()
	}
	if code != hc.ExpectedStatus {
		return false, fmt.Sprintf("unexpected status %d (want %d)", code, hc.ExpectedStatus)
	}
	if strings.ToUpper(hc.Method) == "HEAD" {
		return true, "ok"
	}
	if hc.ExpectedBody != "" && hc.ExpectedBody != "*" && !strings.Contains(body, hc.ExpectedBody) {
		return false, "body mismatch"
	}
	return true, "ok"
}

func (m *Manager) probe(ctx context.Context, p config.Provider) (int, string, error) {
	if err := ctx.Err(); err != nil {
		return 0, "", err
	}
	factory := m.factory
	if factory == nil {
		factory = upstreamhttp.NewTransport
	}
	entry, err := prepareProbe(p, factory)
	if err != nil {
		return 0, "", err
	}
	defer entry.transport.Retire()
	return m.probeWithClient(ctx, entry.provider, entry.client)
}

func (m *Manager) probeWithClient(ctx context.Context, p config.Provider, client *http.Client) (int, string, error) {
	hc := p.Healthcheck
	if hc == nil {
		return 0, "", errors.New("healthcheck is not configured")
	}
	if config.IsHealthcheckAbsoluteURL(hc.Path) {
		return m.probeURL(ctx, p, hc.Path, client)
	}
	base := provider.EffectiveBaseURL(p.Type, p.BaseURL)
	if base == "" {
		return 0, "", fmt.Errorf("no base_url for healthcheck")
	}
	return m.probeURL(ctx, p, config.ResolveHealthcheckURL(base, hc.Path), client)
}

func (m *Manager) probeURL(ctx context.Context, p config.Provider, target string, client *http.Client) (int, string, error) {
	hc := p.Healthcheck
	method := strings.ToUpper(hc.Method)
	if method == "" {
		method = "GET"
	}
	timeout := hc.Timeout
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, method, target, nil)
	if err != nil {
		return 0, "", err
	}
	if p.UserAgent != "" {
		req.Header.Set("User-Agent", p.UserAgent)
	} else {
		req.Header.Set("User-Agent", "aiproxy/"+m.version)
	}
	req.Header.Set("Accept", "*/*")
	if hc.SendAuthorization && p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := upstreamhttp.Do(client, req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	var body string
	if method != "HEAD" {
		data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
		if err != nil {
			return resp.StatusCode, "", err
		}
		if len(data) > maxBodyBytes {
			data = data[:maxBodyBytes]
		}
		body = string(data)
	}
	return resp.StatusCode, body, nil
}

func (m *Manager) Snapshot() []Status {
	if m == nil {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Status, 0, len(m.status))
	for _, st := range m.status {
		out = append(out, st)
	}
	return out
}

func (m *Manager) StatusFor(provider string) (Status, bool) {
	if m == nil {
		return Status{}, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	st, ok := m.status[provider]
	return st, ok
}

func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.updateMu.Lock()
	defer m.updateMu.Unlock()
	m.mu.Lock()
	m.closed = true
	var retired []*upstreamhttp.RetiringTransport
	var stopped []*probe
	for name, pr := range m.probes {
		m.stopLocked(name, pr)
		retired = append(retired, pr.transport)
		stopped = append(stopped, pr)
	}
	m.mu.Unlock()
	for _, transport := range retired {
		transport.Retire()
	}
	for _, pr := range stopped {
		<-pr.done
	}
}
