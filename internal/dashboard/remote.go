package dashboard

import (
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
)

// remoteUsage wraps a dashrpc snapshot's usage state behind the dashboard's
// UsageViewer interface, including bounded recent completion metadata.
type remoteUsage struct {
	summaries              []accounting.Summary
	recent                 []accounting.Event
	providers              []accounting.ProviderSummary
	upstream               []accounting.UpstreamSummary
	rates                  *accounting.RateSnapshot
	billing                *accounting.BillingSnapshot
	providerStatsAvailable bool
}

func (u *remoteUsage) RateSnapshot() *accounting.RateSnapshot       { return u.rates }
func (u *remoteUsage) BillingSnapshot() *accounting.BillingSnapshot { return u.billing }

func (u *remoteUsage) Summaries() []accounting.Summary { return u.summaries }
func (u *remoteUsage) Recent(n int) []accounting.Event {
	if n <= 0 {
		return nil
	}
	if n > len(u.recent) {
		n = len(u.recent)
	}
	return u.recent[len(u.recent)-n:]
}
func (u *remoteUsage) ProviderSummaries() []accounting.ProviderSummary {
	return u.providers
}
func (u *remoteUsage) UpstreamSummaries() []accounting.UpstreamSummary {
	return u.upstream
}

type remoteHealth struct {
	states map[string]bool
}

func (h *remoteHealth) Snapshot() map[string]bool {
	if h.states == nil {
		return map[string]bool{}
	}
	out := make(map[string]bool, len(h.states))
	for k, v := range h.states {
		out[k] = v
	}
	return out
}

type remoteLogs struct {
	entries []observability.LogEntry
}

func (l *remoteLogs) Since(n int) []observability.LogEntry {
	if n <= 0 || len(l.entries) == 0 {
		return nil
	}
	if n > len(l.entries) {
		n = len(l.entries)
	}
	out := make([]observability.LogEntry, n)
	copy(out, l.entries[len(l.entries)-n:])
	return out
}

// SnapshotFromTransport converts a dashrpc.Snapshot into a dashboard
// RuntimeSnapshot suitable for the existing TUI renderer.
func SnapshotFromTransport(s dashrpc.Snapshot) *RuntimeSnapshot {
	usage := s.Usage
	if s.Billing != nil {
		usage = s.Billing.Usage
	}
	cooldowns := make([]CooldownEntry, 0, len(s.Cooldowns))
	for _, c := range s.Cooldowns {
		cooldowns = append(cooldowns, CooldownEntry{
			Alias:       c.Alias,
			Provider:    c.Provider,
			Model:       c.Model,
			RemainingMs: c.RemainingMs,
		})
	}
	var providers, disabled []config.Provider
	metadata := make(map[string]*dashrpc.ProviderDiagnostics)
	modelMetadata := make(map[string]*dashrpc.ModelDetails)
	for _, group := range [][]dashrpc.Provider{s.Providers, s.DisabledProviders} {
		for _, p := range group {
			metadata[p.Name] = p.Diagnostics
			for _, model := range p.Models {
				modelMetadata[p.Name+"/"+model.Name] = model.Details
			}
		}
	}
	for _, p := range s.Providers {
		providers = append(providers, config.Provider{
			Type:        config.ProviderType(p.Type),
			Name:        p.Name,
			DisplayName: p.DisplayName,
			BaseURL:     p.BaseURL,
			Models:      configModels(p.Models),
		})
	}
	for _, p := range s.DisabledProviders {
		disabled = append(disabled, config.Provider{
			Type:        config.ProviderType(p.Type),
			Name:        p.Name,
			DisplayName: p.DisplayName,
			BaseURL:     p.BaseURL,
			Models:      configModels(p.Models),
		})
	}
	var aliases []config.Alias
	affinity := make(map[string]*dashrpc.Affinity)
	for _, a := range s.Aliases {
		affinity[a.Name] = a.SessionAffinity
		var session *config.SessionAffinity
		if a.SessionAffinity != nil && a.SessionAffinity.Enabled {
			session = &config.SessionAffinity{Headers: append([]string(nil), a.SessionAffinity.Headers...)}
		}
		var targets []config.AliasTarget
		for _, t := range a.Targets {
			targets = append(targets, config.AliasTarget{Provider: t.Provider, Model: t.Model})
		}
		aliases = append(aliases, config.Alias{
			Name:             a.Name,
			Algorithm:        config.Algorithm(a.Algorithm),
			RetryStatusCodes: a.RetryStatusCodes,
			Targets:          targets,
			SessionAffinity:  session,
		})
	}
	return &RuntimeSnapshot{
		Version:           s.Version,
		Address:           s.Address,
		AuthMode:          s.AuthMode,
		StartTime:         s.StartTime,
		SnapshotAt:        s.Now,
		Providers:         providers,
		DisabledProviders: disabled,
		Aliases:           aliases,
		Cooldowns:         cooldowns,
		Healthchecks:      healthchecksFromTransport(s.Healthchecks),
		ProviderMetadata:  metadata,
		ModelMetadata:     modelMetadata,
		AliasAffinity:     affinity,
		Usage: &remoteUsage{summaries: usage, recent: s.Recent, providers: s.ProviderStats, upstream: s.Upstream,
			rates: s.Rates, billing: s.Billing, providerStatsAvailable: s.ProviderStats != nil || s.Billing != nil},
		Health:         &remoteHealth{states: s.Health},
		Logs:           &remoteLogs{entries: s.Logs},
		PayloadEnabled: s.PayloadEnabled,
	}
}

func healthchecksFromTransport(in []dashrpc.HealthcheckStatus) []HealthcheckEntry {
	if len(in) == 0 {
		return nil
	}
	out := make([]HealthcheckEntry, 0, len(in))
	for _, h := range in {
		out = append(out, HealthcheckEntry{
			Provider:    h.Provider,
			Configured:  h.Configured,
			Checked:     h.Checked,
			Healthy:     h.Healthy,
			StatusCode:  h.StatusCode,
			Message:     dashrpc.DiagnosticReason(h.Message),
			Path:        dashrpc.DiagnosticURL(h.Path),
			LastChecked: h.LastChecked,
		})
	}
	return out
}

// configModels retains diagnostic metadata without reconstructing credentials.
func configModels(names []dashrpc.ModelPrice) []config.Model {
	if len(names) == 0 {
		return nil
	}
	out := make([]config.Model, len(names))
	for i, n := range names {
		out[i] = config.Model{Name: n.Name, Pricing: modelPricing(n)}
		if d := n.Details; d != nil {
			out[i].DisplayName, out[i].UpstreamName = d.DisplayName, d.UpstreamName
			out[i].Protocol = config.ModelProtocol(d.Protocol)
			out[i].Capabilities = append([]config.Capability(nil), d.Capabilities...)
		}
	}
	return out
}

func modelPricing(mp dashrpc.ModelPrice) *config.ModelPricing {
	if mp.InputPerMillion == nil && mp.OutputPerMillion == nil && mp.CachedPerMillion == nil && mp.CacheWritePerMillion == nil {
		return nil
	}
	out := &config.ModelPricing{}
	if mp.InputPerMillion != nil {
		out.InputPerMillion = *mp.InputPerMillion
	}
	if mp.OutputPerMillion != nil {
		out.OutputPerMillion = *mp.OutputPerMillion
	}
	if mp.CachedPerMillion != nil {
		out.CachedPerMillion = *mp.CachedPerMillion
	}
	if mp.CacheWritePerMillion != nil {
		out.CacheWritePerMillion = *mp.CacheWritePerMillion
	}
	if !out.HasRates() {
		return nil
	}
	return out
}
