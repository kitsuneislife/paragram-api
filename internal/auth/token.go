package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
)

// AccessTTL is how long an access token is valid.
const AccessTTL = 15 * time.Minute

// Claims identify the caller of an authenticated request.
type Claims struct {
	UserID    uuid.UUID
	SessionID uuid.UUID
	Expires   time.Time
}

// ErrInvalidToken covers every reason a token is rejected; callers answer 401
// without saying which.
var ErrInvalidToken = errors.New("auth: invalid token")

var b64url = base64.RawURLEncoding

// The header is fixed, so a token naming any other algorithm is rejected
// before its signature is even looked at.
var jwtHeader = b64url.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))

type jwtPayload struct {
	Sub string `json:"sub"`
	Sid string `json:"sid"`
	Exp int64  `json:"exp"`
}

// SignAccess returns an HS256 JWT for c.
func SignAccess(secret []byte, c Claims) string {
	payload, _ := json.Marshal(jwtPayload{Sub: c.UserID.String(), Sid: c.SessionID.String(), Exp: c.Expires.Unix()})
	unsigned := jwtHeader + "." + b64url.EncodeToString(payload)
	return unsigned + "." + b64url.EncodeToString(mac(secret, unsigned))
}

// VerifyAccess checks the signature and expiry of a token from SignAccess.
func VerifyAccess(secret []byte, token string, now time.Time) (Claims, error) {
	header, rest, ok := strings.Cut(token, ".")
	if !ok || header != jwtHeader {
		return Claims{}, ErrInvalidToken
	}
	payload, sig, ok := strings.Cut(rest, ".")
	if !ok {
		return Claims{}, ErrInvalidToken
	}
	gotSig, err := b64url.DecodeString(sig)
	if err != nil || !hmac.Equal(gotSig, mac(secret, header+"."+payload)) {
		return Claims{}, ErrInvalidToken
	}
	raw, err := b64url.DecodeString(payload)
	if err != nil {
		return Claims{}, ErrInvalidToken
	}
	var p jwtPayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Claims{}, ErrInvalidToken
	}
	userID, err1 := uuid.Parse(p.Sub)
	sessionID, err2 := uuid.Parse(p.Sid)
	expires := time.Unix(p.Exp, 0)
	if err1 != nil || err2 != nil || !now.Before(expires) {
		return Claims{}, ErrInvalidToken
	}
	return Claims{UserID: userID, SessionID: sessionID, Expires: expires}, nil
}

func mac(secret []byte, s string) []byte {
	h := hmac.New(sha256.New, secret)
	h.Write([]byte(s))
	return h.Sum(nil)
}

// NewRefresh returns a refresh token "<session id>.<secret>" and the hash of
// its secret, which is all the database stores.
func NewRefresh(sessionID uuid.UUID) (token string, hash []byte) {
	secret := make([]byte, 32)
	_, _ = rand.Read(secret)
	s := b64url.EncodeToString(secret)
	return sessionID.String() + "." + s, hashSecret(s)
}

// ParseRefresh splits a refresh token into its session ID and secret hash.
func ParseRefresh(token string) (sessionID uuid.UUID, hash []byte, err error) {
	id, secret, ok := strings.Cut(token, ".")
	if !ok || secret == "" {
		return uuid.Nil, nil, ErrInvalidToken
	}
	sessionID, err = uuid.Parse(id)
	if err != nil {
		return uuid.Nil, nil, ErrInvalidToken
	}
	return sessionID, hashSecret(secret), nil
}

func hashSecret(s string) []byte {
	h := sha256.Sum256([]byte(s))
	return h[:]
}
