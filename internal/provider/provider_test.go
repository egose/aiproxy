package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestAdapterRewritesModelAndForwards(t *testing.T) {
	var seenAuth, seenBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","model":"gpt-4o-2024-08-06"}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"openai/gpt-4o-mini","messages":[]}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		BaseURL:       upstream.URL,
		APIKey:        "sk-test",
		UpstreamModel: "gpt-4o-2024-08-06",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("status = %d", res.StatusCode)
	}
	if seenAuth != "Bearer sk-test" {
		t.Errorf("auth = %q", seenAuth)
	}
	if !strings.Contains(seenBody, "\"model\":\"gpt-4o-2024-08-06\"") {
		t.Errorf("upstream body did not get model rewritten: %s", seenBody)
	}
}

func TestJoinBaseURLAndPathAvoidsDuplicateV1(t *testing.T) {
	got := joinBaseURLAndPath("https://api.openai.com/v1", "/v1/chat/completions")
	if got != "https://api.openai.com/v1/chat/completions" {
		t.Fatalf("joined url = %q", got)
	}
	got = joinBaseURLAndPath("https://llm.internal/v1/", "/v1/embeddings")
	if got != "https://llm.internal/v1/embeddings" {
		t.Fatalf("joined url with trailing slash = %q", got)
	}
}

func TestAdapterStreamingFlagPropagated(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"openai/x","stream":true,"messages":[]}`)))
	res, err := a.Do(context.Background(), Request{
		BaseURL:       upstream.URL,
		APIKey:        "k",
		UpstreamModel: "x",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !res.Streaming {
		t.Errorf("expected Streaming=true")
	}
	if res.StreamBody == nil {
		t.Fatalf("expected StreamBody to be populated")
	}
	defer res.StreamBody.Close()
	body, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	if !strings.Contains(string(body), "[DONE]") {
		t.Errorf("expected SSE body to include [DONE], got %q", string(body))
	}
}

func TestOpenAIEmbeddingsRewritesModelAndForwards(t *testing.T) {
	var seenAuth, seenBody, seenPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"list","data":[{"object":"embedding","embedding":[0.1,0.2],"index":0}],"model":"text-embedding-3-large","usage":{"prompt_tokens":4,"total_tokens":4}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/embeddings",
		io.NopCloser(strings.NewReader(`{"model":"openai/text-embedding-3-large","input":"hello"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpEmbeddings,
		ProviderType:  config.ProviderTypeOpenAI,
		PublicModel:   "openai/text-embedding-3-large",
		BaseURL:       upstream.URL,
		APIKey:        "sk-test",
		UpstreamModel: "text-embedding-3-large",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenPath != "/v1/embeddings" {
		t.Fatalf("path = %q", seenPath)
	}
	if seenAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", seenAuth)
	}
	if !strings.Contains(seenBody, `"model":"text-embedding-3-large"`) {
		t.Fatalf("model was not rewritten: %s", seenBody)
	}
	if res.Streaming {
		t.Fatalf("embeddings should not stream")
	}
	if !strings.Contains(string(res.Body), `"object":"list"`) {
		t.Fatalf("unexpected embeddings response: %s", string(res.Body))
	}
}

func TestOpenAIResponsesRewritesModelAndForwards(t *testing.T) {
	var seenAuth, seenBody, seenPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"resp_123","object":"response","model":"gpt-4.1","output":[]}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"openai/gpt-4.1","input":"hello"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  config.ProviderTypeOpenAI,
		PublicModel:   "openai/gpt-4.1",
		BaseURL:       upstream.URL,
		APIKey:        "sk-test",
		UpstreamModel: "gpt-4.1",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenPath != "/v1/responses" {
		t.Fatalf("path = %q", seenPath)
	}
	if seenAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", seenAuth)
	}
	if !strings.Contains(seenBody, `"model":"gpt-4.1"`) {
		t.Fatalf("model was not rewritten: %s", seenBody)
	}
	if res.Streaming {
		t.Fatalf("non-streaming responses request should not stream")
	}
	if !strings.Contains(string(res.Body), `"object":"response"`) {
		t.Fatalf("unexpected responses payload: %s", string(res.Body))
	}
}

