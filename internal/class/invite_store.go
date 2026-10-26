package class

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"errors"
)

// inviteRow is an invite found by its token hash, with the class it opens.
type inviteRow struct {
	expiresAt  int64
	revokedAt  sql.Null[int64]
	className  string
	schoolName string
}

// active reports whether the invite can still be used at nowMs (Unix ms).
func (r *inviteRow) active(nowMs int64) bool { return !r.revokedAt.Valid && r.expiresAt > nowMs }

// inviteByHash finds the invite whose token hash is hash; ErrInviteNotFound when there is none. Whether the row is still usable
// is the caller's decision, so an unknown, an expired and a revoked token take the same query and the same code path. The hash
// the database returns is compared again in constant time.
func (s *Store) inviteByHash(ctx context.Context, hash [sha256.Size]byte) (*inviteRow, error) {
	var r inviteRow
	var stored []byte
	err := s.db.QueryRowContext(ctx,
		`SELECT i.token_hash, i.expires_at, i.revoked_at, c.name, c.school_name FROM class_invite_tab i JOIN class_tab c ON c.id = i.class_id WHERE i.token_hash = ?`,
		hash[:]).Scan(&stored, &r.expiresAt, &r.revokedAt, &r.className, &r.schoolName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteNotFound
	}
	if err != nil {
		return nil, dbErr(err, "read invite")
	}
	if subtle.ConstantTimeCompare(stored, hash[:]) != 1 {
		return nil, ErrInviteNotFound
	}
	return &r, nil
}

// createInvite stores the invite of class publicID (admin only). The cap of MaxActiveInvites active invites is counted under
// the class row lock, so parallel creates cannot both take the last slot.
func (s *Store) createInvite(ctx context.Context, userID uint64, publicID string, inv *Invite, hash [sha256.Size]byte, nowMs int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, publicID, RoleAdmin)
		if err != nil {
			return err
		}
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM class_invite_tab WHERE class_id = ? AND revoked_at IS NULL AND expires_at > ? FOR UPDATE`, classID, nowMs).Scan(&n); err != nil {
			return dbErr(err, "count invites")
		}
		if n >= MaxActiveInvites {
			return ErrInviteLimit
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO class_invite_tab (public_id, class_id, token_hash, created_by, expires_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			inv.ID, classID, hash[:], userID, inv.ExpiresAt, nowMs, nowMs)
		if err != nil {
			return dbErr(err, "insert invite")
		}
		row, err := res.LastInsertId()
		inv.row = uint64(row) //nolint:gosec // G115: AUTO_INCREMENT ids are positive
		return dbErr(err, "insert invite")
	})
}

// invites returns up to limit invites of the class (internal id classID, already authorised as admin), newest first, after the
// invite row `after` (0 = first page), and the row to continue from (0 on the last page).
func (s *Store) invites(ctx context.Context, classID, after uint64, limit int, nowMs int64) ([]Invite, uint64, error) {
	q, args := `SELECT id, public_id, created_at, expires_at, revoked_at FROM class_invite_tab WHERE class_id = ?`, []any{classID}
	if after > 0 {
		q += ` AND id < ?`
		args = append(args, after)
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY id DESC LIMIT ?`, append(args, limit+1)...)
	if err != nil {
		return nil, 0, dbErr(err, "list invites")
	}
	defer func() { _ = rows.Close() }()
	out := make([]Invite, 0, limit+1)
	for rows.Next() {
		var inv Invite
		var revoked sql.Null[int64]
		if err := rows.Scan(&inv.row, &inv.ID, &inv.CreatedAt, &inv.ExpiresAt, &revoked); err != nil {
			return nil, 0, dbErr(err, "scan invite")
		}
		if revoked.Valid {
			inv.RevokedAt = &revoked.V
		}
		inv.Active = !revoked.Valid && inv.ExpiresAt > nowMs
		out = append(out, inv)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, dbErr(err, "list invites")
	}
	var next uint64
	if len(out) > limit {
		out = out[:limit]
		next = out[limit-1].row
	}
	return out, next, nil
}

