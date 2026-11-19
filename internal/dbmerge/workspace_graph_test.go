package dbmerge

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func TestMaterializeProviderWorkspaceReferences(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	for _, test := range []struct {
		name    string
		owner   uuid.UUID
		base    config.Provider
		allowed bool
	}{
		{name: "static System", base: testBases()["base"], allowed: true},
		{name: "static is not shared", owner: a, base: testBases()["base"]},
		{name: "explicit System", base: config.Provider{Workspace: identity.InWorkspace(store.SystemWorkspaceID)}, allowed: true},
		{name: "own managed", owner: a, base: config.Provider{DatabaseManaged: true, Workspace: identity.InWorkspace(a)}, allowed: true},
		{name: "foreign managed", owner: a, base: config.Provider{DatabaseManaged: true, Workspace: identity.InWorkspace(b)}},
		{name: "System cannot inherit tenant", base: config.Provider{DatabaseManaged: true, Workspace: identity.InWorkspace(a)}},
		{name: "unscoped managed is not System", base: config.Provider{DatabaseManaged: true}},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := test.base
			base.Name, base.Type, base.Enabled = "base", config.ProviderTypeOpenAI, true
			base.Models = []config.Model{{Name: "m", UpstreamName: "m"}}
			base.ModelByName = map[string]config.Model{"m": base.Models[0]}
			row := store.DBProvider{Name: "child", Type: string(base.Type), Extends: base.Name, WorkspaceID: test.owner}
			built, err := MaterializeProvider(row, nil, map[string]config.Provider{base.Name: base})
			if test.allowed {
				if err != nil || built.Workspace != identity.InWorkspace(test.owner) || len(built.Models) != 1 {
					t.Fatalf("own inheritance = %+v, %v", built, err)
				}
				return
			}
			_, absent := MaterializeProvider(row, nil, nil)
			if err == nil || absent == nil || err.Error() != absent.Error() {
				t.Fatalf("foreign and missing bases differ: %v vs %v", err, absent)
			}
			base.Enabled, base.Extends, base.TLSClientHello = false, "foreign-chain", "custom"
			base.TLSClientHelloSpecFile = "/private/foreign-recipe.json"
			row.APIKeyEncrypted = []byte("invalid-private-ciphertext")
			_, err = BuildProvider(row, nil, map[string]config.Provider{base.Name: base})
			if err == nil || err.Error() != absent.Error() {
				t.Fatalf("foreign metadata/credential validation ran before scope: %v", err)
			}
		})
	}
}

func workspaceGraphRuntime() *config.Runtime {
	model := config.Model{Name: "m", UpstreamName: "m", Protocol: config.ModelProtocolChat}
	return &config.Runtime{MultiTenancy: config.MultiTenancy{AllowedUpstreamOrigins: []string{"https://opencode.ai"}}, Catalog: config.NewCatalog([]config.Provider{{
		Name: "system-base", Type: config.ProviderTypeOpenCodeZen, Enabled: true,
		Models: []config.Model{model}, ModelByName: map[string]config.Model{"m": model},
	}}, nil, nil)}
}

func TestMergeCatalogWorkspaceGraph(t *testing.T) {
	st := openReasoningMergeStore(t)
	ctx := context.Background()
	a, b, empty := store.Workspace{Name: "a"}, store.Workspace{Name: "b"}, store.Workspace{Name: "empty"}
	for _, workspace := range []*store.Workspace{&a, &b, &empty} {
		if err := st.CreateWorkspace(ctx, workspace); err != nil {
			t.Fatal(err)
		}
	}
	rt := workspaceGraphRuntime()
	for name, owner := range map[string]uuid.UUID{"a-base": a.ID, "b-base": b.ID} {
		createReasoningProvider(t, st, store.DBProvider{Name: name, Type: "opencode-zen", WorkspaceID: owner, Enabled: true},
			[]store.DBProviderModel{{Name: "m", Protocol: "chat"}})
	}
	for _, owner := range []struct {
		name, base string
		id         uuid.UUID
	}{{"system", "system-base", store.SystemWorkspaceID}, {"a", "a-base", a.ID}, {"b", "b-base", b.ID}} {
		child := createReasoningProvider(t, st, store.DBProvider{Name: owner.name + "-child", Type: "opencode-zen",
			WorkspaceID: owner.id, Extends: owner.base, Enabled: true}, nil, store.CatalogEditOptions{Static: rt.Catalog})
		alias := store.DBAlias{Name: owner.name + "-route", WorkspaceID: owner.id, Algorithm: "round_robin"}
		if err := st.CreateAlias(ctx, &alias, []store.DBAliasTarget{{Provider: child.Name, Model: "m"}}, store.CatalogEditOptions{Static: rt.Catalog}); err != nil {
			t.Fatal(err)
		}
	}
	initial, err := MergeCatalog(ctx, st, rt)
	if err != nil {
		t.Fatal(err)
	}
	catalog := config.NewCatalog(initial.Providers, initial.DisabledProviders, initial.Aliases)
	for _, name := range []string{"system", "a", "b"} {
		p, _ := catalog.Provider(name + "-child")
		if !catalog.ModelVisibleInWorkspace("alias/"+name+"-route", p.Workspace) {
			t.Fatalf("same-workspace graph %s unavailable", name)
		}
		if catalog.ModelVisibleInWorkspace(name+"-child/m", identity.InWorkspace(empty.ID)) {
			t.Fatal("empty workspace acquired inventory")
		}
	}
	for _, test := range []struct {
		name    string
		owner   uuid.UUID
		base    string
		enabled bool
		targets []store.DBAliasTarget
	}{
		{name: "a-inherits-b", owner: a.ID, base: "b-base", enabled: true},
		{name: "disabled-a-inherits-b", owner: a.ID, base: "b-base"},
		{name: "a-inherits-System", owner: a.ID, base: "system-base"},
		{name: "System-inherits-a", owner: store.SystemWorkspaceID, base: "a-base"},
		{name: "a-targets-b", owner: a.ID, targets: []store.DBAliasTarget{{Provider: "b-base", Model: "m"}}},
		{name: "a-targets-System", owner: a.ID, targets: []store.DBAliasTarget{{Provider: "system-base", Model: "m"}}},
		{name: "System-targets-a", owner: store.SystemWorkspaceID, targets: []store.DBAliasTarget{{Provider: "a-base", Model: "m"}}},
		{name: "mixed", owner: a.ID, targets: []store.DBAliasTarget{{Provider: "a-base", Model: "m"}, {Provider: "b-child", Model: "m"}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.base != "" {
				row := corruptReasoningProvider(t, st, store.DBProvider{Name: "invalid", Type: "opencode-zen",
					WorkspaceID: test.owner, Extends: test.base, Enabled: test.enabled}, nil)
				t.Cleanup(func() { _ = st.DeleteProvider(ctx, row.ID) })
			} else {
				row := corruptReasoningAlias(t, st, store.DBAlias{Name: "invalid", WorkspaceID: test.owner, Algorithm: "round_robin"}, test.targets)
				t.Cleanup(func() { _ = st.DeleteAlias(ctx, row.ID) })
			}
			rejected, err := MergeCatalog(ctx, st, rt)
			if err == nil || !strings.Contains(err.Error(), "destination workspace") {
				t.Fatalf("foreign graph admitted: %v", err)
			}
			if !reflect.DeepEqual(rejected, Merged{}) || !reflect.DeepEqual(rt, workspaceGraphRuntime()) {
				t.Fatal("failed graph merge published partial output or mutated the static runtime")
			}
			if !catalog.ModelVisibleInWorkspace("alias/a-route", identity.InWorkspace(a.ID)) {
				t.Fatal("rejected graph mutated the previously built catalog")
			}
		})
	}
}

