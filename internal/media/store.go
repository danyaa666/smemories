package media

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Quotas (AC6, T-105). Bytes are those of every stored variant (display, thumb, print).
//
// The owner's quota counts the owner's own uploads: at most maxPerYearbook photos per yearbook and maxBytesPerUser bytes across
// the owner's yearbooks. Photos sent through a collection link have a quota of their own per yearbook, so a leaked link cannot
// use up the owner's room.
const (
	maxPerYearbook  = 200
	maxBytesPerUser = 500 << 20

	maxContributorPhotos = 600
	maxContributorBytes  = 1 << 30

	// maxDesignPerYearbook is the owner's cap of design images (T-152) per yearbook. They are not photos: they do not count toward
	// maxPerYearbook, but their bytes count in maxBytesPerUser with the photos' bytes.
	maxDesignPerYearbook = 30
)

// Values of purpose (VARCHAR column, no ENUM; validated here). A design image is a background or decoration of the layout editor.
const (
	PurposePhoto  = "photo"
	PurposeDesign = "design"
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
	Bytes       int // of the display object
	ThumbBytes  int
	PrintBytes  int
	Width       int
	Height      int
	SHA256      string
	CreatedAt   int64 // Unix ms
	RowID       int64 // internal id, set by insert; for rows that refer to the media (note photos)
	// ByClassAdmin is set by readable only: the caller reads this photo as an admin of a class, not as the owner (T-133).
	ByClassAdmin bool
	Purpose      string // PurposePhoto or PurposeDesign; set by list and insert
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

// ownedYearbook returns the internal id and the STORED public id of a yearbook the user owns. Object keys are built from the
// stored id: the column compares case-insensitively, so the caller's spelling may differ from it (audit M3).
func (s *Store) ownedYearbook(ctx context.Context, ownerID uint64, publicID string) (uint64, string, error) {
	var id uint64
	var stored string
	err := s.db.QueryRowContext(ctx, `SELECT id, public_id FROM yearbook_tab WHERE owner_id = ? AND public_id = ?`, ownerID, publicID).Scan(&id, &stored)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	if err != nil {
		return 0, "", dbError(err, "select yearbook")
	}
	return id, stored, nil
}

// variantBytes is the sum of the stored sizes of a photo's variants, as SQL.
const variantBytes = `(bytes + thumb_bytes + print_bytes)`

// checkQuota fails with ErrQuota (owner photos and design images) or ErrContributorQuota (photos sent through a link) when adding one
// more image of kind and purpose and total size add would pass that limit. The two kinds never count toward each other's quota.
// Within the owner's, the count limit is per purpose (200 photos, 30 design images per yearbook) and the byte limit is shared.
func checkQuota(ctx context.Context, q rowQuerier, ownerID, yearbookID uint64, kind, purpose string, add int) error {
	if kind == KindContributor {
		var n int
		var used int64
		if err := q.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM`+variantBytes+`, 0) FROM media_tab WHERE yearbook_id = ? AND uploader_kind = ?`,
			yearbookID, KindContributor).Scan(&n, &used); err != nil {
			return dbError(err, "sum contributor photos")
		}
		if n >= maxContributorPhotos || used+int64(add) > maxContributorBytes {
			return ErrContributorQuota
		}
		return nil
	}
	var n int
	if err := q.QueryRowContext(ctx, `SELECT COUNT(*) FROM media_tab WHERE yearbook_id = ? AND uploader_kind = ? AND purpose = ?`, yearbookID, KindOwner, purpose).Scan(&n); err != nil {
		return dbError(err, "count photos")
	}
	var used int64
	if err := q.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(m.bytes + m.thumb_bytes + m.print_bytes), 0) FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id
		 WHERE y.owner_id = ? AND m.uploader_kind = ?`, ownerID, KindOwner).Scan(&used); err != nil {
		return dbError(err, "sum photo bytes")
	}
	limit := maxPerYearbook
	if purpose == PurposeDesign {
		limit = maxDesignPerYearbook
	}
	if n >= limit || used+int64(add) > maxBytesPerUser {
		return ErrQuota
	}
	return nil
}

