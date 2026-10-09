//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/redis"
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

var codeRE = regexp.MustCompile(`\b\d{6}\b`)

// code returns the 6-digit code of the newest mail to `to`.
func (e *env) code(to string) string {
	e.t.Helper()
	e.svc.Wait()
	msgs := e.mail.all()
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].To == to {
			c := codeRE.FindString(msgs[i].Text)
			if c == "" {
				e.t.Fatalf("no code in:\n%s", msgs[i].Text)
			}
			return c
		}
	}
	e.t.Fatalf("no mail to %s", to)
	return ""
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

// wrongFor returns a well-formed code that differs from c.
func wrongFor(c string) string {
	if c[0] == '9' {
		return "0" + c[1:]
	}
	return string(c[0]+1) + c[1:]
}

func (e *env) otpKey(purpose, email string) string {
	e.t.Helper()
	return e.rc.Key("otp", purpose, fmt.Sprint(e.count("SELECT id FROM users WHERE email = ?", email)))
}

// otpField reads a field of the code hash in Redis ("" when the key or field is missing).
func (e *env) otpField(purpose, email, field string) string {
	e.t.Helper()
	v, err := e.rc.Client().HGet(context.Background(), e.otpKey(purpose, email), field).Result()
	if err != nil && err.Error() != "redis: nil" {
		e.t.Fatal(err)
	}
	return v
}

func (e *env) post(path, body string, cookie string) *httptest.ResponseRecorder {
	return e.do(req{method: "POST", path: path, body: body, cookie: cookie, origin: origin})
}

func (e *env) verify(code, cookie string) *httptest.ResponseRecorder {
	return e.post("/v1/auth/verify-email", fmt.Sprintf(`{"code":%q}`, code), cookie)
}

func (e *env) forgot(email, remote string) *httptest.ResponseRecorder {
	return e.do(req{method: "POST", path: "/v1/auth/forgot-password", body: fmt.Sprintf(`{"email":%q}`, email), remote: remote})
}

func (e *env) reset(email, code, pw string) *httptest.ResponseRecorder {
	return e.resetFrom(email, code, pw, "")
}

func (e *env) resetFrom(email, code, pw, remote string) *httptest.ResponseRecorder {
	return e.do(req{method: "POST", path: "/v1/auth/reset-password", origin: origin, remote: remote,
		body: fmt.Sprintf(`{"email":%q,"code":%q,"password":%q}`, email, code, pw)})
}

func (e *env) verified(cookie string) bool {
	e.t.Helper()
	return strings.Contains(e.do(req{method: "GET", path: "/v1/me", cookie: cookie}).Body.String(), `"email_verified":true`)
}

const newPW = "Another-fake-pw-for-tests-42"

// AC1, AC3, AC4: registering emails a code; only its HMAC is stored, with a 30 minute life; it works once.
func TestRegisterSendsVerificationCodeAndVerifyIsSingleUse(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	code := e.code(testMail)
	h := e.otpField("verify", testMail, "h")
	if len(h) != 64 || strings.Contains(h, code) || e.otpField("verify", testMail, "a") != "0" {
		t.Fatalf("stored hash %q for code %s", h, code)
	}
	ttl := e.rc.Client().PTTL(context.Background(), e.otpKey("verify", testMail)).Val()
	if ttl < 30*time.Minute || ttl > 30*time.Minute+expiredGrace {
		t.Fatalf("redis ttl %v", ttl)
	}

	want(t, e.verify(code, ""), 401, "unauthenticated")
	want(t, e.do(req{method: "POST", path: "/v1/auth/verify-email", body: `{"code":"123456"}`, cookie: c.Value}), 403, "csrf_origin_mismatch")
	if e.verified(c.Value) {
		t.Fatal("verified before the code was typed")
	}
	want(t, e.verify(code, c.Value), 204, "")
	if !e.verified(c.Value) {
		t.Fatal("not verified")
	}
	if e.rc.Client().Exists(context.Background(), e.otpKey("verify", testMail)).Val() != 0 {
		t.Fatal("code not consumed")
	}
	want(t, e.verify(code, c.Value), 204, "") // already verified: nothing to do
	if strings.Contains(e.logs.String(), code) {
		t.Fatal("code in logs")
	}
}