// revokeInvite revokes the invite inviteID of class publicID (admin only). The lookup is scoped to the class, so the invite id of
// another class is ErrInviteNotFound and nothing changes. Revoking a revoked invite succeeds and keeps its first revoked_at.
func (s *Store) revokeInvite(ctx context.Context, userID uint64, publicID, inviteID string, nowMs int64) error {
	return s.inTx(ctx, func(tx *sql.Tx) error {
		classID, _, err := lockMembership(ctx, tx, userID, publicID, RoleAdmin)
		if err != nil {
			return err
		}
		var row uint64
		var revoked sql.Null[int64]
		err = tx.QueryRowContext(ctx, `SELECT id, revoked_at FROM class_invite_tab WHERE class_id = ? AND public_id = ? FOR UPDATE`, classID, inviteID).Scan(&row, &revoked)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInviteNotFound
		}
		if err != nil {
			return dbErr(err, "lock invite")
		}
		if revoked.Valid {
			return nil
		}
		_, err = tx.ExecContext(ctx, `UPDATE class_invite_tab SET revoked_at = ?, updated_at = ? WHERE id = ?`, nowMs, nowMs, row)
		return dbErr(err, "revoke invite")
	})
}

// accept adds userID to the class the token opens as a student, and returns the class with the caller's membership. Every
// failure of the token itself (unknown, expired, revoked, class gone) is ErrInviteNotFound. Lock order: class row, invite row,
// member rows; the invite is read again under the class lock because revokeInvite takes the same lock, so a revoke that committed
// first is seen. A caller who is already a member is returned untouched (idempotent, role kept). The class holds MaxMembers at most.
func (s *Store) accept(ctx context.Context, userID uint64, hash [sha256.Size]byte, memberID string, nowMs int64) (*Class, error) {
	// Which class? A plain read outside the transaction, so it takes no snapshot; the decision is made under the lock below.
	var inviteRowID, classRow uint64
	err := s.db.QueryRowContext(ctx, `SELECT id, class_id FROM class_invite_tab WHERE token_hash = ?`, hash[:]).Scan(&inviteRowID, &classRow)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrInviteNotFound
	}
	if err != nil {
		return nil, dbErr(err, "read invite")
	}
	var c *Class
	err = s.inTx(ctx, func(tx *sql.Tx) error {
		var locked uint64
		err := tx.QueryRowContext(ctx, `SELECT id FROM class_tab WHERE id = ? FOR UPDATE`, classRow).Scan(&locked)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInviteNotFound
		}
		if err != nil {
			return dbErr(err, "lock class")
		}
		var inv inviteRow
		err = tx.QueryRowContext(ctx, `SELECT expires_at, revoked_at FROM class_invite_tab WHERE id = ? FOR UPDATE`, inviteRowID).Scan(&inv.expiresAt, &inv.revokedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ErrInviteNotFound
		}
		if err != nil {
			return dbErr(err, "lock invite")
		}
		if !inv.active(nowMs) {
			return ErrInviteNotFound
		}
		var member uint64
		err = tx.QueryRowContext(ctx, `SELECT id FROM class_member_tab WHERE class_id = ? AND user_id = ? FOR UPDATE`, classRow, userID).Scan(&member)
		if errors.Is(err, sql.ErrNoRows) {
			var n int
			if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM class_member_tab WHERE class_id = ? FOR UPDATE`, classRow).Scan(&n); err != nil {
				return dbErr(err, "count members")
			}
			if n >= MaxMembers {
				return ErrClassFull
			}
			_, err = tx.ExecContext(ctx,
				`INSERT INTO class_member_tab (public_id, class_id, user_id, role, joined_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
				memberID, classRow, userID, RoleStudent, nowMs, nowMs, nowMs)
			if err != nil {
				return dbErr(err, "insert member")
			}
		} else if err != nil {
			return dbErr(err, "lock member")
		}
		c, err = scanClass(tx.QueryRowContext(ctx, `SELECT `+classCols+classFrom+` WHERE m.user_id = ? AND c.id = ?`, userID, classRow))
		return dbErr(err, "read class")
	})
	if err != nil {
		return nil, err
	}
	return c, nil
}
