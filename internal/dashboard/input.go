package dashboard

import tea "charm.land/bubbletea/v2"

type inputMode uint8

const (
	inputBrowse inputMode = iota
	inputDetail
	inputHelp
	inputTooSmall
	inputSearch
)

func (m *model) inputMode() inputMode {
	if m.search.active {
		return inputSearch
	}
	if m.showHelp {
		return inputHelp
	}
	if (m.width > 0 && m.width < 80) || (m.height > 0 && m.height < 12) {
		return inputTooSmall
	}
	if m.hasDetail() {
		return inputDetail
	}
	return inputBrowse
}

func (m *model) hasDetail() bool {
	return m.requestDetail != nil || m.usageDetail != nil || m.providerDetailName != "" || m.aliasDetailOpen() || m.logDetailOpen || m.payloadDetailOpen() ||
		m.payloadPendingID != "" || m.blockDetailOpen() || m.blockPendingID != ""
}

func (m *model) closeDetail() {
	m.requestDetail, m.usageDetail = nil, nil
	m.metadataScroll = 0
	m.correlationNotice = ""
	m.providerDetailName = ""
	m.providerDetailScroll = 0
	m.aliasDetailName = ""
	m.aliasDetailScroll = 0
	m.logDetailOpen = false
	m.logDetailScroll = 0
	m.payloadDetailRequest.invalidate()
	m.pausedResults.payloadDetail = nil
	m.payloadDetailID, m.payloadPendingID = "", ""
	m.payloadDetail, m.payloadDetailErr = "", ""
	m.payloadDetailScroll = 0
	m.blockDetailRequest.invalidate()
	m.blockDecisionRequest.invalidate()
	m.pausedResults.blockDetail, m.pausedResults.blockDecision = nil, nil
	m.blockDetailID, m.blockPendingID = "", ""
	m.blockDetail = BlockCapture{}
	m.blockDetailErr, m.blockDecisionPending = "", ""
	m.blockDecisionMsg, m.blockDecisionErr = "", ""
	m.blockDetailScroll = 0
	m.blockFindingCursor = 0
	m.blockDecisionSHA = ""
	m.blockDecisionResults = nil
	m.dirty = true
}

func navigationKey(key string) bool {
	switch key {
	case "tab", "shift+tab", "1", "2", "3", "4", "5", "[", "]":
		return true
	}
	return false
}

func (m *model) loadFocusedPane() tea.Cmd {
	if m.focus != focusBottom || m.hasDetail() || m.paused {
		return nil
	}
	switch m.bottomTab {
	case bottomTabPayload:
		if !m.payloadKnown && m.payloadErr == "" {
			return m.requestPayloads()
		}
	case bottomTabBlocks:
		if !m.blockKnown && m.blockErr == "" {
			return m.requestBlocks()
		}
	}
	return nil
}

func (m *model) handleInput(msg tea.KeyMsg) tea.Cmd {
	key := msg.String()
	if key == "ctrl+c" {
		m.quit = true
		return tea.Quit
	}
	if key == "ctrl+r" {
		if m.retry != nil {
			m.retry()
		}
		return nil
	}
	mode := m.inputMode()
	if mode == inputSearch {
		m.handleSearch(msg)
		return nil
	}
	if key == "q" {
		m.quit = true
		return tea.Quit
	}
	if mode == inputHelp {
		m.handleHelpKey(key)
		m.dirty = true
		return nil
	}
	if key == "?" || key == "h" {
		m.showHelp = true
		m.helpScroll = 0
		m.dirty = true
		return nil
	}
	if key == "esc" {
		switch {
		case m.hasDetail():
			m.closeDetail()
		case m.correlation != nil:
			m.leaveCorrelation(true)
		case m.zoomed:
			m.zoomed = false
			m.clampScroll()
			m.dirty = true
		default:
			m.quit = true
			return tea.Quit
		}
		return nil
	}
	if mode == inputTooSmall {
		return nil
	}
	if key == "p" {
		m.togglePause()
		return m.loadFocusedPane()
	}
	if navigationKey(key) {
		if m.hasDetail() {
			m.closeDetail()
		}
		m.leaveCorrelation(false)
		m.handleKey(msg)
		m.clampScroll()
		m.dirty = true
		return m.loadFocusedPane()
	}
	if mode == inputDetail {
		if key == "enter" {
			m.closeDetail()
			return nil
		}
		m.dirty = true
		switch {
		case m.requestDetail != nil:
			_, cmd := m.handleRequestKey(key)
			return cmd
		case m.usageDetail != nil:
			if key == "n" {
				m.nextUsageDetail(1)
			} else if key == "N" {
				m.nextUsageDetail(-1)
			} else {
				m.metadataScrollKey(key)
			}
		case m.providerDetailName != "":
			m.handleProviderDetailKey(key)
		case m.aliasDetailOpen():
			m.handleAliasDetailKey(msg)
		case m.logDetailOpen:
			m.handleLogDetailKey(msg)
		case m.payloadDetailOpen():
			_, cmd := m.handlePayloadKey(msg)
			return cmd
		case m.blockDetailOpen():
			_, cmd := m.handleBlockKey(msg)
			return cmd
		}
		return nil
	}
	if m.searchable() && m.correlation == nil {
		if key == "/" {
			text := []rune(m.queries[m.bottomTab])
			m.search = searchState{active: true, text: text, cursor: len(text)}
			m.dirty = true
			return nil
		}
		if key == "ctrl+u" {
			m.setSearch("")
			return nil
		}
	}
	if m.focus == focusBottom {
		if m.correlation != nil && (key == "l" || key == "o" || key == "s" || key == "e") {
			return nil
		}
		var handled bool
		var cmd tea.Cmd
		switch m.bottomTab {
		case bottomTabRequests:
			handled, cmd = m.handleRequestKey(key)
		case bottomTabAliases:
			handled = m.handleAliasKey(msg)
		case bottomTabLogs:
			handled = m.handleLogKey(msg)
		case bottomTabPayload:
			handled, cmd = m.handlePayloadKey(msg)
		case bottomTabBlocks:
			handled, cmd = m.handleBlockKey(msg)
		}
		if handled {
			m.dirty = true
			return cmd
		}
	}
	if m.handleKey(msg) {
		m.dirty = true
	}
	return nil
}
