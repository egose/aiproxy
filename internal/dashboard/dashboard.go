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

type HealthcheckEntry struct {
	Provider   string
	Configured bool
	Checked    bool
	Healthy    bool
	StatusCode int
	Message    string
	Path       string
}

type RuntimeSnapshot struct {
	Version           string
	Address           string
	Providers         []config.Provider
	DisabledProviders []config.Provider
	Aliases           []config.Alias
	Cooldowns         []CooldownEntry
	Healthchecks      []HealthcheckEntry
	AuthMode          string
	StartTime         time.Time
	SnapshotAt        time.Time
	Usage             UsageViewer
	Health            HealthViewer
	Logs              LogsViewer
	PayloadEnabled    bool
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
	bottomTabPayload
	bottomTabBlocks
)

type model struct {
	snapshot             *RuntimeSnapshot
	pending              *RuntimeSnapshot
	hasPending           bool
	health               map[string]bool
	width                int
	height               int
	now                  time.Time
	quit                 bool
	dirty                bool
	rendered             string
	focus                focusArea
	bottomTab            bottomTab
	bottomHeight         int
	statsHeight          int
	lastRefresh          time.Time
	staleErr             string
	staleAt              time.Time
	paused               bool
	showHelp             bool
	zoomed               bool
	usageScroll          int
	providerScroll       int
	aliasCursor          int
	aliasOffset          int
	aliasDetailName      string
	aliasDetailScroll    int
	logCursor            int
	logOffset            int
	logCursorSeq         uint64
	logFollow            bool
	logOldestFirst       bool
	logDetailOpen        bool
	logDetailEntry       observability.LogEntry
	logDetailScroll      int
	logMinLevel          slog.Level
	logFilterOn          bool
	tenantIndex          int
	errorsOnly           bool
	usageUpstream        bool
	ipCache              map[string]string
	payloadFetcher       PayloadFetcher
	payloads             []PayloadSummary
	payloadKnown         bool
	payloadEnabled       bool
	payloadCursor        int
	payloadOffset        int
	payloadErrorsOnly    bool
	payloadOldestFirst   bool
	payloadLoading       bool
	payloadErr           string
	payloadDetail        string
	payloadDetailErr     string
	payloadDetailID      string
	payloadPendingID     string
	payloadDetailScroll  int
	blockFetcher         BlockFetcher
	blocks               []BlockSummary
	blockKnown           bool
	blockEnabled         bool
	blockCursor          int
	blockOffset          int
	blockLoading         bool
	blockErr             string
	blockDetail          BlockCapture
	blockDetailErr       string
	blockDetailID        string
	blockPendingID       string
	blockDetailScroll    int
	blockDecisionPending string
	blockDecisionMsg     string
	blockDecisionErr     string
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
		logFollow:    true,
	}
	if s != nil && s.Health != nil {
		m.health = s.Health.Snapshot()
	}
	if s != nil && !s.SnapshotAt.IsZero() {
		m.lastRefresh = s.SnapshotAt
	} else {
		m.lastRefresh = m.now
	}
	if s != nil {
		m.payloadEnabled = s.PayloadEnabled
	}
	return m
}

func InitialModelWithPayloadFetcher(s *RuntimeSnapshot, f PayloadFetcher) tea.Model {
	m := InitialModel(s).(*model)
	m.payloadFetcher = f
	return m
}

