package provider

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

type openCodeCapture struct {
	mu          sync.Mutex
	calls       int
	method      string
	path        string
	rawQuery    string
	auth        string
	userAgent   string
	session     string
	apiKey      string
	cookie      string
	custom      string
	contentType string
	body        []byte
}

func (c *openCodeCapture) record(r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	c.method = r.Method
	c.path = r.URL.Path
	c.rawQuery = r.URL.RawQuery
	c.auth = r.Header.Get("Authorization")
	c.userAgent = r.Header.Get("User-Agent")
	c.session = r.Header.Get("x-opencode-session")
	c.apiKey = r.Header.Get("x-api-key")
	c.cookie = r.Header.Get("Cookie")
	c.custom = r.Header.Get("X-Custom")
	c.contentType = r.Header.Get("Content-Type")
	c.body = body
}

func (c *openCodeCapture) snapshot() (calls int, method, path, rawQuery, auth, userAgent, session, body string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls, c.method, c.path, c.rawQuery, c.auth, c.userAgent, c.session, string(c.body)
}

func openCodeUpstream(t *testing.T, cap *openCodeCapture, respond func(w http.ResponseWriter, r *http.Request)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cap.record(r)
		respond(w, r)
	}))
}

func openCodeJSONResponder(payload string) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, payload)
	}
}

func openCodeInbound(method, target, body string) *http.Request {
	req := httptest.NewRequest(method, target, io.NopCloser(strings.NewReader(body)))
	req.Header.Set("User-Agent", "evil-inbound-ua")
	req.Header.Set("Authorization", "Bearer inbound-secret")
	req.Header.Set("Cookie", "session=evil")
	req.Header.Set("X-Custom", "evil")
	return req
}

