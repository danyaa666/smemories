//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/legal"
	"github.com/danyaa666/smemories/internal/mailer"
)

// scriptMailer answers each Send with the next error of script (nil = success; the last entry repeats), records the messages and
// blocks for `delay` first (or until release is closed).
type scriptMailer struct {
	script  []error
	delay   time.Duration
	release chan struct{}

	mu   sync.Mutex
	msgs []mailer.Message
}

func (m *scriptMailer) Send(ctx context.Context, msg mailer.Message) error {
	m.mu.Lock()
	n := len(m.msgs)
	m.msgs = append(m.msgs, msg)
	m.mu.Unlock()
	if m.delay > 0 {
		select {
		case <-time.After(m.delay):
		case <-m.release:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	if len(m.script) == 0 {
		return nil
	}
	return m.script[min(n, len(m.script)-1)]
}

func (m *scriptMailer) sent() int { m.mu.Lock(); defer m.mu.Unlock(); return len(m.msgs) }

// fastRetries shortens the waits between the background retries for a test.
func fastRetries(t *testing.T) {
	t.Helper()
	old := retryDelays
	retryDelays = []time.Duration{5 * time.Millisecond, 10 * time.Millisecond}
	t.Cleanup(func() { retryDelays = old })
}

// T-021 AC2: resend-verification tells the user when the provider is down (503 with Retry-After, and the try is given back) or
// refuses the address (400).
func TestResendMapsMailerErrors(t *testing.T) {
	sm := &scriptMailer{}
	e := newEnv(t, opts{mailer: sm})
	want(t, e.register(testMail, testPW, "Alice"), 200, "")
	c := sessionCookie(t, e.login(testMail, testPW, ""))
	e.svc.Wait()

	sm.script = []error{mailer.ErrUnavailable}
	sm.msgs = nil
	for i := 0; i < 5; i++ { // more than the 3 tries an hour: an outage does not spend them
		rec := e.post("/api/auth/resend-verification", "", c.Value)
		want(t, rec, 503, "ERROR_MAIL_UNAVAILABLE")
		if rec.Header().Get("Retry-After") == "" {
			t.Fatal("Retry-After missing")
		}
	}
	sm.script = []error{mailer.ErrRejected}
	rec := e.post("/api/auth/resend-verification", "", c.Value)
	want(t, rec, 400, "ERROR_EMAIL_UNDELIVERABLE")
	if rec.Header().Get("Retry-After") != "" {
		t.Error("a refusal is final: no Retry-After")
	}
	for _, s := range []string{rec.Body.String(), e.logs.String()} {
		if strings.Contains(s, testMail) {
			t.Errorf("the address leaked into a body or log: %s", s)
		}
	}
	sm.script = nil
	want(t, e.post("/api/auth/resend-verification", "", c.Value), 200, "")
}

// T-021 AC2: registration and forgot-password answer at once while the provider takes 5 s, and forgot-password answers the same
// for a known, an unknown and a refused address.
func TestSlowProviderDoesNotSlowTheAnswers(t *testing.T) {
	sm := &scriptMailer{delay: 5 * time.Second, release: make(chan struct{})}
	e := newEnv(t, opts{mailer: sm})
	t.Cleanup(func() { close(sm.release); e.svc.Wait() })

	start := time.Now()
	want(t, e.register(testMail, testPW, "Alice"), 200, "")
	known := e.forgot(testMail, "")
	unknown := e.forgot("nobody@example.com", "")
	if took := time.Since(start); took > 2*time.Second {
		t.Fatalf("register and two forgot-password calls took %s with a 5 s provider", took)
	}
	want(t, known, 200, "")
	want(t, unknown, 200, "")
	if known.Body.String() != unknown.Body.String() {
		t.Fatalf("bodies differ: %q vs %q", known.Body.String(), unknown.Body.String())
	}
}

func TestForgotPasswordSameAnswerForRefusedAddress(t *testing.T) {
	fastRetries(t)
	sm := &scriptMailer{script: []error{mailer.ErrRejected}}
	e := newEnv(t, opts{mailer: sm})
	want(t, e.register(testMail, testPW, "Alice"), 200, "")
	e.svc.Wait()
	refused := e.forgot(testMail, "")
	unknown := e.forgot("nobody@example.com", "")
	e.svc.Wait()
	want(t, refused, 200, "")
	if refused.Body.String() != unknown.Body.String() || refused.Code != unknown.Code {
		t.Fatalf("a refused address is distinguishable: %d %q vs %d %q", refused.Code, refused.Body.String(), unknown.Code, unknown.Body.String())
	}
}

// T-021: a background send is retried at most twice and only when the provider is unavailable; what still fails is logged once
// (request id, class, no address, no code) and counted.
func TestBackgroundRetryPolicy(t *testing.T) {
	fastRetries(t)
	unavailable := apperr.Wrap(apperr.Internal, mailer.ErrUnavailable, "x")
	for _, c := range []struct {
		name       string
		script     []error
		wantSends  int
		wantFailed string // class counted, "" = none
	}{
		{"works at once", nil, 1, ""},
		{"recovers on the second try", []error{unavailable, nil}, 2, ""},
		{"recovers on the third try", []error{unavailable, unavailable, nil}, 3, ""},
		{"gives up after two retries", []error{unavailable}, 3, "unavailable"},
		{"a misconfiguration is not retried", []error{mailer.ConfigError("account paused", nil)}, 1, "config"},
		{"a refusal is not retried", []error{mailer.ErrRejected}, 1, "rejected"},
		{"an unknown failure is not retried", []error{apperr.New(apperr.Internal, "boom")}, 1, "other"},
	} {
		t.Run(c.name, func(t *testing.T) {
			var failed []string
			var mu sync.Mutex
			sm := &scriptMailer{script: c.script}
			e := newEnv(t, opts{mailer: sm, onFailure: func(class string) { mu.Lock(); failed = append(failed, class); mu.Unlock() }})
			rec := e.do(req{method: "POST", path: "/api/auth/register", body: regBody(testMail, testPW, "Alice"), headers: map[string]string{"X-Request-Id": "req-abcdef123456"}})
			want(t, rec, 200, "")
			e.svc.Wait()
			if got := sm.sent(); got != c.wantSends {
				t.Errorf("%d sends, want %d", got, c.wantSends)
			}
			logs := e.logs.String()
			n := strings.Count(logs, "verification email not sent")
			if c.wantFailed == "" {
				if n != 0 || len(failed) != 0 {
					t.Errorf("unexpected failure report: %d log lines, counted %v", n, failed)
				}
				return
			}
			if n != 1 || len(failed) != 1 || failed[0] != c.wantFailed {
				t.Fatalf("%d log lines, counted %v, want one of class %s\n%s", n, failed, c.wantFailed, logs)
			}
			if !strings.Contains(logs, `"request_id":"req-abcdef123456"`) || !strings.Contains(logs, `"class":"`+c.wantFailed+`"`) {
				t.Errorf("the failure line lacks the request id or the class:\n%s", logs)
			}
			sm.mu.Lock()
			code := codeRE.FindString(sm.msgs[0].Text)
			sm.mu.Unlock()
			if strings.Contains(logs, testMail) || strings.Contains(logs, code) {
				t.Errorf("the address or the code is in the logs:\n%s", logs)
			}
		})
	}
}

// The background retries of one email use one code: the user who finally gets the mail types the code in it.
func TestRetriesCarryTheSameCode(t *testing.T) {
	fastRetries(t)
	sm := &scriptMailer{script: []error{mailer.ErrUnavailable, mailer.ErrUnavailable, nil}}
	e := newEnv(t, opts{mailer: sm})
	want(t, e.register(testMail, testPW, "Alice"), 200, "")
	e.svc.Wait()
	if sm.sent() != 3 || sm.msgs[0].Text != sm.msgs[2].Text || sm.msgs[0].HTML != sm.msgs[2].HTML {
		t.Fatalf("%d sends; the retried message differs", sm.sent())
	}
	c := sessionCookie(t, e.login(testMail, testPW, ""))
	want(t, e.verify(codeRE.FindString(sm.msgs[2].Text), c.Value), 200, "")
}

// Both parts reach the mailer.
func TestMessagesCarryTextAndHTML(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 200, "")
	e.svc.Wait()
	m := e.mail.all()[0]
	if !strings.Contains(m.HTML, "<h1") || !strings.Contains(m.HTML, codeRE.FindString(m.Text)) {
		t.Fatalf("message: %+v", m)
	}
}

// T-101 AC2 (audit L2): the display name is typed by whoever registers, before the address is proven, so neither the verification
// nor the reset email may carry it, in any part and in either language.
func TestCodeEmailsNeverCarryTheDisplayName(t *testing.T) {
	const hostile = "Click evil.example"
	for _, locale := range []string{"en", "vi"} {
		e := newEnv(t, opts{})
		body, _ := json.Marshal(map[string]string{"email": testMail, "password": testPW, "display_name": hostile, "locale": locale, "accept_terms_version": legal.CurrentVersion})
		want(t, e.do(req{method: "POST", path: "/api/auth/register", body: string(body)}), 200, "")
		e.svc.Wait()
		want(t, e.forgot(testMail, ""), 200, "")
		e.svc.Wait()
		msgs := e.mail.all()
		if len(msgs) != 2 {
			t.Fatalf("%s: want a verification and a reset email, got %d", locale, len(msgs))
		}
		for i, m := range msgs {
			for _, part := range []string{m.Subject, m.Text, m.HTML} {
				if strings.Contains(part, "evil") || strings.Contains(part, "Click") {
					t.Errorf("%s message %d carries the display name:\n%s", locale, i, part)
				}
			}
			if codeRE.FindString(m.Text) == "" {
				t.Errorf("%s message %d lost its code: %q", locale, i, m.Text)
			}
		}
	}
}

// T-090 item 12: the try given back during an outage is capped (5 an hour per user), so one user cannot keep hitting a throttling
// provider: 5 refunded sends plus the 3 an hour, then the ordinary 429.
func TestResendRefundIsCapped(t *testing.T) {
	sm := &scriptMailer{}
	e := newEnv(t, opts{mailer: sm})
	want(t, e.register(testMail, testPW, "Alice"), 200, "")
	c := sessionCookie(t, e.login(testMail, testPW, ""))
	e.svc.Wait()

	sm.script = []error{mailer.ErrUnavailable}
	sm.mu.Lock()
	sm.msgs = nil
	sm.mu.Unlock()
	var codes []int
	for range 12 {
		codes = append(codes, e.post("/api/auth/resend-verification", "", c.Value).Code)
	}
	wantCodes := []int{503, 503, 503, 503, 503, 503, 503, 503, 429, 429, 429, 429} // 5 refunded + 3 counted, then rate limited
	for i := range wantCodes {
		if codes[i] != wantCodes[i] {
			t.Fatalf("statuses %v, want %v", codes, wantCodes)
		}
	}
	if got := sm.sent(); got != resendRefundsHour+resendPerHour {
		t.Errorf("%d sends reached the provider, want %d", got, resendRefundsHour+resendPerHour)
	}
}

// T-090 item 10: Drain waits for the background emails only as long as its context allows, drops and counts what is left (class
// shutdown) and returns at once when nothing is pending.
func TestDrainIsBoundedAndCountsTheDropped(t *testing.T) {
	var mu sync.Mutex
	var failed []string
	sm := &scriptMailer{delay: time.Minute, release: make(chan struct{})}
	e := newEnv(t, opts{mailer: sm, onFailure: func(class string) { mu.Lock(); failed = append(failed, class); mu.Unlock() }})
	want(t, e.register(testMail, testPW, "Alice"), 200, "")
	want(t, e.register("second@example.com", testPW, "Bob"), 200, "")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	dropped := e.svc.Drain(ctx)
	if took := time.Since(start); took > 3*time.Second {
		t.Fatalf("Drain took %s with a 200 ms budget", took)
	}
	mu.Lock()
	got := append([]string(nil), failed...)
	mu.Unlock()
	if dropped != 2 || len(got) != 2 || got[0] != mailer.ClassShutdown || got[1] != mailer.ClassShutdown {
		t.Fatalf("dropped %d, counted %v; want 2 dropped, both class %q", dropped, got, mailer.ClassShutdown)
	}
	logs := e.logs.String()
	if strings.Count(logs, "emails dropped at shutdown") != 1 || !strings.Contains(logs, `"class":"shutdown"`) || strings.Contains(logs, testMail) {
		t.Errorf("want one ERROR line with the class and no address:\n%s", logs)
	}

	close(sm.release) // the provider answers: nothing is pending, Drain returns 0 at once
	e.svc.Wait()
	if n := e.svc.Drain(context.Background()); n != 0 {
		t.Errorf("Drain with nothing pending dropped %d", n)
	}
}
