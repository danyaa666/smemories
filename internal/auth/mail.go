package auth

import (
	"bytes"
	"context"
	"embed"
	"errors"
	htmltemplate "html/template"
	"log/slog"
	"text/template"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/mailer"
)

const (
	mailTimeout       = 10 * time.Second
	backgroundTimeout = 90 * time.Second // a detached job: up to 3 sends of mailTimeout and the waits between them
)

// retryDelays are the waits before the first and the second retry of a background send (at most two retries, only for
// mailer.ErrUnavailable; a refusal is final, and so is mailer.ClassConfig: a misconfiguration does not fix itself in seconds).
var retryDelays = []time.Duration{2 * time.Second, 5 * time.Second}

// Mail is what the service needs to send the verification and reset code emails.
//
// Mail is passed by value on purpose: dependencies bundled for the constructor, read once.
type Mail struct {
	Mailer mailer.Mailer
	Logger *slog.Logger
	// OnFailure, when set, is called once per email that could not be sent, with the mailer.Class* of the error
	// (the metric mail_send_failures_total{class}).
	OnFailure func(class string)
}

//go:embed emails/*.tmpl
var emailFS embed.FS

var (
	emailTmpl = template.Must(template.ParseFS(emailFS, "emails/en.tmpl", "emails/vi.tmpl"))
	htmlTmpl  = htmltemplate.Must(htmltemplate.ParseFS(emailFS, "emails/layout.html.tmpl"))
)

// renderEmail fills the subject, the plain-text body and the HTML body of the code email in the user's language (English when
// the locale is unknown). purpose is purposeVerify or purposeReset; minutes is the code's lifetime. The text and the HTML are
// built from the same sentences, so they never disagree. The greeting is generic on purpose: the display name is typed by whoever
// registered, before the address is proven, so it must not reach someone else's inbox (audit L2).
func renderEmail(locale, purpose, code string, minutes int) (subject, text, html string, err error) {
	if emailTmpl.Lookup(locale+"."+purpose+".subject") == nil {
		locale = "en"
	}
	data := struct {
		Minutes    int
		Lang, Code string
	}{minutes, locale, code}
	part := map[string]string{}
	for _, p := range []string{"subject", "hello", "intro", "how", "ignore"} {
		var b bytes.Buffer
		if err = emailTmpl.ExecuteTemplate(&b, locale+"."+purpose+"."+p, data); err != nil {
			return
		}
		part[p] = b.String()
	}
	text = part["hello"] + "\n\n" + part["intro"] + "\n\n" + code + "\n\n" + part["how"] + "\n\n" + part["ignore"] + "\n"
	var h bytes.Buffer
	err = htmlTmpl.ExecuteTemplate(&h, "layout.html.tmpl", struct{ Lang, Subject, Hello, Intro, Code, How, Ignore string }{
		locale, part["subject"], part["hello"], part["intro"], code, part["how"], part["ignore"],
	})
	return part["subject"], text, h.String(), err
}

// prepareCode issues a new one-time code for u (the previous one of that purpose dies) and renders the email carrying it. The
// code only ever lives in Redis (as an HMAC) and in the email; errors returned here never contain it.
func (s *Service) prepareCode(ctx context.Context, u *User, purpose string) (mailer.Message, error) {
	code, err := s.codes.Issue(ctx, purpose, u.InternalID, s.now().UTC())
	if err != nil {
		return mailer.Message{}, err
	}
	subject, text, html, err := renderEmail(u.Locale, purpose, code, int(codeTTL(purpose)/time.Minute))
	if err != nil {
		return mailer.Message{}, apperr.Wrap(apperr.Internal, err, "render "+purpose+" email")
	}
	return mailer.Message{To: u.Email, Subject: subject, Text: text, HTML: html}, nil
}

// deliver makes one bounded attempt to hand m to the mailer. The error keeps the mailer's class (errors.Is) for the caller.
func (s *Service) deliver(ctx context.Context, m mailer.Message, purpose string) error {
	ctx, cancel := context.WithTimeout(ctx, mailTimeout)
	defer cancel()
	if err := s.mail.Mailer.Send(ctx, m); err != nil {
		return apperr.Wrap(apperr.Internal, err, "send "+purpose+" email")
	}
	return nil
}

