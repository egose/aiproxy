package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
)

func TestInferenceCredentialPolicyAcrossDatabaseReload(t *testing.T) {
	a, path, cfg, _ := catalogRuntimeFixtureWithAuth(t, `
auth "main" {
  mode = "bearer_static"
  rate_limit {
    requests_per_minute = 1
    burst = 3
  }
  client "static" {
    token = "static-fixture"
    tenant = "shared-label"
    allowed_models = ["local/old"]
  }
}
`)
	ctx := context.Background()
	st := a.adminStore
	workspace := store.Workspace{Name: "credential-fixture"}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	local, _ := a.Config.Catalog.Provider("local")
	cfg = strings.Replace(cfg, "multi_tenancy { enabled = true }", "multi_tenancy {\n  enabled = true\n  allowed_upstream_origins = [\"https://managed.example.com\"]\n}", 1)
	rewriteConfigFile(t, path, cfg)
	a.buildOpt.transportFactory = tenantMockTransportFactory(map[string]string{"managed.example.com": local.BaseURL})
	encrypted, err := store.EncryptSecret([]byte("managed-upstream-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	managed := store.DBProvider{
		Name: "managed", Type: string(local.Type), BaseURL: "https://managed.example.com", APIKeyEncrypted: encrypted,
		Enabled: true, WorkspaceID: workspace.ID,
	}
	if err := st.CreateProviderAggregate(ctx, &managed, []store.DBProviderModel{
		{Name: "first"}, {Name: "second"},
	}); err != nil {
		t.Fatal(err)
	}
	createKey := func(token, model string) store.InboundKey {
		t.Helper()
		key := store.InboundKey{
			Name: "static", Tenant: "shared-label", WorkspaceID: workspace.ID, Enabled: true,
			TokenHash: store.TokenHash(token), TokenPrefix: "fixture", AllowedModels: []string{model},
		}
		if err := st.CreateInboundKey(ctx, &key); err != nil {
			t.Fatal(err)
		}
		return key
	}
	reload := func() {
		t.Helper()
		if err := a.Reload(); err != nil {
			t.Fatal(err)
		}
	}
	models := func(token string) *http.Response {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		a.Server.Handler.ServeHTTP(w, r)
		return w.Result()
	}
	expectModels := func(token, want string) {
		t.Helper()
		resp := models(token)
		defer resp.Body.Close()
		var body struct {
			Data []httpapi.ModelCard `json:"data"`
		}
		err := json.NewDecoder(resp.Body).Decode(&body)
		if resp.StatusCode != http.StatusOK || err != nil || len(body.Data) != 1 || body.Data[0].ID != want {
			t.Fatalf("catalog = %d %+v, %v; want only %s", resp.StatusCode, body.Data, err, want)
		}
	}
	expectStatus := func(token string, want int) {
		t.Helper()
		resp := models(token)
		defer resp.Body.Close()
		if resp.StatusCode != want || want == http.StatusTooManyRequests && resp.Header.Get("Retry-After") == "" {
			t.Fatalf("catalog = %d, Retry-After=%q; want %d", resp.StatusCode, resp.Header.Get("Retry-After"), want)
		}
	}
	expectChat := func(token, model string, want int) {
		t.Helper()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.Server.Handler.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s inference = %d %s; want %d", model, w.Code, w.Body.String(), want)
		}
	}
	checkIdentity := func(token string, key store.InboundKey) {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
		r.Header.Set("Authorization", "Bearer "+token)
		p, err := a.handler.SnapshotDependencies().Auth.Authenticate(r)
		if err != nil || p == nil || p.Name != key.Name || p.Tenant != key.Tenant ||
			p.CredentialID() != identity.DatabaseKeyID(key.ID) || p.WorkspaceScope() != identity.InWorkspace(workspace.ID) {
			t.Fatalf("active key identity or reporting labels changed: %+v, %v", p, err)
		}
	}

	const file, issued, rotated, replacement = "static-fixture", "issued-fixture", "rotated-fixture", "replacement-fixture"
	key := createKey(issued, "managed/first")
	reload()
	checkIdentity(issued, key)
	expectModels(file, "local/old")
	expectChat(file, "local/new", http.StatusForbidden)
	expectChat(file, "local/old", http.StatusOK)
	expectStatus(file, http.StatusTooManyRequests)
	expectModels(issued, "managed/first")
	expectChat(issued, "managed/second", http.StatusForbidden)
	expectChat(issued, "managed/first", http.StatusOK)
	expectStatus(issued, http.StatusTooManyRequests)

	if err := st.RotateInboundKey(ctx, key.ID, store.TokenHash(rotated), "fixture"); err != nil {
		t.Fatal(err)
	}
	reload()
	checkIdentity(rotated, key)
	expectStatus(issued, http.StatusUnauthorized)
	expectStatus(rotated, http.StatusTooManyRequests)
	if err := st.DeleteInboundKey(ctx, key.ID); err != nil {
		t.Fatal(err)
	}
	next := createKey(replacement, "managed/second")
	if next.ID == key.ID {
		t.Fatal("key recreation reused the deleted key UUID")
	}
	reload()
	checkIdentity(replacement, next)
	expectStatus(rotated, http.StatusUnauthorized)
	expectModels(replacement, "managed/second")
	expectChat(replacement, "managed/first", http.StatusForbidden)
	expectChat(replacement, "managed/second", http.StatusOK)
	expectStatus(replacement, http.StatusTooManyRequests)
	expectStatus(file, http.StatusTooManyRequests)
}