func openCodeDo(t *testing.T, upstream *httptest.Server, providerType config.ProviderType, protocol config.ModelProtocol, op Operation, publicModel, upstreamModel, body string) (*Result, *openCodeCapture, *http.Request) {
	t.Helper()
	cap := &openCodeCapture{}
	var srv *httptest.Server
	if upstream == nil {
		srv = openCodeUpstream(t, cap, openCodeJSONResponder(`{"id":"x"}`))
		t.Cleanup(srv.Close)
		upstream = srv
	}
	inbound := openCodeInbound(http.MethodPost, "/v1/chat/completions", body)
	res, err := New().Do(context.Background(), Request{
		Operation:     op,
		ProviderType:  providerType,
		PublicModel:   publicModel,
		BaseURL:       upstream.URL,
		APIKey:        "sk-opencode",
		UpstreamModel: upstreamModel,
		ModelProtocol: protocol,
		Version:       "1.2.3-test",
		Body:          []byte(body),
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return res, cap, inbound
}

func checkOpenCodeCommonHeaders(t *testing.T, cap *openCodeCapture, wantSession bool) (auth, userAgent, session, body string) {
	t.Helper()
	calls, _, _, _, auth, userAgent, session, body := cap.snapshot()
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1", calls)
	}
	if auth != "Bearer sk-opencode" {
		t.Fatalf("Authorization = %q", auth)
	}
	if userAgent != "aiproxy/1.2.3-test" {
		t.Fatalf("User-Agent = %q", userAgent)
	}
	if wantSession {
		if session == "" {
			t.Fatal("missing x-opencode-session on opencode-go request")
		}
	} else if session != "" {
		t.Fatalf("unexpected x-opencode-session on opencode-zen request: %q", session)
	}
	cap.mu.Lock()
	defer cap.mu.Unlock()
	if cap.apiKey != "" {
		t.Fatalf("x-api-key leaked: %q", cap.apiKey)
	}
	if cap.cookie != "" || cap.custom != "" {
		t.Fatalf("inbound headers forwarded: cookie=%q custom=%q", cap.cookie, cap.custom)
	}
	return auth, userAgent, session, body
}

func TestOpenCodeDefaultBaseURLs(t *testing.T) {
	if got := providerDescriptors[config.ProviderTypeOpenCodeZen].defaultBaseURL; got != "https://opencode.ai/zen/v1" {
		t.Fatalf("zen default = %q", got)
	}
	if got := providerDescriptors[config.ProviderTypeOpenCodeGo].defaultBaseURL; got != "https://opencode.ai/zen/go/v1" {
		t.Fatalf("go default = %q", got)
	}
}

func TestOpenCodeEmptyBaseURLUsesDocumentedPrefix(t *testing.T) {
	for _, tc := range []struct {
		providerType config.ProviderType
		protocol     config.ModelProtocol
		op           Operation
		body         string
		wantHost     string
		wantPath     string
	}{
		{config.ProviderTypeOpenCodeZen, config.ModelProtocolChat, OpChatCompletions, `{"model":"zen/m","messages":[]}`, "opencode.ai", "/zen/v1/chat/completions"},
		{config.ProviderTypeOpenCodeGo, config.ModelProtocolResponses, OpResponses, `{"model":"go/m","input":"hi"}`, "opencode.ai", "/zen/go/v1/responses"},
		{config.ProviderTypeOpenCodeZen, config.ModelProtocolMessages, OpChatCompletions, `{"model":"zen/m","messages":[{"role":"user","content":"hi"}]}`, "opencode.ai", "/zen/v1/messages"},
		{config.ProviderTypeOpenCodeZen, config.ModelProtocolGemini, OpChatCompletions, `{"model":"zen/m","messages":[{"role":"user","content":"hi"}]}`, "opencode.ai", "/zen/v1/models/upstream:generateContent"},
	} {
		t.Run(string(tc.providerType)+"/"+string(tc.protocol), func(t *testing.T) {
			var seenHost, seenPath string
			client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
				seenHost = r.URL.Host
				seenPath = r.URL.Path
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"application/json"}},
					Body:       io.NopCloser(strings.NewReader(`{}`)),
					Request:    r,
				}, nil
			})}
			inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			_, err := New().Do(context.Background(), Request{
				Operation:     tc.op,
				ProviderType:  tc.providerType,
				PublicModel:   "test/m",
				APIKey:        "k",
				UpstreamModel: "upstream",
				ModelProtocol: tc.protocol,
				Body:          []byte(tc.body),
				Inbound:       inbound,
				Client:        client,
			})
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			if seenHost != tc.wantHost || seenPath != tc.wantPath {
				t.Fatalf("target = https://%s%s, want https://%s%s", seenHost, seenPath, tc.wantHost, tc.wantPath)
			}
		})
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestOpenCodeChatJSON(t *testing.T) {
	for _, providerType := range []config.ProviderType{config.ProviderTypeOpenCodeZen, config.ProviderTypeOpenCodeGo} {
		t.Run(string(providerType), func(t *testing.T) {
			cap := &openCodeCapture{}
			upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"id":"chatcmpl-1","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":5,"total_tokens":8}}`))
			defer upstream.Close()
			res, _, _ := openCodeDo(t, upstream, providerType, config.ModelProtocolChat, OpChatCompletions, "test/glm", "glm-5.3", `{"model":"test/glm","messages":[{"role":"user","content":"hi"}]}`)
			_, _, _, body := checkOpenCodeCommonHeaders(t, cap, providerType == config.ProviderTypeOpenCodeGo)
			_, method, path, _, _, _, _, _ := cap.snapshot()
			if method != http.MethodPost || path != "/v1/chat/completions" {
				t.Fatalf("method/path = %s %s", method, path)
			}
			if !strings.Contains(body, `"model":"glm-5.3"`) {
				t.Fatalf("model not rewritten: %s", body)
			}
			if !strings.Contains(string(res.Body), `"content":"hi"`) {
				t.Fatalf("response = %s", res.Body)
			}
			if res.Usage.PromptTokens != 3 || res.Usage.CompletionTokens != 5 || res.Usage.TotalTokens != 8 {
				t.Fatalf("usage = %+v", res.Usage)
			}
		})
	}
}

func TestOpenCodeChatSSEPassthrough(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}],\"usage\":{\"prompt_tokens\":2,\"completion_tokens\":0,\"total_tokens\":2}}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	})
	defer upstream.Close()
	body := `{"model":"go/glm","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	inbound := openCodeInbound(http.MethodPost, "/v1/chat/completions", body)
	res, err := New().Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeGo,
		PublicModel: "go/glm", BaseURL: upstream.URL, APIKey: "sk-opencode",
		UpstreamModel: "glm-5.3", ModelProtocol: config.ModelProtocolChat, Version: "1.2.3-test",
		Body: []byte(body), Inbound: inbound, Client: upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !res.Streaming || res.StreamBody == nil {
		t.Fatal("expected streaming result")
	}
	defer res.StreamBody.Close()
	out, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, "Hel") || !strings.Contains(text, "lo") || !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("stream = %q", text)
	}
	res.Stream.Complete(nil, false)
	if usage := res.Stream.Wait().Usage; usage.PromptTokens != 2 {
		t.Fatalf("stream usage = %+v", usage)
	}
	checkOpenCodeCommonHeaders(t, cap, true)
}

