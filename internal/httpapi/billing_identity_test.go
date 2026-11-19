package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestBillingRetainedModelNamesDoNotUseForeignWorkspacePrices(t *testing.T) {
	workspace := identity.InWorkspace(uuid.New())
	keyID := uuid.New()
	dyn := []auth.DynamicClient{{Name: "shared", Tenant: "shared", TokenHash: store.TokenHash("fixture"),
		KeyID: keyID, Workspace: workspace}}
	cfg := config.Auth{Mode: config.AuthModeBearerStatic}
	usage := accounting.NewAggregator()
	for _, model := range []string{"retained/m", "alias/retained"} {
		usage.Record(accounting.Event{CredentialID: identity.DatabaseKeyID(keyID), WorkspaceScope: workspace,
			Tenant: "shared", Client: "shared", Model: model, Provider: "retained", UpstreamModel: "m",
			Operation: "chat_completions", StatusCode: 200, PromptTokens: 1000000, CompletionTokens: 1000000, TotalTokens: 2000000})
	}
	deps := Dependencies{Auth: auth.NewAuthenticatorWithClients(cfg, dyn), Usage: usage, MultiTenancy: config.MultiTenancy{Enabled: true}}
	h := NewHandler(deps)
	for _, scope := range []identity.WorkspaceScope{workspace, identity.InWorkspace(uuid.New()), identity.InWorkspace(store.SystemWorkspaceID), {}} {
		deps.Catalog = config.NewCatalog([]config.Provider{{
			Name: "retained", Workspace: scope, Models: []config.Model{{Name: "m", Pricing: &config.ModelPricing{InputPerMillion: 1, OutputPerMillion: 2}}},
		}}, nil, nil)
		h.UpdateDependencies(deps)
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/v1/billing/usage?scope=workspace&workspace_id="+uuid.NewString(), nil)
		r.Header.Set("Authorization", "Bearer fixture")
		r.Header.Set("X-Workspace-Id", uuid.NewString())
		h.ServeHTTP(w, r)
		var body billingUsageResponse
		if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &body) != nil || len(body.Data) != 2 {
			t.Fatalf("retained rows lost: %d %s", w.Code, w.Body.String())
		}
		for _, row := range body.Data {
			if row.Count != 1 || row.TotalTokens != 2000000 {
				t.Fatalf("scope-broadening selectors changed own-key totals: %+v", row)
			}
			if scope == workspace {
				if row.EstimatedCostUSD == nil || *row.EstimatedCostUSD != 3 {
					t.Fatalf("own current workspace price unavailable: %+v", row)
				}
			} else if row.EstimatedCostUSD != nil {
				t.Fatalf("a retained name used a foreign/System/unscoped price: %+v", row)
			}
		}
	}
}
