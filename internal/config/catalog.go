package config

import (
	"slices"
	"strings"

	"github.com/egose/aiproxy/internal/identity"
)

type Catalog struct {
	providers         []Provider
	disabledProviders []Provider
	aliases           []Alias
	providerIndex     map[string]int
	aliasIndex        map[string]int
}

func NewCatalog(providers, disabledProviders []Provider, aliases []Alias) Catalog {
	c := Catalog{
		providers:         cloneProviders(providers),
		disabledProviders: cloneProviders(disabledProviders),
		aliases:           cloneAliases(aliases),
		providerIndex:     make(map[string]int, len(providers)),
		aliasIndex:        make(map[string]int, len(aliases)),
	}
	for i, provider := range c.providers {
		c.providerIndex[provider.Name] = i
	}
	for i, alias := range c.aliases {
		c.aliasIndex[alias.Name] = i
	}
	return c
}

func (c Catalog) ProviderCount() int {
	return len(c.providers)
}

func (c Catalog) AliasCount() int {
	return len(c.aliases)
}

func (c Catalog) DisabledProviderCount() int {
	return len(c.disabledProviders)
}

func (c Catalog) Providers() []Provider {
	return cloneProviders(c.providers)
}

func (c Catalog) DisabledProviders() []Provider {
	return cloneProviders(c.disabledProviders)
}

func (c Catalog) Aliases() []Alias {
	return cloneAliases(c.aliases)
}

func (c Catalog) Provider(name string) (Provider, bool) {
	i, ok := c.providerIndex[name]
	if !ok {
		return Provider{}, false
	}
	return cloneProvider(c.providers[i]), true
}

func (c Catalog) HasProvider(name string) bool {
	_, ok := c.providerIndex[name]
	return ok
}

func (c Catalog) Alias(name string) (Alias, bool) {
	i, ok := c.aliasIndex[name]
	if !ok {
		return Alias{}, false
	}
	return cloneAlias(c.aliases[i]), true
}

func (c Catalog) Model(providerName, modelName string) (Provider, Model, bool) {
	i, ok := c.providerIndex[providerName]
	if !ok {
		return Provider{}, Model{}, false
	}
	provider := c.providers[i]
	model, ok := provider.ModelByName[modelName]
	if !ok {
		return Provider{}, Model{}, false
	}
	return cloneProvider(provider), cloneModel(model), true
}

func (c Catalog) MatchModelPassthrough(providerName, suffix string, workspace *identity.WorkspaceScope) (Provider, ModelPassthroughRule, ModelPassthroughMatch) {
	if len(suffix) > MaxModelPassthroughSuffixBytes {
		return Provider{}, ModelPassthroughRule{}, ModelPassthroughNoMatch
	}
	i, ok := c.providerIndex[providerName]
	if !ok {
		return Provider{}, ModelPassthroughRule{}, ModelPassthroughNoMatch
	}
	p := c.providers[i]
	if _, exact := p.ModelByName[suffix]; exact {
		return Provider{}, ModelPassthroughRule{}, ModelPassthroughNoMatch
	}
	if workspace != nil {
		if _, scoped := workspace.ID(); !scoped || p.Workspace != *workspace {
			return Provider{}, ModelPassthroughRule{}, ModelPassthroughNoMatch
		}
	}
	rule, match := p.MatchModelPassthrough(suffix)
	if match != ModelPassthroughMatched {
		return Provider{}, ModelPassthroughRule{}, match
	}
	return cloneProvider(p), rule, match
}

func (c Catalog) ModelVisibleInWorkspace(publicModel string, workspace identity.WorkspaceScope) bool {
	if _, scoped := workspace.ID(); !scoped {
		return false
	}
	provider, model, ok := strings.Cut(publicModel, "/")
	if !ok {
		return false
	}
	if provider != "alias" {
		return c.modelInWorkspace(provider, model, workspace)
	}
	i, ok := c.aliasIndex[model]
	if !ok || c.aliases[i].Workspace != workspace || len(c.aliases[i].Targets) == 0 {
		return false
	}
	for _, target := range c.aliases[i].Targets {
		if !c.modelInWorkspace(target.Provider, target.Model, workspace) {
			return false
		}
	}
	return true
}

func (c Catalog) modelInWorkspace(provider, model string, workspace identity.WorkspaceScope) bool {
	i, ok := c.providerIndex[provider]
	if !ok || c.providers[i].Workspace != workspace {
		return false
	}
	p := c.providers[i]
	if p.Extends != "" {
		base, ok := c.providerIndex[p.Extends]
		if !ok || c.providers[base].Workspace != workspace || c.providers[base].Extends != "" {
			return false
		}
	}
	_, ok = p.ModelByName[model]
	return ok
}

