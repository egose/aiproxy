package dashboard

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/dashrpc"
)

func measurementModel(t *testing.T, usage *accounting.Aggregator, catalog config.Catalog) (*model, dashrpc.Snapshot) {
	t.Helper()
	source := dashrpc.NewRuntimeSource(config.Dashboard{Enabled: true}, "test", ":8080", "none", time.Now(), catalog, usage, nil, nil)
	data, err := json.Marshal(source.Snapshot(t.Context(), 200))
	if err != nil {
		t.Fatal(err)
	}
	var wire dashrpc.Snapshot
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	return &model{snapshot: SnapshotFromTransport(wire), width: 240, height: 70, statsHeight: 40}, wire
}

func TestProductionRatesThroughRPCAndTUI(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 500000000, time.UTC)
	a := accounting.NewAggregatorWithClock(func() time.Time { return now })
	for i := 0; i < 600; i++ {
		status := 200
		if i < 60 {
			status = 500
		} else if i < 90 {
			status = 429
		}
		a.Record(accounting.Event{Timestamp: now.Add(-10 * time.Second), Model: "p/m", Provider: "p", UpstreamModel: "m",
			Tenant: "t", StatusCode: status, TotalTokens: 100, Duration: time.Duration(i+1) * time.Millisecond})
	}
	catalog := config.NewCatalog([]config.Provider{{Name: "p", BaseURL: "http://127.0.0.1"}}, nil, nil)
	m, wire := measurementModel(t, a, catalog)
	if len(wire.Recent) != 200 || wire.Rates.Minute.Requests != 600 || wire.Rates.Minute.Tokens != 60000 || wire.ProviderStats[0].Requests != 600 {
		t.Fatalf("production transport: rates=%+v recent=%d provider=%+v", wire.Rates, len(wire.Recent), wire.ProviderStats)
	}
	for _, label := range []string{"GLOBAL req/s 1m 10.00 5m 2.00", "err/1m 60 (10.0%)", "429/1m 30", "tok/1m 60,000", "req/min 15m", "1s buckets end 12:00:00", "Providers GLOBAL lifetime", "P95/n last≤200 (no time window)", "EST$ rolling 24h/60s"} {
		if got := renderRate(m, 240); !strings.Contains(got, label) {
			t.Fatalf("missing %q: %s", label, got)
		}
	}
	if got := renderProviders(m, 240, 12); !strings.Contains(got, "590ms/200") {
		t.Fatalf("P95 is received positive-duration sample, not all 600: %s", got)
	}
	before := renderRate(m, 240)
	m.tenantIndex, m.errorsOnly = 1, true
	if renderRate(m, 240) != before {
		t.Fatal("selected tenant/error filters changed global rates")
	}
	now = now.Add(2 * time.Minute)
	idle, _ := measurementModel(t, a, catalog)
	if got := renderRate(idle, 240); !strings.Contains(got, "tok/1m 0") || !strings.Contains(got, "1m 0.00 5m 2.00") {
		t.Fatalf("idle uses retained tokens as a rate: %s", got)
	}
	m.now = now.Add(time.Hour)
	if got := renderRate(m, 240); got != before {
		t.Fatal("local clock changed immutable transported rate window")
	}
}

