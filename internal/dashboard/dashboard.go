package dashboard

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
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
	Provider    string
	Configured  bool
	Checked     bool
	Healthy     bool
	StatusCode  int
	Message     string
	Path        string
	LastChecked time.Time
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
	ProviderMetadata  map[string]*dashrpc.ProviderDiagnostics
	ModelMetadata     map[string]*dashrpc.ModelDetails
	AliasAffinity     map[string]*dashrpc.Affinity
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
	statsMinHeight  = 12
	bottomMinHeight = 6
)

type bottomTab int

const (
	bottomTabLogs bottomTab = iota
	bottomTabAliases
	bottomTabPayload
	bottomTabBlocks
	bottomTabRequests
)

type model struct {
	search                       searchState
	queries                      [5]string
	requestCursor, requestOffset int
	requestErrorsOnly            bool
	requestDetail                *accounting.Event
	usageDetail                  *accounting.Summary
	metadataScroll               int
	correlation                  *correlationContext
	correlationNotice            string
	ctx                          context.Context
	retry                        func()
	connection                   ConnectionStatus
	snapshot                     *RuntimeSnapshot
	pending                      *RuntimeSnapshot
	hasPending                   bool
	health                       map[string]bool
	width                        int
	height                       int
	now                          time.Time
	quit                         bool
	dirty                        bool
	rendered                     string
	focus                        focusArea
	bottomTab                    bottomTab
	bottomHeight                 int
	statsHeight                  int
	lastRefresh                  time.Time
	staleErr                     string
	staleAt                      time.Time
	paused                       bool
	showHelp                     bool
	helpScroll                   int
	zoomed                       bool
	usageScroll                  int
	providerScroll               int
	providerCursor               int
	providerDetailName           string
	providerDetailScroll         int
	aliasCursor                  int
	aliasOffset                  int
	aliasDetailName              string
	aliasDetailScroll            int
	logCursor                    int
	logOffset                    int
	logCursorSeq                 uint64
	logFollow                    bool
	logOldestFirst               bool
	logDetailOpen                bool
	logDetailEntry               observability.LogEntry
	logDetailScroll              int
	logMinLevel                  slog.Level
	logFilterOn                  bool
	tenantIndex                  int
	errorsOnly                   bool
	usageUpstream                bool
	payloadFetcher               PayloadFetcher
	payloads                     []PayloadSummary
	payloadKnown                 bool
	payloadEnabled               bool
	payloadCursor                int
	payloadOffset                int
	payloadErrorsOnly            bool
	payloadOldestFirst           bool
	payloadLoading               bool
	payloadErr                   string
	payloadDetail                string
	payloadDetailErr             string
	payloadDetailID              string
	payloadPendingID             string
	payloadDetailScroll          int
	blockFetcher                 BlockFetcher
	blocks                       []BlockSummary
	blockKnown                   bool
	blockEnabled                 bool
	blockCursor                  int
	blockOffset                  int
	blockLoading                 bool
	blockErr                     string
	blockDetail                  BlockCapture
	blockDetailErr               string
	blockDetailID                string
	blockPendingID               string
	blockDetailScroll            int
	blockDecisionPending         string
	blockDecisionMsg             string
	blockDecisionErr             string
	blockFindingCursor           int
	blockDecisionSHA             string
	blockDecisionResults         map[string]blockDecisionMsg
	payloadListRequest           requestSlot
	payloadDetailRequest         requestSlot
	blockListRequest             requestSlot
	blockDetailRequest           requestSlot
	blockDecisionRequest         requestSlot
	pausedResults                pausedResults
	liveNow                      time.Time
	pauseSource                  *RuntimeSnapshot
}

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(refreshInterval, func(now time.Time) tea.Msg { return tickMsg(now) })
}

