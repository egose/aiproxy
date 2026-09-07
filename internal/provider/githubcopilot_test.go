package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
)

type copilotCapture struct {
	path    string
	host    string
	headers http.Header
	body    string
	calls   int
}

func newCopilotStub(t *testing.T, cap *copilotCapture, status int, payload, contentType string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.calls++
		cap.path = r.URL.Path
		cap.host = r.URL.Host
		cap.headers = r.Header.Clone()
		b, _ := io.ReadAll(r.Body)
		cap.body = string(b)
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = io.WriteString(w, payload)
	}))
}

func copilotInbound(body string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(body)))
	req.Header.Set("Authorization", "Bearer inbound-caller-token")
	req.Header.Set("Cookie", "session=abc")
	req.Header.Set("x-api-key", "caller-key")
	req.Header.Set("x-initiator", "agent")
	req.Header.Set("Copilot-Vision-Request", "true")
	req.Header.Set("X-Interaction-Id", "evil-id")
	req.Header.Set("X-Interaction-Type", "evil-type")
	req.Header.Set("Openai-Intent", "caller-intent")
	req.Header.Set("X-GitHub-Api-Version", "1900-01-01")
	return req
}

func TestGitHubCopilotChatPathBearerAndHeaders(t *testing.T) {
	var cap copilotCapture
	upstream := newCopilotStub(t, &cap, http.StatusOK, `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`, "application/json")
	defer upstream.Close()

	_, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		PublicModel:   "copilot/gpt-4o-mini",
		BaseURL:       upstream.URL,
		APIKey:        "sk-must-not-be-used",
		CopilotToken:  "gho_copilot-token",
		UpstreamModel: "gpt-4o-2024-08-06",
		Version:       "test",
		Inbound:       copilotInbound(`{"model":"copilot/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`),
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if cap.calls != 1 {
		t.Fatalf("calls = %d", cap.calls)
	}
	if cap.path != "/chat/completions" {
		t.Fatalf("path = %q, want /chat/completions", cap.path)
	}
	if got := cap.headers.Get("Authorization"); got != "Bearer gho_copilot-token" {
		t.Fatalf("auth = %q", got)
	}
	if strings.Contains(cap.headers.Get("Authorization"), "sk-must-not-be-used") || strings.Contains(cap.headers.Get("Authorization"), "inbound") {
		t.Fatalf("wrong credential forwarded: %q", cap.headers.Get("Authorization"))
	}
	if got := cap.headers.Get("User-Agent"); got != "aiproxy/test" {
		t.Fatalf("user-agent = %q", got)
	}
	if got := cap.headers.Get(copilotlogin.APIVersionHeader); got != copilotlogin.APIVersion {
		t.Fatalf("api version = %q", got)
	}
	if got := cap.headers.Get(copilotlogin.IntentHeader); got != copilotlogin.OpenAIIntent {
		t.Fatalf("intent = %q", got)
	}
	if got := cap.headers.Get(copilotlogin.InitiatorHeader); got != copilotlogin.InitiatorUser {
		t.Fatalf("initiator = %q", got)
	}
	if got := cap.headers.Get(copilotlogin.VisionHeader); got != "" {
		t.Fatalf("vision must be omitted without image parts, got %q", got)
	}
	if got := cap.headers.Get("Cookie"); got != "" {
		t.Fatalf("cookie forwarded: %q", got)
	}
	if got := cap.headers.Get("X-Api-Key"); got != "" {
		t.Fatalf("x-api-key forwarded: %q", got)
	}
	if got := cap.headers.Get("X-Interaction-Id"); got != "" {
		t.Fatalf("interaction id forwarded: %q", got)
	}
	if !strings.Contains(cap.body, `"model":"gpt-4o-2024-08-06"`) {
		t.Fatalf("model was not rewritten: %s", cap.body)
	}
}

func TestGitHubCopilotDefaultOriginAndPath(t *testing.T) {
	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"copilot/m","messages":[]}`)))
	_, err := a.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		CopilotToken:  "gho_tok",
		UpstreamModel: "m",
		Version:       "test",
		Inbound:       inbound,
		Client: &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
			if r.URL.Scheme+"://"+r.URL.Host != copilotlogin.DefaultBaseURL {
				t.Fatalf("origin = %q, want %q", r.URL.Scheme+"://"+r.URL.Host, copilotlogin.DefaultBaseURL)
			}
			if r.URL.Path != copilotlogin.ChatCompletionsPath {
				t.Fatalf("path = %q, want %q", r.URL.Path, copilotlogin.ChatCompletionsPath)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"id":"1"}`)),
				Request:    r,
			}, nil
		})},
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
}

func TestGitHubCopilotVisionDerivedServerSide(t *testing.T) {
	var cap copilotCapture
	upstream := newCopilotStub(t, &cap, http.StatusOK, `{"id":"1"}`, "application/json")
	defer upstream.Close()

	body := `{"model":"copilot/m","messages":[{"role":"user","content":[{"type":"text","text":"what"},{"type":"image_url","image_url":{"url":"data:image/png;base64,xx"}}]}]}`
	inbound := copilotInbound(body)
	inbound.Header.Del("Copilot-Vision-Request")
	_, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		BaseURL:       upstream.URL,
		CopilotToken:  "gho_tok",
		UpstreamModel: "m",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if got := cap.headers.Get(copilotlogin.VisionHeader); got != "true" {
		t.Fatalf("vision = %q, want true", got)
	}
	if got := cap.headers.Get(copilotlogin.InitiatorHeader); got != copilotlogin.InitiatorUser {
		t.Fatalf("initiator = %q, want %q (inbound agent must be ignored)", got, copilotlogin.InitiatorUser)
	}
}

