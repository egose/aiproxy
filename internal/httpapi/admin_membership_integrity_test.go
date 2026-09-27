package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func membershipHTTPUser(t *testing.T, st *store.Store) (store.User, string) {
	t.Helper()
	u := store.User{Email: uuid.NewString() + "@example.com", PasswordHash: "unused"}
	if err := st.CreateUser(context.Background(), &u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := st.DB.NewDelete().Model(&u).Where("id = ?", u.ID).Exec(context.Background()); err != nil {
			t.Error(err)
		}
	})
	access, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	return u, access
}

func membershipRequest(ctx context.Context, h *Handler, access, method, path, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body)).WithContext(ctx)
	req.Header.Set("Authorization", "Bearer "+access)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestMembershipDuplicatePOSTAndExplicitPUT(t *testing.T) {
	for _, scope := range []string{"workspace", "team"} {
		t.Run(scope, func(t *testing.T) {
			st, h, ownerAccess := openUsersTestStore(t)
			ctx := context.Background()
			workspace := store.Workspace{Name: uuid.NewString()}
			if err := st.CreateWorkspace(ctx, &workspace); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
			admin, access := membershipHTTPUser(t, st)
			workspaceRole := "admin"
			if scope == "team" {
				workspaceRole = "member"
			}
			if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: admin.ID, WorkspaceID: workspace.ID, Role: workspaceRole}); err != nil {
				t.Fatal(err)
			}
			base := "/_internal/admin/workspaces/" + workspace.ID.String()
			team := store.WorkspaceTeam{WorkspaceID: workspace.ID, Name: "team"}
			if scope == "team" {
				if err := st.CreateTeam(ctx, &team); err != nil {
					t.Fatal(err)
				}
				if err := st.AddTeamMember(ctx, admin.ID, team.ID, "admin"); err != nil {
					t.Fatal(err)
				}
				base += "/teams/" + team.ID.String()
			}
			base += "/members"
			assertRole := func(id uuid.UUID, want string) {
				t.Helper()
				m, err := st.GetMembership(ctx, id, workspace.ID)
				role := m.Role
				if scope == "team" {
					tm, teamErr := st.GetTeamMember(ctx, id, team.ID)
					role, err = tm.Role, teamErr
				}
				if err != nil || role != want {
					t.Fatalf("stored role=%q want=%q: %v", role, want, err)
				}
			}
			for _, by := range []string{"user_id", "email"} {
				for _, role := range []string{"", "member", "admin"} {
					value := admin.ID.String()
					if by == "email" {
						value = admin.Email
					}
					roleJSON := ""
					if role != "" {
						roleJSON = fmt.Sprintf(`,"role":%q`, role)
					}
					w := membershipRequest(ctx, h, access, http.MethodPost, base, fmt.Sprintf(`{%q:%q%s}`, by, value, roleJSON))
					if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "PUT") || !strings.Contains(w.Body.String(), "already exists") {
						t.Fatalf("duplicate %s/%s = %d %s", by, role, w.Code, w.Body.String())
					}
					assertRole(admin.ID, "admin")
					count, err := st.CountWorkspaceAdmins(ctx, workspace.ID)
					if scope == "team" {
						count, err = st.CountTeamAdmins(ctx, team.ID)
					}
					if err != nil || count != 1 {
						t.Fatalf("sole admin count=%d, %v", count, err)
					}
				}
			}
			for _, method := range []string{http.MethodPut, http.MethodDelete} {
				w := membershipRequest(ctx, h, ownerAccess, method, base+"/"+admin.ID.String(), `{"role":"member"}`)
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "assign another") {
					t.Fatalf("last admin %s = %d %s", method, w.Code, w.Body.String())
				}
				assertRole(admin.ID, "admin")
			}
			for _, by := range []string{"user_id", "email"} {
				member, _ := membershipHTTPUser(t, st)
				if scope == "team" {
					if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: member.ID, WorkspaceID: workspace.ID, Role: "member"}); err != nil {
						t.Fatal(err)
					}
				}
				value := member.ID.String()
				if by == "email" {
					value = member.Email
				}
				w := membershipRequest(ctx, h, access, http.MethodPost, base, fmt.Sprintf(`{%q:%q}`, by, value))
				if w.Code != http.StatusCreated {
					t.Fatalf("retained admin add = %d %s", w.Code, w.Body.String())
				}
				assertRole(member.ID, "member")
				w = membershipRequest(ctx, h, access, http.MethodPost, base, fmt.Sprintf(`{%q:%q,"role":"admin"}`, by, value))
				if w.Code != http.StatusConflict {
					t.Fatalf("implicit promotion = %d %s", w.Code, w.Body.String())
				}
				assertRole(member.ID, "member")
				w = membershipRequest(ctx, h, access, http.MethodPut, base+"/"+member.ID.String(), `{"role":"admin"}`)
				if w.Code != http.StatusOK {
					t.Fatalf("explicit promotion = %d %s", w.Code, w.Body.String())
				}
				assertRole(member.ID, "admin")
			}
			w := membershipRequest(ctx, h, access, http.MethodPut, base+"/"+admin.ID.String(), `{"role":"member"}`)
			if scope == "workspace" {
				if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "own workspace admin role") {
					t.Fatalf("workspace self demotion = %d %s", w.Code, w.Body.String())
				}
				assertRole(admin.ID, "admin")
			} else {
				if w.Code != http.StatusOK {
					t.Fatalf("team self demotion with successor = %d %s", w.Code, w.Body.String())
				}
				assertRole(admin.ID, "member")
			}
		})
	}
}

