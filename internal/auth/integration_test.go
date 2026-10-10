//go:build integration

package auth

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth/oidctest"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/redis"
	"github.com/danyaa666/smemories/internal/redis/redistest"
)

const (
	origin   = "http://localhost:5173"
	testPW   = "Tr0ub4dor&3-fake-test-password" // obviously fake; only ever used in these tests
	testMail = "alice@example.com"
)

type env struct {
	t      *testing.T
	db     *sql.DB
	h      http.Handler
	svc    *Service
	ah     *Handler
	hasher *Hasher
	logs   *bytes.Buffer
	clock  *testClock
	mail   *recMailer
	rc     *redis.Client
	codes  *Codes
	sess   *Sessions
}

type testClock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *testClock) now() time.Time { c.mu.Lock(); defer c.mu.Unlock(); return c.t }
func (c *testClock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

type opts struct {
	limits     Limits
	trustProxy bool
	secure     bool
	maxHashes  int
	hashWait   time.Duration
	hashMemKiB uint32
	google     *oidctest.Provider // enables Google sign-in against this fake
	fixedOTP   string             // SMEM_DEV_FIXED_OTP; "" = off
}

// testOTPKey is the HMAC key of the codes in tests.
var testOTPKey = []byte(strings.Repeat("k", MinOTPKeyBytes))

func newEnv(t *testing.T, o opts) *env {
	t.Helper()
	if o.limits == (Limits{}) {
		o.limits = Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000}
	}
	if o.maxHashes == 0 {
		o.maxHashes = 4
	}
	if o.hashWait == 0 {
		o.hashWait = HashWait
	}
	if o.hashMemKiB == 0 {
		o.hashMemKiB = 64
	}
	d := dbtest.New(t)
	clock := &testClock{t: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	hasher := NewHasher(HashParams{MemoryKiB: o.hashMemKiB, Time: 1, Parallelism: 1}, o.maxHashes, o.hashWait)
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(&syncWriter{w: logs}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	rm := &recMailer{}
	rc := redistest.New(t)
	codes, err := NewCodes(context.Background(), rc, "test", testOTPKey, o.fixedOTP)
	if err != nil {
		t.Fatal(err)
	}
	sess, err := NewSessions(context.Background(), rc, logger)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := NewService(NewStore(d), sess, hasher, o.limits, Mail{Mailer: rm, Logger: logger}, codes, clock.now)
	if err != nil {
		t.Fatal(err)
	}
	ah := NewHandler(svc, HandlerConfig{AllowedOrigins: []string{origin}, SecureCookie: o.secure, TrustProxy: o.trustProxy}, logger)
	if o.google != nil {
		if err := ah.EnableGoogle(GoogleConfig{
			ClientID: oidctest.ClientID, ClientSecret: oidctest.ClientSecret, Issuer: o.google.URL,
			RedirectURL: testBase + "/api/v1/auth/google/callback", CookieKey: []byte(strings.Repeat("k", 32)),
		}); err != nil {
			t.Fatal(err)
		}
	}
	return &env{t: t, db: d, h: httpx.NewRouter(logger, ah.Routes), svc: svc, ah: ah, hasher: hasher, logs: logs, clock: clock, mail: rm, rc: rc, codes: codes, sess: sess}
}

type syncWriter struct {
	mu sync.Mutex
	w  io.Writer
}

func (s *syncWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(b)
}

type req struct {
	method, path, body string
	cookie             string
	oidc               string // smem_oidc cookie value
	origin             string // "" = send none
	contentType        string // "" with a body = application/json
	remote             string // "" = 192.0.2.1:1234
	headers            map[string]string
}

func (e *env) do(r req) *httptest.ResponseRecorder {
	e.t.Helper()
	var rd io.Reader
	if r.body != "" {
		rd = strings.NewReader(r.body)
	}
	hr := httptest.NewRequest(r.method, r.path, rd)
	if r.body != "" {
		ct := r.contentType
		if ct == "" {
			ct = "application/json"
		}
		hr.Header.Set("Content-Type", ct)
	}
	if r.cookie != "" {
		hr.AddCookie(&http.Cookie{Name: CookieName, Value: r.cookie})
	}
	if r.oidc != "" {
		hr.AddCookie(&http.Cookie{Name: oidcCookie, Value: r.oidc})
	}
	if r.origin != "" {
		hr.Header.Set("Origin", r.origin)
	}
	if r.remote != "" {
		hr.RemoteAddr = r.remote
	}
	for k, v := range r.headers {
		hr.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, hr)
	return rec
}

func body(email, pw, name string) string {
	b, _ := json.Marshal(map[string]string{"email": email, "password": pw, "display_name": name})
	return string(b)
}

func (e *env) register(email, pw, name string) *httptest.ResponseRecorder {
	return e.do(req{method: "POST", path: "/v1/auth/register", body: body(email, pw, name)})
}

func (e *env) login(email, pw, remote string) *httptest.ResponseRecorder {
	return e.do(req{method: "POST", path: "/v1/auth/login", body: body(email, pw, ""), remote: remote})
}

func sessionCookie(t *testing.T, rec *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range rec.Result().Cookies() {
		if c.Name == CookieName {
			return c
		}
	}
	t.Fatalf("no %s cookie in %v", CookieName, rec.Header()["Set-Cookie"])
	return nil
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var env struct{ Error httpx.ErrorBody }
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil || env.Error.Code == "" || env.Error.RequestID == "" {
		t.Fatalf("not an error envelope (status %d): %q", rec.Code, rec.Body.String())
	}
	return env.Error.Code
}

func want(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status %d, want %d (%s)", rec.Code, status, rec.Body.String())
	}
	if code != "" && errCode(t, rec) != code {
		t.Fatalf("code %q, want %q", errCode(t, rec), code)
	}
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// sessionCount is the number of sessions in Redis (this test's own key space).
func (e *env) sessionCount() int {
	e.t.Helper()
	keys, err := e.rc.Client().Keys(context.Background(), e.rc.Key("sess", "*")).Result()
	if err != nil {
		e.t.Fatal(err)
	}
	return len(keys)
}

// AC1, AC4, AC5, AC8: the whole life of a session.
func TestRegisterMeLogoutFlow(t *testing.T) {
	e := newEnv(t, opts{})
	rec := e.register("  Alice@Example.COM ", testPW, "  Đặng Thị Hồng 🎓 ")
	want(t, rec, 201, "")

	var out map[string]map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	u := out["user"]
	keys := map[string]bool{}
	for k := range u {
		keys[k] = true
	}
	if len(u) != 6 || !keys["id"] || !keys["email"] || !keys["email_verified"] || !keys["display_name"] || !keys["locale"] || !keys["created_at"] {
		t.Fatalf("user object must be exactly {id,email,email_verified,display_name,locale,created_at}, got %v", u)
	}
	if !regexp.MustCompile(`^[0-9A-HJKMNP-TV-Z]{26}$`).MatchString(u["id"].(string)) {
		t.Fatalf("id is not a ULID: %v", u["id"])
	}
	if u["email"] != "alice@example.com" || u["display_name"] != "Đặng Thị Hồng 🎓" || u["email_verified"] != false || u["locale"] != "en" {
		t.Fatalf("unexpected user: %v", u)
	}
	for _, leak := range []string{"password", "hash", "argon"} {
		if strings.Contains(strings.ToLower(rec.Body.String()), leak) {
			t.Fatalf("response mentions %q: %s", leak, rec.Body.String())
		}
	}

	c := sessionCookie(t, rec)
	me := e.do(req{method: "GET", path: "/v1/me", cookie: c.Value})
	want(t, me, 200, "")
	if me.Body.String() != rec.Body.String() {
		t.Fatalf("/v1/me = %s, register = %s", me.Body.String(), rec.Body.String())
	}

	out204 := e.do(req{method: "POST", path: "/v1/auth/logout", cookie: c.Value, origin: origin})
	want(t, out204, 204, "")
	cleared := sessionCookie(t, out204)
	if cleared.Value != "" || cleared.MaxAge >= 0 {
		t.Fatalf("logout must clear the cookie, got %+v", cleared)
	}
	want(t, e.do(req{method: "GET", path: "/v1/me", cookie: c.Value}), 401, "unauthenticated")            // replay of the old cookie
	want(t, e.do(req{method: "POST", path: "/v1/auth/logout", cookie: c.Value, origin: origin}), 204, "") // idempotent
	want(t, e.do(req{method: "POST", path: "/v1/auth/logout"}), 204, "")                                  // not even signed in
	if n := e.sessionCount(); n != 0 {
		t.Fatalf("%d sessions left", n)
	}
}

// AC4: cookie attributes, token shape, and only the SHA-256 reaches the database.
func TestSessionTokenAndCookieFlags(t *testing.T) {
	for _, secure := range []bool{false, true} {
		e := newEnv(t, opts{secure: secure})
		c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
		if !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.Path != "/" || c.MaxAge != 30*24*3600 || c.Secure != secure {
			t.Fatalf("secure=%v: bad cookie attributes %+v", secure, c)
		}
		if !regexp.MustCompile(`^[A-Za-z0-9_-]{43}$`).MatchString(c.Value) { // 32 bytes, base64url, no padding
			t.Fatalf("token shape: %q", c.Value)
		}
		sum := sha256.Sum256([]byte(c.Value))
		ctx := context.Background()
		if n, err := e.rc.Client().Exists(ctx, e.rc.Key("sess", hex.EncodeToString(sum[:]))).Result(); err != nil || n != 1 {
			t.Fatalf("expected the sha256 of the token to be the stored key: %d, %v", n, err)
		}
		if n, err := e.rc.Client().Exists(ctx, e.rc.Key("sess", c.Value)).Result(); err != nil || n != 0 {
			t.Fatalf("raw token must not be stored: %d, %v", n, err)
		}
		raw := e.do(req{method: "GET", path: "/v1/me", cookie: c.Value})
		if raw.Code != 200 {
			t.Fatal(raw.Body.String())
		}
		hdr := e.register("bob@example.com", testPW, "Bob").Header().Get("Set-Cookie")
		if secure != strings.Contains(hdr, "Secure") || !strings.Contains(hdr, "HttpOnly") || !strings.Contains(hdr, "SameSite=Lax") || !strings.Contains(hdr, "Max-Age=2592000") {
			t.Fatalf("Set-Cookie: %s", hdr)
		}
	}
}

// AC1: validation, normalisation, duplicates.
func TestRegisterValidation(t *testing.T) {
	e := newEnv(t, opts{})
	long := strings.Repeat("a", 243) + "@example.com"
	for _, c := range []struct {
		name, email, pw, display, code string
	}{
		{"no email", "", testPW, "A", "invalid_email"},
		{"not an address", "alice", testPW, "A", "invalid_email"},
		{"no dot in domain", "alice@localhost", testPW, "A", "invalid_email"},
		{"too long", long, testPW, "A", "invalid_email"},
		{"sql injection", "x'; DROP TABLE users;--@example.com", testPW, "A", "invalid_email"},
		{"short password", testMail, "123456789", "A", "weak_password"},
		{"password is the email", testMail, testMail, "A", "weak_password"},
		{"password is the email, other case", testMail, "ALICE@example.com", "A", "weak_password"},
		{"129 characters", testMail, strings.Repeat("p", 129), "A", "weak_password"},
		{"300 KB password is refused", testMail, strings.Repeat("p", 300<<10), "A", "weak_password"},
		{"blank display name", testMail, testPW, "   ", "invalid_display_name"},
		{"101 character display name", testMail, testPW, strings.Repeat("n", 101), "invalid_display_name"},
		{"control character in name", testMail, testPW, "Al\nice", "invalid_display_name"},
	} {
		rec := e.register(c.email, c.pw, c.display)
		if rec.Code != 400 || errCode(t, rec) != c.code {
			t.Errorf("%s: got %d %s, want 400 %s", c.name, rec.Code, rec.Body.String(), c.code)
		}
		if rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s: no cookie on failure", c.name)
		}
	}
	if n := e.count("SELECT COUNT(*) FROM users"); n != 0 {
		t.Fatalf("rejected registrations created %d users", n)
	}
	// Boundaries that are valid: 10 and 128 characters, 100-character name.
	want(t, e.register("a@example.com", strings.Repeat("p", 10), "A"), 201, "")
	want(t, e.register("b@example.com", strings.Repeat("🎓", 128), strings.Repeat("đ", 100)), 201, "")
}

func TestRegisterBadBodies(t *testing.T) {
	e := newEnv(t, opts{})
	for name, b := range map[string]string{
		"not json":       `email=a`,
		"array":          `[]`,
		"wrong type":     `{"email":1,"password":"x","display_name":"y"}`,
		"trailing data":  `{"email":"a@example.com"} {"x":1}`,
		"truncated json": `{"email":"a@exam`,
	} {
		rec := e.do(req{method: "POST", path: "/v1/auth/register", body: b})
		if rec.Code != 400 || errCode(t, rec) != "invalid_body" {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
	rec := e.do(req{method: "POST", path: "/v1/auth/register", body: `{"email":"` + strings.Repeat("a", 2<<20) + `"}`})
	want(t, rec, 413, "payload_too_large")
}

func TestDuplicateEmailIsCaseInsensitive(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register("Dup@Example.com", testPW, "One"), 201, "")
	for _, em := range []string{"dup@example.com", "DUP@EXAMPLE.COM", " dup@Example.com "} {
		want(t, e.register(em, testPW, "Two"), 409, "email_taken")
	}
	if n := e.count("SELECT COUNT(*) FROM users"); n != 1 {
		t.Fatalf("%d users", n)
	}
	// Accents are not folded: they are different addresses.
	want(t, e.register("dưp@example.com", testPW, "Accent"), 201, "")
}

func TestConcurrentDuplicateRegistrations(t *testing.T) {
	e := newEnv(t, opts{})
	var wg sync.WaitGroup
	codes := make(chan int, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- e.register("race@example.com", testPW, "R").Code }()
	}
	wg.Wait()
	close(codes)
	created, taken := 0, 0
	for c := range codes {
		switch c {
		case 201:
			created++
		case 409:
			taken++
		default:
			t.Errorf("unexpected status %d", c)
		}
	}
	if created != 1 || taken != 7 {
		t.Fatalf("created=%d taken=%d", created, taken)
	}
	if n := e.count("SELECT COUNT(*) FROM users"); n != 1 {
		t.Fatalf("%d users", n)
	}
}

// AC2: stored as an argon2id PHC string with the configured parameters.
func TestPasswordStoredAsArgon2id(t *testing.T) {
	e := newEnv(t, opts{hashMemKiB: 1024})
	want(t, e.register(testMail, testPW, "A"), 201, "")
	var phc string
	if err := e.db.QueryRow("SELECT password_hash FROM users").Scan(&phc); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(phc, "$argon2id$v=19$m=1024,t=1,p=1$") || strings.Contains(phc, testPW) {
		t.Fatalf("stored hash: %q", phc)
	}
}

// AC3: login success, new session each time, and indistinguishable failures.
func TestLogin(t *testing.T) {
	e := newEnv(t, opts{})
	first := sessionCookie(t, e.register(testMail, testPW, "Alice"))

	ok := e.login(" ALICE@example.com", testPW, "")
	want(t, ok, 200, "")
	second := sessionCookie(t, ok)
	if second.Value == first.Value {
		t.Fatal("login must issue a new token (no fixation)")
	}
	want(t, e.do(req{method: "GET", path: "/v1/me", cookie: second.Value}), 200, "")
	want(t, e.do(req{method: "GET", path: "/v1/me", cookie: first.Value}), 200, "") // other devices keep their session

	// Logging in while sending an old cookie revokes that cookie's session.
	third := e.do(req{method: "POST", path: "/v1/auth/login", body: body(testMail, testPW, ""), cookie: first.Value, origin: origin})
	want(t, third, 200, "")
	want(t, e.do(req{method: "GET", path: "/v1/me", cookie: first.Value}), 401, "unauthenticated")

	wrong := e.login(testMail, testPW+"x", "")
	unknown := e.login("nobody@example.com", testPW+"x", "")
	for _, rec := range []*httptest.ResponseRecorder{wrong, unknown} {
		want(t, rec, 401, "invalid_credentials")
		if rec.Header().Get("Set-Cookie") != "" {
			t.Fatal("no cookie on failure")
		}
	}
	strip := regexp.MustCompile(`"request_id":"[^"]*"`)
	if a, b := strip.ReplaceAllString(wrong.Body.String(), ""), strip.ReplaceAllString(unknown.Body.String(), ""); a != b {
		t.Fatalf("bodies differ:\n%s\n%s", a, b)
	}
	// Garbage and injection attempts are plain credential failures.
	for _, em := range []string{"' OR '1'='1", "alice@example.com' --", strings.Repeat("x", 5000), ""} {
		want(t, e.login(em, testPW, ""), 401, "invalid_credentials")
	}
}

// L-09: passwords are NFKC-normalised before the rules, hashing and verifying; display names are NFC.
func TestUnicodeNormalisation(t *testing.T) {
	const nfc, nfd = "M\u1eadt kh\u1ea9u ti\u1ebfng Vi\u1ec7t", "Ma\u0323\u0302t kha\u0302\u0309u tie\u0302\u0301ng Vie\u0323\u0302t"
	if nfc == nfd || strings.Contains(nfc, "\u0323") {
		t.Fatal("fixtures must differ in bytes")
	}
	e := newEnv(t, opts{})
	// Registered with NFC, logs in with NFD.
	want(t, e.register("nfc@example.com", nfc, "A"), 201, "")
	want(t, e.login("nfc@example.com", nfd, ""), 200, "")
	// Registered with NFD (NFD is longer in runes), logs in with NFC.
	want(t, e.register("nfd@example.com", nfd, "B"), 201, "")
	want(t, e.login("nfd@example.com", nfc, ""), 200, "")
	// Stored hash is over the NFKC form: it verifies the NFC bytes directly.
	var phc string
	if err := e.db.QueryRow("SELECT password_hash FROM users WHERE email=?", "nfd@example.com").Scan(&phc); err != nil {
		t.Fatal(err)
	}
	if ok, err := e.hasher.Verify(context.Background(), nfc, phc); err != nil || !ok {
		t.Fatalf("hash not over the NFKC form: %v %v", ok, err)
	}
	// NFKC folds compatibility characters: full-width digits log in as ASCII.
	want(t, e.register("fw@example.com", "pass\uff11\uff12\uff13\uff14\uff15\uff16word", "C"), 201, "")
	want(t, e.login("fw@example.com", "pass123456word", ""), 200, "")
	want(t, e.login("fw@example.com", "pass123456wor", ""), 401, "invalid_credentials")
	// The 10-character minimum counts after normalisation: 15 decomposed runes are 5 characters.
	want(t, e.register("short@example.com", strings.Repeat("e\u0323\u0302", 5), "D"), 400, "weak_password")
	// Display name stored (and returned) as NFC.
	rec := e.register("name@example.com", testPW, "Vie\u0323\u0302t")
	want(t, rec, 201, "")
	var out struct {
		User struct {
			DisplayName string `json:"display_name"`
		}
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.User.DisplayName != "Vi\u1ec7t" {
		t.Fatalf("response display_name %q (%v)", out.User.DisplayName, err)
	}
	var name string
	if err := e.db.QueryRow("SELECT display_name FROM users WHERE email=?", "name@example.com").Scan(&name); err != nil || name != "Vi\u1ec7t" {
		t.Fatalf("stored display_name %q (%v)", name, err)
	}
}

// Hardening: the NFC and NFD spellings of one address are one account.
func TestEmailNFCvsNFD(t *testing.T) {
	const nfc, nfd = "nguy\u1ec5n@example.com", "nguye\u0302\u0303n@example.com"
	if nfc == nfd {
		t.Fatal("fixtures must differ in bytes")
	}
	e := newEnv(t, opts{})
	want(t, e.register(nfc, testPW, "A"), 201, "")
	want(t, e.register(nfd, testPW, "B"), 409, "email_taken")
	if n := e.count("SELECT COUNT(*) FROM users"); n != 1 {
		t.Fatalf("%d users", n)
	}
	var stored string
	if err := e.db.QueryRow("SELECT email FROM users").Scan(&stored); err != nil || stored != nfc {
		t.Fatalf("stored email %q (%v)", stored, err)
	}
	want(t, e.login(nfc, testPW, ""), 200, "")
	want(t, e.login(nfd, testPW, ""), 200, "")
}

// Hardening: local part over 64 bytes and control/format characters in an address are invalid_email;
// bidi-override, zero-width and line-separator display names are invalid_display_name.
func TestEmailAndDisplayNameHardening(t *testing.T) {
	e := newEnv(t, opts{})
	for _, em := range []string{strings.Repeat("a", 65) + "@example.com", "a\u200bb@example.com", "a\u202eb@example.com"} {
		want(t, e.register(em, testPW, "A"), 400, "invalid_email")
	}
	want(t, e.register(strings.Repeat("a", 64)+"@example.com", testPW, "A"), 201, "")
	for _, name := range []string{"a\u200bb", "\u200b", "a\u202eb", "a\u2066b\u2069", "a\u200eb", "a\ufeffb", "a\u2028b", "a\u2029b"} {
		want(t, e.register("n@example.com", testPW, name), 400, "invalid_display_name")
	}
	want(t, e.register("n@example.com", testPW, "Vi\u1ec7t \U0001F468\u200d\U0001F393"), 201, "")
}

// AC3 timing: the unknown-email path takes a hashing slot just like a real verify.
func TestUnknownEmailLoginDoesTheSameHashingWork(t *testing.T) {
	e := newEnv(t, opts{maxHashes: 1, hashWait: 30 * time.Millisecond})
	want(t, e.register(testMail, testPW, "A"), 201, "")
	e.hasher.slots <- struct{}{} // occupy the only slot
	want(t, e.login(testMail, testPW, ""), 503, "busy")
	want(t, e.login("nobody@example.com", testPW, ""), 503, "busy")
	want(t, e.register("b@example.com", testPW, "B"), 503, "busy")
	<-e.hasher.slots
	want(t, e.login("nobody@example.com", testPW, ""), 401, "invalid_credentials")
}

// AC8: oversized passwords never reach the hasher (the only slot is held, yet we get 400/401, not 503).
func TestOversizePasswordsAreRejectedBeforeHashing(t *testing.T) {
	e := newEnv(t, opts{maxHashes: 1, hashWait: 30 * time.Millisecond})
	e.hasher.slots <- struct{}{}
	huge := strings.Repeat("p", 500<<10)
	want(t, e.register(testMail, huge, "A"), 400, "weak_password")
	want(t, e.login(testMail, huge, ""), 401, "invalid_credentials")
	want(t, e.login(testMail, strings.Repeat("p", 129), ""), 401, "invalid_credentials")
	want(t, e.login(testMail, "", ""), 401, "invalid_credentials")
}

// AC2: a flood of logins gets answers (200/401/503), never a hang and never more than the cap in flight.
func TestParallelLoginsRespectTheHashCap(t *testing.T) {
	e := newEnv(t, opts{maxHashes: 2, hashWait: 40 * time.Millisecond, hashMemKiB: 65536})
	want(t, e.register(testMail, testPW, "A"), 201, "")

	var wg sync.WaitGroup
	var mu sync.Mutex
	got := map[int]int{}
	maxInFlight := 0
	stop := make(chan struct{})
	go func() { // sample the number of occupied slots
		for {
			select {
			case <-stop:
				return
			default:
				mu.Lock()
				maxInFlight = max(maxInFlight, len(e.hasher.slots))
				mu.Unlock()
				time.Sleep(time.Millisecond)
			}
		}
	}()
	start := time.Now()
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			pw := testPW
			if i%2 == 1 {
				pw += "-wrong"
			}
			code := e.login(testMail, pw, fmt.Sprintf("198.51.100.%d:1", i)).Code
			mu.Lock()
			got[code]++
			mu.Unlock()
		}(i)
	}
	wg.Wait()
	close(stop)
	if time.Since(start) > 10*time.Second {
		t.Fatalf("flood took %v", time.Since(start))
	}
	for code := range got {
		if code != 200 && code != 401 && code != 503 {
			t.Errorf("unexpected status %d (%v)", code, got)
		}
	}
	if got[200]+got[401] == 0 || got[503] == 0 {
		t.Errorf("expected some answered and some busy requests, got %v", got)
	}
	mu.Lock() // the sampler may still be running for one more tick
	defer mu.Unlock()
	if maxInFlight > 2 {
		t.Errorf("%d hashes in flight, cap is 2", maxInFlight)
	}
}

