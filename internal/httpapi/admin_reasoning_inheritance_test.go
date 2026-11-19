package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/store"
)

func adminReasoningStaticRuntime(t *testing.T) *config.Runtime {
	t.Helper()
	rt, err := config.Load([]byte(`
listener "http" "public" { address = "127.0.0.1:8080" }
auth "main" { mode = "none" }
encrypted_reasoning { passthrough = false }
provider "openai-compatible" "base" {
  base_url = "https://upstream.example/v1"
  api_key = "static-private-credential"
  encrypted_reasoning { on_caller_mismatch = "strip_and_retry" }
  model "m" {
    display_name = "Model M"
    upstream_name = "served-m"
    capabilities = ["chat", "responses"]
    encrypted_reasoning { passthrough = false }
    pricing { input_per_million = 2 }
  }
  model "unconfigured" {}
  model "reset" {
    encrypted_reasoning {}
  }
}
provider "openai-compatible" "static-copy" {
  extends = "base"
  api_key = "copy-private-credential"
}
provider "openai-compatible" "static-local" {
  extends = "base"
  api_key = "local-private-credential"
  encrypted_reasoning {}
  model "m" {
    encrypted_reasoning {}
  }
}
provider "openai-compatible" "blank-base" {
  base_url = "https://upstream.example/v1"
  api_key = "blank-private-credential"
  model "unconfigured" {}
}
provider "openai-compatible" "blank-copy" {
  extends = "blank-base"
  api_key = "blank-copy-private-credential"
}
`), "admin-reasoning.hcl")
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestAdminEncryptedReasoningStaticLocalAndInheritedMetadata(t *testing.T) {
	fixture := newAdminReasoningFixture(t, adminReasoningStaticRuntime(t))
	base := requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/providers/base", "", 200)
	assertAdminReasoningView(t, base, `{"on_caller_mismatch":"strip_and_retry"}`, `{"on_caller_mismatch":"strip_and_retry"}`, "provider", false)
	copy := requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/providers/static-copy", "", 200)
	if copy["extends"] != "base" || copy["models_inherited"] != true {
		t.Fatal("static definition inheritance provenance was lost")
	}
	assertAdminReasoningView(t, copy, "null", `{"on_caller_mismatch":"strip_and_retry"}`, "provider", true)
	assertAdminReasoningView(t, adminReasoningModelView(t, copy, "m"), "null", `{"passthrough":false}`, "model", true)
	assertAdminReasoningView(t, adminReasoningModelView(t, copy, "unconfigured"), "null", `{"on_caller_mismatch":"strip_and_retry"}`, "provider", true)
	assertAdminReasoningView(t, adminReasoningModelView(t, copy, "reset"), "null", "{}", "model", true)
	local := requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/providers/static-local", "", 200)
	assertAdminReasoningView(t, local, "{}", "{}", "provider", false)
	assertAdminReasoningView(t, adminReasoningModelView(t, local, "m"), "{}", "{}", "model", false)
	assertAdminReasoningView(t, adminReasoningModelView(t, local, "reset"), "null", "{}", "model", true)
	blank := requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/providers/blank-copy", "", 200)
	assertAdminReasoningView(t, blank, "null", `{"passthrough":false}`, "root", false)
	assertAdminReasoningView(t, adminReasoningModelView(t, blank, "unconfigured"), "null", `{"passthrough":false}`, "root", false)
}

func TestAdminEncryptedReasoningManagedInheritanceInventoryAndRemoval(t *testing.T) {
	for _, kind := range []string{"static-base", "managed-base"} {
		t.Run(kind, func(t *testing.T) {
			rt := adminReasoningStaticRuntime(t)
			fixture := newAdminReasoningFixture(t, rt)
			base := "base"
			if kind == "managed-base" {
				base = "managed-base"
				requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
					`{"name":"managed-base","type":"openai-compatible","base_url":"https://upstream.example/v1","api_key":"base-private-credential","encrypted_reasoning":{"on_caller_mismatch":"strip_and_retry"},"models":[{"name":"m","display_name":"Model M","upstream_name":"served-m","capabilities":["chat","responses"],"pricing":{"input_per_million":2},"encrypted_reasoning":{"passthrough":false}},{"name":"unconfigured"},{"name":"reset","encrypted_reasoning":{}}]}`, 201)
			}
			path := "/_internal/admin/providers/child"
			view := requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
				`{"name":"child","type":"openai-compatible","api_key":"child-private-credential","extends":"`+base+`","encrypted_reasoning":{},"models":[{"name":"m","encrypted_reasoning":{}}]}`, 201)
			if view["models_inherited"] != true || len(view["models"].([]any)) != 3 {
				t.Fatal("derivative GET exposed overlays instead of the complete inherited inventory")
			}
			assertAdminReasoningView(t, view, "{}", "{}", "provider", false)
			m := adminReasoningModelView(t, view, "m")
			assertAdminReasoningView(t, m, "{}", "{}", "model", false)
			if m["display_name"] != "Model M" || m["upstream_name"] != "served-m" || m["pricing"].(map[string]any)["input_per_million"] != float64(2) {
				t.Fatalf("inherited definitions missing from model view: %#v", m)
			}
			row, models := adminReasoningSavedProvider(t, fixture.store, "child")
			if len(models) != 1 || models[0].Name != "m" || models[0].UpstreamName != "" || models[0].DisplayName != "" || models[0].Protocol != "" || len(models[0].Capabilities) != 0 || len(models[0].Pricing) != 0 {
				t.Fatalf("inherited definitions were persisted as local rows: %#v", models)
			}
			assertAdminReasoningStored(t, models[0].EncryptedReasoning, "{}")
			requireAdminReasoningStatus(t, fixture, http.MethodPut, path+"/credential", `{"api_key":"rotated-child-private-credential"}`, 200)
			credentialed, credentialModels := adminReasoningSavedProvider(t, fixture.store, "child")
			if !bytes.Equal(row.EncryptedReasoning, credentialed.EncryptedReasoning) || !reflect.DeepEqual(models, credentialModels) || bytes.Equal(row.APIKeyEncrypted, credentialed.APIKeyEncrypted) {
				t.Fatal("derived credential update changed local policy intent or inherited base credentials")
			}
			for _, raw := range []string{`{}`, `{"models":null}`, `{"display_name":"Child"}`} {
				requireAdminReasoningStatus(t, fixture, http.MethodPut, path, raw, 200)
				after, afterModels := adminReasoningSavedProvider(t, fixture.store, "child")
				if !bytes.Equal(row.EncryptedReasoning, after.EncryptedReasoning) || !reflect.DeepEqual(models, afterModels) {
					t.Fatal("omission/null changed sparse overlays")
				}
			}
			view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"models":[{"name":"m"}]}`, 200)
			assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), "null", `{"passthrough":false}`, "model", true)
			_, rows := adminReasoningSavedProvider(t, fixture.store, "child")
			if len(rows) != 0 {
				t.Fatal("name-only entry was not canonicalized out")
			}
			view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"encrypted_reasoning":null}`, 200)
			assertAdminReasoningView(t, view, "null", `{"on_caller_mismatch":"strip_and_retry"}`, "provider", true)
			assertAdminReasoningView(t, adminReasoningModelView(t, view, "unconfigured"), "null", `{"on_caller_mismatch":"strip_and_retry"}`, "provider", true)
			for _, policy := range []string{`{"passthrough":false}`, `{"on_caller_mismatch":"strip_and_retry","match_messages":[" Child Mismatch "]}`, "{}"} {
				view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"models":[{"name":"m","encrypted_reasoning":`+policy+`}]}`, 200)
				assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), policy, policy, "model", false)
				reopened := requireAdminReasoningStatus(t, fixture, http.MethodGet, path, "", 200)
				if !reflect.DeepEqual(view, reopened) {
					t.Fatal("reopening a sparse overlay changed its intent")
				}
			}
			for _, removal := range []string{`{"models":[{"name":"m","encrypted_reasoning":null}]}`, `{"models":[]}`} {
				view = requireAdminReasoningStatus(t, fixture, http.MethodPut, path, removal, 200)
				assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), "null", `{"passthrough":false}`, "model", true)
				_, rows = adminReasoningSavedProvider(t, fixture.store, "child")
				if len(rows) != 0 || len(view["models"].([]any)) != 3 {
					t.Fatal("clearing local overlays removed inherited definitions or persisted null entries")
				}
			}
			served, ok := fixture.handler.current().Catalog.Provider("child")
			if !ok || len(served.Models) != 3 || len(served.ModelByName) != 3 {
				t.Fatal("read and serve inventory differ")
			}
			for _, model := range served.Models {
				if !reflect.DeepEqual(model, served.ModelByName[model.Name]) {
					t.Fatal("served ordered/indexed inventory diverged")
				}
			}
		})
	}
}

func TestAdminEncryptedReasoningSavedBaseMetadataAndAliasInventory(t *testing.T) {
	fixture := newAdminReasoningFixture(t, nil)
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"saved-base","type":"openai","api_key":"saved-base-private-credential","encrypted_reasoning":{"passthrough":false},"models":[{"name":"m","encrypted_reasoning":{}}]}`, 201)
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"saved-child","type":"openai","extends":"saved-base","api_key":"saved-child-private-credential"}`, 201)
	admitted := fixture.handler.current().Resolver
	deps := fixture.handler.current()
	deps.RequestReload = func() error { return errors.New("private-activation-failure") }
	fixture.handler.UpdateDependencies(deps)
	w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, "/_internal/admin/providers/saved-base",
		`{"encrypted_reasoning":{"on_caller_mismatch":"strip_and_retry"},"models":[{"name":"m","encrypted_reasoning":{"passthrough":false}},{"name":"new-model","upstream_name":"new-served"}]}`)
	if w.Code != 500 || w.Header().Get(catalogSavedHeader) != "true" || w.Body.String() != "saved but activation failed\n" {
		t.Fatalf("durable saved base result = %d %s", w.Code, w.Body.String())
	}
	view := requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/providers/saved-child", "", 200)
	assertAdminReasoningView(t, view, "null", `{"on_caller_mismatch":"strip_and_retry"}`, "provider", true)
	assertAdminReasoningView(t, adminReasoningModelView(t, view, "m"), "null", `{"passthrough":false}`, "model", true)
	if adminReasoningModelView(t, view, "new-model")["upstream_name"] != "new-served" {
		t.Fatal("saved child metadata used an old activated base inventory")
	}
	if _, _, ok := admitted.Model("saved-child", "new-model"); ok {
		t.Fatal("failed activation updated the admitted resolver")
	}
	oldProvider, oldModel, _ := admitted.Model("saved-child", "m")
	oldPolicy := admitted.EncryptedReasoning(oldProvider, oldModel, nil)
	if !oldPolicy.Policy.Passthrough || oldPolicy.Source != config.EncryptedReasoningSourceModel {
		t.Fatal("saved read metadata changed the old admitted policy")
	}
	w, _ = adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPost, "/_internal/admin/aliases",
		`{"name":"saved-alias","targets":[{"provider":"saved-child","model":"new-model"}]}`)
	if w.Code != 500 || w.Body.String() != "saved but activation failed\n" {
		t.Fatalf("alias validation did not see the full saved inherited inventory: %d %s", w.Code, w.Body.String())
	}
	alias, err := fixture.store.GetAlias(context.Background(), "saved-alias")
	if err != nil {
		t.Fatal(err)
	}
	targets, err := fixture.store.ListAliasTargets(context.Background(), alias.ID)
	if err != nil || len(targets) != 1 || targets[0].Model != "new-model" {
		t.Fatal("alias targeting saved inherited model was not durable")
	}
}

func TestAdminEncryptedReasoningLegacyInertRowsAndCredentialFreeViews(t *testing.T) {
	fixture := newAdminReasoningFixture(t, adminReasoningStaticRuntime(t))
	row := store.DBProvider{
		Name: "legacy", Type: "openai-compatible", Extends: "base", Enabled: true,
		WorkspaceID: store.SystemWorkspaceID, APIKeyEncrypted: []byte("invalid-private-ciphertext"),
		APIKeyRefPath: "missing-private-credential-file", APIKeyRefKey: "unused",
	}
	legacy := []store.DBProviderModel{{Name: "obsolete-legacy", UpstreamName: "legacy-definition", Capabilities: []string{"chat"}}}
	if err := fixture.store.CreateProviderAggregate(context.Background(), &row, legacy, store.CatalogEditOptions{Static: fixture.runtime.Catalog}); err != nil {
		t.Fatal(err)
	}
	before, models := adminReasoningSavedProvider(t, fixture.store, "legacy")
	deps := fixture.handler.current()
	deps.RequestReload = nil
	fixture.handler.UpdateDependencies(deps)
	view := requireAdminReasoningStatus(t, fixture, http.MethodGet, "/_internal/admin/providers/legacy", "", 200)
	if len(view["models"].([]any)) != 3 {
		t.Fatal("legacy inert definitions appeared in the materialized inventory")
	}
	assertAdminReasoningView(t, view, "null", `{"on_caller_mismatch":"strip_and_retry"}`, "provider", true)
	assertAdminReasoningProviderUnchanged(t, fixture.store, before, models)
	raw, _ := json.Marshal(view)
	for _, secret := range []string{"invalid-private-ciphertext", "static-private-credential", "base-private-credential"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatal("metadata render exposed credentials")
		}
	}
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/aliases",
		`{"name":"served-target","targets":[{"provider":"legacy","model":"unconfigured"}]}`, 201)
	w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPost, "/_internal/admin/aliases",
		`{"name":"inert-target","targets":[{"provider":"legacy","model":"obsolete-legacy"}]}`)
	if w.Code != 400 {
		t.Fatal("alias validator treated inert stored rows as served models")
	}
	w, _ = adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, "/_internal/admin/providers/legacy",
		`{"models":[{"name":"obsolete-legacy","encrypted_reasoning":null}]}`)
	if w.Code != 400 {
		t.Fatal("policy-free replacement entry bypassed inherited-name validation")
	}
	assertAdminReasoningProviderUnchanged(t, fixture.store, before, models)
}

func TestAdminEncryptedReasoningExplicitArrayTransitionsAndChangedBase(t *testing.T) {
	fixture := newAdminReasoningFixture(t, adminReasoningStaticRuntime(t))
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"moving","type":"openai-compatible","base_url":"https://local.example/v1","api_key":"moving-private-credential","encrypted_reasoning":{"on_caller_mismatch":"strip_and_retry"},"models":[{"name":"m","upstream_name":"local-m","encrypted_reasoning":{}}]}`, 201)
	path := "/_internal/admin/providers/moving"
	for _, extends := range []string{"base", ""} {
		before, models := adminReasoningSavedProvider(t, fixture.store, "moving")
		for _, collection := range []string{"", `,"models":null`} {
			count := fixture.activations
			w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, path, `{"extends":"`+extends+`"`+collection+`}`)
			if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count || !bytes.Contains(w.Body.Bytes(), []byte("explicit models array")) {
				t.Fatalf("policy-bearing transition did not require an array: %d %s", w.Code, w.Body.String())
			}
			assertAdminReasoningProviderUnchanged(t, fixture.store, before, models)
		}
		definition := `{"name":"m","encrypted_reasoning":{}}`
		if extends == "" {
			definition = `{"name":"m","upstream_name":"destination-m","encrypted_reasoning":{"passthrough":false}}`
		}
		view := requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"extends":"`+extends+`","models":[`+definition+`]}`, 200)
		after, _ := adminReasoningSavedProvider(t, fixture.store, "moving")
		if !bytes.Equal(before.EncryptedReasoning, after.EncryptedReasoning) || view["models_inherited"] != (extends != "") {
			t.Fatal("transition changed provider-local intent or definition provenance")
		}
	}
	requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"extends":"base","models":[{"name":"m","encrypted_reasoning":{}}]}`, 200)
	before, models := adminReasoningSavedProvider(t, fixture.store, "moving")
	w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, http.MethodPut, path, `{"extends":"blank-base"}`)
	if w.Code != 400 || w.Header().Get(catalogSavedHeader) != "" {
		t.Fatal("changed base accepted a preserved dangling overlay")
	}
	assertAdminReasoningProviderUnchanged(t, fixture.store, before, models)
	view := requireAdminReasoningStatus(t, fixture, http.MethodPut, path, `{"extends":"blank-base","models":[]}`, 200)
	if len(view["models"].([]any)) != 1 {
		t.Fatal("explicit overlay replacement did not select the new base inventory")
	}
	after, _ := adminReasoningSavedProvider(t, fixture.store, "moving")
	if !bytes.Equal(before.EncryptedReasoning, after.EncryptedReasoning) {
		t.Fatal("base-name change altered the local provider declaration")
	}
	requireAdminReasoningStatus(t, fixture, http.MethodPost, "/_internal/admin/providers",
		`{"name":"policy-free","type":"openai-compatible","base_url":"https://local.example/v1","api_key":"free-private-credential","models":[{"name":"old-definition"}]}`, 201)
	_, old := adminReasoningSavedProvider(t, fixture.store, "policy-free")
	requireAdminReasoningStatus(t, fixture, http.MethodPut, "/_internal/admin/providers/policy-free", `{"extends":"base"}`, 200)
	_, inherited := adminReasoningSavedProvider(t, fixture.store, "policy-free")
	if !reflect.DeepEqual(old, inherited) {
		t.Fatal("policy-free legacy transition deleted or reinterpreted model rows")
	}
	view = requireAdminReasoningStatus(t, fixture, http.MethodPut, "/_internal/admin/providers/policy-free", `{"extends":""}`, 200)
	if adminReasoningModelView(t, view, "old-definition")["encrypted_reasoning"] != nil {
		t.Fatal("policy-free legacy transition synthesized a model declaration")
	}
}
