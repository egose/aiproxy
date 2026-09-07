package config

import (
	"fmt"
	"strconv"
	"time"
)

func buildRuntime(raw *rawFile) (*Runtime, error) {
	rt := &Runtime{
		Logging:               Logging{Level: LogLevelInfo, AccessLog: true},
		UpstreamHeaderTimeout: DefaultUpstreamHeaderTimeout,
	}
	seenProviderNames := make(map[string]bool)
	disabledProviderNames := make(map[string]bool)
	providerByName := make(map[string]Provider)
	aliasByName := make(map[string]bool)
	providers := []Provider(nil)
	disabledProviders := []Provider(nil)
	aliases := []Alias(nil)

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
	if raw.Logging != nil {
		logging, err := buildLogging(raw.Logging)
		if err != nil {
			return nil, err
		}
		rt.Logging = logging
	}
	if raw.ProviderHealth != nil {
		providerHealth, err := buildProviderHealth(raw.ProviderHealth)
		if err != nil {
			return nil, err
		}
		rt.ProviderHealth = providerHealth
	}
	if raw.UpstreamHeaderTimeout != "" {
		d, err := parsePositiveDuration("upstream_header_timeout", raw.UpstreamHeaderTimeout)
		if err != nil {
			return nil, err
		}
		rt.UpstreamHeaderTimeout = d
	}
	if len(raw.Dashboard) > 0 {
		if len(raw.Dashboard) > 1 {
			return nil, fmt.Errorf("only one dashboard block is supported")
		}
		rawDash := raw.Dashboard[0]
		rt.Dashboard = Dashboard{Token: rawDash.Token, TokenFromConfig: rawDash.Token != "", Enabled: true}
		if rawDash.AllowInsecureRemote != nil {
			rt.Dashboard.AllowInsecureRemote = *rawDash.AllowInsecureRemote
			rt.Dashboard.ExplicitAllowInsecure = true
		}
	}
	if len(raw.Metrics) > 0 {
		if len(raw.Metrics) > 1 {
			return nil, fmt.Errorf("only one metrics block is supported")
		}
		rt.Metrics = Metrics{Token: raw.Metrics[0].Token, Enabled: true}
	}

	providerByRawName := make(map[string]rawProvider)
	providerSyntaxByName := make(map[string]rawProviderSyntax)
	for i, p := range raw.Providers {
		if seenProviderNames[p.Name] {
			return nil, fmt.Errorf("duplicate provider %q", p.Name)
		}
		seenProviderNames[p.Name] = true
		providerByRawName[p.Name] = p
		if i < len(raw.providerSyntax) {
			providerSyntaxByName[p.Name] = raw.providerSyntax[i]
		}
	}

	for _, p := range raw.Providers {
		if p.Extends != "" {
			continue
		}
		provider, err := buildProvider(p, rt.UpstreamHeaderTimeout)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}
		if !provider.Enabled {
			disabledProviders = append(disabledProviders, provider)
			disabledProviderNames[provider.Name] = true
			continue
		}
		providers = append(providers, provider)
		providerByName[p.Name] = provider
	}
	for _, p := range raw.Providers {
		if p.Extends == "" {
			continue
		}
		provider, err := buildDerivedProvider(p, providerByRawName, providerSyntaxByName, providerByName)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", p.Name, err)
		}
		providers = append(providers, provider)
		providerByName[p.Name] = provider
	}

	for _, al := range raw.Aliases {
		if aliasByName[al.Name] {
			return nil, fmt.Errorf("duplicate alias %q", al.Name)
		}
		aliasByName[al.Name] = true
		retryCodes, err := parseRetryStatusCodes(al.RetryStatusCodes)
		if err != nil {
			return nil, fmt.Errorf("alias %q: %w", al.Name, err)
		}
		alias := Alias{Name: al.Name, Algorithm: Algorithm(al.Algorithm), RetryStatusCodes: retryCodes}
		for _, t := range al.Targets {
			if disabledProviderNames[t.Provider] {
				continue
			}
			alias.Targets = append(alias.Targets, AliasTarget{Provider: t.Provider, Model: t.Model})
		}
		aliases = append(aliases, alias)
	}
	rt.Catalog = NewCatalog(providers, disabledProviders, aliases)

	return rt, nil
}