func TestOpenAIImagesRewritesModelAndForwards(t *testing.T) {
	var seenAuth, seenBody, seenPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"created":123,"data":[{"url":"https://example.com/image.png"}]}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/images/generations",
		io.NopCloser(strings.NewReader(`{"model":"openai/gpt-image-1","prompt":"a cat"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpImagesGenerations,
		ProviderType:  config.ProviderTypeOpenAI,
		PublicModel:   "openai/gpt-image-1",
		BaseURL:       upstream.URL,
		APIKey:        "sk-test",
		UpstreamModel: "gpt-image-1",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenPath != "/v1/images/generations" {
		t.Fatalf("path = %q", seenPath)
	}
	if seenAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", seenAuth)
	}
	if !strings.Contains(seenBody, `"model":"gpt-image-1"`) {
		t.Fatalf("model was not rewritten: %s", seenBody)
	}
	if res.Streaming {
		t.Fatalf("images should not stream")
	}
	if !strings.Contains(string(res.Body), `"url":"https://example.com/image.png"`) {
		t.Fatalf("unexpected images payload: %s", string(res.Body))
	}
}

func TestOpenAIAudioTranscriptionsRewriteMultipartModel(t *testing.T) {
	var seenAuth, seenPath, seenType, seenBody string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		seenType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hello world"}`))
	}))
	defer upstream.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	modelPart, _ := writer.CreateFormField("model")
	_, _ = io.WriteString(modelPart, "openai/whisper-1")
	filePart, _ := writer.CreateFormFile("file", "sample.wav")
	_, _ = io.WriteString(filePart, "audio-bytes")
	_ = writer.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", io.NopCloser(bytes.NewReader(body.Bytes())))
	inbound.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := a.Do(context.Background(), Request{
		Operation:     OpAudioTranscriptions,
		ProviderType:  config.ProviderTypeOpenAI,
		PublicModel:   "openai/whisper-1",
		BaseURL:       upstream.URL,
		APIKey:        "sk-test",
		UpstreamModel: "gpt-4o-transcribe",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenPath != "/v1/audio/transcriptions" {
		t.Fatalf("path = %q", seenPath)
	}
	if seenAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", seenAuth)
	}
	if !strings.HasPrefix(seenType, "multipart/form-data;") {
		t.Fatalf("content-type = %q", seenType)
	}
	if !strings.Contains(seenBody, "gpt-4o-transcribe") {
		t.Fatalf("model was not rewritten: %s", seenBody)
	}
	if !strings.Contains(string(res.Body), `"text":"hello world"`) {
		t.Fatalf("unexpected audio payload: %s", string(res.Body))
	}
}

func TestOpenAIAudioSpeechRewritesModelAndForwards(t *testing.T) {
	var seenAuth, seenBody, seenPath, seenAccept string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenPath = r.URL.Path
		seenAccept = r.Header.Get("Accept")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "audio/mpeg")
		_, _ = w.Write([]byte("mp3-bytes"))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/audio/speech",
		io.NopCloser(strings.NewReader(`{"model":"openai/tts-1","input":"hello","voice":"alloy"}`)))
	inbound.Header.Set("Accept", "audio/mpeg")
	res, err := a.Do(context.Background(), Request{
		Operation:     OpAudioSpeech,
		ProviderType:  config.ProviderTypeOpenAI,
		PublicModel:   "openai/tts-1",
		BaseURL:       upstream.URL,
		APIKey:        "sk-test",
		UpstreamModel: "tts-1-hd",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenPath != "/v1/audio/speech" {
		t.Fatalf("path = %q", seenPath)
	}
	if seenAuth != "Bearer sk-test" {
		t.Fatalf("auth = %q", seenAuth)
	}
	if seenAccept != "audio/mpeg" {
		t.Fatalf("accept = %q", seenAccept)
	}
	if !strings.Contains(seenBody, `"model":"tts-1-hd"`) {
		t.Fatalf("model was not rewritten: %s", seenBody)
	}
	if res.Streaming {
		t.Fatalf("audio speech should not stream for binary response")
	}
	if string(res.Body) != "mp3-bytes" {
		t.Fatalf("unexpected audio speech body: %q", string(res.Body))
	}
}

func TestAnthropicEmbeddingsUnsupported(t *testing.T) {
	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/embeddings",
		io.NopCloser(strings.NewReader(`{"model":"anthropic/claude","input":"hello"}`)))
	_, err := a.Do(context.Background(), Request{
		Operation:     OpEmbeddings,
		ProviderType:  config.ProviderTypeAnthropic,
		PublicModel:   "anthropic/claude",
		APIKey:        "sk-ant",
		UpstreamModel: "claude-sonnet-4-20250514",
		Inbound:       inbound,
	})
	if err == nil {
		t.Fatal("expected unsupported error")
	}
	var unsupported ErrUnsupportedOperation
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected ErrUnsupportedOperation, got %T: %v", err, err)
	}
}

