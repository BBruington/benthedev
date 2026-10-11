package auth

import (
	"encoding/json"
	"net/http"
)

// problem is the RFC 9457 problem+json body that huma writes for API errors. Writing the same shape
// here, without importing huma, means every route's errors look alike to the web client and the API
// toolchain can be swapped without touching this package (spec §8.6).
type problem struct {
	Title  string `json:"title,omitempty"`
	Status int    `json:"status,omitempty"`
	Detail string `json:"detail,omitempty"`
}

// WriteProblem answers a plain net/http route with a problem+json body.
func WriteProblem(writer http.ResponseWriter, status int, detail string) {
	writer.Header().Set("Content-Type", "application/problem+json")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(problem{
		Title:  http.StatusText(status),
		Status: status,
		Detail: detail,
	})
}
