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
	if err := validateProviders(rt.Catalog.Providers(), true); err != nil {
		return err
	}
	if err := validateProviders(rt.Catalog.DisabledProviders(), false); err != nil {
		return err
	}
	if err := validateAliases(rt.Catalog.Aliases(), rt.Catalog); err != nil {
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
		return fmt.Errorf("dashboard: allow_insecure_remote is unsupported; the dashboard command is local-only")
	}
	return nil
}

func validateListener(l Listener) error {
	if l.Address == "" {
		return fmt.Errorf("listener.http %q: address is required", l.Name)
	}
	if strings.Contains(l.Address, "://") {
		return fmt.Errorf("listener.http %q: address must be a TCP bind address in host:port form, not a URL", l.Name)
	}
	_, port, err := net.SplitHostPort(l.Address)
	if err != nil || port == "" {
		return fmt.Errorf("listener.http %q: address must be a TCP bind address in host:port form", l.Name)
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
		policy, ok := providerTypePolicies[p.Type]
		if !ok {
			return fmt.Errorf("provider %q: unsupported type %q", p.Name, p.Type)
		}
		if policy.requiresBaseURL && p.BaseURL == "" {
			return fmt.Errorf("provider %q: base_url is required for %s", p.Name, p.Type)
		}
		if err := validateProviderBaseURL(p); err != nil {
			return err
		}
		if err := validateProviderUserAgent(p); err != nil {
			return err
		}
		if requireCredential && p.Type == ProviderTypeGitHubCopilot && p.CopilotToken == "" {
			return fmt.Errorf("provider %q: enabled github-copilot providers require a resolvable credential_ref (run login first; set enabled = false to disable a provider intentionally)", p.Name)
		}
		if requireCredential && p.Type != ProviderTypeGitHubCopilot && p.APIKey == "" && p.Type != ProviderTypeOpenCodeZen {
			return fmt.Errorf("provider %q: enabled providers require a non-empty api_key or a resolvable api_key_ref (set enabled = false to disable a provider intentionally; opencode-zen providers may omit the credential for keyless upstream access)", p.Name)
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
			if err := validateModelProtocol(p.Type, p.Name, m); err != nil {
				return err
			}
			served := OpenCodeProtocolCapabilities(m.Protocol)
			seenCaps := make(map[Capability]bool)
			for _, c := range m.Capabilities {
				if !isValidCapability(c) {
					return fmt.Errorf("provider %q: model %q has invalid capability %q", p.Name, m.Name, c)
				}
				if !providerSupportsCapability(p.Type, c) {
					return fmt.Errorf("provider %q: model %q capability %q is not supported by provider type %q", p.Name, m.Name, c, p.Type)
				}
				if IsOpenCodeProviderType(p.Type) && !HasCapability(served, c) {
					return fmt.Errorf("provider %q: model %q capability %q is not served by protocol %q", p.Name, m.Name, c, m.Protocol)
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

func validateModelProtocol(providerType ProviderType, providerName string, m Model) error {
	if !IsOpenCodeProviderType(providerType) {
		if m.Protocol != "" {
			return fmt.Errorf("provider %q: model %q protocol is only supported by opencode-zen and opencode-go", providerName, m.Name)
		}
		return nil
	}
	switch m.Protocol {
	case ModelProtocolChat, ModelProtocolResponses, ModelProtocolMessages, ModelProtocolGemini:
	default:
		return fmt.Errorf("provider %q: model %q has invalid protocol %q (must be chat, responses, messages, or gemini)", providerName, m.Name, m.Protocol)
	}
	if providerType == ProviderTypeOpenCodeGo && m.Protocol == ModelProtocolGemini {
		return fmt.Errorf("provider %q: model %q protocol %q is not supported by provider type %q", providerName, m.Name, m.Protocol, providerType)
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

func validateProviderUserAgent(p Provider) error {
	if p.UserAgent == "" {
		return nil
	}
	if !IsOpenCodeProviderType(p.Type) {
		return fmt.Errorf("provider %q: user_agent is only supported by opencode-zen and opencode-go", p.Name)
	}
	if len(p.UserAgent) > 256 || !isValidUserAgent(p.UserAgent) {
		return fmt.Errorf("provider %q: user_agent must be 1-256 printable ASCII characters without newlines", p.Name)
	}
	return nil
}

func isValidUserAgent(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func validateAliases(aliases []Alias, catalog Catalog) error {
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
			_, _, ok := catalog.Model(t.Provider, t.Model)
			if !ok {
				if _, providerOK := catalog.Provider(t.Provider); !providerOK {
					return fmt.Errorf("alias %q: target provider %q is not defined", a.Name, t.Provider)
				}
				return fmt.Errorf("alias %q: target model %q is not defined on provider %q", a.Name, t.Model, t.Provider)
			}
			key := t.Provider + "/" + t.Model
			if seen[key] {
				return fmt.Errorf("alias %q: duplicate target %q", a.Name, key)
			}
			seen[key] = true
		}
		if len(catalog.AliasEffectiveCapabilities(a)) == 0 {
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
	policy, ok := providerTypePolicies[t]
	if !ok {
		return false
	}
	return HasCapability(policy.supportedCapabilities, c)
}
