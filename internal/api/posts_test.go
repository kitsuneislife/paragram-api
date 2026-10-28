package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

// post uploads n images and creates a post with them, waiting until ready.
func (a *testAPI) post(token, caption string, n int) postJSON {
	a.t.Helper()
	ids := make([]string, n)
	for i := range ids {
		ids[i] = a.upload(token, "post", pngImage(a.t))
	}
	var p postJSON
	a.do(token, "POST", "/v1/posts", map[string]any{"caption": caption, "media_ids": ids}, http.StatusCreated, &p)
	for _, id := range ids {
		if m := a.waitMedia(token, id); m.Status != "ready" {
			a.t.Fatalf("media %s: %s", id, m.Status)
		}
	}
	a.do(token, "GET", "/v1/posts/"+p.ID, nil, http.StatusOK, &p)
	return p
}

func TestPosts(t *testing.T) {
	a := newTestAPI(t)
	alice := a.register("alice")
	bob := a.register("bob")

	ids := []string{a.upload(alice.AccessToken, "post", pngImage(t)), a.upload(alice.AccessToken, "post", pngImage(t))}
	body := map[string]any{"caption": "  first  ", "media_ids": ids}
	key := uuid.NewString()
	var p, again postJSON
	a.doKey(alice.AccessToken, key, "POST", "/v1/posts", body, http.StatusCreated, &p)
	a.doKey(alice.AccessToken, key, "POST", "/v1/posts", body, http.StatusCreated, &again)
	if again.ID != p.ID {
		t.Fatalf("retry created a second post: %s != %s", again.ID, p.ID)
	}
	if p.Caption != "first" || p.Author.ID != alice.User.ID || len(p.Media) != 2 || p.Media[0].ID != ids[0] || p.Media[1].ID != ids[1] {
		t.Fatalf("post = %+v", p)
	}
	for _, id := range ids {
		a.waitMedia(alice.AccessToken, id)
	}
	a.do(bob.AccessToken, "GET", "/v1/posts/"+p.ID, nil, http.StatusOK, &p)
	if !p.Ready || p.Media[0].URLs == nil {
		t.Fatalf("ready post = %+v", p)
	}

	// A failed media keeps the post hidden from everyone but the author.
	bad := a.upload(alice.AccessToken, "post", []byte("not an image"))
	var hidden postJSON
	a.do(alice.AccessToken, "POST", "/v1/posts", map[string]any{"media_ids": []string{bad}}, http.StatusCreated, &hidden)
	a.waitMedia(alice.AccessToken, bad)
	a.do(bob.AccessToken, "GET", "/v1/posts/"+hidden.ID, nil, http.StatusNotFound, nil)
	a.do(alice.AccessToken, "GET", "/v1/posts/"+hidden.ID, nil, http.StatusOK, &hidden)
	if hidden.Ready || hidden.Media[0].Status != "failed" {
		t.Fatalf("hidden = %+v", hidden)
	}

	var u userJSON
	a.do(bob.AccessToken, "GET", "/v1/users/"+alice.User.ID, nil, http.StatusOK, &u)
	if u.PostsCount != 2 {
		t.Errorf("posts_count = %d", u.PostsCount)
	}

	var grid page[postJSON]
	a.do(bob.AccessToken, "GET", "/v1/users/"+alice.User.ID+"/posts", nil, http.StatusOK, &grid)
	if len(grid.Data) != 1 || grid.Data[0].ID != p.ID || grid.NextCursor != nil {
		t.Fatalf("bob's view of the grid = %+v", grid)
	}
	// Alice sees both, newest first, one per page.
	a.do(alice.AccessToken, "GET", "/v1/users/"+alice.User.ID+"/posts?limit=1", nil, http.StatusOK, &grid)
	if len(grid.Data) != 1 || grid.Data[0].ID != hidden.ID || grid.NextCursor == nil {
		t.Fatalf("page 1 = %+v", grid)
	}
	a.do(alice.AccessToken, "GET", "/v1/users/"+alice.User.ID+"/posts?limit=1&cursor="+*grid.NextCursor, nil, http.StatusOK, &grid)
	if len(grid.Data) != 1 || grid.Data[0].ID != p.ID || grid.NextCursor != nil {
		t.Fatalf("page 2 = %+v", grid)
	}
	a.do(bob.AccessToken, "GET", "/v1/users/"+uuid.NewString()+"/posts", nil, http.StatusNotFound, nil)

	a.do(bob.AccessToken, "DELETE", "/v1/posts/"+p.ID, nil, http.StatusNotFound, nil)
	a.do(alice.AccessToken, "DELETE", "/v1/posts/"+p.ID, nil, http.StatusNoContent, nil)
	a.do(alice.AccessToken, "DELETE", "/v1/posts/"+p.ID, nil, http.StatusNotFound, nil)
	a.do(alice.AccessToken, "GET", "/v1/posts/"+p.ID, nil, http.StatusNotFound, nil)
	a.do(alice.AccessToken, "GET", "/v1/me", nil, http.StatusOK, &u)
	if u.PostsCount != 1 {
		t.Errorf("posts_count after delete = %d", u.PostsCount)
	}
}

func TestPostValidation(t *testing.T) {
	a := newTestAPI(t)
	s := a.register("carol")
	tok := s.AccessToken
	img := a.upload(tok, "post", pngImage(t))
	avatar := a.upload(tok, "avatar", pngImage(t))

	eleven := make([]string, 11)
	for i := range eleven {
		eleven[i] = uuid.NewString()
	}
	for name, body := range map[string]map[string]any{
		"no media":      {"media_ids": []string{}},
		"too many":      {"media_ids": eleven},
		"repeated":      {"media_ids": []string{img, img}},
		"malformed":     {"media_ids": []string{"x"}},
		"unknown":       {"media_ids": []string{uuid.NewString()}},
		"avatar upload": {"media_ids": []string{avatar}},
		"long caption":  {"media_ids": []string{img}, "caption": strings.Repeat("a", maxCaption+1)},
	} {
		t.Run(name, func(t *testing.T) {
			a.do(tok, "POST", "/v1/posts", body, http.StatusUnprocessableEntity, nil)
		})
	}
	a.doKey(tok, "nope", "POST", "/v1/posts", map[string]any{"media_ids": []string{img}}, http.StatusUnprocessableEntity, nil)

	// A media goes into one post only.
	a.do(tok, "POST", "/v1/posts", map[string]any{"media_ids": []string{img}}, http.StatusCreated, nil)
	a.do(tok, "POST", "/v1/posts", map[string]any{"media_ids": []string{img}}, http.StatusUnprocessableEntity, nil)

	a.do(tok, "GET", "/v1/users/"+s.User.ID+"/posts?limit=0", nil, http.StatusUnprocessableEntity, nil)
	a.do(tok, "GET", "/v1/users/"+s.User.ID+"/posts?limit=101", nil, http.StatusUnprocessableEntity, nil)
	a.do(tok, "GET", "/v1/users/"+s.User.ID+"/posts?cursor=zzz", nil, http.StatusUnprocessableEntity, nil)
}
