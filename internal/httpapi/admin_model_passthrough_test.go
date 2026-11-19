package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/google/uuid"
)

func TestAdminModelPassthroughRequestsRejectBeforeProviderOrKeyCreation(t *testing.T) {
	t.Setenv("AIPROXY_JWT_SECRET", "admin-model-passthrough-fixture")
	for _, global := range []bool{false, true} {
		access, _, err := adminauth.IssueAccess(uuid.NewString(), "operator@example.test", global)
		if err != nil {
			t.Fatal(err)
		}
		for _, raw := range []string{
			`{"name":"attempt","model_passthrough":null}`,
			`{"name":"attempt","MODEL_PASSTHROUGHS":{}}`,
			`{"name":"attempt","allowed_model_passthroughs":[]}`,
			`{"name":"attempt","allowed_model_passthrough\u017f":["not-for-response"]}`,
			`{"name":"attempt","models":[{"name":"m","model_passthrough":{"not-for-response":true}}],"Models":[]}`,
			`{"name":"attempt","models":[{"name":"m","Allowed_Model_Passthroughs":null}]}`,
			`{"name":"attempt","api_key_ref":{"key":"fixture","model_passthrough":{}},"api_key_ref":null}`,
		} {
			for _, op := range []struct {
				method string
				rest   []string
				keys   bool
			}{
				{method: http.MethodPost},
				{method: http.MethodPut, rest: []string{"attempt"}},
				{method: http.MethodPut, rest: []string{"attempt", "credential"}},
				{method: http.MethodPost, keys: true},
			} {
				req := httptest.NewRequest(op.method, "/", strings.NewReader(raw))
				req.Header.Set("Authorization", "Bearer "+access)
				w := httptest.NewRecorder()
				deps := Dependencies{RequestReload: func() error {
					t.Fatal("rejected model passthrough activated")
					return nil
				}}
				h := &Handler{}
				if op.keys {
					h.adminKeys(deps, w, req, op.rest)
				} else {
					h.adminProviders(deps, w, req, op.rest)
				}
				if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" ||
					!strings.Contains(w.Body.String(), "static-config-only") || strings.Contains(w.Body.String(), "not-for-response") {
					t.Fatalf("write %s %v (key %t) %s: %d %s", op.method, op.rest, op.keys, raw, w.Code, w.Body.String())
				}
			}
		}
	}
}

