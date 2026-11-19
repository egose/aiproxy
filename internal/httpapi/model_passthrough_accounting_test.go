package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/auth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/ratelimit"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

const (
	passthroughFamilyTarget = "@passthrough/model-family"
	passthroughFamilyModel  = "router/" + passthroughFamilyTarget
	passthroughPricedPin    = `
  model "org/pin" {
    upstream_name = "org/variant-00"
    pricing {
      input_per_million = 2
      output_per_million = 3
    }
  }
`
	passthroughUsageSSE = "data: {\"choices\":[{\"delta\":{\"content\":\"fixture\"}}]}\n\n" +
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":13,\"total_tokens\":20}}\n\n" +
		"data: [DONE]\n\n"
)

func passthroughUsageUpstream(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	calls := new(atomic.Int32)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			Model  string `json:"model"`
			Stream bool   `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || r.URL.Path != passthroughChatPath || input.Model == "" {
			t.Errorf("unexpected upstream request: %s model=%q err=%v", r.URL.Path, input.Model, err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if input.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, passthroughUsageSSE)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"model":%q,"choices":[],"usage":{"prompt_tokens":4,"completion_tokens":6,"total_tokens":10}}`, input.Model)
	}))
	t.Cleanup(upstream.Close)
	return upstream, calls
}

func enablePassthroughUsage(deps *Dependencies) *accounting.Aggregator {
	usage := accounting.NewAggregator()
	deps.Metrics, deps.MetricsToken = observability.NewMetrics(), metricsTestToken
	deps.Usage, deps.Accounting = usage, accounting.NewMulti(deps.Metrics, usage)
	return usage
}

func decodePassthroughBilling(t *testing.T, w *httptest.ResponseRecorder) []billingUsageEntry {
	t.Helper()
	var response billingUsageResponse
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &response) != nil || response.Object != "list" {
		t.Fatalf("billing response: %d %s", w.Code, w.Body)
	}
	return response.Data
}

