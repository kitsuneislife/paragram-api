package api

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kitsuneislife/paragram-api/internal/auth"
	"github.com/kitsuneislife/paragram-api/internal/db/store"
)

// refreshTTL is how long a session survives without being refreshed.
const refreshTTL = 30 * 24 * time.Hour

type tokensJSON struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
}

type authJSON struct {
	tokensJSON
	User userJSON `json:"user"`
}

func (s *server) issue(userID, sessionID uuid.UUID, refresh string) tokensJSON {
	access := auth.SignAccess(s.JWTSecret, auth.Claims{
		UserID:    userID,
		SessionID: sessionID,
		Expires:   time.Now().Add(auth.AccessTTL),
	})
	return tokensJSON{AccessToken: access, RefreshToken: refresh, TokenType: "Bearer", ExpiresIn: int(auth.AccessTTL.Seconds())}
}

// startSession creates a session for userID and returns its tokens.
func (s *server) startSession(ctx context.Context, q *store.Queries, userID uuid.UUID) (tokensJSON, error) {
	sid := uuid.Must(uuid.NewV7())
	refresh, hash := auth.NewRefresh(sid)
	if err := q.CreateSession(ctx, store.CreateSessionParams{ID: sid, UserID: userID, RefreshHash: hash}); err != nil {
		return tokensJSON{}, err
	}
	return s.issue(userID, sid, refresh), nil
}

func (s *server) inTx(ctx context.Context, fn func(q *store.Queries) error) error {
	return pgx.BeginFunc(ctx, s.Pool, func(tx pgx.Tx) error { return fn(s.q.WithTx(tx)) })
}

func (s *server) register(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Username    string `json:"username"`
		Email       string `json:"email"`
		Password    string `json:"password"`
		DisplayName string `json:"display_name"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = normalizeUsername(in.Username)
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	in.DisplayName = strings.TrimSpace(in.DisplayName)

	var v fieldErrors
	v.check(usernameRe.MatchString(in.Username), "username", "invalid")
	v.check(validEmail(in.Email), "email", "invalid")
	n := utf8.RuneCountInString(in.Password)
	v.check(n >= minPassword, "password", "too_short")
	v.check(n <= maxPassword, "password", "too_long")
	v.maxLen(in.DisplayName, maxDisplayName, "display_name")
	if len(v) > 0 {
		writeInvalid(w, v)
		return
	}

	hash := auth.HashPassword(in.Password)
	var out authJSON
	err := s.inTx(r.Context(), func(q *store.Queries) error {
		u, err := q.CreateUser(r.Context(), store.CreateUserParams{
			ID:           uuid.Must(uuid.NewV7()),
			Username:     in.Username,
			Email:        in.Email,
			PasswordHash: hash,
			DisplayName:  in.DisplayName,
		})
		if err != nil {
			return err
		}
		out.User = privateUser(u)
		out.tokensJSON, err = s.startSession(r.Context(), q, u.ID)
		return err
	})
	if constraint, ok := uniqueViolation(err); ok {
		field := map[string]string{"users_username_key": "username", "users_email_key": "email"}[constraint]
		if field == "" {
			writeInternal(w, r, s.Log, err)
			return
		}
		writeInvalid(w, []FieldError{{Field: field, Code: "taken"}})
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusCreated, out)
}

func (s *server) login(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Login    string `json:"login"` // username or email
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := s.q.GetUserByLogin(r.Context(), strings.TrimSpace(in.Login))
	if errors.Is(err, pgx.ErrNoRows) {
		auth.BurnPasswordCheck(in.Password)
		writeProblem(w, http.StatusUnauthorized, "invalid-credentials", "")
		return
	}
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	ok, err := auth.CheckPassword(u.PasswordHash, in.Password)
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	if !ok {
		writeProblem(w, http.StatusUnauthorized, "invalid-credentials", "")
		return
	}
	tokens, err := s.startSession(r.Context(), s.q, u.ID)
	if err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	writeJSON(w, http.StatusOK, authJSON{tokensJSON: tokens, User: privateUser(u)})
}

// errInvalidRefresh rolls back a refresh. A reused token is reported through
// a flag instead, because revoking its session must commit.
var errInvalidRefresh = errors.New("invalid refresh token")

func (s *server) refresh(w http.ResponseWriter, r *http.Request) {
	var in struct {
		RefreshToken string `json:"refresh_token"`
	}
	if !decode(w, r, &in) {
		return
	}
	sid, hash, err := auth.ParseRefresh(in.RefreshToken)
	if err != nil {
		writeProblem(w, http.StatusUnauthorized, "invalid-token", "")
		return
	}

	var out tokensJSON
	reused := false
	err = s.inTx(r.Context(), func(q *store.Queries) error {
		sess, err := q.GetSessionForUpdate(r.Context(), sid)
		if errors.Is(err, pgx.ErrNoRows) {
			return errInvalidRefresh
		}
		if err != nil {
			return err
		}
		if sess.RevokedAt.Valid || time.Since(sess.UpdatedAt) > refreshTTL {
			return errInvalidRefresh
		}
		if subtle.ConstantTimeCompare(sess.RefreshHash, hash) != 1 {
			// An old token came back: someone else holds a copy. Kill the session.
			reused = true
			return q.RevokeSession(r.Context(), sid)
		}
		refresh, newHash := auth.NewRefresh(sid)
		if err := q.RotateSession(r.Context(), store.RotateSessionParams{ID: sid, RefreshHash: newHash}); err != nil {
			return err
		}
		out = s.issue(sess.UserID, sid, refresh)
		return nil
	})
	switch {
	case errors.Is(err, errInvalidRefresh) || (err == nil && reused):
		if reused {
			s.Log.WarnContext(r.Context(), "refresh token reuse, session revoked", "session_id", sid)
		}
		writeProblem(w, http.StatusUnauthorized, "invalid-token", "")
	case err != nil:
		writeInternal(w, r, s.Log, err)
	default:
		writeJSON(w, http.StatusOK, out)
	}
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if err := s.q.RevokeSession(r.Context(), caller(r).SessionID); err != nil {
		writeInternal(w, r, s.Log, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type claimsKey struct{}

// caller returns the identity attached by authed. Only valid inside it.
func caller(r *http.Request) auth.Claims {
	return r.Context().Value(claimsKey{}).(auth.Claims)
}

// authed rejects requests without a valid access token for a live session.
// Checking the session makes logout and reuse revocation take effect at once.
func (s *server) authed(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if !ok {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
			return
		}
		c, err := auth.VerifyAccess(s.JWTSecret, token, time.Now())
		if err != nil {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "invalid or expired token")
			return
		}
		active, err := s.q.SessionActive(r.Context(), store.SessionActiveParams{ID: c.SessionID, UserID: c.UserID})
		if err != nil {
			writeInternal(w, r, s.Log, err)
			return
		}
		if !active {
			writeProblem(w, http.StatusUnauthorized, "unauthorized", "session ended")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), claimsKey{}, c)))
	}
}
