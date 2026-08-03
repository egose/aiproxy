package dashboard

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
)

// UsageViewer is the read-only shape of usage state the dashboard needs. Both
// the live *accounting.Aggregator in-process and a remote-JSON-backed adapter
// satisfied by dashrpc adapters implement this interface.
type UsageViewer interface {
	Summaries() []accounting.Summary
	Recent(n int) []accounting.Event
}

// HealthViewer is the read-only shape of provider-health state.
type HealthViewer interface {
	Snapshot() map[string]bool
}

// LogsViewer is the read-only shape of buffered logs.
type LogsViewer interface {
	Since(n int) []observability.LogEntry
}

var _ UsageViewer = (*accounting.Aggregator)(nil)

const (
	refreshInterval = 2 * time.Second
	recentLimit     = 200
)

type RuntimeSnapshot struct {
	Version           string
	Address           string
	Providers         []config.Provider
	DisabledProviders []config.Provider
	Aliases           []config.Alias
	AuthMode          string
	StartTime         time.Time
	Usage             UsageViewer
	Health            HealthViewer
	Logs              LogsViewer
}

type snapshotMsg struct {
	snapshot *RuntimeSnapshot
}

type focusArea int

const (
	focusStats focusArea = iota
	focusLogs
)

type model struct {
	snapshot    *RuntimeSnapshot
	health      map[string]bool
	width       int
	height      int
	now         time.Time
	quit        bool
	dirty       bool
	rendered    string
	focus       focusArea
	logsHeight  int
	statsHeight int
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func InitialModel(s *RuntimeSnapshot) tea.Model {
	return &model{
		snapshot:    s,
		health:      map[string]bool{},
		now:         time.Now(),
		dirty:       true,
		focus:       focusStats,
		statsHeight: 14,
		logsHeight:  10,
	}
}

func (m *model) Init() tea.Cmd {
	return tickCmd()
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.relayout()
		m.dirty = true
		return m, nil
	case tea.KeyMsg:
		if shouldQuit(msg) {
			m.quit = true
			return m, tea.Quit
		}
		handled := m.handleKey(msg)
		if handled {
			m.dirty = true
		}
		return m, nil
	case snapshotMsg:
		if msg.snapshot != nil {
			m.snapshot = msg.snapshot
			if m.snapshot.Health != nil {
				m.health = m.snapshot.Health.Snapshot()
			}
			m.dirty = true
		}
		return m, nil
	case tickMsg:
		m.now = time.Now()
		if m.snapshot != nil && m.snapshot.Health != nil {
			m.health = m.snapshot.Health.Snapshot()
		}
		m.dirty = true
		return m, tickCmd()
	}
	return m, nil
}

func shouldQuit(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "q", "esc", "ctrl+c":
		return true
	}
	return false
}

func (m *model) handleKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "tab":
		if m.focus == focusStats {
			m.focus = focusLogs
		} else {
			m.focus = focusStats
		}
		return true
	case "j", "down":
		if m.focus == focusLogs {
			return m.resize(2)
		}
		return false
	case "k", "up":
		if m.focus == focusLogs {
			return m.resize(-2)
		}
		return false
	}
	return false
}

func (m *model) resize(delta int) bool {
	newLogs := m.logsHeight + delta
	const minPanes = 4
	maxLogs := m.height - 8 - minPanes
	if newLogs < minPanes {
		newLogs = minPanes
	}
	if newLogs > maxLogs {
		newLogs = maxLogs
	}
	if newLogs == m.logsHeight {
		return false
	}
	m.logsHeight = newLogs
	m.statsHeight = m.height - 8 - newLogs
	return true
}

func (m *model) relayout() {
	if m.height <= 8 {
		m.logsHeight = 0
		m.statsHeight = 0
		return
	}
	available := m.height - 8
	if available < 8 {
		m.logsHeight = 0
		m.statsHeight = available
		return
	}
	if m.logsHeight <= 0 {
		m.logsHeight = available / 3
		m.statsHeight = available - m.logsHeight
		return
	}
	if m.statsHeight+m.logsHeight != available {
		diff := available - (m.statsHeight + m.logsHeight)
		if diff > 0 {
			m.statsHeight += diff
		} else {
			m.logsHeight += diff
			if m.logsHeight < 4 {
				m.statsHeight -= (4 - m.logsHeight)
				m.logsHeight = 4
			}
		}
	}
}

func (m *model) View() tea.View {
	if !m.dirty && m.rendered != "" {
		return tea.NewView(m.rendered)
	}
	out := m.render()
	m.rendered = out
	m.dirty = false
	return tea.NewView(out)
}

func (m *model) SetNowForTest(now time.Time) {
	m.now = now
	// keep dirty as-is; tests can set it explicitly.
}

