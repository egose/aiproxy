package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/dashrpc"
	"github.com/egose/aiproxy/internal/guardrails"
	"github.com/egose/aiproxy/internal/payloadlog"
	"github.com/egose/aiproxy/internal/store"
)

type dashboardReadCounter struct {
	*dashrpc.RuntimeSource
	reads int
}

func (s *dashboardReadCounter) Snapshot(ctx context.Context, n int) dashrpc.Snapshot {
	s.reads++
	return s.RuntimeSource.Snapshot(ctx, n)
}

func (s *dashboardReadCounter) Logs(since uint64) dashrpc.Logs {
	s.reads++
	return s.RuntimeSource.Logs(since)
}

func (s *dashboardReadCounter) ListPayloads(limit int, errorsOnly bool) ([]payloadlog.Summary, error) {
	s.reads++
	return s.RuntimeSource.ListPayloads(limit, errorsOnly)
}

func (s *dashboardReadCounter) GetPayload(id string) (json.RawMessage, error) {
	s.reads++
	return s.RuntimeSource.GetPayload(id)
}

func (s *dashboardReadCounter) ListBlocks() []dashrpc.BlockSummary {
	s.reads++
	return s.RuntimeSource.ListBlocks()
}

func (s *dashboardReadCounter) TakeBlock(id string) (dashrpc.BlockCapture, bool) {
	s.reads++
	return s.RuntimeSource.TakeBlock(id)
}

