package dashboard

import (
	"context"
	"fmt"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/egose/aiproxy/internal/dashrpc"
)

type BlockSummary = dashrpc.BlockSummary
type BlockCapture = dashrpc.BlockCapture

type BlockFetcher interface {
	ListBlocks(ctx context.Context) (dashrpc.BlockList, error)
	GetBlock(ctx context.Context, blockID string) (dashrpc.BlockCapture, error)
	DecideBlock(ctx context.Context, blockID, action string, shas []string) (dashrpc.BlockDecisionResponse, error)
}

type blockListMsg struct {
	entries []dashrpc.BlockSummary
	enabled bool
	err     string
}

type blockDetailMsg struct {
	blockID string
	capture dashrpc.BlockCapture
	err     string
}

type blockDecisionMsg struct {
	blockID string
	action  string
	count   int
	err     string
}

func fetchBlocksCmd(f BlockFetcher) tea.Cmd {
	if f == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		list, err := f.ListBlocks(ctx)
		if err != nil {
			return blockListMsg{err: err.Error()}
		}
		if list.Enabled {
			return blockListMsg{enabled: true, entries: list.Blocks}
		}
		return blockListMsg{enabled: false}
	}
}

func fetchBlockDetailCmd(f BlockFetcher, blockID string) tea.Cmd {
	if f == nil || blockID == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		capture, err := f.GetBlock(ctx, blockID)
		msg := blockDetailMsg{blockID: blockID}
		if err != nil {
			msg.err = err.Error()
			return msg
		}
		msg.capture = capture
		return msg
	}
}

func fetchBlockDecisionCmd(f BlockFetcher, blockID, action string, shas []string) tea.Cmd {
	if f == nil || blockID == "" || action == "" || len(shas) == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		res, err := f.DecideBlock(ctx, blockID, action, shas)
		msg := blockDecisionMsg{blockID: blockID, action: action}
		if err != nil {
			msg.err = err.Error()
			return msg
		}
		msg.count = res.Count
		return msg
	}
}

func blockFindingSHAs(c dashrpc.BlockCapture) []string {
	var out []string
	seen := map[string]struct{}{}
	for _, f := range c.Findings {
		if f.SecretSHA == "" {
			continue
		}
		if _, dup := seen[f.SecretSHA]; dup {
			continue
		}
		seen[f.SecretSHA] = struct{}{} // pragma: allowlist secret
		out = append(out, f.SecretSHA)
	}
	return out
}

func (m *model) blockDetailOpen() bool {
	return m.blockDetailID != ""
}

func (m *model) requestBlocks() tea.Cmd {
	if m.blockFetcher == nil || m.blockLoading {
		return nil
	}
	m.blockLoading = true
	return fetchBlocksCmd(m.blockFetcher)
}

func (m *model) applyBlockList(msg blockListMsg) {
	m.blockLoading = false
	if msg.err != "" {
		m.blockErr = msg.err
		m.dirty = true
		return
	}
	m.blockErr = ""
	m.blockEnabled = msg.enabled
	if !msg.enabled {
		m.blocks = nil
		m.blockKnown = false
		m.blockCursor = 0
		m.blockOffset = 0
		m.dirty = true
		return
	}
	m.blockKnown = true
	m.blocks = msg.entries
	m.clampBlockCursor()
	m.dirty = true
}

func (m *model) applyBlockDetail(msg blockDetailMsg) {
	if msg.blockID != m.blockPendingID {
		return
	}
	m.blockPendingID = ""
	if msg.err != "" {
		m.blockDetail = dashrpc.BlockCapture{}
		m.blockDetailErr = msg.err
	} else {
		m.blockDetail = msg.capture
		m.blockDetailErr = ""
	}
	m.blockDetailID = msg.blockID
	m.blockDetailScroll = 0
	m.blockDecisionPending = ""
	m.blockDecisionMsg = ""
	m.blockDecisionErr = ""
	m.dirty = true
}

func (m *model) clampBlockCursor() {
	n := len(m.blocks)
	if n == 0 {
		m.blockCursor = 0
		m.blockOffset = 0
		return
	}
	if m.blockCursor < 0 {
		m.blockCursor = 0
	}
	if m.blockCursor >= n {
		m.blockCursor = n - 1
	}
	visible := m.bottomVisibleRows()
	if m.blockOffset > m.blockCursor {
		m.blockOffset = m.blockCursor
	}
	if m.blockOffset < m.blockCursor-visible+1 {
		m.blockOffset = m.blockCursor - visible + 1
	}
	if m.blockOffset < 0 {
		m.blockOffset = 0
	}
	maxOff := n - visible
	if maxOff < 0 {
		maxOff = 0
	}
	if m.blockOffset > maxOff {
		m.blockOffset = maxOff
	}
}

func (m *model) blockAt(i int) BlockSummary {
	return m.blocks[i]
}

func (m *model) moveBlockCursor(delta int) bool {
	if len(m.blocks) == 0 {
		return false
	}
	next := m.blockCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.blocks) {
		next = len(m.blocks) - 1
	}
	if next == m.blockCursor {
		return false
	}
	m.blockCursor = next
	m.clampBlockCursor()
	return true
}

