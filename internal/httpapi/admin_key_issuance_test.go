package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

func keyIssuanceHTTPFixture(t *testing.T) (*store.Store, *Handler, string, store.Workspace, store.User, store.WorkspaceTeam) {
	t.Helper()
	st, h, access := openUsersTestStore(t)
	ctx := context.Background()
	workspace := store.Workspace{Name: uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
	u, _ := membershipHTTPUser(t, st)
	if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: u.ID, WorkspaceID: workspace.ID}); err != nil {
		t.Fatal(err)
	}
	team := store.WorkspaceTeam{WorkspaceID: workspace.ID, Name: "local"}
	if err := st.CreateTeam(ctx, &team); err != nil {
		t.Fatal(err)
	}
	return st, h, access, workspace, u, team
}

func keyIssuanceHTTPBody(name string, workspace store.Workspace, user store.User, team store.WorkspaceTeam) string {
	return `{"name":"` + name + `","workspace_id":"` + workspace.ID.String() + `","owner_type":"user","owner_id":"` + user.ID.String() + `","user_ids":["` + user.ID.String() + `","` + user.ID.String() + `"],"team_ids":["` + team.ID.String() + `","` + team.ID.String() + `"]}`
}

func TestKeyIssuanceHTTPRollback(t *testing.T) {
	for _, table := range []string{"inbound_keys", "key_users", "key_teams"} {
		t.Run(table, func(t *testing.T) {
			st, h, access, workspace, user, team := keyIssuanceHTTPFixture(t)
			ctx := context.Background()
			count := func(model interface{}) int {
				t.Helper()
				n, err := st.DB.NewSelect().Model(model).Count(ctx)
				if err != nil {
					t.Fatal(err)
				}
				return n
			}
			before := []int{count((*store.InboundKey)(nil)), count((*store.KeyUser)(nil)), count((*store.KeyTeam)(nil))}
			if _, err := st.DB.ExecContext(ctx, "ALTER TABLE "+table+" ADD CONSTRAINT issuance_http_fault CHECK (false) NOT VALID"); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				_, _ = st.DB.ExecContext(ctx, "ALTER TABLE "+table+" DROP CONSTRAINT IF EXISTS issuance_http_fault")
			})
			activations := 0
			deps := h.current()
			deps.RequestReload = func() error { activations++; return nil }
			h.UpdateDependencies(deps)
			w := membershipRequest(ctx, h, access, http.MethodPost, "/_internal/admin/keys", keyIssuanceHTTPBody(uuid.NewString(), workspace, user, team))
			after := []int{count((*store.InboundKey)(nil)), count((*store.KeyUser)(nil)), count((*store.KeyTeam)(nil))}
			if w.Code != 500 || w.Header().Get("X-Aiproxy-Key-Saved") != "" || w.Header().Get("X-Aiproxy-Key-ID") != "" || strings.Contains(w.Body.String(), "issuance_http_fault") || !reflect.DeepEqual(before, after) || activations != 0 {
				t.Fatalf("rollback=%d %s before=%v after=%v reloads=%d", w.Code, w.Body.String(), before, after, activations)
			}
		})
	}
}

type keyIssuancePresentationHook struct{ fail atomic.Bool }

func (h *keyIssuancePresentationHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	if h.fail.Load() && strings.Contains(event.Query, `FROM "key_users"`) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		return ctx
	}
	return ctx
}

