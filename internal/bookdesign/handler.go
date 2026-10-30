package bookdesign

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/ulid"
)

// Handler serves /api/book-design/*: the owner reads, replaces and resets the design of a book.
type Handler struct {
	svc            *Service
	requireUser    func(http.Handler) http.Handler // auth.Handler.RequireUser
	allowedOrigins []string                        // for auth.Guard
}

// NewHandler builds the handler.
func NewHandler(svc *Service, requireUser func(http.Handler) http.Handler, allowedOrigins []string) *Handler {
	return &Handler{svc: svc, requireUser: requireUser, allowedOrigins: allowedOrigins}
}

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.Handle("GET /api/book-design/get", h.requireUser(http.HandlerFunc(h.get)))
	mux.Handle("POST /api/book-design/update", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.update))))
	mux.Handle("POST /api/book-design/reset", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.reset))))
}

type updateInput struct {
	YearbookID string          `json:"yearbook_id"`
	Options    json.RawMessage `json:"options"`
}

type resetInput struct {
	YearbookID string `json:"yearbook_id"`
}

// validID answers ERROR_PARAM before any query for an id that is not a canonical public id.
func validID(w http.ResponseWriter, r *http.Request, id string) bool {
	if !ulid.Valid(id) {
		fail(w, r, apperr.New(apperr.Param, "yearbook_id must be a yearbook id"))
		return false
	}
	return true
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("yearbook_id")
	if !validID(w, r, id) {
		return
	}
	v, err := h.svc.Get(r.Context(), u.InternalID, id)
	respond(w, r, v, err)
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in updateInput
	if !httpx.DecodeJSON(w, r, &in) || !validID(w, r, in.YearbookID) {
		return
	}
	if len(in.Options) == 0 {
		fail(w, r, apperr.New(apperr.Param, "options is required"))
		return
	}
	v, err := h.svc.Update(r.Context(), u.InternalID, in.YearbookID, in.Options)
	respond(w, r, v, err)
}

func (h *Handler) reset(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in resetInput
	if !httpx.DecodeJSON(w, r, &in) || !validID(w, r, in.YearbookID) {
		return
	}
	err := h.svc.Reset(r.Context(), u.InternalID, in.YearbookID)
	respond(w, r, nil, err)
}

func respond(w http.ResponseWriter, r *http.Request, data any, err error) {
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.OK(w, r, data)
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	var ra *RetryAfterError
	if errors.As(err, &ra) {
		w.Header().Set("Retry-After", strconv.Itoa(max(int((ra.After+time.Second-1)/time.Second), 1)))
	}
	httpx.Fail(w, r, err)
}
