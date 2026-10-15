package yearbook

import (
	"context"
	"database/sql"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	// DefaultListLimit and MaxListLimit are the page sizes of yearbook/get-list.
	DefaultListLimit = 20
	MaxListLimit     = 50

	defaultPageSize = "A5"
)

// Purger removes a yearbook's stored files; the media package implements it (T-009).
type Purger interface {
	PurgeYearbook(ctx context.Context, publicID string) error
}

// ChildPurger deletes the rows a domain keeps for a yearbook, inside the delete transaction. The tables have no
// foreign keys, so every domain that stores rows per yearbook registers one (docs/db-conventions.md, "Deleting").
type ChildPurger interface {
	DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error
}

// bookStore is what the Service needs from the store; a fake stands in for it in the unit tests.
type bookStore interface {
	create(ctx context.Context, ownerID uint64, y Yearbook) error
	get(ctx context.Context, ownerID uint64, publicID string) (Yearbook, error)
	list(ctx context.Context, ownerID uint64, c *cursor, limit int) ([]Yearbook, *cursor, error)
	modify(ctx context.Context, ownerID uint64, publicID string, nowMs int64, change func(*Yearbook) error) (Yearbook, error)
	remove(ctx context.Context, ownerID uint64, y *Yearbook, children []ChildPurger) error
}

// Service holds every rule of the yearbook domain: defaults, the 20-book limit, field validation, ownership scoping, the
// media checks and the order of a delete. The handler only decodes and maps; the store only runs SQL.
type Service struct {
	store    bookStore
	tokens   Tokens
	purger   Purger // may be nil (no file storage configured, e.g. in tests)
	now      func() time.Time
	children []ChildPurger
}

// NewService builds the service; purger may be nil, now may be nil (time.Now), children are called in order when a book
// is deleted.
func NewService(store *Store, tokens Tokens, purger Purger, now func() time.Time, children ...ChildPurger) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, tokens: tokens, purger: purger, now: now, children: children}
}

// Create makes a book and its owner profile (named after the user) for ownerID. Title and language are required; a new
// book has no media yet, so a cover is refused. A user owns at most maxBooksOwned books (ErrLimitReached).
func (s *Service) Create(ctx context.Context, ownerID uint64, displayName string, in *BookInput) (*Yearbook, error) {
	now := s.now().UTC()
	y := Yearbook{ID: ulid.New(now), PageSize: defaultPageSize, CreatedAt: now.UnixMilli(), UpdatedAt: now.UnixMilli(),
		Profile: Profile{ID: ulid.New(now), IsOwner: true, FullName: displayName}}
	if err := in.applyTo(&y); err != nil {
		return nil, err
	}
	switch {
	case in.Title == nil:
		return nil, invalid("title")
	case in.Language == nil:
		return nil, invalid("language")
	case y.CoverMediaID != nil:
		return nil, invalid("media")
	}
	if err := s.store.create(ctx, ownerID, y); err != nil {
		return nil, err
	}
	return &y, nil
}

// Get returns the owner's book; a missing book and someone else's are both ErrNotFound.
func (s *Service) Get(ctx context.Context, ownerID uint64, publicID string) (*Yearbook, error) {
	y, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	return &y, nil
}

// List returns one page of the owner's books, newest updated_at first, and the token of the next page ("" on the last).
func (s *Service) List(ctx context.Context, ownerID uint64, limit int, nextID string) ([]Yearbook, string, error) {
	var after *cursor
	if nextID != "" {
		var err error
		if after, err = openCursor(s.tokens, ownerID, nextID); err != nil {
			return nil, "", err
		}
	}
	books, next, err := s.store.list(ctx, ownerID, after, limit)
	if err != nil {
		return nil, "", err
	}
	if books == nil {
		books = []Yearbook{}
	}
	return books, sealCursor(s.tokens, ownerID, next), nil
}

// Update changes the fields present in in on the owner's book and bumps updated_at.
func (s *Service) Update(ctx context.Context, ownerID uint64, publicID string, in *BookInput) (*Yearbook, error) {
	y, err := s.store.modify(ctx, ownerID, publicID, s.now().UnixMilli(), in.applyTo)
	if err != nil {
		return nil, err
	}
	return &y, nil
}

// UpdateProfile replaces the owner's profile information (an absent optional field is cleared) and bumps updated_at.
func (s *Service) UpdateProfile(ctx context.Context, ownerID uint64, publicID string, in *ProfileInput) (*Yearbook, error) {
	now := s.now()
	y, err := s.store.modify(ctx, ownerID, publicID, now.UnixMilli(), func(y *Yearbook) error {
		return in.applyTo(&y.Profile, now)
	})
	if err != nil {
		return nil, err
	}
	return &y, nil
}

// Delete removes the owner's book with everything that belongs to it. The stored files go first (not transactional):
// if the object store fails nothing else is deleted and the caller can retry (CodeStorage). Then Store.remove deletes the
// rows in one transaction.
func (s *Service) Delete(ctx context.Context, ownerID uint64, publicID string) error {
	y, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return err
	}
	if s.purger != nil {
		if err := s.purger.PurgeYearbook(ctx, publicID); err != nil {
			return apperr.Wrap(CodeStorage, err, "object storage failed")
		}
	}
	return s.store.remove(ctx, ownerID, &y, s.children)
}
