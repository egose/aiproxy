package dbmerge

import (
	"context"
	"encoding/json"
	"net/url"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/egose/aiproxy/internal/config"
	"github.com/egose/aiproxy/internal/modelresolver"
	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func openReasoningMergeStore(t *testing.T) *store.Store {
	t.Helper()
	dsn := testDBURL(t)
	ctx := context.Background()
	admin, err := store.Open(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	schema := "reasoning_merge_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.DB.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.DB.ExecContext(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
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
	return st
}

func createReasoningProvider(t *testing.T, st *store.Store, row store.DBProvider, models []store.DBProviderModel) store.DBProvider {
	t.Helper()
	if err := st.CreateProviderAggregate(context.Background(), &row, models); err != nil {
		t.Fatal(err)
	}
	return row
}

func mergedReasoningCatalog(t *testing.T, st *store.Store, rt *config.Runtime) config.Catalog {
	t.Helper()
	merged, err := MergeCatalog(context.Background(), st, rt)
	if err != nil {
		t.Fatal(err)
	}
	return config.NewCatalog(merged.Providers, merged.DisabledProviders, merged.Aliases)
}

func requireReasoningProvider(t *testing.T, catalog config.Catalog, name string) config.Provider {
	t.Helper()
	p, ok := catalog.Provider(name)
	if !ok {
		t.Fatalf("provider %q missing from served catalog", name)
	}
	return p
}

func assertStoredReasoningIntent(t *testing.T, raw json.RawMessage, mode string) {
	t.Helper()
	policy, err := config.DecodeEncryptedReasoningJSON(raw, "stored fixture")
	if err != nil || !reflect.DeepEqual(config.DeclaredEncryptedReasoning(policy), config.DeclaredEncryptedReasoning(mergeReasoningPolicy(t, mode))) {
		t.Fatalf("stored owner-local intent changed: %s %v", raw, err)
	}
}

func TestMergeCatalogEncryptedReasoningMaterialization(t *testing.T) {
	st := openReasoningMergeStore(t)
	base := createReasoningProvider(t, st, store.DBProvider{Name: "managed-base", Type: "opencode-zen", Enabled: true, EncryptedReasoning: storedReasoningJSON("retry")}, []store.DBProviderModel{
		{Name: "m", DisplayName: "Managed M", Protocol: "chat", Capabilities: []string{"chat"}, Pricing: json.RawMessage(`{"input_per_million":2}`), EncryptedReasoning: storedReasoningJSON("strip")},
		{Name: "unconfigured", Protocol: "chat"},
	})
	child := createReasoningProvider(t, st, store.DBProvider{Name: "managed-child", Type: base.Type, Extends: base.Name, Enabled: true, EncryptedReasoning: storedReasoningJSON("reset")}, []store.DBProviderModel{
		{Name: "m", EncryptedReasoning: storedReasoningJSON("reset")},
		{Name: "legacy-ghost", UpstreamName: "ignored", Capabilities: []string{"unknown"}, Pricing: json.RawMessage(`12`)},
	})
	sibling := createReasoningProvider(t, st, store.DBProvider{Name: "managed-sibling", Type: base.Type, Extends: base.Name, Enabled: true}, nil)
	disabled := createReasoningProvider(t, st, store.DBProvider{Name: "managed-disabled", Type: base.Type, Extends: base.Name, Enabled: false, EncryptedReasoning: storedReasoningJSON("strip")}, nil)
	staticBase := reasoningBaseFixture(t, "strip", "retry")
	staticBase.Name, staticBase.Type, staticBase.APIKey = "static-base", config.ProviderTypeOpenCodeZen, ""
	delete(staticBase.ModelByName, "indexed")
	for i := range staticBase.Models {
		staticBase.Models[i].Protocol = config.ModelProtocolChat
		staticBase.Models[i].Capabilities = []config.Capability{config.CapabilityChat}
		staticBase.ModelByName[staticBase.Models[i].Name] = cloneModel(staticBase.Models[i])
	}
	staticChild := createReasoningProvider(t, st, store.DBProvider{Name: "static-child", Type: string(staticBase.Type), Extends: staticBase.Name, Enabled: true}, []store.DBProviderModel{{Name: "m", EncryptedReasoning: storedReasoningJSON("null")}})
	alias := store.DBAlias{Name: "inherited-inventory", Algorithm: "round_robin", EncryptedReasoning: storedReasoningJSON("reset")}
	if err := st.CreateAlias(context.Background(), &alias, []store.DBAliasTarget{{Provider: child.Name, Model: "unconfigured"}}); err != nil {
		t.Fatal(err)
	}
	rt := &config.Runtime{
		Catalog:            config.NewCatalog([]config.Provider{staticBase}, nil, nil),
		EncryptedReasoning: mergeReasoningPolicy(t, "custom"),
		UserAgent:          "root-agent", UpstreamHeaderTimeout: 7 * time.Second,
	}
	catalog := mergedReasoningCatalog(t, st, rt)
	p := requireReasoningProvider(t, catalog, child.Name)
	if p.Extends != base.Name || len(p.Models) != 2 || p.ModelByName["legacy-ghost"].Name != "" || p.UserAgent != rt.UserAgent || p.UpstreamHeaderTimeout != rt.UpstreamHeaderTimeout {
		t.Fatal("managed derivative inventory/provenance/defaults were not materialized consistently")
	}
	if p.EncryptedReasoningInherited || p.ModelByName["m"].EncryptedReasoningInherited || p.ModelByName["m"].DisplayName != "Managed M" || p.ModelByName["m"].Pricing.InputPerMillion != 2 {
		t.Fatal("local reset changed inherited definitions or was marked inherited")
	}
	assertMergedReasoning(t, config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil), "reset", config.EncryptedReasoningSourceModel, false)
	assertMergedReasoning(t, config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["unconfigured"], nil), "reset", config.EncryptedReasoningSourceProvider, false)
	p = requireReasoningProvider(t, catalog, sibling.Name)
	if !p.EncryptedReasoningInherited || !p.ModelByName["m"].EncryptedReasoningInherited {
		t.Fatal("managed base declarations lost inherited provenance")
	}
	assertMergedReasoning(t, config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil), "strip", config.EncryptedReasoningSourceModel, true)
	p = requireReasoningProvider(t, catalog, staticChild.Name)
	if p.Extends != staticBase.Name || !p.EncryptedReasoningInherited || !p.ModelByName["m"].EncryptedReasoningInherited {
		t.Fatal("static base declarations lost inherited provenance")
	}
	assertMergedReasoning(t, config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil), "retry", config.EncryptedReasoningSourceModel, true)
	if rows := catalog.DisabledProviders(); len(rows) != 1 || rows[0].Name != disabled.Name || rows[0].Enabled || len(rows[0].Models) != 2 || rows[0].EncryptedReasoningInherited || !rows[0].ModelByName["m"].EncryptedReasoningInherited {
		t.Fatal("disabled managed derivative lost inventory or policy declarations")
	}
	a, ok := catalog.Alias(alias.Name)
	if !ok || len(a.Targets) != 1 || a.Targets[0].Model != "unconfigured" || a.EncryptedReasoning == nil || !a.EncryptedReasoning.Passthrough {
		t.Fatal("alias saw the sparse overlay rows instead of served inventory, or lost safe-reset semantics")
	}
	if err := config.ValidateDynamicAlias(a, catalog); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	saved, err := st.GetProvider(ctx, child.Name)
	if err != nil {
		t.Fatal(err)
	}
	assertStoredReasoningIntent(t, saved.EncryptedReasoning, "reset")
	local, err := st.ListProviderModels(ctx, child.ID)
	if err != nil || len(local) != 2 {
		t.Fatal("materialization deleted legacy rows or persisted inherited definitions")
	}
	for _, row := range local {
		if row.Name == "m" {
			assertStoredReasoningIntent(t, row.EncryptedReasoning, "reset")
			if row.UpstreamName != "" || row.Protocol != "" || len(row.Capabilities) != 0 || row.Pricing != nil {
				t.Fatal("stored sparse overlay was default-filled")
			}
		} else if row.Name != "legacy-ghost" || row.EncryptedReasoning != nil {
			t.Fatal("legacy NULL-policy row changed")
		}
	}
	if err := st.ReplaceProviderModels(ctx, child.ID, nil); err != nil {
		t.Fatal(err)
	}
	catalog = mergedReasoningCatalog(t, st, rt)
	p = requireReasoningProvider(t, catalog, child.Name)
	if len(p.Models) != 2 || !p.ModelByName["m"].EncryptedReasoningInherited {
		t.Fatal("removing local overlays removed inherited model definitions/policies")
	}
	assertMergedReasoning(t, config.ResolveEncryptedReasoning(rt.EncryptedReasoning, p, p.ModelByName["m"], nil), "strip", config.EncryptedReasoningSourceModel, true)
}

