package provider

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/textproto"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/klauspost/compress/zstd"
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
	Operation        Operation
	ProviderType     config.ProviderType
	PublicModel      string
	BaseURL          string
	APIKey           string
	CopilotToken     string
	UpstreamModel    string
	ModelProtocol    config.ModelProtocol
	UserAgent        string
	ForwardUserAgent bool
	ForwardHeaders   []string
	Version          string
	Body             []byte
	Inbound          *http.Request
	Client           *http.Client
}

type Result struct {
	StatusCode             int
	Header                 http.Header
	Body                   []byte
	StreamBody             io.ReadCloser
	Streaming              bool
	OnClose                func()
	Usage                  Usage
	Stream                 *StreamCompletion
	RetryDelay             time.Duration
	HasRetryDelay          bool
	Provider               string
	UpstreamModel          string
	UpstreamRequestHeaders http.Header
	UpstreamRequestBody    []byte
}

type Usage struct {
	PromptTokens        int64
	CompletionTokens    int64
	TotalTokens         int64
	CachedTokens        int64
	CacheCreationTokens int64
	CacheReadTokens     int64
}

func (u Usage) Has() bool {
	return u.PromptTokens > 0 || u.CompletionTokens > 0 || u.TotalTokens > 0 || u.CachedTokens > 0
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
	if usage.CachedTokens > 0 {
		s.outcome.Usage.CachedTokens = usage.CachedTokens
	}
	if usage.CacheCreationTokens > 0 {
		s.outcome.Usage.CacheCreationTokens = usage.CacheCreationTokens
	}
	if usage.CacheReadTokens > 0 {
		s.outcome.Usage.CacheReadTokens = usage.CacheReadTokens
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

type providerDescriptor struct {
	providerType   config.ProviderType
	defaultBaseURL string
	do             func(*adapter, context.Context, Request) (*Result, error)
}

var providerDescriptors = map[config.ProviderType]providerDescriptor{
	config.ProviderTypeOpenAI: {
		providerType:   config.ProviderTypeOpenAI,
		defaultBaseURL: defaultOpenAIBaseURL,
		do:             (*adapter).doOpenAI,
	},
	config.ProviderTypeOpenAICompatible: {
		providerType:   config.ProviderTypeOpenAICompatible,
		defaultBaseURL: defaultOpenAIBaseURL,
		do:             (*adapter).doOpenAI,
	},
	config.ProviderTypeAnthropic: {
		providerType:   config.ProviderTypeAnthropic,
		defaultBaseURL: defaultAnthropicBaseURL,
		do:             (*adapter).doAnthropic,
	},
	config.ProviderTypeGemini: {
		providerType:   config.ProviderTypeGemini,
		defaultBaseURL: defaultGeminiBaseURL,
		do:             (*adapter).doGemini,
	},
	config.ProviderTypeOpenCodeZen: {
		providerType:   config.ProviderTypeOpenCodeZen,
		defaultBaseURL: defaultOpenCodeZenBaseURL,
		do:             (*adapter).doOpenCode,
	},
	config.ProviderTypeOpenCodeGo: {
		providerType:   config.ProviderTypeOpenCodeGo,
		defaultBaseURL: defaultOpenCodeGoBaseURL,
		do:             (*adapter).doOpenCode,
	},
	config.ProviderTypeGitHubCopilot: {
		providerType:   config.ProviderTypeGitHubCopilot,
		defaultBaseURL: copilotlogin.DefaultBaseURL,
		do:             (*adapter).doGitHubCopilot,
	},
	config.ProviderTypeZenMux: {
		providerType:   config.ProviderTypeZenMux,
		defaultBaseURL: defaultZenMuxBaseURL,
		do:             (*adapter).doOpenAI,
	},
}

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
	for _, desc := range operationDescriptors {
		if desc.operation == o {
			return desc.name
		}
	}
	return "unknown"
}

const (
	defaultOpenAIBaseURL      = "https://api.openai.com"
	defaultAnthropicBaseURL   = "https://api.anthropic.com"
	defaultGeminiBaseURL      = "https://generativelanguage.googleapis.com"
	defaultOpenCodeZenBaseURL = "https://opencode.ai/zen/v1"
	defaultOpenCodeGoBaseURL  = "https://opencode.ai/zen/go/v1"
	defaultZenMuxBaseURL      = "https://zenmux.ai/api/v1"
	anthropicVersion          = "2023-06-01"
	defaultMaxTokens          = 1024
	maxUpstreamBodyBytes      = 32 << 20
)

func (a *adapter) Do(ctx context.Context, r Request) (*Result, error) {
	if r.ProviderType == "" {
		r.ProviderType = config.ProviderTypeOpenAI
	}
	if r.Client == nil {
		r.Client = http.DefaultClient
	}
	desc, ok := providerDescriptors[r.ProviderType]
	if !ok {
		return nil, fmt.Errorf("unsupported provider type %q", r.ProviderType)
	}
	if r.BaseURL == "" {
		r.BaseURL = desc.defaultBaseURL
	}
	return desc.do(a, ctx, r)
}

type operationDescriptor struct {
	operation  Operation
	name       string
	path       string
	capability config.Capability
}

var operationDescriptors = []operationDescriptor{
	{operation: OpChatCompletions, name: "chat_completions", path: "/v1/chat/completions", capability: config.CapabilityChat},
	{operation: OpEmbeddings, name: "embeddings", path: "/v1/embeddings", capability: config.CapabilityEmbeddings},
	{operation: OpResponses, name: "responses", path: "/v1/responses", capability: config.CapabilityResponses},
	{operation: OpImagesGenerations, name: "images_generations", path: "/v1/images/generations", capability: config.CapabilityImages},
	{operation: OpAudioTranscriptions, name: "audio_transcriptions", path: "/v1/audio/transcriptions", capability: config.CapabilityAudioTranscriptions},
	{operation: OpAudioSpeech, name: "audio_speech", path: "/v1/audio/speech", capability: config.CapabilityAudioSpeech},
}

func OperationForHTTP(method, path string) (Operation, bool) {
	if method != http.MethodPost {
		return 0, false
	}
	for _, desc := range operationDescriptors {
		if desc.path == path {
			return desc.operation, true
		}
	}
	return 0, false
}

func RequiredCapability(op Operation) (config.Capability, bool) {
	for _, desc := range operationDescriptors {
		if desc.operation == op {
			return desc.capability, true
		}
	}
	return "", false
}

func openAIPathForOperation(op Operation) (string, error) {
	for _, desc := range operationDescriptors {
		if desc.operation == op {
			return desc.path, nil
		}
	}
	return "", ErrUnsupportedOperation{ProviderType: config.ProviderTypeOpenAICompatible, Operation: op}
}

func EffectiveBaseURL(providerType config.ProviderType, baseURL string) string {
	if baseURL != "" {
		return baseURL
	}
	if desc, ok := providerDescriptors[providerType]; ok {
		return desc.defaultBaseURL
	}
	return ""
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

const inspectDecodeMaxBytes = 1 << 20

func DecodeBodyForInspection(header http.Header, body []byte) []byte {
	if len(body) == 0 || len(header.Get("Content-Encoding")) == 0 {
		return body
	}
	encodings := parseContentEncodings(header.Get("Content-Encoding"))
	if len(encodings) == 0 {
		return body
	}
	decoded := body
	for i := len(encodings) - 1; i >= 0; i-- {
		next, ok := decodeContentEncoding(encodings[i], decoded)
		if !ok {
			return body
		}
		decoded = next
	}
	return decoded
}

func parseContentEncodings(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		token := strings.ToLower(strings.TrimSpace(p))
		if token == "" || token == "identity" {
			continue
		}
		out = append(out, token)
	}
	return out
}

func decodeContentEncoding(encoding string, body []byte) ([]byte, bool) {
	var reader io.Reader
	switch encoding {
	case "gzip", "x-gzip":
		r, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, false
		}
		defer r.Close()
		reader = r
	case "deflate":
		r := flate.NewReader(bytes.NewReader(body))
		defer r.Close()
		reader = r
	case "br":
		reader = brotli.NewReader(bytes.NewReader(body))
	case "zstd":
		r, err := zstd.NewReader(nil)
		if err != nil {
			return nil, false
		}
		defer r.Close()
		decoded, err := r.DecodeAll(body, nil)
		if err != nil {
			return nil, false
		}
		return capInspectDecoded(decoded), true
	default:
		return nil, false
	}
	decoded, err := io.ReadAll(io.LimitReader(reader, inspectDecodeMaxBytes))
	if err != nil {
		return nil, false
	}
	return decoded, true
}

