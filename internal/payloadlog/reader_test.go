package payloadlog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func seedPayloadFile(t *testing.T, dir, name string, entries []Entry) {
	t.Helper()
	var data []byte
	for _, e := range entries {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		data = append(data, line...)
		data = append(data, '\n')
	}
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestListRecentNewestFirst(t *testing.T) {
	dir := t.TempDir()
	seedPayloadFile(t, dir, "payload-20260912.jsonl", []Entry{
		{RequestID: "old-1", Method: "POST", Path: "/v1/chat/completions", Status: 200},
		{RequestID: "old-2", Method: "POST", Path: "/v1/chat/completions", Status: 500},
	})
	seedPayloadFile(t, dir, "payload-20260913.jsonl", []Entry{
		{RequestID: "new-1", Method: "POST", Path: "/v1/embeddings", Status: 200},
		{RequestID: "new-2", Method: "POST", Path: "/v1/chat/completions", Status: 429},
	})
	got, err := ListRecent(dir, 10, false)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("entries = %d, want 4", len(got))
	}
	want := []string{"new-2", "new-1", "old-2", "old-1"}
	for i, id := range want {
		if got[i].RequestID != id {
			t.Fatalf("entry %d = %q, want %q", i, got[i].RequestID, id)
		}
	}
}

func TestListRecentErrorsOnly(t *testing.T) {
	dir := t.TempDir()
	seedPayloadFile(t, dir, "payload-20260913.jsonl", []Entry{
		{RequestID: "ok", Status: 200},
		{RequestID: "bad", Status: 500},
		{RequestID: "throttled", Status: 429},
	})
	got, err := ListRecent(dir, 10, true)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 2 || got[0].RequestID != "throttled" || got[1].RequestID != "bad" {
		t.Fatalf("filtered = %+v, want throttled,bad", got)
	}
}

func TestListRecentMissingDir(t *testing.T) {
	got, err := ListRecent(filepath.Join(t.TempDir(), "absent"), 10, false)
	if err != nil {
		t.Fatalf("ListRecent: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("entries = %d, want 0", len(got))
	}
}

func TestGetRoundTrip(t *testing.T) {
	dir := t.TempDir()
	seedPayloadFile(t, dir, "payload-20260913.jsonl", []Entry{
		{RequestID: "req-1", Method: "POST", Status: 200, Request: EntrySide{Body: Body{Data: "{}"}}},
		{RequestID: "req-2", Method: "POST", Status: 500, Error: "boom"},
	})
	raw, err := Get(dir, "req-2")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	var e Entry
	if err := json.Unmarshal(raw, &e); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if e.RequestID != "req-2" || e.Error != "boom" {
		t.Fatalf("entry = %+v", e)
	}
	if _, err := Get(dir, "absent"); err != ErrPayloadNotFound {
		t.Fatalf("Get absent = %v, want ErrPayloadNotFound", err)
	}
	if _, err := Get(dir, "../evil"); err == nil {
		t.Fatal("expected error for invalid request id")
	}
}

func TestPrettyIndents(t *testing.T) {
	out := Pretty(json.RawMessage(`{"a":1,"b":{"c":[1,2]}}`), 0)
	want := "{\n  \"a\": 1,\n  \"b\": {\n    \"c\": [\n      1,\n      2\n    ]\n  }\n}"
	if out != want {
		t.Fatalf("pretty = %q, want %q", out, want)
	}
}

func TestScanNewestFirstNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload-20260913.jsonl")
	if err := os.WriteFile(path, []byte("{\"request_id\":\"a\"}\n{\"request_id\":\"b\"}"), 0o600); err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := scanNewestFirst(path, func(line []byte) bool {
		var s Summary
		if err := json.Unmarshal(line, &s); err != nil {
			t.Fatalf("unmarshal %q: %v", line, err)
		}
		got = append(got, s.RequestID)
		return true
	}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "b" || got[1] != "a" {
		t.Fatalf("order = %v, want [b a]", got)
	}
}

var _ = config.PayloadLogFilePrefix
