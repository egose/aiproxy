package config

import "time"

type Runtime struct {
	Listener          Listener
	Auth              Auth
	Providers         []Provider
	DisabledProviders []Provider
	Aliases           []Alias

	ProviderByName map[string]Provider
	AliasByName    map[string]Alias
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
	Name  string
	Token string
}

type RateLimit struct {
	RequestsPerMinute int
	Burst             int
}

type ProviderType string

const (
	ProviderTypeOpenAI           ProviderType = "openai"
	ProviderTypeOpenAICompatible ProviderType = "openai-compatible"
	ProviderTypeAnthropic        ProviderType = "anthropic"
	ProviderTypeGemini           ProviderType = "gemini"
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
	Type        ProviderType
	Name        string
	DisplayName string
	BaseURL     string
	APIKey      string
	APIKeyRef   *APIKeyRef
	Models      []Model

	ModelByName map[string]Model
}

type APIKeyRef struct {
	Path     string
	Key      string
	Resolved bool
}

type Model struct {
	Name         string
	DisplayName  string
	UpstreamName string
	Capabilities []Capability
}

type Alias struct {
	Name      string
	Algorithm Algorithm
	Targets   []AliasTarget
}

type AliasTarget struct {
	Provider string
	Model    string
}
