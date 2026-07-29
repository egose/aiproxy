package provider

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/egose/aiproxy/internal/config"
)

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	MaxTokens   int                `json:"max_tokens"`
	System      string             `json:"system,omitempty"`
	Temperature *float64           `json:"temperature,omitempty"`
	TopP        *float64           `json:"top_p,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string                  `json:"role"`
	Content []anthropicContentBlock `json:"content"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

func (a *adapter) doAnthropic(ctx context.Context, r Request) (*Result, error) {
	switch r.Operation {
	case OpChatCompletions:
		return a.doAnthropicChat(ctx, r)
	case OpResponses:
		return a.doAnthropicResponses(ctx, r)
	default:
		return nil, ErrUnsupportedOperation{ProviderType: config.ProviderTypeAnthropic, Operation: r.Operation}
	}
}

func (a *adapter) doAnthropicChat(ctx context.Context, r Request) (*Result, error) {
	if r.BaseURL == "" {
		r.BaseURL = defaultAnthropicBaseURL
	}
	body, err := requestBody(r)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	translated, streaming, err := translateOpenAIToAnthropic(body, r.UpstreamModel)
	if err != nil {
		return nil, ErrInvalidRequest{Message: fmt.Sprintf("translate request: %v", err)}
	}

	target := strings.TrimRight(r.BaseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", r.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Content-Type", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := clientFor(r).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, err := readUpstreamBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body: %w", err)
		}
		return &Result{
			StatusCode: resp.StatusCode,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       translateAnthropicError(errBody),
		}, nil
	}
	if streaming {
		return &Result{
			StatusCode: resp.StatusCode,
			Header:     http.Header{"Content-Type": []string{"text/event-stream"}},
			StreamBody: translateAnthropicStream(resp.Body, r.PublicModel),
			Streaming:  true,
		}, nil
	}
	defer resp.Body.Close()

	outBody, err := readUpstreamBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	translatedBody, err := translateAnthropicResponse(outBody, r.PublicModel)
	if err != nil {
		return nil, fmt.Errorf("translate response: %w", err)
	}
	return &Result{
		StatusCode: resp.StatusCode,
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       translatedBody,
	}, nil
}

func (a *adapter) doAnthropicResponses(ctx context.Context, r Request) (*Result, error) {
	if r.BaseURL == "" {
		r.BaseURL = defaultAnthropicBaseURL
	}
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

	target := strings.TrimRight(r.BaseURL, "/") + "/v1/messages"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(translated))
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-api-key", r.APIKey)
	req.Header.Set("anthropic-version", anthropicVersion)
	req.Header.Set("Content-Type", "application/json")
	if streaming {
		req.Header.Set("Accept", "text/event-stream")
	}

	resp, err := clientFor(r).Do(req)
	if err != nil {
		return nil, fmt.Errorf("upstream call: %w", err)
	}
	if resp.StatusCode >= 400 {
		defer resp.Body.Close()
		errBody, err := readUpstreamBody(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("read error body: %w", err)
		}
		return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translateAnthropicError(errBody)}, nil
	}
	if streaming {
		return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"text/event-stream"}}, StreamBody: translateAnthropicResponsesStream(resp.Body, r.PublicModel), Streaming: true}, nil
	}
	defer resp.Body.Close()
	outBody, err := readUpstreamBody(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read upstream body: %w", err)
	}
	translatedBody, err := translateAnthropicResponsesResponse(outBody, r.PublicModel)
	if err != nil {
		return nil, fmt.Errorf("translate response: %w", err)
	}
	return &Result{StatusCode: resp.StatusCode, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: translatedBody}, nil
}

func translateOpenAIToAnthropic(body []byte, upstreamModel string) ([]byte, bool, error) {
	var req openAIChatRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return nil, false, err
	}
	out := anthropicRequest{
		Model:       upstreamModel,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}
	if out.MaxTokens <= 0 {
		out.MaxTokens = defaultMaxTokens
	}

	var systemParts []string
	for _, msg := range req.Messages {
		blocks, joined, err := parseOpenAIContent(msg.Content)
		if err != nil {
			return nil, false, fmt.Errorf("message role %q: %w", msg.Role, err)
		}
		switch msg.Role {
		case "system":
			if joined != "" {
				systemParts = append(systemParts, joined)
			}
		case "user", "assistant":
			out.Messages = append(out.Messages, anthropicMessage{Role: msg.Role, Content: blocks})
		default:
			return nil, false, fmt.Errorf("unsupported message role %q", msg.Role)
		}
	}
	out.System = strings.Join(systemParts, "\n\n")
	translated, err := json.Marshal(out)
	if err != nil {
		return nil, false, err
	}
	return translated, out.Stream, nil
}

