package dashboard

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/accounting"
)

func requestIdentity(e accounting.Event) accounting.Event { return e }

func requestPublicModel(e accounting.Event) string {
	if e.PublicModel != "" {
		return e.PublicModel
	}
	return e.Model
}

func (m *model) recentRequests() []accounting.Event {
	if m.snapshot == nil || m.snapshot.Usage == nil {
		return nil
	}
	all := m.snapshot.Usage.Recent(recentLimit)
	rows := make([]accounting.Event, 0, len(all))
	for i := len(all) - 1; i >= 0; i-- {
		e := all[i]
		if m.requestErrorsOnly && e.StatusCode < 400 {
			continue
		}
		if metadataMatch(m.queries[bottomTabRequests], map[string]string{
			"id": e.RequestID, "tenant": e.Tenant, "client": e.Client, "model": requestPublicModel(e),
			"resolved": e.UpstreamModel, "provider": e.Provider, "status": fmt.Sprint(e.StatusCode), "op": e.Operation,
		}) {
			rows = append(rows, e)
		}
	}
	return rows
}

func (m *model) clampRequestCursor() {
	n := len(m.recentRequests())
	m.requestCursor = clampInt(m.requestCursor, 0, max(0, n-1))
	visible := m.bottomVisibleRows()
	m.requestOffset = clampInt(m.requestOffset, max(0, m.requestCursor-visible+1), m.requestCursor)
	m.requestOffset = min(m.requestOffset, max(0, n-visible))
}

func (m *model) handleRequestKey(key string) (bool, tea.Cmd) {
	if m.requestDetail != nil {
		switch key {
		case "l":
			return true, m.correlateRequest(bottomTabLogs)
		case "v":
			return true, m.correlateRequest(bottomTabPayload)
		}
		m.metadataScrollKey(key)
		return true, nil
	}
	switch key {
	case "e":
		m.requestErrorsOnly = !m.requestErrorsOnly
		m.requestCursor, m.requestOffset = 0, 0
	case "j", "down":
		m.requestCursor++
	case "k", "up":
		m.requestCursor--
	case "pgdown":
		m.requestCursor += m.bottomVisibleRows()
	case "pgup":
		m.requestCursor -= m.bottomVisibleRows()
	case "g", "home":
		m.requestCursor = 0
	case "G", "end":
		m.requestCursor = len(m.recentRequests()) - 1
	case "enter":
		rows := m.recentRequests()
		if len(rows) > 0 {
			e := rows[m.requestCursor]
			m.requestDetail = &e
			m.metadataScroll = 0
		}
	default:
		return false, nil
	}
	m.clampRequestCursor()
	return true, nil
}

func renderRequests(m *model, width, height int) string {
	frame, _ := paneBox(width, height, m.focus == focusBottom)
	filter := "all"
	if m.requestErrorsOnly {
		filter = "errors"
	}
	rows := []string{fmt.Sprintf("REQUESTS completions only · cap %d · %s", recentLimit, filter), m.searchLabel(bottomTabRequests)}
	all := m.recentRequests()
	if len(all) == 0 {
		rows = append(rows, "no matches / no recent completion metadata")
	}
	for i := m.requestOffset; i < min(len(all), m.requestOffset+m.bottomVisibleRows()); i++ {
		e := all[i]
		mark := "  "
		if i == m.requestCursor {
			mark = "▸ "
		}
		if e.Truncated != (accounting.RecentTruncation{}) {
			mark += "[truncated] "
		}
		rows = append(rows, fmt.Sprintf("%s%s %d %s %s · client:%s tenant:%s", mark, e.Timestamp.Format("15:04:05"), e.StatusCode, metadataText(orDash(e.RequestID)), metadataText(orDash(requestPublicModel(e))), metadataText(orDash(e.Client)), metadataText(orDash(e.Tenant))))
	}
	return frame.Render(strings.Join(rows, "\n"))
}

