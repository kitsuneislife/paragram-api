package api

import (
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultLimit = 20
	maxLimit     = 100
)

// cursor is a keyset position: the (created_at, id) of the last item served.
type cursor struct {
	Time time.Time
	ID   uuid.UUID
}

// Starting positions: newest-first lists begin above everything, oldest-first below.
var (
	newestFirst = cursor{time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC), uuid.Max}
	oldestFirst = cursor{time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), uuid.Nil}
)

func (c cursor) String() string {
	return base64.RawURLEncoding.EncodeToString([]byte(c.Time.Format(time.RFC3339Nano) + "," + c.ID.String()))
}

func parseCursor(s string) (cursor, bool) {
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, false
	}
	ts, id, ok := strings.Cut(string(b), ",")
	if !ok {
		return cursor{}, false
	}
	t, terr := time.Parse(time.RFC3339Nano, ts)
	u, uerr := uuid.Parse(id)
	return cursor{t, u}, terr == nil && uerr == nil
}

// pageQuery reads ?limit and ?cursor; start is the position when no cursor is
// given. It asks for one extra row so the handler knows whether more follow.
// On failure it has already answered.
func pageQuery(w http.ResponseWriter, r *http.Request, start cursor) (c cursor, limit int, ok bool) {
	var v fieldErrors
	limit = defaultLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		v.check(err == nil && n >= 1 && n <= maxLimit, "limit", "invalid")
		limit = n
	}
	c = start
	if s := r.URL.Query().Get("cursor"); s != "" {
		var cok bool
		c, cok = parseCursor(s)
		v.check(cok, "cursor", "invalid")
	}
	if len(v) > 0 {
		writeInvalid(w, v)
		return cursor{}, 0, false
	}
	return c, limit, true
}

type page[T any] struct {
	Data       []T     `json:"data"`
	NextCursor *string `json:"next_cursor"`
}

// trim drops the extra row fetched by pageQuery. next is the cursor after the
// last row kept, or nil at the end of the list.
func trim[T any](rows []T, limit int, at func(T) cursor) (kept []T, next *string) {
	if len(rows) <= limit {
		return rows, nil
	}
	s := at(rows[limit-1]).String()
	return rows[:limit], &s
}
