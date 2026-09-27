package httpapi

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/provider"
)

func TestBillingRetainedAliasAttribution(t *testing.T) {
	for _, missingPrice := range []bool{false, true} {
		t.Run(map[bool]string{false: "different-prices", true: "missing-used-price"}[missingPrice], func(t *testing.T) {
			cheap := &config.ModelPricing{InputPerMillion: 1, OutputPerMillion: 2}
			expensive := &config.ModelPricing{InputPerMillion: 10, OutputPerMillion: 20}
			if missingPrice {
				expensive = nil
			}
			catalog := config.NewCatalog([]config.Provider{
				{Name: "cheap", Models: []config.Model{{Name: "m", Pricing: cheap}}},
				{Name: "expensive", Models: []config.Model{{Name: "m", Pricing: expensive}}},
			}, nil, []config.Alias{
				{Name: "a", Targets: []config.AliasTarget{{Provider: "cheap", Model: "m"}, {Provider: "expensive", Model: "m"}}},
				{Name: "b", Targets: []config.AliasTarget{{Provider: "cheap", Model: "m"}, {Provider: "expensive", Model: "m"}}},
			})
			usage := accounting.NewAggregator()
			clients := map[string]config.Client{}
			for i, identity := range []struct{ tenant, client string }{
				{"team-a", "a1"}, {"team-a", "a2"}, {"team-b", "b1"}, {"", "c1"}, {"", "c2"},
			} {
				clients[identity.client] = config.Client{Name: identity.client, Token: identity.client, Tenant: identity.tenant}
				for _, traffic := range []struct {
					model, provider string
					tokens          int64
				}{
					{"alias/a", "cheap", 1000000}, {"alias/a", "expensive", 2000000},
					{"alias/b", "cheap", 3000000}, {"cheap/m", "cheap", 4000000},
				} {
					n := traffic.tokens * int64(i+1)
					usage.Record(accounting.Event{Tenant: identity.tenant, Client: identity.client,
						Model: traffic.model, Provider: traffic.provider, UpstreamModel: "m",
						Operation: "responses", StatusCode: 200, PromptTokens: n, CompletionTokens: n, TotalTokens: 2 * n})
				}
			}
			h := NewHandler(Dependencies{Auth: auth.NewAuthenticator(config.Auth{Mode: config.AuthModeBearerStatic, Clients: clients}), Catalog: catalog, Usage: usage})
			for _, caller := range []string{"a1", "b1", "c1", "c2"} {
				w := httptest.NewRecorder()
				r := httptest.NewRequest(http.MethodGet, "/v1/billing/usage", nil)
				r.Header.Set("Authorization", "Bearer "+caller)
				h.ServeHTTP(w, r)
				if w.Code != http.StatusOK {
					t.Fatalf("billing %s: %d %s", caller, w.Code, w.Body)
				}
				var resp billingUsageResponse
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatal(err)
				}
				wantRows := 3
				if caller == "a1" {
					wantRows = 6
				}
				if len(resp.Data) != wantRows {
					t.Fatalf("%s rows = %+v", caller, resp.Data)
				}
				for _, row := range resp.Data {
					if row.Tenant != clients[caller].Tenant || (row.Tenant == "" && row.Client != caller) {
						t.Fatalf("%s received another identity: %+v", caller, row)
					}
					if missingPrice && row.Model == "alias/a" {
						if row.EstimatedCostUSD != nil {
							t.Errorf("incomplete alias estimate = %v", *row.EstimatedCostUSD)
						}
						continue
					}
					factor := map[string]float64{"a1": 1, "a2": 2, "b1": 3, "c1": 4, "c2": 5}[row.Client]
					want := map[string]float64{"alias/a": 63, "alias/b": 9, "cheap/m": 12}[row.Model] * factor
					if row.EstimatedCostUSD == nil || math.Abs(*row.EstimatedCostUSD-want) > 1e-9 {
						t.Errorf("%s/%s cost = %v, want %v", row.Client, row.Model, row.EstimatedCostUSD, want)
					}
				}
			}
		})
	}
}

