package notes

import (
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/notefields"
)

const (
	defaultUploadsPerIP = 8
	defaultUploadConns  = 48

	// clientClosed is nginx's 499: the client went away, nobody reads the answer.
	clientClosed = 499
)

// Handler serves the owner endpoints for collection links and the public lookup and submission. It decodes, validates the
// shape, calls the Service and writes the envelope; the rules live in the Service.
type Handler struct {
	svc            *Service
	requireUser    func(http.Handler) http.Handler // auth.Handler.RequireUser
	allowedOrigins []string                        // for auth.Guard
	clientIP       func(*http.Request) string      // auth.Handler.ClientIP
	logger         *slog.Logger

	maxPhoto       int64 // largest accepted photo, bytes
	verifier       Verifier
	upMu           sync.Mutex     // guards upTotal and upIP
	upTotal        int            // submissions in progress (their bodies spooled to disk)
	upIP           map[string]int // ... per client IP
	upMax, upPerIP int
	tmpDir         string // where bodies are spooled; "" = the OS temp dir
}

// NewHandler builds the handler. Photos of submissions are at most maxPhoto bytes each.
func NewHandler(svc *Service, maxPhoto int64, requireUser func(http.Handler) http.Handler, allowedOrigins []string, clientIP func(*http.Request) string, logger *slog.Logger) *Handler {
	return &Handler{svc: svc, requireUser: requireUser, allowedOrigins: allowedOrigins, clientIP: clientIP, logger: logger,
		maxPhoto: maxPhoto, verifier: allowAll{},
		upIP: map[string]int{}, upMax: defaultUploadConns, upPerIP: defaultUploadsPerIP}
}

// SetUploadLimits sets how many public submissions may be in progress at once (default 48) and how many of those
// one client IP may have (default 8), and the directory their bodies are spooled to ("" = the OS temp dir).
func (h *Handler) SetUploadLimits(conns, perIP int, tmpDir string) {
	h.upMax, h.upPerIP, h.tmpDir = conns, perIP, tmpDir
}

// SetVerifier plugs in an abuse check (CAPTCHA) for submissions; the default lets everything pass.
func (h *Handler) SetVerifier(v Verifier) { h.verifier = v }

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	write := func(f http.HandlerFunc) http.Handler { return auth.Guard(h.allowedOrigins, h.requireUser(f)) }
	mux.Handle("POST /api/note-collection/create", write(h.create))
	mux.Handle("GET /api/note-collection/get-list", h.requireUser(http.HandlerFunc(h.list)))
	mux.Handle("POST /api/note-collection/revoke", write(h.revoke))
	mux.Handle("GET /api/note/get-list", h.requireUser(http.HandlerFunc(h.listNotes)))
	mux.Handle("POST /api/note/batch-update-status", write(h.updateStatus))
	mux.Handle("POST /api/note/reorder", write(h.reorder))
	mux.Handle("POST /api/note/batch-delete", write(h.deleteNotes))
	// Public routes: no session, no cookie, no CORS; the token is the credential. The access log records the route, never the query.
	mux.HandleFunc("GET /api/public-collect/get", h.lookup)
	mux.Handle("POST /api/public-collect/submit-note", httpx.WithTimeout(submitRouteTimeout, httpx.WithBodyLimit(maxBody, http.HandlerFunc(h.submit))))
}

type createInput struct {
	YearbookID string  `json:"yearbook_id"`
	Label      *string `json:"label"`
	DeadlineAt *int64  `json:"deadline_at"` // Unix ms
}

// collectionJSON is a Collection as the owner's list shows it; instants are Unix ms.
type collectionJSON struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	DeadlineAt *int64 `json:"deadline_at"`
	CreatedAt  int64  `json:"created_at"`
	RevokedAt  *int64 `json:"revoked_at"`
	NoteCount  int    `json:"note_count"`
}

type createdCollection struct {
	ID         string `json:"id"`
	Label      string `json:"label"`
	DeadlineAt *int64 `json:"deadline_at"`
	CreatedAt  int64  `json:"created_at"`
	Token      string `json:"token"` // shown here and nowhere else
}

type idInput struct {
	ID string `json:"id"`
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in createInput
	if !httpx.DecodeJSON(w, r, &in, true) {
		return
	}
	c, err := h.svc.CreateCollection(r.Context(), u.InternalID, u.EmailVerified, &CreateInput{YearbookID: in.YearbookID, Label: in.Label, DeadlineAt: in.DeadlineAt})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, &createdCollection{c.ID, c.Label, c.DeadlineAt, c.CreatedAt, c.Token})
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	cs, err := h.svc.ListCollections(r.Context(), u.InternalID, r.URL.Query().Get("yearbook_id"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	items := make([]collectionJSON, len(cs))
	for i, c := range cs {
		items[i] = collectionJSON{ID: c.ID, Label: c.Label, DeadlineAt: c.DeadlineAt, CreatedAt: c.CreatedAt, RevokedAt: c.RevokedAt, NoteCount: c.NoteCount}
	}
	httpx.OK(w, r, httpx.Page[collectionJSON]{Items: items}) // not paginated yet: the whole list is one page
}

func (h *Handler) revoke(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in idInput
	if !httpx.DecodeJSON(w, r, &in, true) {
		return
	}
	if err := h.svc.RevokeCollection(r.Context(), u.InternalID, in.ID); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, nil)
}

type lookupJSON struct {
	Yearbook   titleJSON              `json:"yearbook"`
	Owner      ownerJSON              `json:"owner"`
	DeadlineAt *int64                 `json:"deadline_at"`
	Open       bool                   `json:"open"`
	Fields     []notefields.FieldInfo `json:"fields"`
}

type titleJSON struct {
	Title string `json:"title"`
}

type ownerJSON struct {
	DisplayName string `json:"display_name"`
}

func (h *Handler) lookup(w http.ResponseWriter, r *http.Request) {
	p, fields, err := h.svc.Lookup(r.Context(), h.clientIP(r), r.URL.Query().Get("token"))
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, &lookupJSON{titleJSON{p.Title}, ownerJSON{p.DisplayName}, p.DeadlineAt, true, fields})
}

// fail answers err. A request whose client went away gets 499 and a debug line; a *RetryAfterError sets Retry-After; the
// rest is httpx.Fail. Nothing logged here contains the token: errors never carry the URL.
func (h *Handler) fail(w http.ResponseWriter, r *http.Request, err error) {
	if r.Context().Err() != nil { // nobody reads the answer and nothing is wrong on our side
		h.logger.Debug("notes: client gone", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		httpx.WriteError(w, r, clientClosed, "client_closed", "client closed the request")
		return
	}
	var ra *RetryAfterError
	if errors.As(err, &ra) {
		w.Header().Set("Retry-After", strconv.Itoa(max(int((ra.After+time.Second-1)/time.Second), 1)))
	}
	if apperr.Is(err, CodeStorage) { // Fail logs only the codes that are our fault by default
		h.logger.Error("notes: storage failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
	}
	httpx.Fail(w, r, err)
}