func (*keyIssuancePresentationHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestKeyIssuanceHTTPCommitAndRecovery(t *testing.T) {
	for _, outcome := range []string{"success", "activation-failure", "presentation-failure"} {
		t.Run(outcome, func(t *testing.T) {
			st, h, access, workspace, user, team := keyIssuanceHTTPFixture(t)
			ctx := context.Background()
			name := uuid.NewString()
			body := keyIssuanceHTTPBody(name, workspace, user, team)
			hook := &keyIssuancePresentationHook{}
			st.DB.AddQueryHook(hook)
			activations := 0
			deps := h.current()
			deps.RequestReload = func() error {
				activations++
				var key store.InboundKey
				if err := st.DB.NewSelect().Model(&key).Where("name = ?", name).Scan(ctx); err != nil {
					t.Fatal(err)
				}
				users, err := st.ListKeyUserIDs(ctx, key.ID)
				if err != nil {
					t.Fatal(err)
				}
				teams, err := st.ListKeyTeamIDs(ctx, key.ID)
				if err != nil {
					t.Fatal(err)
				}
				if key.OwnerUserID == nil || *key.OwnerUserID != user.ID || key.OwnerTeamID != nil || key.WorkspaceID != workspace.ID || !reflect.DeepEqual(users, []uuid.UUID{user.ID}) || !reflect.DeepEqual(teams, []uuid.UUID{team.ID}) {
					t.Fatal("reload observed incomplete aggregate")
				}
				if outcome == "activation-failure" {
					return errors.New("private fixture failure")
				}
				hook.fail.Store(outcome == "presentation-failure")
				return nil
			}
			h.UpdateDependencies(deps)
			w := membershipRequest(ctx, h, access, http.MethodPost, "/_internal/admin/keys", body)
			hook.fail.Store(false)
			id, err := uuid.Parse(w.Header().Get("X-Aiproxy-Key-ID"))
			if err != nil || w.Header().Get("X-Aiproxy-Key-Saved") != "true" || w.Header().Get("Location") != "/_internal/admin/keys/"+id.String() || w.Header().Get("Cache-Control") != "no-store" || activations != 1 {
				t.Fatalf("missing saved identity: %d %s headers=%v reloads=%d", w.Code, w.Body.String(), w.Header(), activations)
			}
			var saved store.InboundKey
			if err := st.DB.NewSelect().Model(&saved).Where("id = ?", id).Scan(ctx); err != nil {
				t.Fatal(err)
			}
			if outcome == "success" {
				var result struct {
					Key   adminKeyView `json:"key"`
					Token string       `json:"token"`
				}
				if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
					t.Fatal(err)
				}
				if w.Code != 201 || result.Key.ID != id.String() || result.Token == "" || saved.TokenHash != store.TokenHash(result.Token) || saved.TokenHash == result.Token {
					t.Fatal("wrong one-time bearer response")
				}
			} else if w.Code != 500 || !strings.Contains(w.Body.String(), "saved but") || !strings.Contains(w.Body.String(), id.String()) || !strings.Contains(w.Body.String(), "rotate") || !strings.Contains(w.Body.String(), "do not repeat creation") || strings.Contains(w.Body.String(), "private fixture failure") || strings.Contains(w.Body.String(), "sk-admin-") {
				t.Fatalf("ambiguous failure: %d %s", w.Code, w.Body.String())
			}
			deps.RequestReload = func() error { activations++; return nil }
			h.UpdateDependencies(deps)
			duplicate := membershipRequest(ctx, h, access, http.MethodPost, "/_internal/admin/keys", body)
			if duplicate.Code != 409 || activations != 1 {
				t.Fatalf("duplicate=%d %s reloads=%d", duplicate.Code, duplicate.Body.String(), activations)
			}
			detail := membershipRequest(ctx, h, access, http.MethodGet, w.Header().Get("Location"), "")
			if detail.Code != 200 || strings.Contains(detail.Body.String(), saved.TokenHash) || strings.Contains(detail.Body.String(), `"token":`) {
				t.Fatalf("detail=%d %s", detail.Code, detail.Body.String())
			}
			deps.RequestReload = func() error { activations++; return errors.New("private rotation failure") }
			h.UpdateDependencies(deps)
			rotation := membershipRequest(ctx, h, access, http.MethodPost, w.Header().Get("Location")+"/rotate", "")
			if rotation.Code != 500 || rotation.Header().Get("X-Aiproxy-Key-ID") != id.String() || !strings.Contains(rotation.Body.String(), "rotate") || strings.Contains(rotation.Body.String(), "private rotation failure") || activations != 2 {
				t.Fatalf("rotation failure=%d %s", rotation.Code, rotation.Body.String())
			}
			deps.RequestReload = func() error { activations++; return nil }
			h.UpdateDependencies(deps)
			rotation = membershipRequest(ctx, h, access, http.MethodPost, w.Header().Get("Location")+"/rotate", "")
			var result struct {
				Token string `json:"token"`
			}
			if err := json.Unmarshal(rotation.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			var recovered store.InboundKey
			if err := st.DB.NewSelect().Model(&recovered).Where("id = ?", id).Scan(ctx); err != nil {
				t.Fatal(err)
			}
			if rotation.Code != 200 || result.Token == "" || recovered.TokenHash != store.TokenHash(result.Token) || recovered.TokenHash == saved.TokenHash || recovered.ID != saved.ID || recovered.WorkspaceID != saved.WorkspaceID || !reflect.DeepEqual(recovered.OwnerUserID, saved.OwnerUserID) || activations != 3 {
				t.Fatalf("recovery failed: %d %+v", rotation.Code, recovered)
			}
		})
	}
}

func TestKeyIssuanceHTTPConcurrentMembershipChange(t *testing.T) {
	for _, mutation := range []string{"recipient-offboard", "actor-offboard", "actor-demote", "team-delete"} {
		t.Run(mutation, func(t *testing.T) {
			st, h, _, workspace, user, team := keyIssuanceHTTPFixture(t)
			actor, access := membershipHTTPUser(t, st)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: actor.ID, WorkspaceID: workspace.ID}); err != nil {
				t.Fatal(err)
			}
			if err := st.AddTeamMember(ctx, actor.ID, team.ID, "admin"); err != nil {
				t.Fatal(err)
			}
			holder, err := st.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer holder.Rollback()
			var locked store.Workspace
			if err := holder.NewSelect().Model(&locked).Where("id = ?", workspace.ID).For("NO KEY UPDATE").Scan(ctx); err != nil {
				t.Fatal(err)
			}
			var activations atomic.Int32
			deps := h.current()
			deps.RequestReload = func() error { activations.Add(1); return nil }
			h.UpdateDependencies(deps)
			name := uuid.NewString()
			body := `{"name":"` + name + `","workspace_id":"` + workspace.ID.String() + `","owner_type":"team","owner_id":"` + team.ID.String() + `","user_ids":["` + user.ID.String() + `"]}`
			result := make(chan *httptest.ResponseRecorder, 1)
			go func() { result <- membershipRequest(ctx, h, access, http.MethodPost, "/_internal/admin/keys", body) }()
			waitKeyIssuanceHTTPLock(t, ctx, st, workspace.ID)
			switch mutation {
			case "recipient-offboard", "actor-offboard":
				id := user.ID
				if mutation == "actor-offboard" {
					id = actor.ID
				}
				_, err = holder.NewDelete().Model((*store.WorkspaceMember)(nil)).Where("user_id = ? AND workspace_id = ?", id, workspace.ID).Exec(ctx)
			case "actor-demote":
				_, err = holder.NewUpdate().Model((*store.TeamMember)(nil)).Set("role = 'member'").Where("user_id = ? AND team_id = ?", actor.ID, team.ID).Exec(ctx)
			case "team-delete":
				_, err = holder.NewDelete().Model((*store.WorkspaceTeam)(nil)).Where("id = ?", team.ID).Exec(ctx)
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := holder.Commit(); err != nil {
				t.Fatal(err)
			}
			w := <-result
			n, err := st.DB.NewSelect().Model((*store.InboundKey)(nil)).Where("name = ?", name).Count(ctx)
			if err != nil || n != 0 || (w.Code != 400 && w.Code != 403) || w.Header().Get("X-Aiproxy-Key-Saved") != "" || activations.Load() != 0 {
				t.Fatalf("stale issuance=%d %s rows=%d err=%v reloads=%d", w.Code, w.Body.String(), n, err, activations.Load())
			}
		})
	}
}

func waitKeyIssuanceHTTPLock(t *testing.T, ctx context.Context, st *store.Store, workspace uuid.UUID) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var n int
		if err := st.DB.NewRaw(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
			AND cardinality(pg_blocking_pids(pid)) > 0 AND query LIKE ?`, "%"+workspace.String()+"%").Scan(ctx, &n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal("issuance did not wait for workspace lock")
		case <-ticker.C:
		}
	}
}
