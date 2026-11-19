package config

import (
	"fmt"
	"testing"
)

func TestValidateModelCapabilitiesSharedRules(t *testing.T) {
	for _, providerType := range append(ProviderTypes(), ProviderType("unknown")) {
		for _, protocol := range []ModelProtocol{"", ModelProtocolChat, ModelProtocolResponses, ModelProtocolMessages, ModelProtocolGemini, "unknown"} {
			for i, capabilities := range [][]Capability{
				nil, {CapabilityChat}, {CapabilityResponses}, {CapabilityEmbeddings},
				{CapabilityImages}, {"unknown"}, {CapabilityChat, CapabilityChat},
			} {
				t.Run(fmt.Sprintf("%s/%s/%d", providerType, protocol, i), func(t *testing.T) {
					model := Model{Name: "m", UpstreamName: "m", Protocol: protocol, Capabilities: capabilities}
					p := Provider{Name: "fixture", Type: providerType, BaseURL: "https://api.example.com", Models: []Model{model}}
					capabilityErr := ValidateModelCapabilities(p.Type, p.Name, model)
					for _, err := range []error{ValidateDynamicProvider(p), validateProviders([]Provider{p}, false)} {
						if (capabilityErr == nil) != (err == nil) || (err != nil && err.Error() != capabilityErr.Error()) {
							t.Fatalf("shared model validation differs: capabilities=%v provider=%v", capabilityErr, err)
						}
					}
				})
			}
		}
	}
}
