package guardrails

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/viper"
	gitleaksconfig "github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
)

type Mode string

const (
	ModeAudit Mode = "audit"
	ModeBlock Mode = "block"
)

const (
	DefaultMode         = ModeBlock
	DefaultMaxTextBytes = 65536
	DefaultMaxStrings   = 512
	MaxReportedRuleIDs  = 16
)

const (
	MinMaxTextBytes = 1024
	MaxMaxTextBytes = 32 << 20
	MinMaxStrings   = 1
	MaxMaxStrings   = 16384
)

type Policy struct {
	Enabled      bool
	Mode         Mode
	MaxTextBytes int
	MaxStrings   int
}

func (p Policy) WithDefaults() Policy {
	if p.Mode == "" {
		p.Mode = DefaultMode
	}
	if p.MaxTextBytes <= 0 {
		p.MaxTextBytes = DefaultMaxTextBytes
	}
	if p.MaxStrings <= 0 {
		p.MaxStrings = DefaultMaxStrings
	}
	return p
}

func (p Policy) Validate() error {
	if !p.Enabled {
		return nil
	}
	if p.MaxTextBytes < 0 || p.MaxStrings < 0 {
		return fmt.Errorf("max_text_bytes and max_strings must not be negative (0 selects defaults)")
	}
	effective := p.WithDefaults()
	switch effective.Mode {
	case ModeAudit, ModeBlock:
	default:
		return fmt.Errorf("mode must be audit or block, got %q", p.Mode)
	}
	if effective.MaxTextBytes < MinMaxTextBytes || effective.MaxTextBytes > MaxMaxTextBytes {
		return fmt.Errorf("max_text_bytes must be between %d and %d", MinMaxTextBytes, MaxMaxTextBytes)
	}
	if effective.MaxStrings < MinMaxStrings || effective.MaxStrings > MaxMaxStrings {
		return fmt.Errorf("max_strings must be between %d and %d", MinMaxStrings, MaxMaxStrings)
	}
	return nil
}

type Outcome string

const (
	OutcomeClean      Outcome = "clean"
	OutcomeFlagged    Outcome = "flagged"
	OutcomeIncomplete Outcome = "incomplete"
)

const (
	ReasonOversize       = "oversize"
	ReasonTooManyStrings = "too_many_strings"
	ReasonCanceled       = "canceled"
)

type Result struct {
	Outcome      Outcome
	RuleIDs      []string
	FindingCount int
	Reason       string
}

func (r Result) Blocked(mode Mode) bool {
	if mode != ModeBlock {
		return false
	}
	return r.Outcome == OutcomeFlagged || r.Outcome == OutcomeIncomplete
}

type Scanner struct {
	policy   Policy
	detector *detect.Detector
}

func New(policy Policy) (*Scanner, error) {
	if !policy.Enabled {
		return nil, nil
	}
	if err := policy.Validate(); err != nil {
		return nil, err
	}
	effective := policy.WithDefaults()
	detector, err := newDetector()
	if err != nil {
		return nil, err
	}
	return &Scanner{policy: effective, detector: detector}, nil
}

func newDetector() (*detect.Detector, error) {
	v := viper.New()
	v.SetConfigType("toml")
	if err := v.ReadConfig(strings.NewReader(gitleaksconfig.DefaultConfig)); err != nil {
		return nil, fmt.Errorf("read default rules: %w", err)
	}
	var vc gitleaksconfig.ViperConfig
	if err := v.Unmarshal(&vc); err != nil {
		return nil, fmt.Errorf("parse default rules: %w", err)
	}
	cfg, err := vc.Translate()
	if err != nil {
		return nil, fmt.Errorf("compile default rules: %w", err)
	}
	detector := detect.NewDetector(cfg)
	detector.IgnoreGitleaksAllow = true
	detector.MaxDecodeDepth = 0
	return detector, nil
}

func (s *Scanner) Policy() Policy {
	return s.policy
}

func (s *Scanner) RuleCount() int {
	return len(s.detector.Config.Rules)
}

func (s *Scanner) Scan(ctx context.Context, texts []string) Result {
	if len(texts) > s.policy.MaxStrings {
		return Result{Outcome: OutcomeIncomplete, Reason: ReasonTooManyStrings}
	}
	var total int
	for _, text := range texts {
		total += len(text)
		if total > s.policy.MaxTextBytes {
			return Result{Outcome: OutcomeIncomplete, Reason: ReasonOversize}
		}
	}
	seen := make(map[string]struct{})
	var ruleIDs []string
	var count int
	for _, text := range texts {
		if err := ctx.Err(); err != nil {
			return Result{Outcome: OutcomeIncomplete, Reason: ReasonCanceled}
		}
		findings := s.detector.DetectContext(ctx, detect.Fragment{Raw: text})
		if err := ctx.Err(); err != nil {
			return Result{Outcome: OutcomeIncomplete, Reason: ReasonCanceled}
		}
		for _, finding := range findings {
			count++
			if _, ok := seen[finding.RuleID]; !ok {
				seen[finding.RuleID] = struct{}{}
				if len(ruleIDs) < MaxReportedRuleIDs {
					ruleIDs = append(ruleIDs, finding.RuleID)
				}
			}
		}
	}
	if count == 0 {
		return Result{Outcome: OutcomeClean}
	}
	sort.Strings(ruleIDs)
	return Result{Outcome: OutcomeFlagged, RuleIDs: ruleIDs, FindingCount: count}
}