func (m *model) metadataScrollKey(key string) {
	switch key {
	case "j", "down":
		m.metadataScroll++
	case "k", "up":
		m.metadataScroll = max(0, m.metadataScroll-1)
	case "pgdown":
		m.metadataScroll += m.detailVisibleRows()
	case "pgup":
		m.metadataScroll = max(0, m.metadataScroll-m.detailVisibleRows())
	case "home", "g":
		m.metadataScroll = 0
	case "end", "G":
		m.metadataScroll = 1 << 30
	}
}

func (m *model) metadataLines() (string, []string) {
	if e := m.requestDetail; e != nil {
		id := metadataText(e.RequestID)
		if id == "" {
			id = "unavailable (older snapshot)"
		}
		title := "REQUEST completion · [l] logs [v] payload"
		guidance := ""
		if fields := e.Truncated.Fields(); len(fields) > 0 {
			guidance = "Truncated fields (retained prefixes): " + strings.Join(fields, ", ") + ". Search covers retained text only."
		}
		if e.Truncated.RequestID {
			title = "REQUEST completion · truncated ID: correlation unavailable"
			id += " [truncated; not an exact correlation key]"
		}
		lines := []string{
			"request ID: " + id, "tenant: \"" + metadataText(e.Tenant) + "\"", "client: \"" + metadataText(e.Client) + "\"",
			"public model: " + metadataText(orDash(requestPublicModel(*e))), "resolved provider: " + metadataText(orDash(e.Provider)), "resolved model: " + metadataText(orDash(e.UpstreamModel)),
			fmt.Sprintf("operation: %s · HTTP status: %d", metadataText(e.Operation), e.StatusCode),
			fmt.Sprintf("completed: %s · duration: %s", e.Timestamp.Format(time.RFC3339Nano), e.Duration),
			fmt.Sprintf("tokens input/output/total: %d / %d / %d", e.PromptTokens, e.CompletionTokens, e.TotalTokens),
			fmt.Sprintf("tokens cached/create/read: %d / %d / %d", e.CachedTokens, e.CacheCreationTokens, e.CacheReadTokens),
			"Recorded completion only; no attempt/in-flight history. Zero tokens may be unreported.",
			"Process-local last <=200 completions; no timed retention. Empty resolved fields mean unavailable.",
			m.correlationNotice,
		}
		if guidance != "" {
			lines = append([]string{guidance}, lines...)
		}
		return title, lines
	}
	s := m.usageDetail
	return "USAGE identity · [n/N] next/previous group", []string{
		"tenant: \"" + metadataText(s.Tenant) + "\"", "client: \"" + metadataText(s.Client) + "\"", "model: " + metadataText(s.Model),
		fmt.Sprintf("operation: %s · status: %d", metadataText(s.Operation), s.StatusCode),
		fmt.Sprintf("count: %d · input/output/total: %d / %d / %d", s.Count, s.PromptTokens, s.CompletionTokens, s.TotalTokens),
		"Exact tenant/client/model/operation/status group; captured from displayed retained usage.",
	}
}

func renderMetadataDetail(m *model, width, height int) string {
	frame, inner := paneBox(width, height, true)
	title, lines := m.metadataLines()
	var raw []string
	for _, line := range lines {
		raw = append(raw, wrapText(line, inner)...)
	}
	visible := max(1, height-4)
	m.metadataScroll = clampInt(m.metadataScroll, 0, max(0, len(raw)-visible))
	end := min(len(raw), m.metadataScroll+visible)
	rows := append([]string{title}, raw[m.metadataScroll:end]...)
	rows = append(rows, fmt.Sprintf("lines %d-%d/%d", m.metadataScroll+1, end, len(raw)))
	return frame.Render(strings.Join(rows, "\n"))
}

func (m *model) nextUsageDetail(delta int) {
	rows := m.filteredSummaries()
	for i, s := range rows {
		if usageIdentity(s) == usageIdentity(*m.usageDetail) {
			s = rows[(i+delta+len(rows))%len(rows)]
			m.usageDetail = &s
			m.metadataScroll = 0
			return
		}
	}
}
