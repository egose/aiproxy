package config

import "fmt"

func ValidateModelCapabilities(providerType ProviderType, providerName string, model Model) error {
	if _, ok := providerTypePolicies[providerType]; !ok {
		return fmt.Errorf("provider %q: unsupported type %q", providerName, providerType)
	}
	if err := validateModelProtocol(providerType, providerName, model); err != nil {
		return err
	}
	served := OpenCodeProtocolCapabilities(model.Protocol)
	seen := make(map[Capability]bool, len(model.Capabilities))
	for _, capability := range model.Capabilities {
		if !isValidCapability(capability) {
			return fmt.Errorf("provider %q: model %q has invalid capability %q", providerName, model.Name, capability)
		}
		if !providerSupportsCapability(providerType, capability) {
			return fmt.Errorf("provider %q: model %q capability %q is not supported by provider type %q", providerName, model.Name, capability, providerType)
		}
		if IsOpenCodeProviderType(providerType) && !HasCapability(served, capability) {
			return fmt.Errorf("provider %q: model %q capability %q is not served by protocol %q", providerName, model.Name, capability, model.Protocol)
		}
		if seen[capability] {
			return fmt.Errorf("provider %q: model %q has duplicate capability %q", providerName, model.Name, capability)
		}
		seen[capability] = true
	}
	return nil
}
