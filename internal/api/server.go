// Package api is the HTTP layer: routing, middleware and response format.
package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

// Check is a dependency probed by /readyz.
type Check struct {
	Name string
	Ping func(context.Context) error
}

// New builds the full HTTP handler with middleware applied.
func New(log *slog.Logger, checks []Check, corsOrigins []string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", readyz(log, checks))
	// shortcut: catch-all also answers wrong-method requests with 404 instead of 405, revisit if clients need 405.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		writeProblem(w, http.StatusNotFound, "not-found", "")
	})

	var h http.Handler = mux
	h = withLimits(h)
	h = withCORS(corsOrigins, h)
	h = withRecover(log, h)
	h = withLogging(log, h)
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
