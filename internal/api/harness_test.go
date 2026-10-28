package api

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/kitsuneislife/paragram-api/internal/db/dbtest"
	"github.com/kitsuneislife/paragram-api/internal/jobs"
	"github.com/kitsuneislife/paragram-api/internal/storage/storagetest"
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
	st := storagetest.New(t)
	// The same client inserts and works jobs, standing in for `paragram worker`.
	jc, err := jobs.NewClient(quiet, pool, st)
	if err != nil {
		t.Fatal(err)
	}
	if err := jc.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { jc.Stop(context.Background()) })
	srv := httptest.NewServer(New(Deps{
		Log:       quiet,
		Pool:      pool,
		Redis:     rdb,
		JWTSecret: []byte(strings.Repeat("k", 32)),
		Store:     st,
		Jobs:      jc,
	}))
	t.Cleanup(srv.Close)
	return &testAPI{t: t, url: srv.URL, pool: pool}
}

// upload creates a media for purpose and PUTs data to its upload URL.
func (a *testAPI) upload(token, purpose string, data []byte) string {
	a.t.Helper()
	var m struct {
		ID        string `json:"id"`
		UploadURL string `json:"upload_url"`
	}
	a.do(token, "POST", "/v1/media", map[string]string{"purpose": purpose}, http.StatusCreated, &m)
	req, _ := http.NewRequest("PUT", m.UploadURL, bytes.NewReader(data))
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		a.t.Fatalf("upload: status %d", res.StatusCode)
	}
	return m.ID
}

// waitMedia polls until the media leaves processing and returns it.
func (a *testAPI) waitMedia(token, id string) mediaJSON {
	a.t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		var m mediaJSON
		a.do(token, "GET", "/v1/media/"+id, nil, http.StatusOK, &m)
		if m.Status != "processing" || time.Now().After(deadline) {
			return m
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// pngImage is a small valid upload.
func pngImage(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, 64, 48))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// do sends a JSON request and fails the test unless the status is want.
// When out is non-nil the response body is decoded into it.
func (a *testAPI) do(token, method, path string, body any, want int, out any) *http.Response {
	a.t.Helper()
	return a.doKey(token, "", method, path, body, want, out)
}

// doKey is do with an Idempotency-Key header when key is not empty.
func (a *testAPI) doKey(token, key, method, path string, body any, want int, out any) *http.Response {
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
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
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