func TestGeminiEmbeddingsRejectsInvalidInputShape(t *testing.T) {
	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/embeddings",
		io.NopCloser(strings.NewReader(`{"model":"gemini/text-embedding-004","input":[1,2,3]}`)))
	_, err := a.Do(context.Background(), Request{
		Operation:     OpEmbeddings,
		ProviderType:  config.ProviderTypeGemini,
		PublicModel:   "gemini/text-embedding-004",
		APIKey:        "gem-key",
		UpstreamModel: "text-embedding-004",
		Inbound:       inbound,
	})
	if err == nil {
		t.Fatal("expected invalid-request error")
	}
	var invalid ErrInvalidRequest
	if !errors.As(err, &invalid) {
		t.Fatalf("expected ErrInvalidRequest, got %T: %v", err, err)
	}
}

func TestAnthropicResponsesTranslation(t *testing.T) {
	var seenBody []byte
	var seenAccept string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAccept = r.Header.Get("Accept")
		var err error
		seenBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_resp","role":"assistant","content":[{"type":"text","text":"Hello from Claude responses"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":6}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"anthropic/claude-sonnet","instructions":"Be direct.","input":[{"type":"message","role":"user","content":"hello"}]}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  config.ProviderTypeAnthropic,
		PublicModel:   "anthropic/claude-sonnet",
		BaseURL:       upstream.URL,
		APIKey:        "sk-ant",
		UpstreamModel: "claude-sonnet-4-20250514",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	var upstreamReq struct {
		System   string `json:"system"`
		Messages []struct {
			Role string `json:"role"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(seenBody, &upstreamReq); err != nil {
		t.Fatalf("unmarshal upstream request: %v", err)
	}
	if upstreamReq.System != "Be direct." || len(upstreamReq.Messages) != 1 || upstreamReq.Messages[0].Role != "user" {
		t.Fatalf("upstream request = %+v", upstreamReq)
	}
	var out struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Output []struct {
			Type    string `json:"type"`
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
		Usage struct {
			PromptTokens     int `json:"input_tokens"`
			CompletionTokens int `json:"output_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatalf("unmarshal translated response: %v", err)
	}
	if out.Object != "response" || out.Model != "anthropic/claude-sonnet" || len(out.Output) != 1 {
		t.Fatalf("response = %+v", out)
	}
	if out.Output[0].Role != "assistant" || out.Output[0].Content[0].Text != "Hello from Claude responses" {
		t.Fatalf("output = %+v", out.Output)
	}
	if seenAccept != "" {
		t.Fatalf("unexpected Accept header for non-streaming request: %q", seenAccept)
	}
}

