package guardrails

import (
	"context"
	"crypto/rand"
	"strings"
	"sync"
	"testing"
)

func randAlnum(t *testing.T, alphabet string, n int) string {
	t.Helper()
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		t.Fatalf("rand: %v", err)
	}
	out := make([]byte, n)
	for i := range out {
		out[i] = alphabet[int(raw[i])%len(alphabet)]
	}
	return string(out)
}

func awsKey(t *testing.T) string {
	t.Helper()
	return "AKIA" + randAlnum(t, "ABCDEFGHJKLMNPQRSTUVWXYZ234567", 16)
}

func testScanner(t *testing.T, mutate func(*Policy)) *Scanner {
	t.Helper()
	policy := Policy{Enabled: true}
	if mutate != nil {
		mutate(&policy)
	}
	s, err := New(policy)
	if err != nil {
		t.Fatalf("new scanner: %v", err)
	}
	if s == nil {
		t.Fatalf("expected non-nil scanner for enabled policy")
	}
	return s
}

func TestNewDisabledReturnsNil(t *testing.T) {
	s, err := New(Policy{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if s != nil {
		t.Fatalf("expected nil scanner for disabled policy")
	}
}

func TestNewRejectsInvalidPolicy(t *testing.T) {
	for _, policy := range []Policy{
		{Enabled: true, Mode: "watch"},
		{Enabled: true, Mode: ModeBlock, MaxTextBytes: 1},
		{Enabled: true, Mode: ModeBlock, MaxTextBytes: 16 << 20},
		{Enabled: true, Mode: ModeBlock, MaxStrings: 0 - 1},
		{Enabled: true, Mode: ModeBlock, MaxStrings: 8192},
	} {
		if _, err := New(policy); err == nil {
			t.Fatalf("expected error for policy %+v", policy)
		}
	}
}

func TestNewAppliesDefaults(t *testing.T) {
	s := testScanner(t, nil)
	policy := s.Policy()
	if policy.Mode != ModeBlock {
		t.Fatalf("mode = %q, want block", policy.Mode)
	}
	if policy.MaxTextBytes != DefaultMaxTextBytes {
		t.Fatalf("max_text_bytes = %d, want %d", policy.MaxTextBytes, DefaultMaxTextBytes)
	}
	if policy.MaxStrings != DefaultMaxStrings {
		t.Fatalf("max_strings = %d, want %d", policy.MaxStrings, DefaultMaxStrings)
	}
	if n := s.RuleCount(); n <= 100 {
		t.Fatalf("rule count = %d, want a full default rule set", n)
	}
}

func TestScanClean(t *testing.T) {
	s := testScanner(t, nil)
	res := s.Scan(context.Background(), []string{`{"model":"x","messages":[{"role":"user","content":"explain photosynthesis"}]}`})
	if res.Outcome != OutcomeClean {
		t.Fatalf("outcome = %q, want clean", res.Outcome)
	}
	if res.FindingCount != 0 || len(res.RuleIDs) != 0 || res.Reason != "" {
		t.Fatalf("clean result carries metadata: %+v", res)
	}
}

func TestScanFlagsSyntheticSecrets(t *testing.T) {
	s := testScanner(t, nil)
	key := awsKey(t)
	res := s.Scan(context.Background(), []string{`{"model":"x","messages":[{"role":"user","content":"deploy with ` + key + ` now"}]}`})
	if res.Outcome != OutcomeFlagged {
		t.Fatalf("outcome = %q, want flagged", res.Outcome)
	}
	if res.FindingCount == 0 {
		t.Fatalf("flagged result has no findings")
	}
	found := false
	for _, id := range res.RuleIDs {
		if id == "aws-access-token" {
			found = true
		}
	}
	if !found {
		t.Fatalf("rule IDs %+v missing aws-access-token", res.RuleIDs)
	}
	token := "ghp_" + randAlnum(t, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789", 36)
	gh := s.Scan(context.Background(), []string{"review " + token})
	if gh.Outcome != OutcomeFlagged {
		t.Fatalf("github token outcome = %q, want flagged", gh.Outcome)
	}
}

func TestScanIgnoresAllowMarker(t *testing.T) {
	s := testScanner(t, nil)
	key := awsKey(t)
	res := s.Scan(context.Background(), []string{"deploy with " + key + " gitleaks:allow"})
	if res.Outcome != OutcomeFlagged {
		t.Fatalf("outcome = %q, want flagged despite gitleaks:allow", res.Outcome)
	}
}

func TestScanCanceledIsIncomplete(t *testing.T) {
	s := testScanner(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := s.Scan(ctx, []string{"deploy with " + awsKey(t)})
	if res.Outcome != OutcomeIncomplete || res.Reason != ReasonCanceled {
		t.Fatalf("result = %+v, want incomplete/canceled", res)
	}
}

func TestScanOversizeIsIncomplete(t *testing.T) {
	s := testScanner(t, func(p *Policy) { p.MaxTextBytes = 1024 })
	big := strings.Repeat("hello world photosynthesis ", 100)
	res := s.Scan(context.Background(), []string{big})
	if res.Outcome != OutcomeIncomplete || res.Reason != ReasonOversize {
		t.Fatalf("result = %+v, want incomplete/oversize", res)
	}
}

func TestScanTooManyStringsIsIncomplete(t *testing.T) {
	s := testScanner(t, func(p *Policy) { p.MaxStrings = 2 })
	res := s.Scan(context.Background(), []string{"a", "b", "c"})
	if res.Outcome != OutcomeIncomplete || res.Reason != ReasonTooManyStrings {
		t.Fatalf("result = %+v, want incomplete/too_many_strings", res)
	}
}

func TestScanResultExposesNoSecretText(t *testing.T) {
	s := testScanner(t, nil)
	key := awsKey(t)
	res := s.Scan(context.Background(), []string{"deploy with " + key})
	if res.Outcome != OutcomeFlagged {
		t.Fatalf("outcome = %q, want flagged", res.Outcome)
	}
	for _, id := range res.RuleIDs {
		if strings.Contains(id, key) || strings.Contains(key, id) && len(id) > 4 {
			t.Fatalf("rule ID %q leaks secret text", id)
		}
	}
	if strings.Contains(res.Reason, key) {
		t.Fatalf("reason leaks secret text")
	}
}

func TestBlockedMatrix(t *testing.T) {
	flagged := Result{Outcome: OutcomeFlagged, RuleIDs: []string{"aws-access-token"}, FindingCount: 1}
	incomplete := Result{Outcome: OutcomeIncomplete, Reason: ReasonOversize}
	clean := Result{Outcome: OutcomeClean}
	if !flagged.Blocked(ModeBlock) || !incomplete.Blocked(ModeBlock) {
		t.Fatalf("block mode must block flagged and incomplete results")
	}
	if clean.Blocked(ModeBlock) {
		t.Fatalf("block mode must not block clean results")
	}
	if flagged.Blocked(ModeAudit) || incomplete.Blocked(ModeAudit) {
		t.Fatalf("audit mode must never block")
	}
}

func TestScanConcurrentReuse(t *testing.T) {
	s := testScanner(t, nil)
	key := awsKey(t)
	secret := `{"model":"x","messages":[{"role":"user","content":"deploy with ` + key + ` now"}]}`
	clean := `{"model":"x","messages":[{"role":"user","content":"hello"}]}`
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 25; j++ {
				if res := s.Scan(context.Background(), []string{secret}); res.Outcome != OutcomeFlagged {
					t.Error("missed secret under concurrency")
					return
				}
				if res := s.Scan(context.Background(), []string{clean}); res.Outcome != OutcomeClean {
					t.Error("false positive under concurrency")
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestScanMultilineSecret(t *testing.T) {
	s := testScanner(t, nil)
	key := awsKey(t)
	body := "line one\nline two with " + key + "\nline three"
	res := s.Scan(context.Background(), []string{body})
	if res.Outcome != OutcomeFlagged {
		t.Fatalf("outcome = %q, want flagged for multiline input", res.Outcome)
	}
}
