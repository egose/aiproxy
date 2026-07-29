package config

import "fmt"

func buildRuntime(raw *rawFile) (*Runtime, error) {
	rt := &Runtime{
		ProviderByName: make(map[string]Provider),
		AliasByName:    make(map[string]Alias),
	}
	seenProviderNames := make(map[string]bool)
	disabledProviderNames := make(map[string]bool)

	listener, err := buildListener(raw.Listeners)
	if err != nil {
		return nil, err
	}
	rt.Listener = listener

	auth, err := buildAuth(raw.Auth)
	if err != nil {
		return nil, err
	}
	rt.Auth = auth

	for _, p := range raw.Providers {
		if seenProviderNames[p.Name] {
			return nil, fmt.Errorf("duplicate provider %q", p.Name)
		}
		seenProviderNames[p.Name] = true

		provider, err := buildProvider(p)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}
		if provider.APIKey == "" {
			rt.DisabledProviders = append(rt.DisabledProviders, provider)
			disabledProviderNames[provider.Name] = true
			continue
		}
		rt.Providers = append(rt.Providers, provider)
		rt.ProviderByName[p.Name] = provider
	}

	for _, al := range raw.Aliases {
		if _, dup := rt.AliasByName[al.Name]; dup {
			return nil, fmt.Errorf("duplicate alias %q", al.Name)
		}
		alias := Alias{Name: al.Name, Algorithm: Algorithm(al.Algorithm)}
		for _, t := range al.Targets {
			if disabledProviderNames[t.Provider] {
				continue
			}
			alias.Targets = append(alias.Targets, AliasTarget{Provider: t.Provider, Model: t.Model})
		}
		rt.Aliases = append(rt.Aliases, alias)
		rt.AliasByName[al.Name] = alias
	}

	return rt, nil
}

func buildListener(listeners []rawListener) (Listener, error) {
	if len(listeners) == 0 {
		return Listener{}, fmt.Errorf("no listener block defined")
	}
	if len(listeners) > 1 {
		return Listener{}, fmt.Errorf("only one listener block is supported")
	}
	l := listeners[0]
	if l.Type != "http" {
		return Listener{}, fmt.Errorf("unsupported listener type %q (only \"http\" is supported)", l.Type)
	}
	listener := Listener{Name: l.Name, Address: l.Address}
	if l.Timeouts != nil {
		var err error
		listener.Timeouts, err = parseTimeouts(l.Timeouts)
		if err != nil {
			return Listener{}, err
		}
	}
	return listener, nil
}

func buildAuth(rawAuths []rawAuth) (Auth, error) {
	if len(rawAuths) == 0 {
		return Auth{}, fmt.Errorf("no auth block defined")
	}
	if len(rawAuths) > 1 {
		return Auth{}, fmt.Errorf("only one auth block is supported")
	}
	a := rawAuths[0]
	auth := Auth{Name: a.Name, Mode: AuthMode(a.Mode), Clients: make(map[string]Client)}
	if a.RateLimit != nil {
		burst := a.RateLimit.Burst
		if burst <= 0 {
			burst = a.RateLimit.RequestsPerMinute
		}
		auth.RateLimit = &RateLimit{RequestsPerMinute: a.RateLimit.RequestsPerMinute, Burst: burst}
	}
	for _, c := range a.Clients {
		if _, dup := auth.Clients[c.Name]; dup {
			return Auth{}, fmt.Errorf("duplicate client %q in auth %q", c.Name, a.Name)
		}
		auth.Clients[c.Name] = Client{Name: c.Name, Token: c.Token}
	}
	return auth, nil
}

func buildProvider(rawProvider rawProvider) (Provider, error) {
	provider := Provider{
		Type:        ProviderType(rawProvider.Type),
		Name:        rawProvider.Name,
		DisplayName: rawProvider.DisplayName,
		BaseURL:     rawProvider.BaseURL,
		APIKey:      rawProvider.APIKey,
		ModelByName: make(map[string]Model),
	}
	if rawProvider.APIKeyRef != nil {
		provider.APIKeyRef = &APIKeyRef{Path: rawProvider.APIKeyRef.Path, Key: rawProvider.APIKeyRef.Key}
		if provider.APIKeyRef.Path == "" {
			provider.APIKeyRef.Path = defaultKeyFilePath()
		}
	}
	if err := resolveProviderCredential(&provider); err != nil {
		return Provider{}, err
	}
	for _, m := range rawProvider.Models {
		if _, dup := provider.ModelByName[m.Name]; dup {
			return Provider{}, fmt.Errorf("duplicate model %q in provider %q", m.Name, rawProvider.Name)
		}
		upstream := m.UpstreamName
		if upstream == "" {
			upstream = m.Name
		}
		model := Model{Name: m.Name, DisplayName: m.DisplayName, UpstreamName: upstream, Capabilities: make([]Capability, 0, len(m.Capabilities))}
		for _, c := range m.Capabilities {
			model.Capabilities = append(model.Capabilities, Capability(c))
		}
		provider.Models = append(provider.Models, model)
		provider.ModelByName[m.Name] = model
	}
	return provider, nil
}
