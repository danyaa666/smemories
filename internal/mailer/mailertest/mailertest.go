// Package mailertest is the conformance test every mailer.Mailer implementation runs (docs/email.md, "adding another provider").
// An implementation supplies a Fixture around its own stub of the provider; Run checks what any implementation must do.
package mailertest

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/mailer"
)

// Behaviour is what the provider stub does with the next sends.
type Behaviour int

const (
	Healthy       Behaviour = iota // accepts the message
	Throttled                      // "slow down" (HTTP 429 or the provider's equivalent)
	ServerError                    // the provider is failing (5xx)
	Refused                        // permanent refusal of this message because of its address (invalid or suppressed recipient)
	Misconfigured                  // refusal because of our account or setup (suspended or paused account, sender or sandbox recipient not verified)
	Hang                           // accepts the connection and never answers
)

// Fixture is one implementation under test, wired to its stub.
type Fixture struct {
	Mailer mailer.Mailer
	// Logs returns everything the mailer wrote to its logger so far.
	Logs func() string
	// Saw returns everything the provider received (the raw request bodies; the terminal output for the log mailer).
	Saw func() string
	// Behave sets how the stub answers from now on. Nil when the implementation cannot fail (the log mailer): the failure
	// subtests are skipped.
	Behave func(Behaviour)
}

// Values the tests send; distinctive so a leak is found by substring.
const (
	recipient = "leak.check.student@school-mail.invalid"
	code      = "913571"
	textMark  = "plain-part-marker"
	htmlMark  = "html-part-marker"
	subject   = "Subject marker"
)

func message() mailer.Message {
	return mailer.Message{
		To: recipient, Subject: subject,
		Text: textMark + " your code is " + code,
		HTML: "<html><body><p>" + htmlMark + " your code is <b>" + code + "</b></p></body></html>",
	}
}

// Run executes the conformance tests; newFixture builds a fresh Fixture (and registers its own cleanup) per subtest.
func Run(t *testing.T, newFixture func(t *testing.T) Fixture) {
	t.Helper()

	t.Run("sends the text and the HTML part", func(t *testing.T) {
		f := newFixture(t)
		if err := f.Mailer.Send(context.Background(), message()); err != nil {
			t.Fatalf("Send: %v", err)
		}
		saw := f.Saw()
		for _, want := range []string{recipient, subject, textMark, htmlMark} {
			if !strings.Contains(saw, want) {
				t.Errorf("the provider did not receive %q", want)
			}
		}
		noLeak(t, f, nil)
	})

	t.Run("a cancelled context is an error, not a send", func(t *testing.T) {
		f := newFixture(t)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := f.Mailer.Send(ctx, message()); err == nil {
			t.Fatal("Send with a cancelled context succeeded")
		}
		if strings.Contains(f.Saw(), textMark) {
			t.Error("a cancelled send reached the provider")
		}
	})

	failures := []struct {
		name      string
		b         Behaviour
		want      error
		wantClass string
	}{
		{"throttling is ErrUnavailable", Throttled, mailer.ErrUnavailable, mailer.ClassUnavailable},
		{"a 5xx is ErrUnavailable", ServerError, mailer.ErrUnavailable, mailer.ClassUnavailable},
		{"a refused address is ErrRejected", Refused, mailer.ErrRejected, mailer.ClassRejected},
		// An account or setup problem is not the student's address: a loud 503 and its own class for the alarm, never ErrRejected.
		{"an account or setup problem is ErrUnavailable with its own class", Misconfigured, mailer.ErrUnavailable, mailer.ClassConfig},
	}
	for _, c := range failures {
		t.Run(c.name, func(t *testing.T) {
			f := newFixture(t)
			if f.Behave == nil {
				t.Skip("this implementation cannot fail")
			}
			f.Behave(c.b)
			err := f.Mailer.Send(context.Background(), message())
			if !errors.Is(err, c.want) {
				t.Fatalf("Send = %v, want %v", err, c.want)
			}
			if c.want == mailer.ErrRejected && errors.Is(err, mailer.ErrUnavailable) || c.want == mailer.ErrUnavailable && errors.Is(err, mailer.ErrRejected) {
				t.Errorf("the error matches both classes: %v", err)
			}
			if got := mailer.Class(err); got != c.wantClass {
				t.Errorf("Class = %q, want %q", got, c.wantClass)
			}
			noLeak(t, f, err)
		})
	}

	t.Run("honours the context deadline", func(t *testing.T) {
		f := newFixture(t)
		if f.Behave == nil {
			t.Skip("this implementation cannot fail")
		}
		f.Behave(Hang)
		const deadline, slack = 300 * time.Millisecond, 5 * time.Second
		ctx, cancel := context.WithTimeout(context.Background(), deadline)
		defer cancel()
		start := time.Now()
		err := f.Mailer.Send(ctx, message())
		if took := time.Since(start); took > slack {
			t.Fatalf("Send took %s with a %s deadline", took, deadline)
		}
		if !errors.Is(err, mailer.ErrUnavailable) {
			t.Fatalf("Send = %v, want ErrUnavailable (a timeout)", err)
		}
		noLeak(t, f, err)
	})
}

// noLeak asserts that neither the logs nor the error text carry the recipient or the code.
func noLeak(t *testing.T, f Fixture, err error) {
	t.Helper()
	texts := map[string]string{"logs": f.Logs()}
	if err != nil {
		texts["error text"] = err.Error()
	}
	for where, s := range texts {
		for _, secret := range []string{recipient, code, textMark, htmlMark} {
			if strings.Contains(s, secret) {
				t.Errorf("%s contain %q: %s", where, secret, s)
			}
		}
	}
}
