package modelresolver

import (
	"errors"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/identity"
	"github.com/google/uuid"
)

func TestResolveInWorkspaceUsesCompleteOwnedGraph(t *testing.T) {
	system, a, b, empty := identity.InWorkspace(uuid.Nil), identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New())
	providers := []config.Provider{
		{Name: "system", Workspace: system},
		{Name: "a", Workspace: a},
		{Name: "b", Workspace: b},
		{Name: "unowned"},
		{Name: "child", Workspace: a, Extends: "a"},
		{Name: "foreign-child", Workspace: a, Extends: "b"},
		{Name: "system-child", Workspace: a, Extends: "system"},
		{Name: "missing-child", Workspace: a, Extends: "missing"},
		{Name: "unowned-child", Workspace: a, Extends: "unowned"},
		{Name: "chained-child", Workspace: a, Extends: "child"},
	}
	for i := range providers {
		providers[i].Models = []config.Model{{Name: "m/one", UpstreamName: "upstream"}}
	}
	local := config.AliasTarget{Provider: "a", Model: "m/one"}
	aliases := []config.Alias{
		{Name: "system", Workspace: system, Targets: []config.AliasTarget{{Provider: "system", Model: "m/one"}}},
		{Name: "a", Workspace: a, Targets: []config.AliasTarget{local, {Provider: "child", Model: "m/one"}}},
		{Name: "b", Workspace: b, Targets: []config.AliasTarget{{Provider: "b", Model: "m/one"}}},
		{Name: "foreign-target", Workspace: a, Targets: []config.AliasTarget{local, {Provider: "b", Model: "m/one"}}},
		{Name: "system-target", Workspace: a, Targets: []config.AliasTarget{local, {Provider: "system", Model: "m/one"}}},
		{Name: "missing-target", Workspace: a, Targets: []config.AliasTarget{local, {Provider: "a", Model: "missing"}}},
		{Name: "missing-provider", Workspace: a, Targets: []config.AliasTarget{local, {Provider: "missing", Model: "m/one"}}},
		{Name: "unowned-target", Workspace: a, Targets: []config.AliasTarget{local, {Provider: "unowned", Model: "m/one"}}},
		{Name: "invalid-base", Workspace: a, Targets: []config.AliasTarget{local, {Provider: "foreign-child", Model: "m/one"}}},
		{Name: "foreign-owner", Workspace: b, Targets: []config.AliasTarget{local}},
		{Name: "unowned", Targets: []config.AliasTarget{local}},
		{Name: "empty", Workspace: a},
	}
	for i := range aliases {
		aliases[i].Algorithm = config.AlgorithmRoundRobin
	}
	catalog := config.NewCatalog(providers, []config.Provider{{Name: "disabled", Workspace: a, Models: providers[0].Models}}, aliases)
	resolver := New(&config.Runtime{Catalog: catalog})
	providers[1].Workspace, aliases[1].Workspace = b, b
	aliases[1].Targets[0].Provider = "b"

	for _, tc := range []struct {
		name      string
		workspace identity.WorkspaceScope
		model     string
		allowed   bool
	}{
		{"system-direct", system, "system/m/one", true},
		{"system-alias", system, "alias/system", true},
		{"a-direct", a, "a/m/one", true},
		{"a-derived", a, "child/m/one", true},
		{"a-alias", a, "alias/a", true},
		{"b-direct", b, "b/m/one", true},
		{"b-alias", b, "alias/b", true},
		{"foreign-direct", a, "b/m/one", false},
		{"foreign-alias", a, "alias/b", false},
		{"system-direct-not-shared", a, "system/m/one", false},
		{"system-alias-not-shared", a, "alias/system", false},
		{"system-cannot-use-tenant", system, "a/m/one", false},
		{"empty-direct", empty, "a/m/one", false},
		{"empty-alias", empty, "alias/a", false},
		{"missing-scope-is-not-system", identity.WorkspaceScope{}, "system/m/one", false},
		{"missing-scope-is-not-a", identity.WorkspaceScope{}, "a/m/one", false},
		{"unowned-direct", a, "unowned/m/one", false},
		{"disabled", a, "disabled/m/one", false},
		{"foreign-base", a, "foreign-child/m/one", false},
		{"system-base", a, "system-child/m/one", false},
		{"missing-base", a, "missing-child/m/one", false},
		{"unowned-base", a, "unowned-child/m/one", false},
		{"chained-base", a, "chained-child/m/one", false},
		{"foreign-target", a, "alias/foreign-target", false},
		{"system-target", a, "alias/system-target", false},
		{"missing-target", a, "alias/missing-target", false},
		{"missing-provider", a, "alias/missing-provider", false},
		{"unowned-target", a, "alias/unowned-target", false},
		{"invalid-base-target", a, "alias/invalid-base", false},
		{"foreign-owner", a, "alias/foreign-owner", false},
		{"foreign-owner-target", b, "alias/foreign-owner", false},
		{"unowned-alias", a, "alias/unowned", false},
		{"empty-alias-pool", a, "alias/empty", false},
		{"missing-model", a, "a/missing", false},
		{"missing-alias", a, "alias/missing", false},
		{"unqualified", a, "m/one", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if visible := resolver.ModelVisibleInWorkspace(tc.model, tc.workspace); visible != tc.allowed {
				t.Fatalf("discovery visibility = %t, want %t", visible, tc.allowed)
			}
			resolved, err := resolver.ResolveInWorkspace(tc.model, tc.workspace)
			if !tc.allowed {
				var unknown ErrUnknownModel
				if !errors.As(err, &unknown) || resolved.Provider.Name != "" || resolved.Model.Name != "" ||
					resolved.Alias.Name != "" || resolved.Selector != nil {
					t.Fatalf("denied resolution returned inventory or a different error: %v", err)
				}
				return
			}
			if err != nil || resolved.Workspace != tc.workspace {
				t.Fatalf("resolution lost authenticated workspace: %v", err)
			}
			if resolved.Kind == KindDirect {
				p, m, ok := resolver.ModelInWorkspace(resolved.Provider.Name, resolved.Model.Name, tc.workspace)
				if !ok || p.Workspace != tc.workspace || m.Name != "m/one" {
					t.Fatal("target lookup did not retain the same scope")
				}
				if p, m, ok := resolver.ModelInWorkspace(resolved.Provider.Name, resolved.Model.Name, empty); ok || p.Name != "" || m.Name != "" {
					t.Fatal("target lookup disclosed foreign inventory")
				}
			}
		})
	}
	if _, err := resolver.Resolve("unowned/m/one"); err != nil {
		t.Fatalf("file-only unscoped resolution changed: %v", err)
	}
	var missing *Resolver
	if _, err := missing.ResolveInWorkspace("system/m/one", system); err == nil {
		t.Fatal("nil resolver authorized System")
	}
}

