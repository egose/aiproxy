package config

func EffectiveCapabilities(providerType ProviderType, model Model) []Capability {
	if len(model.Capabilities) > 0 {
		out := make([]Capability, len(model.Capabilities))
		copy(out, model.Capabilities)
		return out
	}
	if IsOpenCodeProviderType(providerType) {
		return defaultCapabilitiesForOpenCodeProtocol(model.Protocol)
	}
	return defaultCapabilitiesForProvider(providerType)
}

func IsOpenCodeProviderType(providerType ProviderType) bool {
	return providerType == ProviderTypeOpenCodeZen || providerType == ProviderTypeOpenCodeGo
}

func OpenCodeProtocolCapabilities(protocol ModelProtocol) []Capability {
	switch protocol {
	case ModelProtocolChat:
		return []Capability{CapabilityChat}
	case ModelProtocolResponses:
		return []Capability{CapabilityResponses}
	case ModelProtocolMessages, ModelProtocolGemini:
		return []Capability{CapabilityChat, CapabilityResponses}
	default:
		return nil
	}
}

func defaultCapabilitiesForOpenCodeProtocol(protocol ModelProtocol) []Capability {
	caps := OpenCodeProtocolCapabilities(protocol)
	out := make([]Capability, len(caps))
	copy(out, caps)
	return out
}

func ProviderTypes() []ProviderType {
	out := make([]ProviderType, len(providerTypeOrder))
	copy(out, providerTypeOrder)
	return out
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
	policy, ok := providerTypePolicies[providerType]
	if !ok {
		return nil
	}
	out := make([]Capability, len(policy.defaultCapabilities))
	copy(out, policy.defaultCapabilities)
	return out
}

type providerTypePolicy struct {
	defaultCapabilities   []Capability
	supportedCapabilities []Capability
	requiresBaseURL       bool
}

var providerTypeOrder = []ProviderType{
	ProviderTypeOpenAI,
	ProviderTypeOpenAICompatible,
	ProviderTypeAnthropic,
	ProviderTypeGemini,
	ProviderTypeOpenCodeZen,
	ProviderTypeOpenCodeGo,
	ProviderTypeGitHubCopilot,
}

var providerTypePolicies = map[ProviderType]providerTypePolicy{
	ProviderTypeOpenAI: {
		defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings},
		supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech},
	},
	ProviderTypeOpenAICompatible: {
		defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings},
		supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech},
		requiresBaseURL:       true,
	},
	ProviderTypeAnthropic: {
		defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses},
		supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses},
	},
	ProviderTypeGemini: {
		defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses},
		supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings},
	},
	ProviderTypeOpenCodeZen: {
		defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses},
		supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses},
	},
	ProviderTypeOpenCodeGo: {
		defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses},
		supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses},
	},
	ProviderTypeGitHubCopilot: {
		defaultCapabilities:   []Capability{CapabilityChat},
		supportedCapabilities: []Capability{CapabilityChat},
	},
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
