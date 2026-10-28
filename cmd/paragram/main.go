// Command paragram runs the API server (`paragram api`) or applies database
// migrations (`paragram migrate`).
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

	"github.com/kitsuneislife/paragram/internal/api"
	"github.com/kitsuneislife/paragram/internal/config"
	"github.com/kitsuneislife/paragram/internal/db"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if err := run(log); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

const usage = "usage: paragram <api|migrate>"

func run(log *slog.Logger) error {
	if len(os.Args) != 2 {
		return errors.New(usage)
	}
	cmd := os.Args[1]
	if cmd != "api" && cmd != "migrate" {
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

	checks := []api.Check{{Name: "postgres", Ping: pool.Ping}}
	return serve(ctx, log, cfg.Addr, api.New(log, checks, cfg.CORSOrigins))
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
