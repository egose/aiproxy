package provider

import (
	"encoding/json"
)

type openAIUsageShape struct {
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type anthropicUsageShape struct {
	Usage struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type geminiUsageShape struct {
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
}

type openAIResponsesUsageShape struct {
	Usage struct {
		InputTokens      int `json:"input_tokens"`
		OutputTokens     int `json:"output_tokens"`
		TotalTokens      int `json:"total_tokens"`
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
}

func usageFromBody(body []byte) Usage {
	if len(body) == 0 || body[0] != '{' {
		return Usage{}
	}
	if u := extractGeminiUsage(body); u.Has() {
		return u
	}
	raw := rawTopLevelUsage(body)
	if raw == nil {
		return Usage{}
	}
	if hasUsageKey(raw, "input_tokens") || hasUsageKey(raw, "output_tokens") {
		if u := extractOpenAIResponsesUsage(body); u.Has() {
			return u
		}
		return Usage{}
	}
	if hasUsageKey(raw, "prompt_tokens") || hasUsageKey(raw, "completion_tokens") {
		if u := extractOpenAIUsage(body); u.Has() {
			return u
		}
		return Usage{}
	}
	if u := extractOpenAIResponsesUsage(body); u.Has() {
		return u
	}
	return Usage{}
}

func rawTopLevelUsage(body []byte) json.RawMessage {
	var envelope struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil
	}
	if len(envelope.Usage) == 0 || string(envelope.Usage) == "null" {
		return nil
	}
	return envelope.Usage
}

func hasUsageKey(raw json.RawMessage, key string) bool {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(raw, &keys); err != nil {
		return false
	}
	_, ok := keys[key]
	return ok
}

func usageFromSSEData(data []byte) Usage {
	if u := usageFromBody(data); u.Has() {
		return u
	}
	var envelope struct {
		Response struct {
			Usage json.RawMessage `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return Usage{}
	}
	if len(envelope.Response.Usage) == 0 || string(envelope.Response.Usage) == "null" {
		return Usage{}
	}
	wrapped := append([]byte(`{"usage":`), envelope.Response.Usage...)
	wrapped = append(wrapped, '}')
	return usageFromBody(wrapped)
}

func extractOpenAIUsage(body []byte) Usage {
	var shape openAIUsageShape
	if err := json.Unmarshal(body, &shape); err != nil {
		return Usage{}
	}
	prompt := shape.Usage.PromptTokens
	completion := shape.Usage.CompletionTokens
	total := shape.Usage.TotalTokens
	if total == 0 {
		total = prompt + completion
	}
	return Usage{PromptTokens: int64(prompt), CompletionTokens: int64(completion), TotalTokens: int64(total)}
}

func extractAnthropicUsage(body []byte) Usage {
	var shape anthropicUsageShape
	if err := json.Unmarshal(body, &shape); err != nil {
		return Usage{}
	}
	prompt := shape.Usage.InputTokens
	completion := shape.Usage.OutputTokens
	return Usage{PromptTokens: int64(prompt), CompletionTokens: int64(completion), TotalTokens: int64(prompt + completion)}
}

func extractGeminiUsage(body []byte) Usage {
	var shape geminiUsageShape
	if err := json.Unmarshal(body, &shape); err != nil {
		return Usage{}
	}
	prompt := shape.UsageMetadata.PromptTokenCount
	completion := shape.UsageMetadata.CandidatesTokenCount
	total := shape.UsageMetadata.TotalTokenCount
	if total == 0 {
		total = prompt + completion
	}
	return Usage{PromptTokens: int64(prompt), CompletionTokens: int64(completion), TotalTokens: int64(total)}
}

func extractOpenAIResponsesUsage(body []byte) Usage {
	var shape openAIResponsesUsageShape
	if err := json.Unmarshal(body, &shape); err != nil {
		return Usage{}
	}
	prompt := shape.Usage.PromptTokens
	if prompt == 0 {
		prompt = shape.Usage.InputTokens
	}
	completion := shape.Usage.CompletionTokens
	if completion == 0 {
		completion = shape.Usage.OutputTokens
	}
	total := shape.Usage.TotalTokens
	if total == 0 {
		total = prompt + completion
	}
	return Usage{PromptTokens: int64(prompt), CompletionTokens: int64(completion), TotalTokens: int64(total)}
}