func TestMergeCatalogEncryptedReasoningRootFallbackSnapshots(t *testing.T) {
	st := openReasoningMergeStore(t)
	base := createReasoningProvider(t, st, store.DBProvider{Name: "root-base", Type: "opencode-zen", Enabled: true}, []store.DBProviderModel{{Name: "m", Protocol: "chat"}})
	child := createReasoningProvider(t, st, store.DBProvider{Name: "root-child", Type: base.Type, Extends: base.Name, Enabled: true}, nil)
	rt := &config.Runtime{Catalog: config.NewCatalog(nil, nil, nil), EncryptedReasoning: mergeReasoningPolicy(t, "retry")}
	oldRuntime := *rt
	oldRuntime.Catalog = mergedReasoningCatalog(t, st, rt)
	oldResolver := modelresolver.New(&oldRuntime)
	rt.EncryptedReasoning = mergeReasoningPolicy(t, "strip")
	newRuntime := *rt
	newRuntime.Catalog = mergedReasoningCatalog(t, st, rt)
	newResolver := modelresolver.New(&newRuntime)
	for _, name := range []string{base.Name, child.Name} {
		oldProvider, oldModel, ok := oldResolver.Model(name, "m")
		if !ok {
			t.Fatal("old admitted inventory is missing")
		}
		newProvider, newModel, ok := newResolver.Model(name, "m")
		if !ok || oldProvider.EncryptedReasoning != nil || newProvider.EncryptedReasoning != nil || oldModel.EncryptedReasoning != nil || newModel.EncryptedReasoning != nil || newProvider.EncryptedReasoningInherited || newModel.EncryptedReasoningInherited {
			t.Fatal("root fallback was copied into a managed declaration")
		}
		assertMergedReasoning(t, oldResolver.EncryptedReasoning(oldProvider, oldModel, nil), "retry", config.EncryptedReasoningSourceRoot, false)
		assertMergedReasoning(t, newResolver.EncryptedReasoning(newProvider, newModel, nil), "strip", config.EncryptedReasoningSourceRoot, false)
		saved, err := st.GetProvider(context.Background(), name)
		if err != nil || saved.EncryptedReasoning != nil {
			t.Fatal("root change rewrote provider policy intent")
		}
		models, err := st.ListProviderModels(context.Background(), saved.ID)
		if err != nil || (name == child.Name && len(models) != 0) {
			t.Fatal("root change flattened inherited definitions")
		}
		for _, model := range models {
			if model.EncryptedReasoning != nil {
				t.Fatal("root change rewrote model policy intent")
			}
		}
	}
}