func TestModelPassthroughAggregatesUsageAndSelection(t *testing.T) {
	const (
		variants    = 64
		payloadMax  = 96
		streamModel = "router/org/variant-stream"
	)
	upstream, calls := passthroughUsageUpstream(t)
	rt := loadPassthroughRuntime(t, passthroughAnon, passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughPricedPin+passthroughFamily+`
  model_passthrough "second-family" {
    match = "^vendor/.+$"
    capabilities = ["chat"]
  }
`))
	deps := passthroughDeps(rt, upstream)
	usage := enablePassthroughUsage(&deps)
	payloads := &deferredPayloadRecorder{max: payloadMax}
	var logs bytes.Buffer
	deps.AccessLog, deps.PayloadLog, deps.Logger = true, payloads, slog.New(slog.NewTextHandler(&logs, nil))
	h := NewHandler(deps)

	chat := func(model, wireModel string) {
		t.Helper()
		w := passthroughChat(h, "", model)
		if w.Code != http.StatusOK || extractModel(w.Body.Bytes()) != wireModel {
			t.Fatalf("%q did not reach upstream as %q: %d %s", model, wireModel, w.Code, w.Body)
		}
	}
	for i := range variants {
		suffix := fmt.Sprintf("org/variant-%02d", i)
		chat("router/"+suffix, suffix)
	}
	chat("router/vendor/one", "vendor/one")
	chat("router/vendor/two", "vendor/two")
	chat("router/org/pin", "org/variant-00")
	stream := passthroughJSON(h, passthroughChatPath, "", fmt.Sprintf(`{"model":%q,"messages":[],"stream":true}`, streamModel))
	if stream.Code != http.StatusOK || stream.Body.String() != passthroughUsageSSE || calls.Load() != variants+4 {
		t.Fatalf("stream did not complete or routing made extra calls: %d %q calls=%d", stream.Code, stream.Body.String(), calls.Load())
	}

	billingResponse := passthroughRequest(h, http.MethodGet, "/v1/billing/usage", "", "", "")
	billing := decodePassthroughBilling(t, billingResponse)
	_, retained := usage.BillingSummaries()
	lifetime := usage.UpstreamSummaries()
	metrics := collectMetrics(t, h)
	usageSeries := strings.Count(metrics, "\naiproxy_usage_events_total{")
	selectionSeries := strings.Count(metrics, "\naiproxy_provider_selections_total{")
	if len(billing) != 3 || len(retained) != 3 || len(lifetime) != 3 || usageSeries != 3 || selectionSeries != 3 {
		t.Fatalf("expected one row/series per rule or literal: billing=%d retained=%d lifetime=%d usage series=%d selection series=%d",
			len(billing), len(retained), len(lifetime), usageSeries, selectionSeries)
	}
	for _, want := range []struct {
		target                    string
		count, prompt, completion int64
		cost                      float64
	}{
		{passthroughFamilyTarget, variants + 1, variants*4 + 7, variants*6 + 13, 0},
		{"@passthrough/second-family", 2, 8, 12, 0},
		{"org/pin", 1, 4, 6, 0.000026},
	} {
		model := "router/" + want.target
		summary := accounting.Summary{Model: model, Operation: "chat_completions", StatusCode: http.StatusOK,
			Count: want.count, PromptTokens: want.prompt, CompletionTokens: want.completion, TotalTokens: want.prompt + want.completion}
		i := slices.IndexFunc(billing, func(row billingUsageEntry) bool { return row.Model == model })
		if i < 0 || billing[i].Summary != summary {
			t.Fatalf("billing for %q: want %+v in %+v", model, summary, billing)
		}
		if cost := billing[i].EstimatedCostUSD; (want.cost == 0 && cost != nil) ||
			(want.cost > 0 && (cost == nil || math.Abs(*cost-want.cost) > 1e-12)) {
			t.Errorf("incorrect pricing for %q: got %v, want %g (0 means unpriced)", model, billing[i], want.cost)
		}
		target := accounting.UpstreamSummary{PublicModel: model, Provider: "router", Model: want.target,
			Operation: summary.Operation, StatusCode: summary.StatusCode, Count: summary.Count,
			PromptTokens: summary.PromptTokens, CompletionTokens: summary.CompletionTokens, TotalTokens: summary.TotalTokens}
		if !slices.Contains(retained, target) {
			t.Errorf("missing retained target %+v: %v", target, retained)
		}
		target.PublicModel = ""
		if !slices.Contains(lifetime, target) {
			t.Errorf("missing lifetime target %+v: %v", target, lifetime)
		}
		eventSeries := fmt.Sprintf(`aiproxy_usage_events_total{client="anonymous",model=%q,operation="chat_completions",status="200",tenant="anonymous"}`, model)
		targetSeries := fmt.Sprintf(`aiproxy_provider_selections_total{model=%q,operation="chat_completions",provider="router",public_model=%q}`, want.target, model)
		if got := metricValue(t, metrics, eventSeries); got != float64(want.count) {
			t.Errorf("usage events for %q = %g, want %d", model, got, want.count)
		}
		if got := metricValue(t, metrics, targetSeries); got != float64(want.count) {
			t.Errorf("selections for %q = %g, want %d", model, got, want.count)
		}
	}
	for _, raw := range []string{"variant-", "vendor/one", "vendor/two"} {
		if strings.Contains(metrics, raw) || strings.Contains(billingResponse.Body.String(), raw) {
			t.Fatalf("raw passthrough ID %q reached metrics or billing", raw)
		}
	}
	if got := metricValue(t, metrics, `aiproxy_http_stream_responses_total{method="POST",path="/v1/chat/completions",status="200"}`); got != 1 {
		t.Errorf("completed SSE count = %g, want 1", got)
	}
	if got := metricValue(t, metrics, `aiproxy_upstream_requests_total{operation="chat_completions",outcome="success",provider="router"}`); got != variants+4 {
		t.Errorf("upstream success count = %g, want %d", got, variants+4)
	}
	recent := usage.Recent(1)
	if len(recent) != 1 || recent[0].PublicModel != streamModel || recent[0].Model != passthroughFamilyModel ||
		recent[0].UpstreamModel != passthroughFamilyTarget || recent[0].TotalTokens != 20 {
		t.Fatalf("recent event lost requested or grouped stream identity: %+v", recent)
	}
	entries := payloads.snapshot()
	if len(entries) != variants+4 {
		t.Fatalf("payload entries = %d, want %d", len(entries), variants+4)
	}
	entry := entries[len(entries)-1]
	if entry.RequestID != recent[0].RequestID || entry.PublicModel != streamModel || entry.UpstreamModel != passthroughFamilyTarget ||
		extractModel([]byte(entry.UpstreamRequest.Body.AsString())) != "org/variant-stream" ||
		!entry.Streaming || !entry.Response.Body.Truncated || len(entry.Response.Body.AsString()) > payloadMax {
		t.Errorf("payload did not distinguish the wire model from the group or respect capture bounds: %+v", entry)
	}
	if !strings.Contains(logs.String(), "public_model="+streamModel) || !strings.Contains(logs.String(), "upstream_model=org/variant-stream") {
		t.Error("access log did not retain the admitted request and wire model")
	}
}