func TestMergeCatalogStaticManagedNameCollisions(t *testing.T) {
	for _, staticEnabled := range []bool{true, false} {
		for _, managedEnabled := range []bool{true, false} {
			for _, tenant := range []bool{true, false} {
				t.Run(strings.Join([]string{
					boolGraphName(staticEnabled), boolGraphName(managedEnabled), boolGraphName(tenant),
				}, "/"), func(t *testing.T) {
					st := openReasoningMergeStore(t)
					ctx := context.Background()
					owner := store.SystemWorkspaceID
					if tenant {
						workspace := store.Workspace{Name: "tenant"}
						if err := st.CreateWorkspace(ctx, &workspace); err != nil {
							t.Fatal(err)
						}
						owner = workspace.ID
					}
					row := createReasoningProvider(t, st, store.DBProvider{Name: "system-base", Type: "opencode-zen",
						Enabled: managedEnabled, WorkspaceID: owner}, []store.DBProviderModel{{Name: "m", Protocol: "chat"}})
					rt := workspaceGraphRuntime()
					if !staticEnabled {
						p, _ := rt.Catalog.Provider("system-base")
						p.Enabled = false
						rt.Catalog = config.NewCatalog(nil, []config.Provider{p}, nil)
					}
					before := rt.Catalog
					rejected, err := MergeCatalog(ctx, st, rt)
					if err == nil || !strings.Contains(err.Error(), "already defined in static config") {
						t.Fatalf("static/managed provider collision admitted: %v", err)
					}
					after, getErr := st.GetProvider(ctx, row.Name)
					if getErr != nil || !reflect.DeepEqual(row, after) || !reflect.DeepEqual(rt.Catalog, before) || !reflect.DeepEqual(rejected, Merged{}) {
						t.Fatal("collision overwrote, reassigned or changed a provider")
					}
				})
			}
		}
	}
	t.Run("aliases", func(t *testing.T) {
		st := openReasoningMergeStore(t)
		ctx := context.Background()
		rt := workspaceGraphRuntime()
		rt.Catalog = config.NewCatalog(rt.Catalog.Providers(), nil, []config.Alias{{
			Name: "route", Algorithm: config.AlgorithmRoundRobin, Targets: []config.AliasTarget{{Provider: "system-base", Model: "m"}},
		}})
		workspace := store.Workspace{Name: "tenant"}
		if err := st.CreateWorkspace(ctx, &workspace); err != nil {
			t.Fatal(err)
		}
		row := corruptReasoningAlias(t, st, store.DBAlias{Name: "route", WorkspaceID: workspace.ID, Algorithm: "round_robin"},
			[]store.DBAliasTarget{{Provider: "system-base", Model: "m"}})
		before := rt.Catalog
		if _, err := MergeCatalog(ctx, st, rt); err == nil || !strings.Contains(err.Error(), "already defined in static config") {
			t.Fatalf("static/managed alias collision admitted: %v", err)
		}
		after, err := st.GetAlias(ctx, row.Name)
		if err != nil || !reflect.DeepEqual(after, row) || !reflect.DeepEqual(rt.Catalog, before) {
			t.Fatal("collision overwrote or reassigned an alias")
		}
	})
}

func boolGraphName(value bool) string {
	if value {
		return "enabled"
	}
	return "disabled"
}
