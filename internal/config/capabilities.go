package config

func EffectiveCapabilities(providerType ProviderType, model Model) []Capability {
	if len(model.Capabilities) > 0 {
		out := make([]Capability, len(model.Capabilities))
		copy(out, model.Capabilities)
		return out
	}
	return defaultCapabilitiesForProvider(providerType)
}

func AliasEffectiveCapabilities(alias Alias, providers map[string]Provider) []Capability {
	if len(alias.Targets) == 0 {
		return nil
	}
	var intersection []Capability
	for i, target := range alias.Targets {
		provider, ok := providers[target.Provider]
		if !ok {
			return nil
		}
		model, ok := provider.ModelByName[target.Model]
		if !ok {
			return nil
		}
		caps := EffectiveCapabilities(provider.Type, model)
		if i == 0 {
			intersection = append(intersection, caps...)
			continue
		}
		intersection = intersectCapabilities(intersection, caps)
		if len(intersection) == 0 {
			return nil
		}
	}
	return intersection
}

func HasCapability(caps []Capability, want Capability) bool {
	for _, c := range caps {
		if c == want {
			return true
		}
	}
	return false
}

func defaultCapabilitiesForProvider(providerType ProviderType) []Capability {
	switch providerType {
	case ProviderTypeOpenAI, ProviderTypeOpenAICompatible:
		return []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings}
	case ProviderTypeAnthropic:
		return []Capability{CapabilityChat, CapabilityResponses}
	case ProviderTypeGemini:
		return []Capability{CapabilityChat, CapabilityResponses}
	default:
		return nil
	}
}

func intersectCapabilities(left, right []Capability) []Capability {
	if len(left) == 0 || len(right) == 0 {
		return nil
	}
	rightSet := make(map[Capability]bool, len(right))
	for _, c := range right {
		rightSet[c] = true
	}
	out := make([]Capability, 0, len(left))
	for _, c := range left {
		if rightSet[c] {
			out = append(out, c)
		}
	}
	return out
}
