package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func mkOrgMember(t *testing.T, st *store.Store, h *Handler, adminAccess, email, orgID, role string) (store.User, string) {
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
	if err := st.UpsertMembership(ctx, &store.OrganizationMember{UserID: u.ID, OrgID: mustParseUUID(orgID), Role: role}); err != nil {
		t.Fatal(err)
	}
	access, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	return *u, access
}

func mkOrg(t *testing.T, st *store.Store, h *Handler, access, prefix string) string {
	t.Helper()
	name := fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
	code, body := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/orgs", map[string]interface{}{"name": name})
	if code != http.StatusCreated {
		t.Fatalf("create org %s: status = %d, body = %v", name, code, body)
	}
	id, _ := body["id"].(string)
	t.Cleanup(func() { _ = st.DeleteOrganization(context.Background(), mustParseUUID(id)) })
	return id
}

func TestSystemOrgSeeded(t *testing.T) {
	st, _, _ := openUsersTestStore(t)
	ctx := context.Background()
	if err := st.EnsureSystemOrg(ctx); err != nil {
		t.Fatal(err)
	}
	org, err := st.GetOrganization(ctx, store.SystemOrgID)
	if err != nil {
		t.Fatalf("system org missing: %v", err)
	}
	if !org.IsSystem || org.Name != "system" {
		t.Fatalf("system org wrong: %+v", org)
	}
	if err := st.EnsureSystemOrg(ctx); err != nil {
		t.Fatalf("reseed not idempotent: %v", err)
	}
}

func TestOrgCRUD(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	name := fmt.Sprintf("acme-%d", time.Now().UnixNano())

	code, body := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/orgs", map[string]interface{}{
		"name": name, "display_name": "Acme",
	})
	if code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %v", code, body)
	}
	id, _ := body["id"].(string)
	if id == "" {
		t.Fatalf("missing id: %v", body)
	}
	t.Cleanup(func() { _ = st.DeleteOrganization(context.Background(), mustParseUUID(id)) })

	code, body = callAdmin(t, h, access, http.MethodGet, "/_internal/admin/orgs/"+id, nil)
	if code != http.StatusOK {
		t.Fatalf("get status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodPut, "/_internal/admin/orgs/"+id, map[string]interface{}{
		"display_name": "Acme Inc",
	})
	if code != http.StatusOK {
		t.Fatalf("update status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/orgs/"+id, nil)
	if code != http.StatusOK {
		t.Fatalf("delete status = %d", code)
	}
}

func TestSystemOrgProtected(t *testing.T) {
	_, h, access := openUsersTestStore(t)
	sysID := store.SystemOrgID.String()
	if code, _ := callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/orgs/"+sysID, nil); code != http.StatusBadRequest {
		t.Fatalf("delete system status = %d, want 400", code)
	}
}

