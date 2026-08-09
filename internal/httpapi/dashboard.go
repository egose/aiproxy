package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/egose/aiproxy/internal/dashrpc"
)

const dashboardRecentN = 200

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
			http.Error(w, "unauthorized", http.StatusUnauthorized)
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
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return true
		}
		h.writeDashboardLogs(deps, w, r)
		return true
	}
	return false
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
