package observability

import (
	"encoding/json"
	"io"
	"testing"
)

func TestPinnedLogIdentitySurvivesJSONWithoutParsingAttrs(t *testing.T) {
	b := NewLogBuffer(2)
	logger := NewLogger(io.Discard, LoggerOptions{Buffer: b}).With("request_id", "id with spaces=and-other-text")
	logger.Info("fixture", "request_id", "spoofed")
	e := b.Since(1)[0]
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var decoded LogEntry
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.RequestID != "id with spaces=and-other-text" {
		t.Fatalf("ID parsed from unescaped attrs: %+v", decoded)
	}
	NewLogger(io.Discard, LoggerOptions{Buffer: b}).Info("request_id=spoofed", "request_id", "also-spoofed")
	if b.Since(1)[0].RequestID != "" {
		t.Fatal("untrusted/non-pinned ID promoted")
	}
}
