package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"

	"github.com/egose/aiproxy/internal/config"
)

const openCodeSessionHeader = "x-opencode-session"

func (a *adapter) doOpenCode(ctx context.Context, r Request) (*Result, error) {
	switch r.ProviderType {
	case config.ProviderTypeOpenCodeZen, config.ProviderTypeOpenCodeGo:
	default:
		return nil, fmt.Errorf("unsupported provider type %q", r.ProviderType)
	}
	switch r.ModelProtocol {
	case config.ModelProtocolChat:
		if r.Operation != OpChatCompletions {
			return nil, ErrUnsupportedOperation{ProviderType: r.ProviderType, Operation: r.Operation}
		}
		return a.doOpenCodePassthrough(ctx, r, "/v1/chat/completions")
	case config.ModelProtocolResponses:
		if r.Operation != OpResponses {
			return nil, ErrUnsupportedOperation{ProviderType: r.ProviderType, Operation: r.Operation}
		}
		return a.doOpenCodePassthrough(ctx, r, "/v1/responses")
	case config.ModelProtocolMessages:
		switch r.Operation {
		case OpChatCompletions:
			return a.doOpenCodeMessagesChat(ctx, r)
		case OpResponses:
			return a.doOpenCodeMessagesResponses(ctx, r)
		default:
			return nil, ErrUnsupportedOperation{ProviderType: r.ProviderType, Operation: r.Operation}
		}
	case config.ModelProtocolGemini:
		if r.ProviderType != config.ProviderTypeOpenCodeZen {
			return nil, ErrUnsupportedOperation{ProviderType: r.ProviderType, Operation: r.Operation}
		}
		switch r.Operation {
		case OpChatCompletions:
			return a.doOpenCodeGeminiChat(ctx, r)
		case OpResponses:
			return a.doOpenCodeGeminiResponses(ctx, r)
		default:
			return nil, ErrUnsupportedOperation{ProviderType: r.ProviderType, Operation: r.Operation}
		}
	default:
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("unknown model protocol %q", r.ModelProtocol)}
	}
}

func (a *adapter) doOpenCodePassthrough(ctx context.Context, r Request, path string) (*Result, error) {
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	rewritten, err := rewriteModel(body, r.UpstreamModel)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("rewrite model: %v", err)}
	}
	target := joinBaseURLAndPath(r.BaseURL, path)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(rewritten))
	if err != nil {
		return nil, err
	}
	if err := applyOpenCodeHeaders(req, r); err != nil {
		return nil, err
	}
	return executeUpstream(r, req, openAIPassthroughHandlers(r, isStream(body)))
}

func (a *adapter) doOpenCodeMessagesChat(ctx context.Context, r Request) (*Result, error) {
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	translated, streaming, err := translateOpenAIToAnthropic(body, r.UpstreamModel)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	target := strings.TrimRight(r.BaseURL, "/") + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	if err := applyOpenCodeHeaders(req, r); err != nil {
		return nil, err
	}
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}
	return executeUpstream(r, req, upstreamResponseHandlers{
		IsStreaming: func(*http.Response) bool {
			return streaming
		},
		OnStream: func(resp *http.Response) (*Result, error) {
			stream := NewStreamCompletion()
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, StreamBody: translateAnthropicStream(resp.Body, r.PublicModel, stream), Streaming: true, Stream: stream}, nil
		},
		OnError: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translateAnthropicError(body)}, nil
		},
		OnSuccess: func(resp *http.Response, body []byte) (*Result, error) {
			translatedBody, usageTokens, err := translateAnthropicResponse(body, r.PublicModel)
			if err != nil {
				return nil, fmt.Errorf("translate response: %w", err)
			}
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translatedBody, Usage: usageTokens}, nil
		},
	})
}

func (a *adapter) doOpenCodeMessagesResponses(ctx context.Context, r Request) (*Result, error) {
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	chatReq, err := translateOpenAIResponsesInput(body)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	translated, streaming, err := translateOpenAIToAnthropic(mustMarshalJSON(chatReq), r.UpstreamModel)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	target := strings.TrimRight(r.BaseURL, "/") + "/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	if err := applyOpenCodeHeaders(req, r); err != nil {
		return nil, err
	}
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}
	return executeUpstream(r, req, upstreamResponseHandlers{
		IsStreaming: func(*http.Response) bool {
			return streaming
		},
		OnStream: func(resp *http.Response) (*Result, error) {
			stream := NewStreamCompletion()
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, StreamBody: translateAnthropicResponsesStream(resp.Body, r.PublicModel, stream), Streaming: true, Stream: stream}, nil
		},
		OnError: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translateAnthropicError(body)}, nil
		},
		OnSuccess: func(resp *http.Response, body []byte) (*Result, error) {
			translatedBody, err := translateAnthropicResponsesResponse(body, r.PublicModel)
			if err != nil {
				return nil, fmt.Errorf("translate response: %w", err)
			}
			usage := usageFromAnthropicBody(body)
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translatedBody, Usage: usage}, nil
		},
	})
}

