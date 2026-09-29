package provider

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func TestOpenRouterChatPassthroughSetsAttributionHeaders(t *testing.T) {
	var seenAuth, seenBody, seenReferer, seenTitle string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenAuth = r.Header.Get("Authorization")
		seenReferer = r.Header.Get("HTTP-Referer")
		seenTitle = r.Header.Get("X-Title")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"chatcmpl-1","model":"upstream-model"}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"openrouter/upstream-model","messages":[]}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeOpenRouter,
		PublicModel:   "openrouter/upstream-model",
		BaseURL:       upstream.URL,
		APIKey:        "sk-or-test",
		UpstreamModel: "upstream-model",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", res.StatusCode)
	}
	if seenAuth != "Bearer sk-or-test" {
		t.Fatalf("auth = %q", seenAuth)
	}
	if !strings.Contains(seenBody, `"model":"upstream-model"`) {
		t.Fatalf("model was not rewritten: %s", seenBody)
	}
	if seenReferer != "https://opencode.ai/" {
		t.Fatalf("HTTP-Referer = %q", seenReferer)
	}
	if seenTitle != "opencode" {
		t.Fatalf("X-Title = %q", seenTitle)
	}
}

func TestOpenAIFamilyDoesNotSetAttributionHeaders(t *testing.T) {
	for _, providerType := range []config.ProviderType{
		config.ProviderTypeOpenAI,
		config.ProviderTypeOpenAICompatible,
		config.ProviderTypeZenMux,
	} {
		t.Run(string(providerType), func(t *testing.T) {
			var seenReferer, seenTitle string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenReferer = r.Header.Get("HTTP-Referer")
				seenTitle = r.Header.Get("X-Title")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"chatcmpl-1"}`))
			}))
			defer upstream.Close()

			_, err := New().Do(context.Background(), Request{
				Operation:     OpChatCompletions,
				ProviderType:  providerType,
				PublicModel:   "p/m",
				BaseURL:       upstream.URL,
				APIKey:        "k",
				UpstreamModel: "m",
				Inbound:       httptest.NewRequest(http.MethodPost, "/v1/chat/completions", io.NopCloser(strings.NewReader(`{"model":"p/m","messages":[]}`))),
				Client:        upstream.Client(),
			})
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			if seenReferer != "" {
				t.Fatalf("HTTP-Referer = %q, want empty", seenReferer)
			}
			if seenTitle != "" {
				t.Fatalf("X-Title = %q, want empty", seenTitle)
			}
		})
	}
}

func TestOpenRouterAudioTranscriptionsSetsAttributionHeaders(t *testing.T) {
	var seenReferer, seenTitle string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenReferer = r.Header.Get("HTTP-Referer")
		seenTitle = r.Header.Get("X-Title")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"hello world"}`))
	}))
	defer upstream.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	modelPart, _ := writer.CreateFormField("model")
	_, _ = io.WriteString(modelPart, "openrouter/whisper-1")
	filePart, _ := writer.CreateFormFile("file", "sample.wav")
	_, _ = io.WriteString(filePart, "audio-bytes")
	_ = writer.Close()

	inbound := httptest.NewRequest(http.MethodPost, "/v1/audio/transcriptions", io.NopCloser(bytes.NewReader(body.Bytes())))
	inbound.Header.Set("Content-Type", writer.FormDataContentType())
	_, err := New().Do(context.Background(), Request{
		Operation:     OpAudioTranscriptions,
		ProviderType:  config.ProviderTypeOpenRouter,
		PublicModel:   "openrouter/whisper-1",
		BaseURL:       upstream.URL,
		APIKey:        "sk-or-test",
		UpstreamModel: "whisper-1",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if seenReferer != "https://opencode.ai/" {
		t.Fatalf("HTTP-Referer = %q", seenReferer)
	}
	if seenTitle != "opencode" {
		t.Fatalf("X-Title = %q", seenTitle)
	}
}

func TestOpenRouterRejectsMessages(t *testing.T) {
	inbound := httptest.NewRequest(http.MethodPost, "/v1/messages",
		io.NopCloser(strings.NewReader(`{"model":"openrouter/m","messages":[]}`)))
	_, err := New().Do(context.Background(), Request{
		Operation:     OpMessages,
		ProviderType:  config.ProviderTypeOpenRouter,
		PublicModel:   "openrouter/m",
		BaseURL:       "http://127.0.0.1:9",
		APIKey:        "k",
		UpstreamModel: "m",
		Inbound:       inbound,
		Client:        http.DefaultClient,
	})
	if err == nil {
		t.Fatal("expected unsupported error")
	}
	var unsupported ErrUnsupportedOperation
	if !errors.As(err, &unsupported) {
		t.Fatalf("expected ErrUnsupportedOperation, got %T: %v", err, err)
	}
	if unsupported.ProviderType != config.ProviderTypeOpenRouter || unsupported.Operation != OpMessages {
		t.Fatalf("unsupported = %+v", unsupported)
	}
}

func TestOpenRouterChatSSEPassthroughRecordsUsage(t *testing.T) {
	var seenBody, seenReferer, seenTitle string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenReferer = r.Header.Get("HTTP-Referer")
		seenTitle = r.Header.Get("X-Title")
		b, _ := io.ReadAll(r.Body)
		seenBody = string(b)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"content\":\"Hel\"}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":4,\"completion_tokens\":6,\"total_tokens\":10}}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer upstream.Close()

	body := `{"model":"openrouter/openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}],"stream":true}`
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(body)))
	res, err := New().Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeOpenRouter,
		PublicModel:   "openrouter/openai/gpt-4o-mini",
		BaseURL:       upstream.URL,
		APIKey:        "sk-or-test",
		UpstreamModel: "openai/gpt-4o-mini",
		Inbound:       inbound,
		Client:        upstream.Client(),
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
	if !strings.Contains(text, "Hel") || !strings.Contains(text, "data: [DONE]") {
		t.Fatalf("stream = %q", text)
	}
	if !strings.Contains(seenBody, `"model":"openai/gpt-4o-mini"`) {
		t.Fatalf("model was not rewritten: %s", seenBody)
	}
	if seenReferer != "https://opencode.ai/" {
		t.Fatalf("HTTP-Referer = %q", seenReferer)
	}
	if seenTitle != "opencode" {
		t.Fatalf("X-Title = %q", seenTitle)
	}
	res.Stream.Complete(nil, false)
	if usage := res.Stream.Wait().Usage; usage.PromptTokens != 4 || usage.CompletionTokens != 6 || usage.TotalTokens != 10 {
		t.Fatalf("stream usage = %+v", usage)
	}
}

func TestOpenRouterDefaultBaseURL(t *testing.T) {
	desc, ok := providerDescriptors[config.ProviderTypeOpenRouter]
	if !ok {
		t.Fatal("providerDescriptors[openrouter] missing")
	}
	if desc.defaultBaseURL != "https://openrouter.ai/api/v1" {
		t.Fatalf("defaultBaseURL = %q", desc.defaultBaseURL)
	}
}
