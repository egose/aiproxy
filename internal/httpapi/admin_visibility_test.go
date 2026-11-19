package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/accounting"
	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/observability"
	"github.com/egose/aiproxy/internal/providerhealth"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
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
	if err := st.EnsureSystemWorkspace(ctx); err != nil {
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

	memberWorkspace := mkWorkspace(t, st, h, ownerAccess, "memberorg")
	_, memberAccess := mkWorkspaceMember(t, st, h, ownerAccess, fmt.Sprintf("plain-%d@example.com", suffix), memberWorkspace, "member")

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
	if code, _ := callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers?workspace_id="+store.SystemWorkspaceID.String(), nil); code != http.StatusNotFound {
		t.Fatalf("tenant member filtering by system workspace = %d, want 404", code)
	}

	sysMember, sysAccess := mkWorkspaceMember(t, st, h, ownerAccess, fmt.Sprintf("sys-%d@example.com", suffix), store.SystemWorkspaceID.String(), "member")
	_ = sysMember
	code, body = callAdmin(t, h, sysAccess, http.MethodGet, "/_internal/admin/providers", nil)
	if code != http.StatusOK || !listNames(body, "providers")[staticProv] {
		t.Fatalf("system workspace member must see static provider: %d %v", code, body)
	}

	code, body = callAdmin(t, h, memberAccess, http.MethodGet, "/_internal/admin/providers?workspace_id="+memberWorkspace, nil)
	if code != http.StatusOK || listNames(body, "providers")[staticProv] {
		t.Fatalf("tenant member filtering by own workspace must not see static provider: %d %v", code, body)
	}
}

func TestWorkspaceContextHeader(t *testing.T) {
	st, h, access := openUsersTestStore(t)
	workspaceA := mkWorkspace(t, st, h, access, "hdrorg")
	workspaceB := mkWorkspace(t, st, h, access, "hdrorg")

	headerA := map[string]string{"X-Workspace-ID": workspaceA}

	provA := fmt.Sprintf("hdrprov-a-%d", time.Now().UnixNano())
	code, body := callAdminHeaders(t, h, access, http.MethodPost, "/_internal/admin/providers", map[string]interface{}{
		"name": provA, "type": "openai", "api_key": "sk-test",
		"models": []interface{}{map[string]interface{}{"name": "m1"}},
	}, headerA)
	if code != http.StatusCreated {
		t.Fatalf("create with workspace header status = %d, body = %v", code, body)
	}
	t.Cleanup(func() {
		if p, err := st.GetProvider(context.Background(), provA); err == nil {
			_ = st.DeleteProvider(context.Background(), p.ID)
		}
	})
	if got, _ := body["workspace_id"].(string); got != workspaceA {
		t.Fatalf("created provider workspace_id = %q, want %q", got, workspaceA)
	}

	code, body = callAdminHeaders(t, h, access, http.MethodGet, "/_internal/admin/providers", nil, headerA)
	if code != http.StatusOK || !listNames(body, "providers")[provA] {
		t.Fatalf("list with workspace header must contain created provider: %d %v", code, body)
	}

	headerB := map[string]string{"X-Workspace-ID": workspaceB}
	code, body = callAdminHeaders(t, h, access, http.MethodGet, "/_internal/admin/providers", nil, headerB)
	if code != http.StatusOK || listNames(body, "providers")[provA] {
		t.Fatalf("list with other workspace header must not contain provider: %d %v", code, body)
	}

	code, body = callAdminHeaders(t, h, access, http.MethodGet, "/_internal/admin/providers?workspace_id="+workspaceB, nil, headerA)
	if code != http.StatusOK || listNames(body, "providers")[provA] {
		t.Fatalf("explicit query workspace_id must win over header: %d %v", code, body)
	}

	aliasName := fmt.Sprintf("hdralias-%d", time.Now().UnixNano())
	code, body = callAdminHeaders(t, h, access, http.MethodPost, "/_internal/admin/aliases", map[string]interface{}{
		"name": aliasName, "targets": []interface{}{map[string]interface{}{"provider": provA, "model": "m1"}},
	}, headerA)
	if code != http.StatusCreated {
		t.Fatalf("create alias with workspace header status = %d, body = %v", code, body)
	}
	t.Cleanup(func() {
		if a, err := st.GetAlias(context.Background(), aliasName); err == nil {
			_ = st.DeleteAlias(context.Background(), a.ID)
		}
	})
	if got, _ := body["workspace_id"].(string); got != workspaceA {
		t.Fatalf("created alias workspace_id = %q, want %q", got, workspaceA)
	}

	if code, _ := callAdminHeaders(t, h, access, http.MethodGet, "/_internal/admin/providers", nil, map[string]string{"X-Workspace-ID": "not-a-uuid"}); code != http.StatusBadRequest {
		t.Fatalf("invalid workspace header status = %d, want 400", code)
	}
}