func (m *model) render() string {
	if m.snapshot == nil {
		return "no snapshot"
	}
	if m.width < 80 || m.height < 12 {
		return fmt.Sprintf("Terminal too small (%dx%d). Need at least 80x12.", m.width, m.height)
	}
	header := renderHeader(m)
	sideWidth := clampInt(m.width*30/100, 36, 60)
	usageWidth := m.width - sideWidth - 1
	if usageWidth < 40 {
		usageWidth = 40
	}
	statsHeight := m.statsHeight
	if statsHeight < 4 {
		statsHeight = 4
	}
	side := renderProviders(m, sideWidth, statsHeight)
	usage := renderUsage(m, usageWidth, statsHeight)
	mid := lipgloss.JoinHorizontal(lipgloss.Top, side, usage)
	logs := renderLogs(m, m.width, m.logsHeight)
	return lipgloss.JoinVertical(lipgloss.Left, header, mid, logs, renderFooter(m))
}

func renderFooter(m *model) string {
	focusName := "STATS"
	if m.focus == focusLogs {
		focusName = "LOGS"
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render(
		fmt.Sprintf("focus: %s  [tab] switch  [j/k] resize  [q/Esc/Ctrl+C] quit", focusName))
}

func renderHeader(m *model) string {
	snap := m.snapshot
	uptime := m.now.Sub(snap.StartTime).Round(time.Second)
	left := fmt.Sprintf("aiproxy %s  %s", snap.Version, snap.Address)
	active := len(snap.Providers)
	disabled := len(snap.DisabledProviders)
	right := fmt.Sprintf("providers %d (+%d disabled)  aliases %d  auth %s  uptime %s",
		active, disabled, len(snap.Aliases), snap.AuthMode, uptime)
	leftStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	return leftStyle.Render(left) + "  " + rightStyle.Render(right)
}

func renderProviders(m *model, width, height int) string {
	snap := m.snapshot
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width - 2).
		Height(height - 2)
	rows := []string{headerRow([]col{{"PROVIDER", 20}, {"", 3}, {"REQS", 8}, {"ERR%", 6}, {"P95", 8}, {"TOKENS", 10}})}
	summaries := snap.Usage.Summaries()
	byProvider := accounting.ByProvider(summaries)
	latencyByProvider := p95LatencyByProvider(snap.Usage.Recent(recentLimit))
	for _, p := range snap.Providers {
		rows = append(rows, providerRow(p.Name, m.health[p.Name], lookupProvider(byProvider, p.Name), latencyByProvider[p.Name], false))
	}
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))
	for _, p := range snap.DisabledProviders {
		row := providerRow(p.Name, false, accounting.ProviderSummary{}, 0, true)
		rows = append(rows, dimStyle.Render(row))
	}
	if len(snap.Providers) == 0 && len(snap.DisabledProviders) == 0 {
		rows = append(rows, "no providers configured")
	}
	return border.Render(strings.Join(rows, "\n"))
}

func providerRow(name string, healthy bool, ps accounting.ProviderSummary, p95 time.Duration, disabled bool) string {
	status := "✓"
	if !healthy || disabled {
		status = "✗"
	}
	errPct := 0.0
	if ps.Requests > 0 {
		errPct = 100 * float64(ps.Errors) / float64(ps.Requests)
	}
	return dataRow([]string{
		truncate(name, 20),
		status,
		fmt.Sprintf("%6d", ps.Requests),
		fmt.Sprintf("%5.1f%%", errPct),
		fmt.Sprintf("%8s", p95.Round(time.Millisecond)),
		fmt.Sprintf("%8d", ps.TotalTokens),
	}, []int{20, 3, 8, 6, 8, 10})
}

func lookupProvider(byProvider []accounting.ProviderSummary, name string) accounting.ProviderSummary {
	for _, ps := range byProvider {
		if ps.Provider == name {
			return ps
		}
	}
	return accounting.ProviderSummary{Provider: name}
}

func p95LatencyByProvider(recent []accounting.Event) map[string]time.Duration {
	out := map[string]time.Duration{}
	byProvider := map[string][]time.Duration{}
	for _, e := range recent {
		if e.Duration <= 0 {
			continue
		}
		prov := providerOf(e.Model)
		byProvider[prov] = append(byProvider[prov], e.Duration)
	}
	for prov, durs := range byProvider {
		out[prov] = percentile(durs, 0.95)
	}
	return out
}

