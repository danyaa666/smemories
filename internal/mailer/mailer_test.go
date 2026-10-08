package mailer

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func TestLogMailerOnlyInDevAndTest(t *testing.T) {
	for _, env := range []string{"prod", "", "staging"} {
		if _, err := NewLog(env, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "no real mailer") {
			t.Errorf("SMEM_ENV=%q: want a clear refusal, got %v", env, err)
		}
	}
	for _, env := range []string{"dev", "test"} {
		var buf bytes.Buffer
		m, err := NewLog(env, &buf)
		if err != nil {
			t.Fatalf("SMEM_ENV=%s: %v", env, err)
		}
		if err := m.Send(context.Background(), Message{To: "a@example.com", Subject: "Hi", Text: "click http://x/?token=abc"}); err != nil {
			t.Fatal(err)
		}
		for _, s := range []string{"a@example.com", "Hi", "http://x/?token=abc"} {
			if !strings.Contains(buf.String(), s) {
				t.Errorf("output lacks %q: %s", s, buf.String())
			}
		}
	}
}
