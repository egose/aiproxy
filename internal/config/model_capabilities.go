package config

import "fmt"

func ValidateModelCapabilities(providerType ProviderType, providerName string, model Model) error {
	if _, ok := providerTypePolicies[providerType]; !ok {
		return fmt.Errorf("provider %q: unsupported type %q", providerName, providerType)
	}
	owner := fmt.Sprintf("provider %q: model %q", providerName, model.Name)
	if err := validateModelProtocol(providerType, owner, model.Protocol); err != nil {
		return err
	}
	seen := make(map[Capability]bool, len(model.Capabilities))
	for _, capability := range model.Capabilities {
		if !isValidCapability(capability) {
			return fmt.Errorf("%s has invalid capability %q", owner, capability)
		}
		if err := validateProviderCapability(providerType, model.Protocol, capability); err != nil {
			return fmt.Errorf("%s %w", owner, err)
		}
		if seen[capability] {
			return fmt.Errorf("%s has duplicate capability %q", owner, capability)
		}
		seen[capability] = true
	}
	return nil
}

func validateProviderCapability(providerType ProviderType, protocol ModelProtocol, capability Capability) error {
	if !providerSupportsCapability(providerType, capability) {
		return fmt.Errorf("capability %q is not supported by provider type %q", capability, providerType)
	}
	if IsOpenCodeProviderType(providerType) && !HasCapability(OpenCodeProtocolCapabilities(protocol), capability) {
		return fmt.Errorf("capability %q is not served by protocol %q", capability, protocol)
	}
	return nil
}
