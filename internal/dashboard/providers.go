package dashboard

import (
	"fmt"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/provider"
)

func (m *model) providerEndpoint(p config.Provider) string {
	base := p.BaseURL
	if m.snapshot.ProviderMetadata == nil {
		base = provider.EffectiveBaseURL(p.Type, base)
	}
	endpoint := dashrpc.DiagnosticURL(base)
	if endpoint == "" {
		return "unknown"
	}
	return endpoint
}

func (m *model) providerHost(p config.Provider) string {
	host := baseURLHost(m.providerEndpoint(p))
	if host == "" {
		return "unknown"
	}
	return host
}

func (m *model) probeMarks() map[string]string {
	marks := healthcheckMarks(m.snapshot.Healthchecks)
	for _, p := range m.providerList() {
		if _, ok := marks[p.Name]; ok {
			continue
		}
		d := m.snapshot.ProviderMetadata[p.Name]
		if m.snapshot.ProviderMetadata == nil {
			d = dashrpc.ProviderMetadata(p)
		}
		if d != nil && d.Probe == nil {
			marks[p.Name] = "-"
		}
	}
	return marks
}

func (m *model) clampProviderCursor() {
	m.providerCursor = clampInt(m.providerCursor, 0, max(0, m.providerDataRows()-1))
	m.providerScroll = clampInt(m.providerScroll, 0, m.maxProviderScroll())
	if m.providerCursor < m.providerScroll {
		m.providerScroll = m.providerCursor
	}
	if m.providerCursor >= m.providerScroll+m.providerVisibleRows() {
		m.providerScroll = m.providerCursor - m.providerVisibleRows() + 1
	}
}

func (m *model) handleProviderDetailKey(key string) {
	switch key {
	case "j", "down":
		m.providerDetailScroll++
	case "k", "up":
		m.providerDetailScroll = max(0, m.providerDetailScroll-1)
	case "pgdown", "shift+pgdown", "space":
		m.providerDetailScroll += m.detailVisibleRows()
	case "pgup", "shift+pgup":
		m.providerDetailScroll = max(0, m.providerDetailScroll-m.detailVisibleRows())
	case "g", "home":
		m.providerDetailScroll = 0
	case "G", "end":
		m.providerDetailScroll = 1 << 30
	}
}