func (c Catalog) ProviderNames() []string {
	out := make([]string, 0, len(c.providers))
	for _, provider := range c.providers {
		out = append(out, provider.Name)
	}
	return out
}

func (c Catalog) AliasEffectiveCapabilities(alias Alias) []Capability {
	if len(alias.Targets) == 0 {
		return nil
	}
	var intersection []Capability
	for i, target := range alias.Targets {
		provider, model, ok := c.Model(target.Provider, target.Model)
		if !ok {
			return nil
		}
		caps := EffectiveCapabilities(provider.Type, model)
		if i == 0 {
			intersection = append(intersection, caps...)
			continue
		}
		intersection = intersectCapabilities(intersection, caps)
		if len(intersection) == 0 {
			return nil
		}
	}
	return intersection
}

func cloneProviders(in []Provider) []Provider {
	if len(in) == 0 {
		return nil
	}
	out := make([]Provider, 0, len(in))
	for _, provider := range in {
		out = append(out, cloneProvider(provider))
	}
	return out
}

func cloneProvider(provider Provider) Provider {
	out := provider
	if profile, err := resolveProviderTLSProfile(provider, true); err == nil {
		out.TLSProfile = profile
	}
	if provider.APIKeyRef != nil { // pragma: allowlist secret
		ref := *provider.APIKeyRef
		out.APIKeyRef = &ref
	}
	if provider.CopilotCredentialRef != nil {
		ref := *provider.CopilotCredentialRef
		out.CopilotCredentialRef = &ref
	}
	if provider.Healthcheck != nil {
		hc := *provider.Healthcheck
		out.Healthcheck = &hc
	}
	out.DeferredTransport = cloneDeferredTransport(provider.DeferredTransport)
	out.EncryptedReasoning = CloneEncryptedReasoning(provider.EncryptedReasoning)
	if provider.DeferredReceiver != nil {
		deferred := *provider.DeferredReceiver
		out.DeferredReceiver = &deferred
	}
	out.Models = cloneModels(provider.Models)
	out.ModelPassthroughs = slices.Clone(provider.ModelPassthroughs)
	out.ModelByName = make(map[string]Model, len(out.Models)+len(provider.ModelByName))
	for i := range out.Models {
		model := &out.Models[i]
		if model.DeferredTransport == nil {
			model.DeferredTransport = cloneDeferredTransport(provider.ModelByName[model.Name].DeferredTransport)
		}
		if model.EncryptedReasoning == nil {
			indexed := provider.ModelByName[model.Name]
			model.EncryptedReasoning = CloneEncryptedReasoning(indexed.EncryptedReasoning)
			model.EncryptedReasoningInherited = indexed.EncryptedReasoningInherited
		}
		out.ModelByName[model.Name] = cloneModel(*model)
	}
	for name, indexed := range provider.ModelByName {
		if _, ok := out.ModelByName[name]; ok {
			continue
		}
		out.Models = append(out.Models, cloneModel(indexed))
		out.ModelByName[name] = cloneModel(indexed)
	}
	return out
}

func cloneModels(in []Model) []Model {
	if len(in) == 0 {
		return nil
	}
	out := make([]Model, len(in))
	for i, model := range in {
		out[i] = cloneModel(model)
	}
	return out
}

func cloneModel(model Model) Model {
	model.Capabilities = append([]Capability(nil), model.Capabilities...)
	model.DeferredTransport = cloneDeferredTransport(model.DeferredTransport)
	model.EncryptedReasoning = CloneEncryptedReasoning(model.EncryptedReasoning)
	return model
}

func cloneDeferredTransport(in *DeferredTransport) *DeferredTransport {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func cloneAliases(in []Alias) []Alias {
	if len(in) == 0 {
		return nil
	}
	out := make([]Alias, len(in))
	for i, alias := range in {
		out[i] = cloneAlias(alias)
	}
	return out
}

func cloneAlias(alias Alias) Alias {
	alias.RetryStatusCodes = append([]int(nil), alias.RetryStatusCodes...)
	alias.Targets = append([]AliasTarget(nil), alias.Targets...)
	if alias.SessionAffinity != nil {
		affinity := *alias.SessionAffinity
		affinity.Headers = append([]string(nil), alias.SessionAffinity.Headers...)
		alias.SessionAffinity = &affinity
	}
	alias.EncryptedReasoning = CloneEncryptedReasoning(alias.EncryptedReasoning)
	return alias
}