func buildDerivedProvider(rawProvider rawProvider, rawByName map[string]rawProvider, syntaxByName map[string]rawProviderSyntax, builtByName map[string]Provider) (Provider, error) {
	if rawProvider.Extends == rawProvider.Name {
		return Provider{}, fmt.Errorf("extends %q references itself", rawProvider.Extends)
	}
	baseRaw, ok := rawByName[rawProvider.Extends]
	if !ok {
		return Provider{}, fmt.Errorf("extends %q is not defined", rawProvider.Extends)
	}
	if baseRaw.Extends != "" {
		return Provider{}, fmt.Errorf("extends %q references derived provider %q; inheritance chains are not supported", rawProvider.Extends, rawProvider.Extends)
	}
	base, ok := builtByName[rawProvider.Extends]
	if !ok {
		return Provider{}, fmt.Errorf("extends %q references a disabled provider", rawProvider.Extends)
	}
	if rawProvider.Type != baseRaw.Type {
		return Provider{}, fmt.Errorf("type %q must match base provider %q type %q", rawProvider.Type, rawProvider.Extends, baseRaw.Type)
	}
	if err := validateDerivedProviderSurface(rawProvider, syntaxByName[rawProvider.Name]); err != nil {
		return Provider{}, err
	}
	provider := cloneProvider(base)
	provider.Name = rawProvider.Name
	if rawProvider.DisplayName != "" {
		provider.DisplayName = rawProvider.DisplayName
	}
	provider.APIKey = rawProvider.APIKey // pragma: allowlist secret
	provider.APIKeyRef = nil             // pragma: allowlist secret
	if rawProvider.APIKeyRef != nil {    // pragma: allowlist secret
		provider.APIKeyRef = &APIKeyRef{Path: rawProvider.APIKeyRef.Path, Key: rawProvider.APIKeyRef.Key}
		if provider.APIKeyRef.Path == "" {
			provider.APIKeyRef.Path = defaultKeyFilePath()
		}
	}
	if err := validateProviderCredentialStructure(&provider); err != nil {
		return Provider{}, err
	}
	if err := resolveProviderCredential(&provider); err != nil {
		return Provider{}, err
	}
	return provider, nil
}

func validateDerivedProviderSurface(rawProvider rawProvider, syntax rawProviderSyntax) error {
	for _, name := range []string{"base_url", "upstream_header_timeout", "user_agent", "enabled"} {
		if syntax.Attrs[name] {
			return fmt.Errorf("derived provider cannot declare %s", name)
		}
	}
	if syntax.Blocks["model"] > 0 {
		return fmt.Errorf("derived provider cannot declare model blocks")
	}
	hasAPIKey := syntax.Attrs["api_key"]
	hasAPIKeyRef := syntax.Blocks["api_key_ref"] > 0
	if hasAPIKey && hasAPIKeyRef { // pragma: allowlist secret
		return fmt.Errorf("derived provider requires exactly one local credential: api_key or api_key_ref")
	}
	if !hasAPIKey && !hasAPIKeyRef && rawProvider.Type != string(ProviderTypeOpenCodeZen) {
		return fmt.Errorf("derived provider requires exactly one local credential: api_key or api_key_ref")
	}
	return nil
}

func buildLogging(rawLogging *rawLogging) (Logging, error) {
	out := Logging{Level: LogLevelInfo, AccessLog: true}
	if rawLogging == nil {
		return out, nil
	}
	if rawLogging.Level != "" {
		out.Level = LogLevel(rawLogging.Level)
	}
	if rawLogging.AccessLog != nil {
		out.AccessLog = *rawLogging.AccessLog
	}
	return out, nil
}

