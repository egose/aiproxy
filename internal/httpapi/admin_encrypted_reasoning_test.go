package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"testing"
)

func TestAdminEncryptedReasoningCreateDeclarations(t *testing.T) {
	for _, policy := range []string{
		"", "null", "{}", `{"passthrough":false}`, `{"passthrough":true}`,
		`{"on_caller_mismatch":"strip_and_retry","match_messages":[" Customized Caller Mismatch "]}`,
		`{"on_caller_mismatch":"","match_messages":[]}`,
	} {
		t.Run(policy, func(t *testing.T) {
			fixture := newAdminReasoningFixture(t, nil)
			fixture.runtime.EncryptedReasoning = adminReasoningPolicy(t, `{"passthrough":false}`)
			if err := fixture.reload(); err != nil {
				t.Fatal(err)
			}
			member := ""
			if policy != "" {
				member = `,"encrypted_reasoning":` + policy
			}
			raw := `{"name":"declared","type":"openai","api_key":"local-private-credential"` + member +
				`,"models":[{"name":"m"` + member + `},{"name":"fallback"}]}`
			view := requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers", raw, 201)
			local, effective, source := policy, policy, "provider"
			if policy == "" || policy == "null" {
				local, effective, source = "null", `{"passthrough":false}`, "root"
			}
			assertAdminReasoningView(t, view, local, effective, source, false)
			modelSource := source
			if source == "provider" {
				modelSource = "model"
			}
			assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), local, effective, modelSource, false)
			assertAdminReasoningView(t, adminReasoningModelView(t, view, "fallback"), "null", effective, source, false)
			row, models := adminReasoningSavedProvider(t, fixture.store, "declared")
			assertAdminReasoningStored(t, row.EncryptedReasoning, local)
			for _, model := range models {
				expected := "null"
				if model.Name == "m" {
					expected = local
				}
				assertAdminReasoningStored(t, model.EncryptedReasoning, expected)
			}
			reopened := requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/providers/declared", "", 200)
			if !reflect.DeepEqual(view, reopened) {
				t.Fatal("reopening the provider changed declarations or effective metadata")
			}
			if view["models_inherited"] != false {
				t.Fatal("concrete definitions reported as inherited")
			}
		})
	}
}

