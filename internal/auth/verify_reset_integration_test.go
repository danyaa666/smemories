//go:build integration

package auth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/mailer"
)

const testBase = "https://app.example.com"

// recMailer records every message, including ones it then fails to deliver.
type recMailer struct {
	mu   sync.Mutex
	msgs []mailer.Message
	fail bool
}

func (m *recMailer) Send(_ context.Context, msg mailer.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.msgs = append(m.msgs, msg)
	if m.fail {
		return errors.New("smtp down")
	}
	return nil
}

func (m *recMailer) all() []mailer.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]mailer.Message(nil), m.msgs...)
}

var tokenRE = regexp.MustCompile(`(/verify-email|/reset-password)\?token=([A-Za-z0-9_-]{43})\b`)

// last returns the path ("/verify-email" or "/reset-password") and token of the newest mail to `to`.
func (e *env) last(to string) (path, token string) {
	e.t.Helper()
	e.svc.Wait()
	msgs := e.mail.all()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].To == to {
			m := tokenRE.FindStringSubmatch(msgs[i].Text)
			if m == nil || !strings.Contains(msgs[i].Text, testBase+m[0]) {
				e.t.Fatalf("no %s link in:\n%s", testBase, msgs[i].Text)
			}
			return m[1], m[2]
		}
	}
	e.t.Fatalf("no mail to %s", to)
	return
}

func (e *env) mailCount(to string) int {
	e.svc.Wait()
	n := 0
	for _, m := range e.mail.all() {
		if m.To == to {
			n++
		}
	}
	return n
}

func (e *env) post(path, body string, cookie string) *httptest.ResponseRecorder {
	return e.do(req{method: "POST", path: path, body: body, cookie: cookie, origin: origin})
}

func (e *env) verify(token string) *httptest.ResponseRecorder {
	return e.post("/v1/auth/verify-email", fmt.Sprintf(`{"token":%q}`, token), "")
}

func (e *env) forgot(email, remote string) *httptest.ResponseRecorder {
	return e.do(req{method: "POST", path: "/v1/auth/forgot-password", body: fmt.Sprintf(`{"email":%q}`, email), remote: remote})
}

func (e *env) reset(token, pw string) *httptest.ResponseRecorder {
	return e.post("/v1/auth/reset-password", fmt.Sprintf(`{"token":%q,"password":%q}`, token, pw), "")
}

const newPW = "Another-fake-pw-for-tests-42"

// AC2, AC4: registering sends a verification mail; the token is stored hashed and works once.
func TestRegisterSendsVerificationAndVerifyIsSingleUse(t *testing.T) {
	e := newEnv(t, opts{})
	rec := e.register(testMail, testPW, "Alice")
	want(t, rec, 201, "")
	c := sessionCookie(t, rec)
	path, token := e.last(testMail)
	if path != "/verify-email" {
		t.Fatalf("path %s", path)
	}
	sum := sha256.Sum256([]byte(token))
	if e.count("SELECT COUNT(*) FROM email_tokens WHERE token_hash = ? AND purpose = 'verify' AND used_at IS NULL AND expires_at = created_at + INTERVAL 24 HOUR", sum[:]) != 1 {
		t.Fatal("expected one verify token stored as sha256 with a 24 h lifetime")
	}
	if e.count("SELECT COUNT(*) FROM email_tokens WHERE token_hash = ?", []byte(token)) != 0 {
		t.Fatal("raw token stored")
	}

	want(t, e.verify(token), 204, "")
	me := e.do(req{method: "GET", path: "/v1/me", cookie: c.Value})
	if !strings.Contains(me.Body.String(), `"email_verified":true`) {
		t.Fatalf("not verified: %s", me.Body.String())
	}
	want(t, e.verify(token), 400, "invalid_token") // reuse
	if strings.Contains(e.logs.String(), token) {
		t.Fatal("token in logs")
	}
}

