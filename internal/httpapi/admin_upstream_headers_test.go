package httpapi

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/google/uuid"
)

func TestAdminUpstreamHeadersWritesRejectWithoutDatabase(t *testing.T) {
	t.Setenv("AIPROXY_JWT_SECRET", "scoped-upstream-headers-fixture")
	access, _, err := adminauth.IssueAccess(uuid.NewString(), "operator@example.test", true)
	if err != nil {
		t.Fatal(err)
	}
	h := &Handler{}
	deps := Dependencies{RequestReload: func() error {
		t.Fatal("rejected upstream_headers write activated")
		return nil
	}}
	for _, tc := range []struct {
		name, method, body string
		rest               []string
		alias              bool
	}{
		{"provider create", http.MethodPost, `{"name":"p","type":"openai","upstream_headers":{"X-Fixture":"not-for-response"}}`, nil, false},
		{"provider update", http.MethodPut, `{"Upstream_Headers":null}`, []string{"p"}, false},
		{"model create", http.MethodPost, `{"name":"p","type":"openai","models":[{"name":"m","upstream_headers":{}}]}`, nil, false},
		{"model update", http.MethodPut, `{"models":[{"name":"m","UPSTREAM_HEADERS":{"X-Fixture":"not-for-response"}}]}`, []string{"p"}, false},
		{"credential update", http.MethodPut, `{"API_KEY":"new","upstream_headers":{"X-Fixture":"not-for-response"}}`, []string{"p", "credential"}, false},
		{"credential case variant", http.MethodPut, `{"api_key":"new","Upstream_Headers":null}`, []string{"p", "credential"}, false},
		{"alias update", http.MethodPut, `{"upstream_headers":{"X-Fixture":"not-for-response"}}`, []string{"a"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/", strings.NewReader(tc.body))
			r.Header.Set("Authorization", "Bearer "+access)
			w := httptest.NewRecorder()
			if tc.alias {
				h.adminAliases(deps, w, r, tc.rest)
			} else {
				h.adminProviders(deps, w, r, tc.rest)
			}
			if w.Code != http.StatusBadRequest || w.Header().Get(catalogSavedHeader) != "" || strings.Contains(w.Body.String(), "not-for-response") {
				t.Fatalf("status=%d headers=%v body=%q", w.Code, w.Header(), w.Body.String())
			}
		})
	}
	var alias adminAliasUpsert
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"a","providers":["p"],"model":"m","UPSTREAM_HEADERS":{}}`))
	if err := decodeAdminCatalogBody(r, &alias); err != nil {
		t.Fatal(err)
	}
	if _, _, err := h.buildConfigAlias(alias.Name, "round_robin", alias); err == nil || !strings.Contains(err.Error(), "unsupported JSON field") {
		t.Fatalf("alias creation accepted upstream_headers: %v", err)
	}
}

func TestAdminCredentialUpstreamHeadersRejectionPreservesLegacyUnknownFields(t *testing.T) {
	var req adminCredentialUpsert
	if err := json.Unmarshal([]byte(`{"API_KEY":"fixture","unrelated":{"upstream_headers":{}},"upstream_headers_note":"old client metadata"}`), &req); err != nil ||
		req.inputJSONError != nil || req.APIKey == nil || *req.APIKey != "fixture" {
		t.Fatalf("legacy credential body did not decode: %v, %v", err, req.inputJSONError)
	}
}

func TestAdminUpstreamHeadersAliasCreateAndStaticViewsPostgres(t *testing.T) {
	rt := loadPassthroughRuntime(t, passthroughAnon, `
upstream_headers = { X-Fixture = "fixed-value-not-for-admin" }
provider "openai" "static-provider" {
  api_key = "fixture"
  upstream_headers = { X-Fixture = "fixed-value-not-for-admin" }
  model "m" { upstream_headers = { X-Fixture = "fixed-value-not-for-admin" } }
}
alias "static-alias" {
  algorithm = "round_robin"
  providers = ["static-provider"]
  model = "m"
  upstream_headers = { X-Fixture = "fixed-value-not-for-admin" }
}
`)
	f := newAdminReasoningFixture(t, rt)
	const aliases = "/_internal/admin/aliases"
	w, _ := adminReasoningRequest(t, f.handler, f.access, http.MethodPost, aliases,
		`{"name":"rejected","providers":["static-provider"],"model":"m","upstream_headers":{"X-Fixture":"not-for-response"}}`)
	if w.Code != http.StatusBadRequest || strings.Contains(w.Body.String(), "not-for-response") || f.activations != 0 {
		t.Fatalf("alias write status=%d activations=%d body=%q", w.Code, f.activations, w.Body.String())
	}
	if _, err := f.store.GetAlias(t.Context(), "rejected"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("rejected alias persisted or storage failed: %v", err)
	}
	for _, path := range []string{"/_internal/admin/providers/static-provider", aliases + "/static-alias"} {
		w, _ := adminReasoningRequest(t, f.handler, f.access, http.MethodGet, path, "")
		if w.Code != http.StatusOK || strings.Contains(w.Body.String(), "upstream_headers") || strings.Contains(w.Body.String(), "fixed-value-not-for-admin") {
			t.Fatalf("%s did not hide upstream_headers: status=%d", path, w.Code)
		}
	}
}
