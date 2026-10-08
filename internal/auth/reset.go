package auth

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ForgotPassword emails a reset link when the account exists. It answers the same (nil) for
// known and unknown addresses and does the same work on the request path (one lookup); the
// token insert and the send happen in the background. Limits: 5 per hour per IP, 3 per hour
// per address, counted before the lookup so they cannot reveal whether the account exists.
func (s *Service) ForgotPassword(ctx context.Context, ip, email string) error {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return ValidationError{codeInvalidEmail}
	}
	if ok, retry := s.forgotIP.Take(ip); !ok {
		return RateLimitedError{retry}
	}
	if ok, retry := s.forgotEmail.Take(email); !ok {
		return RateLimitedError{retry}
	}
	u, _, err := s.store.userByEmail(ctx, email)
	if errors.Is(err, errNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	s.background(func(ctx context.Context) {
		if err := s.sendToken(ctx, u, purposeReset); err != nil {
			s.mail.Logger.Error("auth: reset email not sent", "user_id", u.ID, "error", err)
		}
	})
	return nil
}

// ResetPassword sets a new password with a reset token and signs the account out everywhere.
// The token is checked before the (expensive) hash and consumed only if the password is acceptable.
func (s *Service) ResetPassword(ctx context.Context, token, password string) error {
	hash := hashToken(token)
	now := s.now().UTC()
	email, err := s.store.pendingResetEmail(ctx, hash, now)
	if err != nil {
		return err
	}
	password = normalizePassword(password)
	if !validPassword(password, email) {
		return ValidationError{codeWeakPassword}
	}
	phc, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return err
	}
	return s.store.resetPassword(ctx, hash, phc, now)
}

// pendingResetEmail returns the address of the account a live reset token belongs to.
func (s *Store) pendingResetEmail(ctx context.Context, hash []byte, now time.Time) (string, error) {
	var email string
	err := s.db.QueryRowContext(ctx,
		`SELECT u.email FROM email_tokens t JOIN users u ON u.id = t.user_id
		 WHERE t.token_hash = ? AND t.purpose = ? AND t.used_at IS NULL AND t.expires_at > ?`,
		hash, purposeReset, now).Scan(&email)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrInvalidToken
	}
	return email, err
}

// resetPassword atomically consumes the token, stores the new hash, deletes every session of
// the user and retires their other reset links.
func (s *Store) resetPassword(ctx context.Context, hash []byte, phc string, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	userID, err := consumeEmailToken(ctx, tx, hash, purposeReset, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, phc, now, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE email_tokens SET used_at = ? WHERE user_id = ? AND purpose = 'reset' AND used_at IS NULL`, now, userID); err != nil {
		return err
	}
	return tx.Commit()
}
