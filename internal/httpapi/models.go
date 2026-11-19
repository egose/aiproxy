package httpapi

import "github.com/egose/aiproxy/internal/config"

type ModelCard struct {
	ID           string            `json:"id"`
	Object       string            `json:"object"`
	Created      int64             `json:"created"`
	OwnedBy      string            `json:"owned_by"`
	DisplayName  string            `json:"display_name,omitempty"`
	ProviderType string            `json:"provider_type,omitempty"`
	Capabilities []string          `json:"capabilities,omitempty"`
	AliasTargets []AliasTargetCard `json:"alias_targets,omitempty"`
}

type AliasTargetCard struct {
	Provider    string `json:"provider"`
	Model       string `json:"model"`
	DisplayName string `json:"display_name,omitempty"`
}

func buildModelCatalog(catalog config.Catalog, allow func(string) bool) []ModelCard {
	var cards []ModelCard
	for _, provider := range catalog.Providers() {
		for _, model := range provider.Models {
			name := provider.Name + "/" + model.Name
			if !allow(name) {
				continue
			}
			caps := config.EffectiveCapabilities(provider.Type, model)
			cards = append(cards, ModelCard{
				ID:           name,
				Object:       "model",
				Created:      0,
				OwnedBy:      provider.Name,
				DisplayName:  modelDisplayName(provider, model),
				ProviderType: string(provider.Type),
				Capabilities: capabilityStrings(caps),
			})
		}
	}
	for _, alias := range catalog.Aliases() {
		name := "alias/" + alias.Name
		if !allow(name) {
			continue
		}
		caps := catalog.AliasEffectiveCapabilities(alias)
		cards = append(cards, ModelCard{
			ID:           name,
			Object:       "model",
			Created:      0,
			OwnedBy:      "alias",
			Capabilities: capabilityStrings(caps),
			AliasTargets: buildAliasTargets(alias, catalog),
		})
	}
	return cards
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

func buildAliasTargets(alias config.Alias, catalog config.Catalog) []AliasTargetCard {
	if len(alias.Targets) == 0 {
		return nil
	}
	out := make([]AliasTargetCard, 0, len(alias.Targets))
	for _, target := range alias.Targets {
		card := AliasTargetCard{
			Provider: target.Provider,
			Model:    target.Model,
		}
		if provider, model, ok := catalog.Model(target.Provider, target.Model); ok {
			card.DisplayName = modelDisplayName(provider, model)
		}
		out = append(out, card)
	}
	return out
}

func modelDisplayName(provider config.Provider, model config.Model) string {
	if model.DisplayName != "" {
		return model.DisplayName
	}
	if provider.DisplayName != "" {
		return provider.DisplayName + " / " + model.Name
	}
	return model.Name
}