func TestWorkspaceVisibilityScope(t *testing.T) {
	system := store.SystemWorkspaceID
	tenant := uuid.New()
	for _, tc := range []struct {
		name                string
		scope               workspaceVisibility
		wantSystem, wantOwn bool
	}{
		{name: "zero value denies all"},
		{name: "explicit empty set", scope: workspaceVisibility{roles: map[uuid.UUID]string{}}},
		{name: "global", scope: workspaceVisibility{global: true}, wantSystem: true, wantOwn: true},
		{name: "explicit System member", scope: workspaceVisibility{roles: map[uuid.UUID]string{system: roleMember}}, wantSystem: true},
		{name: "tenant only", scope: workspaceVisibility{roles: map[uuid.UUID]string{tenant: roleMember}}, wantOwn: true},
		{name: "global filtered to System", scope: workspaceVisibility{global: true, filter: &store.Workspace{ID: system}}, wantSystem: true},
		{name: "filter grants nothing", scope: workspaceVisibility{roles: map[uuid.UUID]string{tenant: roleAdmin}, filter: &store.Workspace{ID: system}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if system, own := tc.scope.allows(system), tc.scope.allows(tenant); system != tc.wantSystem || own != tc.wantOwn {
				t.Errorf("allows System=%v tenant=%v, want %v %v", system, own, tc.wantSystem, tc.wantOwn)
			}
		})
	}
}

type workspaceMembershipReadHook struct {
	fail  bool
	reads int
}

func (h *workspaceMembershipReadHook) BeforeQuery(ctx context.Context, event *bun.QueryEvent) context.Context {
	if strings.Contains(event.Query, `FROM "workspace_members"`) && strings.Contains(event.Query, `user_id =`) {
		h.reads++
		if h.fail {
			queryCtx, cancel := context.WithCancel(ctx)
			cancel()
			return queryCtx
		}
	}
	return ctx
}

func (*workspaceMembershipReadHook) AfterQuery(context.Context, *bun.QueryEvent) {}

