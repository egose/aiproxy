package store

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/egose/aiproxy/internal/tlsprofile"
	"github.com/google/uuid"
)

func TestTLSClientHelloMigrationPreservesLegacyProviders(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	var schema string
	if err := st.DB.NewRaw(`SELECT current_schema()`).Scan(ctx, &schema); err != nil || !strings.HasPrefix(schema, "ledger_") {
		t.Fatal("migration fixture must own an isolated disposable schema")
	}
	var columns int
	if err := st.DB.NewRaw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'db_providers' AND column_name = 'tls_client_hello' AND data_type = 'text' AND is_nullable = 'NO' AND column_default = '''''::text'`).Scan(ctx, &columns); err != nil || columns != 1 {
		t.Fatalf("fresh selector column contract: count=%d err=%v", columns, err)
	}
	if err := st.DB.NewRaw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name = 'db_providers' AND column_name LIKE 'tls_client_hello_%'`).Scan(ctx, &columns); err != nil || columns != 0 {
		t.Fatal("custom source columns must not exist")
	}
	if _, err := st.DB.ExecContext(ctx, `ALTER TABLE db_providers DROP COLUMN tls_client_hello`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.NewDelete().Table("schema_migrations").Where("name = ?", "tls_client_hello.sql").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	ids := make(map[string]uuid.UUID)
	for _, fixture := range []struct{ name, extends string }{{"legacy", ""}, {"legacy-derived", "legacy"}} {
		var id uuid.UUID
		if err := st.DB.NewRaw(`INSERT INTO db_providers (name,type,enabled,display_name,base_url,extends,api_key_ref_key) VALUES (?,?,?,?,?,?,?) RETURNING id`, fixture.name, "openai", false, "Legacy", "https://example.test/v1", fixture.extends, "preserved-key").Scan(ctx, &id); err != nil {
			t.Fatal(err)
		}
		ids[fixture.name] = id
	}
	applied, err := st.MigrateUp(ctx)
	if err != nil || !reflect.DeepEqual(applied, []string{"tls_client_hello.sql"}) {
		t.Fatalf("legacy migration: applied=%v err=%v", applied, err)
	}
	for name, id := range ids {
		p, err := st.GetProvider(ctx, name)
		if err != nil || p.ID != id || p.TLSClientHello != "" || p.DisplayName != "Legacy" || p.BaseURL != "https://example.test/v1" || p.Enabled || p.APIKeyRefKey != "preserved-key" {
			t.Fatalf("legacy provider values/default were not retained: name=%s err=%v", name, err)
		}
		if (p.Extends != "") != (name == "legacy-derived") {
			t.Fatal("migration changed inheritance")
		}
	}
	if applied, err := st.MigrateUp(ctx); err != nil || len(applied) != 0 {
		t.Fatalf("repeat migration: applied=%v err=%v", applied, err)
	}
}

func TestTLSClientHelloStorePersistence(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	for i, selector := range append([]string{""}, tlsprofile.BuiltinSelectors()...) {
		p := DBProvider{Name: "tls-" + uuid.NewString(), Type: "openai", TLSClientHello: selector, Enabled: true}
		if err := st.CreateProviderAggregate(ctx, &p, []DBProviderModel{{Name: "m"}}); err != nil {
			t.Fatal(err)
		}
		stored, err := st.GetProvider(ctx, p.Name)
		if err != nil || stored.TLSClientHello != selector {
			t.Fatalf("selector %d did not round-trip: %v", i, err)
		}
		stored.DisplayName = "Preserved"
		if err := st.UpdateProviderAggregate(ctx, &stored, nil); err != nil {
			t.Fatal(err)
		}
		stored, err = st.GetProvider(ctx, p.Name)
		if err != nil || stored.TLSClientHello != selector {
			t.Fatal("aggregate metadata update changed selector")
		}
		stored.TLSClientHello = "native"
		if err := st.UpdateProvider(ctx, &stored); err != nil {
			t.Fatal(err)
		}
		stored, err = st.GetProvider(ctx, p.Name)
		if err != nil || stored.TLSClientHello != "native" {
			t.Fatal("explicit native reset was not persisted")
		}
	}
}
