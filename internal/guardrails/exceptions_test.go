package guardrails

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFingerprintStable(t *testing.T) {
	a := Fingerprint("sk-test-value")
	b := Fingerprint("sk-test-value")
	if a != b || !ValidFingerprint(a) {
		t.Fatalf("fingerprint unstable or invalid: %q", a)
	}
	if Fingerprint("other") == a {
		t.Fatalf("distinct secrets share a fingerprint")
	}
	if ValidFingerprint("xyz") || ValidFingerprint(strings.Repeat("a", 63)) {
		t.Fatalf("invalid fingerprints accepted")
	}
}

func TestValidatePlaceholder(t *testing.T) {
	if err := ValidatePlaceholder("REDACTED"); err != nil {
		t.Fatalf("valid placeholder rejected: %v", err)
	}
	for _, ph := range []string{"", strings.Repeat("x", 257), "bad\nvalue"} {
		if err := ValidatePlaceholder(ph); err == nil {
			t.Fatalf("expected error for placeholder %q", ph)
		}
	}
}

func TestExceptionsDecideRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exceptions.json")
	e, err := LoadExceptions(path, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if e.Placeholder() != DefaultRedactPlaceholder {
		t.Fatalf("placeholder = %q, want default", e.Placeholder())
	}
	sha := Fingerprint("sk-test-value")
	if _, ok := e.Lookup(sha); ok {
		t.Fatalf("unexpected entry before decide")
	}
	n, err := e.Decide([]string{sha}, ExceptionActionAllow, "blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", map[string]string{sha: "generic-api-key"})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if n != 1 {
		t.Fatalf("count = %d, want 1", n)
	}
	entry, ok := e.Lookup(sha)
	if !ok || entry.Action != ExceptionActionAllow {
		t.Fatalf("lookup = %+v %v", entry, ok)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if strings.Contains(string(raw), "sk-test-value") {
		t.Fatalf("exceptions file retains raw secret text")
	}
	if !strings.Contains(string(raw), sha) {
		t.Fatalf("exceptions file missing fingerprint: %s", raw)
	}
	reloaded, err := LoadExceptions(path, "CUSTOM")
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if entry, ok := reloaded.Lookup(sha); !ok || entry.Action != ExceptionActionAllow {
		t.Fatalf("reloaded lookup = %+v %v", entry, ok)
	}
	n, err = reloaded.Decide([]string{sha}, ExceptionActionAllow, "blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", nil)
	if err != nil || n != 0 {
		t.Fatalf("idempotent decide = %d %v, want 0 nil", n, err)
	}
}

func TestExceptionsDecideRejectsInvalid(t *testing.T) {
	path := filepath.Join(t.TempDir(), "exceptions.json")
	e, err := LoadExceptions(path, "")
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	sha := Fingerprint("sk-test-value")
	for _, tc := range []struct {
		name   string
		shas   []string
		action string
	}{
		{"empty", nil, ExceptionActionAllow},
		{"bad-sha", []string{"xyz"}, ExceptionActionAllow},
		{"bad-action", []string{sha}, "quarantine"},
		{"dup", []string{sha, sha}, ExceptionActionAllow},
	} {
		if _, err := e.Decide(tc.shas, tc.action, "", nil); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}

func TestScanCaptureIncludesFingerprint(t *testing.T) {
	s := testScanner(t, nil)
	key := awsKey(t)
	_, captured := s.ScanCapture(context.Background(), []string{"deploy with " + key})
	if len(captured) == 0 {
		t.Fatalf("no findings captured")
	}
	for _, f := range captured {
		if !ValidFingerprint(f.SecretSHA) {
			t.Fatalf("finding missing fingerprint: %+v", f)
		}
		if f.SecretSHA != Fingerprint(f.Secret) && !strings.Contains(key, f.Secret) {
			t.Fatalf("fingerprint does not match secret for %+v", f)
		}
	}
}