func TestMembershipHTTPConcurrentTransitionsRetainAccess(t *testing.T) {
	for _, scope := range []string{"workspace", "team"} {
		for _, methods := range [][2]string{{http.MethodPut, http.MethodPut}, {http.MethodPut, http.MethodDelete}, {http.MethodDelete, http.MethodDelete}} {
			t.Run(scope+"/"+methods[0]+"+"+methods[1], func(t *testing.T) {
				st, h, ownerAccess := openUsersTestStore(t)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				workspace := store.Workspace{Name: uuid.NewString()}
				if err := st.CreateWorkspace(ctx, &workspace); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = st.DeleteWorkspace(context.Background(), workspace.ID) })
				team := store.WorkspaceTeam{WorkspaceID: workspace.ID, Name: "team"}
				if err := st.CreateTeam(ctx, &team); err != nil {
					t.Fatal(err)
				}
				base := "/_internal/admin/workspaces/" + workspace.ID.String()
				if scope == "team" {
					base += "/teams/" + team.ID.String()
				}
				base += "/members"
				users := make([]store.User, 2)
				access := make([]string, 2)
				for i := range users {
					users[i], access[i] = membershipHTTPUser(t, st)
					role := "admin"
					if scope == "team" {
						role = "member"
					}
					if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: users[i].ID, WorkspaceID: workspace.ID, Role: role}); err != nil {
						t.Fatal(err)
					}
					if scope == "team" {
						if err := st.AddTeamMember(ctx, users[i].ID, team.ID, "admin"); err != nil {
							t.Fatal(err)
						}
					}
				}
				holder, err := st.DB.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer holder.Rollback()
				if _, err := holder.ExecContext(ctx, `SELECT id FROM workspaces WHERE id = ? FOR NO KEY UPDATE`, workspace.ID); err != nil {
					t.Fatal(err)
				}
				results := make(chan *httptest.ResponseRecorder, 2)
				for i, method := range methods {
					go func(i int, method string) {
						results <- membershipRequest(ctx, h, ownerAccess, method, base+"/"+users[i].ID.String(), `{"role":"member"}`)
					}(i, method)
				}
				ticker := time.NewTicker(time.Millisecond)
				defer ticker.Stop()
				for {
					var n int
					if err := st.DB.NewRaw(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
						AND cardinality(pg_blocking_pids(pid)) > 0 AND query LIKE ?`, "%"+workspace.ID.String()+"%").Scan(ctx, &n); err != nil {
						t.Fatal(err)
					}
					if n == 2 {
						break
					}
					select {
					case <-ctx.Done():
						t.Fatal("both HTTP mutations did not wait for workspace lock")
					case <-ticker.C:
					}
				}
				if err := holder.Commit(); err != nil {
					t.Fatal(err)
				}
				statuses := map[int]int{}
				for range 2 {
					w := <-results
					statuses[w.Code]++
					if w.Code == http.StatusBadRequest && !strings.Contains(w.Body.String(), "assign another") {
						t.Errorf("unhelpful conflict: %s", w.Body.String())
					}
				}
				if statuses[http.StatusOK] != 1 || statuses[http.StatusBadRequest] != 1 {
					t.Fatalf("concurrent statuses=%v", statuses)
				}
				count, err := st.CountWorkspaceAdmins(ctx, workspace.ID)
				if scope == "team" {
					count, err = st.CountTeamAdmins(ctx, team.ID)
				}
				if err != nil || count != 1 {
					t.Fatalf("remaining admins=%d, %v", count, err)
				}
				for i, u := range users {
					m, _ := st.GetMembership(ctx, u.ID, workspace.ID)
					role := m.Role
					if scope == "team" {
						tm, _ := st.GetTeamMember(ctx, u.ID, team.ID)
						role = tm.Role
					}
					w := membershipRequest(ctx, h, access[i], http.MethodPut, base+"/"+u.ID.String(), `{"role":"admin"}`)
					if role == "admin" && w.Code != http.StatusOK {
						t.Fatalf("survivor lost management: %d %s", w.Code, w.Body.String())
					}
					if role != "admin" && w.Code == http.StatusOK {
						t.Fatal("former admin retained role-edit access")
					}
				}
			})
		}
	}
}

func TestDeleteUserCannotCascadeLastMembershipAdmin(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	ctx := context.Background()
	workspace := store.Workspace{Name: uuid.NewString()}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteWorkspace(ctx, workspace.ID) })
	u, _ := membershipHTTPUser(t, st)
	if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: u.ID, WorkspaceID: workspace.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	w := membershipRequest(ctx, h, access, http.MethodDelete, "/_internal/admin/users/"+u.ID.String(), "")
	if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "assign another workspace admin") {
		t.Fatalf("cascade guard=%d %s", w.Code, w.Body.String())
	}
	if _, err := st.GetUserByID(ctx, u.ID); err != nil {
		t.Fatalf("rejected deletion lost account: %v", err)
	}
	if n, err := st.CountWorkspaceAdmins(ctx, workspace.ID); err != nil || n != 1 {
		t.Fatalf("rejected deletion changed membership=%d, %v", n, err)
	}
}
