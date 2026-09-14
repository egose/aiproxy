package mongolog

import (
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/payloadlog"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestNewDisabledWhenURIEmpty(t *testing.T) {
	l, err := New(Options{})
	if err != nil {
		t.Fatal(err)
	}
	if l != nil {
		t.Fatal("expected nil logger when URI is empty")
	}
}

func TestDocumentForPreservesFields(t *testing.T) {
	entry := payloadlog.Entry{
		Timestamp:     "2026-01-01T00:00:00Z",
		RequestID:     "req-123",
		Method:        "POST",
		Path:          "/v1/chat/completions",
		Operation:     "chat",
		PublicModel:   "openai/gpt-4o-mini",
		Provider:      "openai",
		UpstreamModel: "gpt-4o-mini",
		Status:        200,
		DurationMs:    3,
		Request:       payloadlog.EntrySide{Body: payloadlog.EncodeBodyWithContentType([]byte(`{"model":"openai/gpt-4o-mini"}`), 1024, "application/json")},
		Response:      payloadlog.EntrySide{Body: payloadlog.EncodeBodyWithContentType([]byte(`{"id":"chatcmpl-1"}`), 1024, "application/json")},
	}
	doc, err := DocumentFor(entry)
	if err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{
		"request_id":     "req-123",
		"public_model":   "openai/gpt-4o-mini",
		"provider":       "openai",
		"upstream_model": "gpt-4o-mini",
	} {
		if doc[key] != want {
			t.Fatalf("%s not preserved: %v", key, doc[key])
		}
	}
	body := bsonValue(doc["request"], "body")
	data := bsonValue(body, "data")
	model, ok := bsonValue(data, "model").(string)
	if !ok || model != "openai/gpt-4o-mini" {
		t.Fatalf("request.body.data.model not preserved: %v (%T)", bsonValue(data, "model"), bsonValue(data, "model"))
	}
}

func bsonValue(doc any, key string) any {
	switch d := doc.(type) {
	case bson.M:
		return d[key]
	case bson.D:
		for _, e := range d {
			if e.Key == key {
				return e.Value
			}
		}
	}
	return nil
}

func TestNewFailsOnUnreachableURI(t *testing.T) {
	_, err := New(Options{
		URI:     "mongodb://127.0.0.1:1/?serverSelectionTimeoutMS=500&connectTimeoutMS=500",
		Timeout: 2 * time.Second,
	})
	if err == nil {
		t.Fatal("expected error pinging unreachable MongoDB")
	}
}

func TestNilLoggerIsNoop(t *testing.T) {
	var l *Logger
	if l.MaxBodyBytes() != 0 {
		t.Fatal("nil logger MaxBodyBytes should be 0")
	}
	if err := l.Record(payloadlog.Entry{}); err != nil {
		t.Fatalf("nil logger Record should be a no-op: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("nil logger Close should be a no-op: %v", err)
	}
}