// AC3: every way to be wrong is invalid_code; nothing is verified.
func TestVerifyRejectsBadCodes(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	code := e.code(testMail)
	for _, bad := range []string{wrongFor(code), "", "12345", "1234567", "abcdef", " 123456", "12345\u0663"} {
		want(t, e.verify(bad, c.Value), 400, "invalid_code")
	}
	want(t, e.post("/v1/auth/verify-email", `{}`, c.Value), 400, "invalid_code")
	want(t, e.post("/v1/auth/verify-email", `{"code":123456}`, c.Value), 400, "invalid_body")
	want(t, e.post("/v1/auth/verify-email", `not json`, c.Value), 400, "invalid_body")
	if e.otpField("verify", testMail, "a") != "1" { // only the one well-formed wrong guess counted
		t.Fatalf("attempts = %q", e.otpField("verify", testMail, "a"))
	}
	if e.verified(c.Value) {
		t.Fatal("verified by a wrong code")
	}
	want(t, e.verify(code, c.Value), 204, "") // still usable
}

// AC3: after 5 wrong attempts the code is locked, the right code then fails until a new code is sent.
func TestVerifyLocksAfterFiveWrongGuesses(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	code := e.code(testMail)
	bad := wrongFor(code)
	for i := 0; i < 4; i++ {
		want(t, e.verify(bad, c.Value), 400, "invalid_code")
	}
	want(t, e.verify(bad, c.Value), 400, "code_locked")
	want(t, e.verify(code, c.Value), 400, "code_locked")
	want(t, e.verify(bad, c.Value), 400, "code_locked")
	if e.verified(c.Value) {
		t.Fatal("locked code verified")
	}
	want(t, e.post("/v1/auth/verify-email/resend", "", c.Value), 202, "")
	fresh := e.code(testMail)
	if fresh == code {
		t.Skip("drew the same code twice (1 in a million)")
	}
	want(t, e.verify(code, c.Value), 400, "invalid_code") // the old one is dead
	want(t, e.verify(fresh, c.Value), 204, "")
}

// AC1 + QA probe: 20 parallel wrong guesses count exactly 5 attempts; four answer invalid_code, the rest code_locked.
func TestParallelWrongGuessesNeverExceedFiveAttempts(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	bad := wrongFor(e.code(testMail))
	var wg sync.WaitGroup
	codes := make(chan string, verifyTriesPerHour)
	for i := 0; i < verifyTriesPerHour; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- errCode(t, e.verify(bad, c.Value)) }()
	}
	wg.Wait()
	close(codes)
	got := map[string]int{}
	for c := range codes {
		got[c]++
	}
	if got["invalid_code"] != 4 || got["code_locked"] != verifyTriesPerHour-4 {
		t.Fatalf("answers %v", got)
	}
	if a := e.otpField("verify", testMail, "a"); a != "5" {
		t.Fatalf("attempts = %s", a)
	}
}

// AC3: a code expires after 30 minutes and then says so; AC1: one live code per user.
func TestVerifyCodeExpiry(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	code := e.code(testMail)
	e.clock.advance(30*time.Minute - time.Second)
	want(t, e.verify(wrongFor(code), c.Value), 400, "invalid_code") // still live
	e.clock.advance(2 * time.Second)
	want(t, e.verify(code, c.Value), 400, "code_expired")
	want(t, e.verify(wrongFor(code), c.Value), 400, "code_expired")
	if e.verified(c.Value) {
		t.Fatal("expired code verified")
	}
	// no live code at all (consumed or never issued) is invalid_code
	want(t, e.post("/v1/auth/verify-email/resend", "", c.Value), 202, "")
	want(t, e.verify(e.code(testMail), c.Value), 204, "")
	other := sessionCookie(t, e.register("bob@example.com", testPW, "Bob"))
	if err := e.rc.Client().Del(context.Background(), e.otpKey("verify", "bob@example.com")).Err(); err != nil {
		t.Fatal(err)
	}
	want(t, e.verify("000000", other.Value), 400, "invalid_code")
}