func percentile(durs []time.Duration, p float64) time.Duration {
	if len(durs) == 0 {
		return 0
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	idx := int(float64(len(durs)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(durs) {
		idx = len(durs) - 1
	}
	return durs[idx]
}

func providerOf(model string) string {
	if strings.HasPrefix(model, "_") {
		return "aiproxy"
	}
	for i := 0; i < len(model); i++ {
		if model[i] == '/' {
			return model[:i]
		}
	}
	return model
}

func renderUsage(m *model, width, height int) string {
	snap := m.snapshot
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width - 2).
		Height(height - 2)
	header := headerRow([]col{{"MODEL", 26}, {"OP", 18}, {"STATUS", 6}, {"COUNT", 6}, {"TOKENS", 12}})
	rows := []string{header}
	summaries := snap.Usage.Summaries()
	sort.SliceStable(summaries, func(i, j int) bool {
		return summaries[i].Count > summaries[j].Count
	})
	max := height - 4
	for i, s := range summaries {
		if i >= max {
			break
		}
		if strings.HasPrefix(s.Model, "_") {
			continue
		}
		rows = append(rows, dataRow([]string{
			truncate(s.Model, 26),
			truncate(s.Operation, 18),
			fmt.Sprintf("%d", s.StatusCode),
			fmt.Sprintf("%5d", s.Count),
			tokensCell(s),
		}, []int{26, 18, 6, 6, 12}))
	}
	if nonSentinelRows(rows) == 0 {
		rows = append(rows, "no usage recorded yet")
	}
	return border.Render(strings.Join(rows, "\n"))
}

func nonSentinelRows(rows []string) int {
	count := 0
	for i, r := range rows {
		if i == 0 {
			continue
		}
		if strings.HasPrefix(strings.TrimSpace(r), "_") {
			continue
		}
		if r == "no usage recorded yet" {
			continue
		}
		count++
	}
	return count
}

func tokensCell(s accounting.Summary) string {
	if s.TotalTokens == 0 {
		return "        ~"
	}
	return fmt.Sprintf("%11d", s.TotalTokens)
}

func renderLogs(m *model, width, height int) string {
	snap := m.snapshot
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width - 2).
		Height(height - 2)
	if m.focus == focusLogs {
		borderStyle = borderStyle.BorderForeground(lipgloss.Color("#38BDF8"))
	}
	title := headerRow([]col{{"LOGS", 8}, {"LEVEL", 6}, {"MESSAGE", width - 8 - 6 - 4 - 12 - 8}, {"ATTRS", 12}})
	rows := []string{title}
	if snap.Logs != nil && height > 3 {
		n := height - 3
		entries := snap.Logs.Since(n)
		for _, e := range entries {
			rows = append(rows, renderLogEntry(width, e))
		}
	}
	if len(rows) == 1 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("no logs captured"))
	}
	return borderStyle.Render(strings.Join(rows, "\n"))
}

func renderLogEntry(width int, e observability.LogEntry) string {
	levelStyle := levelStyleFor(e.Level)
	msgWidth := width - 8 - 6 - 4 - 12 - 8
	if msgWidth < 12 {
		msgWidth = 12
	}
	attrs := e.Attrs
	if len(attrs) > 12 {
		attrs = truncate(attrs, 12)
	}
	return dataRow([]string{
		e.Time.Format("15:04:05"),
		levelStyle.Render(padRight(e.Level.String(), 6)),
		truncate(e.Message, msgWidth),
		truncate(orDash(attrs), 12),
	}, []int{8, 6, msgWidth, 12})
}

func levelStyleFor(level slog.Level) lipgloss.Style {
	switch {
	case level >= slog.LevelError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true)
	case level >= slog.LevelWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
	case level >= slog.LevelDebug:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE"))
	}
}

type col struct {
	Label string
	Width int
}

func headerRow(cols []col) string {
	parts := make([]string, len(cols))
	style := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CBD5E1"))
	for i, c := range cols {
		parts[i] = style.Render(padRight(truncate(c.Label, c.Width), c.Width))
	}
	return strings.Join(parts, " ")
}

func dataRow(cells []string, widths []int) string {
	out := make([]string, len(cells))
	for i, c := range cells {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		if w > 0 && len(c) > w {
			c = truncate(c, w)
		}
		out[i] = padRight(c, w)
	}
	return strings.Join(out, " ")
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	return s[:n-1] + "…"
}

func padRight(s string, w int) string {
	if w == 0 || len(s) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(s))
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

type Program struct {
	program *tea.Program
	mu      sync.Mutex
	closed  bool
}

type RefreshHook interface {
	Refresh(snap *RuntimeSnapshot)
}

func Run(ctx context.Context, snap *RuntimeSnapshot) *Program {
	opts := []tea.ProgramOption{
		tea.WithContext(ctx),
		tea.WithoutCatchPanics(),
	}
	p := tea.NewProgram(InitialModel(snap), opts...)
	go func() {
		_, _ = p.Run()
	}()
	return &Program{program: p}
}

func (p *Program) Refresh(snap *RuntimeSnapshot) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.program.Send(snapshotMsg{snapshot: snap})
}

func (p *Program) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return
	}
	p.closed = true
	p.program.Quit()
}
