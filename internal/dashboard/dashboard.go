package dashboard

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net"
	"net/url"
	"regexp"
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

type UsageViewer interface {
	Summaries() []accounting.Summary
	Recent(n int) []accounting.Event
	ProviderSummaries() []accounting.ProviderSummary
	UpstreamSummaries() []accounting.UpstreamSummary
}

type HealthViewer interface {
	Snapshot() map[string]bool
}

type LogsViewer interface {
	Since(n int) []observability.LogEntry
}

var _ UsageViewer = (*accounting.Aggregator)(nil)

const (
	refreshInterval = 2 * time.Second
	recentLimit     = 200
)

type CooldownEntry struct {
	Alias       string
	Provider    string
	Model       string
	RemainingMs int64
}

type RuntimeSnapshot struct {
	Version           string
	Address           string
	Providers         []config.Provider
	DisabledProviders []config.Provider
	Aliases           []config.Alias
	Cooldowns         []CooldownEntry
	AuthMode          string
	StartTime         time.Time
	SnapshotAt        time.Time
	Usage             UsageViewer
	Health            HealthViewer
	Logs              LogsViewer
}

type snapshotMsg struct {
	snapshot *RuntimeSnapshot
}

type pollErrorMsg struct {
	err string
	at  time.Time
}

type focusArea int

const (
	focusProviders focusArea = iota
	focusUsage
	focusBottom
)

const (
	headerLines     = 1
	rateLines       = 2
	footerLines     = 2
	chromeLines     = headerLines + rateLines + footerLines
	statsMinHeight  = 4
	bottomMinHeight = 6
)

type bottomTab int

const (
	bottomTabLogs bottomTab = iota
	bottomTabAliases
)

type model struct {
	snapshot       *RuntimeSnapshot
	pending        *RuntimeSnapshot
	hasPending     bool
	health         map[string]bool
	width          int
	height         int
	now            time.Time
	quit           bool
	dirty          bool
	rendered       string
	focus          focusArea
	bottomTab      bottomTab
	bottomHeight   int
	statsHeight    int
	lastRefresh    time.Time
	staleErr       string
	staleAt        time.Time
	paused         bool
	showHelp       bool
	zoomed         bool
	usageScroll    int
	providerScroll int
	aliasScroll    int
	logScroll      int
	logMinLevel    slog.Level
	logFilterOn    bool
	tenantIndex    int
	errorsOnly     bool
	usageUpstream  bool
	ipCache        map[string]string
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

func InitialModel(s *RuntimeSnapshot) tea.Model {
	m := &model{
		snapshot:     s,
		health:       map[string]bool{},
		now:          time.Now(),
		dirty:        true,
		focus:        focusUsage,
		statsHeight:  12,
		bottomHeight: 14,
		logMinLevel:  slog.LevelDebug,
	}
	if s != nil && s.Health != nil {
		m.health = s.Health.Snapshot()
	}
	if s != nil && !s.SnapshotAt.IsZero() {
		m.lastRefresh = s.SnapshotAt
	} else {
		m.lastRefresh = m.now
	}
	return m
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
		m.clampScroll()
		m.dirty = true
		return m, nil
	case tea.KeyMsg:
		if m.zoomed && msg.String() == "esc" {
			m.zoomed = false
			m.dirty = true
			return m, nil
		}
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
			if m.paused {
				m.pending = msg.snapshot
				m.hasPending = true
				m.dirty = true
				return m, nil
			}
			m.applySnapshot(msg.snapshot)
		}
		return m, nil
	case pollErrorMsg:
		m.staleErr = msg.err
		m.staleAt = msg.at
		m.dirty = true
		return m, nil
	case tickMsg:
		m.now = time.Now()
		if m.snapshot != nil && m.snapshot.Health != nil {
			m.health = m.snapshot.Health.Snapshot()
		}
		m.clampScroll()
		m.dirty = true
		return m, tickCmd()
	}
	return m, nil
}

func (m *model) applySnapshot(s *RuntimeSnapshot) {
	m.snapshot = s
	m.pending = nil
	m.hasPending = false
	if s.Health != nil {
		m.health = s.Health.Snapshot()
	}
	if !s.SnapshotAt.IsZero() {
		m.lastRefresh = s.SnapshotAt
	} else {
		m.lastRefresh = m.now
	}
	m.staleErr = ""
	m.usageScroll = 0
	m.providerScroll = 0
	m.aliasScroll = 0
	m.clampScroll()
	m.dirty = true
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
		m.focus = (m.focus + 1) % 3
		return true
	case "enter":
		m.zoomed = !m.zoomed
		m.clampScroll()
		return true
	case "1":
		m.bottomTab = bottomTabAliases
		m.focus = focusBottom
		return true
	case "2":
		m.bottomTab = bottomTabLogs
		m.focus = focusBottom
		return true
	case "[", "]":
		if m.bottomTab == bottomTabLogs {
			m.bottomTab = bottomTabAliases
		} else {
			m.bottomTab = bottomTabLogs
		}
		m.focus = focusBottom
		return true
	case "?", "h":
		m.showHelp = !m.showHelp
		return true
	case "p":
		m.paused = !m.paused
		if !m.paused && m.hasPending && m.pending != nil {
			s := m.pending
			m.pending = nil
			m.hasPending = false
			m.applySnapshot(s)
		}
		return true
	case "e":
		m.errorsOnly = !m.errorsOnly
		m.usageScroll = 0
		return true
	case "u":
		m.usageUpstream = !m.usageUpstream
		m.usageScroll = 0
		return true
	case "t":
		m.tenantIndex++
		if m.tenantIndex > len(m.tenantNames()) {
			m.tenantIndex = 0
		}
		m.usageScroll = 0
		return true
	case "l":
		m.cycleLogLevel()
		m.logScroll = 0
		m.bottomTab = bottomTabLogs
		return true
	case "j", "down":
		return m.scrollFocused(1)
	case "k", "up":
		return m.scrollFocused(-1)
	case "J":
		return m.resize(2)
	case "K":
		return m.resize(-2)
	case "+", "=":
		return m.resize(1)
	case "-", "_":
		return m.resize(-1)
	case "g", "home":
		return m.scrollTop()
	case "G", "end":
		return m.scrollBottom()
	}
	return false
}

