package auth

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"
)

var (
	errNotFound   = errors.New("auth: not found")
	ErrEmailTaken = errors.New("auth: email already registered")
)

const mysqlDuplicateEntry = 1062

// User is the account as the API shows it. InternalID is the BIGINT key other tables
// reference; it is never serialised (L-05).
type User struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	EmailVerified bool      `json:"email_verified"`
	DisplayName   string    `json:"display_name"`
	Locale        string    `json:"locale"`
	CreatedAt     time.Time `json:"created_at"`
	InternalID    uint64    `json:"-"`
}

// Store is the users/sessions persistence. Every query is parameterised.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

const userCols = `u.id, u.public_id, u.email, u.email_verified_at IS NOT NULL, u.display_name, u.locale, u.created_at`

func scanUser(sc interface{ Scan(...any) error }, extra ...any) (User, error) {
	var u User
	dest := append([]any{&u.InternalID, &u.ID, &u.Email, &u.EmailVerified, &u.DisplayName, &u.Locale, &u.CreatedAt}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return User{}, err
	}
	u.CreatedAt = u.CreatedAt.UTC()
	return u, nil
}

// createUserWithSession inserts the user and their first session atomically.
// A duplicate email returns ErrEmailTaken.
func (s *Store) createUserWithSession(ctx context.Context, u *User, passwordHash string, sess session) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (public_id, email, password_hash, display_name, locale, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		u.ID, u.Email, passwordHash, u.DisplayName, u.Locale, u.CreatedAt, u.CreatedAt)
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == mysqlDuplicateEntry {
			return ErrEmailTaken
		}
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	if id <= 0 {
		return errors.New("auth: insert returned a non-positive user id")
	}
	u.InternalID = uint64(id)
	sess.userID = u.InternalID
	if err := insertSession(ctx, tx, sess); err != nil {
		return err
	}
	return tx.Commit()
}

// userByEmail returns the user and their password hash ("" when the account has none).
func (s *Store) userByEmail(ctx context.Context, email string) (User, string, error) {
	var hash sql.NullString
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+`, u.password_hash FROM users u WHERE u.email = ?`, email), &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, "", errNotFound
	}
	return u, hash.String, err
}

type session struct {
	tokenHash []byte
	userID    uint64
	now       time.Time
	expires   time.Time
	userAgent string
}

type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func insertSession(ctx context.Context, x execer, s session) error {
	_, err := x.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, created_at, last_seen_at, expires_at, user_agent) VALUES (?,?,?,?,?,?)`,
		s.tokenHash, s.userID, s.now, s.now, s.expires, cleanUserAgent(s.userAgent))
	return err
}

func (s *Store) createSession(ctx context.Context, sess session) error {
	return insertSession(ctx, s.db, sess)
}

// cleanUserAgent makes a client-supplied header safe for VARCHAR(255) utf8mb4.
func cleanUserAgent(ua string) string {
	ua = strings.ToValidUTF8(ua, "")
	if utf8.RuneCountInString(ua) > 255 {
		ua = string([]rune(ua)[:255])
	}
	return ua
}

// userBySession returns the user owning a live (unexpired) session and its expiry.
func (s *Store) userBySession(ctx context.Context, tokenHash []byte, now time.Time) (User, time.Time, error) {
	var exp time.Time
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userCols+`, s.expires_at FROM sessions s JOIN users u ON u.id = s.user_id WHERE s.token_hash = ? AND s.expires_at > ?`,
		tokenHash, now), &exp)
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, time.Time{}, errNotFound
	}
	return u, exp.UTC(), err
}

func (s *Store) extendSession(ctx context.Context, tokenHash []byte, now, expires time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE token_hash = ?`, now, expires, tokenHash)
	return err
}

func (s *Store) deleteSession(ctx context.Context, tokenHash []byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

// deleteExpiredSessions keeps the table from filling with dead rows: each login sweeps
// that user's expired sessions.
func (s *Store) deleteExpiredSessions(ctx context.Context, userID uint64, now time.Time) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND expires_at <= ?`, userID, now)
	return err
}
