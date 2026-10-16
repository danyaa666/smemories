package notes

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Limits of the moderation endpoints (api/openapi.yaml, tag note).
const (
	DefaultNoteListLimit = 50
	MaxNoteListLimit     = 100
	maxBatchIDs          = 100 // ids in a batch-update-status or batch-delete
	maxReorderIDs        = maxNotes
	moderationPerMinute  = 120 // moderation writes per user
	publicIDLen          = 26  // a ULID
)

// Business codes of the moderation endpoints.
var (
	CodeOrderStale  = apperr.NewCode("ERROR_ORDER_STALE", 409)
	CodeTooManyIDs  = apperr.NewCode("ERROR_TOO_MANY_IDS", 400)
	errNotesStatus  = apperr.New(apperr.Param, "status must be pending, approved or hidden")
	errOrderStale   = apperr.New(CodeOrderStale, "the approved notes changed, reload the list")
	errBadPageToken = apperr.New(apperr.Param, "next_id is not a valid page token")
)

// Tokens makes and reads the opaque next_id tokens; *httpx.Cursors implements it.
type Tokens interface {
	Seal(scope, pos string) string
	Open(scope, token string) (pos string, err error)
}

// PhotoRef names a photo of a note; the bytes come from media/get-content.
type PhotoRef struct {
	ID string `json:"id"`
}

// NoteItem is one note as the owner's list shows it. Answers are stored as they were validated; the display name is answers.name.
type NoteItem struct {
	ID        string          `json:"id"`
	Status    string          `json:"status"`
	Answers   json.RawMessage `json:"answers"`
	Photos    []PhotoRef      `json:"photos"`
	LinkLabel string          `json:"link_label"`
	CreatedAt int64           `json:"created_at"` // Unix ms
	Position  *int            `json:"position"`   // 1-based place in the book, approved notes only

	rowID uint64 // internal id: the keyset position, never leaves the package
	coll  uint64 // internal collection id, for link_label
}

// Counts are the notes of a whole yearbook by status.
type Counts struct {
	All      int `json:"all"`
	Pending  int `json:"pending"`
	Approved int `json:"approved"`
	Hidden   int `json:"hidden"`
}

// NoteList is one page of notes plus the counts of the whole book.
type NoteList struct {
	Items  []*NoteItem `json:"items"`
	NextID string      `json:"next_id"`
	Counts Counts      `json:"counts"`
}

// ListInput is a GET /api/note/get-list request. Status "" means all.
type ListInput struct {
	YearbookID string
	Status     string
	Limit      int
	NextID     string
}

// notePos is a keyset position: the sort_order for the approved list, else (created_at, id) newest first.
type notePos struct {
	sortOrder int
	createdAt int64
	rowID     uint64
}

// modStore is what the moderation methods need from the adapter. Every method is scoped by the book's owner.
type modStore interface {
	listNotes(ctx context.Context, ownerID uint64, bookID, status string, after *notePos, limit int) (*NoteList, *notePos, error)
	setStatus(ctx context.Context, ownerID uint64, bookID string, ids []string, status string, now int64) (*Counts, error)
	reorder(ctx context.Context, ownerID uint64, bookID string, ids []string, now int64) error
	photoRows(ctx context.Context, ownerID uint64, bookID string, ids []string) ([]int64, error)
	approvedNotes(ctx context.Context, ownerID uint64, bookID string) ([]*BookNote, error)
	removeNotes(ctx context.Context, ownerID uint64, bookID string, ids []string, deleteRows func(ctx context.Context, tx *sql.Tx, rowIDs []int64) error) (*Counts, error)
}

// notePhotos is the part of the media service that deleting notes uses.
type notePhotos interface {
	DeleteObjects(ctx context.Context, rowIDs []int64) error
	DeleteRows(ctx context.Context, tx *sql.Tx, rowIDs []int64) error
}

// validStatus reports whether s is one of the named statuses.
func validStatus(s string) bool {
	return s == StatusPending || s == StatusApproved || s == StatusHidden
}

func checkOwner(verified bool, bookID string) error {
	if !verified {
		return apperr.New(CodeEmailNotVerified, "verify your email first")
	}
	if bookID == "" {
		return apperr.New(apperr.Param, "yearbook_id is required")
	}
	if !isPublicID(bookID) {
		return apperr.New(apperr.Param, "yearbook_id must be a yearbook id")
	}
	return nil
}

const crockford = "0123456789ABCDEFGHJKMNPQRSTVWXYZ"

// isPublicID reports whether s is a canonical ULID: 26 upper-case Crockford base32 characters. The public_id columns
// compare case-insensitively, so anything else would match a row under another spelling of its id.
func isPublicID(s string) bool {
	if len(s) != publicIDLen {
		return false
	}
	for i := 0; i < len(s); i++ {
		if strings.IndexByte(crockford, s[i]) < 0 {
			return false
		}
	}
	return true
}

