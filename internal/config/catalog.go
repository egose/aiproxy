package config

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

func (c Catalog) Alias(name string) (Alias, bool) {
	i, ok := c.aliasIndex[name]
	if !ok {
		return Alias{}, false
	}
	return cloneAlias(c.aliases[i]), true
}

func (c Catalog) Model(providerName, modelName string) (Provider, Model, bool) {
	provider, ok := c.Provider(providerName)
	if !ok {
		return Provider{}, Model{}, false
	}
	model, ok := provider.ModelByName[modelName]
	if !ok {
		return Provider{}, Model{}, false
	}
	return provider, cloneModel(model), true
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
	if provider.APIKeyRef != nil { // pragma: allowlist secret
		ref := *provider.APIKeyRef
		out.APIKeyRef = &ref
	}
	out.Models = cloneModels(provider.Models)
	out.ModelByName = make(map[string]Model, len(out.Models)+len(provider.ModelByName))
	for _, model := range out.Models {
		out.ModelByName[model.Name] = cloneModel(model)
	}
	for name, model := range provider.ModelByName {
		if _, ok := out.ModelByName[name]; ok {
			continue
		}
		model = cloneModel(model)
		out.ModelByName[name] = model
		out.Models = append(out.Models, model)
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
	return model
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
	return alias
}
