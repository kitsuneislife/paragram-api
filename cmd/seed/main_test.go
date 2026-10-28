package main

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	paragram "github.com/kitsuneislife/paragram-api/internal/api"
	"github.com/kitsuneislife/paragram-api/internal/db/dbtest"
	"github.com/kitsuneislife/paragram-api/internal/jobs"
	"github.com/kitsuneislife/paragram-api/internal/storage/storagetest"
)

// TestSeed runs the seed twice against the real API and worker.
func TestSeed(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	pool := dbtest.New(t)
	st := storagetest.New(t)
	jc, err := jobs.NewClient(quiet, pool, st)
	if err != nil {
		t.Fatal(err)
	}
	if err := jc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { jc.Stop(context.Background()) })
	srv := httptest.NewServer(paragram.New(paragram.Deps{
		Log:       quiet,
		Pool:      pool,
		Redis:     redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()}),
		JWTSecret: []byte(strings.Repeat("k", 32)),
		Store:     st,
		Jobs:      jc,
	}))
	t.Cleanup(srv.Close)
	api = srv.URL

	seed(3, 2)
	seed(3, 1) // logs in instead of registering

	token, _ := session("seed1")
	var feed struct{ Data []struct{ Ready bool } }
	call(token, "GET", "/v1/feed?limit=100", nil, http.StatusOK, &feed)
	// seed1 follows both others, so the feed holds all 3 users' 3 posts.
	if len(feed.Data) != 9 {
		t.Fatalf("feed has %d posts, want 9", len(feed.Data))
	}
	for _, p := range feed.Data {
		if !p.Ready {
			t.Fatalf("unready post in feed")
		}
	}
}
