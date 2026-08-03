package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/observability"
)

const dashboardRecentN = 200

// handleDashboard routes /_internal/dashboard/* requests. Returns true if
// the request was handled. When the dashboard block is unconfigured, every
// path under /_internal/* returns 404 so the surface stays closed.
func (h *Handler) handleDashboard(deps Dependencies, w http.ResponseWriter, r *http.Request) bool {
	if deps.Dashboard == (config.Dashboard{}) {
		return false
	}
	if r.URL.Path == dashrpc.SnapshotPath {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !dashboardAuthorized(deps.Dashboard, r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return true
		}
		h.writeDashboardSnapshot(deps, w)
		return true
	}
	if r.URL.Path == dashrpc.LogsPath {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return true
		}
		if !dashboardAuthorized(deps.Dashboard, r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return true
		}
		h.writeDashboardLogs(deps, w, r)
		return true
	}
	return false
}

func dashboardAuthorized(c config.Dashboard, r *http.Request) bool {
	if c.Token == "" {
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
	return got[len(dashrpc.AuthScheme):] == c.Token
}

type snapshotResponse struct {
	dashrpc.Snapshot
	LastSeq uint64 `json:"last_seq"`
}

func (h *Handler) writeDashboardSnapshot(deps Dependencies, w http.ResponseWriter) {
	recentN := dashboardRecentN
	snap := dashrpc.Build(
		deps.DashboardVersion, deps.DashboardAddress, deps.DashboardAuthMode,
		deps.DashboardStartTime,
		deps.DashboardProviders, deps.DashboardDisabled, deps.DashboardAliases,
		usageAggregator(deps.Usage), deps.Health, deps.Logs, recentN,
	)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(snapshotResponse{Snapshot: snap, LastSeq: snap.LastSeq})
}

type logsResponse struct {
	Logs    []observability.LogEntry `json:"logs"`
	LastSeq uint64                   `json:"last_seq"`
}

func (h *Handler) writeDashboardLogs(deps Dependencies, w http.ResponseWriter, r *http.Request) {
	if deps.Logs == nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(logsResponse{})
		return
	}
	since, _ := strconv.ParseUint(r.URL.Query().Get("since"), 10, 64)
	entries, lastSeq := deps.Logs.SinceSeq(since)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(logsResponse{Logs: entries, LastSeq: lastSeq})
}

// usageAggregator extracts the *accounting.Aggregator backing the Reader
// interface, if present. The dashboard surfaces recent events (which the
// Reader interface itself does not expose).
func usageAggregator(r accounting.Reader) *accounting.Aggregator {
	if r == nil {
		return nil
	}
	if a, ok := r.(*accounting.Aggregator); ok {
		return a
	}
	return nil
}
