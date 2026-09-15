package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/payloadlog"
)

const (
	payloadFetchLimit   = 100
	payloadDetailMaxOut = 64 << 10
)

type PayloadSummary = dashrpc.PayloadSummary

type PayloadFetcher interface {
	ListPayloads(ctx context.Context, limit int, errorsOnly bool) (dashrpc.PayloadList, error)
	GetPayload(ctx context.Context, requestID string) (string, error)
}

type payloadListMsg struct {
	entries []dashrpc.PayloadSummary
	enabled bool
	err     string
}

type payloadDetailMsg struct {
	requestID string
	pretty    string
	err       string
}

func fetchPayloadsCmd(f PayloadFetcher, limit int, errorsOnly bool) tea.Cmd {
	if f == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		list, err := f.ListPayloads(ctx, limit, errorsOnly)
		if err != nil {
			return payloadListMsg{err: err.Error()}
		}
		if list.Enabled {
			return payloadListMsg{enabled: true, entries: list.Payloads}
		}
		return payloadListMsg{enabled: false}
	}
}

func fetchPayloadDetailCmd(f PayloadFetcher, requestID string) tea.Cmd {
	if f == nil || requestID == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		pretty, err := f.GetPayload(ctx, requestID)
		msg := payloadDetailMsg{requestID: requestID}
		if err != nil {
			msg.err = err.Error()
			return msg
		}
		msg.pretty = pretty
		return msg
	}
}

func (m *model) payloadDetailOpen() bool {
	return m.payloadDetailID != ""
}

func (m *model) requestPayloads() tea.Cmd {
	if m.payloadFetcher == nil || m.payloadLoading {
		return nil
	}
	m.payloadLoading = true
	return fetchPayloadsCmd(m.payloadFetcher, payloadFetchLimit, m.payloadErrorsOnly)
}

func (m *model) applyPayloadList(msg payloadListMsg) {
	m.payloadLoading = false
	if msg.err != "" {
		m.payloadErr = msg.err
		m.dirty = true
		return
	}
	m.payloadErr = ""
	m.payloadEnabled = msg.enabled
	if !msg.enabled {
		m.payloads = nil
		m.payloadKnown = false
		m.payloadCursor = 0
		m.payloadOffset = 0
		m.dirty = true
		return
	}
	m.payloadKnown = true
	m.payloads = msg.entries
	m.clampPayloadCursor()
	m.dirty = true
}

func (m *model) applyPayloadDetail(msg payloadDetailMsg) {
	if msg.requestID != m.payloadPendingID {
		return
	}
	m.payloadPendingID = ""
	if msg.err != "" {
		m.payloadDetail = ""
		m.payloadDetailErr = msg.err
	} else {
		m.payloadDetail = msg.pretty
		m.payloadDetailErr = ""
	}
	m.payloadDetailID = msg.requestID
	m.payloadDetailScroll = 0
	m.dirty = true
}

func (m *model) clampPayloadCursor() {
	n := len(m.payloads)
	if n == 0 {
		m.payloadCursor = 0
		m.payloadOffset = 0
		return
	}
	if m.payloadCursor < 0 {
		m.payloadCursor = 0
	}
	if m.payloadCursor >= n {
		m.payloadCursor = n - 1
	}
	visible := m.bottomVisibleRows()
	if m.payloadOffset > m.payloadCursor {
		m.payloadOffset = m.payloadCursor
	}
	if m.payloadOffset < m.payloadCursor-visible+1 {
		m.payloadOffset = m.payloadCursor - visible + 1
	}
	if m.payloadOffset < 0 {
		m.payloadOffset = 0
	}
	maxOff := n - visible
	if maxOff < 0 {
		maxOff = 0
	}
	if m.payloadOffset > maxOff {
		m.payloadOffset = maxOff
	}
}

func (m *model) movePayloadCursor(delta int) bool {
	if len(m.payloads) == 0 {
		return false
	}
	next := m.payloadCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.payloads) {
		next = len(m.payloads) - 1
	}
	if next == m.payloadCursor {
		return false
	}
	m.payloadCursor = next
	m.clampPayloadCursor()
	return true
}

