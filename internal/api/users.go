package api

import (
	"net/http"
	"time"

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
