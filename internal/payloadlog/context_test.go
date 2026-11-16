package payloadlog

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
)

type contextTestRecorder struct {
	stubRecorder
	contexts []context.Context
	record   func(context.Context) error
}

func (r *contextTestRecorder) RecordContext(ctx context.Context, entry Entry) error {
	r.contexts = append(r.contexts, ctx)
	r.entries = append(r.entries, entry)
	if r.record != nil {
		return r.record(ctx)
	}
	return ctx.Err()
}

func TestFanoutContextPreservesDeadlineAndSkipsExpiredRemoteAndLocalCalls(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	first := &contextTestRecorder{record: func(ctx context.Context) error {
		<-ctx.Done()
		return ctx.Err()
	}}
	remote, local := &contextTestRecorder{}, &stubRecorder{}
	recorder := Combine(first, remote, local)
	if err := RecordContext(ctx, recorder, Entry{RequestID: "once"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("remote sink deadline was lost: %v", err)
	}
	if len(first.contexts) != 1 || first.contexts[0] != ctx || len(first.entries) != 1 || len(remote.contexts) != 0 ||
		len(remote.entries) != 0 || len(local.entries) != 0 {
		t.Fatal("fanout renewed the completion deadline or invoked later sinks after expiry")
	}
	if err := RecordContext(ctx, recorder, Entry{RequestID: "expired"}); !errors.Is(err, context.DeadlineExceeded) ||
		len(first.entries) != 1 {
		t.Fatal("an expired queued record retried persistence")
	}
	active, cancelActive := context.WithTimeout(context.Background(), time.Second)
	defer cancelActive()
	if err := RecordContext(active, Combine(remote, local), Entry{RequestID: "active"}); err != nil ||
		len(remote.contexts) != 1 || remote.contexts[0] != active || len(local.entries) != 1 {
		t.Fatal("context-aware remote and bounded local fallback paths diverged")
	}
}

func TestFileRecordContextRejectsExpiredWorkBeforeFilesystemWrite(t *testing.T) {
	dir := t.TempDir()
	logger, err := New(config.PayloadLog{Enabled: true, Dir: dir, MaxBodyBytes: 64})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = logger.Close() })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := logger.RecordContext(ctx, Entry{RequestID: "expired"}); !errors.Is(err, context.Canceled) {
		t.Fatalf("expired local sink call was not rejected before write: %v", err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 0 {
		t.Fatal("expired completion wrote a local payload file")
	}
	if err := logger.RecordContext(context.Background(), Entry{RequestID: "active"}); err != nil {
		t.Fatal(err)
	}
	files, err = os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatal("active local payload completion did not write once")
	}
}
