package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestAdminDestinationWorkspaceReferences(t *testing.T) {
	rt, err := config.Load([]byte(`
listener "http" "public" { address = "127.0.0.1:8080" }
auth "main" { mode = "none" }
provider "openai" "system-base" {
  api_key = "system-private-fixture"
  model "m" {}
}
provider "openai" "system-disabled" {
  enabled = false
  model "m" {}
}
`), "workspace-graph.hcl")
	if err != nil {
		t.Fatal(err)
	}
	fixture := newAdminReasoningFixture(t, rt)
	st, h, ctx := fixture.store, fixture.handler, context.Background()
	a, b, empty := store.Workspace{Name: "a"}, store.Workspace{Name: "b"}, store.Workspace{Name: "empty"}
	for _, workspace := range []*store.Workspace{&a, &b, &empty} {
		if err := st.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatal(err)
		}
	}
	actor := func(name string, memberships map[uuid.UUID]string) string {
		t.Helper()
		user := store.User{Email: name + "@example.test"}
		if err := st.CreateUser(ctx, &user); err != nil {
			t.Fatal(err)
		}
		for id, role := range memberships {
			if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: user.ID, WorkspaceID: id, Role: role}); err != nil {
				t.Fatal(err)
			}
		}
		access, _, err := adminauth.IssueAccess(user.ID.String(), user.Email, false)
		if err != nil {
			t.Fatal(err)
		}
		return access
	}
	aAdmin := actor("a-admin", map[uuid.UUID]string{a.ID: roleAdmin})
	bAdmin := actor("b-admin", map[uuid.UUID]string{b.ID: roleAdmin})
	both := actor("both", map[uuid.UUID]string{a.ID: roleAdmin, b.ID: roleAdmin})
	systemMember := actor("system-member", map[uuid.UUID]string{store.SystemWorkspaceID: roleMember})
	none := actor("none", nil)
	request := func(access, method, path, header string, body any) *httptest.ResponseRecorder {
		t.Helper()
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r := httptest.NewRequest(method, "/_internal/admin/"+path, strings.NewReader(string(raw)))
		r.Header.Set("Authorization", "Bearer "+access)
		if header != "" {
			r.Header.Set("X-Workspace-ID", header)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	require := func(access, method, path string, owner uuid.UUID, body any, status int) {
		t.Helper()
		w := request(access, method, path, owner.String(), body)
		if w.Code != status {
			t.Fatalf("%s %s in %s = %d %s, want %d", method, path, owner, w.Code, w.Body.String(), status)
		}
	}
	for _, workspace := range []struct {
		id     uuid.UUID
		access string
	}{{a.ID, aAdmin}, {b.ID, bAdmin}} {
		for _, access := range []string{workspace.access, fixture.access} {
			for _, resource := range []string{"providers", "aliases", "keys"} {
				w := request(access, http.MethodGet, resource, workspace.id.String(), nil)
				var body map[string][]json.RawMessage
				if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &body) != nil || body[resource] == nil || len(body[resource]) != 0 {
					t.Fatalf("%s in %s before population = %d, want an empty collection", resource, workspace.id, w.Code)
				}
			}
		}
	}
	for _, workspace := range []struct {
		name, access string
		id           uuid.UUID
	}{{"system", fixture.access, store.SystemWorkspaceID}, {"a", aAdmin, a.ID}, {"b", bAdmin, b.ID}} {
		body := map[string]any{"name": workspace.name + "-managed", "type": "openai", "api_key": "local-fixture",
			"models": []map[string]string{{"name": "m"}}}
		require(workspace.access, "POST", "providers", workspace.id, body, http.StatusCreated)
		require(workspace.access, "POST", "providers", workspace.id, map[string]any{
			"name": workspace.name + "-child", "type": "openai", "api_key": "child-fixture", "extends": workspace.name + "-managed",
		}, http.StatusCreated)
		require(workspace.access, "POST", "aliases", workspace.id, map[string]any{
			"name": workspace.name + "-route", "providers": []string{workspace.name + "-child"}, "model": "m",
		}, http.StatusCreated)
		require(workspace.access, "PUT", "aliases/"+workspace.name+"-route", workspace.id, map[string]any{
			"targets": []map[string]string{{"provider": workspace.name + "-managed", "model": "m"}},
		}, http.StatusOK)
		require(workspace.access, "PUT", "aliases/"+workspace.name+"-route", workspace.id, map[string]any{
			"providers": []string{workspace.name + "-child"}, "model": "m",
		}, http.StatusOK)
	}
	require(fixture.access, "POST", "providers", store.SystemWorkspaceID, map[string]any{
		"name": "system-static-child", "type": "openai", "api_key": "own-fixture", "extends": "system-base",
	}, http.StatusCreated)
	require(fixture.access, "POST", "providers", b.ID, map[string]any{
		"name": "b-disabled", "type": "openai", "enabled": false, "models": []map[string]string{{"name": "private-model"}},
	}, http.StatusCreated)
	foreign := []string{"b-managed", "b-child", "b-disabled", "system-base", "system-disabled", "system-managed"}
	for _, actor := range []struct{ name, token string }{{"tenant", aAdmin}, {"both", both}, {"global", fixture.access}} {
		for _, target := range foreign {
			for _, method := range []string{"POST", "PUT"} {
				t.Run(actor.name+"/"+method+"/"+target, func(t *testing.T) {
					before, beforeModels := adminReasoningSavedProvider(t, st, "a-child")
					aliasBefore, err := st.GetAlias(ctx, "a-route")
					if err != nil {
						t.Fatal(err)
					}
					targetsBefore, err := st.ListAliasTargets(ctx, aliasBefore.ID)
					if err != nil {
						t.Fatal(err)
					}
					count := fixture.activations
					name, path := "rejected", "providers"
					if method == "PUT" {
						name, path = "a-child", "providers/a-child"
					}
					inherit := func(base string) map[string]any {
						return map[string]any{"name": name, "type": "openai", "extends": base, "api_key": "never-saved",
							"workspace_id": a.ID.String(), "display_name": "never-saved", "models": []any{}}
					}
					rejected := request(actor.token, method, path, a.ID.String(), inherit(target))
					missing := request(actor.token, method, path, a.ID.String(), inherit("missing"))
					if rejected.Code != 400 || rejected.Body.String() != missing.Body.String() || rejected.Header().Get(catalogSavedHeader) != "" {
						t.Fatalf("foreign inheritance differs from missing or was saved: %d %s vs %d %s",
							rejected.Code, rejected.Body.String(), missing.Code, missing.Body.String())
					}
					for _, shorthand := range []bool{false, true} {
						path = "aliases"
						if method == "PUT" {
							name, path = "a-route", "aliases/a-route"
						} else {
							name = "rejected"
						}
						aliasBody := func(provider string) map[string]any {
							body := map[string]any{"name": name, "workspace_id": a.ID.String(), "algorithm": "least_connections"}
							if shorthand {
								body["providers"], body["model"] = []string{"a-managed", provider}, "m"
							} else {
								body["targets"] = []map[string]string{{"provider": "a-managed", "model": "m"}, {"provider": provider, "model": "m"}}
							}
							return body
						}
						rejected = request(actor.token, method, path, a.ID.String(), aliasBody(target))
						missing = request(actor.token, method, path, a.ID.String(), aliasBody("missing"))
						if rejected.Code != 400 || rejected.Body.String() != missing.Body.String() {
							t.Fatalf("foreign alias target differs from missing (shorthand %t): %d %s vs %d %s",
								shorthand, rejected.Code, rejected.Body.String(), missing.Code, missing.Body.String())
						}
						if strings.Contains(rejected.Body.String(), target) || strings.Contains(rejected.Body.String(), "private-model") {
							t.Fatal("rejection disclosed foreign inventory")
						}
					}
					if fixture.activations != count {
						t.Fatal("rejected graph requested activation")
					}
					assertAdminReasoningProviderUnchanged(t, st, before, beforeModels)
					after, err := st.GetAlias(ctx, aliasBefore.Name)
					if err != nil {
						t.Fatal(err)
					}
					afterTargets, err := st.ListAliasTargets(ctx, aliasBefore.ID)
					if err != nil || !reflect.DeepEqual(aliasBefore, after) || !reflect.DeepEqual(targetsBefore, afterTargets) {
						t.Fatal("rejected alias update changed its aggregate")
					}
					if _, err := st.GetProvider(ctx, "rejected"); err == nil {
						t.Fatal("rejected provider create left a row")
					}
					if _, err := st.GetAlias(ctx, "rejected"); err == nil {
						t.Fatal("rejected alias create left a row")
					}
				})
			}
		}
	}
	for _, destination := range []struct {
		name string
		id   uuid.UUID
	}{{"system", store.SystemWorkspaceID}, {"b", b.ID}} {
		t.Run("global-author-destination-"+destination.name, func(t *testing.T) {
			before, beforeModels := adminReasoningSavedProvider(t, st, destination.name+"-child")
			count := fixture.activations
			for _, method := range []string{"POST", "PUT"} {
				name, providerPath, aliasPath := "rejected", "providers", "aliases"
				if method == "PUT" {
					name, providerPath, aliasPath = destination.name+"-child", "providers/"+destination.name+"-child", "aliases/"+destination.name+"-route"
				}
				for _, base := range []string{"a-managed", "missing"} {
					w := request(fixture.access, method, providerPath, destination.id.String(), map[string]any{
						"name": name, "type": "openai", "extends": base, "api_key": "never-saved", "models": []any{},
					})
					if w.Code != 400 || strings.Contains(w.Body.String(), base) || !strings.Contains(w.Body.String(), "destination workspace") {
						t.Fatalf("global author used a foreign base in %s: %d %s", destination.name, w.Code, w.Body.String())
					}
				}
				for _, shorthand := range []bool{false, true} {
					body := map[string]any{"name": name}
					if shorthand {
						body["providers"], body["model"] = []string{"a-managed"}, "m"
					} else {
						body["targets"] = []map[string]string{{"provider": "a-managed", "model": "m"}}
					}
					w := request(fixture.access, method, aliasPath, destination.id.String(), body)
					if w.Code != 400 || strings.Contains(w.Body.String(), "a-managed") || !strings.Contains(w.Body.String(), "destination workspace") {
						t.Fatalf("global author used a foreign target in %s: %d %s", destination.name, w.Code, w.Body.String())
					}
				}
			}
			assertAdminReasoningProviderUnchanged(t, st, before, beforeModels)
			if fixture.activations != count {
				t.Fatal("a global author's invalid destination graph requested activation")
			}
		})
	}
	for _, path := range []string{"providers", "aliases"} {
		w := request(fixture.access, "GET", path, empty.ID.String(), nil)
		if w.Code != 200 || !strings.Contains(w.Body.String(), fmt.Sprintf(`"%s":[]`, path)) {
			t.Fatalf("fresh tenant inventory is not empty: %d %s", w.Code, w.Body.String())
		}
	}
	for _, token := range []string{none, systemMember} {
		count := fixture.activations
		for _, path := range []string{"providers", "aliases"} {
			w := request(token, "POST", path, "", map[string]any{"name": "unauthorized"})
			if w.Code < 400 || fixture.activations != count {
				t.Fatal("no tenant write target was granted")
			}
		}
	}
	for _, name := range []string{"system-base", "system-disabled", "b-managed"} {
		w := request(aAdmin, "POST", "providers", a.ID.String(), map[string]any{
			"name": name, "type": "openai", "enabled": false, "models": []map[string]string{{"name": "m"}},
		})
		if w.Code != 400 {
			t.Fatalf("globally unique name %s could be overwritten: %d", name, w.Code)
		}
	}
	t.Run("selected context cannot retarget saved mutations", func(t *testing.T) {
		before, models := adminReasoningSavedProvider(t, st, "a-child")
		count := fixture.activations
		for _, path := range []string{"providers/a-child", "providers/a-child/credential", "aliases/a-route"} {
			for _, selection := range []struct {
				header string
				status int
			}{{b.ID.String(), http.StatusNotFound}, {"invalid", http.StatusBadRequest}} {
				body := map[string]string{"api_key": "never-saved"}
				if strings.HasPrefix(path, "aliases/") {
					body = map[string]string{"algorithm": "round_robin"}
				}
				w := request(fixture.access, "PUT", path, selection.header, body)
				if w.Code != selection.status || fixture.activations != count {
					t.Fatalf("stale selected context mutation %s = %d %s", path, w.Code, w.Body.String())
				}
			}
		}
		w := request(fixture.access, "PUT", "providers/a-child", a.ID.String(), map[string]string{"workspace_id": b.ID.String()})
		if w.Code != http.StatusNotFound {
			t.Fatal("provider update accepted a different destination workspace")
		}
		assertAdminReasoningProviderUnchanged(t, st, before, models)
		require(fixture.access, "PUT", "providers/a-child/credential", a.ID, map[string]string{"api_key": "rotated-own"}, 200)
	})
}

