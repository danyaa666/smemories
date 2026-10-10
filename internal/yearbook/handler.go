// Package yearbook is the user's yearbooks and the owner's profile page information (T-008).
// Every endpoint needs a session and only ever sees the caller's own books.
package yearbook

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	defaultLimit = 20
	maxLimit     = 50

	// A cursor time outside years 1..9999 is a tampered token, not a position in anyone's list.
	minCursorMs = -62135596800000 // 0001-01-01T00:00:00Z
	maxCursorMs = 253402300799999 // 9999-12-31T23:59:59.999Z
)

// Handler serves /v1/yearbooks.
type Handler struct {
	store          *Store
	svc            *Service
	requireUser    func(http.Handler) http.Handler // auth.Handler.RequireUser
	allowedOrigins []string                        // for auth.Guard
	logger         *slog.Logger
	now            func() time.Time
}

// NewHandler builds the handler; now may be nil (time.Now). T-070 folds the store calls into the service.
func NewHandler(store *Store, svc *Service, requireUser func(http.Handler) http.Handler, allowedOrigins []string, logger *slog.Logger, now func() time.Time) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{store: store, svc: svc, requireUser: requireUser, allowedOrigins: allowedOrigins, logger: logger, now: now}
}

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	read := func(f http.HandlerFunc) http.Handler { return h.requireUser(f) }
	write := func(f http.HandlerFunc) http.Handler { return auth.Guard(h.allowedOrigins, h.requireUser(f)) }
	mux.Handle("POST /v1/yearbooks", write(h.create))
	mux.Handle("GET /v1/yearbooks", read(h.list))
	mux.Handle("GET /v1/yearbooks/{id}", read(h.get))
	mux.Handle("PATCH /v1/yearbooks/{id}", write(h.patch))
	mux.Handle("PUT /v1/yearbooks/{id}/profile", write(h.putProfile))
	mux.Handle("DELETE /v1/yearbooks/{id}", write(h.delete))
}

// owner is the signed-in user's internal id; RequireUser has always stored the user by now.
func owner(r *http.Request) uint64 {
	u, _ := auth.UserFrom(r.Context())
	return u.InternalID
}

type bookEnvelope struct {
	Yearbook Yearbook `json:"yearbook"`
}

// MarshalJSON renders the stored milliseconds as RFC 3339 UTC, as the API always has.
func (y Yearbook) MarshalJSON() ([]byte, error) {
	type plain Yearbook // no methods, so no recursion
	return json.Marshal(struct {
		plain
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}{plain(y), time.UnixMilli(y.CreatedAt).UTC(), time.UnixMilli(y.UpdatedAt).UTC()})
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in bookInput
	if !httpx.DecodeJSON(w, r, &in, true) {
		return
	}
	now := h.now().UTC()
	u, _ := auth.UserFrom(r.Context())
	y := Yearbook{ID: ulid.New(now), PageSize: "A5", CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
		Profile: Profile{ID: ulid.New(now), IsOwner: true, FullName: u.DisplayName}}
	if err := in.applyTo(&y); err != nil {
		h.fail(w, r, err)
		return
	}
	if in.Title == nil {
		h.fail(w, r, ValidationError{"invalid_title"})
		return
	}
	if in.Language == nil {
		h.fail(w, r, ValidationError{"invalid_language"})
		return
	}
	if y.CoverMediaID != nil { // a new book has no media yet
		h.fail(w, r, ValidationError{"invalid_media"})
		return
	}
	if err := h.store.create(r.Context(), owner(r), y); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, bookEnvelope{y})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit := defaultLimit
	if s := r.URL.Query().Get("limit"); s != "" {
		n, err := strconv.Atoi(s)
		if err != nil || n < 1 || n > maxLimit {
			h.fail(w, r, ValidationError{"invalid_limit"})
			return
		}
		limit = n
	}
	var cur *cursor
	if s := r.URL.Query().Get("cursor"); s != "" {
		c, ok := parseCursor(s)
		if !ok {
			h.fail(w, r, ValidationError{"invalid_cursor"})
			return
		}
		cur = &c
	}
	books, err := h.store.list(r.Context(), owner(r), cur, limit+1) // one extra row says whether another page exists
	if err != nil {
		h.fail(w, r, err)
		return
	}
	var next *string
	if len(books) > limit {
		books = books[:limit]
		last := books[limit-1]
		c := formatCursor(cursor{last.UpdatedAt, last.ID})
		next = &c
	}
	if books == nil {
		books = []Yearbook{}
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Yearbooks  []Yearbook `json:"yearbooks"`
		NextCursor *string    `json:"next_cursor"`
	}{books, next})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	y, err := h.store.get(r.Context(), owner(r), r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, bookEnvelope{y})
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	var in bookInput
	if !httpx.DecodeJSON(w, r, &in, true) {
		return
	}
	y, err := h.store.modify(r.Context(), owner(r), r.PathValue("id"), h.now().UnixMilli(), in.applyTo)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, bookEnvelope{y})
}

func (h *Handler) putProfile(w http.ResponseWriter, r *http.Request) {
	var in profileInput
	if !httpx.DecodeJSON(w, r, &in, true) {
		return
	}
	now := h.now()
	y, err := h.store.modify(r.Context(), owner(r), r.PathValue("id"), now.UnixMilli(), func(y *Yearbook) error {
		return in.applyTo(&y.Profile, now)
	})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, bookEnvelope{y})
}

// delete removes the book's stored files first: if the store fails, nothing is deleted and the
// caller can retry (502 storage_error).
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if err := h.svc.Delete(r.Context(), owner(r), r.PathValue("id")); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// fail maps a store or validation error to the shared envelope; anything else is logged
// (never with field values) and answers 500.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	var ve ValidationError
	var se StorageError
	switch {
	case errors.As(err, &ve):
		httpx.WriteError(w, r, http.StatusBadRequest, ve.Code, "invalid input: "+ve.Code)
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "yearbook not found")
	case errors.As(err, &se):
		h.logger.Error("yearbook: storage purge failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		httpx.WriteError(w, r, http.StatusBadGateway, "storage_error", "object storage failed")
	case errors.Is(err, ErrLimitReached):
		httpx.WriteError(w, r, http.StatusConflict, "limit_reached", fmt.Sprintf("at most %d yearbooks per user", maxBooksOwned))
	default:
		if !errors.Is(err, context.Canceled) {
			h.logger.Error("yearbook: request failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}

// formatCursor / parseCursor: an opaque token "<unix milliseconds>.<book id>". Tampering can
// only move the page position inside the caller's own books, since every query is owner-scoped.
func formatCursor(c cursor) string {
	return base64.RawURLEncoding.EncodeToString([]byte(strconv.FormatInt(c.updatedAt, 10) + "." + c.publicID))
}

func parseCursor(s string) (cursor, bool) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return cursor{}, false
	}
	ms, id, ok := strings.Cut(string(raw), ".")
	n, perr := strconv.ParseInt(ms, 10, 64)
	if !ok || perr != nil || len(id) != 26 || n < minCursorMs || n > maxCursorMs {
		return cursor{}, false
	}
	return cursor{n, id}, true
}
