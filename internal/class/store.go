package class

import (
	"context"
	"database/sql"
	"errors"
	"strings"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Class is a class as the caller sees it: the class plus the caller's own membership. It never carries the roster.
// The ids of rows are never serialised (L-05); times are Unix milliseconds.
type Class struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	SchoolName     string `json:"school_name"`
	GraduationYear *int   `json:"graduation_year"`
	Language       string `json:"language"`
	MyRole         string `json:"my_role"`     // admin or student
	PageLinked     bool   `json:"page_linked"` // the caller has chosen which of their yearbooks is their class page
	// Page is the caller's own page status, set by class/get only (the list and the other answers leave it out).
	Page       *PageStatus `json:"page,omitempty"`
	CreatedAt  int64       `json:"created_at"` // Unix ms
	UpdatedAt  int64       `json:"updated_at"` // Unix ms
	internalID uint64
	memberRow  uint64 // class_member_tab.id of the caller: the list cursor
}

// Member is a roster row, seen by an admin only.
type Member struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Role       string `json:"role"`
	JoinedAt   int64  `json:"joined_at"`   // Unix ms
	PageLinked bool   `json:"page_linked"` // the member has chosen a class page; nothing about its content
	// Counts and flags about the page, never its text, its notes or its photo ids (T-126). Zero when nothing is linked.
	ProfileComplete bool `json:"profile_complete"`
	ApprovedNotes   int  `json:"approved_notes"`
	row             uint64
	userID          uint64
	book            sql.Null[uint64] // yearbook_tab.id of the page, only when the member owns it; never serialised
}

// Store persists classes and memberships. Every access check is a join on class_member_tab (user id and role) or, inside a
// transaction that changes membership, a locking read of it; nothing is decided in memory from an earlier read.
//
// Lock order of every transaction that changes membership: the class_tab row FOR UPDATE first (this serialises all
// membership changes of one class, which is what keeps "the last admin stays" true under concurrency), then the
// class_member_tab rows. class/create locks the user_tab row first (it creates the class, so there is no class row yet).
// A yearbook delete takes the yearbook_tab row first and then clears class_member_tab.yearbook_id (clearBook); it never
// takes a class lock. setBook (the one transaction that touches both) therefore takes the yearbook row first, then the class
// and member rows; no other class transaction takes a yearbook row, so the orders cannot form a cycle.
//
// Reads that decide a cap or a state are locking reads (FOR UPDATE), never plain ones: a plain read takes the transaction's
// snapshot, which may be older than the lock just won. Caps (60 members, 5 active invites) are counted under the class row lock.
// A promotion to admin (setRole) then also takes the promoted user's user_tab row, so two classes promoting the same user, and
// class/create, serialise there and the 10-admin-classes cap holds; no transaction holding a user row waits for a class lock,
// so there is no cycle. Accepting an invite takes the class row, then the invite row, then the member rows.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

type scanner interface{ Scan(...any) error }

// The join is 1:1 and scoped by the caller's user id; it is the access check itself (DESIGN.md, permission matrix).
const (
	classCols = `c.id, c.public_id, c.name, c.school_name, c.graduation_year, c.language, c.created_at, c.updated_at, m.id, m.role, m.yearbook_id IS NOT NULL`
	classFrom = ` FROM class_member_tab m JOIN class_tab c ON c.id = m.class_id`
)

func scanClass(sc scanner) (*Class, error) {
	c := new(Class)
	var grad sql.Null[int]
	if err := sc.Scan(&c.internalID, &c.ID, &c.Name, &c.SchoolName, &grad, &c.Language, &c.CreatedAt, &c.UpdatedAt, &c.memberRow, &c.MyRole, &c.PageLinked); err != nil {
		return nil, err
	}
	if grad.Valid {
		c.GraduationYear = &grad.V
	}
	return c, nil
}

func dbErr(err error, what string) error {
	if err == nil {
		return nil
	}
	return apperr.Wrap(apperr.Internal, err, what)
}