func buildProviderHealth(rawHealth *rawProviderHealth) (ProviderHealth, error) {
	out := ProviderHealth{RedisURL: rawHealth.RedisURL, KeyPrefix: rawHealth.KeyPrefix}
	if out.KeyPrefix == "" {
		out.KeyPrefix = "aiproxy:provider-health"
	}
	if rawHealth.Cooldown != "" {
		d, err := time.ParseDuration(rawHealth.Cooldown)
		if err != nil {
			return ProviderHealth{}, fmt.Errorf("provider_health.cooldown: %w", err)
		}
		out.Cooldown = d
	}
	if rawHealth.CacheTTL != "" {
		d, err := time.ParseDuration(rawHealth.CacheTTL)
		if err != nil {
			return ProviderHealth{}, fmt.Errorf("provider_health.cache_ttl: %w", err)
		}
		if d <= 0 {
			return ProviderHealth{}, fmt.Errorf("provider_health.cache_ttl must be positive")
		}
		out.CacheTTL = d
	}
	return out, nil
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
		client := Client{Name: c.Name, Token: c.Token, Tenant: c.Tenant, AllowedModels: make([]string, 0, len(c.AllowedModels))}
		client.AllowedModels = append(client.AllowedModels, c.AllowedModels...)
		auth.Clients[c.Name] = client
	}
	return auth, nil
}

func buildProvider(rawProvider rawProvider, rootUpstreamHeaderTimeout time.Duration) (Provider, error) {
	provider := Provider{
		Type:                  ProviderType(rawProvider.Type),
		Name:                  rawProvider.Name,
		DisplayName:           rawProvider.DisplayName,
		BaseURL:               rawProvider.BaseURL,
		UpstreamHeaderTimeout: rootUpstreamHeaderTimeout,
		UserAgent:             rawProvider.UserAgent,
		APIKey:                rawProvider.APIKey,
		Enabled:               true,
		ModelByName:           make(map[string]Model),
	}
	if rawProvider.Enabled != nil {
		provider.Enabled = *rawProvider.Enabled
	}
	if rawProvider.UpstreamHeaderTimeout != "" {
		d, err := parsePositiveDuration("upstream_header_timeout", rawProvider.UpstreamHeaderTimeout)
		if err != nil {
			return Provider{}, err
		}
		provider.UpstreamHeaderTimeout = d
	}
	if rawProvider.APIKeyRef != nil { // pragma: allowlist secret
		provider.APIKeyRef = &APIKeyRef{Path: rawProvider.APIKeyRef.Path, Key: rawProvider.APIKeyRef.Key}
		if provider.APIKeyRef.Path == "" {
			provider.APIKeyRef.Path = defaultKeyFilePath()
		}
	}
	if err := validateProviderCredentialStructure(&provider); err != nil {
		return Provider{}, err
	}
	if provider.Enabled {
		if err := resolveProviderCredential(&provider); err != nil {
			return Provider{}, err
		}
	}
	for _, m := range rawProvider.Models {
		if _, dup := provider.ModelByName[m.Name]; dup {
			return Provider{}, fmt.Errorf("duplicate model %q in provider %q", m.Name, rawProvider.Name)
		}
		upstream := m.UpstreamName
		if upstream == "" {
			upstream = m.Name
		}
		model := Model{Name: m.Name, DisplayName: m.DisplayName, UpstreamName: upstream, Protocol: ModelProtocol(m.Protocol), Capabilities: make([]Capability, 0, len(m.Capabilities))}
		for _, c := range m.Capabilities {
			model.Capabilities = append(model.Capabilities, Capability(c))
		}
		provider.Models = append(provider.Models, model)
		provider.ModelByName[m.Name] = model
	}
	return provider, nil
}

var defaultRetryStatusCodes = []int{500, 502, 503, 504}

func parseRetryStatusCodes(raw []string) ([]int, error) {
	if len(raw) == 0 {
		codes := make([]int, len(defaultRetryStatusCodes))
		copy(codes, defaultRetryStatusCodes)
		return codes, nil
	}
	codes := make([]int, 0, len(raw))
	for _, s := range raw {
		code, err := strconv.Atoi(s)
		if err != nil {
			return nil, fmt.Errorf("invalid retry status code %q: expected integer", s)
		}
		if !isRetryableStatusCode(code) {
			return nil, fmt.Errorf("invalid retry status code %q: must be between 400 and 599", s)
		}
		codes = append(codes, code)
	}
	return codes, nil
}

func isRetryableStatusCode(code int) bool {
	return code >= 400 && code <= 599
}