func InitialModel(s *RuntimeSnapshot) tea.Model {
	m := &model{
		snapshot:     s,
		health:       map[string]bool{},
		now:          time.Now(),
		dirty:        true,
		focus:        focusUsage,
		statsHeight:  16,
		bottomHeight: 10,
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
		return m, m.handleInput(msg)
	case snapshotMsg:
		if msg.snapshot != nil {
			if m.paused {
				m.pending = frozenSnapshot(msg.snapshot)
				m.pauseSource = nil
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
	case ConnectionStatus:
		m.connection = msg
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
		m.liveNow = time.Time(msg)
		if m.liveNow.IsZero() {
			m.liveNow = time.Now()
		}
		if m.paused {
			return m, tickCmd()
		}
		m.now = m.liveNow
		if m.snapshot != nil && m.snapshot.Health != nil {
			m.health = m.snapshot.Health.Snapshot()
		}
		m.clampScroll()
		m.dirty = true
		if m.bottomTab == bottomTabPayload && m.payloadKnown && !m.payloadDetailOpen() &&
			m.payloadPendingID == "" && m.payloadFetcher != nil && !m.payloadLoading && m.payloadErr == "" {
			return m, tea.Batch(tickCmd(), m.requestPayloads())
		}
		if m.bottomTab == bottomTabBlocks && m.blockKnown && !m.blockDetailOpen() &&
			m.blockPendingID == "" && m.blockFetcher != nil && !m.blockLoading && m.blockErr == "" {
			return m, tea.Batch(tickCmd(), m.requestBlocks())
		}
		return m, tickCmd()
	}
	return m, nil
}

func (m *model) applySnapshot(s *RuntimeSnapshot) {
	requests := m.recentRequests()
	providers, usage, aliases := m.providerList(), m.filteredSummaries(), m.aliasList()
	logs := m.filteredLogs(1 << 30)
	tenant := m.activeTenant()
	m.snapshot = s
	m.tenantIndex = 0
	for i, name := range m.tenantNames() {
		if name == tenant {
			m.tenantIndex = i + 1
			break
		}
	}
	m.pending = nil
	m.hasPending = false
	if s.Health != nil {
		m.health = s.Health.Snapshot()
	} else {
		m.health = map[string]bool{}
	}
	m.payloadEnabled = s.PayloadEnabled
	if !s.SnapshotAt.IsZero() {
		m.lastRefresh = s.SnapshotAt
	} else {
		m.lastRefresh = m.now
	}
	m.staleErr = ""
	m.requestCursor = anchoredIndex(requests, m.recentRequests(), m.requestCursor, requestIdentity)
	m.requestOffset = anchoredIndex(requests, m.recentRequests(), m.requestOffset, requestIdentity)
	m.usageScroll = anchoredIndex(usage, m.filteredSummaries(), m.usageScroll, usageIdentity)
	m.providerScroll = anchoredIndex(providers, m.providerList(), m.providerScroll, providerIdentity)
	m.providerCursor = anchoredIndex(providers, m.providerList(), m.providerCursor, providerIdentity)
	m.aliasCursor = anchoredIndex(aliases, m.aliasList(), m.aliasCursor, aliasIdentity)
	m.aliasOffset = anchoredIndex(aliases, m.aliasList(), m.aliasOffset, aliasIdentity)
	if !m.logFollow {
		m.logOffset = anchoredIndex(logs, m.filteredLogs(1<<30), m.logOffset, func(e observability.LogEntry) uint64 { return e.Seq })
	}
	m.clampScroll()
	m.dirty = true
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
		if m.focus == focusUsage {
			rows := m.filteredSummaries()
			if m.usageScroll < len(rows) {
				s := rows[m.usageScroll]
				m.usageDetail = &s
				m.metadataScroll = 0
				return true
			}
		}
		if m.focus == focusProviders {
			providers := m.providerList()
			if m.providerCursor < len(providers) {
				m.providerDetailName = providers[m.providerCursor].Name
				m.providerDetailScroll = 0
				return true
			}
		}
		return false
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
	case "5":
		m.bottomTab = bottomTabRequests
		m.focus = focusBottom
		return true
	case "[", "]":
		tabs := []bottomTab{bottomTabAliases, bottomTabLogs, bottomTabPayload, bottomTabBlocks, bottomTabRequests}
		for i, tab := range tabs {
			if tab == m.bottomTab {
				delta := 1
				if msg.String() == "[" {
					delta = -1
				}
				m.bottomTab = tabs[(i+delta+len(tabs))%len(tabs)]
				break
			}
		}
		m.focus = focusBottom
		return true
	case "e":
		if m.focus != focusUsage {
			return false
		}
		m.errorsOnly = !m.errorsOnly
		m.usageScroll = 0
		return true
	case "u":
		if m.focus != focusUsage {
			return false
		}
		m.usageUpstream = !m.usageUpstream
		m.usageScroll = 0
		return true
	case "t":
		if m.focus != focusUsage {
			return false
		}
		m.tenantIndex++
		if m.tenantIndex > len(m.tenantNames()) {
			m.tenantIndex = 0
		}
		m.usageScroll = 0
		return true
	case "l":
		if m.focus != focusBottom || m.bottomTab != bottomTabLogs {
			return false
		}
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
		if m.focus != focusBottom {
			return false
		}
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
	if m.blockDetailOpen() {
		n--
	}
	if n < 1 {
		n = 1
	}
	return n
}

func (m *model) scrollTop() bool {
	switch m.focus {
	case focusProviders:
		return m.scrollProviders(-m.providerDataRows())
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
		return m.scrollProviders(m.providerDataRows())
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
	next := clampInt(m.providerCursor+delta, 0, max(0, m.providerDataRows()-1))
	if next == m.providerCursor {
		return false
	}
	m.providerCursor = next
	m.clampProviderCursor()
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
	m.clampRequestCursor()
	m.clampProviderCursor()
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
	if m.focusedLayout() {
		return zoomBodyHeight(m.height) - 1
	}
	return m.statsHeight
}

func (m *model) effBottomHeight() int {
	if m.focusedLayout() {
		return zoomBodyHeight(m.height)
	}
	return m.bottomHeight
}

func (m *model) usageVisibleRows() int {
	_, usageH := m.splitStatsHeight(m.effStatsHeight())
	if m.focusedLayout() {
		usageH = m.effStatsHeight()
	}
	n := usageH - 5
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
	if m.focusedLayout() {
		return false
	}
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
	m.clampScroll()
	return true
}

func (m *model) relayout() {
	if m.height <= 11 {
		m.bottomHeight = 0
		m.statsHeight = 0
		return
	}
	available := m.height - chromeLines
	if m.compactLayout() {
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
	if m.showHelp {
		return m.renderHelp()
	}
	if m.width < 80 || m.height < 12 {
		return m.renderNotice(fmt.Sprintf("Terminal too small (%dx%d). Need at least 80x12.", m.width, m.height))
	}
	if m.snapshot == nil {
		return m.renderNotice("no snapshot")
	}
	header := renderHeader(m)
	rate := renderRate(m, m.width)
	if m.requestDetail != nil || m.usageDetail != nil {
		body := renderMetadataDetail(m, m.width, zoomBodyHeight(m.height))
		return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, body, renderFooter(m)), m.width)
	}
	if m.providerDetailName != "" {
		body := renderProviderDetail(m, m.width, zoomBodyHeight(m.height))
		return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, body, renderFooter(m)), m.width)
	}
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
	if m.focusedLayout() {
		bodyHeight := zoomBodyHeight(m.height)
		var body string
		switch m.focus {
		case focusProviders:
			body = lipgloss.JoinVertical(lipgloss.Left, renderTabStrip(m, m.width), renderProviders(m, m.width, bodyHeight-1))
		case focusUsage:
			body = lipgloss.JoinVertical(lipgloss.Left, renderTabStrip(m, m.width), renderUsage(m, m.width, bodyHeight-1))
		default:
			body = renderBottom(m, m.width, bodyHeight)
		}
		return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, body, renderFooter(m)), m.width)
	}
	statsHeight := m.effStatsHeight()
	if statsHeight < 4 {
		statsHeight = 4
	}
	top := m.renderTopStacked(m.width, statsHeight)
	bottom := renderBottom(m, m.width, m.effBottomHeight())
	return fitView(lipgloss.JoinVertical(lipgloss.Left, header, rate, top, bottom, renderFooter(m)), m.width)
}

