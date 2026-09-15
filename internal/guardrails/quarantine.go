package guardrails

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/zricethezav/gitleaks/v8/detect"
)

const (
	DefaultQuarantineMaxEntries = 128
	DefaultQuarantineTTL        = 15 * time.Minute
	DefaultQuarantineMaxSnippet = 512

	MaxQuarantineMaxEntries = 4096
	MaxQuarantineMaxSnippet = 8 << 10

	BlockIDPrefix = "blk_"
	BlockIDBytes  = 16

	MaxCapturedFindings = 16
)

type QuarantinePolicy struct {
	Enabled    bool
	MaxEntries int
	TTL        time.Duration
	MaxSnippet int
}

func (p QuarantinePolicy) WithDefaults() QuarantinePolicy {
	if p.MaxEntries <= 0 {
		p.MaxEntries = DefaultQuarantineMaxEntries
	}
	if p.TTL <= 0 {
		p.TTL = DefaultQuarantineTTL
	}
	if p.MaxSnippet <= 0 {
		p.MaxSnippet = DefaultQuarantineMaxSnippet
	}
	return p
}

func (p QuarantinePolicy) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.MaxEntries < 0 || p.MaxEntries > MaxQuarantineMaxEntries {
		return fmt.Errorf("max_entries must be between 0 and %d (0 selects default %d)", MaxQuarantineMaxEntries, DefaultQuarantineMaxEntries)
	}
	if p.MaxSnippet < 0 || p.MaxSnippet > MaxQuarantineMaxSnippet {
		return fmt.Errorf("max_snippet_bytes must be between 0 and %d (0 selects default %d)", MaxQuarantineMaxSnippet, DefaultQuarantineMaxSnippet)
	}
	if p.MaxSnippet > 0 && p.MaxSnippet < 16 {
		return fmt.Errorf("max_snippet_bytes must be 0 or between 16 and %d", MaxQuarantineMaxSnippet)
	}
	if p.TTL < 0 || p.TTL > 24*time.Hour {
		return fmt.Errorf("ttl must be 0 or a positive duration at most 24h (0 selects default)")
	}
	effective := p.WithDefaults()
	if effective.MaxEntries < 1 || effective.MaxEntries > MaxQuarantineMaxEntries {
		return fmt.Errorf("max_entries must be between 1 and %d", MaxQuarantineMaxEntries)
	}
	if effective.TTL <= 0 || effective.TTL > 24*time.Hour {
		return fmt.Errorf("ttl must be positive and at most 24h")
	}
	if effective.MaxSnippet < 16 || effective.MaxSnippet > MaxQuarantineMaxSnippet {
		return fmt.Errorf("max_snippet_bytes must be between 16 and %d", MaxQuarantineMaxSnippet)
	}
	return nil
}

type CapturedFinding struct {
	RuleID      string `json:"rule_id"`
	Description string `json:"description,omitempty"`
	Secret      string `json:"secret"`
	Match       string `json:"match,omitempty"`
	Line        string `json:"line,omitempty"`
}

type Capture struct {
	BlockID     string            `json:"block_id"`
	Timestamp   time.Time         `json:"ts"`
	Operation   string            `json:"operation,omitempty"`
	PublicModel string            `json:"public_model,omitempty"`
	RuleIDs     []string          `json:"rule_ids"`
	Findings    []CapturedFinding `json:"findings"`
}

type BlockSummary struct {
	BlockID      string    `json:"block_id"`
	Timestamp    time.Time `json:"ts"`
	Operation    string    `json:"operation,omitempty"`
	PublicModel  string    `json:"public_model,omitempty"`
	RuleIDs      []string  `json:"rule_ids"`
	FindingCount int       `json:"finding_count"`
}

type Quarantine struct {
	mu      sync.Mutex
	policy  QuarantinePolicy
	entries map[string]*quarantineEntry
	order   []string
	now     func() time.Time
}

type quarantineEntry struct {
	capture Capture
	expires time.Time
}

func NewQuarantine(policy QuarantinePolicy) *Quarantine {
	if !policy.Enabled {
		return nil
	}
	effective := policy.WithDefaults()
	return &Quarantine{
		policy:  effective,
		entries: make(map[string]*quarantineEntry),
		now:     time.Now,
	}
}

func (q *Quarantine) Policy() QuarantinePolicy {
	return q.policy
}