// QA probes: another user's code, a reset code, and a verify code do not cross over.
func TestCodesAreBoundToUserAndPurpose(t *testing.T) {
	e := newEnv(t, opts{})
	ca := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	cb := sessionCookie(t, e.register("bob@example.com", testPW, "Bob"))
	aCode, bCode := e.code(testMail), e.code("bob@example.com")
	if aCode == bCode {
		t.Skip("same code for both users (1 in a million)")
	}
	want(t, e.verify(bCode, ca.Value), 400, "invalid_code") // Bob's code, Alice's session
	want(t, e.forgot(testMail, ""), 202, "")
	resetCode := e.code(testMail)
	if resetCode == aCode {
		t.Skip("reset and verify codes are equal (1 in a million)")
	}
	want(t, e.verify(resetCode, ca.Value), 400, "invalid_code")   // reset code as verify code
	want(t, e.reset(testMail, aCode, newPW), 400, "invalid_code") // verify code as reset code
	want(t, e.reset(testMail, bCode, newPW), 400, "invalid_code")
	want(t, e.verify(aCode, ca.Value), 204, "")
	want(t, e.verify(bCode, cb.Value), 204, "")
	want(t, e.reset(testMail, resetCode, newPW), 204, "")
}

// AC3: 20 verify attempts per hour per user, then 429 with Retry-After, whatever the code.
func TestVerifyAttemptsAreLimitedPerUser(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	code := e.code(testMail)
	for i := 0; i < verifyTriesPerHour; i++ {
		e.verify(wrongFor(code), c.Value)
	}
	rec := e.verify(code, c.Value)
	want(t, rec, 429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}
	e.clock.advance(time.Hour + time.Second)
	want(t, e.post("/v1/auth/verify-email/resend", "", c.Value), 202, "")
	want(t, e.verify(e.code(testMail), c.Value), 204, "")
}

// AC4: resend is limited to 3 per hour, each one kills the previous code; a verified user gets already_verified.
func TestResendVerification(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	want(t, e.do(req{method: "POST", path: "/v1/auth/verify-email/resend", origin: origin}), 401, "unauthenticated")
	want(t, e.do(req{method: "POST", path: "/v1/auth/verify-email/resend", cookie: c.Value}), 403, "csrf_origin_mismatch")
	first := e.code(testMail)
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
	if got := e.code(testMail); got != first {
		want(t, e.verify(first, c.Value), 400, "invalid_code") // only the newest code lives
	}
	e.clock.advance(time.Hour + time.Second)
	want(t, e.post("/v1/auth/verify-email/resend", "", c.Value), 202, "") // window passed
	want(t, e.verify(e.code(testMail), c.Value), 204, "")
	before := e.mailCount(testMail)
	rec = e.post("/v1/auth/verify-email/resend", "", c.Value)
	want(t, rec, 200, "")
	if !strings.Contains(rec.Body.String(), `"already_verified":true`) || e.mailCount(testMail) != before {
		t.Fatalf("verified user: %s, mails %d->%d", rec.Body.String(), before, e.mailCount(testMail))
	}
}

// AC4: a mailer failure never fails registration, and the code never reaches the logs.
func TestRegisterSurvivesMailerFailure(t *testing.T) {
	e := newEnv(t, opts{})
	e.mail.fail = true
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	code := e.code(testMail) // the recorder kept the attempted message
	logs := e.logs.String()
	if !strings.Contains(logs, "verification email not sent") {
		t.Fatalf("failure not logged:\n%s", logs)
	}
	if strings.Contains(logs, code) {
		t.Fatal("code in logs")
	}
	e.mail.fail = false
	c := sessionCookie(t, e.login(testMail, testPW, ""))
	want(t, e.post("/v1/auth/verify-email/resend", "", c.Value), 202, "") // user can resend
	if e.mailCount(testMail) != 2 {
		t.Fatalf("mails: %d", e.mailCount(testMail))
	}
}

