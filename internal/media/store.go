package media

import (
	"context"
	"database/sql"
	"errors"
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
	RowID       int64 // internal id, set by insert; for rows that refer to the media (note photos)
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
func (s *Store) insert(ctx context.Context, ownerID, yearbookID uint64, kind string, m Media) (Media, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Media{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var locked uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM users WHERE id = ? FOR UPDATE`, ownerID).Scan(&locked); err != nil {
		return Media{}, err
	}
	err = tx.QueryRowContext(ctx, `SELECT id FROM yearbooks WHERE id = ? AND owner_id = ? FOR SHARE`, yearbookID, ownerID).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return Media{}, ErrNotFound
	}
	if err != nil {
		return Media{}, err
	}
	if err := checkQuota(ctx, tx, ownerID, yearbookID, m.Bytes); err != nil {
		return Media{}, err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, yearbookID, kind, m.ObjectKey, m.ThumbKey, m.ContentType, m.Bytes, m.Width, m.Height, m.SHA256, m.CreatedAt)
	if err != nil {
		return Media{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Media{}, err
	}
	m.RowID = id
	return m, tx.Commit()
}

// yearbookByRow returns the owner and the public id of a yearbook given its internal id.
func (s *Store) yearbookByRow(ctx context.Context, id uint64) (ownerID uint64, publicID string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT owner_id, public_id FROM yearbooks WHERE id = ?`, id).Scan(&ownerID, &publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	return ownerID, publicID, err
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
