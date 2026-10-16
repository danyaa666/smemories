package notes

import (
	"net/http"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

type moderationInput struct {
	YearbookID string   `json:"yearbook_id"`
	IDs        []string `json:"ids"`
	Status     string   `json:"status"`
}

type countsOut struct {
	Counts *Counts `json:"counts"`
}

func (h *Handler) listNotes(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	limit, err := httpx.ParseLimit(r, DefaultNoteListLimit, MaxNoteListLimit)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	q := r.URL.Query()
	list, err := h.svc.ListNotes(r.Context(), u.InternalID, u.EmailVerified,
		&ListInput{YearbookID: q.Get("yearbook_id"), Status: q.Get("status"), Limit: limit, NextID: q.Get("next_id")})
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, list)
}

func (h *Handler) updateStatus(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in moderationInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.UpdateStatus(r.Context(), u.InternalID, u.EmailVerified, in.YearbookID, in.IDs, in.Status)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, &countsOut{c})
}

func (h *Handler) reorder(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in moderationInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	if err := h.svc.ReorderNotes(r.Context(), u.InternalID, u.EmailVerified, in.YearbookID, in.IDs); err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, nil)
}

func (h *Handler) deleteNotes(w http.ResponseWriter, r *http.Request) {
	u, _ := auth.UserFrom(r.Context())
	var in moderationInput
	if !httpx.DecodeJSON(w, r, &in) {
		return
	}
	c, err := h.svc.DeleteNotes(r.Context(), u.InternalID, u.EmailVerified, in.YearbookID, in.IDs)
	if err != nil {
		h.fail(w, r, err)
		return
	}
	httpx.OK(w, r, &countsOut{c})
}
