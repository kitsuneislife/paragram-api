package api

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kitsuneislife/paragram-api/internal/db/store"
)

// follow and unfollow are idempotent: repeating one changes nothing.
func (s *server) follow(w http.ResponseWriter, r *http.Request) {
	s.setFollow(w, r, true)
}

func (s *server) unfollow(w http.ResponseWriter, r *http.Request) {
	s.setFollow(w, r, false)
}

func (s *server) setFollow(w http.ResponseWriter, r *http.Request, on bool) {
	target, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	me := caller(r).UserID
	if target == me {
		writeInvalid(w, []FieldError{{Field: "id", Code: "self"}})
		return
	}
	ctx := r.Context()
	p := store.FollowParams{FollowerID: me, FolloweeID: target}
	err := s.inTx(ctx, func(_ pgx.Tx, q *store.Queries) error {
		var err error
		delta := int32(1)
		if on {
			_, err = q.Follow(ctx, p)
		} else {
			_, err = q.Unfollow(ctx, store.UnfollowParams(p))
			delta = -1
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // already in the wanted state
		}
		if err != nil {
			return err
		}
		return q.AddFollowCounts(ctx, store.AddFollowCountsParams{FollowerID: me, FolloweeID: target, Delta: delta})
	})
	if isFKViolation(err) {
		writeProblem(w, http.StatusNotFound, "not-found", "user not found")
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) listFollowers(w http.ResponseWriter, r *http.Request) {
	s.listFollows(w, r, func(p store.ListFollowersParams) ([]store.ListFollowersRow, error) {
		return s.q.ListFollowers(r.Context(), p)
	})
}

func (s *server) listFollowing(w http.ResponseWriter, r *http.Request) {
	s.listFollows(w, r, func(p store.ListFollowersParams) ([]store.ListFollowersRow, error) {
		rows, err := s.q.ListFollowing(r.Context(), store.ListFollowingParams(p))
		out := make([]store.ListFollowersRow, len(rows))
		for i, row := range rows {
			out[i] = store.ListFollowersRow(row)
		}
		return out, err
	})
}

// listFollows pages through either side of a user's follows, newest first.
func (s *server) listFollows(w http.ResponseWriter, r *http.Request, list func(store.ListFollowersParams) ([]store.ListFollowersRow, error)) {
	user, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	c, limit, ok := pageQuery(w, r, newestFirst)
	if !ok {
		return
	}
	if _, err := s.q.GetUser(r.Context(), user); errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not-found", "user not found")
		return
	} else if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	rows, err := list(store.ListFollowersParams{
		Viewer:     caller(r).UserID,
		UserID:     user,
		CursorTime: c.Time,
		CursorID:   c.ID,
		Lim:        int32(limit + 1),
	})
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	rows, next := trim(rows, limit, func(f store.ListFollowersRow) cursor { return cursor{f.FollowedAt, f.User.ID} })
	users := make([]userJSON, len(rows))
	for i, f := range rows {
		users[i] = s.publicUser(f.User)
		users[i].FollowedByMe = &f.FollowedByMe
	}
	writeJSON(w, http.StatusOK, page[userJSON]{Data: users, NextCursor: next})
}

// feed is the home timeline: the caller's and their followees' posts, newest first.
func (s *server) feed(w http.ResponseWriter, r *http.Request) {
	c, limit, ok := pageQuery(w, r, newestFirst)
	if !ok {
		return
	}
	rows, err := s.q.Feed(r.Context(), store.FeedParams{
		Viewer:     caller(r).UserID,
		CursorTime: c.Time,
		CursorID:   c.ID,
		Lim:        int32(limit + 1),
	})
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	rows, next := trim(rows, limit, func(p store.FeedRow) cursor { return cursor{p.CreatedAt, p.ID} })
	ids := make([]uuid.UUID, len(rows))
	for i, p := range rows {
		ids[i] = p.ID
	}
	s.sendPostPage(w, r, ids, next)
}
