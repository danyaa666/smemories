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
)

const maxActive = 5 // not-revoked collections per yearbook

// Collection is one link as the owner sees it in a list. The token is never part of it.
type Collection struct {
	ID         string     `json:"id"`
	Label      string     `json:"label"`
	DeadlineAt *time.Time `json:"deadline_at"`
	CreatedAt  time.Time  `json:"created_at"`
	RevokedAt  *time.Time `json:"revoked_at"`
	NoteCount  int        `json:"note_count"` // ponytail: always 0 until T-034 adds the notes table; then count them here
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
		`SELECT public_id, label, deadline_at, created_at, revoked_at FROM note_collections WHERE yearbook_id = ? ORDER BY created_at DESC, id DESC`, id)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := []Collection{}
	for rows.Next() {
		var c Collection
		var deadline, revoked sql.NullTime
		if err := rows.Scan(&c.ID, &c.Label, &deadline, &c.CreatedAt, &revoked); err != nil {
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

// Public is what a visitor with a valid link may learn.
type Public struct {
	Title       string
	DisplayName string
	DeadlineAt  *time.Time
}

// lookup finds an active collection by token hash with one indexed query; unknown and revoked are both ErrNotFound.
func (s *Store) lookup(ctx context.Context, hash [32]byte) (Public, error) {
	var p Public
	var deadline sql.NullTime
	err := s.db.QueryRowContext(ctx,
		`SELECT y.title, u.display_name, c.deadline_at FROM note_collections c
		 JOIN yearbooks y ON y.id = c.yearbook_id JOIN users u ON u.id = y.owner_id
		 WHERE c.token_hash = ? AND c.revoked_at IS NULL`, hash[:]).Scan(&p.Title, &p.DisplayName, &deadline)
	if errors.Is(err, sql.ErrNoRows) {
		return Public{}, ErrNotFound
	}
	p.DeadlineAt = utc(deadline)
	return p, err
}

func utc(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	u := t.Time.UTC()
	return &u
}
