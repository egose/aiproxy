package dashrpc

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/filestore"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
)

const (
	SnapshotPath        = "/_internal/dashboard/snapshot"
	LogsPath            = "/_internal/dashboard/logs"
	PayloadsPath        = "/_internal/dashboard/payloads"
	PayloadPathPrefix   = "/_internal/dashboard/payloads/"
	BlocksPath          = "/_internal/dashboard/blocks"
	BlockPathPrefix     = "/_internal/dashboard/blocks/"
	BlockDecisionSuffix = "/decision"
	AuthHeaderName      = "Authorization"
	AuthScheme          = "Bearer "
	PayloadListDefault  = 100
	PayloadListMax      = 500
	BlocksListMax       = 500
)

type Snapshot struct {
	Version           string                   `json:"version"`
	Address           string                   `json:"address"`
	AuthMode          string                   `json:"auth_mode"`
	StartTime         time.Time                `json:"start_time"`
	Now               time.Time                `json:"now"`
	Providers         []Provider               `json:"providers"`
	DisabledProviders []Provider               `json:"disabled_providers"`
	Aliases           []Alias                  `json:"aliases"`
	Health            map[string]bool          `json:"health"`
	Cooldowns         []CooldownInfo           `json:"cooldowns,omitempty"`
	Healthchecks      []HealthcheckStatus      `json:"healthchecks,omitempty"`
	Usage             []Usage                  `json:"usage"`
	ProviderStats     []ProviderStat           `json:"provider_stats,omitempty"`
	Upstream          []UpstreamUsage          `json:"upstream,omitempty"`
	Recent            []Recent                 `json:"recent"`
	Logs              []observability.LogEntry `json:"logs"`
	LastSeq           uint64                   `json:"last_seq"`
	PayloadEnabled    bool                     `json:"payload_enabled,omitempty"`
}

