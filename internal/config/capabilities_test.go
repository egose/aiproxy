package config

import "testing"

func TestProviderTypePoliciesCoverCapabilityMatrix(t *testing.T) {
	tests := []struct {
		providerType          ProviderType
		defaultCapabilities   []Capability
		supportedCapabilities []Capability
		requiresBaseURL       bool
	}{
		{
			providerType:          ProviderTypeOpenAI,
			defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings},
			supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech},
		},
		{
			providerType:          ProviderTypeOpenAICompatible,
			defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings},
			supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech},
			requiresBaseURL:       true,
		},
		{
			providerType:          ProviderTypeAnthropic,
			defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses},
			supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses},
		},
		{
			providerType:          ProviderTypeGemini,
			defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses},
			supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings},
		},
		{
			providerType:          ProviderTypeOpenCodeZen,
			defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses},
			supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses},
		},
		{
			providerType:          ProviderTypeOpenCodeGo,
			defaultCapabilities:   []Capability{CapabilityChat, CapabilityResponses},
			supportedCapabilities: []Capability{CapabilityChat, CapabilityResponses},
		},
		{
			providerType:          ProviderTypeGitHubCopilot,
			defaultCapabilities:   []Capability{CapabilityChat},
			supportedCapabilities: []Capability{CapabilityChat},
		},
	}

	if got := ProviderTypes(); len(got) != len(tests) {
		t.Fatalf("ProviderTypes length = %d, want %d", len(got), len(tests))
	}
	if len(providerTypePolicies) != len(tests) {
		t.Fatalf("providerTypePolicies length = %d, want %d", len(providerTypePolicies), len(tests))
	}

	for i, tt := range tests {
		if got := ProviderTypes()[i]; got != tt.providerType {
			t.Fatalf("ProviderTypes()[%d] = %q, want %q", i, got, tt.providerType)
		}
		policy, ok := providerTypePolicies[tt.providerType]
		if !ok {
			t.Fatalf("providerTypePolicies[%q] missing", tt.providerType)
		}
		if policy.requiresBaseURL != tt.requiresBaseURL {
			t.Fatalf("%q requiresBaseURL = %v, want %v", tt.providerType, policy.requiresBaseURL, tt.requiresBaseURL)
		}
		assertCapabilities(t, defaultCapabilitiesForProvider(tt.providerType), tt.defaultCapabilities)
		for _, capability := range tt.supportedCapabilities {
			if !providerSupportsCapability(tt.providerType, capability) {
				t.Fatalf("%q should support capability %q", tt.providerType, capability)
			}
		}
		for _, capability := range allCapabilitiesForTest() {
			if providerSupportsCapability(tt.providerType, capability) != HasCapability(tt.supportedCapabilities, capability) {
				t.Fatalf("%q support for %q does not match policy", tt.providerType, capability)
			}
		}
	}
}

func TestDefaultCapabilitiesReturnsCopy(t *testing.T) {
	caps := defaultCapabilitiesForProvider(ProviderTypeOpenAI)
	caps[0] = CapabilityAudioSpeech
	assertCapabilities(t, defaultCapabilitiesForProvider(ProviderTypeOpenAI), []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings})
}

func allCapabilitiesForTest() []Capability {
	return []Capability{CapabilityChat, CapabilityResponses, CapabilityEmbeddings, CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech}
}

func assertCapabilities(t *testing.T, got, want []Capability) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("capabilities = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("capabilities = %v, want %v", got, want)
		}
	}
}
