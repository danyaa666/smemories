package booklayout

import (
	"context"
	"database/sql"
	"encoding/json"
	"strconv"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/templates"
)

const (
	// DefaultSavesPerMinute is the default of SMEM_RATE_LAYOUT_SAVE_PER_MINUTE; Burst saves are allowed in any burstWindow.
	DefaultSavesPerMinute = 60
	Burst                 = 20
	burstWindow           = 10 * time.Second
)

// Codes of the save endpoint.
var (
	// CodeVersionConflict is the 409: base_version is not the stored version. The answer carries the stored document.
	CodeVersionConflict = apperr.NewCode("ERROR_VERSION_CONFLICT", apperr.Conflict.HTTP)
	// CodeInvalidLayout is the 400 for a rejected document. The answer carries the problem list.
	CodeInvalidLayout = apperr.NewCode("ERROR_INVALID_LAYOUT", apperr.Param.HTTP)
)

// Results of a save, the label of smem_layout_saves_total.
const (
	ResultOK          = "ok"
	ResultConflict    = "conflict"
	ResultInvalid     = "invalid"
	ResultTooLarge    = "too_large"
	ResultRateLimited = "rate_limited"
)

// RetryAfterError is the 429: the handler turns After into the Retry-After header.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// InvalidError is a rejected document with its problems (at most MaxProblems).
type InvalidError struct{ Problems []Problem }

func (e *InvalidError) Error() string { return "invalid layout" }

// ConflictError is a save with a stale base_version; Layout and Version are the stored ones (null and 0 when the row is gone).
type ConflictError struct {
	Version int64
	Layout  json.RawMessage
}

func (e *ConflictError) Error() string { return "layout version conflict" }

// DesignImages is the part of the media service the layout needs: which of the ids are design images of the owner's book.
type DesignImages interface {
	DesignImages(ctx context.Context, ownerID uint64, yearbookID string, ids []string) (map[string]bool, error)
}

// View is the answer of get.
type View struct {
	Layout     json.RawMessage `json:"layout"`      // the stored document; null when there is none
	Version    int64           `json:"version"`     // 0 when there is none
	TemplateID *string         `json:"template_id"` // the book's template; null when it has none
	UpdatedAt  *int64          `json:"updated_at"`  // Unix ms; null without a layout
}

// Saved is the answer of save.
type Saved struct {
	Version   int64 `json:"version"`
	UpdatedAt int64 `json:"updated_at"`
}

// Resolved is what the print route gets: the stored document re-validated (parts that no longer validate left out) and its version.
type Resolved struct {
	Layout  json.RawMessage
	Version int64
}

// Service holds the rules: owner scoping, validation, optimistic concurrency and the save rate limit.
type Service struct {
	store  *Store
	media  DesignImages
	minute ratelimit.Limiter // nil = unlimited (tests, no Redis)
	burst  ratelimit.Limiter
	now    func() time.Time
	count  func(result string) // smem_layout_saves_total; nil = not counted
}

// NewService builds the service; limiters and now may be nil (no rate limit, time.Now). perMinute is
// SMEM_RATE_LAYOUT_SAVE_PER_MINUTE.
func NewService(store *Store, media DesignImages, limiters *ratelimit.Factory, perMinute int, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	s := &Service{store: store, media: media, now: now}
	if limiters != nil {
		s.minute = limiters.Open("book_layout", perMinute, time.Minute) // fail open: it bounds effort, not guessing
		s.burst = limiters.Open("book_layout_burst", Burst, burstWindow)
	}
	return s
}

// SetCounter makes the service report every save's result (ResultOK, ...) to f.
func (s *Service) SetCounter(f func(result string)) { s.count = f }

// Counted lets the handler report the one result the service never sees: a body over the size limit.
func (s *Service) Counted(result string) {
	if s.count != nil {
		s.count(result)
	}
}

// Get returns the owner's layout of the book.
func (s *Service) Get(ctx context.Context, ownerID uint64, publicID string) (*View, error) {
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	v := &View{TemplateID: st.templateID}
	if st.row != nil {
		v.Layout, v.Version, v.UpdatedAt = st.row.layout, st.row.version, &st.row.updatedAt
	}
	return v, nil
}

