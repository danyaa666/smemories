package class

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

// Lock order of the class page writes: the class_tab row FOR UPDATE, the caller's member row (lockMembership), then the page
// (and, for the order, member) rows. It is the order of every other class write, so they queue and never deadlock. The page cap
// and the next position are read under that lock with locking reads.

const pageCols = `public_id, kind, title, body, placement, created_at, updated_at`

func scanPage(sc scanner) (*Page, error) {
	p := new(Page)
	err := sc.Scan(&p.ID, &p.Kind, &p.Title, &p.Body, &p.Placement, &p.CreatedAt, &p.UpdatedAt)
	return p, err
}

// pages returns the pages of the class (internal id): front first, then back, each by stored order.
func (s *Store) pages(ctx context.Context, classID uint64) ([]Page, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+pageCols+` FROM class_page_tab WHERE class_id = ? ORDER BY placement = ?, sort_order, id LIMIT ?`,
		classID, PlacementBack, MaxPages)
	if err != nil {
		return nil, dbErr(err, "list pages")
	}
	defer func() { _ = rows.Close() }()
	out := []Page{}
	for rows.Next() {
		p, err := scanPage(rows)
		if err != nil {
			return nil, dbErr(err, "scan page")
		}
		out = append(out, *p)
	}
	return out, dbErr(rows.Err(), "list pages")
}

// lastPosition is the highest sort_order among the class's pages (0 when none), read under the class lock.
func lastPosition(ctx context.Context, tx *sql.Tx, classID uint64) (n int, last int, err error) {
	err = tx.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(MAX(sort_order), 0) FROM class_page_tab WHERE class_id = ? FOR UPDATE`, classID).Scan(&n, &last)
	return n, last, dbErr(err, "count pages")
}

// createPage inserts p at the end of the class's pages (admin only; ErrPagesFull at MaxPages).
func (s *Store) createPage(ctx context.Context, userID uint64, classPublicID string, p *Page) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, classPublicID, RoleAdmin)
		if err != nil {
			return err
		}
		n, last, err := lastPosition(ctx, tx, classID)
		if err != nil {
			return err
		}
		if n >= MaxPages {
			return ErrPagesFull
		}
		_, err = tx.ExecContext(ctx,
			`INSERT INTO class_page_tab (public_id, class_id, kind, title, body, placement, sort_order, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			p.ID, classID, p.Kind, p.Title, p.Body, p.Placement, last+1, p.CreatedAt, p.UpdatedAt)
		return dbErr(err, "insert page")
	})
}

// updatePage replaces the fields of the page pageID of the class (admin only). The page is looked up inside the class: an id of
// another class is ErrPageNotFound.
func (s *Store) updatePage(ctx context.Context, userID uint64, classPublicID, pageID string, in *PageInput, nowMs int64) (*Page, error) {
	var out *Page
	err := s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, classPublicID, RoleAdmin)
		if err != nil {
			return err
		}
		var row uint64
		var placement string
		var order int
		var created int64
		err = tx.QueryRowContext(ctx, `SELECT id, placement, sort_order, created_at FROM class_page_tab WHERE class_id = ? AND public_id = ? FOR UPDATE`, classID, pageID).
			Scan(&row, &placement, &order, &created)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrPageNotFound
		}
		if err != nil {
			return dbErr(err, "lock page")
		}
		if placement != in.Placement { // moved to the other side: last there
			_, last, err := lastPosition(ctx, tx, classID)
			if err != nil {
				return err
			}
			order = last + 1
		}
		if _, err = tx.ExecContext(ctx, `UPDATE class_page_tab SET kind = ?, title = ?, body = ?, placement = ?, sort_order = ?, updated_at = ? WHERE id = ?`,
			in.Kind, in.Title, in.Body, in.Placement, order, nowMs, row); err != nil {
			return dbErr(err, "update page")
		}
		out = &Page{ID: pageID, Kind: in.Kind, Title: in.Title, Body: in.Body, Placement: in.Placement, CreatedAt: created, UpdatedAt: nowMs}
		return nil
	})
	return out, err
}

