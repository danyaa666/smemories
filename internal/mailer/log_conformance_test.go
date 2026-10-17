package mailer_test

import (
	"bytes"
	"testing"

	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/mailer/mailertest"
)

// The log mailer runs the same suite; it cannot fail, so the failure subtests skip.
func TestLogMailerConformance(t *testing.T) {
	mailertest.Run(t, func(t *testing.T) mailertest.Fixture {
		var out bytes.Buffer
		m, err := mailer.NewLog("test", &out)
		if err != nil {
			t.Fatal(err)
		}
		return mailertest.Fixture{Mailer: m, Logs: func() string { return "" }, Saw: out.String}
	})
}
