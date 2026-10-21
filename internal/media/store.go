package media

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Quotas (AC6).
const (
	maxPerYearbook  = 200
	maxBytesPerUser = 500 << 20
)

// Values of uploader_kind (VARCHAR column, no ENUM; validated here).
const (
	KindOwner       = "owner"
	KindContributor = "contributor"
)

// Media is one stored photo. Keys are server-generated; nothing here comes from the uploaded file name.
type Media struct {
	ID          string
	ObjectKey   string
	ThumbKey    string
	PrintKey    string // empty until the backfill has created it (T-057)
	ContentType string
	Bytes       int
	Width       int
	Height      int
	SHA256      string
	CreatedAt   int64 // Unix ms
	RowID       int64 // internal id, set by insert; for rows that refer to the media (note photos)
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
	err := s.db.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE owner_id = ? AND public_id = ?`, ownerID, publicID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	if err != nil {
		return 0, dbError(err, "select yearbook")
	}
	return id, nil
}

// checkQuota fails with ErrQuota when adding one more photo of size add would pass a limit.
func checkQuota(ctx context.Context, q rowQuerier, ownerID, yearbookID uint64, add int) error {
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_tab WHERE yearbook_id = ?`, yearbookID).Scan(&n); err != nil {
		return dbError(err, "count photos")
	}
	var used int64
	if err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(m.bytes), 0) FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE y.owner_id = ?`, ownerID).Scan(&used); err != nil {
		return dbError(err, "sum photo bytes")
	}
	if n >= maxPerYearbook || used+int64(add) > maxBytesPerUser {
		return ErrQuota
	}
	return nil
}

// insert stores the row after re-checking the quotas under a lock on the user row, so parallel
// uploads cannot all slip under a limit. It also locks the yearbook so it cannot vanish meanwhile.
func (s *Store) insert(ctx context.Context, ownerID, yearbookID uint64, kind string, m *Media) (*Media, error) {
	if kind != KindOwner && kind != KindContributor {
		return nil, apperr.New(apperr.Internal, "media: unknown uploader_kind "+kind)
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, dbError(err, "begin insert photo")
	}
	defer func() { _ = tx.Rollback() }()
	var locked uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM user_tab WHERE id = ? FOR UPDATE`, ownerID).Scan(&locked); err != nil {
		return nil, dbError(err, "lock user")
	}
	err = tx.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE id = ? AND owner_id = ? FOR SHARE`, yearbookID, ownerID).Scan(&locked)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, dbError(err, "lock yearbook")
	}
	if err := checkQuota(ctx, tx, ownerID, yearbookID, m.Bytes); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, yearbookID, kind, m.ObjectKey, m.ThumbKey, m.PrintKey, m.ContentType, m.Bytes, m.Width, m.Height, m.SHA256, m.CreatedAt, m.CreatedAt)
	if err != nil {
		return nil, dbError(err, "insert photo")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, dbError(err, "read photo id")
	}
	m.RowID = id
	if err := tx.Commit(); err != nil {
		return nil, dbError(err, "commit photo")
	}
	return m, nil
}

// yearbookByRow returns the owner and the public id of a yearbook given its internal id.
func (s *Store) yearbookByRow(ctx context.Context, id uint64) (ownerID uint64, publicID string, err error) {
	err = s.db.QueryRowContext(ctx, `SELECT owner_id, public_id FROM yearbook_tab WHERE id = ?`, id).Scan(&ownerID, &publicID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	if err != nil {
		return 0, "", dbError(err, "select yearbook by row")
	}
	return ownerID, publicID, nil
}

// owned returns the media with that public id if its yearbook belongs to the user.
func (s *Store) owned(ctx context.Context, ownerID uint64, publicID string) (*Media, error) {
	m := new(Media)
	var printKey sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT m.id, m.public_id, m.object_key, m.thumb_key, m.print_key, m.content_type, m.bytes, m.width, m.height, m.sha256, m.created_at
		 FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE y.owner_id = ? AND m.public_id = ?`, ownerID, publicID).
		Scan(&m.RowID, &m.ID, &m.ObjectKey, &m.ThumbKey, &printKey, &m.ContentType, &m.Bytes, &m.Width, &m.Height, &m.SHA256, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, dbError(err, "select photo")
	}
	m.PrintKey = printKey.String
	return m, nil
}

// ShareableNoteStatus is the status a note must have for its photos to be reachable through a public link. It is
// notes.StatusApproved (notes imports media, so the value is repeated here; a notes test pins them together).
const ShareableNoteStatus = "approved"

// publicPhoto is what the public endpoint needs to decide about one link: the photo, its book's public id and current link epoch,
// and whether the photo may be shared at all right now.
type publicPhoto struct {
	m          Media
	yearbookID string
	epoch      uint32
	shareable  bool
}

// publicPhoto loads a photo by public id for the public endpoint, without any ownership scope: the token is the credential.
// One row, one statement, whatever the answer, so a bad link, a hidden note and a deleted book take about the same time.
// A photo is shareable when it is the cover or a profile photo of its book, or belongs to an approved note of its book; a
// deleted note, book or photo has no row left to match. The media-to-book join is 1:1 by primary key (like owned), and the
// three tests use the indexes on cover_media_id, photo_media_id and note_photo_tab.media_id.
// <=> is null-safe equality: a book without a cover is false, not NULL.
func (s *Store) publicPhoto(ctx context.Context, mediaID string) (*publicPhoto, error) {
	p := new(publicPhoto)
	err := s.db.QueryRowContext(ctx,
		`SELECT m.id, m.public_id, m.object_key, m.content_type, m.bytes, m.sha256, m.created_at, y.public_id, y.link_epoch,
		   (y.cover_media_id <=> m.id
		    OR EXISTS (SELECT 1 FROM profile_tab p WHERE p.yearbook_id = y.id AND p.photo_media_id = m.id)
		    OR EXISTS (SELECT 1 FROM note_photo_tab np JOIN note_tab n ON n.id = np.note_id
		               WHERE np.media_id = m.id AND n.yearbook_id = y.id AND n.status = ?))
		 FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE m.public_id = ?`, ShareableNoteStatus, mediaID).
		Scan(&p.m.RowID, &p.m.ID, &p.m.ObjectKey, &p.m.ContentType, &p.m.Bytes, &p.m.SHA256, &p.m.CreatedAt, &p.yearbookID, &p.epoch, &p.shareable)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, dbError(err, "select public photo")
	}
	return p, nil
}

