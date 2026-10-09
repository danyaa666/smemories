// Package httpx holds the HTTP plumbing shared by every domain package:
// router, middleware, JSON and error-envelope helpers.
package httpx

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
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

// BodyPace is what a slow-upload route demands of its client.
type BodyPace struct {
	Idle    time.Duration // longest silence between two reads
	Total   time.Duration // longest time for the whole body
	MinRate int64         // bytes per second the body must average once Grace has passed; 0 = no minimum
	Grace   time.Duration
}

// PaceBody makes every read of r.Body move the connection's read deadline to p.Idle from now, but never past
// p.Total from this call, and fails the read when the average rate stays under p.MinRate after p.Grace. A client
// that stalls or drips bytes is dropped (the read fails with os.ErrDeadlineExceeded, which WriteBodyError reports
// as 408) instead of holding resources until Total.
func PaceBody(w http.ResponseWriter, r *http.Request, p BodyPace) {
	now := time.Now()
	r.Body = &pacedBody{ReadCloser: r.Body, rc: http.NewResponseController(w), p: p, start: now, end: now.Add(p.Total)}
}

type pacedBody struct {
	io.ReadCloser
	rc    *http.ResponseController
	p     BodyPace
	start time.Time
	end   time.Time
	n     int64
}

func (b *pacedBody) Read(p []byte) (int, error) {
	_ = b.rc.SetReadDeadline(minTime(time.Now().Add(b.p.Idle), b.end))
	n, err := b.ReadCloser.Read(p)
	b.n += int64(n)
	if err == nil && b.p.MinRate > 0 {
		if el := time.Since(b.start); el > b.p.Grace && float64(b.n) < float64(b.p.MinRate)*el.Seconds() {
			return n, os.ErrDeadlineExceeded
		}
	}
	return n, err
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// statusOK is the v2 success marker (docs/api-contract.md section 2).
const statusOK = "OK"

type okEnvelope struct {
	Status string `json:"status"`
	Data   any    `json:"data"`
}

type v2ErrorEnvelope struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	RequestID    string `json:"request_id"`
}

// OK writes HTTP 200 {"status":"OK","data":data}; nil data becomes {}.
func OK(w http.ResponseWriter, _ *http.Request, data any) {
	if data == nil {
		data = struct{}{}
	}
	WriteJSON(w, http.StatusOK, okEnvelope{Status: statusOK, Data: data})
}

// genericMessage is all a client learns about a server-side failure.
const genericMessage = "internal server error"

// Fail answers err with the v2 error envelope and the HTTP status of its apperr code
// (unknown errors are ERROR_INTERNAL 500). Internal, Unavailable and BadGateway are logged
// at ERROR with the request id and the full wrapped error and reach the client only as a
// generic message; other codes send the apperr message and log nothing.
func Fail(w http.ResponseWriter, r *http.Request, err error) {
	code := apperr.CodeOf(err)
	msg := apperr.Message(err)
	switch code {
	case apperr.Internal, apperr.Unavailable, apperr.BadGateway:
		LoggerFrom(r.Context()).LogAttrs(r.Context(), slog.LevelError, "request failed",
			slog.String("request_id", RequestIDFrom(r.Context())),
			slog.String("code", code.Name),
			slog.Any("error", err),
		)
		msg = genericMessage
	default:
		if msg == "" {
			msg = strings.ToLower(http.StatusText(code.HTTP))
		}
	}
	writeV2Error(w, r, code.HTTP, code.Name, msg)
}

func writeV2Error(w http.ResponseWriter, r *http.Request, status int, code, msg string) {
	WriteJSON(w, status, v2ErrorEnvelope{Status: code, ErrorMessage: msg, RequestID: RequestIDFrom(r.Context())})
}

// Page is the keyset-paginated list payload. NextID is "" on the last page. Items must be
// non-nil (use make) or it encodes as null.
type Page[T any] struct {
	Items  []T    `json:"items"`
	NextID string `json:"next_id"`
}

// ParseLimit reads the "limit" query parameter: def when absent, ERROR_PARAM unless 1..max.
func ParseLimit(r *http.Request, def, limitMax int) (int, error) {
	raw := r.URL.Query().Get("limit")
	if raw == "" {
		return def, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > limitMax {
		return 0, apperr.New(apperr.Param, "limit must be a number from 1 to "+strconv.Itoa(limitMax))
	}
	return n, nil
}

type loggerKeyType struct{}

// withLogger stores the server logger for Fail.
func withLogger(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), loggerKeyType{}, logger)))
	})
}

// LoggerFrom returns the logger NewRouter put in the context, or the default logger.
func LoggerFrom(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKeyType{}).(*slog.Logger); ok {
		return l
	}
	return slog.Default()
}
