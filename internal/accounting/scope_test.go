package accounting

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/identity"
	"github.com/google/uuid"
)

func TestBillingGroupsImmutableIdentityBeforeLabels(t *testing.T) {
	system := identity.InWorkspace(uuid.Nil)
	a, b := identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New())
	key := identity.DatabaseKeyID(uuid.New())
	replacement := identity.DatabaseKeyID(uuid.New())
	events := []Event{
		{CredentialID: identity.StaticClientID("shared"), WorkspaceScope: system, PromptTokens: 1},
		{CredentialID: identity.StaticClientID("other"), WorkspaceScope: system, PromptTokens: 2},
		{CredentialID: identity.DatabaseKeyID(uuid.New()), WorkspaceScope: system, PromptTokens: 4},
		{CredentialID: key, WorkspaceScope: a, PromptTokens: 8},
		{CredentialID: identity.DatabaseKeyID(uuid.New()), WorkspaceScope: b, PromptTokens: 16},
		{CredentialID: key, WorkspaceScope: b, PromptTokens: 32},
		{PromptTokens: 64},
		{WorkspaceScope: system, PromptTokens: 128},
		{CredentialID: replacement, WorkspaceScope: a, PromptTokens: 256},
	}
	for i := range events {
		events[i].Tenant, events[i].Client = "shared", "shared"
		events[i].Model, events[i].Provider, events[i].UpstreamModel = "alias/shared", "p", "m"
		events[i].Operation, events[i].StatusCode = "chat_completions", 200
		events[i].CompletionTokens, events[i].TotalTokens = events[i].PromptTokens, 2*events[i].PromptTokens
	}
	rotated := events[3]
	edited := events[3]
	edited.Tenant, edited.Client = "edited-tenant", "edited-name"
	for _, recorder := range []struct {
		name string
		new  func() interface {
			Recorder
			Reader
		}
	}{
		{"aggregator", func() interface {
			Recorder
			Reader
		} {
			return NewAggregator()
		}},
		{"memory", func() interface {
			Recorder
			Reader
		} {
			return &MemoryRecorder{}
		}},
	} {
		t.Run(recorder.name, func(t *testing.T) {
			r := recorder.new()
			for _, event := range append(append([]Event(nil), events...), rotated, edited) {
				r.Record(event)
			}
			summaries, upstream := r.BillingSummaries()
			if len(summaries) != len(events)+1 || len(upstream) != len(summaries) || len(r.Summaries()) != len(summaries) {
				t.Fatalf("identity merged before filtering: %+v %+v", summaries, upstream)
			}
			again, againUpstream := r.BillingSummaries()
			if !reflect.DeepEqual(summaries, again) || !reflect.DeepEqual(upstream, againUpstream) {
				t.Fatal("identical-label identity rows have unstable ordering")
			}
			for _, test := range []struct {
				name   string
				filter Filter
				rows   int
				count  int64
				tokens int64
			}{
				{"own-edited-and-rotated-key", CredentialUsage(key, a), 2, 3, 24},
				{"wrong-workspace", CredentialUsage(key, system), 0, 0, 0},
				{"same-key-other-workspace", CredentialUsage(key, b), 1, 1, 32},
				{"replacement", CredentialUsage(replacement, a), 1, 1, 256},
				{"static-tenant", StaticTenantUsage("shared", system), 2, 2, 3},
				{"static-client", CredentialUsage(events[0].CredentialID, system), 1, 1, 1},
				{"system-key", CredentialUsage(events[2].CredentialID, system), 1, 1, 4},
				{"missing-filter", Filter{}, 0, 0, 0},
				{"missing-credential", CredentialUsage(identity.CredentialID{}, system), 0, 0, 0},
				{"missing-workspace", CredentialUsage(key, identity.WorkspaceScope{}), 0, 0, 0},
				{"empty-static-tenant", StaticTenantUsage("", system), 0, 0, 0},
				{"anonymous-file-only-all", AllUsage(), len(events) + 1, int64(len(events)) + 2, 527},
			} {
				t.Run(test.name, func(t *testing.T) {
					rows := FilterSummaries(summaries, test.filter)
					targets := FilterUpstreamSummaries(upstream, test.filter)
					var count, tokens, targetCount, targetTokens int64
					for _, row := range rows {
						count += row.Count
						tokens += row.PromptTokens
					}
					for _, row := range targets {
						targetCount += row.Count
						targetTokens += row.PromptTokens
					}
					if len(rows) != test.rows || len(targets) != test.rows ||
						count != test.count || tokens != test.tokens || targetCount != count || targetTokens != tokens {
						t.Fatalf("scope = %d/%d %d/%d tokens %d/%d, want %d rows %d requests %d tokens",
							len(rows), len(targets), count, targetCount, tokens, targetTokens, test.rows, test.count, test.tokens)
					}
				})
			}
		})
	}
}