func (m *model) cycleLogLevel() {
	if !m.logFilterOn {
		m.logFilterOn = true
		m.logMinLevel = slog.LevelDebug
		return
	}
	switch m.logMinLevel {
	case slog.LevelDebug:
		m.logMinLevel = slog.LevelInfo
	case slog.LevelInfo:
		m.logMinLevel = slog.LevelWarn
	case slog.LevelWarn:
		m.logMinLevel = slog.LevelError
	default:
		m.logFilterOn = false
		m.logMinLevel = slog.LevelDebug
	}
}

func (m *model) scrollFocused(delta int) bool {
	switch m.focus {
	case focusProviders:
		return m.scrollProviders(delta)
	case focusUsage:
		return m.scrollUsage(delta)
	default:
		if m.bottomTab == bottomTabAliases {
			return m.scrollAliases(delta)
		}
		return m.scrollLogs(delta)
	}
}

func (m *model) scrollTop() bool {
	switch m.focus {
	case focusProviders:
		if m.providerScroll == 0 {
			return false
		}
		m.providerScroll = 0
		return true
	case focusUsage:
		if m.usageScroll == 0 {
			return false
		}
		m.usageScroll = 0
		return true
	default:
		if m.bottomTab == bottomTabAliases {
			if m.aliasScroll == 0 {
				return false
			}
			m.aliasScroll = 0
			return true
		}
		total := len(m.filteredLogs(1 << 30))
		max := total - m.bottomVisibleRows()
		if max < 0 {
			max = 0
		}
		if m.logScroll == max {
			return false
		}
		m.logScroll = max
		return true
	}
}

func (m *model) scrollBottom() bool {
	switch m.focus {
	case focusProviders:
		max := m.maxProviderScroll()
		if m.providerScroll == max {
			return false
		}
		m.providerScroll = max
		return true
	case focusUsage:
		max := m.maxUsageScroll()
		if m.usageScroll == max {
			return false
		}
		m.usageScroll = max
		return true
	default:
		if m.bottomTab == bottomTabAliases {
			max := m.maxAliasScroll()
			if m.aliasScroll == max {
				return false
			}
			m.aliasScroll = max
			return true
		}
		if m.logScroll == 0 {
			return false
		}
		m.logScroll = 0
		return true
	}
}

func (m *model) scrollProviders(delta int) bool {
	max := m.maxProviderScroll()
	next := m.providerScroll + delta
	if next < 0 {
		next = 0
	}
	if next > max {
		next = max
	}
	if next == m.providerScroll {
		return false
	}
	m.providerScroll = next
	return true
}

func (m *model) scrollUsage(delta int) bool {
	max := m.maxUsageScroll()
	next := m.usageScroll + delta
	if next < 0 {
		next = 0
	}
	if next > max {
		next = max
	}
	if next == m.usageScroll {
		return false
	}
	m.usageScroll = next
	return true
}

func (m *model) scrollAliases(delta int) bool {
	max := m.maxAliasScroll()
	next := m.aliasScroll + delta
	if next < 0 {
		next = 0
	}
	if next > max {
		next = max
	}
	if next == m.aliasScroll {
		return false
	}
	m.aliasScroll = next
	return true
}

func (m *model) scrollLogs(delta int) bool {
	if delta < 0 {
		m.logScroll -= delta
		m.clampLogScroll()
		return true
	}
	if delta > 0 {
		if m.logScroll < delta {
			if m.logScroll == 0 {
				return false
			}
			m.logScroll = 0
			return true
		}
		m.logScroll -= delta
		return true
	}
	return false
}

func (m *model) clampScroll() {
	if m.usageScroll > m.maxUsageScroll() {
		m.usageScroll = m.maxUsageScroll()
	}
	if m.usageScroll < 0 {
		m.usageScroll = 0
	}
	if m.providerScroll > m.maxProviderScroll() {
		m.providerScroll = m.maxProviderScroll()
	}
	if m.providerScroll < 0 {
		m.providerScroll = 0
	}
	if m.aliasScroll > m.maxAliasScroll() {
		m.aliasScroll = m.maxAliasScroll()
	}
	if m.aliasScroll < 0 {
		m.aliasScroll = 0
	}
	m.clampLogScroll()
}

func (m *model) clampLogScroll() {
	if m.logScroll < 0 {
		m.logScroll = 0
	}
	if m.snapshot == nil || m.snapshot.Logs == nil {
		if m.logScroll > 0 && m.snapshot == nil {
			m.logScroll = 0
		}
		return
	}
	total := len(m.filteredLogs(1 << 30))
	visible := m.logVisibleRows()
	max := total - visible
	if max < 0 {
		max = 0
	}
	if m.logScroll > max {
		m.logScroll = max
	}
}

func providerNoteCount(summaries []accounting.Summary) int {
	for _, s := range summaries {
		if strings.HasPrefix(s.Model, "_") {
			return 1
		}
	}
	return 0
}

func (m *model) providerDataRows() int {
	if m.snapshot == nil {
		return 0
	}
	return len(m.snapshot.Providers) + len(m.snapshot.DisabledProviders)
}

func (m *model) providerVisibleRows() int {
	notes := 0
	if m.snapshot != nil {
		notes = providerNoteCount(m.snapshot.Usage.Summaries())
	}
	n := m.effStatsHeight() - 4 - notes
	if n < 1 {
		n = 1
	}
	return n
}

func (m *model) maxProviderScroll() int {
	if m.snapshot == nil {
		return 0
	}
	max := m.providerDataRows() - m.providerVisibleRows()
	if max < 0 {
		max = 0
	}
	return max
}

func (m *model) maxUsageScroll() int {
	total := len(m.filteredSummaries())
	visible := m.usageVisibleRows()
	max := total - visible
	if max < 0 {
		max = 0
	}
	return max
}