// AC5: forgot-password stays silent about unknown addresses and sends a code to known ones.
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
	if e.otpField("reset", testMail, "h") == "" {
		t.Fatal("no reset code stored")
	}
	if ttl := e.rc.Client().PTTL(context.Background(), e.otpKey("reset", testMail)).Val(); ttl < 15*time.Minute || ttl > 15*time.Minute+expiredGrace {
		t.Fatalf("reset ttl %v", ttl)
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

// AC5: the whole reset flow, including session removal, single use and a weak password sparing the code.
func TestResetPasswordFlow(t *testing.T) {
	e := newEnv(t, opts{})
	c1 := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	c2 := sessionCookie(t, e.login(testMail, testPW, ""))
	want(t, e.forgot(testMail, ""), 202, "")
	code := e.code(testMail)

	// a weak password is refused, with a right or wrong code alike, and does not touch the code
	want(t, e.reset(testMail, code, "short"), 400, "weak_password")
	want(t, e.reset(testMail, code, testMail), 400, "weak_password")
	want(t, e.reset(testMail, wrongFor(code), "short"), 400, "weak_password")
	want(t, e.reset("nobody@example.com", code, "short"), 400, "weak_password")
	if e.otpField("reset", testMail, "a") != "0" {
		t.Fatal("a weak password counted as an attempt")
	}
	want(t, e.reset("not-an-email", code, newPW), 400, "invalid_email")
	want(t, e.reset(testMail, wrongFor(code), newPW), 400, "invalid_code")

	want(t, e.reset("  ALICE@example.com ", code, newPW), 204, "")
	for _, c := range []string{c1.Value, c2.Value} {
		want(t, e.do(req{method: "GET", path: "/v1/me", cookie: c}), 401, "unauthenticated")
	}
	if e.count("SELECT COUNT(*) FROM sessions") != 0 {
		t.Fatal("sessions left")
	}
	want(t, e.login(testMail, testPW, ""), 401, "invalid_credentials")
	want(t, e.login(testMail, newPW, ""), 200, "")
	want(t, e.reset(testMail, code, "Third-fake-password-99"), 400, "invalid_code") // reuse
	want(t, e.login(testMail, newPW, ""), 200, "")
	if strings.Contains(e.logs.String(), code) || strings.Contains(e.logs.String(), newPW) {
		t.Fatal("secret in logs")
	}
}

// AC5: unknown address, wrong code and no code at all answer alike (same status, code and message).
func TestResetAnswersAreIndistinguishable(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	want(t, e.register("bob@example.com", testPW, "Bob"), 201, "") // Bob never asks for a reset code
	want(t, e.forgot(testMail, ""), 202, "")
	code := e.code(testMail)
	shape := func(rec *httptest.ResponseRecorder) string {
		var b struct {
			Error struct{ Code, Message string }
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
			t.Fatal(err)
		}
		return fmt.Sprint(rec.Code, b.Error.Code, b.Error.Message)
	}
	a := shape(e.reset("ghost@example.com", "123456", newPW))
	b := shape(e.reset(testMail, wrongFor(code), newPW))
	c := shape(e.reset("bob@example.com", "123456", newPW))
	if a != b || b != c || !strings.Contains(a, "invalid_code") {
		t.Fatalf("answers differ: %q %q %q", a, b, c)
	}
}

// AC5: a reset code lives 15 minutes, then code_expired for the account that has one; the password stays.
func TestResetCodeExpiresAfterFifteenMinutes(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	want(t, e.forgot(testMail, ""), 202, "")
	code := e.code(testMail)
	e.clock.advance(15*time.Minute - time.Second)
	want(t, e.reset(testMail, wrongFor(code), newPW), 400, "invalid_code") // a near miss is just wrong
	e.clock.advance(2 * time.Second)
	want(t, e.reset(testMail, code, newPW), 400, "code_expired")
	want(t, e.reset("ghost@example.com", code, newPW), 400, "invalid_code")
	want(t, e.login(testMail, testPW, ""), 200, "") // password unchanged
}

// AC5: 5 wrong guesses lock a reset code; a new forgot-password gives a fresh one.
func TestResetLocksAfterFiveWrongGuesses(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	want(t, e.forgot(testMail, "10.0.0.1:1"), 202, "")
	code := e.code(testMail)
	for i := 0; i < 4; i++ {
		want(t, e.reset(testMail, wrongFor(code), newPW), 400, "invalid_code")
	}
	want(t, e.reset(testMail, wrongFor(code), newPW), 400, "code_locked")
	want(t, e.reset(testMail, code, newPW), 400, "code_locked")
	want(t, e.login(testMail, newPW, ""), 401, "invalid_credentials")
	want(t, e.forgot(testMail, "10.0.0.2:1"), 202, "")
	fresh := e.code(testMail)
	if fresh == code {
		t.Skip("same code twice (1 in a million)")
	}
	want(t, e.reset(testMail, code, newPW), 400, "invalid_code")
	want(t, e.reset(testMail, fresh, newPW), 204, "")
}

// AC5: reset attempts are limited to 20 per hour per IP.
func TestResetAttemptsAreLimitedPerIP(t *testing.T) {
	e := newEnv(t, opts{})
	for i := 0; i < resetTriesPerHour; i++ {
		want(t, e.resetFrom("ghost@example.com", "123456", newPW, "192.0.2.5:1"), 400, "invalid_code")
	}
	rec := e.resetFrom("ghost@example.com", "123456", newPW, "192.0.2.5:1")
	want(t, rec, 429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}
	want(t, e.resetFrom("ghost@example.com", "123456", newPW, "192.0.2.6:1"), 400, "invalid_code") // another IP
}

// AC1 + QA probe: one code used by 8 parallel resets has exactly one winner.
func TestConcurrentResetHasOneWinner(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "Alice"), 201, "")
	want(t, e.forgot(testMail, ""), 202, "")
	code := e.code(testMail)
	var wg sync.WaitGroup
	statuses := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			statuses <- e.reset(testMail, code, fmt.Sprintf("Concurrent-fake-pw-%d-xx", i)).Code
		}()
	}
	wg.Wait()
	close(statuses)
	ok := 0
	for c := range statuses {
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

// AC6: users.locale picks the language and the mail carries the code, in both languages.
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
	if !strings.Contains(got.Subject, "Mã đặt lại mật khẩu") || !strings.Contains(got.Text, e.code(testMail)) || !strings.Contains(got.Text, "Hồng") || strings.Contains(got.Text, "http") {
		t.Fatalf("not the Vietnamese reset code email: %+v", got)
	}
	if first := msgs[0]; !strings.Contains(first.Subject, "verification code") || !strings.Contains(first.Text, "30 minutes") {
		t.Fatalf("registration mail should be English (default locale): %q\n%s", first.Subject, first.Text)
	}
}

