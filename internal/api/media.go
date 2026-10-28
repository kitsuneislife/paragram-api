package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/kitsuneislife/paragram-api/internal/db/store"
	"github.com/kitsuneislife/paragram-api/internal/jobs"
	"github.com/kitsuneislife/paragram-api/internal/media"
	"github.com/kitsuneislife/paragram-api/internal/storage"
)

const uploadTTL = 15 * time.Minute

type mediaURLs struct {
	Thumb  string `json:"thumb"`
	Medium string `json:"medium"`
	Full   string `json:"full"`
}

type mediaJSON struct {
	ID     string     `json:"id"`
	Status string     `json:"status"`
	Width  int32      `json:"width,omitempty"`
	Height int32      `json:"height,omitempty"`
	URLs   *mediaURLs `json:"urls,omitempty"` // set once ready
}

func (s *server) urls(id uuid.UUID) *mediaURLs {
	return &mediaURLs{
		Thumb:  s.Store.URL(jobs.VariantKey(id, "thumb")),
		Medium: s.Store.URL(jobs.VariantKey(id, "medium")),
		Full:   s.Store.URL(jobs.VariantKey(id, "full")),
	}
}

func (s *server) mediaJSON(m store.Media) mediaJSON {
	j := mediaJSON{ID: m.ID.String(), Status: m.Status}
	if m.Status == "ready" {
		j.Width, j.Height, j.URLs = m.Width.Int32, m.Height.Int32, s.urls(m.ID)
	}
	return j
}

func (s *server) createMedia(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Purpose string `json:"purpose"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Purpose != "post" && in.Purpose != "avatar" {
		writeInvalid(w, []FieldError{{Field: "purpose", Code: "invalid"}})
		return
	}
	m, err := s.q.CreateMedia(r.Context(), store.CreateMediaParams{
		ID:      uuid.Must(uuid.NewV7()),
		OwnerID: caller(r).UserID,
		Purpose: in.Purpose,
	})
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	u, err := s.Store.PresignPut(r.Context(), jobs.UploadKey(m.ID), uploadTTL)
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, struct {
		mediaJSON
		UploadURL       string    `json:"upload_url"`
		UploadExpiresAt time.Time `json:"upload_expires_at"`
	}{s.mediaJSON(m), u, time.Now().Add(uploadTTL).UTC()})
}

// getMedia lets the owner poll processing status.
func (s *server) getMedia(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	m, err := s.q.GetMedia(r.Context(), id)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && m.OwnerID != caller(r).UserID) {
		writeProblem(w, http.StatusNotFound, "not-found", "media not found")
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, s.mediaJSON(m))
}

// errMediaInvalid means a media id given by the client cannot be attached.
var errMediaInvalid = errors.New("media not attachable")

// checkUploads fails with errMediaInvalid unless every upload exists and is
// within the size limit. It runs before the transaction: storage calls are slow.
func (s *server) checkUploads(ctx context.Context, ids []uuid.UUID) error {
	for _, id := range ids {
		n, err := s.Store.Size(ctx, jobs.UploadKey(id))
		if errors.Is(err, storage.ErrNotFound) || (err == nil && n > media.MaxBytes) {
			return errMediaInvalid
		}
		if err != nil {
			return err
		}
	}
	return nil
}

// attachMedia claims the caller's pending media for purpose and queues their
// processing in the same transaction.
func (s *server) attachMedia(ctx context.Context, tx pgx.Tx, q *store.Queries, owner uuid.UUID, purpose string, ids []uuid.UUID) error {
	claimed, err := q.StartProcessing(ctx, store.StartProcessingParams{Ids: ids, OwnerID: owner, Purpose: purpose})
	if err != nil {
		return err
	}
	if len(claimed) != len(ids) {
		return errMediaInvalid
	}
	params := make([]river.InsertManyParams, len(ids))
	for i, id := range ids {
		params[i] = river.InsertManyParams{Args: jobs.ProcessMediaArgs{MediaID: id}}
	}
	if _, err := s.Jobs.InsertManyTx(ctx, tx, params); err != nil {
		return fmt.Errorf("queue media: %w", err)
	}
	return nil
}
