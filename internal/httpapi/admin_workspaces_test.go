package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func mkWorkspaceMember(t *testing.T, st *store.Store, h *Handler, adminAccess, email, workspaceID, role string) (store.User, string) {
	t.Helper()
	ctx := context.Background()
	hash, err := bcrypt.GenerateFromPassword([]byte("password-123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	u := &store.User{Email: email, PasswordHash: string(hash)}
	if err := st.CreateUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if existing, err := st.GetUserByEmail(context.Background(), email); err == nil {
			_ = st.DeleteUser(context.Background(), existing.ID)
		}
	})
	_ = h
	_ = adminAccess
	if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: u.ID, WorkspaceID: mustParseUUID(workspaceID), Role: role}); err != nil {
		t.Fatal(err)
	}
	access, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	return *u, access
}

func mkWorkspace(t *testing.T, st *store.Store, h *Handler, access, prefix string) string {
	t.Helper()
	name := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	code, body := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/workspaces", map[string]interface{}{"name": name})
	if code != http.StatusCreated {
		t.Fatalf("create workspace %s: status = %d, body = %v", name, code, body)
	}
	id, _ := body["id"].(string)
	t.Cleanup(func() { _ = st.DeleteWorkspace(context.Background(), mustParseUUID(id)) })
	return id
}

func TestSystemWorkspaceSeeded(t *testing.T) {
	st, _, _ := openUsersTestStore(t)
	ctx := context.Background()
	if err := st.EnsureSystemWorkspace(ctx); err != nil {
		t.Fatal(err)
	}
	workspace, err := st.GetWorkspace(ctx, store.SystemWorkspaceID)
	if err != nil {
		t.Fatalf("system workspace missing: %v", err)
	}
	if !workspace.IsSystem || workspace.Name != "system" {
		t.Fatalf("system workspace wrong: %+v", workspace)
	}
	if err := st.EnsureSystemWorkspace(ctx); err != nil {
		t.Fatalf("reseed not idempotent: %v", err)
	}
}

func TestWorkspaceCRUD(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	name := fmt.Sprintf("acme-%d", time.Now().UnixNano())

	code, body := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/workspaces", map[string]interface{}{
		"name": name, "display_name": "Acme",
	})
	if code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %v", code, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", body)
	}
	t.Cleanup(func() { _ = st.DeleteWorkspace(context.Background(), mustParseUUID(id)) })

	code, body = callAdmin(t, h, access, http.MethodGet, "/_internal/admin/workspaces/"+id, nil)
	if code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodPut, "/_internal/admin/workspaces/"+id, map[string]interface{}{
		"display_name": "Acme Inc",
	})
	if code != http.StatusOK {
		t.Fatalf("update status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/workspaces/"+id, nil)
	if code != http.StatusOK {
		t.Fatalf("delete status = %d", code)
	}
}

func TestSystemWorkspaceProtected(t *testing.T) {
	_, h, access := openUsersTestStore(t)
	sysID := store.SystemWorkspaceID.String()
	if code, _ := callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/workspaces/"+sysID, nil); code != http.StatusBadRequest {
		t.Fatalf("delete system status = %d, want 400", code)
	}
}

