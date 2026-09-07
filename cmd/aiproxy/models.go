package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"charm.land/huh/v2"
	"github.com/egose/aiproxy/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

func newModelsCommand() *cobra.Command {
	var cfgPath string
	var providerName string
	cmd := &cobra.Command{
		Use:   "models",
		Short: "List models for a provider from the config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runModels(cfgPath, providerName, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().StringVarP(&cfgPath, "config", "c", defaultConfigPath(), "path to config file")
	cmd.Flags().StringVarP(&providerName, "provider", "p", "", "provider name (skips the interactive prompt)")
	return cmd
}

func runModels(cfgPath, providerName string, stdout, stderr io.Writer) error {
	rt, err := config.LoadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("config: %w", err)
	}
	providers := rt.Catalog.Providers()
	if len(providers) == 0 {
		fmt.Fprintln(stderr, "no enabled providers in config")
		return fmt.Errorf("no enabled providers in config")
	}
	sort.Slice(providers, func(i, j int) bool { return providers[i].Name < providers[j].Name })

	provider, err := resolveModelsProvider(providers, providerName, stdout)
	if err != nil {
		fmt.Fprintln(stderr, err.Error())
		return err
	}

	fmt.Fprintf(stdout, "Provider %q (%s) — %d model(s):\n", provider.Name, provider.Type, len(provider.Models))
	for _, m := range provider.Models {
		fmt.Fprintf(stdout, "  %s\n", formatPublicModel(provider, m))
	}
	return nil
}

func resolveModelsProvider(providers []config.Provider, name string, stdout io.Writer) (config.Provider, error) {
	if name != "" {
		for _, p := range providers {
			if p.Name == name {
				return p, nil
			}
		}
		return config.Provider{}, fmt.Errorf("unknown provider %q (available: %s)", name, providerNames(providers))
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return config.Provider{}, fmt.Errorf("no provider given and stdin is not a terminal (use --provider with one of: %s)", providerNames(providers))
	}
	options := make([]huh.Option[string], 0, len(providers))
	for _, p := range providers {
		label := p.Name
		if p.DisplayName != "" {
			label += " — " + p.DisplayName
		}
		label += fmt.Sprintf(" (%s, %d model(s))", p.Type, len(p.Models))
		options = append(options, huh.NewOption(label, p.Name))
	}
	var selected string
	if err := huh.NewSelect[string]().Title("Provider").Options(options...).Value(&selected).Run(); err != nil {
		return config.Provider{}, fmt.Errorf("provider prompt: %w", err)
	}
	for _, p := range providers {
		if p.Name == selected {
			return p, nil
		}
	}
	return config.Provider{}, fmt.Errorf("unknown provider %q", selected)
}

func providerNames(providers []config.Provider) string {
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name)
	}
	return strings.Join(names, ", ")
}

func formatPublicModel(provider config.Provider, model config.Model) string {
	var b strings.Builder
	b.WriteString(provider.Name + "/" + model.Name)
	if model.DisplayName != "" {
		b.WriteString(" — " + model.DisplayName)
	}
	caps := make([]string, 0, len(config.EffectiveCapabilities(provider.Type, model)))
	for _, c := range config.EffectiveCapabilities(provider.Type, model) {
		caps = append(caps, string(c))
	}
	b.WriteString(" [" + strings.Join(caps, ", ") + "]")
	if model.Protocol != "" {
		b.WriteString(" (protocol: " + string(model.Protocol) + ")")
	}
	if model.UpstreamName != "" && model.UpstreamName != model.Name {
		b.WriteString(" (upstream: " + model.UpstreamName + ")")
	}
	return b.String()
}
