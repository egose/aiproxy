package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/store"
	"golang.org/x/crypto/bcrypt"
)

func openUsersTestStore(t *testing.T) (*store.Store, *Handler, string) {
	t.Helper()
	dbURL := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dbURL == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "test-jwt-secret-1234567890")
	ctx := context.Background()
	st, err := store.Open(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	email := fmt.Sprintf("owner-%d@example.com", time.Now().UnixNano())
	hash, err := bcrypt.GenerateFromPassword([]byte("owner-password"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatal(err)
	}
	owner := &store.User{Email: email, PasswordHash: string(hash), IsAdmin: true}
	if err := st.CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if u, err := st.GetUserByEmail(context.Background(), email); err == nil {
			_ = st.DeleteUser(context.Background(), u.ID)
		}
	})
	access, _, err := adminauth.IssueAccess(owner.ID.String(), owner.Email, true)
	if err != nil {
		t.Fatal(err)
	}
	rt := &config.Runtime{MultiTenancy: config.MultiTenancy{Enabled: true, AllowPublicRegistration: true}}
	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	logs := observability.NewLogBuffer(10)
	deps := newDashboardDeps(rt, time.Now(), usage, health, logs)
	deps.MultiTenancy = rt.MultiTenancy
	deps.AdminStore = st
	deps.AdminAuthConfig = rt.Auth
	return st, NewHandler(deps), access
}

func callAdmin(t *testing.T, h *Handler, access, method, path string, body interface{}) (int, map[string]interface{}) {
	return callAdminHeaders(t, h, access, method, path, body, nil)
}

func callAdminHeaders(t *testing.T, h *Handler, access, method, path string, body interface{}, headers map[string]string) (int, map[string]interface{}) {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if access != "" {
		req.Header.Set("Authorization", "Bearer "+access)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	var parsed map[string]interface{}
	if len(w.Body.Bytes()) > 0 {
		_ = json.Unmarshal(w.Body.Bytes(), &parsed)
	}
	if parsed == nil {
		parsed = map[string]interface{}{}
	}
	return w.Code, parsed
}

func TestInviteAcceptLoginFlow(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	email := fmt.Sprintf("invited-%d@example.com", time.Now().UnixNano())

	code, body := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/invites", map[string]interface{}{
		"email": email, "role": "user",
	})
	if code != http.StatusCreated {
		t.Fatalf("invite status = %d, want 201", code)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("invite response missing one-time token: %v", body)
	}

	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/invites", map[string]interface{}{
		"email": email, "role": "user",
	})
	_ = code

	code, _ = callAdmin(t, h, "", http.MethodPost, "/_internal/admin/invites/accept", map[string]interface{}{
		"token": token, "password": "invited-password",
	})
	if code != http.StatusCreated {
		t.Fatalf("accept status = %d, want 201", code)
	}

	code, _ = callAdmin(t, h, "", http.MethodPost, "/_internal/admin/invites/accept", map[string]interface{}{
		"token": token, "password": "invited-password-2",
	})
	if code != http.StatusBadRequest {
		t.Fatalf("reuse status = %d, want 400", code)
	}

	code, loginBody := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/login", map[string]interface{}{
		"email": email, "password": "invited-password",
	})
	if code != http.StatusOK || loginBody["access_token"] == nil {
		t.Fatalf("login status = %d, want 200 with token", code)
	}

	code, usersBody := callAdmin(t, h, access, http.MethodGet, "/_internal/admin/users", nil)
	if code != http.StatusOK {
		t.Fatalf("list status = %d", code)
	}
	found := false
	for _, item := range usersBody["users"].([]interface{}) {
		m := item.(map[string]interface{})
		if m["email"] == email {
			found = true
			if m["role"] != "user" {
				t.Errorf("role = %v, want user", m["role"])
			}
		}
	}
	if !found {
		t.Errorf("invited user missing from list")
	}
	t.Cleanup(func() {
		if u, err := st.GetUserByEmail(context.Background(), email); err == nil {
			_ = st.DeleteUser(context.Background(), u.ID)
		}
	})
}

