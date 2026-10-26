package class

import (
	"context"
	"database/sql"
	"errors"
)

// The student's page (T-126): which of the member's own yearbooks is their page in the class book. The link is a pointer
// (class_member_tab.yearbook_id) and nothing else: it gives nobody a right over the yearbook. Only the member can set or clear
// it, and only to a yearbook they own (the same owner_id = ? check as everywhere); a yearbook is the page of at most one class.

// noteApproved is note_tab.status of a note the owner approved for the book (internal/notes.StatusApproved).
const noteApproved = "approved"

// PageStatus is what a member sees about their own page. The roster shows an admin only the flags and the count, never the ids.
type PageStatus struct {
	Linked          bool    `json:"linked"`
	YearbookID      *string `json:"yearbook_id"`      // public id of the member's own yearbook, null when not linked
	ProfileComplete bool    `json:"profile_complete"` // the owner profile has a name and a photo
	ApprovedNotes   int     `json:"approved_notes"`   // notes the owner approved in that yearbook
}

// profileDone is the SQL for "profile_complete": the owner profile of the book has a name and a photo.
const profileDone = `is_owner = 1 AND full_name <> '' AND photo_media_id IS NOT NULL`

// pageStatus reads the caller's own page status in the class classID (internal id). A link to a yearbook the member does not own
// (it cannot happen through the API) counts as not linked.
func (s *Store) pageStatus(ctx context.Context, userID, classID uint64) (*PageStatus, error) {
	st := new(PageStatus)
	var bookID uint64
	var public string
	err := s.db.QueryRowContext(ctx,
		`SELECT y.id, y.public_id FROM class_member_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id AND y.owner_id = m.user_id WHERE m.class_id = ? AND m.user_id = ?`,
		classID, userID).Scan(&bookID, &public)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return nil, dbErr(err, "read class page")
	}
	st.Linked, st.YearbookID = true, &public
	if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM profile_tab WHERE yearbook_id = ? AND `+profileDone+`)`, bookID).Scan(&st.ProfileComplete); err != nil {
		return nil, dbErr(err, "read profile")
	}
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM note_tab WHERE yearbook_id = ? AND status = ?`, bookID, noteApproved).Scan(&st.ApprovedNotes)
	return st, dbErr(err, "count approved notes")
}

// pageFlags fills ProfileComplete and ApprovedNotes of the roster rows whose yearbook (m.book, owned by the member) is set:
// two grouped queries for the whole page, whatever its size.
func (s *Store) pageFlags(ctx context.Context, ms []Member) error {
	var ids []any
	at := map[uint64][]int{}
	for i, m := range ms {
		if m.book.Valid {
			ids = append(ids, m.book.V)
			at[m.book.V] = append(at[m.book.V], i)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	in := inList(len(ids))
	rows, err := s.db.QueryContext(ctx, `SELECT yearbook_id FROM profile_tab WHERE yearbook_id IN (`+in+`) AND `+profileDone, ids...) //nolint:gosec // G202: inList emits only "?" placeholders
	if err != nil {
		return dbErr(err, "read profiles")
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var id uint64
		if err := rows.Scan(&id); err != nil {
			return dbErr(err, "scan profile")
		}
		for _, i := range at[id] {
			ms[i].ProfileComplete = true
		}
	}
	if err := rows.Err(); err != nil {
		return dbErr(err, "read profiles")
	}
	_ = rows.Close()
	nrows, err := s.db.QueryContext(ctx, `SELECT yearbook_id, COUNT(*) FROM note_tab WHERE yearbook_id IN (`+in+`) AND status = ? GROUP BY yearbook_id`, append(ids, noteApproved)...) //nolint:gosec // G202: as above
	if err != nil {
		return dbErr(err, "count approved notes")
	}
	defer func() { _ = nrows.Close() }()
	for nrows.Next() {
		var id uint64
		var n int
		if err := nrows.Scan(&id, &n); err != nil {
			return dbErr(err, "scan note count")
		}
		for _, i := range at[id] {
			ms[i].ApprovedNotes = n
		}
	}
	return dbErr(nrows.Err(), "count approved notes")
}

// setBook links the caller's own membership of the class to the yearbook bookID (public id), or clears the link when it is nil.
//
// Lock order: the yearbook_tab row first, then the class row, then the member row. A yearbook delete takes the yearbook row and
// then clears the member row (clearBook), so this transaction must take them in the same order; the rest of the class code takes
// the class row and then member rows and never a yearbook, so there is no cycle. Under the yearbook lock the "already the page of
// another class" check cannot race another link of the same yearbook (only its owner can link it, and that takes this lock);
// the unique key on yearbook_id stays as the backstop. A delete that wins the lock leaves nothing to link (404), one that
// loses clears the link it just saw.
func (s *Store) setBook(ctx context.Context, userID uint64, classPublicID string, bookPublicID *string, nowMs int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		var book any // NULL unlinks
		if bookPublicID != nil {
			var id uint64
			err := tx.QueryRowContext(ctx, `SELECT id FROM yearbook_tab WHERE public_id = ? AND owner_id = ? FOR UPDATE`, *bookPublicID, userID).Scan(&id)
			if errors.Is(err, sql.ErrNoRows) {
				return ErrYearbookNotFound
			}
			if err != nil {
				return dbErr(err, "lock yearbook")
			}
			book = id
		}
		classID, _, err := lockMembership(ctx, tx, userID, classPublicID, RoleStudent)
		if err != nil {
			return err
		}
		if book != nil {
			var other uint64
			err := tx.QueryRowContext(ctx, `SELECT id FROM class_member_tab WHERE yearbook_id = ? AND NOT (class_id = ? AND user_id = ?) FOR UPDATE`, book, classID, userID).Scan(&other)
			if err == nil {
				return ErrBookLinked
			}
			if !errors.Is(err, sql.ErrNoRows) {
				return dbErr(err, "check yearbook link")
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE class_member_tab SET yearbook_id = ?, updated_at = ? WHERE class_id = ? AND user_id = ?`, book, nowMs, classID, userID)
		return dbErr(err, "link yearbook")
	})
}

// SetBook is class-member/set-book: the caller (any member) links their own membership to a yearbook they own, replaces the
// link, or clears it with a nil yearbook. It returns the new page status. A non-member gets 404 before anything else.
func (s *Service) SetBook(ctx context.Context, userID uint64, classID string, yearbookID *string) (*PageStatus, error) {
	c, err := s.Authorize(ctx, userID, classID, RoleStudent)
	if err != nil {
		return nil, err
	}
	if err := s.throttle(ctx, userID); err != nil {
		return nil, err
	}
	if err := s.store.setBook(ctx, userID, classID, yearbookID, s.now().UnixMilli()); err != nil {
		return nil, err
	}
	return s.store.pageStatus(ctx, userID, c.internalID)
}