func (m *model) maxAliasScroll() int {
	total := len(m.aliasRows())
	visible := m.aliasVisibleRows()
	max := total - visible
	if max < 0 {
		max = 0
	}
	return max
}

func zoomBodyHeight(height int) int {
	h := height - chromeLines
	if h < 4 {
		h = 4
	}
	return h
}

func (m *model) effStatsHeight() int {
	if m.zoomed && m.focus != focusBottom {
		return zoomBodyHeight(m.height)
	}
	return m.statsHeight
}

func (m *model) effBottomHeight() int {
	if m.zoomed && m.focus == focusBottom {
		return zoomBodyHeight(m.height)
	}
	return m.bottomHeight
}

func (m *model) usageVisibleRows() int {
	n := m.effStatsHeight() - 5
	if n < 1 {
		n = 1
	}
	return n
}

func (m *model) bottomVisibleRows() int {
	n := m.effBottomHeight() - 5
	if n < 1 {
		n = 1
	}
	return n
}

func (m *model) aliasVisibleRows() int {
	return m.bottomVisibleRows()
}

func (m *model) logVisibleRows() int {
	return m.bottomVisibleRows()
}

func (m *model) resize(delta int) bool {
	newBottom := m.bottomHeight + delta
	maxBottom := m.height - chromeLines - statsMinHeight
	if maxBottom < bottomMinHeight {
		maxBottom = bottomMinHeight
	}
	if newBottom < bottomMinHeight {
		newBottom = bottomMinHeight
	}
	if newBottom > maxBottom {
		newBottom = maxBottom
	}
	if newBottom == m.bottomHeight {
		return false
	}
	m.bottomHeight = newBottom
	m.relayoutStatsBottom()
	return true
}

func (m *model) relayout() {
	if m.height <= 11 {
		m.bottomHeight = 0
		m.statsHeight = 0
		return
	}
	available := m.height - chromeLines
	if available < 12 {
		m.statsHeight = available
		m.bottomHeight = 0
		return
	}
	if m.bottomHeight <= 0 {
		m.bottomHeight = available / 3
	}
	m.relayoutStatsBottom()
}

func (m *model) relayoutStatsBottom() {
	if m.height <= 11 {
		return
	}
	available := m.height - chromeLines
	bottom := m.bottomHeight
	if bottom < bottomMinHeight {
		bottom = bottomMinHeight
	}
	if bottom > available-statsMinHeight {
		bottom = available - statsMinHeight
		if bottom < bottomMinHeight {
			bottom = bottomMinHeight
		}
	}
	m.bottomHeight = bottom
	m.statsHeight = available - bottom
	if m.statsHeight < statsMinHeight {
		m.statsHeight = statsMinHeight
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
}

func (m *model) render() string {
	if m.snapshot == nil {
		return "no snapshot"
	}
	if m.showHelp {
		return m.renderHelp()
	}
	if m.width < 80 || m.height < 12 {
		return fmt.Sprintf("Terminal too small (%dx%d). Need at least 80x12.", m.width, m.height)
	}
	header := renderHeader(m)
	rate := renderRate(m, m.width)
	if m.zoomed {
		bodyHeight := zoomBodyHeight(m.height)
		var body string
		switch m.focus {
		case focusProviders:
			body = renderProviders(m, m.width, bodyHeight)
		case focusUsage:
			body = renderUsage(m, m.width, bodyHeight)
		default:
			body = renderBottom(m, m.width, bodyHeight)
		}
		return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, body, renderFooter(m)), m.width)
	}
	statsHeight := m.effStatsHeight()
	if statsHeight < 4 {
		statsHeight = 4
	}
	sideWidth := m.sideWidth()
	usageWidth := m.width - sideWidth
	side := renderProviders(m, sideWidth, statsHeight)
	usage := renderUsage(m, usageWidth, statsHeight)
	mid := lipgloss.JoinHorizontal(lipgloss.Top, side, usage)
	bottom := renderBottom(m, m.width, m.effBottomHeight())
	return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, mid, bottom, renderFooter(m)), m.width)
}

var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;:?]*[ -/]*[@-~]|\x1b\\][^\x07]*(?:\x07|\x1b\\\\)|\x1b[()][0-9A-Za-z]")

func visibleLen(s string) int {
	return len([]rune(ansiSeq.ReplaceAllString(s, "")))
}

func padLine(s string, w int) string {
	if n := visibleLen(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func fitView(out string, width int) string {
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		lines[i] = padLine(l, width)
	}
	return strings.Join(lines, "\n")
}

func (m *model) sideWidth() int {
	if m.width <= 0 {
		return 0
	}
	return m.width / 2
}

func renderBottom(m *model, width, height int) string {
	paneHeight := height - 1
	if paneHeight < 4 {
		paneHeight = 4
	}
	strip := renderTabStrip(m, width)
	var pane string
	if m.bottomTab == bottomTabAliases {
		pane = renderAliases(m, width, paneHeight)
	} else {
		pane = renderLogs(m, width, paneHeight)
	}
	return lipgloss.JoinVertical(lipgloss.Left, strip, pane)
}

func renderTabStrip(m *model, width int) string {
	activeStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))
	aliasLabel := fmt.Sprintf("1:Aliases(%d) cool:%d", len(m.snapshot.Aliases), len(m.snapshot.Cooldowns))
	level := "all"
	if m.logFilterOn {
		level = ">=" + m.logMinLevel.String()
	}
	logsLabel := fmt.Sprintf("2:Logs(%s)", level)
	var left, right string
	if m.bottomTab == bottomTabAliases {
		left = activeStyle.Render("▸ " + aliasLabel)
		right = dimStyle.Render("  " + logsLabel)
	} else {
		left = dimStyle.Render("  " + aliasLabel)
		right = activeStyle.Render("▸ " + logsLabel)
	}
	return left + "  " + right
}