// sendCode issues a code and emails it, once, on the caller's request path (resend-verification: the user is waiting and sees
// the outcome).
func (s *Service) sendCode(ctx context.Context, u *User, purpose string) error {
	m, err := s.prepareCode(ctx, u, purpose)
	if err != nil {
		return err
	}
	return s.deliver(ctx, m, purpose)
}

// deliverWithRetry is deliver, repeated after each of retryDelays while the provider is unavailable (a refusal, an unknown
// failure or a failure of class config is final). The same message, and so the same code, goes out every time.
func (s *Service) deliverWithRetry(ctx context.Context, m mailer.Message, purpose string) error {
	err := s.deliver(ctx, m, purpose)
	for _, wait := range retryDelays {
		if !errors.Is(err, mailer.ErrUnavailable) || mailer.Class(err) == mailer.ClassConfig {
			return err
		}
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return err
		}
		err = s.deliver(ctx, m, purpose)
	}
	return err
}

// sendCodeLater issues the code and emails it in the background, so the request never waits for the mail provider. An
// unavailable provider is retried at most twice with a growing wait; whatever still fails is logged once (class and request id,
// never the address or the code) and counted. what names the email in the log line.
func (s *Service) sendCodeLater(ctx context.Context, u *User, purpose, what string) {
	s.background(ctx, func(ctx context.Context) {
		m, err := s.prepareCode(ctx, u, purpose)
		if err == nil {
			if err = s.deliverWithRetry(ctx, m, purpose); err == nil {
				return
			}
		}
		class := mailer.Class(err)
		s.mail.Logger.Error(what+" not sent", "request_id", httpx.RequestIDFrom(ctx), "user_id", u.ID, "class", class, "error", err)
		if s.mail.OnFailure != nil {
			s.mail.OnFailure(class)
		}
	})
}

// background runs f detached from the request (it keeps only the request id of parent, for the logs), so the caller's latency
// does not depend on whether there was anything to do. Wait blocks until all of them have finished.
//
// ponytail: no queue; a crash loses an in-flight email and the user asks again. Move to an
// outbox table when delivery must be guaranteed.
func (s *Service) background(parent context.Context, f func(context.Context)) {
	rid := httpx.RequestIDFrom(parent)
	s.bg.Add(1)
	s.bgActive.Add(1)
	go func() {
		defer s.bg.Done()
		defer s.bgActive.Add(-1)
		ctx, cancel := context.WithTimeout(httpx.WithRequestID(context.Background(), rid), backgroundTimeout)
		defer cancel()
		f(ctx)
	}()
}

// Wait blocks until the emails sent in the background have been handed to the mailer. Tests use it; shutdown uses Drain, which is bounded.
func (s *Service) Wait() { s.bg.Wait() }

// Drain is Wait bounded by ctx (the rest of the shutdown budget, SMEM_SHUTDOWN_TIMEOUT): a hung provider would otherwise keep the process
// alive for up to backgroundTimeout, past the platform's stop timeout, and the task would be killed. When ctx ends first, the emails still
// unsent are dropped: each is counted as mailer.ClassShutdown and one ERROR line says how many. It returns how many were dropped. The
// user of a dropped email asks again on the resend screen.
func (s *Service) Drain(ctx context.Context) (dropped int) {
	done := make(chan struct{})
	go func() { s.bg.Wait(); close(done) }()
	select {
	case <-done:
		return 0
	case <-ctx.Done():
	}
	dropped = int(s.bgActive.Load())
	if dropped > 0 {
		s.mail.Logger.Error("emails dropped at shutdown", "class", mailer.ClassShutdown, "count", dropped)
		if s.mail.OnFailure != nil {
			for range dropped {
				s.mail.OnFailure(mailer.ClassShutdown)
			}
		}
	}
	return dropped
}
