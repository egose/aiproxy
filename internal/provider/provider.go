package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/egose/aiproxy/internal/config"
)

type Operation int

const (
	OpChatCompletions Operation = iota
	OpEmbeddings
	OpResponses
)

type Request struct {
	Operation     Operation
	ProviderType  config.ProviderType
	PublicModel   string
	BaseURL       string
	APIKey        string
	UpstreamModel string
	Inbound       *http.Request
	Client        *http.Client
}

type Result struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	StreamBody io.ReadCloser
	Streaming  bool
	OnClose    func()
}

type Adapter interface {
	Do(ctx context.Context, r Request) (*Result, error)
}

func New() Adapter {
	return &adapter{}
}

type adapter struct{}

type ErrUnsupportedOperation struct {
	ProviderType config.ProviderType
	Operation    Operation
}

func (e ErrUnsupportedOperation) Error() string {
	return fmt.Sprintf("provider type %q does not support operation %q", e.ProviderType, e.Operation)
}

type ErrInvalidRequest struct {
	Message string
}

func (e ErrInvalidRequest) Error() string {
	if e.Message == "" {
		return "invalid request"
	}
	return e.Message
}

func (o Operation) String() string {
	switch o {
	case OpChatCompletions:
		return "chat_completions"
	case OpEmbeddings:
		return "embeddings"
	case OpResponses:
		return "responses"
	default:
		return "unknown"
	}
}

const (
	defaultOpenAIBaseURL    = "https://api.openai.com"
	defaultAnthropicBaseURL = "https://api.anthropic.com"
	defaultGeminiBaseURL    = "https://generativelanguage.googleapis.com"
	anthropicVersion        = "2023-06-01"
	defaultMaxTokens        = 1024
)

func (a *adapter) Do(ctx context.Context, r Request) (*Result, error) {
	if r.ProviderType == "" {
		r.ProviderType = config.ProviderTypeOpenAI
	}
	if r.Client == nil {
		r.Client = http.DefaultClient
	}

	switch r.ProviderType {
	case config.ProviderTypeOpenAI, config.ProviderTypeOpenAICompatible:
		return a.doOpenAI(ctx, r)
	case config.ProviderTypeAnthropic:
		return a.doAnthropic(ctx, r)
	case config.ProviderTypeGemini:
		return a.doGemini(ctx, r)
	default:
		return nil, fmt.Errorf("unsupported provider type %q", r.ProviderType)
	}
}

func clientFor(r Request) *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return http.DefaultClient
}

func stringPtr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
