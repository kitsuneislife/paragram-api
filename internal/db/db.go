// Package db owns the Postgres connection and schema migrations.
package db

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Open creates a connection pool and verifies the database is reachable.
func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("db: open: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping: %w", err)
	}
	return pool, nil
}

// Migrate applies all pending migrations. A Postgres advisory lock makes it
// safe to run from several instances at once.
func Migrate(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool) error {
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		return fmt.Errorf("db: migrate lock: %w", err)
	}
	fsys, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("db: migrate fs: %w", err)
	}
	sqlDB := stdlib.OpenDBFromPool(pool)
	defer sqlDB.Close()

	p, err := goose.NewProvider(goose.DialectPostgres, sqlDB, fsys, goose.WithSessionLocker(locker))
	if err != nil {
		return fmt.Errorf("db: migrate: %w", err)
	}
	results, err := p.Up(ctx)
	if err != nil {
		return fmt.Errorf("db: migrate up: %w", err)
	}
	for _, r := range results {
		log.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration)
	}
	return nil
}