func capInspectDecoded(decoded []byte) []byte {
	if len(decoded) > inspectDecodeMaxBytes {
		return decoded[:inspectDecodeMaxBytes]
	}
	return decoded
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

func upstreamUserAgent(r Request) string {
	if r.UserAgent != "" {
		return r.UserAgent
	}
	if r.ForwardUserAgent && r.Inbound != nil {
		if ua := r.Inbound.Header.Get("User-Agent"); isForwardableUserAgent(ua) {
			return ua
		}
	}
	return "aiproxy/" + defaultVersion(r.Version)
}

func isForwardableUserAgent(s string) bool {
	if s == "" || len(s) > 256 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

func defaultVersion(version string) string {
	if version == "" {
		return "dev"
	}
	return version
}

func executeUpstream(r Request, req *http.Request, handlers upstreamResponseHandlers) (*Result, error) {
	req.Header.Set("User-Agent", upstreamUserAgent(r))
	applyForwardedHeaders(r, req)
	sentHeaders := req.Header.Clone()
	sentBody, err := readUpstreamRequestBody(req)
	if err != nil {
		return nil, fmt.Errorf("read upstream request body: %w", err)
	}
	attachSent := func(res *Result) *Result {
		if res != nil {
			res.UpstreamRequestHeaders = sentHeaders
			res.UpstreamRequestBody = sentBody
		}
		return res
	}
	resp, err := clientFor(r).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	delay, hasDelay := ParseRetryCooldown(resp.Header, time.Now())
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		body, err := readUpstreamBody(resp.Body)
		if err != nil {
			return nil, withCooldownError(fmt.Errorf("read error body: %w", err), delay, hasDelay)
		}
		if handlers.OnError == nil {
			return attachCooldownDelay(attachSent(&Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}), delay, hasDelay), nil
		}
		res, handlerErr := handlers.OnError(resp, body)
		return attachCooldownDelay(attachSent(res), delay, hasDelay), withCooldownError(handlerErr, delay, hasDelay)
	}
	isStreaming := handlers.IsStreaming != nil && handlers.IsStreaming(resp)
	if handlers.PreferStreaming && isStreaming {
		res, streamErr := handlers.OnStream(resp)
		return attachCooldownDelay(attachSent(res), delay, hasDelay), withCooldownError(streamErr, delay, hasDelay)
	}
	if isStreaming {
		res, streamErr := handlers.OnStream(resp)
		return attachCooldownDelay(attachSent(res), delay, hasDelay), withCooldownError(streamErr, delay, hasDelay)
	}
	if handlers.StreamSuccess {
		return attachCooldownDelay(attachSent(&Result{StatusCode: resp.StatusCode, Header: resp.Header, StreamBody: resp.Body, Streaming: true}), delay, hasDelay), nil
	}
	defer resp.Body.Close()
	body, err := readUpstreamBody(resp.Body)
	if err != nil {
		return nil, withCooldownError(fmt.Errorf("read upstream body: %w", err), delay, hasDelay)
	}
	if handlers.OnSuccess == nil {
		return attachCooldownDelay(attachSent(&Result{StatusCode: resp.StatusCode, Header: resp.Header, Body: body}), delay, hasDelay), nil
	}
	res, handlerErr := handlers.OnSuccess(resp, body)
	return attachCooldownDelay(attachSent(res), delay, hasDelay), withCooldownError(handlerErr, delay, hasDelay)
}

