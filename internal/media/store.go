package media

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

var (
	// ErrNotFound covers media or a yearbook that does not exist and one owned by someone else.
	ErrNotFound = errors.New("media: not found")
	// ErrQuota: the yearbook or the user is at a storage limit.
	ErrQuota = errors.New("media: quota exceeded")
)

// Quotas (AC6).
const (
	maxPerYearbook  = 200
	maxBytesPerUser = 500 << 20
)

// Media is one stored photo. Keys are server-generated; nothing here comes from the uploaded file name.
type Media struct {
	ID          string
	ObjectKey   string
	ThumbKey    string
	ContentType string
	Bytes       int
	Width       int
	Height      int
	SHA256      string
	CreatedAt   time.Time
	// Set by list only.
	UploaderKind string
	seq          uint64 // the database id, the keyset cursor position; never leaves the package
}

// Store persists media rows. Every query is parameterised and scoped by the owner of the yearbook.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// ownedYearbook returns the internal id of a yearbook the user owns.
func (s *Store) ownedYearbook(ctx context.Context, ownerID uint64, publicID string) (uint64, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM yearbooks WHERE owner_id = ? AND public_id = ?`, ownerID, publicID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// checkQuota fails with ErrQuota when adding one more photo of size add would pass a limit.
func checkQuota(ctx context.Context, q rowQuerier, ownerID, yearbookID uint64, add int) error {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE yearbook_id = ?`, yearbookID).Scan(&n); err != nil {
		return err
	}
	var used int64
	if err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(m.bytes), 0) FROM media m JOIN yearbooks y ON y.id = m.yearbook_id WHERE y.owner_id = ?`, ownerID).Scan(&used); err != nil {
		return err
	}
	if n >= maxPerYearbook || used+int64(add) > maxBytesPerUser {
		return ErrQuota
	}
	return nil
}

// insert stores the row after re-checking the quotas under a lock on the user row, so parallel
// uploads cannot all slip under a limit. It also locks the yearbook so it cannot vanish meanwhile.
func (s *Store) insert(ctx context.Context, ownerID, yearbookID uint64, m Media) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var locked uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = ? FOR UPDATE`, ownerID).Scan(&locked); err != nil {
		return err
	}
	err = tx.QueryRowContext(ctx, `SELECT id FROM yearbooks WHERE id = ? AND owner_id = ? FOR SHARE`, yearbookID, ownerID).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if err := checkQuota(ctx, tx, ownerID, yearbookID, m.Bytes); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
		`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at)
		 VALUES (?,?,'owner',?,?,?,?,?,?,?,?)`,
		m.ID, yearbookID, m.ObjectKey, m.ThumbKey, m.ContentType, m.Bytes, m.Width, m.Height, m.SHA256, m.CreatedAt)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// owned returns the media with that public id if its yearbook belongs to the user.
func (s *Store) owned(ctx context.Context, ownerID uint64, publicID string) (Media, error) {
	var m Media
	err := s.db.QueryRowContext(ctx,
		`SELECT m.public_id, m.object_key, m.thumb_key, m.content_type, m.bytes, m.width, m.height, m.sha256, m.created_at
		 FROM media m JOIN yearbooks y ON y.id = m.yearbook_id WHERE y.owner_id = ? AND m.public_id = ?`, ownerID, publicID).
		Scan(&m.ID, &m.ObjectKey, &m.ThumbKey, &m.ContentType, &m.Bytes, &m.Width, &m.Height, &m.SHA256, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return Media{}, ErrNotFound
	}
	m.CreatedAt = m.CreatedAt.UTC()
	return m, err
}

func (s *Store) remove(ctx context.Context, publicID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM media WHERE public_id = ?`, publicID)
	return err
}

// list returns up to limit photos of a yearbook, newest first (id descending), with id < before when before > 0.
// kinds is the set of uploader_kind values to include. The (yearbook_id) index carries the primary key, so the
// read is an index range scan in id order with no sort. FORCE INDEX because, on a big table, the optimizer is
// tempted by a reverse primary-key scan for ORDER BY id DESC LIMIT n, which walks every yearbook's rows.
func (s *Store) list(ctx context.Context, yearbookID uint64, kinds []string, before uint64, limit int) ([]Media, error) {
	q := `SELECT id, public_id, uploader_kind, bytes, width, height, created_at FROM media FORCE INDEX (ix_media_yearbook) WHERE yearbook_id = ? AND uploader_kind IN (?` +
		strings.Repeat(",?", len(kinds)-1) + `)`
	args := []any{yearbookID}
	for _, k := range kinds {
		args = append(args, k)
	}
	if before > 0 {
		q += ` AND id < ?`
		args = append(args, before)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Media
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.seq, &m.ID, &m.UploaderKind, &m.Bytes, &m.Width, &m.Height, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.CreatedAt = m.CreatedAt.UTC()
		out = append(out, m)
	}
	return out, rows.Err()
}
