package dashboard

import (
	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
)

// remoteUsage wraps a dashrpc snapshot's usage state behind the dashboard's
// UsageViewer interface. The dashboard TUI reads recent events for p95
// latency calculations only; the JSON snapshot already carries them.
type remoteUsage struct {
	summaries []accounting.Summary
	recent    []accounting.Event
}

func (u *remoteUsage) Summaries() []accounting.Summary { return u.summaries }
func (u *remoteUsage) Recent(int) []accounting.Event   { return u.recent }

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
	return l.entries[len(l.entries)-n:]
}

// SnapshotFromTransport converts a dashrpc.Snapshot into a dashboard
// RuntimeSnapshot suitable for the existing TUI renderer.
func SnapshotFromTransport(s dashrpc.Snapshot) *RuntimeSnapshot {
	var providers, disabled []config.Provider
	for _, p := range s.Providers {
		providers = append(providers, config.Provider{
			Type:        config.ProviderType(p.Type),
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Models:      configModels(p.Models),
		})
	}
	for _, p := range s.DisabledProviders {
		disabled = append(disabled, config.Provider{
			Type:        config.ProviderType(p.Type),
			Name:        p.Name,
			DisplayName: p.DisplayName,
			Models:      configModels(p.Models),
		})
	}
	var aliases []config.Alias
	for _, a := range s.Aliases {
		var targets []config.AliasTarget
		for _, t := range a.Targets {
			targets = append(targets, config.AliasTarget{Provider: t.Provider, Model: t.Model})
		}
		aliases = append(aliases, config.Alias{
			Name:             a.Name,
			Algorithm:        config.Algorithm(a.Algorithm),
			RetryStatusCodes: a.RetryStatusCodes,
			Targets:          targets,
		})
	}
	return &RuntimeSnapshot{
		Version:           s.Version,
		Address:           s.Address,
		AuthMode:          s.AuthMode,
		StartTime:         s.StartTime,
		Providers:         providers,
		DisabledProviders: disabled,
		Aliases:           aliases,
		Usage:             &remoteUsage{summaries: s.Usage, recent: s.Recent},
		Health:            &remoteHealth{states: s.Health},
		Logs:              &remoteLogs{entries: s.Logs},
	}
}

// configModels converts a list of model names into the minimal config.Model
// shape the dashboard renderer reads (it only inspects m.Name).
func configModels(names []string) []config.Model {
	if len(names) == 0 {
		return nil
	}
	out := make([]config.Model, len(names))
	for i, n := range names {
		out[i] = config.Model{Name: n}
	}
	return out
}
