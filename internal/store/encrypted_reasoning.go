package store

import (
	"bytes"
	"encoding/json"
)

func nullableEncryptedReasoning(raw json.RawMessage) json.RawMessage {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	return raw
}
