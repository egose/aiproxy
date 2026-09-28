package dashboard

import (
	"context"
	"fmt"
	"strings"

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
	generation uint64
	entries    []dashrpc.BlockSummary
	enabled    bool
	err        string
}

type blockDetailMsg struct {
	generation uint64
	blockID    string
	capture    dashrpc.BlockCapture
	err        string
}

type blockDecisionMsg struct {
	generation uint64
	blockID    string
	action     string
	sha        string
	count      int
	err        string
}

func fetchBlocksCmd(f BlockFetcher, parents ...context.Context) tea.Cmd {
	if f == nil {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := fetchContext(parents)
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

func fetchBlockDetailCmd(f BlockFetcher, blockID string, parents ...context.Context) tea.Cmd {
	if f == nil || blockID == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := fetchContext(parents)
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

func fetchBlockDecisionCmd(f BlockFetcher, blockID, action string, shas []string, parents ...context.Context) tea.Cmd {
	if f == nil || blockID == "" || action == "" || len(shas) == 0 {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := fetchContext(parents)
		defer cancel()
		res, err := f.DecideBlock(ctx, blockID, action, shas)
		msg := blockDecisionMsg{blockID: blockID, action: action, sha: shas[0]}
		if err != nil {
			msg.err = err.Error()
			return msg
		}
		if !res.Ok || res.Action != action || res.Count != 1 {
			msg.err = "unexpected acknowledgment; outcome unknown"
			return msg
		}
		msg.count = res.Count
		return msg
	}
}

func (m *model) blockDetailOpen() bool {
	return m.blockDetailID != ""
}

func (m *model) blockListVisible() bool {
	if m.blockFetcher == nil {
		return false
	}
	if !m.blockKnown || m.blockErr != "" {
		return false
	}
	return len(m.blocks) > 0
}

func (m *model) requestBlocks() tea.Cmd {
	if m.paused || m.blockFetcher == nil || m.blockLoading {
		return nil
	}
	m.blockLoading = true
	return m.blockListRequest.start(m.ctx, func(ctx context.Context) tea.Cmd {
		return fetchBlocksCmd(m.blockFetcher, ctx)
	})
}

func (m *model) applyBlockList(msg blockListMsg) {
	if msg.generation != m.blockListRequest.generation {
		return
	}
	if m.paused {
		m.pausedResults.blockList = &msg
		return
	}
	m.blockListRequest.invalidate()
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
	before := m.blocks
	m.blocks = msg.entries
	m.blockCursor = anchoredIndex(before, m.blocks, m.blockCursor, blockIdentity)
	m.blockOffset = anchoredIndex(before, m.blocks, m.blockOffset, blockIdentity)
	m.clampBlockCursor()
	m.dirty = true
}

func (m *model) applyBlockDetail(msg blockDetailMsg) {
	if msg.generation != m.blockDetailRequest.generation || msg.blockID != m.blockPendingID {
		return
	}
	if m.paused {
		m.pausedResults.blockDetail = &msg
		return
	}
	m.blockDetailRequest.invalidate()
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
	m.blockFindingCursor = 0
	m.blockDecisionResults = nil
	m.blockDecisionSHA = ""
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
	if !m.blockListVisible() {
		return false
	}
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
	if !m.blockListVisible() {
		return false
	}
	if m.blockCursor == 0 && m.blockOffset == 0 {
		return false
	}
	m.blockCursor = 0
	m.blockOffset = 0
	return true
}

func (m *model) blockCursorBottom() bool {
	if !m.blockListVisible() {
		return false
	}
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
	case "n", "N":
		if !m.blockDetailOpen() || m.blockDecisionPending != "" || len(m.blockDetail.Findings) == 0 {
			return false, nil
		}
		delta := 1
		if msg.String() == "N" {
			delta = -1
		}
		m.blockFindingCursor = (m.blockFindingCursor + delta + len(m.blockDetail.Findings)) % len(m.blockDetail.Findings)
		m.blockDetailScroll = 0
		m.selectedDecisionStatus()
		return true, nil
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
		if m.paused {
			return true, nil
		}
		if m.blockDetailOpen() {
			return true, nil
		}
		if m.blockPendingID != "" {
			return true, nil
		}
		if !m.blockListVisible() {
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
		return true, m.blockDetailRequest.start(m.ctx, func(ctx context.Context) tea.Cmd {
			return fetchBlockDetailCmd(m.blockFetcher, id, ctx)
		})
	case "r":
		if m.paused || m.blockDetailOpen() {
			return false, nil
		}
		m.blockKnown = false
		return true, m.requestBlocks()
	case "a":
		return m.requestBlockDecision("allow")
	case "s":
		return m.requestBlockDecision("redact")
	case "d":
		return m.requestBlockDecision("deny")
	}
	return false, nil
}

func (m *model) requestBlockDecision(action string) (bool, tea.Cmd) {
	if m.paused || !m.blockDetailOpen() || m.blockDetailErr != "" || m.blockDecisionPending != "" {
		return false, nil
	}
	f, ok := m.selectedBlockFinding()
	if !ok || !validFindingSHA(f.SecretSHA) || m.blockFetcher == nil {
		return false, nil
	}
	if action != "allow" && action != "redact" && action != "deny" {
		return false, nil
	}
	if previous, ok := m.blockDecisionResults[f.SecretSHA]; ok && previous.err == "" && previous.action == action {
		return false, nil
	}
	shas := []string{f.SecretSHA}
	m.blockDecisionPending = action
	m.blockDecisionSHA = f.SecretSHA
	m.blockDecisionMsg = ""
	m.blockDecisionErr = ""
	m.dirty = true
	return true, m.blockDecisionRequest.start(m.ctx, func(ctx context.Context) tea.Cmd {
		return fetchBlockDecisionCmd(m.blockFetcher, m.blockDetailID, action, shas, ctx)
	})
}

func (m *model) applyBlockDecision(msg blockDecisionMsg) {
	if msg.generation != m.blockDecisionRequest.generation || msg.blockID != m.blockDetailID || msg.action != m.blockDecisionPending || msg.sha != m.blockDecisionSHA {
		return
	}
	if m.paused {
		m.pausedResults.blockDecision = &msg
		return
	}
	m.blockDecisionRequest.invalidate()
	m.blockDecisionPending = ""
	m.blockDecisionSHA = ""
	if m.blockDecisionResults == nil {
		m.blockDecisionResults = make(map[string]blockDecisionMsg)
	}
	m.blockDecisionResults[msg.sha] = msg
	m.selectedDecisionStatus()
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
	title := "BLOCKS · Enter consumes take-once capture; re-open unavailable · [r]efresh"
	rows := []string{title}
	if m.blockFetcher == nil {
		rows = append(rows, "block viewer unavailable")
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if m.blockErr != "" {
		hint := " · [r] retry"
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Render("fetch failed: "+truncate(m.blockErr, max(0, inner-len(hint)))+hint))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if !m.blockKnown && !m.blockEnabled && len(m.blocks) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("quarantine disabled on server (enable ingress_guardrails quarantine in config)"))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if !m.blockKnown {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("loading… · [r] retry"))
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
	title := "BLOCK " + m.blockDetailID
	if m.blockPendingID != "" {
		title = "BLOCK " + m.blockPendingID + " (loading…)"
	}
	c := m.blockDetail
	number := 0
	if len(c.Findings) > 0 {
		number = m.blockFindingCursor + 1
	}
	headers := []string{
		fmt.Sprintf("Finding %d/%d · %s", number, len(c.Findings), title),
		"SELECTED hash only · persistent GLOBAL future matches · no replay",
		m.blockInspectionStatus(),
	}
	lines := []string{title, fmt.Sprintf("at=%s op=%s model=%s rules=%s", c.Timestamp, c.Operation, c.PublicModel, strings.Join(c.RuleIDs, ","))}
	if m.blockDetailErr != "" {
		lines = append(lines, "fetch failed: "+m.blockDetailErr)
	} else if f, ok := m.selectedBlockFinding(); ok {
		lines = append(lines, "rule: "+f.RuleID, "SHA: "+f.SecretSHA, "desc: "+f.Description,
			"secret: "+f.Secret, "match: "+f.Match, "line: "+f.Line)
		if m.blockDecisionErr != "" {
			lines = append(lines, "decision error: "+m.blockDecisionErr)
		}
	} else {
		lines = append(lines, "no captured findings")
	}
	return renderInspection(width, height, headers, strings.Join(lines, "\n"),
		"server-capped snippets; truncation unknown", &m.blockDetailScroll)
}