func (m *model) renderTopStacked(width, statsHeight int) string {
	provH, usageH := m.splitStatsHeight(statsHeight)
	providers := renderProviders(m, width, provH)
	usage := renderUsage(m, width, usageH)
	return lipgloss.JoinVertical(lipgloss.Left, providers, usage)
}

func (m *model) splitStatsHeight(statsHeight int) (provH, usageH int) {
	const minUsage = 6
	if statsHeight < 4+minUsage {
		statsHeight = 4 + minUsage
	}
	rows := 0
	if m.snapshot != nil {
		rows = len(m.snapshot.Providers) + len(m.snapshot.DisabledProviders)
	}
	need := rows + 4
	if noteRows := providerNoteCount(m.snapshotUsageSummaries()); noteRows > 0 {
		need += noteRows
	}
	if need > statsHeight-minUsage {
		need = statsHeight - minUsage
	}
	if need < 4 {
		need = 4
	}
	provH = need
	usageH = statsHeight - provH
	if usageH < minUsage {
		usageH = minUsage
		provH = statsHeight - usageH
	}
	return provH, usageH
}

func (m *model) snapshotUsageSummaries() []accounting.Summary {
	if m == nil || m.snapshot == nil || m.snapshot.Usage == nil {
		return nil
	}
	return m.snapshot.Usage.Summaries()
}

var ansiSeq = regexp.MustCompile("\x1b\\[[0-9;:?]*[ -/]*[@-~]|\x1b\\][^\x07]*(?:\x07|\x1b\\\\)|\x1b[()][0-9A-Za-z]")

func visibleLen(s string) int {
	return ansi.StringWidth(s)
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
		lines[i] = padLine(ansi.Truncate(logOneLine(l), max(0, width), ""), width)
	}
	return strings.Join(lines, "\n")
}

func (m *model) providerVisibleRows() int {
	notes := 0
	if m.snapshot != nil && m.snapshot.Usage != nil {
		notes = providerNoteCount(m.snapshot.Usage.Summaries())
	}
	provH, _ := m.splitStatsHeight(m.effStatsHeight())
	if m.focusedLayout() {
		provH = m.effStatsHeight()
	}
	n := provH - 4 - notes
	if n < 1 {
		n = 1
	}
	return n
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
	case bottomTabRequests:
		pane = renderRequests(m, width, paneHeight)
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
	aliasLabel := "1:Aliases"
	if width >= 100 && m.snapshot != nil {
		aliasLabel = fmt.Sprintf("1:Aliases(%d) cool:%d", len(m.snapshot.Aliases), len(m.snapshot.Cooldowns))
	}
	logsLabel := "2:Logs"
	payloadLabel := "3:Payloads"
	alias := dimStyle.Render("  " + aliasLabel)
	logs := dimStyle.Render("  " + logsLabel)
	payload := dimStyle.Render("  " + payloadLabel)
	blocksLabel := "4:Blocks"
	if width >= 100 && m.blockKnown {
		blocksLabel = fmt.Sprintf("4:Blocks(%d)", len(m.blocks))
	}
	blocks := dimStyle.Render("  " + blocksLabel)
	requests := dimStyle.Render("  5:Requests")
	switch m.bottomTab {
	case bottomTabAliases:
		alias = activeStyle.Render("▸ " + aliasLabel)
	case bottomTabLogs:
		logs = activeStyle.Render("▸ " + logsLabel)
	case bottomTabBlocks:
		blocks = activeStyle.Render("▸ " + blocksLabel)
	case bottomTabRequests:
		requests = activeStyle.Render("▸ 5:Requests")
	default:
		payload = activeStyle.Render("▸ " + payloadLabel)
	}
	return fitRow(alias+" "+logs+" "+payload+" "+blocks+" "+requests+" [prev ]next", width)
}

