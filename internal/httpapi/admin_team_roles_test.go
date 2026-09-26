package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func teamMemberRoles(body map[string]interface{}) map[string]string {
	out := map[string]string{}
	items, _ := body["members"].([]interface{})
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		uid, _ := m["user_id"].(string)
		role, _ := m["role"].(string)
		out[uid] = role
	}
	return out
}

func TestTeamRolesAndManagement(t *testing.T) {
	st, h, ownerAccess := openUsersTestStore(t)
	suffix := time.Now().UnixNano()

	orgID := mkOrg(t, st, h, ownerAccess, "teamroles")
	adminEmail := fmt.Sprintf("teamadmin-%d@example.com", suffix)
	memberEmail := fmt.Sprintf("teammember-%d@example.com", suffix)
	_, adminAccess := mkOrgMember(t, st, h, ownerAccess, adminEmail, orgID, "member")
	_, memberAccess := mkOrgMember(t, st, h, ownerAccess, memberEmail, orgID, "member")
	adminID := userIDByEmail(t, st, adminEmail)
	memberID := userIDByEmail(t, st, memberEmail)

	code, body := callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams", map[string]interface{}{
		"name": fmt.Sprintf("roles-%d", suffix),
	})
	if code != http.StatusCreated {
		t.Fatalf("create team status = %d, body = %v", code, body)
	}
	teamID, _ := body["id"].(string)

	code, _ = callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", map[string]interface{}{
		"email": adminEmail, "role": "admin",
	})
	if code != http.StatusCreated {
		t.Fatalf("add team admin status = %d", code)
	}
	code, _ = callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", map[string]interface{}{
		"email": memberEmail,
	})
	if code != http.StatusCreated {
		t.Fatalf("add team member status = %d", code)
	}

	code, body = callAdmin(t, h, adminAccess, http.MethodGet, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", nil)
	if code != http.StatusOK {
		t.Fatalf("list team members status = %d", code)
	}
	roles := teamMemberRoles(body)
	if roles[adminID] != "admin" || roles[memberID] != "member" {
		t.Fatalf("team roles wrong: %v", roles)
	}

	code, body = callAdmin(t, h, adminAccess, http.MethodGet, "/_internal/admin/orgs/"+orgID+"/teams", nil)
	if code != http.StatusOK {
		t.Fatalf("list teams status = %d", code)
	}
	found := false
	for _, item := range body["teams"].([]interface{}) {
		m, _ := item.(map[string]interface{})
		if m["id"] == teamID {
			found = true
			if m["my_role"] != "admin" {
				t.Fatalf("my_role wrong: %v", m)
			}
		}
	}
	if !found {
		t.Fatalf("team missing from list: %v", body)
	}

	thirdEmail := fmt.Sprintf("teamthird-%d@example.com", suffix)
	_, _ = mkOrgMember(t, st, h, ownerAccess, thirdEmail, orgID, "member")
	code, _ = callAdmin(t, h, adminAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", map[string]interface{}{
		"email": thirdEmail, "role": "member",
	})
	if code != http.StatusCreated {
		t.Fatalf("team admin must add org members: %d", code)
	}
	code, _ = callAdmin(t, h, adminAccess, http.MethodPut, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members/"+userIDByEmail(t, st, thirdEmail), map[string]interface{}{
		"role": "admin",
	})
	if code != http.StatusOK {
		t.Fatalf("team admin must set team roles: %d", code)
	}

	if code, _ := callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", map[string]interface{}{
		"email": thirdEmail,
	}); code == http.StatusCreated {
		t.Fatalf("plain team member must not add members")
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPut, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members/"+memberID, map[string]interface{}{
		"role": "admin",
	}); code == http.StatusOK {
		t.Fatalf("plain team member must not set roles")
	}

	if code, _ := callAdmin(t, h, adminAccess, http.MethodDelete, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID, nil); code == http.StatusOK {
		t.Fatalf("team admin must not delete teams")
	}

	thirdID := userIDByEmail(t, st, thirdEmail)
	code, _ = callAdmin(t, h, adminAccess, http.MethodDelete, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members/"+thirdID, nil)
	if code != http.StatusOK {
		t.Fatalf("team admin must remove members: %d", code)
	}
	if code, _ := callAdmin(t, h, adminAccess, http.MethodPut, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members/"+adminID, map[string]interface{}{
		"role": "member",
	}); code == http.StatusOK {
		t.Fatalf("demoting the last team admin must fail")
	}
	if code, _ := callAdmin(t, h, adminAccess, http.MethodDelete, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members/"+adminID, nil); code == http.StatusOK {
		t.Fatalf("removing the last team admin must fail")
	}
}

