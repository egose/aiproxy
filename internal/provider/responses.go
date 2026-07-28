package provider

import (
	"encoding/json"
	"fmt"
)

func translateOpenAIResponsesInput(body []byte) (openAIChatRequest, error) {
	var req openAIResponsesRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return openAIChatRequest{}, err
	}
	if req.Stream {
		return openAIChatRequest{}, fmt.Errorf("streaming responses are not supported for translated providers")
	}
	chatReq := openAIChatRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxOutputTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      false,
	}
	if req.Instructions != "" {
		chatReq.Messages = append(chatReq.Messages, openAIMessage{Role: "system", Content: mustMarshalJSON(req.Instructions)})
	}
	msgs, err := parseOpenAIResponsesMessages(req.Input)
	if err != nil {
		return openAIChatRequest{}, err
	}
	chatReq.Messages = append(chatReq.Messages, msgs...)
	return chatReq, nil
}

func parseOpenAIResponsesMessages(raw json.RawMessage) ([]openAIMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("input is required")
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return []openAIMessage{{Role: "user", Content: mustMarshalJSON(text)}}, nil
	}
	var items []struct {
		Type    string          `json:"type,omitempty"`
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("input must be a string or array of message objects")
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("input array must not be empty")
	}
	out := make([]openAIMessage, 0, len(items))
	for _, item := range items {
		if item.Type != "" && item.Type != "message" {
			return nil, fmt.Errorf("unsupported input item type %q", item.Type)
		}
		normalized, err := normalizeResponsesContent(item.Content)
		if err != nil {
			return nil, err
		}
		out = append(out, openAIMessage{Role: item.Role, Content: normalized})
	}
	return out, nil
}

func normalizeResponsesContent(raw json.RawMessage) (json.RawMessage, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return mustMarshalJSON(text), nil
	}
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(raw, &parts); err != nil {
		return nil, fmt.Errorf("unsupported content shape")
	}
	normalized := make([]map[string]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "text", "input_text", "output_text":
			normalized = append(normalized, map[string]string{"type": "text", "text": part.Text})
		default:
			return nil, fmt.Errorf("unsupported content part type %q", part.Type)
		}
	}
	return mustMarshalJSON(normalized), nil
}

func buildResponsesOutput(id, publicModel, text string, usage openAIUsage) ([]byte, error) {
	out := openAIResponsesResponse{
		ID:     id,
		Object: "response",
		Model:  publicModel,
		Output: []openAIResponsesOutputItem{{
			ID:   id + "_msg",
			Type: "message",
			Role: "assistant",
			Content: []openAIResponsesContentItem{{
				Type:        "output_text",
				Text:        text,
				Annotations: []any{},
			}},
		}},
		Usage:  usage,
		Status: "completed",
	}
	return json.Marshal(out)
}

func mustMarshalJSON(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
