package auth

import (
	"context"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/mailer"
)

type failingMailer struct {
	err   error
	sends int
}

func (m *failingMailer) Send(context.Context, mailer.Message) error { m.sends++; return m.err }

// T-091: a background send is retried while the provider is merely unavailable, but never for class config (the operator must act,
// seconds will not help), a refusal or an unknown failure.
func TestDeliverWithRetryStopsOnConfigClass(t *testing.T) {
	old := retryDelays
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryDelays = old })
	for _, c := range []struct {
		name  string
		err   error
		sends int
	}{
		{"unavailable is retried twice", apperr.Wrap(apperr.Internal, mailer.ErrUnavailable, "x"), 3},
		{"config is not retried", mailer.ConfigError("account paused", nil), 1},
		{"a refusal is not retried", mailer.ErrRejected, 1},
	} {
		t.Run(c.name, func(t *testing.T) {
			fm := &failingMailer{err: c.err}
			s := &Service{mail: Mail{Mailer: fm}}
			if err := s.deliverWithRetry(context.Background(), mailer.Message{}, purposeVerify); err == nil {
				t.Fatal("want the failure back")
			}
			if fm.sends != c.sends {
				t.Errorf("%d sends, want %d", fm.sends, c.sends)
			}
		})
	}
}
