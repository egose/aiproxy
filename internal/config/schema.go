package config

type rawFile struct {
	UpstreamHeaderTimeout string             `hcl:"upstream_header_timeout,optional"`
	Listeners             []rawListener      `hcl:"listener,block"`
	Auth                  []rawAuth          `hcl:"auth,block"`
	Logging               *rawLogging        `hcl:"logging,block"`
	ProviderHealth        *rawProviderHealth `hcl:"provider_health,block"`
	Metrics               []*rawMetrics      `hcl:"metrics,block"`
	Dashboard             []*rawDashboard    `hcl:"dashboard,block"`
	Providers             []rawProvider      `hcl:"provider,block"`
	Aliases               []rawAlias         `hcl:"alias,block"`
	providerSyntax        []rawProviderSyntax
}

type rawProviderSyntax struct {
	Attrs  map[string]bool
	Blocks map[string]int
}

type rawMetrics struct {
	Token string `hcl:"token,optional"`
}

type rawDashboard struct {
	Token               string `hcl:"token,optional"`
	AllowInsecureRemote *bool  `hcl:"allow_insecure_remote,optional"`
}

type rawLogging struct {
	Level      string         `hcl:"level,optional"`
	AccessLog  *bool          `hcl:"access_log,optional"`
	PayloadLog *rawPayloadLog `hcl:"payload_log,block"`
}

type rawPayloadLog struct {
	Enabled      *bool  `hcl:"enabled,optional"`
	Dir          string `hcl:"dir,optional"`
	Rotation     string `hcl:"rotation,optional"`
	Retention    string `hcl:"retention,optional"`
	MaxBodyBytes *int   `hcl:"max_body_bytes,optional"`
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
	CacheTTL  string `hcl:"cache_ttl,optional"`
}

type rawProvider struct {
	Type                  string            `hcl:"type,label"`
	Name                  string            `hcl:"name,label"`
	Extends               string            `hcl:"extends,optional"`
	DisplayName           string            `hcl:"display_name,optional"`
	BaseURL               string            `hcl:"base_url,optional"`
	UpstreamHeaderTimeout string            `hcl:"upstream_header_timeout,optional"`
	UserAgent             string            `hcl:"user_agent,optional"`
	APIKey                string            `hcl:"api_key,optional"`
	APIKeyRef             *rawAPIKeyRef     `hcl:"api_key_ref,block"`
	CredentialRef         *rawCredentialRef `hcl:"credential_ref,block"`
	Enabled               *bool             `hcl:"enabled,optional"`
	Healthcheck           *rawHealthcheck   `hcl:"healthcheck,block"`
	Models                []rawModel        `hcl:"model,block"`
}

type rawHealthcheck struct {
	Path              string `hcl:"path"`
	Method            string `hcl:"method,optional"`
	ExpectedStatus    *int   `hcl:"expected_status,optional"`
	ExpectedBody      string `hcl:"expected_body,optional"`
	Interval          string `hcl:"interval,optional"`
	Timeout           string `hcl:"timeout,optional"`
	FailureThreshold  *int   `hcl:"failure_threshold,optional"`
	SuccessThreshold  *int   `hcl:"success_threshold,optional"`
	SendAuthorization *bool  `hcl:"send_authorization,optional"`
}

type rawAPIKeyRef struct {
	Path string `hcl:"path,optional"`
	Key  string `hcl:"key"`
}

type rawCredentialRef struct {
	Path string `hcl:"path,optional"`
	Name string `hcl:"name"`
}

type rawModel struct {
	Name         string   `hcl:"name,label"`
	DisplayName  string   `hcl:"display_name,optional"`
	UpstreamName string   `hcl:"upstream_name,optional"`
	Protocol     string   `hcl:"protocol,optional"`
	Capabilities []string `hcl:"capabilities,optional"`
}

type rawAlias struct {
	Name             string              `hcl:"name,label"`
	Algorithm        string              `hcl:"algorithm"`
	RetryStatusCodes []string            `hcl:"retry_status_codes,optional"`
	SessionAffinity  *rawSessionAffinity `hcl:"session_affinity,block"`
	Targets          []rawTarget         `hcl:"target,block"`
}

type rawSessionAffinity struct {
	Headers []string `hcl:"headers,optional"`
}

type rawTarget struct {
	Provider string `hcl:"provider"`
	Model    string `hcl:"model"`
}
