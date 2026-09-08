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
)

// STREAM-02 release note: translated Anthropic/Gemini streams track protocol
// success separately from transport EOF. Protocol error envelopes fail the
// pipe with an "upstream stream error"; EOF without the terminal event fails
// with "upstream stream truncated" instead of synthesizing [DONE] or
// response.completed. Valid EOF-delimited terminals still succeed exactly
// once; ping/empty non-error extensions remain ignorable.

func TestTranslatedStreamErrorAndEOF(t *testing.T) {
	anthropicStart := "event: message_start\n" + "data: {\"message\":{\"id\":\"msg_1\"}}\n\n"
	anthropicDelta := "event: content_block_delta\n" + "data: {\"delta\":{\"type\":\"text_delta\",\"text\":\"Hello\"}}\n\n"
	anthropicStopDelta := "event: message_delta\n" + "data: {\"delta\":{\"stop_reason\":\"end_turn\"}}\n\n"
	anthropicStop := "event: message_stop\n" + "data: {}\n\n"
	anthropicStopNoDelim := "event: message_stop\n" + "data: {}"
	anthropicError := "event: error\n" + "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n"
	anthropicPing := "event: ping\n" + "data: {\"type\":\"ping\"}\n\n"

	geminiContent := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello\"}]}}]}\n\n"
	geminiFinish := "data: {\"candidates\":[{\"content\":{\"parts\":[]},\"finishReason\":\"STOP\"}]}\n\n"
	geminiFinishNoDelim := "data: {\"candidates\":[{\"content\":{\"parts\":[]},\"finishReason\":\"STOP\"}]}"
	geminiError := "data: {\"error\":{\"status\":\"UNAVAILABLE\",\"message\":\"busy\"}}\n\n"

	tests := []struct {
		name           string
		kind           string
		translate      func(io.ReadCloser, string, *StreamCompletion) io.ReadCloser
		upstream       string
		wantErr        string
		wantTerminal   string
		wantTerminalCt int
		wantContent    string
		allowNoContent bool
	}{
		{
			name:           "anthropic chat error before content",
			kind:           "chat",
			translate:      translateAnthropicStream,
			upstream:       anthropicError,
			wantErr:        "upstream stream error",
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 0,
			allowNoContent: true,
		},
		{
			name:           "anthropic chat error after content",
			kind:           "chat",
			translate:      translateAnthropicStream,
			upstream:       anthropicStart + anthropicDelta + anthropicError,
			wantErr:        "overloaded_error",
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 0,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "anthropic chat premature EOF",
			kind:           "chat",
			translate:      translateAnthropicStream,
			upstream:       anthropicStart + anthropicDelta,
			wantErr:        "truncated",
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 0,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "anthropic chat valid terminal exactly once",
			kind:           "chat",
			translate:      translateAnthropicStream,
			upstream:       anthropicStart + anthropicDelta + anthropicStopDelta + anthropicStop,
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 1,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "anthropic chat EOF-delimited terminal",
			kind:           "chat",
			translate:      translateAnthropicStream,
			upstream:       anthropicStart + anthropicDelta + anthropicStopDelta + anthropicStopNoDelim,
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 1,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "anthropic chat ping ignored then terminal",
			kind:           "chat",
			translate:      translateAnthropicStream,
			upstream:       anthropicStart + anthropicPing + anthropicDelta + anthropicStopDelta + anthropicStop,
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 1,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "anthropic responses error after content",
			kind:           "responses",
			translate:      translateAnthropicResponsesStream,
			upstream:       anthropicStart + anthropicDelta + anthropicError,
			wantErr:        "overloaded_error",
			wantTerminal:   `"type":"response.completed"`,
			wantTerminalCt: 0,
		},
		{
			name:           "anthropic responses premature EOF",
			kind:           "responses",
			translate:      translateAnthropicResponsesStream,
			upstream:       anthropicStart + anthropicDelta,
			wantErr:        "truncated",
			wantTerminal:   `"type":"response.completed"`,
			wantTerminalCt: 0,
		},
		{
			name:           "anthropic responses valid terminal exactly once",
			kind:           "responses",
			translate:      translateAnthropicResponsesStream,
			upstream:       anthropicStart + anthropicDelta + anthropicDelta + anthropicStop,
			wantTerminal:   `"type":"response.completed"`,
			wantTerminalCt: 1,
		},
		{
			name:           "gemini chat error after content",
			kind:           "chat",
			translate:      translateGeminiStream,
			upstream:       geminiContent + geminiError,
			wantErr:        "upstream stream error",
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 0,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "gemini chat error before content",
			kind:           "chat",
			translate:      translateGeminiStream,
			upstream:       geminiError,
			wantErr:        "unavailable",
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 0,
			allowNoContent: true,
		},
		{
			name:           "gemini chat premature EOF without finish",
			kind:           "chat",
			translate:      translateGeminiStream,
			upstream:       geminiContent,
			wantErr:        "truncated",
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 0,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "gemini chat valid terminal",
			kind:           "chat",
			translate:      translateGeminiStream,
			upstream:       geminiContent + geminiFinish,
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 1,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "gemini chat EOF-delimited terminal",
			kind:           "chat",
			translate:      translateGeminiStream,
			upstream:       geminiContent + geminiFinishNoDelim,
			wantTerminal:   "data: [DONE]",
			wantTerminalCt: 1,
			wantContent:    `"content":"Hello"`,
		},
		{
			name:           "gemini responses error after content",
			kind:           "responses",
			translate:      translateGeminiResponsesStream,
			upstream:       geminiContent + geminiError,
			wantErr:        "upstream stream error",
			wantTerminal:   `"type":"response.completed"`,
			wantTerminalCt: 0,
		},
		{
			name:           "gemini responses premature EOF",
			kind:           "responses",
			translate:      translateGeminiResponsesStream,
			upstream:       geminiContent,
			wantErr:        "truncated",
			wantTerminal:   `"type":"response.completed"`,
			wantTerminalCt: 0,
		},
		{
			name:           "gemini responses valid terminal exactly once",
			kind:           "responses",
			translate:      translateGeminiResponsesStream,
			upstream:       geminiContent + geminiFinish,
			wantTerminal:   `"type":"response.completed"`,
			wantTerminalCt: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := &observedReadCloser{Reader: strings.NewReader(tt.upstream), closed: make(chan struct{})}
			stream := tt.translate(upstream, "test/model", NewStreamCompletion())
			body, err := io.ReadAll(stream)
			_ = stream.Close()
			text := string(body)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got body %q err %v", tt.wantErr, text, err)
				}
				if !strings.Contains(err.Error(), "upstream") {
					t.Fatalf("error must be distinguishable as upstream failure, got %v", err)
				}
				if ct := strings.Count(text, tt.wantTerminal); ct != tt.wantTerminalCt {
					t.Fatalf("terminal %q count = %d, want %d (body %q)", tt.wantTerminal, ct, tt.wantTerminalCt, text)
				}
				if tt.wantContent != "" && !strings.Contains(text, tt.wantContent) {
					t.Fatalf("expected partial content %q before error, got %q", tt.wantContent, text)
				}
			} else {
				if err != nil {
					t.Fatalf("expected success, got err %v body %q", err, text)
				}
				if ct := strings.Count(text, tt.wantTerminal); ct != tt.wantTerminalCt {
					t.Fatalf("terminal %q count = %d, want %d (body %q)", tt.wantTerminal, ct, tt.wantTerminalCt, text)
				}
				if tt.wantContent != "" && !strings.Contains(text, tt.wantContent) {
					t.Fatalf("missing content %q in %q", tt.wantContent, text)
				}
			}
			select {
			case <-upstream.closed:
			case <-time.After(time.Second):
				t.Fatal("upstream was not closed")
			}
		})
	}
}

