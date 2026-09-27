package dashboard

import (
	"fmt"
	"strings"
	"unicode"

	tea "charm.land/bubbletea/v2"
)

const searchLimit = 256
const searchControls = " [enter] apply [esc] cancel ^U clear Ctrl+C quit"

type searchState struct {
	active bool
	text   []rune
	cursor int
}

func metadataMatch(query string, fields map[string]string) bool {
	for _, term := range strings.Fields(strings.ToLower(query)) {
		key, value, qualified := strings.Cut(term, ":")
		if !qualified {
			value = term
		}
		matched := false
		for field, text := range fields {
			if (!qualified || field == key) && strings.Contains(strings.ToLower(text), value) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	return true
}

func (m *model) searchable() bool {
	return m.focus == focusBottom && !m.hasDetail() && (m.bottomTab == bottomTabRequests || m.bottomTab == bottomTabLogs || m.bottomTab == bottomTabPayload)
}

func (m *model) handleSearch(msg tea.KeyMsg) {
	s := &m.search
	switch msg.String() {
	case "esc":
		*s = searchState{}
	case "enter":
		m.setSearch(strings.TrimSpace(string(s.text)))
		*s = searchState{}
	case "ctrl+u":
		s.text, s.cursor = nil, 0
	case "left":
		s.cursor = max(0, s.cursor-1)
	case "right":
		s.cursor = min(len(s.text), s.cursor+1)
	case "home", "ctrl+a":
		s.cursor = 0
	case "end", "ctrl+e":
		s.cursor = len(s.text)
	case "backspace":
		if s.cursor > 0 {
			s.text = append(s.text[:s.cursor-1], s.text[s.cursor:]...)
			s.cursor--
		}
	case "delete":
		if s.cursor < len(s.text) {
			s.text = append(s.text[:s.cursor], s.text[s.cursor+1:]...)
		}
	default:
		text := msg.Key().Text
		if text == "" && unicode.IsPrint(msg.Key().Code) && msg.Key().Mod == 0 {
			text = string(msg.Key().Code)
		}
		for _, r := range text {
			if !unicode.IsPrint(r) || len(s.text) >= searchLimit {
				continue
			}
			s.text = append(s.text, 0)
			copy(s.text[s.cursor+1:], s.text[s.cursor:])
			s.text[s.cursor] = r
			s.cursor++
		}
	}
	m.dirty = true
}

func (m *model) setSearch(query string) {
	m.queries[m.bottomTab] = query
	switch m.bottomTab {
	case bottomTabRequests:
		m.requestCursor, m.requestOffset = 0, 0
	case bottomTabLogs:
		m.logCursor, m.logOffset, m.logCursorSeq = 0, 0, 0
		m.logFollow = false
	case bottomTabPayload:
		m.payloadCursor, m.payloadOffset = 0, 0
	}
	m.clampScroll()
	m.dirty = true
}

func (m *model) searchLabel(tab bottomTab) string {
	if m.correlation != nil && tab == m.bottomTab {
		return "ID=" + metadataText(m.correlation.event.RequestID) + " [esc] Requests"
	}
	if m.queries[tab] != "" {
		return "filter: " + metadataText(m.queries[tab]) + " [ctrl+u] clear"
	}
	return "[/] search"
}

func (m *model) searchPrompt() string {
	s := m.search
	prefix := fmt.Sprintf("SEARCH %d/%d /", len(s.text), searchLimit)
	budget := max(0, m.width-visibleLen(searchControls)-visibleLen(prefix)-1)
	start := s.cursor
	for start > 0 && visibleLen(string(s.text[start-1:s.cursor])) <= budget {
		start--
	}
	return prefix + string(s.text[start:s.cursor]) + "│" + string(s.text[s.cursor:])
}
