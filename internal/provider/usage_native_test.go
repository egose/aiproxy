package provider

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

const nativeResponsesUsageBody = `{"id":"resp_1","object":"response","model":"gpt-4.1","output":[{"type":"message","role":"assistant","content":[{"type":"output_text","text":"hello"}]}],"status":"completed","usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}}`

func nativeResponsesRequest(t *testing.T, providerType config.ProviderType, protocol config.ModelProtocol, upstream *httptest.Server, body string) *Result {
	t.Helper()
	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(body)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  providerType,
		PublicModel:   "test/gpt",
		BaseURL:       upstream.URL,
		APIKey:        "sk-test",
		UpstreamModel: "gpt-4.1",
		ModelProtocol: protocol,
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	return res
}

func TestNativeResponsesJSONUsageCounts(t *testing.T) {
	for _, tc := range []struct {
		name         string
		providerType config.ProviderType
		protocol     config.ModelProtocol
		path         string
	}{
		{"openai", config.ProviderTypeOpenAI, "", "/v1/responses"},
		{"openai-compatible", config.ProviderTypeOpenAICompatible, "", "/v1/responses"},
		{"opencode-zen", config.ProviderTypeOpenCodeZen, config.ModelProtocolResponses, "/v1/responses"},
		{"opencode-go", config.ProviderTypeOpenCodeGo, config.ModelProtocolResponses, "/v1/responses"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seenPath string
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seenPath = r.URL.Path
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, nativeResponsesUsageBody)
			}))
			defer upstream.Close()
			res := nativeResponsesRequest(t, tc.providerType, tc.protocol, upstream, `{"model":"test/gpt","input":"hi"}`)
			if seenPath != tc.path {
				t.Fatalf("path = %q, want %q", seenPath, tc.path)
			}
			if res.Streaming {
				t.Fatal("non-streaming responses request should not stream")
			}
			if string(res.Body) != nativeResponsesUsageBody {
				t.Fatalf("body not preserved: got %q want %q", string(res.Body), nativeResponsesUsageBody)
			}
			if res.Usage.PromptTokens != 4 || res.Usage.CompletionTokens != 6 || res.Usage.TotalTokens != 10 {
				t.Fatalf("usage = %+v, want 4/6/10", res.Usage)
			}
		})
	}
}

func TestNativeResponsesJSONAbsentAndZero(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
	}{
		{"absent", `{"id":"resp_1","object":"response","model":"gpt-4.1","output":[],"status":"completed"}`},
		{"zero", `{"id":"resp_1","object":"response","model":"gpt-4.1","output":[],"status":"completed","usage":{"input_tokens":0,"output_tokens":0,"total_tokens":0}}`},
		{"empty-usage", `{"id":"resp_1","object":"response","model":"gpt-4.1","output":[],"status":"completed","usage":{}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.body)
			}))
			defer upstream.Close()
			res := nativeResponsesRequest(t, config.ProviderTypeOpenAI, "", upstream, `{"model":"test/gpt","input":"hi"}`)
			if string(res.Body) != tc.body {
				t.Fatalf("body not preserved: got %q want %q", string(res.Body), tc.body)
			}
			if res.Usage.Has() {
				t.Fatalf("usage must be absent, got %+v", res.Usage)
			}
		})
	}
}

func TestNativeResponsesChatKeysUnchanged(t *testing.T) {
	const chatBody = `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4.1","choices":[],"usage":{"prompt_tokens":12,"completion_tokens":8,"total_tokens":20}}`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, chatBody)
	}))
	defer upstream.Close()
	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"openai/gpt-4.1","messages":[{"role":"user","content":"hi"}]}`)))
	res, err := a.Do(context.Background(), Request{
		Operation: OpChatCompletions, ProviderType: config.ProviderTypeOpenAI,
		PublicModel: "openai/gpt-4.1", BaseURL: upstream.URL, APIKey: "sk-test",
		UpstreamModel: "gpt-4.1", Inbound: inbound, Client: upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	if string(res.Body) != chatBody {
		t.Fatalf("body not preserved: %q", string(res.Body))
	}
	if res.Usage.PromptTokens != 12 || res.Usage.CompletionTokens != 8 || res.Usage.TotalTokens != 20 {
		t.Fatalf("usage = %+v", res.Usage)
	}
}

func TestNativeResponsesStreamNestedUsageFragmented(t *testing.T) {
	completed := `{"type":"response.completed","response":{"id":"resp_1","object":"response","model":"gpt-4.1","status":"completed","output":[],"usage":{"input_tokens":4,"output_tokens":6,"total_tokens":10}}}`
	var wire strings.Builder
	delta := `data: {"type":"response.output_text.delta","item_id":"resp_1_msg","output_index":0,"content_index":0,"delta":"hello"}` + "\n\n"
	wire.WriteString(delta)
	fullCompleted := "data: " + completed + "\n\n"
	wire.WriteString(fullCompleted)
	wantWire := wire.String()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = io.WriteString(w, delta)
		if flusher != nil {
			flusher.Flush()
		}
		mid := len(fullCompleted) / 2
		_, _ = io.WriteString(w, fullCompleted[:mid])
		if flusher != nil {
			flusher.Flush()
		}
		_, _ = io.WriteString(w, fullCompleted[mid:])
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"test/gpt","stream":true,"input":"hi"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation: OpResponses, ProviderType: config.ProviderTypeOpenAI,
		PublicModel: "test/gpt", BaseURL: upstream.URL, APIKey: "sk-test",
		UpstreamModel: "gpt-4.1", Inbound: inbound, Client: upstream.Client(),
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
	if string(out) != wantWire {
		t.Fatalf("stream bytes not preserved: got %q want %q", string(out), wantWire)
	}
	res.Stream.Complete(nil, false)
	usage := res.Stream.Wait().Usage
	if usage.PromptTokens != 4 || usage.CompletionTokens != 6 || usage.TotalTokens != 10 {
		t.Fatalf("stream usage = %+v, want 4/6/10", usage)
	}
}

func TestNativeResponsesStreamNoUsageStaysAbsent(t *testing.T) {
	payload := "data: " + `{"type":"response.completed","response":{"id":"resp_1","status":"completed"}}` + "\n\n"
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, payload)
	}))
	defer upstream.Close()
	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"test/gpt","stream":true,"input":"hi"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation: OpResponses, ProviderType: config.ProviderTypeOpenAI,
		PublicModel: "test/gpt", BaseURL: upstream.URL, APIKey: "sk-test",
		UpstreamModel: "gpt-4.1", Inbound: inbound, Client: upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer res.StreamBody.Close()
	out, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	if string(out) != payload {
		t.Fatalf("stream bytes not preserved: %q", string(out))
	}
	res.Stream.Complete(nil, false)
	if got := res.Stream.Wait().Usage; got.Has() {
		t.Fatalf("usage must be absent, got %+v", got)
	}
}
