// Package mailer sends transactional email. M1 has only the development LogMailer; a real
// provider is an M2 decision (it plugs in behind Mailer and is chosen in cmd/smemories-api).
package mailer

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/danyaa666/smemories/internal/apperr"
)

// Message is one email. HTML may be empty (plain text only).
//
// Message is passed by value on purpose: a small immutable value.
type Message struct {
	To, Subject, Text, HTML string
}

// Mailer delivers a Message. Implementations must not put the message body in returned errors.
type Mailer interface {
	Send(ctx context.Context, m Message) error
}

// LogMailer prints the whole message, links and tokens included, so a developer can click
// them. That is a secret leak by design, so NewLog refuses to exist outside dev and test.
type LogMailer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewLog returns a LogMailer writing to w, or an error when env (SMEM_ENV) is not dev or test.
func NewLog(env string, w io.Writer) (*LogMailer, error) {
	if env != "dev" && env != "test" {
		return nil, apperr.New(apperr.Internal, fmt.Sprintf("mailer: SMEM_ENV=%q has no real mailer configured and the log mailer prints secrets; configure a mailer provider (none exists yet)", env))
	}
	return &LogMailer{w: w}, nil
}

func (l *LogMailer) Send(_ context.Context, m Message) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	_, err := fmt.Fprintf(l.w, "---- email (log mailer) ----\nTo: %s\nSubject: %s\n\n%s\n---- end email ----\n", m.To, m.Subject, m.Text)
	return err
}