func TestAnthropicResponsesStreamingTranslation(t *testing.T) {
	var seenBody []byte
	var seenAccept string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAccept = r.Header.Get("Accept")
		var err error
		seenBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\n")
		_, _ = io.WriteString(w, "data: {\"message\":{\"id\":\"msg_resp_stream\",\"usage\":{\"input_tokens\":9}}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello from Claude \"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"stream\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_delta\n")
		_, _ = io.WriteString(w, "data: {\"usage\":{\"input_tokens\":9,\"output_tokens\":4}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\n")
		_, _ = io.WriteString(w, "data: {}\n\n")
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"anthropic/claude-sonnet","stream":true,"input":"hello"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  config.ProviderTypeAnthropic,
		PublicModel:   "anthropic/claude-sonnet",
		BaseURL:       upstream.URL,
		APIKey:        "sk-ant",
		UpstreamModel: "claude-sonnet-4-20250514",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenAccept != "text/event-stream" {
		t.Fatalf("accept = %q", seenAccept)
	}
	if !strings.Contains(string(seenBody), `"stream":true`) {
		t.Fatalf("translated request missing stream flag: %s", string(seenBody))
	}
	if !res.Streaming || res.StreamBody == nil {
		t.Fatalf("expected streaming result")
	}
	defer res.StreamBody.Close()
	body, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"type":"response.created"`) {
		t.Fatalf("missing response.created event: %q", text)
	}
	if !strings.Contains(text, `"type":"response.output_text.delta"`) || !strings.Contains(text, `"delta":"Hello from Claude `) {
		t.Fatalf("missing translated delta event: %q", text)
	}
	if !strings.Contains(text, `"type":"response.output_text.done"`) {
		t.Fatalf("missing output_text.done event: %q", text)
	}
	if !strings.Contains(text, `"type":"response.completed"`) || !strings.Contains(text, `"text":"Hello from Claude stream"`) {
		t.Fatalf("missing completed response payload: %q", text)
	}
	if !strings.Contains(text, `"total_tokens":13`) {
		t.Fatalf("missing usage in completed payload: %q", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("missing done marker: %q", text)
	}
}

func TestGeminiResponsesTranslation(t *testing.T) {
	var seenBody []byte
	var seenAccept string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAccept = r.Header.Get("Accept")
		var err error
		seenBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello from Gemini responses"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":4,"totalTokenCount":9}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"gemini/gemini-2.5-pro","input":"hello","instructions":"Be direct."}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  config.ProviderTypeGemini,
		PublicModel:   "gemini/gemini-2.5-pro",
		BaseURL:       upstream.URL,
		APIKey:        "gem-key",
		UpstreamModel: "gemini-2.5-pro",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	var upstreamReq struct {
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
		Contents []struct {
			Role string `json:"role"`
		} `json:"contents"`
	}
	if err := json.Unmarshal(seenBody, &upstreamReq); err != nil {
		t.Fatalf("unmarshal upstream request: %v", err)
	}
	if upstreamReq.SystemInstruction.Parts[0].Text != "Be direct." || len(upstreamReq.Contents) != 1 || upstreamReq.Contents[0].Role != "user" {
		t.Fatalf("upstream request = %+v", upstreamReq)
	}
	var out struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Output []struct {
			Role    string `json:"role"`
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
		} `json:"output"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatalf("unmarshal translated response: %v", err)
	}
	if out.Object != "response" || out.Model != "gemini/gemini-2.5-pro" || len(out.Output) != 1 {
		t.Fatalf("response = %+v", out)
	}
	if out.Output[0].Role != "assistant" || out.Output[0].Content[0].Text != "Hello from Gemini responses" {
		t.Fatalf("output = %+v", out.Output)
	}
	if seenAccept != "" {
		t.Fatalf("unexpected Accept header for non-streaming request: %q", seenAccept)
	}
}

func TestGeminiResponsesStreamingTranslation(t *testing.T) {
	var seenBody []byte
	var seenPath string
	var seenAccept string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.RequestURI()
		seenAccept = r.Header.Get("Accept")
		var err error
		seenBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello from Gemini \"}]}}],\"usageMetadata\":{\"promptTokenCount\":5}}\n\n")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"stream\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":5,\"candidatesTokenCount\":3,\"totalTokenCount\":8}}\n\n")
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"gemini/gemini-2.5-pro","stream":true,"input":"hello"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  config.ProviderTypeGemini,
		PublicModel:   "gemini/gemini-2.5-pro",
		BaseURL:       upstream.URL,
		APIKey:        "gem-key",
		UpstreamModel: "gemini-2.5-pro",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenPath != "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse" {
		t.Fatalf("stream path = %q", seenPath)
	}
	if seenAccept != "text/event-stream" {
		t.Fatalf("accept = %q", seenAccept)
	}
	if !strings.Contains(string(seenBody), `"generationConfig"`) {
		t.Fatalf("translated request missing generationConfig: %s", string(seenBody))
	}
	if !res.Streaming || res.StreamBody == nil {
		t.Fatalf("expected streaming result")
	}
	defer res.StreamBody.Close()
	body, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"type":"response.created"`) {
		t.Fatalf("missing response.created event: %q", text)
	}
	if !strings.Contains(text, `"type":"response.output_text.delta"`) || !strings.Contains(text, `"delta":"Hello from Gemini `) {
		t.Fatalf("missing translated delta event: %q", text)
	}
	if !strings.Contains(text, `"type":"response.completed"`) || !strings.Contains(text, `"text":"Hello from Gemini stream"`) {
		t.Fatalf("missing completed response payload: %q", text)
	}
	if !strings.Contains(text, `"total_tokens":8`) {
		t.Fatalf("missing usage in completed payload: %q", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("missing done marker: %q", text)
	}
}

