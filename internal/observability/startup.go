package observability

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/egose/aiproxy/internal/config"
)

func LogStartup(logger *slog.Logger, rt *config.Runtime) {
	logger.Info("config loaded",
		"providers", len(rt.Providers),
		"disabled_providers", len(rt.DisabledProviders),
		"aliases", len(rt.Aliases),
		"auth_mode", rt.Auth.Mode,
	)
	for _, p := range rt.DisabledProviders {
		logger.Warn("provider skipped because api key is empty",
			"name", p.Name,
			"type", p.Type,
			"display_name", p.DisplayName,
			"models", len(p.Models),
		)
	}
	for _, p := range rt.Providers {
		logger.Info("provider configured",
			"name", p.Name,
			"type", p.Type,
			"display_name", p.DisplayName,
			"models", len(p.Models),
		)
	}
	for _, a := range rt.Aliases {
		targets := make([]string, 0, len(a.Targets))
		for _, t := range a.Targets {
			targets = append(targets, t.Provider+"/"+t.Model)
		}
		logger.Info("alias configured",
			"name", a.Name,
			"algorithm", a.Algorithm,
			"targets", targets,
			"target_count", len(a.Targets),
			"capabilities", capabilityStrings(config.AliasEffectiveCapabilities(a, rt.ProviderByName)),
		)
	}
	logger.Info(StartupSummary(rt))
}

func StartupSummary(rt *config.Runtime) string {
	var b strings.Builder
	b.WriteString("startup summary\n")
	fmt.Fprintf(&b, "  enabled providers: %d\n", len(rt.Providers))
	for _, p := range rt.Providers {
		display := p.DisplayName
		if display == "" {
			display = p.Name
		}
		fmt.Fprintf(&b, "    - %s (%s) models=%d display=%q\n", p.Name, p.Type, len(p.Models), display)
	}
	fmt.Fprintf(&b, "  skipped providers: %d\n", len(rt.DisabledProviders))
	for _, p := range rt.DisabledProviders {
		display := p.DisplayName
		if display == "" {
			display = p.Name
		}
		fmt.Fprintf(&b, "    - %s (%s) models=%d display=%q reason=%q\n", p.Name, p.Type, len(p.Models), display, "empty api key")
	}
	fmt.Fprintf(&b, "  aliases: %d\n", len(rt.Aliases))
	for _, a := range rt.Aliases {
		targets := make([]string, 0, len(a.Targets))
		for _, t := range a.Targets {
			targets = append(targets, t.Provider+"/"+t.Model)
		}
		fmt.Fprintf(&b, "    - %s algorithm=%s targets=%d capabilities=%v pool=%v\n", a.Name, a.Algorithm, len(a.Targets), capabilityStrings(config.AliasEffectiveCapabilities(a, rt.ProviderByName)), targets)
	}
	return strings.TrimRight(b.String(), "\n")
}

func capabilityStrings(caps []config.Capability) []string {
	if len(caps) == 0 {
		return nil
	}
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, string(c))
	}
	return out
}