// Save validates layout against the book's current template and stores it if baseVersion is the stored version (0 = none yet).
// Another user's and an unknown book are ERROR_NOT_FOUND, also for an invalid document. A rejected document is an *InvalidError, a stale
// baseVersion a *ConflictError, a template without a layout block an *InvalidError too.
func (s *Service) Save(ctx context.Context, ownerID uint64, publicID string, baseVersion int64, layout []byte) (*Saved, error) {
	res, err := s.save(ctx, ownerID, publicID, baseVersion, layout)
	switch {
	case err == nil:
		s.Counted(ResultOK)
	case apperr.Is(err, apperr.RateLimited):
		s.Counted(ResultRateLimited)
	case apperr.Is(err, CodeVersionConflict):
		s.Counted(ResultConflict)
	case apperr.Is(err, CodeInvalidLayout):
		s.Counted(ResultInvalid)
	}
	return res, err
}

func (s *Service) save(ctx context.Context, ownerID uint64, publicID string, baseVersion int64, layout []byte) (*Saved, error) {
	if baseVersion < 0 {
		return nil, apperr.New(apperr.Param, "base_version must be 0 or the version read")
	}
	if err := s.take(ctx, ownerID); err != nil {
		return nil, err
	}
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	block := layoutOf(st.templateID)
	if block == nil {
		return nil, invalid(Problem{"", "template_not_customisable"})
	}
	own, err := s.ownImages(ctx, ownerID, publicID, layout)
	if err != nil {
		return nil, err
	}
	doc, probs := Validate(layout, block, own)
	if len(probs) > 0 {
		return nil, invalid(probs...)
	}
	now := s.now().UnixMilli()
	version, lost, err := s.store.save(ctx, ownerID, publicID, *st.templateID, baseVersion, doc, now)
	if err != nil {
		return nil, err
	}
	if lost != nil {
		return nil, conflictOf(lost.row)
	}
	return &Saved{Version: version, UpdatedAt: now}, nil
}

// ownImages asks the media service which of the media ids the document names are design images of the owner's book: the one check that
// a media id is not a profile photo, a note photo or another user's image.
func (s *Service) ownImages(ctx context.Context, ownerID uint64, publicID string, doc []byte) (map[string]bool, error) {
	ids := MediaRefs(doc)
	if len(ids) == 0 {
		return nil, nil
	}
	return s.media.DesignImages(ctx, ownerID, publicID, ids)
}

// Reset deletes the layout (back to the template's own pages); it succeeds when there is none.
func (s *Service) Reset(ctx context.Context, ownerID uint64, publicID string) error {
	return s.store.reset(ctx, ownerID, publicID)
}

// Resolved is the layout to print for the owner's book (book-data/get): nil when there is none, the row was made for another template, the
// template has no layout block or nothing of it validates. templateID is the book's template as the caller read it.
func (s *Service) Resolved(ctx context.Context, ownerID uint64, publicID string, templateID *string) (*Resolved, error) {
	block := layoutOf(templateID)
	if block == nil {
		return nil, nil
	}
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	if st.row == nil || st.row.templateID != *templateID {
		return nil, nil
	}
	own, err := s.ownImages(ctx, ownerID, publicID, st.row.layout)
	if err != nil {
		return nil, err
	}
	doc := Sanitize(st.row.layout, block, own)
	if doc == nil {
		return nil, nil
	}
	return &Resolved{Layout: doc, Version: st.row.version}, nil
}

// DeleteByYearbook removes the book's layout row in the caller's transaction. It is the yearbook.ChildPurger of a book delete and the
// hook of a template change (a layout is only valid for the template it was made for).
func (s *Service) DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	return deleteRow(ctx, tx, yearbookID)
}

// take spends one save: Burst per burstWindow and the configured number per minute. Both fail open when Redis is down.
func (s *Service) take(ctx context.Context, ownerID uint64) error {
	if s.minute == nil {
		return nil
	}
	key := strconv.FormatUint(ownerID, 10)
	if ok, wait, _ := s.burst.Take(ctx, key); !ok {
		return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many saves, try again later"), After: wait}
	}
	if ok, wait, _ := s.minute.Take(ctx, key); !ok {
		_ = s.burst.Refund(ctx, key) // a refused save is not a burst hit
		return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many saves, try again later"), After: wait}
	}
	return nil
}

func layoutOf(templateID *string) *templates.LayoutBlock {
	if templateID == nil {
		return nil
	}
	d, ok := templates.Design(*templateID)
	if !ok {
		return nil
	}
	return d.Layout
}

func invalid(p ...Problem) error {
	return apperr.Wrap(CodeInvalidLayout, &InvalidError{Problems: p}, "layout is not valid")
}

func conflictOf(r *row) error {
	c := &ConflictError{}
	if r != nil {
		c.Version, c.Layout = r.version, r.layout
	}
	return apperr.Wrap(CodeVersionConflict, c, "the layout was saved elsewhere")
}
