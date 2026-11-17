package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func DecodeEncryptedReasoningJSON(raw []byte, owner string) (*EncryptedReasoning, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil, nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return nil, fmt.Errorf("%s: encrypted_reasoning must be an object or null", owner)
	}
	declaration := &EncryptedReasoningDeclaration{}
	seen := make(map[string]bool, 3)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return nil, fmt.Errorf("%s: encrypted_reasoning has invalid JSON: %w", owner, err)
		}
		field, ok := token.(string)
		if !ok {
			return nil, fmt.Errorf("%s: encrypted_reasoning has invalid JSON fields", owner)
		}
		key := strings.ToLower(field)
		if seen[key] {
			return nil, fmt.Errorf("%s: encrypted_reasoning has duplicate field %q", owner, field)
		}
		seen[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, fmt.Errorf("%s: encrypted_reasoning.%s has invalid JSON: %w", owner, field, err)
		}
		if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return nil, fmt.Errorf("%s: encrypted_reasoning.%s must not be null", owner, field)
		}
		switch field {
		case "passthrough":
			var passthrough bool
			if err := json.Unmarshal(value, &passthrough); err != nil {
				return nil, fmt.Errorf("%s: encrypted_reasoning.passthrough must be a boolean", owner)
			}
			declaration.Passthrough = &passthrough
		case "on_caller_mismatch":
			var mismatch string
			if err := json.Unmarshal(value, &mismatch); err != nil {
				return nil, fmt.Errorf("%s: encrypted_reasoning.on_caller_mismatch must be a string", owner)
			}
			declaration.OnCallerMismatch = &mismatch
		case "match_messages":
			var entries []json.RawMessage
			if err := json.Unmarshal(value, &entries); err != nil {
				return nil, fmt.Errorf("%s: encrypted_reasoning.match_messages must be a list of strings", owner)
			}
			messages := make([]string, len(entries))
			for i, entry := range entries {
				if bytes.Equal(bytes.TrimSpace(entry), []byte("null")) || json.Unmarshal(entry, &messages[i]) != nil {
					return nil, fmt.Errorf("%s: encrypted_reasoning.match_messages must contain non-null strings", owner)
				}
			}
			declaration.MatchMessages = &messages
		default:
			return nil, fmt.Errorf("%s: encrypted_reasoning has unsupported field %q", owner, field)
		}
	}
	if _, err := decoder.Token(); err != nil {
		return nil, fmt.Errorf("%s: encrypted_reasoning has invalid JSON: %w", owner, err)
	}
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF {
		return nil, fmt.Errorf("%s: encrypted_reasoning must contain exactly one JSON value", owner)
	}
	policy := NewEncryptedReasoning(declaration)
	if err := ValidateEncryptedReasoning(policy, owner); err != nil {
		return nil, err
	}
	return policy, nil
}

func EncodeEncryptedReasoningJSON(policy *EncryptedReasoning, owner string) (json.RawMessage, error) {
	if err := ValidateEncryptedReasoning(policy, owner); err != nil {
		return nil, err
	}
	declaration := DeclaredEncryptedReasoning(policy)
	if declaration == nil {
		return nil, nil
	}
	if declaration.MatchMessages != nil && *declaration.MatchMessages == nil {
		messages := []string{}
		declaration.MatchMessages = &messages
	}
	raw, err := json.Marshal(declaration)
	if err != nil {
		return nil, fmt.Errorf("%s: encrypted_reasoning cannot be encoded: %w", owner, err)
	}
	return raw, nil
}