func (m *model) renderHelp() string {
	lines := []string{
		"aiproxy dashboard — keys",
		"",
		"  tab        cycle focus PROVIDERS / USAGE / bottom tabs",
		"  1/2 or [/] switch bottom tab (Aliases / Logs)",
		"  enter      zoom focused pane to full screen",
		"  esc        unzoom (or quit when not zoomed)",
		"  j/k dn/up  scroll focused pane   g/G,home/end top/bottom",
		"  +/- J/K    resize bottom pane",
		"  t          cycle tenant filter    e toggle errors-only",
		"  u          toggle usage view (public vs upstream model)",
		"  l          cycle log level filter (all/debug/info/warn/error)",
		"  p          pause live updates (buffer one snapshot)",
		"  ?/h        toggle this help       q/Esc/Ctrl+C quit",
		"",
		"Legend: ✓ healthy · ✗ unhealthy · ? unknown (no health report yet)",
		"  ERR% excludes 429 (throttled shown separately) · ~ = streaming or",
		"  untokenized response (no token accounting) · n/a = too few latency",
		"  samples · cool Ns = alias target cooling with remaining time.",
		"",
		"Press ? to close.",
	}
	return lipgloss.NewStyle().Width(m.width).Render(strings.Join(lines, "\n"))
}

func renderFooter(m *model) string {
	focusName := "USAGE"
	if m.focus == focusProviders {
		focusName = "PROVIDERS"
	} else if m.focus == focusBottom {
		focusName = "ALIASES"
		if m.bottomTab == bottomTabLogs {
			focusName = "LOGS"
		}
	}
	state := "LIVE"
	if m.paused {
		state = "PAUSED"
	} else if m.staleErr != "" {
		state = "STALE"
	}
	zoomHint := "[enter] zoom"
	if m.zoomed {
		zoomHint = "[esc] unzoom"
	}
	base := fmt.Sprintf("%s focus:%s [tab] pane [1/2] tabs [j/k] scroll [t]enant [e]rrs [u]pstream [l]evel [p]ause %s [?]help [q]uit", state, focusName, zoomHint)
	if len([]rune(base)) > m.width && m.width > 20 {
		base = truncate(base, m.width)
	}
	legend := "ERR% excl 429 · ~=stream/no-tokens · ?=unknown health · n/a=sparse latency"
	return lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render(base + "\n" + legend)
}

func renderHeader(m *model) string {
	snap := m.snapshot
	uptime := m.now.Sub(snap.StartTime).Round(time.Second)
	left := fmt.Sprintf("aiproxy %s  %s", snap.Version, snap.Address)
	active := len(snap.Providers)
	disabled := len(snap.DisabledProviders)
	status := "LIVE"
	statusColor := "#22C55E"
	if m.paused {
		status = "PAUSED"
		statusColor = "#FBBF24"
		if m.hasPending {
			status = "PAUSED+1"
		}
	} else if m.staleErr != "" {
		age := m.now.Sub(m.staleAt).Round(time.Second)
		status = fmt.Sprintf("STALE %s (%s)", age, truncate(m.staleErr, 40))
		statusColor = "#F87171"
	}
	right := fmt.Sprintf("providers %d (+%d disabled)  aliases %d  auth %s  uptime %s",
		active, disabled, len(snap.Aliases), snap.AuthMode, uptime)
	leftStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true)
	rightStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	statusStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(statusColor)).Bold(true)
	full := len([]rune(left)) + len([]rune(status)) + len([]rune(right)) + 6
	if overflow := full - m.width; overflow > 0 {
		keep := len([]rune(right)) - overflow
		if keep < 0 {
			keep = 0
		}
		right = truncate(right, keep)
	}
	return leftStyle.Render(left) + "  " + statusStyle.Render("["+status+"]") + "  " + rightStyle.Render(right)
}

type rateStats struct {
	perMin     [15]int64
	errPerMin  [15]int64
	r429PerMin [15]int64
	req1m      int64
	req5m      int64
	err1m      int64
	r429       int64
	tokensMin  int64
	total      int64
	unresolved int64
	cooldowns  int
}

func computeRates(m *model) rateStats {
	var rs rateStats
	rs.cooldowns = len(m.snapshot.Cooldowns)
	summaries := m.snapshot.Usage.Summaries()
	for _, s := range summaries {
		if strings.HasPrefix(s.Model, "_") {
			rs.unresolved += s.Count
			continue
		}
		rs.total += s.Count
	}
	recent := m.snapshot.Usage.Recent(recentLimit)
	now := m.now
	if now.IsZero() {
		now = time.Now()
	}
	var tokensRecent int64
	var tokensN int64
	for _, e := range recent {
		if e.Timestamp.IsZero() {
			continue
		}
		age := now.Sub(e.Timestamp)
		if age < 0 || age > 15*time.Minute {
			continue
		}
		bucket := int(age / time.Minute)
		if bucket > 14 {
			bucket = 14
		}
		rs.perMin[14-bucket]++
		if e.StatusCode == 429 {
			rs.r429PerMin[14-bucket]++
		} else if e.StatusCode >= 400 || e.StatusCode == 0 {
			rs.errPerMin[14-bucket]++
		}
		if e.TotalTokens > 0 && age <= time.Minute {
			tokensRecent += e.TotalTokens
			tokensN++
		}
	}
	for i := 14; i >= 0; i-- {
		if 14-i < 1 {
			rs.req1m += rs.perMin[i]
			rs.err1m += rs.errPerMin[i]
			rs.r429 += rs.r429PerMin[i]
		}
		if 14-i < 5 {
			rs.req5m += rs.perMin[i]
		}
	}
	_ = tokensN
	rs.tokensMin = tokensRecent
	if rs.req1m == 0 && rs.total > 0 {
		rs.tokensMin = tokensPerMinuteFallback(summaries)
	}
	return rs
}

func tokensPerMinuteFallback(summaries []accounting.Summary) int64 {
	var total int64
	for _, s := range summaries {
		total += s.TotalTokens
	}
	return total
}

func sparkline(vals [15]int64) string {
	const glyphs = " ▁▂▃▄▅▆▇█"
	var max int64
	for _, v := range vals {
		if v > max {
			max = v
		}
	}
	var b strings.Builder
	for _, v := range vals {
		if max == 0 {
			b.WriteString(" ")
			continue
		}
		idx := int(math.Ceil(float64(v) / float64(max) * 8))
		if idx < 0 {
			idx = 0
		}
		if idx > 8 {
			idx = 8
		}
		b.WriteRune([]rune(glyphs)[idx])
	}
	return b.String()
}