// AC5: expiry and sliding expiry (clock injected).
func TestSessionExpiryAndSliding(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "A")).Value
	expiry := func() time.Time { // last_seen_at in the hash + the TTL; the Redis TTL itself is checked below
		keys, err := e.rc.Client().Keys(context.Background(), e.rc.Key("sess", "*")).Result()
		if err != nil || len(keys) != 1 {
			t.Fatalf("session keys: %v, %v", keys, err)
		}
		ms, err := e.rc.Client().HGet(context.Background(), keys[0], "last_seen_at").Int64()
		if err != nil {
			t.Fatal(err)
		}
		if ttl := e.rc.Client().PTTL(context.Background(), keys[0]).Val(); ttl < SessionTTL-time.Minute || ttl > SessionTTL {
			t.Fatalf("Redis TTL %v, want about %v", ttl, SessionTTL)
		}
		return time.UnixMilli(ms).UTC().Add(SessionTTL)
	}
	t0 := e.clock.now()
	if got := expiry(); !got.Equal(t0.Add(SessionTTL)) {
		t.Fatalf("initial expiry %v", got)
	}

	e.clock.advance(10 * 24 * time.Hour) // a third of the TTL: no refresh
	rec := e.do(req{method: "GET", path: "/v1/me", cookie: c})
	want(t, rec, 200, "")
	if rec.Header().Get("Set-Cookie") != "" || !expiry().Equal(t0.Add(SessionTTL)) {
		t.Fatal("must not slide before half the TTL has passed")
	}

	e.clock.advance(6 * 24 * time.Hour) // 16 days: more than half has elapsed
	rec = e.do(req{method: "GET", path: "/v1/me", cookie: c})
	want(t, rec, 200, "")
	re := sessionCookie(t, rec)
	if re.Value != c || re.MaxAge != 30*24*3600 {
		t.Fatalf("refreshed cookie %+v", re)
	}
	if got := expiry(); !got.Equal(e.clock.now().Add(SessionTTL)) {
		t.Fatalf("slid expiry %v, want now+30d", got)
	}

	e.clock.advance(SessionTTL + time.Second) // past the new expiry
	want(t, e.do(req{method: "GET", path: "/v1/me", cookie: c}), 401, "unauthenticated")

	// Expired rows are swept by the user's next login.
	if n := e.sessionCount(); n != 1 {
		t.Fatalf("%d sessions before login", n)
	}
	want(t, e.login(testMail, testPW, ""), 200, "")
	if n := e.sessionCount(); n != 1 {
		t.Fatalf("%d sessions after login, want only the new one", n)
	}
}