func TestAnthropicAdapterTranslatesRequestAndResponse(t *testing.T) {
	var seenAuth, seenVersion string
	var seenBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("x-api-key")
		seenVersion = r.Header.Get("anthropic-version")
		var err error
		seenBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_123","role":"assistant","content":[{"type":"text","text":"Hello from Claude"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"alias/chat_default","messages":[{"role":"system","content":"You are terse."},{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi"},{"role":"user","content":[{"type":"text","text":"Again"}]}],"max_tokens":64}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeAnthropic,
		PublicModel:   "alias/chat_default",
		BaseURL:       upstream.URL,
		APIKey:        "sk-ant-test",
		UpstreamModel: "claude-sonnet-4-20250514",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if seenAuth != "sk-ant-test" {
		t.Fatalf("x-api-key = %q", seenAuth)
	}
	if seenVersion != anthropicVersion {
		t.Fatalf("anthropic-version = %q", seenVersion)
	}
	var upstreamReq struct {
		Model     string `json:"model"`
		System    string `json:"system"`
		MaxTokens int    `json:"max_tokens"`
		Messages  []struct {
			Role    string `json:"role"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(seenBody, &upstreamReq); err != nil {
		t.Fatalf("unmarshal upstream request: %v", err)
	}
	if upstreamReq.Model != "claude-sonnet-4-20250514" {
		t.Fatalf("upstream model = %q", upstreamReq.Model)
	}
	if upstreamReq.System != "You are terse." {
		t.Fatalf("system = %q", upstreamReq.System)
	}
	if upstreamReq.MaxTokens != 64 {
		t.Fatalf("max_tokens = %d", upstreamReq.MaxTokens)
	}
	if len(upstreamReq.Messages) != 3 {
		t.Fatalf("message count = %d", len(upstreamReq.Messages))
	}
	if upstreamReq.Messages[2].Content[0].Text != "Again" {
		t.Fatalf("last content text = %q", upstreamReq.Messages[2].Content[0].Text)
	}

	var out struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatalf("unmarshal translated response: %v", err)
	}
	if out.Object != "chat.completion" {
		t.Fatalf("object = %q", out.Object)
	}
	if out.Model != "alias/chat_default" {
		t.Fatalf("model = %q", out.Model)
	}
	if out.Choices[0].Message.Role != "assistant" || out.Choices[0].Message.Content != "Hello from Claude" {
		t.Fatalf("choice message = %+v", out.Choices[0].Message)
	}
	if out.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %q", out.Choices[0].FinishReason)
	}
	if out.Usage.PromptTokens != 11 || out.Usage.CompletionTokens != 7 || out.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %+v", out.Usage)
	}
}

func TestAnthropicAdapterStreamingTranslation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\n")
		_, _ = io.WriteString(w, "data: {\"message\":{\"id\":\"msg_stream\"}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\n")
		_, _ = io.WriteString(w, "data: {}\n\n")
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"anthropic/claude-sonnet","stream":true,"messages":[{"role":"user","content":"Hi"}]}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeAnthropic,
		PublicModel:   "anthropic/claude-sonnet",
		BaseURL:       upstream.URL,
		APIKey:        "sk-ant-test",
		UpstreamModel: "claude-sonnet-4-20250514",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if !res.Streaming || res.StreamBody == nil {
		t.Fatalf("expected streaming result with body")
	}
	defer res.StreamBody.Close()
	body, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"role":"assistant"`) {
		t.Fatalf("expected assistant role chunk, got %q", text)
	}
	if !strings.Contains(text, `"content":"Hello"`) {
		t.Fatalf("expected translated content chunk, got %q", text)
	}
	if !strings.Contains(text, `"finish_reason":"stop"`) {
		t.Fatalf("expected translated finish reason, got %q", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("expected done marker, got %q", text)
	}
}

func TestGeminiAdapterTranslatesRequestAndResponse(t *testing.T) {
	var seenAuth string
	var seenBody []byte
	var seenPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("x-goog-api-key")
		seenPath = r.URL.Path
		var err error
		seenBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hello from Gemini"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":13,"candidatesTokenCount":5,"totalTokenCount":18}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"alias/chat_default","messages":[{"role":"system","content":"Be direct."},{"role":"user","content":"Hello"},{"role":"assistant","content":"Hi"},{"role":"user","content":[{"type":"text","text":"Again"}]}],"max_tokens":32}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGemini,
		PublicModel:   "alias/chat_default",
		BaseURL:       upstream.URL,
		APIKey:        "gem-key",
		UpstreamModel: "gemini-2.5-pro",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if seenAuth != "gem-key" {
		t.Fatalf("x-goog-api-key = %q", seenAuth)
	}
	if seenPath != "/v1beta/models/gemini-2.5-pro:generateContent" {
		t.Fatalf("path = %q", seenPath)
	}
	var upstreamReq struct {
		Contents []struct {
			Role  string `json:"role"`
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"contents"`
		SystemInstruction struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"systemInstruction"`
		GenerationConfig struct {
			MaxOutputTokens int `json:"maxOutputTokens"`
		} `json:"generationConfig"`
	}
	if err := json.Unmarshal(seenBody, &upstreamReq); err != nil {
		t.Fatalf("unmarshal upstream request: %v", err)
	}
	if upstreamReq.SystemInstruction.Parts[0].Text != "Be direct." {
		t.Fatalf("systemInstruction = %+v", upstreamReq.SystemInstruction)
	}
	if upstreamReq.GenerationConfig.MaxOutputTokens != 32 {
		t.Fatalf("maxOutputTokens = %d", upstreamReq.GenerationConfig.MaxOutputTokens)
	}
	if len(upstreamReq.Contents) != 3 {
		t.Fatalf("contents count = %d", len(upstreamReq.Contents))
	}
	if upstreamReq.Contents[1].Role != "model" {
		t.Fatalf("assistant role not mapped to model: %+v", upstreamReq.Contents[1])
	}
	if upstreamReq.Contents[2].Parts[0].Text != "Again" {
		t.Fatalf("last content text = %q", upstreamReq.Contents[2].Parts[0].Text)
	}

	var out struct {
		Object  string `json:"object"`
		Model   string `json:"model"`
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatalf("unmarshal translated response: %v", err)
	}
	if out.Object != "chat.completion" {
		t.Fatalf("object = %q", out.Object)
	}
	if out.Model != "alias/chat_default" {
		t.Fatalf("model = %q", out.Model)
	}
	if out.Choices[0].Message.Role != "assistant" || out.Choices[0].Message.Content != "Hello from Gemini" {
		t.Fatalf("choice message = %+v", out.Choices[0].Message)
	}
	if out.Choices[0].FinishReason != "stop" {
		t.Fatalf("finish_reason = %q", out.Choices[0].FinishReason)
	}
	if out.Usage.PromptTokens != 13 || out.Usage.CompletionTokens != 5 || out.Usage.TotalTokens != 18 {
		t.Fatalf("usage = %+v", out.Usage)
	}
}