func InitialModelWithBlockFetcher(s *RuntimeSnapshot, f PayloadFetcher, b BlockFetcher) tea.Model {
	m := InitialModelWithPayloadFetcher(s, f).(*model)
	m.blockFetcher = b
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
		if m.aliasDetailOpen() {
			if msg.String() == "esc" || msg.String() == "enter" {
				m.aliasDetailName = ""
				m.aliasDetailScroll = 0
				m.dirty = true
				return m, nil
			}
			if shouldQuit(msg) {
				m.quit = true
				return m, tea.Quit
			}
			if handled := m.handleAliasDetailKey(msg); handled {
				m.dirty = true
				return m, nil
			}
			return m, nil
		}
		if m.payloadDetailOpen() || m.payloadPendingID != "" {
			if msg.String() == "esc" || msg.String() == "enter" {
				m.payloadDetailID = ""
				m.payloadPendingID = ""
				m.payloadDetail = ""
				m.payloadDetailErr = ""
				m.payloadDetailScroll = 0
				m.dirty = true
				return m, nil
			}
			if shouldQuit(msg) {
				m.quit = true
				return m, tea.Quit
			}
			if handled, cmd := m.handlePayloadKey(msg); handled {
				m.dirty = true
				return m, cmd
			}
			return m, nil
		}
		if m.blockDetailOpen() || m.blockPendingID != "" {
			if msg.String() == "esc" || msg.String() == "enter" {
				m.blockDetailID = ""
				m.blockPendingID = ""
				m.blockDetail = BlockCapture{}
				m.blockDetailErr = ""
				m.blockDetailScroll = 0
				m.blockDecisionPending = ""
				m.blockDecisionMsg = ""
				m.blockDecisionErr = ""
				m.dirty = true
				return m, nil
			}
			if shouldQuit(msg) {
				m.quit = true
				return m, tea.Quit
			}
			if handled, cmd := m.handleBlockKey(msg); handled {
				m.dirty = true
				return m, cmd
			}
			return m, nil
		}
		if m.zoomed && msg.String() == "esc" {
			if m.aliasDetailOpen() {
				m.aliasDetailName = ""
				m.aliasDetailScroll = 0
				m.dirty = true
				return m, nil
			}
			if m.logDetailOpen {
				m.logDetailOpen = false
				m.logDetailScroll = 0
				m.dirty = true
				return m, nil
			}
			m.zoomed = false
			m.dirty = true
			return m, nil
		}
		if m.logDetailOpen {
			if msg.String() == "esc" || msg.String() == "enter" {
				m.logDetailOpen = false
				m.logDetailScroll = 0
				m.dirty = true
				return m, nil
			}
			if shouldQuit(msg) {
				m.quit = true
				return m, tea.Quit
			}
			if handled := m.handleLogDetailKey(msg); handled {
				m.dirty = true
				return m, nil
			}
			return m, nil
		}
		if shouldQuit(msg) {
			m.quit = true
			return m, tea.Quit
		}
		if m.focus == focusBottom && m.bottomTab == bottomTabAliases {
			if handled := m.handleAliasKey(msg); handled {
				m.dirty = true
				return m, nil
			}
		}
		if m.focus == focusBottom && m.bottomTab == bottomTabPayload {
			if handled, cmd := m.handlePayloadKey(msg); handled {
				m.dirty = true
				return m, cmd
			}
		}
		if m.focus == focusBottom && m.bottomTab == bottomTabBlocks {
			if handled, cmd := m.handleBlockKey(msg); handled {
				m.dirty = true
				return m, cmd
			}
		}
		if m.focus == focusBottom && m.bottomTab == bottomTabLogs {
			if handled := m.handleLogKey(msg); handled {
				m.dirty = true
				return m, nil
			}
		}
		if handled := m.handleKey(msg); handled {
			m.dirty = true
			if m.bottomTab == bottomTabPayload && !m.payloadKnown {
				if cmd := m.requestPayloads(); cmd != nil {
					return m, cmd
				}
			}
			if m.bottomTab == bottomTabBlocks && !m.blockKnown {
				if cmd := m.requestBlocks(); cmd != nil {
					return m, cmd
				}
			}
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
	case payloadListMsg:
		m.applyPayloadList(msg)
		return m, nil
	case payloadDetailMsg:
		m.applyPayloadDetail(msg)
		return m, nil
	case blockListMsg:
		m.applyBlockList(msg)
		return m, nil
	case blockDetailMsg:
		m.applyBlockDetail(msg)
		return m, nil
	case blockDecisionMsg:
		m.applyBlockDecision(msg)
		return m, nil
	case tickMsg:
		m.now = time.Now()
		if m.snapshot != nil && m.snapshot.Health != nil {
			m.health = m.snapshot.Health.Snapshot()
		}
		m.clampScroll()
		m.dirty = true
		if m.bottomTab == bottomTabPayload && m.payloadKnown && !m.payloadDetailOpen() &&
			m.payloadPendingID == "" && m.payloadFetcher != nil && !m.payloadLoading && m.payloadErr == "" {
			m.payloadLoading = true
			return m, tea.Batch(tickCmd(), fetchPayloadsCmd(m.payloadFetcher, payloadFetchLimit, m.payloadErrorsOnly))
		}
		if m.bottomTab == bottomTabBlocks && m.blockKnown && !m.blockDetailOpen() &&
			m.blockPendingID == "" && m.blockFetcher != nil && !m.blockLoading && m.blockErr == "" {
			m.blockLoading = true
			return m, tea.Batch(tickCmd(), fetchBlocksCmd(m.blockFetcher))
		}
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
	m.payloadEnabled = s.PayloadEnabled
	if !s.SnapshotAt.IsZero() {
		m.lastRefresh = s.SnapshotAt
	} else {
		m.lastRefresh = m.now
	}
	m.staleErr = ""
	m.usageScroll = 0
	m.providerScroll = 0
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
	case "shift+tab":
		m.focus = (m.focus + 2) % 3
		return true
	case "enter":
		if m.focus == focusBottom {
			switch m.bottomTab {
			case bottomTabAliases:
				if m.handleAliasKey(msg) {
					return true
				}
			case bottomTabPayload:
				if handled, cmd := m.handlePayloadKey(msg); handled {
					_ = cmd
					return true
				}
			default:
				if m.handleLogKey(msg) {
					return true
				}
			}
			return false
		}
		m.zoomed = !m.zoomed
		m.clampScroll()
		return true
	case "z":
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
	case "3":
		m.bottomTab = bottomTabPayload
		m.focus = focusBottom
		return true
	case "4":
		m.bottomTab = bottomTabBlocks
		m.focus = focusBottom
		return true
	case "[", "]":
		switch m.bottomTab {
		case bottomTabLogs:
			m.bottomTab = bottomTabAliases
		case bottomTabAliases:
			m.bottomTab = bottomTabPayload
		case bottomTabPayload:
			m.bottomTab = bottomTabBlocks
		default:
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
		m.logCursor = 0
		m.logOffset = 0
		m.logCursorSeq = 0
		m.logFollow = true
		m.logDetailOpen = false
		m.logDetailScroll = 0
		m.bottomTab = bottomTabLogs
		m.clampLogCursor()
		return true
	case "o":
		switch m.bottomTab {
		case bottomTabLogs:
			m.toggleLogOrder()
			return true
		case bottomTabPayload:
			m.togglePayloadOrder()
			return true
		}
		return false
	case "j", "down":
		return m.scrollFocused(1)
	case "k", "up":
		return m.scrollFocused(-1)
	case "pgdown", "shift+pgdown":
		return m.scrollFocusedPage(1)
	case "pgup", "shift+pgup":
		return m.scrollFocusedPage(-1)
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
			if m.aliasDetailOpen() {
				return m.scrollAliasDetail(delta)
			}
			return m.moveAliasCursor(delta)
		}
		if m.bottomTab == bottomTabPayload {
			return m.movePayloadCursor(delta)
		}
		if m.bottomTab == bottomTabBlocks {
			return m.moveBlockCursor(delta)
		}
		if m.logDetailOpen {
			return m.scrollLogDetail(delta)
		}
		return m.moveLogCursor(delta)
	}
}