func TestBillingLiveAliasWindowExpiry(t *testing.T) {
	rt := twoProviderAliasRT(config.AlgorithmRoundRobin, nil)
	providers := rt.Catalog.Providers()
	for i := range providers {
		providers[i].Models[0].UpstreamName = "wire-model"
		providers[i].Models[0].Pricing = &config.ModelPricing{InputPerMillion: float64(i + 1), OutputPerMillion: float64(2 * (i + 1))}
	}
	aliases := append(rt.Catalog.Aliases(), config.Alias{Name: "b", Algorithm: config.AlgorithmRoundRobin,
		Targets: []config.AliasTarget{{Provider: "p1", Model: "m"}}})
	rt.Catalog = config.NewCatalog(providers, nil, aliases)
	now := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	usage := accounting.NewAggregatorWithClock(func() time.Time { return now })
	adapter := &stubAdapter{result: &provider.Result{StatusCode: 200, Body: []byte(`{}`),
		Usage: provider.Usage{PromptTokens: 1000000, CompletionTokens: 1000000, TotalTokens: 2000000}}}
	h := NewHandler(Dependencies{Resolver: modelresolver.New(rt), Adapter: adapter,
		Auth: auth.NewAuthenticator(config.Auth{Mode: config.AuthModeNone}), Catalog: rt.Catalog, Usage: usage,
		Accounting: accounting.RecorderFunc(func(e accounting.Event) { e.Timestamp = time.Time{}; usage.Record(e) })})
	infer := func(model string) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(`{"model":"`+model+`","messages":[]}`)))
		if w.Code != 200 {
			t.Fatalf("infer %s: %d %s", model, w.Code, w.Body)
		}
		if adapter.got.UpstreamModel != "wire-model" {
			t.Fatalf("upstream model = %q", adapter.got.UpstreamModel)
		}
	}
	check := func(want map[string]float64) {
		t.Helper()
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/v1/billing/usage", nil))
		var resp billingUsageResponse
		if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &resp) != nil {
			t.Fatalf("billing: %d %s", w.Code, w.Body)
		}
		if len(resp.Data) != len(want) {
			t.Fatalf("billing rows: %+v, want %v", resp.Data, want)
		}
		for _, row := range resp.Data {
			cost, ok := want[row.Model]
			if !ok || row.EstimatedCostUSD == nil || math.Abs(*row.EstimatedCostUSD-cost) > 1e-9 {
				t.Fatalf("row %+v, want %v", row, want)
			}
		}
	}
	infer("alias/a")
	infer("alias/a")
	infer("alias/b")
	infer("p1/m")
	check(map[string]float64{"alias/a": 9, "alias/b": 3, "p1/m": 3})
	now = now.Add(time.Hour)
	infer("alias/a")
	check(map[string]float64{"alias/a": 12, "alias/b": 3, "p1/m": 3})
	lifetime, providerTotals := usage.UpstreamSummaries(), usage.ProviderSummaries()
	now = now.Add(23 * time.Hour)
	check(map[string]float64{"alias/a": 12, "alias/b": 3, "p1/m": 3})
	now = now.Add(time.Nanosecond)
	check(map[string]float64{"alias/a": 3})
	now = now.Add(time.Hour)
	check(map[string]float64{})
	if !reflect.DeepEqual(lifetime, usage.UpstreamSummaries()) || !reflect.DeepEqual(providerTotals, usage.ProviderSummaries()) {
		t.Fatal("billing expiry reset dashboard lifetime counters")
	}
}