func (q *Quarantine) SetNowFunc(fn func() time.Time) {
	if q == nil {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if fn == nil {
		q.now = time.Now
		return
	}
	q.now = fn
}

func MintBlockID() (string, error) {
	raw := make([]byte, BlockIDBytes)
	if _, err := rand.Read(raw); err != nil {
		return "", fmt.Errorf("generate block id: %w", err)
	}
	return BlockIDPrefix + hex.EncodeToString(raw), nil
}

func (s *Scanner) ScanCapture(ctx context.Context, texts []string) (Result, []CapturedFinding) {
	res := s.Scan(ctx, texts)
	if res.Outcome != OutcomeFlagged {
		return res, nil
	}
	maxSnippet := DefaultQuarantineMaxSnippet
	var out []CapturedFinding
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return Result{Outcome: OutcomeIncomplete, Reason: ReasonCanceled}, nil
		}
		findings := s.detector.DetectContext(ctx, detect.Fragment{Raw: text})
		if err := ctx.Err(); err != nil {
			return Result{Outcome: OutcomeIncomplete, Reason: ReasonCanceled}, nil
		}
		for _, f := range findings {
			out = append(out, CapturedFinding{
				RuleID:      f.RuleID,
				Description: f.Description,
				Secret:      truncateSecret(f.Secret, maxSnippet),
				Match:       truncateSecret(f.Match, maxSnippet),
				Line:        truncateSecret(f.Line, maxSnippet),
			})
			if len(out) >= MaxCapturedFindings {
				return res, out
			}
		}
	}
	return res, out
}

func truncateSecret(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max]
}

func (q *Quarantine) Store(blockID string, capture Capture) {
	if q == nil || blockID == "" {
		return
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()
	q.evictExpiredLocked(now)
	capture.BlockID = blockID
	if capture.Timestamp.IsZero() {
		capture.Timestamp = now
	}
	snippets := q.policy.MaxSnippet
	for i := range capture.Findings {
		capture.Findings[i].Secret = truncateSecret(capture.Findings[i].Secret, snippets)
		capture.Findings[i].Match = truncateSecret(capture.Findings[i].Match, snippets)
		capture.Findings[i].Line = truncateSecret(capture.Findings[i].Line, snippets)
	}
	if len(capture.RuleIDs) > MaxReportedRuleIDs {
		ruleIDs := append([]string(nil), capture.RuleIDs...)
		sort.Strings(ruleIDs)
		capture.RuleIDs = ruleIDs[:MaxReportedRuleIDs]
	}
	if _, ok := q.entries[blockID]; !ok {
		q.order = append(q.order, blockID)
	}
	q.entries[blockID] = &quarantineEntry{capture: capture, expires: now.Add(q.policy.TTL)}
	for len(q.order) > q.policy.MaxEntries {
		oldest := q.order[0]
		q.order = q.order[1:]
		delete(q.entries, oldest)
	}
}

func (q *Quarantine) Take(blockID string) (Capture, bool) {
	if q == nil || blockID == "" {
		return Capture{}, false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()
	entry, ok := q.entries[blockID]
	if !ok {
		return Capture{}, false
	}
	if !now.Before(entry.expires) {
		q.removeLocked(blockID)
		return Capture{}, false
	}
	out := entry.capture
	q.removeLocked(blockID)
	return out, true
}

func (q *Quarantine) List() []BlockSummary {
	if q == nil {
		return nil
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	now := q.now()
	q.evictExpiredLocked(now)
	out := make([]BlockSummary, 0, len(q.order))
	for _, id := range q.order {
		entry, ok := q.entries[id]
		if !ok {
			continue
		}
		c := entry.capture
		out = append(out, BlockSummary{
			BlockID:      c.BlockID,
			Timestamp:    c.Timestamp,
			Operation:    c.Operation,
			PublicModel:  c.PublicModel,
			RuleIDs:      append([]string(nil), c.RuleIDs...),
			FindingCount: len(c.Findings),
		})
	}
	return out
}

func (q *Quarantine) Len() int {
	if q == nil {
		return 0
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.evictExpiredLocked(q.now())
	return len(q.entries)
}

func (q *Quarantine) evictExpiredLocked(now time.Time) {
	kept := q.order[:0]
	for _, id := range q.order {
		entry, ok := q.entries[id]
		if !ok {
			continue
		}
		if !now.Before(entry.expires) {
			delete(q.entries, id)
			continue
		}
		kept = append(kept, id)
	}
	for i := len(kept); i < len(q.order); i++ {
		q.order[i] = ""
	}
	q.order = kept
}

func (q *Quarantine) removeLocked(blockID string) {
	delete(q.entries, blockID)
	for i, id := range q.order {
		if id == blockID {
			q.order = append(q.order[:i], q.order[i+1:]...)
			return
		}
	}
}
