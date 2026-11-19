package store

import (
	"context"
	"errors"
	"testing"
)

func TestServingRevisionsMutationCoverage(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	if again, err := st.MigrateUp(ctx); err != nil || len(again) != 0 {
		t.Fatal(again, err)
	}
	for _, group := range []struct {
		tables  []string
		catalog bool
	}{
		{[]string{"db_providers", "db_provider_models", "db_aliases", "db_alias_targets"}, true},
		{[]string{"inbound_keys", "key_users", "key_teams", "scope_quotas"}, false},
	} {
		for _, table := range group.tables {
			for _, statement := range []string{"UPDATE " + table + " SET " + map[string]string{"db_providers": "name = name", "db_provider_models": "name = name", "db_aliases": "name = name", "db_alias_targets": "model = model", "inbound_keys": "enabled = enabled", "key_users": "key_id = key_id", "key_teams": "key_id = key_id", "scope_quotas": "model = model"}[table] + " WHERE FALSE", "DELETE FROM " + table + " WHERE FALSE"} {
				before, err := st.ServingRevisions(ctx)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := st.DB.ExecContext(ctx, statement); err != nil {
					t.Fatal(err)
				}
				after, err := st.ServingRevisions(ctx)
				if err != nil {
					t.Fatal(err)
				}
				want := before
				if group.catalog {
					want.Catalog++
				} else {
					want.Keys++
				}
				if after != want {
					t.Fatalf("%s: %v -> %v, want %v", statement, before, after, want)
				}
			}
		}
	}
	before, err := st.ServingRevisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, "UPDATE inbound_keys SET enabled = FALSE"); err != nil {
		t.Fatal(err)
	}
	during, err := st.ServingRevisions(ctx)
	if err != nil || during != before {
		t.Fatal("uncommitted revision visible", during, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	after, err := st.ServingRevisions(ctx)
	if err != nil || after != before {
		t.Fatal("rollback advanced revision", after, err)
	}
}

func TestServingRevisionsDeletionCascades(t *testing.T) {
	st := openLedgerTestStore(t)
	ctx := context.Background()
	workspace, user, _, key := ledgerFixture(t, st)
	retained := User{Email: "retained@example.invalid"}
	if err := st.CreateUser(ctx, &retained); err != nil {
		t.Fatal(err)
	}
	if err := st.AddMembership(ctx, &WorkspaceMember{UserID: retained.ID, WorkspaceID: workspace.ID, Role: "admin"}); err != nil {
		t.Fatal(err)
	}
	before, err := st.ServingRevisions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteUser(ctx, user.ID); err != nil {
		t.Fatal(err)
	}
	after, err := st.ServingRevisions(ctx)
	if err != nil || after.Keys <= before.Keys {
		t.Fatal("account/key cascade not revisioned", after, err)
	}
	if err := st.DB.NewSelect().Model(&InboundKey{}).Where("id = ?", key.ID).Scan(ctx); err == nil {
		t.Fatal("key survived account deletion")
	}
	before = after
	if err := st.DeleteWorkspace(ctx, workspace.ID); err != nil {
		t.Fatal(err)
	}
	after, err = st.ServingRevisions(ctx)
	if err != nil || after.Catalog <= before.Catalog || after.Keys <= before.Keys {
		t.Fatal("workspace deletion not revisioned", after, err)
	}
	if err := st.DB.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ServingRevisions(ctx); err == nil || errors.Is(err, context.Canceled) {
		t.Fatal("closed DB certified freshness", err)
	}
}