func TestMergeCatalogEncryptedReasoningAliasStoredContract(t *testing.T) {
	st := openReasoningMergeStore(t)
	base := reasoningBaseFixture(t, "strip", "strip")
	rt := &config.Runtime{Catalog: config.NewCatalog([]config.Provider{base}, nil, nil), EncryptedReasoning: mergeReasoningPolicy(t, "retry")}
	for _, tc := range []struct {
		name, raw, mode string
	}{
		{"null", `null`, "absent"},
		{"reset", `{}`, "reset"},
		{"false", `{"passthrough":false}`, "strip"},
		{"full", `{"passthrough":true,"on_caller_mismatch":"strip_and_retry","match_messages":[" Vendor Marker "]}`, "custom"},
		{"legacy-empty-patterns", `{"passthrough":false,"on_caller_mismatch":"fail","match_messages":[]}`, "strip"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			row := store.DBAlias{Name: "alias-" + tc.name, Algorithm: "round_robin", EncryptedReasoning: json.RawMessage(tc.raw)}
			if err := st.CreateAlias(context.Background(), &row, []store.DBAliasTarget{{Provider: base.Name, Model: "m"}}); err != nil {
				t.Fatal(err)
			}
			if tc.name == "null" {
				if _, err := st.DB.NewRaw(`UPDATE db_aliases SET encrypted_reasoning = 'null'::jsonb WHERE id = ?`, row.ID).Exec(context.Background()); err != nil {
					t.Fatal(err)
				}
			}
			catalog := mergedReasoningCatalog(t, st, rt)
			a, ok := catalog.Alias(row.Name)
			if !ok {
				t.Fatal("stored alias missing")
			}
			if tc.mode == "absent" {
				if a.EncryptedReasoning != nil {
					t.Fatal("stored JSON null became an alias override")
				}
				return
			}
			if a.EncryptedReasoning == nil || a.EncryptedReasoning.Declaration == nil {
				t.Fatal("nonnull alias declaration lost explicit/sticky semantics")
			}
			assertMergedReasoning(t, config.ResolveEncryptedReasoning(rt.EncryptedReasoning, base, base.Models[0], &a), tc.mode, config.EncryptedReasoningSourceAlias, false)
			encoded, err := config.EncodeEncryptedReasoningJSON(a.EncryptedReasoning, `alias "stored"`)
			if err != nil {
				t.Fatal(err)
			}
			var got, want any
			if json.Unmarshal(encoded, &got) != nil || json.Unmarshal([]byte(tc.raw), &want) != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("stored full/sparse alias members were default-filled or discarded")
			}
		})
	}
}

