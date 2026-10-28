// Package dbtest gives integration tests a fresh, migrated Postgres database.
package dbtest

import (
	"context"
	"crypto/rand"
	"io"
	"log/slog"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/kitsuneislife/paragram-api/internal/db"
)

var (
	once         sync.Once
	containerURL string
	containerErr error
)

// New returns a pool on a new, empty database with every migration applied,
// dropped when the test ends. The server is PARAGRAM_TEST_DATABASE_URL when
// set, otherwise one testcontainers Postgres shared by the test binary. The
// test is skipped when neither is available.
func New(t *testing.T) *pgxpool.Pool {
	t.Helper()
	ctx := context.Background()

	server := os.Getenv("PARAGRAM_TEST_DATABASE_URL")
	if server == "" {
		testcontainers.SkipIfProviderIsNotHealthy(t)
		once.Do(func() {
			// Never terminated here: the testcontainers reaper removes it when the binary exits.
			ctr, err := postgres.Run(ctx, "postgres:17-alpine", postgres.BasicWaitStrategies())
			if err != nil {
				containerErr = err
				return
			}
			containerURL, containerErr = ctr.ConnectionString(ctx, "sslmode=disable")
		})
		if containerErr != nil {
			t.Fatal(containerErr)
		}
		server = containerURL
	}

	admin, err := pgx.Connect(ctx, server)
	if err != nil {
		t.Fatal(err)
	}
	name := "test_" + strings.ToLower(rand.Text())
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "DROP DATABASE "+name+" WITH (FORCE)")
		_ = admin.Close(ctx)
	})

	u, err := url.Parse(server)
	if err != nil {
		t.Fatal(err)
	}
	u.Path = "/" + name
	pool, err := db.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)

	if err := db.Migrate(ctx, slog.New(slog.NewTextHandler(io.Discard, nil)), pool); err != nil {
		t.Fatal(err)
	}
	return pool
}
