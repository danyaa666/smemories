package notes

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
)

// marks returns "?,?,?" for n placeholders and the ids as query arguments.
func marks(n int) string { return "?" + strings.Repeat(",?", n-1) }

// withIn puts n placeholders where the query says (IN).
func withIn(query string, n int) string { return strings.Replace(query, "(IN)", "("+marks(n)+")", 1) }

func anys[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// lockOwnedBook takes the exclusive lock on the owner's yearbook row: the first lock of every moderation transaction
// (docs/db-conventions.md, Writes), so moderations of one book queue instead of deadlocking and sort_order stays dense.
// ErrNotFound for an unknown book and for another user's book alike.
func lockOwnedBook(ctx context.Context, tx *sql.Tx, ownerID uint64, bookID string) (uint64, error) {
	var id uint64
	err := tx.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE owner_id = ? AND public_id = ? FOR UPDATE`, ownerID, bookID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// ownedBook is the id of the owner's book without locking it (reads).
func (s *Store) ownedBook(ctx context.Context, ownerID uint64, bookID string) (uint64, error) {
	var id uint64
	err := s.db.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE owner_id = ? AND public_id = ?`, ownerID, bookID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrNotFound
	}
	return id, err
}

// countsOf reads the counts of a whole book; an unknown status value in the table is an internal error, never a default.
func countsOf(ctx context.Context, q queryer, bookRow uint64) (*Counts, error) {
	rows, err := q.QueryContext(ctx, `SELECT status, COUNT(*) FROM note_tab WHERE yearbook_id = ? GROUP BY status`, bookRow)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	c := &Counts{}
	for rows.Next() {
		var st string
		var n int
		if err := rows.Scan(&st, &n); err != nil {
			return nil, err
		}
		switch st {
		case StatusPending:
			c.Pending = n
		case StatusApproved:
			c.Approved = n
		case StatusHidden:
			c.Hidden = n
		default:
			return nil, apperr.New(apperr.Internal, "note with unknown status in the table")
		}
	}
	c.All = c.Pending + c.Approved + c.Hidden
	return c, rows.Err()
}

// listNotes reads one page. The approved list is in book order (the idx_yearbook_id_status_sort_order index); every other list
// is newest first. Photos and link labels are read with one query each for the whole page (no N+1).
func (s *Store) listNotes(ctx context.Context, ownerID uint64, bookID, status string, after *notePos, limit int) (*NoteList, *notePos, error) {
	book, err := s.ownedBook(ctx, ownerID, bookID)
	if err != nil {
		return nil, nil, err
	}
	q := `SELECT id, public_id, collection_id, answers, status, sort_order, created_at FROM note_tab WHERE yearbook_id = ?`
	args := []any{book}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	switch {
	case status == StatusApproved && after != nil:
		q += ` AND sort_order > ?`
		args = append(args, after.sortOrder)
	case after != nil:
		q += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, after.createdAt, after.createdAt, after.rowID)
	}
	if status == StatusApproved {
		q += ` ORDER BY sort_order, id`
	} else {
		q += ` ORDER BY created_at DESC, id DESC`
	}
	q += ` LIMIT ?`
	args = append(args, limit+1)

	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	items := []*NoteItem{}
	for rows.Next() {
		n := &NoteItem{Photos: []PhotoRef{}}
		var answers []byte
		var order sql.NullInt64
		if err := rows.Scan(&n.rowID, &n.ID, &n.coll, &answers, &n.Status, &order, &n.CreatedAt); err != nil {
			return nil, nil, err
		}
		if !validStatus(n.Status) {
			return nil, nil, apperr.New(apperr.Internal, "note with unknown status in the table")
		}
		n.Answers = answers
		if n.Status == StatusApproved && order.Valid {
			p := int(order.Int64)
			n.Position = &p
		}
		items = append(items, n)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	var next *notePos
	if len(items) > limit {
		items = items[:limit]
		last := items[limit-1]
		next = &notePos{createdAt: last.CreatedAt, rowID: last.rowID}
		if last.Position != nil {
			next.sortOrder = *last.Position
		}
	}
	if err := s.fillNotes(ctx, items); err != nil {
		return nil, nil, err
	}
	counts, err := countsOf(ctx, s.db, book)
	if err != nil {
		return nil, nil, err
	}
	return &NoteList{Items: items, Counts: *counts}, next, nil
}