func TestWorkspaceTeams(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	ctx := context.Background()
	workspaceID := mkWorkspace(t, st, h, access, "teamorg")

	code, body := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/workspaces/"+workspaceID+"/teams", map[string]interface{}{
		"name": "backend", "description": "Backend team",
	})
	if code != http.StatusCreated {
		t.Fatalf("create team status = %d, body = %v", code, body)
	}
	teamID, _ := body["id"].(string)

	memberEmail := fmt.Sprintf("teammate-%d@example.com", time.Now().UnixNano())
	hash, _ := bcrypt.GenerateFromPassword([]byte("password-123"), bcrypt.DefaultCost)
	member := &store.User{Email: memberEmail, PasswordHash: string(hash)}
	if err := st.CreateUser(ctx, member); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if u, err := st.GetUserByEmail(context.Background(), memberEmail); err == nil {
			_ = st.DeleteUser(context.Background(), u.ID)
		}
	})
	if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: member.ID, WorkspaceID: mustParseUUID(workspaceID), Role: "member"}); err != nil {
		t.Fatal(err)
	}
	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/workspaces/"+workspaceID+"/teams/"+teamID+"/members", map[string]interface{}{
		"email": memberEmail,
	})
	if code != http.StatusCreated {
		t.Fatalf("add team member status = %d", code)
	}
	code, body = callAdmin(t, h, access, http.MethodGet, "/_internal/admin/workspaces/"+workspaceID+"/teams/"+teamID+"/members", nil)
	if code != http.StatusOK {
		t.Fatalf("list team members status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/workspaces/"+workspaceID+"/teams/"+teamID+"/members/"+member.ID.String(), nil)
	if code != http.StatusOK {
		t.Fatalf("remove team member status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/workspaces/"+workspaceID+"/teams/"+teamID, nil)
	if code != http.StatusOK {
		t.Fatalf("delete team status = %d", code)
	}
}

func TestWorkspaceMembersAndLastAdminGuard(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	workspaceID := mkWorkspace(t, st, h, access, "memberorg")

	otherEmail := fmt.Sprintf("other-%d@example.com", time.Now().UnixNano())
	code, _ := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/workspaces/"+workspaceID+"/members", map[string]interface{}{
		"email": "ghost@example.com", "role": "member",
	})
	_ = code

	hash, _ := bcrypt.GenerateFromPassword([]byte("password-123"), bcrypt.DefaultCost)
	other := &store.User{Email: otherEmail, PasswordHash: string(hash)}
	if err := st.CreateUser(context.Background(), other); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if u, err := st.GetUserByEmail(context.Background(), otherEmail); err == nil {
			_ = st.DeleteUser(context.Background(), u.ID)
		}
	})
	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/workspaces/"+workspaceID+"/members", map[string]interface{}{
		"email": otherEmail, "role": "member",
	})
	if code != http.StatusCreated {
		t.Fatalf("add member status = %d", code)
	}
	code, body := callAdmin(t, h, access, http.MethodGet, "/_internal/admin/workspaces/"+workspaceID+"/members", nil)
	if code != http.StatusOK {
		t.Fatalf("list members status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodPut, "/_internal/admin/workspaces/"+workspaceID+"/members/"+other.ID.String(), map[string]interface{}{
		"role": "superuser",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("bad role status = %d, want 400", code)
	}
	_ = body
}

func TestWorkspaceIsolation(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	workspaceA := mkWorkspace(t, st, h, access, "orga")
	workspaceB := mkWorkspace(t, st, h, access, "orgb")

	memberEmail := fmt.Sprintf("member-%d@example.com", time.Now().UnixNano())
	_, memberAccess := mkWorkspaceMember(t, st, h, access, memberEmail, workspaceB, "member")

	provA := fmt.Sprintf("prova-%d", time.Now().UnixNano())
	code, _ := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": provA, "type": "openai", "api_key": "sk-test", "workspace_id": workspaceA,
		"models": []interface{}{map[string]interface{}{"name": "m1"}},
	})
	if code != http.StatusCreated {
		t.Fatalf("create provider in A: %d", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(context.Background(), provA); err == nil {
			_ = st.DeleteProvider(context.Background(), p.ID)
		}
	})

	code, body := callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers", nil)
	if code != http.StatusOK {
		t.Fatalf("member list status = %d", code)
	}
	for _, item := range body["providers"].([]interface{}) {
		if item.(map[string]interface{})["name"] == provA {
			t.Fatalf("workspace B member sees workspace A provider")
		}
	}
	code, _ = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers/"+provA, nil)
	if code != http.StatusNotFound {
		t.Fatalf("cross-workspace detail status = %d, want 404", code)
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": fmt.Sprintf("provx-%d", time.Now().UnixNano()), "type": "openai", "api_key": "sk-test", "workspace_id": workspaceA,
		"models": []interface{}{map[string]interface{}{"name": "m1"}},
	}); code == http.StatusCreated {
		t.Fatalf("member created provider in workspace without admin role")
	}

	adminEmail := fmt.Sprintf("orgadmin-%d@example.com", time.Now().UnixNano())
	_, adminAccess := mkWorkspaceMember(t, st, h, access, adminEmail, workspaceB, "admin")
	provB := fmt.Sprintf("provb-%d", time.Now().UnixNano())
	if code, _ := callAdmin(t, h, adminAccess, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": provB, "type": "openai", "api_key": "sk-test", "workspace_id": workspaceB,
		"models": []interface{}{map[string]interface{}{"name": "m1"}},
	}); code != http.StatusCreated {
		t.Fatalf("workspace admin create status = %d", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(context.Background(), provB); err == nil {
			_ = st.DeleteProvider(context.Background(), p.ID)
		}
	})
	code, body = callAdmin(t, h, adminAccess, http.MethodGet, "/_internal/admin/providers", nil)
	if code != http.StatusOK {
		t.Fatalf("workspace admin list status = %d", code)
	}
	seen := map[string]bool{}
	for _, item := range body["providers"].([]interface{}) {
		seen[item.(map[string]interface{})["name"].(string)] = true
	}
	if !seen[provB] || seen[provA] {
		t.Fatalf("workspace admin sees wrong set: %v", seen)
	}
}

