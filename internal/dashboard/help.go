package dashboard

import (
	"fmt"
	"strings"
)

func (m *model) helpRows() int {
	return max(1, m.height-footerLines-1)
}

func (m *model) handleHelpKey(key string) {
	switch key {
	case "?", "h", "esc", "enter":
		m.showHelp = false
	case "j", "down":
		m.helpScroll++
	case "k", "up":
		m.helpScroll = max(0, m.helpScroll-1)
	case "pgdown", "shift+pgdown", "space":
		m.helpScroll += m.helpRows()
	case "pgup", "shift+pgup":
		m.helpScroll = max(0, m.helpScroll-m.helpRows())
	case "g", "home":
		m.helpScroll = 0
	case "G", "end":
		m.helpScroll = 1 << 30
	}
}

func (m *model) renderHelpPage(lines []string) string {
	var wrapped []string
	for _, line := range lines {
		wrapped = append(wrapped, wrapText(line, max(1, m.width))...)
	}
	rows := m.helpRows()
	m.helpScroll = clampInt(m.helpScroll, 0, max(0, len(wrapped)-rows))
	end := min(len(wrapped), m.helpScroll+rows)
	page := []string{fmt.Sprintf("HELP %d-%d/%d · j/k or PgUp/PgDn to scroll", m.helpScroll+1, end, len(wrapped))}
	page = append(page, wrapped[m.helpScroll:end]...)
	for len(page) < rows+1 {
		page = append(page, "")
	}
	page = append(page, renderFooter(m))
	return fitView(strings.Join(page, "\n"), m.width)
}
