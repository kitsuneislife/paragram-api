package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kitsuneislife/paragram-api/internal/db/store"
)

type postJSON struct {
	ID     string      `json:"id"`
	Author userJSON    `json:"author"`
	Media  []mediaJSON `json:"media"`
	// False until every media is processed. Only the author sees such posts.
	Ready         bool      `json:"ready"`
	Caption       string    `json:"caption"`
	LikesCount    int32     `json:"likes_count"`
	CommentsCount int32     `json:"comments_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// loadPosts returns the posts named by ids, in that order, skipping missing ones.
func (s *server) loadPosts(ctx context.Context, ids []uuid.UUID) ([]postJSON, error) {
	rows, err := s.q.GetPosts(ctx, ids)
	if err != nil {
		return nil, err
	}
	media, err := s.q.ListPostMedia(ctx, ids)
	if err != nil {
		return nil, err
	}
	byPost := make(map[uuid.UUID][]mediaJSON, len(ids))
	for _, m := range media {
		byPost[m.PostID] = append(byPost[m.PostID], s.mediaJSON(m.Media))
	}
	byID := make(map[uuid.UUID]postJSON, len(rows))
	for _, r := range rows {
		byID[r.Post.ID] = postJSON{
			ID:            r.Post.ID.String(),
			Author:        s.publicUser(r.User),
			Media:         byPost[r.Post.ID],
			Ready:         r.Ready,
			Caption:       r.Post.Caption,
			LikesCount:    r.Post.LikesCount,
			CommentsCount: r.Post.CommentsCount,
			CreatedAt:     r.Post.CreatedAt.UTC(),
		}
	}
	out := make([]postJSON, 0, len(ids))
	for _, id := range ids {
		if p, ok := byID[id]; ok {
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *server) createPost(w http.ResponseWriter, r *http.Request) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var in struct {
		Caption  string   `json:"caption"`
		MediaIDs []string `json:"media_ids"`
	}
	if !decode(w, r, &in) {
		return
	}
	var v fieldErrors
	caption := strings.TrimSpace(in.Caption)
	v.maxLen(caption, maxCaption, "caption")
	ids, idsOK := parseIDs(in.MediaIDs)
	v.check(idsOK && len(ids) >= 1 && len(ids) <= maxPostMedia, "media_ids", "invalid")
	if len(v) > 0 {
		writeInvalid(w, v)
		return
	}

	ctx, me := r.Context(), caller(r).UserID
	if key.Valid {
		id, err := s.q.PostIDByKey(ctx, store.PostIDByKeyParams{AuthorID: me, IdempotencyKey: key})
		if err == nil {
			s.sendPost(w, r, id, http.StatusCreated)
			return
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			writeInternal(w, r, s.Log, err)
			return
		}
	}
	errMedia := []FieldError{{Field: "media_ids", Code: "invalid"}}
	if err := s.checkUploads(ctx, ids); errors.Is(err, errMediaInvalid) {
		writeInvalid(w, errMedia)
		return
	} else if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	id := uuid.Must(uuid.NewV7())
	err := s.inTx(ctx, func(tx pgx.Tx, q *store.Queries) error {
		_, err := q.CreatePost(ctx, store.CreatePostParams{ID: id, AuthorID: me, Caption: caption, IdempotencyKey: key})
		if errors.Is(err, pgx.ErrNoRows) {
			// A concurrent request with the same key won.
			id, err = q.PostIDByKey(ctx, store.PostIDByKeyParams{AuthorID: me, IdempotencyKey: key})
			return err
		}
		if err != nil {
			return err
		}
		if err := s.attachMedia(ctx, tx, q, me, "post", ids); err != nil {
			return err
		}
		if err := q.AddPostMedia(ctx, store.AddPostMediaParams{PostID: id, MediaIds: ids}); err != nil {
			return err
		}
		// shortcut: counts posts still processing or failed, count only ready ones if users notice.
		return q.AddPostsCount(ctx, store.AddPostsCountParams{ID: me, Delta: 1})
	})
	if errors.Is(err, errMediaInvalid) {
		writeInvalid(w, errMedia)
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	s.sendPost(w, r, id, http.StatusCreated)
}

func (s *server) getPost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	s.sendPost(w, r, id, http.StatusOK)
}

// sendPost answers with the post, or 404 when the caller may not see it.
func (s *server) sendPost(w http.ResponseWriter, r *http.Request, id uuid.UUID, status int) {
	posts, err := s.loadPosts(r.Context(), []uuid.UUID{id})
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	if len(posts) == 0 || (!posts[0].Ready && posts[0].Author.ID != caller(r).UserID.String()) {
		writeProblem(w, http.StatusNotFound, "not-found", "post not found")
		return
	}
	writeJSON(w, status, posts[0])
}

func (s *server) deletePost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	me := caller(r).UserID
	err := s.inTx(r.Context(), func(_ pgx.Tx, q *store.Queries) error {
		if _, err := q.DeletePost(r.Context(), store.DeletePostParams{ID: id, AuthorID: me}); err != nil {
			return err
		}
		return q.AddPostsCount(r.Context(), store.AddPostsCountParams{ID: me, Delta: -1})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not-found", "post not found")
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listUserPosts is a profile grid, newest first.
func (s *server) listUserPosts(w http.ResponseWriter, r *http.Request) {
	author, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	c, limit, ok := pageQuery(w, r, newestFirst)
	if !ok {
		return
	}
	if _, err := s.q.GetUser(r.Context(), author); errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not-found", "user not found")
		return
	} else if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	rows, err := s.q.ListUserPosts(r.Context(), store.ListUserPostsParams{
		AuthorID:       author,
		CursorTime:     c.Time,
		CursorID:       c.ID,
		IncludeUnready: author == caller(r).UserID,
		Lim:            int32(limit + 1),
	})
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	rows, next := trim(rows, limit, func(p store.ListUserPostsRow) cursor { return cursor{p.CreatedAt, p.ID} })
	ids := make([]uuid.UUID, len(rows))
	for i, p := range rows {
		ids[i] = p.ID
	}
	s.sendPostPage(w, r, ids, next)
}

func (s *server) sendPostPage(w http.ResponseWriter, r *http.Request, ids []uuid.UUID, next *string) {
	posts, err := s.loadPosts(r.Context(), ids)
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, page[postJSON]{Data: posts, NextCursor: next})
}

// idempotencyKey reads the optional Idempotency-Key header. On failure it has
// already answered.
func idempotencyKey(w http.ResponseWriter, r *http.Request) (uuid.NullUUID, bool) {
	h := r.Header.Get("Idempotency-Key")
	if h == "" {
		return uuid.NullUUID{}, true
	}
	k, err := uuid.Parse(h)
	if err != nil {
		writeInvalid(w, []FieldError{{Field: "Idempotency-Key", Code: "invalid"}})
		return uuid.NullUUID{}, false
	}
	return uuid.NullUUID{UUID: k, Valid: true}, true
}
