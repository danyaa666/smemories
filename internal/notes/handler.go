package notes

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/textx"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	maxLabel        = 60
	publicPerWindow = 60 // public lookups per client IP
	publicWindow    = 15 * time.Minute
)

// Handler serves the owner endpoints for collection links and the public lookup.
type Handler struct {
	store          *Store
	requireUser    func(http.Handler) http.Handler // auth.Handler.RequireUser
	allowedOrigins []string                        // for auth.Guard
	clientIP       func(*http.Request) string      // auth.Handler.ClientIP
	lookups        *ratelimit.Limiter
	logger         *slog.Logger
	now            func() time.Time
}

// NewHandler builds the handler; now may be nil (time.Now).
func NewHandler(store *Store, requireUser func(http.Handler) http.Handler, allowedOrigins []string, clientIP func(*http.Request) string, logger *slog.Logger, now func() time.Time) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{store: store, requireUser: requireUser, allowedOrigins: allowedOrigins, clientIP: clientIP,
		lookups: ratelimit.New(publicPerWindow, publicWindow, now), logger: logger, now: now}
}

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	write := func(f http.HandlerFunc) http.Handler { return auth.Guard(h.allowedOrigins, h.requireUser(f)) }
	mux.Handle("POST /v1/yearbooks/{id}/collections", write(h.create))
	mux.Handle("GET /v1/yearbooks/{id}/collections", h.requireUser(http.HandlerFunc(h.list)))
	mux.Handle("DELETE /v1/collections/{id}", write(h.revoke))
	mux.HandleFunc("GET /v1/public/collect/{token}", h.lookup) // no session, no cookie, no CORS: the token is the credential
}

type createInput struct {
	Label      *string `json:"label"`
	DeadlineAt *string `json:"deadline_at"`
}

type createdCollection struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	DeadlineAt *time.Time `json:"deadline_at"`
	CreatedAt  time.Time  `json:"created_at"`
	Token      string     `json:"token"` // shown here and nowhere else
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if !u.EmailVerified {
		httpx.WriteError(w, r, http.StatusForbidden, "email_not_verified", "verify your email first")
		return
	}
	var in createInput
	if !httpx.DecodeJSON(w, r, &in, true) {
		return
	}
	now := h.now().UTC()
	c := Collection{ID: ulid.New(now), CreatedAt: now}
	if in.Label != nil {
		l, ok := textx.Clean(*in.Label, 0, maxLabel)
		if !ok {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_label", "invalid input: invalid_label")
			return
		}
		c.Label = l
	}
	if in.DeadlineAt != nil {
		t, err := time.Parse(time.RFC3339, *in.DeadlineAt)
		if err != nil || !t.After(now) || t.After(now.AddDate(1, 0, 0)) {
			httpx.WriteError(w, r, http.StatusBadRequest, "invalid_deadline", "invalid input: invalid_deadline")
			return
		}
		t = t.UTC()
		c.DeadlineAt = &t
	}
	token, hash, err := newToken()
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if err := h.store.create(r.Context(), u.InternalID, r.PathValue("id"), c, hash); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, struct {
		Collection createdCollection `json:"collection"`
	}{createdCollection{c.ID, c.Label, c.DeadlineAt, c.CreatedAt, token}})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	cs, err := h.store.list(r.Context(), u.InternalID, r.PathValue("id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Collections []Collection `json:"collections"`
	}{cs})
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	if err := h.store.revoke(r.Context(), u.InternalID, r.PathValue("id"), h.now()); err != nil {
		h.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	if ok, wait := h.lookups.Take(h.clientIP(r)); !ok {
		w.Header().Set("Retry-After", strconv.Itoa(max(int((wait+time.Second-1)/time.Second), 1)))
		httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "too many requests")
		return
	}
	hash, ok := tokenHash(r.PathValue("token"))
	if !ok {
		h.fail(w, r, ErrNotFound)
		return
	}
	p, err := h.store.lookup(r.Context(), hash)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	if p.DeadlineAt != nil && !h.now().Before(*p.DeadlineAt) {
		httpx.WriteError(w, r, http.StatusGone, "collection_closed", "this link no longer accepts notes")
		return
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Yearbook struct {
			Title string `json:"title"`
		} `json:"yearbook"`
		Owner struct {
			DisplayName string `json:"display_name"`
		} `json:"owner"`
		DeadlineAt *time.Time `json:"deadline_at"`
		Open       bool       `json:"open"`
	}{struct {
		Title string `json:"title"`
	}{p.Title}, struct {
		DisplayName string `json:"display_name"`
	}{p.DisplayName}, p.DeadlineAt, true})
}

// fail maps a store error to the shared envelope; anything else is logged (never with the token) and answers 500.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrLimitReached):
		httpx.WriteError(w, r, http.StatusConflict, "limit_reached", fmt.Sprintf("at most %d active links per yearbook", maxActive))
	default:
		if !errors.Is(err, context.Canceled) {
			h.logger.Error("notes: request failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