// deletePage removes the page pageID of the class (admin only).
func (s *Store) deletePage(ctx context.Context, userID uint64, classPublicID, pageID string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, classPublicID, RoleAdmin)
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM class_page_tab WHERE class_id = ? AND public_id = ?`, classID, pageID)
		if err != nil {
			return dbErr(err, "delete page")
		}
		n, err := res.RowsAffected()
		if err != nil {
			return dbErr(err, "delete page")
		}
		if n == 0 {
			return ErrPageNotFound
		}
		return nil
	})
}

// idRow is a public id with its row id and one more column (the placement of a page).
type idRow struct {
	public, extra string
	id            uint64
}

// lockIDs reads and locks the rows of `SELECT public_id, id, <extra> FROM ...` for the class.
func lockIDs(ctx context.Context, tx *sql.Tx, q string, classID uint64, what string) ([]idRow, error) {
	rows, err := tx.QueryContext(ctx, q, classID)
	if err != nil {
		return nil, dbErr(err, what)
	}
	defer func() { _ = rows.Close() }()
	var out []idRow
	for rows.Next() {
		var r idRow
		if err := rows.Scan(&r.public, &r.id, &r.extra); err != nil {
			return nil, dbErr(err, what)
		}
		out = append(out, r)
	}
	return out, dbErr(rows.Err(), what)
}

// byPlacement maps the public ids of the rows whose extra column is `only` to their row ids ("" keeps every row).
func byPlacement(rs []idRow, only string) map[string]uint64 {
	out := make(map[string]uint64, len(rs))
	for _, r := range rs {
		if only == "" || r.extra == only {
			out[r.public] = r.id
		}
	}
	return out
}

// permutation turns ids into row ids when ids are exactly the keys of have (each once: the service already refused repeats),
// else ok is false.
func permutation(ids []string, have map[string]uint64) (rows []uint64, ok bool) {
	if len(ids) != len(have) {
		return nil, false
	}
	rows = make([]uint64, len(ids))
	for i, id := range ids {
		row, found := have[id]
		if !found {
			return nil, false
		}
		rows[i] = row
	}
	return rows, true
}

// setOrder gives the rows of table (a constant of this file) the positions 1..n in the order of rows, in one statement.
func setOrder(ctx context.Context, tx *sql.Tx, table string, rows []uint64, nowMs int64) error {
	if len(rows) == 0 {
		return nil
	}
	var q strings.Builder
	q.WriteString("UPDATE " + table + " SET sort_order = CASE id")
	args := make([]any, 0, 3*len(rows)+1)
	for i, r := range rows {
		q.WriteString(" WHEN ? THEN ?")
		args = append(args, r, i+1)
	}
	q.WriteString(" END, updated_at = ? WHERE id IN (" + inList(len(rows)) + ")")
	args = append(args, nowMs)
	for _, r := range rows {
		args = append(args, r)
	}
	_, err := tx.ExecContext(ctx, q.String(), args...) //nolint:gosec // G202: table is a constant of this package and the rest are "?" placeholders
	return dbErr(err, "set order")
}

// updateOrder applies the book order (admin only). The members and pages are read under the class lock and compared with the
// lists: a missing, extra, repeated or foreign id is a 400 for that list and nothing is written.
func (s *Store) updateOrder(ctx context.Context, userID uint64, classPublicID string, in *OrderInput, nowMs int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, classPublicID, RoleAdmin)
		if err != nil {
			return err
		}
		ms, err := lockIDs(ctx, tx, `SELECT public_id, id, role FROM class_member_tab WHERE class_id = ? ORDER BY id FOR UPDATE`, classID, "lock members")
		if err != nil {
			return err
		}
		ps, err := lockIDs(ctx, tx, `SELECT public_id, id, placement FROM class_page_tab WHERE class_id = ? ORDER BY id FOR UPDATE`, classID, "lock pages")
		if err != nil {
			return err
		}
		memberRows, ok := permutation(in.MemberIDs, byPlacement(ms, ""))
		if !ok {
			return invalid("member_ids")
		}
		frontRows, ok := permutation(in.FrontPageIDs, byPlacement(ps, PlacementFront))
		if !ok {
			return invalid("front_page_ids")
		}
		backRows, ok := permutation(in.BackPageIDs, byPlacement(ps, PlacementBack))
		if !ok {
			return invalid("back_page_ids")
		}
		for _, w := range []struct {
			table string
			rows  []uint64
		}{{"class_member_tab", memberRows}, {"class_page_tab", frontRows}, {"class_page_tab", backRows}} {
			if err := setOrder(ctx, tx, w.table, w.rows, nowMs); err != nil {
				return err
			}
		}
		return nil
	})
}
