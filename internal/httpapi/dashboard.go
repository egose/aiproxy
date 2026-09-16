package httpapi

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/payloadlog"
)

const dashboardRecentN = 200

const (
	dashboardAuthRatePerMinute = 20
	dashboardAuthBurst         = 5
)

type dashboardAuthLimiter struct {
	mu      sync.Mutex
	bucket  int
	last    time.Time
	maxOnce time.Duration
	now     func() time.Time
}

func newDashboardAuthLimiter() *dashboardAuthLimiter {
	return &dashboardAuthLimiter{
		bucket:  dashboardAuthBurst,
		now:     time.Now,
		maxOnce: time.Minute,
	}
}

func (l *dashboardAuthLimiter) Allow() (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.last.IsZero() {
		l.last = now
	}
	elapsed := now.Sub(l.last).Seconds()
	l.bucket += int(elapsed * float64(dashboardAuthRatePerMinute) / 60)
	if l.bucket > dashboardAuthBurst {
		l.bucket = dashboardAuthBurst
	}
	l.last = now
	if l.bucket >= 1 {
		l.bucket--
		return true, 0
	}
	deficit := 1 - l.bucket
	retryAfter := time.Duration(float64(deficit) / float64(dashboardAuthRatePerMinute) * float64(time.Minute))
	if retryAfter < time.Second {
		retryAfter = time.Second
	}
	return false, retryAfter
}

// handleDashboard routes /_internal/dashboard/* requests. Returns true if
// the request was handled. When the dashboard block is unconfigured, every
// path under /_internal/* returns 404 so the surface stays closed.
func (h *Handler) handleDashboard(deps Dependencies, w http.ResponseWriter, r *http.Request) bool {
	if deps.Dashboard == nil || !deps.Dashboard.Enabled() {
		return false
	}
	if r.URL.Path == dashrpc.SnapshotPath {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !dashboardAuthorized(deps.Dashboard, r) {
			h.respondDashboardAuthFailure(w, r)
			return true
		}
		h.writeDashboardSnapshot(deps, w, r)
		return true
	}
	if r.URL.Path == dashrpc.LogsPath {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !dashboardAuthorized(deps.Dashboard, r) {
			h.respondDashboardAuthFailure(w, r)
			return true
		}
		h.writeDashboardLogs(deps, w, r)
		return true
	}
	if r.URL.Path == dashrpc.PayloadsPath {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !dashboardAuthorized(deps.Dashboard, r) {
			h.respondDashboardAuthFailure(w, r)
			return true
		}
		h.writeDashboardPayloads(deps, w, r)
		return true
	}
	if strings.HasPrefix(r.URL.Path, dashrpc.PayloadPathPrefix) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !dashboardAuthorized(deps.Dashboard, r) {
			h.respondDashboardAuthFailure(w, r)
			return true
		}
		h.writeDashboardPayload(deps, w, r)
		return true
	}
	if r.URL.Path == dashrpc.BlocksPath {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !dashboardAuthorized(deps.Dashboard, r) {
			h.respondDashboardAuthFailure(w, r)
			return true
		}
		h.writeDashboardBlocks(deps, w, r)
		return true
	}
	if strings.HasPrefix(r.URL.Path, dashrpc.BlockPathPrefix) {
		if strings.HasSuffix(r.URL.Path, dashrpc.BlockDecisionSuffix) {
			if r.Method != http.MethodPost {
				w.Header().Set("Allow", http.MethodPost)
				http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
				return true
			}
			if !dashboardAuthorized(deps.Dashboard, r) {
				h.respondDashboardAuthFailure(w, r)
				return true
			}
			h.writeDashboardBlockDecision(deps, w, r)
			return true
		}
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !dashboardAuthorized(deps.Dashboard, r) {
			h.respondDashboardAuthFailure(w, r)
			return true
		}
		h.writeDashboardBlock(deps, w, r)
		return true
	}
	return false
}

