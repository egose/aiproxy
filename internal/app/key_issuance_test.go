package app

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestAppKeyIssuanceRollbackAndRotationRecovery(t *testing.T) {
	for _, outcome := range []string{"success", "activation-failure"} {
		t.Run(outcome, func(t *testing.T) {
			a, path, _, request := catalogRuntimeFixture(t)
			ctx := context.Background()
			st := a.adminStore
			member := store.User{Email: "key-owner@example.invalid"}
			if err := st.CreateUser(ctx, &member); err != nil {
				t.Fatal(err)
			}
			if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: member.ID, WorkspaceID: store.SystemWorkspaceID}); err != nil {
				t.Fatal(err)
			}
			team := store.WorkspaceTeam{Name: "issuance", WorkspaceID: store.SystemWorkspaceID}
			if err := st.CreateTeam(ctx, &team); err != nil {
				t.Fatal(err)
			}
			body := `{"name":"issued","workspace_id":"` + store.SystemWorkspaceID.String() + `","owner_type":"user","owner_id":"` + member.ID.String() + `","allowed_models":["local/old"],"team_ids":["` + team.ID.String() + `"]}`
			reloads := 0
			deps := a.handler.SnapshotDependencies()
			deps.RequestReload = func() error { reloads++; return a.Reload() }
			a.handler.UpdateDependencies(deps)
			before := a.Config
			if _, err := st.DB.ExecContext(ctx, "ALTER TABLE key_teams ADD CONSTRAINT issuance_fault CHECK (false) NOT VALID"); err != nil {
				t.Fatal(err)
			}
			w := request(http.MethodPost, "/_internal/admin/keys", body)
			rows, err := st.ListInboundKeys(ctx)
			if err != nil || len(rows) != 0 || w.Code != 500 || reloads != 0 || a.Config != before {
				t.Fatalf("rollback=%d %s rows=%v err=%v reloads=%d", w.Code, w.Body.String(), rows, err, reloads)
			}
			if _, err := st.DB.ExecContext(ctx, "ALTER TABLE key_teams DROP CONSTRAINT issuance_fault"); err != nil {
				t.Fatal(err)
			}
			if outcome == "activation-failure" {
				deps.RequestReload = func() error { reloads++; return errors.New("fixture activation failed") }
				a.handler.UpdateDependencies(deps)
			}
			w = request(http.MethodPost, "/_internal/admin/keys", body)
			id, err := uuid.Parse(w.Header().Get("X-Aiproxy-Key-ID"))
			if err != nil || reloads != 1 || w.Header().Get("X-Aiproxy-Key-Saved") != "true" {
				t.Fatalf("commit=%d %s reloads=%d", w.Code, w.Body.String(), reloads)
			}
			var saved store.InboundKey
			if err := st.DB.NewSelect().Model(&saved).Where("id = ?", id).Scan(ctx); err != nil {
				t.Fatal(err)
			}
			users, err := st.ListKeyUserIDs(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			teams, err := st.ListKeyTeamIDs(ctx, id)
			if err != nil || !reflect.DeepEqual(users, []uuid.UUID{member.ID}) || !reflect.DeepEqual(teams, []uuid.UUID{team.ID}) || saved.OwnerUserID == nil || *saved.OwnerUserID != member.ID || saved.WorkspaceID != store.SystemWorkspaceID {
				t.Fatal("wrong committed ownership/grants")
			}
			var issued struct {
				Token string `json:"token"`
			}
			if outcome == "success" {
				if w.Code != 201 || json.Unmarshal(w.Body.Bytes(), &issued) != nil || issued.Token == "" {
					t.Fatal("successful issuance missing bearer")
				}
			} else if w.Code != 500 || a.Config != before || !strings.Contains(w.Body.String(), "rotate") {
				t.Fatal("activation failure changed runtime or omitted recovery")
			}
			chat := func(app *App, token string, status int) {
				t.Helper()
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"local/old","messages":[]}`))
				r.Header.Set("Authorization", "Bearer "+token)
				w := httptest.NewRecorder()
				app.Server.Handler.ServeHTTP(w, r)
				if w.Code != status {
					t.Fatalf("inference=%d %s want=%d", w.Code, w.Body.String(), status)
				}
			}
			if issued.Token != "" {
				chat(a, issued.Token, 200)
			}
			deps = a.handler.SnapshotDependencies()
			deps.RequestReload = func() error { reloads++; return a.Reload() }
			a.handler.UpdateDependencies(deps)
			rotation := request(http.MethodPost, "/_internal/admin/keys/"+id.String()+"/rotate", "")
			var recovered struct {
				Token string `json:"token"`
			}
			if rotation.Code != 200 || json.Unmarshal(rotation.Body.Bytes(), &recovered) != nil || recovered.Token == "" || reloads != 2 {
				t.Fatal("rotation recovery failed")
			}
			rows, err = st.ListInboundKeys(ctx)
			if err != nil || len(rows) != 1 || rows[0].ID != id || rows[0].WorkspaceID != saved.WorkspaceID || rows[0].TokenHash != store.TokenHash(recovered.Token) || rows[0].TokenHash == saved.TokenHash {
				t.Fatal("rotation changed identity or created duplicate")
			}
			chat(a, recovered.Token, 200)
			if issued.Token != "" {
				chat(a, issued.Token, 401)
			}
			dependencyRuntimeRestart(t, path, func(restarted *App) { chat(restarted, recovered.Token, 200) })
		})
	}
}
