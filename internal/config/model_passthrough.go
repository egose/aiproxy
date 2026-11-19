package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

const (
	MaxModelPassthroughRules        = 32
	MaxModelPassthroughPatternBytes = 512
	MaxModelPassthroughSuffixBytes  = 256
)

type ModelPassthroughMatch uint8

const (
	ModelPassthroughNoMatch ModelPassthroughMatch = iota
	ModelPassthroughMatched
	ModelPassthroughAmbiguous
)

type ModelPassthroughRule struct {
	name         string
	protocol     ModelProtocol
	capabilities []Capability
	pattern      *regexp.Regexp
}

func (r ModelPassthroughRule) Name() string { return r.name }

func (r ModelPassthroughRule) Protocol() ModelProtocol { return r.protocol }

func (r ModelPassthroughRule) Capabilities() []Capability {
	return slices.Clone(r.capabilities)
}

func (p Provider) MatchModelPassthrough(suffix string) (ModelPassthroughRule, ModelPassthroughMatch) {
	if !p.Enabled || p.DatabaseManaged || p.Extends != "" || len(p.ModelPassthroughs) == 0 ||
		len(p.ModelPassthroughs) > MaxModelPassthroughRules || !supportsModelPassthrough(p.Type) ||
		len(suffix) > MaxModelPassthroughSuffixBytes || !IsLowercaseModelName(suffix) {
		return ModelPassthroughRule{}, ModelPassthroughNoMatch
	}
	var matched ModelPassthroughRule
	for _, rule := range p.ModelPassthroughs {
		if rule.pattern == nil {
			return ModelPassthroughRule{}, ModelPassthroughNoMatch
		}
		span := rule.pattern.FindStringIndex(suffix)
		if span == nil || span[0] != 0 || span[1] != len(suffix) {
			continue
		}
		if matched.pattern != nil {
			return ModelPassthroughRule{}, ModelPassthroughAmbiguous
		}
		matched = rule
	}
	if matched.pattern == nil || (ModelPassthroughUsesModelPath(p.Type, matched.protocol) && !IsBareModelPassthroughID(suffix)) {
		return ModelPassthroughRule{}, ModelPassthroughNoMatch
	}
	return matched, ModelPassthroughMatched
}

func ModelPassthroughUsesModelPath(t ProviderType, protocol ModelProtocol) bool {
	return t == ProviderTypeGemini || (t == ProviderTypeOpenCodeZen && protocol == ModelProtocolGemini)
}

func IsBareModelPassthroughID(suffix string) bool {
	return len(suffix) <= MaxModelPassthroughSuffixBytes && IsLowercaseName(suffix)
}

func supportsModelPassthrough(t ProviderType) bool {
	switch t {
	case ProviderTypeOpenAI, ProviderTypeOpenAICompatible, ProviderTypeOpenRouter, ProviderTypeZenMux,
		ProviderTypeAnthropic, ProviderTypeGitHubCopilot, ProviderTypeOpenCodeZen, ProviderTypeOpenCodeGo, ProviderTypeGemini:
		return true
	default:
		return false
	}
}

func validateModelPassthroughSurface(t ProviderType, extends string, managed bool, count int) error {
	switch {
	case managed:
		return fmt.Errorf("model_passthrough is static-config-only")
	case extends != "":
		return fmt.Errorf("derived provider cannot declare model_passthrough")
	case !supportsModelPassthrough(t):
		return fmt.Errorf("model_passthrough is not supported by provider type %q", t)
	case count > MaxModelPassthroughRules:
		return fmt.Errorf("at most %d model_passthrough rules are supported", MaxModelPassthroughRules)
	}
	return nil
}

