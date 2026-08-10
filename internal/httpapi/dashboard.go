package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/egose/aiproxy/internal/dashrpc"
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
	return got[len(dashrpc.AuthScheme):] == source.Token()
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
