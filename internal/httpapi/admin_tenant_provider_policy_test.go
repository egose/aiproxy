package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func tenantProviderPolicyFixture(t *testing.T) (*adminReasoningFixture, store.Workspace, string) {
	t.Helper()
	fixture := newAdminReasoningFixture(t, nil)
	origins := []string{"https://api.example.com", "https://probe.example.com:8443", "https://api.githubcopilot.com"}
	fixture.runtime.MultiTenancy.AllowedUpstreamOrigins = origins
	deps := fixture.handler.current()
	deps.MultiTenancy.AllowedUpstreamOrigins = origins
	fixture.handler.UpdateDependencies(deps)
	ctx := context.Background()
	workspace := store.Workspace{Name: "tenant-policy"}
	if err := fixture.store.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	user := store.User{Email: "tenant-policy@example.test"}
	if err := fixture.store.CreateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.AddMembership(ctx, &store.WorkspaceMember{UserID: user.ID, WorkspaceID: workspace.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	access, _, err := adminauth.IssueAccess(user.ID.String(), user.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, workspace, access
}

func tenantProviderPolicyRequest(t *testing.T, fixture *adminReasoningFixture, token, method, path string, workspace uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	raw, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(method, "/_internal/admin/"+path, strings.NewReader(string(raw)))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Workspace-ID", workspace.String())
	w := httptest.NewRecorder()
	fixture.handler.ServeHTTP(w, req)
	return w
}

func TestTenantProviderManagementPolicyRejectsBeforePersistence(t *testing.T) {
	fixture, workspace, tenant := tenantProviderPolicyFixture(t)
	ctx, st := context.Background(), fixture.store
	create := map[string]any{"name": "own", "type": "openai", "base_url": "https://api.example.com/v1",
		"api_key": "own-encrypted-fixture", "models": []map[string]string{{"name": "m"}}, "tls_client_hello": "chrome"}
	if w := tenantProviderPolicyRequest(t, fixture, tenant, "POST", "providers", workspace.ID, create); w.Code != 201 || strings.Contains(w.Body.String(), "own-encrypted-fixture") {
		t.Fatalf("own approved provider create = %d %s", w.Code, w.Body.String())
	}
	row, models := adminReasoningSavedProvider(t, st, "own")
	plain, err := store.DecryptSecret(row.APIKeyEncrypted)
	if err != nil || string(plain) != "own-encrypted-fixture" || row.APIKeyRefKey != "" || row.WorkspaceID != workspace.ID {
		t.Fatal("tenant own API key was not stored encrypted in its workspace")
	}
	for _, actor := range []struct{ name, token string }{{"tenant", tenant}, {"global", fixture.access}} {
		for _, tc := range []struct {
			name string
			edit map[string]any
		}{
			{"api-key-ref", map[string]any{"api_key_ref": map[string]string{"path": "/private/keys.json", "key": "host-key"}}},
			{"sidecar", map[string]any{"type": "github-copilot", "api_key": "", "credential_ref": map[string]string{"path": "/private/keys.json", "name": "host-login"}}},
			{"forward-authorization", map[string]any{"forward_authorization": true}},
			{"cleartext-exception", map[string]any{"allow_insecure_base_url": true}},
			{"loopback-http", map[string]any{"base_url": "http://127.0.0.1/v1"}},
			{"loopback-https", map[string]any{"base_url": "https://127.0.0.1/v1"}},
			{"ipv6-loopback", map[string]any{"base_url": "https://[::1]/v1"}},
			{"private", map[string]any{"base_url": "https://10.0.0.1/v1"}},
			{"metadata", map[string]any{"base_url": "https://169.254.169.254/latest"}},
			{"unapproved", map[string]any{"base_url": "https://unapproved.example.com/v1"}},
			{"port", map[string]any{"base_url": "https://api.example.com:8443/v1"}},
			{"subdomain", map[string]any{"base_url": "https://sub.api.example.com/v1"}},
			{"suffix", map[string]any{"base_url": "https://api.example.com.attacker.com/v1"}},
			{"userinfo", map[string]any{"base_url": "https://caller@api.example.com/v1"}},
			{"unapproved-default", map[string]any{"type": "anthropic", "base_url": ""}},
			{"probe-private", map[string]any{"healthcheck": map[string]string{"path": "https://169.254.169.254/latest"}}},
			{"probe-cleartext", map[string]any{"healthcheck": map[string]string{"path": "http://api.example.com/ready"}}},
			{"probe-unapproved-port", map[string]any{"healthcheck": map[string]string{"path": "https://probe.example.com/ready"}}},
		} {
			t.Run(actor.name+"/"+tc.name, func(t *testing.T) {
				before, beforeModels := adminReasoningSavedProvider(t, st, row.Name)
				activations := fixture.activations
				for _, method := range []string{"POST", "PUT"} {
					body := make(map[string]any, len(create))
					for key, value := range create {
						body[key] = value
					}
					body["name"] = "rejected"
					for key, value := range tc.edit {
						body[key] = value
					}
					path := "providers"
					if method == "PUT" {
						path = "providers/own"
					}
					w := tenantProviderPolicyRequest(t, fixture, actor.token, method, path, workspace.ID, body)
					if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || strings.Contains(w.Body.String(), "/private/") ||
						strings.Contains(w.Body.String(), "own-encrypted-fixture") || fixture.activations != activations {
						t.Fatalf("forbidden config saved, activated or disclosed a secret: %d %s", w.Code, w.Body.String())
					}
				}
				assertAdminReasoningProviderUnchanged(t, st, before, beforeModels)
				if _, err := st.GetProvider(ctx, "rejected"); !errors.Is(err, sql.ErrNoRows) {
					t.Fatal("forbidden tenant create left a provider row")
				}
			})
		}
		for _, edit := range []map[string]string{
			{"api_key_ref_path": "/private/keys.json"}, {"api_key_ref_key": "host-key"},
			{"copilot_credential_path": "/private/keys.json"}, {"copilot_credential_name": "host-login"},
		} {
			before, beforeModels := adminReasoningSavedProvider(t, st, row.Name)
			activations := fixture.activations
			edit["api_key"] = "never-persisted"
			w := tenantProviderPolicyRequest(t, fixture, actor.token, "PUT", "providers/own/credential", workspace.ID, edit)
			if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != activations ||
				strings.Contains(w.Body.String(), "/private/") {
				t.Fatalf("credential-only update bypassed destination policy: %d %s", w.Code, w.Body.String())
			}
			assertAdminReasoningProviderUnchanged(t, st, before, beforeModels)
		}
	}
	assertAdminReasoningProviderUnchanged(t, st, row, models)
	w := tenantProviderPolicyRequest(t, fixture, tenant, "PUT", "providers/own/credential", workspace.ID, map[string]string{"api_key": "own-rotated-fixture"})
	if w.Code != 200 {
		t.Fatalf("own credential rotation = %d %s", w.Code, w.Body.String())
	}
	after, _ := st.GetProvider(ctx, row.Name)
	plain, err = store.DecryptSecret(after.APIKeyEncrypted)
	if err != nil || string(plain) != "own-rotated-fixture" {
		t.Fatal("own credential rotation did not persist the own encrypted value")
	}
	for _, probe := range []string{"/ready", "https://probe.example.com:8443/ready"} {
		w := tenantProviderPolicyRequest(t, fixture, fixture.access, "PUT", "providers/own", workspace.ID, map[string]any{"healthcheck": map[string]string{"path": probe}, "tls_client_hello": "safari"})
		if w.Code != 200 {
			t.Fatalf("approved own absolute/relative probe rejected: %d %s", w.Code, w.Body.String())
		}
	}
}

func TestTenantProviderOldRowsAndInheritedPolicyCannotBypassCredentialUpdates(t *testing.T) {
	fixture, workspace, tenant := tenantProviderPolicyFixture(t)
	st, ctx := fixture.store, context.Background()
	encrypted, err := store.EncryptSecret([]byte("old-own-fixture"))
	if err != nil {
		t.Fatal(err)
	}
	base := store.DBProvider{Name: "base", Type: "openai", WorkspaceID: workspace.ID, BaseURL: "https://api.example.com/v1", Enabled: true, APIKeyEncrypted: encrypted}
	if err := st.CreateProviderAggregate(ctx, &base, []store.DBProviderModel{{Name: "m"}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name   string
		mutate func(*store.DBProvider)
	}{
		{"private-destination", func(p *store.DBProvider) { p.BaseURL = "https://10.0.0.1/v1" }},
		{"cleartext-exception", func(p *store.DBProvider) { p.AllowInsecureBaseURL = true }},
		{"private-probe", func(p *store.DBProvider) { p.Healthcheck = json.RawMessage(`{"path":"https://[::1]/ready"}`) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy := base
			tc.mutate(&legacy)
			legacy.APIKeyEncrypted = []byte("corrupt-must-not-decrypt")
			if err := st.UpdateProvider(ctx, &legacy); err != nil {
				t.Fatal(err)
			}
			before, models := adminReasoningSavedProvider(t, st, base.Name)
			for _, actor := range []string{tenant, fixture.access} {
				count := fixture.activations
				for _, path := range []string{"providers/base", "providers/base/credential"} {
					w := tenantProviderPolicyRequest(t, fixture, actor, "PUT", path, workspace.ID, map[string]string{"api_key": "new-own-but-forbidden"})
					if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || strings.Contains(w.Body.String(), "decrypt") || fixture.activations != count {
						t.Fatalf("old forbidden row accepted update: %d %s", w.Code, w.Body.String())
					}
				}
				w := tenantProviderPolicyRequest(t, fixture, actor, "POST", "providers", workspace.ID, map[string]string{
					"name": "child", "type": "openai", "extends": "base", "api_key": "new-own-but-forbidden",
				})
				if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || strings.Contains(w.Body.String(), "decrypt") || fixture.activations != count {
					t.Fatalf("inherited effective restriction was bypassed: %d %s", w.Code, w.Body.String())
				}
			}
			assertAdminReasoningProviderUnchanged(t, st, before, models)
			if _, err := st.GetProvider(ctx, "child"); !errors.Is(err, sql.ErrNoRows) {
				t.Fatal("forbidden inherited provider was persisted")
			}
			base.UpdatedAt = legacy.UpdatedAt
			if err := st.UpdateProvider(ctx, &base); err != nil {
				t.Fatal(err)
			}
		})
	}
	legacy := base
	legacy.APIKeyRefKey, legacy.APIKeyRefPath, legacy.ForwardAuthorization, legacy.AllowInsecureBaseURL = "host-key", "/private/keys.json", true, true
	if err := st.UpdateProvider(ctx, &legacy); err != nil {
		t.Fatal(err)
	}
	w := tenantProviderPolicyRequest(t, fixture, fixture.access, "PUT", "providers/base", workspace.ID, map[string]any{
		"api_key_ref": map[string]string{"path": "", "key": ""}, "api_key": "replacement-own",
		"forward_authorization": false, "allow_insecure_base_url": false,
	})
	if w.Code != 200 {
		t.Fatalf("explicit removal of legacy reserved settings could not repair the row: %d %s", w.Code, w.Body.String())
	}
	repaired, _ := st.GetProvider(ctx, "base")
	if repaired.APIKeyRefPath != "" || repaired.APIKeyRefKey != "" || repaired.ForwardAuthorization || repaired.AllowInsecureBaseURL {
		t.Fatal("repair did not remove legacy reserved intent")
	}
}

func TestTenantApprovalNeverGrantsForeignGraphOrCredentialRights(t *testing.T) {
	fixture, workspace, tenant := tenantProviderPolicyFixture(t)
	st, ctx := fixture.store, context.Background()
	foreign := store.Workspace{Name: "foreign"}
	if err := st.CreateWorkspace(ctx, &foreign); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []uuid.UUID{foreign.ID, store.SystemWorkspaceID} {
		name := "foreign"
		if owner == store.SystemWorkspaceID {
			name = "system"
		}
		base := store.DBProvider{Name: name, Type: "openai", WorkspaceID: owner, BaseURL: "https://api.example.com/v1",
			Enabled: true, APIKeyEncrypted: []byte("foreign-corrupt-never-decrypt")}
		if err := st.CreateProviderAggregate(ctx, &base, []store.DBProviderModel{{Name: "private-model"}}); err != nil {
			t.Fatal(err)
		}
		for _, actor := range []string{tenant, fixture.access} {
			w := tenantProviderPolicyRequest(t, fixture, actor, "POST", "providers", workspace.ID, map[string]string{
				"name": "rejected", "type": "openai", "extends": name, "api_key": "own",
			})
			if w.Code != 400 || !strings.Contains(w.Body.String(), "destination workspace") || strings.Contains(w.Body.String(), "decrypt") ||
				strings.Contains(w.Body.String(), "private-model") {
				t.Fatalf("origin approval granted foreign inheritance: %d %s", w.Code, w.Body.String())
			}
		}
		if owner == foreign.ID {
			w := tenantProviderPolicyRequest(t, fixture, tenant, "PUT", "providers/"+name+"/credential", workspace.ID, map[string]string{"api_key": "replacement"})
			if w.Code < 400 {
				t.Fatal("origin approval granted foreign credential update rights")
			}
			after, _ := st.GetProvider(ctx, name)
			if !reflect.DeepEqual(after, base) {
				t.Fatal("foreign credential was modified")
			}
		}
	}
}
