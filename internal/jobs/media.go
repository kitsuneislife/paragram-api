// Package jobs holds the background work run by `paragram worker`, queued
// in Postgres by River in the same transaction as the write that needs it.
package jobs

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/riverqueue/river"

	"github.com/kitsuneislife/paragram-api/internal/db/store"
	"github.com/kitsuneislife/paragram-api/internal/media"
	"github.com/kitsuneislife/paragram-api/internal/storage"
)

// UploadKey is where the client PUTs the original file.
func UploadKey(id uuid.UUID) string { return "uploads/" + id.String() }

// VariantKey is where the worker writes one processed size.
func VariantKey(id uuid.UUID, size string) string {
	return "media/" + id.String() + "/" + size + ".jpg"
}

type ProcessMediaArgs struct {
	MediaID uuid.UUID `json:"media_id"`
}

func (ProcessMediaArgs) Kind() string { return "process_media" }

func (ProcessMediaArgs) InsertOpts() river.InsertOpts { return river.InsertOpts{MaxAttempts: 5} }

type ProcessMedia struct {
	river.WorkerDefaults[ProcessMediaArgs]
	Pool  *pgxpool.Pool
	Store *storage.Store
}

func (w *ProcessMedia) Work(ctx context.Context, job *river.Job[ProcessMediaArgs]) error {
	err := w.process(ctx, job.Args.MediaID)
	if err == nil {
		return nil
	}
	// A bad file never gets better; a transient error is retried until the last attempt.
	if errors.Is(err, media.ErrUnsupported) || errors.Is(err, storage.ErrNotFound) || job.Attempt >= job.MaxAttempts {
		if ferr := store.New(w.Pool).MediaFailed(ctx, job.Args.MediaID); ferr != nil {
			return errors.Join(err, ferr)
		}
		return river.JobCancel(err)
	}
	return err
}

func (w *ProcessMedia) process(ctx context.Context, id uuid.UUID) error {
	q := store.New(w.Pool)
	m, err := q.GetMedia(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil // owner deleted since; nothing to do
	}
	if err != nil {
		return err
	}
	if m.Status != "processing" {
		return nil // already finished by an earlier attempt
	}

	size, err := w.Store.Size(ctx, UploadKey(id))
	if err != nil {
		return err
	}
	if size > media.MaxBytes {
		return fmt.Errorf("%w: %d bytes", media.ErrUnsupported, size)
	}
	obj, err := w.Store.Get(ctx, UploadKey(id))
	if err != nil {
		return err
	}
	defer obj.Close()
	variants, err := media.Process(obj)
	if err != nil {
		return err
	}
	var full media.Variant
	for _, v := range variants {
		if err := w.Store.Put(ctx, VariantKey(id, v.Name), v.JPEG, "image/jpeg"); err != nil {
			return err
		}
		full = v // Sizes ends with the largest
	}

	return pgx.BeginFunc(ctx, w.Pool, func(tx pgx.Tx) error {
		q := q.WithTx(tx)
		if err := q.MediaReady(ctx, store.MediaReadyParams{ID: id, Width: int32(full.Width), Height: int32(full.Height)}); err != nil {
			return err
		}
		if m.Purpose == "avatar" {
			return q.SetAvatar(ctx, store.SetAvatarParams{UserID: m.OwnerID, MediaID: uuid.NullUUID{UUID: id, Valid: true}})
		}
		return nil
	})
}
