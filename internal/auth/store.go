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

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ulid"
)

var errNotFound = apperr.New(apperr.Internal, "auth: not found") // callers test it with errors.Is and pick their own code

const (
	maxUserAgentLen = 255

	// retryTx waits retryBase<<i plus up to retryGrowth times that, at most retryCap.
	retryBase   = 5 * time.Millisecond
	retryGrowth = 5
	retryCap    = 200 * time.Millisecond

	mysqlDuplicateEntry = 1062
	mysqlDeadlock       = 1213
	mysqlLockTimeout    = 1205

	txAttempts = 6
)

// User is the account as the API shows it. InternalID is the BIGINT key other tables
// reference; it is never serialised (L-05).
type User struct {
	ID            string `json:"id"`
	Email         string `json:"email"`
	EmailVerified bool   `json:"email_verified"`
	DisplayName   string `json:"display_name"`
	Locale        string `json:"locale"`
	CreatedAt     int64  `json:"created_at"` // Unix ms
	Username      string `json:"username"`   // lower case, unique, public (T-081)
	// UsernameNextChangeAt is when the username may change again (Unix ms); nil when a change is allowed now. The service fills it
	// from its clock (Service.expose); the store only knows UsernameChangedAt.
	UsernameNextChangeAt *int64 `json:"username_next_change_at"`
	UsernameChangedAt    int64  `json:"-"` // Unix ms of the last user-made change, 0 = never
	InternalID           uint64 `json:"-"`
	AuthEpoch            uint32 `json:"-"` // credential epoch (T-103): sessions issued under another epoch are refused
	TermsVersion         string `json:"-"` // version of the Terms and Privacy Policy accepted, "" = never (T-111)
}

// Store is the users persistence (sessions live in Redis, see sessions.go). Every query is parameterised.
type Store struct{ db *sql.DB }

func NewStore(db *sql.DB) *Store { return &Store{db: db} }

const userCols = `u.id, u.public_id, u.email, u.email_verified_at IS NOT NULL, u.display_name, u.locale, u.created_at, u.username, u.username_changed_at, u.auth_epoch, COALESCE(u.terms_version, '')`

func scanUser(sc interface{ Scan(...any) error }, extra ...any) (*User, error) {
	u := new(User)
	dest := append([]any{&u.InternalID, &u.ID, &u.Email, &u.EmailVerified, &u.DisplayName, &u.Locale, &u.CreatedAt, &u.Username, &u.UsernameChangedAt, &u.AuthEpoch, &u.TermsVersion}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return nil, err
	}
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
	if err := insertUser(ctx, tx, u, passwordHash, nil); err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == mysqlDuplicateEntry && !isUsernameConflict(err) {
			return ErrEmailTaken
		}
		return err
	}
	if err := withUser(u.InternalID); err != nil {
		return err
	}
	return tx.Commit()
}

// isUsernameConflict reports whether err is the unique key on user_tab.username (and not the email key).
func isUsernameConflict(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == mysqlDuplicateEntry && strings.Contains(me.Message, "uq_user_tab_username")
}

// insertUser inserts u (public_id, email, display_name, locale, created_at set; passwordHash and verifiedAt may be nil) and
// sets InternalID and Username. The default username is tried with each slice of the public id in turn on a username
// conflict, so a registration never fails because of it; any other error (a duplicate email included) is returned as is. A
// failed statement does not abort the surrounding transaction.
func insertUser(ctx context.Context, tx *sql.Tx, u *User, passwordHash, verifiedAt any) error {
	// Register sets TermsVersion (the account and its acceptance are one transaction); a Google account has none yet and
	// must accept at its first request (T-111).
	var termsVersion, termsAt any
	if u.TermsVersion != "" {
		termsVersion, termsAt = u.TermsVersion, u.CreatedAt
	}
	for attempt := 0; ; attempt++ { // the last attempt always returns, so the loop has no end
		u.Username = defaultUsername(u.ID, attempt)
		res, err := tx.ExecContext(ctx,
			`INSERT INTO user_tab (public_id, email, password_hash, email_verified_at, display_name, locale, username, terms_version, terms_accepted_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
			u.ID, u.Email, passwordHash, verifiedAt, u.DisplayName, u.Locale, u.Username, termsVersion, termsAt, u.CreatedAt, u.CreatedAt)
		if err != nil {
			if isUsernameConflict(err) && attempt < len(defaultSuffixLens)-1 {
				continue
			}
			return err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return err
		}
		if id <= 0 {
			return apperr.New(apperr.Internal, "auth: insert returned a non-positive user id")
		}
		u.InternalID = uint64(id)
		return nil
	}
}

// changeUsername sets the username in one conditional UPDATE: it matches only while the cooldown is over (username_changed_at <=
// allowedBefore), so two concurrent changes of one user cannot both pass. changed is false when nothing matched (cooldown still
// running, or the user is gone); a taken name is ErrUsernameTaken. A taken name is only reported when the cooldown is over,
// because the WHERE clause is checked before the unique key, so the cooldown answer never leaks whether a name exists.
func (s *Store) changeUsername(ctx context.Context, id uint64, name string, nowMs, allowedBefore int64) (changed bool, err error) {
	res, err := s.db.ExecContext(ctx,
		`UPDATE user_tab SET username = ?, username_changed_at = ?, updated_at = ? WHERE id = ? AND username_changed_at <= ?`,
		name, nowMs, nowMs, id, allowedBefore)
	if err != nil {
		if isUsernameConflict(err) {
			return false, ErrUsernameTaken
		}
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

// acceptTerms records that the user accepted version at nowMs. Repeating it is harmless and keeps the time of the first acceptance
// (the WHERE matches only a user who has not accepted this version yet).
func (s *Store) acceptTerms(ctx context.Context, id uint64, version string, nowMs int64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE user_tab SET terms_version = ?, terms_accepted_at = ?, updated_at = ? WHERE id = ? AND (terms_version IS NULL OR terms_version <> ?)`, version, nowMs, nowMs, id, version)
	return err
}

// userByEmail returns the user and their password hash ("" when the account has none).
func (s *Store) userByEmail(ctx context.Context, email string) (*User, string, error) {
	var hash sql.NullString
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+`, u.password_hash FROM user_tab u WHERE u.email = ?`, email), &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, "", errNotFound
	}
	return u, hash.String, err
}