func (m *model) renderHelp() string {
	lines := []string{
		"aiproxy dashboard — keys",
		"",
		"  tab/shift+tab cycle focus PROVIDERS / USAGE / bottom tabs",
		"  1/2/3/4/5 or [/] switch tabs (Aliases / Logs / Payloads / Blocks / Requests)",
		"  [ previous / ] next tab, in numbered display order (wraps)",
		"  enter      open/close detail; Usage inspects top visible group (n/N cycles groups)",
		"  /          search Requests/Logs/Payloads; Ctrl+U clears applied search",
		"  Search: AND case-insensitive substrings; field:value or bare words (max 256 chars)",
		"  Requests: id/client/tenant/model/resolved/provider/status/op; e toggles errors",
		"  Logs: id/level (structured metadata only); Payloads: id/model/resolved/provider/status/method/path",
		"  Editor: arrows/Home/End/Backspace/Delete, Ctrl+U clear, Enter apply, Esc cancel",
		"  While editing q/p/h/?/digits/brackets are text; Ctrl+C quit, Ctrl+R retry",
		"  Request detail l/v: exact-ID logs/payload list; Esc returns through detail to Requests",
		"  Correlation ignores target filters temporarily; no history beyond retained lists",
		"  z          zoom focused pane to full screen",
		"  esc        close help, then detail, then zoom; otherwise quit",
		"  tab/number/bracket navigation closes detail before switching",
		"  Under 30 rows: focused pane only; Tab still cycles all panes",
		"  j/k dn/up  move selection / scroll   g/G,home/end top/bottom",
		"  pgup/pgdn  page selection / scroll (bottom panes + detail)",
		"  +/- J/K    resize bottom pane (stacked layout only)",
		"  t/e/u      tenant/errors/upstream (focused USAGE only)",
		"  s          toggle payload errs filter (payloads tab)",
		"  r          refresh payload/block list (focused list only)",
		"  ctrl+r     retry dashboard connection (does not reload credentials)",
		"  o          toggle newest/oldest order (logs/payloads tab)",
		"  l          cycle log level (focused LOGS only)",
		"  n/N        next/previous block finding (locked while recording)",
		"  a/s/d      allow non-secret / redact / deny SELECTED finding hash only",
		"             Persistent GLOBAL future-match effect; never replays the request.",
		"             Pending actions cannot repeat; success suppresses the same action.",
		"             Failure outcome may be unknown; a/s/d deliberately retries.",
		"  Blocks     Enter consumes take-once capture; re-open is unavailable.",
		"             Enter works only from a visible row; loading/error lists ignore it.",
		"  Inspection Payload/block text wraps; j/k, PgUp/PgDn, Home/End inspect all rows.",
		"             ID, finding scope, status and cap notice stay visible while scrolling.",
		"             Payload pretty output: 64 KiB; body/pretty truncation is labeled.",
		"             Block snippets are server-capped; truncation status is unavailable.",
		"             Controls/invalid UTF-8 display as escapes, never terminal commands.",
		"  1/2/3/4/5  navigate tabs outside search; never record a decision",
		"  p          freeze data/clock; resume latest results (connection stays live)",
		"  ?/h/Esc/Enter close help; q quits outside search; Ctrl+C quits every mode",
		"  Help owns input: scroll keys work, pane actions are ignored",
		"",
		"Legend: ✓ healthy · ✗ unhealthy · ? unknown (no health report yet)",
		"  HC = upstream healthcheck: ✓ passing · ✗ failing · ? pending · - none.",
		"  HOST is configured, never DNS-resolved. Provider Enter: probe/models/settings.",
		"  Probe age uses stored last-check time; missing metadata is unknown.",
		"  Endpoints omit secrets/opaque paths. Alias counters are provider-wide, not target counts.",
		"  ERR% excludes 429 (throttled shown separately) · ~ = streaming or",
		"  untokenized response (no token accounting) · n/a = no positive duration.",
		"  P95/n: positive durations from last <=200 global completions; no time window.",
		"  Provider counts: global lifetime. Usage/EST$: retained rolling buckets.",
		"  EST$ uses current prices; - means unavailable, never a partial subtotal.",
		"  Rates: global last 60/300 complete seconds; graph: 15 one-minute bins.",
		"  cool Ns = alias target cooling with remaining time.",
		"",
		"End of help. Press ? or Esc to return.",
	}
	return m.renderHelpPage(lines)
}

func renderFooter(m *model) string {
	return m.renderContextFooter()
}

func renderHeader(m *model) string {
	snap := m.snapshot
	uptime := m.now.Sub(snap.StartTime).Round(time.Second)
	left := logOneLine(fmt.Sprintf("aiproxy %s  %s", snap.Version, snap.Address))
	active := len(snap.Providers)
	disabled := len(snap.DisabledProviders)
	status := "LIVE"
	statusColor := "#22C55E"
	if m.paused {
		status = "PAUSED"
		statusColor = "#FBBF24"
	} else if m.staleErr != "" {
		age := m.now.Sub(m.staleAt).Round(time.Second)
		status = fmt.Sprintf("STALE %s (%s)", age, truncate(m.staleErr, 40))
		statusColor = "#F87171"
	}
	if m.connection.Denied || m.connection.Reconnecting {
		status = "RECONNECTING"
		if m.connection.Denied {
			status = "DENIED"
		}
		if m.paused {
			status += "/PAUSED"
		}
		statusColor = "#F87171"
	}
	right := fmt.Sprintf("providers %d (+%d disabled)  aliases %d  auth %s  uptime %s",
		active, disabled, len(snap.Aliases), snap.AuthMode, uptime)
	left = truncate(left, max(0, m.width-visibleLen(status)-6))
	right = logOneLine(right)
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
	line1 := rateLine(m.snapshot.Usage)
	if runeLen(line1) > width && width > 20 {
		line1 = truncate(line1, width)
	}
	line2 := providerScope(m.snapshot.Usage) + " · P95/n last≤200 (no time window) · EST$ " + retentionLabel(m.snapshot.Usage)
	if width < 100 {
		line2 = providerScope(m.snapshot.Usage) + " · P95/n ≤200 untimed · EST$ " + retentionLabel(m.snapshot.Usage)
	}
	if runeLen(line2) > width && width > 20 {
		line2 = truncate(line2, width)
	}
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("#94A3B8"))
	return style.Render(line1 + "\n" + line2)
}