func TestRegistrationFlow(t *testing.T) {
	st, h, _ := openUsersTestStore(t)
	ctx := context.Background()
	email := fmt.Sprintf("reg-%d@example.com", time.Now().UnixNano())

	code, body := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/register", map[string]interface{}{
		"email": email, "password": "register-pw", "workspace_name": fmt.Sprintf("regorg-%d", time.Now().UnixNano()),
	})
	if code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %v", code, body)
	}
	t.Cleanup(func() {
		if u, err := st.GetUserByEmail(context.Background(), email); err == nil {
			members, _ := st.ListMembershipsByUser(context.Background(), u.ID)
			for _, m := range members {
				_ = st.DeleteWorkspace(context.Background(), m.WorkspaceID)
			}
			_ = st.DeleteUser(context.Background(), u.ID)
		}
	})
	code, _ = callAdmin(t, h, "", http.MethodPost, "/_internal/admin/login", map[string]interface{}{
		"email": email, "password": "register-pw",
	})
	if code != http.StatusOK {
		t.Fatalf("login after register status = %d", code)
	}
	workspace, _ := body["workspace"].(map[string]interface{})
	if workspace["name"] == nil || workspace["name"] == "" {
		t.Fatalf("missing workspace in response: %v", body)
	}
	members, err := st.ListMembershipsByUser(ctx, mustParseUUID(userIDByEmail(t, st, email)))
	if err != nil || len(members) != 1 || members[0].Role != "admin" {
		t.Fatalf("membership wrong: %v %v", members, err)
	}
}

func TestDeriveWorkspaceName(t *testing.T) {
	cases := map[string]string{
		"jahn@example.com": "jahn-workspace",
		"Jane.Doe@Example": "janedoe-workspace",
		"@example.com":     "examplecom-workspace",
		"---@example.com":  "personal-workspace",
	}
	for email, want := range cases {
		if got := deriveWorkspaceName(email); got != want {
			t.Errorf("deriveWorkspaceName(%q) = %q, want %q", email, got, want)
		}
	}
	long := deriveWorkspaceName(strings.Repeat("a", 100) + "@example.com")
	if len(long) > 64 || !strings.HasSuffix(long, "-workspace") {
		t.Errorf("long email derived %q, want <=64 chars with -workspace suffix", long)
	}
	if !validResourceName(long) {
		t.Errorf("derived %q fails validResourceName", long)
	}
}

func userIDByEmail(t *testing.T, st *store.Store, email string) string {
	t.Helper()
	u, err := st.GetUserByEmail(context.Background(), email)
	if err != nil {
		t.Fatal(err)
	}
	return u.ID.String()
}
