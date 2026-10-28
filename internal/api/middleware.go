package api

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"time"
)

const (
	maxBodyBytes   = 1 << 20 // media goes straight to storage, never through the API
	requestTimeout = 10 * time.Second
)

type ctxKey struct{}

// RequestID returns the ID assigned to the request by withRequestID.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(ctxKey{}).(string)
	return id
}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// withRequestID keeps a client-sent X-Request-Id when it is safe to log,
// otherwise generates one, and echoes it back on the response.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !validRequestID.MatchString(id) {
			id = rand.Text()
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, id)))
	})
}

// statusWriter records the status code for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach Flush/Hijack on the real writer.
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

func withLogging(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		log.LogAttrs(r.Context(), slog.LevelInfo, "request",
			slog.String("request_id", RequestID(r.Context())),
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.Duration("duration", time.Since(start)),
		)
	})
}

// withRecover turns a handler panic into a 500 without leaking details.
func withRecover(log *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			v := recover()
			if v == nil {
				return
			}
			if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
				panic(v)
			}
			id := RequestID(r.Context())
			log.Error("panic", "request_id", id, "panic", v, "stack", string(debug.Stack()))
			writeProblem(w, http.StatusInternalServerError, "internal", "request id "+id)
		}()
		next.ServeHTTP(w, r)
	})
}

// withCORS answers browsers from allowed origins. Native apps send no Origin
// and are unaffected.
func withCORS(origins []string, next http.Handler) http.Handler {
	allowed := make(map[string]bool, len(origins))
	for _, o := range origins {
		allowed[o] = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("Vary", "Origin")
		origin := r.Header.Get("Origin")
		if !allowed[origin] {
			next.ServeHTTP(w, r)
			return
		}
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Expose-Headers", "X-Request-Id, ETag, Retry-After, RateLimit-Limit, RateLimit-Remaining, RateLimit-Reset")
		if r.Method == http.MethodOptions && r.Header.Get("Access-Control-Request-Method") != "" {
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Idempotency-Key, If-None-Match, X-Request-Id")
			h.Set("Access-Control-Max-Age", "600")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// etagWriter holds back the response so its ETag can be computed.
type etagWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (w *etagWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *etagWriter) Write(b []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	return w.body.Write(b)
}

// withETag tags successful GET responses with a hash of their body and
// answers 304 when the client already has it.
// shortcut: the handler still does all its work, store versions in the DB if reads get hot.
func withETag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}
		ew := &etagWriter{ResponseWriter: w}
		next.ServeHTTP(ew, r)
		if ew.status == 0 {
			ew.status = http.StatusOK
		}
		if ew.status == http.StatusOK {
			sum := sha256.Sum256(ew.body.Bytes())
			tag := `"` + hex.EncodeToString(sum[:16]) + `"`
			w.Header().Set("ETag", tag)
			if etagMatches(r.Header.Get("If-None-Match"), tag) {
				w.Header().Del("Content-Type")
				w.WriteHeader(http.StatusNotModified)
				return
			}
		}
		w.WriteHeader(ew.status)
		_, _ = w.Write(ew.body.Bytes())
	})
}

// etagMatches applies If-None-Match's weak comparison.
func etagMatches(header, tag string) bool {
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimPrefix(strings.TrimSpace(t), "W/")
		if t == tag || t == "*" {
			return true
		}
	}
	return false
}

func withLimits(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
