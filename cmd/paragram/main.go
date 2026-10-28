// Command paragram runs the API server (`paragram api`), the background job
// worker (`paragram worker`) or applies database migrations (`paragram migrate`).
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/kitsuneislife/paragram-api/internal/api"
	"github.com/kitsuneislife/paragram-api/internal/config"
	"github.com/kitsuneislife/paragram-api/internal/db"
	"github.com/kitsuneislife/paragram-api/internal/jobs"
	"github.com/kitsuneislife/paragram-api/internal/storage"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

const usage = "usage: paragram <api|worker|migrate>"

func run(log *slog.Logger) error {
	if len(os.Args) != 2 {
		return errors.New(usage)
	}
	cmd := os.Args[1]
	if cmd != "api" && cmd != "worker" && cmd != "migrate" {
		return errors.New(usage)
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	if cmd == "migrate" {
		return db.Migrate(ctx, log, pool)
	}

	st, err := storage.New(cfg.S3)
	if err != nil {
		return err
	}
	if cmd == "worker" {
		return work(ctx, log, pool, st)
	}
	jc, err := jobs.NewClient(log, pool, nil)
	if err != nil {
		return err
	}

	redisOpts, err := redis.ParseURL(cfg.RedisURL)
	if err != nil {
		return fmt.Errorf("redis: %w", err)
	}
	rdb := redis.NewClient(redisOpts)
	defer rdb.Close()
	pingRedis := func(ctx context.Context) error { return rdb.Ping(ctx).Err() }
	if err := pingRedis(ctx); err != nil {
		return fmt.Errorf("redis: ping: %w", err)
	}

	checks := []api.Check{
		{Name: "postgres", Ping: pool.Ping},
		{Name: "redis", Ping: pingRedis},
		{Name: "storage", Ping: st.Ping},
	}
	return serve(ctx, log, cfg.Addr, api.New(api.Deps{
		Log:         log,
		Checks:      checks,
		CORSOrigins: cfg.CORSOrigins,
		Pool:        pool,
		Redis:       rdb,
		JWTSecret:   cfg.JWTSecret,
		Store:       st,
		Jobs:        jc,
	}))
}

// work runs background jobs until ctx is cancelled, then lets running ones finish.
func work(ctx context.Context, log *slog.Logger, pool *pgxpool.Pool, st *storage.Store) error {
	jc, err := jobs.NewClient(log, pool, st)
	if err != nil {
		return err
	}
	if err := jc.Start(ctx); err != nil {
		return fmt.Errorf("worker: start: %w", err)
	}
	log.Info("worker started")
	<-ctx.Done()
	log.Info("worker stopping")
	stopCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return jc.Stop(stopCtx)
}

// serve runs the server until ctx is cancelled, then drains in-flight requests.
func serve(ctx context.Context, log *slog.Logger, addr string, h http.Handler) error {
	srv := &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Info("listening", "addr", addr)
		errc <- srv.ListenAndServe()
	}()

	select {
	case err := <-errc:
		return fmt.Errorf("listen: %w", err)
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
