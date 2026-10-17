// Package mailer sends transactional email. The application depends on Mailer, Message and the two errors below and
// on nothing provider-specific; a provider is one file (log.go for development, ses.go for Amazon SES) and one case in New.
// Adding another means: a new file with its own SMEM_<PROVIDER>_* settings, one case in New, the production-config rows and
// the shared conformance test mailertest.Run (docs/email.md has the checklist).
package mailer

import (
	"context"
	"errors"
	"io"
	"log/slog"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Message is one email. HTML may be empty (plain text only). Providers send both parts as multipart/alternative.
//
// Message is passed by value on purpose: a small immutable value.
type Message struct {
	To, Subject, Text, HTML string
}

// Mailer delivers a Message. Implementations must not put the message, the recipient or a provider's error text in returned
// errors or logs, and classify a failure as ErrUnavailable or ErrRejected (see below) so callers never learn provider names.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// What a caller can tell apart; test with errors.Is. Anything else is an unexpected failure (a misconfiguration, bad credentials).
var (
	// ErrUnavailable: the provider could not take the message now (throttled, 5xx, timeout, network). Trying again later may work.
	ErrUnavailable = apperr.New(apperr.Unavailable, "mail provider unavailable")
	// ErrRejected: the provider refused this message for good (invalid or suppressed address, sending paused). Do not retry.
	ErrRejected = apperr.New(apperr.Param, "mail rejected by the provider")
)

// Failure classes for metrics and logs (label values of mail_send_failures_total).
const (
	ClassUnavailable = "unavailable"
	ClassRejected    = "rejected"
	ClassOther       = "other"
)

// Class names the failure class of err from Send.
func Class(err error) string {
	switch {
	case errors.Is(err, ErrUnavailable):
		return ClassUnavailable
	case errors.Is(err, ErrRejected):
		return ClassRejected
	}
	return ClassOther
}

// Config selects and configures the implementation. Only Provider and From are common; each provider reads its own part.
//
// Config is passed by value on purpose: start-up settings, read once.
type Config struct {
	Provider string // SMEM_MAIL_PROVIDER: "log" (default) or "ses"
	From     string // SMEM_MAIL_FROM: "SMemories <no-reply@mail.example.com>"
	Env      string // SMEM_ENV; the log mailer refuses anything but dev and test
	LogOut   io.Writer
	SES      SESConfig
}

// Provider names accepted by New.
const (
	ProviderLog = "log"
	ProviderSES = "ses"
)

// New picks the implementation from cfg.Provider. It is the only place that knows the provider names.
func New(cfg Config, logger *slog.Logger) (Mailer, error) {
	switch cfg.Provider {
	case "", ProviderLog:
		m, err := NewLog(cfg.Env, cfg.LogOut)
		if err != nil {
			return nil, err
		}
		return m, nil
	case ProviderSES:
		cfg.SES.From = cfg.From
		m, err := NewSES(cfg.SES, logger)
		if err != nil {
			return nil, err
		}
		return m, nil
	}
	return nil, apperr.New(apperr.Internal, "mailer: SMEM_MAIL_PROVIDER must be log or ses")
}