func TestRetainedCostsThroughRPCAndTUIIsolateAliasesAndIdentities(t *testing.T) {
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	a := accounting.NewAggregatorWithClock(func() time.Time { return now })
	identities := []struct {
		tenant, client string
		scale          int64
	}{{"", "same", 1}, {"t", "same", 10}, {"t", "other", 100}, {"other", "same", 1000}}
	for _, id := range identities {
		for i, model := range []string{"alias/a", "alias/b", "p/m"} {
			tokens := []int64{1000, 3000, 6000}[i] * id.scale
			a.Record(accounting.Event{Model: model, Provider: "p", UpstreamModel: "m", Tenant: id.tenant, Client: id.client,
				Operation: "chat", StatusCode: 200, PromptTokens: tokens, TotalTokens: tokens, Duration: time.Millisecond})
		}
	}
	catalog := config.NewCatalog([]config.Provider{{Name: "p", BaseURL: "http://127.0.0.1", Models: []config.Model{{Name: "m", Pricing: &config.ModelPricing{InputPerMillion: 1}}}}}, nil,
		[]config.Alias{{Name: "a", Targets: []config.AliasTarget{{Provider: "unpriced-unused", Model: "m"}}}, {Name: "b"}})
	m, wire := measurementModel(t, a, catalog)
	if wire.Billing == nil || wire.Billing.RetentionSeconds != 86400 || wire.Billing.BucketSeconds != 60 || len(wire.Billing.Upstream) != 12 || len(wire.Upstream) != 4 {
		t.Fatalf("retained versus lifetime transport: %+v", wire.Billing)
	}
	prices := pricingIndex(m.snapshot.Providers)
	publicView := renderUsage(m, 240, 30)
	for _, label := range []string{"$0.0010", "$0.0030", "$0.0060"} {
		if !strings.Contains(publicView, label) {
			t.Fatalf("actual public/alias rendered costs missing %s: %s", label, publicView)
		}
	}
	for _, s := range m.snapshot.Usage.Summaries() {
		cost, ok := summaryCostWithAliases(s, prices, m.snapshot.Aliases, retainedUpstream(m.snapshot.Usage))
		if !ok || math.Abs(cost-float64(s.PromptTokens)/1e6) > 1e-9 {
			t.Fatalf("identity/alias misattribution: %+v cost=%v ok=%v", s, cost, ok)
		}
	}
	if got := providerCostTextWithAliases("p", wire.Billing.Usage, prices, nil, wire.Billing.Upstream); got != "$11.11" {
		t.Fatalf("provider subtotal duplicates aliases or direct traffic: %s", got)
	}
	if got := renderProviders(m, 240, 12); !strings.Contains(got, "$11.11") {
		t.Fatalf("actual provider rendered subtotal missing: %s", got)
	}
	for _, scope := range []struct {
		tenant, client string
		rows           int
		cost           string
	}{{"", "same", 3, "$0.01"}, {"t", "same", 6, "$1.10"}} {
		rows := accounting.FilterSummaries(wire.Billing.Usage, scope.tenant, scope.client)
		if len(rows) != scope.rows {
			t.Fatalf("scope %+v: %+v", scope, rows)
		}
		got := providerCostTextWithAliases("p", rows, prices, nil, wire.Billing.Upstream)
		if got != scope.cost {
			t.Fatalf("scoped provider subtotal: got %s want %s", got, scope.cost)
		}
	}
	m.tenantIndex = 2
	view := renderUsage(m, 240, 20)
	for _, label := range []string{"rolling 24h/60s", "estimated cost", "tenant:t", "alias/a", "alias/b"} {
		if !strings.Contains(view, label) {
			t.Fatalf("missing %q: %s", label, view)
		}
	}
	m.usageUpstream = true
	if got := m.filteredSummaries(); len(got) != 2 || got[0].Count != 3 || got[1].Count != 3 {
		t.Fatalf("retained upstream selected scope: %+v", got)
	}
	if got := renderUsage(m, 240, 20); !strings.Contains(got, "$0.10") || !strings.Contains(got, "$1.00") {
		t.Fatalf("upstream regrouped estimates: %s", got)
	}
	now = now.Add(24 * time.Hour)
	boundary, _ := measurementModel(t, a, catalog)
	if len(boundary.snapshot.Usage.Summaries()) != 12 {
		t.Fatal("billing inclusive boundary changed")
	}
	now = now.Add(time.Nanosecond)
	expired, expiredWire := measurementModel(t, a, catalog)
	if len(expiredWire.Billing.Usage) != 0 || len(expiredWire.Billing.Upstream) != 0 || expiredWire.ProviderStats[0].Requests != 12 || len(expiredWire.Upstream) != 4 || len(expiredWire.Recent) != 12 {
		t.Fatalf("expiration crossed measurement scopes: %+v", expiredWire)
	}
	if got := renderProviders(expired, 240, 12); strings.Contains(got, "$11.11") || !strings.Contains(got, "1ms/12") {
		t.Fatalf("retained cost expiry/lifetime sample: %s", got)
	}
	expired.usageUpstream = true
	if len(expired.filteredSummaries()) != 0 {
		t.Fatal("upstream toggle resurrected lifetime costs")
	}
}