func (m *model) scrollFocusedPage(sign int) bool {
	if sign >= 0 {
		sign = 1
	} else {
		sign = -1
	}
	switch m.focus {
	case focusProviders:
		step := m.providerVisibleRows()
		if step < 1 {
			step = 1
		}
		return m.scrollProviders(sign * step)
	case focusUsage:
		step := m.usageVisibleRows()
		if step < 1 {
			step = 1
		}
		return m.scrollUsage(sign * step)
	default:
		step := m.bottomVisibleRows()
		if step < 1 {
			step = 1
		}
		detailStep := m.detailVisibleRows()
		if m.bottomTab == bottomTabAliases {
			if m.aliasDetailOpen() {
				return m.scrollAliasDetail(sign * detailStep)
			}
			return m.moveAliasCursor(sign * step)
		}
		if m.bottomTab == bottomTabPayload {
			if m.payloadDetailOpen() {
				return m.scrollPayloadDetail(sign * detailStep)
			}
			return m.movePayloadCursor(sign * step)
		}
		if m.bottomTab == bottomTabBlocks {
			if m.blockDetailOpen() {
				return m.scrollBlockDetail(sign * detailStep)
			}
			return m.moveBlockCursor(sign * step)
		}
		if m.logDetailOpen {
			return m.scrollLogDetail(sign * detailStep)
		}
		return m.moveLogCursor(sign * step)
	}
}

func (m *model) detailVisibleRows() int {
	n := zoomBodyHeight(m.height) - 5
	if n < 1 {
		n = 1
	}
	return n
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
			if m.aliasDetailOpen() {
				if m.aliasDetailScroll == 0 {
					return false
				}
				m.aliasDetailScroll = 0
				return true
			}
			return m.aliasCursorTop()
		}
		if m.bottomTab == bottomTabPayload {
			return m.payloadCursorTop()
		}
		if m.bottomTab == bottomTabBlocks {
			return m.blockCursorTop()
		}
		if m.logDetailOpen {
			if m.logDetailScroll == 0 {
				return false
			}
			m.logDetailScroll = 0
			return true
		}
		return m.logCursorTop()
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
			if m.aliasDetailOpen() {
				m.aliasDetailScroll = 1 << 30
				return true
			}
			return m.aliasCursorBottom()
		}
		if m.bottomTab == bottomTabPayload {
			return m.payloadCursorBottom()
		}
		if m.bottomTab == bottomTabBlocks {
			return m.blockCursorBottom()
		}
		if m.logDetailOpen {
			m.logDetailScroll = 1 << 30
			return true
		}
		return m.logCursorBottom()
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

func (m *model) aliasList() []config.Alias {
	if m.snapshot == nil {
		return nil
	}
	return m.snapshot.Aliases
}

func (m *model) clampAliasCursor() {
	n := len(m.aliasList())
	if n == 0 {
		m.aliasCursor = 0
		m.aliasOffset = 0
		return
	}
	if m.aliasCursor < 0 {
		m.aliasCursor = 0
	}
	if m.aliasCursor >= n {
		m.aliasCursor = n - 1
	}
	visible := m.bottomVisibleRows()
	if visible < 1 {
		visible = 1
	}
	if m.aliasOffset > m.aliasCursor {
		m.aliasOffset = m.aliasCursor
	}
	if m.aliasOffset < m.aliasCursor-visible+1 {
		m.aliasOffset = m.aliasCursor - visible + 1
	}
	if m.aliasOffset < 0 {
		m.aliasOffset = 0
	}
	maxOff := n - visible
	if maxOff < 0 {
		maxOff = 0
	}
	if m.aliasOffset > maxOff {
		m.aliasOffset = maxOff
	}
}

func (m *model) moveAliasCursor(delta int) bool {
	if len(m.aliasList()) == 0 {
		return false
	}
	m.clampAliasCursor()
	next := m.aliasCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(m.aliasList()) {
		next = len(m.aliasList()) - 1
	}
	if next == m.aliasCursor {
		return false
	}
	m.aliasCursor = next
	m.clampAliasCursor()
	return true
}

func (m *model) aliasCursorTop() bool {
	if m.aliasCursor == 0 && m.aliasOffset == 0 {
		return false
	}
	m.aliasCursor = 0
	m.aliasOffset = 0
	return true
}

func (m *model) aliasCursorBottom() bool {
	n := len(m.aliasList())
	if n == 0 || m.aliasCursor == n-1 {
		return false
	}
	m.aliasCursor = n - 1
	m.clampAliasCursor()
	return true
}

func (m *model) scrollAliasDetail(delta int) bool {
	if delta > 0 {
		m.aliasDetailScroll += delta
		return true
	}
	if delta < 0 {
		if m.aliasDetailScroll <= 0 {
			return false
		}
		m.aliasDetailScroll += delta
		if m.aliasDetailScroll < 0 {
			m.aliasDetailScroll = 0
		}
		return true
	}
	return false
}

func (m *model) scrollPayloadDetail(delta int) bool {
	if delta > 0 {
		m.payloadDetailScroll += delta
		return true
	}
	if delta < 0 {
		if m.payloadDetailScroll <= 0 {
			return false
		}
		m.payloadDetailScroll += delta
		if m.payloadDetailScroll < 0 {
			m.payloadDetailScroll = 0
		}
		return true
	}
	return false
}

func (m *model) scrollBlockDetail(delta int) bool {
	if delta > 0 {
		m.blockDetailScroll += delta
		return true
	}
	if delta < 0 {
		if m.blockDetailScroll <= 0 {
			return false
		}
		m.blockDetailScroll += delta
		if m.blockDetailScroll < 0 {
			m.blockDetailScroll = 0
		}
		return true
	}
	return false
}

func (m *model) handleAliasKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "j", "down":
		return m.moveAliasCursor(1)
	case "k", "up":
		return m.moveAliasCursor(-1)
	case "pgdown", "shift+pgdown":
		return m.moveAliasCursor(m.bottomVisibleRows())
	case "pgup", "shift+pgup":
		return m.moveAliasCursor(-m.bottomVisibleRows())
	case "g", "home":
		return m.aliasCursorTop()
	case "G", "end":
		return m.aliasCursorBottom()
	case "enter":
		aliases := m.aliasList()
		if len(aliases) == 0 {
			return false
		}
		m.clampAliasCursor()
		if m.aliasCursor < 0 || m.aliasCursor >= len(aliases) {
			return false
		}
		m.aliasDetailName = aliases[m.aliasCursor].Name
		m.aliasDetailScroll = 0
		return true
	}
	return false
}

