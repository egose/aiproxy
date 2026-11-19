package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/uptrace/bun"
)

var ErrCatalogConflict = errors.New("catalog changed concurrently; read current state and retry")

func nextCatalogRevision(previous time.Time) time.Time {
	next := time.Now().UTC().Truncate(time.Microsecond)
	if !next.After(previous) {
		next = previous.Add(time.Microsecond)
	}
	return next
}

func replaceProviderModels(ctx context.Context, tx bun.Tx, id uuid.UUID, models []DBProviderModel) error {
	if _, err := tx.NewDelete().Model((*DBProviderModel)(nil)).Where("provider_id = ?", id).Exec(ctx); err != nil {
		return err
	}
	for _, model := range models {
		model.ID = uuid.New()
		model.ProviderID = id
		model.EncryptedReasoning = nullableEncryptedReasoning(model.EncryptedReasoning)
		if _, err := tx.NewInsert().Model(&model).Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func createProviderAggregateTx(ctx context.Context, tx bun.Tx, p *DBProvider, models []DBProviderModel, options ...CatalogEditOptions) (DBProvider, error) {
	if err := lockMembershipWorkspace(ctx, tx, p.WorkspaceID); err != nil {
		return DBProvider{}, err
	}
	next := *p
	next.EncryptedReasoning = nullableEncryptedReasoning(next.EncryptedReasoning)
	next.ID = uuid.New()
	next.CreatedAt = nextCatalogRevision(time.Time{})
	next.UpdatedAt = next.CreatedAt
	if _, err := tx.NewInsert().Model(&next).Exec(ctx); err != nil {
		return DBProvider{}, err
	}
	if err := replaceProviderModels(ctx, tx, next.ID, models); err != nil {
		return DBProvider{}, err
	}
	if err := validateProviderGraph(ctx, tx, next, options); err != nil {
		return DBProvider{}, err
	}
	return next, nil
}

func updateProviderAggregateTx(ctx context.Context, tx bun.Tx, p *DBProvider, models *[]DBProviderModel, options ...CatalogEditOptions) (DBProvider, error) {
	current, err := lockCatalogProvider(ctx, tx, p.ID)
	if errors.Is(err, sql.ErrNoRows) {
		return DBProvider{}, ErrCatalogConflict
	}
	if err != nil {
		return DBProvider{}, err
	}
	if current.Name != p.Name || current.WorkspaceID != p.WorkspaceID || !current.UpdatedAt.Equal(p.UpdatedAt) {
		return DBProvider{}, ErrCatalogConflict
	}
	next := *p
	next.EncryptedReasoning = nullableEncryptedReasoning(next.EncryptedReasoning)
	next.UpdatedAt = nextCatalogRevision(p.UpdatedAt)
	result, err := tx.NewUpdate().Model(&next).Where("id = ? AND updated_at = ?", p.ID, p.UpdatedAt).Exec(ctx)
	if err != nil {
		return DBProvider{}, err
	}
	if n, err := result.RowsAffected(); err != nil {
		return DBProvider{}, err
	} else if n != 1 {
		return DBProvider{}, ErrCatalogConflict
	}
	if models != nil {
		if err := replaceProviderModels(ctx, tx, p.ID, *models); err != nil {
			return DBProvider{}, err
		}
	}
	if err := validateProviderGraph(ctx, tx, next, options); err != nil {
		return DBProvider{}, err
	}
	return next, nil
}

func (s *Store) CreateProviderAggregate(ctx context.Context, p *DBProvider, models []DBProviderModel, options ...CatalogEditOptions) error {
	var next DBProvider
	err := s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		next, err = createProviderAggregateTx(ctx, tx, p, models, options...)
		return err
	})
	if err != nil {
		return err
	}
	*p = next
	return nil
}

func (s *Store) UpdateProviderAggregate(ctx context.Context, p *DBProvider, models *[]DBProviderModel, options ...CatalogEditOptions) error {
	var next DBProvider
	err := s.DB.RunInTx(ctx, nil, func(ctx context.Context, tx bun.Tx) error {
		var err error
		next, err = updateProviderAggregateTx(ctx, tx, p, models, options...)
		return err
	})
	if err != nil {
		return err
	}
	*p = next
	return nil
}
