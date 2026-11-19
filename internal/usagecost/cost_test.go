package usagecost

import (
	"math"
	"testing"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/google/uuid"
)

func TestAliasCostJoinsImmutableIdentityAndWorkspace(t *testing.T) {
	key := identity.DatabaseKeyID(uuid.New())
	workspace := identity.InWorkspace(uuid.New())
	s := accounting.Summary{CredentialID: key, WorkspaceScope: workspace, Tenant: "shared", Client: "shared",
		Model: "alias/a", Operation: "chat_completions", StatusCode: 200, Count: 1,
		PromptTokens: 1000000, CompletionTokens: 1000000, TotalTokens: 2000000}
	own := accounting.UpstreamSummary{CredentialID: key, WorkspaceScope: workspace, Tenant: s.Tenant, Client: s.Client,
		PublicModel: s.Model, Provider: "cheap", Model: "m", Operation: s.Operation, StatusCode: s.StatusCode, Count: s.Count,
		PromptTokens: s.PromptTokens, CompletionTokens: s.CompletionTokens, TotalTokens: s.TotalTokens}
	prices := map[string]*config.ModelPricing{
		"cheap/m":     {InputPerMillion: 1, OutputPerMillion: 2},
		"expensive/m": {InputPerMillion: 100, OutputPerMillion: 200},
	}
	for _, mismatch := range []string{"credential", "workspace", "static", "anonymous", "unscoped", "tenant-edit", "name-edit", "alias", "direct", "operation", "status"} {
		t.Run(mismatch, func(t *testing.T) {
			foreign := own
			foreign.Provider = "expensive"
			switch mismatch {
			case "credential":
				foreign.CredentialID = identity.DatabaseKeyID(uuid.New())
			case "workspace":
				foreign.WorkspaceScope = identity.InWorkspace(uuid.Nil)
			case "static":
				foreign.CredentialID = identity.StaticClientID("shared")
			case "anonymous":
				foreign.CredentialID, foreign.WorkspaceScope = identity.CredentialID{}, identity.WorkspaceScope{}
			case "unscoped":
				foreign.WorkspaceScope = identity.WorkspaceScope{}
			case "tenant-edit":
				foreign.Tenant = "edited"
			case "name-edit":
				foreign.Client = "edited"
			case "alias":
				foreign.PublicModel = "alias/b"
			case "direct":
				foreign.PublicModel = "cheap/m"
			case "operation":
				foreign.Operation = "responses"
			case "status":
				foreign.StatusCode = 500
			}
			if _, ok := AliasCost(s, prices, []accounting.UpstreamSummary{foreign}); ok {
				t.Fatal("foreign identical-token attribution priced another key")
			}
			upstream := []accounting.UpstreamSummary{foreign, own}
			if cost, ok := Cost(s, prices, upstream); !ok || math.Abs(cost-3) > 1e-9 {
				t.Fatalf("own alias cost = %v/%v, want 3", cost, ok)
			}
			if cost, ok := ProviderCost("cheap", []accounting.Summary{s}, prices, upstream); !ok || math.Abs(cost-3) > 1e-9 {
				t.Fatalf("own provider subtotal = %v/%v, want 3", cost, ok)
			}
			if cost, ok := ProviderCost("expensive", []accounting.Summary{s}, prices, upstream); ok || cost != 0 {
				t.Fatalf("foreign provider subtotal = %v/%v", cost, ok)
			}
		})
	}
	unscoped := s
	unscoped.CredentialID, unscoped.WorkspaceScope = identity.CredentialID{}, identity.WorkspaceScope{}
	if _, ok := AliasCost(unscoped, prices, []accounting.UpstreamSummary{own}); ok {
		t.Fatal("unscoped summary joined a scoped key")
	}
}

func TestCostsBeforeOperatorAggregationRetainKeyAttribution(t *testing.T) {
	a := accounting.NewAggregator()
	keys := []identity.CredentialID{
		identity.StaticClientID("shared"), identity.DatabaseKeyID(uuid.New()), identity.DatabaseKeyID(uuid.New()),
	}
	workspaces := []identity.WorkspaceScope{
		identity.InWorkspace(uuid.Nil), identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New()),
	}
	prices := map[string]*config.ModelPricing{
		"cheap/m": {InputPerMillion: 1}, "expensive/m": {InputPerMillion: 10},
	}
	for i, key := range keys {
		target := []string{"cheap", "expensive", "cheap"}[i]
		a.Record(accounting.Event{CredentialID: key, WorkspaceScope: workspaces[i], Tenant: "shared", Client: "shared",
			Model: "alias/a", Provider: target, UpstreamModel: "m", Operation: "chat_completions", StatusCode: 200,
			PromptTokens: 1000000, TotalTokens: 1000000})
	}
	summaries, upstream := a.BillingSummaries()
	for i, key := range keys {
		rows := accounting.FilterSummaries(summaries, accounting.CredentialUsage(key, workspaces[i]))
		if len(rows) != 1 {
			t.Fatalf("key totals merged: %+v", rows)
		}
		want := []float64{1, 10, 1}[i]
		if cost, ok := Cost(rows[0], prices, upstream); !ok || cost != want {
			t.Fatalf("key cost = %v/%v, want %v", cost, ok, want)
		}
	}
	operator := a.OperatorBillingSnapshot()
	if len(operator.Usage) != 1 {
		t.Fatalf("operator label aggregate lost: %+v", operator.Usage)
	}
	if cost, ok := Cost(operator.Usage[0], prices, operator.Upstream); !ok || cost != 12 {
		t.Fatalf("explicit operator aggregate = %v/%v, want 12", cost, ok)
	}
}
