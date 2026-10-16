package notes

import (
	"context"
	"database/sql"
	"errors"
)

const (
	maxActive = 5   // not-revoked collections per yearbook
	maxNotes  = 300 // notes per collection, in any status
)

// Note statuses (note_tab.status is a VARCHAR; docs/db-conventions.md). A new note is pending.
const (
	StatusPending  = "pending"
	StatusApproved = "approved"
	StatusHidden   = "hidden"
)

// Collection is one link as the owner sees it in a list. The token is never part of it. Instants are Unix milliseconds.
type Collection struct {
	ID         string
	Label      string
	DeadlineAt *int64
	CreatedAt  int64
	RevokedAt  *int64
	NoteCount  int // notes in any status
}

// Store persists collections. Every owner query is scoped by yearbook_tab.owner_id: a row is never fetched by id and compared afterwards.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// create locks the owner's book row so two concurrent creates cannot both slip under maxActive.
//
// Lock order (docs/db-conventions.md, Writes): yearbook_tab row first, then note_collection_tab, then note_tab and note_photo_tab.
// insertNote, revoke and the yearbook delete take the same order.
func (s *Store) create(ctx context.Context, ownerID uint64, bookID string, c *Collection, hash [32]byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var id uint64
	err = tx.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, bookID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM note_collection_tab WHERE yearbook_id = ? AND revoked_at IS NULL`, id).Scan(&n); err != nil {
		return err
	}
	if n >= maxActive {
		return ErrLimitReached
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, deadline_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		c.ID, id, hash[:], c.Label, c.DeadlineAt, c.CreatedAt, c.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// list returns the book's collections, newest first. ponytail: unpaged; revoked links stay in the list, add a cursor if that gets long.
func (s *Store) list(ctx context.Context, ownerID uint64, bookID string) ([]*Collection, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE owner_id = ? AND public_id = ?`, ownerID, bookID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.public_id, c.label, c.deadline_at, c.created_at, c.revoked_at, (SELECT COUNT(*) FROM note_tab n WHERE n.collection_id = c.id)
		 FROM note_collection_tab c WHERE c.yearbook_id = ? ORDER BY c.created_at DESC, c.id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []*Collection{}
	for rows.Next() {
		c := &Collection{}
		var deadline, revoked sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Label, &deadline, &c.CreatedAt, &revoked, &c.NoteCount); err != nil {
			return nil, err
		}
		c.DeadlineAt, c.RevokedAt = ms(deadline), ms(revoked)
		out = append(out, c)
	}
	return out, rows.Err()
}

// revoke sets revoked_at once; revoking again changes nothing and succeeds.
//
// Lock order: the collection's yearbook_tab row is locked (FOR SHARE: the collection row lock below serialises revokes) before the
// note_collection_tab row, like the yearbook delete does; a book or collection that vanished meanwhile is ErrNotFound.
func (s *Store) revoke(ctx context.Context, ownerID uint64, id string, now int64) error {
	var rowID, bookID uint64
	err := s.db.QueryRowContext(ctx,
		`SELECT c.id, c.yearbook_id FROM note_collection_tab c JOIN yearbook_tab y ON y.id = c.yearbook_id WHERE y.owner_id = ? AND c.public_id = ?`,
		ownerID, id).Scan(&rowID, &bookID) // 1:1 and owner-scoped, so the join stays (no locks are taken here)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockBook(ctx, tx, bookID); err != nil {
		return err
	}
	var revoked sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT revoked_at FROM note_collection_tab WHERE id = ? FOR UPDATE`, rowID).Scan(&revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !revoked.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE note_collection_tab SET revoked_at = ?, updated_at = ? WHERE id = ?`, now, now, rowID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// lockBook takes a share lock on the yearbook row, the first lock of every notes transaction that does not own the book
