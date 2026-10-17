package printfeedback

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

const maxBody = 1 << 10 // {"verdict":"yes","browser_family":"chromium_desktop"} is about 60 bytes

// Handler serves POST /api/print-feedback/create. It is public: no session, so nothing identifies the caller.
type Handler struct {
	svc            *Service
	allowedOrigins []string                   // for auth.Guard
	clientIP       func(*http.Request) string // auth.Handler.ClientIP
}

func NewHandler(svc *Service, allowedOrigins []string, clientIP func(*http.Request) string) *Handler {
	return &Handler{svc: svc, allowedOrigins: allowedOrigins, clientIP: clientIP}
}

// Routes registers the endpoint on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	mux.Handle("POST /api/print-feedback/create", auth.Guard(h.allowedOrigins, httpx.WithBodyLimit(maxBody, http.HandlerFunc(h.create))))
}

// createInput is the whole body; an unknown field is ERROR_PARAM (httpx.DecodeJSON).
type createInput struct {
	Verdict       string `json:"verdict"`
	BrowserFamily string `json:"browser_family"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in createInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.Create(r.Context(), h.clientIP(r), in.Verdict, in.BrowserFamily); err != nil {
		var ra *RetryAfterError
		if errors.As(err, &ra) {
			w.Header().Set("Retry-After", strconv.Itoa(int((ra.After+time.Second-1)/time.Second)))
		}
		httpx.Fail(w, r, err)
		return
	}
	httpx.OK(w, r, nil)
}
