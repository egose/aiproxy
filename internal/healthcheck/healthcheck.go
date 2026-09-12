package healthcheck

import (
	"context"
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
	mu           sync.Mutex
	tracker      *providerhealth.Tracker
	metrics      *observability.Metrics
	version      string
	client       *http.Client
	probes       map[string]*probe
	fingerprints map[string]string
	providers    map[string]config.Provider
	status       map[string]Status
}

type probe struct {
	cancel context.CancelFunc
	done   chan struct{}
}

func New(tracker *providerhealth.Tracker, metrics *observability.Metrics, version string) *Manager {
	if version == "" {
		version = "dev"
	}
	return &Manager{
		tracker:      tracker,
		metrics:      metrics,
		version:      version,
		client:       &http.Client{},
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

func (m *Manager) SetProviders(catalog config.Catalog) {
	if m == nil {
		return
	}
	want := make(map[string]config.Provider)
	for _, p := range catalog.Providers() {
		if p.Healthcheck == nil {
			continue
		}
		want[p.Name] = p
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, p := range want {
		fp := fingerprint(p)
		m.providers[name] = p
		if existing, ok := m.probes[name]; ok {
			if existingFingerprint, ok := m.fingerprints[name]; ok && existingFingerprint == fp {
				continue
			}
			m.stopLocked(name, existing)
		}
		ctx, cancel := context.WithCancel(context.Background())
		pr := &probe{cancel: cancel, done: make(chan struct{})}
		m.probes[name] = pr
		m.fingerprints[name] = fp
		if _, ok := m.status[name]; !ok {
			m.status[name] = Status{Provider: name, Configured: true, Path: p.Healthcheck.Path}
		} else {
			st := m.status[name]
			st.Configured = true
			st.Path = p.Healthcheck.Path
			m.status[name] = st
		}
		go m.loop(ctx, pr, p)
	}
	for name, pr := range m.probes {
		if _, ok := want[name]; !ok {
			m.stopLocked(name, pr)
			delete(m.providers, name)
			if m.metrics != nil {
				m.metrics.RemoveHealthcheck(name)
			}
		}
	}
	for name, st := range m.status {
		if _, ok := want[name]; !ok {
			st = Status{Provider: name, Configured: false}
			if !st.Configured && !st.Checked {
				delete(m.status, name)
				continue
			}
			m.status[name] = st
		}
	}
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
	hc := p.Healthcheck
	m.check(ctx, p)
	ticker := time.NewTicker(hc.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.mu.Lock()
			current, ok := m.providers[p.Name]
			m.mu.Unlock()
			if ok && current.Healthcheck != nil {
				p = current
				hc = current.Healthcheck
			}
			m.check(ctx, p)
		}
	}
}

func (m *Manager) check(ctx context.Context, p config.Provider) {
	hc := p.Healthcheck
	if hc == nil {
		return
	}
	start := time.Now()
	code, body, err := m.probe(ctx, p)
	latency := time.Since(start)
	healthy, message := evaluate(hc, code, body, err)
	m.mu.Lock()
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
	m.mu.Unlock()
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
	hc := p.Healthcheck
	base := provider.EffectiveBaseURL(p.Type, p.BaseURL)
	if base == "" {
		return 0, "", fmt.Errorf("no base_url for healthcheck")
	}
	target := strings.TrimRight(base, "/") + hc.Path
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
	req.Header.Set("User-Agent", "aiproxy/"+m.version)
	req.Header.Set("Accept", "*/*")
	if hc.SendAuthorization && p.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.APIKey)
	}
	resp, err := m.client.Do(req)
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
	m.mu.Lock()
	defer m.mu.Unlock()
	for name, pr := range m.probes {
		pr.cancel()
		delete(m.probes, name)
	}
}