// (docs/db-conventions.md, Writes). ErrNotFound when the book is gone: the caller answers as for an unknown link.
func lockBook(ctx context.Context, tx *sql.Tx, bookID uint64) error {
	var id uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE id = ? FOR SHARE`, bookID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// Public is what a visitor with a valid link may learn, plus the internal ids the submission needs.
type Public struct {
	Title        string
	DisplayName  string
	DeadlineAt   *int64 // Unix ms
	CollectionID uint64
	YearbookID   uint64
	TemplateID   string // "" until the owner chooses one
}

// lookup finds an active collection by token hash with one indexed query; unknown and revoked are both ErrNotFound.
func (s *Store) lookup(ctx context.Context, hash [32]byte) (*Public, error) {
	p := &Public{}
	var deadline sql.NullInt64
	var tmpl sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT y.title, u.display_name, c.deadline_at, c.id, y.id, y.template_id FROM note_collection_tab c
		 JOIN yearbook_tab y ON y.id = c.yearbook_id JOIN user_tab u ON u.id = y.owner_id
		 WHERE c.token_hash = ? AND c.revoked_at IS NULL`, hash[:]).Scan(&p.Title, &p.DisplayName, &deadline, &p.CollectionID, &p.YearbookID, &tmpl)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p.DeadlineAt, p.TemplateID = ms(deadline), tmpl.String
	return p, nil
}

// noteCount is how many notes the collection holds, in any status.
func (s *Store) noteCount(ctx context.Context, collectionID uint64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM note_tab WHERE collection_id = ?`, collectionID).Scan(&n)
	return n, err
}

// newNote is a validated submission ready to store. answers is the JSON object of cleaned answers.
type newNote struct {
	ID         string
	Answers    []byte
	MediaRows  []int64 // internal ids of the photos, in order
	CreatedAt  int64   // Unix ms
	Collection uint64
	Yearbook   uint64
}

// insertNote stores the note and its photo links in one transaction.
//
// Lock order (docs/db-conventions.md, Writes): the yearbook_tab row first (FOR SHARE), then the note_collection_tab row FOR UPDATE,
// then note_tab and note_photo_tab. The yearbook delete locks the book row exclusively before it deletes the collection, notes
// and photo links, so a submit racing it either commits first or finds the book gone (ErrNotFound, 404) and never deadlocks.
// The collection lock makes the revoked, deadline and 300-note checks hold for the state the insert commits into: parallel
// submissions cannot all slip under the cap and a link revoked while photos were processed stores nothing.
func (s *Store) insertNote(ctx context.Context, n *newNote) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := lockBook(ctx, tx, n.Yearbook); err != nil {
		return err
	}
	var deadline, revoked sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT deadline_at, revoked_at FROM note_collection_tab WHERE id = ? AND yearbook_id = ? FOR UPDATE`, n.Collection, n.Yearbook).Scan(&deadline, &revoked)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && revoked.Valid) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if deadline.Valid && n.CreatedAt >= deadline.Int64 {
		return ErrClosed
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM note_tab WHERE collection_id = ?`, n.Collection).Scan(&count); err != nil {
		return err
	}
	if count >= maxNotes {
		return ErrFull
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		n.ID, n.Collection, n.Yearbook, string(n.Answers), StatusPending, n.CreatedAt, n.CreatedAt)
	if err != nil {
		return err
	}
	noteID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for pos, mediaRow := range n.MediaRows { // not N+1: at most maxPhotos (3) rows per note, in this one transaction
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) VALUES (?,?,?,?,?)`,
			noteID, mediaRow, pos, n.CreatedAt, n.CreatedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// deleteByYearbook removes the collections, notes and photo links of a yearbook inside the caller's transaction, which holds
// the yearbook lock. Children first, so no row is ever visible without its parent.
func deleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	for _, q := range []string{
		`DELETE FROM note_photo_tab WHERE note_id IN (SELECT id FROM note_tab WHERE yearbook_id = ?)`,
		`DELETE FROM note_tab WHERE yearbook_id = ?`,
		`DELETE FROM note_collection_tab WHERE yearbook_id = ?`,
	} {
		if _, err := tx.ExecContext(ctx, q, yearbookID); err != nil {
			return err
		}
	}
	return nil
}

// deleteMediaLinks removes the note links of a photo that is being deleted (there is no ON DELETE CASCADE).
func deleteMediaLinks(ctx context.Context, tx *sql.Tx, mediaID int64) error {
	_, err := tx.ExecContext(ctx, `DELETE FROM note_photo_tab WHERE media_id = ?`, mediaID)
	return err
}

func ms(n sql.NullInt64) *int64 {
	if !n.Valid {
		return nil
	}
	return &n.Int64
}
