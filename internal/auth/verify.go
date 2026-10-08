package auth

import (
	"context"
	"time"
)

// VerifyEmail consumes a verification token and marks the account's email verified.
func (s *Service) VerifyEmail(ctx context.Context, token string) error {
	return s.store.verifyEmail(ctx, hashToken(token), s.now().UTC())
}

// ResendVerification emails a new link (3 per hour per user). alreadyVerified is true when
// there is nothing to send.
func (s *Service) ResendVerification(ctx context.Context, u User) (alreadyVerified bool, err error) {
	if u.EmailVerified {
		return true, nil
	}
	if ok, retry := s.resend.Take(u.ID); !ok {
		return false, RateLimitedError{retry}
	}
	return false, s.sendToken(ctx, u, purposeVerify)
}

func (s *Store) verifyEmail(ctx context.Context, hash []byte, now time.Time) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	userID, err := consumeEmailToken(ctx, tx, hash, purposeVerify, now)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE users SET email_verified_at = ?, updated_at = ? WHERE id = ? AND email_verified_at IS NULL`, now, now, userID); err != nil {
		return err
	}
	return tx.Commit()
}