func TestMergeCatalogEncryptedReasoningRejectedCandidatePreservesRuntime(t *testing.T) {
	st := openReasoningMergeStore(t)
	ctx := context.Background()
	base := createReasoningProvider(t, st, store.DBProvider{Name: "managed-base", Type: "opencode-zen", Enabled: true, EncryptedReasoning: storedReasoningJSON("retry")}, []store.DBProviderModel{
		{Name: "m", Protocol: "chat", EncryptedReasoning: storedReasoningJSON("strip")},
		{Name: "unconfigured", Protocol: "chat"},
	})
	child := createReasoningProvider(t, st, store.DBProvider{Name: "managed-child", Type: base.Type, Extends: base.Name, Enabled: true}, []store.DBProviderModel{{Name: "m", EncryptedReasoning: storedReasoningJSON("reset")}})
	a := store.DBAlias{Name: "inventory", Algorithm: "round_robin", EncryptedReasoning: storedReasoningJSON("custom")}
	if err := st.CreateAlias(ctx, &a, []store.DBAliasTarget{{Provider: child.Name, Model: "unconfigured"}}); err != nil {
		t.Fatal(err)
	}
	baseModels, err := st.ListProviderModels(ctx, base.ID)
	if err != nil {
		t.Fatal(err)
	}
	overlays, err := st.ListProviderModels(ctx, child.ID)
	if err != nil {
		t.Fatal(err)
	}
	var baseModel store.DBProviderModel
	for _, model := range baseModels {
		if model.Name == "m" {
			baseModel = model
		}
	}
	rt := &config.Runtime{Catalog: config.NewCatalog(nil, nil, nil), EncryptedReasoning: mergeReasoningPolicy(t, "custom")}
	rt.Catalog = mergedReasoningCatalog(t, st, rt)
	admitted := modelresolver.New(rt)
	beforeProviders, beforeAliases := rt.Catalog.Providers(), rt.Catalog.Aliases()
	beforeRoot := config.CloneEncryptedReasoning(rt.EncryptedReasoning)
	policyMutation := func(table string, id uuid.UUID, raw string, previous json.RawMessage) func(*testing.T) {
		return func(t *testing.T) {
			t.Helper()
			if _, err := st.DB.NewRaw(`UPDATE `+table+` SET encrypted_reasoning = ?::jsonb WHERE id = ?`, raw, id).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := st.DB.NewRaw(`UPDATE `+table+` SET encrypted_reasoning = ?::jsonb WHERE id = ?`, previous, id).Exec(ctx); err != nil {
					t.Error(err)
				}
			})
		}
	}
	for _, tc := range []struct {
		name, owner string
		mutate      func(*testing.T)
	}{
		{"provider-unknown", `provider "managed-base"`, policyMutation("db_providers", base.ID, `{"unknown":true}`, base.EncryptedReasoning)},
		{"provider-case-duplicate", `provider "managed-base"`, policyMutation("db_providers", base.ID, `{"passthrough":false,"Passthrough":true}`, base.EncryptedReasoning)},
		{"model-type", `provider "managed-base": model "m"`, policyMutation("db_provider_models", baseModel.ID, `{"passthrough":0}`, baseModel.EncryptedReasoning)},
		{"overlay-patterns", `provider "managed-child": model "m"`, policyMutation("db_provider_models", overlays[0].ID, `{"match_messages":["ABC","abc"]}`, overlays[0].EncryptedReasoning)},
		{"alias-unknown", `alias "inventory"`, policyMutation("db_aliases", a.ID, `{"unknown":true}`, a.EncryptedReasoning)},
		{"alias-case-duplicate", `alias "inventory"`, policyMutation("db_aliases", a.ID, `{"match_messages":[],"MATCH_MESSAGES":["marker"]}`, a.EncryptedReasoning)},
		{"overlay-redefinition", `provider "managed-child": model "m"`, func(t *testing.T) {
			if _, err := st.DB.NewRaw(`UPDATE db_provider_models SET display_name = 'redefined' WHERE id = ?`, overlays[0].ID).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := st.DB.NewRaw(`UPDATE db_provider_models SET display_name = '' WHERE id = ?`, overlays[0].ID).Exec(ctx); err != nil {
					t.Error(err)
				}
			})
		}},
		{"overlay-dangling", `provider "managed-child": model "m"`, func(t *testing.T) {
			if _, err := st.DB.NewDelete().Model(&baseModel).WherePK().Exec(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := st.DB.NewInsert().Model(&baseModel).Exec(ctx); err != nil {
					t.Error(err)
				}
			})
		}},
		{"disabled-base", `provider "managed-child"`, func(t *testing.T) {
			if _, err := st.DB.NewRaw(`UPDATE db_providers SET enabled = false WHERE id = ?`, base.ID).Exec(ctx); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := st.DB.NewRaw(`UPDATE db_providers SET enabled = true WHERE id = ?`, base.ID).Exec(ctx); err != nil {
					t.Error(err)
				}
			})
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.mutate(t)
			if _, err := MergeCatalog(ctx, st, rt); err == nil || !strings.Contains(err.Error(), tc.owner) {
				t.Fatalf("invalid stored candidate activated or missed scope: %v", err)
			}
			if !reflect.DeepEqual(rt.Catalog.Providers(), beforeProviders) || !reflect.DeepEqual(rt.Catalog.Aliases(), beforeAliases) || !reflect.DeepEqual(rt.EncryptedReasoning, beforeRoot) {
				t.Fatal("rejected database candidate mutated the old runtime")
			}
			p, m, ok := admitted.Model(child.Name, "unconfigured")
			if !ok {
				t.Fatal("rejected database candidate changed admitted inventory")
			}
			assertMergedReasoning(t, admitted.EncryptedReasoning(p, m, nil), "retry", config.EncryptedReasoningSourceProvider, true)
		})
	}
	mergedReasoningCatalog(t, st, rt)
}

