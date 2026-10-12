package notes

import (
	"context"
	"database/sql"
)

// Service holds what used to be foreign keys of the notes tables (docs/db-conventions.md, "Deleting"): the yearbook service calls
// it to delete a book's collections, notes and photo links (yearbook.ChildPurger), and the media service calls it before a photo
// row is deleted (media.RefClearer).
type Service struct{ store *Store }

func NewService(store *Store) *Service { return &Service{store: store} }

// DeleteByYearbook deletes the collections, notes and photo links of a yearbook in the caller's transaction, which holds the
// yearbook row lock (yearbook.Service.Delete). Collections and notes go together; revoking a collection keeps its notes.
func (s *Service) DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	return deleteByYearbook(ctx, tx, yearbookID)
}

// ClearMediaRefs deletes the note links of a photo, in the caller's transaction, just before the photo row is deleted. The media
// service calls it after the yearbook's own ClearMediaRefs, so the yearbook row is already locked.
func (s *Service) ClearMediaRefs(ctx context.Context, tx *sql.Tx, mediaID int64) error {
	return deleteMediaLinks(ctx, tx, mediaID)
}
