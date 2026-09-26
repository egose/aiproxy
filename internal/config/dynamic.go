package config

import (
	"fmt"
	"strings"
)

type ProviderCredentialKind string

const (
	ProviderCredentialAPIKey     ProviderCredentialKind = "api_key"
	ProviderCredentialCopilotRef ProviderCredentialKind = "credential_ref"
	ProviderCredentialOptional   ProviderCredentialKind = "optional"
)

type ProviderTypeInfo struct {
	Type                  ProviderType
	Credential            ProviderCredentialKind
	RequiresBaseURL       bool
	SupportsHealthcheck   bool
	ModelProtocolRequired bool
	Protocols             []ModelProtocol
	DefaultCapabilities   []Capability
	SupportedCapabilities []Capability
}

func DescribeProviderTypes() []ProviderTypeInfo {
	out := make([]ProviderTypeInfo, 0, len(providerTypeOrder))
	for _, t := range providerTypeOrder {
		policy := providerTypePolicies[t]
		info := ProviderTypeInfo{
			Type:                  t,
			Credential:            ProviderCredentialAPIKey,
			RequiresBaseURL:       policy.requiresBaseURL,
			SupportsHealthcheck:   t != ProviderTypeGitHubCopilot,
			DefaultCapabilities:   append([]Capability(nil), policy.defaultCapabilities...),
			SupportedCapabilities: append([]Capability(nil), policy.supportedCapabilities...),
		}
		switch t {
		case ProviderTypeGitHubCopilot:
			info.Credential = ProviderCredentialCopilotRef
		case ProviderTypeOpenCodeZen:
			info.Credential = ProviderCredentialOptional
		}
		if IsOpenCodeProviderType(t) {
			info.ModelProtocolRequired = true
			info.Protocols = []ModelProtocol{ModelProtocolChat, ModelProtocolResponses, ModelProtocolMessages, ModelProtocolGemini}
			if t == ProviderTypeOpenCodeGo {
				info.Protocols = []ModelProtocol{ModelProtocolChat, ModelProtocolResponses, ModelProtocolMessages}
			}
		}
		out = append(out, info)
	}
	return out
}

func DefaultRetryStatusCodes() []int {
	codes := make([]int, len(defaultRetryStatusCodes))
	copy(codes, defaultRetryStatusCodes)
	return codes
}

func NormalizeAlias(a Alias) Alias {
	out := a
	if len(out.RetryStatusCodes) == 0 {
		out.RetryStatusCodes = DefaultRetryStatusCodes()
	}
	if out.SessionAffinity != nil {
		headers := make([]string, 0, len(out.SessionAffinity.Headers))
		for _, h := range out.SessionAffinity.Headers {
			headers = append(headers, strings.ToLower(strings.TrimSpace(h)))
		}
		if len(headers) == 0 {
			headers = append([]string(nil), DefaultSessionAffinityHeaders...)
		}
		out.SessionAffinity = &SessionAffinity{Headers: headers}
	}
	if out.EncryptedReasoning != nil {
		er := &EncryptedReasoning{
			Passthrough:      out.EncryptedReasoning.Passthrough,
			OnCallerMismatch: out.EncryptedReasoning.OnCallerMismatch,
		}
		if er.OnCallerMismatch == "" {
			er.OnCallerMismatch = EncryptedReasoningFail
		}
		for _, m := range out.EncryptedReasoning.MatchMessages {
			er.MatchMessages = append(er.MatchMessages, strings.ToLower(strings.TrimSpace(m)))
		}
		if len(er.MatchMessages) == 0 {
			er.MatchMessages = append([]string(nil), DefaultEncryptedReasoningMatchMessages...)
		}
		out.EncryptedReasoning = er
	}
	return out
}

