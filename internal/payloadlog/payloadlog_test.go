package payloadlog

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

func testLogger(t *testing.T, cfg config.PayloadLog) (*Logger, string) {
	t.Helper()
	dir := t.TempDir()
	cfg.Enabled = true
	cfg.Dir = dir
	if cfg.Rotation == "" {
		cfg.Rotation = config.PayloadLogRotationDaily
	}
	l, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	return l, dir
}

func readEntries(t *testing.T, dir string) []Entry {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, "payload-*.jsonl"))
	if err != nil {
		t.Fatalf("glob: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no payload files in %s", dir)
	}
	var out []Entry
	for _, f := range files {
		fh, err := os.Open(f)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		sc := bufio.NewScanner(fh)
		sc.Buffer(make([]byte, 1024*1024), 1024*1024)
		for sc.Scan() {
			var e Entry
			if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
				fh.Close()
				t.Fatalf("unmarshal %q: %v", sc.Text(), err)
			}
			out = append(out, e)
		}
		fh.Close()
		if err := sc.Err(); err != nil {
			t.Fatalf("scan: %v", err)
		}
	}
	return out
}

func TestDisabledReturnsNil(t *testing.T) {
	l, err := New(config.PayloadLog{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if l != nil {
		_ = l.Close()
		t.Fatalf("expected nil logger when disabled")
	}
}

func TestRecordRedactsAndTruncates(t *testing.T) {
	l, dir := testLogger(t, config.PayloadLog{Retention: time.Hour, MaxBodyBytes: 4})
	inHeaders := http.Header{
		"Authorization": []string{"Bearer secret"},
		"Content-Type":  []string{"application/json"},
	}
	if err := l.Record(Entry{
		Method: "POST",
		Path:   "/v1/chat/completions",
		Status: 200,
		Request: EntrySide{
			Headers: RedactHeaders(inHeaders),
			Body:    EncodeBody([]byte(`{"model":"m"}`), l.MaxBodyBytes()),
		},
		Response: EntrySide{
			Headers: RedactHeaders(http.Header{"Content-Type": []string{"application/json"}}),
			Body:    EncodeBody([]byte(`{"ok":true}`), l.MaxBodyBytes()),
		},
	}); err != nil {
		t.Fatalf("Record: %v", err)
	}
	entries := readEntries(t, dir)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if got := e.Request.Headers["Authorization"]; len(got) != 1 || got[0] != "[REDACTED]" {
		t.Fatalf("authorization header = %v, want redacted", got)
	}
	if e.Request.Body.Data != `{"mo` || !e.Request.Body.Truncated || e.Request.Body.Bytes != 13 {
		t.Fatalf("request body = %+v", e.Request.Body)
	}
	if e.Timestamp == "" {
		t.Fatalf("missing timestamp")
	}
}

func TestEncodeBinaryBody(t *testing.T) {
	b := EncodeBody([]byte{0xff, 0xfe, 0x00}, 0)
	if b.Encoding != "base64" || b.Data == "" {
		t.Fatalf("body = %+v, want base64", b)
	}
}

func TestRotationBuckets(t *testing.T) {
	ts := time.Date(2026, 9, 13, 10, 30, 0, 0, time.UTC)
	if got := BucketFor(ts, config.PayloadLogRotationDaily); got != "20260913" {
		t.Fatalf("daily bucket = %q", got)
	}
	if got := BucketFor(ts, config.PayloadLogRotationHourly); got != "20260913-10" {
		t.Fatalf("hourly bucket = %q", got)
	}
}

func TestHourlyRotationCreatesSeparateFiles(t *testing.T) {
	l, dir := testLogger(t, config.PayloadLog{Rotation: config.PayloadLogRotationHourly})
	l.mu.Lock()
	if err := l.rotateLocked("20260913-10"); err != nil {
		l.mu.Unlock()
		t.Fatalf("rotate: %v", err)
	}
	if err := l.rotateLocked("20260913-11"); err != nil {
		l.mu.Unlock()
		t.Fatalf("rotate: %v", err)
	}
	l.mu.Unlock()
	for _, name := range []string{"payload-20260913-10.jsonl", "payload-20260913-11.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
	}
}

func TestRetentionSweep(t *testing.T) {
	l, dir := testLogger(t, config.PayloadLog{Retention: time.Hour})
	old := filepath.Join(dir, "payload-20200101.jsonl")
	fresh := filepath.Join(dir, "payload-29990101.jsonl")
	other := filepath.Join(dir, "notes.txt")
	for _, p := range []string{old, fresh, other} {
		if err := os.WriteFile(p, []byte("x\n"), 0o600); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	l.mu.Lock()
	l.sweepLocked(time.Now())
	l.mu.Unlock()
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old file should be removed")
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh file should be kept: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("non-payload file should be kept: %v", err)
	}
}

func TestRetentionZeroKeepsForever(t *testing.T) {
	l, dir := testLogger(t, config.PayloadLog{Retention: 0})
	old := filepath.Join(dir, "payload-20200101.jsonl")
	if err := os.WriteFile(old, []byte("x\n"), 0o600); err != nil {
		t.Fatalf("seed: %v", err)
	}
	past := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
	l.mu.Lock()
	l.sweepLocked(time.Now())
	l.mu.Unlock()
	if _, err := os.Stat(old); err != nil {
		t.Fatalf("file should be kept when retention is 0: %v", err)
	}
}

func TestCaptureTruncates(t *testing.T) {
	c := NewCapture(io.NopCloser(strings.NewReader("hello world")), 5)
	buf := make([]byte, 64)
	for {
		_, err := c.Read(buf)
		if err != nil {
			break
		}
	}
	body := c.Body()
	if body.Bytes != 11 || !body.Truncated || body.Data != "hello" {
		t.Fatalf("body = %+v", body)
	}
}
