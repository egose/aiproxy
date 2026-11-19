package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/provider"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun/driver/pgdriver"
)

func TestQuotaContractOwnerlessAndMissingData(t *testing.T) {
	st, h, _, workspaceID, _, _, memberID, _ := quotaSetup(t)
	ctx := context.Background()
	workspace, member := mustParseUUID(workspaceID), mustParseUUID(memberID)
	key := store.InboundKey{Name: uuid.NewString(), TokenHash: uuid.NewString(), TokenPrefix: "test", WorkspaceID: workspace, Enabled: true}
	if err := st.CreateInboundKey(ctx, &key); err != nil {
		t.Fatal(err)
	}
	deps := Dependencies{MultiTenancy: config.MultiTenancy{Enabled: true}, AdminStore: st, Quota: NewQuotaTracker(st),
		Catalog: config.NewCatalog([]config.Provider{{Name: "p", Models: []config.Model{{Name: "m", UpstreamName: "m"}}}}, nil, nil)}
	if err := st.UpsertScopeQuota(ctx, &store.ScopeQuota{WorkspaceID: workspace, ScopeType: "user", ScopeID: member, BudgetMicros: 1}); err != nil {
		t.Fatal(err)
	}
	owned := store.InboundKey{Name: uuid.NewString(), TokenHash: uuid.NewString(), TokenPrefix: "test", WorkspaceID: workspace, OwnerUserID: &member, Enabled: true}
	if err := st.CreateInboundKey(ctx, &owned); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertSpendEntry(ctx, &store.SpendEntry{WorkspaceID: workspace, KeyID: owned.ID, UserID: &member, CostMicros: 1}); err != nil {
		t.Fatal(err)
	}
	a := auth.NewAuthenticatorWithClients(config.Auth{Mode: config.AuthModeBearerStatic}, []auth.DynamicClient{{
		Name: key.Name, TokenHash: store.TokenHash("ownerless-fixture"), KeyID: key.ID, Workspace: identity.InWorkspace(workspace),
	}})
	p := authenticatedTestPrincipal(t, a, "ownerless-fixture")
	if !h.allowQuota(deps, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil), p, "p/m") {
		t.Fatal("ownerless key acquired another user's exhausted quota")
	}
	for _, usage := range []provider.Usage{{}, {TotalTokens: 7}, {PromptTokens: 2, CompletionTokens: 3, TotalTokens: 5}} {
		if err := h.persistQuotaUsageContext(ctx, deps, p, "p/m", "m", "p", usage); err != nil {
			t.Fatal(err)
		}
	}
	var rows []store.SpendEntry
	if err := st.DB.NewSelect().Model(&rows).Where("key_id = ? AND user_id IS NULL", key.ID).Scan(ctx); err != nil || len(rows) != 2 {
		t.Fatalf("missing usage should add no row; unpriced usage should persist: %+v %v", rows, err)
	}
	for _, row := range rows {
		if row.CostMicros != 0 || row.TeamID != nil || row.WorkspaceID != workspace {
			t.Fatalf("unpriced ownerless usage invented attribution/cost: %+v", row)
		}
	}
	static := auth.NewAuthenticator(config.Auth{Mode: config.AuthModeBearerStatic, Clients: map[string]config.Client{"static": {Name: "static", Token: "static-fixture"}}})
	for _, principal := range []*auth.Principal{nil, authenticatedTestPrincipal(t, static, "static-fixture")} {
		if err := h.persistQuotaUsageContext(ctx, deps, principal, "p/m", "m", "p", provider.Usage{TotalTokens: 9}); err != nil {
			t.Fatal(err)
		}
	}
	count, err := st.DB.NewSelect().Model((*store.SpendEntry)(nil)).Where("workspace_id = ?", workspace).Count(ctx)
	if err != nil || count != 3 {
		t.Fatalf("static/anonymous added durable charges: count=%d %v", count, err)
	}
}

