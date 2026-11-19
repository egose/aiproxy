package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestUserDeletionHTTPCommitActivation(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "activation-failure"}[fail], func(t *testing.T) {
			st, h, access := openUsersTestStore(t)
			user, _ := membershipHTTPUser(t, st)
			ctx := context.Background()
			key := store.InboundKey{Name: uuid.NewString(), WorkspaceID: store.SystemWorkspaceID, OwnerUserID: &user.ID, TokenHash: store.TokenHash("synthetic-deletion"), Enabled: true}
			if err := st.CreateInboundKey(ctx, &key); err != nil {
				t.Fatal(err)
			}
			reloads := 0
			deps := h.current()
			deps.RequestReload = func() error {
				reloads++
				if _, err := st.GetUserByID(ctx, user.ID); err == nil {
					t.Fatal("activation before deletion commit")
				}
				n, err := st.DB.NewSelect().Model((*store.InboundKey)(nil)).Where("id = ?", key.ID).Count(ctx)
				if err != nil || n != 0 {
					t.Fatal("activation before key cascade", err)
				}
				if fail {
					return errors.New("private activation fixture detail")
				}
				return nil
			}
			h.UpdateDependencies(deps)
			path := "/_internal/admin/users/" + user.ID.String()
			w := membershipRequest(ctx, h, access, http.MethodDelete, path, "")
			want := http.StatusOK
			if fail {
				want = http.StatusInternalServerError
			}
			if w.Code != want || reloads != 1 || w.Header().Get("X-Aiproxy-User-Deleted") != "true" || w.Header().Get("X-Aiproxy-User-ID") != user.ID.String() || w.Header().Get("Cache-Control") != "no-store" || strings.Contains(w.Body.String(), "private activation") {
				t.Fatalf("delete=%d %s headers=%v reloads=%d", w.Code, w.Body.String(), w.Header(), reloads)
			}
			if fail && w.Body.String() != "saved but activation failed\n" {
				t.Fatal("ambiguous committed failure", w.Body.String())
			}
			w = membershipRequest(ctx, h, access, http.MethodDelete, path, "")
			if w.Code != 404 || reloads != 1 || w.Header().Get("X-Aiproxy-User-Deleted") != "" {
				t.Fatal("repeat deletion requested activation or claimed commit")
			}
		})
	}
}

func TestUserDeletionHTTPRejectionDoesNotActivate(t *testing.T) {
	for _, reason := range []string{"self", "last-workspace-admin", "last-team-admin", "store-failure"} {
		t.Run(reason, func(t *testing.T) {
			st, h, access := openUsersTestStore(t)
			ctx := context.Background()
			user, _ := membershipHTTPUser(t, st)
			if reason == "self" {
				claims, err := adminauth.VerifyAccess(access)
				if err != nil {
					t.Fatal(err)
				}
				ownerID := uuid.MustParse(claims.Subject)
				user, err = st.GetUserByID(ctx, ownerID)
				if err != nil {
					t.Fatal(err)
				}
			}
			if reason == "last-workspace-admin" || reason == "last-team-admin" {
				workspace := store.Workspace{Name: uuid.NewString()}
				if err := st.CreateWorkspace(ctx, &workspace); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
				role := "member"
				if reason == "last-workspace-admin" {
					role = "admin"
				}
				if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: user.ID, WorkspaceID: workspace.ID, Role: role}); err != nil {
					t.Fatal(err)
				}
				if reason == "last-team-admin" {
					team := store.WorkspaceTeam{Name: "sole", WorkspaceID: workspace.ID}
					if err := st.CreateTeam(ctx, &team); err != nil {
						t.Fatal(err)
					}
					if err := st.AddTeamMember(ctx, user.ID, team.ID, "admin"); err != nil {
						t.Fatal(err)
					}
				}
			}
			if reason == "store-failure" {
				_, err := st.DB.ExecContext(ctx, `CREATE FUNCTION fail_user_deletion() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'private deletion fault'; END $$;
					CREATE TRIGGER fail_user_deletion BEFORE DELETE ON users FOR EACH ROW EXECUTE FUNCTION fail_user_deletion()`)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					_, _ = st.DB.ExecContext(ctx, `DROP TRIGGER fail_user_deletion ON users; DROP FUNCTION fail_user_deletion()`)
				})
			}
			reloads := 0
			deps := h.current()
			deps.RequestReload = func() error { reloads++; return nil }
			h.UpdateDependencies(deps)
			w := membershipRequest(ctx, h, access, http.MethodDelete, "/_internal/admin/users/"+user.ID.String(), "")
			want := 400
			if reason == "store-failure" {
				want = 500
			}
			if w.Code != want || reloads != 0 || w.Header().Get("X-Aiproxy-User-Deleted") != "" || w.Header().Get("X-Aiproxy-User-ID") != "" || strings.Contains(w.Body.String(), "private deletion fault") {
				t.Fatalf("rejection=%d %s reloads=%d", w.Code, w.Body.String(), reloads)
			}
			if _, err := st.GetUserByID(ctx, user.ID); err != nil {
				t.Fatal("rejected deletion removed user", err)
			}
		})
	}
}