func TestOpenCodeChatPreservesTools(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"id":"1","choices":[]}`))
	defer upstream.Close()
	body := `{"model":"zen/m","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function","function":{"name":"f"}}],"tool_choice":"auto"}`
	openCodeDo(t, upstream, config.ProviderTypeOpenCodeZen, config.ModelProtocolChat, OpChatCompletions, "zen/m", "upstream-m", body)
	_, _, _, seen := checkOpenCodeCommonHeaders(t, cap, false)
	if !strings.Contains(seen, `"tool_choice":"auto"`) || !strings.Contains(seen, `"name":"f"`) {
		t.Fatalf("tools not preserved: %s", seen)
	}
}

func TestOpenCodeResponsesJSON(t *testing.T) {
	for _, providerType := range []config.ProviderType{config.ProviderTypeOpenCodeZen, config.ProviderTypeOpenCodeGo} {
		t.Run(string(providerType), func(t *testing.T) {
			cap := &openCodeCapture{}
			upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"id":"resp_1","object":"response","model":"upstream","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`))
			defer upstream.Close()
			res, _, _ := openCodeDo(t, upstream, providerType, config.ModelProtocolResponses, OpResponses, "test/gpt", "gpt-5.5", `{"model":"test/gpt","input":"hi"}`)
			_, method, path, _, _, _, _, body := cap.snapshot()
			if method != http.MethodPost || path != "/v1/responses" {
				t.Fatalf("method/path = %s %s", method, path)
			}
			if !strings.Contains(body, `"model":"gpt-5.5"`) {
				t.Fatalf("model not rewritten: %s", body)
			}
			if res.Usage.PromptTokens != 4 || res.Usage.CompletionTokens != 6 || res.Usage.TotalTokens != 10 {
				t.Fatalf("usage = %+v", res.Usage)
			}
			checkOpenCodeCommonHeaders(t, cap, providerType == config.ProviderTypeOpenCodeGo)
		})
	}
}

func TestOpenCodeMessagesChatJSON(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"Hello"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":6}}`))
	defer upstream.Close()
	res, _, _ := openCodeDo(t, upstream, config.ProviderTypeOpenCodeGo, config.ModelProtocolMessages, OpChatCompletions, "go/minimax", "minimax-m3", `{"model":"go/minimax","messages":[{"role":"user","content":"hi"}]}`)
	_, method, path, _, _, _, _, seenBody := cap.snapshot()
	if method != http.MethodPost || path != "/messages" {
		t.Fatalf("method/path = %s %s", method, path)
	}
	if !strings.Contains(seenBody, `"model":"minimax-m3"`) {
		t.Fatalf("upstream model missing: %s", seenBody)
	}
	if !strings.Contains(string(res.Body), `"content":"Hello"`) || !strings.Contains(string(res.Body), `"model":"go/minimax"`) {
		t.Fatalf("translated response = %s", res.Body)
	}
	if res.Usage.PromptTokens != 9 || res.Usage.CompletionTokens != 6 {
		t.Fatalf("usage = %+v", res.Usage)
	}
	checkOpenCodeCommonHeaders(t, cap, true)
}