// access returns the class publicID with the caller's membership, or ErrNotFound when there is no such class or the caller
// is not in it (the same answer, on purpose).
func (s *Store) access(ctx context.Context, userID uint64, publicID string) (*Class, error) {
	c, err := scanClass(s.db.QueryRowContext(ctx, `SELECT `+classCols+classFrom+` WHERE m.user_id = ? AND c.public_id = ?`, userID, publicID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return c, dbErr(err, "read class")
}

// list returns up to limit of the caller's classes, newest membership first, after the membership row `after` (0 = first
// page), and the row to continue from (0 on the last page). "More" is decided by reading one extra row.
func (s *Store) list(ctx context.Context, userID, after uint64, limit int) ([]Class, uint64, error) {
	q, args := `SELECT `+classCols+classFrom+` WHERE m.user_id = ?`, []any{userID}
	if after > 0 {
		q += ` AND m.id < ?`
		args = append(args, after)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY m.id DESC LIMIT ?`, append(args, limit+1)...)
	if err != nil {
		return nil, 0, dbErr(err, "list classes")
	}
	defer func() { _ = rows.Close() }()
	out := make([]Class, 0, limit+1)
	for rows.Next() {
		c, err := scanClass(rows)
		if err != nil {
			return nil, 0, dbErr(err, "scan class")
		}
		out = append(out, *c)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, dbErr(err, "list classes")
	}
	var next uint64
	if len(out) > limit {
		out = out[:limit]
		next = out[limit-1].memberRow
	}
	return out, next, nil
}

// inTx runs fn in a transaction: commit when it returns nil, rollback otherwise.
func (s *Store) inTx(ctx context.Context, fn func(tx *sql.Tx) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return dbErr(err, "begin")
	}
	defer func() { _ = tx.Rollback() }()
	if err := fn(tx); err != nil {
		return err
	}
	return dbErr(tx.Commit(), "commit")
}

// create inserts the class and its first member (the creator, admin). The creator's user_tab row is locked first so two
// concurrent creates cannot both slip under maxAdminClasses.
func (s *Store) create(ctx context.Context, userID uint64, c *Class, memberID string, nowMs int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		if err := capAdminClasses(ctx, tx, userID); err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO class_tab (public_id, name, school_name, graduation_year, language, created_by, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			c.ID, c.Name, c.SchoolName, c.GraduationYear, c.Language, userID, nowMs, nowMs)
		if err != nil {
			return dbErr(err, "insert class")
		}
		classID, err := res.LastInsertId()
		if err != nil {
			return dbErr(err, "insert class")
		}
		res, err = tx.ExecContext(ctx,
			`INSERT INTO class_member_tab (public_id, class_id, user_id, role, joined_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			memberID, classID, userID, RoleAdmin, nowMs, nowMs, nowMs)
		if err != nil {
			return dbErr(err, "insert member")
		}
		row, err := res.LastInsertId()
		if err != nil {
			return dbErr(err, "insert member")
		}
		c.internalID, c.memberRow = uint64(classID), uint64(row) //nolint:gosec // G115: AUTO_INCREMENT ids are positive
		c.MyRole, c.CreatedAt, c.UpdatedAt = RoleAdmin, nowMs, nowMs
		return nil
	})
}

// lockMembership is the first step of every transaction that changes a class: it locks the class row, then the caller's
// own member row, and checks the role. No such class, or the caller is not in it: ErrNotFound; role below min: ErrForbidden.
// It re-checks what Service.Authorize saw, because the caller may have been removed or demoted since.
func lockMembership(ctx context.Context, tx *sql.Tx, userID uint64, publicID, min string) (classID uint64, role string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id FROM class_tab WHERE public_id = ? FOR UPDATE`, publicID).Scan(&classID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	if err != nil {
		return 0, "", dbErr(err, "lock class")
	}
	err = tx.QueryRowContext(ctx, `SELECT role FROM class_member_tab WHERE class_id = ? AND user_id = ? FOR UPDATE`, classID, userID).Scan(&role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrNotFound
	}
	if err != nil {
		return 0, "", dbErr(err, "lock member")
	}
	if !atLeast(role, min) {
		return 0, "", ErrForbidden
	}
	return classID, role, nil
}

// inList is n comma-separated placeholders.
func inList(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

// keepAnAdmin is the last-admin rule: ErrLastAdmin when a member whose role is `role` is about to stop being an admin (leave,
// removal or demotion) and is the only admin. The count is read under the class lock, so two admins leaving at once cannot
// both see "another admin is left".
func keepAnAdmin(ctx context.Context, tx *sql.Tx, classID uint64, role string) error {
	if role != RoleAdmin {
		return nil
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM class_member_tab WHERE class_id = ? AND role = ? FOR UPDATE`, classID, RoleAdmin).Scan(&n); err != nil {
		return dbErr(err, "count admins")
	}
	if n <= 1 {
		return ErrLastAdmin
	}
	return nil
}