func TestProductionCostPriceGapsAndProviderSubtotals(t *testing.T) {
	for _, missing := range []bool{false, true} {
		t.Run(fmt.Sprint(missing), func(t *testing.T) {
			a := accounting.NewAggregator()
			for _, p := range []string{"p", "q"} {
				a.Record(accounting.Event{Model: "alias/a", Provider: p, UpstreamModel: "m", StatusCode: 200, PromptTokens: 1000, TotalTokens: 1000})
			}
			qPrice := &config.ModelPricing{InputPerMillion: 3}
			if missing {
				qPrice = &config.ModelPricing{OutputPerMillion: 3}
			}
			catalog := config.NewCatalog([]config.Provider{
				{Name: "p", Models: []config.Model{{Name: "m", Pricing: &config.ModelPricing{InputPerMillion: 1}}}},
				{Name: "q", Models: []config.Model{{Name: "m", Pricing: qPrice}}},
			}, nil, nil)
			m, wire := measurementModel(t, a, catalog)
			prices := pricingIndex(m.snapshot.Providers)
			s := wire.Billing.Usage[0]
			got := summaryCostTextWithAliases(s, prices, nil, wire.Billing.Upstream)
			want := "$0.0040"
			if missing {
				want = "-"
			}
			if got != want {
				t.Fatalf("alias used-price gap: %s want %s", got, want)
			}
			if got := providerCostTextWithAliases("p", []accounting.Summary{s}, prices, nil, wire.Billing.Upstream); got != "$0.0010" {
				t.Fatalf("fully priced provider subtotal should survive other provider gap: %s", got)
			}
			qWant := "$0.0030"
			if missing {
				qWant = "-"
			}
			if got := providerCostTextWithAliases("q", []accounting.Summary{s}, prices, nil, wire.Billing.Upstream); got != qWant {
				t.Fatalf("provider price gap: %s", got)
			}
		})
	}
}

func TestOldSnapshotMeasurementsAreUnavailable(t *testing.T) {
	var wire dashrpc.Snapshot
	if err := json.Unmarshal([]byte(`{"usage":[{"Model":"alias/a","Count":600,"PromptTokens":60000}],"upstream":[{"Provider":"p","Model":"m","Count":600,"PromptTokens":60000}],"providers":[{"name":"p","models":[{"name":"m","input_per_million":1}]}],"recent":[]}`), &wire); err != nil {
		t.Fatal(err)
	}
	m := &model{snapshot: SnapshotFromTransport(wire), width: 240, statsHeight: 40}
	for _, label := range []string{"rates/15m graph unavailable", "provider counters unavailable", "window unavailable"} {
		if got := renderRate(m, 240); !strings.Contains(got, label) {
			t.Fatalf("missing fallback %q: %s", label, got)
		}
	}
	if got := renderUsage(m, 240, 12); strings.Contains(got, "$0.") || !strings.Contains(got, "window unavailable") {
		t.Fatalf("fabricated old-snapshot cost/window: %s", got)
	}
	if got := renderProviders(m, 240, 12); !strings.Contains(got, "n/a") || strings.Contains(got, "0.0%") {
		t.Fatalf("fabricated lifetime stats: %s", got)
	}
	m.usageUpstream = true
	if got := renderUsage(m, 240, 12); !strings.Contains(got, "retained upstream usage unavailable") {
		t.Fatalf("lifetime fallback for retained upstream: %s", got)
	}
}