func TestOpenCodeMessagesStreamingTranslation(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\n")
		_, _ = io.WriteString(w, "data: {\"message\":{\"id\":\"msg_stream\"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\n")
		_, _ = io.WriteString(w, "data: {}\n\n")
	})
	defer upstream.Close()
	body := `{"model":"zen/claude","stream":true,"messages":[{"role":"user","content":"Hi"}]}`
	inbound := openCodeInbound(http.MethodPost, "/v1/chat/completions", body)
	res, err := New().Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeZen,
		PublicModel: "zen/claude", BaseURL: upstream.URL, APIKey: "sk-opencode",
		UpstreamModel: "claude-sonnet-5", ModelProtocol: config.ModelProtocolMessages, Version: "1.2.3-test",
		Body: []byte(body), Inbound: inbound, Client: upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !res.Streaming || res.StreamBody == nil {
		t.Fatal("expected streaming result")
	}
	defer res.StreamBody.Close()
	out, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, `"content":"Hello"`) || !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("stream = %q", text)
	}
	checkOpenCodeCommonHeaders(t, cap, false)
}

func TestOpenCodeMessagesResponsesJSON(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"id":"msg_resp","role":"assistant","content":[{"type":"text","text":"Hello from Claude"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":6}}`))
	defer upstream.Close()
	res, _, _ := openCodeDo(t, upstream, config.ProviderTypeOpenCodeZen, config.ModelProtocolMessages, OpResponses, "zen/claude", "claude-sonnet-5", `{"model":"zen/claude","instructions":"Be direct.","input":[{"type":"message","role":"user","content":"hello"}]}`)
	if !strings.Contains(string(res.Body), `"object":"response"`) || !strings.Contains(string(res.Body), "Hello from Claude") {
		t.Fatalf("translated responses body = %s", res.Body)
	}
	if res.Usage.PromptTokens != 9 || res.Usage.CompletionTokens != 6 {
		t.Fatalf("usage = %+v", res.Usage)
	}
	checkOpenCodeCommonHeaders(t, cap, false)
}

func TestOpenCodeMessagesRejectsToolsBeforeIO(t *testing.T) {
	var called atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	inbound := openCodeInbound(http.MethodPost, "/v1/chat/completions", `{"model":"zen/m","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function"}]}`)
	_, err := New().Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeZen,
		PublicModel: "zen/m", BaseURL: upstream.URL, APIKey: "k",
		UpstreamModel: "m", ModelProtocol: config.ModelProtocolMessages,
		Body:    []byte(`{"model":"zen/m","messages":[{"role":"user","content":"hi"}],"tools":[{"type":"function"}]}`),
		Inbound: inbound, Client: upstream.Client(),
	})
	var invalid ErrInvalidRequest
	if !errors.As(err, &invalid) {
		t.Fatalf("expected ErrInvalidRequest, got %T: %v", err, err)
	}
	if called.Load() {
		t.Fatal("upstream called for unsupported translated feature")
	}
}

func TestOpenCodeGeminiChatJSON(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7,"candidatesTokenCount":3,"totalTokenCount":10}}`))
	defer upstream.Close()
	res, _, _ := openCodeDo(t, upstream, config.ProviderTypeOpenCodeZen, config.ModelProtocolGemini, OpChatCompletions, "zen/gemini", "gemini-3.8-flash", `{"model":"zen/gemini","messages":[{"role":"user","content":"hi"}]}`)
	calls, method, path, query, auth, _, session, seenBody := cap.snapshot()
	if calls != 1 || method != http.MethodPost || path != "/models/gemini-3.8-flash:generateContent" || query != "" {
		t.Fatalf("target = %s %s?%s", method, path, query)
	}
	if auth != "Bearer sk-opencode" || session != "" {
		t.Fatalf("auth=%q session=%q", auth, session)
	}
	cap.mu.Lock()
	apiKey := cap.apiKey
	cap.mu.Unlock()
	if apiKey != "" {
		t.Fatalf("x-goog-api-key must not be sent: %q", apiKey)
	}
	if !strings.Contains(seenBody, `"role":"user"`) {
		t.Fatalf("gemini request = %s", seenBody)
	}
	if !strings.Contains(string(res.Body), `"content":"Hello"`) || !strings.Contains(string(res.Body), `"model":"zen/gemini"`) {
		t.Fatalf("translated response = %s", res.Body)
	}
}

func TestOpenCodeGeminiStreamingTranslation(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[]},\"finishReason\":\"STOP\"}]}\n\n")
	})
	defer upstream.Close()
	body := `{"model":"zen/gemini","stream":true,"messages":[{"role":"user","content":"Hi"}]}`
	inbound := openCodeInbound(http.MethodPost, "/v1/chat/completions", body)
	res, err := New().Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeZen,
		PublicModel: "zen/gemini", BaseURL: upstream.URL, APIKey: "sk-opencode",
		UpstreamModel: "gemini-3.8-flash", ModelProtocol: config.ModelProtocolGemini, Version: "v",
		Body: []byte(body), Inbound: inbound, Client: upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !res.Streaming || res.StreamBody == nil {
		t.Fatal("expected streaming result")
	}
	defer res.StreamBody.Close()
	out, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	text := string(out)
	if !strings.Contains(text, `"content":"Hello"`) || !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("stream = %q", text)
	}
	_, _, path, query, _, _, _, _ := cap.snapshot()
	if path != "/models/gemini-3.8-flash:streamGenerateContent" || query != "alt=sse" {
		t.Fatalf("stream target = %s?%s", path, query)
	}
}

func TestOpenCodeGeminiResponsesJSON(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":7}}`))
	defer upstream.Close()
	res, _, _ := openCodeDo(t, upstream, config.ProviderTypeOpenCodeZen, config.ModelProtocolGemini, OpResponses, "zen/gemini", "gemini-3.8-flash", `{"model":"zen/gemini","input":"hi"}`)
	if !strings.Contains(string(res.Body), `"object":"response"`) {
		t.Fatalf("translated responses body = %s", res.Body)
	}
	if res.Usage.TotalTokens != 7 {
		t.Fatalf("usage = %+v", res.Usage)
	}
}

