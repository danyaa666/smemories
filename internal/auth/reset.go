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
		if err := s.sendCode(ctx, u, purposeReset); err != nil {
			s.mail.Logger.Error("auth: reset email not sent", "user_id", u.ID, "error", err)
		}
	})
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
		return ValidationError{codeInvalidEmail}
	}
	if ok, retry := s.resetIP.Take(ip); !ok {
		return RateLimitedError{retry}
	}
	password = normalizePassword(password)
	if !validPassword(password, email) {
		return ValidationError{codeWeakPassword}
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
	return s.store.resetPassword(ctx, u.InternalID, phc, now, s.sessions.DeleteAll)
}

// resetPassword stores the new hash and deletes every session of the user. The sessions go before the
// commit: if Redis fails the password is not changed either, and the sign-out can never be skipped.
func (s *Store) resetPassword(ctx context.Context, userID uint64, phc string, now time.Time, dropSessions func(ctx context.Context, userID uint64) error) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?`, phc, now, userID); err != nil {
		return err
	}
	if err := dropSessions(ctx, userID); err != nil {
		return err
	}
	return tx.Commit()
}
