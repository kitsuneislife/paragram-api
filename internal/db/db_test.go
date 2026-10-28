package db_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/kitsuneislife/paragram-api/internal/db"
	"github.com/kitsuneislife/paragram-api/internal/db/dbtest"
)

// TestMigrate runs against a real Postgres (see dbtest for how one is found).
func TestMigrate(t *testing.T) {
	pool := dbtest.New(t) // already migrated once
	ctx := context.Background()

	// A second run must be a no-op.
	if err := db.Migrate(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), pool); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pg_extension WHERE extname IN ('citext', 'pg_trgm')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("extensions installed = %d, want 2", n)
	}
}
