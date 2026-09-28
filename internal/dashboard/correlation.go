package dashboard

import (
	tea "charm.land/bubbletea/v2"
	"github.com/egose/aiproxy/internal/accounting"
)

type correlationContext struct {
	zoomed                bool
	event                 accounting.Event
	scroll                int
	tab                   bottomTab
	cursor, offset        int
	logSeq                uint64
	logTopSeq             uint64
	logFollow             bool
	payloadID, payloadTop string
}

func (m *model) correlateRequest(tab bottomTab) tea.Cmd {
	e := *m.requestDetail
	if e.Truncated.RequestID {
		m.correlationNotice = "Correlation unavailable: request ID is truncated; prefix is not an exact key."
		return nil
	}
	if e.RequestID == "" {
		m.correlationNotice = "Correlation unavailable: this snapshot has no request ID."
		return nil
	}
	c := &correlationContext{event: e, scroll: m.metadataScroll, tab: tab, zoomed: m.zoomed}
	if tab == bottomTabLogs {
		c.cursor, c.offset, c.logSeq, c.logFollow = m.logCursor, m.logOffset, m.logCursorSeq, m.logFollow
		rows := m.filteredLogs(1 << 30)
		if c.offset < len(rows) {
			c.logTopSeq = rows[c.offset].Seq
		}
		m.logCursor, m.logOffset, m.logCursorSeq, m.logFollow = 0, 0, 0, false
	} else {
		c.cursor, c.offset = m.payloadCursor, m.payloadOffset
		rows := m.orderedPayloads()
		if c.cursor < len(rows) {
			c.payloadID = rows[c.cursor].RequestID
		}
		if c.offset < len(rows) {
			c.payloadTop = rows[c.offset].RequestID
		}
		m.payloadCursor, m.payloadOffset = 0, 0
		m.payloadListRequest.invalidate()
		m.pausedResults.payloadList = nil
		m.payloadLoading = false
	}
	m.correlation = c
	m.requestDetail = nil
	m.bottomTab, m.focus = tab, focusBottom
	m.clampScroll()
	if tab == bottomTabPayload {
		return m.requestPayloads()
	}
	return nil
}

func (m *model) leaveCorrelation(back bool) {
	c := m.correlation
	if c == nil {
		return
	}
	m.correlation = nil
	if c.tab == bottomTabLogs {
		m.logCursor, m.logOffset, m.logCursorSeq, m.logFollow = c.cursor, c.offset, c.logSeq, c.logFollow
		if c.logTopSeq != 0 {
			for i, e := range m.filteredLogs(1 << 30) {
				if e.Seq == c.logTopSeq {
					m.logOffset = i
					break
				}
			}
		}
	} else {
		m.payloadListRequest.invalidate()
		m.pausedResults.payloadList = nil
		m.payloadLoading = false
		m.payloadCursor, m.payloadOffset = c.cursor, c.offset
		for i, e := range m.orderedPayloads() {
			if e.RequestID == c.payloadID {
				m.payloadCursor = i
			}
			if e.RequestID == c.payloadTop {
				m.payloadOffset = i
			}
		}
	}
	if back {
		m.zoomed = c.zoomed
		m.bottomTab, m.focus = bottomTabRequests, focusBottom
		m.requestDetail = &c.event
		m.metadataScroll = c.scroll
	}
	m.clampScroll()
	m.dirty = true
}

func (m *model) emptySearchText(tab bottomTab, fallback string) string {
	if m.correlation != nil && m.bottomTab == tab {
		if tab == bottomTabLogs {
			return "No matching logs: not captured, disabled, expired, or old ID metadata."
		}
		return "No matching payload: not captured, expired, or outside latest 100 entries."
	}
	if m.queries[tab] != "" {
		return "no matches in retained metadata (Ctrl+U clears search)"
	}
	return fallback
}