// fillNotes adds the photo ids and the link labels with two queries for the page. The photo query joins media_tab: 1:1 on
// the primary key and scoped by the page's note ids, to read the public id.
func (s *Store) fillNotes(ctx context.Context, items []*NoteItem) error {
	if len(items) == 0 {
		return nil
	}
	byRow := make(map[uint64]*NoteItem, len(items))
	noteIDs := make([]uint64, 0, len(items))
	colls := map[uint64]struct{}{}
	for _, n := range items {
		byRow[n.rowID] = n
		noteIDs = append(noteIDs, n.rowID)
		colls[n.coll] = struct{}{}
	}
	rows, err := s.db.QueryContext(ctx,
		withIn(`SELECT np.note_id, m.public_id FROM note_photo_tab np JOIN media_tab m ON m.id = np.media_id
		 WHERE np.note_id IN (IN) ORDER BY np.note_id, np.sort_order`, len(noteIDs)), anys(noteIDs)...)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var note uint64
		var photo string
		if err := rows.Scan(&note, &photo); err != nil {
			return err
		}
		byRow[note].Photos = append(byRow[note].Photos, PhotoRef{ID: photo})
	}
	if err := rows.Err(); err != nil {
		return err
	}
	collIDs := make([]uint64, 0, len(colls))
	for id := range colls {
		collIDs = append(collIDs, id)
	}
	lrows, err := s.db.QueryContext(ctx, withIn(`SELECT id, label FROM note_collection_tab WHERE id IN (IN)`, len(collIDs)), anys(collIDs)...)
	if err != nil {
		return err
	}
	defer func() { _ = lrows.Close() }()
	labels := make(map[uint64]string, len(collIDs))
	for lrows.Next() {
		var id uint64
		var label string
		if err := lrows.Scan(&id, &label); err != nil {
			return err
		}
		labels[id] = label
	}
	if err := lrows.Err(); err != nil {
		return err
	}
	for _, n := range items {
		n.LinkLabel = labels[n.coll]
	}
	return nil
}

// noteRow is a locked note.
type noteRow struct {
	id     uint64
	status string
	order  sql.NullInt64
}

