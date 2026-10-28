package storage_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/kitsuneislife/paragram-api/internal/storage"
	"github.com/kitsuneislife/paragram-api/internal/storage/storagetest"
)

func TestPresignedUploadRoundTrip(t *testing.T) {
	s := storagetest.New(t)
	ctx := context.Background()
	if err := s.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Size(ctx, "uploads/x"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("missing object: err = %v", err)
	}

	u, err := s.PresignPut(ctx, "uploads/x", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest("PUT", u, strings.NewReader("hello"))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("upload status = %d", resp.StatusCode)
	}

	if n, err := s.Size(ctx, "uploads/x"); err != nil || n != 5 {
		t.Fatalf("size = %d, %v", n, err)
	}
	if err := s.Put(ctx, "media/x/thumb.jpg", []byte("jpeg"), "image/jpeg"); err != nil {
		t.Fatal(err)
	}
	r, err := s.Get(ctx, "media/x/thumb.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if b, _ := io.ReadAll(r); string(b) != "jpeg" {
		t.Errorf("get = %q", b)
	}
	if got := s.URL("media/x/thumb.jpg"); !strings.HasSuffix(got, "/media/media/x/thumb.jpg") {
		t.Errorf("url = %q", got)
	}
}
