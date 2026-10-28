// Command seed fills a running local stack with fake users, follows, posts,
// likes and comments through the public API, so every write takes the real
// path (presigned upload, worker, counters). Run the api and worker first.
// Rerunning logs the same users in and adds more posts.
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"log"
	"net/http"
	"time"
)

const password = "seed-password"

// api is the base URL every request goes to.
var api string

func main() {
	flag.StringVar(&api, "api", "http://localhost:8080", "API base URL")
	users := flag.Int("users", 8, "how many users")
	posts := flag.Int("posts", 2, "posts per user")
	flag.Parse()
	if *users < 2 || *users > 9 {
		// The auth rate limit allows 20 requests a minute per IP.
		log.Fatal("-users must be between 2 and 9")
	}
	seed(*users, *posts)
}

// seed fails the process on the first unexpected response.
func seed(users, posts int) {
	tokens := make([]string, users)
	ids := make([]string, users)
	for i := range tokens {
		tokens[i], ids[i] = session(fmt.Sprintf("seed%d", i+1))
	}

	// Everyone follows the next three users, wrapping around.
	for i, t := range tokens {
		for j := 1; j <= 3 && j < len(ids); j++ {
			call(t, "PUT", "/v1/users/"+ids[(i+j)%len(ids)]+"/follow", nil, http.StatusNoContent, nil)
		}
	}

	var postIDs []string
	var authors []string // token per post; others get 404 until it is ready
	for i, t := range tokens {
		for n := range posts {
			media := []string{upload(t, i*31+n*7), upload(t, i*17+n*13+5)}
			var p struct{ ID string }
			call(t, "POST", "/v1/posts", map[string]any{
				"caption":   fmt.Sprintf("post %d from seed%d", n+1, i+1),
				"media_ids": media[:1+n%2],
			}, http.StatusCreated, &p)
			postIDs = append(postIDs, p.ID)
			authors = append(authors, t)
		}
	}

	// Likes and comments need ready posts, so wait for the worker.
	for k, id := range postIDs {
		waitReady(authors[k], id)
	}
	for i, t := range tokens {
		for k, id := range postIDs {
			if (i+k)%3 == 0 {
				call(t, "PUT", "/v1/posts/"+id+"/like", nil, http.StatusNoContent, nil)
			}
			if (i+k)%5 == 0 {
				call(t, "POST", "/v1/posts/"+id+"/comments", map[string]string{"body": fmt.Sprintf("nice one, from seed%d", i+1)}, http.StatusCreated, nil)
			}
		}
	}
	log.Printf("seeded %d users (seed1..seed%d, password %q) and %d posts", len(ids), len(ids), password, len(postIDs))
}

// session logs the user in, registering them first if needed.
func session(username string) (token, id string) {
	var out struct {
		AccessToken string `json:"access_token"`
		User        struct{ ID string }
	}
	if status := try("", "POST", "/v1/auth/login", map[string]string{"login": username, "password": password}, &out); status == http.StatusOK {
		return out.AccessToken, out.User.ID
	}
	call("", "POST", "/v1/auth/register", map[string]string{
		"username": username, "email": username + "@example.com", "password": password, "display_name": "Seed " + username[4:],
	}, http.StatusCreated, &out)
	return out.AccessToken, out.User.ID
}

// upload sends a generated gradient JPEG and returns its media id.
func upload(token string, hue int) string {
	var m struct {
		ID        string `json:"id"`
		UploadURL string `json:"upload_url"`
	}
	call(token, "POST", "/v1/media", map[string]string{"purpose": "post"}, http.StatusCreated, &m)
	img := image.NewRGBA(image.Rect(0, 0, 1080, 1080))
	for y := range 1080 {
		for x := range 1080 {
			img.Set(x, y, color.RGBA{uint8(hue * 40), uint8(x / 5), uint8(y / 5), 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		log.Fatal(err)
	}
	req, err := http.NewRequest("PUT", m.UploadURL, &buf)
	if err != nil {
		log.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != http.StatusOK {
		log.Fatalf("upload: status %d", res.StatusCode)
	}
	return m.ID
}

func waitReady(token, id string) {
	for range 60 {
		var p struct{ Ready bool }
		call(token, "GET", "/v1/posts/"+id, nil, http.StatusOK, &p)
		if p.Ready {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
	log.Fatalf("post %s not ready after 30s; is the worker running?", id)
}

func call(token, method, path string, body any, want int, out any) {
	if got := try(token, method, path, body, out); got != want {
		log.Fatalf("%s %s: status %d, want %d", method, path, got, want)
	}
}

// try sends one request and decodes a 2xx JSON body into out.
func try(token, method, path string, body any, out any) int {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			log.Fatal(err)
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, api+path, r)
	if err != nil {
		log.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode/100 == 2 && out != nil {
		if err := json.NewDecoder(res.Body).Decode(out); err != nil {
			log.Fatal(err)
		}
	}
	return res.StatusCode
}