// AC4: unknown, expired and wrong-purpose tokens all look the same.
func TestVerifyRejectsBadTokens(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	_, vtok := e.last(testMail)
	want(t, e.verify("nope"), 400, "invalid_token")
	want(t, e.verify(""), 400, "invalid_token")
	want(t, e.post("/v1/auth/verify-email", `{}`, ""), 400, "invalid_token")
	want(t, e.post("/v1/auth/verify-email", `not json`, ""), 400, "invalid_body")

	// a reset token must not verify, and a verify token must not reset
	want(t, e.forgot(testMail, ""), 202, "")
	_, rtok := e.last(testMail)
	want(t, e.verify(rtok), 400, "invalid_token")
	want(t, e.reset(vtok, newPW), 400, "invalid_token")
	if e.count("SELECT COUNT(*) FROM email_tokens WHERE used_at IS NOT NULL") != 0 {
		t.Fatal("a rejected token was consumed")
	}

	e.clock.advance(24*time.Hour + time.Second)
	want(t, e.verify(vtok), 400, "invalid_token") // expired
	if e.count("SELECT COUNT(*) FROM users WHERE email_verified_at IS NOT NULL") != 0 {
		t.Fatal("expired token verified the user")
	}
}

// AC2: a mailer failure never fails registration and never logs the token.
func TestRegisterSurvivesMailerFailure(t *testing.T) {
	e := newEnv(t, opts{})
	e.mail.fail = true
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	_, token := e.last(testMail) // the recorder kept the attempted message
	logs := e.logs.String()
	if !strings.Contains(logs, "verification email not sent") {
		t.Fatalf("failure not logged:\n%s", logs)
	}
	if strings.Contains(logs, token) {
		t.Fatal("token in logs")
	}
	e.mail.fail = false
	c := sessionCookie(t, e.login(testMail, testPW, ""))
	want(t, e.post("/v1/auth/verify-email/resend", "", c.Value), 202, "") // user can resend
	if e.mailCount(testMail) != 2 {
		t.Fatalf("mails: %d", e.mailCount(testMail))
	}
}

// AC3
func TestResendVerification(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	want(t, e.do(req{method: "POST", path: "/v1/auth/verify-email/resend", origin: origin}), 401, "unauthenticated")
	want(t, e.do(req{method: "POST", path: "/v1/auth/verify-email/resend", cookie: c.Value}), 403, "csrf_origin_mismatch")
	for i := 0; i < 3; i++ {
		rec := e.post("/v1/auth/verify-email/resend", "", c.Value)
		want(t, rec, 202, "")
		if rec.Body.String() != "{}\n" && rec.Body.String() != "{}" {
			t.Fatalf("body %q", rec.Body.String())
		}
	}
	rec := e.post("/v1/auth/verify-email/resend", "", c.Value)
	want(t, rec, 429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}
	e.clock.advance(time.Hour + time.Second)
	want(t, e.post("/v1/auth/verify-email/resend", "", c.Value), 202, "") // window passed
	_, token := e.last(testMail)
	// the newest link works; then the user is verified and resend says so without mailing
	want(t, e.verify(token), 204, "")
	before := e.mailCount(testMail)
	rec = e.post("/v1/auth/verify-email/resend", "", c.Value)
	want(t, rec, 200, "")
	if !strings.Contains(rec.Body.String(), `"already_verified":true`) || e.mailCount(testMail) != before {
		t.Fatalf("verified user: %s, mails %d->%d", rec.Body.String(), before, e.mailCount(testMail))
	}
}

// AC5
func TestForgotPasswordDoesNotRevealAccounts(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	known := e.forgot(testMail, "")
	unknown := e.forgot("nobody@example.com", "")
	want(t, known, 202, "")
	want(t, unknown, 202, "")
	if known.Body.String() != unknown.Body.String() {
		t.Fatalf("bodies differ: %q vs %q", known.Body.String(), unknown.Body.String())
	}
	if e.mailCount("nobody@example.com") != 0 || e.mailCount(testMail) != 2 { // register + reset
		t.Fatalf("mails: known %d unknown %d", e.mailCount(testMail), e.mailCount("nobody@example.com"))
	}
	if e.count("SELECT COUNT(*) FROM email_tokens WHERE purpose = 'reset'") != 1 {
		t.Fatal("expected exactly one reset token")
	}
	// the address is normalised like at login
	want(t, e.forgot("  ALICE@Example.com ", "192.0.2.9:1"), 202, "")
	if e.mailCount(testMail) != 3 {
		t.Fatal("upper-case address did not match the account")
	}
	want(t, e.forgot("not-an-email", ""), 400, "invalid_email")
}