func TestQuotaContractIndependentSpendCaches(t *testing.T) {
	st, h, _, workspaceID, memberAccess, _, memberID, _ := quotaSetup(t)
	ctx := context.Background()
	workspace, member := mustParseUUID(workspaceID), mustParseUUID(memberID)
	code, body := callAdmin(t, h, memberAccess, http.MethodPost, "/_internal/admin/keys", map[string]interface{}{"name": uuid.NewString(), "workspace_id": workspaceID})
	if code != http.StatusCreated {
		t.Fatal(code, body)
	}
	keyID := mustParseUUID(body["key"].(map[string]interface{})["id"].(string))
	if err := st.UpsertScopeQuota(ctx, &store.ScopeQuota{WorkspaceID: workspace, ScopeType: "user", ScopeID: member, BudgetMicros: 100}); err != nil {
		t.Fatal(err)
	}
	first, second := NewQuotaTracker(st), NewQuotaTracker(st)
	principal := quotaTestPrincipal(t, workspace, member)
	allow := func(tracker *QuotaTracker) bool {
		return h.allowQuota(Dependencies{MultiTenancy: config.MultiTenancy{Enabled: true}, Quota: tracker}, httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil), principal, "p/m")
	}
	if !allow(first) || !allow(second) {
		t.Fatal("empty budget did not admit on both trackers")
	}
	if err := st.InsertSpendEntry(ctx, &store.SpendEntry{WorkspaceID: workspace, KeyID: keyID, UserID: &member, CostMicros: 150}); err != nil {
		t.Fatal(err)
	}
	if !allow(first) || !allow(second) {
		t.Fatal("fresh spend cache unexpectedly coordinated with ledger writes")
	}
	first.Invalidate(workspace, quotaScope{typ: "user", id: member})
	if allow(first) || !allow(second) || allow(NewQuotaTracker(st)) {
		t.Fatal("cache invalidation should be local; a fresh tracker should see durable exhaustion")
	}
}

func TestQuotaContractLateCompletionAfterOwnerDeletion(t *testing.T) {
	for _, scopeType := range []string{"user", "team"} {
		t.Run(scopeType, func(t *testing.T) {
			st, h, _, workspaceID, _, _, memberID, _ := quotaSetup(t)
			ctx := context.Background()
			workspace, member := mustParseUUID(workspaceID), mustParseUUID(memberID)
			teams, err := st.ListTeamsByWorkspace(ctx, workspace)
			if err != nil || len(teams) != 1 {
				t.Fatal(teams, err)
			}
			key := store.InboundKey{Name: uuid.NewString(), TokenHash: uuid.NewString(), TokenPrefix: "test", WorkspaceID: workspace, Enabled: true}
			dyn := auth.DynamicClient{Name: key.Name, TokenHash: store.TokenHash("late-fixture"), Workspace: identity.InWorkspace(workspace)}
			if scopeType == "user" {
				key.OwnerUserID, dyn.OwnerUserID = &member, member
			} else {
				key.OwnerTeamID, dyn.OwnerTeamID = &teams[0].ID, teams[0].ID
			}
			if err := st.CreateInboundKey(ctx, &key); err != nil {
				t.Fatal(err)
			}
			dyn.KeyID = key.ID
			p := authenticatedTestPrincipal(t, auth.NewAuthenticatorWithClients(config.Auth{Mode: config.AuthModeBearerStatic}, []auth.DynamicClient{dyn}), "late-fixture")
			deps := Dependencies{MultiTenancy: config.MultiTenancy{Enabled: true}, Quota: NewQuotaTracker(st), Catalog: config.NewCatalog(nil, nil, nil)}
			if err := h.persistQuotaUsageContext(ctx, deps, p, "p/m", "m", "p", provider.Usage{TotalTokens: 7}); err != nil {
				t.Fatal(err)
			}
			if scopeType == "user" {
				err = st.DeleteUser(ctx, member)
			} else {
				err = st.DeleteTeam(ctx, teams[0].ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			var pgErr pgdriver.Error
			if err := h.persistQuotaUsageContext(ctx, deps, p, "p/m", "m", "p", provider.Usage{TotalTokens: 9}); !errors.As(err, &pgErr) || pgErr.Field('C') != "23503" {
				t.Fatalf("late completion must fail deleted-owner foreign key: %v", err)
			}
			var rows []store.SpendEntry
			if err := st.DB.NewSelect().Model(&rows).Where("key_id = ?", key.ID).Scan(ctx); err != nil || len(rows) != 1 || rows[0].Tokens != 7 || rows[0].UserID != nil || rows[0].TeamID != nil {
				t.Fatalf("existing history vs failed late charge: %+v %v", rows, err)
			}
		})
	}
}