func TestGitHubCopilotJSONUsageAndTools(t *testing.T) {
	var cap copilotCapture
	payload := `{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"get_time","arguments":"{}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":4,"total_tokens":14}}`
	upstream := newCopilotStub(t, &cap, http.StatusOK, payload, "application/json")
	defer upstream.Close()

	res, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		PublicModel:   "copilot/m",
		BaseURL:       upstream.URL,
		CopilotToken:  "gho_tok",
		UpstreamModel: "m",
		Inbound:       httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"copilot/m","messages":[],"tools":[{"type":"function","function":{"name":"get_time"}}]}`))),
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.Streaming {
		t.Fatalf("expected non-streaming result")
	}
	if !strings.Contains(string(res.Body), `"tool_calls"`) {
		t.Fatalf("tool calls missing: %s", string(res.Body))
	}
	if res.Usage.PromptTokens != 10 || res.Usage.CompletionTokens != 4 || res.Usage.TotalTokens != 14 {
		t.Fatalf("usage = %+v", res.Usage)
	}
}

func TestGitHubCopilotSSEUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get(copilotlogin.InitiatorHeader); got != copilotlogin.InitiatorUser {
			t.Errorf("initiator = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":3,\"total_tokens\":10}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	res, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		PublicModel:   "copilot/m",
		BaseURL:       upstream.URL,
		CopilotToken:  "gho_tok",
		UpstreamModel: "m",
		Inbound:       httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"copilot/m","stream":true,"messages":[]}`))),
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !res.Streaming || res.StreamBody == nil || res.Stream == nil {
		t.Fatalf("expected streaming result with completion tracker")
	}
	defer res.StreamBody.Close()
	body, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if !strings.Contains(string(body), "Hello") || !strings.Contains(string(body), "[DONE]") {
		t.Fatalf("unexpected stream body: %q", string(body))
	}
	res.Stream.Complete(nil, false)
	usage := res.Stream.Wait().Usage
	if usage.PromptTokens != 7 || usage.CompletionTokens != 3 || usage.TotalTokens != 10 {
		t.Fatalf("stream usage = %+v", usage)
	}
}

func TestGitHubCopilotRejectsUnsupportedOperationsWithoutUpstreamCalls(t *testing.T) {
	ops := []Operation{OpEmbeddings, OpResponses, OpImagesGenerations, OpAudioTranscriptions, OpAudioSpeech}
	for _, op := range ops {
		called := false
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}))
		_, err := New().Do(context.Background(), Request{
			Operation:     op,
			ProviderType:  config.ProviderTypeGitHubCopilot,
			BaseURL:       upstream.URL,
			CopilotToken:  "gho_tok",
			UpstreamModel: "m",
			Inbound:       httptest.NewRequest(http.MethodPost, "/", http.NoBody),
			Client:        upstream.Client(),
		})
		upstream.Close()
		if err == nil {
			t.Fatalf("op %v: expected error", op)
		}
		var unsupported ErrUnsupportedOperation
		if !errors.As(err, &unsupported) {
			t.Fatalf("op %v: expected ErrUnsupportedOperation, got %T: %v", op, err, err)
		}
		if called {
			t.Fatalf("op %v: upstream was called", op)
		}
	}
}

func TestGitHubCopilotRejectsMissingCredentialWithoutUpstreamCalls(t *testing.T) {
	called := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	_, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		BaseURL:       upstream.URL,
		UpstreamModel: "m",
		Inbound:       httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"copilot/m","messages":[]}`))),
		Client:        upstream.Client(),
	})
	if err == nil {
		t.Fatal("expected missing-credential error")
	}
	var invalid ErrInvalidRequest
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Message, "login") {
		t.Fatalf("expected login hint invalid request, got %v", err)
	}
	if called {
		t.Fatal("upstream was called without credential")
	}
}

func TestGitHubCopilotAuthFailurePassesThroughVerbatim(t *testing.T) {
	var cap copilotCapture
	upstream := newCopilotStub(t, &cap, http.StatusUnauthorized, `{"error":{"message":"invalid token","type":"invalid_request_error"}}`, "application/json")
	defer upstream.Close()

	res, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		BaseURL:       upstream.URL,
		CopilotToken:  "gho_revoked",
		UpstreamModel: "m",
		Inbound:       httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"copilot/m","messages":[]}`))),
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("auth failure must not be an error: %v", err)
	}
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if !strings.Contains(string(res.Body), "invalid token") {
		t.Fatalf("body was not passed through verbatim: %s", string(res.Body))
	}
	if cap.calls != 1 {
		t.Fatalf("expected exactly one upstream call (no auto-replay), got %d", cap.calls)
	}
}

func TestGitHubCopilotCancellation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer upstream.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := New().Do(ctx, Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		BaseURL:       upstream.URL,
		CopilotToken:  "gho_tok",
		UpstreamModel: "m",
		Inbound:       httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"copilot/m","messages":[]}`))),
		Client:        upstream.Client(),
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
}

func TestGitHubCopilotSlowUpstreamCanceled(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(5 * time.Second):
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer upstream.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	_, err := New().Do(ctx, Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGitHubCopilot,
		BaseURL:       upstream.URL,
		CopilotToken:  "gho_tok",
		UpstreamModel: "m",
		Inbound:       httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"copilot/m","messages":[]}`))),
		Client:        upstream.Client(),
	})
	if err == nil {
		t.Fatal("expected deadline error")
	}
}
