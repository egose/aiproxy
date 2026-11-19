package store

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func assertStoredReasoningJSON(t *testing.T, got json.RawMessage, want string) {
	t.Helper()
	if want == "" {
		if got != nil {
			t.Fatalf("policy = %s, want SQL NULL", got)
		}
		return
	}
	var actual, expected any
	if err := json.Unmarshal(got, &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("policy = %s, want %s", got, want)
	}
}

func assertReasoningColumns(t *testing.T, st *Store) {
	t.Helper()
	var columns int
	if err := st.DB.NewRaw(`SELECT count(*) FROM information_schema.columns WHERE table_schema = current_schema() AND table_name IN ('db_providers', 'db_provider_models') AND column_name = 'encrypted_reasoning' AND data_type = 'jsonb' AND is_nullable = 'YES' AND column_default IS NULL`).Scan(context.Background(), &columns); err != nil || columns != 2 {
		t.Fatalf("nullable local declaration column contract: count=%d err=%v", columns, err)
	}
}

func TestEncryptedReasoningMigrationFreshAndRepeat(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	assertReasoningColumns(t, st)
	status, err := st.Status(ctx)
	if err != nil || len(status.Pending) != 0 || len(status.Applied) == 0 || status.Applied[len(status.Applied)-1] != "serving_revisions.sql" {
		t.Fatalf("fresh migration registration: %+v %v", status, err)
	}
	if applied, err := st.MigrateUp(ctx); err != nil || len(applied) != 0 {
		t.Fatalf("repeat registered migration: applied=%v err=%v", applied, err)
	}
	if _, err := st.DB.ExecContext(ctx, encryptedReasoningSQL); err != nil {
		t.Fatalf("repeat additive SQL: %v", err)
	}
	assertReasoningColumns(t, st)
}

