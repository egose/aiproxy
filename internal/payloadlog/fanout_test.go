package payloadlog

import (
	"errors"
	"testing"
)

type stubRecorder struct {
	max     int
	entries []Entry
	err     error
}

func (s *stubRecorder) MaxBodyBytes() int { return s.max }

func (s *stubRecorder) Record(e Entry) error {
	s.entries = append(s.entries, e)
	return s.err
}

func TestCombineNil(t *testing.T) {
	if Combine(nil, nil) != nil {
		t.Fatal("expected nil recorder when all members are nil")
	}
}

func TestCombineSingle(t *testing.T) {
	s := &stubRecorder{max: 128}
	if Combine(nil, s) != Recorder(s) {
		t.Fatal("expected single member to pass through")
	}
}

func TestCombineFansOut(t *testing.T) {
	a := &stubRecorder{max: 1024}
	b := &stubRecorder{max: 512}
	rec := Combine(a, b)
	if rec == nil {
		t.Fatal("expected non-nil recorder")
	}
	entry := Entry{RequestID: "req-1", Status: 200}
	if err := rec.Record(entry); err != nil {
		t.Fatalf("record: %v", err)
	}
	if len(a.entries) != 1 || len(b.entries) != 1 {
		t.Fatalf("entries = %d/%d, want 1/1", len(a.entries), len(b.entries))
	}
	if got := rec.MaxBodyBytes(); got != 512 {
		t.Fatalf("max body = %d, want minimum 512", got)
	}
}

func TestCombineReturnsFirstError(t *testing.T) {
	sentinel := errors.New("boom")
	a := &stubRecorder{err: sentinel}
	b := &stubRecorder{}
	if err := Combine(a, b).Record(Entry{}); !errors.Is(err, sentinel) {
		t.Fatalf("error = %v, want %v", err, sentinel)
	}
	if len(b.entries) != 1 {
		t.Fatal("second member should still record after first fails")
	}
}