func (m *model) handleAliasDetailKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "j", "down":
		m.aliasDetailScroll++
		return true
	case "k", "up":
		if m.aliasDetailScroll > 0 {
			m.aliasDetailScroll--
			return true
		}
		return false
	case "pgdown", "shift+pgdown":
		return m.scrollAliasDetail(m.detailVisibleRows())
	case "pgup", "shift+pgup":
		return m.scrollAliasDetail(-m.detailVisibleRows())
	case "g", "home":
		if m.aliasDetailScroll == 0 {
			return false
		}
		m.aliasDetailScroll = 0
		return true
	case "G", "end":
		m.aliasDetailScroll = 1 << 30
		return true
	}
	return false
}

func (m *model) logMaxOffset(n int) int {
	visible := m.bottomVisibleRows()
	if visible < 1 {
		visible = 1
	}
	maxOff := n - visible
	if maxOff < 0 {
		maxOff = 0
	}
	return maxOff
}

func (m *model) aliasDetailOpen() bool {
	return m.aliasDetailName != ""
}

func (m *model) clampLogCursor() {
	m.clampLogCursorTo(m.filteredLogs(1 << 30))
}

func (m *model) clampLogCursorTo(entries []observability.LogEntry) {
	n := len(entries)
	if n == 0 {
		m.logCursor = 0
		m.logOffset = 0
		m.logCursorSeq = 0
		m.logFollow = true
		return
	}
	maxOff := m.logMaxOffset(n)
	if m.logFollow {
		newest := m.logNewestIdx(n)
		m.logCursor = newest
		if newest == 0 {
			m.logOffset = 0
		} else {
			m.logOffset = maxOff
		}
		if n > 0 {
			m.logCursorSeq = entries[newest].Seq
		} else {
			m.logCursorSeq = 0
		}
		return
	}
	if m.logCursorSeq != 0 {
		if idx, ok := findLogSeq(entries, m.logCursorSeq); ok {
			m.logCursor = idx
		}
	}
	if m.logCursor < 0 {
		m.logCursor = 0
	}
	if m.logCursor >= n {
		m.logCursor = n - 1
	}
	m.logCursorSeq = entries[m.logCursor].Seq
	visible := m.bottomVisibleRows()
	if visible < 1 {
		visible = 1
	}
	if m.logOffset > m.logCursor {
		m.logOffset = m.logCursor
	}
	if m.logOffset < m.logCursor-visible+1 {
		m.logOffset = m.logCursor - visible + 1
	}
	if m.logOffset < 0 {
		m.logOffset = 0
	}
	if m.logOffset > maxOff {
		m.logOffset = maxOff
	}
}

func findLogSeq(entries []observability.LogEntry, seq uint64) (int, bool) {
	for i, e := range entries {
		if e.Seq == seq {
			return i, true
		}
	}
	return 0, false
}

func (m *model) moveLogCursor(delta int) bool {
	entries := m.filteredLogs(1 << 30)
	if len(entries) == 0 {
		return false
	}
	m.clampLogCursorTo(entries)
	next := m.logCursor + delta
	if next < 0 {
		next = 0
	}
	if next >= len(entries) {
		next = len(entries) - 1
	}
	m.logFollow = next == m.logNewestIdx(len(entries))
	if next == m.logCursor {
		return false
	}
	m.logCursor = next
	m.logCursorSeq = entries[next].Seq
	m.clampLogCursorTo(entries)
	return true
}

func (m *model) logCursorTop() bool {
	entries := m.filteredLogs(1 << 30)
	if len(entries) == 0 {
		return false
	}
	if m.logCursor == 0 && m.logOffset == 0 {
		return false
	}
	m.logCursor = 0
	m.logOffset = 0
	m.logCursorSeq = entries[0].Seq
	m.logFollow = !m.logOldestFirst
	return true
}

func (m *model) logCursorBottom() bool {
	entries := m.filteredLogs(1 << 30)
	if len(entries) == 0 {
		return false
	}
	maxOff := m.logMaxOffset(len(entries))
	if m.logCursor == len(entries)-1 && m.logOffset == maxOff {
		return false
	}
	m.logCursor = len(entries) - 1
	m.logCursorSeq = entries[m.logCursor].Seq
	m.logOffset = maxOff
	m.logFollow = m.logOldestFirst
	return true
}

func (m *model) scrollLogDetail(delta int) bool {
	if delta > 0 {
		m.logDetailScroll += delta
		return true
	}
	if delta < 0 {
		if m.logDetailScroll <= 0 {
			return false
		}
		m.logDetailScroll += delta
		if m.logDetailScroll < 0 {
			m.logDetailScroll = 0
		}
		return true
	}
	return false
}

func (m *model) handleLogKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "j", "down":
		return m.moveLogCursor(1)
	case "k", "up":
		return m.moveLogCursor(-1)
	case "pgdown", "shift+pgdown":
		return m.moveLogCursor(m.bottomVisibleRows())
	case "pgup", "shift+pgup":
		return m.moveLogCursor(-m.bottomVisibleRows())
	case "g", "home":
		return m.logCursorTop()
	case "G", "end":
		return m.logCursorBottom()
	case "o":
		m.toggleLogOrder()
		return true
	case "enter":
		entries := m.filteredLogs(1 << 30)
		if len(entries) == 0 {
			return false
		}
		m.clampLogCursorTo(entries)
		if m.logCursor < 0 || m.logCursor >= len(entries) {
			return false
		}
		m.logDetailEntry = entries[m.logCursor]
		m.logCursorSeq = entries[m.logCursor].Seq
		m.logDetailOpen = true
		m.logDetailScroll = 0
		return true
	}
	return false
}

