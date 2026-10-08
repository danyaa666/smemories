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
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notefields"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/textx"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	maxLabel = 60

	// Public routes. A class behind one campus address opens the same link, so only requests that miss (unknown,
	// malformed or revoked token) count against the tight per-IP limit; the generous one is a cost guard.
	missesPerWindow = 60
	allPerWindow    = 600
	publicWindow    = 15 * time.Minute
)

// Handler serves the owner endpoints for collection links and the public lookup.
type Handler struct {
	store          *Store
	requireUser    func(http.Handler) http.Handler // auth.Handler.RequireUser
	allowedOrigins []string                        // for auth.Guard
	clientIP       func(*http.Request) string      // auth.Handler.ClientIP
	misses, all    *ratelimit.Limiter              // public routes, per client IP
	logger         *slog.Logger
	now            func() time.Time

	media                      *media.Service // photos of submissions
	maxPhoto                   int64          // largest accepted photo, bytes
	verifier                   Verifier
	inflight                   chan struct{}      // submissions holding photo bytes in memory
	subIPHr, subIPDay, subColl *ratelimit.Limiter // submissions per IP per hour and day, per collection per hour
}

// NewHandler builds the handler; now may be nil (time.Now). svc stores the photos of submissions, each at
// most maxPhoto bytes.
func NewHandler(store *Store, svc *media.Service, maxPhoto int64, requireUser func(http.Handler) http.Handler, allowedOrigins []string, clientIP func(*http.Request) string, logger *slog.Logger, now func() time.Time) *Handler {
	if now == nil {
		now = time.Now
	}
	return &Handler{store: store, requireUser: requireUser, allowedOrigins: allowedOrigins, clientIP: clientIP,
		misses: ratelimit.New(missesPerWindow, publicWindow, now), all: ratelimit.New(allPerWindow, publicWindow, now),
		logger: logger, now: now,
		media: svc, maxPhoto: maxPhoto, verifier: allowAll{}, inflight: make(chan struct{}, 2*svc.Slots()),
		subIPHr: ratelimit.New(submitPerIPHour, time.Hour, now), subIPDay: ratelimit.New(submitPerIPDay, 24*time.Hour, now),
		subColl: ratelimit.New(submitPerCollectionHour, time.Hour, now)}
}

// SetVerifier plugs in an abuse check (CAPTCHA) for submissions; the default lets everything pass.
func (h *Handler) SetVerifier(v Verifier) { h.verifier = v }

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	write := func(f http.HandlerFunc) http.Handler { return auth.Guard(h.allowedOrigins, h.requireUser(f)) }
	mux.Handle("POST /v1/yearbooks/{id}/collections", write(h.create))
	mux.Handle("GET /v1/yearbooks/{id}/collections", h.requireUser(http.HandlerFunc(h.list)))
	mux.Handle("DELETE /v1/collections/{id}", write(h.revoke))
	// Public routes: no session, no cookie, no CORS; the token is the credential.
	mux.HandleFunc("GET /v1/public/collect/{token}", h.lookup)
	mux.Handle("POST /v1/public/collect/{token}/notes", httpx.WithBodyLimit(maxBody, http.HandlerFunc(h.submit)))
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

// resolve is the front of both public routes: the per-IP limits, then the token. It answers 429, 404 or 410
// itself and returns false; on success the collection is open.
func (h *Handler) resolve(w http.ResponseWriter, r *http.Request) (Public, bool) {
	ip := h.clientIP(r)
	if ok, wait := h.all.Take(ip); !ok {
		tooMany(w, r, wait)
		return Public{}, false
	}
	if ok, wait := h.misses.Take(ip); !ok {
		tooMany(w, r, wait)
		return Public{}, false
	}
	var p Public
	hash, valid := tokenHash(r.PathValue("token"))
	err := ErrNotFound
	if valid {
		p, err = h.store.lookup(r.Context(), hash)
	}
	if !errors.Is(err, ErrNotFound) {
		h.misses.Refund(ip) // a valid token is not a guess
	}
	if err != nil {
		h.fail(w, r, err)
		return Public{}, false
	}
	if p.DeadlineAt != nil && !h.now().Before(*p.DeadlineAt) {
		httpx.WriteError(w, r, http.StatusGone, "collection_closed", "this link no longer accepts notes")
		return Public{}, false
	}
	return p, true
}

func tooMany(w http.ResponseWriter, r *http.Request, wait time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(max(int((wait+time.Second-1)/time.Second), 1)))
	httpx.WriteError(w, r, http.StatusTooManyRequests, "rate_limited", "too many requests")
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	p, ok := h.resolve(w, r)
	if !ok {
		return
	}
	refs, err := FieldsFor(r.Context(), p.YearbookID)
	var fields []notefields.FieldInfo
	if err == nil {
		fields, err = notefields.Info(refs)
	}
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, struct {
		Yearbook struct {
			Title string `json:"title"`
		} `json:"yearbook"`
		Owner struct {
			DisplayName string `json:"display_name"`
		} `json:"owner"`
		DeadlineAt *time.Time             `json:"deadline_at"`
		Open       bool                   `json:"open"`
		Fields     []notefields.FieldInfo `json:"fields"`
	}{struct {
		Title string `json:"title"`
	}{p.Title}, struct {
		DisplayName string `json:"display_name"`
	}{p.DisplayName}, p.DeadlineAt, true, fields})
}

// fail maps a store error to the shared envelope; anything else is logged (never with the token) and answers 500.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if status, code, msg, ok := mediaStatus(err); ok {
		if status == http.StatusServiceUnavailable {
			w.Header().Set("Retry-After", "2")
		}
		if status == http.StatusBadGateway {
			h.logger.Error("notes: storage failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		}
		httpx.WriteError(w, r, status, code, msg)
		return
	}
	switch {
	case errors.Is(err, ErrNotFound), errors.Is(err, media.ErrNotFound):
		httpx.WriteError(w, r, http.StatusNotFound, "not_found", "not found")
	case errors.Is(err, ErrClosed):
		httpx.WriteError(w, r, http.StatusGone, "collection_closed", "this link no longer accepts notes")
	case errors.Is(err, ErrFull):
		httpx.WriteError(w, r, http.StatusConflict, "collection_full", "this link has reached its limit of notes")
	case errors.Is(err, ErrLimitReached):
		httpx.WriteError(w, r, http.StatusConflict, "limit_reached", fmt.Sprintf("at most %d active links per yearbook", maxActive))
	default:
		if !errors.Is(err, context.Canceled) {
			h.logger.Error("notes: request failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		}
		httpx.WriteError(w, r, http.StatusInternalServerError, "internal_error", "internal server error")
	}
}
