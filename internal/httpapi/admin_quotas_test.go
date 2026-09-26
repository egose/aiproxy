package httpapi

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func quotaSetup(t *testing.T) (*store.Store, *Handler, string, string, string, string, string, string) {
	t.Helper()
	st, h, ownerAccess := openUsersTestStore(t)
	suffix := time.Now().UnixNano()
	orgID := mkOrg(t, st, h, ownerAccess, "quota")
	memberEmail := fmt.Sprintf("qmember-%d@example.com", suffix)
	adminEmail := fmt.Sprintf("qteamadmin-%d@example.com", suffix)
	_, memberAccess := mkOrgMember(t, st, h, ownerAccess, memberEmail, orgID, "member")
	_, adminAccess := mkOrgMember(t, st, h, ownerAccess, adminEmail, orgID, "member")
	memberID := userIDByEmail(t, st, memberEmail)
	adminID := userIDByEmail(t, st, adminEmail)

	code, body := callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams", map[string]interface{}{
		"name": fmt.Sprintf("qteam-%d", suffix),
	})
	if code != http.StatusCreated {
		t.Fatalf("create team status = %d, body = %v", code, body)
	}
	teamID, _ := body["id"].(string)
	for _, tc := range []struct{ email, role string }{{adminEmail, "admin"}, {memberEmail, "member"}} {
		if code, _ := callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/orgs/"+orgID+"/teams/"+teamID+"/members", map[string]interface{}{
			"email": tc.email, "role": tc.role,
		}); code != http.StatusCreated {
			t.Fatalf("add %s status = %d", tc.email, code)
		}
	}
	return st, h, ownerAccess, orgID, memberAccess, adminAccess, memberID, adminID
}

func TestUserQuotaPermissions(t *testing.T) {
	_, h, ownerAccess, orgID, memberAccess, _, memberID, _ := quotaSetup(t)

	userQuota := "/_internal/admin/orgs/" + orgID + "/users/" + memberID + "/quota"

	code, body := callAdmin(t, h, ownerAccess, http.MethodPut, userQuota, map[string]interface{}{
		"budget_micros": 5000000,
		"tpm":           []interface{}{map[string]interface{}{"model": "alias/fast", "ceiling": 60000, "effective": 30000}},
	})
	if code != http.StatusOK {
		t.Fatalf("org admin set quota status = %d, body = %v", code, body)
	}
	if body["budget_micros"] != float64(5000000) {
		t.Fatalf("budget not echoed: %v", body)
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodGet, userQuota, nil)
	if code != http.StatusOK {
		t.Fatalf("self get quota status = %d, body = %v", code, body)
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodPut, userQuota, map[string]interface{}{
		"tpm": []interface{}{map[string]interface{}{"model": "alias/fast", "effective": 10000}},
	})
	if code != http.StatusOK {
		t.Fatalf("self lower effective status = %d, body = %v", code, body)
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPut, userQuota, map[string]interface{}{
		"tpm": []interface{}{map[string]interface{}{"model": "alias/fast", "effective": 70000}},
	}); code == http.StatusOK {
		t.Fatalf("effective above ceiling must fail")
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPut, userQuota, map[string]interface{}{
		"budget_micros": 10,
	}); code == http.StatusOK {
		t.Fatalf("member must not set budget")
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPut, userQuota, map[string]interface{}{
		"tpm": []interface{}{map[string]interface{}{"model": "alias/fast", "ceiling": 10}},
	}); code == http.StatusOK {
		t.Fatalf("member must not set ceiling")
	}
}