func buildModelPassthroughs(p rawProvider) ([]ModelPassthroughRule, error) {
	if len(p.ModelPassthroughs) == 0 {
		return nil, nil
	}
	providerType := ProviderType(p.Type)
	if err := validateModelPassthroughSurface(providerType, p.Extends, false, len(p.ModelPassthroughs)); err != nil {
		return nil, err
	}
	rules := make([]ModelPassthroughRule, 0, len(p.ModelPassthroughs))
	seen := make(map[string]bool, len(p.ModelPassthroughs))
	for _, raw := range p.ModelPassthroughs {
		owner := fmt.Sprintf("model_passthrough %q", raw.Name)
		if !IsLowercaseName(raw.Name) {
			return nil, fmt.Errorf("%s: name must be lowercase, no spaces, no '/', and start with [a-z0-9]", owner)
		}
		if seen[raw.Name] {
			return nil, fmt.Errorf("duplicate %s rule", owner)
		}
		seen[raw.Name] = true
		if raw.Match == nil {
			return nil, fmt.Errorf("%s: match is required", owner)
		}
		if raw.Capabilities == nil {
			return nil, fmt.Errorf("%s: capabilities is required", owner)
		}
		value, diags := raw.Match.Expr.Value(nil)
		if diags.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String ||
			value.AsString() == "" || len(value.AsString()) > MaxModelPassthroughPatternBytes {
			return nil, fmt.Errorf("%s: match must be a nonempty string of at most %d bytes", owner, MaxModelPassthroughPatternBytes)
		}
		protocol, err := buildModelPassthroughProtocol(providerType, raw.Protocol, owner)
		if err != nil {
			return nil, err
		}
		caps, err := modelPassthroughStrings(raw.Capabilities.Expr, owner+": capabilities")
		if err != nil {
			return nil, err
		}
		if len(caps) == 0 {
			return nil, fmt.Errorf("%s: capabilities must not be empty", owner)
		}
		capabilities := make([]Capability, 0, len(caps))
		for _, value := range caps {
			capability := Capability(value)
			if !isValidCapability(capability) {
				return nil, fmt.Errorf("%s: invalid capability %q", owner, capability)
			}
			if err := validateProviderCapability(providerType, protocol, capability); err != nil {
				return nil, fmt.Errorf("%s: %w", owner, err)
			}
			if HasCapability(capabilities, capability) {
				return nil, fmt.Errorf("%s: duplicate capability %q", owner, capability)
			}
			capabilities = append(capabilities, capability)
		}
		pattern, err := regexp.Compile(value.AsString())
		if err != nil {
			return nil, fmt.Errorf("%s: invalid match regexp: %w", owner, err)
		}
		pattern.Longest()
		rules = append(rules, ModelPassthroughRule{name: raw.Name, protocol: protocol, capabilities: capabilities, pattern: pattern})
	}
	return rules, nil
}

func buildModelPassthroughProtocol(providerType ProviderType, attr *hcl.Attribute, owner string) (ModelProtocol, error) {
	if !IsOpenCodeProviderType(providerType) {
		if attr != nil {
			return "", fmt.Errorf("%s: protocol is only supported by opencode-zen and opencode-go", owner)
		}
		return "", nil
	}
	if attr == nil {
		return "", fmt.Errorf("%s: protocol is required", owner)
	}
	value, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || !value.IsKnown() || value.IsNull() || value.Type() != cty.String || value.AsString() == "" {
		return "", fmt.Errorf("%s: protocol must be a nonempty string", owner)
	}
	protocol := ModelProtocol(value.AsString())
	if err := validateModelProtocol(providerType, owner+":", protocol); err != nil {
		return "", err
	}
	switch protocol {
	case ModelProtocolChat, ModelProtocolResponses, ModelProtocolMessages, ModelProtocolGemini:
		return protocol, nil
	default:
		return "", fmt.Errorf("%s: protocol %q is not supported for model_passthrough", owner, protocol)
	}
}

func validateProviderModelPassthroughs(p Provider) error {
	if err := validateModelPassthroughSurface(p.Type, p.Extends, p.DatabaseManaged, len(p.ModelPassthroughs)); err != nil {
		return err
	}
	for _, rule := range p.ModelPassthroughs {
		if rule.pattern == nil {
			return fmt.Errorf("model_passthrough requires rules compiled during config construction")
		}
	}
	return validateModelPassthroughBaseURL(p.Type, p.BaseURL, p.ModelPassthroughs)
}

func validateModelPassthroughBaseURL(t ProviderType, baseURL string, rules []ModelPassthroughRule) error {
	if strings.ContainsAny(baseURL, "?#") {
		for _, rule := range rules {
			if ModelPassthroughUsesModelPath(t, rule.protocol) {
				return fmt.Errorf("base_url must not include a query or fragment for path-based model_passthrough rules")
			}
		}
	}
	return nil
}

func buildModelPassthroughGrants(authName string, c rawClient) ([]string, error) {
	if c.AllowedModelPassthroughs == nil {
		return nil, nil
	}
	return modelPassthroughStrings(c.AllowedModelPassthroughs.Expr, fmt.Sprintf("auth %q: client %q: allowed_model_passthroughs", authName, c.Name))
}

func modelPassthroughStrings(expr hcl.Expression, owner string) ([]string, error) {
	value, diags := expr.Value(nil)
	if diags.HasErrors() || value.IsNull() || !value.IsKnown() || (!value.Type().IsTupleType() && !value.Type().IsListType()) {
		return nil, fmt.Errorf("%s must be a list of strings", owner)
	}
	out := make([]string, 0, value.LengthInt())
	for it := value.ElementIterator(); it.Next(); {
		_, entry := it.Element()
		if !entry.IsKnown() || entry.IsNull() || entry.Type() != cty.String {
			return nil, fmt.Errorf("%s must contain only non-null strings", owner)
		}
		out = append(out, entry.AsString())
	}
	return out, nil
}

