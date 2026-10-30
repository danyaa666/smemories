package bookdesign

import (
	"context"
	"database/sql"
	"errors"

	"github.com/danyaa666/smemories/internal/apperr"
)

// errNotFound is a missing book and another user's book alike.
var errNotFound = apperr.New(apperr.NotFound, "yearbook not found")

// Store persists the design rows. Every query is scoped by yearbook_tab.owner_id.
type Store struct{ db *sql.DB }

// NewStore builds the store.
func NewStore(db *sql.DB) *Store { return &Store{db: db} }

// state is the owner's book and its design row, if any.
type state struct {
	templateID *string // the book's template
	row        *row
}

type row struct {
	templateID string
	options    []byte
	updatedAt  int64
}

// get reads the owner's book and its design row. A 1:1 owner-scoped join, so it is one query.
func (s *Store) get(ctx context.Context, ownerID uint64, publicID string) (*state, error) {
	var st state
	var tpl, dTpl sql.NullString
	var opts []byte
	var at sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT y.template_id, d.template_id, d.options_json, d.updated_at
		 FROM yearbook_tab y LEFT JOIN book_design_tab d ON d.yearbook_id = y.id
		 WHERE y.owner_id = ? AND y.public_id = ?`, ownerID, publicID).Scan(&tpl, &dTpl, &opts, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "read book design")
	}
	if tpl.Valid {
		st.templateID = &tpl.String
	}
	if dTpl.Valid {
		st.row = &row{templateID: dTpl.String, options: opts, updatedAt: at.Int64}
	}
	return &st, nil
}

// put locks the owner's book, lets build validate the options against the book's template (read under the lock, so a
// concurrent template change cannot slip in between) and replaces the row.
//
// Lock order (docs/db-conventions.md, Writes): yearbook_tab row (FOR UPDATE) first, then book_design_tab, like yearbook
// modify and remove.
func (s *Store) put(ctx context.Context, ownerID uint64, publicID string, nowMs int64, build func(templateID *string) ([]byte, error)) (*state, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "begin")
	}
	defer func() { _ = tx.Rollback() }()
	id, tpl, err := lockBook(ctx, tx, ownerID, publicID)
	if err != nil {
		return nil, err
	}
	opts, err := build(tpl)
	if err != nil {
		return nil, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO book_design_tab (yearbook_id, template_id, options_json, created_at, updated_at) VALUES (?,?,?,?,?)
		 ON DUPLICATE KEY UPDATE template_id = VALUES(template_id), options_json = VALUES(options_json), updated_at = VALUES(updated_at)`,
		id, *tpl, opts, nowMs, nowMs); err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "write book design")
	}
	if err := tx.Commit(); err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "commit book design")
	}
	return &state{templateID: tpl, row: &row{templateID: *tpl, options: opts, updatedAt: nowMs}}, nil
}

// reset deletes the owner's design row; deleting nothing is fine. Same lock order as put.
func (s *Store) reset(ctx context.Context, ownerID uint64, publicID string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return apperr.Wrap(apperr.Internal, err, "begin")
	}
	defer func() { _ = tx.Rollback() }()
	id, _, err := lockBook(ctx, tx, ownerID, publicID)
	if err != nil {
		return err
	}
	if err := deleteRow(ctx, tx, id); err != nil {
		return err
	}
	return apperr.WithCode(apperr.Internal, tx.Commit())
}

func lockBook(ctx context.Context, tx *sql.Tx, ownerID uint64, publicID string) (id uint64, tpl *string, err error) {
	var t sql.NullString
	err = tx.QueryRowContext(ctx, `SELECT id, template_id FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, publicID).Scan(&id, &t)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, errNotFound
	}
	if err != nil {
		return 0, nil, apperr.Wrap(apperr.Internal, err, "lock book")
	}
	if t.Valid {
		tpl = &t.String
	}
	return id, tpl, nil
}

func deleteRow(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM book_design_tab WHERE yearbook_id = ?`, yearbookID); err != nil {
		return apperr.Wrap(apperr.Internal, err, "delete book design")
	}
	return nil
}