// AC5: 5 per hour per IP and 3 per hour per address, applied the same to unknown addresses.
func TestForgotPasswordRateLimits(t *testing.T) {
	e := newEnv(t, opts{})
	for i := 0; i < 5; i++ {
		want(t, e.forgot(fmt.Sprintf("u%d@example.com", i), "192.0.2.1:1"), 202, "")
	}
	rec := e.forgot("u9@example.com", "192.0.2.1:1")
	want(t, rec, 429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}
	for i := 0; i < 3; i++ { // per address, from different IPs; the address does not exist
		want(t, e.forgot("ghost@example.com", fmt.Sprintf("192.0.2.%d:1", 20+i)), 202, "")
	}
	want(t, e.forgot("ghost@example.com", "192.0.2.99:1"), 429, "rate_limited")
	e.clock.advance(time.Hour + time.Second)
	want(t, e.forgot("ghost@example.com", "192.0.2.1:1"), 202, "")
}

// AC6: the whole reset flow, including session removal and single use.
func TestResetPasswordFlow(t *testing.T) {
	e := newEnv(t, opts{})
	c1 := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	c2 := sessionCookie(t, e.login(testMail, testPW, ""))
	want(t, e.forgot(testMail, ""), 202, "")
	path, token := e.last(testMail)
	if path != "/reset-password" {
		t.Fatalf("path %s", path)
	}

	// a weak password is refused and does not burn the link
	want(t, e.reset(token, "short"), 400, "weak_password")
	want(t, e.reset(token, testMail), 400, "weak_password")
	want(t, e.reset("bogus", newPW), 400, "invalid_token")

	want(t, e.reset(token, newPW), 204, "")
	for _, c := range []string{c1.Value, c2.Value} {
		want(t, e.do(req{method: "GET", path: "/v1/me", cookie: c}), 401, "unauthenticated")
	}
	if e.count("SELECT COUNT(*) FROM sessions") != 0 {
		t.Fatal("sessions left")
	}
	want(t, e.login(testMail, testPW, ""), 401, "invalid_credentials")
	want(t, e.login(testMail, newPW, ""), 200, "")
	want(t, e.reset(token, "Third-fake-password-99"), 400, "invalid_token") // reuse
	want(t, e.login(testMail, newPW, ""), 200, "")
	if strings.Contains(e.logs.String(), token) || strings.Contains(e.logs.String(), newPW) {
		t.Fatal("secret in logs")
	}
}

func TestResetTokenExpiresAfterAnHour(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	want(t, e.forgot(testMail, ""), 202, "")
	_, token := e.last(testMail)
	e.clock.advance(time.Hour - time.Second)
	want(t, e.reset("x"+token[1:], newPW), 400, "invalid_token") // a near miss is just unknown
	e.clock.advance(2 * time.Second)
	want(t, e.reset(token, newPW), 400, "invalid_token")
	want(t, e.login(testMail, testPW, ""), 200, "") // password unchanged
}

func TestUsingOneResetLinkRetiresTheOthers(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	want(t, e.forgot(testMail, "10.0.0.1:1"), 202, "")
	_, a := e.last(testMail)
	want(t, e.forgot(testMail, "10.0.0.2:1"), 202, "")
	_, b := e.last(testMail)
	want(t, e.reset(b, newPW), 204, "")
	want(t, e.reset(a, "Yet-another-fake-pw-1"), 400, "invalid_token")
	want(t, e.login(testMail, newPW, ""), 200, "")
}

// AC6 + QA probe: one token used twice at once has exactly one winner.
func TestConcurrentResetHasOneWinner(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	want(t, e.forgot(testMail, ""), 202, "")
	_, token := e.last(testMail)
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- e.reset(token, fmt.Sprintf("Concurrent-fake-pw-%d-xx", i)).Code }()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		if c == 204 {
			ok++
		} else if c != 400 {
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok != 1 {
		t.Fatalf("%d winners, want 1", ok)
	}
}

// AC7: users.locale picks the language; links come from SMEM_PUBLIC_BASE_URL.
func TestEmailLocale(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Hồng"), 201, "")
	if _, err := e.db.Exec("UPDATE users SET locale = 'vi'"); err != nil {
		t.Fatal(err)
	}
	want(t, e.forgot(testMail, ""), 202, "")
	e.svc.Wait()
	msgs := e.mail.all()
	got := msgs[len(msgs)-1]
	if !strings.Contains(got.Subject, "Đặt lại mật khẩu") || !strings.Contains(got.Text, testBase+"/reset-password?token=") || !strings.Contains(got.Text, "Hồng") {
		t.Fatalf("not the Vietnamese reset email: %+v", got)
	}
	if first := msgs[0]; !strings.Contains(first.Subject, "email address") {
		t.Fatalf("registration mail should be English (default locale): %q", first.Subject)
	}
}

