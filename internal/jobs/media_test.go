package jobs

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"testing"

	"github.com/google/uuid"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/kitsuneislife/paragram-api/internal/db/dbtest"
	"github.com/kitsuneislife/paragram-api/internal/db/store"
	"github.com/kitsuneislife/paragram-api/internal/storage/storagetest"
)

func TestProcessMedia(t *testing.T) {
	ctx := context.Background()
	pool := dbtest.New(t)
	st := storagetest.New(t)
	q := store.New(pool)
	w := &ProcessMedia{Pool: pool, Store: st}

	user, err := q.CreateUser(ctx, store.CreateUserParams{ID: uuid.Must(uuid.NewV7()), Username: "u", Email: "u@example.com", PasswordHash: "x"})
	if err != nil {
		t.Fatal(err)
	}
	// newMedia makes a processing media row whose upload holds data.
	newMedia := func(purpose string, data []byte) uuid.UUID {
		id := uuid.Must(uuid.NewV7())
		if _, err := q.CreateMedia(ctx, store.CreateMediaParams{ID: id, OwnerID: user.ID, Purpose: purpose}); err != nil {
			t.Fatal(err)
		}
		if _, err := q.StartProcessing(ctx, store.StartProcessingParams{Ids: []uuid.UUID{id}, OwnerID: user.ID, Purpose: purpose}); err != nil {
			t.Fatal(err)
		}
		if err := st.Put(ctx, UploadKey(id), data, "application/octet-stream"); err != nil {
			t.Fatal(err)
		}
		return id
	}
	work := func(id uuid.UUID) error {
		return w.Work(ctx, &river.Job[ProcessMediaArgs]{
			JobRow: &rivertype.JobRow{Attempt: 1, MaxAttempts: 5},
			Args:   ProcessMediaArgs{MediaID: id},
		})
	}

	var img bytes.Buffer
	if err := png.Encode(&img, image.NewGray(image.Rect(0, 0, 400, 300))); err != nil {
		t.Fatal(err)
	}
	good := newMedia("avatar", img.Bytes())
	if err := work(good); err != nil {
		t.Fatal(err)
	}
	m, _ := q.GetMedia(ctx, good)
	if m.Status != "ready" || m.Width.Int32 != 400 || m.Height.Int32 != 300 {
		t.Errorf("media = %+v", m)
	}
	if u, _ := q.GetUser(ctx, user.ID); u.AvatarMediaID.UUID != good {
		t.Errorf("avatar = %v", u.AvatarMediaID)
	}
	if n, err := st.Size(ctx, VariantKey(good, "thumb")); err != nil || n == 0 {
		t.Errorf("thumb: %d, %v", n, err)
	}
	// A retry after success is a no-op.
	if err := work(good); err != nil {
		t.Fatal(err)
	}

	bad := newMedia("post", []byte("not an image"))
	if err := work(bad); !errors.As(err, new(*river.JobCancelError)) {
		t.Fatalf("bad upload: err = %v, want JobCancel", err)
	}
	if m, _ := q.GetMedia(ctx, bad); m.Status != "failed" {
		t.Errorf("bad status = %s", m.Status)
	}
}
