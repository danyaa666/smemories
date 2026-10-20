package auth

import (
	"context"
	"errors"
	"time"
)

// ForgotPassword emails a reset code when the account exists. It answers the same (nil) for
// known and unknown addresses and does the same work on the request path (one lookup); issuing
// the code and sending happen in the background. Limits: 5 per hour per IP, 3 per hour per
// address, counted before the lookup so they cannot reveal whether the account exists.
func (s *Service) ForgotPassword(ctx context.Context, ip, email string) error {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return errInvalidEmail
	}
	// Both limiters fail open: a reset request is not a guess, and the code itself is guarded in Redis.
	if ok, retry, _ := s.forgotIP.Take(ctx, ip); !ok {
		return RateLimitedError{retry}
	}
	if ok, retry, _ := s.forgotEmail.Take(ctx, hashKey(email)); !ok {
		return RateLimitedError{retry}
	}
	u, _, err := s.store.userByEmail(ctx, email)
	if errors.Is(err, errNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	s.sendCodeLater(ctx, u, purposeReset, "auth: reset email")
	return nil
}

// ResetPassword sets a new password with the emailed code and signs the account out everywhere.
// The password rules are checked first, so a weak password never reveals or uses up a code. An
// unknown address, a wrong code and a missing code all answer ErrInvalidCode after the same work.
//
// ponytail: the code is consumed before the (slow) hash; if hashing then fails with ErrBusy the
// student asks for a new code. Split check and consume when that shows up in practice.
func (s *Service) ResetPassword(ctx context.Context, ip, email, code, password string) error {
	email = normalizeEmail(email)
	if !validEmail(email) {
		return errInvalidEmail
	}
	// Fails closed (503 limiter_unavailable): this limit bounds code guessing per IP.
	ok, retry, err := s.resetIP.Take(ctx, ip)
	if err != nil {
		return err
	}
	if !ok {
		return RateLimitedError{retry}
	}
	password = normalizePassword(password)
	if !validPassword(password, email) {
		return errWeakPassword
	}
	now := s.now().UTC()
	u, _, err := s.store.userByEmail(ctx, email)
	if errors.Is(err, errNotFound) {
		return s.codes.Decoy(ctx, purposeReset, code, now)
	}
	if err != nil {
		return err
	}
	if err := s.codes.Check(ctx, purposeReset, u.InternalID, code, now); err != nil {
		return err
	}
	phc, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return err
	}
	return s.store.resetPassword(ctx, u.InternalID, phc, now, s.afterCommit(s.sessions.DeleteAll))
}

// resetPassword stores the new hash and raises the user's auth_epoch in one transaction. The epoch is what signs every
// existing session (and any a racing login is about to create) out; dropSessions runs after the commit as a clean-up and its
// error is returned (Service.afterCommit logs it and makes it nil).
func (s *Store) resetPassword(ctx context.Context, userID uint64, phc string, now time.Time, dropSessions func(ctx context.Context, userID uint64) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE user_tab SET password_hash = ?, auth_epoch = auth_epoch + 1, updated_at = ? WHERE id = ?`, phc, now.UnixMilli(), userID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return dropSessions(ctx, userID)
}

// afterCommit wraps a session purge that runs after a credential change committed: the auth_epoch already revokes every
// session, so a purge failure is logged and does not fail the request.
func (s *Service) afterCommit(drop func(ctx context.Context, userID uint64) error) func(ctx context.Context, userID uint64) error {
	return func(ctx context.Context, userID uint64) error {
		if err := drop(ctx, userID); err != nil {
			s.sessions.logger.Error("auth: session purge after credential change failed; the epoch still revokes them", "error", err)
		}
		return nil
	}
}
