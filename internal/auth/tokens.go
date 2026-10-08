package auth

import (
	"bytes"
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"text/template"
	"time"

	"github.com/danyaa666/smemories/internal/mailer"
)

const (
	purposeVerify = "verify"
	purposeReset  = "reset"
	verifyTTL     = 24 * time.Hour
	resetTTL      = time.Hour

	resendPerHour      = 3 // per user
	forgotPerIPHour    = 5
	forgotPerEmailHour = 3
	mailTimeout        = 10 * time.Second
	cleanupEvery       = 24 * time.Hour
)

// ErrInvalidToken is an unknown, expired, used or wrong-purpose email token.
var ErrInvalidToken = errors.New("auth: invalid token")

// Mail is what the service needs to send the verification and reset emails.
type Mail struct {
	Mailer  mailer.Mailer
	BaseURL string // SMEM_PUBLIC_BASE_URL without a trailing slash
	Logger  *slog.Logger
}

//go:embed emails/*.tmpl
var emailFS embed.FS

var emailTmpl = template.Must(template.ParseFS(emailFS, "emails/*.tmpl"))

// renderEmail fills the subject and text of the email in the user's language (English when the
// locale is unknown). kind is "verify" or "reset".
func renderEmail(locale, kind, name, link string) (subject, text string, err error) {
	if emailTmpl.Lookup(locale+"."+kind+".text") == nil {
		locale = "en"
	}
	data := struct{ Name, Link string }{name, link}
	var s, t bytes.Buffer
	if err = emailTmpl.ExecuteTemplate(&s, locale+"."+kind+".subject", data); err != nil {
		return
	}
	if err = emailTmpl.ExecuteTemplate(&t, locale+"."+kind+".text", data); err != nil {
		return
	}
	return s.String(), t.String(), nil
}

// sendToken creates a single-use token for u and emails the link. The token only ever lives in
// the email and the link; errors returned here never contain it.
func (s *Service) sendToken(ctx context.Context, u User, purpose string) error {
	token, hash, err := newToken()
	if err != nil {
		return err
	}
	now := s.now().UTC()
	ttl, kind, path := verifyTTL, "verify", "/verify-email?token="
	if purpose == purposeReset {
		ttl, kind, path = resetTTL, "reset", "/reset-password?token="
	}
	if err := s.store.insertEmailToken(ctx, hash, u.InternalID, purpose, now, now.Add(ttl)); err != nil {
		return err
	}
	subject, text, err := renderEmail(u.Locale, kind, u.DisplayName, s.mail.BaseURL+path+token)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, mailTimeout)
	defer cancel()
	if err := s.mail.Mailer.Send(ctx, mailer.Message{To: u.Email, Subject: subject, Text: text}); err != nil {
		return fmt.Errorf("send %s email: %w", kind, err)
	}
	return nil
}

// background runs f detached from the request, so the caller's latency does not depend on
// whether there was anything to do. Wait blocks until all of them have finished.
//
// ponytail: no queue; a crash loses an in-flight email and the user asks again. Move to an
// outbox table when delivery must be guaranteed.
func (s *Service) background(f func(context.Context)) {
	s.bg.Add(1)
	go func() {
		defer s.bg.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 3*mailTimeout)
		defer cancel()
		f(ctx)
	}()
}

// Wait blocks until the emails sent in the background have been handed to the mailer.
func (s *Service) Wait() { s.bg.Wait() }

// CleanupTokens deletes expired and used email tokens.
func (s *Service) CleanupTokens(ctx context.Context) (int64, error) {
	return s.store.deleteDeadEmailTokens(ctx, s.now().UTC())
}

// RunCleanup cleans now and then every 24 h until ctx ends.
func (s *Service) RunCleanup(ctx context.Context) {
	t := time.NewTicker(cleanupEvery)
	defer t.Stop()
	for {
		if n, err := s.CleanupTokens(ctx); err != nil {
			if ctx.Err() != nil {
				return
			}
			s.mail.Logger.Error("auth: email token cleanup failed", "error", err)
		} else if n > 0 {
			s.mail.Logger.Info("auth: removed dead email tokens", "count", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (s *Store) insertEmailToken(ctx context.Context, hash []byte, userID uint64, purpose string, now, expires time.Time) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO email_tokens (token_hash, user_id, purpose, expires_at, created_at) VALUES (?,?,?,?,?)`,
		hash, userID, purpose, expires, now)
	return err
}

// consumeEmailToken marks a live token of the given purpose used and returns its user. Of two
// concurrent calls with the same token exactly one succeeds (the UPDATE's row lock serialises
// them and the loser sees used_at set); the other gets ErrInvalidToken.
func consumeEmailToken(ctx context.Context, tx *sql.Tx, hash []byte, purpose string, now time.Time) (uint64, error) {
	res, err := tx.ExecContext(ctx,
		`UPDATE email_tokens SET used_at = ? WHERE token_hash = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?`,
		now, hash, purpose, now)
	if err != nil {
		return 0, err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		if err != nil {
			return 0, err
		}
		return 0, ErrInvalidToken
	}
	var userID uint64
	err = tx.QueryRowContext(ctx, `SELECT user_id FROM email_tokens WHERE token_hash = ?`, hash).Scan(&userID)
	return userID, err
}

func (s *Store) deleteDeadEmailTokens(ctx context.Context, now time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM email_tokens WHERE expires_at <= ? OR used_at IS NOT NULL`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
