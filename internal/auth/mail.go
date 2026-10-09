package auth

import (
	"bytes"
	"context"
	"embed"
	"log/slog"
	"text/template"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/mailer"
)

const mailTimeout = 10 * time.Second

// Mail is what the service needs to send the verification and reset code emails.
type Mail struct {
	Mailer mailer.Mailer
	Logger *slog.Logger
}

//go:embed emails/*.tmpl
var emailFS embed.FS

var emailTmpl = template.Must(template.ParseFS(emailFS, "emails/*.tmpl"))

// renderEmail fills the subject and text of the code email in the user's language (English when
// the locale is unknown). purpose is purposeVerify or purposeReset; minutes is the code's lifetime.
func renderEmail(locale, purpose, name, code string, minutes int) (subject, text string, err error) {
	if emailTmpl.Lookup(locale+"."+purpose+".text") == nil {
		locale = "en"
	}
	data := struct {
		Name, Code string
		Minutes    int
	}{name, code, minutes}
	var s, t bytes.Buffer
	if err = emailTmpl.ExecuteTemplate(&s, locale+"."+purpose+".subject", data); err != nil {
		return
	}
	if err = emailTmpl.ExecuteTemplate(&t, locale+"."+purpose+".text", data); err != nil {
		return
	}
	return s.String(), t.String(), nil
}

// sendCode issues a new one-time code for u (the previous one of that purpose dies) and emails
// it. The code only ever lives in Redis (as an HMAC) and in the email; errors returned here
// never contain it.
func (s *Service) sendCode(ctx context.Context, u User, purpose string) error {
	code, err := s.codes.Issue(ctx, purpose, u.InternalID, s.now().UTC())
	if err != nil {
		return err
	}
	subject, text, err := renderEmail(u.Locale, purpose, u.DisplayName, code, int(codeTTL(purpose)/time.Minute))
	if err != nil {
		return apperr.Wrap(apperr.Internal, err, "render "+purpose+" email")
	}
	ctx, cancel := context.WithTimeout(ctx, mailTimeout)
	defer cancel()
	if err := s.mail.Mailer.Send(ctx, mailer.Message{To: u.Email, Subject: subject, Text: text}); err != nil {
		return apperr.Wrap(apperr.Internal, err, "send "+purpose+" email")
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