func TestTeamQuotaPermissions(t *testing.T) {
	_, h, ownerAccess, orgID, memberAccess, adminAccess, _, _ := quotaSetup(t)

	code, body := callAdmin(t, h, ownerAccess, http.MethodGet, "/_internal/admin/orgs/"+orgID+"/teams", nil)
	if code != http.StatusOK {
		t.Fatalf("list teams status = %d, body = %v", code, body)
	}
	items, _ := body["teams"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("want 1 team: %v", body)
	}
	teamID, _ := items[0].(map[string]interface{})["id"].(string)
	teamQuota := "/_internal/admin/orgs/" + orgID + "/teams/" + teamID + "/quota"

	code, _ = callAdmin(t, h, ownerAccess, http.MethodPut, teamQuota, map[string]interface{}{
		"budget_micros": 9000000,
		"tpm":           []interface{}{map[string]interface{}{"model": "alias/fast", "ceiling": 100000}},
	})
	if code != http.StatusOK {
		t.Fatalf("org admin set team quota status = %d", code)
	}

	code, _ = callAdmin(t, h, adminAccess, http.MethodPut, teamQuota, map[string]interface{}{
		"tpm": []interface{}{map[string]interface{}{"model": "alias/fast", "effective": 50000}},
	})
	if code != http.StatusOK {
		t.Fatalf("team admin set effective status = %d", code)
	}
	if code, _ := callAdmin(t, h, adminAccess, http.MethodPut, teamQuota, map[string]interface{}{
		"tpm": []interface{}{map[string]interface{}{"model": "alias/fast", "ceiling": 10}},
	}); code == http.StatusOK {
		t.Fatalf("team admin must not set ceiling")
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodPut, teamQuota, map[string]interface{}{
		"tpm": []interface{}{map[string]interface{}{"model": "alias/fast", "effective": 10}},
	}); code == http.StatusOK {
		t.Fatalf("plain member must not set team tpm")
	}
	code, body = callAdmin(t, h, memberAccess, http.MethodGet, teamQuota, nil)
	if code != http.StatusOK {
		t.Fatalf("team member get quota status = %d, body = %v", code, body)
	}
	if body["budget_micros"] != float64(9000000) {
		t.Fatalf("team budget not visible: %v", body)
	}
}

func TestBudgetEnforcement(t *testing.T) {
	st, h, _, orgID, memberAccess, _, memberID, _ := quotaSetup(t)
	ctx := context.Background()
	orgUUID := mustParseUUID(orgID)
	memberUUID := mustParseUUID(memberID)

	code, body := callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": fmt.Sprintf("qkey-%d", time.Now().UnixNano()), "org_id": orgID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create key status = %d, body = %v", code, body)
	}
	created, _ := body["key"].(map[string]interface{})
	keyID, _ := created["id"].(string)
	t.Cleanup(func() { _ = st.DeleteInboundKey(context.Background(), mustParseUUID(keyID)) })

	deps := Dependencies{MultiTenancy: config.MultiTenancy{Enabled: true}, AdminStore: st, Quota: NewQuotaTracker(st)}
	if err := st.UpsertScopeQuota(ctx, &store.ScopeQuota{OrgID: orgUUID, ScopeType: "user", ScopeID: memberUUID, BudgetMicros: 100}); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSpendEntry(ctx, &store.SpendEntry{OrgID: orgUUID, KeyID: mustParseUUID(keyID), UserID: &memberUUID, Model: "alias/fast", Tokens: 10, CostMicros: 150}); err != nil {
		t.Fatal(err)
	}
	principal := &auth.Principal{Name: "k", KeyID: uuid.New(), OrgID: orgUUID, OwnerUserID: memberUUID}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	if h.allowQuota(deps, w, req, principal, "alias/fast") {
		t.Fatalf("exhausted budget must block")
	}
	if w.Code != http.StatusForbidden {
		t.Fatalf("budget status = %d, want 403", w.Code)
	}
	if !strings.Contains(w.Body.String(), "budget_exceeded") {
		t.Fatalf("budget body wrong: %s", w.Body.String())
	}
}

