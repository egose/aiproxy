package config

import "testing"

func TestCatalogBuildsLookupFromOrderedProvidersAliasesAndModels(t *testing.T) {
	catalog := NewCatalog([]Provider{{
		Name:   "openai",
		Type:   ProviderTypeOpenAI,
		Models: []Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini"}},
	}}, nil, []Alias{{
		Name:      "chat",
		Algorithm: AlgorithmRoundRobin,
		Targets:   []AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}},
	}})

	providers := catalog.Providers()
	if len(providers) != 1 || providers[0].Name != "openai" {
		t.Fatalf("providers = %+v", providers)
	}
	provider, ok := catalog.Provider("openai")
	if !ok || provider.Name != providers[0].Name {
		t.Fatalf("provider lookup = %+v, %v", provider, ok)
	}
	aliases := catalog.Aliases()
	if len(aliases) != 1 || aliases[0].Name != "chat" {
		t.Fatalf("aliases = %+v", aliases)
	}
	alias, ok := catalog.Alias("chat")
	if !ok || alias.Name != aliases[0].Name {
		t.Fatalf("alias lookup = %+v, %v", alias, ok)
	}
	_, model, ok := catalog.Model("openai", "gpt-4o-mini")
	if !ok || model.UpstreamName != "gpt-4o-mini" {
		t.Fatalf("model lookup = %+v, %v", model, ok)
	}
}

func TestCatalogReturnedValuesCannotMutateStoredCatalog(t *testing.T) {
	catalog := NewCatalog([]Provider{{
		Name: "openai",
		Models: []Model{{
			Name:         "gpt-4o-mini",
			UpstreamName: "gpt-4o-mini",
			Capabilities: []Capability{CapabilityChat},
		}},
	}}, nil, []Alias{{
		Name:             "chat",
		Algorithm:        AlgorithmRoundRobin,
		RetryStatusCodes: []int{500},
		Targets:          []AliasTarget{{Provider: "openai", Model: "gpt-4o-mini"}},
	}})

	providers := catalog.Providers()
	providers[0].Name = "mutated"
	providers[0].Models[0].Name = "mutated"
	providers[0].Models[0].Capabilities[0] = CapabilityEmbeddings
	providers[0].ModelByName["gpt-4o-mini"] = Model{Name: "mutated"}
	provider, model, ok := catalog.Model("openai", "gpt-4o-mini")
	if !ok || provider.Name != "openai" || model.Name != "gpt-4o-mini" || model.Capabilities[0] != CapabilityChat {
		t.Fatalf("catalog mutated through provider copy: provider=%+v model=%+v ok=%v", provider, model, ok)
	}

	aliases := catalog.Aliases()
	aliases[0].Name = "mutated"
	aliases[0].RetryStatusCodes[0] = 429
	aliases[0].Targets[0].Provider = "mutated"
	alias, ok := catalog.Alias("chat")
	if !ok || alias.Name != "chat" || alias.RetryStatusCodes[0] != 500 || alias.Targets[0].Provider != "openai" {
		t.Fatalf("catalog mutated through alias copy: alias=%+v ok=%v", alias, ok)
	}
}
