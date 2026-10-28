package auth

import (
	"crypto/subtle"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPassword(t *testing.T) {
	h := HashPassword("correct horse")
	if h == HashPassword("correct horse") {
		t.Error("hashes of the same password must differ (salt)")
	}
	for pw, want := range map[string]bool{"correct horse": true, "correct horsE": false, "": false} {
		got, err := CheckPassword(h, pw)
		if err != nil || got != want {
			t.Errorf("CheckPassword(%q) = %v, %v; want %v", pw, got, err, want)
		}
	}
	for _, bad := range []string{"", "plain", "$argon2i$v=19$m=1,t=1,p=1$AA$AA", "$argon2id$v=19$m=x$AA$AA"} {
		if ok, err := CheckPassword(bad, "x"); ok || err == nil {
			t.Errorf("CheckPassword(%q) = %v, %v; want error", bad, ok, err)
		}
	}
}

func TestAccessToken(t *testing.T) {
	secret := []byte("0123456789abcdef0123456789abcdef")
	now := time.Unix(1_800_000_000, 0)
	c := Claims{UserID: uuid.New(), SessionID: uuid.New(), Expires: now.Add(AccessTTL)}
	tok := SignAccess(secret, c)

	got, err := VerifyAccess(secret, tok, now)
	if err != nil || got != c {
		t.Fatalf("VerifyAccess = %+v, %v; want %+v", got, err, c)
	}

	parts := strings.Split(tok, ".")
	noneHeader := b64url.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	rejected := map[string]struct {
		token  string
		secret []byte
		now    time.Time
	}{
		"expired":       {tok, secret, c.Expires},
		"wrong secret":  {tok, []byte("other"), now},
		"tampered":      {parts[0] + "." + b64url.EncodeToString([]byte(`{"sub":"x"}`)) + "." + parts[2], secret, now},
		"alg none":      {noneHeader + "." + parts[1] + ".", secret, now},
		"garbage":       {"abc", secret, now},
		"empty":         {"", secret, now},
		"bad signature": {parts[0] + "." + parts[1] + ".!!", secret, now},
	}
	for name, tc := range rejected {
		if _, err := VerifyAccess(tc.secret, tc.token, tc.now); err != ErrInvalidToken {
			t.Errorf("%s: err = %v, want ErrInvalidToken", name, err)
		}
	}
}

func TestRefreshToken(t *testing.T) {
	sid := uuid.New()
	tok, hash := NewRefresh(sid)
	gotID, gotHash, err := ParseRefresh(tok)
	if err != nil || gotID != sid || subtle.ConstantTimeCompare(gotHash, hash) != 1 {
		t.Fatalf("ParseRefresh = %v, %v", gotID, err)
	}
	if tok2, _ := NewRefresh(sid); tok2 == tok {
		t.Error("refresh tokens must be random")
	}
	for _, bad := range []string{"", "nodot", "not-a-uuid.secret", sid.String() + "."} {
		if _, _, err := ParseRefresh(bad); err != ErrInvalidToken {
			t.Errorf("ParseRefresh(%q) err = %v", bad, err)
		}
	}
}
