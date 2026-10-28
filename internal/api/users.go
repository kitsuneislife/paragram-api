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
	ID             string     `json:"id"`
	Username       string     `json:"username"`
	DisplayName    string     `json:"display_name"`
	Bio            string     `json:"bio"`
	Website        string     `json:"website"`
	PostsCount     int32      `json:"posts_count"`
	FollowersCount int32      `json:"followers_count"`
	FollowingCount int32      `json:"following_count"`
	CreatedAt      time.Time  `json:"created_at"`
	Avatar         *mediaURLs `json:"avatar"`
	FollowedByMe   *bool      `json:"followed_by_me,omitempty"` // set where the API looks it up
	Email          string     `json:"email,omitempty"`          // only shown to the user themself
}

func (s *server) publicUser(u store.User) userJSON {
	j := userJSON{
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
	if u.AvatarMediaID.Valid {
		j.Avatar = s.urls(u.AvatarMediaID.UUID)
	}
	return j
}

func (s *server) privateUser(u store.User) userJSON {
	j := s.publicUser(u)
	j.Email = u.Email
	return j
}

func (s *server) getMe(w http.ResponseWriter, r *http.Request) {
	u, err := s.q.GetUser(r.Context(), caller(r).UserID)
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, s.privateUser(u))
}

func (s *server) updateMe(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    *string `json:"username"`
		DisplayName *string `json:"display_name"`
		Bio         *string `json:"bio"`
		Website     *string `json:"website"`
		// A pending avatar upload; it replaces the current one once processed. "" removes it.
		AvatarMediaID *string `json:"avatar_media_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	var v fieldErrors
	me := caller(r).UserID
	p := store.UpdateUserParams{ID: me}
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
	var avatar uuid.UUID // Nil with a non-nil AvatarMediaID means remove
	if in.AvatarMediaID != nil && *in.AvatarMediaID != "" {
		id, err := uuid.Parse(*in.AvatarMediaID)
		v.check(err == nil, "avatar_media_id", "invalid")
		avatar = id
	}
	if len(v) > 0 {
		writeInvalid(w, v)
		return
	}

	errAvatar := []FieldError{{Field: "avatar_media_id", Code: "invalid"}}
	if avatar != uuid.Nil {
		if err := s.checkUploads(r.Context(), []uuid.UUID{avatar}); errors.Is(err, errMediaInvalid) {
			writeInvalid(w, errAvatar)
			return
		} else if err != nil {
			writeInternal(w, r, s.Log, err)
			return
		}
	}
	var u store.User
	err := s.inTx(r.Context(), func(tx pgx.Tx, q *store.Queries) error {
		if in.AvatarMediaID != nil {
			var err error
			if avatar == uuid.Nil {
				err = q.SetAvatar(r.Context(), store.SetAvatarParams{UserID: me})
			} else {
				err = s.attachMedia(r.Context(), tx, q, me, "avatar", []uuid.UUID{avatar})
			}
			if err != nil {
				return err
			}
		}
		var err error
		u, err = q.UpdateUser(r.Context(), p)
		return err
	})
	if c, ok := uniqueViolation(err); ok && c == "users_username_key" {
		writeInvalid(w, []FieldError{{Field: "username", Code: "taken"}})
		return
	}
	if errors.Is(err, errMediaInvalid) {
		writeInvalid(w, errAvatar)
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, s.privateUser(u))
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
	me := caller(r).UserID
	if u.ID == me {
		writeJSON(w, http.StatusOK, s.privateUser(u))
		return
	}
	following, err := s.q.IsFollowing(r.Context(), store.IsFollowingParams{FollowerID: me, FolloweeID: u.ID})
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	j := s.publicUser(u)
	j.FollowedByMe = &following
	writeJSON(w, http.StatusOK, j)
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
