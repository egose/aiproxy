package provider

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func decodeResponsesUsage(t *testing.T, body []byte) (map[string]int, string) {
	t.Helper()
	var envelope struct {
		Usage map[string]int `json:"usage"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		t.Fatalf("unmarshal responses body: %v", err)
	}
	return envelope.Usage, string(body)
}

func assertResponsesWire(t *testing.T, raw string, usage map[string]int, wantInput, wantOutput, wantTotal int) {
	t.Helper()
	if strings.Contains(raw, "prompt_tokens") || strings.Contains(raw, "completion_tokens") {
		t.Fatalf("responses wire must not contain chat usage keys: %q", raw)
	}
	if usage["input_tokens"] != wantInput || usage["output_tokens"] != wantOutput || usage["total_tokens"] != wantTotal {
		t.Fatalf("wire usage = %v, want input/output/total %d/%d/%d", usage, wantInput, wantOutput, wantTotal)
	}
	if len(usage) != 3 {
		t.Fatalf("wire usage keys = %v, want exactly input/output/total", usage)
	}
}

func completedUsageFromStream(t *testing.T, text string) (map[string]int, string) {
	t.Helper()
	var found map[string]int
	for _, chunk := range strings.Split(text, "data: ") {
		chunk = strings.TrimSpace(chunk)
		if chunk == "" || chunk == "[DONE]" {
			continue
		}
		end := strings.Index(chunk, "\n")
		payload := chunk
		if end >= 0 {
			payload = chunk[:end]
		}
		var evt struct {
			Type     string `json:"type"`
			Response struct {
				Usage map[string]int `json:"usage"`
			} `json:"response"`
		}
		if err := json.Unmarshal([]byte(payload), &evt); err != nil {
			continue
		}
		if evt.Type == "response.completed" {
			found = evt.Response.Usage
		}
	}
	if found == nil {
		t.Fatalf("missing response.completed event in %q", text)
	}
	return found, text
}

func TestResponsesJSONAnthropicWireUsage(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_resp","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":6}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"anthropic/m","input":"hello"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  config.ProviderTypeAnthropic,
		PublicModel:   "anthropic/m",
		BaseURL:       upstream.URL,
		APIKey:        "k",
		UpstreamModel: "m",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	usage, raw := decodeResponsesUsage(t, res.Body)
	assertResponsesWire(t, raw, usage, 9, 6, 15)
	if res.Usage.PromptTokens != 9 || res.Usage.CompletionTokens != 6 || res.Usage.TotalTokens != 15 {
		t.Fatalf("internal usage = %+v", res.Usage)
	}
}

func TestResponsesJSONGeminiAuthoritativeAndMissingTotals(t *testing.T) {
	for _, tc := range []struct {
		name                           string
		upstream                       string
		wantInput, wantOutput, wantSum int
	}{
		{"authoritative", `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":4,"totalTokenCount":20}}`, 5, 4, 20},
		{"missing", `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":4}}`, 5, 4, 9},
		{"zero", `{"candidates":[{"content":{"role":"model","parts":[{"text":"hi"}]},"finishReason":"STOP"}]}`, 0, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(tc.upstream))
			}))
			defer upstream.Close()

			a := New()
			inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
				io.NopCloser(strings.NewReader(`{"model":"gemini/m","input":"hello"}`)))
			res, err := a.Do(context.Background(), Request{
				Operation:     OpResponses,
				ProviderType:  config.ProviderTypeGemini,
				PublicModel:   "gemini/m",
				BaseURL:       upstream.URL,
				APIKey:        "k",
				UpstreamModel: "m",
				Inbound:       inbound,
				Client:        upstream.Client(),
			})
			if err != nil {
				t.Fatalf("do: %v", err)
			}
			raw := string(res.Body)
			if tc.name == "zero" {
				if strings.Contains(raw, `"usage"`) {
					t.Fatalf("zero usage must be omitted: %q", raw)
				}
				if res.Usage.Has() {
					t.Fatalf("internal usage must be empty: %+v", res.Usage)
				}
				return
			}
			usage, _ := decodeResponsesUsage(t, res.Body)
			assertResponsesWire(t, raw, usage, tc.wantInput, tc.wantOutput, tc.wantSum)
		})
	}
}