// lockNotes locks the book's notes with these public ids in id order and returns them by public id. Any id that is not in
// the book (also another user's) makes the whole call ErrNotFound.
func lockNotes(ctx context.Context, tx *sql.Tx, book uint64, ids []string) (map[string]*noteRow, error) {
	rows, err := tx.QueryContext(ctx,
		withIn(`SELECT id, public_id, status, sort_order FROM note_tab WHERE yearbook_id = ? AND public_id IN (IN) ORDER BY id FOR UPDATE`, len(ids)),
		append([]any{book}, anys(ids)...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	out := make(map[string]*noteRow, len(ids))
	for rows.Next() {
		r := &noteRow{}
		var pub string
		if err := rows.Scan(&r.id, &pub, &r.status, &r.order); err != nil {
			return nil, err
		}
		if !validStatus(r.status) {
			return nil, apperr.New(apperr.Internal, "note with unknown status in the table")
		}
		out[pub] = r
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// The collation is case-insensitive: a row can match under a spelling the map lacks. Never hand callers a hole.
	for _, id := range ids {
		if out[id] == nil {
			return nil, ErrNotFound
		}
	}
	return out, nil
}

type placed struct {
	id  uint64
	pos int
}

// place sets sort_order (and the status) of the given notes in one statement.
func place(ctx context.Context, tx *sql.Tx, status string, now int64, ps []placed) error {
	if len(ps) == 0 {
		return nil
	}
	q := withIn(`UPDATE note_tab SET status = ?, updated_at = ?, sort_order = CASE id`+strings.Repeat(` WHEN ? THEN ?`, len(ps))+` END WHERE id IN (IN)`, len(ps))
	args := []any{status, now}
	for _, p := range ps {
		args = append(args, p.id, p.pos)
	}
	for _, p := range ps {
		args = append(args, p.id)
	}
	_, err := tx.ExecContext(ctx, q, args...)
	return err
}

// renumber makes the approved notes of the book 1..n again, keeping their order (after a note left the list).
func renumber(ctx context.Context, tx *sql.Tx, book uint64) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE note_tab t JOIN (SELECT id, ROW_NUMBER() OVER (ORDER BY sort_order, id) AS rn FROM note_tab WHERE yearbook_id = ? AND status = 'approved') r ON r.id = t.id
		 SET t.sort_order = r.rn WHERE t.sort_order IS NULL OR t.sort_order <> r.rn`, book)
	return err
}

// setStatus changes the status of the notes in one transaction.
//
// Lock order (docs/db-conventions.md, Writes): the yearbook_tab row FOR UPDATE first, then the note rows in id order. Every
// moderation write (reorder, remove) and the book delete take the book first, so they queue and never deadlock; the public
// submit takes the book FOR SHARE, so it waits for a moderation in flight. Approving appends to the end of the order in the
// order given (a note that is already approved keeps its place); leaving the approved set closes the gap.
func (s *Store) setStatus(ctx context.Context, ownerID uint64, bookID string, ids []string, status string, now int64) (*Counts, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	book, err := lockOwnedBook(ctx, tx, ownerID, bookID)
	if err != nil {
		return nil, err
	}
	notes, err := lockNotes(ctx, tx, book, ids)
	if err != nil {
		return nil, err
	}
	if status == StatusApproved {
		var last sql.NullInt64
		if err := tx.QueryRowContext(ctx, `SELECT MAX(sort_order) FROM note_tab WHERE yearbook_id = ? AND status = 'approved'`, book).Scan(&last); err != nil {
			return nil, err
		}
		var ps []placed
		next := int(last.Int64)
		for _, id := range ids {
			if n := notes[id]; n.status != StatusApproved {
				next++
				ps = append(ps, placed{n.id, next})
			}
		}
		if err := place(ctx, tx, status, now, ps); err != nil {
			return nil, err
		}
	} else {
		var changed []any
		wasApproved := false
		for _, id := range ids {
			if n := notes[id]; n.status != status {
				changed = append(changed, n.id)
				wasApproved = wasApproved || n.status == StatusApproved
			}
		}
		if len(changed) > 0 {
			if _, err := tx.ExecContext(ctx, withIn(`UPDATE note_tab SET status = ?, sort_order = NULL, updated_at = ? WHERE id IN (IN)`, len(changed)),
				append([]any{status, now}, changed...)...); err != nil {
				return nil, err
			}
		}
		if wasApproved {
			if err := renumber(ctx, tx, book); err != nil {
				return nil, err
			}
		}
	}
	counts, err := countsOf(ctx, tx, book)
	if err != nil {
		return nil, err
	}
	return counts, tx.Commit()
}

// reorder applies the order of ids to the approved notes; ids must be exactly the approved set, else ErrOrderStale.
// Lock order as setStatus.
func (s *Store) reorder(ctx context.Context, ownerID uint64, bookID string, ids []string, now int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	book, err := lockOwnedBook(ctx, tx, ownerID, bookID)
	if err != nil {
		return err
	}
	rows, err := tx.QueryContext(ctx, `SELECT id, public_id, sort_order FROM note_tab WHERE yearbook_id = ? AND status = 'approved' ORDER BY id FOR UPDATE`, book)
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	approved := map[string]*noteRow{}
	for rows.Next() {
		r := &noteRow{}
		var pub string
		if err := rows.Scan(&r.id, &pub, &r.order); err != nil {
			return err
		}
		approved[pub] = r
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(approved) != len(ids) {
		return errOrderStale
	}
	var ps []placed
	for i, id := range ids {
		r, ok := approved[id]
		if !ok {
			return errOrderStale
		}
		if !r.order.Valid || int(r.order.Int64) != i+1 {
			ps = append(ps, placed{r.id, i + 1})
		}
	}
	if err := place(ctx, tx, StatusApproved, now, ps); err != nil {
		return err
	}
	return tx.Commit()
}

// photoRows checks that every id is a note of the owner's book (else ErrNotFound) and returns the internal ids of their photos.
// It takes no lock: the delete re-checks under the book lock.
func (s *Store) photoRows(ctx context.Context, ownerID uint64, bookID string, ids []string) ([]int64, error) {
	book, err := s.ownedBook(ctx, ownerID, bookID)
	if err != nil {
		return nil, err
	}
	noteRows, err := noteRowIDs(ctx, s.db, book, ids)
	if err != nil {
		return nil, err
	}
	return mediaOf(ctx, s.db, noteRows)
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func noteRowIDs(ctx context.Context, q queryer, book uint64, ids []string) ([]uint64, error) {
	rows, err := q.QueryContext(ctx, withIn(`SELECT id FROM note_tab WHERE yearbook_id = ? AND public_id IN (IN)`, len(ids)), append([]any{book}, anys(ids)...)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []uint64
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(ids) {
		return nil, ErrNotFound
	}
	return out, nil
}

func mediaOf(ctx context.Context, q queryer, noteRows []uint64) ([]int64, error) {
	rows, err := q.QueryContext(ctx, withIn(`SELECT media_id FROM note_photo_tab WHERE note_id IN (IN)`, len(noteRows)), anys(noteRows)...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// removeNotes deletes the notes, their photo links and (through deleteRows) their photo rows in one transaction; the objects
// were removed before. Lock order as setStatus: the book row first, then the notes, then the media rows.
func (s *Store) removeNotes(ctx context.Context, ownerID uint64, bookID string, ids []string, deleteRows func(ctx context.Context, tx *sql.Tx, rowIDs []int64) error) (*Counts, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	book, err := lockOwnedBook(ctx, tx, ownerID, bookID)
	if err != nil {
		return nil, err
	}
	locked, err := lockNotes(ctx, tx, book, ids)
	if err != nil {
		return nil, err
	}
	noteRows := make([]uint64, 0, len(locked))
	for _, n := range locked {
		noteRows = append(noteRows, n.id)
	}
	photos, err := mediaOf(ctx, tx, noteRows)
	if err != nil {
		return nil, err
	}
	if err := deleteRows(ctx, tx, photos); err != nil { // also clears the note_photo_tab links of those photos
		return nil, err
	}
	for _, q := range []string{
		`DELETE FROM note_photo_tab WHERE note_id IN (IN)`,
		`DELETE FROM note_tab WHERE id IN (IN)`,
	} {
		if _, err := tx.ExecContext(ctx, withIn(q, len(noteRows)), anys(noteRows)...); err != nil {
			return nil, err
		}
	}
	if err := renumber(ctx, tx, book); err != nil {
		return nil, err
	}
	counts, err := countsOf(ctx, tx, book)
	if err != nil {
		return nil, err
	}
	return counts, tx.Commit()
}

var _ modStore = (*Store)(nil)
