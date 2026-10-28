package api

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/kitsuneislife/paragram-api/internal/db/store"
)

type userJSON struct {
	ID             string    `json:"id"`
	Username       string    `json:"username"`
	DisplayName    string    `json:"display_name"`
	Bio            string    `json:"bio"`
	Website        string    `json:"website"`
	PostsCount     int32     `json:"posts_count"`
	FollowersCount int32     `json:"followers_count"`
	FollowingCount int32     `json:"following_count"`
	CreatedAt      time.Time `json:"created_at"`
	Email          string    `json:"email,omitempty"` // only shown to the user themself
}

func publicUser(u store.User) userJSON {
	return userJSON{
		ID:             u.ID.String(),
		Username:       u.Username,
		DisplayName:    u.DisplayName,
		Bio:            u.Bio,
		Website:        u.Website,
		PostsCount:     u.PostsCount,
		FollowersCount: u.FollowersCount,
		FollowingCount: u.FollowingCount,
		CreatedAt:      u.CreatedAt.UTC(),
	}
}

func privateUser(u store.User) userJSON {
	j := publicUser(u)
	j.Email = u.Email
	return j
}

func (s *server) getMe(w http.ResponseWriter, r *http.Request) {
	u, err := s.q.GetUser(r.Context(), caller(r).UserID)
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, privateUser(u))
}

func (s *server) updateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    *string `json:"username"`
		DisplayName *string `json:"display_name"`
		Bio         *string `json:"bio"`
		Website     *string `json:"website"`
	}
	if !decode(w, r, &in) {
		return
	}
	var v fieldErrors
	p := store.UpdateUserParams{ID: caller(r).UserID}
	if in.Username != nil {
		u := normalizeUsername(*in.Username)
		v.check(usernameRe.MatchString(u), "username", "invalid")
		p.Username = pgtype.Text{String: u, Valid: true}
	}
	if in.DisplayName != nil {
		d := strings.TrimSpace(*in.DisplayName)
		v.maxLen(d, maxDisplayName, "display_name")
		p.DisplayName = pgtype.Text{String: d, Valid: true}
	}
	if in.Bio != nil {
		b := strings.TrimSpace(*in.Bio)
		v.maxLen(b, maxBio, "bio")
		p.Bio = pgtype.Text{String: b, Valid: true}
	}
	if in.Website != nil {
		site := strings.TrimSpace(*in.Website)
		v.maxLen(site, maxWebsite, "website")
		v.check(site == "" || validWebsite(site), "website", "invalid")
		p.Website = pgtype.Text{String: site, Valid: true}
	}
	if len(v) > 0 {
		writeInvalid(w, v)
		return
	}

	u, err := s.q.UpdateUser(r.Context(), p)
	if c, ok := uniqueViolation(err); ok && c == "users_username_key" {
		writeInvalid(w, []FieldError{{Field: "username", Code: "taken"}})
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, privateUser(u))
}

func (s *server) getUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathUUID(w, r, "id")
	if !ok {
		return
	}
	s.sendUser(w, r, func(ctx context.Context) (store.User, error) { return s.q.GetUser(ctx, id) })
}

func (s *server) getUserByUsername(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("username")
	s.sendUser(w, r, func(ctx context.Context) (store.User, error) { return s.q.GetUserByUsername(ctx, name) })
}

func (s *server) sendUser(w http.ResponseWriter, r *http.Request, get func(context.Context) (store.User, error)) {
	u, err := get(r.Context())
	if errors.Is(err, pgx.ErrNoRows) {
		writeProblem(w, http.StatusNotFound, "not-found", "user not found")
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	if u.ID == caller(r).UserID {
		writeJSON(w, http.StatusOK, privateUser(u))
		return
	}
	writeJSON(w, http.StatusOK, publicUser(u))
}

// pathUUID parses a UUID path parameter. A malformed one cannot name any
// resource, so it is a 404 like any other unknown ID.
func pathUUID(w http.ResponseWriter, r *http.Request, name string) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue(name))
	if err != nil {
		writeProblem(w, http.StatusNotFound, "not-found", "")
		return uuid.Nil, false
	}
	return id, true
}