func TestOpenCodeV1SuffixBaseAvoidsDuplication(t *testing.T) {
	for _, tc := range []struct {
		name     string
		protocol config.ModelProtocol
		op       Operation
		body     string
		wantPath string
	}{
		{"chat", config.ModelProtocolChat, OpChatCompletions, `{"model":"m","messages":[]}`, "/zen/v1/chat/completions"},
		{"responses", config.ModelProtocolResponses, OpResponses, `{"model":"m","input":"hi"}`, "/zen/v1/responses"},
		{"messages", config.ModelProtocolMessages, OpChatCompletions, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`, "/zen/v1/messages"},
		{"gemini", config.ModelProtocolGemini, OpChatCompletions, `{"model":"m","messages":[{"role":"user","content":"hi"}]}`, "/zen/v1/models/upstream:generateContent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cap := &openCodeCapture{}
			base := openCodeUpstream(t, cap, openCodeJSONResponder(`{"id":"x","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
			defer base.Close()
			inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			_, err := New().Do(context.Background(), Request{
				Operation: tc.op, ProviderType: config.ProviderTypeOpenCodeZen,
				PublicModel: "zen/m", BaseURL: base.URL + "/zen/v1/", APIKey: "k",
				UpstreamModel: "upstream", ModelProtocol: tc.protocol,
				Body: []byte(tc.body), Inbound: inbound, Client: base.Client(),
			})
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			_, _, path, _, _, _, _, _ := cap.snapshot()
			if path != tc.wantPath {
				t.Fatalf("path = %q, want %q", path, tc.wantPath)
			}
		})
	}
}

func TestOpenCodeSameModelDifferentProtocol(t *testing.T) {
	zenCap := &openCodeCapture{}
	zen := openCodeUpstream(t, zenCap, openCodeJSONResponder(`{"id":"1","choices":[]}`))
	defer zen.Close()
	goCap := &openCodeCapture{}
	goUp := openCodeUpstream(t, goCap, openCodeJSONResponder(`{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`))
	defer goUp.Close()

	chatBody := `{"model":"zen/minimax-m3","messages":[{"role":"user","content":"hi"}]}`
	zenInbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(chatBody))
	if _, err := New().Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeZen,
		PublicModel: "zen/minimax-m3", BaseURL: zen.URL, APIKey: "k",
		UpstreamModel: "minimax-m3", ModelProtocol: config.ModelProtocolChat,
		Body: []byte(chatBody), Inbound: zenInbound, Client: zen.Client(),
	}); err != nil {
		t.Fatalf("zen do: %v", err)
	}
	goInbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(chatBody))
	if _, err := New().Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeGo,
		PublicModel: "go/minimax-m3", BaseURL: goUp.URL, APIKey: "k",
		UpstreamModel: "minimax-m3", ModelProtocol: config.ModelProtocolMessages,
		Body: []byte(chatBody), Inbound: goInbound, Client: goUp.Client(),
	}); err != nil {
		t.Fatalf("go do: %v", err)
	}
	_, _, zenPath, _, _, _, zenSession, _ := zenCap.snapshot()
	_, _, goPath, _, _, _, goSession, _ := goCap.snapshot()
	if zenPath != "/v1/chat/completions" {
		t.Fatalf("zen path = %q", zenPath)
	}
	if goPath != "/messages" {
		t.Fatalf("go path = %q", goPath)
	}
	if zenSession != "" {
		t.Fatalf("zen sent session: %q", zenSession)
	}
	if goSession == "" {
		t.Fatal("go missing session")
	}
}