// insert stores the row after re-checking the quotas under a lock on the user row, so parallel
// uploads cannot all slip under a limit. It also locks the yearbook so it cannot vanish meanwhile.
func (s *Store) insert(ctx context.Context, ownerID, yearbookID uint64, kind, purpose string, m *Media) (*Media, error) {
	if kind != KindOwner && kind != KindContributor {
		return nil, apperr.New(apperr.Internal, "media: unknown uploader_kind "+kind)
	}
	if purpose != PurposePhoto && (purpose != PurposeDesign || kind != KindOwner) { // friends send photos only
		return nil, apperr.New(apperr.Internal, "media: unknown purpose "+purpose)
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
	if err := checkQuota(ctx, tx, ownerID, yearbookID, kind, purpose, m.Bytes+m.ThumbBytes+m.PrintBytes); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, purpose, object_key, thumb_key, print_key, content_type, bytes, thumb_bytes, print_bytes, width, height, sha256, created_at, updated_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		m.ID, yearbookID, kind, purpose, m.ObjectKey, m.ThumbKey, m.PrintKey, m.ContentType, m.Bytes, m.ThumbBytes, m.PrintBytes, m.Width, m.Height, m.SHA256, m.CreatedAt, m.CreatedAt)
	if err != nil {
		return nil, dbError(err, "insert photo")
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, dbError(err, "read photo id")
	}
	m.RowID, m.Purpose = id, purpose
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

const mediaCols = `m.id, m.public_id, m.object_key, m.thumb_key, m.print_key, m.content_type, m.bytes, m.width, m.height, m.sha256, m.created_at`

// scanMedia reads mediaCols; extra are further columns the query selected after them.
func scanMedia(row *sql.Row, extra ...any) (*Media, error) {
	m := new(Media)
	var printKey sql.NullString
	err := row.Scan(append([]any{&m.RowID, &m.ID, &m.ObjectKey, &m.ThumbKey, &printKey, &m.ContentType, &m.Bytes, &m.Width, &m.Height, &m.SHA256, &m.CreatedAt}, extra...)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, dbError(err, "select photo")
	}
	m.PrintKey = printKey.String
	return m, nil
}

// owned returns the media with that public id if its yearbook belongs to the user.
func (s *Store) owned(ctx context.Context, ownerID uint64, publicID string) (*Media, error) {
	return scanMedia(s.db.QueryRowContext(ctx,
		`SELECT `+mediaCols+` FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE y.owner_id = ? AND m.public_id = ?`, ownerID, publicID))
}

// ClassAdminRole repeats class.RoleAdmin (class sits above media); a class unit test pins them together.
const ClassAdminRole = "admin"

// readable is owned plus the one read path of a class admin (E07, T-027, docs/class-permissions.md): the media is also readable
// when the caller is an admin of a class in which the media's yearbook is a member's linked page (the member owns it), and the media
// is that yearbook's owner profile photo or a photo of one of its approved notes, or a design image of that yearbook (T-152: the
// backgrounds and decorations of the student's layout, which the class book prints). Nothing else: not the cover, not a photo of a
// pending or hidden note, not a photo that no note uses, not a yearbook of a removed member or of another class. One statement, so
// a note hidden a moment ago is already a 404. Read only: Delete and every write keep using owned.
func (s *Store) readable(ctx context.Context, userID uint64, publicID string) (*Media, error) {
	var owner bool
	m, err := scanMedia(s.db.QueryRowContext(ctx,
		`SELECT `+mediaCols+`, y.owner_id = ? FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id
		 WHERE m.public_id = ? AND (y.owner_id = ? OR EXISTS (
		   SELECT 1 FROM class_member_tab cm
		   JOIN class_member_tab adm ON adm.class_id = cm.class_id AND adm.user_id = ? AND adm.role = ?
		   WHERE cm.yearbook_id = y.id AND cm.user_id = y.owner_id
		     AND (m.purpose = ?
		       OR EXISTS (SELECT 1 FROM profile_tab p WHERE p.yearbook_id = y.id AND p.is_owner = 1 AND p.photo_media_id = m.id)
		       OR EXISTS (SELECT 1 FROM note_photo_tab np JOIN note_tab n ON n.id = np.note_id
		                  WHERE np.media_id = m.id AND n.yearbook_id = y.id AND n.status = ?))))`,
		userID, publicID, userID, userID, ClassAdminRole, PurposeDesign, ShareableNoteStatus), &owner)
	if m != nil {
		m.ByClassAdmin = !owner
	}
	return m, err
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
// A design image is never shareable (T-152). A photo is shareable when it is the cover or a profile photo of its book, or belongs to an approved note of its book; a
// deleted note, book or photo has no row left to match. The media-to-book join is 1:1 by primary key (like owned), and the
// three tests use the indexes on cover_media_id, photo_media_id and note_photo_tab.media_id.
// <=> is null-safe equality: a book without a cover is false, not NULL.
func (s *Store) publicPhoto(ctx context.Context, mediaID string) (*publicPhoto, error) {
	p := new(publicPhoto)
	err := s.db.QueryRowContext(ctx,
		`SELECT m.id, m.public_id, m.object_key, m.content_type, m.bytes, m.sha256, m.created_at, y.public_id, y.link_epoch,
		   (m.purpose = ? AND (y.cover_media_id <=> m.id
		    OR EXISTS (SELECT 1 FROM profile_tab p WHERE p.yearbook_id = y.id AND p.photo_media_id = m.id)
		    OR EXISTS (SELECT 1 FROM note_photo_tab np JOIN note_tab n ON n.id = np.note_id
		               WHERE np.media_id = m.id AND n.yearbook_id = y.id AND n.status = ?)))
		 FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE m.public_id = ?`, PurposePhoto, ShareableNoteStatus, mediaID).
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
// kinds is the set of uploader_kind values to include and purpose the one purpose listed. The (yearbook_id) index carries the primary key, so the
// read is an index range scan in id order with no sort. FORCE INDEX because, on a big table, the optimizer is
// tempted by a reverse primary-key scan for ORDER BY id DESC LIMIT n, which walks every yearbook's rows.
func (s *Store) list(ctx context.Context, yearbookID uint64, kinds []string, purpose string, before uint64, limit int) ([]Media, error) {
	q := `SELECT id, public_id, uploader_kind, content_type, bytes, width, height, created_at FROM media_tab FORCE INDEX (idx_yearbook_id) WHERE yearbook_id = ? AND purpose = ? AND uploader_kind IN (?` +
		strings.Repeat(",?", len(kinds)-1) + `)`
	args := []any{yearbookID, purpose}
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

// setPrintKey records the print object of a photo and its size at nowMs; false when the photo was deleted or already has one.
// One statement on one row, so it takes no lock that could be part of a cycle.
func (s *Store) setPrintKey(ctx context.Context, rowID int64, key string, size int, nowMs int64) (bool, error) {
	res, err := s.db.ExecContext(ctx, `UPDATE media_tab SET print_key = ?, print_bytes = ?, updated_at = ? WHERE id = ? AND print_key IS NULL`, key, size, nowMs, rowID)
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

// designImages returns the public ids, among ids, of the design images of the yearbook (internal id yb): the check a layout needs
// before it may reference an image. A photo id, an id of another book and an unknown id are not in the set.
func (s *Store) designImages(ctx context.Context, yb uint64, ids []string) (map[string]bool, error) {
	out := make(map[string]bool, len(ids))
	if len(ids) == 0 {
		return out, nil
	}
	args := []any{yb, PurposeDesign}
	for _, id := range ids {
		args = append(args, id)
	}
	q := strings.Replace(`SELECT public_id FROM media_tab WHERE yearbook_id = ? AND purpose = ? AND public_id IN (?)`, "(?)", "(?"+strings.Repeat(",?", len(ids)-1)+")", 1)
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, dbError(err, "select design images")
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, dbError(err, "scan design image")
		}
		out[id] = true
	}
	return out, dbErrorOrNil(rows.Err(), "read design images")
}
