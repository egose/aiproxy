package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/adminauth"
	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/deferred"
	"github.com/egose/aiproxy/internal/httpapi"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func workspaceInferenceDB(t *testing.T) (*store.Store, string) {
	t.Helper()
	dsn := os.Getenv("AIPROXY_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("AIPROXY_TEST_DATABASE_URL not set")
	}
	t.Setenv("AIPROXY_JWT_SECRET", "workspace-inference-signing-fixture")
	t.Setenv("AIPROXY_DB_ENCRYPTION_KEY", "workspace-inference-db-fixture")
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "workspace_inference_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.DB.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.DB.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	st, err := store.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.MigrateUp(ctx); err != nil {
		t.Fatal(err)
	}
	return st, u.String()
}

func workspaceInferenceRequest(t *testing.T, proxy *httptest.Server, method, path, token, body string) appDeferredReply {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	r, err := http.NewRequestWithContext(ctx, method, proxy.URL+path, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if body != "" {
		r.Header.Set("Content-Type", "application/json")
	}
	resp, err := proxy.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return appDeferredReply{status: resp.StatusCode, header: resp.Header, body: data}
}

func TestAppWorkspaceInferenceIsolation(t *testing.T) {
	st, dsn := workspaceInferenceDB(t)
	ctx := context.Background()
	workspaceA, workspaceB, empty := store.Workspace{Name: "tenant-a"}, store.Workspace{Name: "tenant-b"}, store.Workspace{Name: "empty"}
	for _, workspace := range []*store.Workspace{&workspaceA, &workspaceB, &empty} {
		if err := st.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatal(err)
		}
	}
	admin := store.User{Email: "admin@example.invalid", IsAdmin: true}
	if err := st.CreateUser(ctx, &admin); err != nil {
		t.Fatal(err)
	}
	jwt, _, err := adminauth.IssueAccess(admin.ID.String(), admin.Email, true)
	if err != nil {
		t.Fatal(err)
	}

	const system, first, second, tenantB = 0, 1, 2, 3
	upstreams := []string{"system", "a-first", "a-second", "b-local"}
	var calls [4]atomic.Int32
	var failFirst atomic.Bool
	var holdSystem atomic.Bool
	systemStarted, systemGate := make(chan struct{}, 1), make(chan struct{})
	releaseSystem := sync.OnceFunc(func() { close(systemGate) })
	snapshot := func() (got [4]int32) {
		for i := range calls {
			got[i] = calls[i].Load()
		}
		return got
	}
	addresses := make([]string, len(upstreams))
	mockOrigins := make(map[string]string)
	for i, name := range upstreams {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls[i].Add(1)
			var input struct {
				Model  string `json:"model"`
				Stream bool   `json:"stream"`
			}
			if err := json.NewDecoder(r.Body).Decode(&input); err != nil || r.URL.Path != "/v1/chat/completions" ||
				r.Header.Get("Authorization") != "Bearer "+name+"-upstream" {
				t.Errorf("unexpected inference dispatched to %s", name)
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if i == first && failFirst.Load() {
				w.WriteHeader(http.StatusTooManyRequests)
				return
			}
			if i == system && holdSystem.Load() {
				systemStarted <- struct{}{}
				select {
				case <-systemGate:
				case <-r.Context().Done():
					return
				}
			}
			data := fmt.Sprintf(`{"model":%q,"choices":[]}`, input.Model)
			if input.Stream {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprintf(w, "data: %s\n\ndata: [DONE]\n\n", data)
			} else {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, data)
			}
		}))
		t.Cleanup(upstream.Close)
		addresses[i] = upstream.URL
		if i != system {
			host := name + ".upstream.example.com"
			mockOrigins[host] = upstream.URL
			addresses[i] = "https://" + host
		}
	}
	t.Cleanup(releaseSystem)
	putProvider := func(i int, workspace uuid.UUID, models ...store.DBProviderModel) *store.DBProvider {
		t.Helper()
		encrypted, err := store.EncryptSecret([]byte(upstreams[i] + "-upstream"))
		if err != nil {
			t.Fatal(err)
		}
		row := &store.DBProvider{Name: upstreams[i], Type: "openai-compatible", BaseURL: addresses[i],
			Enabled: true, WorkspaceID: workspace, APIKeyEncrypted: encrypted}
		if err := st.CreateProviderAggregate(ctx, row, models); err != nil {
			t.Fatal(err)
		}
		return row
	}
	populateTenants := func() *store.DBProvider {
		putProvider(first, workspaceA.ID, store.DBProviderModel{Name: "m", UpstreamName: "a-main"})
		putProvider(second, workspaceA.ID, store.DBProviderModel{Name: "m", UpstreamName: "a-backup"})
		dbProviderB := putProvider(tenantB, workspaceB.ID, store.DBProviderModel{Name: "m/one", UpstreamName: "b-v1"})
		for _, child := range []struct{ name, base string }{{"a-child-local", "a-first"}} {
			encrypted, err := store.EncryptSecret([]byte("a-first-upstream"))
			if err != nil {
				t.Fatal(err)
			}
			row := store.DBProvider{Name: child.name, Type: "openai-compatible", Extends: child.base, Enabled: true,
				WorkspaceID: workspaceA.ID, APIKeyEncrypted: encrypted}
			if err := st.CreateProviderAggregate(ctx, &row, nil); err != nil {
				t.Fatal(err)
			}
		}
		for _, row := range []struct {
			name    string
			owner   uuid.UUID
			targets []store.DBAliasTarget
		}{
			{"a-route", workspaceA.ID, []store.DBAliasTarget{{Provider: "a-first", Model: "m"}, {Provider: "a-second", Model: "m"}}},
			{"b-route", workspaceB.ID, []store.DBAliasTarget{{Provider: "b-local", Model: "m/one"}}},
		} {
			alias := store.DBAlias{Name: row.name, WorkspaceID: row.owner, Algorithm: "least_connections", RetryStatusCodes: []int{429}}
			if err := st.CreateAlias(ctx, &alias, row.targets); err != nil {
				t.Fatal(err)
			}
		}
		return dbProviderB
	}
	for _, row := range []struct {
		token     string
		workspace uuid.UUID
		allow     []string
	}{
		{"system-key", store.SystemWorkspaceID, []string{}},
		{"a-key", workspaceA.ID, []string{}},
		{"b-key", workspaceB.ID, []string{}},
		{"empty-key", empty.ID, []string{}},
		{"empty-allowlist-key", empty.ID, []string{"alias/a-route", "b-local/m/one", "local/m", "absent/m"}},
	} {
		key := store.InboundKey{Name: row.token, TokenHash: store.TokenHash(row.token), TokenPrefix: "fixture",
			Tenant: "same-label", Enabled: true, WorkspaceID: row.workspace, AllowedModels: row.allow}
		if err := st.CreateInboundKey(ctx, &key); err != nil {
			t.Fatal(err)
		}
	}

	hosted := fmt.Sprintf(`
database { url = %q }
multi_tenancy {
  allowed_upstream_origins = ["https://a-first.upstream.example.com", "https://a-second.upstream.example.com", "https://b-local.upstream.example.com"]
}
alias "system-route" {
  algorithm = "round_robin"
  providers = ["local"]
  model = "m"
}
`, dsn)
	cfg := deferredLifecycleConfig(addresses[system], "system-file", "system-upstream", "", true, 4)
	cfg = strings.Replace(cfg, `    allowed_models = ["local/m"]`+"\n", "", 1) + hosted
	cfg = strings.Replace(cfg, `control_timeout = "100ms"`, `control_timeout = "2s"`, 1)
	path := writeConfigFile(t, cfg)
	static, err := config.Load([]byte(cfg), path)
	if err != nil {
		t.Fatal(err)
	}
	systemAlias := store.DBAlias{Name: "db-system-route", WorkspaceID: store.SystemWorkspaceID, Algorithm: "least_connections", RetryStatusCodes: []int{429}}
	if err := st.CreateAlias(ctx, &systemAlias, []store.DBAliasTarget{{Provider: "local", Model: "m"}}, store.CatalogEditOptions{Static: static.Catalog}); err != nil {
		t.Fatal(err)
	}
	build := func(t *testing.T) (*App, *httptest.Server) {
		t.Helper()
		instance, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard, transportFactory: tenantMockTransportFactory(mockOrigins)})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := instance.Close(); err != nil {
				t.Error(err)
			}
		})
		proxy := httptest.NewServer(instance.Server.Handler)
		t.Cleanup(proxy.Close)
		return instance, proxy
	}
	a, proxy := build(t)
	call := func(t *testing.T, method, route, token, body string) appDeferredReply {
		t.Helper()
		return workspaceInferenceRequest(t, proxy, method, route, token, body)
	}

	chat := func(t *testing.T, token, model string, stream bool) appDeferredReply {
		t.Helper()
		return call(t, http.MethodPost, "/v1/chat/completions", token, fmt.Sprintf(`{"model":%q,"messages":[],"stream":%t}`, model, stream))
	}
	list := func(t *testing.T, token string, want ...string) {
		t.Helper()
		resp := call(t, http.MethodGet, "/v1/models", token, "")
		var body struct{ Data []httpapi.ModelCard }
		if resp.status != http.StatusOK || json.Unmarshal(resp.body, &body) != nil || body.Data == nil {
			t.Fatalf("model list = %d %s", resp.status, resp.body)
		}
		names := make([]string, len(body.Data))
		for i, card := range body.Data {
			names[i] = card.ID
			if strings.HasPrefix(card.ID, "alias/") {
				wantTargets := map[string][]string{
					"alias/db-system-route": {"local/m"},
					"alias/system-route":    {"local/m"},
					"alias/a-route":         {"a-first/m", "a-second/m"},
					"alias/b-route":         {"b-local/m/one"},
				}[card.ID]
				targets := make([]string, len(card.AliasTargets))
				for i, target := range card.AliasTargets {
					targets[i] = target.Provider + "/" + target.Model
				}
				slices.Sort(targets)
				if wantTargets == nil || !slices.Equal(targets, wantTargets) {
					t.Fatalf("%s disclosed unexpected alias targets: %v", card.ID, targets)
				}
			}
		}
		slices.Sort(names)
		if !slices.Equal(names, want) {
			t.Fatalf("listed %v, want %v", names, want)
		}
	}
	t.Run("before-tenant-population", func(t *testing.T) {
		list(t, "system-file", "alias/db-system-route", "alias/system-route", "local/m")
		before := snapshot()
		for _, token := range []string{"a-key", "b-key"} {
			list(t, token)
			for _, stream := range []bool{false, true} {
				for _, model := range []string{"local/m", "alias/system-route"} {
					if resp := chat(t, token, model, stream); resp.status != http.StatusNotFound {
						t.Fatalf("empty workspace request for %s (stream %t) = %d, want 404", model, stream, resp.status)
					}
				}
			}
		}
		if after := snapshot(); after != before {
			t.Fatalf("empty workspace requests changed upstream counters from %v to %v", before, after)
		}
	})
	dbProviderB := populateTenants()
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	keyInfo := call(t, http.MethodPost, "/_internal/admin/keys", jwt,
		fmt.Sprintf(`{"name":"a-narrow","workspace_id":%q,"tenant":"same-label","allowed_models":["alias/a-route","local/m","alias/b-route","absent/m","alias/absent"]}`, workspaceA.ID.String()))
	var issued struct {
		Token string
		Key   struct{ ID uuid.UUID }
	}
	if keyInfo.status != http.StatusCreated || json.Unmarshal(keyInfo.body, &issued) != nil || issued.Token == "" || issued.Key.ID == uuid.Nil {
		t.Fatalf("issue workspace A key as system admin: status %d", keyInfo.status)
	}
	invoke := func(t *testing.T, token, model, upstreamModel string, stream bool, delta [4]int32) {
		t.Helper()
		before := snapshot()
		resp := chat(t, token, model, stream)
		if resp.status != http.StatusOK || !strings.Contains(string(resp.body), fmt.Sprintf(`"model":%q`, upstreamModel)) ||
			(stream && (!strings.Contains(string(resp.body), "[DONE]") || !strings.HasPrefix(resp.header.Get("Content-Type"), "text/event-stream"))) {
			t.Fatalf("%s (stream %t) = %d %s", model, stream, resp.status, resp.body)
		}
		after := snapshot()
		for i := range after {
			after[i] -= before[i]
		}
		if after != delta {
			t.Fatalf("%s (stream %t) contacted %v, want %v", model, stream, after, delta)
		}
	}
	check := func(t *testing.T, bRevision, narrowOwn string) {
		t.Helper()
		list(t, "system-file", "alias/db-system-route", "alias/system-route", "local/m")
		list(t, "system-key", "alias/db-system-route", "alias/system-route", "local/m")
		list(t, "a-key", "a-child-local/m", "a-first/m", "a-second/m", "alias/a-route")
		list(t, issued.Token, narrowOwn)
		list(t, "b-key", "alias/b-route", "b-local/m/one")
		list(t, "empty-key")
		list(t, "empty-allowlist-key")
		for _, stream := range []bool{false, true} {
			invoke(t, "system-file", "local/m", "m", stream, [4]int32{1})
			invoke(t, "system-key", "alias/db-system-route", "m", stream, [4]int32{1})
			invoke(t, "system-file", "alias/system-route", "m", stream, [4]int32{1})
			invoke(t, "a-key", "a-first/m", "a-main", stream, [4]int32{first: 1})
			invoke(t, "a-key", "a-child-local/m", "a-main", stream, [4]int32{first: 1})
			failFirst.Store(true)
			aliasToken := "a-key"
			if narrowOwn == "alias/a-route" {
				aliasToken = issued.Token
			}
			invoke(t, aliasToken, "alias/a-route", "a-backup", stream, [4]int32{first: 1, second: 1})
			failFirst.Store(false)
			if narrowOwn == "a-first/m" {
				invoke(t, issued.Token, "a-first/m", "a-main", stream, [4]int32{first: 1})
			}
			invoke(t, "b-key", "b-local/m/one", bRevision, stream, [4]int32{tenantB: 1})
			invoke(t, "b-key", "alias/b-route", bRevision, stream, [4]int32{tenantB: 1})

			before := snapshot()
			failFirst.Store(true)
			defer failFirst.Store(false)
			for _, test := range []struct{ token, direct, alias string }{
				{"system-file", "a-first/m", "alias/b-route"},
				{"system-key", "b-local/m/one", "alias/a-route"},
				{"a-key", "local/m", "alias/b-route"},
				{issued.Token, "local/m", "alias/b-route"},
				{"b-key", "a-first/m", "alias/system-route"},
				{"empty-key", "local/m", "alias/a-route"},
			} {
				for _, names := range [][2]string{{test.direct, "absent/m"}, {test.alias, "alias/absent"}} {
					foreign, missing := chat(t, test.token, names[0], stream), chat(t, test.token, names[1], stream)
					if foreign.status != http.StatusNotFound || foreign.status != missing.status ||
						string(foreign.body) != string(missing.body) || !strings.Contains(string(foreign.body), `"type":"model_not_found"`) {
						t.Fatalf("%s vs missing: %d %s vs %d %s", names[0], foreign.status, foreign.body, missing.status, missing.body)
					}
				}
			}
			if r := chat(t, "empty-allowlist-key", "alias/a-route", stream); r.status != http.StatusNotFound {
				t.Fatalf("an explicit allowlist populated an empty workspace: %d %s", r.status, r.body)
			}
			for _, test := range []struct {
				token  string
				models []string
			}{
				{"a-key", []string{"a-child-foreign/m/one", "alias/a-mixed", "alias/a-system-mixed", "alias/a-foreign-base"}},
				{"b-key", []string{"alias/b-mixed"}},
				{"system-key", []string{"alias/system-mixed"}},
				{"empty-allowlist-key", []string{"b-local/m/one", "local/m"}},
			} {
				missing := chat(t, test.token, "absent/m", stream)
				for _, model := range test.models {
					resp := chat(t, test.token, model, stream)
					if resp.status != http.StatusNotFound || resp.status != missing.status || string(resp.body) != string(missing.body) {
						t.Fatalf("foreign reference %s differed from a missing model: %d %s vs %d %s",
							model, resp.status, resp.body, missing.status, missing.body)
					}
				}
			}
			denied := "alias/a-route"
			if narrowOwn == "alias/a-route" {
				denied = "a-first/m"
			}
			if r := chat(t, issued.Token, denied, stream); r.status != http.StatusForbidden {
				t.Fatalf("explicit allowlist did not narrow own inventory: %d %s", r.status, r.body)
			}
			if after := snapshot(); after != before {
				t.Fatalf("denied requests changed upstream counters from %v to %v", before, after)
			}
			failFirst.Store(false)
		}
	}
	t.Run("initial", func(t *testing.T) { check(t, "b-v1", "alias/a-route") })

	t.Run("foreign-graphs-reject-reload-and-startup", func(t *testing.T) {
		for _, test := range []struct {
			name    string
			owner   uuid.UUID
			base    string
			targets []store.DBAliasTarget
		}{
			{name: "a-child-foreign", owner: workspaceA.ID, base: "b-local"},
			{name: "a-child-system", owner: workspaceA.ID, base: "local"},
			{name: "a-mixed", owner: workspaceA.ID, targets: []store.DBAliasTarget{{Provider: "a-first", Model: "m"}, {Provider: "b-local", Model: "m/one"}}},
			{name: "a-system-mixed", owner: workspaceA.ID, targets: []store.DBAliasTarget{{Provider: "a-first", Model: "m"}, {Provider: "local", Model: "m"}}},
			{name: "b-mixed", owner: workspaceB.ID, targets: []store.DBAliasTarget{{Provider: "a-first", Model: "m"}, {Provider: "b-local", Model: "m/one"}}},
			{name: "system-mixed", owner: store.SystemWorkspaceID, targets: []store.DBAliasTarget{{Provider: "a-first", Model: "m"}, {Provider: "local", Model: "m"}}},
		} {
			t.Run(test.name, func(t *testing.T) {
				if test.base != "" {
					row := store.DBProvider{Name: test.name, Type: "openai-compatible", Extends: test.base,
						ID: uuid.New(), WorkspaceID: test.owner, Enabled: false}
					if _, err := st.DB.NewInsert().Model(&row).Exec(ctx); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() { _ = st.DeleteProvider(ctx, row.ID) })
				} else {
					row := store.DBAlias{ID: uuid.New(), Name: test.name, WorkspaceID: test.owner, Algorithm: "round_robin"}
					if _, err := st.DB.NewInsert().Model(&row).Exec(ctx); err != nil {
						t.Fatal(err)
					}
					for _, target := range test.targets {
						target.ID, target.AliasID = uuid.New(), row.ID
						if _, err := st.DB.NewInsert().Model(&target).Exec(ctx); err != nil {
							t.Fatal(err)
						}
					}
					t.Cleanup(func() { _ = st.DeleteAlias(ctx, row.ID) })
				}
				before := snapshot()
				old := a.Config
				if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "destination workspace") {
					t.Fatalf("foreign graph reload admission = %v", err)
				}
				if a.Config != old {
					t.Fatal("rejected graph replaced the active runtime")
				}
				candidate, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard, transportFactory: tenantMockTransportFactory(mockOrigins)})
				if candidate != nil {
					_ = candidate.Close()
				}
				if err == nil || !strings.Contains(err.Error(), "destination workspace") {
					t.Fatalf("foreign graph startup admission = %v", err)
				}
				if after := snapshot(); after != before {
					t.Fatalf("graph admission made upstream calls: %v -> %v", before, after)
				}
				check(t, "b-v1", "alias/a-route")
			})
		}
	})

	updateB := func(upstreamModel string) {
		t.Helper()
		models := []store.DBProviderModel{{Name: "m/one", UpstreamName: upstreamModel}}
		if err := st.UpdateProviderAggregate(ctx, dbProviderB, &models); err != nil {
			t.Fatal(err)
		}
	}
	updateB("b-v2")
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	t.Run("reload", func(t *testing.T) { check(t, "b-v2", "alias/a-route") })

	updateB("b-v3")
	if _, err := st.DB.NewUpdate().Model((*store.InboundKey)(nil)).
		Set("allowed_models = ?", `["a-first/m","local/m","alias/b-route","absent/m","alias/absent"]`).
		Where("id = ?", issued.Key.ID).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	rewriteConfigFile(t, path, strings.Replace(cfg, "127.0.0.1:0", "127.0.0.1:12345", 1))
	if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "listener changes require restart") {
		t.Fatalf("expected reload rejection after catalog preparation, got %v", err)
	}
	t.Run("failed-reload-keeps-catalog-but-refreshes-key-policy", func(t *testing.T) { check(t, "b-v2", "a-first/m") })
	rewriteConfigFile(t, path, cfg)
	proxy.Close()
	if err := a.Close(); err != nil {
		t.Fatal(err)
	}
	a, proxy = build(t)
	t.Run("restart", func(t *testing.T) { check(t, "b-v3", "a-first/m") })

	t.Run("bearer-and-peer-boundaries", func(t *testing.T) {
		before := snapshot()
		for _, token := range []string{"", jwt} {
			for _, route := range []string{"/v1/models", "/v1/billing/usage"} {
				if r := call(t, http.MethodGet, route, token, ""); r.status != http.StatusUnauthorized {
					t.Fatalf("%s without inference identity = %d %s", route, r.status, r.body)
				}
			}
			if r := chat(t, token, "local/m", false); r.status != http.StatusUnauthorized {
				t.Fatalf("chat without inference identity = %d %s", r.status, r.body)
			}
			if r := callAppDeferred(t, proxy, http.MethodGet, deferred.Prefix+"/capabilities?model=local/m", token, "", nil); r.status != http.StatusUnauthorized {
				t.Fatalf("capability without inference identity = %d %s", r.status, r.body)
			}
		}
		missing := callAppDeferred(t, proxy, http.MethodGet, deferred.Prefix+"/capabilities?model=absent/m", "a-key", "", nil)
		foreign := callAppDeferred(t, proxy, http.MethodGet, deferred.Prefix+"/capabilities?model=local/m", "a-key", "", nil)
		if foreign.status != http.StatusNotFound || foreign.status != missing.status || string(foreign.body) != string(missing.body) {
			t.Fatalf("foreign peer discovery differs from missing: %d %s vs %d %s", foreign.status, foreign.body, missing.status, missing.body)
		}
		if after := snapshot(); after != before {
			t.Fatalf("auth checks changed upstream counters from %v to %v", before, after)
		}

		capability := appDeferredCapability(t, proxy, "system-key")
		_, wire := appDeferredSubmit(t, capability)
		holdSystem.Store(true)
		defer releaseSystem()
		ack := callAppDeferred(t, proxy, http.MethodPost, deferred.Prefix+"/submit", "system-key", "", wire)
		var receipt deferred.Receipt
		if ack.status != http.StatusAccepted || deferred.DecodeControl(ack.body, &receipt) != nil {
			t.Fatalf("System key peer submission = %d %s", ack.status, ack.body)
		}
		select {
		case <-systemStarted:
		case <-time.After(time.Second):
			t.Fatal("System deferred execution did not start")
		}
		job := deferred.Prefix + "/jobs/" + receipt.ID
		for _, token := range []string{"a-key", "b-key", "empty-key", "system-file"} {
			for _, route := range []struct{ method, path string }{
				{http.MethodGet, job}, {http.MethodGet, job + "/result"},
				{http.MethodGet, job + "/read?cursor=0"}, {http.MethodDelete, job},
			} {
				foreign = callAppDeferred(t, proxy, route.method, route.path, token, receipt.Generation, nil)
				if foreign.status != http.StatusNotFound || !strings.Contains(string(foreign.body), `"type":"job_unavailable"`) ||
					strings.Contains(string(foreign.body), receipt.ID) {
					t.Fatalf("another owner accessed a System job: %d %s", foreign.status, foreign.body)
				}
			}
			foreign = callAppDeferred(t, proxy, http.MethodPost, deferred.Prefix+"/submit", token, "", wire)
			if foreign.status != http.StatusBadRequest || foreign.header.Get("Location") != "" || strings.Contains(string(foreign.body), receipt.ID) {
				t.Fatalf("another owner replayed System work: %d %s", foreign.status, foreign.body)
			}
		}
		draining := deferredLifecycleConfig(addresses[system], "system-file", "system-upstream", "", false, 4) + hosted
		rewriteConfigFile(t, path, draining)
		oldStore := a.DeferredStore()
		if err := a.Reload(); err != nil {
			t.Fatal(err)
		}
		if a.DeferredStore() != oldStore || oldStore.Options().Enabled || oldStore.Generation() != receipt.Generation {
			t.Fatal("receiver removal lost the admitted workspace's deferred store")
		}
		status := callAppDeferred(t, proxy, http.MethodGet, job, "system-key", receipt.Generation, nil)
		var info deferred.JobInfo
		if status.status != http.StatusOK || deferred.DecodeControl(status.body, &info) != nil || info.State != deferred.StateRunning {
			t.Fatalf("the admitted System owner could not drain: %d %s", status.status, status.body)
		}
		replay := callAppDeferred(t, proxy, http.MethodPost, deferred.Prefix+"/submit", "system-key", "", wire)
		var receipt2 deferred.Receipt
		if replay.status != http.StatusAccepted || deferred.DecodeControl(replay.body, &receipt2) != nil ||
			!receipt2.Replayed || receipt2.ID != receipt.ID {
			t.Fatalf("System key peer replay = %d %s", replay.status, replay.body)
		}
		if r := callAppDeferred(t, proxy, http.MethodGet, deferred.Prefix+"/capabilities?model=local/m", "system-key", "", nil); r.status != http.StatusNotFound {
			t.Fatalf("removed receiver admitted new discovery: %d %s", r.status, r.body)
		}
		_, fresh := appDeferredSubmit(t, capability)
		if r := callAppDeferred(t, proxy, http.MethodPost, deferred.Prefix+"/submit", "system-key", "", fresh); r.status != http.StatusNotFound {
			t.Fatalf("removed receiver admitted fresh work: %d %s", r.status, r.body)
		}
		releaseSystem()
		holdSystem.Store(false)
		awaitResourceCondition(t, "System deferred job finalization", func() bool { return a.DeferredStore().Stats().FinalizationCredits == 0 })
		result := callAppDeferred(t, proxy, http.MethodGet, job+"/result", "system-key", receipt.Generation, nil)
		var manifest deferred.ResultManifest
		if result.status != http.StatusOK || deferred.DecodeControl(result.body, &manifest) != nil ||
			manifest.State != deferred.StateCompleted || manifest.Response == nil || manifest.Response.StatusCode != http.StatusOK {
			t.Fatalf("System deferred result = %d %s", result.status, result.body)
		}
		read := callAppDeferred(t, proxy, http.MethodGet, job+"/read?cursor=0", "system-key", receipt.Generation, nil)
		var frame deferred.ReadFrame
		if read.status != http.StatusOK || deferred.DecodeControl(read.body, &frame) != nil || !frame.Done ||
			!strings.Contains(string(frame.Data), `"model":"m"`) {
			t.Fatalf("draining result lost its admitted response: %d %s", read.status, read.body)
		}
		if r := callAppDeferred(t, proxy, http.MethodGet, job+"/result", "b-key", receipt.Generation, nil); r.status != http.StatusNotFound {
			t.Fatalf("foreign workspace saw the completed draining result: %d %s", r.status, r.body)
		}
		if r := callAppDeferred(t, proxy, http.MethodDelete, job, "system-key", receipt.Generation, nil); r.status != http.StatusNoContent {
			t.Fatalf("the admitted owner could not release draining work: %d %s", r.status, r.body)
		}
		before[system]++
		if after := snapshot(); after != before {
			t.Fatalf("peer replay/controls processed more than the one System call: %v, want %v", after, before)
		}
	})

	t.Run("management-only", func(t *testing.T) {
		proxy.Close()
		if err := a.Close(); err != nil {
			t.Fatal(err)
		}
		management := deferredLifecycleConfig(addresses[system], "system-file", "system-upstream", "", false, 4) + hosted
		authStart, authEnd := strings.Index(management, `auth "main"`), strings.Index(management, `provider "openai-compatible"`)
		rewriteConfigFile(t, path, management[:authStart]+`auth "main" { mode = "none" }`+"\n"+management[authEnd:])
		_, proxy = build(t)
		before := snapshot()
		if r := call(t, http.MethodGet, "/_internal/admin/me", jwt, ""); r.status != http.StatusOK {
			t.Fatalf("management-only browser session = %d %s", r.status, r.body)
		}
		for _, token := range []string{"", "system-key", "system-file", jwt} {
			for _, route := range []string{"/v1/models", "/v1/billing/usage"} {
				if r := call(t, http.MethodGet, route, token, ""); r.status != http.StatusUnauthorized {
					t.Fatalf("management-only %s = %d %s", route, r.status, r.body)
				}
			}
			if r := chat(t, token, "local/m", true); r.status != http.StatusUnauthorized {
				t.Fatalf("management-only chat = %d %s", r.status, r.body)
			}
			if r := callAppDeferred(t, proxy, http.MethodGet, deferred.Prefix+"/capabilities?model=local/m", token, "", nil); r.status != http.StatusUnauthorized {
				t.Fatalf("management-only peer discovery = %d %s", r.status, r.body)
			}
			if r := callAppDeferred(t, proxy, http.MethodPost, deferred.Prefix+"/submit", token, "", nil); r.status != http.StatusUnauthorized {
				t.Fatalf("management-only peer submission = %d %s", r.status, r.body)
			}
		}
		if after := snapshot(); after != before {
			t.Fatalf("management-only routes changed upstream counters from %v to %v", before, after)
		}
	})
}
