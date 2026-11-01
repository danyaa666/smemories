package booklayout

import (
	"context"
	"database/sql"
	"errors"

	"github.com/go-sql-driver/mysql"

	"github.com/danyaa666/smemories/internal/apperr"
)

const mysqlDuplicateEntry = 1062

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
		`SELECT y.id, y.template_id, l.template_id, l.layout_json, l.version, l.updated_at
		 FROM yearbook_tab y LEFT JOIN book_layout_tab l ON l.yearbook_id = y.id
		 WHERE y.owner_id = ? AND y.public_id = ?`, ownerID, publicID).Scan(&st.yearbookID, &tpl, &lTpl, &layout, &version, &at)
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

// conflict is a save that lost: the stored row (nil when there is none) is what the caller must start from.
type conflict struct{ row *row }

// save writes layout for the book if the stored version is base (0 = no row yet) and the book still has template tpl, the one the
// layout was validated against. It returns the new version, or the stored row when base is stale.
//
// Lock order (docs/db-conventions.md, Writes): yearbook_tab row (FOR UPDATE) first, then book_layout_tab, like yearbook modify and remove
// and the template change. The write itself is one conditional statement, so it does not rely on the lock for correctness.
func (s *Store) save(ctx context.Context, ownerID uint64, publicID, tpl string, base int64, layout []byte, nowMs int64) (int64, *conflict, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, apperr.Wrap(apperr.Internal, err, "begin")
	}
	defer func() { _ = tx.Rollback() }()
	var id uint64
	var cur sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, template_id FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, publicID).Scan(&id, &cur)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, errNotFound
	}
	if err != nil {
		return 0, nil, apperr.Wrap(apperr.Internal, err, "lock book")
	}
	if !cur.Valid || cur.String != tpl {
		return 0, nil, apperr.New(apperr.Conflict, "the book's template changed, reload the editor")
	}
	version := int64(1)
	if base == 0 {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO book_layout_tab (yearbook_id, template_id, layout_json, version, created_at, updated_at) VALUES (?,?,?,1,?,?)`,
			id, tpl, layout, nowMs, nowMs)
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == mysqlDuplicateEntry {
			return s.lost(ctx, tx, id)
		}
		if err != nil {
			return 0, nil, apperr.Wrap(apperr.Internal, err, "insert book layout")
		}
	} else {
		res, err := tx.ExecContext(ctx,
			`UPDATE book_layout_tab SET layout_json = ?, version = version + 1, updated_at = ? WHERE yearbook_id = ? AND version = ?`,
			layout, nowMs, id, base)
		if err != nil {
			return 0, nil, apperr.Wrap(apperr.Internal, err, "update book layout")
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return s.lost(ctx, tx, id)
		}
		version = base + 1
	}
	if err := tx.Commit(); err != nil {
		return 0, nil, apperr.Wrap(apperr.Internal, err, "commit book layout")
	}
	return version, nil, nil
}

// lost reads the row that won, in the same transaction.
func (s *Store) lost(ctx context.Context, tx *sql.Tx, yearbookID uint64) (int64, *conflict, error) {
	r := &row{}
	err := tx.QueryRowContext(ctx, `SELECT template_id, layout_json, version, updated_at FROM book_layout_tab WHERE yearbook_id = ?`, yearbookID).
		Scan(&r.templateID, &r.layout, &r.version, &r.updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, &conflict{}, nil // base was above 0 but the row is gone (reset, template change)
	}
	if err != nil {
		return 0, nil, apperr.Wrap(apperr.Internal, err, "read book layout")
	}
	return 0, &conflict{row: r}, nil
}

// reset deletes the owner's layout row; deleting nothing is fine. Same lock order as save.
func (s *Store) reset(ctx context.Context, ownerID uint64, publicID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return apperr.Wrap(apperr.Internal, err, "begin")
	}
	defer func() { _ = tx.Rollback() }()
	var id uint64
	err = tx.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, publicID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return errNotFound
	}
	if err != nil {
		return apperr.Wrap(apperr.Internal, err, "lock book")
	}
	if err := deleteRow(ctx, tx, id); err != nil {
		return err
	}
	return apperr.WithCode(apperr.Internal, tx.Commit())
}

func deleteRow(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_layout_tab WHERE yearbook_id = ?`, yearbookID); err != nil {
		return apperr.Wrap(apperr.Internal, err, "delete book layout")
	}
	return nil
}