func TestWorkspaceOwnershipReloadSeparatesRoutingState(t *testing.T) {
	a, b := identity.InWorkspace(uuid.New()), identity.InWorkspace(uuid.New())
	ownedRuntime := func(algorithm config.Algorithm, workspace identity.WorkspaceScope) *config.Runtime {
		rt := buildAliasRT(algorithm, "first", "second")
		providers, aliases := rt.Catalog.Providers(), rt.Catalog.Aliases()
		for i := range providers {
			providers[i].Workspace = workspace
		}
		aliases[0].Workspace = workspace
		rt.Catalog = config.NewCatalog(providers, nil, aliases)
		return rt
	}
	for _, algorithm := range []config.Algorithm{config.AlgorithmRoundRobin, config.AlgorithmLeastConnections} {
		t.Run(string(algorithm), func(t *testing.T) {
			rt := ownedRuntime(algorithm, a)
			old := New(rt)
			admitted, err := old.ResolveInWorkspace("alias/chat_default", a)
			if err != nil {
				t.Fatal(err)
			}
			first, release := admitted.Selector.Acquire(nil)
			defer release()
			if first.Provider != "first" {
				t.Fatal("unexpected initial target")
			}
			fp := fingerprintFor(t, rt, "chat_default", "first", "m")
			old.Cooldowns().Observe(fp, time.Minute)
			unchanged := NewWithPrevious(rt, old)
			same, err := unchanged.ResolveInWorkspace("alias/chat_default", a)
			if err != nil || same.Selector != admitted.Selector {
				t.Fatal("unchanged ownership did not retain selector state")
			}
			if _, cooling := unchanged.Cooldowns().Remaining(fp); !cooling {
				t.Fatal("unchanged ownership did not retain cooldown")
			}

			movedRT := ownedRuntime(algorithm, b)
			moved := NewWithPrevious(movedRT, old)
			if _, err := moved.ResolveInWorkspace("alias/chat_default", a); err == nil {
				t.Fatal("old workspace retained new admission")
			}
			current, err := moved.ResolveInWorkspace("alias/chat_default", b)
			if err != nil || current.Selector == admitted.Selector {
				t.Fatal("new workspace inherited old selector state")
			}
			next, releaseNext := current.Selector.Acquire(nil)
			releaseNext()
			if next.Provider != "first" {
				t.Fatal("new workspace inherited a lease or round-robin cursor")
			}
			movedFP := fingerprintFor(t, movedRT, "chat_default", "first", "m")
			if movedFP == fp {
				t.Fatal("target ownership was omitted from the cooldown fingerprint")
			}
			if _, cooling := moved.Cooldowns().Remaining(movedFP); cooling {
				t.Fatal("new workspace inherited old cooldown")
			}
			if _, cooling := old.Cooldowns().Remaining(fp); !cooling || !old.ModelVisibleInWorkspace("alias/chat_default", a) {
				t.Fatal("reload invalidated the admitted snapshot")
			}

			providers := rt.Catalog.Providers()
			providers[1].Workspace = b
			invalid := NewWithPrevious(&config.Runtime{Catalog: config.NewCatalog(providers, nil, rt.Catalog.Aliases())}, old)
			if len(invalid.Cooldowns().Active()) != 0 || invalid.ModelVisibleInWorkspace("alias/chat_default", a) {
				t.Fatal("an invalid mixed-workspace pool retained routing state")
			}
		})
	}
}
