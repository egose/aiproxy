package store

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestAllowInsecureBaseURLMigrationPreservesLegacyProvider(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	var schema string
	if err := st.DB.NewRaw(`SELECT current_schema()`).Scan(ctx, &schema); err != nil || !strings.HasPrefix(schema, "ledger_") {
		t.Fatalf("migration fixture must own an isolated schema: schema=%s err=%v", schema, err)
	}
	var columns int
	if err := st.DB.NewRaw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'db_providers' AND column_name = 'allow_insecure_base_url' AND data_type = 'boolean' AND is_nullable = 'NO' AND column_default = 'false'`).Scan(ctx, &columns); err != nil || columns != 1 {
		t.Fatalf("fresh column contract: count=%d err=%v", columns, err)
	}
	if _, err := st.DB.ExecContext(ctx, `ALTER TABLE db_providers DROP COLUMN allow_insecure_base_url`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.NewDelete().Table("schema_migrations").Where("name = ?", "allow_insecure_base_url.sql").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	workspace := Workspace{Name: "migration-workspace"}
	if err := st.CreateWorkspace(ctx, &workspace); err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	if err := st.DB.NewRaw(`INSERT INTO db_providers (name, type, workspace_id, enabled, display_name, base_url) VALUES (?, ?, ?, ?, ?, ?) RETURNING id`, "legacy-provider", "openai", workspace.ID, false, "Legacy", "https://example.com/v1").Scan(ctx, &id); err != nil {
		t.Fatal(err)
	}
	applied, err := st.MigrateUp(ctx)
	if err != nil || !reflect.DeepEqual(applied, []string{"allow_insecure_base_url.sql"}) {
		t.Fatalf("legacy upgrade: applied=%v err=%v", applied, err)
	}
	row, err := st.GetProvider(ctx, "legacy-provider")
	if err != nil || row.ID != id || row.DisplayName != "Legacy" || row.BaseURL != "https://example.com/v1" || row.AllowInsecureBaseURL {
		t.Fatalf("legacy provider/default was not preserved: row=%+v err=%v", row, err)
	}
	row.AllowInsecureBaseURL = true
	if _, err := st.DB.NewUpdate().Model(&row).Column("allow_insecure_base_url").WherePK().Exec(ctx); err != nil {
		t.Fatal(err)
	}
	row, err = st.GetProvider(ctx, row.Name)
	if err != nil || !row.AllowInsecureBaseURL {
		t.Fatalf("migrated column did not persist the opt-in: flag=%t err=%v", row.AllowInsecureBaseURL, err)
	}
	if applied, err := st.MigrateUp(ctx); err != nil || len(applied) != 0 {
		t.Fatalf("repeat migration: applied=%v err=%v", applied, err)
	}
}
