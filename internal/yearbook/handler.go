// Package yearbook is the user's yearbooks and the owner's profile page information (T-008, v2 in T-070).
// Every endpoint needs a session and only ever sees the caller's own books.
package yearbook

import (
	"context"
	"errors"
	"net/http"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

// codeClientClosed is nginx's 499: the client went away, nobody reads the answer.
var codeClientClosed = apperr.NewCode("ERROR_CLIENT_CLOSED", 499)

// Handler serves /api/yearbook/*: it decodes, checks the shape, calls the Service and writes the answer.
type Handler struct {
	svc            *Service
	requireUser    func(http.Handler) http.Handler // auth.Handler.RequireUser
	allowedOrigins []string                        // for auth.Guard
}

func NewHandler(svc *Service, requireUser func(http.Handler) http.Handler, allowedOrigins []string) *Handler {
	return &Handler{svc: svc, requireUser: requireUser, allowedOrigins: allowedOrigins}
}

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	read := func(f http.HandlerFunc) http.Handler { return h.requireUser(f) }
	write := func(f http.HandlerFunc) http.Handler { return auth.Guard(h.allowedOrigins, h.requireUser(f)) }
	mux.Handle("POST /api/yearbook/create", write(h.create))
	mux.Handle("GET /api/yearbook/get-list", read(h.list))
	mux.Handle("GET /api/yearbook/get", read(h.get))
	mux.Handle("POST /api/yearbook/update", write(h.update))
	mux.Handle("POST /api/yearbook/update-profile", write(h.updateProfile))
	mux.Handle("POST /api/yearbook/delete", write(h.delete))
}

// The request bodies: the id (or yearbook_id) next to the fields the service validates.
type (
	updateInput struct {
		ID string `json:"id"`
		BookInput
	}
	profileUpdateInput struct {
		YearbookID string `json:"yearbook_id"`
		ProfileInput
	}
	deleteInput struct {
		ID string `json:"id"`
	}
)

// caller is the signed-in user; RequireUser has always stored it by now.
func caller(r *http.Request) auth.User {
	u, _ := auth.UserFrom(r.Context())
	return u
}

// required is ERROR_PARAM for a missing id.
func required(w http.ResponseWriter, r *http.Request, id, name string) bool {
	if id == "" {
		fail(w, r, apperr.New(apperr.Param, name+" is required"))
	}
	return id != ""
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var in BookInput
	if !httpx.DecodeJSON(w, r, &in, true) {
		return
	}
	u := caller(r)
	y, err := h.svc.Create(r.Context(), u.InternalID, u.DisplayName, &in)
	respond(w, r, y, err)
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	limit, err := httpx.ParseLimit(r, DefaultListLimit, MaxListLimit)
	if err != nil {
		fail(w, r, err)
		return
	}
	books, next, err := h.svc.List(r.Context(), caller(r).InternalID, limit, r.URL.Query().Get("next_id"))
	respond(w, r, httpx.Page[Yearbook]{Items: books, NextID: next}, err)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if required(w, r, id, "id") {
		y, err := h.svc.Get(r.Context(), caller(r).InternalID, id)
		respond(w, r, y, err)
	}
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	var in updateInput
	if httpx.DecodeJSON(w, r, &in, true) && required(w, r, in.ID, "id") {
		y, err := h.svc.Update(r.Context(), caller(r).InternalID, in.ID, &in.BookInput)
		respond(w, r, y, err)
	}
}

func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	var in profileUpdateInput
	if httpx.DecodeJSON(w, r, &in, true) && required(w, r, in.YearbookID, "yearbook_id") {
		y, err := h.svc.UpdateProfile(r.Context(), caller(r).InternalID, in.YearbookID, &in.ProfileInput)
		respond(w, r, y, err)
	}
}

// delete removes the book's stored files first: if the object store fails, nothing is deleted and the caller can retry.
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	var in deleteInput
	if httpx.DecodeJSON(w, r, &in, true) && required(w, r, in.ID, "id") {
		respond(w, r, nil, h.svc.Delete(r.Context(), caller(r).InternalID, in.ID))
	}
}

// respond answers data, or err when there is one.
func respond(w http.ResponseWriter, r *http.Request, data any, err error) {
	if err != nil {
		fail(w, r, err)
		return
	}
	httpx.OK(w, r, data)
}

// fail answers err. A request whose client went away gets 499 and a debug line, a storage failure is logged (Fail logs
// only the codes that are our fault), the rest is httpx.Fail.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	log := httpx.LoggerFrom(r.Context())
	switch {
	case errors.Is(err, context.Canceled):
		log.Debug("yearbook: client gone", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
		err = apperr.New(codeClientClosed, "client closed the request")
	case apperr.Is(err, CodeStorage):
		log.Error("yearbook: storage failed", "request_id", httpx.RequestIDFrom(r.Context()), "error", err)
	}
	httpx.Fail(w, r, err)
}