func TestModelPassthroughDenialsKeepBoundedTelemetry(t *testing.T) {
	upstream, calls := passthroughUsageUpstream(t)
	rt := loadPassthroughRuntime(t, passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughFamily+`
  model_passthrough "overlap" {
    match = "^(@passthrough/.+|org/overlap-.+)$"
    capabilities = ["chat"]
  }
`), `
auth "fixture" {
  mode = "bearer_static"
  client "granted" {
    token = "granted"
    allowed_model_passthroughs = ["router/model-family", "router/overlap"]
  }
  client "restricted" {
    token = "restricted"
    allowed_models = ["router/literal"]
  }
}
`)
	deps := passthroughDeps(rt, upstream)
	usage := enablePassthroughUsage(&deps)
	h := NewHandler(deps)
	deny := func(token, model string, status int, code string) {
		t.Helper()
		w := passthroughChat(h, token, model)
		if w.Code != status || guardrailErrorType(t, w.Body.String()) != code {
			t.Fatalf("denial for %q: %d %s, want %d %s", model, w.Code, w.Body, status, code)
		}
	}
	const variants = 8
	for i := range variants {
		deny("granted", fmt.Sprintf("router/org/overlap-%d", i), http.StatusNotFound, "model_not_found")
		deny("granted", fmt.Sprintf("router/unmatched-%d", i), http.StatusNotFound, "model_not_found")
		deny("restricted", fmt.Sprintf("router/org/ungranted-%d", i), http.StatusForbidden, "forbidden")
	}
	deny("granted", "", http.StatusBadRequest, "invalid_model")
	deny("granted", passthroughFamilyModel, http.StatusNotFound, "model_not_found")
	deny("granted", "router/org/"+strings.Repeat("a", accounting.RecentModelBytes), http.StatusNotFound, "model_not_found")
	if rows := usage.Recent(1); len(rows) != 1 || !rows[0].Truncated.PublicModel || len(rows[0].PublicModel) > accounting.RecentModelBytes {
		t.Fatal("oversized rejected model escaped the recent diagnostic bound")
	}

	summaries, targets := usage.BillingSummaries()
	metrics := collectMetrics(t, h)
	if calls.Load() != 0 || len(targets) != 0 || len(usage.UpstreamSummaries()) != 0 || len(summaries) != 3 ||
		strings.Contains(metrics, "aiproxy_provider_selections_total") ||
		strings.Count(metrics, "\naiproxy_usage_events_total{") != 3 || strings.Count(metrics, "\naiproxy_http_errors_total{") != 3 {
		t.Fatalf("denials produced targets or unbounded rows/series: calls=%d usage=%v retained targets=%v", calls.Load(), summaries, targets)
	}
	for _, want := range []struct {
		client, model, code string
		status              int
		count               int64
	}{
		{"granted", accountingModelNotFound, "model_not_found", http.StatusNotFound, variants*2 + 2},
		{"restricted", accountingModelForbidden, "forbidden", http.StatusForbidden, variants},
		{"granted", accountingModelInvalidBody, "invalid_model", http.StatusBadRequest, 1},
	} {
		summary := accounting.Summary{CredentialID: identity.StaticClientID(want.client), WorkspaceScope: identity.InWorkspace(store.SystemWorkspaceID),
			Client: want.client, Model: want.model, Operation: "chat_completions", StatusCode: want.status, Count: want.count}
		if !slices.Contains(summaries, summary) {
			t.Errorf("missing denied usage group %+v: %v", summary, summaries)
		}
		eventSeries := fmt.Sprintf(`aiproxy_usage_events_total{client=%q,model=%q,operation="chat_completions",status="%d",tenant="anonymous"}`, want.client, want.model, want.status)
		errorSeries := fmt.Sprintf(`aiproxy_http_errors_total{error_type=%q,method="POST",path=%q,status="%d"}`, want.code, passthroughChatPath, want.status)
		if got := metricValue(t, metrics, eventSeries); got != float64(want.count) {
			t.Errorf("usage events for %q = %g, want %d", want.model, got, want.count)
		}
		if got := metricValue(t, metrics, errorSeries); got != float64(want.count) {
			t.Errorf("HTTP errors for %q = %g, want %d", want.code, got, want.count)
		}
	}
}

