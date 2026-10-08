// Package httpx holds the HTTP plumbing shared by every domain package:
// router, middleware, JSON and error-envelope helpers.
package httpx

import (
	"encoding/json"
	"io"
	"net/http"
	"time"
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

// ExtendDeadlines moves the connection's read and write deadlines of this request out, for routes whose
// bodies are slow to arrive (phone uploads) while the global timeouts stay short. A server or writer that
// does not support it keeps the global deadlines.
func ExtendDeadlines(w http.ResponseWriter, read, write time.Duration) {
	rc := http.NewResponseController(w)
	now := time.Now()
	if read > 0 {
		_ = rc.SetReadDeadline(now.Add(read))
	}
	if write > 0 {
		_ = rc.SetWriteDeadline(now.Add(write))
	}
}

// IdleBody makes every read of r.Body move the connection's read deadline to idle from now, but never past total
// from this call. A client that stops sending for idle is dropped (the read fails with os.ErrDeadlineExceeded,
// which WriteBodyError reports as 408) instead of holding resources until total.
func IdleBody(w http.ResponseWriter, r *http.Request, idle, total time.Duration) {
	r.Body = &idleBody{ReadCloser: r.Body, rc: http.NewResponseController(w), idle: idle, end: time.Now().Add(total)}
}

type idleBody struct {
	io.ReadCloser
	rc   *http.ResponseController
	idle time.Duration
	end  time.Time
}

func (b *idleBody) Read(p []byte) (int, error) {
	_ = b.rc.SetReadDeadline(minTime(time.Now().Add(b.idle), b.end))
	return b.ReadCloser.Read(p)
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}