func TestEncryptedReasoningMigrationPreservesLegacyRows(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	var schema string
	if err := st.DB.NewRaw(`SELECT current_schema()`).Scan(ctx, &schema); err != nil || !strings.HasPrefix(schema, "ledger_") {
		t.Fatal("migration fixture must own an isolated disposable schema")
	}
	for _, table := range []string{"db_providers", "db_provider_models"} {
		if _, err := st.DB.ExecContext(ctx, `ALTER TABLE `+table+` DROP COLUMN encrypted_reasoning`); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.DB.NewDelete().Table("schema_migrations").Where("name = ?", "encrypted_reasoning.sql").Exec(ctx); err != nil {
		t.Fatal(err)
	}
	type snapshot struct {
		Body    json.RawMessage
		Version string
	}
	readSnapshot := func(table string, id uuid.UUID) snapshot {
		t.Helper()
		var out snapshot
		if err := st.DB.NewRaw(`SELECT to_jsonb(r) - 'encrypted_reasoning' AS body, xmin::text AS version FROM `+table+` AS r WHERE id = ?`, id).Scan(ctx, &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	providers := make(map[uuid.UUID]snapshot)
	models := make(map[uuid.UUID]snapshot)
	for _, fixture := range []struct{ name, extends string }{{"legacy-base", ""}, {"legacy-child", "legacy-base"}} {
		var providerID, modelID uuid.UUID
		if err := st.DB.NewRaw(`INSERT INTO db_providers (name,type,display_name,base_url,extends,enabled,api_key_ref_key) VALUES (?,?,?,?,?,?,?) RETURNING id`, fixture.name, "openai", "Preserved", "https://example.test/v1", fixture.extends, fixture.extends == "", "preserved-key").Scan(ctx, &providerID); err != nil {
			t.Fatal(err)
		}
		if err := st.DB.NewRaw(`INSERT INTO db_provider_models (provider_id,name,display_name,upstream_name,capabilities,pricing) VALUES (?,?,'Preserved Model','upstream/m','["chat"]','{"input_per_million":2}') RETURNING id`, providerID, "legacy-model").Scan(ctx, &modelID); err != nil {
			t.Fatal(err)
		}
		providers[providerID] = readSnapshot("db_providers", providerID)
		models[modelID] = readSnapshot("db_provider_models", modelID)
	}
	applied, err := st.MigrateUp(ctx)
	if err != nil || !reflect.DeepEqual(applied, []string{"encrypted_reasoning.sql"}) {
		t.Fatalf("legacy migration: applied=%v err=%v", applied, err)
	}
	assertReasoningColumns(t, st)
	verifyUnchanged := func() {
		t.Helper()
		for id, before := range providers {
			if after := readSnapshot("db_providers", id); !reflect.DeepEqual(before, after) {
				t.Fatal("migration rewrote legacy provider data or tuple versions")
			}
			var absent bool
			if err := st.DB.NewRaw(`SELECT encrypted_reasoning IS NULL FROM db_providers WHERE id = ?`, id).Scan(ctx, &absent); err != nil || !absent {
				t.Fatalf("legacy provider became explicit: null=%t err=%v", absent, err)
			}
			rows, err := st.ListProviderModels(ctx, id)
			if err != nil || len(rows) != 1 || rows[0].EncryptedReasoning != nil {
				t.Fatalf("legacy models deleted or default-filled: %v", err)
			}
		}
		for id, before := range models {
			if after := readSnapshot("db_provider_models", id); !reflect.DeepEqual(before, after) {
				t.Fatal("migration rewrote legacy model data or tuple versions")
			}
		}
	}
	verifyUnchanged()
	if applied, err := st.MigrateUp(ctx); err != nil || len(applied) != 0 {
		t.Fatalf("repeat migration: applied=%v err=%v", applied, err)
	}
	if _, err := st.DB.ExecContext(ctx, encryptedReasoningSQL); err != nil {
		t.Fatal(err)
	}
	verifyUnchanged()
}

func TestEncryptedReasoningStoreDeclarationRoundTrip(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	base := DBProvider{Name: "base", Type: "openai", Enabled: true, WorkspaceID: SystemWorkspaceID}
	if err := st.CreateProviderAggregate(ctx, &base, []DBProviderModel{{Name: "m"}, {Name: "n"}, {Name: "free"}}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ name, raw, want string }{
		{"absent", "", ""},
		{"json-null", " null ", ""},
		{"reset", `{}`, `{}`},
		{"false", `{"passthrough":false}`, `{"passthrough":false}`},
		{"retry", `{"on_caller_mismatch":"strip_and_retry"}`, `{"on_caller_mismatch":"strip_and_retry"}`},
		{"empty-list", `{"match_messages":[]}`, `{"match_messages":[]}`},
		{"patterns", `{"match_messages":[" Vendor Marker "]}`, `{"match_messages":[" Vendor Marker "]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			for _, extends := range []string{"", "base"} {
				p := DBProvider{Name: "policy-" + uuid.NewString(), Type: "openai", Extends: extends, Enabled: true, EncryptedReasoning: json.RawMessage(tc.raw)}
				if err := st.CreateProviderAggregate(ctx, &p, []DBProviderModel{{Name: "m", EncryptedReasoning: json.RawMessage(tc.raw)}}); err != nil {
					t.Fatal(err)
				}
				stored, err := st.GetProvider(ctx, p.Name)
				if err != nil {
					t.Fatal(err)
				}
				assertStoredReasoningJSON(t, stored.EncryptedReasoning, tc.want)
				models, err := st.ListProviderModels(ctx, p.ID)
				if err != nil || len(models) != 1 {
					t.Fatal(err)
				}
				assertStoredReasoningJSON(t, models[0].EncryptedReasoning, tc.want)
				var providerNull, modelNull bool
				if err := st.DB.NewRaw(`SELECT encrypted_reasoning IS NULL FROM db_providers WHERE id = ?`, p.ID).Scan(ctx, &providerNull); err != nil {
					t.Fatal(err)
				}
				if err := st.DB.NewRaw(`SELECT encrypted_reasoning IS NULL FROM db_provider_models WHERE id = ?`, models[0].ID).Scan(ctx, &modelNull); err != nil || providerNull != (tc.want == "") || modelNull != providerNull {
					t.Fatal("SQL NULL was conflated with a JSON object/null")
				}
				stored.APIKeyEncrypted = []byte("updated credential")
				if err := st.UpdateProvider(ctx, &stored); err != nil {
					t.Fatal(err)
				}
				reopened, err := st.GetProvider(ctx, p.Name)
				if err != nil {
					t.Fatal(err)
				}
				assertStoredReasoningJSON(t, reopened.EncryptedReasoning, tc.want)
				unchanged, err := st.ListProviderModels(ctx, p.ID)
				if err != nil || !reflect.DeepEqual(unchanged, models) {
					t.Fatal("credential-only storage update changed model declarations")
				}
				replacement := []DBProviderModel{
					{Name: "m", EncryptedReasoning: json.RawMessage(`{}`)},
					{Name: "n", EncryptedReasoning: json.RawMessage(`{"passthrough":false}`)},
					{Name: "free", EncryptedReasoning: json.RawMessage(`null`)},
				}
				if err := st.UpdateProviderAggregate(ctx, &reopened, &replacement); err != nil {
					t.Fatal(err)
				}
				reopened, err = st.GetProvider(ctx, p.Name)
				if err != nil {
					t.Fatal(err)
				}
				assertStoredReasoningJSON(t, reopened.EncryptedReasoning, tc.want)
				saved, err := st.ListProviderModels(ctx, p.ID)
				if err != nil || len(saved) != 3 {
					t.Fatal(err)
				}
				for _, model := range saved {
					assertStoredReasoningJSON(t, model.EncryptedReasoning, map[string]string{"m": `{}`, "n": `{"passthrough":false}`, "free": ""}[model.Name])
				}
				reopened.EncryptedReasoning = json.RawMessage(`null`)
				replacement = []DBProviderModel{{Name: "m"}}
				if err := st.UpdateProviderAggregate(ctx, &reopened, &replacement); err != nil {
					t.Fatal(err)
				}
				reopened, err = st.GetProvider(ctx, p.Name)
				if err != nil {
					t.Fatal(err)
				}
				saved, err = st.ListProviderModels(ctx, p.ID)
				if err != nil || len(saved) != 1 || saved[0].EncryptedReasoning != nil || reopened.EncryptedReasoning != nil {
					t.Fatal("removal did not persist SQL NULL")
				}
			}
		})
	}
	a := DBAlias{Name: "null-alias", Algorithm: "round_robin", EncryptedReasoning: json.RawMessage(`null`)}
	p := DBProvider{Name: "alias-base", Type: "openai", WorkspaceID: SystemWorkspaceID, Enabled: true}
	if err := st.CreateProviderAggregate(ctx, &p, []DBProviderModel{{Name: "m"}}); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateAlias(ctx, &a, []DBAliasTarget{{Provider: p.Name, Model: "m"}}); err != nil {
		t.Fatal(err)
	}
	alias, err := st.GetAlias(ctx, a.Name)
	if err != nil || alias.EncryptedReasoning != nil {
		t.Fatal("alias JSON null was not canonicalized to SQL NULL")
	}
}

func TestEncryptedReasoningStoreAggregateRollback(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	if _, err := st.DB.ExecContext(ctx, `ALTER TABLE db_provider_models ADD CONSTRAINT reasoning_fault CHECK (name <> 'fault')`); err != nil {
		t.Fatal(err)
	}
	p := DBProvider{Name: "policy-rollback", Type: "openai", Enabled: true, EncryptedReasoning: json.RawMessage(`{"passthrough":false}`)}
	models := []DBProviderModel{{Name: "m", EncryptedReasoning: json.RawMessage(`{"on_caller_mismatch":"strip_and_retry"}`)}, {Name: "fault", EncryptedReasoning: json.RawMessage(`{}`)}}
	input, inputModels := p, append([]DBProviderModel(nil), models...)
	if err := st.CreateProviderAggregate(ctx, &p, models); err == nil {
		t.Fatal("invalid model aggregate accepted")
	}
	if !reflect.DeepEqual(p, input) || !reflect.DeepEqual(models, inputModels) {
		t.Fatal("create rollback changed caller declarations")
	}
	if _, err := st.GetProvider(ctx, p.Name); err == nil {
		t.Fatal("failed aggregate left a provider/policy orphan")
	}
	if err := st.CreateProviderAggregate(ctx, &p, models[:1]); err != nil {
		t.Fatal(err)
	}
	before, err := st.GetProvider(ctx, p.Name)
	if err != nil {
		t.Fatal(err)
	}
	beforeModels, err := st.ListProviderModels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	p = before
	p.EncryptedReasoning = json.RawMessage(`{}`)
	p.APIKeyEncrypted = []byte("replacement credential")
	input = p
	models[0].EncryptedReasoning = json.RawMessage(`{"passthrough":false}`)
	inputModels = append([]DBProviderModel(nil), models...)
	if err := st.UpdateProviderAggregate(ctx, &p, &models); err == nil {
		t.Fatal("failed policy/model replacement accepted")
	}
	after, err := st.GetProvider(ctx, p.Name)
	if err != nil {
		t.Fatal(err)
	}
	afterModels, err := st.ListProviderModels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(beforeModels, afterModels) || !reflect.DeepEqual(p, input) || !reflect.DeepEqual(models, inputModels) {
		t.Fatal("rollback changed provider/model policy, credential, revision, or caller inputs")
	}
}
