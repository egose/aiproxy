package guardrails

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/filestore"
)

const (
	ExceptionActionAllow  = "allow"
	ExceptionActionRedact = "redact"
	ExceptionActionDeny   = "deny"

	DefaultRedactPlaceholder = "REDACTED"
	DefaultExceptionsFile    = "guardrail-exceptions.json"

	MaxExceptionsEntries = 4096
	MaxDecisionSHAs      = 16

	MinPlaceholderLen = 1
	MaxPlaceholderLen = 256
)

func Fingerprint(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func ValidFingerprint(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func ValidExceptionAction(a string) bool {
	return a == ExceptionActionAllow || a == ExceptionActionRedact || a == ExceptionActionDeny
}

func ValidatePlaceholder(ph string) error {
	if len(ph) < MinPlaceholderLen || len(ph) > MaxPlaceholderLen {
		return fmt.Errorf("redact_placeholder must be between %d and %d characters", MinPlaceholderLen, MaxPlaceholderLen)
	}
	for i := 0; i < len(ph); i++ {
		if ph[i] < 0x20 || ph[i] > 0x7e {
			return fmt.Errorf("redact_placeholder must be printable ASCII without newlines")
		}
	}
	return nil
}

func DefaultExceptionsPath() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "aiproxy", DefaultExceptionsFile)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join("aiproxy", DefaultExceptionsFile)
	}
	return filepath.Join(home, ".config", "aiproxy", DefaultExceptionsFile)
}

type ExceptionEntry struct {
	Action      string    `json:"action"`
	CreatedAt   time.Time `json:"created_at"`
	SourceBlock string    `json:"source_block,omitempty"`
	RuleID      string    `json:"rule_id,omitempty"`
}

type exceptionsFile struct {
	Version int                       `json:"version"`
	Entries map[string]ExceptionEntry `json:"entries"`
}

type Exceptions struct {
	mu          sync.Mutex
	path        string
	placeholder string
	entries     map[string]ExceptionEntry
	now         func() time.Time
}

func LoadExceptions(path, placeholder string) (*Exceptions, error) {
	if path == "" {
		path = DefaultExceptionsPath()
	}
	if placeholder == "" {
		placeholder = DefaultRedactPlaceholder
	}
	if err := ValidatePlaceholder(placeholder); err != nil {
		return nil, err
	}
	e := &Exceptions{
		path:        path,
		placeholder: placeholder,
		entries:     make(map[string]ExceptionEntry),
		now:         time.Now,
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return e, nil
		}
		return nil, fmt.Errorf("read guardrail exceptions %q: %w", path, err)
	}
	if len(data) == 0 {
		return e, nil
	}
	var file exceptionsFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("parse guardrail exceptions %q: %w", path, err)
	}
	if file.Version != 0 && file.Version != 1 {
		return nil, fmt.Errorf("guardrail exceptions %q has unsupported version %d", path, file.Version)
	}
	if len(file.Entries) > MaxExceptionsEntries {
		return nil, fmt.Errorf("guardrail exceptions %q holds %d entries, max %d", path, len(file.Entries), MaxExceptionsEntries)
	}
	for sha, entry := range file.Entries {
		if !ValidFingerprint(sha) {
			return nil, fmt.Errorf("guardrail exceptions %q has invalid fingerprint %q", path, sha)
		}
		if !ValidExceptionAction(entry.Action) {
			return nil, fmt.Errorf("guardrail exceptions %q entry %q has invalid action %q", path, sha, entry.Action)
		}
		e.entries[sha] = entry
	}
	return e, nil
}

func (e *Exceptions) Path() string {
	if e == nil {
		return ""
	}
	return e.path
}

func (e *Exceptions) Placeholder() string {
	if e == nil {
		return ""
	}
	return e.placeholder
}

func (e *Exceptions) SetNowFunc(fn func() time.Time) {
	if e == nil {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if fn == nil {
		e.now = time.Now
		return
	}
	e.now = fn
}

func (e *Exceptions) Lookup(sha string) (ExceptionEntry, bool) {
	if e == nil || sha == "" {
		return ExceptionEntry{}, false
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	entry, ok := e.entries[sha]
	return entry, ok
}

func (e *Exceptions) Len() int {
	if e == nil {
		return 0
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.entries)
}

func (e *Exceptions) Decide(shas []string, action, sourceBlock string, ruleIDs map[string]string) (int, error) {
	if e == nil {
		return 0, fmt.Errorf("guardrail exceptions store is not configured")
	}
	if !ValidExceptionAction(action) {
		return 0, fmt.Errorf("invalid action %q (must be allow, redact, or deny)", action)
	}
	if len(shas) == 0 || len(shas) > MaxDecisionSHAs {
		return 0, fmt.Errorf("decision must list between 1 and %d secret fingerprints", MaxDecisionSHAs)
	}
	seen := make(map[string]struct{}, len(shas))
	for _, sha := range shas {
		if !ValidFingerprint(sha) {
			return 0, fmt.Errorf("invalid secret fingerprint %q", sha)
		}
		if _, dup := seen[sha]; dup {
			return 0, fmt.Errorf("duplicate secret fingerprint %q", sha)
		}
		seen[sha] = struct{}{}
	}
	e.mu.Lock()
	if len(e.entries)+len(seen) > MaxExceptionsEntries+len(seen) {
		e.mu.Unlock()
		return 0, fmt.Errorf("guardrail exceptions store is full (%d entries)", MaxExceptionsEntries)
	}
	now := e.now()
	count := 0
	for sha := range seen {
		if existing, ok := e.entries[sha]; ok && existing.Action == action {
			continue
		}
		entry := ExceptionEntry{Action: action, CreatedAt: now, SourceBlock: sourceBlock}
		if ruleIDs != nil {
			entry.RuleID = ruleIDs[sha]
		}
		e.entries[sha] = entry
		count++
	}
	snapshot := make(map[string]ExceptionEntry, len(e.entries))
	for sha, entry := range e.entries {
		snapshot[sha] = entry
	}
	e.mu.Unlock()
	if err := e.persist(snapshot); err != nil {
		return 0, err
	}
	return count, nil
}

func (e *Exceptions) persist(snapshot map[string]ExceptionEntry) error {
	shas := make([]string, 0, len(snapshot))
	for sha := range snapshot {
		shas = append(shas, sha)
	}
	sort.Strings(shas)
	ordered := make(map[string]ExceptionEntry, len(snapshot))
	for _, sha := range shas {
		ordered[sha] = snapshot[sha]
	}
	data, err := json.MarshalIndent(exceptionsFile{Version: 1, Entries: ordered}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode guardrail exceptions: %w", err)
	}
	data = append(data, '\n')
	if err := filestore.WriteFile(e.path, data, 0o600, filestore.Options{DirMode: 0o700, Secret: true}); err != nil {
		return fmt.Errorf("write guardrail exceptions %q: %w", e.path, err)
	}
	return nil
}