func TestQuotaProxyEndToEnd(t *testing.T) {
	st, h, ownerAccess := openUsersTestStore(t)
	suffix := time.Now().UnixNano()
	orgID := mkOrg(t, st, h, ownerAccess, "qproxy")
	memberEmail := fmt.Sprintf("qproxy-%d@example.com", suffix)
	_, _ = mkOrgMember(t, st, h, ownerAccess, memberEmail, orgID, "member")
	memberID := userIDByEmail(t, st, memberEmail)

	code, body := callAdmin(t, h, ownerAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{
		"name": fmt.Sprintf("qproxy-key-%d", suffix), "org_id": orgID,
		"allowed_models": []interface{}{"openai/gpt-4o-mini"},
		"owner_type":     "user", "owner_id": memberID,
	})
	if code != http.StatusCreated {
		t.Fatalf("create key status = %d, body = %v", code, body)
	}
	created, _ := body["key"].(map[string]interface{})
	keyID, _ := created["id"].(string)
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatalf("missing token: %v", body)
	}
	t.Cleanup(func() { _ = st.DeleteInboundKey(context.Background(), mustParseUUID(keyID)) })

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"chatcmpl-1","object":"chat.completion","created":1,"model":"gpt-4o-mini","choices":[{"index":0,"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}}`)
	}))
	defer upstream.Close()

	rt := &config.Runtime{
		MultiTenancy: config.MultiTenancy{Enabled: true},
		Catalog: config.NewCatalog([]config.Provider{{
			Type: config.ProviderTypeOpenAI, Name: "openai", BaseURL: upstream.URL, APIKey: "sk-test",
			Models: []config.Model{{Name: "gpt-4o-mini", UpstreamName: "gpt-4o-mini",
				Pricing: &config.ModelPricing{InputPerMillion: 1000, OutputPerMillion: 2000}}},
		}}, nil, nil),
		Auth: config.Auth{Mode: config.AuthModeBearerStatic},
	}
	dyn := []auth.DynamicClient{{
		TokenHash: store.TokenHash(token), Name: created["name"].(string),
		AllowedModels: []string{"openai/gpt-4o-mini"},
		KeyID:         mustParseUUID(keyID), OrgID: mustParseUUID(orgID), OwnerUserID: mustParseUUID(memberID),
	}}
	agg := accounting.NewAggregator()
	proxy := NewHandler(Dependencies{
		Resolver:     modelresolver.New(rt),
		Adapter:      provider.New(),
		Auth:         auth.NewAuthenticatorWithClients(rt.Auth, dyn),
		Authorizer:   auth.NewAuthorizerWithClients(rt.Auth, dyn),
		Catalog:      rt.Catalog,
		Metrics:      observability.NewMetrics(),
		Health:       providerhealth.New(nil, config.ProviderHealth{}),
		Accounting:   agg,
		Usage:        agg,
		MultiTenancy: rt.MultiTenancy,
		AdminStore:   st,
		Quota:        NewQuotaTracker(st),
	})

	chatBody := `{"model":"openai/gpt-4o-mini","messages":[{"role":"user","content":"hi"}]}`
	callProxy := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader([]byte(chatBody)))
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		proxy.ServeHTTP(w, r)
		return w
	}

	if w := callProxy(); w.Code != http.StatusOK {
		t.Fatalf("proxy status = %d, body = %s", w.Code, w.Body.String())
	}
	used, err := st.KeyUsageByOrg(context.Background(), mustParseUUID(orgID))
	if err != nil {
		t.Fatal(err)
	}
	if len(used) != 1 || used[0].Requests != 1 || used[0].Tokens != 15 || used[0].Spend != 20000 {
		t.Fatalf("ledger wrong: %+v", used)
	}

	userQuota := "/_internal/admin/orgs/" + orgID + "/users/" + memberID + "/quota"
	if code, _ := callAdmin(t, h, ownerAccess, http.MethodPut, userQuota, map[string]interface{}{"budget_micros": 10000}); code != http.StatusOK {
		t.Fatalf("set budget status = %d", code)
	}
	if w := callProxy(); w.Code != http.StatusForbidden || !strings.Contains(w.Body.String(), "budget_exceeded") {
		t.Fatalf("budget proxy = %d %s, want 403 budget_exceeded", w.Code, w.Body.String())
	}

	if code, _ := callAdmin(t, h, ownerAccess, http.MethodPut, userQuota, map[string]interface{}{
		"budget_micros": 100000000,
		"tpm":           []interface{}{map[string]interface{}{"model": "openai/gpt-4o-mini", "ceiling": 10}},
	}); code != http.StatusOK {
		t.Fatalf("set tpm status = %d", code)
	}
	if w := callProxy(); w.Code != http.StatusTooManyRequests || !strings.Contains(w.Body.String(), "tpm_exceeded") {
		t.Fatalf("tpm proxy = %d %s, want 429 tpm_exceeded", w.Code, w.Body.String())
	}
}

func TestTPMEnforcement(t *testing.T) {
	st, h, _, orgID, _, _, memberID, _ := quotaSetup(t)
	ctx := context.Background()
	orgUUID := mustParseUUID(orgID)
	memberUUID := mustParseUUID(memberID)

	deps := Dependencies{MultiTenancy: config.MultiTenancy{Enabled: true}, AdminStore: st, Quota: NewQuotaTracker(st)}
	if err := st.UpsertScopeQuota(ctx, &store.ScopeQuota{OrgID: orgUUID, ScopeType: "user", ScopeID: memberUUID, Model: "alias/fast", TPMCeiling: 100}); err != nil {
		t.Fatal(err)
	}
	deps.Quota.recordTokens(orgUUID, quotaScope{typ: "user", id: memberUUID}, "alias/fast", 100)
	principal := &auth.Principal{Name: "k", KeyID: uuid.New(), OrgID: orgUUID, OwnerUserID: memberUUID}
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	w := httptest.NewRecorder()
	if h.allowQuota(deps, w, req, principal, "alias/fast") {
		t.Fatalf("exhausted tpm must block")
	}
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("tpm status = %d, want 429", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatalf("tpm must set Retry-After")
	}
	if !strings.Contains(w.Body.String(), "tpm_exceeded") {
		t.Fatalf("tpm body wrong: %s", w.Body.String())
	}
}