func TestBillingAliasIncompleteAttribution(t *testing.T) {
	base := accounting.Event{Model: "alias/a", Provider: "p", UpstreamModel: "m", Operation: "responses", StatusCode: 200,
		PromptTokens: 1000000, CompletionTokens: 1000000, TotalTokens: 2000000}
	for _, tc := range []struct {
		name   string
		change func(*accounting.Event)
		price  *config.ModelPricing
		priced bool
	}{
		{"complete", func(*accounting.Event) {}, &config.ModelPricing{InputPerMillion: 1, OutputPerMillion: 2}, true},
		{"missing-target", func(e *accounting.Event) { e.Provider = "" }, &config.ModelPricing{InputPerMillion: 1}, false},
		{"missing-price", func(*accounting.Event) {}, nil, false},
		{"empty-price", func(*accounting.Event) {}, &config.ModelPricing{}, false},
		{"missing-output-rate", func(*accounting.Event) {}, &config.ModelPricing{InputPerMillion: 1}, false},
		{"missing-input-rate", func(*accounting.Event) {}, &config.ModelPricing{OutputPerMillion: 2}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := accounting.NewAggregator()
			a.Record(base)
			e := base
			tc.change(&e)
			a.Record(e)
			s, u := a.BillingSummaries()
			_, priced := billingAliasCost(s[0], map[string]*config.ModelPricing{"p/m": tc.price}, u)
			if priced != tc.priced {
				t.Fatalf("priced = %v, want %v", priced, tc.priced)
			}
		})
	}
	s := accounting.Summary{Model: base.Model, Operation: base.Operation, StatusCode: base.StatusCode, Count: 1, PromptTokens: base.PromptTokens}
	prices := map[string]*config.ModelPricing{"p/m": {InputPerMillion: 1}}
	if _, ok := billingAliasCost(s, prices, nil); ok {
		t.Fatal("missing attribution must not use an even split")
	}
	for _, mismatch := range []string{"public-model", "tenant", "client", "operation", "status", "tokens", "count"} {
		t.Run(mismatch, func(t *testing.T) {
			u := accounting.UpstreamSummary{PublicModel: s.Model, Provider: "p", Model: "m", Operation: s.Operation, StatusCode: s.StatusCode, Count: 1, PromptTokens: s.PromptTokens}
			switch mismatch {
			case "public-model":
				u.PublicModel = "alias/b"
			case "tenant":
				u.Tenant = "other"
			case "client":
				u.Client = "other"
			case "operation":
				u.Operation = "chat_completions"
			case "status":
				u.StatusCode = 500
			case "tokens":
				u.PromptTokens++
			case "count":
				u.Count++
			}
			if _, ok := billingAliasCost(s, prices, []accounting.UpstreamSummary{u}); ok {
				t.Fatalf("mismatched attribution accepted: %+v", u)
			}
		})
	}
}

func TestBillingModelPriceCompleteness(t *testing.T) {
	for _, tc := range []struct {
		name                                    string
		price                                   config.ModelPricing
		prompt, completion, cached, write, read int64
		want                                    float64
		priced                                  bool
	}{
		{name: "generic-cache-missing", price: config.ModelPricing{InputPerMillion: 1}, prompt: 1000000, cached: 1000000},
		{name: "cache-read-input-fallback", price: config.ModelPricing{InputPerMillion: 1}, prompt: 1000000, cached: 1000000, read: 1000000, want: 1, priced: true},
		{name: "cache-write-input-fallback", price: config.ModelPricing{InputPerMillion: 1}, prompt: 1000000, cached: 1000000, write: 1000000, want: 1, priced: true},
		{name: "cache-read-missing", price: config.ModelPricing{OutputPerMillion: 1}, prompt: 1000000, cached: 1000000, read: 1000000},
		{name: "cache-write-missing", price: config.ModelPricing{CachedPerMillion: 1}, prompt: 1000000, cached: 1000000, write: 1000000},
		{name: "only-cache-price-needed", price: config.ModelPricing{CachedPerMillion: 2}, prompt: 1000000, cached: 1000000, want: 2, priced: true},
		{name: "zero-usage-priced", price: config.ModelPricing{InputPerMillion: 1}, priced: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := accounting.Summary{Model: "p/m", PromptTokens: tc.prompt, CompletionTokens: tc.completion, CachedTokens: tc.cached, CacheCreationTokens: tc.write, CacheReadTokens: tc.read}
			cost, priced := billingCost(s, map[string]*config.ModelPricing{"p/m": &tc.price}, nil)
			if priced != tc.priced || math.Abs(cost-tc.want) > 1e-9 {
				t.Fatalf("cost = %v/%v, want %v/%v", cost, priced, tc.want, tc.priced)
			}
		})
	}
}
