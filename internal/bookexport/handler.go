package bookexport

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/ulid"
)

// Handler serves GET /api/book-export/get-text.
type Handler struct {
	svc         *Service
	requireUser func(http.Handler) http.Handler // auth.Handler.RequireUser
}

// NewHandler builds the handler.
func NewHandler(svc *Service, requireUser func(http.Handler) http.Handler) *Handler {
	return &Handler{svc: svc, requireUser: requireUser}
}

// Routes registers the endpoint on mux (pass to httpx.NewRouter). A GET changes nothing, so there is no origin guard.
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.Handle("GET /api/book-export/get-text", h.requireUser(http.HandlerFunc(h.getText)))
}

func (h *Handler) getText(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	q := r.URL.Query()
	if !ulid.Valid(q.Get("yearbook_id")) { // canonical public id only (T-100), before any query
		httpx.Fail(w, r, apperr.New(apperr.Param, "yearbook_id must be a yearbook id"))
		return
	}
	e, err := h.svc.GetText(r.Context(), u.InternalID, u.EmailVerified, q.Get("yearbook_id"), q.Get("lang"))
	if err != nil {
		var ra *RetryAfterError
		if errors.As(err, &ra) {
			w.Header().Set("Retry-After", strconv.Itoa(max(int((ra.After+time.Second-1)/time.Second), 1)))
		}
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, r, e)
}
