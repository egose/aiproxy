package dashboard

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

func (m *model) compactLayout() bool {
	return m.height > 0 && m.height < 30
}

func (m *model) focusedLayout() bool {
	return m.compactLayout() || m.zoomed
}

type paneFrame struct {
	style         lipgloss.Style
	width, height int
}

func (p paneFrame) Render(content string) string {
	lines := strings.Split(fitView(content, p.width-2), "\n")
	lines = lines[:min(len(lines), max(0, p.height-2))]
	return p.style.Render(strings.Join(lines, "\n"))
}

func (m *model) renderNotice(text string) string {
	lines := wrapText(text, max(1, m.width))
	rows := max(0, m.height-footerLines)
	lines = lines[:min(len(lines), rows)]
	for len(lines) < rows {
		lines = append(lines, "")
	}
	return fitView(strings.Join(append(lines, renderFooter(m)), "\n"), m.width)
}

func (m *model) focusName() string {
	switch m.focus {
	case focusProviders:
		return "PROVIDERS"
	case focusUsage:
		return "USAGE"
	default:
		switch m.bottomTab {
		case bottomTabAliases:
			return "ALIASES"
		case bottomTabPayload:
			return "PAYLOADS"
		case bottomTabBlocks:
			return "BLOCKS"
		case bottomTabRequests:
			return "REQUESTS"
		default:
			return "LOGS"
		}
	}
}

func (m *model) renderContextFooter() string {
	back := "[esc] back/quit"
	actions := "[j/k] move [enter] detail [z] zoom"
	if m.focus == focusUsage {
		actions = "[tab] focus [t/e/u] filters [z] zoom"
	} else if m.searchable() && m.correlation == nil {
		actions = "[/] search [Ctrl+U] clear [j/k] move [enter] detail"
	}
	if m.zoomed {
		back = "[esc] back"
	}
	switch m.inputMode() {
	case inputHelp:
		actions = "[j/k pgup/pgdn] scroll"
		back = "[esc] back"
	case inputTooSmall:
		actions = "Resize to 80x12"
	case inputDetail:
		actions = "[j/k] scroll [enter] close"
		back = "[esc] back"
		if m.requestDetail != nil {
			actions = "[l] logs [v] payload [j/k] scroll"
			if m.requestDetail.Truncated.RequestID {
				actions = "truncated ID: no correlation · [j/k] scroll"
			}
		}
		if m.usageDetail != nil {
			actions = "[n/N] group [j/k] scroll"
		}
		if m.blockDetailOpen() && m.blockDetailErr == "" {
			actions = "[a/s/d] selected [n/N] finding"
			if m.blockDecisionPending != "" {
				actions = "Recording " + m.blockDecisionPending
			} else if m.paused {
				actions = "PAUSED [p] resume [j/k] scroll"
			}
		}
	}
	if m.inputMode() == inputBrowse && m.bottomTab == bottomTabBlocks && m.focus == focusBottom && !m.blockListVisible() {
		switch {
		case m.paused:
			actions = "PAUSED [p] resume"
		case m.blockFetcher == nil:
			actions = "block viewer unavailable"
		case m.blockErr != "":
			actions = "fetch failed · [r] retry"
		case !m.blockKnown:
			actions = "loading… · [r] retry"
		default:
			actions = "[r] refresh"
		}
	}
	essential := "[?] help " + back + " [q/Ctrl+C] quit"
	space := max(0, m.width-visibleLen(essential)-2)
	hints := padRight(truncate(actions, space), space) + "  " + essential
	state := "LIVE"
	if m.paused {
		state = "PAUSED"
	} else if m.staleErr != "" {
		state = "STALE"
	}
	layout := "stacked"
	if m.compactLayout() {
		layout = "compact"
	}
	if m.zoomed {
		layout = "zoom"
	}
	status := fmt.Sprintf("%s %s %s · [p] pause · [Ctrl+R] retry", state, m.focusName(), layout)
	if m.showHelp {
		status = "HELP · " + status
	}
	if !m.connection.LastSuccess.IsZero() || m.connection.Reconnecting || m.connection.Denied {
		status = m.connection.text() + " · " + state + " " + m.focusName() + " " + layout
	}
	if m.search.active {
		hints = truncate(m.searchPrompt(), max(0, m.width-visibleLen(searchControls))) + searchControls
	}
	return fitView(hints+"\n"+truncate(status, m.width), m.width)
}