func renderRate(m *model, width int) string {
	rs := computeRates(m)
	reqRate1 := float64(rs.req1m) / 60.0
	reqRate5 := float64(rs.req5m) / 300.0
	errPct := 0.0
	if rs.req1m > 0 {
		errPct = 100 * float64(rs.err1m) / float64(rs.req1m)
	}
	line1 := fmt.Sprintf("req/s 1m %.2f 5m %.2f · err1m %.1f%% · 429/1m %s · tok/1m %s · total %s · cool %s · unresolved %s  req/min ▁15m→ [%s]",
		reqRate1, reqRate5, errPct, comma(rs.r429), comma(rs.tokensMin), comma(rs.total), comma(int64(rs.cooldowns)), comma(rs.unresolved), sparkline(rs.perMin))
	if runeLen(line1) > width && width > 20 {
		line1 = truncate(line1, width)
	}
	filter := "tenant:all"
	if t := m.activeTenant(); t != "" {
		filter = "tenant:" + t
	}
	if m.errorsOnly {
		filter += " errs-only"
	}
	level := "level:all"
	if m.logFilterOn {
		level = "level>=" + m.logMinLevel.String()
	}
	usageTotal := len(m.filteredSummaries())
	usagePos := 0
	if usageTotal > 0 {
		usagePos = m.usageScroll + 1
	}
	line2 := fmt.Sprintf("filters: %s · %s · usage %d/%d · aliases %d · %s",
		filter, level, usagePos, usageTotal, len(m.snapshot.Aliases), rateHint(m))
	if runeLen(line2) > width && width > 20 {
		line2 = truncate(line2, width)
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	return style.Render(line1 + "\n" + line2)
}

func rateHint(m *model) string {
	if m.paused {
		return "paused — p resumes"
	}
	if m.staleErr != "" {
		return "poll failing — showing last snapshot"
	}
	return "poll 2s"
}

func (m *model) tenantNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range m.snapshot.Usage.Summaries() {
		if s.Tenant == "" || seen[s.Tenant] {
			continue
		}
		seen[s.Tenant] = true
		out = append(out, s.Tenant)
	}
	sort.Strings(out)
	return out
}

func (m *model) activeTenant() string {
	names := m.tenantNames()
	if len(names) == 0 || m.tenantIndex <= 0 {
		return ""
	}
	return names[(m.tenantIndex-1)%len(names)]
}

func (m *model) filteredSummaries() []accounting.Summary {
	summaries := m.snapshot.Usage.Summaries()
	if m.usageUpstream {
		summaries = upstreamAsSummaries(m.snapshot.Usage.UpstreamSummaries())
	}
	tenant := ""
	if names := m.tenantNames(); len(names) > 0 {
		if m.tenantIndex > 0 {
			tenant = names[(m.tenantIndex-1)%len(names)]
		}
	}
	var out []accounting.Summary
	for _, s := range summaries {
		if strings.HasPrefix(s.Model, "_") {
			continue
		}
		if tenant != "" && s.Tenant != tenant {
			continue
		}
		if m.errorsOnly && !(s.StatusCode >= 400 || s.StatusCode == 0) {
			continue
		}
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Count > out[j].Count
	})
	return out
}

func upstreamAsSummaries(upstream []accounting.UpstreamSummary) []accounting.Summary {
	out := make([]accounting.Summary, 0, len(upstream))
	for _, u := range upstream {
		out = append(out, accounting.Summary{
			Tenant:           u.Tenant,
			Client:           u.Client,
			Model:            u.Provider + "/" + u.Model,
			Operation:        u.Operation,
			StatusCode:       u.StatusCode,
			Count:            u.Count,
			PromptTokens:     u.PromptTokens,
			CompletionTokens: u.CompletionTokens,
			TotalTokens:      u.TotalTokens,
		})
	}
	return out
}

func renderProviders(m *model, width, height int) string {
	snap := m.snapshot
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width - 2).
		Height(height - 2)
	if m.focus == focusProviders {
		border = border.BorderForeground(lipgloss.Color("#38BDF8"))
	}
	inner := width - 2
	summaries := snap.Usage.Summaries()
	stats := snap.Usage.ProviderSummaries()
	if len(stats) == 0 && len(summaries) > 0 {
		stats = accounting.ByProvider(summaries)
	}
	byName := make(map[string]accounting.ProviderSummary, len(stats))
	for _, ps := range stats {
		byName[ps.Provider] = ps
	}
	latency, samples := p95WithSamples(snap.Usage.Recent(recentLimit))
	var names []string
	var ips []string
	var reqs, t429s, toks []int64
	for _, p := range snap.Providers {
		ps := byName[p.Name]
		names = append(names, p.Name)
		ips = append(ips, m.providerIP(p.BaseURL))
		reqs = append(reqs, ps.Requests)
		t429s = append(t429s, ps.Throttled)
		toks = append(toks, ps.TotalTokens)
	}
	for _, p := range snap.DisabledProviders {
		names = append(names, p.Name)
		ips = append(ips, m.providerIP(p.BaseURL))
		reqs = append(reqs, 0)
		t429s = append(t429s, 0)
		toks = append(toks, 0)
	}
	nameW, ipW, reqW, t429W, tokW := providerColWidths(names, ips, reqs, t429s, toks, inner)
	rows := []string{headerStyle.Render(fitRow(headerCells([]col{{"PROVIDER", nameW}, {"", 1}, {"REQS", reqW}, {"ERR%", 6}, {"429", t429W}, {"P95", 8}, {"TOKENS", tokW}, {"IP", ipW}}), inner))}
	unresolved := int64(0)
	for _, s := range summaries {
		if strings.HasPrefix(s.Model, "_") {
			unresolved += s.Count
		}
	}
	type providerLine struct {
		text string
		dim  bool
	}
	var data []providerLine
	for _, p := range snap.Providers {
		known, healthy := healthKnown(m.health, p.Name)
		ps := byName[p.Name]
		data = append(data, providerLine{text: providerRow(p.Name, known, healthy, ps, ps.Throttled, latency[p.Name], samples[p.Name], false, m.providerIP(p.BaseURL), nameW, ipW, reqW, t429W, tokW)})
	}
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))
	for _, p := range snap.DisabledProviders {
		data = append(data, providerLine{
			text: providerRow(p.Name, true, false, accounting.ProviderSummary{}, 0, 0, 0, true, m.providerIP(p.BaseURL), nameW, ipW, reqW, t429W, tokW),
			dim:  true,
		})
	}
	notes := 0
	if unresolved > 0 {
		notes = 1
	}
	visible := height - 4 - notes
	if visible < 1 {
		visible = 1
	}
	maxStart := len(data) - visible
	if maxStart < 0 {
		maxStart = 0
	}
	start := m.providerScroll
	if start > maxStart {
		start = maxStart
	}
	end := start + visible
	if end > len(data) {
		end = len(data)
	}
	for _, d := range data[start:end] {
		line := fitRow(d.text, inner)
		if d.dim {
			line = dimStyle.Render(line)
		}
		rows = append(rows, line)
	}
	if len(data) == 0 {
		rows = append(rows, "no providers configured")
	}
	if end < len(data) {
		rows = append(rows, dimStyle.Render(fitRow(fmt.Sprintf("… %d more (j/k scroll)", len(data)-end), inner)))
	}
	if unresolved > 0 {
		rows = append(rows, dimStyle.Render(fitRow(fmt.Sprintf("unresolved/forbidden: %s reqs (hidden)", comma(unresolved)), inner)))
	}
	return border.Render(strings.Join(rows, "\n"))
}