func (m *model) payloadCursorTop() bool {
	if m.payloadCursor == 0 && m.payloadOffset == 0 {
		return false
	}
	m.payloadCursor = 0
	m.payloadOffset = 0
	return true
}

func (m *model) payloadCursorBottom() bool {
	n := len(m.payloads)
	if n == 0 || m.payloadCursor == n-1 {
		return false
	}
	m.payloadCursor = n - 1
	m.clampPayloadCursor()
	return true
}

func (m *model) payloadAt(i int) PayloadSummary {
	if m.payloadOldestFirst {
		return m.payloads[len(m.payloads)-1-i]
	}
	return m.payloads[i]
}

func (m *model) orderedPayloads() []PayloadSummary {
	if !m.payloadOldestFirst {
		return m.payloads
	}
	out := make([]PayloadSummary, len(m.payloads))
	for i, j := 0, len(m.payloads)-1; i < len(m.payloads); i, j = i+1, j-1 {
		out[i] = m.payloads[j]
	}
	return out
}

func (m *model) handlePayloadKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "s", "e":
		m.payloadErrorsOnly = !m.payloadErrorsOnly
		m.payloadCursor = 0
		m.payloadOffset = 0
		m.payloadKnown = false
		return true, m.requestPayloads()
	case "o":
		if m.payloadDetailOpen() {
			return false, nil
		}
		m.togglePayloadOrder()
		return true, nil
	case "j", "down":
		if m.payloadDetailOpen() {
			m.payloadDetailScroll++
			return true, nil
		}
		return m.movePayloadCursor(1), nil
	case "k", "up":
		if m.payloadDetailOpen() {
			if m.payloadDetailScroll > 0 {
				m.payloadDetailScroll--
				return true, nil
			}
			return false, nil
		}
		return m.movePayloadCursor(-1), nil
	case "g", "home":
		if m.payloadDetailOpen() {
			if m.payloadDetailScroll == 0 {
				return false, nil
			}
			m.payloadDetailScroll = 0
			return true, nil
		}
		return m.payloadCursorTop(), nil
	case "G", "end":
		if m.payloadDetailOpen() {
			m.payloadDetailScroll = 1 << 30
			return true, nil
		}
		return m.payloadCursorBottom(), nil
	case "enter":
		if m.payloadDetailOpen() {
			return true, nil
		}
		if m.payloadPendingID != "" {
			return true, nil
		}
		if len(m.payloads) == 0 || m.payloadFetcher == nil {
			return false, nil
		}
		id := m.payloadAt(m.payloadCursor).RequestID
		if id == "" {
			return false, nil
		}
		m.payloadPendingID = id
		m.payloadDetail = ""
		m.payloadDetailErr = ""
		m.payloadDetailID = ""
		m.payloadDetailScroll = 0
		return true, fetchPayloadDetailCmd(m.payloadFetcher, id)
	case "r":
		if m.payloadDetailOpen() {
			return false, nil
		}
		m.payloadKnown = false
		return true, m.requestPayloads()
	}
	return false, nil
}

func payloadStatusCell(status int) string {
	if status == 0 {
		return "   0"
	}
	return fmt.Sprintf("%4d", status)
}

func payloadRow(s dashrpc.PayloadSummary, methodW, modelW, pathW int) string {
	ts := s.Timestamp
	if len(ts) > 19 {
		if idx := strings.Index(ts, "T"); idx >= 0 && idx+9 <= len(ts) {
			ts = ts[idx+1 : idx+9]
		} else {
			ts = ts[:19]
		}
	}
	model := s.PublicModel
	if model == "" {
		model = s.Provider + "/" + s.UpstreamModel
	}
	ms := fmt.Sprintf("%dms", s.DurationMs)
	return dataRow([]string{
		truncate(ts, 8),
		truncate(s.Method, methodW),
		payloadStatusCell(s.Status),
		fmt.Sprintf("%8s", truncate(ms, 8)),
		truncate(model, modelW),
		truncate(s.Path, pathW),
	}, []int{8, methodW, 4, 8, modelW, pathW})
}

