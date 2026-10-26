package class

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

// Handler serves /api/class/* and /api/class-member/*: it decodes, checks the shape, calls the Service and writes the answer.
// Every endpoint needs a session; who may do what is decided in the Service (Authorize).
type Handler struct {
	svc            *Service
	requireUser    func(http.Handler) http.Handler // auth.Handler.RequireUser
	allowedOrigins []string                        // for auth.Guard
	clientIP       func(*http.Request) string      // auth.Handler.ClientIP, for the invite limits
}

func NewHandler(svc *Service, requireUser func(http.Handler) http.Handler, allowedOrigins []string, clientIP func(*http.Request) string) *Handler {
	return &Handler{svc: svc, requireUser: requireUser, allowedOrigins: allowedOrigins, clientIP: clientIP}
}

// Routes registers the endpoints on mux (pass to httpx.NewRouter).
func (h *Handler) Routes(mux *http.ServeMux) {
	read := func(f http.HandlerFunc) http.Handler { return h.requireUser(f) }
	write := func(f http.HandlerFunc) http.Handler { return auth.Guard(h.allowedOrigins, h.requireUser(f)) }
	mux.Handle("POST /api/class/create", write(h.create))
	mux.Handle("GET /api/class/get-list", read(h.list))
	mux.Handle("GET /api/class/get", read(h.get))
	mux.Handle("POST /api/class/update", write(h.update))
	mux.Handle("POST /api/class/delete", write(h.delete))
	mux.Handle("GET /api/class-member/get-list", read(h.members))
	mux.Handle("POST /api/class-member/update-role", write(h.updateRole))
	mux.Handle("POST /api/class-member/remove", write(h.removeMember))
	mux.Handle("POST /api/class-member/leave", write(h.leave))
	mux.Handle("POST /api/class-invite/create", write(h.createInvite))
	mux.Handle("GET /api/class-invite/get-list", read(h.invites))
	mux.Handle("POST /api/class-invite/revoke", write(h.revokeInvite))
	mux.Handle("POST /api/class-invite/accept", write(h.acceptInvite))
	// Public, a POST only so the token travels in the body and never in a URL (docs/api-contract.md, section 6).
	mux.Handle("POST /api/class-invite/lookup", auth.Guard(h.allowedOrigins, http.HandlerFunc(h.lookupInvite)))
}

// The request bodies: the ids next to the fields the service validates.
type (
	updateInput struct {
		ID string `json:"id"`
		ClassInput
	}
	idInput struct {
		ID string `json:"id"`
	}
	classIDInput struct {
		ClassID string `json:"class_id"`
	}
	memberInput struct {
		ClassID  string `json:"class_id"`
		MemberID string `json:"member_id"`
	}
	roleInput struct {
		memberInput
		Role string `json:"role"`
	}
	createInviteInput struct {
		ClassID   string `json:"class_id"`
		ExpiresAt *int64 `json:"expires_at"`
	}
	revokeInviteInput struct {
		ClassID string `json:"class_id"`
		ID      string `json:"id"`
	}
	tokenInput struct {
		Token string `json:"token"`
	}
)

// validID is ERROR_PARAM, answered before any query, for a missing id and for one that is not a canonical public id
// (ulid.Valid): the public_id columns match any spelling of an id.
func validID(w http.ResponseWriter, r *http.Request, id, name string) bool {
	switch {
	case id == "":
		fail(w, r, apperr.New(apperr.Param, name+" is required"))
	case !ulid.Valid(id):
		fail(w, r, apperr.New(apperr.Param, name+" is not a valid id"))
	default:
		return true
	}
	return false
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in ClassInput
	if httpx.DecodeJSON(w, r, &in) {
		c, err := h.svc.Create(r.Context(), u.InternalID, u.EmailVerified, &in)
		respond(w, r, c, err)
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	limit, err := httpx.ParseLimit(r, DefaultListLimit, MaxListLimit)
	if err != nil {
		fail(w, r, err)
		return
	}
	items, next, err := h.svc.List(r.Context(), u.InternalID, limit, r.URL.Query().Get("next_id"))
	respond(w, r, httpx.Page[Class]{Items: items, NextID: next}, err)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	if id := r.URL.Query().Get("id"); validID(w, r, id, "id") {
		c, err := h.svc.Get(r.Context(), u.InternalID, id)
		respond(w, r, c, err)
	}
}

func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in updateInput
	if httpx.DecodeJSON(w, r, &in) && validID(w, r, in.ID, "id") {
		c, err := h.svc.Update(r.Context(), u.InternalID, in.ID, &in.ClassInput)
		respond(w, r, c, err)
	}
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in idInput
	if httpx.DecodeJSON(w, r, &in) && validID(w, r, in.ID, "id") {
		respond(w, r, nil, h.svc.Delete(r.Context(), u.InternalID, in.ID))
	}
}