// session is what Sessions.Create stores; tokenHash is the sha256 of the cookie value.
//
// session is passed by value on purpose: a small immutable record handed to the session store.
type session struct {
	tokenHash []byte
	userID    uint64
	now       time.Time
	expires   time.Time
	userAgent string
	epoch     uint32 // the user's auth_epoch when the credential was read
}

// cleanUserAgent makes a client-supplied header safe to keep (valid UTF-8, at most maxUserAgentLen characters).
func cleanUserAgent(ua string) string {
	ua = strings.ToValidUTF8(ua, "")
	if utf8.RuneCountInString(ua) > maxUserAgentLen {
		ua = string([]rune(ua)[:maxUserAgentLen])
	}
	return ua
}

// userByID returns the user with the given internal id (the owner of a session).
func (s *Store) userByID(ctx context.Context, id uint64) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM user_tab u WHERE u.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return u, err
}

// googleUser finds or creates the account for a Google identity in one transaction (see
// Service.SignInGoogle for the rules). want carries the display name and locale to use if
// the account is created. A concurrent callback for the same person can hit the unique key
// or a deadlock; the loser retries and then finds the winner's rows. When an unverified local account was taken over,
// dropSessions runs after the commit (the auth_epoch bump is what revokes the old sessions; the purge only cleans up) and its
// error is returned.
func (s *Store) googleUser(ctx context.Context, id *GoogleIdentity, want *User, now time.Time, dropSessions func(ctx context.Context, userID uint64) error) (*User, error) {
	var u *User
	var merged bool
	err := retryTx(ctx, sleepCtx, func() (err error) {
		u, merged, err = s.googleUserTx(ctx, id, want, now)
		return err
	})
	if err != nil {
		return nil, err
	}
	if merged {
		return u, dropSessions(ctx, u.InternalID)
	}
	return u, nil
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
		lo := retryBase << i
		hi := min(retryGrowth*lo, retryCap)
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

func (s *Store) googleUserTx(ctx context.Context, id *GoogleIdentity, want *User, now time.Time) (u *User, merged bool, err error) {
	ms := now.UnixMilli()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback() }()

	// 1:1 primary-key join inside the sign-in transaction (docs/db-conventions.md allows it); see T-047.
	u, err = scanUser(tx.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM user_identity_tab i JOIN user_tab u ON u.id = i.user_id WHERE i.provider = 'google' AND i.subject = ?`, id.Subject))
	if err == nil {
		return u, false, nil // known identity
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, false, err
	}

	u, err = scanUser(tx.QueryRowContext(ctx, `SELECT `+userCols+` FROM user_tab u WHERE u.email = ? FOR UPDATE`, id.Email))
	switch {
	case err == nil && !u.EmailVerified:
		// Pre-hijacking defence: whoever registered this address never proved they own it, so their password and sessions go
		// before the real owner is linked. The epoch bump revokes every session, including one that a login which already read
		// the old row is about to create; the caller purges the Redis entries after the commit.
		if _, err = tx.ExecContext(ctx, `UPDATE user_tab SET password_hash = NULL, email_verified_at = ?, auth_epoch = auth_epoch + 1, updated_at = ? WHERE id = ?`, ms, ms, u.InternalID); err != nil {
			return nil, false, err
		}
		u.EmailVerified, u.AuthEpoch, merged = true, u.AuthEpoch+1, true // the row is locked (FOR UPDATE), so +1 is exact
	case err == nil:
	case errors.Is(err, sql.ErrNoRows):
		u = &User{ID: ulid.New(now), Email: id.Email, EmailVerified: true, DisplayName: want.DisplayName, Locale: want.Locale, CreatedAt: ms}
		if err := insertUser(ctx, tx, u, nil, ms); err != nil {
			return nil, false, err
		}
	default:
		return nil, false, err
	}
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO user_identity_tab (user_id, provider, subject, email, created_at, updated_at) VALUES (?,'google',?,?,?,?)`, u.InternalID, id.Subject, id.Email, ms, ms); err != nil {
		return nil, false, err
	}
	return u, merged, tx.Commit()
}