func (h *Handler) respondDashboardAuthFailure(w http.ResponseWriter, r *http.Request) {
	limiter := dashboardAuthLimiterFor(h)
	allowed, retryAfter := limiter.Allow()
	if !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":"dashboard auth rate limited"}`))
		return
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="aiproxy dashboard"`)
	http.Error(w, "unauthorized", http.StatusUnauthorized)
}

func dashboardAuthorized(source dashrpc.Source, r *http.Request) bool {
	if source == nil || source.Token() == "" {
		return false
	}
	got := r.Header.Get(dashrpc.AuthHeaderName)
	if got == "" {
		return false
	}
	if len(got) < len(dashrpc.AuthScheme) {
		return false
	}
	if got[:len(dashrpc.AuthScheme)] != dashrpc.AuthScheme {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got[len(dashrpc.AuthScheme):]), []byte(source.Token())) == 1
}

type snapshotResponse struct {
	dashrpc.Snapshot
	LastSeq uint64 `json:"last_seq"`
}

func (h *Handler) writeDashboardSnapshot(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	snap := deps.Dashboard.Snapshot(r.Context(), dashboardRecentN)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshotResponse{Snapshot: snap, LastSeq: snap.LastSeq})
}

func (h *Handler) writeDashboardLogs(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(deps.Dashboard.Logs(since))
}

type dashboardPayloadSource interface {
	PayloadEnabled() bool
	ListPayloads(limit int, errorsOnly bool) ([]payloadlog.Summary, error)
	GetPayload(requestID string) (json.RawMessage, error)
}

func dashboardPayloads(source dashrpc.Source) (dashboardPayloadSource, bool) {
	ps, ok := source.(dashboardPayloadSource)
	if !ok || ps == nil {
		return nil, false
	}
	return ps, true
}

func (h *Handler) writeDashboardPayloads(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ps, ok := dashboardPayloads(deps.Dashboard)
	if !ok || !ps.PayloadEnabled() {
		_ = json.NewEncoder(w).Encode(dashrpc.PayloadList{Enabled: false})
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 {
		limit = dashrpc.PayloadListDefault
	}
	if limit > dashrpc.PayloadListMax {
		limit = dashrpc.PayloadListMax
	}
	q := r.URL.Query()
	errorsOnly := q.Get("errors_only") == "true" || q.Get("errors_only") == "1"
	entries, err := ps.ListPayloads(limit, errorsOnly)
	if err != nil {
		http.Error(w, "read payload log: "+err.Error(), http.StatusInternalServerError)
		return
	}
	if entries == nil {
		entries = []payloadlog.Summary{}
	}
	_ = json.NewEncoder(w).Encode(dashrpc.PayloadList{Enabled: true, Payloads: entries})
}

func (h *Handler) writeDashboardPayload(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ps, ok := dashboardPayloads(deps.Dashboard)
	if !ok || !ps.PayloadEnabled() {
		http.Error(w, "payload log not enabled", http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, dashrpc.PayloadPathPrefix)
	if id == "" || strings.Contains(id, "/") {
		http.Error(w, "invalid request id", http.StatusBadRequest)
		return
	}
	raw, err := ps.GetPayload(id)
	if err != nil {
		if errors.Is(err, payloadlog.ErrPayloadNotFound) {
			http.Error(w, "payload entry not found", http.StatusNotFound)
			return
		}
		http.Error(w, "read payload log: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_, _ = w.Write(append(raw, '\n'))
}

type dashboardBlockSource interface {
	BlocksEnabled() bool
	ListBlocks() []dashrpc.BlockSummary
	TakeBlock(blockID string) (dashrpc.BlockCapture, bool)
}

func dashboardBlocks(source dashrpc.Source) (dashboardBlockSource, bool) {
	bs, ok := source.(dashboardBlockSource)
	if !ok || bs == nil {
		return nil, false
	}
	return bs, true
}

func validBlockID(id string) bool {
	if !strings.HasPrefix(id, "blk_") || len(id) != len("blk_")+32 {
		return false
	}
	for _, c := range id[len("blk_"):] {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func (h *Handler) writeDashboardBlocks(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	bs, ok := dashboardBlocks(deps.Dashboard)
	if !ok || !bs.BlocksEnabled() {
		_ = json.NewEncoder(w).Encode(dashrpc.BlockList{Enabled: false})
		return
	}
	blocks := bs.ListBlocks()
	if blocks == nil {
		blocks = []dashrpc.BlockSummary{}
	}
	if len(blocks) > dashrpc.BlocksListMax {
		blocks = blocks[len(blocks)-dashrpc.BlocksListMax:]
	}
	_ = json.NewEncoder(w).Encode(dashrpc.BlockList{Enabled: true, Blocks: blocks})
}

func (h *Handler) writeDashboardBlockDecision(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if deps.Exceptions == nil {
		http.Error(w, "guardrail exceptions not enabled", http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, dashrpc.BlockPathPrefix)
	id = strings.TrimSuffix(id, dashrpc.BlockDecisionSuffix)
	if !validBlockID(id) {
		http.Error(w, "invalid block id", http.StatusBadRequest)
		return
	}
	var req dashrpc.BlockDecisionRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&req); err != nil {
		http.Error(w, "invalid decision body", http.StatusBadRequest)
		return
	}
	if req.Action != "allow" && req.Action != "redact" && req.Action != "deny" {
		http.Error(w, "invalid action (must be allow, redact, or deny)", http.StatusBadRequest)
		return
	}
	if len(req.FindingSHAs) == 0 {
		http.Error(w, "decision must list at least one secret fingerprint", http.StatusBadRequest)
		return
	}
	for _, sha := range req.FindingSHAs {
		if !validDecisionFingerprint(sha) {
			http.Error(w, "invalid secret fingerprint", http.StatusBadRequest)
			return
		}
	}
	count, err := deps.Exceptions.Decide(req.FindingSHAs, req.Action, id, nil)
	if err != nil {
		if strings.Contains(err.Error(), "is full") {
			http.Error(w, err.Error(), http.StatusTooManyRequests)
			return
		}
		http.Error(w, "record decision: "+err.Error(), http.StatusInternalServerError)
		return
	}
	_ = json.NewEncoder(w).Encode(dashrpc.BlockDecisionResponse{Ok: true, Action: req.Action, Count: count})
}

func validDecisionFingerprint(s string) bool {
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

func (h *Handler) writeDashboardBlock(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	bs, ok := dashboardBlocks(deps.Dashboard)
	if !ok || !bs.BlocksEnabled() {
		http.Error(w, "guardrail quarantine not enabled", http.StatusNotFound)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, dashrpc.BlockPathPrefix)
	if !validBlockID(id) {
		http.Error(w, "invalid block id", http.StatusBadRequest)
		return
	}
	capture, found := bs.TakeBlock(id)
	if !found {
		http.Error(w, "block not found (expired or already consumed)", http.StatusNotFound)
		return
	}
	_ = json.NewEncoder(w).Encode(capture)
}