// checkIDs validates the ids of a batch: at most max, each a public id, no duplicates; empty only when allowEmpty.
func checkIDs(ids []string, limit int, allowEmpty bool) error {
	if len(ids) == 0 && !allowEmpty {
		return apperr.New(apperr.Param, "ids is required")
	}
	if len(ids) > limit {
		return apperr.New(CodeTooManyIDs, "at most "+strconv.Itoa(limit)+" ids")
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if !isPublicID(id) {
			return apperr.New(apperr.Param, "ids must be note ids")
		}
		if _, dup := seen[id]; dup {
			return apperr.New(apperr.Param, "ids must not repeat")
		}
		seen[id] = struct{}{}
	}
	return nil
}

// takeModeration counts a moderation write against the user's limit.
func (s *Service) takeModeration(ctx context.Context, ownerID uint64) error {
	if ok, wait, _ := s.moderation.Take(ctx, strconv.FormatUint(ownerID, 10)); !ok {
		return tooMany(wait)
	}
	return nil
}

func listScope(bookID, status string) string { return "note/get-list:" + bookID + ":" + status }

func sealPos(t Tokens, scope string, status string, p *notePos) string {
	if p == nil {
		return ""
	}
	if status == StatusApproved {
		return t.Seal(scope, "o:"+strconv.Itoa(p.sortOrder))
	}
	return t.Seal(scope, "c:"+strconv.FormatInt(p.createdAt, 10)+":"+strconv.FormatUint(p.rowID, 10))
}

func openPos(t Tokens, scope, status, token string) (*notePos, error) {
	raw, err := t.Open(scope, token)
	if err != nil {
		return nil, err
	}
	kind, rest, _ := strings.Cut(raw, ":")
	if status == StatusApproved && kind == "o" {
		if n, err := strconv.Atoi(rest); err == nil && n >= 0 {
			return &notePos{sortOrder: n}, nil
		}
	}
	if status != StatusApproved && kind == "c" {
		at, id, ok := strings.Cut(rest, ":")
		t1, e1 := strconv.ParseInt(at, 10, 64)
		t2, e2 := strconv.ParseUint(id, 10, 64)
		if ok && e1 == nil && e2 == nil {
			return &notePos{createdAt: t1, rowID: t2}, nil
		}
	}
	return nil, errBadPageToken
}

// ListNotes returns one page of the book's notes (approved ones in book order, the rest newest first) and the counts.
func (s *Service) ListNotes(ctx context.Context, ownerID uint64, verified bool, in *ListInput) (*NoteList, error) {
	if err := checkOwner(verified, in.YearbookID); err != nil {
		return nil, err
	}
	if in.Status != "" && !validStatus(in.Status) {
		return nil, errNotesStatus
	}
	var after *notePos
	scope := listScope(in.YearbookID, in.Status)
	if in.NextID != "" {
		var err error
		if after, err = openPos(s.tokens, scope, in.Status, in.NextID); err != nil {
			return nil, err
		}
	}
	list, next, err := s.mod.listNotes(ctx, ownerID, in.YearbookID, in.Status, after, in.Limit)
	if err != nil {
		return nil, err
	}
	list.NextID = sealPos(s.tokens, scope, in.Status, next)
	return list, nil
}

// UpdateStatus sets the status of up to 100 notes of a book, all or nothing (the store documents the lock order).
func (s *Service) UpdateStatus(ctx context.Context, ownerID uint64, verified bool, bookID string, ids []string, status string) (*Counts, error) {
	if err := checkOwner(verified, bookID); err != nil {
		return nil, err
	}
	if !validStatus(status) {
		return nil, errNotesStatus
	}
	if err := checkIDs(ids, maxBatchIDs, false); err != nil {
		return nil, err
	}
	if err := s.takeModeration(ctx, ownerID); err != nil {
		return nil, err
	}
	return s.mod.setStatus(ctx, ownerID, bookID, ids, status, s.now().UnixMilli())
}

// ReorderNotes puts the approved notes in the order of ids, which must be exactly the approved set (else ERROR_ORDER_STALE).
func (s *Service) ReorderNotes(ctx context.Context, ownerID uint64, verified bool, bookID string, ids []string) error {
	if err := checkOwner(verified, bookID); err != nil {
		return err
	}
	if err := checkIDs(ids, maxReorderIDs, true); err != nil {
		return err
	}
	if err := s.takeModeration(ctx, ownerID); err != nil {
		return err
	}
	return s.mod.reorder(ctx, ownerID, bookID, ids, s.now().UnixMilli())
}

// DeleteNotes deletes up to 100 notes with their photos. The photo objects go first: if the object store fails nothing else
// changed and the caller retries (like the yearbook delete); then one transaction removes the rows. No undo.
func (s *Service) DeleteNotes(ctx context.Context, ownerID uint64, verified bool, bookID string, ids []string) (*Counts, error) {
	if err := checkOwner(verified, bookID); err != nil {
		return nil, err
	}
	if err := checkIDs(ids, maxBatchIDs, false); err != nil {
		return nil, err
	}
	if err := s.takeModeration(ctx, ownerID); err != nil {
		return nil, err
	}
	rows, err := s.mod.photoRows(ctx, ownerID, bookID, ids) // ErrNotFound for any id that is not in the owner's book, before anything is removed
	if err != nil {
		return nil, err
	}
	if err := s.purge.DeleteObjects(ctx, rows); err != nil {
		return nil, mediaError(err)
	}
	return s.mod.removeNotes(ctx, ownerID, bookID, ids, s.purge.DeleteRows)
}
