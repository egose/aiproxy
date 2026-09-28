package store

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
)

func TestProviderAggregateRollback(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	p := DBProvider{Name: uuid.NewString(), Type: "openai", WorkspaceID: SystemWorkspaceID, Enabled: true, DisplayName: "old"}
	models := []DBProviderModel{{Name: "first"}, {Name: "fault"}}
	before, beforeModels := p, append([]DBProviderModel(nil), models...)
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE db_provider_models ADD CONSTRAINT flow02_fault CHECK (name <> 'fault') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.DB.ExecContext(ctx, "ALTER TABLE db_provider_models DROP CONSTRAINT IF EXISTS flow02_fault")
	})
	if err := st.CreateProviderAggregate(ctx, &p, models); err == nil {
		t.Fatal("model failure accepted")
	}
	if !reflect.DeepEqual(p, before) || !reflect.DeepEqual(models, beforeModels) {
		t.Fatal("rollback mutated caller outputs")
	}
	if _, err := st.GetProvider(ctx, p.Name); err == nil {
		t.Fatal("orphan provider")
	}
	if err := st.CreateProviderAggregate(ctx, &p, models[:1]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteProvider(ctx, p.ID) })
	old, err := st.GetProvider(ctx, p.Name)
	if err != nil {
		t.Fatal(err)
	}
	oldModels, err := st.ListProviderModels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	p = old
	p.DisplayName = "new"
	input := p
	if err := st.UpdateProviderAggregate(ctx, &p, &models); err == nil {
		t.Fatal("update accepted")
	}
	got, err := st.GetProvider(ctx, p.Name)
	if err != nil {
		t.Fatal(err)
	}
	gotModels, err := st.ListProviderModels(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, old) || !reflect.DeepEqual(gotModels, oldModels) || !reflect.DeepEqual(p, input) || !reflect.DeepEqual(models, beforeModels) {
		t.Fatal("aggregate rollback changed rows or inputs")
	}
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE db_provider_models DROP CONSTRAINT flow02_fault"); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateProviderAggregate(ctx, &p, &models); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateProvider(ctx, &p); err != nil {
		t.Fatalf("returned revision unusable: %v", err)
	}
}

func TestProviderAggregateConcurrentCredentials(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	for _, first := range []string{"aggregate", "credential", "models"} {
		t.Run(first, func(t *testing.T) {
			p := DBProvider{Name: uuid.NewString(), Type: "openai", WorkspaceID: SystemWorkspaceID, Enabled: true, APIKeyEncrypted: []byte("old")}
			if err := st.CreateProviderAggregate(ctx, &p, []DBProviderModel{{Name: "old"}}); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = st.DeleteProvider(ctx, p.ID) })
			stale := p
			models := []DBProviderModel{{Name: "new"}}
			tx, err := st.DB.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			var locked DBProvider
			if err := tx.NewSelect().Model(&locked).Where("id = ?", p.ID).For("UPDATE").Scan(ctx); err != nil {
				t.Fatal(err)
			}
			started, done := make(chan struct{}), make(chan error, 1)
			go func() {
				close(started)
				if first == "credential" {
					stale.DisplayName = "stale edit"
					done <- st.UpdateProviderAggregate(ctx, &stale, &models)
				} else {
					stale.APIKeyEncrypted = []byte("stale credential")
					done <- st.UpdateProvider(ctx, &stale)
				}
			}()
			<-started
			locked.UpdatedAt = nextCatalogRevision(locked.UpdatedAt)
			if first == "credential" {
				locked.APIKeyEncrypted = []byte("new credential")
			} else {
				locked.DisplayName = "committed"
			}
			if _, err := tx.NewUpdate().Model(&locked).WherePK().Exec(ctx); err != nil {
				t.Fatal(err)
			}
			if first != "credential" {
				if err := replaceProviderModels(ctx, tx, p.ID, models); err != nil {
					t.Fatal(err)
				}
			}
			if err := tx.Commit(); err != nil {
				t.Fatal(err)
			}
			if err := <-done; !errors.Is(err, ErrCatalogConflict) {
				t.Fatalf("stale write = %v", err)
			}
			current, err := st.GetProvider(ctx, p.Name)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(current, locked) {
				t.Fatal("stale writer overwrote current state")
			}
			current.APIKeyEncrypted = []byte("retry credential")
			if err := st.UpdateProvider(ctx, &current); err != nil {
				t.Fatal(err)
			}
			beforeModels, err := st.ListProviderModels(ctx, p.ID)
			if err != nil {
				t.Fatal(err)
			}
			stale = current
			if err := st.ReplaceProviderModels(ctx, p.ID, models); err != nil {
				t.Fatal(err)
			}
			if err := st.UpdateProviderAggregate(ctx, &stale, nil); !errors.Is(err, ErrCatalogConflict) {
				t.Fatalf("model revision lost: %v", err)
			}
			if len(beforeModels) != 1 {
				t.Fatal("credential edit replaced models")
			}
		})
	}
}