func (m *model) providerDetailLines() []string {
	var p *config.Provider
	enabled := false
	for i, candidate := range m.providerList() {
		if candidate.Name == m.providerDetailName {
			p = &candidate
			enabled = i < len(m.snapshot.Providers)
			break
		}
	}
	if p == nil {
		return []string{"Provider removed from current catalog; back returns to the nearest row."}
	}
	display := p.DisplayName
	if display == "" {
		display = "(not set)"
	}
	endpoint := m.providerEndpoint(*p)
	state := "unknown"
	if known, healthy := healthKnown(m.health, p.Name); known {
		state = "healthy"
		if !healthy {
			state = "unhealthy"
		}
	}
	if !enabled {
		state = "disabled (not routed)"
	}
	lines := []string{"name: " + p.Name, "display: " + display, "type: " + string(p.Type),
		"endpoint (sanitized): " + endpoint, fmt.Sprintf("enabled: %t; routing health: %s", enabled, state)}
	d := m.snapshot.ProviderMetadata[p.Name]
	if m.snapshot.ProviderMetadata == nil {
		d = dashrpc.ProviderMetadata(*p)
	}
	if d == nil {
		lines = append(lines, "settings/probe configuration: unknown (snapshot metadata unavailable)")
	} else {
		lines = append(lines, "upstream header timeout: "+d.HeaderTimeout)
		if h := d.Probe; h != nil {
			lines = append(lines, fmt.Sprintf("probe: %s %s; expected HTTP %d", h.Method, dashrpc.DiagnosticURL(h.Path), h.ExpectedStatus),
				fmt.Sprintf("probe interval: %s; timeout: %s; failure/success thresholds: %d/%d", h.Interval, h.Timeout, h.FailureThreshold, h.SuccessThreshold))
		} else {
			lines = append(lines, "probe: not configured")
		}
	}
	found := false
	for _, h := range m.snapshot.Healthchecks {
		if h.Provider != p.Name {
			continue
		}
		found = true
		probeState := "pending (no completed check)"
		if !h.Configured {
			probeState = "not configured"
		} else if h.Checked {
			probeState = "healthy (threshold state)"
			if !h.Healthy {
				probeState = "unhealthy (threshold state)"
			}
		}
		reason := dashrpc.DiagnosticReason(h.Message)
		if reason == "" {
			reason = "unavailable"
		}
		age := "unknown"
		if !h.LastChecked.IsZero() {
			delta := m.now.Sub(h.LastChecked)
			if delta < 0 {
				age = "unknown (clock ahead)"
			} else {
				age = delta.Truncate(time.Second).String() + " ago"
			}
		}
		status := "unavailable"
		if h.StatusCode > 0 {
			status = fmt.Sprint(h.StatusCode)
		}
		lines = append(lines, "probe state: "+probeState, "probe path (sanitized): "+dashrpc.DiagnosticURL(h.Path),
			fmt.Sprintf("last probe HTTP: %s; reason: %s", status, reason), "last checked: "+age)
	}
	if !found {
		switch {
		case !enabled:
			lines = append(lines, "probe status: inactive (provider disabled)")
		case d != nil && d.Probe == nil:
			lines = append(lines, "probe status: not applicable")
		default:
			lines = append(lines, "probe status/last checked: unknown (status unavailable)")
		}
	}
	if providerCountersAvailable(m.snapshot.Usage) {
		ps := accounting.ProviderSummary{}
		for _, candidate := range m.snapshot.Usage.ProviderSummaries() {
			if candidate.Provider == p.Name {
				ps = candidate
				break
			}
		}
		lines = append(lines, fmt.Sprintf("provider-wide lifetime: reqs:%s errors:%s 429:%s tokens:%s", comma(ps.Requests), comma(ps.Errors), comma(ps.Throttled), comma(ps.TotalTokens)))
	} else {
		lines = append(lines, "provider-wide lifetime counters: unavailable")
	}
	lines = append(lines, fmt.Sprintf("configured models: %d", len(p.Models)))
	for _, model := range p.Models {
		lines = append(lines, "model: "+p.Name+"/"+model.Name)
		if m.snapshot.ModelMetadata != nil && m.snapshot.ModelMetadata[p.Name+"/"+model.Name] == nil {
			lines = append(lines, "  upstream/protocol/capabilities: unknown (snapshot metadata unavailable)")
			continue
		}
		upstream := model.UpstreamName
		if upstream == "" {
			upstream = model.Name
		}
		protocol := string(model.Protocol)
		if protocol == "" {
			protocol = "provider-native (not separately configured)"
		}
		caps := make([]string, len(model.Capabilities))
		for i, cap := range model.Capabilities {
			caps[i] = string(cap)
		}
		capabilities := strings.Join(caps, ", ")
		if capabilities == "" {
			capabilities = "unknown (not supplied)"
		}
		lines = append(lines, "  upstream: "+upstream, "  protocol: "+protocol, "  capabilities: "+capabilities)
		if model.DisplayName != "" {
			lines = append(lines, "  display: "+model.DisplayName)
		}
	}
	return lines
}

func renderProviderDetail(m *model, width, height int) string {
	frame, inner := paneBox(width, height, true)
	var raw []string
	for _, line := range m.providerDetailLines() {
		raw = append(raw, wrapText(line, inner)...)
	}
	visible := max(1, height-4)
	m.providerDetailScroll = clampInt(m.providerDetailScroll, 0, max(0, len(raw)-visible))
	end := min(len(raw), m.providerDetailScroll+visible)
	lines := []string{"PROVIDER " + m.providerDetailName}
	lines = append(lines, raw[m.providerDetailScroll:end]...)
	lines = append(lines, fmt.Sprintf("lines %d-%d/%d · j/k PgUp/PgDn Home/End", m.providerDetailScroll+1, end, len(raw)))
	return frame.Render(strings.Join(lines, "\n"))
}
