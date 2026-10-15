package store

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"strings"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

//go:embed schema.sql
var schemaSQL string

//go:embed forward_authorization.sql
var forwardAuthorizationSQL string

const schemaVersion = "schema.sql"

var migrations = []struct {
	name string
	sql  string
}{
	{schemaVersion, schemaSQL},
	{"forward_authorization.sql", forwardAuthorizationSQL},
}

type Store struct {
	DB *bun.DB
}

func Open(ctx context.Context, url string) (*Store, error) {
	if strings.TrimSpace(url) == "" {
		return nil, fmt.Errorf("database url is empty")
	}
	conn := pgdriver.NewConnector(pgdriver.WithDSN(url))
	sqldb := sql.OpenDB(conn)
	db := bun.NewDB(sqldb, pgdialect.New())
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("database ping: %w", err)
	}
	return &Store{DB: db}, nil
}

func (s *Store) Close() error {
	return s.DB.Close()
}

func (s *Store) Ping(ctx context.Context) error {
	return s.DB.PingContext(ctx)
}

type MigrationStatus struct {
	Applied []string
	Pending []string
}

func ensureMigrationsTable(ctx context.Context, db *bun.DB) error {
	_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (name TEXT PRIMARY KEY, applied_at TIMESTAMPTZ NOT NULL DEFAULT now())`)
	return err
}

func appliedMigrations(ctx context.Context, db *bun.DB) (map[string]bool, error) {
	out := map[string]bool{}
	var names []string
	if err := db.NewSelect().Table("schema_migrations").Column("name").Scan(ctx, &names); err != nil {
		if strings.Contains(err.Error(), "does not exist") {
			return out, nil
		}
		return nil, err
	}
	for _, n := range names {
		out[n] = true
	}
	return out, nil
}

func (s *Store) Status(ctx context.Context) (MigrationStatus, error) {
	return migrationStatus(ctx, s.DB)
}

func migrationStatus(ctx context.Context, db *bun.DB) (MigrationStatus, error) {
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return MigrationStatus{}, err
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return MigrationStatus{}, err
	}
	var status MigrationStatus
	for _, migration := range migrations {
		if applied[migration.name] {
			status.Applied = append(status.Applied, migration.name)
		} else {
			status.Pending = append(status.Pending, migration.name)
		}
	}
	return status, nil
}

func (s *Store) MigrateUp(ctx context.Context) ([]string, error) {
	return migrateUp(ctx, s.DB)
}

func migrateUp(ctx context.Context, db *bun.DB) ([]string, error) {
	st, err := migrationStatus(ctx, db)
	if err != nil {
		return nil, err
	}
	if len(st.Pending) == 0 {
		return nil, nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, migration := range migrations {
		pending := false
		for _, name := range st.Pending {
			pending = pending || name == migration.name
		}
		if !pending {
			continue
		}
		if _, err := tx.ExecContext(ctx, migration.sql); err != nil {
			_ = tx.Rollback()
			return nil, fmt.Errorf("migration %s: %w", migration.name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?) ON CONFLICT DO NOTHING`, migration.name); err != nil {
			_ = tx.Rollback()
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return st.Pending, nil
}
