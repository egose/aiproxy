package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func keyNames(body map[string]interface{}) map[string]map[string]interface{} {
	out := map[string]map[string]interface{}{}
	items, _ := body["keys"].([]interface{})
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		name, _ := m["name"].(string)
		out[name] = m
	}
	return out
}

func TestKeySharingBindings(t *testing.T) {
	st, h, ownerAccess := openUsersTestStore(t)
	suffix := time.Now().UnixNano()

	orgID := mkOrg(t, st, h, ownerAccess, "keyshare")
	memberEmail := fmt.Sprintf("keymember-%d@example.com", suffix)
	_, memberAccess := mkOrgMember(t, st, h, ownerAccess, memberEmail, orgID, "member")
	otherEmail := fmt.Sprintf("keyother-%d@example.com", suffix)
	otherOrg := mkOrg(t, st, h, ownerAccess, "keyother")
	_, _ = mkOrgMember(t, st, h, ownerAccess, otherEmail, otherOrg, "member")

	memberID := userIDByEmail(t, st, memberEmail)
	otherID := userIDByEmail(t, st, otherEmail)

	code, body := callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams", map[string]interface{}{
		"name": fmt.Sprintf("kteam-%d", suffix),
	})
	if code != http.StatusCreated {
		t.Fatalf("create team status = %d, body = %v", code, body)
	}
	teamID, _ := body["id"].(string)
	if teamID == "" {
		t.Fatalf("missing team id: %v", body)
	}
	code, _ = callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", map[string]interface{}{
		"email": memberEmail,
	})
	if code != http.StatusCreated && code != http.StatusOK {
		t.Fatalf("add team member status = %d", code)
	}

	expiry := time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339)
	sharedName := fmt.Sprintf("shared-%d", suffix)
	code, body = callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": sharedName, "org_id": orgID, "description": "shared key", "expires_at": expiry,
		"allowed_models": []interface{}{"alias/fast"}, "user_ids": []interface{}{memberID},
	})
	if code != http.StatusCreated {
		t.Fatalf("create shared key status = %d, body = %v", code, body)
	}
	created, _ := body["key"].(map[string]interface{})
	sharedID, _ := created["id"].(string)
	t.Cleanup(func() { _ = st.DeleteInboundKey(context.Background(), mustParseUUID(sharedID)) })
	if created["description"] != "shared key" || created["expires_at"] != expiry {
		t.Fatalf("description/expiry not echoed: %v", created)
	}
	users, _ := created["user_ids"].([]interface{})
	if len(users) != 1 || users[0] != memberID {
		t.Fatalf("user binding not echoed: %v", created)
	}

	privateName := fmt.Sprintf("private-%d", suffix)
	code, body = callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": privateName, "org_id": orgID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create private key status = %d", code)
	}
	if private, ok := body["key"].(map[string]interface{}); ok {
		if pid, ok := private["id"].(string); ok {
			t.Cleanup(func() { _ = st.DeleteInboundKey(context.Background(), mustParseUUID(pid)) })
		}
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/keys", nil)
	if code != http.StatusOK {
		t.Fatalf("member list status = %d", code)
	}
	seen := keyNames(body)
	shared, ok := seen[sharedName]
	if !ok {
		t.Fatalf("member must see shared key: %v", seen)
	}
	if _, ok := seen[privateName]; ok {
		t.Fatalf("member must not see unbound key")
	}
	if shared["description"] != "shared key" || shared["expires_at"] != expiry {
		t.Fatalf("member must see description/expiry: %v", shared)
	}
	if _, has := shared["user_ids"]; has {
		t.Fatalf("member view must hide user associations: %v", shared)
	}
	if _, has := shared["tenant"]; has {
		t.Fatalf("member view must hide tenant: %v", shared)
	}
	models, _ := shared["allowed_models"].([]interface{})
	if len(models) != 1 || models[0] != "alias/fast" {
		t.Fatalf("member must see allowed models: %v", shared)
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/keys/"+sharedID, nil)
	if code != http.StatusOK {
		t.Fatalf("member detail status = %d, body = %v", code, body)
	}
	detail, _ := body["key"].(map[string]interface{})
	if detail["name"] != sharedName || detail["description"] != "shared key" {
		t.Fatalf("member detail wrong: %v", detail)
	}
	if _, has := detail["user_ids"]; has {
		t.Fatalf("member detail must hide user associations")
	}

	personalName := fmt.Sprintf("mine-%d", suffix)
	code, body = callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": personalName, "org_id": orgID,
	})
	if code != http.StatusCreated {
		t.Fatalf("member personal create status = %d, body = %v", code, body)
	}
	personal, _ := body["key"].(map[string]interface{})
	personalID, _ := personal["id"].(string)
	t.Cleanup(func() { _ = st.DeleteInboundKey(context.Background(), mustParseUUID(personalID)) })
	if personal["owner_user_id"] != memberID {
		t.Fatalf("personal key must be owned by creator: %v", personal)
	}
	pusers, _ := personal["user_ids"].([]interface{})
	if len(pusers) != 1 || pusers[0] != memberID {
		t.Fatalf("personal key must auto-bind creator: %v", personal)
	}
	if personal["can_manage"] != true {
		t.Fatalf("owner must be able to manage own key: %v", personal)
	}

	if code, _ := callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": fmt.Sprintf("other-%d", suffix), "org_id": orgID, "owner_type": "user", "owner_id": otherID,
	}); code == http.StatusCreated {
		t.Fatalf("member must not create personal keys for other users")
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": fmt.Sprintf("tmkey-%d", suffix), "org_id": orgID, "owner_type": "team", "owner_id": teamID,
	}); code == http.StatusCreated {
		t.Fatalf("plain team member must not create team keys")
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPut, "/_internal/admin/keys/"+sharedID, map[string]interface{}{
		"description": "hijacked",
	}); code == http.StatusOK {
		t.Fatalf("member must not update keys")
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodDelete, "/_internal/admin/keys/"+sharedID, nil); code == http.StatusOK {
		t.Fatalf("member must not delete keys")
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/keys/"+sharedID+"/rotate", nil); code == http.StatusOK {
		t.Fatalf("member must not rotate keys")
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/keys/"+personalID+"/rotate", nil)
	if code != http.StatusOK {
		t.Fatalf("owner must rotate own key: %d %v", code, body)
	}
	if _, ok := body["token"].(string); !ok {
		t.Fatalf("rotate must return token: %v", body)
	}

	code, _ = callAdmin(t, h, ownerAccess, http.MethodPut, "/_internal/admin/keys/"+sharedID, map[string]interface{}{
		"description": "team key", "user_ids": []interface{}{}, "team_ids": []interface{}{teamID},
	})
	if code != http.StatusOK {
		t.Fatalf("rebind to team status = %d", code)
	}
	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/keys", nil)
	if code != http.StatusOK || keyNames(body)[sharedName] == nil {
		t.Fatalf("member must see team-bound key: %d %v", code, body)
	}

	if code, _ := callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": fmt.Sprintf("badbind-%d", suffix), "org_id": orgID, "user_ids": []interface{}{otherID},
	}); code == http.StatusCreated {
		t.Fatalf("cross-org user binding must be rejected")
	}
	if code, _ := callAdmin(t, h, ownerAccess, http.MethodPut, "/_internal/admin/keys/"+sharedID, map[string]interface{}{
		"expires_at": "not-a-date",
	}); code == http.StatusOK {
		t.Fatalf("invalid expiry must be rejected")
	}

	code, body = callAdmin(t, h, ownerAccess, http.MethodGet, "/_internal/admin/keys/"+sharedID, nil)
	if code != http.StatusOK {
		t.Fatalf("admin detail status = %d", code)
	}
	full, _ := body["key"].(map[string]interface{})
	if full["description"] != "team key" {
		t.Fatalf("admin detail description wrong: %v", full)
	}
	teams, _ := full["team_ids"].([]interface{})
	if len(teams) != 1 || teams[0] != teamID {
		t.Fatalf("admin detail team binding wrong: %v", full)
	}
}
