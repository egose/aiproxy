package dashrpc

import (
	"net/http"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
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
	Usage             []Usage                  `json:"usage"`
	Recent            []Recent                 `json:"recent"`
	Logs              []observability.LogEntry `json:"logs"`
	LastSeq           uint64                   `json:"last_seq"`
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

// Build constructs a transport snapshot from live in-process state. It must
// copy data out of shared trackers under their locks; callers must not retain
// the live pointers after Build returns.
func Build(version, address, authMode string, startTime time.Time,
	providers, disabledProviders []config.Provider, aliases []config.Alias,
	usage *accounting.Aggregator, health *providerhealth.Tracker,
	logs *observability.LogBuffer, recentN int) Snapshot {

	snap := Snapshot{
		Version:           version,
		Address:           address,
		AuthMode:          authMode,
		StartTime:         startTime,
		Now:               time.Now(),
		Providers:         toProviders(providers),
		DisabledProviders: toProviders(disabledProviders),
		Aliases:           toAliases(aliases),
	}
	if health != nil {
		snap.Health = health.Snapshot()
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
