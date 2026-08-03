package config

type rawFile struct {
	Listeners      []rawListener      `hcl:"listener,block"`
	Auth           []rawAuth          `hcl:"auth,block"`
	Logging        *rawLogging        `hcl:"logging,block"`
	ProviderHealth *rawProviderHealth `hcl:"provider_health,block"`
	Dashboard      []*rawDashboard    `hcl:"dashboard,block"`
	Providers      []rawProvider      `hcl:"provider,block"`
	Aliases        []rawAlias         `hcl:"alias,block"`
}

type rawDashboard struct {
	Token string `hcl:"token,optional"`
}

type rawLogging struct {
	Level     string `hcl:"level,optional"`
	AccessLog *bool  `hcl:"access_log,optional"`
}

type rawListener struct {
	Type     string       `hcl:"type,label"`
	Name     string       `hcl:"name,label"`
	Address  string       `hcl:"address"`
	Timeouts *rawTimeouts `hcl:"timeouts,block"`
}

type rawTimeouts struct {
	ReadHeader string `hcl:"read_header,optional"`
	Idle       string `hcl:"idle,optional"`
	Write      string `hcl:"write,optional"`
}

type rawAuth struct {
	Name      string        `hcl:"name,label"`
	Mode      string        `hcl:"mode"`
	Clients   []rawClient   `hcl:"client,block"`
	RateLimit *rawRateLimit `hcl:"rate_limit,block"`
}

type rawClient struct {
	Name          string   `hcl:"name,label"`
	Token         string   `hcl:"token"`
	Tenant        string   `hcl:"tenant,optional"`
	AllowedModels []string `hcl:"allowed_models,optional"`
}

type rawRateLimit struct {
	RequestsPerMinute int `hcl:"requests_per_minute"`
	Burst             int `hcl:"burst,optional"`
}

type rawProviderHealth struct {
	RedisURL  string `hcl:"redis_url,optional"`
	KeyPrefix string `hcl:"key_prefix,optional"`
	Cooldown  string `hcl:"cooldown,optional"`
}

type rawProvider struct {
	Type        string        `hcl:"type,label"`
	Name        string        `hcl:"name,label"`
	DisplayName string        `hcl:"display_name,optional"`
	BaseURL     string        `hcl:"base_url,optional"`
	APIKey      string        `hcl:"api_key,optional"`
	APIKeyRef   *rawAPIKeyRef `hcl:"api_key_ref,block"`
	Models      []rawModel    `hcl:"model,block"`
}

type rawAPIKeyRef struct {
	Path string `hcl:"path,optional"`
	Key  string `hcl:"key"`
}

type rawModel struct {
	Name         string   `hcl:"name,label"`
	DisplayName  string   `hcl:"display_name,optional"`
	UpstreamName string   `hcl:"upstream_name,optional"`
	Capabilities []string `hcl:"capabilities,optional"`
}

type rawAlias struct {
	Name             string      `hcl:"name,label"`
	Algorithm        string      `hcl:"algorithm"`
	RetryStatusCodes []string    `hcl:"retry_status_codes,optional"`
	Targets          []rawTarget `hcl:"target,block"`
}

type rawTarget struct {
	Provider string `hcl:"provider"`
	Model    string `hcl:"model"`
}