func TestManagedModelPassthroughKeyBodyPreservesDecoderBehavior(t *testing.T) {
	var key struct {
		Name string `json:"name"`
	}
	for _, tc := range []struct {
		name, raw, want string
	}{
		{"first-value-and-unknown-fields", `{"name":"fixture","unknown":[{"note":"allowed_model_passthroughs"}]} {}`, ""},
		{"body-limit", `{"name":"` + strings.Repeat("x", 1<<20) + `","model_passthrough":null}`, "invalid body"},
		{"depth-limit", `{"unknown":` + strings.Repeat("[", 10001) + `0` + strings.Repeat("]", 10001) + `,"model_passthrough":null}`, "invalid body"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tc.raw))
			err := decodeManagedKeyBody(r, &key)
			if tc.want == "" {
				if err != nil || key.Name != "fixture" {
					t.Fatalf("ordinary body: name %q, error %v", key.Name, err)
				}
			} else if err == nil || err.Error() != tc.want {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestAdminManagedModelPassthroughWritesFailWithoutPersistence(t *testing.T) {
	st := openAdminReasoningStore(t)
	h, access := validationTestHandler(t, st)
	ctx := context.Background()
	const keys = "/_internal/admin/keys"
	w, view := adminReasoningRequest(t, h, access, http.MethodPost, keys, `{"name":"unchanged","description":"before","allowed_models":["openai/gpt-4o-mini"]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create control key: %d %s", w.Code, w.Body)
	}
	id := uuid.MustParse(view["key"].(map[string]any)["id"].(string))
	before, err := st.GetInboundKey(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	const providers = "/_internal/admin/providers"
	w, _ = adminReasoningRequest(t, h, access, http.MethodPost, providers, `{"name":"unchanged-provider","type":"openai","api_key":"fixture","models":[{"name":"m"}]}`)
	if w.Code != http.StatusCreated {
		t.Fatalf("create control provider: %d %s", w.Code, w.Body)
	}
	providerBefore, err := st.GetProvider(ctx, "unchanged-provider")
	if err != nil {
		t.Fatal(err)
	}
	deps := h.current()
	deps.RequestReload = func() error {
		t.Fatal("rejected model passthrough activated")
		return nil
	}
	h.UpdateDependencies(deps)
	key := keys + "/" + id.String()
	provider := providers + "/unchanged-provider"
	flows := "/_internal/admin/copilot-device-flows"
	for _, op := range []struct {
		method, path, raw string
	}{
		{http.MethodPut, key, `{"description":"after","allowed_models":[],"allowed_model_passthroughs":null}`},
		{http.MethodPut, provider, `{"display_name":"after","model_passthrough":{}}`},
		{http.MethodPut, provider + "/credential", `{"api_key":"changed","allowed_model_passthroughs":null}`},
		{http.MethodPost, flows, `{"client_id":"fixture","model_passthrough":{}}`},
		{http.MethodPost, flows, `{"client_id":"fixture","unknown":[{"allowed_model_passthroughs":null}]}`},
	} {
		w, _ := adminReasoningRequest(t, h, access, op.method, op.path, op.raw)
		if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" ||
			w.Header().Get("X-Aiproxy-Key-Saved") != "" || !strings.Contains(w.Body.String(), "static-config-only") {
			t.Fatalf("%s %s: %d %s", op.method, op.path, w.Code, w.Body)
		}
	}
	if after, err := st.GetInboundKey(ctx, id); err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("rejected write changed key: %v", err)
	}
	if after, err := st.GetProvider(ctx, "unchanged-provider"); err != nil || !reflect.DeepEqual(providerBefore, after) {
		t.Fatalf("rejected write changed provider: %v", err)
	}
}

func TestAdminManagedModelPassthroughBaseExposesOnlyInheritedLiterals(t *testing.T) {
	src := `
listener "http" "public" { address = ":8080" }
auth "main" { mode = "none" }
provider "openai" "static-mixed" {
  api_key = "fixture"
  model "pinned" { upstream_name = "served-pinned" }
  model_passthrough "family" {
    match = "^vendor/.+$"
    capabilities = ["chat"]
  }
}
provider "openai" "static-rule-only" {
  api_key = "fixture"
  model_passthrough "family" {
    match = "^vendor/.+$"
    capabilities = ["chat"]
  }
}`
	rt, err := config.Load([]byte(src), "base.hcl")
	if err != nil {
		t.Fatal(err)
	}
	f := newAdminReasoningFixture(t, rt)
	const providers = "/_internal/admin/providers"
	requireAdminReasoningStatus(t, f, http.MethodPost, providers,
		`{"name":"managed-child","type":"openai","extends":"static-mixed","api_key":"managed-key"}`, http.StatusCreated)
	catalog := f.handler.current().Catalog
	base, ok := catalog.Provider("static-mixed")
	if _, status := base.MatchModelPassthrough("vendor/new"); !ok || status != config.ModelPassthroughMatched {
		t.Fatal("DB merge lost the static rule")
	}
	child, literal, ok := catalog.Model("managed-child", "pinned")
	if !ok || child.APIKey != "managed-key" || len(child.ModelPassthroughs) != 0 || literal.UpstreamName != "served-pinned" {
		t.Fatal("DB merge lost the literal or adopted the static rule or credential")
	}
	w, _ := adminReasoningRequest(t, f.handler, f.access, http.MethodPost, providers,
		`{"name":"bad-child","type":"openai","extends":"static-rule-only","api_key":"managed-key"}`)
	if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || !strings.Contains(w.Body.String(), "at least one literal model") {
		t.Fatalf("managed derivative accepted a rule-only base: %d %s", w.Code, w.Body)
	}
}
