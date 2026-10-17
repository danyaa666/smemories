package mailer

import (
	"context"
	"fmt"
	"io"
	"sync"

	"github.com/danyaa666/smemories/internal/apperr"
)

// LogMailer prints the whole message, codes included, so a developer can read them. That is a secret leak by design, so NewLog
// refuses to exist outside dev and test. It writes to its own writer (the developer's terminal), never to the application log.
type LogMailer struct {
	mu sync.Mutex
	w  io.Writer
}

// NewLog returns a LogMailer writing to w, or an error when env (SMEM_ENV) is not dev or test.
func NewLog(env string, w io.Writer) (*LogMailer, error) {
	if env != "dev" && env != "test" {
		return nil, apperr.New(apperr.Internal, fmt.Sprintf("mailer: SMEM_ENV=%q has no real mailer configured and the log mailer prints secrets; set SMEM_MAIL_PROVIDER=ses", env))
	}
	return &LogMailer{w: w}, nil
}

func (l *LogMailer) Send(ctx context.Context, m Message) error {
	if err := ctx.Err(); err != nil { // like a real provider: no work for a caller that has gone
		return classify(ErrUnavailable, "context", err)
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	body := "---- email (log mailer) ----\nTo: %s\nSubject: %s\n\n%s\n"
	args := []any{m.To, m.Subject, m.Text}
	if m.HTML != "" {
		body += "---- html part ----\n%s\n"
		args = append(args, m.HTML)
	}
	_, err := fmt.Fprintf(l.w, body+"---- end email ----\n", args...)
	return err
}