func TestDashboardOperatorRoutes(t *testing.T) {
	st, adminHandler, systemToken := openUsersTestStore(t)
	ctx := context.Background()
	workspaceA := mkWorkspace(t, st, adminHandler, systemToken, "dashboard-a")
	workspaceB := mkWorkspace(t, st, adminHandler, systemToken, "dashboard-b")
	type actor struct {
		name, token string
		status      int
	}
	actors := []actor{{"dashboard-secret", dashboardTestToken, http.StatusOK}, {"system-admin", systemToken, http.StatusOK}}
	for _, workspace := range []string{workspaceA, workspaceB} {
		for _, role := range []string{roleMember, roleAdmin} {
			name := role + "-" + workspace
			_, token := mkWorkspaceMember(t, st, adminHandler, systemToken, name+"@example.com", workspace, role)
			actors = append(actors, actor{name, token, http.StatusForbidden})
		}
	}
	for _, state := range []string{"demoted", "disabled", "deleted", "promoted"} {
		u := &store.User{Email: fmt.Sprintf("dashboard-%s-%s@example.com", state, workspaceA), IsAdmin: state != "promoted"}
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = st.DeleteUser(context.Background(), u.ID) })
		token, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, u.IsAdmin)
		if err != nil {
			t.Fatal(err)
		}
		status := http.StatusUnauthorized
		switch state {
		case "demoted":
			u.IsAdmin = false
			status = http.StatusForbidden
		case "disabled":
			u.Disabled = true
		case "promoted":
			u.IsAdmin = true
			status = http.StatusOK
		}
		if state == "deleted" {
			err = st.DeleteUser(ctx, u.ID)
		} else {
			err = st.UpdateUser(ctx, u)
		}
		if err != nil {
			t.Fatal(err)
		}
		actors = append(actors, actor{state, token, status})
		code, me := callAdmin(t, adminHandler, token, http.MethodGet, adminPathPrefix+"me", nil)
		if status == http.StatusUnauthorized {
			if code != http.StatusUnauthorized {
				t.Fatalf("%s /me status = %d", state, code)
			}
		} else if code != http.StatusOK || me["is_admin"] != u.IsAdmin {
			t.Fatalf("%s /me must reflect stored authority: %d %v", state, code, me)
		}
		code, _ = callAdmin(t, adminHandler, token, http.MethodGet, adminPathPrefix+"users", nil)
		if code != status {
			t.Fatalf("%s admin users status = %d, want %d", state, code, status)
		}
	}
	actors = append(actors, actor{"invalid-token", "garbage", http.StatusUnauthorized}, actor{"missing-token", "", http.StatusUnauthorized})
	const blockID = "blk_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	const secret = "captured-other-workspace-secret"
	sha := guardrails.Fingerprint(secret)
	routes := []struct{ method, path string }{
		{http.MethodGet, dashrpc.SnapshotPath},
		{http.MethodGet, dashrpc.LogsPath},
		{http.MethodGet, dashrpc.PayloadsPath},
		{http.MethodGet, dashrpc.PayloadPathPrefix + "req-ok"},
		{http.MethodGet, dashrpc.BlocksPath},
		{http.MethodGet, dashrpc.BlockPathPrefix + blockID},
		{http.MethodPost, dashrpc.BlockDecisionPath(blockID)},
	}
	for _, a := range actors {
		t.Run(a.name, func(t *testing.T) {
			for _, route := range routes {
				t.Run(route.method+route.path, func(t *testing.T) {
					dir := t.TempDir()
					seedDashboardPayload(t, dir)
					deps := newPayloadDeps(t, dir, true)
					deps.AdminStore = st
					q := guardrails.NewQuarantine(guardrails.QuarantinePolicy{Enabled: true})
					q.Store(blockID, guardrails.Capture{Findings: []guardrails.CapturedFinding{{Secret: secret}}})
					source := &dashboardReadCounter{RuntimeSource: deps.Dashboard.(*dashrpc.RuntimeSource)}
					source.SetBlockSource(quarantineTestAdapter{q})
					deps.Dashboard = source
					exc := exceptionTestStore(t)
					if _, err := exc.Decide([]string{sha}, "deny", blockID, nil); err != nil {
						t.Fatal(err)
					}
					before, err := os.ReadFile(exc.Path())
					if err != nil {
						t.Fatal(err)
					}
					deps.Exceptions = exc
					h := NewHandler(deps)
					r := httptest.NewRequest(route.method, route.path, strings.NewReader(`{"action":"allow","finding_shas":["`+sha+`"]}`))
					if a.token != "" {
						r.Header.Set("Authorization", "Bearer "+a.token)
					}
					r.Header.Set("X-Workspace-ID", workspaceB)
					w := httptest.NewRecorder()
					h.ServeHTTP(w, r)
					if w.Code != a.status {
						t.Errorf("status = %d, want %d: %s", w.Code, a.status, w.Body.String())
					}
					if a.status != http.StatusOK {
						if source.reads != 0 {
							t.Errorf("denied request performed %d dashboard reads", source.reads)
						}
						if strings.Contains(w.Body.String(), secret) || strings.Contains(w.Body.String(), "req-ok") {
							t.Error("denied request exposed global data")
						}
						capture, ok := q.Take(blockID)
						if !ok || len(capture.Findings) != 1 || capture.Findings[0].Secret != secret {
							t.Error("denied request consumed capture")
						}
						after, err := os.ReadFile(exc.Path())
						if err != nil || !bytes.Equal(before, after) {
							t.Errorf("denied request changed persistent decisions: %v", err)
						}
						if decision, ok := exc.Lookup(sha); !ok || decision.Action != "deny" {
							t.Error("denied request changed in-memory decision")
						}
					} else if route.method == http.MethodPost {
						reloaded, err := guardrails.LoadExceptions(exc.Path(), "")
						if err != nil {
							t.Fatal(err)
						}
						if decision, ok := reloaded.Lookup(sha); !ok || decision.Action != "allow" {
							t.Error("operator decision was not persisted")
						}
					} else if route.path == dashrpc.BlockPathPrefix+blockID {
						if !strings.Contains(w.Body.String(), secret) {
							t.Error("operator did not receive capture")
						}
						if _, ok := q.Take(blockID); ok {
							t.Error("operator capture read did not consume take-once entry")
						}
					} else if source.reads != 1 {
						t.Errorf("operator dashboard reads = %d, want 1", source.reads)
					}
				})
			}
		})
	}
}