const (
	maxForwardedHeaderValueBytes = 4096
	maxForwardedHeaderValues     = 32
)

func applyForwardedHeaders(r Request, req *http.Request) {
	if r.Inbound == nil || len(r.ForwardHeaders) == 0 {
		return
	}
	for _, name := range r.ForwardHeaders {
		if req.Header.Get(name) != "" {
			continue
		}
		var kept []string
		for _, v := range r.Inbound.Header.Values(name) {
			if !isForwardableHeaderValue(v) {
				continue
			}
			kept = append(kept, v)
			if len(kept) >= maxForwardedHeaderValues {
				break
			}
		}
		if len(kept) == 0 {
			continue
		}
		cp := make([]string, len(kept))
		copy(cp, kept)
		req.Header[textproto.CanonicalMIMEHeaderKey(name)] = cp
	}
}

func isForwardableHeaderValue(v string) bool {
	if v == "" || len(v) > maxForwardedHeaderValueBytes {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == 0x09 || c >= 0x20 && c <= 0x7e || c >= 0x80 {
			continue
		}
		return false
	}
	return true
}

func readUpstreamRequestBody(req *http.Request) ([]byte, error) {
	if req.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(req.Body)
	_ = req.Body.Close()
	if err != nil {
		return nil, err
	}
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.GetBody = func() (io.ReadCloser, error) {
		return io.NopCloser(bytes.NewReader(body)), nil
	}
	req.ContentLength = int64(len(body))
	return body, nil
}