func validateCatalogModelPassthroughGrants(catalog Catalog, auth Auth) error {
	declared := make(map[string][]ModelPassthroughRule)
	for _, providers := range [][]Provider{catalog.providers, catalog.disabledProviders} {
		for _, p := range providers {
			if !p.DatabaseManaged && p.Extends == "" && len(p.ModelPassthroughs) != 0 {
				declared[p.Name] = p.ModelPassthroughs
			}
		}
	}
	for _, c := range auth.Clients {
		if err := validateModelPassthroughGrants(auth.Name, c.Name, c.AllowedModelPassthroughs, declared); err != nil {
			return err
		}
	}
	return nil
}

func validateModelPassthroughGrants(auth, client string, grants []string, declared map[string][]ModelPassthroughRule) error {
	for _, grant := range grants {
		provider, rule, ok := strings.Cut(grant, "/")
		if !ok || !IsLowercaseName(provider) || !IsLowercaseName(rule) {
			return fmt.Errorf("auth %q: client %q: allowed_model_passthroughs entry %q must be <provider>/<rule>", auth, client, grant)
		}
		if !slices.ContainsFunc(declared[provider], func(candidate ModelPassthroughRule) bool { return candidate.name == rule }) {
			return fmt.Errorf("auth %q: client %q: allowed_model_passthroughs entry %q is not a declared static model_passthrough rule", auth, client, grant)
		}
	}
	return nil
}

func validateEditableModelPassthrough(auth []rawAuth, providers []rawProvider, byName map[string]rawProvider) error {
	declared := make(map[string][]ModelPassthroughRule)
	for _, p := range providers {
		if len(p.ModelPassthroughs) != 0 {
			rules, err := buildModelPassthroughs(p)
			if err != nil {
				return fmt.Errorf("provider %q: %w", p.Name, err)
			}
			if err := validateModelPassthroughBaseURL(ProviderType(p.Type), p.BaseURL, rules); err != nil {
				return fmt.Errorf("provider %q: %w", p.Name, err)
			}
			declared[p.Name] = rules
		}
		if base := byName[p.Extends]; p.Extends != "" && len(base.ModelPassthroughs) != 0 && len(base.Models) == 0 {
			return fmt.Errorf("provider %q: extends %q requires a base with at least one literal model", p.Name, p.Extends)
		}
	}
	for _, a := range auth {
		for _, c := range a.Clients {
			if c.AllowedModelPassthroughs == nil {
				continue
			}
			if AuthMode(a.Mode) != AuthModeBearerStatic {
				return fmt.Errorf("auth %q: client %q: allowed_model_passthroughs requires bearer_static mode", a.Name, c.Name)
			}
			grants, err := buildModelPassthroughGrants(a.Name, c)
			if err != nil {
				return err
			}
			if err := validateModelPassthroughGrants(a.Name, c.Name, grants, declared); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateModelPassthroughBlockShapes(body hcl.Body) error {
	if _, native := body.(*hclsyntax.Body); native {
		return nil
	}
	providers, _, diags := body.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "provider", LabelNames: []string{"type", "name"}}},
	})
	if diags.HasErrors() {
		return fmt.Errorf("cannot inspect model_passthrough provider declarations: %s", diags.Error())
	}
	for _, p := range providers.Blocks {
		content, _, diags := p.Body.PartialContent(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{{Name: "model_passthrough"}},
		})
		if diags.HasErrors() {
			return fmt.Errorf("provider %q: cannot inspect model_passthrough declarations: %s", p.Labels[1], diags.Error())
		}
		attr := content.Attributes["model_passthrough"]
		if attr == nil {
			continue
		}
		value, diags := attr.Expr.Value(nil)
		if diags.HasErrors() || !value.IsKnown() || value.IsNull() {
			return fmt.Errorf("provider %q: model_passthrough requires distinct named block objects", p.Labels[1])
		}
		if !allModelPassthroughObjects(value, func(group cty.Value) bool {
			if group.LengthInt() == 0 {
				return false
			}
			for it := group.ElementIterator(); it.Next(); {
				_, rule := it.Element()
				if !allModelPassthroughObjects(rule, func(cty.Value) bool { return true }) {
					return false
				}
			}
			return true
		}) {
			return fmt.Errorf("provider %q: model_passthrough requires named non-null block objects", p.Labels[1])
		}
	}
	return nil
}

func allModelPassthroughObjects(value cty.Value, check func(cty.Value) bool) bool {
	if value.IsNull() {
		return false
	}
	if value.Type().IsObjectType() {
		return check(value)
	}
	if !value.Type().IsTupleType() || value.LengthInt() == 0 {
		return false
	}
	for it := value.ElementIterator(); it.Next(); {
		_, entry := it.Element()
		if entry.IsNull() || !entry.Type().IsObjectType() || !check(entry) {
			return false
		}
	}
	return true
}
