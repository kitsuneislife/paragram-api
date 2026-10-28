package api

import (
	"net/http"
	"testing"
)

func TestAvatarUpload(t *testing.T) {
	a := newTestAPI(t)
	alice := a.register("alice")
	bob := a.register("bob")

	id := a.upload(alice.AccessToken, "avatar", pngImage(t))
	// Bob cannot see or use Alice's media.
	a.do(bob.AccessToken, "GET", "/v1/media/"+id, nil, http.StatusNotFound, nil)
	a.do(bob.AccessToken, "PATCH", "/v1/me", map[string]string{"avatar_media_id": id}, http.StatusUnprocessableEntity, nil)

	var u userJSON
	a.do(alice.AccessToken, "PATCH", "/v1/me", map[string]string{"avatar_media_id": id}, http.StatusOK, &u)
	if u.Avatar != nil {
		t.Errorf("avatar set before processing: %+v", u.Avatar)
	}
	m := a.waitMedia(alice.AccessToken, id)
	if m.Status != "ready" || m.Width != 64 || m.Height != 48 || m.URLs == nil {
		t.Fatalf("media = %+v", m)
	}

	a.do(bob.AccessToken, "GET", "/v1/users/"+alice.User.ID, nil, http.StatusOK, &u)
	if u.Avatar == nil || u.Avatar.Thumb != m.URLs.Thumb {
		t.Fatalf("avatar = %+v", u.Avatar)
	}
	res, err := http.Get(u.Avatar.Thumb)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK || res.Header.Get("Content-Type") != "image/jpeg" {
		t.Errorf("thumb: %d %s", res.StatusCode, res.Header.Get("Content-Type"))
	}

	// A media is used once.
	a.do(alice.AccessToken, "PATCH", "/v1/me", map[string]string{"avatar_media_id": id}, http.StatusUnprocessableEntity, nil)

	a.do(alice.AccessToken, "PATCH", "/v1/me", map[string]string{"avatar_media_id": ""}, http.StatusOK, &u)
	if u.Avatar != nil {
		t.Errorf("avatar not removed: %+v", u.Avatar)
	}
}

func TestMediaRejects(t *testing.T) {
	a := newTestAPI(t)
	s := a.register("carol")

	a.do(s.AccessToken, "POST", "/v1/media", map[string]string{"purpose": "banner"}, http.StatusUnprocessableEntity, nil)

	// Created but never uploaded.
	var m mediaJSON
	a.do(s.AccessToken, "POST", "/v1/media", map[string]string{"purpose": "avatar"}, http.StatusCreated, &m)
	if m.Status != "pending" {
		t.Errorf("status = %s", m.Status)
	}
	a.do(s.AccessToken, "PATCH", "/v1/me", map[string]string{"avatar_media_id": m.ID}, http.StatusUnprocessableEntity, nil)

	// Uploaded for posts, not avatars.
	post := a.upload(s.AccessToken, "post", pngImage(t))
	a.do(s.AccessToken, "PATCH", "/v1/me", map[string]string{"avatar_media_id": post}, http.StatusUnprocessableEntity, nil)
	a.do(s.AccessToken, "PATCH", "/v1/me", map[string]string{"avatar_media_id": "nope"}, http.StatusUnprocessableEntity, nil)

	// Not an image: accepted, then fails in the worker.
	bad := a.upload(s.AccessToken, "avatar", []byte("hello"))
	a.do(s.AccessToken, "PATCH", "/v1/me", map[string]string{"avatar_media_id": bad}, http.StatusOK, nil)
	if m := a.waitMedia(s.AccessToken, bad); m.Status != "failed" {
		t.Errorf("bad upload status = %s", m.Status)
	}
	var u userJSON
	a.do(s.AccessToken, "GET", "/v1/me", nil, http.StatusOK, &u)
	if u.Avatar != nil {
		t.Errorf("avatar = %+v", u.Avatar)
	}
}
