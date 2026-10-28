package api

import (
	"errors"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgconn"
)

// Limits mirror the CHECK constraints in the migrations; the database
// enforces them again.
const (
	maxDisplayName = 50
	maxBio         = 150
	maxWebsite     = 200
	maxEmail       = 254
	minPassword    = 8
	maxPassword    = 128
)

var usernameRe = regexp.MustCompile(`^[a-z0-9._]{1,30}$`)

// fieldErrors collects validation failures for one request.
type fieldErrors []FieldError

func (v *fieldErrors) check(ok bool, field, code string) {
	if !ok {
		*v = append(*v, FieldError{Field: field, Code: code})
	}
}

func (v *fieldErrors) maxLen(s string, n int, field string) {
	v.check(utf8.RuneCountInString(s) <= n, field, "too_long")
}

func normalizeUsername(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func validEmail(s string) bool {
	a, err := mail.ParseAddress(s)
	return err == nil && a.Address == s && len(s) <= maxEmail
}

func validWebsite(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// uniqueViolation returns the constraint name when err is a unique violation.
func uniqueViolation(err error) (string, bool) {
	if pgErr, ok := errors.AsType[*pgconn.PgError](err); ok && pgErr.Code == "23505" {
		return pgErr.ConstraintName, true
	}
	return "", false
}