func TestMergeCatalogEncryptedReasoningRejectsBaseChains(t *testing.T) {
	for _, kind := range []string{"managed", "shadowed-managed", "static-derived"} {
		t.Run(kind, func(t *testing.T) {
			st := openReasoningMergeStore(t)
			root := createReasoningProvider(t, st, store.DBProvider{Name: "root", Type: "opencode-zen", Enabled: true}, []store.DBProviderModel{{Name: "m", Protocol: "chat"}})
			static := []config.Provider{}
			if kind == "static-derived" {
				middle := config.Provider{Name: "z-middle", Type: config.ProviderTypeOpenCodeZen, Extends: root.Name, Enabled: true, Models: []config.Model{{Name: "m", UpstreamName: "m", Protocol: config.ModelProtocolChat}}}
				static = append(static, middle)
			} else {
				createReasoningProvider(t, st, store.DBProvider{Name: "z-middle", Type: root.Type, Extends: root.Name, Enabled: true}, nil)
				if kind == "shadowed-managed" {
					static = append(static, config.Provider{Name: "z-middle", Type: config.ProviderTypeOpenCodeZen, Enabled: true, Models: []config.Model{{Name: "m", UpstreamName: "m", Protocol: config.ModelProtocolChat}}})
				}
			}
			createReasoningProvider(t, st, store.DBProvider{Name: "a-child", Type: root.Type, Extends: "z-middle", Enabled: true, EncryptedReasoning: storedReasoningJSON("reset")}, nil)
			rt := &config.Runtime{Catalog: config.NewCatalog(static, nil, nil)}
			if _, err := MergeCatalog(context.Background(), st, rt); err == nil || !strings.Contains(err.Error(), "inheritance chains") {
				t.Fatalf("base chain passed based on row order or flattened/static shadow provenance: %v", err)
			}
		})
	}
}
