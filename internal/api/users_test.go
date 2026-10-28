package api

import (
	"net/http"
	"testing"
)

func TestProfile(t *testing.T) {
	a := newTestAPI(t)
	alice := a.register("alice")
	bob := a.register("bob")

	var u userJSON
	a.do(alice.AccessToken, "PATCH", "/v1/me", map[string]string{
		"display_name": "  Alice A. ",
		"bio":          "hello",
		"website":      "https://alice.example",
	}, http.StatusOK, &u)
	if u.DisplayName != "Alice A." || u.Bio != "hello" || u.Website != "https://alice.example" || u.Username != "alice" {
		t.Errorf("updated = %+v", u)
	}

	// Others see the profile without the email.
	var seen userJSON
	a.do(bob.AccessToken, "GET", "/v1/users/"+alice.User.ID, nil, http.StatusOK, &seen)
	if seen.Bio != "hello" || seen.Email != "" {
		t.Errorf("seen by bob = %+v", seen)
	}
	a.do(bob.AccessToken, "GET", "/v1/usernames/ALICE", nil, http.StatusOK, &seen)
	if seen.ID != alice.User.ID {
		t.Errorf("by username = %+v", seen)
	}

	a.do(bob.AccessToken, "GET", "/v1/usernames/nobody", nil, http.StatusNotFound, nil)
	a.do(bob.AccessToken, "GET", "/v1/users/not-a-uuid", nil, http.StatusNotFound, nil)
	a.do(bob.AccessToken, "GET", "/v1/users/00000000-0000-7000-8000-000000000000", nil, http.StatusNotFound, nil)

	// Rename, then the old name is free and the new one is taken.
	a.do(alice.AccessToken, "PATCH", "/v1/me", map[string]string{"username": "Alice2"}, http.StatusOK, &u)
	if u.Username != "alice2" {
		t.Errorf("renamed = %q", u.Username)
	}
	var p Problem
	a.do(bob.AccessToken, "PATCH", "/v1/me", map[string]string{"username": "alice2"}, http.StatusUnprocessableEntity, &p)
	if len(p.Errors) != 1 || p.Errors[0].Code != "taken" {
		t.Errorf("errors = %+v", p.Errors)
	}
	a.do(bob.AccessToken, "PATCH", "/v1/me", map[string]string{"username": "alice"}, http.StatusOK, nil)
}

func TestProfileValidation(t *testing.T) {
	a := newTestAPI(t)
	s := a.register("carol")
	long := string(make([]rune, 151))
	cases := map[string]map[string]string{
		"website":  {"website": "javascript:alert(1)"},
		"bio":      {"bio": long},
		"username": {"username": "bad name"},
	}
	for field, body := range cases {
		var p Problem
		a.do(s.AccessToken, "PATCH", "/v1/me", body, http.StatusUnprocessableEntity, &p)
		if len(p.Errors) != 1 || p.Errors[0].Field != field {
			t.Errorf("%s: errors = %+v", field, p.Errors)
		}
	}
	// Clearing optional fields is allowed.
	a.do(s.AccessToken, "PATCH", "/v1/me", map[string]string{"website": "", "bio": ""}, http.StatusOK, nil)
}
