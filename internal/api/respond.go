package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
)

// Problem is an RFC 9457 error body. Every error response uses it.
type Problem struct {
	Type   string       `json:"type"`
	Title  string       `json:"title"`
	Status int          `json:"status"`
	Detail string       `json:"detail,omitempty"`
	Errors []FieldError `json:"errors,omitempty"`
}

// FieldError names one invalid request field in a validation problem.
type FieldError struct {
	Field string `json:"field"`
	Code  string `json:"code"` // required, invalid, too_long, taken, ...
}

const problemTypeBase = "https://paragram.dev/errors/"

// writeProblem sends an error. kind is the last segment of the problem type URL.
func writeProblem(w http.ResponseWriter, status int, kind, detail string) {
	sendProblem(w, Problem{Type: problemTypeBase + kind, Title: http.StatusText(status), Status: status, Detail: detail})
}

func sendProblem(w http.ResponseWriter, p Problem) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(p.Status)
	_ = json.NewEncoder(w).Encode(p)
}

// writeInvalid sends a 422 listing every invalid field.
func writeInvalid(w http.ResponseWriter, errs []FieldError) {
	sendProblem(w, Problem{
		Type:   problemTypeBase + "validation",
		Title:  "Validation failed",
		Status: http.StatusUnprocessableEntity,
		Errors: errs,
	})
}

// writeInternal logs err and sends a 500 that reveals only the request ID.
func writeInternal(w http.ResponseWriter, r *http.Request, log *slog.Logger, err error) {
	id := RequestID(r.Context())
	log.ErrorContext(r.Context(), "internal error", "request_id", id, "err", err)
	writeProblem(w, http.StatusInternalServerError, "internal", "request id "+id)
}

// decode reads a JSON body into v. On failure it has already answered.
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	err := json.NewDecoder(r.Body).Decode(v)
	if err == nil {
		return true
	}
	if _, tooBig := errors.AsType[*http.MaxBytesError](err); tooBig {
		writeProblem(w, http.StatusRequestEntityTooLarge, "too-large", "body exceeds 1 MB")
	} else {
		writeProblem(w, http.StatusBadRequest, "malformed", "invalid JSON body")
	}
	return false
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