func TestResponsesStreamSplitUsageReconciles(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "event: message_start\n")
		_, _ = io.WriteString(w, "data: {\"message\":{\"id\":\"msg_split\",\"usage\":{\"input_tokens\":7}}}\n\n")
		_, _ = io.WriteString(w, "event: content_block_delta\n")
		_, _ = io.WriteString(w, "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"hi\"}}\n\n")
		_, _ = io.WriteString(w, "event: message_delta\n")
		_, _ = io.WriteString(w, "data: {\"usage\":{\"output_tokens\":11}}\n\n")
		_, _ = io.WriteString(w, "event: message_stop\n")
		_, _ = io.WriteString(w, "data: {}\n\n")
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/responses",
		io.NopCloser(strings.NewReader(`{"model":"anthropic/m","stream":true,"input":"hello"}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpResponses,
		ProviderType:  config.ProviderTypeAnthropic,
		PublicModel:   "anthropic/m",
		BaseURL:       upstream.URL,
		APIKey:        "k",
		UpstreamModel: "m",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer res.StreamBody.Close()
	body, err := io.ReadAll(res.StreamBody)
	if err != nil {
		t.Fatalf("read stream: %v", err)
	}
	text := string(body)
	if strings.Contains(text, "prompt_tokens") || strings.Contains(text, "completion_tokens") {
		t.Fatalf("stream wire must not contain chat usage keys: %q", text)
	}
	usage, _ := completedUsageFromStream(t, text)
	assertResponsesWire(t, text, usage, 7, 11, 18)

	state := newResponsesStreamState("anthropic/m", "msg_split")
	stream := NewStreamCompletion()
	if _, err := processAnthropicResponsesEvent(io.Discard, "message_start", `{"message":{"id":"msg_split","usage":{"input_tokens":7}}}`, state, stream); err != nil {
		t.Fatalf("message_start: %v", err)
	}
	if _, err := processAnthropicResponsesEvent(io.Discard, "message_delta", `{"usage":{"output_tokens":11}}`, state, stream); err != nil {
		t.Fatalf("message_delta: %v", err)
	}
	if state.Usage.InputTokens != 7 || state.Usage.OutputTokens != 11 || state.Usage.TotalTokens != 18 {
		t.Fatalf("wire state = %+v", state.Usage)
	}
	stream.Complete(nil, false)
	internal := stream.Wait().Usage
	if internal.PromptTokens != 7 || internal.CompletionTokens != 11 || internal.TotalTokens != 18 {
		t.Fatalf("internal usage = %+v", internal)
	}
}

func TestResponsesStreamZeroUsageOmitted(t *testing.T) {
	state := newResponsesStreamState("anthropic/m", "msg_zero")
	stream := NewStreamCompletion()
	if _, err := processAnthropicResponsesEvent(io.Discard, "message_start", `{"message":{"id":"msg_zero"}}`, state, stream); err != nil {
		t.Fatalf("message_start: %v", err)
	}
	if _, err := processAnthropicResponsesEvent(io.Discard, "message_delta", `{"usage":{}}`, state, stream); err != nil {
		t.Fatalf("message_delta: %v", err)
	}
	var sb strings.Builder
	if err := writeResponsesCompleted(&sb, state); err != nil {
		t.Fatalf("completed: %v", err)
	}
	if strings.Contains(sb.String(), `"usage"`) {
		t.Fatalf("zero usage must be omitted: %q", sb.String())
	}
	stream.Complete(nil, false)
	if got := stream.Wait().Usage; got.Has() {
		t.Fatalf("internal usage must be empty: %+v", got)
	}
}

func TestResponsesStreamGeminiAuthoritativeTotal(t *testing.T) {
	state := newResponsesStreamState("gemini/m", "resp_gemini")
	stream := NewStreamCompletion()
	if _, err := processGeminiResponsesPayload(io.Discard, `{"candidates":[{"content":{"parts":[{"text":"hi"}]}}],"usageMetadata":{"promptTokenCount":5}}`, state, stream); err != nil {
		t.Fatalf("first payload: %v", err)
	}
	if _, err := processGeminiResponsesPayload(io.Discard, `{"candidates":[{"content":{"parts":[{"text":"!"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":4,"totalTokenCount":20}}`, state, stream); err != nil {
		t.Fatalf("second payload: %v", err)
	}
	if state.Usage.InputTokens != 5 || state.Usage.OutputTokens != 4 || state.Usage.TotalTokens != 20 {
		t.Fatalf("wire state = %+v", state.Usage)
	}
	stream.Complete(nil, false)
	internal := stream.Wait().Usage
	if internal.PromptTokens != 5 || internal.CompletionTokens != 4 || internal.TotalTokens != 20 {
		t.Fatalf("internal usage = %+v", internal)
	}
}

func TestResponsesChatSerializationUnchanged(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"msg_1","role":"assistant","content":[{"type":"text","text":"hi"}],"stop_reason":"end_turn","usage":{"input_tokens":11,"output_tokens":7}}`))
	}))
	defer upstream.Close()

	a := New()
	inbound := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		io.NopCloser(strings.NewReader(`{"model":"anthropic/m","messages":[{"role":"user","content":"hi"}]}`)))
	res, err := a.Do(context.Background(), Request{
		Operation:     OpChatCompletions,
		ProviderType:  config.ProviderTypeAnthropic,
		PublicModel:   "anthropic/m",
		BaseURL:       upstream.URL,
		APIKey:        "k",
		UpstreamModel: "m",
		Inbound:       inbound,
		Client:        upstream.Client(),
	})
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	raw := string(res.Body)
	if !strings.Contains(raw, `"prompt_tokens":11`) || !strings.Contains(raw, `"completion_tokens":7`) || !strings.Contains(raw, `"total_tokens":18`) {
		t.Fatalf("chat wire must keep chat keys: %q", raw)
	}
	if strings.Contains(raw, "input_tokens") || strings.Contains(raw, "output_tokens") {
		t.Fatalf("chat wire must not contain responses keys: %q", raw)
	}
}

func TestResponsesOpenCodeReuseWireUsage(t *testing.T) {
	t.Run("messages", func(t *testing.T) {
		cap := &openCodeCapture{}
		upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"id":"msg_resp","role":"assistant","content":[{"type":"text","text":"Hello"}],"stop_reason":"end_turn","usage":{"input_tokens":9,"output_tokens":6}}`))
		defer upstream.Close()
		res, _, _ := openCodeDo(t, upstream, config.ProviderTypeOpenCodeZen, config.ModelProtocolMessages, OpResponses, "zen/claude", "claude-sonnet-5", `{"model":"zen/claude","input":"hello"}`)
		usage, raw := decodeResponsesUsage(t, res.Body)
		assertResponsesWire(t, raw, usage, 9, 6, 15)
		if res.Usage.PromptTokens != 9 || res.Usage.CompletionTokens != 6 || res.Usage.TotalTokens != 15 {
			t.Fatalf("internal usage = %+v", res.Usage)
		}
	})
	t.Run("gemini", func(t *testing.T) {
		cap := &openCodeCapture{}
		upstream := openCodeUpstream(t, cap, openCodeJSONResponder(`{"candidates":[{"content":{"role":"model","parts":[{"text":"Hi"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":5,"candidatesTokenCount":2,"totalTokenCount":12}}`))
		defer upstream.Close()
		res, _, _ := openCodeDo(t, upstream, config.ProviderTypeOpenCodeZen, config.ModelProtocolGemini, OpResponses, "zen/gemini", "gemini-3.8-flash", `{"model":"zen/gemini","input":"hi"}`)
		usage, raw := decodeResponsesUsage(t, res.Body)
		assertResponsesWire(t, raw, usage, 5, 2, 12)
	})
	t.Run("messages-stream-split", func(t *testing.T) {
		cap := &openCodeCapture{}
		upstream := openCodeUpstream(t, cap, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "event: message_start\n")
			_, _ = io.WriteString(w, "data: {\"message\":{\"id\":\"msg_oc\",\"usage\":{\"input_tokens\":7}}}\n\n")
			_, _ = io.WriteString(w, "event: message_delta\n")
			_, _ = io.WriteString(w, "data: {\"usage\":{\"output_tokens\":11}}\n\n")
			_, _ = io.WriteString(w, "event: message_stop\n")
			_, _ = io.WriteString(w, "data: {}\n\n")
		})
		defer upstream.Close()
		inbound := openCodeInbound(http.MethodPost, "/v1/responses", `{"model":"zen/claude","stream":true,"input":"hello"}`)
		res, err := New().Do(context.Background(), Request{
			Operation: OpResponses, ProviderType: config.ProviderTypeOpenCodeZen,
			PublicModel: "zen/claude", BaseURL: upstream.URL, APIKey: "sk-opencode",
			UpstreamModel: "claude-sonnet-5", ModelProtocol: config.ModelProtocolMessages,
			Version: "1.2.3-test", Body: []byte(`{"model":"zen/claude","stream":true,"input":"hello"}`),
			Inbound: inbound, Client: upstream.Client(),
		})
		if err != nil {
			t.Fatalf("do: %v", err)
		}
		defer res.StreamBody.Close()
		body, err := io.ReadAll(res.StreamBody)
		if err != nil {
			t.Fatalf("read stream: %v", err)
		}
		usage, text := completedUsageFromStream(t, string(body))
		assertResponsesWire(t, text, usage, 7, 11, 18)
	})
}
