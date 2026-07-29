package config

import "fmt"

func Validate(rt *Runtime) error {
	if err := validateListener(rt.Listener); err != nil {
		return err
	}
	if err := validateAuth(rt.Auth); err != nil {
		return err
	}
	if err := validateProviderHealth(rt.ProviderHealth); err != nil {
		return err
	}
	if err := validateProviders(rt.Providers); err != nil {
		return err
	}
	if err := validateAliases(rt.Aliases, rt.ProviderByName); err != nil {
		return err
	}
	return nil
}

func validateProviderHealth(h ProviderHealth) error {
	if h.RedisURL == "" {
		return nil
	}
	if h.Cooldown < 0 {
		return fmt.Errorf("provider_health: cooldown must not be negative")
	}
	if h.KeyPrefix == "" {
		return fmt.Errorf("provider_health: key_prefix must not be empty")
	}
	return nil
}

func validateListener(l Listener) error {
	if l.Address == "" {
		return fmt.Errorf("listener.http %q: address is required", l.Name)
	}
	return nil
}

func validateAuth(a Auth) error {
	if a.RateLimit != nil {
		if a.RateLimit.RequestsPerMinute <= 0 {
			return fmt.Errorf("auth %q: rate_limit.requests_per_minute must be greater than zero", a.Name)
		}
		if a.RateLimit.Burst <= 0 {
			return fmt.Errorf("auth %q: rate_limit.burst must be greater than zero", a.Name)
		}
	}
	switch a.Mode {
	case AuthModeNone:
		if len(a.Clients) > 0 {
			return fmt.Errorf("auth %q: clients cannot be defined when mode is none", a.Name)
		}
	case AuthModeBearerStatic:
		tokens := make(map[string]string)
		for _, c := range a.Clients {
			if c.Token == "" {
				return fmt.Errorf("auth %q: client %q has empty token", a.Name, c.Name)
			}
			for _, model := range c.AllowedModels {
				if model == "" {
					return fmt.Errorf("auth %q: client %q has empty allowed_models entry", a.Name, c.Name)
				}
			}
			if existing, dup := tokens[c.Token]; dup {
				return fmt.Errorf("auth %q: clients %q and %q share the same token", a.Name, existing, c.Name)
			}
			tokens[c.Token] = c.Name
		}
		if len(a.Clients) == 0 {
			return fmt.Errorf("auth %q: at least one client is required for bearer_static mode", a.Name)
		}
	default:
		return fmt.Errorf("auth %q: invalid mode %q (must be none or bearer_static)", a.Name, a.Mode)
	}
	return nil
}

func validateProviders(providers []Provider) error {
	for _, p := range providers {
		if !isLowercaseName(p.Name) {
			return fmt.Errorf("provider %q: name must be lowercase, no spaces, no '/', and start with [a-z0-9]", p.Name)
		}
		switch p.Type {
		case ProviderTypeOpenAI, ProviderTypeOpenAICompatible, ProviderTypeAnthropic, ProviderTypeGemini:
		default:
			return fmt.Errorf("provider %q: unsupported type %q", p.Name, p.Type)
		}
		if p.Type == ProviderTypeOpenAICompatible && p.BaseURL == "" {
			return fmt.Errorf("provider %q: base_url is required for openai-compatible", p.Name)
		}
		if p.APIKey == "" {
			return fmt.Errorf("provider %q: exactly one of api_key or api_key_ref must be set (no credential resolved)", p.Name)
		}
		if len(p.Models) == 0 {
			return fmt.Errorf("provider %q: at least one model is required", p.Name)
		}
		for _, m := range p.Models {
			if !isLowercaseModelName(m.Name) {
				return fmt.Errorf("provider %q: model %q name must be lowercase, no spaces, and each '/'-separated segment must start with [a-z0-9]", p.Name, m.Name)
			}
			if m.UpstreamName == "" {
				return fmt.Errorf("provider %q: model %q has empty upstream_name", p.Name, m.Name)
			}
			seenCaps := make(map[Capability]bool)
			for _, c := range m.Capabilities {
				if !isValidCapability(c) {
					return fmt.Errorf("provider %q: model %q has invalid capability %q", p.Name, m.Name, c)
				}
				if !providerSupportsCapability(p.Type, c) {
					return fmt.Errorf("provider %q: model %q capability %q is not supported by provider type %q", p.Name, m.Name, c, p.Type)
				}
				if seenCaps[c] {
					return fmt.Errorf("provider %q: model %q has duplicate capability %q", p.Name, m.Name, c)
				}
				seenCaps[c] = true
			}
		}
	}
	return nil
}

func validateAliases(aliases []Alias, providers map[string]Provider) error {
	for _, a := range aliases {
		if !isLowercaseName(a.Name) {
			return fmt.Errorf("alias %q: name must be lowercase, no spaces, no '/', and start with [a-z0-9]", a.Name)
		}
		switch a.Algorithm {
		case AlgorithmRoundRobin, AlgorithmLeastConnections:
		default:
			return fmt.Errorf("alias %q: invalid algorithm %q", a.Name, a.Algorithm)
		}
		if len(a.Targets) == 0 {
			return fmt.Errorf("alias %q: at least one target is required", a.Name)
		}
		seen := make(map[string]bool)
		for _, t := range a.Targets {
			prov, ok := providers[t.Provider]
			if !ok {
				return fmt.Errorf("alias %q: target provider %q is not defined", a.Name, t.Provider)
			}
			if _, ok := prov.ModelByName[t.Model]; !ok {
				return fmt.Errorf("alias %q: target model %q is not defined on provider %q", a.Name, t.Model, t.Provider)
			}
			key := t.Provider + "/" + t.Model
			if seen[key] {
				return fmt.Errorf("alias %q: duplicate target %q", a.Name, key)
			}
			seen[key] = true
		}
		if len(AliasEffectiveCapabilities(a, providers)) == 0 {
			return fmt.Errorf("alias %q: targets do not share any capabilities", a.Name)
		}
	}
	return nil
}

func isValidCapability(c Capability) bool {
	switch c {
	case CapabilityChat, CapabilityResponses, CapabilityEmbeddings:
		return true
	case CapabilityImages, CapabilityAudioTranscriptions, CapabilityAudioSpeech:
		return true
	default:
		return false
	}
}

func providerSupportsCapability(t ProviderType, c Capability) bool {
	switch t {
	case ProviderTypeOpenAI, ProviderTypeOpenAICompatible:
		return c == CapabilityChat || c == CapabilityResponses || c == CapabilityEmbeddings || c == CapabilityImages || c == CapabilityAudioTranscriptions || c == CapabilityAudioSpeech
	case ProviderTypeAnthropic:
		return c == CapabilityChat || c == CapabilityResponses
	case ProviderTypeGemini:
		return c == CapabilityChat || c == CapabilityResponses || c == CapabilityEmbeddings
	default:
		return false
	}
}
