// Package api is the HTTP layer: routing, middleware and response format.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/riverqueue/river"

	"github.com/kitsuneislife/paragram-api/internal/db/store"
	"github.com/kitsuneislife/paragram-api/internal/storage"
)

// Check is a dependency probed by /readyz.
type Check struct {
	Name string
	Ping func(context.Context) error
}

// Deps is everything the HTTP layer needs from the outside world.
type Deps struct {
	Log         *slog.Logger
	Checks      []Check
	CORSOrigins []string
	Pool        *pgxpool.Pool
	Redis       *redis.Client
	JWTSecret   []byte
	Store       *storage.Store
	Jobs        *river.Client[pgx.Tx] // inserts only; `paragram worker` runs them
}

type server struct {
	Deps
	q *store.Queries
}

// New builds the full HTTP handler with middleware applied.
func New(d Deps) http.Handler {
	s := &server{Deps: d}
	if d.Pool != nil {
		s.q = store.New(d.Pool)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", readyz(d.Log, d.Checks))

	// One shared budget per IP across these slows password guessing and signup spam.
	limitAuth := func(h http.HandlerFunc) http.HandlerFunc { return s.rateLimit("auth", 20, time.Minute, h) }
	mux.HandleFunc("POST /v1/auth/register", limitAuth(s.register))
	mux.HandleFunc("POST /v1/auth/login", limitAuth(s.login))
	mux.HandleFunc("POST /v1/auth/refresh", limitAuth(s.refresh))
	mux.HandleFunc("POST /v1/auth/logout", s.authed(s.logout))

	mux.HandleFunc("GET /v1/me", s.authed(s.getMe))
	mux.HandleFunc("PATCH /v1/me", s.authed(s.updateMe))
	mux.HandleFunc("GET /v1/users/{id}", s.authed(s.getUser))
	// Not under /v1/users/: "/v1/users/by-username/posts" would be ambiguous with /v1/users/{id}/posts.
	mux.HandleFunc("GET /v1/usernames/{username}", s.authed(s.getUserByUsername))

	mux.HandleFunc("POST /v1/media", s.authed(s.rateLimit("media", 60, time.Minute, s.createMedia)))
	mux.HandleFunc("GET /v1/media/{id}", s.authed(s.getMedia))

	mux.HandleFunc("POST /v1/posts", s.authed(s.rateLimit("posts", 30, time.Minute, s.createPost)))
	mux.HandleFunc("GET /v1/posts/{id}", s.authed(s.getPost))
	mux.HandleFunc("DELETE /v1/posts/{id}", s.authed(s.deletePost))
	mux.HandleFunc("GET /v1/users/{id}/posts", s.authed(s.listUserPosts))

	mux.HandleFunc("PUT /v1/users/{id}/follow", s.authed(s.follow))
	mux.HandleFunc("DELETE /v1/users/{id}/follow", s.authed(s.unfollow))
	mux.HandleFunc("GET /v1/users/{id}/followers", s.authed(s.listFollowers))
	mux.HandleFunc("GET /v1/users/{id}/following", s.authed(s.listFollowing))
	mux.HandleFunc("GET /v1/feed", s.authed(s.feed))

	mux.HandleFunc("PUT /v1/posts/{id}/like", s.authed(s.like))
	mux.HandleFunc("DELETE /v1/posts/{id}/like", s.authed(s.unlike))
	mux.HandleFunc("POST /v1/posts/{id}/comments", s.authed(s.rateLimit("comments", 60, time.Minute, s.createComment)))
	mux.HandleFunc("GET /v1/posts/{id}/comments", s.authed(s.listComments))
	mux.HandleFunc("DELETE /v1/comments/{id}", s.authed(s.deleteComment))

	// shortcut: catch-all also answers wrong-method requests with 404 instead of 405, revisit if clients need 405.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, http.StatusNotFound, "not-found", "")
	})

	var h http.Handler = mux
	h = withETag(h)
	h = withLimits(h)
	h = withCORS(d.CORSOrigins, h)
	h = withRecover(d.Log, h)
	h = withLogging(d.Log, h)
	h = withRequestID(h)
	return h
}

func readyz(log *slog.Logger, checks []Check) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		var failed []string
		for _, c := range checks {
			if err := c.Ping(ctx); err != nil {
				log.Warn("readiness check failed", "check", c.Name, "err", err)
				failed = append(failed, c.Name)
			}
		}
		if len(failed) > 0 {
			writeProblem(w, http.StatusServiceUnavailable, "not-ready", "unavailable: "+strings.Join(failed, ", "))
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}
}