func healthKnown(health map[string]bool, name string) (bool, bool) {
	if health == nil {
		return false, false
	}
	v, ok := health[name]
	return ok, v
}

var lookupHostIPs = func(ctx context.Context, host string) ([]string, error) {
	addrs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.IP.String())
	}
	sort.Strings(out)
	return out, nil
}

func baseURLHost(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	u, err := url.Parse(baseURL)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Hostname()
}

func formatIPs(addrs []string) string {
	if len(addrs) == 0 {
		return "-"
	}
	if len(addrs) == 1 {
		return addrs[0]
	}
	return fmt.Sprintf("%s +%d", addrs[0], len(addrs)-1)
}

func (m *model) providerIP(baseURL string) string {
	host := baseURLHost(baseURL)
	if host == "" {
		return "-"
	}
	if ip := net.ParseIP(host); ip != nil {
		return host
	}
	if m == nil {
		return "-"
	}
	if m.ipCache == nil {
		m.ipCache = map[string]string{}
	}
	if cached, ok := m.ipCache[host]; ok {
		return cached
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	addrs, err := lookupHostIPs(ctx, host)
	if err != nil || len(addrs) == 0 {
		m.ipCache[host] = "-"
		return "-"
	}
	sort.Strings(addrs)
	display := formatIPs(addrs)
	m.ipCache[host] = display
	return display
}

func runeLen(s string) int {
	return len([]rune(s))
}

func comma(n int64) string {
	s := fmt.Sprintf("%d", n)
	neg := strings.HasPrefix(s, "-")
	if neg {
		s = s[1:]
	}
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	rem := len(s) % 3
	if rem > 0 {
		b.WriteString(s[:rem])
	}
	for i := rem; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func providerColWidths(names, ips []string, requests, throttled, tokens []int64, inner int) (nameW, ipW, reqW, t429W, tokW int) {
	nameW, ipW, reqW, t429W, tokW = len("PROVIDER"), len("IP"), len("REQS"), len("429"), len("TOKENS")
	for _, n := range names {
		nameW = max(nameW, runeLen(n))
	}
	for _, ip := range ips {
		ipW = max(ipW, runeLen(ip))
	}
	for _, r := range requests {
		reqW = max(reqW, runeLen(comma(r)))
	}
	for _, r := range throttled {
		t429W = max(t429W, runeLen(comma(r)))
	}
	for _, t := range tokens {
		tokW = max(tokW, runeLen(comma(t)))
	}
	nameW = min(nameW, 24)
	ipW = min(ipW, 21)
	reqW = min(reqW, 10)
	t429W = min(t429W, 8)
	tokW = min(tokW, 14)
	const fixed = 1 + 6 + 8
	const gaps = 7
	limit := inner - 2
	if limit < 20 {
		limit = 20
	}
	for nameW+ipW+reqW+t429W+tokW+fixed+gaps > limit && nameW > 8 {
		nameW--
	}
	for nameW+ipW+reqW+t429W+tokW+fixed+gaps > limit && ipW > 7 {
		ipW--
	}
	for nameW+ipW+reqW+t429W+tokW+fixed+gaps > limit && tokW > 6 {
		tokW--
	}
	for nameW+ipW+reqW+t429W+tokW+fixed+gaps > limit && reqW > 4 {
		reqW--
	}
	for nameW+ipW+reqW+t429W+tokW+fixed+gaps > limit && t429W > 3 {
		t429W--
	}
	for nameW+ipW+reqW+t429W+tokW+fixed+gaps > limit && ipW > 2 {
		ipW--
	}
	return nameW, ipW, reqW, t429W, tokW
}

func usageColWidths(summaries []accounting.Summary, inner int) (modelW, opW, countW, tokW int) {
	modelW, opW, countW, tokW = len("MODEL"), len("OP"), len("COUNT"), len("TOKENS")
	for _, s := range summaries {
		modelW = max(modelW, runeLen(s.Model))
		opW = max(opW, runeLen(s.Operation))
		countW = max(countW, runeLen(comma(s.Count)))
		tokW = max(tokW, runeLen(tokensText(s)))
	}
	modelW = min(modelW, 64)
	opW = min(opW, 20)
	countW = min(countW, 10)
	tokW = min(tokW, 14)
	const statusW = 6
	const gaps = 4
	for modelW+opW+statusW+countW+tokW+gaps > inner && modelW > 10 {
		modelW--
	}
	for modelW+opW+statusW+countW+tokW+gaps > inner && tokW > 6 {
		tokW--
	}
	for modelW+opW+statusW+countW+tokW+gaps > inner && opW > 8 {
		opW--
	}
	for modelW+opW+statusW+countW+tokW+gaps > inner && countW > 4 {
		countW--
	}
	return modelW, opW, countW, tokW
}

func tokensText(s accounting.Summary) string {
	if s.TotalTokens == 0 {
		return "~"
	}
	return comma(s.TotalTokens)
}

func providerRow(name string, known, healthy bool, ps accounting.ProviderSummary, throttled int64, p95 time.Duration, samples int, disabled bool, ip string, nameW, ipW, reqW, t429W, tokW int) string {
	status := "✓"
	if disabled {
		status = "✗"
	} else if !known {
		status = "?"
	} else if !healthy {
		status = "✗"
	}
	errPct := 0.0
	if ps.Requests > 0 {
		errPct = 100 * float64(ps.Errors) / float64(ps.Requests)
	}
	p95cell := "     n/a"
	if samples > 0 {
		p95cell = fmt.Sprintf("%8s", p95.Round(time.Millisecond))
	}
	return dataRow([]string{
		truncate(name, nameW),
		status,
		fmt.Sprintf("%*s", reqW, comma(ps.Requests)),
		fmt.Sprintf("%5.1f%%", errPct),
		fmt.Sprintf("%*s", t429W, comma(throttled)),
		p95cell,
		fmt.Sprintf("%*s", tokW, comma(ps.TotalTokens)),
		truncate(ip, ipW),
	}, []int{nameW, 1, reqW, 6, t429W, 8, tokW, ipW})
}

func p95LatencyByProvider(recent []accounting.Event) map[string]time.Duration {
	latency, _ := p95WithSamples(recent)
	return latency
}

func p95WithSamples(recent []accounting.Event) (map[string]time.Duration, map[string]int) {
	out := map[string]time.Duration{}
	counts := map[string]int{}
	byProvider := map[string][]time.Duration{}
	for _, e := range recent {
		if e.Duration <= 0 {
			continue
		}
		prov := accounting.EventProvider(e)
		byProvider[prov] = append(byProvider[prov], e.Duration)
	}
	for prov, durs := range byProvider {
		out[prov] = percentile(durs, 0.95)
		counts[prov] = len(durs)
	}
	return out, counts
}

func percentile(durs []time.Duration, p float64) time.Duration {
	if len(durs) == 0 {
		return 0
	}
	sort.Slice(durs, func(i, j int) bool { return durs[i] < durs[j] })
	idx := int(math.Ceil(p*float64(len(durs)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(durs) {
		idx = len(durs) - 1
	}
	return durs[idx]
}

func renderUsage(m *model, width, height int) string {
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width - 2).
		Height(height - 2)
	if m.focus == focusUsage {
		border = border.BorderForeground(lipgloss.Color("#38BDF8"))
	}
	tenant := m.activeTenant()
	title := "USAGE"
	if m.usageUpstream {
		title += " upstream"
	}
	if tenant != "" {
		title += " " + tenant
	}
	if m.errorsOnly {
		title += " errs-only"
	}
	inner := width - 2
	summaries := m.filteredSummaries()
	modelW, opW, countW, tokW := usageColWidths(summaries, inner)
	header := headerStyle.Render(fitRow(headerCells([]col{{"MODEL", modelW}, {"OP", opW}, {"STATUS", 6}, {"COUNT", countW}, {"TOKENS", tokW}}), inner))
	rows := []string{title, header}
	visible := m.usageVisibleRows()
	total := len(summaries)
	start := m.usageScroll
	if start > total {
		start = total
	}
	end := start + visible
	if end > total {
		end = total
	}
	for _, s := range summaries[start:end] {
		rows = append(rows, fitRow(dataRow([]string{
			truncate(s.Model, modelW),
			truncate(s.Operation, opW),
			fmt.Sprintf("%d", s.StatusCode),
			fmt.Sprintf("%*s", countW, comma(s.Count)),
			tokensCell(s, tokW),
		}, []int{modelW, opW, 6, countW, tokW}), inner))
	}
	if total == 0 {
		rows = append(rows, "no usage recorded yet")
	} else if end < total {
		rows = append(rows, fmt.Sprintf("… %d more (j/k scroll)", total-end))
	}
	return border.Render(strings.Join(rows, "\n"))
}

func tokensCell(s accounting.Summary, w int) string {
	return fmt.Sprintf("%*s", w, tokensText(s))
}

func (m *model) aliasRows() []string {
	var rows []string
	cooldown := map[string]time.Duration{}
	for _, c := range m.snapshot.Cooldowns {
		key := c.Alias + "\x00" + c.Provider + "\x00" + c.Model
		d := time.Duration(c.RemainingMs) * time.Millisecond
		if d > cooldown[key] {
			cooldown[key] = d
		}
	}
	for _, a := range m.snapshot.Aliases {
		retry := "default"
		if len(a.RetryStatusCodes) > 0 {
			parts := make([]string, len(a.RetryStatusCodes))
			for i, code := range a.RetryStatusCodes {
				parts[i] = fmt.Sprintf("%d", code)
			}
			retry = strings.Join(parts, ",")
		}
		algo := string(a.Algorithm)
		if algo == "" {
			algo = "-"
		}
		rows = append(rows, fmt.Sprintf("alias/%s [%s] retry:%s", a.Name, algo, retry))
		for _, t := range a.Targets {
			key := a.Name + "\x00" + t.Provider + "\x00" + t.Model
			state := "-"
			if d, ok := cooldown[key]; ok && d > 0 {
				state = "cool " + d.Round(time.Second).String()
			}
			known, healthy := healthKnown(m.health, t.Provider)
			mark := "✓"
			if !known {
				mark = "?"
			} else if !healthy {
				mark = "✗"
			}
			rows = append(rows, fmt.Sprintf("  %s %s/%s %s", mark, t.Provider, t.Model, state))
		}
	}
	return rows
}

func renderAliases(m *model, width, height int) string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width - 2).
		Height(height - 2)
	if m.focus == focusBottom && m.bottomTab == bottomTabAliases {
		borderStyle = borderStyle.BorderForeground(lipgloss.Color("#38BDF8"))
	}
	rows := []string{fmt.Sprintf("ALIASES (%d) cool:%d", len(m.snapshot.Aliases), len(m.snapshot.Cooldowns))}
	all := m.aliasRows()
	if len(all) == 0 {
		rows = append(rows, "no aliases configured")
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	visible := m.aliasVisibleRows()
	start := m.aliasScroll
	if start > len(all) {
		start = len(all)
	}
	end := start + visible
	if end > len(all) {
		end = len(all)
	}
	for _, r := range all[start:end] {
		rows = append(rows, truncate(r, width-2))
	}
	if end < len(all) {
		rows = append(rows, fmt.Sprintf("… %d more (j/k scroll)", len(all)-end))
	}
	return borderStyle.Render(strings.Join(rows, "\n"))
}

func (m *model) filteredLogs(limit int) []observability.LogEntry {
	if m.snapshot == nil || m.snapshot.Logs == nil {
		return nil
	}
	all := m.snapshot.Logs.Since(1 << 30)
	if !m.logFilterOn {
		if len(all) > limit {
			return all[len(all)-limit:]
		}
		return all
	}
	out := make([]observability.LogEntry, 0, len(all))
	for _, e := range all {
		if e.Level >= m.logMinLevel {
			out = append(out, e)
		}
	}
	if len(out) > limit {
		return out[len(out)-limit:]
	}
	return out
}

func renderLogs(m *model, width, height int) string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width - 2).
		Height(height - 2)
	if m.focus == focusBottom && m.bottomTab == bottomTabLogs {
		borderStyle = borderStyle.BorderForeground(lipgloss.Color("#38BDF8"))
	}
	var page []observability.LogEntry
	var total int
	if height > 3 {
		visible := m.logVisibleRows()
		total = len(m.filteredLogs(1 << 30))
		all := m.filteredLogs(m.logScroll + visible)
		start := 0
		if len(all) > visible {
			start = len(all) - visible
		}
		page = all[start:]
	}
	attrsWidth := len("ATTRS")
	for _, e := range page {
		attrsWidth = max(attrsWidth, runeLen(orDash(e.Attrs)))
	}
	msgWidth := width - 8 - 6 - 4 - attrsWidth - 8
	for msgWidth < 12 && attrsWidth > 12 {
		attrsWidth--
		msgWidth = width - 8 - 6 - 4 - attrsWidth - 8
	}
	if msgWidth < 12 {
		msgWidth = 12
	}
	levelName := "all"
	if m.logFilterOn {
		levelName = ">=" + m.logMinLevel.String()
	}
	title := fmt.Sprintf("LOGS (%s scroll:%d)", levelName, m.logScroll)
	rows := []string{title, headerRow([]col{{"AT", 8}, {"LEVEL", 6}, {"MESSAGE", msgWidth}, {"ATTRS", attrsWidth}})}
	if height > 3 {
		visible := m.logVisibleRows()
		for _, e := range page {
			rows = append(rows, renderLogEntry(msgWidth, attrsWidth, e))
		}
		if len(page) == 0 {
			rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("no logs captured"))
		} else if total > visible {
			shown := len(page)
			rows[len(rows)-1] += fmt.Sprintf(" (%d/%d)", shown, total)
		}
	}
	return borderStyle.Render(strings.Join(rows, "\n"))
}