func (m *model) blockCursorTop() bool {
	if m.blockCursor == 0 && m.blockOffset == 0 {
		return false
	}
	m.blockCursor = 0
	m.blockOffset = 0
	return true
}

func (m *model) blockCursorBottom() bool {
	n := len(m.blocks)
	if n == 0 || m.blockCursor == n-1 {
		return false
	}
	m.blockCursor = n - 1
	m.clampBlockCursor()
	return true
}

func (m *model) handleBlockKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.blockDetailOpen() {
			m.blockDetailScroll++
			return true, nil
		}
		return m.moveBlockCursor(1), nil
	case "k", "up":
		if m.blockDetailOpen() {
			if m.blockDetailScroll > 0 {
				m.blockDetailScroll--
				return true, nil
			}
			return false, nil
		}
		return m.moveBlockCursor(-1), nil
	case "pgdown", "shift+pgdown":
		if m.blockDetailOpen() {
			return m.scrollBlockDetail(m.detailVisibleRows()), nil
		}
		return m.moveBlockCursor(m.bottomVisibleRows()), nil
	case "pgup", "shift+pgup":
		if m.blockDetailOpen() {
			return m.scrollBlockDetail(-m.detailVisibleRows()), nil
		}
		return m.moveBlockCursor(-m.bottomVisibleRows()), nil
	case "g", "home":
		if m.blockDetailOpen() {
			if m.blockDetailScroll == 0 {
				return false, nil
			}
			m.blockDetailScroll = 0
			return true, nil
		}
		return m.blockCursorTop(), nil
	case "G", "end":
		if m.blockDetailOpen() {
			m.blockDetailScroll = 1 << 30
			return true, nil
		}
		return m.blockCursorBottom(), nil
	case "enter":
		if m.blockDetailOpen() {
			return true, nil
		}
		if m.blockPendingID != "" {
			return true, nil
		}
		if len(m.blocks) == 0 || m.blockFetcher == nil {
			return false, nil
		}
		id := m.blockAt(m.blockCursor).BlockID
		if id == "" {
			return false, nil
		}
		m.blockPendingID = id
		m.blockDetail = dashrpc.BlockCapture{}
		m.blockDetailErr = ""
		m.blockDetailID = ""
		m.blockDetailScroll = 0
		m.blockDecisionPending = ""
		m.blockDecisionMsg = ""
		m.blockDecisionErr = ""
		return true, fetchBlockDetailCmd(m.blockFetcher, id)
	case "r":
		if m.blockDetailOpen() {
			return false, nil
		}
		m.blockKnown = false
		return true, m.requestBlocks()
	case "a", "1":
		return m.requestBlockDecision("allow")
	case "s", "2":
		return m.requestBlockDecision("redact")
	case "d", "3":
		return m.requestBlockDecision("deny")
	}
	return false, nil
}

func (m *model) requestBlockDecision(action string) (bool, tea.Cmd) {
	if !m.blockDetailOpen() || m.blockDetailErr != "" || m.blockDecisionPending != "" {
		return false, nil
	}
	shas := blockFindingSHAs(m.blockDetail)
	if len(shas) == 0 || m.blockFetcher == nil {
		return false, nil
	}
	m.blockDecisionPending = action
	m.blockDecisionMsg = ""
	m.blockDecisionErr = ""
	m.dirty = true
	return true, fetchBlockDecisionCmd(m.blockFetcher, m.blockDetailID, action, shas)
}

func (m *model) applyBlockDecision(msg blockDecisionMsg) {
	if msg.blockID != m.blockDetailID {
		return
	}
	m.blockDecisionPending = ""
	if msg.err != "" {
		m.blockDecisionErr = msg.err
		m.blockDecisionMsg = ""
	} else {
		m.blockDecisionErr = ""
		m.blockDecisionMsg = fmt.Sprintf("%s recorded for %d secret(s)", msg.action, msg.count)
	}
	m.dirty = true
}

func blockRow(s dashrpc.BlockSummary, idW, rulesW, modelW int) string {
	ts := s.Timestamp
	if len(ts) > 19 {
		if idx := strings.Index(ts, "T"); idx >= 0 && idx+9 <= len(ts) {
			ts = ts[idx+1 : idx+9]
		} else {
			ts = ts[:19]
		}
	}
	return dataRow([]string{
		truncate(ts, 8),
		truncate(s.BlockID, idW),
		truncate(strings.Join(s.RuleIDs, ","), rulesW),
		fmt.Sprintf("%4d", s.FindingCount),
		truncate(s.PublicModel, modelW),
	}, []int{8, idW, rulesW, 4, modelW})
}

