package app

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestAppUserDeletionRevokesPersonalKeys(t *testing.T) {
	for _, outcome := range []string{"success", "activation-failure"} {
		t.Run(outcome, func(t *testing.T) {
			a, path, cfg, request := catalogRuntimeFixture(t)
			ctx := context.Background()
			st := a.adminStore
			users := []store.User{{Email: "deleted@example.invalid"}, {Email: "retained@example.invalid"}}
			for i := range users {
				if err := st.CreateUser(ctx, &users[i]); err != nil {
					t.Fatal(err)
				}
				if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: users[i].ID, WorkspaceID: store.SystemWorkspaceID}); err != nil {
					t.Fatal(err)
				}
			}
			team := store.WorkspaceTeam{Name: "retained", WorkspaceID: store.SystemWorkspaceID}
			if err := st.CreateTeam(ctx, &team); err != nil {
				t.Fatal(err)
			}
			for i := range users {
				if err := st.AddTeamMember(ctx, users[i].ID, team.ID, "admin"); err != nil {
					t.Fatal(err)
				}
			}
			keys := []store.InboundKey{
				{Name: "personal-one", OwnerUserID: &users[0].ID},
				{Name: "personal-two", OwnerUserID: &users[0].ID},
				{Name: "team", OwnerTeamID: &team.ID},
				{Name: "other-user", OwnerUserID: &users[1].ID},
			}
			entries := make([]store.SpendEntry, len(keys))
			for i := range keys {
				k := &keys[i]
				k.TokenHash, k.Enabled, k.WorkspaceID = store.TokenHash(k.Name+"-fixture"), true, store.SystemWorkspaceID
				k.AllowedModels = []string{"local/old"}
				if err := st.CreateInboundKey(ctx, k); err != nil {
					t.Fatal(err)
				}
				if err := st.SetKeyBindings(ctx, k.ID, []uuid.UUID{users[0].ID, users[1].ID}, []uuid.UUID{team.ID}); err != nil {
					t.Fatal(err)
				}
				if err := st.DB.NewSelect().Model(k).Where("id = ?", k.ID).Scan(ctx); err != nil {
					t.Fatal(err)
				}
				entries[i] = store.SpendEntry{WorkspaceID: k.WorkspaceID, KeyID: k.ID, UserID: k.OwnerUserID, TeamID: k.OwnerTeamID, Model: "local/old", Tokens: 17, CostMicros: int64(123 + i)}
				if _, err := st.DB.NewInsert().Model(&entries[i]).Returning("*").Exec(ctx); err != nil {
					t.Fatal(err)
				}
			}
			gate, started := make(chan struct{}), make(chan struct{}, 1)
			release := sync.OnceFunc(func() { close(gate) })
			var calls atomic.Int32
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(r.Body)
				if strings.Contains(string(body), "hold") {
					started <- struct{}{}
					select {
					case <-gate:
					case <-r.Context().Done():
						return
					}
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"model":"old","choices":[],"usage":{"prompt_tokens":2,"completion_tokens":3,"total_tokens":5}}`)
			}))
			defer upstream.Close()
			defer release()
			local, _ := a.Config.Catalog.Provider("local")
			cfg = strings.Replace(cfg, local.BaseURL, upstream.URL, 1)
			rewriteConfigFile(t, path, cfg)
			if err := a.Reload(); err != nil {
				t.Fatal(err)
			}
			chat := func(key store.InboundKey, body string) *httptest.ResponseRecorder {
				r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(body))
				r.Header.Set("Authorization", "Bearer "+key.Name+"-fixture")
				w := httptest.NewRecorder()
				a.Server.Handler.ServeHTTP(w, r)
				return w
			}
			ordinary := `{"model":"local/old","messages":[]}`
			reloads := 0
			deps := a.handler.SnapshotDependencies()
			deps.RequestReload = func() error { reloads++; return a.Reload() }
			a.handler.UpdateDependencies(deps)
			w := request(http.MethodDelete, "/_internal/admin/workspaces/"+store.SystemWorkspaceID.String()+"/members/"+users[0].ID.String(), "")
			if w.Code != 200 || reloads != 0 || chat(keys[0], ordinary).Code != 200 {
				t.Fatalf("membership removal redefined bearer access: %d %s reloads=%d", w.Code, w.Body.String(), reloads)
			}
			if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: users[0].ID, WorkspaceID: store.SystemWorkspaceID}); err != nil {
				t.Fatal(err)
			}
			if err := st.AddTeamMember(ctx, users[0].ID, team.ID, "member"); err != nil {
				t.Fatal(err)
			}
			admitted := make(chan *httptest.ResponseRecorder, 1)
			go func() {
				admitted <- chat(keys[0], `{"model":"local/old","messages":[{"role":"user","content":"hold"}]}`)
			}()
			select {
			case <-started:
			case <-time.After(10 * time.Second):
				t.Fatal("request never admitted")
			}
			before := a.Config
			if outcome == "activation-failure" {
				rewriteConfigFile(t, path, strings.Replace(cfg, `address = "127.0.0.1:0"`, `address = "127.0.0.1:1"`, 1))
			}
			w = request(http.MethodDelete, "/_internal/admin/users/"+users[0].ID.String(), "")
			want := 200
			if outcome == "activation-failure" {
				want = 500
			}
			if w.Code != want || reloads != 1 || w.Header().Get("X-Aiproxy-User-Deleted") != "true" || w.Header().Get("X-Aiproxy-User-ID") != users[0].ID.String() {
				t.Fatalf("delete=%d %s reloads=%d", w.Code, w.Body.String(), reloads)
			}
			if _, err := st.GetUserByID(ctx, users[0].ID); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("account not deleted", err)
			}
			if outcome == "activation-failure" {
				if a.Config != before || chat(keys[0], ordinary).Code != 401 || w.Body.String() != "saved but activation failed\n" {
					t.Fatal("catalog failure did not retain catalog, independently revoke keys, or distinguish commit")
				}
				rewriteConfigFile(t, path, cfg)
				if err := a.Reload(); err != nil {
					t.Fatal(err)
				}
			}
			for _, key := range keys[:2] {
				priorCalls := calls.Load()
				if w := chat(key, ordinary); w.Code != 401 || calls.Load() != priorCalls {
					t.Fatal("deleted personal bearer reached upstream", w.Code)
				}
			}
			release()
			select {
			case completed := <-admitted:
				if completed.Code != 200 {
					t.Fatal("deletion cancelled admitted work", completed.Code, completed.Body.String())
				}
			case <-time.After(10 * time.Second):
				t.Fatal("admitted work did not complete")
			}
			for _, key := range keys[2:] {
				var saved store.InboundKey
				if err := st.DB.NewSelect().Model(&saved).Where("id = ?", key.ID).Scan(ctx); err != nil || !reflect.DeepEqual(saved, key) {
					t.Fatal("unrelated key changed", err)
				}
				r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
				r.Header.Set("Authorization", "Bearer "+key.Name+"-fixture")
				p, err := a.handler.SnapshotDependencies().Auth.Authenticate(r)
				if err != nil || p.CredentialID() != identity.DatabaseKeyID(key.ID) || p.WorkspaceScope() != identity.InWorkspace(key.WorkspaceID) || chat(key, ordinary).Code != 200 {
					t.Fatal("unrelated bearer identity/scope or serving changed", err)
				}
				teams, err := st.ListKeyTeamIDs(ctx, key.ID)
				if err != nil || !reflect.DeepEqual(teams, []uuid.UUID{team.ID}) {
					t.Fatal("surviving team share changed", teams, err)
				}
				shares, err := st.ListKeyUserIDs(ctx, key.ID)
				if err != nil || !reflect.DeepEqual(shares, []uuid.UUID{users[1].ID}) {
					t.Fatal("surviving user share changed", shares, err)
				}
			}
			for i, entry := range entries {
				var saved store.SpendEntry
				if err := st.DB.NewSelect().Model(&saved).Where("id = ?", entry.ID).Scan(ctx); err != nil {
					t.Fatal(err)
				}
				if i < 2 {
					entry.UserID = nil
				}
				if !reflect.DeepEqual(saved, entry) {
					t.Fatalf("spend history changed beyond deleted user FK: %+v != %+v", saved, entry)
				}
			}
			dependencyRuntimeRestart(t, path, func(restarted *App) {
				for i, key := range keys {
					r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
					r.Header.Set("Authorization", "Bearer "+key.Name+"-fixture")
					w := httptest.NewRecorder()
					restarted.Server.Handler.ServeHTTP(w, r)
					want := 200
					if i < 2 {
						want = 401
					}
					if w.Code != want {
						t.Fatal("restart lost deletion/survivor state", w.Code)
					}
				}
			})
		})
	}
}
