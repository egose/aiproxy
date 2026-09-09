package dashrpc

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"net/http"
	"strings"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/filestore"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
)

const (
	SnapshotPath   = "/_internal/dashboard/snapshot"
	LogsPath       = "/_internal/dashboard/logs"
	AuthHeaderName = "Authorization"
	AuthScheme     = "Bearer "
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
	Usage             []Usage                  `json:"usage"`
	ProviderStats     []ProviderStat           `json:"provider_stats,omitempty"`
	Upstream          []UpstreamUsage          `json:"upstream,omitempty"`
	Recent            []Recent                 `json:"recent"`
	Logs              []observability.LogEntry `json:"logs"`
	LastSeq           uint64                   `json:"last_seq"`
}

type CooldownInfo struct {
	Alias       string `json:"alias"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	RemainingMs int64  `json:"remaining_ms"`
}

type Provider struct {
	Type        string   `json:"type"`
	Name        string   `json:"name"`
	DisplayName string   `json:"display_name,omitempty"`
	BaseURL     string   `json:"base_url,omitempty"`
	Models      []string `json:"models"`
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

type Source interface {
	Enabled() bool
	Token() string
	Snapshot(context.Context, int) Snapshot
	Logs(uint64) Logs
}

type RuntimeSource struct {
	dashboard config.Dashboard
	version   string
	address   string
	authMode  string
	startTime time.Time
	catalog   config.Catalog
	usage     *accounting.Aggregator
	health    *providerhealth.Tracker
	logs      *observability.LogBuffer
	cooldowns func() []CooldownInfo
}

func (s *RuntimeSource) SetCooldownSource(fn func() []CooldownInfo) {
	if s == nil {
		return
	}
	s.cooldowns = fn
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
	if s.usage != nil {
		snap.ProviderStats = s.usage.ProviderSummaries()
		snap.Upstream = s.usage.UpstreamSummaries()
	}
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
		models := make([]string, 0, len(p.Models))
		for _, m := range p.Models {
			models = append(models, m.Name)
		}
		out[i] = Provider{
			Type:        string(p.Type),
			Name:        p.Name,
			DisplayName: p.DisplayName,
			BaseURL:     p.BaseURL,
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
