package httpapi

import (
	"context"
	"fmt"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/copilotlogin"
	"github.com/egose/aiproxy/internal/dbmerge"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

type adminSavedGraph struct {
	Providers []store.DBProvider
	Models    map[uuid.UUID][]store.DBProviderModel
	Aliases   []store.DBAlias
	Targets   map[uuid.UUID][]store.DBAliasTarget
}

func readAdminSavedGraph(t *testing.T, st *store.Store) adminSavedGraph {
	t.Helper()
	ctx := context.Background()
	out := adminSavedGraph{Models: make(map[uuid.UUID][]store.DBProviderModel), Targets: make(map[uuid.UUID][]store.DBAliasTarget)}
	var err error
	out.Providers, err = st.ListProviders(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range out.Providers {
		out.Models[p.ID], err = st.ListProviderModels(ctx, p.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	out.Aliases, err = st.ListAliases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range out.Aliases {
		out.Targets[a.ID], err = st.ListAliasTargets(ctx, a.ID)
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

func TestAdminCatalogDependencyRejections(t *testing.T) {
	for _, test := range []struct {
		name, method, resource, body string
		child, alias, overlay        bool
		typ                          string
	}{
		{name: "delete-direct-target", method: "DELETE", alias: true},
		{name: "disable-direct-target", body: `{"enabled":false}`, alias: true},
		{name: "remove-direct-model", body: `{"models":[{"name":"keep","capabilities":["chat"]}]}`, alias: true},
		{name: "remove-inherited-model", body: `{"models":[{"name":"keep","capabilities":["chat"]}]}`, child: true, alias: true},
		{name: "remove-disabled-child-overlay", body: `{"models":[{"name":"keep"}]}`, child: true, overlay: true},
		{name: "delete-disabled-child-base", method: "DELETE", child: true},
		{name: "disable-disabled-child-base", body: `{"enabled":false}`, child: true},
		{name: "change-base-type", body: `{"type":"gemini"}`, child: true},
		{name: "turn-base-into-derivative", body: `{"extends":"peer","models":[]}`, child: true},
		{name: "retarget-derivative", resource: "child", body: `{"extends":"peer","models":[]}`, child: true, alias: true},
		{name: "exit-derivation", resource: "child", body: `{"extends":"","models":[{"name":"keep","capabilities":["chat"]}]}`, child: true, alias: true},
		{name: "incompatible-capabilities", body: `{"models":[{"name":"m","capabilities":["embeddings"]},{"name":"keep","capabilities":["chat"]}]}`, alias: true},
		{name: "incompatible-protocol", typ: "opencode-zen", body: `{"models":[{"name":"m","protocol":"responses"},{"name":"keep","protocol":"chat"}]}`, alias: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAdminReasoningFixture(t, nil)
			typ := test.typ
			if typ == "" {
				typ = "openai"
			}
			protocol := ""
			if typ == "opencode-zen" {
				protocol = `,"protocol":"chat"`
			}
			body := fmt.Sprintf(`{"name":"base","type":%q,"api_key":"private-base-fixture","models":[{"name":"m","capabilities":["chat"]%s},{"name":"keep","capabilities":["chat"]%s}]}`, typ, protocol, protocol)
			requireAdminReasoningStatus(t, fixture, "POST", "/_internal/admin/providers", body, 201)
			body = fmt.Sprintf(`{"name":"peer","type":%q,"api_key":"private-peer-fixture","models":[{"name":"keep","capabilities":["chat"]%s}]}`, typ, protocol)
			requireAdminReasoningStatus(t, fixture, "POST", "/_internal/admin/providers", body, 201)
			target := "base"
			if test.child {
				overlays := ""
				if test.overlay {
					overlays = `,"models":[{"name":"m","encrypted_reasoning":{}}]`
				}
				body = fmt.Sprintf(`{"name":"child","type":%q,"extends":"base","api_key":"private-child-fixture","enabled":%t%s}`, typ, test.alias, overlays)
				requireAdminReasoningStatus(t, fixture, "POST", "/_internal/admin/providers", body, 201)
				target = "child"
			}
			if test.alias {
				body = fmt.Sprintf(`{"name":"private-referencer","targets":[{"provider":%q,"model":"m"},{"provider":"peer","model":"keep"}]}`, target)
				requireAdminReasoningStatus(t, fixture, "POST", "/_internal/admin/aliases", body, 201)
			}
			before, activations := readAdminSavedGraph(t, fixture.store), fixture.activations
			method, resource := test.method, test.resource
			if method == "" {
				method = "PUT"
			}
			if resource == "" {
				resource = "base"
			}
			w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, method, "/_internal/admin/providers/"+resource, test.body)
			if w.Code != 400 || w.Body.String() != store.ErrCatalogDependency.Error()+"\n" || w.Header().Get(catalogSavedHeader) != "" {
				t.Fatalf("dependency rejection = %d %s saved=%q", w.Code, w.Body.String(), w.Header().Get(catalogSavedHeader))
			}
			if fixture.activations != activations || !reflect.DeepEqual(before, readAdminSavedGraph(t, fixture.store)) {
				t.Fatal("rejected dependency edit persisted an aggregate or requested reload")
			}
			if _, err := dbmerge.MergeCatalog(context.Background(), fixture.store, fixture.runtime); err != nil {
				t.Fatalf("rejected edit stranded saved graph: %v", err)
			}
		})
	}
}

func TestAdminCatalogDependencyStaticAliasDelete(t *testing.T) {
	fixture := newAdminReasoningFixture(t, nil)
	requireAdminReasoningStatus(t, fixture, "POST", "/_internal/admin/providers",
		`{"name":"base","type":"openai","api_key":"fixture","models":[{"name":"m","capabilities":["chat"]}]}`, 201)
	staticAlias := config.Alias{
		Name: "static-route", Algorithm: config.AlgorithmRoundRobin,
		Targets: []config.AliasTarget{{Provider: "base", Model: "m"}},
	}
	fixture.runtime.Catalog = config.NewCatalog(
		fixture.runtime.Catalog.Providers(), fixture.runtime.Catalog.DisabledProviders(),
		append(fixture.runtime.Catalog.Aliases(), staticAlias),
	)
	deps := fixture.handler.current()
	deps.Catalog = config.NewCatalog(
		deps.Catalog.Providers(), deps.Catalog.DisabledProviders(), append(deps.Catalog.Aliases(), staticAlias),
	)
	fixture.handler.UpdateDependencies(deps)
	before, activations := readAdminSavedGraph(t, fixture.store), fixture.activations
	w, _ := adminReasoningRequest(t, fixture.handler, fixture.access, "DELETE", "/_internal/admin/providers/base", "")
	if w.Code != 400 || w.Body.String() != store.ErrCatalogDependency.Error()+"\n" || w.Header().Get(catalogSavedHeader) != "" {
		t.Fatalf("static alias target deletion = %d %s saved=%q", w.Code, w.Body.String(), w.Header().Get(catalogSavedHeader))
	}
	if fixture.activations != activations || !reflect.DeepEqual(before, readAdminSavedGraph(t, fixture.store)) {
		t.Fatal("static alias dependency rejection persisted or requested activation")
	}
	if _, err := dbmerge.MergeCatalog(context.Background(), fixture.store, fixture.runtime); err != nil {
		t.Fatalf("rejected static alias target deletion stranded saved graph: %v", err)
	}
}

type graphHTTPPauseKey struct{}

type graphHTTPPauseHook struct {
	once    sync.Once
	locked  chan struct{}
	release chan struct{}
}

func (h *graphHTTPPauseHook) BeforeQuery(ctx context.Context, _ *bun.QueryEvent) context.Context {
	return ctx
}

func (h *graphHTTPPauseHook) AfterQuery(ctx context.Context, event *bun.QueryEvent) {
	if ctx.Value(graphHTTPPauseKey{}) == true && event.Err == nil &&
		strings.Contains(event.Query, `FROM "workspaces"`) && strings.Contains(event.Query, "FOR NO KEY UPDATE") {
		h.once.Do(func() {
			close(h.locked)
			select {
			case <-h.release:
			case <-ctx.Done():
			}
		})
	}
}

func waitCatalogHTTPBlocked(t *testing.T, ctx context.Context, st *store.Store, workspace uuid.UUID) {
	t.Helper()
	ticker := time.NewTicker(time.Millisecond)
	defer ticker.Stop()
	for {
		var n int
		if err := st.DB.NewRaw(`SELECT count(*) FROM pg_stat_activity WHERE datname = current_database()
			AND cardinality(pg_blocking_pids(pid)) > 0 AND query LIKE ?`, "%"+workspace.String()+"%").Scan(ctx, &n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			return
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatal("catalog write did not wait for the workspace graph lock")
		}
	}
}

func TestAdminCatalogDependencyConcurrentWrites(t *testing.T) {
	for _, referencer := range []string{"alias-create", "alias-update", "child-create", "child-update"} {
		for _, dependentFirst := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dependent-first=%t", referencer, dependentFirst), func(t *testing.T) {
				fixture := newAdminReasoningFixture(t, nil)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				for _, name := range []string{"base", "peer"} {
					requireAdminReasoningStatus(t, fixture, "POST", "/_internal/admin/providers",
						fmt.Sprintf(`{"name":%q,"type":"openai","api_key":"fixture","models":[{"name":"m"}]}`, name), 201)
				}
				method, path := "POST", "/_internal/admin/aliases"
				body := `{"name":"route","targets":[{"provider":"base","model":"m"}]}`
				if strings.HasPrefix(referencer, "child-") {
					path = "/_internal/admin/providers"
					body = `{"name":"child","type":"openai","extends":"base","api_key":"child-fixture","enabled":false}`
				}
				if strings.HasSuffix(referencer, "-update") {
					method = "PUT"
					resource := "route"
					setup := `{"name":"route","targets":[{"provider":"peer","model":"m"}]}`
					if strings.HasPrefix(referencer, "child-") {
						resource = "child"
						setup = `{"name":"child","type":"openai","extends":"peer","api_key":"child-fixture","enabled":false}`
					}
					requireAdminReasoningStatus(t, fixture, "POST", path, setup, 201)
					path += "/" + resource
				}
				pause := &graphHTTPPauseHook{locked: make(chan struct{}), release: make(chan struct{})}
				fixture.store.DB.AddQueryHook(pause)
				release := sync.OnceFunc(func() { close(pause.release) })
				defer release()
				count := fixture.activations
				firstMethod, firstPath, firstBody := "DELETE", "/_internal/admin/providers/base", ""
				secondMethod, secondPath, secondBody := method, path, body
				if dependentFirst {
					firstMethod, firstPath, firstBody, secondMethod, secondPath, secondBody =
						secondMethod, secondPath, secondBody, firstMethod, firstPath, firstBody
				}
				results := make(chan *httptest.ResponseRecorder, 2)
				go func() {
					results <- membershipRequest(context.WithValue(ctx, graphHTTPPauseKey{}, true), fixture.handler, fixture.access, firstMethod, firstPath, firstBody)
				}()
				select {
				case <-pause.locked:
				case <-ctx.Done():
					t.Fatal("first catalog write did not acquire graph lock")
				}
				go func() {
					results <- membershipRequest(ctx, fixture.handler, fixture.access, secondMethod, secondPath, secondBody)
				}()
				waitCatalogHTTPBlocked(t, ctx, fixture.store, store.SystemWorkspaceID)
				release()
				saved, rejected := 0, 0
				for range 2 {
					w := <-results
					if w.Code < 300 {
						saved++
					} else if w.Code == 400 && w.Body.String() == store.ErrCatalogDependency.Error()+"\n" && w.Header().Get(catalogSavedHeader) == "" {
						rejected++
					} else {
						t.Fatalf("concurrent HTTP outcome = %d %s", w.Code, w.Body.String())
					}
				}
				if saved != 1 || rejected != 1 || fixture.activations != count+1 {
					t.Fatalf("nonserial HTTP outcomes saved=%d rejected=%d activations=%d", saved, rejected, fixture.activations-count)
				}
				if _, err := dbmerge.MergeCatalog(ctx, fixture.store, fixture.runtime); err != nil {
					t.Fatalf("HTTP race stranded saved catalog: %v", err)
				}
			})
		}
	}
}

func TestAdminCatalogDependencyCurrentBasePolicyAfterWait(t *testing.T) {
	for _, operation := range []string{"create-child", "credential", "alias"} {
		for _, corruption := range []string{"unapproved-origin", "foreign-workspace", "host-reference"} {
			t.Run(operation+"/"+corruption, func(t *testing.T) {
				fixture := newAdminReasoningFixture(t, nil)
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				a, b := store.Workspace{Name: "a"}, store.Workspace{Name: "b"}
				for _, workspace := range []*store.Workspace{&a, &b} {
					if err := fixture.store.CreateWorkspace(ctx, workspace); err != nil {
						t.Fatal(err)
					}
				}
				base := store.DBProvider{Name: "base", Type: "openai", WorkspaceID: a.ID, Enabled: true}
				encrypted, err := store.EncryptSecret([]byte("own-fixture"))
				if err != nil {
					t.Fatal(err)
				}
				base.APIKeyEncrypted = encrypted
				if err := fixture.store.CreateProviderAggregate(ctx, &base, []store.DBProviderModel{{Name: "m"}}); err != nil {
					t.Fatal(err)
				}
				child := store.DBProvider{Name: "child", Type: base.Type, Extends: base.Name, WorkspaceID: a.ID, Enabled: true, APIKeyEncrypted: encrypted}
				if err := fixture.store.CreateProvider(ctx, &child); err != nil {
					t.Fatal(err)
				}
				method, path := "POST", "/_internal/admin/providers"
				body := fmt.Sprintf(`{"name":"candidate","workspace_id":%q,"type":"openai","extends":"base","api_key":"never-saved"}`, a.ID)
				if operation == "credential" {
					method, path, body = "PUT", "/_internal/admin/providers/child/credential", `{"api_key":"never-saved"}`
				} else if operation == "alias" {
					path = "/_internal/admin/aliases"
					body = fmt.Sprintf(`{"name":"candidate","workspace_id":%q,"targets":[{"provider":"child","model":"m"}]}`, a.ID)
				}
				before, models := adminReasoningSavedProvider(t, fixture.store, child.Name)
				holder, err := fixture.store.DB.BeginTx(ctx, nil)
				if err != nil {
					t.Fatal(err)
				}
				defer holder.Rollback()
				if _, err := holder.ExecContext(ctx, "SELECT id FROM workspaces WHERE id = ? FOR NO KEY UPDATE", a.ID); err != nil {
					t.Fatal(err)
				}
				count := fixture.activations
				done := make(chan *httptest.ResponseRecorder, 1)
				go func() { done <- membershipRequest(ctx, fixture.handler, fixture.access, method, path, body) }()
				waitCatalogHTTPBlocked(t, ctx, fixture.store, a.ID)
				query, value := "base_url", "https://unapproved.example.com"
				if corruption == "foreign-workspace" {
					query, value = "workspace_id", b.ID.String()
				} else if corruption == "host-reference" {
					query, value = "api_key_ref_path", "/never-open-private-fixture/keys.json"
				}
				if _, err := holder.ExecContext(ctx, "UPDATE db_providers SET "+query+" = ? WHERE id = ?", value, base.ID); err != nil {
					t.Fatal(err)
				}
				if err := holder.Commit(); err != nil {
					t.Fatal(err)
				}
				w := <-done
				if w.Code != 400 || w.Body.String() != store.ErrCatalogDependency.Error()+"\n" || w.Header().Get(catalogSavedHeader) != "" || fixture.activations != count {
					t.Fatalf("stale policy/reference preflight committed: %d %s", w.Code, w.Body.String())
				}
				assertAdminReasoningProviderUnchanged(t, fixture.store, before, models)
				if _, err := fixture.store.GetProvider(ctx, "candidate"); err == nil {
					t.Fatal("rejected child persisted")
				}
				if _, err := fixture.store.GetAlias(ctx, "candidate"); err == nil {
					t.Fatal("rejected alias persisted")
				}
			})
		}
	}
}

func TestAdminCatalogDependencyCopilotFlowPreserved(t *testing.T) {
	f := openCopilotFlowHarness(t)
	ctx := context.Background()
	_, access := f.user(t, true)
	name := uniqName(t, "dependency-copilot")
	credential, err := copilotlogin.NewCredential("Ov23flowtestclient", "private-old-copilot-fixture", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	encrypted, err := store.EncryptCopilotCredential(credential, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	base := store.DBProvider{Name: name, Type: "github-copilot", Enabled: true, WorkspaceID: store.SystemWorkspaceID, CopilotCredentialEncrypted: encrypted}
	if err := f.st.CreateProviderAggregate(ctx, &base, []store.DBProviderModel{{Name: "m"}, {Name: "keep"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.st.DeleteProvider(ctx, base.ID) })
	alias := store.DBAlias{Name: uniqName(t, "dependency-route"), Algorithm: "round_robin", WorkspaceID: store.SystemWorkspaceID}
	if err := f.st.CreateAlias(ctx, &alias, []store.DBAliasTarget{{Provider: base.Name, Model: "m"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = f.st.DeleteAlias(ctx, alias.ID) })
	start := f.call(t, access, "POST", "/_internal/admin/copilot-device-flows", map[string]any{
		"client_id": "Ov23flowtestclient", "provider_name": base.Name,
	})
	if start.code != 201 {
		t.Fatalf("flow start = %d %s", start.code, start.raw)
	}
	id := flowIDOf(t, start)
	f.expeditePoll(t, id)
	f.issuer.enqueue("success")
	ready := f.call(t, access, "POST", "/_internal/admin/copilot-device-flows/"+id+"/poll", nil)
	if ready.code != 200 || ready.parsed["status"] != "ready" {
		t.Fatalf("flow ready = %d %s", ready.code, ready.raw)
	}
	before, models := adminReasoningSavedProvider(t, f.st, base.Name)
	count := 0
	deps := f.h.current()
	deps.RequestReload = func() error { count++; return nil }
	f.h.UpdateDependencies(deps)
	body := map[string]any{"copilot_device_flow_id": id, "expected_updated_at": before.UpdatedAt.Format(time.RFC3339Nano),
		"models": []map[string]string{{"name": "keep"}}, "display_name": "never-saved"}
	rejected := f.call(t, access, "PUT", "/_internal/admin/providers/"+base.Name, body)
	if rejected.code != 400 || rejected.raw != store.ErrCatalogDependency.Error()+"\n" || rejected.header.Get(catalogSavedHeader) != "" || count != 0 {
		t.Fatalf("Copilot dependency rejection = %d %s count=%d", rejected.code, rejected.raw, count)
	}
	assertAdminReasoningProviderUnchanged(t, f.st, before, models)
	kept := f.call(t, access, "GET", "/_internal/admin/copilot-device-flows/"+id, nil)
	if kept.code != 200 || kept.parsed["status"] != "ready" {
		t.Fatal("rejected aggregate consumed the ready flow")
	}
	if err := f.st.DeleteAlias(ctx, alias.ID); err != nil {
		t.Fatal(err)
	}
	saved := f.call(t, access, "PUT", "/_internal/admin/providers/"+base.Name, body)
	if saved.code != 200 || saved.header.Get(catalogSavedHeader) != "true" || count != 1 {
		t.Fatalf("cleanup/retry = %d %s count=%d", saved.code, saved.raw, count)
	}
	consumed := f.call(t, access, "GET", "/_internal/admin/copilot-device-flows/"+id, nil)
	if consumed.code != 200 || consumed.parsed["status"] != "consumed" {
		t.Fatal("valid graph cleanup did not consume the retained flow")
	}
}
