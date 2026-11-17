package app

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
)

func managedReasoningRuntimeFixture(t *testing.T) (*App, string, string, *appReasoningRequests, func(string, string, string) *httptest.ResponseRecorder) {
	t.Helper()
	a, path, contents, request := catalogRuntimeFixture(t)
	observations := &appReasoningRequests{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		observations.record(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[],"output":[]}`)
	}))
	t.Cleanup(upstream.Close)
	local, _ := a.Config.Catalog.Provider("local")
	contents = strings.ReplaceAll(contents, local.BaseURL, upstream.URL) + "\nencrypted_reasoning { passthrough = false }\n"
	rewriteConfigFile(t, path, contents)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	return a, path, contents, observations, request
}

func requireManagedReasoningAdmin(t *testing.T, request func(string, string, string) *httptest.ResponseRecorder, method, path, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	w := request(method, path, body)
	if w.Code != status {
		t.Fatalf("%s %s: got=%d want=%d body=%s", method, path, w.Code, status, w.Body.String())
	}
	if strings.Contains(path, "/providers") && method != "GET" && status < 300 && w.Header().Get("X-Aiproxy-Catalog-Saved") != "true" {
		t.Fatal("managed runtime mutation lost the durability header")
	}
	return w
}

func assertManagedReasoningInference(t *testing.T, observations *appReasoningRequests, request func(string, string, string) *httptest.ResponseRecorder, model, upstream, credential string, stripped bool) {
	t.Helper()
	body, _ := appReasoningBodies("chat/completions", model)
	before := len(observations.snapshot())
	w := request("POST", "/v1/chat/completions", string(body))
	if w.Code != 200 {
		t.Fatalf("managed inference = %d %s", w.Code, w.Body.String())
	}
	calls := observations.snapshot()[before:]
	if len(calls) != 1 {
		t.Fatalf("managed inference made %d calls, want one", len(calls))
	}
	call := calls[0]
	var object map[string]any
	if err := json.Unmarshal(call.body, &object); err != nil {
		t.Fatal(err)
	}
	if object["model"] != upstream || call.key != "Bearer "+credential || bytes.Contains(call.body, []byte("encrypted_content")) == stripped {
		t.Fatalf("managed policy, model or credential generation mismatch: model=%s stripped=%v call=%+v", model, stripped, call)
	}
}

func TestEncryptedReasoningManagedAdminReloadPolicyAndAliasRemoval(t *testing.T) {
	a, path, contents, observations, request := managedReasoningRuntimeFixture(t)
	local, _ := a.Config.Catalog.Provider("local")
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/providers",
		fmt.Sprintf(`{"name":"managed","type":"openai-compatible","base_url":%q,"api_key":"managed-private-credential","encrypted_reasoning":{},"models":[{"name":"m","upstream_name":"served-m","encrypted_reasoning":{"passthrough":false}},{"name":"n","upstream_name":"served-n"}]}`, local.BaseURL), 201)
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/providers",
		`{"name":"managed-child","type":"openai-compatible","extends":"managed","api_key":"child-private-credential","models":[{"name":"m","encrypted_reasoning":{}}]}`, 201)
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/aliases",
		`{"name":"managed-alias","targets":[{"provider":"managed-child","model":"m"}]}`, 201)
	assertManagedReasoningInference(t, observations, request, "managed/m", "served-m", "managed-private-credential", true)
	assertManagedReasoningInference(t, observations, request, "managed-child/m", "served-m", "child-private-credential", false)
	assertManagedReasoningInference(t, observations, request, "managed-child/n", "served-n", "child-private-credential", false)
	childPath := "/_internal/admin/providers/managed-child"
	requireManagedReasoningAdmin(t, request, "PUT", childPath, `{"models":null,"display_name":"Unchanged intent"}`, 200)
	assertManagedReasoningInference(t, observations, request, "managed-child/m", "served-m", "child-private-credential", false)
	requireManagedReasoningAdmin(t, request, "PUT", childPath, `{"models":[{"name":"m","encrypted_reasoning":null}]}`, 200)
	assertManagedReasoningInference(t, observations, request, "managed-child/m", "served-m", "child-private-credential", true)
	assertManagedReasoningInference(t, observations, request, "alias/managed-alias", "served-m", "child-private-credential", true)
	aliasPath := "/_internal/admin/aliases/managed-alias"
	requireManagedReasoningAdmin(t, request, "PUT", aliasPath, `{"encrypted_reasoning":{}}`, 200)
	assertManagedReasoningInference(t, observations, request, "alias/managed-alias", "served-m", "child-private-credential", false)
	requireManagedReasoningAdmin(t, request, "PUT", aliasPath, `{"algorithm":"least_connections"}`, 200)
	assertManagedReasoningInference(t, observations, request, "alias/managed-alias", "served-m", "child-private-credential", false)
	requireManagedReasoningAdmin(t, request, "PUT", aliasPath, `{"encrypted_reasoning":null}`, 200)
	assertManagedReasoningInference(t, observations, request, "alias/managed-alias", "served-m", "child-private-credential", true)
	requireManagedReasoningAdmin(t, request, "PUT", childPath, `{"encrypted_reasoning":{"passthrough":false}}`, 200)
	assertManagedReasoningInference(t, observations, request, "managed-child/n", "served-n", "child-private-credential", true)
	requireManagedReasoningAdmin(t, request, "PUT", childPath, `{"encrypted_reasoning":null}`, 200)
	assertManagedReasoningInference(t, observations, request, "managed-child/n", "served-n", "child-private-credential", false)
	requireManagedReasoningAdmin(t, request, "PUT", "/_internal/admin/providers/managed", `{"encrypted_reasoning":null}`, 200)
	assertManagedReasoningInference(t, observations, request, "managed-child/n", "served-n", "child-private-credential", true)
	contents = strings.ReplaceAll(contents, "encrypted_reasoning { passthrough = false }", "encrypted_reasoning { passthrough = true }")
	rewriteConfigFile(t, path, contents)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	assertManagedReasoningInference(t, observations, request, "managed-child/n", "served-n", "child-private-credential", false)
	assertManagedReasoningInference(t, observations, request, "managed-child/m", "served-m", "child-private-credential", true)
	child, err := a.adminStore.GetProvider(context.Background(), "managed-child")
	if err != nil {
		t.Fatal(err)
	}
	models, err := a.adminStore.ListProviderModels(context.Background(), child.ID)
	if err != nil || len(child.EncryptedReasoning) != 0 || len(models) != 0 {
		t.Fatalf("live root fallback was persisted as child-local intent: %+v %+v %v", child, models, err)
	}
	w := requireManagedReasoningAdmin(t, request, "GET", childPath, "", 200)
	var view map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
		t.Fatal(err)
	}
	if view["encrypted_reasoning"] != nil || view["encrypted_reasoning_source"] != "root" || view["encrypted_reasoning_inherited"] != false || view["models_inherited"] != true {
		t.Fatalf("managed live-root view metadata mismatch: %#v", view)
	}
}

func TestEncryptedReasoningManagedCorruptRowsAndDanglingOverlayRollback(t *testing.T) {
	a, _, _, observations, request := managedReasoningRuntimeFixture(t)
	ctx := context.Background()
	local, _ := a.Config.Catalog.Provider("local")
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/providers",
		fmt.Sprintf(`{"name":"managed-base","type":"openai-compatible","base_url":%q,"api_key":"base-private-credential","encrypted_reasoning":{},"models":[{"name":"m","upstream_name":"served-m","encrypted_reasoning":{"passthrough":false}},{"name":"n","upstream_name":"served-n"}]}`, local.BaseURL), 201)
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/providers",
		`{"name":"managed-child","type":"openai-compatible","extends":"managed-base","api_key":"child-private-credential","models":[{"name":"m","encrypted_reasoning":{}}]}`, 201)
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/aliases",
		`{"name":"unrelated","encrypted_reasoning":{},"targets":[{"provider":"local","model":"old"}]}`, 201)
	base, err := a.adminStore.GetProvider(ctx, "managed-base")
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"provider", "model", "alias"} {
		t.Run(owner, func(t *testing.T) {
			table, where, restore := "db_providers", `name = 'managed-base'`, "{}"
			if owner == "model" {
				table, where, restore = "db_provider_models", fmt.Sprintf(`provider_id = '%s' AND name = 'm'`, base.ID), `{"passthrough":false}`
			} else if owner == "alias" {
				table, where = "db_aliases", `name = 'unrelated'`
			}
			if _, err := a.adminStore.DB.ExecContext(ctx, `UPDATE `+table+` SET encrypted_reasoning = '{"passthrough":null}'::jsonb WHERE `+where); err != nil {
				t.Fatal(err)
			}
			oldRuntime, oldResolver, oldClients := a.Config, a.resolver, a.clients
			admittedProvider, admittedModel, ok := oldResolver.Model("managed-child", "m")
			if !ok {
				t.Fatal("old admitted child is absent")
			}
			admitted := oldResolver.EncryptedReasoning(admittedProvider, admittedModel, nil)
			if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "encrypted_reasoning") {
				t.Fatalf("corrupt managed %s policy activated: %v", owner, err)
			}
			if a.Config != oldRuntime || a.resolver != oldResolver || a.clients != oldClients || !reflect.DeepEqual(admitted, oldResolver.EncryptedReasoning(admittedProvider, admittedModel, nil)) {
				t.Fatal("failed managed reload changed runtime resources or admitted policy")
			}
			assertManagedReasoningInference(t, observations, request, "managed-child/m", "served-m", "child-private-credential", false)
			if owner != "alias" {
				before, _ := a.adminStore.GetProvider(ctx, "managed-base")
				w := request("PUT", "/_internal/admin/providers/managed-base/credential", `{"api_key":"rejected-private-credential"}`)
				after, _ := a.adminStore.GetProvider(ctx, "managed-base")
				if w.Code != 400 || w.Header().Get("X-Aiproxy-Catalog-Saved") != "" || !bytes.Equal(before.APIKeyEncrypted, after.APIKeyEncrypted) || !before.UpdatedAt.Equal(after.UpdatedAt) {
					t.Fatalf("credential rebuild committed despite invalid stored policy: %d %s", w.Code, w.Body.String())
				}
			} else {
				w := request("PUT", "/_internal/admin/providers/managed-child", `{"encrypted_reasoning":{"passthrough":false}}`)
				if w.Code != 500 || w.Header().Get("X-Aiproxy-Catalog-Saved") != "true" || w.Body.String() != "saved but activation failed\n" || a.Config != oldRuntime || a.resolver != oldResolver {
					t.Fatalf("valid declaration save lost the durable/activation distinction: %d %s", w.Code, w.Body.String())
				}
				w = requireManagedReasoningAdmin(t, request, "GET", "/_internal/admin/providers/managed-child", "", 200)
				var view map[string]any
				if err := json.Unmarshal(w.Body.Bytes(), &view); err != nil {
					t.Fatal(err)
				}
				local, _ := view["encrypted_reasoning"].(map[string]any)
				if local["passthrough"] != false || view["encrypted_reasoning_inherited"] != false {
					t.Fatal("saved-intent read falsely returned an older activated declaration")
				}
				assertManagedReasoningInference(t, observations, request, "managed-child/n", "served-n", "child-private-credential", false)
			}
			if _, err := a.adminStore.DB.ExecContext(ctx, `UPDATE `+table+` SET encrypted_reasoning = ?::jsonb WHERE `+where, restore); err != nil {
				t.Fatal(err)
			}
			if err := a.Reload(); err != nil {
				t.Fatal(err)
			}
			if owner == "alias" {
				requireManagedReasoningAdmin(t, request, "PUT", "/_internal/admin/providers/managed-child", `{"encrypted_reasoning":null}`, 200)
			}
		})
	}
	oldRuntime, oldResolver, oldClients := a.Config, a.resolver, a.clients
	w := request("PUT", "/_internal/admin/providers/managed-base", `{"models":[{"name":"n","upstream_name":"saved-n"}]}`)
	if w.Code != 500 || w.Header().Get("X-Aiproxy-Catalog-Saved") != "true" || w.Body.String() != "saved but activation failed\n" {
		t.Fatalf("dangling overlay save/activation result = %d %s", w.Code, w.Body.String())
	}
	if a.Config != oldRuntime || a.resolver != oldResolver || a.clients != oldClients {
		t.Fatal("dangling overlay reload replaced the old runtime")
	}
	baseModels, err := a.adminStore.ListProviderModels(ctx, base.ID)
	if err != nil || len(baseModels) != 1 || baseModels[0].Name != "n" || baseModels[0].UpstreamName != "saved-n" {
		t.Fatalf("failed activation rolled back a durable base save: %+v %v", baseModels, err)
	}
	child, _ := a.adminStore.GetProvider(ctx, "managed-child")
	overlays, err := a.adminStore.ListProviderModels(ctx, child.ID)
	if err != nil || len(overlays) != 1 || overlays[0].Name != "m" {
		t.Fatal("dangling overlay was silently dropped")
	}
	assertManagedReasoningInference(t, observations, request, "managed-child/m", "served-m", "child-private-credential", false)
	if err := a.Reload(); err == nil || a.Config != oldRuntime || a.resolver != oldResolver {
		t.Fatal("repeated dangling overlay reload did not preserve the old runtime")
	}
	w = request("GET", "/_internal/admin/providers/managed-child", "")
	if w.Code != 500 {
		t.Fatal("saved-intent read falsely described the old activated inventory as the saved candidate")
	}
	requireManagedReasoningAdmin(t, request, "PUT", "/_internal/admin/providers/managed-child", `{"models":[]}`, 200)
	if _, _, ok := a.resolver.Model("managed-child", "m"); ok {
		t.Fatal("overlay repair kept a removed inherited model in the new runtime")
	}
	if _, _, ok := oldResolver.Model("managed-child", "m"); !ok {
		t.Fatal("overlay repair mutated an old admitted inventory")
	}
	assertManagedReasoningInference(t, observations, request, "managed-child/n", "saved-n", "child-private-credential", false)
	overlays, err = a.adminStore.ListProviderModels(ctx, child.ID)
	if err != nil || len(overlays) != 0 {
		t.Fatal("explicit repair did not remove local overlays")
	}
	p, m, _ := a.resolver.Model("managed-child", "n")
	resolved := a.resolver.EncryptedReasoning(p, m, nil)
	if resolved.Source != config.EncryptedReasoningSourceProvider || !resolved.Inherited || !resolved.Policy.Passthrough {
		t.Fatalf("base provider reset was not inherited after repair: %+v", resolved)
	}
}

func TestEncryptedReasoningManagedBaseDeletionRejectsStaticFallback(t *testing.T) {
	a, _, _, observations, request := managedReasoningRuntimeFixture(t)
	ctx := context.Background()
	local, _ := a.Config.Catalog.Provider("local")
	baseBody := fmt.Sprintf(`{"name":"removed-base","type":"openai-compatible","base_url":%q,"api_key":"base-private-credential","models":[{"name":"m","upstream_name":"served-m","encrypted_reasoning":{"passthrough":false}}]}`, local.BaseURL)
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/providers", baseBody, 201)
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/providers",
		`{"name":"draining-child","type":"openai-compatible","extends":"removed-base","api_key":"child-private-credential","models":[{"name":"m","encrypted_reasoning":{}}]}`, 201)
	oldRuntime, oldResolver := a.Config, a.resolver
	w := request("DELETE", "/_internal/admin/providers/removed-base", "")
	if w.Code != 500 || !strings.HasPrefix(w.Body.String(), "saved but activation failed: ") {
		t.Fatalf("base removal activation result = %d %s", w.Code, w.Body.String())
	}
	if _, err := a.adminStore.GetProvider(ctx, "removed-base"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("base removal was not durable")
	}
	if a.Config != oldRuntime || a.resolver != oldResolver {
		t.Fatal("base removal replaced the old admitted runtime")
	}
	assertManagedReasoningInference(t, observations, request, "draining-child/m", "served-m", "child-private-credential", false)
	if err := a.Reload(); err == nil || a.Config != oldRuntime || a.resolver != oldResolver {
		t.Fatal("deleted managed base was reused as a static fallback during reload")
	}
	w = request("GET", "/_internal/admin/providers/draining-child", "")
	if w.Code != 500 {
		t.Fatal("saved child read used a deleted managed base as static metadata")
	}
	w = request("POST", "/_internal/admin/aliases",
		`{"name":"rejected-target","targets":[{"provider":"draining-child","model":"m"}]}`)
	if w.Code != 400 {
		t.Fatalf("alias validator reused a deleted managed base: %d %s", w.Code, w.Body.String())
	}
	if _, err := a.adminStore.GetAlias(ctx, "rejected-target"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatal("invalid alias committed against a deleted base")
	}
	requireManagedReasoningAdmin(t, request, "POST", "/_internal/admin/providers", baseBody, 201)
	assertManagedReasoningInference(t, observations, request, "draining-child/m", "served-m", "child-private-credential", false)
	p, _, ok := a.resolver.Model("draining-child", "m")
	if !ok || !p.DatabaseManaged || p.Extends != "removed-base" {
		t.Fatal("recreated managed provider lost its origin or inheritance provenance")
	}
}