func TestGeminiAdapterStreamingTranslation(t *testing.T) {
	var seenPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[]},\"finishReason\":\"STOP\"}]}\n\n")
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"gemini/gemini-2.5-pro","stream":true,"messages":[{"role":"user","content":"Hi"}]}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeGemini,
		PublicModel:   "gemini/gemini-2.5-pro",
		BaseURL:       upstream.URL,
		APIKey:        "gem-key",
		UpstreamModel: "gemini-2.5-pro",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenPath != "/v1beta/models/gemini-2.5-pro:streamGenerateContent?alt=sse" {
		t.Fatalf("stream path = %q", seenPath)
	}
	if !res.Streaming || res.StreamBody == nil {
		t.Fatalf("expected streaming result with body")
	}
	defer res.StreamBody.Close()
	body, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	text := string(body)
	if !strings.Contains(text, `"role":"assistant"`) {
		t.Fatalf("expected assistant role chunk, got %q", text)
	}
	if !strings.Contains(text, `"content":"Hello"`) {
		t.Fatalf("expected translated content chunk, got %q", text)
	}
	if !strings.Contains(text, `"finish_reason":"stop"`) {
		t.Fatalf("expected translated finish reason, got %q", text)
	}
	if !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("expected done marker, got %q", text)
	}
}

