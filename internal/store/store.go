package store

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"sort"
	"strings"

	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/pgdialect"
	"github.com/uptrace/bun/driver/pgdriver"
)

//go:embed migrations/*.sql
var migrationFS embed.FS

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

func migrationNames() ([]string, error) {
	entries, err := migrationFS.ReadDir("migrations")
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names, nil
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
	names, err := migrationNames()
	if err != nil {
		return MigrationStatus{}, err
	}
	if err := ensureMigrationsTable(ctx, db); err != nil {
		return MigrationStatus{}, err
	}
	applied, err := appliedMigrations(ctx, db)
	if err != nil {
		return MigrationStatus{}, err
	}
	var st MigrationStatus
	for _, n := range names {
		if applied[n] {
			st.Applied = append(st.Applied, n)
		} else {
			st.Pending = append(st.Pending, n)
		}
	}
	return st, nil
}

func (s *Store) MigrateUp(ctx context.Context) ([]string, error) {
	return migrateUp(ctx, s.DB)
}

func migrateUp(ctx context.Context, db *bun.DB) ([]string, error) {
	st, err := migrationStatus(ctx, db)
	if err != nil {
		return nil, err
	}
	var applied []string
	for _, name := range st.Pending {
		raw, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return applied, err
		}
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			return applied, err
		}
		if _, err := tx.ExecContext(ctx, string(raw)); err != nil {
			_ = tx.Rollback()
			return applied, fmt.Errorf("migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (name) VALUES (?) ON CONFLICT DO NOTHING`, name); err != nil {
			_ = tx.Rollback()
			return applied, err
		}
		if err := tx.Commit(); err != nil {
			return applied, err
		}
		applied = append(applied, name)
	}
	return applied, nil
}
