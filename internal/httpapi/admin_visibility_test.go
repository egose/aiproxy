package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/store"
)

func listNames(body map[string]interface{}, key string) map[string]bool {
	out := map[string]bool{}
	items, _ := body[key].([]interface{})
	for _, item := range items {
		m, _ := item.(map[string]interface{})
		name, _ := m["name"].(string)
		out[name] = true
	}
	return out
}

func TestConfigResourcesHiddenFromNonSystemUsers(t *testing.T) {
	st, _, ownerAccess := openUsersTestStore(t)
	ctx := context.Background()
	if err := st.EnsureSystemOrg(ctx); err != nil {
		t.Fatal(err)
	}

	suffix := time.Now().UnixNano()
	staticProv := fmt.Sprintf("staticprov-%d", suffix)
	staticAlias := fmt.Sprintf("staticalias-%d", suffix)

	rt := &config.Runtime{MultiTenancy: config.MultiTenancy{Enabled: true}}
	rt.Catalog = config.NewCatalog(
		[]config.Provider{
			{Type: config.ProviderTypeOpenAI, Name: staticProv, APIKey: "sk-static", BaseURL: "https://x", Models: []config.Model{{Name: "m1", UpstreamName: "m1"}}},
		},
		nil,
		[]config.Alias{
			{Name: staticAlias, Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: staticProv, Model: "m1"}}},
		},
	)
	rt.Auth = config.Auth{Clients: map[string]config.Client{"static-client": {Name: "static-client", Tenant: "static-tenant"}}}

	usage := accounting.NewAggregator()
	health := providerhealth.New(nil, config.ProviderHealth{})
	health.SetProviders(rt.Catalog)
	logs := observability.NewLogBuffer(10)
	deps := newDashboardDeps(rt, time.Now(), usage, health, logs)
	deps.MultiTenancy = rt.MultiTenancy
	deps.AdminStore = st
	deps.AdminAuthConfig = rt.Auth
	h := NewHandler(deps)

	memberOrg := mkOrg(t, st, h, ownerAccess, "memberorg")
	_, memberAccess := mkOrgMember(t, st, h, ownerAccess, fmt.Sprintf("plain-%d@example.com", suffix), memberOrg, "member")

	code, body := callAdmin(t, h, ownerAccess, http.MethodGet, "/_internal/admin/providers", nil)
	if code != http.StatusOK || !listNames(body, "providers")[staticProv] {
		t.Fatalf("global admin must see static provider: %d %v", code, body)
	}
	code, body = callAdmin(t, h, ownerAccess, http.MethodGet, "/_internal/admin/aliases", nil)
	if code != http.StatusOK || !listNames(body, "aliases")[staticAlias] {
		t.Fatalf("global admin must see static alias: %d %v", code, body)
	}
	code, body = callAdmin(t, h, ownerAccess, http.MethodGet, "/_internal/admin/keys", nil)
	if code != http.StatusOK || !listNames(body, "keys")["static-client"] {
		t.Fatalf("global admin must see static client: %d %v", code, body)
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers", nil)
	if code != http.StatusOK || listNames(body, "providers")[staticProv] {
		t.Fatalf("tenant member must not see static provider: %d %v", code, body)
	}
	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/aliases", nil)
	if code != http.StatusOK || listNames(body, "aliases")[staticAlias] {
		t.Fatalf("tenant member must not see static alias: %d %v", code, body)
	}
	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/keys", nil)
	if code != http.StatusOK || listNames(body, "keys")["static-client"] {
		t.Fatalf("tenant member must not see static client: %d %v", code, body)
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers/"+staticProv, nil); code != http.StatusNotFound {
		t.Fatalf("tenant member detail for static provider = %d, want 404", code)
	}
	if code, _ := callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers?org_id="+store.SystemOrgID.String(), nil); code != http.StatusNotFound {
		t.Fatalf("tenant member filtering by system org = %d, want 404", code)
	}

	sysMember, sysAccess := mkOrgMember(t, st, h, ownerAccess, fmt.Sprintf("sys-%d@example.com", suffix), store.SystemOrgID.String(), "member")
	_ = sysMember
	code, body = callAdmin(t, h, sysAccess, http.MethodGet, "/_internal/admin/providers", nil)
	if code != http.StatusOK || !listNames(body, "providers")[staticProv] {
		t.Fatalf("system org member must see static provider: %d %v", code, body)
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers?org_id="+memberOrg, nil)
	if code != http.StatusOK || listNames(body, "providers")[staticProv] {
		t.Fatalf("tenant member filtering by own org must not see static provider: %d %v", code, body)
	}
}

func TestOrgContextHeader(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	orgA := mkOrg(t, st, h, access, "hdrorg")
	orgB := mkOrg(t, st, h, access, "hdrorg")

	headerA := map[string]string{"X-Org-ID": orgA}

	provA := fmt.Sprintf("hdrprov-a-%d", time.Now().UnixNano())
	code, body := callAdminHeaders(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": provA, "type": "openai", "api_key": "sk-test",
		"models": []interface{}{map[string]interface{}{"name": "m1"}},
	}, headerA)
	if code != http.StatusCreated {
		t.Fatalf("create with org header status = %d, body = %v", code, body)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(context.Background(), provA); err == nil {
			_ = st.DeleteProvider(context.Background(), p.ID)
		}
	})
	if got, _ := body["org_id"].(string); got != orgA {
		t.Fatalf("created provider org_id = %q, want %q", got, orgA)
	}

	code, body = callAdminHeaders(t, h, access, http.MethodGet, "/_internal/admin/providers", nil, headerA)
	if code != http.StatusOK || !listNames(body, "providers")[provA] {
		t.Fatalf("list with org header must contain created provider: %d %v", code, body)
	}

	headerB := map[string]string{"X-Org-ID": orgB}
	code, body = callAdminHeaders(t, h, access, http.MethodGet, "/_internal/admin/providers", nil, headerB)
	if code != http.StatusOK || listNames(body, "providers")[provA] {
		t.Fatalf("list with other org header must not contain provider: %d %v", code, body)
	}

	code, body = callAdminHeaders(t, h, access, http.MethodGet, "/_internal/admin/providers?org_id="+orgB, nil, headerA)
	if code != http.StatusOK || listNames(body, "providers")[provA] {
		t.Fatalf("explicit query org_id must win over header: %d %v", code, body)
	}

	aliasName := fmt.Sprintf("hdralias-%d", time.Now().UnixNano())
	code, body = callAdminHeaders(t, h, access, http.MethodPost, "/_internal/admin/aliases", map[string]interface{}{
		"name": aliasName, "targets": []interface{}{map[string]interface{}{"provider": provA, "model": "m1"}},
	}, headerA)
	if code != http.StatusCreated {
		t.Fatalf("create alias with org header status = %d, body = %v", code, body)
	}
	t.Cleanup(func() {
		if a, err := st.GetAlias(context.Background(), aliasName); err == nil {
			_ = st.DeleteAlias(context.Background(), a.ID)
		}
	})
	if got, _ := body["org_id"].(string); got != orgA {
		t.Fatalf("created alias org_id = %q, want %q", got, orgA)
	}

	if code, _ := callAdminHeaders(t, h, access, http.MethodGet, "/_internal/admin/providers", nil, map[string]string{"X-Org-ID": "not-a-uuid"}); code != http.StatusBadRequest {
		t.Fatalf("invalid org header status = %d, want 400", code)
	}
}