// D-23: with Redis down the code endpoints fail closed with 503 code_store_unavailable.
func TestRedisDownFailsClosed(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	code := e.code(testMail)
	dead, err := redis.New(config.Config{Env: "test", RedisURL: "redis://127.0.0.1:1/0",
		RedisDialTimeout: time.Second, RedisReadTimeout: time.Second, RedisWriteTimeout: time.Second, RedisPoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dead.Close() }()
	e.codes.rc = dead // nothing listens on port 1
	for name, rec := range map[string]*httptest.ResponseRecorder{
		"verify": e.verify(code, c.Value),
		"resend": e.post("/v1/auth/verify-email/resend", "", c.Value),
		"reset":  e.reset(testMail, code, newPW),
		"decoy":  e.reset("ghost@example.com", code, newPW),
	} {
		if rec.Code != 503 || errCode(t, rec) != "code_store_unavailable" || rec.Header().Get("Retry-After") == "" {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	if e.verified(c.Value) {
		t.Fatal("verified while Redis was down")
	}
	want(t, e.forgot(testMail, ""), 202, "") // stays silent; the send failure is logged
	e.svc.Wait()
	if !errors.Is(e.codes.Check(context.Background(), purposeVerify, 1, code, e.clock.now()), redis.ErrUnavailable) {
		t.Fatal("Check should report ErrUnavailable")
	}
}

// AC7 DEV-SHORTCUT(otp): with the fixed code on, 123123 verifies the session user and resets any existing account.
func TestDevFixedCode(t *testing.T) {
	const fixed = "123123"
	e := newEnv(t, opts{fixedOTP: fixed})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	real := e.code(testMail)
	want(t, e.verify(wrongFor(real), c.Value), 400, "invalid_code") // a wrong 6-digit code is still rejected
	want(t, e.verify("123124", c.Value), 400, "invalid_code")
	want(t, e.verify(fixed, c.Value), 204, "")
	if !e.verified(c.Value) {
		t.Fatal("fixed code did not verify")
	}
	if e.otpField("verify", testMail, "a") != "2" { // the fixed code consumed nothing
		t.Fatalf("attempts = %q", e.otpField("verify", testMail, "a"))
	}

	// reset: no forgot-password needed; an unknown address is still invalid_code; a weak password still counts first
	want(t, e.reset("ghost@example.com", fixed, newPW), 400, "invalid_code")
	want(t, e.reset(testMail, fixed, "short"), 400, "weak_password")
	want(t, e.reset(testMail, "123124", newPW), 400, "invalid_code")
	want(t, e.reset(testMail, fixed, newPW), 204, "")
	want(t, e.login(testMail, newPW, ""), 200, "")
	want(t, e.reset(testMail, fixed, "Yet-another-fake-pw-7"), 204, "") // not single use: it is a dev shortcut
}

// AC7 DEV-SHORTCUT(otp): with the variable unset, 123123 is just a wrong code.
func TestDevFixedCodeOffByDefault(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	if e.code(testMail) == "123123" {
		t.Skip("drew 123123 (1 in a million)")
	}
	want(t, e.verify("123123", c.Value), 400, "invalid_code")
	want(t, e.reset(testMail, "123123", newPW), 400, "invalid_code")
	if e.verified(c.Value) {
		t.Fatal("verified")
	}
}

// AC7 DEV-SHORTCUT(otp): NewCodes refuses the fixed code outside dev and test, in every spelling.
func TestNewCodesRefusesFixedCodeOutsideDevAndTest(t *testing.T) {
	rc := newEnv(t, opts{}).rc
	for _, env := range []string{"prod", "production", "", "staging", "DEV"} {
		if _, err := NewCodes(context.Background(), rc, env, testOTPKey, "123123"); err == nil {
			t.Errorf("SMEM_ENV=%q: want a refusal", env)
		}
	}
	for _, env := range []string{"dev", "test"} {
		if _, err := NewCodes(context.Background(), rc, env, testOTPKey, "123123"); err != nil {
			t.Errorf("SMEM_ENV=%q: %v", env, err)
		}
	}
	if _, err := NewCodes(context.Background(), rc, "prod", testOTPKey, ""); err != nil {
		t.Errorf("prod without the fixed code: %v", err)
	}
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
	// The Vietnamese mail has the Vietnamese subject and the verify code.
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
	if !strings.Contains(vi.Subject, "Mã xác minh") || !strings.Contains(vi.Text, "Mã xác minh") || !codeRE.MatchString(vi.Text) {
		t.Errorf("vi mail: %q\n%s", vi.Subject, vi.Text)
	}
	if strings.Contains(en.Subject, "Mã xác minh") || en.Subject == "" {
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
