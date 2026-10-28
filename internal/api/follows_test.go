package api

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func TestFollow(t *testing.T) {
	a := newTestAPI(t)
	alice := a.register("alice")
	bob := a.register("bob")
	carol := a.register("carol")
	follow := "/v1/users/" + alice.User.ID + "/follow"

	counts := func(s authJSON) (followers, following int32) {
		t.Helper()
		var u userJSON
		a.do(s.AccessToken, "GET", "/v1/me", nil, http.StatusOK, &u)
		return u.FollowersCount, u.FollowingCount
	}

	a.do(bob.AccessToken, "PUT", follow, nil, http.StatusNoContent, nil)
	a.do(bob.AccessToken, "PUT", follow, nil, http.StatusNoContent, nil)
	a.do(carol.AccessToken, "PUT", follow, nil, http.StatusNoContent, nil)
	if f, _ := counts(alice); f != 2 {
		t.Errorf("alice followers = %d", f)
	}
	if _, f := counts(bob); f != 1 {
		t.Errorf("bob following = %d", f)
	}

	var u userJSON
	a.do(bob.AccessToken, "GET", "/v1/users/"+alice.User.ID, nil, http.StatusOK, &u)
	if u.FollowedByMe == nil || !*u.FollowedByMe {
		t.Errorf("alice as seen by bob: followed_by_me = %v", u.FollowedByMe)
	}
	a.do(alice.AccessToken, "GET", "/v1/users/"+bob.User.ID, nil, http.StatusOK, &u)
	if u.FollowedByMe == nil || *u.FollowedByMe {
		t.Errorf("bob as seen by alice: followed_by_me = %v", u.FollowedByMe)
	}

	// Newest first, one per page.
	var list page[userJSON]
	a.do(bob.AccessToken, "GET", "/v1/users/"+alice.User.ID+"/followers?limit=1", nil, http.StatusOK, &list)
	if len(list.Data) != 1 || list.Data[0].ID != carol.User.ID || list.NextCursor == nil {
		t.Fatalf("followers page 1 = %+v", list)
	}
	a.do(bob.AccessToken, "GET", "/v1/users/"+alice.User.ID+"/followers?limit=1&cursor="+*list.NextCursor, nil, http.StatusOK, &list)
	if len(list.Data) != 1 || list.Data[0].ID != bob.User.ID || list.NextCursor != nil {
		t.Fatalf("followers page 2 = %+v", list)
	}
	a.do(carol.AccessToken, "GET", "/v1/users/"+bob.User.ID+"/following", nil, http.StatusOK, &list)
	if len(list.Data) != 1 || list.Data[0].ID != alice.User.ID || !*list.Data[0].FollowedByMe {
		t.Fatalf("bob's following as seen by carol = %+v", list)
	}

	a.do(bob.AccessToken, "DELETE", follow, nil, http.StatusNoContent, nil)
	a.do(bob.AccessToken, "DELETE", follow, nil, http.StatusNoContent, nil)
	if f, _ := counts(alice); f != 1 {
		t.Errorf("alice followers after unfollow = %d", f)
	}
	if _, f := counts(bob); f != 0 {
		t.Errorf("bob following after unfollow = %d", f)
	}

	a.do(alice.AccessToken, "PUT", follow, nil, http.StatusUnprocessableEntity, nil)
	a.do(alice.AccessToken, "PUT", "/v1/users/"+uuid.NewString()+"/follow", nil, http.StatusNotFound, nil)
	a.do(alice.AccessToken, "GET", "/v1/users/"+uuid.NewString()+"/followers", nil, http.StatusNotFound, nil)
}

func TestFeed(t *testing.T) {
	a := newTestAPI(t)
	alice := a.register("alice")
	bob := a.register("bob")
	carol := a.register("carol")

	fromAlice := a.post(alice.AccessToken, "alice", 1)
	a.post(carol.AccessToken, "carol", 1) // bob does not follow carol
	a.do(bob.AccessToken, "PUT", "/v1/users/"+alice.User.ID+"/follow", nil, http.StatusNoContent, nil)
	fromBob := a.post(bob.AccessToken, "bob", 1)

	feed := func() []string {
		t.Helper()
		var p page[postJSON]
		a.do(bob.AccessToken, "GET", "/v1/feed", nil, http.StatusOK, &p)
		ids := make([]string, len(p.Data))
		for i, post := range p.Data {
			ids[i] = post.ID
		}
		return ids
	}
	if got := feed(); len(got) != 2 || got[0] != fromBob.ID || got[1] != fromAlice.ID {
		t.Fatalf("feed = %v, want [%s %s]", got, fromBob.ID, fromAlice.ID)
	}

	var p page[postJSON]
	a.do(bob.AccessToken, "GET", "/v1/feed?limit=1", nil, http.StatusOK, &p)
	a.do(bob.AccessToken, "GET", "/v1/feed?limit=1&cursor="+*p.NextCursor, nil, http.StatusOK, &p)
	if len(p.Data) != 1 || p.Data[0].ID != fromAlice.ID || p.NextCursor != nil {
		t.Fatalf("feed page 2 = %+v", p)
	}

	a.do(bob.AccessToken, "DELETE", "/v1/users/"+alice.User.ID+"/follow", nil, http.StatusNoContent, nil)
	if got := feed(); len(got) != 1 || got[0] != fromBob.ID {
		t.Fatalf("feed after unfollow = %v", got)
	}
}