func renderLogEntry(msgWidth, attrsWidth int, e observability.LogEntry) string {
	levelStyle := levelStyleFor(e.Level)
	attrs := e.Attrs
	if runeLen(attrs) > attrsWidth {
		attrs = truncate(attrs, attrsWidth)
	}
	return dataRow([]string{
		e.Time.Format("15:04:05"),
		levelStyle.Render(padRight(truncate(e.Level.String(), 6), 6)),
		truncate(e.Message, msgWidth),
		truncate(orDash(attrs), attrsWidth),
	}, []int{8, 6, msgWidth, attrsWidth})
}

func levelStyleFor(level slog.Level) lipgloss.Style {
	switch {
	case level >= slog.LevelError:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#F87171")).Bold(true)
	case level >= slog.LevelWarn:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#FBBF24"))
	case level >= slog.LevelDebug && level < slog.LevelInfo:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	default:
		return lipgloss.NewStyle().Foreground(lipgloss.Color("#22D3EE"))
	}
}

type col struct {
	Label string
	Width int
}

var headerStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#CBD5E1"))

func headerCells(cols []col) string {
	parts := make([]string, len(cols))
	for i, c := range cols {
		parts[i] = padRight(truncate(c.Label, c.Width), c.Width)
	}
	return strings.Join(parts, " ")
}

func headerRow(cols []col) string {
	return headerStyle.Render(headerCells(cols))
}

func fitRow(s string, inner int) string {
	if len([]rune(s)) > inner && inner > 0 {
		return truncate(s, inner)
	}
	return s
}

func dataRow(cells []string, widths []int) string {
	out := make([]string, len(cells))
	for i, c := range cells {
		w := 0
		if i < len(widths) {
			w = widths[i]
		}
		if w > 0 && visibleLen(c) > w {
			c = truncate(c, w)
		}
		out[i] = padANSI(c, w)
	}
	return strings.Join(out, " ")
}

func padANSI(s string, w int) string {
	if w == 0 {
		return s
	}
	if n := visibleLen(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n <= 0 {
		return ""
	}
	return string(r[:n-1]) + "…"
}

func padRight(s string, w int) string {
	n := len([]rune(s))
	if w == 0 || n >= w {
		return s
	}
	return s + strings.Repeat(" ", w-n)
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

func (p *Program) RefreshError(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || err == nil {
		return
	}
	p.program.Send(pollErrorMsg{err: err.Error(), at: time.Now()})
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