func TestMeWithoutValidSession(t *testing.T) {
	e := newEnv(t, opts{})
	for name, cookie := range map[string]string{"none": "", "garbage": "not-a-token", "unknown but well-formed": strings.Repeat("A", 43), "sql": "' OR 1=1 --"} {
		rec := e.do(req{method: "GET", path: "/v1/me", cookie: cookie})
		if rec.Code != 401 || errCode(t, rec) != "unauthenticated" {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
	}
}

// AC6: rate limits.
func TestLoginRateLimitPerEmailAndIP(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 100, LoginFailsPerPair: 3, LoginFailsPerIP: 5}})
	want(t, e.register(testMail, testPW, "A"), 201, "")
	want(t, e.register("bob@example.com", testPW, "B"), 201, "")

	for i := 0; i < 3; i++ {
		want(t, e.login(testMail, "wrong-password-"+fmt.Sprint(i), ""), 401, "invalid_credentials")
	}
	blocked := e.login(testMail, "wrong-password-again", "")
	want(t, blocked, 429, "rate_limited")
	if ra := blocked.Header().Get("Retry-After"); ra == "" || ra == "0" {
		t.Fatalf("Retry-After = %q", ra)
	}
	want(t, e.login(testMail, testPW, ""), 429, "rate_limited") // even the right password is refused while blocked
	want(t, e.login(testMail, testPW, "192.0.2.77:1"), 200, "") // another IP is not affected

	// Per-IP bucket: 3 failures on alice already count as 3 of 5 for 192.0.2.1.
	want(t, e.login("bob@example.com", "wrong-1-xxxxxx", ""), 401, "invalid_credentials")
	want(t, e.login("bob@example.com", "wrong-2-xxxxxx", ""), 401, "invalid_credentials")
	want(t, e.login("bob@example.com", testPW, ""), 429, "rate_limited")

	e.clock.advance(15*time.Minute + time.Second)
	want(t, e.login(testMail, testPW, ""), 200, "")
}

