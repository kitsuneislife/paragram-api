package api

import (
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestLikes(t *testing.T) {
	a := newTestAPI(t)
	alice := a.register("alice")
	bob := a.register("bob")
	p := a.post(alice.AccessToken, "hi", 1)
	like := "/v1/posts/" + p.ID + "/like"

	get := func(s authJSON) postJSON {
		t.Helper()
		var got postJSON
		a.do(s.AccessToken, "GET", "/v1/posts/"+p.ID, nil, http.StatusOK, &got)
		return got
	}

	a.do(bob.AccessToken, "PUT", like, nil, http.StatusNoContent, nil)
	a.do(bob.AccessToken, "PUT", like, nil, http.StatusNoContent, nil)
	a.do(alice.AccessToken, "PUT", like, nil, http.StatusNoContent, nil)
	if got := get(bob); got.LikesCount != 2 || !got.LikedByMe {
		t.Fatalf("after likes, bob sees %+v", got)
	}

	a.do(bob.AccessToken, "DELETE", like, nil, http.StatusNoContent, nil)
	a.do(bob.AccessToken, "DELETE", like, nil, http.StatusNoContent, nil)
	if got := get(bob); got.LikesCount != 1 || got.LikedByMe {
		t.Fatalf("after unlike, bob sees %+v", got)
	}
	if got := get(alice); !got.LikedByMe {
		t.Fatalf("alice sees %+v", got)
	}

	// Feed carries liked_by_me too.
	var feed page[postJSON]
	a.do(alice.AccessToken, "GET", "/v1/feed", nil, http.StatusOK, &feed)
	if len(feed.Data) != 1 || !feed.Data[0].LikedByMe {
		t.Fatalf("feed = %+v", feed)
	}

	a.do(bob.AccessToken, "PUT", "/v1/posts/"+uuid.NewString()+"/like", nil, http.StatusNotFound, nil)

	// Unready posts cannot be liked, even by their author.
	bad := a.upload(alice.AccessToken, "post", []byte("not an image"))
	var hidden postJSON
	a.do(alice.AccessToken, "POST", "/v1/posts", map[string]any{"media_ids": []string{bad}}, http.StatusCreated, &hidden)
	a.waitMedia(alice.AccessToken, bad)
	a.do(alice.AccessToken, "PUT", "/v1/posts/"+hidden.ID+"/like", nil, http.StatusNotFound, nil)

	// Deleting the post drops its likes.
	a.do(alice.AccessToken, "DELETE", "/v1/posts/"+p.ID, nil, http.StatusNoContent, nil)
	a.do(alice.AccessToken, "DELETE", like, nil, http.StatusNotFound, nil)
}

func TestComments(t *testing.T) {
	a := newTestAPI(t)
	alice := a.register("alice")
	bob := a.register("bob")
	carol := a.register("carol")
	p := a.post(alice.AccessToken, "hi", 1)
	comments := "/v1/posts/" + p.ID + "/comments"

	count := func() int32 {
		t.Helper()
		var got postJSON
		a.do(bob.AccessToken, "GET", "/v1/posts/"+p.ID, nil, http.StatusOK, &got)
		return got.CommentsCount
	}

	var first, again, second commentJSON
	key := uuid.NewString()
	a.doKey(bob.AccessToken, key, "POST", comments, map[string]any{"body": "  first  "}, http.StatusCreated, &first)
	a.doKey(bob.AccessToken, key, "POST", comments, map[string]any{"body": "first"}, http.StatusCreated, &again)
	if first.Body != "first" || first.PostID != p.ID || first.Author.ID != bob.User.ID || again.ID != first.ID {
		t.Fatalf("first = %+v, retry = %+v", first, again)
	}
	a.do(carol.AccessToken, "POST", comments, map[string]any{"body": "second"}, http.StatusCreated, &second)
	if n := count(); n != 2 {
		t.Fatalf("comments_count = %d", n)
	}

	// Oldest first, one per page.
	var list page[commentJSON]
	a.do(alice.AccessToken, "GET", comments+"?limit=1", nil, http.StatusOK, &list)
	if len(list.Data) != 1 || list.Data[0].ID != first.ID || list.NextCursor == nil {
		t.Fatalf("page 1 = %+v", list)
	}
	a.do(alice.AccessToken, "GET", comments+"?limit=1&cursor="+*list.NextCursor, nil, http.StatusOK, &list)
	if len(list.Data) != 1 || list.Data[0].ID != second.ID || list.NextCursor != nil {
		t.Fatalf("page 2 = %+v", list)
	}

	// Only the comment's author or the post's author may delete it.
	a.do(carol.AccessToken, "DELETE", "/v1/comments/"+first.ID, nil, http.StatusNotFound, nil)
	a.do(bob.AccessToken, "DELETE", "/v1/comments/"+first.ID, nil, http.StatusNoContent, nil)
	a.do(alice.AccessToken, "DELETE", "/v1/comments/"+second.ID, nil, http.StatusNoContent, nil)
	a.do(alice.AccessToken, "DELETE", "/v1/comments/"+second.ID, nil, http.StatusNotFound, nil)
	if n := count(); n != 0 {
		t.Fatalf("comments_count after deletes = %d", n)
	}

	for _, body := range []string{"   ", strings.Repeat("x", maxComment+1)} {
		a.do(bob.AccessToken, "POST", comments, map[string]any{"body": body}, http.StatusUnprocessableEntity, nil)
	}
	a.doKey(bob.AccessToken, "nope", "POST", comments, map[string]any{"body": "x"}, http.StatusUnprocessableEntity, nil)
	missing := "/v1/posts/" + uuid.NewString() + "/comments"
	a.do(bob.AccessToken, "POST", missing, map[string]any{"body": "x"}, http.StatusNotFound, nil)
	a.do(bob.AccessToken, "GET", missing, nil, http.StatusNotFound, nil)
	a.do(bob.AccessToken, "GET", comments+"?cursor=bad", nil, http.StatusUnprocessableEntity, nil)
}
