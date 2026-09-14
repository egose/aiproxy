package httpapi

import (
	"encoding/json"
	"strings"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/provider"
)

const encryptedReasoningScanLimit = 64 << 10

var opaqueChatPartTypes = map[string]bool{
	"thinking":          true,
	"reasoning":         true,
	"reasoning_content": true,
	"reasoning_text":    true,
	"redacted_thinking": true,
	"redacted_content":  true,
	"encrypted_content": true,
}

var opaqueResponsesItemTypes = map[string]bool{
	"reasoning":         true,
	"thinking":          true,
	"redacted_thinking": true,
	"redacted_content":  true,
}

func encryptedReasoningEffective(a config.Alias) *config.EncryptedReasoning {
	if a.EncryptedReasoning == nil {
		return &config.EncryptedReasoning{
			Passthrough:      true,
			OnCallerMismatch: config.EncryptedReasoningFail,
			MatchMessages:    append([]string(nil), config.DefaultEncryptedReasoningMatchMessages...),
		}
	}
	out := &config.EncryptedReasoning{
		Passthrough:      a.EncryptedReasoning.Passthrough,
		OnCallerMismatch: a.EncryptedReasoning.OnCallerMismatch,
		MatchMessages:    append([]string(nil), a.EncryptedReasoning.MatchMessages...),
	}
	if out.OnCallerMismatch == "" {
		out.OnCallerMismatch = config.EncryptedReasoningFail
	}
	if len(out.MatchMessages) == 0 {
		out.MatchMessages = append([]string(nil), config.DefaultEncryptedReasoningMatchMessages...)
	}
	return out
}

func encryptedReasoningApplies(op provider.Operation) bool {
	return op == provider.OpChatCompletions || op == provider.OpResponses
}

func requestHadOpaqueBlocks(body []byte) bool {
	if len(body) == 0 {
		return false
	}
	lowered := strings.ToLower(string(body))
	return strings.Contains(lowered, "encrypted_content") ||
		strings.Contains(lowered, "redacted_thinking") ||
		strings.Contains(lowered, "redacted_content") ||
		strings.Contains(lowered, "\"signature\"")
}

func isCallerMismatch(status int, respBody []byte, patterns []string) bool {
	if status != 400 || len(respBody) == 0 || len(patterns) == 0 {
		return false
	}
	capped := respBody
	if len(capped) > encryptedReasoningScanLimit {
		capped = capped[:encryptedReasoningScanLimit]
	}
	lowered := strings.ToLower(string(capped))
	for _, p := range patterns {
		trimmed := strings.ToLower(strings.TrimSpace(p))
		if trimmed == "" {
			continue
		}
		if strings.Contains(lowered, trimmed) {
			return true
		}
	}
	return false
}

func stripOpaqueBlocks(op provider.Operation, body []byte) ([]byte, bool) {
	if len(body) == 0 {
		return body, false
	}
	var doc map[string]any
	if err := json.Unmarshal(body, &doc); err != nil {
		return body, false
	}
	var changed bool
	switch op {
	case provider.OpChatCompletions:
		changed = stripChatMessages(doc)
	case provider.OpResponses:
		changed = stripResponsesInput(doc)
	default:
		return body, false
	}
	if !changed {
		return body, false
	}
	out, err := json.Marshal(doc)
	if err != nil {
		return body, false
	}
	return out, true
}

func stripChatMessages(doc map[string]any) bool {
	raw, ok := doc["messages"]
	if !ok {
		return false
	}
	msgs, ok := raw.([]any)
	if !ok {
		return false
	}
	changed := false
	for i, m := range msgs {
		msg, ok := m.(map[string]any)
		if !ok {
			continue
		}
		if _, ok := msg["reasoning_content"]; ok {
			delete(msg, "reasoning_content")
			changed = true
		}
		if _, ok := msg["reasoning"]; ok {
			delete(msg, "reasoning")
			changed = true
		}
		content, ok := msg["content"]
		if !ok {
			continue
		}
		switch c := content.(type) {
		case string:
		case []any:
			kept := make([]any, 0, len(c))
			for _, part := range c {
				if shouldDropChatPart(part) {
					changed = true
					continue
				}
				kept = append(kept, part)
			}
			if changed {
				msg["content"] = kept
			}
		case map[string]any:
			if shouldDropChatPart(c) {
				msg["content"] = []any{}
				changed = true
			}
		}
		msgs[i] = msg
	}
	if changed {
		doc["messages"] = msgs
	}
	return changed
}

func shouldDropChatPart(part any) bool {
	m, ok := part.(map[string]any)
	if !ok {
		return false
	}
	if t, ok := m["type"].(string); ok {
		if opaqueChatPartTypes[strings.ToLower(t)] {
			return true
		}
	}
	for _, k := range []string{"encrypted_content", "signature", "redacted_thinking", "thinking_blocks", "reasoning_content"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	if data, ok := m["data"]; ok && len(m) <= 2 {
		if t, hasType := m["type"].(string); hasType && t != "text" {
			_ = data
			return true
		}
	}
	return false
}

func stripResponsesInput(doc map[string]any) bool {
	raw, ok := doc["input"]
	if !ok {
		return false
	}
	items, ok := raw.([]any)
	if !ok {
		return false
	}
	changed := false
	kept := make([]any, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			kept = append(kept, item)
			continue
		}
		if t, ok := m["type"].(string); ok {
			if opaqueResponsesItemTypes[strings.ToLower(t)] {
				changed = true
				continue
			}
		}
		if _, ok := m["encrypted_content"]; ok {
			delete(m, "encrypted_content")
			changed = true
		}
		if _, ok := m["signature"]; ok {
			delete(m, "signature")
			changed = true
		}
		kept = append(kept, m)
	}
	if changed {
		doc["input"] = kept
	}
	return changed
}

func hasOpaqueField(m map[string]any) bool {
	for _, k := range []string{"encrypted_content", "signature", "redacted_thinking"} {
		if _, ok := m[k]; ok {
			return true
		}
	}
	return false
}

var _ = hasOpaqueField