func TestAccountingIdentityIsInternalToJSON(t *testing.T) {
	key, workspace := uuid.New(), uuid.New()
	event := Event{CredentialID: identity.DatabaseKeyID(key), WorkspaceScope: identity.InWorkspace(workspace),
		Client: "label", Tenant: "label", Model: "alias/a", Provider: "p", UpstreamModel: "m", PromptTokens: 1}
	a := NewAggregator()
	a.Record(event)
	snapshot := a.BillingSnapshot()
	for _, value := range []any{event, snapshot.Usage, snapshot.Upstream, snapshot, a.Recent(1), a.UpstreamSummaries()} {
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"CredentialID", "WorkspaceScope", key.String(), workspace.String()} {
			if strings.Contains(string(data), forbidden) {
				t.Fatalf("internal accounting identity in JSON: %s", data)
			}
		}
	}
}

func TestStaticReportingPreservesTenantAndTenantlessContract(t *testing.T) {
	system := identity.InWorkspace(uuid.Nil)
	credential := identity.StaticClientID("static")
	rows := []Summary{
		{CredentialID: credential, WorkspaceScope: system, Tenant: "old", Client: "static"},
		{CredentialID: credential, WorkspaceScope: system, Client: "static"},
		{CredentialID: identity.DatabaseKeyID(uuid.New()), WorkspaceScope: system, Client: "static"},
		{CredentialID: identity.StaticClientID("peer"), WorkspaceScope: system, Tenant: "old", Client: "peer"},
		{CredentialID: identity.DatabaseKeyID(uuid.New()), WorkspaceScope: identity.InWorkspace(uuid.New()), Tenant: "old", Client: "static"},
	}
	if got := FilterSummaries(rows, StaticClientUsage(credential, system)); len(got) != 1 || got[0].Tenant != "" {
		t.Fatalf("tenantless static reporting changed: %+v", got)
	}
	if got := FilterSummaries(rows, StaticTenantUsage("old", system)); len(got) != 2 || got[1].Client != "peer" {
		t.Fatalf("trusted static tenant reporting changed: %+v", got)
	}
	if got := FilterSummaries(rows, CredentialUsage(credential, system)); len(got) != 2 {
		t.Fatalf("immutable own-credential filter lost label history: %+v", got)
	}
}

func TestOperatorAggregationIsExplicitAndDetached(t *testing.T) {
	a := NewAggregator()
	for _, workspace := range []identity.WorkspaceScope{identity.InWorkspace(uuid.Nil), identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New())} {
		a.Record(Event{CredentialID: identity.DatabaseKeyID(uuid.New()), WorkspaceScope: workspace,
			Client: "shared", Tenant: "shared", Model: "alias/a", Provider: "p", UpstreamModel: "m", PromptTokens: 10})
	}
	snapshot := a.OperatorBillingSnapshot()
	if len(snapshot.Usage) != 1 || len(snapshot.Upstream) != 1 || snapshot.Usage[0].Count != 3 || snapshot.Upstream[0].PromptTokens != 30 ||
		snapshot.Usage[0].CredentialID.Valid() || snapshot.Usage[0].WorkspaceScope != (identity.WorkspaceScope{}) ||
		snapshot.Upstream[0].CredentialID.Valid() || snapshot.Upstream[0].WorkspaceScope != (identity.WorkspaceScope{}) {
		t.Fatalf("operator aggregation: %+v", snapshot)
	}
	snapshot.Usage[0].Count, snapshot.Upstream[0].Count = -1, -1
	summaries, upstream := a.BillingSummaries()
	if len(summaries) != 3 || len(upstream) != 3 || len(a.UpstreamSummaries()) != 3 ||
		a.ProviderSummaries()[0].Requests != 3 {
		t.Fatal("operator view replaced credential-scoped storage or lifetime aggregates")
	}
	for _, row := range summaries {
		if !row.CredentialID.Valid() || row.Count != 1 {
			t.Fatalf("operator snapshot mutated key totals: %+v", row)
		}
	}
}