func TestAdminLegacyForeignGraphDoesNotDiscloseInventory(t *testing.T) {
	fixture := newAdminReasoningFixture(t, nil)
	st, ctx := fixture.store, context.Background()
	a, b := store.Workspace{Name: "a"}, store.Workspace{Name: "b"}
	for _, workspace := range []*store.Workspace{&a, &b} {
		if err := st.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatal(err)
		}
	}
	user := store.User{Email: "tenant@example.test"}
	if err := st.CreateUser(ctx, &user); err != nil {
		t.Fatal(err)
	}
	if err := st.AddMembership(ctx, &store.WorkspaceMember{UserID: user.ID, WorkspaceID: a.ID, Role: roleAdmin}); err != nil {
		t.Fatal(err)
	}
	token, _, err := adminauth.IssueAccess(user.ID.String(), user.Email, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []store.DBProvider{
		{Name: "foreign-private", Type: "openai", WorkspaceID: b.ID, Enabled: true},
		{Name: "legacy", Type: "openai", WorkspaceID: a.ID, Extends: "foreign-private", Enabled: false},
	} {
		row.ID = uuid.New()
		if _, err := st.DB.NewInsert().Model(&row).Exec(ctx); err != nil {
			t.Fatal(err)
		}
		model := store.DBProviderModel{ID: uuid.New(), ProviderID: row.ID, Name: "foreign-private-model"}
		if _, err := st.DB.NewInsert().Model(&model).Exec(ctx); err != nil {
			t.Fatal(err)
		}
	}
	alias := store.DBAlias{ID: uuid.New(), Name: "legacy-route", WorkspaceID: a.ID, Algorithm: "round_robin"}
	if _, err := st.DB.NewInsert().Model(&alias).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	target := store.DBAliasTarget{ID: uuid.New(), AliasID: alias.ID, Provider: "foreign-private", Model: "foreign-private-model"}
	if _, err := st.DB.NewInsert().Model(&target).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"providers", "providers/legacy", "aliases", "aliases/legacy-route"} {
		w, _ := adminReasoningRequest(t, fixture.handler, token, "GET", "/_internal/admin/"+path, "")
		if w.Code != 500 || strings.Contains(w.Body.String(), "foreign-private") {
			t.Fatalf("legacy graph disclosed inventory: %d %s", w.Code, w.Body.String())
		}
	}
	deps := fixture.handler.current()
	deps.RequestReload = nil
	fixture.handler.UpdateDependencies(deps)
	w, _ := adminReasoningRequest(t, fixture.handler, token, "PUT", "/_internal/admin/providers/legacy",
		`{"extends":"","models":[{"name":"m"}]}`)
	if w.Code != 200 {
		t.Fatalf("local graph recovery failed: %d %s", w.Code, w.Body.String())
	}
	w, _ = adminReasoningRequest(t, fixture.handler, token, "PUT", "/_internal/admin/aliases/legacy-route",
		`{"targets":[{"provider":"legacy","model":"m"}]}`)
	if w.Code != 400 || !strings.Contains(w.Body.String(), "disabled") {
		t.Fatalf("repaired but disabled local target = %d %s", w.Code, w.Body.String())
	}
	w, _ = adminReasoningRequest(t, fixture.handler, token, "PUT", "/_internal/admin/providers/legacy",
		`{"enabled":true,"api_key":"own-fixture"}`)
	if w.Code != 200 {
		t.Fatalf("enable repaired provider = %d %s", w.Code, w.Body.String())
	}
	w, _ = adminReasoningRequest(t, fixture.handler, token, "PUT", "/_internal/admin/aliases/legacy-route",
		`{"targets":[{"provider":"legacy","model":"m"}]}`)
	if w.Code != 200 {
		t.Fatalf("local alias recovery consulted foreign credentials: %d %s", w.Code, w.Body.String())
	}
}