func TestAdminWorkspaceVisibility(t *testing.T) {
	st := openAdminReasoningStore(t)
	ctx := context.Background()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	a, b := store.Workspace{Name: "workspace-a"}, store.Workspace{Name: "workspace-b"}
	must(st.CreateWorkspace(ctx, &a))
	must(st.CreateWorkspace(ctx, &b))
	user := func(label string, global bool, workspace *uuid.UUID, role string) (uuid.UUID, string) {
		t.Helper()
		u := store.User{Email: label + "@example.test", IsAdmin: global}
		must(st.CreateUser(ctx, &u))
		if workspace != nil {
			must(st.AddMembership(ctx, &store.WorkspaceMember{UserID: u.ID, WorkspaceID: *workspace, Role: role}))
		}
		access, _, err := adminauth.IssueAccess(u.ID.String(), u.Email, global)
		must(err)
		return u.ID, access
	}
	system := store.SystemWorkspaceID
	_, global := user("global", true, nil, "")
	_, systemMember := user("system-member", false, &system, roleMember)
	aMemberID, aMember := user("a-member", false, &a.ID, roleMember)
	_, aAdmin := user("a-admin", false, &a.ID, roleAdmin)
	_, noMemberships := user("no-memberships", false, nil, "")

	addKey := func(id uuid.UUID, name string) uuid.UUID {
		t.Helper()
		key := store.InboundKey{
			Name: name, WorkspaceID: id, TokenHash: store.TokenHash("fixture-" + name), TokenPrefix: "prefix-" + name,
			Tenant: "tenant-" + name, Enabled: true, AllowedModels: []string{"fixture-model"},
		}
		must(st.CreateInboundKey(ctx, &key))
		return key.ID
	}
	for label, id := range map[string]uuid.UUID{"a": a.ID, "b": b.ID} {
		p := store.DBProvider{
			Name: label + "-provider", WorkspaceID: id, Type: "openai", Enabled: true,
			APIKeyRefPath: "/fixture/" + label + ".json", APIKeyRefKey: label + "-credential-ref",
		}
		must(st.CreateProviderAggregate(ctx, &p, []store.DBProviderModel{{Name: "fixture-model"}}))
		alias := store.DBAlias{Name: label + "-alias", WorkspaceID: id, Algorithm: string(config.AlgorithmRoundRobin)}
		must(st.CreateAlias(ctx, &alias, []store.DBAliasTarget{{Provider: p.Name, Model: "fixture-model"}}))
	}
	aSharedKey := addKey(a.ID, "a-shared")
	must(st.SetKeyBindings(ctx, aSharedKey, []uuid.UUID{aMemberID}, nil))
	aPrivateKey := addKey(a.ID, "a-private")
	bKey := addKey(b.ID, "b-key")
	for label, id := range map[string]*uuid.UUID{"a": &a.ID, "b": &b.ID, "system": &system, "unscoped": nil} {
		inv := store.Invite{
			Email: label + "-invite@example.test", WorkspaceID: id, IsAdmin: id == nil, WorkspaceRole: roleMember,
			TokenHash: store.TokenHash("invite-fixture-" + label), ExpiresAt: time.Now().Add(time.Hour),
		}
		must(st.CreateInvite(ctx, &inv))
	}
	h := NewHandler(Dependencies{
		Catalog: config.NewCatalog(
			[]config.Provider{{Name: "system-provider", Type: config.ProviderTypeOpenAI, APIKey: "fixture-credential", Models: []config.Model{{Name: "fixture-model"}}}},
			nil,
			[]config.Alias{{Name: "system-alias", Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "system-provider", Model: "fixture-model"}}}},
		),
		AdminAuthConfig: config.Auth{Clients: map[string]config.Client{"system-client": {Name: "system-client"}}},
		AdminStore:      st,
		MultiTenancy:    config.MultiTenancy{Enabled: true},
		HasAccessLog:    true,
	})
	hook := &workspaceMembershipReadHook{}
	st.DB.AddQueryHook(hook)
	get := func(access, path, header string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, adminPathPrefix+path, nil)
		req.Header.Set("Authorization", "Bearer "+access)
		if header != "" {
			req.Header.Set("X-Workspace-ID", header)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		return w
	}
	assertList := func(t *testing.T, access, kind, filter, header string, want []string) {
		t.Helper()
		w := get(access, kind+filter, header)
		if w.Code != http.StatusOK {
			t.Fatalf("GET %s%s: status=%d body=%s", kind, filter, w.Code, w.Body.String())
		}
		var body map[string][]map[string]any
		if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil || body[kind] == nil {
			t.Fatalf("expected %s array, got %s (%v)", kind, w.Body.String(), err)
		}
		field := "name"
		if kind == "invites" {
			field = "email"
		}
		var got []string
		for _, entry := range body[kind] {
			value, ok := entry[field].(string)
			if !ok {
				t.Fatalf("missing %s in entry %v", field, entry)
			}
			got = append(got, value)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Errorf("%s entries=%v, want %v", kind, got, want)
		}
	}
	lists := []struct {
		kind              string
		global, system, a []string
	}{
		{"providers", []string{"a-provider", "b-provider", "system-provider"}, []string{"system-provider"}, []string{"a-provider"}},
		{"aliases", []string{"a-alias", "b-alias", "system-alias"}, []string{"system-alias"}, []string{"a-alias"}},
		{"keys", []string{"a-private", "a-shared", "b-key", "system-client"}, []string{"system-client"}, []string{"a-shared"}},
		{"workspaces", []string{"system", "workspace-a", "workspace-b"}, []string{"system"}, []string{"workspace-a"}},
		{"invites", []string{"a-invite@example.test", "b-invite@example.test", "system-invite@example.test", "unscoped-invite@example.test"}, []string{"system-invite@example.test"}, []string{"a-invite@example.test"}},
	}
	for _, list := range lists {
		t.Run(list.kind, func(t *testing.T) {
			assertList(t, global, list.kind, "", "", list.global)
			assertList(t, systemMember, list.kind, "", "", list.system)
			assertList(t, aMember, list.kind, "", "", list.a)
			assertList(t, noMemberships, list.kind, "", "", nil)
		})
	}
	for _, list := range lists[:3] {
		t.Run(list.kind+"/filter", func(t *testing.T) {
			assertList(t, aMember, list.kind, "?workspace_id="+a.ID.String(), system.String(), list.a)
			assertList(t, aMember, list.kind, "", a.ID.String(), list.a)
			assertList(t, systemMember, list.kind, "?workspace_id="+system.String(), "", list.system)
			for _, w := range []*httptest.ResponseRecorder{
				get(aMember, list.kind, system.String()),
				get(aMember, list.kind+"?workspace_id="+b.ID.String(), a.ID.String()),
				get(noMemberships, list.kind+"?workspace_id="+a.ID.String(), ""),
			} {
				if w.Code != http.StatusNotFound || w.Body.String() != "workspace not found\n" {
					t.Errorf("inaccessible workspace: status=%d body=%s", w.Code, w.Body.String())
				}
			}
		})
	}
	for _, tc := range []struct {
		access, path, header string
		code                 int
	}{
		{systemMember, "providers/system-provider", system.String(), http.StatusOK},
		{systemMember, "aliases/system-alias", system.String(), http.StatusOK},
		{aMember, "providers/a-provider", a.ID.String(), http.StatusOK},
		{aMember, "aliases/a-alias", a.ID.String(), http.StatusOK},
		{aMember, "keys/" + aSharedKey.String(), a.ID.String(), http.StatusOK},
		{aMember, "workspaces/" + a.ID.String(), "", http.StatusOK},
		{aMember, "workspaces/" + b.ID.String(), "", http.StatusNotFound},
		{noMemberships, "workspaces/" + a.ID.String(), "", http.StatusNotFound},
		{aMember, "providers/b-provider", "", http.StatusNotFound},
		{aMember, "aliases/system-alias", "", http.StatusNotFound},
		{aMember, "keys/" + bKey.String(), "", http.StatusNotFound},
		{aMember, "keys/" + aPrivateKey.String(), "", http.StatusNotFound},
		{aAdmin, "keys/" + aPrivateKey.String(), "", http.StatusOK},
		{global, "keys/" + bKey.String(), a.ID.String(), http.StatusNotFound},
		{global, "providers/b-provider?workspace_id=" + a.ID.String(), "", http.StatusNotFound},
		{global, "aliases/b-alias", a.ID.String(), http.StatusNotFound},
	} {
		w := get(tc.access, tc.path, tc.header)
		if w.Code != tc.code {
			t.Errorf("GET %s: status=%d want=%d body=%s", tc.path, w.Code, tc.code, w.Body.String())
		}
	}
	w := get(global, "providers/b-provider", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"api_key_ref_key":"b-credential-ref"`) {
		t.Fatalf("fixture provider metadata: status=%d body=%s", w.Code, w.Body.String())
	}
	w = get(aMember, "keys/"+aSharedKey.String(), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"token_prefix":"prefix-a-shared"`) || strings.Contains(w.Body.String(), `"tenant"`) {
		t.Fatalf("shared key must retain its restricted detail view: status=%d body=%s", w.Code, w.Body.String())
	}
	w = get(aMember, "workspaces/"+a.ID.String(), "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"resources":{"aliases":1,"keys":2,"providers":1}`) {
		t.Fatalf("fixture workspace resource counts: status=%d body=%s", w.Code, w.Body.String())
	}

	t.Run("membership read fault", func(t *testing.T) {
		hook.fail = true
		if _, err := st.ListMembershipsByUser(ctx, aMemberID); !errors.Is(err, context.Canceled) {
			t.Fatalf("membership dependency did not fault: %v", err)
		}
		for _, list := range lists {
			before := hook.reads
			assertList(t, global, list.kind, "", "", list.global)
			if hook.reads != before {
				t.Errorf("global %s unexpectedly consulted memberships", list.kind)
			}
		}
		assertFailure := func(access, path, header string) {
			t.Helper()
			before := hook.reads
			w := get(access, path, header)
			if w.Code != http.StatusInternalServerError || w.Body.String() != "could not resolve workspace visibility\n" {
				t.Errorf("GET %s: status=%d body=%s, want only the generic visibility error", path, w.Code, w.Body.String())
			}
			if got := hook.reads - before; got != 1 {
				t.Errorf("GET %s consulted memberships %d times, want once", path, got)
			}
		}
		for _, list := range lists {
			assertFailure(aMember, list.kind, "")
			assertFailure(systemMember, list.kind, "")
		}
		for _, list := range lists[:3] {
			assertFailure(aMember, list.kind, a.ID.String())
			assertFailure(aMember, list.kind+"?workspace_id="+b.ID.String(), a.ID.String())
		}
		assertFailure(aMember, "providers/b-provider", "")
		assertFailure(aMember, "aliases/a-alias", a.ID.String())
		assertFailure(aMember, "keys/"+aSharedKey.String(), a.ID.String())
		assertFailure(aAdmin, "api-keys/"+aPrivateKey.String(), "")
		assertFailure(aMember, "workspaces/"+a.ID.String(), "")
		assertFailure(aMember, "workspaces/"+b.ID.String(), "")
		assertFailure(noMemberships, "keys/"+bKey.String(), "")
		assertFailure(systemMember, "providers/system-provider", system.String())
	})
}
