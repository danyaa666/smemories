// Package httpx holds the HTTP plumbing shared by every domain package:
// router, middleware, JSON and error-envelope helpers.
package httpx

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
