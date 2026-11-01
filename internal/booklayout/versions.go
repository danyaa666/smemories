package booklayout

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ulid"
)

// Kinds of snapshot (VARCHAR column, no ENUM): the write that replaced the document.
const (
	KindAuto    = "auto"    // a save that started a new editing burst
	KindReset   = "reset"   // book-layout/reset
	KindRestore = "restore" // book-layout/restore
)

// snapshotCurrent stores the book's current layout document as a snapshot of the given kind, in the caller's transaction, and prunes the
// book to pol.keep snapshots. KindAuto does nothing while the newest snapshot is not older than pol.gapMs; the other kinds always store.
// Nothing is stored when the book has no layout. The caller holds the yearbook row lock, so the check and the insert cannot race.
// It returns the kind stored ("" = none) and the number of rows pruned.
func snapshotCurrent(ctx context.Context, tx *sql.Tx, yearbookID uint64, kind string, pol policy) (string, int64, error) {
	var tpl string
	var doc []byte
	var version int64
	err := tx.QueryRowContext(ctx, `SELECT template_id, layout_json, version FROM book_layout_tab WHERE yearbook_id = ?`, yearbookID).Scan(&tpl, &doc, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	if err != nil {
		return "", 0, apperr.Wrap(apperr.Internal, err, "read book layout")
	}
	if kind == KindAuto {
		var newest int64
		err := tx.QueryRowContext(ctx, `SELECT created_at FROM book_layout_version_tab WHERE yearbook_id = ? ORDER BY created_at DESC, id DESC LIMIT 1`, yearbookID).Scan(&newest)
		if err == nil && newest >= pol.nowMs-pol.gapMs {
			return "", 0, nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", 0, apperr.Wrap(apperr.Internal, err, "read newest snapshot")
		}
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO book_layout_version_tab (public_id, yearbook_id, version, kind, template_id, layout_json, item_count, bytes, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		ulid.New(time.UnixMilli(pol.nowMs)), yearbookID, version, kind, tpl, doc, itemCount(doc), len(doc), pol.nowMs); err != nil {
		return "", 0, apperr.Wrap(apperr.Internal, err, "write layout snapshot")
	}
	// The derived table is what lets MySQL take LIMIT inside IN; ties on created_at fall back to the id, as the list does.
	res, err := tx.ExecContext(ctx,
		`DELETE FROM book_layout_version_tab WHERE yearbook_id = ? AND id NOT IN
		   (SELECT id FROM (SELECT id FROM book_layout_version_tab WHERE yearbook_id = ? ORDER BY created_at DESC, id DESC LIMIT ?) newest)`,
		yearbookID, yearbookID, pol.keep)
	if err != nil {
		return "", 0, apperr.Wrap(apperr.Internal, err, "prune layout snapshots")
	}
	pruned, _ := res.RowsAffected()
	return kind, pruned, nil
}

// deleteVersions removes every snapshot of the book (template change, yearbook delete). A reset keeps them.
func deleteVersions(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_layout_version_tab WHERE yearbook_id = ?`, yearbookID); err != nil {
		return apperr.Wrap(apperr.Internal, err, "delete layout snapshots")
	}
	return nil
}

// VersionInfo is one entry of version-list: no document body.
type VersionInfo struct {
	ID        string `json:"id"`         // public id of the snapshot
	Version   int64  `json:"version"`    // the layout version the snapshot held
	Kind      string `json:"kind"`       // auto, reset or restore
	CreatedAt int64  `json:"created_at"` // Unix ms, when the document was replaced
	ItemCount int    `json:"item_count"` // items on all pages
	Bytes     int    `json:"bytes"`      // length of the stored document
}

// snap is one stored snapshot with its body, and the template the book has now.
type snap struct {
	VersionInfo
	templateID string  // the template the snapshot was made for
	layout     []byte  // the document
	bookTpl    *string // the book's current template
}

// bookOf resolves the owner's book: its internal id and current template. Another user's and an unknown book are errNotFound.
func (s *Store) bookOf(ctx context.Context, ownerID uint64, publicID string) (uint64, *string, error) {
	var id uint64
	var tpl sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT id, template_id FROM yearbook_tab WHERE owner_id = ? AND public_id = ?`, ownerID, publicID).Scan(&id, &tpl)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, errNotFound
	}
	if err != nil {
		return 0, nil, apperr.Wrap(apperr.Internal, err, "read book")
	}
	if !tpl.Valid {
		return id, nil, nil
	}
	return id, &tpl.String, nil
}

// versions lists the owner's book's snapshots, newest first, at most limit, without bodies.
func (s *Store) versions(ctx context.Context, ownerID uint64, publicID string, limit int) ([]VersionInfo, error) {
	id, _, err := s.bookOf(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT public_id, version, kind, created_at, item_count, bytes FROM book_layout_version_tab WHERE yearbook_id = ? ORDER BY created_at DESC, id DESC LIMIT ?`, id, limit)
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "list layout snapshots")
	}
	defer func() { _ = rows.Close() }()
	out := make([]VersionInfo, 0, limit)
	for rows.Next() {
		var v VersionInfo
		if err := rows.Scan(&v.ID, &v.Version, &v.Kind, &v.CreatedAt, &v.ItemCount, &v.Bytes); err != nil {
			return nil, apperr.Wrap(apperr.Internal, err, "scan layout snapshot")
		}
		out = append(out, v)
	}
	return out, apperr.WithCode(apperr.Internal, rows.Err())
}

// version reads one snapshot of the owner's book; an unknown id and one of another book are errNotFound.
func (s *Store) version(ctx context.Context, ownerID uint64, publicID, versionID string) (*snap, error) {
	id, tpl, err := s.bookOf(ctx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	v := &snap{bookTpl: tpl}
	err = s.db.QueryRowContext(ctx,
		`SELECT public_id, version, kind, created_at, item_count, bytes, template_id, layout_json FROM book_layout_version_tab WHERE yearbook_id = ? AND public_id = ?`,
		id, versionID).Scan(&v.ID, &v.Version, &v.Kind, &v.CreatedAt, &v.ItemCount, &v.Bytes, &v.templateID, &v.layout)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, apperr.New(apperr.NotFound, "layout version not found")
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "read layout snapshot")
	}
	return v, nil
}
