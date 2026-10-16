package dashboard

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	accentStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true)
	mutedStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	sectionStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#A78BFA")).Bold(true)
	successStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ADE80")).Bold(true)
	warningStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24")).Bold(true)
	errorStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true)
	selectionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#F8FAFC")).Background(lipgloss.Color("#1E3A5F")).Bold(true)
)

type detailSection struct {
	title string
	lines []string
	style lipgloss.Style
	raw   bool
}

func detailSectionLines(sections []detailSection, width int) []string {
	var rows []string
	for i, section := range sections {
		if i > 0 {
			rows = append(rows, "")
		}
		style := section.style.Inherit(sectionStyle)
		title := truncate("── "+logOneLine(inspectionText(section.title))+" ", width)
		rows = append(rows, style.Render(title+strings.Repeat("─", max(0, width-visibleLen(title)))))
		for _, line := range section.lines {
			text := inspectionText(line)
			if label, value, ok := strings.Cut(text, ": "); ok && !section.raw {
				text = mutedStyle.Render(label+": ") + value
			}
			rows = append(rows, wrapText(text, width)...)
		}
	}
	return rows
}

func renderDetailPage(width, height int, headers, parts []string, notice string, scroll *int) string {
	frame, inner := paneBox(width, height, true)
	visible := max(1, height-3-len(headers))
	*scroll = clampInt(*scroll, 0, max(0, len(parts)-visible))
	end := min(len(parts), *scroll+visible)
	rows := make([]string, 0, height-2)
	for _, header := range headers {
		rows = append(rows, truncate(header, inner))
	}
	rows = append(rows, parts[*scroll:end]...)
	for len(rows) < height-3 {
		rows = append(rows, "")
	}
	start := *scroll + 1
	if len(parts) == 0 {
		start = 0
	}
	position := fmt.Sprintf("lines %d-%d/%d", start, end, len(parts))
	if end < len(parts) {
		position += " ↓"
	}
	if notice != "" {
		position += " · " + notice
	}
	rows = append(rows, mutedStyle.Render(truncate(position, inner)))
	return frame.Render(strings.Join(rows, "\n"))
}

func paneTitle(title string, focused bool, width int) string {
	mark, style := "  ", mutedStyle.Bold(true)
	if focused {
		mark, style = "◆ ", accentStyle
	}
	return style.Render(truncate(mark+title, width))
}

func selectedRow(line string, selected, focused bool, width int) string {
	if selected && focused {
		return selectionStyle.Render(padRight(truncate(ansi.Strip(line), width), width))
	}
	return truncate(line, width)
}

func keyHint(key, label string, style lipgloss.Style) string {
	return style.Render("[" + key + "] " + label)
}

func shortcutHints(text string) string {
	var out strings.Builder
	for len(text) > 0 {
		before, rest, ok := strings.Cut(text, "[")
		out.WriteString(mutedStyle.Render(before))
		if !ok {
			break
		}
		key, after, closed := strings.Cut(rest, "]")
		if !closed {
			out.WriteString(mutedStyle.Render("[" + rest))
			break
		}
		out.WriteString(accentStyle.Render("[" + key + "]"))
		text = after
	}
	return out.String()
}

func (m *model) renderDetailContext() string {
	name := m.focusName()
	trail := "DASHBOARD / " + name + " / DETAIL"
	back := keyHint("esc", "Back to "+strings.ToLower(name), mutedStyle)
	if m.blockDetailOpen() || m.blockPendingID != "" {
		trail = "BLOCKS / Quarantined block"
		if len(m.blockDetail.Findings) > 0 {
			trail += fmt.Sprintf(" / Finding %d/%d", m.blockFindingCursor+1, len(m.blockDetail.Findings))
		}
	}
	space := max(0, m.width-visibleLen(back)-2)
	breadcrumb := padRight(accentStyle.Render(truncate(trail, space)), space) + "  " + back
	actions := keyHint("↑/↓ j/k", "Scroll", accentStyle) + "  " + keyHint("PgUp/PgDn", "Page", accentStyle) + "  " + keyHint("Home/End", "Top/bottom", accentStyle)
	if m.blockDetailOpen() || m.blockPendingID != "" {
		f, ok := m.selectedBlockFinding()
		finding := keyHint("←/→ n/N", "Finding", accentStyle)
		switch {
		case m.blockPendingID != "":
			actions = mutedStyle.Render("Loading capture…")
		case m.blockDetailErr != "":
			actions = errorStyle.Render("Capture unavailable · [esc] back to blocks")
		case m.paused:
			actions = keyHint("p", "Resume decisions", warningStyle) + "  " + finding
		case m.blockDecisionPending != "":
			actions = warningStyle.Render("Recording " + m.blockDecisionPending + "… · finding locked")
		case !ok || !validFindingSHA(f.SecretSHA) || m.blockFetcher == nil:
			actions = mutedStyle.Render("Decisions unavailable") + "  " + finding
		default:
			actions = keyHint("a", "Allow non-secret", successStyle) + "  " + keyHint("s", "Redact", warningStyle) + "  " + keyHint("d", "Deny", errorStyle) + "  " + finding
		}
	}
	return fitView(breadcrumb+"\n"+actions, m.width)
}