func TestTeamKeyOwnership(t *testing.T) {
	st, h, ownerAccess := openUsersTestStore(t)
	suffix := time.Now().UnixNano()

	orgID := mkOrg(t, st, h, ownerAccess, "teamkeys")
	adminEmail := fmt.Sprintf("tkeyadmin-%d@example.com", suffix)
	memberEmail := fmt.Sprintf("tkeymember-%d@example.com", suffix)
	_, adminAccess := mkOrgMember(t, st, h, ownerAccess, adminEmail, orgID, "member")
	_, memberAccess := mkOrgMember(t, st, h, ownerAccess, memberEmail, orgID, "member")

	code, body := callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams", map[string]interface{}{
		"name": fmt.Sprintf("tkeys-%d", suffix),
	})
	if code != http.StatusCreated {
		t.Fatalf("create team status = %d, body = %v", code, body)
	}
	teamID, _ := body["id"].(string)
	for _, tc := range []struct {
		access, email, role string
	}{
		{adminAccess, adminEmail, "admin"},
		{memberAccess, memberEmail, "member"},
	} {
		if code, _ := callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", map[string]interface{}{
			"email": tc.email, "role": tc.role,
		}); code != http.StatusCreated {
			t.Fatalf("add %s status = %d", tc.email, code)
		}
	}

	teamKeyName := fmt.Sprintf("tkey-%d", suffix)
	code, body = callAdmin(t, h, adminAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": teamKeyName, "org_id": orgID, "owner_type": "team", "owner_id": teamID,
	})
	if code != http.StatusCreated {
		t.Fatalf("team admin create team key status = %d, body = %v", code, body)
	}
	created, _ := body["key"].(map[string]interface{})
	teamKeyID, _ := created["id"].(string)
	t.Cleanup(func() { _ = st.DeleteInboundKey(context.Background(), mustParseUUID(teamKeyID)) })
	if created["owner_team_id"] != teamID {
		t.Fatalf("team key must be owned by team: %v", created)
	}
	bound, _ := created["team_ids"].([]interface{})
	if len(bound) != 1 || bound[0] != teamID {
		t.Fatalf("team key must auto-bind owner team: %v", created)
	}

	code, _ = callAdmin(t, h, adminAccess, http.MethodPut, "/_internal/admin/keys/"+teamKeyID, map[string]interface{}{
		"description": "managed by team admin",
	})
	if code != http.StatusOK {
		t.Fatalf("team admin must update team key: %d", code)
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPut, "/_internal/admin/keys/"+teamKeyID, map[string]interface{}{
		"description": "hijacked",
	}); code == http.StatusOK {
		t.Fatalf("plain team member must not update team key")
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/keys", nil)
	if code != http.StatusOK {
		t.Fatalf("member list status = %d", code)
	}
	seen := keyNames(body)
	row, ok := seen[teamKeyName]
	if !ok {
		t.Fatalf("team member must see team key: %v", seen)
	}
	if row["owner_team_id"] != teamID {
		t.Fatalf("member view must show team owner: %v", row)
	}
	if _, has := row["team_ids"]; has {
		t.Fatalf("member view must hide team associations: %v", row)
	}

	code, body = callAdmin(t, h, adminAccess, http.MethodGet, "/_internal/admin/keys", nil)
	if code != http.StatusOK {
		t.Fatalf("team admin list status = %d", code)
	}
	arow := keyNames(body)[teamKeyName]
	if arow == nil || arow["can_manage"] != true {
		t.Fatalf("team admin must see manageable team key: %v", arow)
	}
	if arow["owner_name"] == "" || arow["owner_name"] == nil {
		t.Fatalf("team key must carry owner name: %v", arow)
	}

	if code, _ := callAdmin(t, h, ownerAccess, http.MethodDelete, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID, nil); code == http.StatusOK {
		t.Fatalf("team owning keys must not be deletable")
	}
}