func renderBlocks(m *model, width, height int) string {
	borderStyle, inner := paneBox(width, height, m.focus == focusBottom && m.bottomTab == bottomTabBlocks)
	title := "BLOCKS newest-first (take-once detail) [r]efresh"
	rows := []string{title}
	if m.blockFetcher == nil {
		rows = append(rows, "block viewer unavailable")
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if m.blockErr != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("fetch failed: "+truncate(m.blockErr, inner)))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if !m.blockKnown && !m.blockEnabled && len(m.blocks) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("quarantine disabled on server (enable ingress_guardrails quarantine in config)"))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if !m.blockKnown {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("loading…"))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if len(m.blocks) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("no quarantined blocks"))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	idContent, rulesContent, modelContent := len("BLOCK_ID"), len("RULES"), len("MODEL")
	for _, b := range m.blocks {
		idContent = max(idContent, runeLen(b.BlockID))
		rulesContent = max(rulesContent, runeLen(strings.Join(b.RuleIDs, ",")))
		modelContent = max(modelContent, runeLen(b.PublicModel))
	}
	got := flexWidths([]flexCol{
		{content: 8, min: 8, max: 8},
		{content: idContent, min: 8, max: 40, flex: 3},
		{content: rulesContent, min: 5, max: 32, flex: 2},
		{content: 4, min: 4, max: 4},
		{content: modelContent, min: 5, max: 40, flex: 2},
	}, inner)
	idW, rulesW, modelW := got[1], got[2], got[4]
	rows = append(rows, headerStyle.Render(fitRow(headerCells([]col{{"AT", 8}, {"BLOCK_ID", idW}, {"RULES", rulesW}, {"N", 4}, {"MODEL", modelW}}), inner)))
	visible := m.bottomVisibleRows()
	start := m.blockOffset
	end := start + visible
	if end > len(m.blocks) {
		end = len(m.blocks)
	}
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true)
	for i := start; i < end; i++ {
		line := fitRow(blockRow(m.blocks[i], idW, rulesW, modelW), inner-2)
		if i == m.blockCursor {
			line = cursorStyle.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		rows = append(rows, line)
	}
	if end < len(m.blocks) {
		rows = append(rows, fmt.Sprintf("… %d more (j/k move)", len(m.blocks)-end))
	}
	return borderStyle.Render(strings.Join(rows, "\n"))
}

func renderBlockDetail(m *model, width, height int) string {
	borderStyle, inner := paneBox(width, height, true)
	title := "BLOCK " + m.blockDetailID
	if m.blockPendingID != "" {
		title = "BLOCK " + m.blockPendingID + " (loading…)"
	}
	lines := []string{title}
	if m.blockPendingID != "" {
		lines = append(lines, "loading…")
		return borderStyle.Render(strings.Join(lines, "\n"))
	}
	if m.blockDetailErr != "" {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("fetch failed: "+m.blockDetailErr))
		lines = append(lines, "[esc] back")
		return borderStyle.Render(strings.Join(lines, "\n"))
	}
	c := m.blockDetail
	lines = append(lines, fmt.Sprintf("op=%s model=%s rules=%s", c.Operation, c.PublicModel, strings.Join(c.RuleIDs, ",")))
	lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Render("consumed: re-open returns 404 (take-once)"))
	decisionHint := "[a]llow non-secret  [s]anitize with placeholder  [d]eny (keep blocking)"
	if m.blockDecisionPending != "" {
		decisionHint = "recording " + m.blockDecisionPending + "…"
	} else if m.blockDecisionErr != "" {
		decisionHint = lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("decision failed: " + m.blockDecisionErr)
	} else if m.blockDecisionMsg != "" {
		decisionHint = lipgloss.NewStyle().Foreground(lipgloss.Color("#34D399")).Render(m.blockDecisionMsg)
	}
	lines = append(lines, decisionHint)
	for i, f := range c.Findings {
		lines = append(lines, fmt.Sprintf("--- finding %d rule=%s", i+1, f.RuleID))
		if f.Description != "" {
			lines = append(lines, "desc: "+f.Description)
		}
		lines = append(lines, "secret: "+f.Secret)
		if f.Match != "" && f.Match != f.Secret {
			lines = append(lines, "match: "+f.Match)
		}
		if f.Line != "" && f.Line != f.Match && f.Line != f.Secret {
			lines = append(lines, "line: "+f.Line)
		}
	}
	raw := strings.Join(lines, "\n")
	parts := strings.Split(raw, "\n")
	visible := height - 4
	if visible < 1 {
		visible = 1
	}
	start := m.blockDetailScroll
	if start > len(parts)-1 {
		start = len(parts) - 1
	}
	if start < 0 {
		start = 0
	}
	m.blockDetailScroll = start
	end := start + visible
	if end > len(parts) {
		end = len(parts)
	}
	out := make([]string, 0, end-start+1)
	for _, l := range parts[start:end] {
		out = append(out, truncate(l, inner))
	}
	if end < len(parts) {
		out = append(out, fmt.Sprintf("… %d more lines (j/k scroll, esc back)", len(parts)-end))
	} else {
		out = append(out, lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render("[j/k] scroll [a/s/d] flag [esc] back"))
	}
	return borderStyle.Render(strings.Join(out, "\n"))
}