// update replaces the editable fields of the class (admin only).
func (s *Store) update(ctx context.Context, userID uint64, publicID string, in *ClassInput, nowMs int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, publicID, RoleAdmin)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `UPDATE class_tab SET name = ?, school_name = ?, graduation_year = ?, language = ?, updated_at = ? WHERE id = ?`,
			in.Name, in.SchoolName, in.GraduationYear, in.Language, nowMs, classID)
		return dbErr(err, "update class")
	})
}

// remove deletes the class, its member rows and its invites (admin only). Yearbooks are never touched: a member row only points at one.
func (s *Store) remove(ctx context.Context, userID uint64, publicID string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, publicID, RoleAdmin)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM class_member_tab WHERE class_id = ?`, classID); err != nil {
			return dbErr(err, "delete members")
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM class_invite_tab WHERE class_id = ?`, classID); err != nil {
			return dbErr(err, "delete invites")
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM class_tab WHERE id = ?`, classID)
		return dbErr(err, "delete class")
	})
}

// members returns up to limit members of the class (internal id classID, already authorised as admin), oldest first,
// after the member row `after` (0 = first page), with the display names, and the row to continue from (0 on the last page).
// Two queries whatever the page size: the member rows, then their users.
func (s *Store) members(ctx context.Context, classID, after uint64, limit int) ([]Member, uint64, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT m.id, m.public_id, m.user_id, m.role, m.joined_at, y.id FROM class_member_tab m LEFT JOIN yearbook_tab y ON y.id = m.yearbook_id AND y.owner_id = m.user_id WHERE m.class_id = ? AND m.id > ? ORDER BY m.id LIMIT ?`,
		classID, after, limit+1)
	if err != nil {
		return nil, 0, dbErr(err, "list members")
	}
	defer func() { _ = rows.Close() }()
	out := make([]Member, 0, limit+1)
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.row, &m.ID, &m.userID, &m.Role, &m.JoinedAt, &m.book); err != nil {
			return nil, 0, dbErr(err, "scan member")
		}
		m.PageLinked = m.book.Valid
		out = append(out, m)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, dbErr(err, "list members")
	}
	_ = rows.Close() // the connection is free for the names query
	var next uint64
	if len(out) > limit {
		out = out[:limit]
		next = out[limit-1].row
	}
	if len(out) == 0 {
		return out, 0, nil
	}
	ids := make([]any, len(out))
	for i, m := range out {
		ids[i] = m.userID
	}
	nrows, err := s.db.QueryContext(ctx, `SELECT id, display_name FROM user_tab WHERE id IN (`+inList(len(ids))+`)`, ids...) //nolint:gosec // G202: inList emits only "?" placeholders; the ids are arguments
	if err != nil {
		return nil, 0, dbErr(err, "read member names")
	}
	defer func() { _ = nrows.Close() }()
	names := make(map[uint64]string, len(ids))
	for nrows.Next() {
		var id uint64
		var name string
		if err := nrows.Scan(&id, &name); err != nil {
			return nil, 0, dbErr(err, "scan member name")
		}
		names[id] = name
	}
	if err := nrows.Err(); err != nil {
		return nil, 0, dbErr(err, "read member names")
	}
	for i := range out {
		out[i].Name = names[out[i].userID]
	}
	return out, next, s.pageFlags(ctx, out)
}

