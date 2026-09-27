package httpapi

import (
	"context"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestKeyPolicyAtomicHTTP(t *testing.T) {
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
	foreign := store.WorkspaceTeam{WorkspaceID: store.SystemWorkspaceID, Name: uuid.NewString()}
	if err := st.CreateTeam(ctx, &foreign); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteTeam(ctx, foreign.ID) })
	k := store.InboundKey{Name: uuid.NewString(), WorkspaceID: workspace.ID, TokenHash: uuid.NewString(), Enabled: true, Description: "original", Tenant: "original", AllowedModels: []string{"original/model"}}
	if err := st.CreateInboundKey(ctx, &k); err != nil {
		t.Fatal(err)
	}
	if err := st.SetKeyBindings(ctx, k.ID, []uuid.UUID{u.ID}, []uuid.UUID{team.ID}); err != nil {
		t.Fatal(err)
	}
	var before store.InboundKey
	if err := st.DB.NewSelect().Model(&before).Where("id = ?", k.ID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	activations := 0
	deps := h.current()
	deps.RequestReload = func() error { activations++; return nil }
	h.UpdateDependencies(deps)
	path := "/_internal/admin/keys/" + k.ID.String()
	for _, bindings := range []string{
		`"user_ids":["invalid"]`, `"team_ids":["invalid"]`,
		`"user_ids":["` + uuid.NewString() + `"]`,
		`"team_ids":["` + foreign.ID.String() + `"]`,
	} {
		w := membershipRequest(ctx, h, access, http.MethodPut, path, `{"description":"rejected","tenant":"rejected","allowed_models":[],"expires_at":"2000-01-01T00:00:00Z",`+bindings+`}`)
		if w.Code != http.StatusBadRequest {
			t.Fatalf("rejection = %d %s", w.Code, w.Body.String())
		}
		var after store.InboundKey
		if err := st.DB.NewSelect().Model(&after).Where("id = ?", k.ID).Scan(ctx); err != nil {
			t.Fatal(err)
		}
		users, err := st.ListKeyUserIDs(ctx, k.ID)
		if err != nil {
			t.Fatal(err)
		}
		teams, err := st.ListKeyTeamIDs(ctx, k.ID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(users, []uuid.UUID{u.ID}) || !reflect.DeepEqual(teams, []uuid.UUID{team.ID}) || activations != 0 {
			t.Fatalf("rejected edit changed state: before=%+v after=%+v users=%v teams=%v activations=%d", before, after, users, teams, activations)
		}
	}
	constraint := "life04_" + strings.ReplaceAll(k.ID.String(), "-", "")
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE key_teams ADD CONSTRAINT "+constraint+" CHECK (key_id <> '"+k.ID.String()+"') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = st.DB.ExecContext(ctx, "ALTER TABLE key_teams DROP CONSTRAINT IF EXISTS "+constraint) })
	failure := membershipRequest(ctx, h, access, http.MethodPut, path, `{"description":"storage rejected","user_ids":[],"team_ids":["`+team.ID.String()+`"]}`)
	if failure.Code != http.StatusInternalServerError || failure.Body.String() != "could not update key\n" || activations != 0 {
		t.Fatalf("storage failure = %d %s activations=%d", failure.Code, failure.Body.String(), activations)
	}
	var rolledBack store.InboundKey
	if err := st.DB.NewSelect().Model(&rolledBack).Where("id = ?", k.ID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	boundUsers, err := st.ListKeyUserIDs(ctx, k.ID)
	if err != nil {
		t.Fatal(err)
	}
	boundTeams, err := st.ListKeyTeamIDs(ctx, k.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, rolledBack) || !reflect.DeepEqual(boundUsers, []uuid.UUID{u.ID}) || !reflect.DeepEqual(boundTeams, []uuid.UUID{team.ID}) {
		t.Fatal("failed binding write was not rolled back")
	}
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE key_teams DROP CONSTRAINT "+constraint); err != nil {
		t.Fatal(err)
	}
	w := membershipRequest(ctx, h, access, http.MethodPut, path, `{"description":"saved","tenant":"saved","allowed_models":[],"expires_at":"","user_ids":[]}`)
	if w.Code != http.StatusOK || activations != 1 {
		t.Fatalf("commit = %d %s activations=%d", w.Code, w.Body.String(), activations)
	}
	users, _ := st.ListKeyUserIDs(ctx, k.ID)
	teams, _ := st.ListKeyTeamIDs(ctx, k.ID)
	if len(users)+len(teams) != 0 {
		t.Fatal("empty replacement did not clear both sets")
	}
	deps.RequestReload = func() error { activations++; return errors.New("fixture activation failure") }
	h.UpdateDependencies(deps)
	w = membershipRequest(ctx, h, access, http.MethodPut, path, `{"description":"saved despite activation failure"}`)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "saved but activation failed") || activations != 2 {
		t.Fatalf("activation failure = %d %s count=%d", w.Code, w.Body.String(), activations)
	}
	var saved store.InboundKey
	if err := st.DB.NewSelect().Model(&saved).Where("id = ?", k.ID).Scan(ctx); err != nil {
		t.Fatal(err)
	}
	if saved.Description != "saved despite activation failure" || saved.Tenant != "saved" || len(saved.AllowedModels) != 0 || saved.ExpiresAt != nil || saved.TokenHash != before.TokenHash || saved.Enabled != before.Enabled {
		t.Fatalf("saved policy = %+v", saved)
	}
}
