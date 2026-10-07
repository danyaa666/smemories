// Package httpx holds the HTTP plumbing shared by every domain package:
// router, middleware, JSON and error-envelope helpers.
package httpx

import (
	"encoding/json"
	"net/http"
)

// WriteJSON writes v as JSON with the given status. It marshals first so a
// marshalling failure becomes a clean 500 instead of a half-written body.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}
