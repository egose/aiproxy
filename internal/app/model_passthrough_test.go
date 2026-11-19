package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestAppModelPassthroughWithMergedWorkspaceKeys(t *testing.T) {
	st, dsn := workspaceInferenceDB(t)
	ctx := t.Context()
	const (
		staticToken  = "fixture-static-token"
		systemToken  = "fixture-system-db-token"
		tenantToken  = "fixture-tenant-db-token"
		tenantOrigin = "tenant-passthrough.example.com"
		ruleID       = "router/vendor/live"
		ruleGrant    = "router/family"
		missing      = "router/vendor/missing"
		systemModel  = "router/vendor/pinned"
		tenantModel  = "tenant-router/literal"
	)
	tenant := store.Workspace{Name: "tenant-passthrough"}
	if err := st.CreateWorkspace(ctx, &tenant); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		name, token, literal string
		workspace            uuid.UUID
	}{
		{"app", systemToken, systemModel, store.SystemWorkspaceID},
		{"tenant-app", tenantToken, tenantModel, tenant.ID},
	} {
		key := store.InboundKey{Name: row.name, TokenHash: store.TokenHash(row.token), TokenPrefix: "fixture",
			Tenant: "fixture", Enabled: true, WorkspaceID: row.workspace,
			AllowedModels: []string{row.literal, ruleID, ruleGrant, missing}}
		if err := st.CreateInboundKey(ctx, &key); err != nil {
			t.Fatal(err)
		}
	}
	encrypted, err := store.EncryptSecret([]byte("fixture-tenant-upstream"))
	if err != nil {
		t.Fatal(err)
	}
	tenantProvider := store.DBProvider{Name: "tenant-router", Type: "openai-compatible",
		BaseURL: "https://" + tenantOrigin + "/v1", Enabled: true, WorkspaceID: tenant.ID, APIKeyEncrypted: encrypted}
	if err := st.CreateProviderAggregate(ctx, &tenantProvider, []store.DBProviderModel{{Name: "literal", UpstreamName: "tenant-pinned"}}); err != nil {
		t.Fatal(err)
	}

	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			Model string `json:"model"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil ||
			r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected upstream request: %s %s, decode error %v", r.Method, r.URL.Path, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		expectedKey := "Bearer fixture-static-upstream"
		if input.Model == "tenant-pinned" {
			expectedKey = "Bearer fixture-tenant-upstream"
		}
		if r.Header.Get("Authorization") != expectedKey {
			t.Errorf("unexpected upstream credential for %q", input.Model)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"model":%q,"choices":[]}`, input.Model)
	}))
	t.Cleanup(upstream.Close)
	source := fmt.Sprintf(`listener "http" "public" { address = "127.0.0.1:0" }
database { url = %q }
multi_tenancy { allowed_upstream_origins = [%q] }
auth "main" {
  mode = "bearer_static"
  client "app" {
    token = %q
    tenant = "fixture"
    allowed_models = [%q]
    allowed_model_passthroughs = [%q]
  }
}
provider "openai-compatible" "router" {
  base_url = %q
  api_key = "fixture-static-upstream"
  model "vendor/pinned" { upstream_name = "system-pinned" }
  model_passthrough "family" {
    match = "^vendor/live$"
    capabilities = ["chat"]
  }
}
`, dsn, "https://"+tenantOrigin, staticToken, systemModel, ruleGrant, upstream.URL+"/v1")
	a, err := Build(ctx, BuildOptions{ConfigPath: writeConfigFile(t, source), LogOutput: io.Discard,
		transportFactory: tenantMockTransportFactory(map[string]string{tenantOrigin: upstream.URL})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := a.Close(); err != nil {
			t.Error(err)
		}
	})
	proxy := httptest.NewServer(a.Server.Handler)
	t.Cleanup(proxy.Close)
	chat := func(token, model string) appDeferredReply {
		t.Helper()
		return workspaceInferenceRequest(t, proxy, http.MethodPost, "/v1/chat/completions", token, fmt.Sprintf(`{"model":%q,"messages":[]}`, model))
	}
	for _, tc := range []struct{ token, model, wire string }{
		{staticToken, ruleID, "vendor/live"},
		{staticToken, systemModel, "system-pinned"},
		{systemToken, systemModel, "system-pinned"},
		{tenantToken, tenantModel, "tenant-pinned"},
	} {
		before := calls.Load()
		got := chat(tc.token, tc.model)
		if got.status != http.StatusOK || string(got.body) != fmt.Sprintf(`{"model":%q,"choices":[]}`, tc.wire) || calls.Load() != before+1 {
			t.Fatalf("%s did not reach %q once: %d %s", tc.model, tc.wire, got.status, got.body)
		}
	}
	before := calls.Load()
	for _, token := range []string{systemToken, tenantToken} {
		baseline := chat(token, missing)
		if baseline.status != http.StatusNotFound || !strings.Contains(string(baseline.body), `"type":"model_not_found"`) {
			t.Fatalf("literal miss = %d %s", baseline.status, baseline.body)
		}
		for _, model := range []string{ruleID, ruleGrant} {
			got := chat(token, model)
			if got.status != baseline.status || string(got.body) != string(baseline.body) {
				t.Fatalf("DB key request to %s differed from a literal miss: %d %s", model, got.status, got.body)
			}
		}
	}
	if after := calls.Load(); after != before {
		t.Fatalf("DB key misses contacted upstream: %d -> %d", before, after)
	}
}