func TestGeminiAdapterEmbeddingsSingle(t *testing.T) {
	var seenAuth, seenPath string
	var seenBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("x-goog-api-key")
		seenPath = r.URL.Path
		var err error
		seenBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embedding":{"values":[0.1,0.2,0.3]},"usageMetadata":{"promptTokenCount":4,"totalTokenCount":4}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/embeddings",
		io.NopCloser(strings.NewReader(`{"model":"gemini/text-embedding-004","input":"hello","dimensions":3}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpEmbeddings,
		ProviderType:  config.ProviderTypeGemini,
		PublicModel:   "gemini/text-embedding-004",
		BaseURL:       upstream.URL,
		APIKey:        "gem-key",
		UpstreamModel: "text-embedding-004",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenAuth != "gem-key" {
		t.Fatalf("x-goog-api-key = %q", seenAuth)
	}
	if seenPath != "/v1beta/models/text-embedding-004:embedContent" {
		t.Fatalf("path = %q", seenPath)
	}
	var upstreamReq struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		OutputDimensionality int `json:"outputDimensionality"`
	}
	if err := json.Unmarshal(seenBody, &upstreamReq); err != nil {
		t.Fatalf("unmarshal upstream request: %v", err)
	}
	if upstreamReq.Content.Parts[0].Text != "hello" {
		t.Fatalf("text = %q", upstreamReq.Content.Parts[0].Text)
	}
	if upstreamReq.OutputDimensionality != 3 {
		t.Fatalf("outputDimensionality = %d", upstreamReq.OutputDimensionality)
	}
	var out struct {
		Object string `json:"object"`
		Model  string `json:"model"`
		Data   []struct {
			Object    string    `json:"object"`
			Embedding []float64 `json:"embedding"`
			Index     int       `json:"index"`
		} `json:"data"`
		Usage struct {
			PromptTokens int `json:"prompt_tokens"`
			TotalTokens  int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatalf("unmarshal translated response: %v", err)
	}
	if out.Object != "list" || out.Model != "gemini/text-embedding-004" {
		t.Fatalf("response = %+v", out)
	}
	if len(out.Data) != 1 || len(out.Data[0].Embedding) != 3 || out.Data[0].Index != 0 {
		t.Fatalf("data = %+v", out.Data)
	}
	if out.Usage.PromptTokens != 4 || out.Usage.TotalTokens != 4 {
		t.Fatalf("usage = %+v", out.Usage)
	}
}

func TestGeminiAdapterEmbeddingsBatch(t *testing.T) {
	var seenPath string
	var seenBody []byte
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		var err error
		seenBody, err = io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"embeddings":[{"values":[0.1,0.2]},{"values":[0.3,0.4]}],"usageMetadata":{"promptTokenCount":8,"totalTokenCount":8}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/embeddings",
		io.NopCloser(strings.NewReader(`{"model":"gemini/text-embedding-004","input":["hello","world"]}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpEmbeddings,
		ProviderType:  config.ProviderTypeGemini,
		PublicModel:   "gemini/text-embedding-004",
		BaseURL:       upstream.URL,
		APIKey:        "gem-key",
		UpstreamModel: "text-embedding-004",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenPath != "/v1beta/models/text-embedding-004:batchEmbedContents" {
		t.Fatalf("path = %q", seenPath)
	}
	var upstreamReq struct {
		Requests []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"requests"`
	}
	if err := json.Unmarshal(seenBody, &upstreamReq); err != nil {
		t.Fatalf("unmarshal upstream request: %v", err)
	}
	if len(upstreamReq.Requests) != 2 || upstreamReq.Requests[1].Content.Parts[0].Text != "world" {
		t.Fatalf("requests = %+v", upstreamReq.Requests)
	}
	var out struct {
		Data []struct {
			Index int `json:"index"`
		} `json:"data"`
	}
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatalf("unmarshal translated response: %v", err)
	}
	if len(out.Data) != 2 || out.Data[1].Index != 1 {
		t.Fatalf("data = %+v", out.Data)
	}
}