func (m *model) tenantNames() []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range m.snapshotUsageSummaries() {
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
	if m.tenantIndex > len(names) {
		return ""
	}
	return names[m.tenantIndex-1]
}

func (m *model) filteredSummaries() []accounting.Summary {
	if m.snapshot == nil || m.snapshot.Usage == nil {
		return nil
	}
	summaries := m.snapshot.Usage.Summaries()
	if m.usageUpstream {
		summaries = upstreamAsSummaries(retainedUpstream(m.snapshot.Usage))
	}
	tenant := m.activeTenant()
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
	indices := make(map[accounting.Summary]int)
	for _, u := range upstream {
		key := accounting.Summary{Tenant: u.Tenant, Client: u.Client, Model: u.Provider + "/" + u.Model, Operation: u.Operation, StatusCode: u.StatusCode}
		if i, ok := indices[key]; ok {
			s := &out[i]
			s.Count += u.Count
			s.PromptTokens += u.PromptTokens
			s.CompletionTokens += u.CompletionTokens
			s.TotalTokens += u.TotalTokens
			s.CachedTokens += u.CachedTokens
			s.CacheCreationTokens += u.CacheCreationTokens
			s.CacheReadTokens += u.CacheReadTokens
			continue
		}
		indices[key] = len(out)
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
	border, inner := paneBox(width, height, m.focus == focusProviders)
	summaries := snap.Usage.Summaries()
	recent := snap.Usage.Recent(recentLimit)
	stats := snap.Usage.ProviderSummaries()
	byName := make(map[string]accounting.ProviderSummary, len(stats))
	for _, ps := range stats {
		byName[ps.Provider] = ps
	}
	latency, samples := p95WithSamples(recent, stats)
	prices := pricingIndex(snap.Providers)
	upstream := retainedUpstream(snap.Usage)
	if retainedBilling(snap.Usage) == nil {
		prices = nil
	}
	var names []string
	var ips []string
	var reqs, t429s []int64
	var toks, costs []string
	for _, p := range snap.Providers {
		ps := byName[p.Name]
		names = append(names, p.Name)
		ips = append(ips, m.providerHost(p))
		reqs = append(reqs, ps.Requests)
		t429s = append(t429s, ps.Throttled)
		toks = append(toks, providerTokensText(ps))
		costs = append(costs, providerCostTextWithAliases(p.Name, summaries, prices, snap.Aliases, upstream))
	}
	for _, p := range snap.DisabledProviders {
		ps := byName[p.Name]
		names = append(names, p.Name)
		ips = append(ips, m.providerHost(p))
		reqs = append(reqs, ps.Requests)
		t429s = append(t429s, ps.Throttled)
		toks = append(toks, providerTokensText(ps))
		costs = append(costs, "-")
	}
	nameW, ipW, reqW, t429W, tokW, costW := providerColWidths(names, ips, reqs, t429s, toks, costs, inner-2)
	rows := []string{headerStyle.Render(fitRow("  "+headerCells([]col{{"PROVIDER", nameW}, {"", 1}, {"HC", 2}, {"REQS", reqW}, {"ERR%", 6}, {"429", t429W}, {"P95/n", 12}, {"TOKENS", tokW}, {"EST$", costW}, {"HOST", ipW}}), inner))}
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
	hcByName := m.probeMarks()
	for _, p := range snap.Providers {
		known, healthy := healthKnown(m.health, p.Name)
		ps := byName[p.Name]
		data = append(data, providerLine{text: providerRow(p.Name, known, healthy, hcByName[p.Name], ps, ps.Throttled, latency[p.Name], samples[p.Name], false, m.providerHost(p), providerCostTextWithAliases(p.Name, summaries, prices, snap.Aliases, upstream), nameW, ipW, reqW, t429W, tokW, costW, providerCountersAvailable(snap.Usage))})
	}
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))
	for _, p := range snap.DisabledProviders {
		ps := byName[p.Name]
		data = append(data, providerLine{
			text: providerRow(p.Name, true, false, "-", ps, ps.Throttled, latency[p.Name], samples[p.Name], true, m.providerHost(p), "-", nameW, ipW, reqW, t429W, tokW, costW, providerCountersAvailable(snap.Usage)),
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
	for i, d := range data[start:end] {
		prefix := "  "
		if start+i == m.providerCursor {
			prefix = "> "
		}
		line := fitRow(prefix+d.text, inner)
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
		rows = append(rows, dimStyle.Render(fitRow(fmt.Sprintf("unresolved/forbidden: %s retained reqs (hidden)", comma(unresolved)), inner)))
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
			return "hc pending " + dashrpc.DiagnosticURL(e.Path)
		}
		if e.Healthy {
			return fmt.Sprintf("hc ✓ %s %d", dashrpc.DiagnosticURL(e.Path), e.StatusCode)
		}
		msg := dashrpc.DiagnosticReason(e.Message)
		if msg == "" {
			msg = fmt.Sprintf("status %d", e.StatusCode)
		}
		return fmt.Sprintf("hc ✗ %s %s", dashrpc.DiagnosticURL(e.Path), truncate(msg, 40))
	}
	return ""
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

func runeLen(s string) int {
	return visibleLen(s)
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

func providerColWidths(names, ips []string, requests, throttled []int64, tokens, costs []string, inner int) (nameW, ipW, reqW, t429W, tokW, costW int) {
	nameW, ipW, reqW, t429W, tokW, costW = len("PROVIDER"), len("HOST"), len("REQS"), len("429"), len("TOKENS"), len("COST")
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
	for _, c := range costs {
		costW = max(costW, runeLen(c))
	}
	got := flexWidths([]flexCol{
		{content: nameW, min: 8, max: 32, flex: 2},
		{content: 1, min: 1, max: 1},
		{content: 2, min: 2, max: 2},
		{content: reqW, min: 4, max: 10},
		{content: 6, min: 6, max: 6},
		{content: t429W, min: 3, max: 8},
		{content: 12, min: 12, max: 12},
		{content: tokW, min: 6, max: 0, flex: 3},
		{content: costW, min: 4, max: 12, flex: 1},
		{content: ipW, min: 2, max: 32, flex: 1},
	}, inner)
	return got[0], got[9], got[3], got[5], got[7], got[8]
}

func costTexts(summaries []accounting.Summary, prices map[string]*config.ModelPricing, aliases []config.Alias, upstream []accounting.UpstreamSummary) map[accounting.Summary]string {
	out := make(map[accounting.Summary]string, len(summaries))
	for _, s := range summaries {
		out[s] = summaryCostTextWithAliases(s, prices, aliases, upstream)
	}
	return out
}

func usageColWidths(summaries []accounting.Summary, costs map[accounting.Summary]string, inner int) (modelW, opW, countW, tokW, costW int) {
	modelW, opW, countW, tokW, costW = len("MODEL"), len("OP"), len("COUNT"), len("TOKENS"), len("COST")
	for _, s := range summaries {
		modelW = max(modelW, runeLen(s.Model))
		opW = max(opW, runeLen(s.Operation))
		countW = max(countW, runeLen(comma(s.Count)))
		tokW = max(tokW, runeLen(tokensText(s)))
		if c, ok := costs[s]; ok {
			costW = max(costW, runeLen(c))
		}
	}
	got := flexWidths([]flexCol{
		{content: modelW, min: 10, max: 48, flex: 2},
		{content: opW, min: 8, max: 24, flex: 1},
		{content: 6, min: 6, max: 6},
		{content: countW, min: 4, max: 10},
		{content: tokW, min: 6, max: 0, flex: 3},
		{content: costW, min: 4, max: 12, flex: 1},
	}, inner)
	return got[0], got[1], got[3], got[4], got[5]
}

func pricingIndex(providers []config.Provider) map[string]*config.ModelPricing {
	out := map[string]*config.ModelPricing{}
	for _, p := range providers {
		for _, m := range p.Models {
			if m.Pricing != nil && m.Pricing.HasRates() {
				out[p.Name+"/"+m.Name] = m.Pricing
			}
		}
	}
	return out
}

func formatCost(dollars float64) string {
	if dollars < 0 {
		dollars = 0
	}
	if dollars == 0 {
		return "$0.00"
	}
	if dollars < 0.01 {
		return fmt.Sprintf("$%.4f", dollars)
	}
	if dollars < 1000 {
		return fmt.Sprintf("$%.2f", dollars)
	}
	return "$" + comma(int64(dollars+0.5))
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

func providerRow(name string, known, healthy bool, hcMark string, ps accounting.ProviderSummary, throttled int64, p95 time.Duration, samples int, disabled bool, ip, cost string, nameW, ipW, reqW, t429W, tokW, costW int, countersAvailable ...bool) string {
	status := "✓"
	if disabled {
		status = "✗"
	} else if !known {
		status = "?"
	} else if !healthy {
		status = "✗"
	}
	if hcMark == "" {
		hcMark = "?"
	}
	errPct := 0.0
	if ps.Requests > 0 {
		errPct = 100 * float64(ps.Errors) / float64(ps.Requests)
	}
	p95cell := "n/a/0"
	if samples > 0 {
		p95cell = fmt.Sprintf("%s/%d", p95.Round(time.Millisecond), samples)
	}
	reqs, errors, throttles, tokens := comma(ps.Requests), fmt.Sprintf("%.1f%%", errPct), comma(throttled), providerTokensText(ps)
	if len(countersAvailable) > 0 && !countersAvailable[0] {
		reqs, errors, throttles, tokens = "n/a", "n/a", "n/a", "n/a"
	}
	return dataRow([]string{
		truncate(name, nameW),
		status,
		hcMark,
		fmt.Sprintf("%*s", reqW, reqs),
		fmt.Sprintf("%6s", errors),
		fmt.Sprintf("%*s", t429W, throttles),
		p95cell,
		fmt.Sprintf("%*s", tokW, truncate(tokens, tokW)),
		fmt.Sprintf("%*s", costW, truncate(cost, costW)),
		truncate(ip, ipW),
	}, []int{nameW, 1, 2, reqW, 6, t429W, 12, tokW, costW, ipW})
}

func p95LatencyByProvider(recent []accounting.Event) map[string]time.Duration {
	latency, _ := p95WithSamples(recent)
	return latency
}

func p95WithSamples(recent []accounting.Event, stats ...[]accounting.ProviderSummary) (map[string]time.Duration, map[string]int) {
	out := map[string]time.Duration{}
	counts := map[string]int{}
	byProvider := map[string][]time.Duration{}
	names := map[uint64]string{}
	for _, group := range stats {
		for _, p := range group {
			if p.ProviderID != 0 {
				names[p.ProviderID] = p.Provider
			}
		}
	}
	for _, e := range recent {
		if e.Duration <= 0 {
			continue
		}
		prov := accounting.EventProvider(e)
		if name, ok := names[e.ProviderID]; ok {
			prov = name
		} else if e.Truncated.Provider || (e.Provider == "" && e.Truncated.Model) {
			continue
		}
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
	border, inner := paneBox(width, height, m.focus == focusUsage)
	tenant := m.activeTenant()
	title := "USAGE " + retentionLabel(m.snapshot.Usage) + " · estimated cost (- unavailable)"
	if m.focus == focusUsage {
		title = "USAGE [enter] top identity · " + retentionLabel(m.snapshot.Usage) + " · EST$"
	}
	if m.usageUpstream {
		title += " upstream"
	}
	if tenant != "" {
		title += " · tenant:" + tenant
	} else {
		title += " · all tenants/clients"
	}
	if m.errorsOnly {
		title += " errs-only"
	}
	summaries := m.filteredSummaries()
	prices := pricingIndex(m.snapshot.Providers)
	aliases := m.snapshot.Aliases
	upstream := retainedUpstream(m.snapshot.Usage)
	if retainedBilling(m.snapshot.Usage) == nil {
		prices = nil
	}
	modelW, opW, countW, tokW, costW := usageColWidths(summaries, costTexts(summaries, prices, aliases, upstream), inner)
	header := headerStyle.Render(fitRow(headerCells([]col{{"MODEL", modelW}, {"OP", opW}, {"STATUS", 6}, {"COUNT", countW}, {"TOKENS", tokW}, {"EST$", costW}}), inner))
	rows := []string{fitRow(title, inner), header}
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
			fmt.Sprintf("%*s", costW, truncate(summaryCostTextWithAliases(s, prices, aliases, upstream), costW)),
		}, []int{modelW, opW, 6, countW, tokW, costW}), inner))
	}
	if total == 0 {
		if m.usageUpstream && retainedBilling(m.snapshot.Usage) == nil {
			rows = append(rows, "retained upstream usage unavailable")
		} else {
			rows = append(rows, "no usage recorded yet")
		}
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
	borderStyle, inner := paneBox(width, height, m.focus == focusBottom && m.bottomTab == bottomTabAliases)
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
		rows = append(rows, truncate(line, inner))
	}
	if end < len(aliases) {
		rows = append(rows, fmt.Sprintf("… %d more (j/k move)", len(aliases)-end))
	} else if len(aliases) > visible {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#475569")).Render("[j/k] move [enter] detail"))
	}
	return borderStyle.Render(strings.Join(rows, "\n"))
}

