package bookdata

import (
	"net/http"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

// Handler serves GET /api/book-data/get.
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
	mux.Handle("GET /api/book-data/get", h.requireUser(http.HandlerFunc(h.get)))
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	b, err := h.svc.Get(r.Context(), u.InternalID, u.EmailVerified, r.URL.Query().Get("yearbook_id"))
	if err != nil {
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, r, b)
}