func ValidateDynamicProvider(p Provider) error {
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
	if err := ValidateForwardHeaders(p.ForwardHeaders, fmt.Sprintf("provider %q", p.Name)); err != nil {
		return err
	}
	if err := validateProviderHealthcheck(p); err != nil {
		return err
	}
	if err := validateDynamicCredential(p); err != nil {
		return err
	}
	if len(p.Models) == 0 {
		return fmt.Errorf("provider %q: at least one model is required", p.Name)
	}
	seenModels := make(map[string]bool, len(p.Models))
	for _, m := range p.Models {
		if seenModels[m.Name] {
			return fmt.Errorf("provider %q: duplicate model %q", p.Name, m.Name)
		}
		seenModels[m.Name] = true
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
		if m.Pricing != nil {
			if err := validatePricingRates(p.Name, m.Name, m.Pricing); err != nil {
				return err
			}
		}
	}
	return nil
}

func validatePricingRates(providerName, modelName string, pricing *ModelPricing) error {
	for _, item := range []struct {
		field string
		rate  float64
	}{
		{"input_per_million", pricing.InputPerMillion},
		{"output_per_million", pricing.OutputPerMillion},
		{"cached_per_million", pricing.CachedPerMillion},
		{"cache_write_per_million", pricing.CacheWritePerMillion},
	} {
		if item.rate < 0 {
			return fmt.Errorf("provider %q: model %q pricing %q must not be negative", providerName, modelName, item.field)
		}
	}
	return nil
}

func BuildDynamicPricing(providerName, modelName string, rates map[string]float64) (*ModelPricing, error) {
	if rates == nil {
		return nil, nil
	}
	out := &ModelPricing{}
	set := map[string]*float64{
		"input_per_million":       &out.InputPerMillion,
		"output_per_million":      &out.OutputPerMillion,
		"cached_per_million":      &out.CachedPerMillion,
		"cache_write_per_million": &out.CacheWritePerMillion,
	}
	declared := 0
	for field, rate := range rates {
		target, ok := set[field]
		if !ok {
			return nil, fmt.Errorf("provider %q: model %q has unknown pricing rate %q", providerName, modelName, field)
		}
		if rate < 0 {
			return nil, fmt.Errorf("provider %q: model %q pricing %q must not be negative", providerName, modelName, field)
		}
		*target = rate
		declared++
	}
	if declared == 0 {
		return nil, fmt.Errorf("provider %q: model %q pricing must declare at least one rate", providerName, modelName)
	}
	return out, nil
}

func validateDynamicCredential(p Provider) error {
	if p.Type == ProviderTypeGitHubCopilot {
		if p.APIKey != "" || p.APIKeyRef != nil { // pragma: allowlist secret
			return fmt.Errorf("provider %q: api_key and api_key_ref are not supported by github-copilot; use credential_ref", p.Name)
		}
		if p.CopilotCredentialRef == nil || p.CopilotCredentialRef.Name == "" {
			if !p.Enabled {
				return nil
			}
			return fmt.Errorf("provider %q: enabled github-copilot providers require a credential_ref name (run login first; set enabled = false to disable a provider intentionally)", p.Name)
		}
		return nil
	}
	if p.CopilotCredentialRef != nil {
		return fmt.Errorf("provider %q: credential_ref is only supported by github-copilot", p.Name)
	}
	if p.APIKey != "" && p.APIKeyRef != nil { // pragma: allowlist secret
		return fmt.Errorf("provider %q: only one of api_key or api_key_ref may be set", p.Name)
	}
	if p.APIKeyRef != nil && p.APIKeyRef.Key == "" { // pragma: allowlist secret
		return fmt.Errorf("provider %q: api_key_ref.key is required", p.Name)
	}
	if !p.Enabled {
		return nil
	}
	if p.APIKey == "" && p.APIKeyRef == nil && p.Type != ProviderTypeOpenCodeZen { // pragma: allowlist secret
		return fmt.Errorf("provider %q: enabled providers require a non-empty api_key or api_key_ref (set enabled = false to disable a provider intentionally; opencode-zen providers may omit the credential for keyless upstream access)", p.Name)
	}
	return nil
}

func ValidateDynamicAlias(a Alias, catalog Catalog) error {
	return validateAliases([]Alias{a}, catalog)
}