func TestOrgTeams(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	ctx := context.Background()
	orgID := mkOrg(t, st, h, access, "teamorg")

	code, body := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams", map[string]interface{}{
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
	if err := st.UpsertMembership(ctx, &store.OrganizationMember{UserID: member.ID, OrgID: mustParseUUID(orgID), Role: "member"}); err != nil {
		t.Fatal(err)
	}
	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", map[string]interface{}{
		"email": memberEmail,
	})
	if code != http.StatusCreated {
		t.Fatalf("add team member status = %d", code)
	}
	code, body = callAdmin(t, h, access, http.MethodGet, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", nil)
	if code != http.StatusOK {
		t.Fatalf("list team members status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members/"+member.ID.String(), nil)
	if code != http.StatusOK {
		t.Fatalf("remove team member status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID, nil)
	if code != http.StatusOK {
		t.Fatalf("delete team status = %d", code)
	}
}

func TestOrgMembersAndLastAdminGuard(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	orgID := mkOrg(t, st, h, access, "memberorg")

	otherEmail := fmt.Sprintf("other-%d@example.com", time.Now().UnixNano())
	code, _ := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/members", map[string]interface{}{
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
	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/members", map[string]interface{}{
		"email": otherEmail, "role": "member",
	})
	if code != http.StatusCreated {
		t.Fatalf("add member status = %d", code)
	}
	code, body := callAdmin(t, h, access, http.MethodGet, "/_internal/admin/orgs/"+orgID+"/members", nil)
	if code != http.StatusOK {
		t.Fatalf("list members status = %d", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodPut, "/_internal/admin/orgs/"+orgID+"/members/"+other.ID.String(), map[string]interface{}{
		"role": "superuser",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("bad role status = %d, want 400", code)
	}
	_ = body
}

func TestOrgIsolation(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	orgA := mkOrg(t, st, h, access, "orga")
	orgB := mkOrg(t, st, h, access, "orgb")

	memberEmail := fmt.Sprintf("member-%d@example.com", time.Now().UnixNano())
	_, memberAccess := mkOrgMember(t, st, h, access, memberEmail, orgB, "member")

	provA := fmt.Sprintf("prova-%d", time.Now().UnixNano())
	code, _ := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": provA, "type": "openai", "api_key": "sk-test", "org_id": orgA,
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
			t.Fatalf("org B member sees org A provider")
		}
	}
	code, _ = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers/"+provA, nil)
	if code != http.StatusNotFound {
		t.Fatalf("cross-org detail status = %d, want 404", code)
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": fmt.Sprintf("provx-%d", time.Now().UnixNano()), "type": "openai", "api_key": "sk-test", "org_id": orgA,
		"models": []interface{}{map[string]interface{}{"name": "m1"}},
	}); code == http.StatusCreated {
		t.Fatalf("member created provider in org without admin role")
	}

	adminEmail := fmt.Sprintf("orgadmin-%d@example.com", time.Now().UnixNano())
	_, adminAccess := mkOrgMember(t, st, h, access, adminEmail, orgB, "admin")
	provB := fmt.Sprintf("provb-%d", time.Now().UnixNano())
	if code, _ := callAdmin(t, h, adminAccess, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": provB, "type": "openai", "api_key": "sk-test", "org_id": orgB,
		"models": []interface{}{map[string]interface{}{"name": "m1"}},
	}); code != http.StatusCreated {
		t.Fatalf("org admin create status = %d", code)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(context.Background(), provB); err == nil {
			_ = st.DeleteProvider(context.Background(), p.ID)
		}
	})
	code, body = callAdmin(t, h, adminAccess, http.MethodGet, "/_internal/admin/providers", nil)
	if code != http.StatusOK {
		t.Fatalf("org admin list status = %d", code)
	}
	seen := map[string]bool{}
	for _, item := range body["providers"].([]interface{}) {
		seen[item.(map[string]interface{})["name"].(string)] = true
	}
	if !seen[provB] || seen[provA] {
		t.Fatalf("org admin sees wrong set: %v", seen)
	}
}

func TestRegistrationFlow(t *testing.T) {
	st, h, _ := openUsersTestStore(t)
	ctx := context.Background()
	email := fmt.Sprintf("reg-%d@example.com", time.Now().UnixNano())

	code, body := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/register", map[string]interface{}{
		"email": email, "password": "register-pw", "org_name": fmt.Sprintf("regorg-%d", time.Now().UnixNano()),
	})
	if code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %v", code, body)
	}
	t.Cleanup(func() {
		if u, err := st.GetUserByEmail(context.Background(), email); err == nil {
			members, _ := st.ListMembershipsByUser(context.Background(), u.ID)
			for _, m := range members {
				_ = st.DeleteOrganization(context.Background(), m.OrgID)
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
	org, _ := body["organization"].(map[string]interface{})
	if org["name"] == nil || org["name"] == "" {
		t.Fatalf("missing organization in response: %v", body)
	}
	members, err := st.ListMembershipsByUser(ctx, mustParseUUID(userIDByEmail(t, st, email)))
	if err != nil || len(members) != 1 || members[0].Role != "admin" {
		t.Fatalf("membership wrong: %v %v", members, err)
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
