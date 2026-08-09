package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/egose/aiproxy/internal/config"
)

type Operation int

const (
	OpChatCompletions Operation = iota
	OpEmbeddings
	OpResponses
	OpImagesGenerations
	OpAudioTranscriptions
	OpAudioSpeech
)

type Request struct {
	Operation     Operation
	ProviderType  config.ProviderType
	PublicModel   string
	BaseURL       string
	APIKey        string
	UpstreamModel string
	Body          []byte
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
	Usage      Usage
	Stream     *StreamCompletion
}

type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
	TotalTokens      int64
}

func (u Usage) Has() bool {
	return u.PromptTokens > 0 || u.CompletionTokens > 0 || u.TotalTokens > 0
}

type StreamCompletion struct {
	mu      sync.Mutex
	done    chan struct{}
	once    sync.Once
	outcome StreamOutcome
}

type StreamOutcome struct {
	Err                error
	DownstreamCanceled bool
	Usage              Usage
}

func NewStreamCompletion() *StreamCompletion {
	return &StreamCompletion{done: make(chan struct{})}
}

func (s *StreamCompletion) SetUsage(usage Usage) {
	if s == nil || !usage.Has() {
		return
	}
	s.mu.Lock()
	if usage.PromptTokens > 0 {
		s.outcome.Usage.PromptTokens = usage.PromptTokens
	}
	if usage.CompletionTokens > 0 {
		s.outcome.Usage.CompletionTokens = usage.CompletionTokens
	}
	if usage.TotalTokens > 0 {
		s.outcome.Usage.TotalTokens = usage.TotalTokens
	}
	if total := s.outcome.Usage.PromptTokens + s.outcome.Usage.CompletionTokens; total > s.outcome.Usage.TotalTokens {
		s.outcome.Usage.TotalTokens = total
	}
	s.mu.Unlock()
}

func (s *StreamCompletion) Complete(err error, downstreamCanceled bool) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		s.mu.Lock()
		s.outcome.Err = err
		s.outcome.DownstreamCanceled = downstreamCanceled
		s.mu.Unlock()
		close(s.done)
	})
}

func (s *StreamCompletion) Wait() StreamOutcome {
	if s == nil {
		return StreamOutcome{}
	}
	<-s.done
	s.mu.Lock()
	outcome := s.outcome
	s.mu.Unlock()
	return outcome
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
	case OpImagesGenerations:
		return "images_generations"
	case OpAudioTranscriptions:
		return "audio_transcriptions"
	case OpAudioSpeech:
		return "audio_speech"
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
	maxUpstreamBodyBytes    = 32 << 20
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

func readUpstreamBody(r io.Reader) ([]byte, error) {
	limited := &io.LimitedReader{R: r, N: maxUpstreamBodyBytes + 1}
	body, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(body) > maxUpstreamBodyBytes {
		return nil, fmt.Errorf("upstream response body exceeds %d bytes", maxUpstreamBodyBytes)
	}
	return body, nil
}

func requestBody(r Request) ([]byte, error) {
	if r.Body != nil {
		return r.Body, nil
	}
	if r.Inbound == nil || r.Inbound.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(r.Inbound.Body)
	if err != nil {
		return nil, err
	}
	r.Body = body
	return body, nil
}

type upstreamResponseHandlers struct {
	PreferStreaming bool
	StreamSuccess   bool
	IsStreaming     func(*http.Response) bool
	OnStream        func(*http.Response) (*Result, error)
	OnError         func(*http.Response, []byte) (*Result, error)
	OnSuccess       func(*http.Response, []byte) (*Result, error)
}

func executeUpstream(r Request, req *http.Request, handlers upstreamResponseHandlers) (*Result, error) {
	resp, err := clientFor(r).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	isStreaming := handlers.IsStreaming != nil && handlers.IsStreaming(resp)
	if handlers.PreferStreaming && isStreaming {
		return handlers.OnStream(resp)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, err := readUpstreamBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body: %w", err)
		}
		if handlers.OnError == nil {
			return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
		}
		return handlers.OnError(resp, body)
	}
	if isStreaming {
		return handlers.OnStream(resp)
	}
	if handlers.StreamSuccess {
		return &Result{StatusCode: resp.StatusCode, Header: resp.Header, StreamBody: resp.Body, Streaming: true}, nil
	}
	defer resp.Body.Close()
	body, err := readUpstreamBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	if handlers.OnSuccess == nil {
		return &Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}, nil
	}
	return handlers.OnSuccess(resp, body)
}
