package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/store"
	"github.com/google/uuid"
)

func dependencyRuntimeChat(t *testing.T, a *App, model, upstream string) {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", strings.NewReader(fmt.Sprintf(`{"model":%q,"messages":[]}`, model)))
	r.Header.Set("Authorization", "Bearer static-fixture")
	w := httptest.NewRecorder()
	a.Server.Handler.ServeHTTP(w, r)
	if w.Code != 200 || !strings.Contains(w.Body.String(), fmt.Sprintf(`"model":%q`, upstream)) {
		t.Fatalf("runtime route %s = %d %s, want %s", model, w.Code, w.Body.String(), upstream)
	}
}

func dependencyRuntimeRestart(t *testing.T, path string, check func(*App)) {
	t.Helper()
	restarted, err := Build(context.Background(), BuildOptions{ConfigPath: path, LogOutput: io.Discard})
	if err != nil {
		t.Fatalf("saved graph cannot restart: %v", err)
	}
	defer restarted.Close()
	check(restarted)
}

func TestAppCatalogDependencyRejectionAndCleanup(t *testing.T) {
	a, path, _, request := catalogRuntimeFixture(t)
	ctx := context.Background()
	local, _ := a.Config.Catalog.Provider("local")
	save := func(method, endpoint, body string, status int) {
		t.Helper()
		w := request(method, "/_internal/admin/"+endpoint, body)
		if w.Code != status {
			t.Fatalf("%s %s = %d %s", method, endpoint, w.Code, w.Body.String())
		}
	}
	save("POST", "providers", fmt.Sprintf(`{"name":"base","type":"openai-compatible","base_url":%q,"api_key":"fixture","models":[{"name":"m","upstream_name":"old","capabilities":["chat"]},{"name":"keep","upstream_name":"new","capabilities":["chat"]}]}`, local.BaseURL), 201)
	save("POST", "providers", fmt.Sprintf(`{"name":"peer","type":"openai-compatible","base_url":%q,"api_key":"fixture","models":[{"name":"keep","upstream_name":"old","capabilities":["chat"]}]}`, local.BaseURL), 201)
	save("POST", "providers", `{"name":"child","type":"openai-compatible","extends":"base","api_key":"fixture"}`, 201)
	save("POST", "providers", `{"name":"disabled","type":"openai-compatible","extends":"base","enabled":false,"models":[{"name":"m","encrypted_reasoning":{}}]}`, 201)
	save("POST", "aliases", `{"name":"direct","targets":[{"provider":"base","model":"m"},{"provider":"peer","model":"keep"}]}`, 201)
	save("POST", "aliases", `{"name":"inherited","targets":[{"provider":"child","model":"m"},{"provider":"peer","model":"keep"}]}`, 201)
	base, err := a.adminStore.GetProvider(ctx, "base")
	if err != nil {
		t.Fatal(err)
	}
	models, err := a.adminStore.ListProviderModels(ctx, base.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct{ method, resource, body string }{
		{"DELETE", "base", ""},
		{"PUT", "base", `{"enabled":false}`},
		{"PUT", "base", `{"models":[{"name":"keep","upstream_name":"new","capabilities":["chat"]}]}`},
		{"PUT", "base", `{"extends":"peer","models":[]}`},
		{"PUT", "base", `{"type":"openai"}`},
		{"PUT", "base", `{"models":[{"name":"m","capabilities":["embeddings"]},{"name":"keep","capabilities":["chat"]}]}`},
		{"PUT", "child", `{"extends":"peer","models":[]}`},
		{"PUT", "child", fmt.Sprintf(`{"extends":"","base_url":%q,"models":[{"name":"keep"}]}`, local.BaseURL)},
	} {
		t.Run(test.method+"/"+test.resource+"/"+test.body, func(t *testing.T) {
			before := a.Config
			reloads := 0
			deps := a.handler.SnapshotDependencies()
			deps.RequestReload = func() error { reloads++; return a.Reload() }
			a.handler.UpdateDependencies(deps)
			w := request(test.method, "/_internal/admin/providers/"+test.resource, test.body)
			if w.Code != 400 || w.Body.String() != store.ErrCatalogDependency.Error()+"\n" || w.Header().Get("X-Aiproxy-Catalog-Saved") != "" {
				t.Fatalf("dependency rejection = %d %s", w.Code, w.Body.String())
			}
			after, err := a.adminStore.GetProvider(ctx, base.Name)
			if err != nil {
				t.Fatal(err)
			}
			afterModels, err := a.adminStore.ListProviderModels(ctx, base.ID)
			if err != nil || !reflect.DeepEqual(base, after) || !reflect.DeepEqual(models, afterModels) || reloads != 0 || a.Config != before {
				t.Fatal("rejected edit changed durable state, requested activation, or replaced runtime")
			}
			dependencyRuntimeChat(t, a, "base/m", "old")
			dependencyRuntimeChat(t, a, "alias/direct", "old")
			dependencyRuntimeChat(t, a, "alias/inherited", "old")
			if err := a.Reload(); err != nil {
				t.Fatal(err)
			}
			dependencyRuntimeRestart(t, path, func(restarted *App) {
				dependencyRuntimeChat(t, restarted, "alias/direct", "old")
				dependencyRuntimeChat(t, restarted, "alias/inherited", "old")
			})
		})
	}
	save("PUT", "aliases/direct", `{"targets":[{"provider":"peer","model":"keep"}]}`, 200)
	save("PUT", "aliases/inherited", `{"targets":[{"provider":"peer","model":"keep"}]}`, 200)
	save("DELETE", "providers/child", "", 200)
	if w := request("DELETE", "/_internal/admin/providers/base", ""); w.Code != 400 || w.Body.String() != store.ErrCatalogDependency.Error()+"\n" {
		t.Fatal("disabled derivative no longer requires its base")
	}
	save("PUT", "providers/disabled", `{"extends":"peer","models":[]}`, 200)
	save("DELETE", "providers/base", "", 200)
	save("PUT", "providers/peer", `{"models":[{"name":"keep","upstream_name":"new","capabilities":["chat"]}]}`, 200)
	dependencyRuntimeChat(t, a, "alias/direct", "new")
	dependencyRuntimeChat(t, a, "alias/inherited", "new")
	dependencyRuntimeRestart(t, path, func(restarted *App) {
		dependencyRuntimeChat(t, restarted, "alias/direct", "new")
		dependencyRuntimeChat(t, restarted, "alias/inherited", "new")
	})
	save("DELETE", "aliases/direct", "", 200)
	save("DELETE", "aliases/inherited", "", 200)
	save("DELETE", "providers/disabled", "", 200)
	save("PUT", "providers/peer", `{"enabled":false}`, 200)
	dependencyRuntimeRestart(t, path, func(restarted *App) {
		if _, ok := restarted.Config.Catalog.Provider("peer"); ok {
			t.Fatal("valid disable did not survive restart")
		}
	})
	save("DELETE", "providers/peer", "", 200)
	dependencyRuntimeRestart(t, path, func(restarted *App) {
		if len(restarted.Config.Catalog.Aliases()) != 0 || len(restarted.Config.Catalog.Providers()) != 1 {
			t.Fatal("valid cleanup left dependent resources behind")
		}
	})
}

func TestAppCatalogDependencyValidCommitActivationFailure(t *testing.T) {
	a, path, cfg, request := catalogRuntimeFixture(t)
	local, _ := a.Config.Catalog.Provider("local")
	w := request("POST", "/_internal/admin/providers",
		fmt.Sprintf(`{"name":"base","type":"openai-compatible","base_url":%q,"api_key":"fixture","models":[{"name":"m","upstream_name":"old"}]}`, local.BaseURL))
	if w.Code != 201 {
		t.Fatalf("setup = %d %s", w.Code, w.Body.String())
	}
	w = request("POST", "/_internal/admin/aliases", `{"name":"route","targets":[{"provider":"base","model":"m"}]}`)
	if w.Code != 201 {
		t.Fatalf("alias setup = %d %s", w.Code, w.Body.String())
	}
	ctx := context.Background()
	operational := strings.Replace(cfg, `address = "127.0.0.1:0"`, `address = "127.0.0.1:1"`, 1)
	rewriteConfigFile(t, path, operational)
	before := a.Config
	w = request("PUT", "/_internal/admin/providers/base", `{"models":[{"name":"m","upstream_name":"new"}]}`)
	if w.Code != 500 || w.Body.String() != "saved but activation failed\n" || w.Header().Get("X-Aiproxy-Catalog-Saved") != "true" || a.Config != before {
		t.Fatalf("valid edit operational failure = %d %s", w.Code, w.Body.String())
	}
	base, err := a.adminStore.GetProvider(ctx, "base")
	if err != nil {
		t.Fatal(err)
	}
	models, err := a.adminStore.ListProviderModels(ctx, base.ID)
	if err != nil || len(models) != 1 || models[0].UpstreamName != "new" {
		t.Fatal("valid dependency-preserving edit was not fully saved")
	}
	dependencyRuntimeChat(t, a, "alias/route", "old")
	rewriteConfigFile(t, path, cfg)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	dependencyRuntimeChat(t, a, "alias/route", "new")
	w = request("DELETE", "/_internal/admin/aliases/route", "")
	if w.Code != 200 {
		t.Fatal(w.Body.String())
	}
	rewriteConfigFile(t, path, operational)
	before = a.Config
	w = request("DELETE", "/_internal/admin/providers/base", "")
	if w.Code != 500 || w.Body.String() != "saved but activation failed\n" || w.Header().Get("X-Aiproxy-Catalog-Saved") != "true" || a.Config != before {
		t.Fatalf("valid deletion operational failure = %d %s", w.Code, w.Body.String())
	}
	if _, err := a.adminStore.GetProvider(ctx, "base"); err == nil {
		t.Fatal("valid deletion was rolled back after activation failure")
	}
	dependencyRuntimeChat(t, a, "base/m", "new")
	rewriteConfigFile(t, path, cfg)
	if err := a.Reload(); err != nil {
		t.Fatal(err)
	}
	dependencyRuntimeRestart(t, path, func(restarted *App) {
		if _, ok := restarted.Config.Catalog.Provider("base"); ok {
			t.Fatal("saved valid deletion did not survive restart")
		}
	})
}

func TestAppCatalogDependencyUnrelatedCorruptionRepair(t *testing.T) {
	a, path, _, request := catalogRuntimeFixture(t)
	ctx := context.Background()
	local, _ := a.Config.Catalog.Provider("local")
	w := request("POST", "/_internal/admin/providers",
		fmt.Sprintf(`{"name":"base","type":"openai-compatible","base_url":%q,"api_key":"fixture","models":[{"name":"m","upstream_name":"old"}]}`, local.BaseURL))
	if w.Code != 201 {
		t.Fatal(w.Body.String())
	}
	bad := store.DBProvider{ID: uuid.New(), Name: "corrupt", Type: "openai", Extends: "absent", WorkspaceID: store.SystemWorkspaceID}
	if _, err := a.adminStore.DB.NewInsert().Model(&bad).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := a.adminStore.DB.ExecContext(ctx, `UPDATE db_providers SET forward_headers = '{}'::jsonb WHERE id = ?`, bad.ID); err != nil {
		t.Fatal(err)
	}
	broken := store.DBAlias{ID: uuid.New(), Name: "broken", Algorithm: "round_robin", WorkspaceID: store.SystemWorkspaceID}
	if _, err := a.adminStore.DB.NewInsert().Model(&broken).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	target := store.DBAliasTarget{ID: uuid.New(), AliasID: broken.ID, Provider: "absent", Model: "m"}
	if _, err := a.adminStore.DB.NewInsert().Model(&target).Exec(ctx); err != nil {
		t.Fatal(err)
	}
	before := a.Config
	w = request("PUT", "/_internal/admin/aliases/broken", `{"targets":[{"provider":"base","model":"m"}]}`)
	if w.Code != 500 || w.Body.String() != "saved but activation failed\n" || a.Config != before {
		t.Fatalf("unrelated corrupt provider blocked alias repair before save: %d %s", w.Code, w.Body.String())
	}
	targets, err := a.adminStore.ListAliasTargets(ctx, broken.ID)
	if err != nil || len(targets) != 1 || targets[0].Provider != "base" {
		t.Fatal("alias repair was not durable")
	}
	w = request("PUT", "/_internal/admin/providers/base", `{"models":[{"name":"m","upstream_name":"new"}]}`)
	if w.Code != 500 || w.Body.String() != "saved but activation failed\n" || w.Header().Get("X-Aiproxy-Catalog-Saved") != "true" || a.Config != before {
		t.Fatalf("unrelated corrupt graph blocked valid provider save: %d %s", w.Code, w.Body.String())
	}
	dependencyRuntimeChat(t, a, "base/m", "old")
	w = request("DELETE", "/_internal/admin/providers/corrupt", "")
	if w.Code != 200 {
		t.Fatalf("unreferenced corrupt provider could not be removed: %d %s", w.Code, w.Body.String())
	}
	dependencyRuntimeChat(t, a, "alias/broken", "new")
	dependencyRuntimeRestart(t, path, func(restarted *App) {
		dependencyRuntimeChat(t, restarted, "alias/broken", "new")
	})
}
