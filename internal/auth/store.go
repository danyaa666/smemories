package auth

import (
	"context"
	"database/sql"
	"errors"
	"math/rand/v2"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-sql-driver/mysql"

	"github.com/danyaa666/smemories/internal/ulid"
)

var (
	errNotFound   = errors.New("auth: not found")
	ErrEmailTaken = errors.New("auth: email already registered")
)

const (
	mysqlDuplicateEntry = 1062
	mysqlDeadlock       = 1213
	mysqlLockTimeout    = 1205

	txAttempts = 6
)

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

// Store is the users persistence (sessions live in Redis, see sessions.go). Every query is parameterised.
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

// createUser inserts the user and then calls withUser (which creates the first session in Redis) before
// committing, so a session store failure leaves no account behind. A duplicate email returns ErrEmailTaken.
func (s *Store) createUser(ctx context.Context, u *User, passwordHash string, withUser func(userID uint64) error) error {
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
	if err := withUser(u.InternalID); err != nil {
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

// session is what Sessions.Create stores; tokenHash is the sha256 of the cookie value.
type session struct {
	tokenHash []byte
	userID    uint64
	now       time.Time
	expires   time.Time
	userAgent string
}

// cleanUserAgent makes a client-supplied header safe to keep (valid UTF-8, at most 255 characters).
func cleanUserAgent(ua string) string {
	ua = strings.ToValidUTF8(ua, "")
	if utf8.RuneCountInString(ua) > 255 {
		ua = string([]rune(ua)[:255])
	}
	return ua
}

// userByID returns the user with the given internal id (the owner of a session).
func (s *Store) userByID(ctx context.Context, id uint64) (User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users u WHERE u.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return User{}, errNotFound
	}
	return u, err
}

// googleUser finds or creates the account for a Google identity in one transaction (see
// Service.SignInGoogle for the rules). want carries the display name and locale to use if
// the account is created. A concurrent callback for the same person can hit the unique key
// or a deadlock; the loser retries and then finds the winner's rows.
func (s *Store) googleUser(ctx context.Context, id GoogleIdentity, want User, now time.Time, dropSessions func(ctx context.Context, userID uint64) error) (User, error) {
	var u User
	err := retryTx(ctx, sleepCtx, func() (err error) {
		u, err = s.googleUserTx(ctx, id, want, now, dropSessions)
		return err
	})
	return u, err
}

func isRetryable(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == mysqlDuplicateEntry || me.Number == mysqlDeadlock || me.Number == mysqlLockTimeout)
}

// retryTx runs fn up to txAttempts times while it fails with a retryable MySQL error, waiting a
// random time between attempts (5-25 ms, doubling, upper bound capped at 200 ms) so colliding
// transactions spread out. It stops at once when ctx ends and returns the last error.
func retryTx(ctx context.Context, sleep func(context.Context, time.Duration) error, fn func() error) error {
	err := fn()
	for i := 0; i < txAttempts-1 && isRetryable(err); i++ {
		lo := 5 * time.Millisecond << i
		hi := min(5*lo, 200*time.Millisecond)
		if serr := sleep(ctx, lo+rand.N(hi-lo)); serr != nil { //nolint:gosec // jitter, not a secret
			return err
		}
		err = fn()
	}
	return err
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *Store) googleUserTx(ctx context.Context, id GoogleIdentity, want User, now time.Time, dropSessions func(ctx context.Context, userID uint64) error) (User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return User{}, err
	}
	defer func() { _ = tx.Rollback() }()

	u, err := scanUser(tx.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM user_identities i JOIN users u ON u.id = i.user_id WHERE i.provider = 'google' AND i.subject = ?`, id.Subject))
	if err == nil {
		return u, nil // known identity
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return User{}, err
	}

	u, err = scanUser(tx.QueryRowContext(ctx, `SELECT `+userCols+` FROM users u WHERE u.email = ? FOR UPDATE`, id.Email))
	switch {
	case err == nil && !u.EmailVerified:
		// Pre-hijacking defence: whoever registered this address never proved they own it, so
		// their password and sessions go before the real owner is linked.
		if _, err = tx.ExecContext(ctx, `UPDATE users SET password_hash = NULL, email_verified_at = ?, updated_at = ? WHERE id = ?`, now, now, u.InternalID); err != nil {
			return User{}, err
		}
		// Before the commit, so a Redis failure rolls everything back and the next try still finds the account unverified.
		if err = dropSessions(ctx, u.InternalID); err != nil {
			return User{}, err
		}
		u.EmailVerified = true
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		u = User{ID: ulid.New(now), Email: id.Email, EmailVerified: true, DisplayName: want.DisplayName, Locale: want.Locale, CreatedAt: now}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO users (public_id, email, password_hash, email_verified_at, display_name, locale, created_at, updated_at) VALUES (?,?,NULL,?,?,?,?,?)`,
			u.ID, u.Email, now, u.DisplayName, u.Locale, now, now)
		if err != nil {
			return User{}, err
		}
		n, err := res.LastInsertId()
		if err != nil || n <= 0 {
			return User{}, errors.New("auth: insert returned no user id")
		}
		u.InternalID = uint64(n)
	default:
		return User{}, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_identities (user_id, provider, subject, email, created_at) VALUES (?,'google',?,?,?)`, u.InternalID, id.Subject, id.Email, now); err != nil {
		return User{}, err
	}
	return u, tx.Commit()
}
