package config

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/redis/go-redis/v9"
)

func Validate(rt *Runtime) error {
	if err := validateListener(rt.Listener); err != nil {
		return err
	}
	if err := validateAuth(rt.Auth); err != nil {
		return err
	}
	if err := validateLogging(rt.Logging); err != nil {
		return err
	}
	if err := validateProviderHealth(rt.ProviderHealth); err != nil {
		return err
	}
	if err := validateMetrics(rt.Metrics); err != nil {
		return err
	}
	if err := validateDashboard(rt.Dashboard); err != nil {
		return err
	}
	if err := validateProviders(rt.Providers, true); err != nil {
		return err
	}
	if err := validateProviders(rt.DisabledProviders, false); err != nil {
		return err
	}
	if err := validateAliases(rt.Aliases, rt.ProviderByName); err != nil {
		return err
	}
	return nil
}

func validateLogging(l Logging) error {
	switch l.Level {
	case LogLevelDebug, LogLevelInfo, LogLevelWarn, LogLevelError:
		return nil
	default:
		return fmt.Errorf("logging: invalid level %q (must be debug, info, warn, or error)", l.Level)
	}
}

func validateProviderHealth(h ProviderHealth) error {
	if h.Cooldown < 0 {
		return fmt.Errorf("provider_health: cooldown must not be negative")
	}
	if h.RedisURL == "" {
		return nil
	}
	if h.KeyPrefix == "" {
		return fmt.Errorf("provider_health: key_prefix must not be empty")
	}
	if _, err := redis.ParseURL(h.RedisURL); err != nil {
		return fmt.Errorf("provider_health: redis_url must be a valid Redis URL: %w", err)
	}
	return nil
}

const minInsecureRemoteDashboardTokenLen = 32

func validateMetrics(m Metrics) error {
	if !m.Enabled {
		return nil
	}
	if m.Token == "" {
		return fmt.Errorf("metrics: token is required when metrics block is present")
	}
	return nil
}

func validateDashboard(d Dashboard) error {
	if !d.Enabled {
		return nil
	}
	if d.AllowInsecureRemote {
		if d.Token == "" || !d.TokenFromConfig {
			return fmt.Errorf("dashboard: allow_insecure_remote = true requires an explicit token declared in config (minted tokens are not permitted for remote cleartext access)")
		}
		if len(d.Token) < minInsecureRemoteDashboardTokenLen {
			return fmt.Errorf("dashboard: allow_insecure_remote = true requires a strong token of at least %d characters", minInsecureRemoteDashboardTokenLen)
		}
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

func validateProviders(providers []Provider, requireCredential bool) error {
	for _, p := range providers {
		if !IsLowercaseName(p.Name) {
			return fmt.Errorf("provider %q: name must be lowercase, no spaces, no '/', and start with [a-z0-9]", p.Name)
		}
		if p.Name == "alias" {
			return fmt.Errorf("provider %q: name is reserved for alias model routing", p.Name)
		}
		switch p.Type {
		case ProviderTypeOpenAI, ProviderTypeOpenAICompatible, ProviderTypeAnthropic, ProviderTypeGemini:
		default:
			return fmt.Errorf("provider %q: unsupported type %q", p.Name, p.Type)
		}
		if p.Type == ProviderTypeOpenAICompatible && p.BaseURL == "" {
			return fmt.Errorf("provider %q: base_url is required for openai-compatible", p.Name)
		}
		if err := validateProviderBaseURL(p); err != nil {
			return err
		}
		if requireCredential && p.APIKey == "" {
			return fmt.Errorf("provider %q: enabled providers require a non-empty api_key or a resolvable api_key_ref (set enabled = false to disable a provider intentionally)", p.Name)
		}
		if len(p.Models) == 0 {
			return fmt.Errorf("provider %q: at least one model is required", p.Name)
		}
		for _, m := range p.Models {
			if !IsLowercaseModelName(m.Name) {
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

func validateProviderBaseURL(p Provider) error {
	if p.BaseURL == "" {
		return nil
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("provider %q: base_url must be an absolute http or https URL", p.Name)
	}
	if u.User != nil {
		return fmt.Errorf("provider %q: base_url must not include userinfo", p.Name)
	}
	switch u.Scheme {
	case "https":
		return nil
	case "http":
		if isLoopbackHost(u.Hostname()) {
			return nil
		}
		return fmt.Errorf("provider %q: non-HTTPS base_url is allowed only for loopback hosts", p.Name)
	default:
		return fmt.Errorf("provider %q: base_url scheme must be http or https", p.Name)
	}
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateAliases(aliases []Alias, providers map[string]Provider) error {
	for _, a := range aliases {
		if !IsLowercaseName(a.Name) {
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
		for _, code := range a.RetryStatusCodes {
			if !isRetryableStatusCode(code) {
				return fmt.Errorf("alias %q: invalid retry status code %d: must be between 400 and 599", a.Name, code)
			}
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