func (h *Handler) members(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("class_id")
	if !validID(w, r, id, "class_id") {
		return
	}
	limit, err := httpx.ParseLimit(r, MaxMembers, MaxMembers)
	if err != nil {
		fail(w, r, err)
		return
	}
	items, next, err := h.svc.Members(r.Context(), u.InternalID, id, limit, r.URL.Query().Get("next_id"))
	respond(w, r, httpx.Page[Member]{Items: items, NextID: next}, err)
}

func (h *Handler) updateRole(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in roleInput
	if httpx.DecodeJSON(w, r, &in) && validID(w, r, in.ClassID, "class_id") && validID(w, r, in.MemberID, "member_id") {
		respond(w, r, nil, h.svc.UpdateRole(r.Context(), u.InternalID, in.ClassID, in.MemberID, in.Role))
	}
}

func (h *Handler) removeMember(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in memberInput
	if httpx.DecodeJSON(w, r, &in) && validID(w, r, in.ClassID, "class_id") && validID(w, r, in.MemberID, "member_id") {
		respond(w, r, nil, h.svc.RemoveMember(r.Context(), u.InternalID, in.ClassID, in.MemberID))
	}
}

func (h *Handler) leave(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in classIDInput
	if httpx.DecodeJSON(w, r, &in) && validID(w, r, in.ClassID, "class_id") {
		respond(w, r, nil, h.svc.Leave(r.Context(), u.InternalID, in.ClassID))
	}
}

func (h *Handler) createInvite(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in createInviteInput
	if httpx.DecodeJSON(w, r, &in) && validID(w, r, in.ClassID, "class_id") {
		inv, err := h.svc.CreateInvite(r.Context(), u.InternalID, in.ClassID, in.ExpiresAt)
		respond(w, r, inv, err)
	}
}

func (h *Handler) invites(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	id := r.URL.Query().Get("class_id")
	if !validID(w, r, id, "class_id") {
		return
	}
	limit, err := httpx.ParseLimit(r, DefaultInviteListLimit, MaxInviteListLimit)
	if err != nil {
		fail(w, r, err)
		return
	}
	items, next, err := h.svc.Invites(r.Context(), u.InternalID, id, limit, r.URL.Query().Get("next_id"))
	respond(w, r, httpx.Page[Invite]{Items: items, NextID: next}, err)
}

func (h *Handler) revokeInvite(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in revokeInviteInput
	if httpx.DecodeJSON(w, r, &in) && validID(w, r, in.ClassID, "class_id") && validID(w, r, in.ID, "id") {
		respond(w, r, nil, h.svc.RevokeInvite(r.Context(), u.InternalID, in.ClassID, in.ID))
	}
}

// lookupInvite and acceptInvite take the token from the body. A token is never validated here beyond being a string: a wrong
// one, a malformed one and a missing one all get the one ErrInviteNotFound answer from the service.
func (h *Handler) lookupInvite(w http.ResponseWriter, r *http.Request) {
	var in tokenInput
	if httpx.DecodeJSON(w, r, &in) {
		p, err := h.svc.LookupInvite(r.Context(), h.clientIP(r), in.Token)
		respond(w, r, p, err)
	}
}

func (h *Handler) acceptInvite(w http.ResponseWriter, r *http.Request) {
	u, ok := auth.MustUser(w, r)
	if !ok {
		return
	}
	var in tokenInput
	if httpx.DecodeJSON(w, r, &in) {
		c, err := h.svc.AcceptInvite(r.Context(), u.InternalID, h.clientIP(r), u.EmailVerified, in.Token)
		respond(w, r, c, err)
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

// fail answers err; a rate-limited one carries Retry-After.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	var ra *RetryAfterError
	if errors.As(err, &ra) {
		w.Header().Set("Retry-After", strconv.Itoa(max(int((ra.After+time.Second-1)/time.Second), 1)))
	}
	httpx.Fail(w, r, err)
}