func (m *model) handleLogDetailKey(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "j", "down":
		m.logDetailScroll++
		return true
	case "k", "up":
		if m.logDetailScroll > 0 {
			m.logDetailScroll--
			return true
		}
		return false
	case "pgdown", "shift+pgdown":
		return m.scrollLogDetail(m.detailVisibleRows())
	case "pgup", "shift+pgup":
		return m.scrollLogDetail(-m.detailVisibleRows())
	case "g", "home":
		if m.logDetailScroll == 0 {
			return false
		}
		m.logDetailScroll = 0
		return true
	case "G", "end":
		m.logDetailScroll = 1 << 30
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
	m.clampAliasCursor()
	m.clampPayloadCursor()
	m.clampBlockCursor()
	m.clampLogCursor()
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
	if m.aliasDetailOpen() {
		bodyHeight := zoomBodyHeight(m.height)
		body := renderAliasDetail(m, m.width, bodyHeight)
		return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, body, renderFooter(m)), m.width)
	}
	if m.payloadDetailOpen() || m.payloadPendingID != "" {
		bodyHeight := zoomBodyHeight(m.height)
		body := renderPayloadDetail(m, m.width, bodyHeight)
		return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, body, renderFooter(m)), m.width)
	}
	if m.blockDetailOpen() || m.blockPendingID != "" {
		bodyHeight := zoomBodyHeight(m.height)
		body := renderBlockDetail(m, m.width, bodyHeight)
		return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, body, renderFooter(m)), m.width)
	}
	if m.logDetailOpen {
		bodyHeight := zoomBodyHeight(m.height)
		body := renderLogDetail(m, m.width, bodyHeight)
		return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, body, renderFooter(m)), m.width)
	}
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
	switch m.bottomTab {
	case bottomTabAliases:
		pane = renderAliases(m, width, paneHeight)
	case bottomTabPayload:
		pane = renderPayloads(m, width, paneHeight)
	case bottomTabBlocks:
		pane = renderBlocks(m, width, paneHeight)
	default:
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
	logsLabel := fmt.Sprintf("2:Logs(%s,%s)", level, m.logOrderLabel())
	payloadLabel := fmt.Sprintf("3:Payloads(%s)", m.payloadOrderLabel())
	if m.payloadKnown {
		filter := ""
		if m.payloadErrorsOnly {
			filter = " errs"
		}
		payloadLabel = fmt.Sprintf("3:Payloads(%d%s,%s)", len(m.payloads), filter, m.payloadOrderLabel())
	} else if !m.payloadEnabled {
		payloadLabel = "3:Payloads(off)"
	}
	alias := dimStyle.Render("  " + aliasLabel)
	logs := dimStyle.Render("  " + logsLabel)
	payload := dimStyle.Render("  " + payloadLabel)
	blocksLabel := "4:Blocks"
	if m.blockKnown {
		blocksLabel = fmt.Sprintf("4:Blocks(%d)", len(m.blocks))
	}
	blocks := dimStyle.Render("  " + blocksLabel)
	switch m.bottomTab {
	case bottomTabAliases:
		alias = activeStyle.Render("▸ " + aliasLabel)
	case bottomTabLogs:
		logs = activeStyle.Render("▸ " + logsLabel)
	case bottomTabBlocks:
		blocks = activeStyle.Render("▸ " + blocksLabel)
	default:
		payload = activeStyle.Render("▸ " + payloadLabel)
	}
	return alias + "  " + logs + "  " + payload + "  " + blocks
}