func TestSuccessfulLoginsDoNotUseUpTheFailureBudget(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 100, LoginFailsPerPair: 2, LoginFailsPerIP: 2}})
	want(t, e.register(testMail, testPW, "A"), 201, "")
	for i := 0; i < 10; i++ {
		want(t, e.login(testMail, testPW, ""), 200, "")
	}
}

func TestParallelGuessesCannotOutrunTheLimit(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 100, LoginFailsPerPair: 5, LoginFailsPerIP: 100}})
	want(t, e.register(testMail, testPW, "A"), 201, "")
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for i := 0; i < 40; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := e.login(testMail, "wrong-guess-xxx", "").Code
			mu.Lock()
			codes[c]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes[401] != 5 || codes[429] != 35 {
		t.Fatalf("got %v, want exactly 5 judged guesses", codes)
	}
}

func TestRegisterRateLimit(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 3, LoginFailsPerPair: 10, LoginFailsPerIP: 100}})
	for i := 0; i < 3; i++ {
		want(t, e.register(fmt.Sprintf("u%d@example.com", i), testPW, "U"), 201, "")
	}
	rec := e.register("u9@example.com", testPW, "U")
	want(t, rec, 429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("Retry-After missing")
	}
	want(t, e.do(req{method: "POST", path: "/v1/auth/register", body: body("v@example.com", testPW, "V"), remote: "192.0.2.50:9"}), 201, "")
	e.clock.advance(time.Hour + time.Second)
	want(t, e.register("u10@example.com", testPW, "U"), 201, "")
	// Invalid input is refused before it can use up the budget.
	for i := 0; i < 5; i++ {
		want(t, e.register("bad", testPW, "U"), 400, "invalid_email")
	}
}