func TestAdminEncryptedReasoningProviderAndModelUpdateIntent(t *testing.T) {
	fixture := newAdminReasoningFixture(t, nil)
	fixture.runtime.EncryptedReasoning = adminReasoningPolicy(t, `{"on_caller_mismatch":"strip_and_retry"}`)
	if err := fixture.reload(); err != nil {
		t.Fatal(err)
	}
	view := requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"editable","type":"openai","api_key":"local-private-credential","models":[{"name":"m","upstream_name":"served-m","capabilities":["chat"],"pricing":{"input_per_million":2}},{"name":"sticky","encrypted_reasoning":{"passthrough":false}}]}`, 201)
	assertAdminReasoningView(t, view, "null", `{"on_caller_mismatch":"strip_and_retry"}`, "root", false)
	path := "/_internal/admin/providers/editable"
	for _, policy := range []string{
		"{}", `{"passthrough":false}`, `{"passthrough":true,"on_caller_mismatch":"strip_and_retry","match_messages":[" Custom Mismatch "]}`,
		`{"on_caller_mismatch":"strip_and_retry"}`, `{"match_messages":[]}`, "null",
	} {
		t.Run("provider/"+policy, func(t *testing.T) {
			before, oldModels := adminReasoningSavedProvider(t, fixture.store, "editable")
			view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"encrypted_reasoning":`+policy+`}`, 200)
			effective, source := policy, "provider"
			if policy == "null" {
				effective, source = `{"on_caller_mismatch":"strip_and_retry"}`, "root"
			}
			assertAdminReasoningView(t, view, policy, effective, source, false)
			assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), "null", effective, source, false)
			assertAdminReasoningView(t, adminReasoningModelView(t, view, "sticky"), `{"passthrough":false}`, `{"passthrough":false}`, "model", false)
			saved, models := adminReasoningSavedProvider(t, fixture.store, "editable")
			assertAdminReasoningStored(t, saved.EncryptedReasoning, policy)
			if !reflect.DeepEqual(oldModels, models) || !bytes.Equal(before.APIKeyEncrypted, saved.APIKeyEncrypted) {
				t.Fatal("provider policy edit changed model rows or credentials")
			}
			for _, noPolicy := range []string{`{}`, `{"display_name":"Unrelated"}`, `{"models":null}`} {
				view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path, noPolicy, 200)
				after, afterModels := adminReasoningSavedProvider(t, fixture.store, "editable")
				if !bytes.Equal(saved.EncryptedReasoning, after.EncryptedReasoning) || !reflect.DeepEqual(models, afterModels) {
					t.Fatal("omitted policy or null model collection did not preserve exact local declarations")
				}
			}
		})
	}
	requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"encrypted_reasoning":{"passthrough":false}}`, 200)
	for _, policy := range []string{
		"{}", `{"on_caller_mismatch":"strip_and_retry","match_messages":[" Model Mismatch "]}`,
		`{"passthrough":false}`, "", "null",
	} {
		t.Run("model/"+policy, func(t *testing.T) {
			member := ""
			local, effective, source := "null", `{"passthrough":false}`, "provider"
			if policy != "" {
				member = `,"encrypted_reasoning":` + policy
			}
			if policy != "" && policy != "null" {
				local, effective, source = policy, policy, "model"
			}
			view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path,
				`{"models":[{"name":"m","upstream_name":"served-m"`+member+`},{"name":"sticky","encrypted_reasoning":{"passthrough":false}}]}`, 200)
			assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), local, effective, source, false)
			saved, models := adminReasoningSavedProvider(t, fixture.store, "editable")
			assertAdminReasoningStored(t, saved.EncryptedReasoning, `{"passthrough":false}`)
			for _, model := range models {
				if model.Name == "m" {
					assertAdminReasoningStored(t, model.EncryptedReasoning, local)
				}
			}
		})
	}
	view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path,
		`{"encrypted_reasoning":null,"models":[{"name":"m","encrypted_reasoning":{}},{"name":"fallback"}]}`, 200)
	assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), "{}", "{}", "model", false)
	assertAdminReasoningView(t, adminReasoningModelView(t, view, "fallback"), "null", `{"on_caller_mismatch":"strip_and_retry"}`, "root", false)
	before, models := adminReasoningSavedProvider(t, fixture.store, "editable")
	fixture.runtime.EncryptedReasoning = adminReasoningPolicy(t, `{"passthrough":false}`)
	if err := fixture.reload(); err != nil {
		t.Fatal(err)
	}
	view = requireAdminReasoningStatus(t, fixture, http.MethodGet, path, "", 200)
	assertAdminReasoningView(t, view, "null", `{"passthrough":false}`, "root", false)
	assertAdminReasoningView(t, adminReasoningModelView(t, view, "fallback"), "null", `{"passthrough":false}`, "root", false)
	assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), "{}", "{}", "model", false)
	assertAdminReasoningProviderUnchanged(t, fixture.store, before, models)
	fixture.runtime.EncryptedReasoning = nil
	if err := fixture.reload(); err != nil {
		t.Fatal(err)
	}
	view = requireAdminReasoningStatus(t, fixture, http.MethodGet, path, "", 200)
	assertAdminReasoningView(t, view, "null", "{}", "builtin", false)
	assertAdminReasoningProviderUnchanged(t, fixture.store, before, models)
}

func TestAdminEncryptedReasoningAliasIntent(t *testing.T) {
	fixture := newAdminReasoningFixture(t, nil)
	fixture.runtime.EncryptedReasoning = adminReasoningPolicy(t, `{"passthrough":false}`)
	if err := fixture.reload(); err != nil {
		t.Fatal(err)
	}
	path := "/_internal/admin/aliases/routing"
	view := requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/aliases",
		`{"name":"routing","targets":[{"provider":"openai","model":"gpt-4o-mini"}]}`, 201)
	if local, present := view["encrypted_reasoning"]; !present || local != nil || view["encrypted_reasoning_effective"] != nil {
		t.Fatal("undeclared alias reported a target-independent effective policy")
	}
	for _, policy := range []string{
		"{}", `{"passthrough":false}`, `{"on_caller_mismatch":"strip_and_retry","match_messages":[" Alias Mismatch "]}`, "null",
	} {
		t.Run(policy, func(t *testing.T) {
			view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"encrypted_reasoning":`+policy+`}`, 200)
			var expected any
			if err := json.Unmarshal([]byte(policy), &expected); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(view["encrypted_reasoning"], expected) {
				t.Fatalf("alias local declaration was default-filled: %#v", view)
			}
			if policy == "null" {
				if view["encrypted_reasoning_effective"] != nil {
					t.Fatal("removed alias retained an effective override")
				}
			} else {
				raw, _ := json.Marshal(adminReasoningEffective(*adminReasoningPolicy(t, policy)))
				var effective any
				_ = json.Unmarshal(raw, &effective)
				if !reflect.DeepEqual(view["encrypted_reasoning_effective"], effective) {
					t.Fatalf("alias effective policy did not normalize: %#v", view)
				}
			}
			saved, err := fixture.store.GetAlias(context.Background(), "routing")
			if err != nil {
				t.Fatal(err)
			}
			assertAdminReasoningStored(t, saved.EncryptedReasoning, policy)
			requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"algorithm":"least_connections"}`, 200)
			after, _ := fixture.store.GetAlias(context.Background(), "routing")
			if !bytes.Equal(saved.EncryptedReasoning, after.EncryptedReasoning) {
				t.Fatal("omitted alias policy did not preserve exact sparse intent")
			}
			reopened := requireAdminReasoningStatus(t, fixture, http.MethodGet, path, "", 200)
			if !reflect.DeepEqual(reopened["encrypted_reasoning"], view["encrypted_reasoning"]) {
				t.Fatal("reopened alias lost local intent")
			}
		})
	}
	legacy := json.RawMessage(`{"passthrough":true,"on_caller_mismatch":"strip_and_retry","match_messages":["legacy caller mismatch"]}`)
	if _, err := fixture.store.DB.ExecContext(context.Background(),
		`UPDATE db_aliases SET encrypted_reasoning = ?::jsonb WHERE name = ?`, string(legacy), "routing"); err != nil {
		t.Fatal(err)
	}
	requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{}`, 200)
	saved, _ := fixture.store.GetAlias(context.Background(), "routing")
	assertAdminReasoningStored(t, saved.EncryptedReasoning, string(legacy))
	view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"encrypted_reasoning":null}`, 200)
	if view["encrypted_reasoning"] != nil || view["encrypted_reasoning_effective"] != nil {
		t.Fatal("explicit null did not remove a legacy nonnull override")
	}
	saved, _ = fixture.store.GetAlias(context.Background(), "routing")
	assertAdminReasoningStored(t, saved.EncryptedReasoning, "null")
}

func TestAdminEncryptedReasoningCredentialPreservationAndValidation(t *testing.T) {
	fixture := newAdminReasoningFixture(t, nil)
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"credentialed","type":"openai","api_key":"original-private-token","encrypted_reasoning":{"on_caller_mismatch":"strip_and_retry"},"models":[{"name":"m","encrypted_reasoning":{}},{"name":"other","encrypted_reasoning":{"passthrough":false}}]}`, 201)
	before, models := adminReasoningSavedProvider(t, fixture.store, "credentialed")
	path := "/_internal/admin/providers/credentialed/credential"
	requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"api_key":"rotated-private-token"}`, 200)
	saved, afterModels := adminReasoningSavedProvider(t, fixture.store, "credentialed")
	if !bytes.Equal(before.EncryptedReasoning, saved.EncryptedReasoning) || !reflect.DeepEqual(models, afterModels) || bytes.Equal(before.APIKeyEncrypted, saved.APIKeyEncrypted) {
		t.Fatal("credential update did not retain provider/model policy intent")
	}
	for _, edit := range []string{
		`"encrypted_reasoning":null`, `"encrypted_reasoning":{}`, `"Encrypted_Reasoning":{"passthrough":false}`,
		`"encrypted_reasoning_effective":null`, `"models":null`, `"models":[]`,
		`"models_inherited":true`,
		`"models":[{"name":"m","encrypted_reasoning":{}}]`,
		`"models":[{"name":"m","encrypted_reasoning":{}}],"models":null`,
		`"api_key":"other-private-token"`,
	} {
		t.Run(edit, func(t *testing.T) {
			count := fixture.activations
			w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, path, `{"api_key":"rejected-private-token",`+edit+`}`)
			if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count || bytes.Contains(w.Body.Bytes(), []byte("private-token")) {
				t.Fatalf("credential endpoint accepted or disclosed an edit: %d %s", w.Code, w.Body.String())
			}
			assertAdminReasoningProviderUnchanged(t, fixture.store, saved, afterModels)
		})
	}
	for _, column := range []string{"provider", "model"} {
		t.Run("corrupt/"+column, func(t *testing.T) {
			table, where := "db_providers", `name = 'credentialed'`
			if column == "model" {
				table, where = "db_provider_models", fmt.Sprintf(`provider_id = '%s' AND name = 'm'`, saved.ID)
			}
			query := `UPDATE ` + table + ` SET encrypted_reasoning = '{"passthrough":null}'::jsonb WHERE ` + where
			if _, err := fixture.store.DB.ExecContext(context.Background(), query); err != nil {
				t.Fatal(err)
			}
			corrupt, corruptModels := adminReasoningSavedProvider(t, fixture.store, "credentialed")
			count := fixture.activations
			w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, path, `{"api_key":"invalid-private-token"}`)
			if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count {
				t.Fatalf("credential rebuild accepted a corrupt stored policy: %d %s", w.Code, w.Body.String())
			}
			assertAdminReasoningProviderUnchanged(t, fixture.store, corrupt, corruptModels)
			var restore string
			if column == "provider" {
				restore = string(saved.EncryptedReasoning)
			} else {
				restore = "{}"
			}
			if _, err := fixture.store.DB.ExecContext(context.Background(), `UPDATE `+table+` SET encrypted_reasoning = ?::jsonb WHERE `+where, restore); err != nil {
				t.Fatal(err)
			}
		})
	}
}
