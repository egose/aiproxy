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

func BuildModelCatalog(rt *config.Runtime) []ModelCard {
	cards := make([]ModelCard, 0, countCatalogSize(rt))
	for _, provider := range rt.Providers {
		for _, model := range provider.Models {
			caps := config.EffectiveCapabilities(provider.Type, model)
			cards = append(cards, ModelCard{
				ID:           provider.Name + "/" + model.Name,
				Object:       "model",
				Created:      0,
				OwnedBy:      provider.Name,
				DisplayName:  modelDisplayName(provider, model),
				ProviderType: string(provider.Type),
				Capabilities: capabilityStrings(caps),
			})
		}
	}
	for _, alias := range rt.Aliases {
		caps := config.AliasEffectiveCapabilities(alias, rt.ProviderByName)
		cards = append(cards, ModelCard{
			ID:           "alias/" + alias.Name,
			Object:       "model",
			Created:      0,
			OwnedBy:      "alias",
			Capabilities: capabilityStrings(caps),
			AliasTargets: buildAliasTargets(alias, rt.ProviderByName),
		})
	}
	return cards
}

func countCatalogSize(rt *config.Runtime) int {
	total := len(rt.Aliases)
	for _, provider := range rt.Providers {
		total += len(provider.Models)
	}
	return total
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

func buildAliasTargets(alias config.Alias, providers map[string]config.Provider) []AliasTargetCard {
	if len(alias.Targets) == 0 {
		return nil
	}
	out := make([]AliasTargetCard, 0, len(alias.Targets))
	for _, target := range alias.Targets {
		card := AliasTargetCard{
			Provider: target.Provider,
			Model:    target.Model,
		}
		if provider, ok := providers[target.Provider]; ok {
			if model, ok := provider.ModelByName[target.Model]; ok {
				card.DisplayName = modelDisplayName(provider, model)
			}
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