func TestAliasAggregateConflict(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	a := DBAlias{Name: uuid.NewString(), Algorithm: "round_robin", WorkspaceID: SystemWorkspaceID}
	targets := []DBAliasTarget{{Provider: "p", Model: "old"}}
	if err := st.CreateAlias(ctx, &a, targets); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteAlias(ctx, a.ID) })
	stale := a
	a.Algorithm = "least_connections"
	if err := st.UpdateAlias(ctx, &a, targets); err != nil {
		t.Fatal(err)
	}
	targets[0].Model = "stale"
	if err := st.UpdateAlias(ctx, &stale, targets); !errors.Is(err, ErrCatalogConflict) {
		t.Fatalf("stale alias = %v", err)
	}
	got, err := st.ListAliasTargets(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Model != "old" {
		t.Fatal("conflict replaced targets")
	}
}

func TestAliasAggregateRollbackOutputs(t *testing.T) {
	st := openInviteTestStore(t)
	ctx := context.Background()
	a := DBAlias{Name: uuid.NewString(), Algorithm: "round_robin", WorkspaceID: SystemWorkspaceID}
	targets := []DBAliasTarget{{Provider: "p", Model: "old"}, {Provider: "p", Model: "fault"}}
	input, inputTargets := a, append([]DBAliasTarget(nil), targets...)
	if _, err := st.DB.ExecContext(ctx, "ALTER TABLE db_alias_targets ADD CONSTRAINT flow02_fault CHECK (model <> 'fault') NOT VALID"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = st.DB.ExecContext(ctx, "ALTER TABLE db_alias_targets DROP CONSTRAINT IF EXISTS flow02_fault")
	})
	if err := st.CreateAlias(ctx, &a, targets); err == nil {
		t.Fatal("invalid targets accepted")
	}
	if !reflect.DeepEqual(a, input) || !reflect.DeepEqual(targets, inputTargets) {
		t.Fatal("rollback changed caller outputs")
	}
	if _, err := st.GetAlias(ctx, a.Name); err == nil {
		t.Fatal("orphan alias")
	}
	if err := st.CreateAlias(ctx, &a, targets[:1]); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.DeleteAlias(ctx, a.ID) })
	before, err := st.GetAlias(ctx, a.Name)
	if err != nil {
		t.Fatal(err)
	}
	beforeTargets, err := st.ListAliasTargets(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	a.Algorithm = "least_connections"
	input = a
	if err := st.UpdateAlias(ctx, &a, targets); err == nil {
		t.Fatal("invalid update accepted")
	}
	got, err := st.GetAlias(ctx, a.Name)
	if err != nil {
		t.Fatal(err)
	}
	gotTargets, err := st.ListAliasTargets(ctx, a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a, input) || !reflect.DeepEqual(targets, inputTargets) || !reflect.DeepEqual(got, before) || !reflect.DeepEqual(gotTargets, beforeTargets) {
		t.Fatal("alias rollback changed rows or outputs")
	}
}