// target locks the member memberID of the class (the caller is already locked in as admin). A member id of another class is
// not found here, since the lookup is scoped to classID.
func target(ctx context.Context, tx *sql.Tx, classID uint64, memberID string) (row, userID uint64, role string, err error) {
	err = tx.QueryRowContext(ctx, `SELECT id, user_id, role FROM class_member_tab WHERE class_id = ? AND public_id = ? FOR UPDATE`, classID, memberID).Scan(&row, &userID, &role)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, "", ErrMemberNotFound
	}
	return row, userID, role, dbErr(err, "lock target")
}

// capAdminClasses is ErrLimitReached when userID is already admin of maxAdminClasses classes. It locks the user_tab row first, so
// concurrent promotions of one user in different classes (and class/create) serialise; the count that follows is the transaction's
// first plain read (every earlier read of a membership change is a locking one), so it sees what the previous holder committed.
func capAdminClasses(ctx context.Context, tx *sql.Tx, userID uint64) error {
	var locked uint64
	if err := tx.QueryRowContext(ctx, `SELECT id FROM user_tab WHERE id = ? FOR UPDATE`, userID).Scan(&locked); err != nil {
		return dbErr(err, "lock user")
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM class_member_tab WHERE user_id = ? AND role = ?`, userID, RoleAdmin).Scan(&n); err != nil {
		return dbErr(err, "count classes")
	}
	if n >= maxAdminClasses {
		return ErrLimitReached
	}
	return nil
}

// setRole gives the member memberID the role newRole (admin only). Demoting the last admin is ErrLastAdmin.
func (s *Store) setRole(ctx context.Context, userID uint64, publicID, memberID, newRole string, nowMs int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, publicID, RoleAdmin)
		if err != nil {
			return err
		}
		row, memberUser, role, err := target(ctx, tx, classID, memberID)
		if err != nil {
			return err
		}
		if role == newRole {
			return nil
		}
		if err := keepAnAdmin(ctx, tx, classID, role); err != nil {
			return err
		}
		if newRole == RoleAdmin {
			if err := capAdminClasses(ctx, tx, memberUser); err != nil {
				return err
			}
		}
		_, err = tx.ExecContext(ctx, `UPDATE class_member_tab SET role = ?, updated_at = ? WHERE id = ?`, newRole, nowMs, row)
		return dbErr(err, "update role")
	})
}

// removeMember deletes the member memberID from the class (admin only). Removing the last admin is ErrLastAdmin. The
// member's yearbook is untouched.
func (s *Store) removeMember(ctx context.Context, userID uint64, publicID, memberID string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, publicID, RoleAdmin)
		if err != nil {
			return err
		}
		row, _, role, err := target(ctx, tx, classID, memberID)
		if err != nil {
			return err
		}
		if err := keepAnAdmin(ctx, tx, classID, role); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM class_member_tab WHERE id = ?`, row)
		return dbErr(err, "delete member")
	})
}

// leave removes the caller from the class (any member). The last admin cannot leave.
func (s *Store) leave(ctx context.Context, userID uint64, publicID string) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, role, err := lockMembership(ctx, tx, userID, publicID, RoleStudent)
		if err != nil {
			return err
		}
		if err := keepAnAdmin(ctx, tx, classID, role); err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM class_member_tab WHERE class_id = ? AND user_id = ?`, classID, userID)
		return dbErr(err, "delete member")
	})
}

// clearBook un-links a deleted yearbook from the class it was the page of, in the yearbook delete's transaction.
func (s *Store) clearBook(ctx context.Context, tx *sql.Tx, yearbookID uint64, nowMs int64) error {
	_, err := tx.ExecContext(ctx, `UPDATE class_member_tab SET yearbook_id = NULL, updated_at = ? WHERE yearbook_id = ?`, nowMs, yearbookID)
	return dbErr(err, "clear class page link")
}