func parseOpenAIContent(raw json.RawMessage) ([]anthropicContentBlock, string, error) {
	if len(raw) == 0 {
		return nil, "", nil
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []anthropicContentBlock{{Type: "text", Text: text}}, text, nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, "", fmt.Errorf("unsupported content shape")
	}
	blocks := make([]anthropicContentBlock, 0, len(parts))
	joined := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type != "text" {
			return nil, "", fmt.Errorf("unsupported content part type %q", part.Type)
		}
		blocks = append(blocks, anthropicContentBlock{Type: "text", Text: part.Text})
		joined = append(joined, part.Text)
	}
	return blocks, strings.Join(joined, ""), nil
}

func translateAnthropicResponse(body []byte, publicModel string) ([]byte, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	out := openAIResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: 0,
		Model:   publicModel,
		Choices: []openAIChoice{{
			Index: 0,
			Message: openAIMessageOut{
				Role:    "assistant",
				Content: joinAnthropicText(resp.Content),
			},
			FinishReason: mapAnthropicStopReason(resp.StopReason),
		}},
		Usage: openAIUsage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
	}
	return json.Marshal(out)
}

func translateAnthropicResponsesResponse(body []byte, publicModel string) ([]byte, error) {
	var resp anthropicResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, err
	}
	usage := openAIUsage{
		PromptTokens:     resp.Usage.InputTokens,
		CompletionTokens: resp.Usage.OutputTokens,
		TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
	}
	return buildResponsesOutput(resp.ID, publicModel, joinAnthropicText(resp.Content), usage)
}

func joinAnthropicText(blocks []anthropicContentBlock) string {
	var b strings.Builder
	for _, block := range blocks {
		if block.Type == "text" {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

func translateAnthropicError(body []byte) []byte {
	var resp struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &resp); err != nil || resp.Error.Message == "" {
		return []byte(`{"error":{"type":"upstream_error","message":"anthropic upstream error"}}`)
	}
	out := struct {
		Error struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}{}
	out.Error.Type = resp.Error.Type
	out.Error.Message = resp.Error.Message
	encoded, err := json.Marshal(out)
	if err != nil {
		return []byte(`{"error":{"type":"upstream_error","message":"anthropic upstream error"}}`)
	}
	return encoded
}

func translateAnthropicStream(src io.ReadCloser, publicModel string) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer src.Close()
		defer pw.Close()

		reader := bufio.NewReader(src)
		var eventType string
		var dataLines []string
		var streamID string
		var finishReason string

		for {
			line, err := reader.ReadString('\n')
			if err != nil && len(line) == 0 {
				if err != io.EOF {
					_ = pw.CloseWithError(err)
				}
				return
			}
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed == "" {
				if len(dataLines) > 0 || eventType != "" {
					if err := processAnthropicEvent(pw, eventType, strings.Join(dataLines, "\n"), publicModel, &streamID, &finishReason); err != nil {
						_ = pw.CloseWithError(err)
						return
					}
				}
				eventType = ""
				dataLines = nil
			} else if strings.HasPrefix(trimmed, "event:") {
				eventType = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			} else if strings.HasPrefix(trimmed, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			}
			if err == io.EOF {
				if len(dataLines) > 0 || eventType != "" {
					if err := processAnthropicEvent(pw, eventType, strings.Join(dataLines, "\n"), publicModel, &streamID, &finishReason); err != nil {
						_ = pw.CloseWithError(err)
					}
				}
				return
			}
		}
	}()
	return pr
}

func translateAnthropicResponsesStream(src io.ReadCloser, publicModel string) io.ReadCloser {
	pr, pw := io.Pipe()
	go func() {
		defer src.Close()
		defer pw.Close()

		reader := bufio.NewReader(src)
		var eventType string
		var dataLines []string
		state := newResponsesStreamState(publicModel, "resp_anthropic")

		for {
			line, err := reader.ReadString('\n')
			if err != nil && len(line) == 0 {
				if err != io.EOF {
					_ = pw.CloseWithError(err)
					return
				}
				if !state.Completed {
					if err := writeResponsesCompleted(pw, state); err != nil {
						_ = pw.CloseWithError(err)
					}
				}
				return
			}
			trimmed := strings.TrimRight(line, "\r\n")
			if trimmed == "" {
				if len(dataLines) > 0 || eventType != "" {
					done, err := processAnthropicResponsesEvent(pw, eventType, strings.Join(dataLines, "\n"), state)
					if err != nil {
						_ = pw.CloseWithError(err)
						return
					}
					if done {
						return
					}
				}
				eventType = ""
				dataLines = nil
			} else if strings.HasPrefix(trimmed, "event:") {
				eventType = strings.TrimSpace(strings.TrimPrefix(trimmed, "event:"))
			} else if strings.HasPrefix(trimmed, "data:") {
				dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(trimmed, "data:")))
			}
			if err == io.EOF {
				if len(dataLines) > 0 || eventType != "" {
					done, processErr := processAnthropicResponsesEvent(pw, eventType, strings.Join(dataLines, "\n"), state)
					if processErr != nil {
						_ = pw.CloseWithError(processErr)
						return
					}
					if done {
						return
					}
				}
				if !state.Completed {
					if err := writeResponsesCompleted(pw, state); err != nil {
						_ = pw.CloseWithError(err)
					}
				}
				return
			}
		}
	}()
	return pr
}

