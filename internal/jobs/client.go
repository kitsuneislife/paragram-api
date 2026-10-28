package jobs

import (
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"

	"github.com/kitsuneislife/paragram-api/internal/storage"
)

// NewClient returns a River client. With a store it also works jobs
// (`paragram worker`); with nil it only inserts them (the API).
func NewClient(log *slog.Logger, pool *pgxpool.Pool, st *storage.Store) (*river.Client[pgx.Tx], error) {
	cfg := &river.Config{Logger: log}
	if st != nil {
		workers := river.NewWorkers()
		river.AddWorker(workers, &ProcessMedia{Pool: pool, Store: st})
		cfg.Workers = workers
		cfg.Queues = map[string]river.QueueConfig{river.QueueDefault: {MaxWorkers: 4}}
	}
	return river.NewClient(riverpgxv5.New(pool), cfg)
}
