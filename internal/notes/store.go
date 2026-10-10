package notes

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	// ErrNotFound covers a missing collection or yearbook and one owned by someone else: callers cannot tell them apart.
	ErrNotFound     = errors.New("notes: not found")
	ErrLimitReached = errors.New("notes: collection limit reached")
	// ErrFull: the collection already holds maxNotes notes.
	ErrFull = errors.New("notes: collection full")
	// ErrClosed: the collection's deadline has passed.
	ErrClosed = errors.New("notes: collection closed")
)

const (
	maxActive = 5   // not-revoked collections per yearbook
	maxNotes  = 300 // notes per collection, in any status
)

// Collection is one link as the owner sees it in a list. The token is never part of it.
type Collection struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	DeadlineAt *time.Time `json:"deadline_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	NoteCount  int        `json:"note_count"` // notes in any status
}

// Store persists collections. Every owner query is scoped by yearbooks.owner_id: a row is never fetched by id and compared afterwards.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// create locks the owner's book row so two concurrent creates cannot both slip under maxActive.
func (s *Store) create(ctx context.Context, ownerID uint64, bookID string, c Collection, hash [32]byte) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var id uint64
	err = tx.QueryRowContext(ctx, `SELECT id FROM yearbooks WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, bookID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM note_collections WHERE yearbook_id = ? AND revoked_at IS NULL`, id).Scan(&n); err != nil {
		return err
	}
	if n >= maxActive {
		return ErrLimitReached
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO note_collections (public_id, yearbook_id, token_hash, label, deadline_at, created_at) VALUES (?,?,?,?,?,?)`,
		c.ID, id, hash[:], c.Label, c.DeadlineAt, c.CreatedAt); err != nil {
		return err
	}
	return tx.Commit()
}

// list returns the book's collections, newest first. ponytail: unpaged; revoked links stay in the list, add a cursor if that gets long.
func (s *Store) list(ctx context.Context, ownerID uint64, bookID string) ([]Collection, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM yearbooks WHERE owner_id = ? AND public_id = ?`, ownerID, bookID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.public_id, c.label, c.deadline_at, c.created_at, c.revoked_at, (SELECT COUNT(*) FROM notes n WHERE n.collection_id = c.id)
		 FROM note_collections c WHERE c.yearbook_id = ? ORDER BY c.created_at DESC, c.id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Collection{}
	for rows.Next() {
		var c Collection
		var deadline, revoked sql.NullTime
		if err := rows.Scan(&c.ID, &c.Label, &deadline, &c.CreatedAt, &revoked, &c.NoteCount); err != nil {
			return nil, err
		}
		c.DeadlineAt, c.RevokedAt = utc(deadline), utc(revoked)
		c.CreatedAt = c.CreatedAt.UTC()
		out = append(out, c)
	}
	return out, rows.Err()
}

// revoke sets revoked_at once; revoking again changes nothing and succeeds.
func (s *Store) revoke(ctx context.Context, ownerID uint64, id string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var rowID uint64
	var revoked sql.NullTime
	err = tx.QueryRowContext(ctx,
		`SELECT c.id, c.revoked_at FROM note_collections c JOIN yearbooks y ON y.id = c.yearbook_id WHERE y.owner_id = ? AND c.public_id = ? FOR UPDATE`,
		ownerID, id).Scan(&rowID, &revoked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if !revoked.Valid {
		if _, err := tx.ExecContext(ctx, `UPDATE note_collections SET revoked_at = ? WHERE id = ?`, now.UTC(), rowID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Public is what a visitor with a valid link may learn, plus the internal ids the submission needs.
type Public struct {
	Title        string
	DisplayName  string
	DeadlineAt   *time.Time
	CollectionID uint64
	YearbookID   uint64
	TemplateID   string // "" until the owner chooses one
}

// lookup finds an active collection by token hash with one indexed query; unknown and revoked are both ErrNotFound.
func (s *Store) lookup(ctx context.Context, hash [32]byte) (Public, error) {
	var p Public
	var deadline sql.NullTime
	var tmpl sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT y.title, u.display_name, c.deadline_at, c.id, y.id, y.template_id FROM note_collections c
		 JOIN yearbooks y ON y.id = c.yearbook_id JOIN users u ON u.id = y.owner_id
		 WHERE c.token_hash = ? AND c.revoked_at IS NULL`, hash[:]).Scan(&p.Title, &p.DisplayName, &deadline, &p.CollectionID, &p.YearbookID, &tmpl)
	if errors.Is(err, sql.ErrNoRows) {
		return Public{}, ErrNotFound
	}
	p.DeadlineAt, p.TemplateID = utc(deadline), tmpl.String
	return p, err
}

// noteCount is how many notes the collection holds, in any status.
func (s *Store) noteCount(ctx context.Context, collectionID uint64) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes WHERE collection_id = ?`, collectionID).Scan(&n)
	return n, err
}

// newNote is a validated submission ready to store. answers is the JSON object of cleaned answers.
type newNote struct {
	ID         string
	Answers    []byte
	MediaRows  []int64 // internal ids of the photos, in order
	CreatedAt  time.Time
	Collection uint64
	Yearbook   uint64
}

// insertNote stores the note and its photo links in one transaction. It locks the collection row first, so
// the revoked, deadline and 300-note checks are made against the state the insert commits into: parallel
// submissions cannot all slip under the cap and a link revoked while photos were processed stores nothing.
func (s *Store) insertNote(ctx context.Context, n newNote) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var deadline, revoked sql.NullTime
	err = tx.QueryRowContext(ctx, `SELECT deadline_at, revoked_at FROM note_collections WHERE id = ? FOR UPDATE`, n.Collection).Scan(&deadline, &revoked)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && revoked.Valid) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if deadline.Valid && !n.CreatedAt.Before(deadline.Time) {
		return ErrClosed
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM notes WHERE collection_id = ?`, n.Collection).Scan(&count); err != nil {
		return err
	}
	if count >= maxNotes {
		return ErrFull
	}
	res, err := tx.ExecContext(ctx, `INSERT INTO notes (public_id, collection_id, yearbook_id, answers, created_at) VALUES (?,?,?,?,?)`,
		n.ID, n.Collection, n.Yearbook, string(n.Answers), n.CreatedAt.UTC())
	if err != nil {
		return err
	}
	noteID, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for pos, mediaRow := range n.MediaRows {
		if _, err := tx.ExecContext(ctx, `INSERT INTO note_photos (note_id, media_id, position) VALUES (?,?,?)`, noteID, mediaRow, pos); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func utc(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}