func processAnthropicEvent(w io.Writer, eventType, data, publicModel string, streamID *string, finishReason *string) error {
	switch eventType {
	case "message_start":
		var evt struct {
			Message struct {
				ID string `json:"id"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return err
		}
		if evt.Message.ID != "" {
			*streamID = evt.Message.ID
		}
		return writeOpenAIChunk(w, openAIChunk{
			ID:      fallbackStreamID(*streamID),
			Object:  "chat.completion.chunk",
			Created: 0,
			Model:   publicModel,
			Choices: []openAIChunkChoice{{
				Index: 0,
				Delta: openAIChunkDelta{Role: "assistant"},
			}},
		})
	case "content_block_delta":
		var evt struct {
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return err
		}
		if evt.Delta.Type != "text_delta" || evt.Delta.Text == "" {
			return nil
		}
		return writeOpenAIChunk(w, openAIChunk{
			ID:      fallbackStreamID(*streamID),
			Object:  "chat.completion.chunk",
			Created: 0,
			Model:   publicModel,
			Choices: []openAIChunkChoice{{
				Index: 0,
				Delta: openAIChunkDelta{Content: evt.Delta.Text},
			}},
		})
	case "message_delta":
		var evt struct {
			Delta struct {
				StopReason string `json:"stop_reason"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return err
		}
		if evt.Delta.StopReason != "" {
			*finishReason = mapAnthropicStopReason(evt.Delta.StopReason)
		}
		return nil
	case "message_stop":
		if err := writeOpenAIChunk(w, openAIChunk{
			ID:      fallbackStreamID(*streamID),
			Object:  "chat.completion.chunk",
			Created: 0,
			Model:   publicModel,
			Choices: []openAIChunkChoice{{
				Index:        0,
				Delta:        openAIChunkDelta{},
				FinishReason: stringPtr(*finishReason),
			}},
		}); err != nil {
			return err
		}
		_, err := io.WriteString(w, "data: [DONE]\n\n")
		return err
	default:
		return nil
	}
}

func processAnthropicResponsesEvent(w io.Writer, eventType, data string, state *responsesStreamState) (bool, error) {
	switch eventType {
	case "message_start":
		var evt struct {
			Message struct {
				ID    string         `json:"id"`
				Usage anthropicUsage `json:"usage"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return false, err
		}
		if evt.Message.ID != "" {
			state.ResponseID = evt.Message.ID
			state.ItemID = evt.Message.ID + "_msg"
		}
		state.setUsage(openAIUsage{
			PromptTokens:     evt.Message.Usage.InputTokens,
			CompletionTokens: evt.Message.Usage.OutputTokens,
			TotalTokens:      evt.Message.Usage.InputTokens + evt.Message.Usage.OutputTokens,
		})
		return false, writeResponsesCreated(w, state)
	case "content_block_delta":
		var evt struct {
			Delta struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"delta"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return false, err
		}
		if evt.Delta.Type != "text_delta" || evt.Delta.Text == "" {
			return false, nil
		}
		return false, writeResponsesDelta(w, state, evt.Delta.Text)
	case "message_delta":
		var evt struct {
			Usage anthropicUsage `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &evt); err != nil {
			return false, err
		}
		state.setUsage(openAIUsage{
			PromptTokens:     evt.Usage.InputTokens,
			CompletionTokens: evt.Usage.OutputTokens,
			TotalTokens:      evt.Usage.InputTokens + evt.Usage.OutputTokens,
		})
		return false, nil
	case "message_stop":
		return true, writeResponsesCompleted(w, state)
	default:
		return false, nil
	}
}

func fallbackStreamID(id string) string {
	if id != "" {
		return id
	}
	return "chatcmpl-anthropic"
}

func mapAnthropicStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "tool_use":
		return "tool_calls"
	default:
		return reason
	}
}