func TestModelPassthroughBillingScopesAndDatabaseQuota(t *testing.T) {
	upstream, calls := passthroughUsageUpstream(t)
	rt := loadPassthroughRuntime(t,
		passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughPricedPin+passthroughFamily),
		passthroughProvider("openai-compatible", "foreign", upstream.URL+"/v1", passthroughLiteral), `
auth "fixture" {
  mode = "bearer_static"
  client "shared" {
    token = "static-token"
    tenant = "shared-tenant"
    allowed_model_passthroughs = ["router/model-family"]
  }
  client "teammate" {
    token = "teammate-token"
    tenant = "shared-tenant"
    allowed_model_passthroughs = ["router/model-family"]
  }
  client "outsider" {
    token = "outsider-token"
    tenant = "different-tenant"
    allowed_model_passthroughs = ["router/model-family"]
  }
}
`)
	const pin = "router/org/pin"
	const denied = "router/org/db-denied"
	foreignID := uuid.New()
	system, foreign := identity.InWorkspace(store.SystemWorkspaceID), identity.InWorkspace(foreignID)
	p, _ := rt.Catalog.Provider("router")
	q, _ := rt.Catalog.Provider("foreign")
	p.Workspace, q.Workspace, q.DatabaseManaged = system, foreign, true
	rt.Catalog = config.NewCatalog([]config.Provider{p, q}, nil, nil)
	key := auth.DynamicClient{TokenHash: store.TokenHash("system-key"), KeyID: uuid.New(), Name: "shared", Tenant: "shared-tenant",
		Workspace: system, OwnerUserID: uuid.New(), AllowedModels: []string{pin, denied}}
	foreignKey := auth.DynamicClient{TokenHash: store.TokenHash("foreign-key"), KeyID: uuid.New(), Name: key.Name, Tenant: key.Tenant, Workspace: foreign}
	quota := &deferredQuotaStore{model: store.ScopeQuota{TPMEffective: 10}}
	deps := passthroughDeps(rt, upstream, key, foreignKey)
	enablePassthroughUsage(&deps)
	deps.MultiTenancy.Enabled, deps.Quota = true, NewQuotaTrackerWithStore(quota)
	deps.ServingRevisions = store.ServingRevisions{Catalog: 1, Keys: 1}
	deps.KeysConfirmedAt, deps.CatalogConfirmedAt = time.Now(), time.Now()
	h := NewHandler(deps)

	staticTraffic := []struct{ token, model string }{
		{"static-token", "router/org/shared-request"}, {"static-token", pin}, {"static-token", pin},
		{"teammate-token", "router/org/teammate-request"}, {"outsider-token", "router/org/outsider-request"},
	}
	for _, request := range staticTraffic {
		if w := passthroughChat(h, request.token, request.model); w.Code != http.StatusOK {
			t.Fatalf("static request %q: %d %s", request.model, w.Code, w.Body)
		}
	}
	if w := passthroughChat(h, "system-key", denied); w.Code != http.StatusNotFound ||
		guardrailErrorType(t, w.Body.String()) != "model_not_found" || calls.Load() != int32(len(staticTraffic)) ||
		quota.reads.Load() != 0 || quota.inserts.Load() != 0 {
		t.Fatalf("static usage or DB-key rule denial reached quota/ledger, or rule admitted to DB key: %d %s; calls=%d reads=%d inserts=%d",
			w.Code, w.Body, calls.Load(), quota.reads.Load(), quota.inserts.Load())
	}
	if w := passthroughChat(h, "system-key", pin); w.Code != http.StatusOK {
		t.Fatalf("DB key literal: %d %s", w.Code, w.Body)
	}
	if w := passthroughChat(h, "system-key", pin); w.Code != http.StatusTooManyRequests ||
		guardrailErrorType(t, w.Body.String()) != "tpm_exceeded" || w.Header().Get("Retry-After") == "" ||
		calls.Load() != int32(len(staticTraffic))+1 || quota.reads.Load() != 4 || quota.inserts.Load() != 1 {
		t.Fatalf("DB literal quota path changed: %d %s; calls=%d reads=%d inserts=%d", w.Code, w.Body, calls.Load(), quota.reads.Load(), quota.inserts.Load())
	}
	if entry := quota.entries[0]; entry.KeyID != key.KeyID || entry.WorkspaceID != store.SystemWorkspaceID ||
		entry.UserID == nil || *entry.UserID != key.OwnerUserID || entry.Model != pin || entry.Tokens != 10 || entry.CostMicros != 26 {
		t.Fatalf("DB spend lost credential, literal identity, or price: %+v", entry)
	}
	if w := passthroughChat(h, "foreign-key", "foreign/org/pin"); w.Code != http.StatusOK {
		t.Fatalf("foreign workspace literal: %d %s", w.Code, w.Body)
	}

	type row struct {
		client, model string
		status        int
		count, tokens int64
	}
	for _, tc := range []struct {
		token string
		rows  []row
	}{
		{"static-token", []row{
			{"shared", passthroughFamilyModel, 200, 1, 10},
			{"shared", pin, 200, 2, 20},
			{"teammate", passthroughFamilyModel, 200, 1, 10},
		}},
		{"system-key", []row{
			{"shared", accountingModelNotFound, 404, 1, 0},
			{"shared", pin, 200, 1, 10},
			{"shared", pin, 429, 1, 0},
		}},
		{"foreign-key", []row{{"shared", "foreign/org/pin", 200, 1, 10}}},
	} {
		t.Run(tc.token, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/billing/usage?scope=workspace&workspace_id="+foreignID.String(), nil)
			r.Header.Set("Authorization", "Bearer "+tc.token)
			r.Header.Set("X-Workspace-Id", store.SystemWorkspaceID.String())
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			rows := decodePassthroughBilling(t, w)
			if len(rows) != len(tc.rows) {
				t.Fatalf("unexpected scope: %s", w.Body)
			}
			for _, expected := range tc.rows {
				if !slices.ContainsFunc(rows, func(got billingUsageEntry) bool {
					return got.Client == expected.client && got.Model == expected.model && got.StatusCode == expected.status &&
						got.TotalTokens == expected.tokens && got.Tenant == "shared-tenant" && got.Count == expected.count
				}) {
					t.Errorf("scope lost or merged %v: %s", expected, w.Body)
				}
			}
		})
	}
}