// keys lists the objects of a photo that exist (the print object is missing until the backfill ran).
func (m *Media) keys() []string {
	if m.PrintKey == "" {
		return []string{m.ObjectKey, m.ThumbKey}
	}
	return []string{m.ObjectKey, m.ThumbKey, m.PrintKey}
}

// removeRow deletes the photo row. The covers and profile photos that point at it are cleared in the same transaction
// (refs may be nil in tests that wire no yearbooks): there is no foreign key to do it.
//
// Lock order (docs/db-conventions.md, Writes): refs.ClearMediaRefs locks the photo's yearbook_tab row first, then
// yearbook_tab and profile_tab are updated, then the media_tab row is deleted. Every writer of media_tab rows (insert,
// DeleteByYearbook, this) takes the yearbook lock before touching a media row.
func (s *Store) removeRow(ctx context.Context, rowID int64, refs RefClearer) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return dbError(err, "begin delete photo")
	}
	defer func() { _ = tx.Rollback() }()
	if refs != nil {
		if err := refs.ClearMediaRefs(ctx, tx, rowID); err != nil {
			return err // the clearers wrap their own failures
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_tab WHERE id = ?`, rowID); err != nil {
		return dbError(err, "delete photo")
	}
	if err := tx.Commit(); err != nil {
		return dbError(err, "commit delete photo")
	}
	return nil
}

// byRows returns the photos with these internal ids (those that still exist).
func (s *Store) byRows(ctx context.Context, rowIDs []int64) ([]Media, error) {
	if len(rowIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(rowIDs))
	for i, id := range rowIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		strings.Replace(`SELECT id, object_key, thumb_key, print_key FROM media_tab WHERE id IN (?)`, "(?)", "(?"+strings.Repeat(",?", len(rowIDs)-1)+")", 1), args...)
	if err != nil {
		return nil, dbError(err, "select photos by row")
	}
	defer func() { _ = rows.Close() }()
	var out []Media
	for rows.Next() {
		var m Media
		var printKey sql.NullString
		if err := rows.Scan(&m.RowID, &m.ObjectKey, &m.ThumbKey, &printKey); err != nil {
			return nil, dbError(err, "scan photo")
		}
		m.PrintKey = printKey.String
		out = append(out, m)
	}
	return out, dbErrorOrNil(rows.Err(), "read photos by row")
}

func dbErrorOrNil(err error, what string) error {
	if err == nil {
		return nil
	}
	return dbError(err, what)
}

// deleteByYearbook removes every photo row of a yearbook inside the caller's transaction (which holds the yearbook lock).
func deleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM media_tab WHERE yearbook_id = ?`, yearbookID); err != nil {
		return dbError(err, "delete yearbook photos")
	}
	return nil
}

// list returns up to limit photos of a yearbook, newest first (id descending), with id < before when before > 0.
// kinds is the set of uploader_kind values to include. The (yearbook_id) index carries the primary key, so the
// read is an index range scan in id order with no sort. FORCE INDEX because, on a big table, the optimizer is
// tempted by a reverse primary-key scan for ORDER BY id DESC LIMIT n, which walks every yearbook's rows.
func (s *Store) list(ctx context.Context, yearbookID uint64, kinds []string, before uint64, limit int) ([]Media, error) {
	q := `SELECT id, public_id, uploader_kind, content_type, bytes, width, height, created_at FROM media_tab FORCE INDEX (idx_yearbook_id) WHERE yearbook_id = ? AND uploader_kind IN (?` +
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
		return nil, dbError(err, "list photos")
	}
	defer func() { _ = rows.Close() }()
	var out []Media
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.seq, &m.ID, &m.UploaderKind, &m.ContentType, &m.Bytes, &m.Width, &m.Height, &m.CreatedAt); err != nil {
			return nil, dbError(err, "scan photo")
		}
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, dbError(err, "list photos")
	}
	return out, nil
}

// withoutPrint returns up to limit photos with id > after that have no print object yet, oldest first.
func (s *Store) withoutPrint(ctx context.Context, after int64, limit int) ([]Media, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, public_id, object_key FROM media_tab WHERE id > ? AND print_key IS NULL ORDER BY id LIMIT ?`, after, limit)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []Media
	for rows.Next() {
		var m Media
		if err := rows.Scan(&m.RowID, &m.ID, &m.ObjectKey); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// setPrintKey records the print object of a photo at nowMs; false when the photo was deleted or already has one.
// One statement on one row, so it takes no lock that could be part of a cycle.
func (s *Store) setPrintKey(ctx context.Context, rowID int64, key string, nowMs int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE media_tab SET print_key = ?, updated_at = ? WHERE id = ? AND print_key IS NULL`, key, nowMs, rowID)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// exists reports whether the photo row is still there.
func (s *Store) exists(ctx context.Context, rowID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_tab WHERE id = ?`, rowID).Scan(&n)
	return n == 1, err
}
