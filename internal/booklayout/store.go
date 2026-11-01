package booklayout

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danyaa666/smemories/internal/apperr"
)

// errNotFound is a missing book and another user's book alike.
var errNotFound = apperr.New(apperr.NotFound, "yearbook not found")

// Store persists the layout rows. Every query is scoped by yearbook_tab.owner_id.
type Store struct{ db *sql.DB }

// NewStore builds the store.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// state is the owner's book and its layout row, if any.
type state struct {
	templateID *string // the book's template
	yearbookID uint64
	row        *row
	version    int64 // the last version issued for the book (0 = never saved); the row's version, or the next base after a reset
}

type row struct {
	templateID string
	layout     []byte
	version    int64
	updatedAt  int64
}

// get reads the owner's book and its layout row in one 1:1 owner-scoped join.
func (s *Store) get(ctx context.Context, ownerID uint64, publicID string) (*state, error) {
	var st state
	var tpl, lTpl sql.NullString
	var layout []byte
	var version, at sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT y.id, y.template_id, l.template_id, l.layout_json, l.version, l.updated_at, y.layout_version
		 FROM yearbook_tab y LEFT JOIN book_layout_tab l ON l.yearbook_id = y.id
		 WHERE y.owner_id = ? AND y.public_id = ?`, ownerID, publicID).Scan(&st.yearbookID, &tpl, &lTpl, &layout, &version, &at, &st.version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "read book layout")
	}
	if tpl.Valid {
		st.templateID = &tpl.String
	}
	if lTpl.Valid {
		st.row = &row{templateID: lTpl.String, layout: layout, version: version.Int64, updatedAt: at.Int64}
	}
	return &st, nil
}

// conflict is a save that lost: the stored row (nil when there is none) and the current version are what the caller must start from.
type conflict struct {
	row     *row
	version int64
}

// policy is what the service decides about snapshots; the store only applies it. nowMs is the time of the write, a new automatic snapshot
// is taken when the newest one is older than gapMs, and at most keep snapshots stay per book.
type policy struct {
	nowMs, gapMs int64
	keep         int
}

// written is what a write did besides the document: the version it issued, the kind of snapshot it stored ("" = none) and how many old
// snapshots it pruned.
type written struct {
	version int64
	snap    string
	pruned  int64
}

// save writes layout for the book if base is the book's current version (yearbook_tab.layout_version; 0 = never saved, and after a reset
// or a template change the version those left behind) and the book still has template tpl, the one the layout was validated against.
// The document it replaces is stored as a snapshot of the given kind in the same transaction: kindAuto only when the newest snapshot is
// older than pol.gapMs, kindRestore always. It returns the new version, or the current state when base is stale.
//
// Lock order (docs/db-conventions.md, Writes): yearbook_tab row (FOR UPDATE) first, then book_layout_tab and book_layout_version_tab, like
// yearbook modify and remove and the template change. The compare, the snapshot check and the write happen under that lock, so two
// parallel saves cannot both see "no recent snapshot".
func (s *Store) save(ctx context.Context, ownerID uint64, publicID, tpl string, base int64, layout []byte, kind string, pol policy) (*written, *conflict, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.Internal, err, "begin")
	}
	defer func() { _ = tx.Rollback() }()
	var id uint64
	var cur sql.NullString
	var version int64
	err = tx.QueryRowContext(ctx, `SELECT id, template_id, layout_version FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, publicID).Scan(&id, &cur, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, errNotFound
	}
	if err != nil {
		return nil, nil, apperr.Wrap(apperr.Internal, err, "lock book")
	}
	if !cur.Valid || cur.String != tpl {
		return nil, nil, apperr.New(apperr.Conflict, "the book's template changed, reload the editor")
	}
	if base != version {
		_, c, err := s.lost(ctx, tx, id, version)
		return nil, c, err
	}
	w := &written{}
	if w.snap, w.pruned, err = snapshotCurrent(ctx, tx, id, kind, pol); err != nil {
		return nil, nil, err
	}
	version++
	if _, err = tx.ExecContext(ctx, `UPDATE yearbook_tab SET layout_version = ? WHERE id = ?`, version, id); err != nil {
		return nil, nil, apperr.Wrap(apperr.Internal, err, "bump layout version")
	}
	if _, err = tx.ExecContext(ctx,
		`INSERT INTO book_layout_tab (yearbook_id, template_id, layout_json, version, created_at, updated_at) VALUES (?,?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE template_id = VALUES(template_id), layout_json = VALUES(layout_json), version = VALUES(version), updated_at = VALUES(updated_at)`,
		id, tpl, layout, version, pol.nowMs, pol.nowMs); err != nil {
		return nil, nil, apperr.Wrap(apperr.Internal, err, "write book layout")
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, apperr.Wrap(apperr.Internal, err, "commit book layout")
	}
	w.version = version
	return w, nil, nil
}

// lost reads the row that won (nil after a reset or a template change), in the same transaction.
func (s *Store) lost(ctx context.Context, tx *sql.Tx, yearbookID uint64, version int64) (int64, *conflict, error) {
	r := &row{}
	err := tx.QueryRowContext(ctx, `SELECT template_id, layout_json, version, updated_at FROM book_layout_tab WHERE yearbook_id = ?`, yearbookID).
		Scan(&r.templateID, &r.layout, &r.version, &r.updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, &conflict{version: version}, nil
	}
	if err != nil {
		return 0, nil, apperr.Wrap(apperr.Internal, err, "read book layout")
	}
	return 0, &conflict{row: r, version: version}, nil
}

// reset deletes the owner's layout row and moves the version on; deleting nothing is fine. The deleted document is stored as a kindReset
// snapshot first, whatever the age of the newest one. Same lock order as save. It returns the kind stored ("" = no layout) and the number
// of snapshots pruned.
func (s *Store) reset(ctx context.Context, ownerID uint64, publicID string, pol policy) (*written, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "begin")
	}
	defer func() { _ = tx.Rollback() }()
	var id uint64
	err = tx.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, publicID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "lock book")
	}
	w := &written{}
	if w.snap, w.pruned, err = snapshotCurrent(ctx, tx, id, KindReset, pol); err != nil {
		return nil, err
	}
	if err := deleteRow(ctx, tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "commit reset")
	}
	return w, nil
}

// deleteRow removes the layout row and bumps the book's version, so no version issued before comes back (a stale editor's save is a
// conflict). The caller holds the yearbook row lock (reset, template change) or is about to delete the yearbook (the bump is then moot).
func deleteRow(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_layout_tab WHERE yearbook_id = ?`, yearbookID); err != nil {
		return apperr.Wrap(apperr.Internal, err, "delete book layout")
	}
	if _, err := tx.ExecContext(ctx, `UPDATE yearbook_tab SET layout_version = layout_version + 1 WHERE id = ?`, yearbookID); err != nil {
		return apperr.Wrap(apperr.Internal, err, "bump layout version")
	}
	return nil
}
