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
	IngressGuardrails     IngressGuardrails
	UpstreamHeaderTimeout time.Duration
	UserAgent             string
	ForwardUserAgent      bool
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

type GuardrailMode string

const (
	GuardrailModeAudit GuardrailMode = "audit"
	GuardrailModeBlock GuardrailMode = "block"
)

type IngressGuardrails struct {
	Enabled           bool
	Mode              GuardrailMode
	MaxTextBytes      int
	MaxStrings        int
	ExceptionsFile    string
	RedactPlaceholder string
	Quarantine        GuardrailQuarantine
}

type GuardrailQuarantine struct {
	Enabled    bool
	MaxEntries int
	TTL        time.Duration
	MaxSnippet int
	HasTTL     bool
	HasMax     bool
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
	Level      LogLevel
	AccessLog  bool
	PayloadLog PayloadLog
}

type PayloadLogRotation string

const (
	PayloadLogRotationDaily  PayloadLogRotation = "daily"
	PayloadLogRotationHourly PayloadLogRotation = "hourly"
)

const (
	DefaultPayloadLogRotation   = PayloadLogRotationDaily
	DefaultPayloadLogRetention  = 7 * 24 * time.Hour
	DefaultPayloadLogMaxBody    = 1 << 20
	PayloadLogFilePrefix        = "payload-"
	PayloadLogFileExt           = ".jsonl"
	PayloadLogRetentionDisabled = time.Duration(0)

	DefaultPayloadMongoDatabase   = "aiproxy"
	DefaultPayloadMongoCollection = "payloads"
	DefaultPayloadMongoTimeout    = 5 * time.Second
)

type PayloadLog struct {
	Enabled      bool
	Dir          string
	Rotation     PayloadLogRotation
	Retention    time.Duration
	MaxBodyBytes int
	HasRetention bool
	HasMaxBody   bool
	Mongo        PayloadMongo
}

type PayloadMongo struct {
	URI        string
	Database   string
	Collection string
	Timeout    time.Duration
	HasTimeout bool
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
	ForwardUserAgent      bool
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
	Pricing      *ModelPricing
}

type ModelPricing struct {
	InputPerMillion      float64
	OutputPerMillion     float64
	CachedPerMillion     float64
	CacheWritePerMillion float64
}

func (p *ModelPricing) HasRates() bool {
	return p != nil && (p.InputPerMillion > 0 || p.OutputPerMillion > 0 || p.CachedPerMillion > 0 || p.CacheWritePerMillion > 0)
}

func (p *ModelPricing) Cost(promptTokens, completionTokens, cachedTokens, cacheWriteTokens, cacheReadTokens int64) (float64, bool) {
	if !p.HasRates() {
		return 0, false
	}
	cached := cachedTokens
	if cached < 0 {
		cached = 0
	}
	if cached > promptTokens {
		cached = promptTokens
	}
	regular := promptTokens - cached
	if regular < 0 {
		regular = 0
	}
	writeShare := cacheWriteTokens
	if writeShare < 0 {
		writeShare = 0
	}
	if writeShare > cached {
		writeShare = cached
	}
	readShare := cacheReadTokens
	if readShare < 0 {
		readShare = 0
	}
	if writeShare+readShare > cached {
		if writeShare >= cached {
			writeShare, readShare = cached, 0
		} else {
			readShare = cached - writeShare
		}
	}
	genericCached := cached - writeShare - readShare
	if genericCached < 0 {
		genericCached = 0
	}
	cachedReadRate := p.CachedPerMillion
	if cachedReadRate == 0 {
		cachedReadRate = p.InputPerMillion
	}
	cacheWriteRate := p.CacheWritePerMillion
	if cacheWriteRate == 0 {
		cacheWriteRate = p.InputPerMillion
	}
	cost := float64(regular)/1e6*p.InputPerMillion +
		float64(completionTokens)/1e6*p.OutputPerMillion +
		float64(genericCached)/1e6*p.CachedPerMillion +
		float64(writeShare)/1e6*cacheWriteRate +
		float64(readShare)/1e6*cachedReadRate
	return cost, true
}

type Alias struct {
	Name               string
	Algorithm          Algorithm
	RetryStatusCodes   []int
	SessionAffinity    *SessionAffinity
	EncryptedReasoning *EncryptedReasoning
	Targets            []AliasTarget
}

const (
	EncryptedReasoningFail          = "fail"
	EncryptedReasoningStripAndRetry = "strip_and_retry"
)

type EncryptedReasoning struct {
	Passthrough      bool
	OnCallerMismatch string
	MatchMessages    []string
}

var DefaultEncryptedReasoningMatchMessages = []string{
	"not issued to this caller",
	"invalid_encrypted_content",
	"could not be verified",
	"could not be decrypted",
	"item_id did not match",
	"invalid signature",
}

func EncryptedReasoningMatchMessages(a Alias) []string {
	if a.EncryptedReasoning == nil || len(a.EncryptedReasoning.MatchMessages) == 0 {
		return append([]string(nil), DefaultEncryptedReasoningMatchMessages...)
	}
	return append([]string(nil), a.EncryptedReasoning.MatchMessages...)
}

type SessionAffinity struct {
	Headers []string
}

var DefaultSessionAffinityHeaders = []string{
	"x-opencode-session",
	"x-session-affinity",
	"x-session-id",
	"x-opencode-session-id",
	"x-claude-code-session-id",
	"session-id",
	"thread-id",
	"x-codex-window-id",
	"x-client-request-id",
}

func SessionAffinityHeaders(a Alias) []string {
	if a.SessionAffinity == nil {
		return nil
	}
	if len(a.SessionAffinity.Headers) == 0 {
		return append([]string(nil), DefaultSessionAffinityHeaders...)
	}
	return append([]string(nil), a.SessionAffinity.Headers...)
}

type AliasTarget struct {
	Provider string
	Model    string
}