func TestOpenCodeUnsupportedCombosMakeZeroUpstreamCalls(t *testing.T) {
	var called atomic.Bool
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called.Store(true)
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	for _, tc := range []struct {
		name         string
		providerType config.ProviderType
		protocol     config.ModelProtocol
		op           Operation
		body         string
		wantErr      string
	}{
		{"chat protocol with responses op", config.ProviderTypeOpenCodeZen, config.ModelProtocolChat, OpResponses, `{"model":"m","input":"hi"}`, "does not support operation"},
		{"responses protocol with chat op", config.ProviderTypeOpenCodeZen, config.ModelProtocolResponses, OpChatCompletions, `{"model":"m","messages":[]}`, "does not support operation"},
		{"gemini protocol on go chat", config.ProviderTypeOpenCodeGo, config.ModelProtocolGemini, OpChatCompletions, `{"model":"m","messages":[]}`, "does not support operation"},
		{"gemini protocol on go responses", config.ProviderTypeOpenCodeGo, config.ModelProtocolGemini, OpResponses, `{"model":"m","input":"hi"}`, "does not support operation"},
		{"embeddings on zen chat", config.ProviderTypeOpenCodeZen, config.ModelProtocolChat, OpEmbeddings, `{"model":"m","input":"hi"}`, "does not support operation"},
		{"messages protocol with embeddings", config.ProviderTypeOpenCodeGo, config.ModelProtocolMessages, OpEmbeddings, `{"model":"m","input":"hi"}`, "does not support operation"},
		{"missing protocol", config.ProviderTypeOpenCodeZen, config.ModelProtocol(""), OpChatCompletions, `{"model":"m","messages":[]}`, "unknown model protocol"},
		{"unknown protocol", config.ProviderTypeOpenCodeGo, config.ModelProtocol("bogus"), OpChatCompletions, `{"model":"m","messages":[]}`, "unknown model protocol"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			called.Store(false)
			inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.body))
			_, err := New().Do(context.Background(), Request{
				Operation: tc.op, ProviderType: tc.providerType,
				PublicModel: "test/m", BaseURL: upstream.URL, APIKey: "k",
				UpstreamModel: "m", ModelProtocol: tc.protocol,
				Body: []byte(tc.body), Inbound: inbound, Client: upstream.Client(),
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			var unsupported ErrUnsupportedOperation
			var invalid ErrInvalidRequest
			if strings.Contains(tc.wantErr, "does not support operation") && !errors.As(err, &unsupported) {
				t.Fatalf("expected ErrUnsupportedOperation, got %T", err)
			}
			if strings.Contains(tc.wantErr, "unknown model protocol") && !errors.As(err, &invalid) {
				t.Fatalf("expected ErrInvalidRequest, got %T", err)
			}
			if called.Load() {
				t.Fatal("upstream was called for unsupported combination")
			}
		})
	}
}

