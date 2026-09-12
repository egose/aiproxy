package config

import "time"

const DefaultUpstreamHeaderTimeout = 90 * time.Second

type Runtime struct {
	Listener              Listener
	Auth                  Auth
	Logging               Logging
	ProviderHealth        ProviderHealth
	Metrics               Metrics
	Dashboard             Dashboard
	UpstreamHeaderTimeout time.Duration
	Catalog               Catalog
}

type Metrics struct {
	Token   string
	Enabled bool
}

type Dashboard struct {
	Token                 string
	AllowInsecureRemote   bool
	ExplicitAllowInsecure bool
	TokenFromConfig       bool
	Enabled               bool
}

type Listener struct {
	Name    string
	Address string

	Timeouts Timeouts
}

type Timeouts struct {
	ReadHeader time.Duration
	Idle       time.Duration
	Write      time.Duration
}

type LogLevel string

const (
	LogLevelDebug LogLevel = "debug"
	LogLevelInfo  LogLevel = "info"
	LogLevelWarn  LogLevel = "warn"
	LogLevelError LogLevel = "error"
)

type Logging struct {
	Level     LogLevel
	AccessLog bool
}

type AuthMode string

const (
	AuthModeNone         AuthMode = "none"
	AuthModeBearerStatic AuthMode = "bearer_static"
)

type Auth struct {
	Name      string
	Mode      AuthMode
	Clients   map[string]Client
	RateLimit *RateLimit
}

type Client struct {
	Name          string
	Token         string
	Tenant        string
	AllowedModels []string
}

type RateLimit struct {
	RequestsPerMinute int
	Burst             int
}

type ProviderHealth struct {
	RedisURL  string
	KeyPrefix string
	Cooldown  time.Duration
	CacheTTL  time.Duration
}

type ProviderType string

const (
	ProviderTypeOpenAI           ProviderType = "openai"
	ProviderTypeOpenAICompatible ProviderType = "openai-compatible"
	ProviderTypeAnthropic        ProviderType = "anthropic"
	ProviderTypeGemini           ProviderType = "gemini"
	ProviderTypeOpenCodeZen      ProviderType = "opencode-zen"
	ProviderTypeOpenCodeGo       ProviderType = "opencode-go"
	ProviderTypeGitHubCopilot    ProviderType = "github-copilot"
)

type ModelProtocol string

const (
	ModelProtocolChat      ModelProtocol = "chat"
	ModelProtocolResponses ModelProtocol = "responses"
	ModelProtocolMessages  ModelProtocol = "messages"
	ModelProtocolGemini    ModelProtocol = "gemini"
)

type Capability string

const (
	CapabilityChat                Capability = "chat"
	CapabilityResponses           Capability = "responses"
	CapabilityEmbeddings          Capability = "embeddings"
	CapabilityImages              Capability = "images"
	CapabilityAudioTranscriptions Capability = "audio_transcriptions"
	CapabilityAudioSpeech         Capability = "audio_speech"
)

type Algorithm string

const (
	AlgorithmRoundRobin       Algorithm = "round_robin"
	AlgorithmLeastConnections Algorithm = "least_connections"
)

type Provider struct {
	Type                  ProviderType
	Name                  string
	DisplayName           string
	BaseURL               string
	UpstreamHeaderTimeout time.Duration
	UserAgent             string
	APIKey                string
	APIKeyRef             *APIKeyRef
	CopilotCredentialRef  *CopilotCredentialRef
	CopilotToken          string
	Enabled               bool
	Models                []Model
	Healthcheck           *ProviderHealthcheck

	ModelByName map[string]Model
}

type APIKeyRef struct {
	Path     string
	Key      string
	Resolved bool
}

type CopilotCredentialRef struct {
	Path     string
	Name     string
	Resolved bool
}

type ProviderHealthcheck struct {
	Path              string
	Method            string
	ExpectedStatus    int
	ExpectedBody      string
	Interval          time.Duration
	Timeout           time.Duration
	FailureThreshold  int
	SuccessThreshold  int
	SendAuthorization bool
}

type Model struct {
	Name         string
	DisplayName  string
	UpstreamName string
	Protocol     ModelProtocol
	Capabilities []Capability
}

type Alias struct {
	Name             string
	Algorithm        Algorithm
	RetryStatusCodes []int
	Targets          []AliasTarget
}

type AliasTarget struct {
	Provider string
	Model    string
}
