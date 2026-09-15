package guardrails

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMintBlockIDUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 100; i++ {
		id, err := MintBlockID()
		if err != nil {
			t.Fatalf("mint: %v", err)
		}
		if !strings.HasPrefix(id, BlockIDPrefix) {
			t.Fatalf("block id %q missing prefix", id)
		}
		if seen[id] {
			t.Fatalf("duplicate block id %q", id)
		}
		seen[id] = true
	}
}

func TestQuarantineDisabledReturnsNil(t *testing.T) {
	if q := NewQuarantine(QuarantinePolicy{}); q != nil {
		t.Fatalf("expected nil quarantine for disabled policy")
	}
}

func TestQuarantinePolicyValidation(t *testing.T) {
	valid := QuarantinePolicy{Enabled: true, MaxEntries: 10, TTL: time.Minute, MaxSnippet: 64}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid policy rejected: %v", err)
	}
	for _, p := range []QuarantinePolicy{
		{Enabled: true, MaxEntries: 0 - 1},
		{Enabled: true, MaxEntries: MaxQuarantineMaxEntries + 1},
		{Enabled: true, TTL: -time.Second},
		{Enabled: true, MaxSnippet: 1},
		{Enabled: true, MaxSnippet: MaxQuarantineMaxSnippet + 1},
	} {
		if err := p.Validate(); err == nil {
			t.Fatalf("expected error for policy %+v", p)
		}
	}
}

func TestScanCaptureRetainsSecretOnlyOnFlagged(t *testing.T) {
	s := testScanner(t, nil)
	cleanRes, cleanCap := s.ScanCapture(context.Background(), []string{"explain photosynthesis"})
	if cleanRes.Outcome != OutcomeClean || len(cleanCap) != 0 {
		t.Fatalf("clean capture = %+v %+v", cleanRes, cleanCap)
	}
	key := awsKey(t)
	res, captured := s.ScanCapture(context.Background(), []string{"deploy with " + key})
	if res.Outcome != OutcomeFlagged {
		t.Fatalf("outcome = %q, want flagged", res.Outcome)
	}
	if len(captured) == 0 {
		t.Fatalf("flagged capture has no findings")
	}
	found := false
	for _, f := range captured {
		if strings.Contains(f.Secret, key) || strings.Contains(f.Match, key) {
			found = true
		}
		if f.RuleID == "" {
			t.Fatalf("finding missing rule id: %+v", f)
		}
	}
	if !found {
		t.Fatalf("capture does not contain the synthetic key: %+v", captured)
	}
}

func TestQuarantineStoreTakeOnceAndList(t *testing.T) {
	q := NewQuarantine(QuarantinePolicy{Enabled: true})
	key := awsKey(t)
	q.Store("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Capture{
		Operation:   "chat_completions",
		PublicModel: "alias/x",
		RuleIDs:     []string{"aws-access-token"},
		Findings:    []CapturedFinding{{RuleID: "aws-access-token", Secret: key}},
	})
	summaries := q.List()
	if len(summaries) != 1 {
		t.Fatalf("list = %+v, want 1 entry", summaries)
	}
	if len(summaries[0].RuleIDs) != 1 {
		t.Fatalf("summary = %+v", summaries[0])
	}
	for _, id := range summaries[0].RuleIDs {
		if strings.Contains(id, key) {
			t.Fatalf("summary leaks secret text")
		}
	}
	if summaries[0].FindingCount != 1 {
		t.Fatalf("finding count = %d, want 1", summaries[0].FindingCount)
	}
	got, ok := q.Take("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if !ok {
		t.Fatalf("take missed stored block")
	}
	if len(got.Findings) != 1 || !strings.Contains(got.Findings[0].Secret, key) {
		t.Fatalf("capture = %+v", got)
	}
	if _, ok := q.Take("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); ok {
		t.Fatalf("second take should miss (take-once)")
	}
	if n := len(q.List()); n != 0 {
		t.Fatalf("list after take = %d, want 0", n)
	}
}

func TestQuarantineExpiryAndEviction(t *testing.T) {
	now := time.Now()
	q := NewQuarantine(QuarantinePolicy{Enabled: true, MaxEntries: 2, TTL: time.Minute, MaxSnippet: 512})
	q.SetNowFunc(func() time.Time { return now })
	q.Store("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Capture{RuleIDs: []string{"a"}})
	q.Store("blk_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", Capture{RuleIDs: []string{"b"}})
	q.Store("blk_cccccccccccccccccccccccccccccccc", Capture{RuleIDs: []string{"c"}})
	if got := q.Len(); got != 2 {
		t.Fatalf("len = %d, want 2 after eviction", got)
	}
	if _, ok := q.Take("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"); ok {
		t.Fatalf("oldest entry should have been evicted")
	}
	now = now.Add(2 * time.Minute)
	if got := q.Len(); got != 0 {
		t.Fatalf("len after TTL = %d, want 0", got)
	}
	if _, ok := q.Take("blk_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"); ok {
		t.Fatalf("expired entry should miss")
	}
}

func TestQuarantineTruncatesSnippets(t *testing.T) {
	q := NewQuarantine(QuarantinePolicy{Enabled: true, MaxSnippet: 16})
	q.Store("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Capture{
		Findings: []CapturedFinding{{RuleID: "r", Secret: strings.Repeat("x", 100), Match: strings.Repeat("y", 100)}},
	})
	got, ok := q.Take("blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if !ok {
		t.Fatalf("take missed")
	}
	if len(got.Findings[0].Secret) != 16 || len(got.Findings[0].Match) != 16 {
		t.Fatalf("snippets not truncated: %+v", got.Findings[0])
	}
}