func (a *adapter) doOpenCodeGeminiChat(ctx context.Context, r Request) (*Result, error) {
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	translated, streaming, err := translateOpenAIToGemini(body)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	req, err := newOpenCodeGeminiRequest(ctx, r, translated, streaming)
	if err != nil {
		return nil, err
	}
	return executeUpstream(r, req, upstreamResponseHandlers{
		IsStreaming: func(*http.Response) bool {
			return streaming
		},
		OnStream: func(resp *http.Response) (*Result, error) {
			stream := NewStreamCompletion()
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, StreamBody: translateGeminiStream(resp.Body, r.PublicModel, stream), Streaming: true, Stream: stream}, nil
		},
		OnError: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translateGeminiError(body)}, nil
		},
		OnSuccess: func(resp *http.Response, body []byte) (*Result, error) {
			translatedBody, err := translateGeminiResponse(body, r.PublicModel)
			if err != nil {
				return nil, fmt.Errorf("translate response: %w", err)
			}
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translatedBody, Usage: usageFromBody(body)}, nil
		},
	})
}

func (a *adapter) doOpenCodeGeminiResponses(ctx context.Context, r Request) (*Result, error) {
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	chatReq, err := translateOpenAIResponsesInput(body)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	translated, streaming, err := translateOpenAIToGemini(mustMarshalJSON(chatReq))
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}
	req, err := newOpenCodeGeminiRequest(ctx, r, translated, streaming)
	if err != nil {
		return nil, err
	}
	return executeUpstream(r, req, upstreamResponseHandlers{
		IsStreaming: func(*http.Response) bool {
			return streaming
		},
		OnStream: func(resp *http.Response) (*Result, error) {
			stream := NewStreamCompletion()
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, StreamBody: translateGeminiResponsesStream(resp.Body, r.PublicModel, stream), Streaming: true, Stream: stream}, nil
		},
		OnError: func(resp *http.Response, body []byte) (*Result, error) {
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translateGeminiError(body)}, nil
		},
		OnSuccess: func(resp *http.Response, body []byte) (*Result, error) {
			translatedBody, err := translateGeminiResponsesResponse(body, r.PublicModel)
			if err != nil {
				return nil, fmt.Errorf("translate response: %w", err)
			}
			return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translatedBody, Usage: usageFromBody(body)}, nil
		},
	})
}

func newOpenCodeGeminiRequest(ctx context.Context, r Request, translated []byte, streaming bool) (*http.Request, error) {
	path := "/models/" + r.UpstreamModel + ":generateContent"
	if streaming {
		path = "/models/" + r.UpstreamModel + ":streamGenerateContent?alt=sse"
	}
	target := strings.TrimRight(r.BaseURL, "/") + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	if err := applyOpenCodeHeaders(req, r); err != nil {
		return nil, err
	}
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}
	return req, nil
}

func applyOpenCodeHeaders(req *http.Request, r Request) error {
	req.Header.Set("Authorization", "Bearer "+r.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", openCodeUserAgent(r.Version))
	if r.ProviderType == config.ProviderTypeOpenCodeGo {
		session, err := openCodeSessionValue(r)
		if err != nil {
			return err
		}
		req.Header.Set(openCodeSessionHeader, session)
	}
	return nil
}

func openCodeUserAgent(version string) string {
	if version == "" {
		version = "dev"
	}
	return "aiproxy/" + version
}

func openCodeSessionValue(r Request) (string, error) {
	if r.Inbound != nil {
		if v := r.Inbound.Header.Get(openCodeSessionHeader); isValidOpenCodeSessionID(v) {
			return v, nil
		}
	}
	var entropy [16]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	return "ses_" + hex.EncodeToString(entropy[:]), nil
}

func isValidOpenCodeSessionID(s string) bool {
	if len(s) < 1 || len(s) > 128 {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '_' || c == '-' {
			continue
		}
		return false
	}
	return true
}
