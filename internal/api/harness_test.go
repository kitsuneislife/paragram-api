package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/kitsuneislife/paragram-api/internal/db/dbtest"
)

// testAPI is the real handler on a real server, over a fresh database.
type testAPI struct {
	t    *testing.T
	url  string
	pool *pgxpool.Pool
}

func newTestAPI(t *testing.T) *testAPI {
	t.Helper()
	pool := dbtest.New(t)
	rdb := redis.NewClient(&redis.Options{Addr: miniredis.RunT(t).Addr()})
	srv := httptest.NewServer(New(Deps{
		Log:       quiet,
		Pool:      pool,
		Redis:     rdb,
		JWTSecret: []byte(strings.Repeat("k", 32)),
	}))
	t.Cleanup(srv.Close)
	return &testAPI{t: t, url: srv.URL, pool: pool}
}

// do sends a JSON request and fails the test unless the status is want.
// When out is non-nil the response body is decoded into it.
func (a *testAPI) do(token, method, path string, body any, want int, out any) *http.Response {
	a.t.Helper()
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			a.t.Fatal(err)
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, a.url+path, rd)
	if err != nil {
		a.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	if res.StatusCode != want {
		a.t.Fatalf("%s %s: status %d, want %d; body: %s", method, path, res.StatusCode, want, raw)
	}
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			a.t.Fatalf("%s %s: decode: %v; body: %s", method, path, err, raw)
		}
	}
	return res
}

// register creates a user named username (password "password123").
func (a *testAPI) register(username string) authJSON {
	a.t.Helper()
	var out authJSON
	a.do("", "POST", "/v1/auth/register", map[string]string{
		"username": username,
		"email":    username + "@example.com",
		"password": "password123",
	}, http.StatusCreated, &out)
	return out
}
