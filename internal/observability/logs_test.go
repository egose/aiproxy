package observability

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
)

func TestLogBufferRing(t *testing.T) {
	b := NewLogBuffer(3)
	for i := 0; i < 5; i++ {
		b.Add(LogEntry{Message: string(rune('a' + i))})
	}
	got := b.Since(10)
	if len(got) != 3 {
		t.Fatalf("Since(10) len = %d, want 3 (capacity)", len(got))
	}
	if got[0].Message != "c" || got[1].Message != "d" || got[2].Message != "e" {
		t.Fatalf("entries = %v, want c,d,e", got)
	}
}

func TestLogBufferSinceZero(t *testing.T) {
	b := NewLogBuffer(4)
	b.Add(LogEntry{Message: "x"})
	if got := b.Since(0); len(got) != 0 {
		t.Fatalf("Since(0) = %v, want empty", got)
	}
}

func TestLogBufferNotifySignal(t *testing.T) {
	b := NewLogBuffer(4)
	// Drain any startup signal so the test starts clean.
	select {
	case <-b.Notify():
	default:
	}
	b.Add(LogEntry{Message: "after"})
	select {
	case <-b.Notify():
	default:
		t.Fatalf("Notify did not fire after Add")
	}
	// Second add should not block; notify has buffer size 1.
	b.Add(LogEntry{Message: "second"})
}

func TestWrapWithBufferNil(t *testing.T) {
	var h slog.Handler
	if got := WrapWithBuffer(h, nil); got != nil {
		t.Fatalf("WrapWithBuffer(nil,nil) = %v, want nil", got)
	}
}

func TestWrapWithBufferForwardsAndBuffers(t *testing.T) {
	var sink bytes.Buffer
	base := slog.NewJSONHandler(&sink, &slog.HandlerOptions{Level: slog.LevelDebug})
	buf := NewLogBuffer(10)
	wrapped := WrapWithBuffer(base, buf)
	logger := slog.New(wrapped)
	logger.LogAttrs(context.Background(), slog.LevelInfo, "hello", slog.String("k", "v"))

	// Buffered entry
	entries := buf.Since(10)
	if len(entries) != 1 {
		t.Fatalf("buffered %d entries, want 1", len(entries))
	}
	if entries[0].Message != "hello" {
		t.Fatalf("buffered message = %q, want hello", entries[0].Message)
	}
	if !strings.Contains(entries[0].Attrs, "k=v") {
		t.Fatalf("buffered attrs = %q, want to contain k=v", entries[0].Attrs)
	}

	// Forwarded to underlying sink as JSON
	out := sink.String()
	if !strings.Contains(out, `"msg":"hello"`) {
		t.Fatalf("sink = %q, want msg hello", out)
	}
}

func TestWrapWithBufferPreservesAttrs(t *testing.T) {
	var sink bytes.Buffer
	base := slog.NewJSONHandler(&sink, &slog.HandlerOptions{Level: slog.LevelDebug})
	buf := NewLogBuffer(10)
	logger := slog.New(WrapWithBuffer(base, buf)).With(slog.String("pinned", "yes"))
	logger.Info("test")

	entries := buf.Since(10)
	if len(entries) != 1 {
		t.Fatalf("buffered %d entries, want 1", len(entries))
	}
	if !strings.Contains(entries[0].Attrs, "pinned=yes") {
		t.Fatalf("buffered attrs = %q, want to contain pinned=yes", entries[0].Attrs)
	}
}
