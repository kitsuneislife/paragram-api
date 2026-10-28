package api

import (
	"encoding/json"
	"net/http"
)

// Problem is an RFC 9457 error body. Every error response uses it.
type Problem struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail,omitempty"`
}

const problemTypeBase = "https://paragram.dev/errors/"

// writeProblem sends an error. kind is the last segment of the problem type URL.
func writeProblem(w http.ResponseWriter, status int, kind, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(Problem{
		Type:   problemTypeBase + kind,
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
