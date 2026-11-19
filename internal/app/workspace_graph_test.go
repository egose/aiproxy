package app

import (
	"context"
	"fmt"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/identity"
	"github.com/egose/aiproxy/internal/store"
)

func TestAppStaticManagedCollisionRejectsReloadAndStartup(t *testing.T) {
	for _, managedEnabled := range []bool{true, false} {
		for _, staticEnabled := range []bool{true, false} {
			t.Run(fmt.Sprintf("managed=%t/static=%t", managedEnabled, staticEnabled), func(t *testing.T) {
				a, path, cfg, _ := catalogRuntimeFixture(t)
				ctx := context.Background()
				workspace := store.Workspace{Name: "tenant"}
				if err := a.adminStore.CreateWorkspace(ctx, &workspace); err != nil {
					t.Fatal(err)
				}
				local, _ := a.Config.Catalog.Provider("local")
				cfg = strings.Replace(cfg, "multi_tenancy { enabled = true }", "multi_tenancy {\n  enabled = true\n  allowed_upstream_origins = [\"https://managed.example.com\"]\n}", 1)
				rewriteConfigFile(t, path, cfg)
				encrypted, err := store.EncryptSecret([]byte("managed-fixture"))
				if err != nil {
					t.Fatal(err)
				}
				row := store.DBProvider{Name: "managed", WorkspaceID: workspace.ID, Type: "openai-compatible",
					BaseURL: "https://managed.example.com", Enabled: managedEnabled, APIKeyEncrypted: encrypted}
				if err := a.adminStore.CreateProviderAggregate(ctx, &row, []store.DBProviderModel{{Name: "m"}}); err != nil {
					t.Fatal(err)
				}
				if err := a.Reload(); err != nil {
					t.Fatal(err)
				}
				old := a.Config
				if managedEnabled && !old.Catalog.ModelVisibleInWorkspace("managed/m", identity.InWorkspace(workspace.ID)) {
					t.Fatal("managed graph lost its original workspace")
				}
				rewriteConfigFile(t, path, cfg+fmt.Sprintf(`
provider "openai-compatible" "managed" {
  base_url = %q
  api_key = "static-fixture"
  enabled = %t
  model "m" {}
}
`, local.BaseURL, staticEnabled))
				if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "already defined in static config") {
					t.Fatalf("collision reload admission = %v", err)
				}
				if a.Config != old {
					t.Fatal("collision replaced the active runtime")
				}
				candidate, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard})
				if candidate != nil {
					_ = candidate.Close()
				}
				if err == nil || !strings.Contains(err.Error(), "already defined in static config") {
					t.Fatalf("collision startup admission = %v", err)
				}
				after, err := a.adminStore.GetProvider(ctx, row.Name)
				if err != nil || !reflect.DeepEqual(after, row) {
					t.Fatal("collision rewrote or reassigned the saved managed provider")
				}
				rewriteConfigFile(t, path, cfg)
				if err := a.Reload(); err != nil {
					t.Fatalf("collision repair did not recover: %v", err)
				}
			})
		}
	}
	t.Run("alias", func(t *testing.T) {
		a, path, cfg, _ := catalogRuntimeFixture(t)
		ctx := context.Background()
		row := store.DBAlias{Name: "route", WorkspaceID: store.SystemWorkspaceID, Algorithm: "round_robin"}
		targets := []store.DBAliasTarget{{Provider: "local", Model: "old"}}
		if err := a.adminStore.CreateAlias(ctx, &row, targets, store.CatalogEditOptions{Static: a.Config.Catalog}); err != nil {
			t.Fatal(err)
		}
		if err := a.Reload(); err != nil {
			t.Fatal(err)
		}
		old := a.Config
		rewriteConfigFile(t, path, cfg+`
alias "route" {
  algorithm = "round_robin"
  providers = ["local"]
  model = "new"
}
`)
		if err := a.Reload(); err == nil || !strings.Contains(err.Error(), "already defined in static config") {
			t.Fatalf("alias collision reload admission = %v", err)
		}
		candidate, err := Build(ctx, BuildOptions{ConfigPath: path, LogOutput: io.Discard})
		if candidate != nil {
			_ = candidate.Close()
		}
		if err == nil || !strings.Contains(err.Error(), "already defined in static config") || a.Config != old {
			t.Fatalf("alias collision startup admission = %v", err)
		}
		after, err := a.adminStore.GetAlias(ctx, row.Name)
		if err != nil || !reflect.DeepEqual(after, row) {
			t.Fatal("collision rewrote or reassigned the saved managed alias")
		}
		rewriteConfigFile(t, path, cfg)
		if err := a.Reload(); err != nil {
			t.Fatal(err)
		}
	})
}
