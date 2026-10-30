package bookdesign

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
	writesPerWindow = 60
	writeWindow     = 10 * time.Minute
)

// RetryAfterError is the 429: the handler turns After into the Retry-After header.
type RetryAfterError struct {
	Err   error
	After time.Duration
}

func (e *RetryAfterError) Error() string { return e.Err.Error() }
func (e *RetryAfterError) Unwrap() error { return e.Err }

// View is the answer of the three endpoints: the stored document and the choices the screen offers.
type View struct {
	TemplateID *string                `json:"template_id"` // the book's template; null when it has none
	Options    json.RawMessage        `json:"options"`     // the stored document; null when the book uses the defaults
	UpdatedAt  *int64                 `json:"updated_at"`  // Unix ms; null with options
	Design     *templates.DesignBlock `json:"design"`      // the template's design block; null when it has none
}

// Service holds the rules: owner scoping, validation against the book's template and the write rate limit.
type Service struct {
	store *Store
	limit ratelimit.Limiter // nil = unlimited (tests, no Redis)
	now   func() time.Time
}

// NewService builds the service; limiters and now may be nil (no rate limit, time.Now).
func NewService(store *Store, limiters *ratelimit.Factory, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	s := &Service{store: store, now: now}
	if limiters != nil {
		s.limit = limiters.Open("book_design", writesPerWindow, writeWindow) // fails open: it bounds effort, not guessing
	}
	return s
}

// Get returns the owner's design of the book. A row made for another template than the book's current one counts as no row.
func (s *Service) Get(ctx context.Context, ownerID uint64, publicID string) (*View, error) {
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	return view(st), nil
}

// Update replaces the design after validating it against the book's current template. A book whose template has no design block
// is ERROR_INVALID_DESIGN. Another user's and an unknown book are ERROR_NOT_FOUND.
func (s *Service) Update(ctx context.Context, ownerID uint64, publicID string, options []byte) (*View, error) {
	if err := s.take(ctx, ownerID); err != nil {
		return nil, err
	}
	st, err := s.store.put(ctx, ownerID, publicID, s.now().UnixMilli(), func(tpl *string) ([]byte, error) {
		d, ok := designOf(tpl)
		if !ok {
			return nil, invalid("the book's template cannot be customised")
		}
		return Validate(d, options)
	})
	if err != nil {
		return nil, err
	}
	return view(st), nil
}

// Reset deletes the design (back to the template's defaults); it succeeds when there is none.
func (s *Service) Reset(ctx context.Context, ownerID uint64, publicID string) error {
	if err := s.take(ctx, ownerID); err != nil {
		return err
	}
	return s.store.reset(ctx, ownerID, publicID)
}

// Resolved is the design to print for the owner's book (book-data/get): nil when the template has no design block. templateID is
// the book's template as the caller read it.
func (s *Service) Resolved(ctx context.Context, ownerID uint64, publicID string, templateID *string) (*Resolved, error) {
	d, ok := designOf(templateID)
	if !ok {
		return nil, nil
	}
	st, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	var stored []byte
	if st.row != nil && templateID != nil && st.row.templateID == *templateID {
		stored = st.row.options
	}
	return Resolve(d, stored), nil
}

// DeleteByYearbook removes the book's design row in the caller's transaction. It is the yearbook.ChildPurger of a book delete and
// the hook of a template change (a design is only valid for the template it was made for).
func (s *Service) DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	return deleteRow(ctx, tx, yearbookID)
}

func (s *Service) take(ctx context.Context, ownerID uint64) error {
	if s.limit == nil {
		return nil
	}
	if ok, wait, _ := s.limit.Take(ctx, strconv.FormatUint(ownerID, 10)); !ok {
		return &RetryAfterError{Err: apperr.New(apperr.RateLimited, "too many requests, try again later"), After: wait}
	}
	return nil
}

func designOf(templateID *string) (*templates.DesignBlock, bool) {
	if templateID == nil {
		return nil, false
	}
	return templates.Design(*templateID)
}

func view(st *state) *View {
	v := &View{TemplateID: st.templateID}
	if d, ok := designOf(st.templateID); ok {
		v.Design = d
	}
	if st.row != nil && st.templateID != nil && st.row.templateID == *st.templateID {
		v.Options = st.row.options
		v.UpdatedAt = &st.row.updatedAt
	}
	return v
}