func (m *model) renderHelp() string {
	lines := []string{
		"aiproxy dashboard — keys",
		"",
		"  tab/shift+tab cycle focus PROVIDERS / USAGE / bottom tabs",
		"  1/2/3/4 or [/] switch bottom tab (Aliases / Logs / Payloads / Blocks)",
		"  enter      open selected row detail (all bottom tabs)",
		"  z          zoom focused pane to full screen",
		"  esc        unzoom / close detail (or quit when not zoomed)",
		"  j/k dn/up  move selection / scroll   g/G,home/end top/bottom",
		"  pgup/pgdn  page selection / scroll (bottom panes + detail)",
		"  +/- J/K    resize bottom pane",
		"  t          cycle tenant filter    e toggle errors-only",
		"  s          toggle payload errs filter (payloads tab)",
		"  r          refresh payload list (payloads tab)",
		"  o          toggle newest/oldest order (logs/payloads tab)",
		"  u          toggle usage view (public vs upstream model)",
		"  l          cycle log level filter (all/debug/info/warn/error)",
		"  p          pause live updates (buffer one snapshot)",
		"  ?/h        toggle this help       q/Esc/Ctrl+C quit",
		"",
		"Legend: ✓ healthy · ✗ unhealthy · ? unknown (no health report yet)",
		"  HC = upstream healthcheck: ✓ passing · ✗ failing · ? pending · - none.",
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
		switch m.bottomTab {
		case bottomTabLogs:
			focusName = "LOGS"
		case bottomTabPayload:
			focusName = "PAYLOADS"
		case bottomTabBlocks:
			focusName = "BLOCKS"
		}
	}
	state := "LIVE"
	if m.paused {
		state = "PAUSED"
	} else if m.staleErr != "" {
		state = "STALE"
	}
	zoomHint := "[enter] detail [z] zoom"
	if m.zoomed {
		zoomHint = "[esc] unzoom"
	}
	base := fmt.Sprintf("%s focus:%s [tab/shift+tab] pane [1/2/3/4] tabs [j/k/pgup/pgdn] scroll [t]enant [e]rrs [s]tatus [r]efresh [o]rder [u]pstream [l]evel [p]ause %s [?]help [q]uit", state, focusName, zoomHint)
	if len([]rune(base)) > m.width && m.width > 20 {
		base = truncate(base, m.width)
	}
	legend := "ERR% excl 429 · ~=stream/no-tokens · ?=unknown health · HC=healthcheck · n/a=sparse latency · TOKENS=in/out (+cached w=write r=read)"
	if len([]rune(legend)) > m.width && m.width > 20 {
		legend = truncate(legend, m.width)
	}
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
			Tenant:              u.Tenant,
			Client:              u.Client,
			Model:               u.Provider + "/" + u.Model,
			Operation:           u.Operation,
			StatusCode:          u.StatusCode,
			Count:               u.Count,
			PromptTokens:        u.PromptTokens,
			CompletionTokens:    u.CompletionTokens,
			TotalTokens:         u.TotalTokens,
			CachedTokens:        u.CachedTokens,
			CacheCreationTokens: u.CacheCreationTokens,
			CacheReadTokens:     u.CacheReadTokens,
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
	var reqs, t429s []int64
	var toks []string
	for _, p := range snap.Providers {
		ps := byName[p.Name]
		names = append(names, p.Name)
		ips = append(ips, m.providerIP(p.BaseURL))
		reqs = append(reqs, ps.Requests)
		t429s = append(t429s, ps.Throttled)
		toks = append(toks, providerTokensText(ps))
	}
	for _, p := range snap.DisabledProviders {
		names = append(names, p.Name)
		ips = append(ips, m.providerIP(p.BaseURL))
		reqs = append(reqs, 0)
		t429s = append(t429s, 0)
		toks = append(toks, providerTokensText(accounting.ProviderSummary{}))
	}
	nameW, ipW, reqW, t429W, tokW := providerColWidths(names, ips, reqs, t429s, toks, inner)
	rows := []string{headerStyle.Render(fitRow(headerCells([]col{{"PROVIDER", nameW}, {"", 1}, {"HC", 2}, {"REQS", reqW}, {"ERR%", 6}, {"429", t429W}, {"P95", 8}, {"TOKENS", tokW}, {"IP", ipW}}), inner))}
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
	hcByName := healthcheckMarks(m.snapshot.Healthchecks)
	for _, p := range snap.Providers {
		known, healthy := healthKnown(m.health, p.Name)
		ps := byName[p.Name]
		data = append(data, providerLine{text: providerRow(p.Name, known, healthy, hcByName[p.Name], ps, ps.Throttled, latency[p.Name], samples[p.Name], false, m.providerIP(p.BaseURL), nameW, ipW, reqW, t429W, tokW)})
	}
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))
	for _, p := range snap.DisabledProviders {
		data = append(data, providerLine{
			text: providerRow(p.Name, true, false, "-", accounting.ProviderSummary{}, 0, 0, 0, true, m.providerIP(p.BaseURL), nameW, ipW, reqW, t429W, tokW),
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

func healthcheckMarks(entries []HealthcheckEntry) map[string]string {
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if !e.Configured {
			out[e.Provider] = "-"
			continue
		}
		if !e.Checked {
			out[e.Provider] = "?"
			continue
		}
		if e.Healthy {
			out[e.Provider] = "✓"
			continue
		}
		out[e.Provider] = "✗"
	}
	return out
}

func healthcheckDetail(entries []HealthcheckEntry, provider string) string {
	for _, e := range entries {
		if e.Provider != provider {
			continue
		}
		if !e.Configured {
			return ""
		}
		if !e.Checked {
			return "hc pending " + e.Path
		}
		if e.Healthy {
			return fmt.Sprintf("hc ✓ %s %d", e.Path, e.StatusCode)
		}
		msg := e.Message
		if msg == "" {
			msg = fmt.Sprintf("status %d", e.StatusCode)
		}
		return fmt.Sprintf("hc ✗ %s %s", e.Path, truncate(msg, 40))
	}
	return ""
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

func providerColWidths(names, ips []string, requests, throttled []int64, tokens []string, inner int) (nameW, ipW, reqW, t429W, tokW int) {
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
		tokW = max(tokW, runeLen(t))
	}
	nameW = min(nameW, 24)
	ipW = min(ipW, 21)
	reqW = min(reqW, 10)
	t429W = min(t429W, 8)
	tokW = min(tokW, 20)
	const fixed = 1 + 2 + 6 + 8
	const gaps = 8
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
	tokW = min(tokW, 20)
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
	return formatTokenSplit(s.PromptTokens, s.CompletionTokens, s.TotalTokens, s.CachedTokens, s.CacheCreationTokens, s.CacheReadTokens)
}

func providerTokensText(ps accounting.ProviderSummary) string {
	return formatTokenSplit(ps.PromptTokens, ps.CompletionTokens, ps.TotalTokens, ps.CachedTokens, ps.CacheCreationTokens, ps.CacheReadTokens)
}

func formatTokenSplit(prompt, completion, total, cached, cacheWrite, cacheRead int64) string {
	if prompt == 0 && completion == 0 && total == 0 && cached == 0 {
		return "~"
	}
	if prompt == 0 && completion == 0 && cached == 0 {
		return comma(total)
	}
	split := comma(prompt) + "/" + comma(completion)
	if cacheWrite > 0 || cacheRead > 0 {
		split += " (" + comma(cacheWrite) + "w+" + comma(cacheRead) + "r)"
	} else if cached > 0 {
		split += " (" + comma(cached) + "c)"
	}
	if total > 0 && total != prompt+completion {
		return comma(total) + " " + split
	}
	return split
}

func providerRow(name string, known, healthy bool, hcMark string, ps accounting.ProviderSummary, throttled int64, p95 time.Duration, samples int, disabled bool, ip string, nameW, ipW, reqW, t429W, tokW int) string {
	status := "✓"
	if disabled {
		status = "✗"
	} else if !known {
		status = "?"
	} else if !healthy {
		status = "✗"
	}
	if hcMark == "" {
		hcMark = "-"
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
		hcMark,
		fmt.Sprintf("%*s", reqW, comma(ps.Requests)),
		fmt.Sprintf("%5.1f%%", errPct),
		fmt.Sprintf("%*s", t429W, comma(throttled)),
		p95cell,
		fmt.Sprintf("%*s", tokW, truncate(providerTokensText(ps), tokW)),
		truncate(ip, ipW),
	}, []int{nameW, 1, 2, reqW, 6, t429W, 8, tokW, ipW})
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
	m.clampAliasCursor()
	aliases := m.aliasList()
	rows := []string{fmt.Sprintf("ALIASES (%d) cool:%d", len(aliases), len(m.snapshot.Cooldowns))}
	if len(aliases) == 0 {
		rows = append(rows, "no aliases configured")
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if height <= 3 {
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	visible := m.bottomVisibleRows()
	if visible < 1 {
		visible = 1
	}
	cursor := m.aliasCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(aliases) {
		cursor = len(aliases) - 1
	}
	offset := m.aliasOffset
	if offset < 0 {
		offset = 0
	}
	maxOff := len(aliases) - visible
	if maxOff < 0 {
		maxOff = 0
	}
	if offset > maxOff {
		offset = maxOff
	}
	if offset > cursor {
		offset = cursor
	}
	if offset < cursor-visible+1 {
		offset = cursor - visible + 1
	}
	if offset < 0 {
		offset = 0
	}
	start := offset
	end := start + visible
	if end > len(aliases) {
		end = len(aliases)
	}
	cooldown := map[string]time.Duration{}
	for _, c := range m.snapshot.Cooldowns {
		key := c.Alias + "\x00" + c.Provider + "\x00" + c.Model
		d := time.Duration(c.RemainingMs) * time.Millisecond
		if d > cooldown[key] {
			cooldown[key] = d
		}
	}
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true)
	for i := start; i < end; i++ {
		a := aliases[i]
		retry := "default"
		if len(a.RetryStatusCodes) > 0 {
			parts := make([]string, len(a.RetryStatusCodes))
			for j, code := range a.RetryStatusCodes {
				parts[j] = fmt.Sprintf("%d", code)
			}
			retry = strings.Join(parts, ",")
		}
		algo := string(a.Algorithm)
		if algo == "" {
			algo = "-"
		}
		cool := 0
		for _, t := range a.Targets {
			key := a.Name + "\x00" + t.Provider + "\x00" + t.Model
			if d, ok := cooldown[key]; ok && d > 0 {
				cool++
			}
		}
		line := fmt.Sprintf("alias/%s [%s] retry:%s targets:%d cool:%d", a.Name, algo, retry, len(a.Targets), cool)
		if i == cursor {
			line = cursorStyle.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		rows = append(rows, truncate(line, width-2))
	}
	if end < len(aliases) {
		rows = append(rows, fmt.Sprintf("… %d more (j/k move)", len(aliases)-end))
	} else if len(aliases) > visible {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render("[j/k] move [enter] detail"))
	}
	return borderStyle.Render(strings.Join(rows, "\n"))
}

func renderAliasDetail(m *model, width, height int) string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#38BDF8")).
		Width(width - 2).
		Height(height - 2)
	inner := width - 4
	if inner < 10 {
		inner = 10
	}
	var a *config.Alias
	for i := range m.snapshot.Aliases {
		if m.snapshot.Aliases[i].Name == m.aliasDetailName {
			a = &m.snapshot.Aliases[i]
			break
		}
	}
	if a == nil {
		return borderStyle.Render("alias not found\n[esc] back")
	}
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
	affinity := "-"
	if a.SessionAffinity != nil {
		affinity = strings.Join(config.SessionAffinityHeaders(*a), ",")
		if affinity == "" {
			affinity = "default"
		}
	}
	cooldown := map[string]time.Duration{}
	for _, c := range m.snapshot.Cooldowns {
		key := c.Alias + "\x00" + c.Provider + "\x00" + c.Model
		d := time.Duration(c.RemainingMs) * time.Millisecond
		if d > cooldown[key] {
			cooldown[key] = d
		}
	}
	stats := map[string]accounting.ProviderSummary{}
	if m.snapshot.Usage != nil {
		for _, ps := range m.snapshot.Usage.ProviderSummaries() {
			stats[ps.Provider] = ps
		}
	}
	hcByName := healthcheckMarks(m.snapshot.Healthchecks)
	raw := []string{
		"alias: " + a.Name,
		"algorithm: " + algo,
		"retry: " + retry,
		"session_affinity: " + affinity,
	}
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
		ps := stats[t.Provider]
		hc := hcByName[t.Provider]
		if hc == "" {
			hc = "-"
		}
		line := fmt.Sprintf("%s %s/%s hc:%s reqs:%s errs:%s 429:%s tok:%s %s",
			mark, t.Provider, t.Model, hc, comma(ps.Requests), comma(ps.Errors),
			comma(ps.Throttled), comma(ps.TotalTokens), state)
		raw = append(raw, wrapText(line, inner)...)
		if detail := healthcheckDetail(m.snapshot.Healthchecks, t.Provider); detail != "" {
			raw = append(raw, wrapText("  "+detail, inner)...)
		}
	}
	lines := []string{"ALIAS " + a.Name}
	visible := height - 5
	if visible < 1 {
		visible = 1
	}
	start := m.aliasDetailScroll
	if start > len(raw)-1 {
		start = len(raw) - 1
	}
	if start < 0 {
		start = 0
	}
	m.aliasDetailScroll = start
	end := start + visible
	if end > len(raw) {
		end = len(raw)
	}
	for _, l := range raw[start:end] {
		lines = append(lines, truncate(l, inner))
	}
	if end < len(raw) {
		lines = append(lines, fmt.Sprintf("… %d more lines (j/k scroll, esc back)", len(raw)-end))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render("[j/k] scroll [esc] back"))
	}
	return borderStyle.Render(strings.Join(lines, "\n"))
}