func renderPayloads(m *model, width, height int) string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width - 2).
		Height(height - 2)
	if m.focus == focusBottom && m.bottomTab == bottomTabPayload {
		borderStyle = borderStyle.BorderForeground(lipgloss.Color("#38BDF8"))
	}
	filter := "all"
	if m.payloadErrorsOnly {
		filter = "errs-only"
	}
	order := "newest-first"
	if m.payloadOldestFirst {
		order = "oldest-first"
	}
	title := fmt.Sprintf("PAYLOADS %s (%s) [o]rder", order, filter)
	rows := []string{title}
	if m.payloadFetcher == nil {
		rows = append(rows, "payload viewer unavailable")
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if m.snapshot != nil && !m.snapshot.PayloadEnabled && !m.payloadEnabled && len(m.payloads) == 0 && !m.payloadKnown {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("payload log disabled on server (enable payload_log in config)"))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if m.payloadErr != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("fetch failed: "+truncate(m.payloadErr, width-4)))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if !m.payloadKnown {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("loading…"))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if len(m.payloads) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("no payload entries yet"))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	inner := width - 2
	methodW, modelW, pathW := 6, 24, 24
	fixed := 8 + 1 + methodW + 1 + 4 + 1 + 8 + 1 + modelW + 1 + pathW
	if fixed > inner {
		shrink := fixed - inner
		for shrink > 0 && pathW > 8 {
			pathW--
			shrink--
		}
		for shrink > 0 && modelW > 10 {
			modelW--
			shrink--
		}
	}
	rows = append(rows, headerStyle.Render(fitRow(headerCells([]col{{"AT", 8}, {"METHOD", methodW}, {"ST", 4}, {"DUR", 8}, {"MODEL", modelW}, {"PATH", pathW}}), inner)))
	visible := m.bottomVisibleRows()
	view := m.orderedPayloads()
	start := m.payloadOffset
	end := start + visible
	if end > len(view) {
		end = len(view)
	}
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true)
	for i := start; i < end; i++ {
		line := fitRow(payloadRow(view[i], methodW, modelW, pathW), inner-2)
		if i == m.payloadCursor {
			line = cursorStyle.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		rows = append(rows, line)
	}
	if end < len(view) {
		rows = append(rows, fmt.Sprintf("… %d more (j/k move)", len(view)-end))
	}
	return borderStyle.Render(strings.Join(rows, "\n"))
}

func renderPayloadDetail(m *model, width, height int) string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#38BDF8")).
		Width(width - 2).
		Height(height - 2)
	inner := width - 4
	if inner < 10 {
		inner = 10
	}
	title := "PAYLOAD " + m.payloadDetailID
	if m.payloadPendingID != "" {
		title = "PAYLOAD " + m.payloadPendingID + " (loading…)"
	}
	lines := []string{title}
	if m.payloadPendingID != "" {
		lines = append(lines, "loading…")
		return borderStyle.Render(strings.Join(lines, "\n"))
	}
	if m.payloadDetailErr != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("fetch failed: "+m.payloadDetailErr))
		lines = append(lines, "[esc] back")
		return borderStyle.Render(strings.Join(lines, "\n"))
	}
	body := m.payloadDetail
	if body == "" {
		body = "(empty)"
	}
	raw := strings.Split(body, "\n")
	visible := height - 5
	if visible < 1 {
		visible = 1
	}
	start := m.payloadDetailScroll
	if start > len(raw)-1 {
		start = len(raw) - 1
	}
	if start < 0 {
		start = 0
	}
	m.payloadDetailScroll = start
	end := start + visible
	if end > len(raw) {
		end = len(raw)
	}
	for _, l := range raw[start:end] {
		lines = append(lines, truncate(l, inner))
	}
	if end < len(raw) {
		lines = append(lines, fmt.Sprintf("… %d more lines (j/k scroll, esc back)", len(raw)-end))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render("[j/k] scroll [esc] back"))
	}
	return borderStyle.Render(strings.Join(lines, "\n"))
}

func prettyPayload(raw []byte) string {
	return payloadlog.Pretty(raw, payloadDetailMaxOut)
}
