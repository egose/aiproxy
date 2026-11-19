package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

type billingFixtureTotal struct {
	count int64
	cost  float64
}

type billingFixtureCredential struct {
	token     string
	name      string
	tenant    string
	id        identity.CredentialID
	workspace identity.WorkspaceScope
	rows      map[[3]string]billingFixtureTotal
}

func TestAppBillingImmutableKeyWorkspaceAndCostAttribution(t *testing.T) {
	st, dsn := workspaceInferenceDB(t)
	ctx := context.Background()
	workspaces := []store.Workspace{{Name: "billing-a"}, {Name: "billing-b"}}
	for i := range workspaces {
		if err := st.CreateWorkspace(ctx, &workspaces[i]); err != nil {
			t.Fatal(err)
		}
	}
	admin := store.User{Email: "billing-admin@example.invalid", IsAdmin: true}
	if err := st.CreateUser(ctx, &admin); err != nil {
		t.Fatal(err)
	}
	access, _, err := adminauth.IssueAccess(admin.ID.String(), admin.Email, true)
	if err != nil {
		t.Fatal(err)
	}
	gate, started := make(chan struct{}), make(chan struct{}, 1)
	release := sync.OnceFunc(func() { close(gate) })
	defer release()
	var calls atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var input struct {
			Model    string                     `json:"model"`
			Stream   bool                       `json:"stream"`
			Messages []struct{ Content string } `json:"messages"`
		}
		if r.URL.Path != "/v1/chat/completions" || json.NewDecoder(r.Body).Decode(&input) != nil {
			t.Error("unexpected upstream request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		wantSecret := map[string]string{"m": "system-secret", "a-cheap": "a-secret", "a-expensive": "a-secret", "b-model": "b-secret"}[input.Model]
		if wantSecret == "" || r.Header.Get("Authorization") != "Bearer "+wantSecret {
			t.Error("upstream model or credential was mixed")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if len(input.Messages) > 0 && input.Messages[0].Content == "hold-deferred" {
			started <- struct{}{}
			select {
			case <-gate:
			case <-r.Context().Done():
				return
			}
		}
		body := fmt.Sprintf(`{"model":%q,"choices":[],"usage":{"prompt_tokens":1000000,"completion_tokens":1000000,"total_tokens":2000000}}`, input.Model)
		if input.Stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", body)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}
	}))
	t.Cleanup(upstream.Close)
	for i, providerName := range []string{"billing-a", "billing-b"} {
		secret, err := store.EncryptSecret([]byte([]string{"a-secret", "b-secret"}[i]))
		if err != nil {
			t.Fatal(err)
		}
		p := store.DBProvider{Name: providerName, Type: "openai-compatible", Enabled: true, WorkspaceID: workspaces[i].ID,
			BaseURL: "https://" + providerName + ".example.com/v1", APIKeyEncrypted: secret}
		models := []store.DBProviderModel{{Name: "m", UpstreamName: "b-model", Pricing: json.RawMessage(`{"input_per_million":100,"output_per_million":200}`)}}
		if i == 0 {
			models = []store.DBProviderModel{
				{Name: "cheap", UpstreamName: "a-cheap", Pricing: json.RawMessage(`{"input_per_million":1,"output_per_million":2}`)},
				{Name: "expensive", UpstreamName: "a-expensive", Pricing: json.RawMessage(`{"input_per_million":10,"output_per_million":20}`)},
			}
		}
		if err := st.CreateProviderAggregate(ctx, &p, models); err != nil {
			t.Fatal(err)
		}
		alias := store.DBAlias{Name: providerName, WorkspaceID: workspaces[i].ID, Algorithm: "round_robin"}
		if err := st.CreateAlias(ctx, &alias, []store.DBAliasTarget{{Provider: providerName, Model: models[0].Name}}); err != nil {
			t.Fatal(err)
		}
	}
	cfg := deferredLifecycleConfig(upstream.URL+"/v1", "static-shared", "system-secret", "", true, 4)
	cfg = strings.Replace(cfg, `client "peer"`, `client "shared"`, 1)
	cfg = strings.Replace(cfg, `    allowed_models = ["local/m"]`, `    tenant = "shared-label"`, 1)
	cfg = strings.Replace(cfg, `  model "m" {}`, `  model "m" {
    pricing {
      input_per_million = 1
      output_per_million = 2
    }
  }`, 1)
	cfg = strings.Replace(cfg, `control_timeout = "100ms"`, `control_timeout = "2s"`, 1)
	cfg = strings.Replace(cfg, `finalization_timeout = "100ms"`, `finalization_timeout = "2s"`, 1)
	cfg = strings.Replace(cfg, `execution_timeout = "10s"`, `execution_timeout = "30s"`, 1)
	cfg = strings.Replace(cfg, "\n}\nprovider ", `
  client "trusted-peer" {
    token = "static-peer"
    tenant = "shared-label"
  }
  client "tenantless" { token = "static-tenantless" }
}
provider `, 1)
	cfg += fmt.Sprintf(`
alias "system-route" {
  algorithm = "round_robin"
  providers = ["local"]
  model = "m"
}
database { url = %q }
multi_tenancy { allowed_upstream_origins = ["https://billing-a.example.com", "https://billing-b.example.com"] }
`, dsn)
	a, err := Build(ctx, BuildOptions{ConfigPath: writeConfigFile(t, cfg), LogOutput: io.Discard,
		transportFactory: tenantMockTransportFactory(map[string]string{"billing-a.example.com": upstream.URL, "billing-b.example.com": upstream.URL})})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = a.Close() })
	proxy := httptest.NewServer(a.handler)
	t.Cleanup(proxy.Close)
	call := func(method, path, token, body string) appDeferredReply {
		t.Helper()
		return workspaceInferenceRequest(t, proxy, method, path, token, body)
	}
	var credentials []*billingFixtureCredential
	static := func(name, token, tenant string) *billingFixtureCredential {
		c := &billingFixtureCredential{token: token, name: name, tenant: tenant, id: identity.StaticClientID(name),
			workspace: identity.InWorkspace(store.SystemWorkspaceID), rows: map[[3]string]billingFixtureTotal{}}
		credentials = append(credentials, c)
		return c
	}
	file := static("shared", "static-shared", "shared-label")
	peer := static("trusted-peer", "static-peer", "shared-label")
	tenantless := static("tenantless", "static-tenantless", "")
	issue := func(name, tenant string, workspace uuid.UUID) *billingFixtureCredential {
		t.Helper()
		resp := call(http.MethodPost, "/_internal/admin/keys", access,
			fmt.Sprintf(`{"name":%q,"tenant":%q,"workspace_id":%q}`, name, tenant, workspace.String()))
		var issued struct {
			Token string
			Key   struct{ ID uuid.UUID }
		}
		if resp.status != http.StatusCreated || json.Unmarshal(resp.body, &issued) != nil || issued.Token == "" || issued.Key.ID == uuid.Nil {
			t.Fatalf("issue billing key: status %d", resp.status)
		}
		c := &billingFixtureCredential{token: issued.Token, name: name, tenant: tenant, id: identity.DatabaseKeyID(issued.Key.ID),
			workspace: identity.InWorkspace(workspace), rows: map[[3]string]billingFixtureTotal{}}
		credentials = append(credentials, c)
		return c
	}
	system := issue("system-key", "shared-label", store.SystemWorkspaceID)
	first := issue("shared", "shared-label", workspaces[0].ID)
	firstPeer := issue("a-peer", "shared-label", workspaces[0].ID)
	second := issue("b-key", "shared-label", workspaces[1].ID)
	add := func(c *billingFixtureCredential, model string, price float64) {
		t.Helper()
		key := [3]string{c.tenant, c.name, model}
		row := c.rows[key]
		row.count++
		row.cost += price
		c.rows[key] = row
		var want int64
		for _, row := range c.rows {
			want += row.count
		}
		awaitResourceCondition(t, "identity-scoped HTTP accounting", func() bool {
			var count int64
			for _, row := range accounting.FilterSummaries(a.usage.Summaries(), accounting.CredentialUsage(c.id, c.workspace)) {
				count += row.Count
			}
			return count == want
		})
	}
	infer := func(c *billingFixtureCredential, model, upstreamModel string, price float64, stream bool) {
		t.Helper()
		resp := call(http.MethodPost, "/v1/chat/completions", c.token, fmt.Sprintf(`{"model":%q,"messages":[],"stream":%t}`, model, stream))
		if resp.status != http.StatusOK || !strings.Contains(string(resp.body), fmt.Sprintf(`"model":%q`, upstreamModel)) ||
			stream && !strings.Contains(string(resp.body), "[DONE]") {
			t.Fatalf("billing inference %s: %d %s", model, resp.status, resp.body)
		}
		add(c, model, price)
	}
	check := func(c *billingFixtureCredential) {
		t.Helper()
		resp := call(http.MethodGet, "/v1/billing/usage?scope=workspace&workspace_id="+workspaces[1].ID.String()+"&tenant=shared-label&client=b-key", c.token, "")
		var body struct {
			Object string
			Data   []struct {
				accounting.Summary
				Cost *float64 `json:"estimated_cost_usd"`
			}
		}
		if resp.status != http.StatusOK || json.Unmarshal(resp.body, &body) != nil || body.Object != "list" || body.Data == nil {
			t.Fatalf("billing response: %d %s", resp.status, resp.body)
		}
		want := c.rows
		if _, isStatic := c.id.StaticClientName(); isStatic && c.tenant != "" {
			want = map[[3]string]billingFixtureTotal{}
			for _, other := range credentials {
				if _, isStatic := other.id.StaticClientName(); isStatic {
					for key, row := range other.rows {
						if key[0] == c.tenant {
							want[key] = row
						}
					}
				}
			}
		}
		if len(body.Data) != len(want) {
			t.Fatalf("%s billing rows = %+v, want %v", c.name, body.Data, want)
		}
		seen := map[[3]string]bool{}
		for _, row := range body.Data {
			key := [3]string{row.Tenant, row.Client, row.Model}
			expected, ok := want[key]
			if !ok || seen[key] || row.StatusCode != http.StatusOK || row.Operation != "chat_completions" || row.Count != expected.count ||
				row.PromptTokens != expected.count*1000000 || row.CompletionTokens != expected.count*1000000 || row.TotalTokens != expected.count*2000000 ||
				row.Cost == nil || math.Abs(*row.Cost-expected.cost) > 1e-9 || row.CredentialID.Valid() || row.WorkspaceScope != (identity.WorkspaceScope{}) {
				t.Fatalf("%s received mixed totals/cost/labels: %+v, want %v", c.name, row, expected)
			}
			seen[key] = true
		}
		for _, other := range credentials {
			if id, isKey := other.id.DatabaseKeyID(); isKey && strings.Contains(string(resp.body), id.String()) {
				t.Fatal("billing JSON disclosed a credential ID")
			}
		}
		if strings.Contains(string(resp.body), "CredentialID") || strings.Contains(string(resp.body), "WorkspaceScope") {
			t.Fatal("billing JSON disclosed internal attribution fields")
		}
	}
	checkAll := func() {
		t.Helper()
		for _, c := range credentials {
			check(c)
		}
	}
	checkAll()
	infer(file, "local/m", "m", 3, false)
	infer(peer, "alias/system-route", "m", 3, false)
	infer(tenantless, "local/m", "m", 3, false)
	infer(system, "local/m", "m", 3, false)
	infer(system, "alias/system-route", "m", 3, false)
	infer(first, "billing-a/cheap", "a-cheap", 3, false)
	infer(first, "alias/billing-a", "a-cheap", 3, false)
	infer(firstPeer, "billing-a/cheap", "a-cheap", 3, false)
	infer(second, "billing-b/m", "b-model", 300, false)
	infer(second, "alias/billing-b", "b-model", 300, true)
	alias, err := st.GetAlias(ctx, "billing-a")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAlias(ctx, &alias, []store.DBAliasTarget{{Provider: "billing-a", Model: "expensive"}}); err != nil {
		t.Fatal(err)
	}
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	infer(first, "alias/billing-a", "a-expensive", 30, true)
	infer(firstPeer, "alias/billing-a", "a-expensive", 30, false)
	checkAll()

	keyPath := func(c *billingFixtureCredential) string {
		id, _ := c.id.DatabaseKeyID()
		return "/_internal/admin/keys/" + id.String()
	}
	editTenant := func(c *billingFixtureCredential, tenant string) {
		t.Helper()
		if resp := call(http.MethodPut, keyPath(c), access, fmt.Sprintf(`{"tenant":%q}`, tenant)); resp.status != http.StatusOK {
			t.Fatalf("edit billing label: status %d", resp.status)
		}
		c.tenant = tenant
	}
	rotate := func(c *billingFixtureCredential) {
		t.Helper()
		old := c.token
		resp := call(http.MethodPost, keyPath(c)+"/rotate", access, "")
		var body struct{ Token string }
		if resp.status != http.StatusOK || json.Unmarshal(resp.body, &body) != nil || body.Token == "" || body.Token == old {
			t.Fatalf("rotate billing key: status %d", resp.status)
		}
		c.token = body.Token
		if resp := call(http.MethodGet, "/v1/billing/usage", old, ""); resp.status != http.StatusUnauthorized {
			t.Fatal("old rotated bearer still read usage")
		}
		check(c)
	}
	editTenant(first, "")
	id, _ := first.id.DatabaseKeyID()
	if _, err := st.DB.NewUpdate().Model((*store.InboundKey)(nil)).Set("name = ?", "tenantless").Where("id = ?", id).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	first.name = "tenantless"
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	checkAll()
	rotate(first)
	infer(first, "alias/billing-a", "a-expensive", 30, false)
	checkAll()

	capability := appDeferredCapability(t, proxy, system.token)
	request, _ := appDeferredSubmit(t, capability)
	request.Body = []byte(`{"model":"local/m","messages":[{"role":"user","content":"hold-deferred"}]}`)
	wire, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	reply := callAppDeferred(t, proxy, http.MethodPost, deferred.Prefix+"/submit", system.token, "", wire)
	var receipt deferred.Receipt
	if reply.status != http.StatusAccepted || deferred.DecodeControl(reply.body, &receipt) != nil {
		t.Fatalf("deferred billing submission: %d %s", reply.status, reply.body)
	}
	select {
	case <-started:
	case <-time.After(4 * time.Second):
		t.Fatal("deferred billing execution did not start")
	}
	oldTenant := system.tenant
	editTenant(system, "")
	rotate(system)
	release()
	awaitResourceCondition(t, "deferred immutable billing finalization", func() bool {
		return a.DeferredStore().Stats().Running == 0 && a.DeferredStore().Stats().FinalizationCredits == 0
	})
	system.tenant = oldTenant
	add(system, "local/m", 3)
	system.tenant = ""
	replayed := callAppDeferred(t, proxy, http.MethodPost, deferred.Prefix+"/submit", system.token, "", wire)
	var replay deferred.Receipt
	if replayed.status != http.StatusAccepted || deferred.DecodeControl(replayed.body, &replay) != nil || !replay.Replayed || replay.ID != receipt.ID {
		t.Fatal("rotated deferred replay did not retain the admitted identity")
	}
	checkAll()
	for _, old := range []*billingFixtureCredential{first, system} {
		if resp := call(http.MethodDelete, keyPath(old), access, ""); resp.status != http.StatusOK {
			t.Fatalf("delete billing key: status %d", resp.status)
		}
		workspace, _ := old.workspace.ID()
		next := issue(old.name, old.tenant, workspace)
		if next.id == old.id {
			t.Fatal("replacement billing key reused immutable identity")
		}
		check(next)
		if resp := call(http.MethodGet, "/v1/billing/usage", old.token, ""); resp.status != http.StatusUnauthorized {
			t.Fatal("deleted bearer still read usage")
		}
		if old == first {
			infer(next, "alias/billing-a", "a-expensive", 30, false)
		} else {
			infer(next, "local/m", "m", 3, false)
		}
		check(next)
		ledger, err := st.KeyUsageByWorkspace(ctx, workspace)
		if err != nil {
			t.Fatal(err)
		}
		var expected billingFixtureTotal
		for _, row := range old.rows {
			expected.count += row.count
			expected.cost += row.cost
		}
		oldID, _ := old.id.DatabaseKeyID()
		found := false
		for _, usage := range ledger {
			if usage.KeyID == oldID {
				found = usage.Requests == expected.count && usage.Tokens == expected.count*2000000 &&
					usage.Spend == int64(expected.cost*1000000)
			}
		}
		if !found {
			t.Fatalf("durable immutable key attribution changed: %+v", ledger)
		}
	}
	check(file)
	check(peer)
	check(tenantless)
	check(firstPeer)
	check(second)
	var lifetime int64
	for _, summary := range a.usage.ProviderSummaries() {
		lifetime += summary.Requests
	}
	if lifetime != calls.Load() {
		t.Fatalf("operator lifetime aggregates dropped traffic: %d vs %d", lifetime, calls.Load())
	}
}