type CooldownInfo struct {
	Alias       string `json:"alias"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	RemainingMs int64  `json:"remaining_ms"`
}

type HealthcheckStatus struct {
	Provider    string    `json:"provider"`
	Configured  bool      `json:"configured"`
	Checked     bool      `json:"checked"`
	Healthy     bool      `json:"healthy"`
	StatusCode  int       `json:"status_code,omitempty"`
	Message     string    `json:"message,omitempty"`
	Path        string    `json:"path,omitempty"`
	LastChecked time.Time `json:"last_checked,omitempty"`
}

type Provider struct {
	Type        string       `json:"type"`
	Name        string       `json:"name"`
	DisplayName string       `json:"display_name,omitempty"`
	BaseURL     string       `json:"base_url,omitempty"`
	Models      []ModelPrice `json:"models"`
}

type ModelPrice struct {
	Name                 string   `json:"name"`
	InputPerMillion      *float64 `json:"input_per_million,omitempty"`
	OutputPerMillion     *float64 `json:"output_per_million,omitempty"`
	CachedPerMillion     *float64 `json:"cached_per_million,omitempty"`
	CacheWritePerMillion *float64 `json:"cache_write_per_million,omitempty"`
}

type Alias struct {
	Name             string        `json:"name"`
	Algorithm        string        `json:"algorithm"`
	RetryStatusCodes []int         `json:"retry_status_codes,omitempty"`
	Targets          []AliasTarget `json:"targets"`
}

type AliasTarget struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

type Usage = accounting.Summary
type Recent = accounting.Event
type ProviderStat = accounting.ProviderSummary
type UpstreamUsage = accounting.UpstreamSummary

type Logs struct {
	Logs    []observability.LogEntry `json:"logs"`
	LastSeq uint64                   `json:"last_seq"`
}

type PayloadSummary = payloadlog.Summary

type PayloadList struct {
	Enabled  bool             `json:"enabled"`
	Payloads []PayloadSummary `json:"payloads"`
}

type BlockSummary struct {
	BlockID      string   `json:"block_id"`
	Timestamp    string   `json:"ts"`
	Operation    string   `json:"operation,omitempty"`
	PublicModel  string   `json:"public_model,omitempty"`
	RuleIDs      []string `json:"rule_ids"`
	FindingCount int      `json:"finding_count"`
}

type BlockCapture struct {
	BlockID     string         `json:"block_id"`
	Timestamp   string         `json:"ts"`
	Operation   string         `json:"operation,omitempty"`
	PublicModel string         `json:"public_model,omitempty"`
	RuleIDs     []string       `json:"rule_ids"`
	Findings    []BlockFinding `json:"findings"`
}

type BlockFinding struct {
	RuleID      string `json:"rule_id"`
	Description string `json:"description,omitempty"`
	Secret      string `json:"secret"`
	SecretSHA   string `json:"secret_sha256,omitempty"`
	Match       string `json:"match,omitempty"`
	Line        string `json:"line,omitempty"`
}

type BlockDecisionRequest struct {
	Action      string   `json:"action"`
	FindingSHAs []string `json:"finding_shas"`
}

type BlockDecisionResponse struct {
	Ok     bool   `json:"ok"`
	Action string `json:"action"`
	Count  int    `json:"count"`
}

type BlockList struct {
	Enabled bool           `json:"enabled"`
	Blocks  []BlockSummary `json:"blocks"`
}

type Source interface {
	Enabled() bool
	Token() string
	Snapshot(context.Context, int) Snapshot
	Logs(uint64) Logs
}

type RuntimeSource struct {
	dashboard    config.Dashboard
	version      string
	address      string
	authMode     string
	startTime    time.Time
	catalog      config.Catalog
	usage        *accounting.Aggregator
	health       *providerhealth.Tracker
	logs         *observability.LogBuffer
	cooldowns    func() []CooldownInfo
	healthchecks func() []HealthcheckStatus
	payloadDir   string
	payloadOn    bool
	blocks       BlockStore
}

type BlockStore interface {
	ListBlocks() []BlockSummary
	TakeBlock(blockID string) (BlockCapture, bool)
}

func (s *RuntimeSource) SetPayloadSource(dir string, enabled bool) {
	if s == nil {
		return
	}
	s.payloadDir = dir
	s.payloadOn = enabled
}

func (s *RuntimeSource) SetBlockSource(store BlockStore) {
	if s == nil {
		return
	}
	s.blocks = store
}

func (s *RuntimeSource) BlocksEnabled() bool {
	return s != nil && s.blocks != nil
}

func (s *RuntimeSource) ListBlocks() []BlockSummary {
	if s == nil || s.blocks == nil {
		return nil
	}
	return s.blocks.ListBlocks()
}

func (s *RuntimeSource) TakeBlock(blockID string) (BlockCapture, bool) {
	if s == nil || s.blocks == nil {
		return BlockCapture{}, false
	}
	return s.blocks.TakeBlock(blockID)
}

func (s *RuntimeSource) PayloadEnabled() bool {
	return s != nil && s.payloadOn && s.payloadDir != ""
}

func (s *RuntimeSource) ListPayloads(limit int, errorsOnly bool) ([]PayloadSummary, error) {
	if s == nil || !s.PayloadEnabled() {
		return nil, nil
	}
	return payloadlog.ListRecent(s.payloadDir, limit, errorsOnly)
}

func (s *RuntimeSource) GetPayload(requestID string) (json.RawMessage, error) {
	if s == nil || !s.PayloadEnabled() {
		return nil, payloadlog.ErrPayloadNotFound
	}
	return payloadlog.Get(s.payloadDir, requestID)
}

func (s *RuntimeSource) SetCooldownSource(fn func() []CooldownInfo) {
	if s == nil {
		return
	}
	s.cooldowns = fn
}

func (s *RuntimeSource) SetHealthcheckSource(fn func() []HealthcheckStatus) {
	if s == nil {
		return
	}
	s.healthchecks = fn
}

func NewRuntimeSource(dashboard config.Dashboard, version, address, authMode string, startTime time.Time, catalog config.Catalog, usage *accounting.Aggregator, health *providerhealth.Tracker, logs *observability.LogBuffer) *RuntimeSource {
	return &RuntimeSource{
		dashboard: dashboard,
		version:   version,
		address:   address,
		authMode:  authMode,
		startTime: startTime,
		catalog:   catalog,
		usage:     usage,
		health:    health,
		logs:      logs,
	}
}

func (s *RuntimeSource) Enabled() bool {
	return s != nil && s.dashboard.Enabled
}

func (s *RuntimeSource) Token() string {
	if s == nil {
		return ""
	}
	return s.dashboard.Token
}

func (s *RuntimeSource) Snapshot(ctx context.Context, recentN int) Snapshot {
	if s == nil {
		return Snapshot{}
	}
	snap := BuildContext(ctx, s.version, s.address, s.authMode, s.startTime, s.catalog, s.usage, s.health, s.logs, recentN)
	if s.cooldowns != nil {
		snap.Cooldowns = s.cooldowns()
	}
	if s.healthchecks != nil {
		snap.Healthchecks = s.healthchecks()
	}
	if s.usage != nil {
		snap.ProviderStats = s.usage.ProviderSummaries()
		snap.Upstream = s.usage.UpstreamSummaries()
	}
	snap.PayloadEnabled = s.PayloadEnabled()
	return snap
}

func (s *RuntimeSource) Logs(since uint64) Logs {
	if s == nil || s.logs == nil {
		return Logs{}
	}
	entries, lastSeq := s.logs.SinceSeq(since)
	return Logs{Logs: entries, LastSeq: lastSeq}
}

// Build constructs a transport snapshot from live in-process state. It must
// copy data out of shared trackers under their locks; callers must not retain
// the live pointers after Build returns.
func Build(version, address, authMode string, startTime time.Time,
	catalog config.Catalog,
	usage *accounting.Aggregator, health *providerhealth.Tracker,
	logs *observability.LogBuffer, recentN int) Snapshot {
	return BuildContext(context.Background(), version, address, authMode, startTime, catalog, usage, health, logs, recentN)
}

func BuildContext(ctx context.Context, version, address, authMode string, startTime time.Time,
	catalog config.Catalog,
	usage *accounting.Aggregator, health *providerhealth.Tracker,
	logs *observability.LogBuffer, recentN int) Snapshot {

	snap := Snapshot{
		Version:           version,
		Address:           address,
		AuthMode:          authMode,
		StartTime:         startTime,
		Now:               time.Now(),
		Providers:         toProviders(catalog.Providers()),
		DisabledProviders: toProviders(catalog.DisabledProviders()),
		Aliases:           toAliases(catalog.Aliases()),
	}
	if health != nil {
		snap.Health = health.SnapshotContext(ctx)
	}
	if usage != nil {
		snap.Usage = usage.Summaries()
		snap.Recent = usage.Recent(recentN)
	}
	if logs != nil {
		entries, lastSeq := logs.SinceSeq(0)
		snap.Logs = entries
		snap.LastSeq = lastSeq
	}
	return snap
}

func toProviders(in []config.Provider) []Provider {
	if len(in) == 0 {
		return nil
	}
	out := make([]Provider, len(in))
	for i, p := range in {
		models := make([]ModelPrice, 0, len(p.Models))
		for _, m := range p.Models {
			mp := ModelPrice{Name: m.Name}
			if m.Pricing != nil && m.Pricing.HasRates() {
				inRate := m.Pricing.InputPerMillion
				outRate := m.Pricing.OutputPerMillion
				cachedRate := m.Pricing.CachedPerMillion
				writeRate := m.Pricing.CacheWritePerMillion
				mp.InputPerMillion = &inRate
				mp.OutputPerMillion = &outRate
				mp.CachedPerMillion = &cachedRate
				mp.CacheWritePerMillion = &writeRate
			}
			models = append(models, mp)
		}
		out[i] = Provider{
			Type:        string(p.Type),
			Name:        p.Name,
			DisplayName: p.DisplayName,
			BaseURL:     provider.EffectiveBaseURL(p.Type, p.BaseURL),
			Models:      models,
		}
	}
	return out
}

func toAliases(in []config.Alias) []Alias {
	if len(in) == 0 {
		return nil
	}
	out := make([]Alias, len(in))
	for i, a := range in {
		targets := make([]AliasTarget, len(a.Targets))
		for j, t := range a.Targets {
			targets[j] = AliasTarget{Provider: t.Provider, Model: t.Model}
		}
		out[i] = Alias{
			Name:             a.Name,
			Algorithm:        string(a.Algorithm),
			RetryStatusCodes: a.RetryStatusCodes,
			Targets:          targets,
		}
	}
	return out
}

type AuthenticatedClient struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

func NewClient(baseURL, token string) *AuthenticatedClient {
	return &AuthenticatedClient{
		BaseURL: baseURL,
		Token:   token,
		HTTP:    &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *AuthenticatedClient) authHeader() string {
	return AuthScheme + c.Token
}

func (c *AuthenticatedClient) doGet(ctx context.Context, path string) ([]byte, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set(AuthHeaderName, c.authHeader())
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, err
	}
	return body, resp.StatusCode, nil
}

func (c *AuthenticatedClient) FetchPayloads(ctx context.Context, limit int, errorsOnly bool) (PayloadList, error) {
	if limit <= 0 {
		limit = PayloadListDefault
	}
	if limit > PayloadListMax {
		limit = PayloadListMax
	}
	path := PayloadsPath + "?limit=" + strconv.Itoa(limit)
	if errorsOnly {
		path += "&errors_only=true"
	}
	body, status, err := c.doGet(ctx, path)
	if err != nil {
		return PayloadList{}, err
	}
	if status != http.StatusOK {
		return PayloadList{}, fmt.Errorf("payloads endpoint returned %d: %s", status, bytes.TrimSpace(body))
	}
	var out PayloadList
	if err := json.Unmarshal(body, &out); err != nil {
		return PayloadList{}, fmt.Errorf("decode payloads: %w", err)
	}
	return out, nil
}

func (c *AuthenticatedClient) FetchPayload(ctx context.Context, requestID string) (json.RawMessage, error) {
	body, status, err := c.doGet(ctx, PayloadPathPrefix+requestID)
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, payloadlog.ErrPayloadNotFound
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("payload endpoint returned %d: %s", status, bytes.TrimSpace(body))
	}
	return json.RawMessage(append([]byte(nil), bytes.TrimSpace(body)...)), nil
}

func (c *AuthenticatedClient) FetchBlocks(ctx context.Context) (BlockList, error) {
	body, status, err := c.doGet(ctx, BlocksPath)
	if err != nil {
		return BlockList{}, err
	}
	if status != http.StatusOK {
		return BlockList{}, fmt.Errorf("blocks endpoint returned %d: %s", status, bytes.TrimSpace(body))
	}
	var out BlockList
	if err := json.Unmarshal(body, &out); err != nil {
		return BlockList{}, fmt.Errorf("decode blocks: %w", err)
	}
	return out, nil
}

func BlockDecisionPath(blockID string) string {
	return BlockPathPrefix + blockID + BlockDecisionSuffix
}

func (c *AuthenticatedClient) DecideBlock(ctx context.Context, blockID, action string, shas []string) (BlockDecisionResponse, error) {
	payload, err := json.Marshal(BlockDecisionRequest{Action: action, FindingSHAs: shas})
	if err != nil {
		return BlockDecisionResponse{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+BlockDecisionPath(blockID), bytes.NewReader(payload))
	if err != nil {
		return BlockDecisionResponse{}, err
	}
	req.Header.Set(AuthHeaderName, c.authHeader())
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return BlockDecisionResponse{}, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return BlockDecisionResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return BlockDecisionResponse{}, fmt.Errorf("block decision endpoint returned %d: %s", resp.StatusCode, bytes.TrimSpace(body))
	}
	var out BlockDecisionResponse
	if err := json.Unmarshal(body, &out); err != nil {
		return BlockDecisionResponse{}, fmt.Errorf("decode block decision: %w", err)
	}
	return out, nil
}

func (c *AuthenticatedClient) FetchBlock(ctx context.Context, blockID string) (BlockCapture, error) {
	body, status, err := c.doGet(ctx, BlockPathPrefix+blockID)
	if err != nil {
		return BlockCapture{}, err
	}
	if status == http.StatusNotFound {
		return BlockCapture{}, fmt.Errorf("block %q not found (expired or already consumed)", blockID)
	}
	if status != http.StatusOK {
		return BlockCapture{}, fmt.Errorf("block endpoint returned %d: %s", status, bytes.TrimSpace(body))
	}
	var out BlockCapture
	if err := json.Unmarshal(body, &out); err != nil {
		return BlockCapture{}, fmt.Errorf("decode block: %w", err)
	}
	return out, nil
}

// TokenFilePath returns the canonical location of the persisted dashboard
// token. The serve process writes a freshly-minted secret here when the
// config declares a dashboard block without a token, and publishes the
// carried-over secret here when a reload drops a previously declared token;
// the dashboard command reads from this path to authenticate to a running
// server.
func TokenFilePath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aiproxy", "dashboard.token")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("aiproxy", "dashboard.token")
	}
	return filepath.Join(home, ".config", "aiproxy", "dashboard.token")
}

// MintToken generates a 32-byte random hex token. It is used by the serve
// process when the dashboard block is declared without a token.
func MintToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate dashboard token: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// PersistToken writes the given token to TokenFilePath() so the dashboard
// command can read it. The parent directory is created if missing. It is
// used for freshly-minted secrets and for publishing a carried-over secret
// when a reload drops a previously declared token.
func PersistToken(token string) error {
	path := TokenFilePath()
	if err := filestore.WriteFile(path, []byte(token+"\n"), 0o600, filestore.Options{DirMode: 0o700, Secret: true}); err != nil {
		return fmt.Errorf("write token file: %w", err)
	}
	return nil
}

// LoadToken reads the persisted dashboard token. Returns os.ErrNotExist-style
// errors verbatim when the file is missing.
func LoadToken() (string, error) {
	data, err := os.ReadFile(TokenFilePath())
	if err != nil {
		return "", err
	}
	out := strings.TrimRightFunc(string(data), func(r rune) bool {
		return r == '\n' || r == '\r' || r == ' ' || r == '\t'
	})
	out = strings.TrimLeftFunc(out, func(r rune) bool {
		return r == ' ' || r == '\t'
	})
	if out == "" {
		return "", errors.New("dashboard token file is empty")
	}
	return out, nil
}