func TestModelPassthroughRateLimitUsesCredentialNotModel(t *testing.T) {
	upstream, calls := passthroughUsageUpstream(t)
	rt := loadPassthroughRuntime(t, passthroughProvider("openai-compatible", "router", upstream.URL+"/v1", passthroughLiteral+passthroughFamily), `
auth "fixture" {
  mode = "bearer_static"
  rate_limit {
    requests_per_minute = 1
    burst = 1
  }
  client "first" {
    token = "first-token"
    allowed_model_passthroughs = ["router/model-family"]
  }
  client "second" {
    token = "second-token"
    allowed_model_passthroughs = ["router/model-family"]
  }
}
`)
	deps := passthroughDeps(rt, upstream)
	deps.RateLimiter = ratelimit.New(rt.Auth)
	h := NewHandler(deps)
	for _, tc := range []struct {
		token, model string
		status       int
	}{
		{"first-token", "router/org/rate-a", http.StatusOK},
		{"first-token", "router/org/rate-b", http.StatusTooManyRequests},
		{"first-token", "router/org/pin", http.StatusTooManyRequests},
		{"second-token", "router/org/rate-b", http.StatusOK},
	} {
		w := passthroughChat(h, tc.token, tc.model)
		if w.Code != tc.status || (tc.status == http.StatusTooManyRequests &&
			(guardrailErrorType(t, w.Body.String()) != "rate_limited" || w.Header().Get("Retry-After") == "")) {
			t.Fatalf("%s %q: %d %s, want %d", tc.token, tc.model, w.Code, w.Body, tc.status)
		}
	}
	if calls.Load() != 2 {
		t.Fatalf("rate-limited requests reached upstream: calls=%d, want 2", calls.Load())
	}
}
