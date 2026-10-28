package api

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kitsuneislife/paragram-api/internal/db/store"
)

type commentJSON struct {
	ID        string    `json:"id"`
	PostID    string    `json:"post_id"`
	Author    userJSON  `json:"author"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *server) commentJSON(c store.Comment, author store.User) commentJSON {
	return commentJSON{
		ID:        c.ID.String(),
		PostID:    c.PostID.String(),
		Author:    s.publicUser(author),
		Body:      c.Body,
		CreatedAt: c.CreatedAt.UTC(),
	}
}

// visiblePost reads the {id} path post and answers 404 unless it is ready.
// On failure it has already answered.
func (s *server) visiblePost(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return id, false
	}
	visible, err := s.q.PostVisible(r.Context(), id)
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return id, false
	}
	if !visible {
		writeProblem(w, http.StatusNotFound, "not-found", "post not found")
	}
	return id, visible
}

// like and unlike are idempotent: repeating one changes nothing.
func (s *server) like(w http.ResponseWriter, r *http.Request) {
	s.setLike(w, r, true)
}

func (s *server) unlike(w http.ResponseWriter, r *http.Request) {
	s.setLike(w, r, false)
}

func (s *server) setLike(w http.ResponseWriter, r *http.Request, on bool) {
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	p := store.LikeParams{UserID: caller(r).UserID, PostID: post}
	err := s.inTx(ctx, func(_ pgx.Tx, q *store.Queries) error {
		var err error
		delta := int32(1)
		if on {
			_, err = q.Like(ctx, p)
		} else {
			_, err = q.Unlike(ctx, store.UnlikeParams(p))
			delta = -1
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already in the wanted state
		}
		if err != nil {
			return err
		}
		return q.AddLikesCount(ctx, store.AddLikesCountParams{ID: post, Delta: delta})
	})
	if isFKViolation(err) {
		// Deleted since visiblePost looked.
		writeProblem(w, http.StatusNotFound, "not-found", "post not found")
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) createComment(w http.ResponseWriter, r *http.Request) {
	key, ok := idempotencyKey(w, r)
	if !ok {
		return
	}
	var in struct {
		Body string `json:"body"`
	}
	if !decode(w, r, &in) {
		return
	}
	var v fieldErrors
	body := strings.TrimSpace(in.Body)
	v.check(body != "", "body", "too_short")
	v.maxLen(body, maxComment, "body")
	if len(v) > 0 {
		writeInvalid(w, v)
		return
	}
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}

	ctx, me := r.Context(), caller(r).UserID
	byKey := store.CommentByKeyParams{AuthorID: me, IdempotencyKey: key}
	var c store.Comment
	err := s.inTx(ctx, func(_ pgx.Tx, q *store.Queries) error {
		var err error
		c, err = q.CreateComment(ctx, store.CreateCommentParams{
			ID: uuid.Must(uuid.NewV7()), PostID: post, AuthorID: me, Body: body, IdempotencyKey: key,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			// A retry, or a concurrent request with the same key won.
			c, err = q.CommentByKey(ctx, byKey)
			return err
		}
		if err != nil {
			return err
		}
		return q.AddCommentsCount(ctx, store.AddCommentsCountParams{ID: post, Delta: 1})
	})
	if isFKViolation(err) {
		writeProblem(w, http.StatusNotFound, "not-found", "post not found")
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	author, err := s.q.GetUser(ctx, c.AuthorID)
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.commentJSON(c, author))
}

// listComments pages through a post's comments, oldest first.
func (s *server) listComments(w http.ResponseWriter, r *http.Request) {
	c, limit, ok := pageQuery(w, r, oldestFirst)
	if !ok {
		return
	}
	post, ok := s.visiblePost(w, r)
	if !ok {
		return
	}
	rows, err := s.q.ListComments(r.Context(), store.ListCommentsParams{
		PostID:     post,
		CursorTime: c.Time,
		CursorID:   c.ID,
		Lim:        int32(limit + 1),
	})
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	rows, next := trim(rows, limit, func(c store.ListCommentsRow) cursor { return cursor{c.Comment.CreatedAt, c.Comment.ID} })
	out := make([]commentJSON, len(rows))
	for i, row := range rows {
		out[i] = s.commentJSON(row.Comment, row.User)
	}
	writeJSON(w, http.StatusOK, page[commentJSON]{Data: out, NextCursor: next})
}

// deleteComment lets the comment's author or the post's author remove it.
func (s *server) deleteComment(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	ctx := r.Context()
	err := s.inTx(ctx, func(_ pgx.Tx, q *store.Queries) error {
		post, err := q.DeleteComment(ctx, store.DeleteCommentParams{ID: id, UserID: caller(r).UserID})
		if err != nil {
			return err
		}
		return q.AddCommentsCount(ctx, store.AddCommentsCountParams{ID: post, Delta: -1})
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not-found", "comment not found")
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
