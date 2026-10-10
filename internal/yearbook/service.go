package yearbook

import (
	"context"
	"database/sql"
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

// StorageError wraps a failure of the object store while the files of a book were removed (502 storage_error).
type StorageError struct{ Err error }

func (e StorageError) Error() string { return "yearbook: storage: " + e.Err.Error() }
func (e StorageError) Unwrap() error { return e.Err }

// Service holds the rules that used to be foreign keys. T-070 moves the other business rules here.
type Service struct {
	store    *Store
	purger   Purger // may be nil (no file storage configured, e.g. in tests)
	children []ChildPurger
}

// NewService builds the service; purger may be nil, children are called in order when a book is deleted.
func NewService(store *Store, purger Purger, children ...ChildPurger) *Service {
	return &Service{store: store, purger: purger, children: children}
}

// Delete removes the owner's book with everything that belongs to it. The stored files go first (not transactional):
// if the object store fails nothing else is deleted and the caller can retry. Then one transaction deletes the
// profiles, calls every child purger and deletes the book; any failure rolls all of it back.
//
// Lock order (docs/db-conventions.md, Writes): yearbook_tab row first, then profile_tab, then the other children (media,
// notes). Store.modify and Store.ClearMediaRefs take the same order; taking the book row last deadlocked (MySQL 1213)
// with concurrent edits. Keep it when adding a child.
func (s *Service) Delete(ctx context.Context, ownerID uint64, publicID string) error {
	y, err := s.store.get(ctx, ownerID, publicID)
	if err != nil {
		return err
	}
	if s.purger != nil {
		if err := s.purger.PurgeYearbook(ctx, publicID); err != nil {
			return StorageError{err}
		}
	}
	tx, err := s.store.begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockBook(ctx, tx, ownerID, publicID); err != nil {
		return err
	}
	if err := deleteProfiles(ctx, tx, y.internalID); err != nil {
		return err
	}
	for _, c := range s.children {
		if err := c.DeleteByYearbook(ctx, tx, y.internalID); err != nil {
			return err
		}
	}
	if err := deleteBook(ctx, tx, ownerID, y.internalID); err != nil {
		return err
	}
	return tx.Commit()
}
