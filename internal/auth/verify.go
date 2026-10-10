package auth

import (
	"context"
	"time"
)

// VerifyEmail checks the 6-digit code typed by the signed-in user and marks their email verified.
// An already verified user is a no-op. Attempts are limited to 20 per hour per user on top of the
// 5 wrong guesses a single code allows.
func (s *Service) VerifyEmail(ctx context.Context, u User, code string) error {
	if u.EmailVerified {
		return nil
	}
	// Fails closed (503 limiter_unavailable): this limit bounds code guessing per user.
	ok, retry, err := s.verifyTries.Take(ctx, u.ID)
	if err != nil {
		return err
	}
	if !ok {
		return RateLimitedError{retry}
	}
	now := s.now().UTC()
	if err := s.codes.Check(ctx, purposeVerify, u.InternalID, code, now); err != nil {
		return err
	}
	return s.store.markEmailVerified(ctx, u.InternalID, now)
}

// ResendVerification emails a new code (3 per hour per user; the old code dies). alreadyVerified
// is true when there is nothing to send.
func (s *Service) ResendVerification(ctx context.Context, u User) (alreadyVerified bool, err error) {
	if u.EmailVerified {
		return true, nil
	}
	// Fails open: a resend is not a guess.
	if ok, retry, _ := s.resend.Take(ctx, u.ID); !ok {
		return false, RateLimitedError{retry}
	}
	return false, s.sendCode(ctx, u, purposeVerify)
}

func (s *Store) markEmailVerified(ctx context.Context, userID uint64, now time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE users SET email_verified_at = ?, updated_at = ? WHERE id = ? AND email_verified_at IS NULL`, now, now, userID)
	return err
}