func renderAliasDetail(m *model, width, height int) string {
	borderStyle, inner := paneBox(width, height, true)
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
	affinity := "disabled"
	if a.SessionAffinity != nil {
		affinity = strings.Join(config.SessionAffinityHeaders(*a), ",")
		if affinity == "" {
			affinity = "default"
		}
	}
	if m.snapshot.AliasAffinity != nil && m.snapshot.AliasAffinity[a.Name] == nil {
		affinity = "unknown (snapshot metadata unavailable)"
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
	hcByName := m.probeMarks()
	raw := []string{
		"alias: " + a.Name,
		"algorithm: " + algo,
		"retry: " + retry,
		"session_affinity: " + affinity,
		"Counters: provider-wide lifetime; repeated per target, NOT target/alias counts.",
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
			hc = "?"
		}
		line := fmt.Sprintf("%s %s/%s hc:%s provider-wide lifetime reqs:%s errs:%s 429:%s tok:%s %s",
			mark, t.Provider, t.Model, hc, comma(ps.Requests), comma(ps.Errors),
			comma(ps.Throttled), comma(ps.TotalTokens), state)
		if !providerCountersAvailable(m.snapshot.Usage) {
			line = fmt.Sprintf("%s %s/%s hc:%s provider counters unavailable %s", mark, t.Provider, t.Model, hc, state)
		}
		raw = append(raw, wrapText(line, inner)...)
		if detail := healthcheckDetail(m.snapshot.Healthchecks, t.Provider); detail != "" {
			raw = append(raw, wrapText("  "+detail, inner)...)
		}
	}
	var wrapped []string
	for _, line := range raw {
		wrapped = append(wrapped, wrapText(line, inner)...)
	}
	raw = wrapped
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
	before := m.orderedPayloads()
	m.payloadOldestFirst = !m.payloadOldestFirst
	m.payloadCursor = anchoredIndex(before, m.orderedPayloads(), m.payloadCursor, payloadIdentity)
	m.payloadOffset = anchoredIndex(before, m.orderedPayloads(), m.payloadOffset, payloadIdentity)
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
	filtered := make([]observability.LogEntry, 0, len(all))
	for _, e := range all {
		if m.correlation != nil && m.bottomTab == bottomTabLogs {
			if e.RequestID == m.correlation.event.RequestID {
				filtered = append(filtered, e)
			}
		} else if metadataMatch(m.queries[bottomTabLogs], map[string]string{"id": e.RequestID, "level": e.Level.String()}) {
			filtered = append(filtered, e)
		}
	}
	all = filtered
	var out []observability.LogEntry
	if !m.logFilterOn || (m.correlation != nil && m.bottomTab == bottomTabLogs) {
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
	borderStyle, inner := paneBox(width, height, m.focus == focusBottom && m.bottomTab == bottomTabLogs)
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
	attrsContent := len("ATTRS")
	msgContent := len("MESSAGE")
	for _, e := range page {
		attrsContent = max(attrsContent, runeLen(logOneLine(orDash(e.Attrs))))
		msgContent = max(msgContent, runeLen(logOneLine(e.Message)))
	}
	if len(page) == 0 {
		for _, e := range entries {
			attrsContent = max(attrsContent, runeLen(logOneLine(orDash(e.Attrs))))
			msgContent = max(msgContent, runeLen(logOneLine(e.Message)))
		}
	}
	budget := inner - 2
	if budget < 40 {
		budget = 40
	}
	msgWidth, attrsWidth := logColWidths(msgContent, attrsContent, budget)
	levelName := "all"
	if m.logFilterOn {
		levelName = ">=" + m.logMinLevel.String()
	}
	if m.correlation != nil && m.bottomTab == bottomTabLogs {
		levelName = "ID-only"
	}
	order := "newest-first"
	if m.logOldestFirst {
		order = "oldest-first"
	}
	title := fmt.Sprintf("LOGS %s (%s) %s", order, levelName, m.searchLabel(bottomTabLogs))
	rows := []string{title, headerStyle.Render(fitRow(headerCells([]col{{"TIME", 8}, {"LEVEL", 6}, {"MESSAGE", msgWidth}, {"ATTRS", attrsWidth}}), inner))}
	if height <= 3 {
		return borderStyle.Render(strings.Join(rows, "\n"))
	}
	if len(entries) == 0 {
		rows = append(rows, m.emptySearchText(bottomTabLogs, "no logs captured"))
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
	borderStyle, inner := paneBox(width, height, true)
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
	return strings.Split(ansi.Hardwrap(s, width, true), "\n")
}

func logOneLine(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\t", " ")
	return s
}

func logColWidths(msgContent, attrsContent, budget int) (msgW, attrsW int) {
	got := flexWidths([]flexCol{
		{content: 8, min: 8, max: 8},
		{content: 6, min: 6, max: 6},
		{content: msgContent, min: 8, max: 80, flex: 1},
		{content: attrsContent, min: 16, max: 0, flex: 3},
	}, budget)
	return got[2], got[3]
}

func renderLogEntry(msgWidth, attrsWidth int, e observability.LogEntry) string {
	levelStyle := levelStyleFor(e.Level)
	attrs := logOneLine(orDash(e.Attrs))
	msg := logOneLine(e.Message)
	return dataRow([]string{
		e.Time.Format("15:04:05"),
		levelStyle.Render(padRight(truncate(e.Level.String(), 6), 6)),
		truncate(msg, msgWidth),
		truncate(attrs, attrsWidth),
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
	if n <= 0 {
		return ""
	}
	return ansi.Truncate(logOneLine(s), n, "…")
}

func padRight(s string, w int) string {
	n := visibleLen(s)
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

func paneBox(width, height int, focused bool) (paneFrame, int) {
	inner := width - 2
	if inner < 10 {
		inner = 10
	}
	border := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Width(width).
		Height(height)
	if focused {
		border = border.BorderForeground(lipgloss.Color("#38BDF8"))
	}
	return paneFrame{style: border, width: width, height: height}, inner
}

type flexCol struct {
	content int
	min     int
	max     int
	flex    int
}

func flexWidths(cols []flexCol, inner int) []int {
	out := make([]int, len(cols))
	if len(cols) == 0 {
		return out
	}
	total := 0
	for i, c := range cols {
		w := c.content
		if w < c.min {
			w = c.min
		}
		if c.max > 0 && w > c.max {
			w = c.max
		}
		out[i] = w
		total += w
	}
	gaps := len(cols) - 1
	if gaps < 0 {
		gaps = 0
	}
	total += gaps
	if total <= inner {
		return distributeFlexGrow(out, cols, inner-gaps)
	}
	return shrinkFlex(out, cols, inner-gaps)
}

func distributeFlexGrow(out []int, cols []flexCol, budget int) []int {
	used := 0
	for _, w := range out {
		used += w
	}
	slack := budget - used
	if slack <= 0 {
		return out
	}
	for slack > 0 {
		totalFlex := 0
		for i, c := range cols {
			if c.flex <= 0 {
				continue
			}
			if c.max > 0 && out[i] >= c.max {
				continue
			}
			totalFlex += c.flex
		}
		if totalFlex <= 0 {
			break
		}
		progress := false
		base := slack
		for i, c := range cols {
			if base <= 0 {
				break
			}
			if c.flex <= 0 {
				continue
			}
			if c.max > 0 && out[i] >= c.max {
				continue
			}
			share := base * c.flex / totalFlex
			if share < 1 {
				share = 1
			}
			if c.max > 0 && out[i]+share > c.max {
				share = c.max - out[i]
			}
			if share > slack {
				share = slack
			}
			if share <= 0 {
				continue
			}
			out[i] += share
			slack -= share
			progress = true
			if slack <= 0 {
				break
			}
		}
		if !progress {
			break
		}
	}
	return out
}

func shrinkFlex(out []int, cols []flexCol, budget int) []int {
	used := 0
	for _, w := range out {
		used += w
	}
	over := used - budget
	if over <= 0 {
		return out
	}
	shrinkable := func(i int) int {
		if out[i] > cols[i].min {
			return out[i] - cols[i].min
		}
		return 0
	}
	for over > 0 {
		totalFlex := 0
		for i := range cols {
			if shrinkable(i) > 0 && cols[i].flex > 0 {
				totalFlex += cols[i].flex
			}
		}
		if totalFlex <= 0 {
			break
		}
		progress := false
		for i, c := range cols {
			if over <= 0 {
				break
			}
			if c.flex <= 0 || shrinkable(i) <= 0 {
				continue
			}
			share := over * c.flex / totalFlex
			if share < 1 {
				share = 1
			}
			if share > shrinkable(i) {
				share = shrinkable(i)
			}
			out[i] -= share
			over -= share
			progress = true
		}
		if !progress {
			break
		}
	}
	return out
}
