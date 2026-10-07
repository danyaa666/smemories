package httpx

import (
	"cmp"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"runtime/debug"
	"time"
)

// DefaultMaxBody is the request-body cap applied to every route unless overridden.
const DefaultMaxBody = 1 << 20 // 1 MiB

type ctxKey int

const (
	requestIDKey ctxKey = iota
	rawBodyKey
	routeKey
)

// routeHolder lets the router tell the outer AccessLog which mux pattern matched: the mux
// sets Request.Pattern only on the request it dispatches, not on the one AccessLog holds.
type routeHolder struct{ pattern string }

func setRoute(ctx context.Context, pattern string) {
	if h, ok := ctx.Value(routeKey).(*routeHolder); ok {
		h.pattern = pattern
	}
}

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

// RequestIDFrom returns the request id set by RequestID, or "" outside a request.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

// RequestID reuses a client X-Request-Id only if it is 8-64 chars of [A-Za-z0-9-]
// (anything else could forge or inject log lines) and otherwise generates one.
// The id is echoed in the response header and stored in the request context.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-Id")
		if !validRequestID.MatchString(id) {
			var b [16]byte
			_, _ = rand.Read(b[:]) // never fails on supported platforms
			id = hex.EncodeToString(b[:])
		}
		w.Header().Set("X-Request-Id", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

// statusWriter records the status code. Unwrap keeps http.ResponseController working.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusWriter) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(b)
}

func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// AccessLog writes one JSON line per request. It logs the matched route pattern (e.g.
// "GET /v1/x/{token}", "-" when nothing matched), never the raw path, query string,
// headers or body, so tokens and credentials never reach the log.
// It must wrap Recover so that a recovered panic is logged as a 500.
func AccessLog(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w}
		route := &routeHolder{}
		r = r.WithContext(context.WithValue(r.Context(), routeKey, route))
		defer func() {
			if sw.status == 0 {
				sw.status = http.StatusOK
			}
			logger.LogAttrs(r.Context(), slog.LevelInfo, "request",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.String("method", r.Method),
				slog.String("route", cmp.Or(route.pattern, "-")),
				slog.Int("status", sw.status),
				slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			)
		}()
		next.ServeHTTP(sw, r)
	})
}

// SecurityHeaders sets the baseline response headers on every response.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// Recover turns a handler panic into a 500 envelope, logs the stack and keeps serving.
func Recover(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if rec == http.ErrAbortHandler {
				panic(rec) // deliberate abort, let net/http handle it
			}
			logger.LogAttrs(r.Context(), slog.LevelError, "panic",
				slog.String("request_id", RequestIDFrom(r.Context())),
				slog.Any("panic", rec),
				slog.String("stack", string(debug.Stack())),
			)
			if sw, ok := w.(*statusWriter); ok && sw.status != 0 {
				return // response already started, nothing more we can send
			}
			WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
		}()
		next.ServeHTTP(w, r)
	})
}

// BodyLimit caps every request body at max bytes. Reads past the cap fail with
// *http.MaxBytesError; report it with WriteBodyError (413 payload_too_large).
func BodyLimit(max int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := r.Body
		r.Body = http.MaxBytesReader(w, raw, max)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), rawBodyKey, raw)))
	})
}

// WithBodyLimit overrides the default body cap for one route, e.g. an upload endpoint.
func WithBodyLimit(max int64, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if raw, ok := r.Context().Value(rawBodyKey).(io.ReadCloser); ok {
			r.Body = http.MaxBytesReader(w, raw, max)
		}
		next.ServeHTTP(w, r)
	})
}
