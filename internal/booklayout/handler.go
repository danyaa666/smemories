package booklayout

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/ulid"
)

// maxBody caps the save request before it is decoded: the document plus the other two fields.
const maxBody = MaxBytes + 4<<10

// Handler serves /api/book-layout/*: the owner reads, saves and resets the free layout of a book.
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
	mux.Handle("GET /api/book-layout/get", h.requireUser(http.HandlerFunc(h.get)))
	mux.Handle("POST /api/book-layout/save", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.save))))
	mux.Handle("POST /api/book-layout/reset", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.reset))))
	mux.Handle("POST /api/book-layout/version-list", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.versionList))))
	mux.Handle("POST /api/book-layout/version-get", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.versionGet))))
	mux.Handle("POST /api/book-layout/restore", auth.Guard(h.allowedOrigins, h.requireUser(http.HandlerFunc(h.restore))))
}

type versionInput struct {
	YearbookID  string `json:"yearbook_id"`
	ID          string `json:"id"`
	BaseVersion *int64 `json:"base_version"`
}

func (h *Handler) versionList(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in resetInput
	if !httpx.DecodeJSON(w, r, &in) || !validID(w, r, in.YearbookID) {
		return
	}
	items, err := h.svc.Versions(r.Context(), u.InternalID, in.YearbookID)
	respond(w, r, map[string]any{"items": items, "next_id": ""}, err) // at most 30, so never a next page (api-contract.md section 5 shape)
}

// decodeVersion decodes the body of version-get and restore and checks the two ids.
func decodeVersion(w http.ResponseWriter, r *http.Request) (*versionInput, bool) {
	var in versionInput
	if !httpx.DecodeJSON(w, r, &in) || !validID(w, r, in.YearbookID) {
		return nil, false
	}
	if !ulid.Valid(in.ID) {
		fail(w, r, apperr.New(apperr.Param, "id must be a layout version id"))
		return nil, false
	}
	return &in, true
}

func (h *Handler) versionGet(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	in, ok := decodeVersion(w, r)
	if !ok {
		return
	}
	v, err := h.svc.Version(r.Context(), u.InternalID, in.YearbookID, in.ID)
	respond(w, r, v, err)
}

func (h *Handler) restore(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	in, ok := decodeVersion(w, r)
	if !ok {
		return
	}
	if in.BaseVersion == nil {
		fail(w, r, apperr.New(apperr.Param, "base_version is required"))
		return
	}
	v, err := h.svc.Restore(r.Context(), u.InternalID, in.YearbookID, in.ID, *in.BaseVersion)
	respond(w, r, v, err)
}

type saveInput struct {
	YearbookID  string          `json:"yearbook_id"`
	BaseVersion *int64          `json:"base_version"`
	Layout      json.RawMessage `json:"layout"`
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

// watchBody notes that the body cap was hit, so the handler can count it.
type watchBody struct {
	io.ReadCloser
	tooLarge bool
}

func (b *watchBody) Read(p []byte) (int, error) {
	n, err := b.ReadCloser.Read(p)
	var big *http.MaxBytesError
	if errors.As(err, &big) {
		b.tooLarge = true
	}
	return n, err
}

func (h *Handler) save(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	body := &watchBody{ReadCloser: http.MaxBytesReader(w, r.Body, maxBody)}
	r.Body = body
	var in saveInput
	if !httpx.DecodeJSON(w, r, &in) {
		if body.tooLarge {
			h.svc.Counted(ResultTooLarge)
		}
		return
	}
	if !validID(w, r, in.YearbookID) {
		return
	}
	if len(in.Layout) > MaxBytes {
		h.svc.Counted(ResultTooLarge)
		fail(w, r, apperr.New(apperr.TooLarge, "layout is larger than 131072 bytes"))
		return
	}
	if in.BaseVersion == nil || len(in.Layout) == 0 || string(in.Layout) == "null" {
		fail(w, r, apperr.New(apperr.Param, "base_version and layout are required"))
		return
	}
	v, err := h.svc.Save(r.Context(), u.InternalID, in.YearbookID, *in.BaseVersion, in.Layout)
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

// dataError is the v2 error envelope with a data member: the problem list of a 400 and the stored document of a 409.
type dataError struct {
	Status       string `json:"status"`
	ErrorMessage string `json:"error_message"`
	RequestID    string `json:"request_id"`
	Data         any    `json:"data"`
}

func fail(w http.ResponseWriter, r *http.Request, err error) {
	var ra *RetryAfterError
	var inv *InvalidError
	var con *ConflictError
	switch {
	case errors.As(err, &ra):
		w.Header().Set("Retry-After", strconv.Itoa(max(int((ra.After+time.Second-1)/time.Second), 1)))
	case errors.As(err, &inv):
		writeData(w, r, apperr.CodeOf(err), apperr.Message(err), map[string]any{"problems": inv.Problems})
		return
	case errors.As(err, &con):
		writeData(w, r, apperr.CodeOf(err), apperr.Message(err), map[string]any{"version": con.Version, "layout": con.Layout})
		return
	}
	httpx.Fail(w, r, err)
}

func writeData(w http.ResponseWriter, r *http.Request, code apperr.Code, msg string, data any) {
	httpx.WriteJSON(w, code.HTTP, dataError{Status: code.Name, ErrorMessage: msg, RequestID: httpx.RequestIDFrom(r.Context()), Data: data})
}