func TestClientIPAndTrustProxy(t *testing.T) {
	lim := Limits{RegisterPerHour: 100, LoginFailsPerPair: 2, LoginFailsPerIP: 100}
	for _, trust := range []bool{false, true} {
		e := newEnv(t, opts{limits: lim, trustProxy: trust})
		want(t, e.register(testMail, testPW, "A"), 201, "")
		send := func(xff string) int {
			return e.do(req{method: "POST", path: "/v1/auth/login", body: body(testMail, "wrong-password-x", ""), headers: map[string]string{"X-Forwarded-For": xff}}).Code
		}
		send("1.1.1.1")
		send("2.2.2.2") // a spoofed first hop must not give a fresh bucket
		got := send("3.3.3.3, 9.9.9.9")
		// Without trust all three share RemoteAddr: the third is blocked. With trust the last
		// hops (1.1.1.1, 2.2.2.2, 9.9.9.9) are three different clients: nobody is blocked.
		if trust && got != 401 || !trust && got != 429 {
			t.Fatalf("trustProxy=%v: third attempt got %d", trust, got)
		}
	}
}

// AC7 end to end: CSRF and content type on the real routes.
func TestCSRFAndContentTypeOnRoutes(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "A")).Value

	for name, r := range map[string]req{
		"logout without origin":   {method: "POST", path: "/v1/auth/logout", cookie: c},
		"logout wrong origin":     {method: "POST", path: "/v1/auth/logout", cookie: c, origin: "https://evil.example.com"},
		"logout null origin":      {method: "POST", path: "/v1/auth/logout", cookie: c, origin: "null"},
		"login with cookie, evil": {method: "POST", path: "/v1/auth/login", cookie: c, origin: "https://evil.example.com", body: body(testMail, testPW, "")},
	} {
		want(t, e.do(r), 403, "csrf_origin_mismatch")
		_ = name
	}
	want(t, e.do(req{method: "GET", path: "/v1/me", cookie: c}), 200, "") // the session survived all of those

	want(t, e.do(req{method: "POST", path: "/v1/auth/login", body: "email=a&password=b", contentType: "application/x-www-form-urlencoded"}), 415, "unsupported_media_type")
	want(t, e.do(req{method: "POST", path: "/v1/auth/register", body: body("z@example.com", testPW, "Z"), contentType: "text/plain"}), 415, "unsupported_media_type")
	want(t, e.do(req{method: "POST", path: "/v1/auth/register", body: body("z@example.com", testPW, "Z"), contentType: "application/json; charset=utf-8"}), 201, "")

	rec := e.do(req{method: "POST", path: "/v1/auth/logout", cookie: c, headers: map[string]string{"Referer": origin + "/settings"}})
	want(t, rec, 204, "")
}

// AC8: nothing secret in logs, nothing secret in bodies.
func TestSecretsNeverLogged(t *testing.T) {
	e := newEnv(t, opts{})
	const pw = "Zx9-very-fake-test-secret-7Qp"
	rec := e.register(testMail, pw, "A")
	c := sessionCookie(t, rec)
	e.login(testMail, pw, "")
	e.login(testMail, pw+"bad", "")
	e.do(req{method: "GET", path: "/v1/me", cookie: c.Value})
	e.do(req{method: "POST", path: "/v1/auth/logout", cookie: c.Value, origin: origin})
	var phc string
	_ = e.db.QueryRow("SELECT password_hash FROM users").Scan(&phc)

	logs := e.logs.String()
	if logs == "" {
		t.Fatal("expected access log lines")
	}
	for _, secret := range []string{pw, c.Value, phc, "Set-Cookie"} {
		if strings.Contains(logs, secret) {
			t.Fatalf("log contains %q:\n%s", secret, logs)
		}
	}
}
