// Package storagetest gives tests an in-process S3 server.
package storagetest

import (
	"net/http/httptest"
	"testing"

	"github.com/johannesboyne/gofakes3"
	"github.com/johannesboyne/gofakes3/backend/s3mem"

	"github.com/kitsuneislife/paragram-api/internal/storage"
)

// New returns a Store backed by a fresh in-memory bucket.
func New(t testing.TB) *storage.Store {
	t.Helper()
	backend := s3mem.New()
	if err := backend.CreateBucket("media"); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(gofakes3.New(backend).Server())
	t.Cleanup(srv.Close)
	s, err := storage.New(storage.Config{
		Endpoint:  srv.URL,
		AccessKey: "test",
		SecretKey: "test-secret",
		Bucket:    "media",
		PublicURL: srv.URL + "/media",
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}