func TestTranslatedStreamCancellationDistinctFromUpstreamError(t *testing.T) {
	upstream := newBlockingReadCloser()
	stream := translateGeminiStream(upstream, "gemini/model", NewStreamCompletion())
	if err := stream.Close(); err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("close stream: %v", err)
	}
	select {
	case <-upstream.closed:
	case <-time.After(time.Second):
		t.Fatal("upstream was not closed on downstream cancel")
	}

	truncatedSrc := &observedReadCloser{Reader: strings.NewReader("data: {\"candidates\":[]}\n\n"), closed: make(chan struct{})}
	truncated := translateGeminiStream(truncatedSrc, "gemini/model", NewStreamCompletion())
	_, err := io.ReadAll(truncated)
	_ = truncated.Close()
	if err == nil || !strings.Contains(err.Error(), "upstream") {
		t.Fatalf("truncated error must carry upstream marker, got %v", err)
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("upstream failure must not look like cancellation: %v", err)
	}
}

func TestOpenCodeTranslatedStreamErrorReuse(t *testing.T) {
	anthropicErr := "event: error\n" + "data: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"busy\"}}\n\n"
	geminiErr := "data: {\"error\":{\"status\":\"UNAVAILABLE\",\"message\":\"busy\"}}\n\n"
	anthropicTruncated := "event: message_start\n" + "data: {\"message\":{\"id\":\"msg_1\"}}\n\n"
	geminiTruncated := "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hi\"}]}}]}\n\n"

	tests := []struct {
		name         string
		providerType config.ProviderType
		protocol     config.ModelProtocol
		op           Operation
		body         string
		upstreamSSE  string
		wantErr      string
	}{
		{
			name:         "zen messages chat error",
			providerType: config.ProviderTypeOpenCodeZen,
			protocol:     config.ModelProtocolMessages,
			op:           OpChatCompletions,
			body:         `{"model":"zen/claude","stream":true,"messages":[{"role":"user","content":"Hi"}]}`,
			upstreamSSE:  anthropicErr,
			wantErr:      "overloaded_error",
		},
		{
			name:         "zen messages responses truncated",
			providerType: config.ProviderTypeOpenCodeZen,
			protocol:     config.ModelProtocolMessages,
			op:           OpResponses,
			body:         `{"model":"zen/claude","stream":true,"input":"hi"}`,
			upstreamSSE:  anthropicTruncated,
			wantErr:      "truncated",
		},
		{
			name:         "zen gemini chat error",
			providerType: config.ProviderTypeOpenCodeZen,
			protocol:     config.ModelProtocolGemini,
			op:           OpChatCompletions,
			body:         `{"model":"zen/gemini","stream":true,"messages":[{"role":"user","content":"Hi"}]}`,
			upstreamSSE:  geminiErr,
			wantErr:      "upstream stream error",
		},
		{
			name:         "zen gemini responses truncated",
			providerType: config.ProviderTypeOpenCodeZen,
			protocol:     config.ModelProtocolGemini,
			op:           OpResponses,
			body:         `{"model":"zen/gemini","stream":true,"input":"hi"}`,
			upstreamSSE:  geminiTruncated,
			wantErr:      "truncated",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(w, tt.upstreamSSE)
			}))
			defer upstream.Close()
			inbound := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
			res, err := New().Do(context.Background(), Request{
				Operation:     tt.op,
				ProviderType:  tt.providerType,
				PublicModel:   "zen/m",
				BaseURL:       upstream.URL,
				APIKey:        "k",
				UpstreamModel: "m",
				ModelProtocol: tt.protocol,
				Body:          []byte(tt.body),
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
			out, readErr := io.ReadAll(res.StreamBody)
			if readErr == nil || !strings.Contains(readErr.Error(), tt.wantErr) {
				t.Fatalf("expected stream error containing %q, got %q err %v", tt.wantErr, string(out), readErr)
			}
			if strings.Contains(string(out), `"type":"response.completed"`) && tt.op == OpResponses {
				t.Fatalf("failed responses stream must not emit response.completed: %q", string(out))
			}
			if strings.Contains(string(out), "data: [DONE]") {
				t.Fatalf("failed stream must not emit successful terminal: %q", string(out))
			}
		})
	}
}