func TestInviteValidation(t *testing.T) {
	_, h, access := openUsersTestStore(t)

	for name, body := range map[string]map[string]interface{}{
		"bad email":    {"email": "not-an-email", "role": "user"},
		"bad role":     {"email": "x@example.com", "role": "superuser"},
		"missing role": {"email": "x@example.com"},
	} {
		if code, _ := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/invites", body); code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", name, code)
		}
	}
	if code, _ := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/invites/accept", map[string]interface{}{
		"token": "nope", "password": "long-enough-password",
	}); code != http.StatusNotFound {
		t.Errorf("bad token: status = %d, want 404", code)
	}
	if code, _ := callAdmin(t, h, "", http.MethodPost, "/_internal/admin/invites/accept", map[string]interface{}{
		"token": "nope", "password": "short",
	}); code != http.StatusBadRequest {
		t.Errorf("short password: status = %d, want 400", code)
	}
}

func TestUserRoleAndDeleteGuards(t *testing.T) {
	st, h, access := openUsersTestStore(t)

	secondEmail := fmt.Sprintf("second-%d@example.com", time.Now().UnixNano())
	code, secondBody := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/users", map[string]interface{}{
		"email": secondEmail, "password": "second-password", "role": "admin",
	})
	if code != http.StatusCreated {
		t.Fatalf("create admin status = %d", code)
	}
	secondID, _ := secondBody["id"].(string)
	t.Cleanup(func() {
		if u, err := st.GetUserByEmail(context.Background(), secondEmail); err == nil {
			_ = st.DeleteUser(context.Background(), u.ID)
		}
	})

	usersCode, usersBody := callAdmin(t, h, access, http.MethodGet, "/_internal/admin/users", nil)
	if usersCode != http.StatusOK {
		t.Fatalf("list status = %d", usersCode)
	}
	for _, item := range usersBody["users"].([]interface{}) {
		m := item.(map[string]interface{})
		if m["role"] != "admin" && m["role"] != "user" {
			t.Errorf("user %v has invalid role %v", m["email"], m["role"])
		}
	}

	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/users/"+secondID+"/role", map[string]interface{}{"role": "user"})
	if code != http.StatusOK {
		t.Fatalf("demote second admin status = %d, want 200", code)
	}

	var ownerUserID string
	for _, item := range usersBody["users"].([]interface{}) {
		m := item.(map[string]interface{})
		if m["email"] != secondEmail && m["role"] == "admin" {
			ownerUserID, _ = m["id"].(string)
		}
	}
	if ownerUserID == "" {
		t.Fatalf("owner admin not found in user list")
	}
	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/users/"+ownerUserID+"/role", map[string]interface{}{"role": "user"})
	if code != http.StatusBadRequest {
		t.Errorf("demote last admin status = %d, want 400", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/users/"+ownerUserID+"/disable", map[string]interface{}{})
	if code != http.StatusBadRequest {
		t.Errorf("disable last admin status = %d, want 400", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodDelete, "/_internal/admin/users/"+ownerUserID, nil)
	if code != http.StatusBadRequest {
		t.Errorf("delete last admin status = %d, want 400", code)
	}
}

func TestUserResetPassword(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	email := fmt.Sprintf("reset-%d@example.com", time.Now().UnixNano())

	code, body := callAdmin(t, h, access, http.MethodPost, "/_internal/admin/users", map[string]interface{}{
		"email": email, "password": "original-password", "role": "user",
	})
	if code != http.StatusCreated {
		t.Fatalf("create status = %d", code)
	}
	id, _ := body["id"].(string)
	t.Cleanup(func() {
		if u, err := st.GetUserByEmail(context.Background(), email); err == nil {
			_ = st.DeleteUser(context.Background(), u.ID)
		}
	})
	if body["role"] != "user" {
		t.Errorf("role = %v, want user", body["role"])
	}

	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/users/"+id+"/reset-password", map[string]interface{}{
		"password": "short",
	})
	if code != http.StatusBadRequest {
		t.Errorf("short reset status = %d, want 400", code)
	}
	code, _ = callAdmin(t, h, access, http.MethodPost, "/_internal/admin/users/"+id+"/reset-password", map[string]interface{}{
		"password": "brand-new-password",
	})
	if code != http.StatusOK {
		t.Fatalf("reset status = %d, want 200", code)
	}
	code, _ = callAdmin(t, h, "", http.MethodPost, "/_internal/admin/login", map[string]interface{}{
		"email": email, "password": "brand-new-password",
	})
	if code != http.StatusOK {
		t.Errorf("login with new password status = %d, want 200", code)
	}
}