func TestOpenCodeSessionForwardingPolicy(t *testing.T) {
	valid := []string{"a", "ses_abc-XYZ_019", strings.Repeat("x", 128)}
	for _, v := range valid {
		cap := &openCodeCapture{}
		upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{}`))
		inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"m","messages":[]}`))
		inbound.Header.Set("x-opencode-session", v)
		_, err := New().Do(context.Background(), Request{
			Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeGo,
			PublicModel: "go/m", BaseURL: upstream.URL, APIKey: "k",
			UpstreamModel: "m", ModelProtocol: config.ModelProtocolChat,
			Body: []byte(`{"model":"m","messages":[]}`), Inbound: inbound, Client: upstream.Client(),
		})
		upstream.Close()
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		_, _, _, _, _, _, session, _ := cap.snapshot()
		if session != v {
			t.Fatalf("session = %q, want forwarded %q", session, v)
		}
	}

	generated := map[string]bool{}
	invalid := []string{"", "has space", "semi;colon", "quote\"x", "slash/x", "uniçode", strings.Repeat("y", 129)}
	sessionPattern := regexp.MustCompile(`^ses_[0-9a-f]{32}$`)
	for _, v := range invalid {
		cap := &openCodeCapture{}
		upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{}`))
		inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"m","messages":[]}`))
		if v != "" {
			inbound.Header.Set("x-opencode-session", v)
		}
		_, err := New().Do(context.Background(), Request{
			Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeGo,
			PublicModel: "go/m", BaseURL: upstream.URL, APIKey: "k",
			UpstreamModel: "m", ModelProtocol: config.ModelProtocolChat,
			Body: []byte(`{"model":"m","messages":[]}`), Inbound: inbound, Client: upstream.Client(),
		})
		upstream.Close()
		if err != nil {
			t.Fatalf("do with invalid session %q: %v", v, err)
		}
		_, _, _, _, _, _, session, _ := cap.snapshot()
		if !sessionPattern.MatchString(session) {
			t.Fatalf("session = %q for inbound %q, want generated ses_+hex", session, v)
		}
		if generated[session] {
			t.Fatalf("duplicate generated session %q", session)
		}
		generated[session] = true
	}
}

func TestOpenCodeUserAgentPolicy(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{}`))
	defer upstream.Close()
	inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"m","messages":[]}`))
	inbound.Header.Set("User-Agent", "opencode/1.0 should-not-forward")
	_, err := New().Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeZen,
		PublicModel: "zen/m", BaseURL: upstream.URL, APIKey: "k",
		UpstreamModel: "m", ModelProtocol: config.ModelProtocolChat, Version: "7.8.9",
		Body: []byte(`{"model":"m","messages":[]}`), Inbound: inbound, Client: upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_, _, _, _, _, ua, _, _ := cap.snapshot()
	if ua != "aiproxy/7.8.9" {
		t.Fatalf("User-Agent = %q", ua)
	}

	cap2 := &openCodeCapture{}
	upstream2 := openCodeUpstream(t, cap2, openCodeJSONResponder(`{}`))
	defer upstream2.Close()
	inbound2 := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"m","messages":[]}`))
	_, err = New().Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeZen,
		PublicModel: "zen/m", BaseURL: upstream2.URL, APIKey: "k",
		UpstreamModel: "m", ModelProtocol: config.ModelProtocolChat,
		Body: []byte(`{"model":"m","messages":[]}`), Inbound: inbound2, Client: upstream2.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_, _, _, _, _, ua2, _, _ := cap2.snapshot()
	if ua2 != "aiproxy/dev" {
		t.Fatalf("default User-Agent = %q", ua2)
	}
}

func TestOpenCodeUpstreamErrorPassthrough(t *testing.T) {
	cap := &openCodeCapture{}
	upstream := openCodeUpstream(t, cap, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":{"type":"upstream_error","message":"boom"}}`)
	})
	defer upstream.Close()
	res, _, _ := openCodeDo(t, upstream, config.ProviderTypeOpenCodeZen, config.ModelProtocolChat, OpChatCompletions, "zen/m", "m", `{"model":"zen/m","messages":[]}`)
	if res.StatusCode != http.StatusBadGateway || !strings.Contains(string(res.Body), "boom") {
		t.Fatalf("result = %d %s", res.StatusCode, res.Body)
	}
}

func TestOpenCodeCancellationBeforeIO(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		calls.Add(1)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"model":"m","messages":[]}`))
	_, err := New().Do(ctx, Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenCodeGo,
		PublicModel: "go/m", BaseURL: "http://127.0.0.1:9", APIKey: "k",
		UpstreamModel: "m", ModelProtocol: config.ModelProtocolChat,
		Body: []byte(`{"model":"m","messages":[]}`), Inbound: inbound.WithContext(ctx), Client: client,
	})
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "canceled") && !strings.Contains(err.Error(), "cancel") {
		t.Fatalf("error = %v, want cancellation", err)
	}
}