// AC8
func TestCleanupRemovesExpiredAndUsedTokens(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "") // one live verify token
	now := e.clock.now()
	ins := func(b byte, exp time.Time, used any) {
		h := sha256.Sum256([]byte{b})
		if _, err := e.db.Exec(`INSERT INTO email_tokens (token_hash, user_id, purpose, expires_at, used_at, created_at)
			SELECT ?, id, 'reset', ?, ?, ? FROM users`, h[:], exp, used, now); err != nil {
			t.Fatal(err)
		}
	}
	ins(1, now.Add(-time.Second), nil) // expired
	ins(2, now.Add(time.Hour), now)    // used
	ins(3, now.Add(time.Hour), nil)    // live
	n, err := e.svc.CleanupTokens(context.Background())
	if err != nil || n != 2 {
		t.Fatalf("removed %d, %v", n, err)
	}
	if e.count("SELECT COUNT(*) FROM email_tokens") != 2 {
		t.Fatal("live tokens must stay")
	}

	// RunCleanup does it at startup
	ins(4, now.Add(-time.Hour), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.svc.RunCleanup(ctx); close(done) }()
	deadline := time.Now().Add(5 * time.Second)
	for e.count("SELECT COUNT(*) FROM email_tokens") != 2 {
		if time.Now().After(deadline) {
			t.Fatal("startup cleanup did not run")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
}

// T-045 AC1, AC2: register stores the locale; absent or null means en; the verification mail follows it.
func TestRegisterLocale(t *testing.T) {
	e := newEnv(t, opts{})
	reg := func(email, locale string) *httptest.ResponseRecorder {
		return e.do(req{method: "POST", path: "/v1/auth/register", body: fmt.Sprintf(`{"email":%q,"password":%q,"display_name":"A"%s}`, email, testPW, locale)})
	}
	for i, c := range []struct{ field, stored string }{
		{``, "en"}, {`,"locale":null`, "en"}, {`,"locale":"en"`, "en"}, {`,"locale":"vi"`, "vi"},
	} {
		mail := fmt.Sprintf("ok%d@example.com", i)
		rec := reg(mail, c.field)
		want(t, rec, 201, "")
		if !strings.Contains(rec.Body.String(), `"locale":"`+c.stored+`"`) {
			t.Errorf("%q: response %s, want locale %s", c.field, rec.Body.String(), c.stored)
		}
		if n := e.count("SELECT COUNT(*) FROM users WHERE email = ? AND locale = ?", mail, c.stored); n != 1 {
			t.Errorf("%q: stored locale is not %s", c.field, c.stored)
		}
		me := e.do(req{method: "GET", path: "/v1/me", cookie: sessionCookie(t, rec).Value})
		if !strings.Contains(me.Body.String(), `"locale":"`+c.stored+`"`) {
			t.Errorf("%q: /v1/me %s", c.field, me.Body.String())
		}
	}
	// The Vietnamese mail has the Vietnamese subject and the verify link.
	e.svc.Wait()
	var vi, en mailer.Message
	for _, m := range e.mail.all() {
		switch m.To {
		case "ok3@example.com":
			vi = m
		case "ok2@example.com":
			en = m
		}
	}
	if !strings.Contains(vi.Subject, "Xác nhận") || !strings.Contains(vi.Text, testBase+"/verify-email?token=") {
		t.Errorf("vi mail: %q\n%s", vi.Subject, vi.Text)
	}
	if strings.Contains(en.Subject, "Xác nhận") || en.Subject == "" {
		t.Errorf("en mail subject %q", en.Subject)
	}

	before := e.count("SELECT COUNT(*) FROM users")
	for name, f := range map[string]string{
		"fr": `,"locale":"fr"`, "upper case": `,"locale":"VI"`, "empty": `,"locale":""`,
		"number": `,"locale":1`, "bool": `,"locale":false`, "object": `,"locale":{}`, "array": `,"locale":["vi"]`,
	} {
		rec := reg("bad@example.com", f)
		if rec.Code != 400 || errCode(t, rec) != "invalid_locale" || rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if n := e.count("SELECT COUNT(*) FROM users"); n != before || e.mailCount("bad@example.com") != 0 {
		t.Fatal("rejected locale created a user or sent mail")
	}
}