func (m *model) logNewestIdx(n int) int {
	if m.logOldestFirst {
		return n - 1
	}
	return 0
}

func (m *model) logOrderLabel() string {
	if m.logOldestFirst {
		return "old-first"
	}
	return "new-first"
}

func (m *model) payloadOrderLabel() string {
	if m.payloadOldestFirst {
		return "old-first"
	}
	return "new-first"
}

func (m *model) toggleLogOrder() {
	m.logOldestFirst = !m.logOldestFirst
	m.logFollow = true
	m.logCursorSeq = 0
	m.clampLogCursor()
}

func (m *model) togglePayloadOrder() {
	m.payloadOldestFirst = !m.payloadOldestFirst
	m.payloadCursor = 0
	m.payloadOffset = 0
	m.clampPayloadCursor()
}

func reverseLogEntries(in []observability.LogEntry) {
	for i, j := 0, len(in)-1; i < j; i, j = i+1, j-1 {
		in[i], in[j] = in[j], in[i]
	}
}

func (m *model) filteredLogs(limit int) []observability.LogEntry {
	if m.snapshot == nil || m.snapshot.Logs == nil {
		return nil
	}
	all := m.snapshot.Logs.Since(1 << 30)
	var out []observability.LogEntry
	if !m.logFilterOn {
		if len(all) > limit {
			out = all[len(all)-limit:]
		} else {
			out = all
		}
	} else {
		out = make([]observability.LogEntry, 0, len(all))
		for _, e := range all {
			if e.Level >= m.logMinLevel {
				out = append(out, e)
			}
		}
		if len(out) > limit {
			out = out[len(out)-limit:]
		}
	}
	if !m.logOldestFirst {
		cp := make([]observability.LogEntry, len(out))
		copy(cp, out)
		reverseLogEntries(cp)
		return cp
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
	entries := m.filteredLogs(1 << 30)
	visible := m.logVisibleRows()
	if visible < 1 {
		visible = 1
	}
	cursor := m.logCursor
	if cursor < 0 {
		cursor = 0
	}
	if cursor >= len(entries) {
		cursor = len(entries) - 1
	}
	offset := m.logOffset
	if offset < 0 {
		offset = 0
	}
	maxOff := len(entries) - visible
	if maxOff < 0 {
		maxOff = 0
	}
	if offset > maxOff {
		offset = maxOff
	}
	if offset > cursor {
		offset = cursor
	}
	if offset < cursor-visible+1 {
		offset = cursor - visible + 1
	}
	if offset < 0 {
		offset = 0
	}
	start := offset
	end := start + visible
	if end > len(entries) {
		end = len(entries)
	}
	page := entries[start:end]
	attrsWidth := len("ATTRS")
	for _, e := range entries {
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
	order := "newest-first"
	if m.logOldestFirst {
		order = "oldest-first"
	}
	title := fmt.Sprintf("LOGS %s (%s) [o]rder", order, levelName)
	inner := width - 2
	rows := []string{title, headerStyle.Render(fitRow(headerCells([]col{{"AT", 8}, {"LEVEL", 6}, {"MESSAGE", msgWidth}, {"ATTRS", attrsWidth}}), inner))}
	if height <= 3 {
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if len(entries) == 0 {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B")).Render("no logs captured"))
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	cursorStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#38BDF8")).Bold(true)
	for i, e := range page {
		idx := start + i
		line := renderLogEntry(msgWidth, attrsWidth, e)
		if idx == cursor {
			line = cursorStyle.Render("▸ " + line)
		} else {
			line = "  " + line
		}
		rows = append(rows, line)
	}
	if end < len(entries) {
		rows = append(rows, fmt.Sprintf("… %d more (j/k move)", len(entries)-end))
	} else if len(entries) > visible {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render("[j/k] move [enter] detail"))
	}
	return borderStyle.Render(strings.Join(rows, "\n"))
}

func renderLogDetail(m *model, width, height int) string {
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#38BDF8")).
		Width(width - 2).
		Height(height - 2)
	inner := width - 4
	if inner < 10 {
		inner = 10
	}
	e := m.logDetailEntry
	title := "LOG " + e.Time.Format(time.RFC3339) + " " + e.Level.String()
	lines := []string{title}
	attrs := orDash(e.Attrs)
	raw := []string{"level: " + e.Level.String(), "time: " + e.Time.Format(time.RFC3339Nano)}
	raw = append(raw, wrapText("message: "+e.Message, inner)...)
	raw = append(raw, wrapText("attrs: "+attrs, inner)...)
	visible := height - 5
	if visible < 1 {
		visible = 1
	}
	start := m.logDetailScroll
	if start > len(raw)-1 {
		start = len(raw) - 1
	}
	if start < 0 {
		start = 0
	}
	m.logDetailScroll = start
	end := start + visible
	if end > len(raw) {
		end = len(raw)
	}
	for _, l := range raw[start:end] {
		lines = append(lines, truncate(l, inner))
	}
	if end < len(raw) {
		lines = append(lines, fmt.Sprintf("… %d more lines (j/k scroll, esc back)", len(raw)-end))
	} else {
		lines = append(lines, lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render("[j/k] scroll [esc] back"))
	}
	return borderStyle.Render(strings.Join(lines, "\n"))
}

func wrapText(s string, width int) []string {
	if width < 1 {
		return []string{s}
	}
	r := []rune(s)
	var out []string
	for len(r) > width {
		out = append(out, string(r[:width]))
		r = r[width:]
	}
	out = append(out, string(r))
	return out
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

func Run(ctx context.Context, snap *RuntimeSnapshot, fetcher PayloadFetcher) *Program {
	return RunWithBlockFetcher(ctx, snap, fetcher, nil)
}

func RunWithBlockFetcher(ctx context.Context, snap *RuntimeSnapshot, fetcher PayloadFetcher, blocks BlockFetcher) *Program {
	opts := []tea.ProgramOption{
		tea.WithContext(ctx),
		tea.WithoutCatchPanics(),
	}
	p := tea.NewProgram(InitialModelWithBlockFetcher(snap, fetcher, blocks), opts...)
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
