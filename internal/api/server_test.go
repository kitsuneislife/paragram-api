package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

var quiet = slog.New(slog.NewTextHandler(io.Discard, nil))

func serve(h http.Handler, r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func decodeProblem(t *testing.T, rec *httptest.ResponseRecorder) Problem {
	t.Helper()
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Fatalf("Content-Type = %q", ct)
	}
	var p Problem
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestHealthz(t *testing.T) {
	rec := serve(New(quiet, nil, nil), httptest.NewRequest("GET", "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestReadyz(t *testing.T) {
	ok := Check{"db", func(context.Context) error { return nil }}
	down := Check{"redis", func(context.Context) error { return errors.New("down") }}

	rec := serve(New(quiet, []Check{ok}, nil), httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("all up: status = %d", rec.Code)
	}

	rec = serve(New(quiet, []Check{ok, down}, nil), httptest.NewRequest("GET", "/readyz", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("one down: status = %d", rec.Code)
	}
	if p := decodeProblem(t, rec); !strings.Contains(p.Detail, "redis") || strings.Contains(p.Detail, "db") {
		t.Errorf("detail = %q, want only redis", p.Detail)
	}
}

func TestUnknownRouteIsProblem(t *testing.T) {
	rec := serve(New(quiet, nil, nil), httptest.NewRequest("GET", "/nope", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d", rec.Code)
	}
	if p := decodeProblem(t, rec); p.Status != 404 || p.Type != problemTypeBase+"not-found" {
		t.Errorf("problem = %+v", p)
	}
}

func TestRequestID(t *testing.T) {
	h := New(quiet, nil, nil)

	r := httptest.NewRequest("GET", "/healthz", nil)
	r.Header.Set("X-Request-Id", "abc-123")
	if got := serve(h, r).Header().Get("X-Request-Id"); got != "abc-123" {
		t.Errorf("kept id = %q", got)
	}

	r = httptest.NewRequest("GET", "/healthz", nil)
	r.Header.Set("X-Request-Id", "bad id\nwith newline")
	if got := serve(h, r).Header().Get("X-Request-Id"); got == "" || strings.ContainsAny(got, " \n") {
		t.Errorf("replaced id = %q", got)
	}
}

func TestRecoverHidesPanic(t *testing.T) {
	h := withRequestID(withRecover(quiet, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("secret detail")
	})))
	rec := serve(h, httptest.NewRequest("GET", "/", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d", rec.Code)
	}
	p := decodeProblem(t, rec)
	if strings.Contains(p.Detail, "secret") || !strings.Contains(p.Detail, rec.Header().Get("X-Request-Id")) {
		t.Errorf("detail = %q", p.Detail)
	}
}

func TestCORS(t *testing.T) {
	h := New(quiet, nil, []string{"https://app.example"})

	r := httptest.NewRequest("OPTIONS", "/healthz", nil)
	r.Header.Set("Origin", "https://app.example")
	r.Header.Set("Access-Control-Request-Method", "POST")
	rec := serve(h, r)
	if rec.Code != http.StatusNoContent || rec.Header().Get("Access-Control-Allow-Origin") != "https://app.example" {
		t.Errorf("preflight: status %d, allow-origin %q", rec.Code, rec.Header().Get("Access-Control-Allow-Origin"))
	}

	r = httptest.NewRequest("GET", "/healthz", nil)
	r.Header.Set("Origin", "https://evil.example")
	if got := serve(h, r).Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("unknown origin got allow-origin %q", got)
	}
}
