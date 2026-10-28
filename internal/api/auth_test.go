package api

import (
	"net/http"
	"testing"
)

func TestRegisterLoginMe(t *testing.T) {
	a := newTestAPI(t)
	reg := a.register("Alice")
	if reg.User.Username != "alice" || reg.User.Email != "alice@example.com" || reg.AccessToken == "" || reg.RefreshToken == "" {
		t.Fatalf("register = %+v", reg)
	}

	var me userJSON
	a.do(reg.AccessToken, "GET", "/v1/me", nil, http.StatusOK, &me)
	if me.ID != reg.User.ID || me.Email == "" {
		t.Errorf("me = %+v", me)
	}

	for _, login := range []string{"alice", "ALICE", "Alice@Example.com"} {
		var out authJSON
		a.do("", "POST", "/v1/auth/login", map[string]string{"login": login, "password": "password123"}, http.StatusOK, &out)
		if out.User.ID != reg.User.ID {
			t.Errorf("login %q gave user %s", login, out.User.ID)
		}
	}
	a.do("", "POST", "/v1/auth/login", map[string]string{"login": "alice", "password": "wrong-password"}, http.StatusUnauthorized, nil)
	a.do("", "POST", "/v1/auth/login", map[string]string{"login": "nobody", "password": "password123"}, http.StatusUnauthorized, nil)
}

func TestRegisterValidation(t *testing.T) {
	a := newTestAPI(t)
	a.register("bob")

	cases := map[string]struct {
		body  map[string]string
		field string
		code  string
	}{
		"bad username":   {map[string]string{"username": "no spaces", "email": "x@example.com", "password": "password123"}, "username", "invalid"},
		"long username":  {map[string]string{"username": "a234567890123456789012345678901", "email": "x@example.com", "password": "password123"}, "username", "invalid"},
		"bad email":      {map[string]string{"username": "x", "email": "not-an-email", "password": "password123"}, "email", "invalid"},
		"short password": {map[string]string{"username": "x", "email": "x@example.com", "password": "short"}, "password", "too_short"},
		"username taken": {map[string]string{"username": "BOB", "email": "x@example.com", "password": "password123"}, "username", "taken"},
		"email taken":    {map[string]string{"username": "x", "email": "Bob@example.com", "password": "password123"}, "email", "taken"},
	}
	for name, tc := range cases {
		var p Problem
		a.do("", "POST", "/v1/auth/register", tc.body, http.StatusUnprocessableEntity, &p)
		if len(p.Errors) != 1 || p.Errors[0].Field != tc.field || p.Errors[0].Code != tc.code {
			t.Errorf("%s: errors = %+v, want %s/%s", name, p.Errors, tc.field, tc.code)
		}
	}

	a.do("", "POST", "/v1/auth/register", "not an object", http.StatusBadRequest, nil)
}

func TestRefreshRotationAndReuse(t *testing.T) {
	a := newTestAPI(t)
	first := a.register("carol")

	var second tokensJSON
	a.do("", "POST", "/v1/auth/refresh", map[string]string{"refresh_token": first.RefreshToken}, http.StatusOK, &second)
	if second.RefreshToken == first.RefreshToken {
		t.Fatal("refresh token did not rotate")
	}
	a.do(second.AccessToken, "GET", "/v1/me", nil, http.StatusOK, nil)

	// Replaying the first token means it leaked: the whole session dies.
	a.do("", "POST", "/v1/auth/refresh", map[string]string{"refresh_token": first.RefreshToken}, http.StatusUnauthorized, nil)
	a.do("", "POST", "/v1/auth/refresh", map[string]string{"refresh_token": second.RefreshToken}, http.StatusUnauthorized, nil)
	a.do(second.AccessToken, "GET", "/v1/me", nil, http.StatusUnauthorized, nil)

	a.do("", "POST", "/v1/auth/refresh", map[string]string{"refresh_token": "garbage"}, http.StatusUnauthorized, nil)
}

func TestLogout(t *testing.T) {
	a := newTestAPI(t)
	s := a.register("dave")
	other := authJSON{}
	a.do("", "POST", "/v1/auth/login", map[string]string{"login": "dave", "password": "password123"}, http.StatusOK, &other)

	a.do(s.AccessToken, "POST", "/v1/auth/logout", nil, http.StatusNoContent, nil)
	a.do(s.AccessToken, "GET", "/v1/me", nil, http.StatusUnauthorized, nil)
	a.do("", "POST", "/v1/auth/refresh", map[string]string{"refresh_token": s.RefreshToken}, http.StatusUnauthorized, nil)

	// Other devices stay logged in.
	a.do(other.AccessToken, "GET", "/v1/me", nil, http.StatusOK, nil)
}

func TestAuthRequired(t *testing.T) {
	a := newTestAPI(t)
	a.do("", "GET", "/v1/me", nil, http.StatusUnauthorized, nil)
	a.do("not-a-token", "GET", "/v1/me", nil, http.StatusUnauthorized, nil)
}
