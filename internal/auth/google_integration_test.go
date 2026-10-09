//go:build integration

package auth

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth/oidctest"
)

func newGoogleEnv(t *testing.T, o opts) (*env, *oidctest.Provider) {
	t.Helper()
	p := oidctest.New(t)
	o.google = p
	return newEnv(t, o), p
}

func (e *env) gStart(returnTo, remote string) *httptest.ResponseRecorder {
	path := "/v1/auth/google/start"
	if returnTo != "" {
		path += "?return_to=" + url.QueryEscape(returnTo)
	}
	return e.do(req{method: "GET", path: path, remote: remote})
}

func (e *env) gCallback(q url.Values, oidc, session, remote string) *httptest.ResponseRecorder {
	return e.do(req{method: "GET", path: "/v1/auth/google/callback?" + q.Encode(), oidc: oidc, cookie: session, remote: remote})
}

func cookieNamed(rec *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range rec.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// flow is one browser trip to the provider and back, paused just before the callback.
type flow struct {
	e     *env
	p     *oidctest.Provider
	start *httptest.ResponseRecorder
	oidc  string // smem_oidc cookie value
	state string
	code  string
}

func (e *env) newFlow(p *oidctest.Provider, returnTo string, c oidctest.Claims) *flow {
	e.t.Helper()
	rec := e.gStart(returnTo, "")
	if rec.Code != 302 {
		e.t.Fatalf("start: %d %s", rec.Code, rec.Body.String())
	}
	loc := rec.Header().Get("Location")
	u, err := url.Parse(loc)
	if err != nil {
		e.t.Fatal(err)
	}
	f := &flow{e: e, p: p, start: rec, state: u.Query().Get("state")}
	if ck := cookieNamed(rec, oidcCookie); ck != nil {
		f.oidc = ck.Value
	}
	f.code = p.Authorize(loc, c)
	return f
}

func (f *flow) callback(session string) *httptest.ResponseRecorder {
	return f.e.gCallback(url.Values{"code": {f.code}, "state": {f.state}}, f.oidc, session, "")
}

// google runs a complete sign-in and returns the callback response.
func (e *env) google(p *oidctest.Provider, returnTo string, c oidctest.Claims) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.newFlow(p, returnTo, c).callback("")
}

func wantRedirect(t *testing.T, rec *httptest.ResponseRecorder, loc string) {
	t.Helper()
	if rec.Code != 302 || rec.Header().Get("Location") != loc {
		t.Fatalf("got %d Location=%q, want 302 %q (body %q)", rec.Code, rec.Header().Get("Location"), loc, rec.Body.String())
	}
}

func (e *env) me(session string) *httptest.ResponseRecorder {
	return e.do(req{method: "GET", path: "/v1/me", cookie: session})
}

func (e *env) nothingCreated() {
	e.t.Helper()
	if n := e.count(`SELECT COUNT(*) FROM users`); n != 0 {
		e.t.Errorf("%d users created", n)
	}
	if n := e.sessionCount(); n != 0 {
		e.t.Errorf("%d sessions created", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM user_identities`); n != 0 {
		e.t.Errorf("%d identities created", n)
	}
}

// AC1
func TestGoogleStartRedirect(t *testing.T) {
	for _, secure := range []bool{false, true} {
		e, p := newGoogleEnv(t, opts{secure: secure})
		rec := e.gStart("/yearbooks", "")
		if rec.Code != 302 {
			t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
		}
		loc := rec.Header().Get("Location")
		if !strings.HasPrefix(loc, p.URL+"/authorize?") {
			t.Fatalf("Location %q", loc)
		}
		u, _ := url.Parse(loc)
		q := u.Query()
		if q.Get("response_type") != "code" || q.Get("scope") != "openid email profile" || q.Get("client_id") != oidctest.ClientID ||
			q.Get("redirect_uri") != testBase+"/api/v1/auth/google/callback" || q.Get("code_challenge_method") != "S256" {
			t.Errorf("query: %v", q)
		}
		for _, k := range []string{"state", "nonce", "code_challenge"} {
			if len(q.Get(k)) < 43 {
				t.Errorf("%s too short: %q", k, q.Get(k))
			}
		}
		if rec.Header().Get("Cache-Control") != "no-store" {
			t.Errorf("Cache-Control %q", rec.Header().Get("Cache-Control"))
		}
		c := cookieNamed(rec, oidcCookie)
		if c == nil || !c.HttpOnly || c.SameSite != http.SameSiteLaxMode || c.MaxAge != 600 || c.Secure != secure || c.Path != "/" {
			t.Fatalf("cookie: %+v", c)
		}
		// The cookie holds the same state and nonce as the URL, plus the verifier and return_to.
		r := httptest.NewRequest("GET", "/", nil)
		r.AddCookie(c)
		st, ok := e.ah.google.open(r, e.clock.now())
		if !ok || st.State != q.Get("state") || st.Nonce != q.Get("nonce") || st.ReturnTo != "/yearbooks" || st.Verifier == "" {
			t.Errorf("cookie content %+v does not match the URL", st)
		}
	}
}

// AC9
func TestGoogleDisabledAnswers404(t *testing.T) {
	e := newEnv(t, opts{})
	for _, path := range []string{"/v1/auth/google/start", "/v1/auth/google/callback?code=x&state=y"} {
		want(t, e.do(req{method: "GET", path: path}), 404, "not_found")
	}
}

// AC6
func TestGoogleCreatesSocialOnlyAccount(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	rec := e.google(p, "/yearbooks/new", oidctest.Claims{Sub: "sub-vi", Email: "Linh.Nguyen@Example.com", Name: "  Nguyễn Thị Linh ", Locale: "vi-VN"})
	wantRedirect(t, rec, "/yearbooks/new")
	if c := cookieNamed(rec, oidcCookie); c == nil || c.MaxAge >= 0 {
		t.Errorf("smem_oidc must be cleared: %+v", c)
	}
	sc := sessionCookie(t, rec)
	if !sc.HttpOnly || sc.SameSite != http.SameSiteLaxMode {
		t.Errorf("session cookie flags: %+v", sc)
	}
	me := e.me(sc.Value)
	want(t, me, 200, "")
	for _, s := range []string{`"email":"linh.nguyen@example.com"`, `"email_verified":true`, `"display_name":"Nguyễn Thị Linh"`, `"locale":"vi"`} {
		if !strings.Contains(me.Body.String(), s) {
			t.Errorf("/v1/me lacks %s: %s", s, me.Body.String())
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM users WHERE password_hash IS NULL AND email_verified_at IS NOT NULL`); n != 1 {
		t.Errorf("want one verified social-only user, got %d", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM user_identities WHERE provider='google' AND subject='sub-vi' AND email='linh.nguyen@example.com'`); n != 1 {
		t.Errorf("identity row missing")
	}
	// A social-only account cannot sign in with a password and looks like an unknown account.
	want(t, e.login("linh.nguyen@example.com", testPW, ""), 401, "invalid_credentials")
}

// AC6: display name and locale fallbacks.
func TestGoogleNameAndLocaleFallbacks(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	for i, c := range []struct{ name, locale, wantName, wantLocale string }{
		{"", "", "kim", "en"},
		{"   ", "fr", "kim", "en"},
		{"Bad\u202eName", "vi", "kim", "vi"},
		{strings.Repeat("x", 101), "VI", "kim", "vi"},
		{"Kim", "en-GB", "Kim", "en"},
	} {
		email := fmt.Sprintf("kim%d@example.com", i)
		rec := e.google(p, "", oidctest.Claims{Sub: fmt.Sprintf("s%d", i), Email: email, Name: c.name, Locale: c.locale})
		wantRedirect(t, rec, "/")
		wantName := strings.Replace(c.wantName, "kim", fmt.Sprintf("kim%d", i), 1)
		var name, locale string
		if err := e.db.QueryRow(`SELECT display_name, locale FROM users WHERE email = ?`, email).Scan(&name, &locale); err != nil {
			t.Fatal(err)
		}
		if name != wantName || locale != c.wantLocale {
			t.Errorf("%+v: got %q/%q", c, name, locale)
		}
	}
}

// AC3
func TestGoogleKnownIdentitySignsInAndRevokesOldSession(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	first := e.google(p, "", oidctest.Claims{Sub: "same-sub", Email: "a@example.com", Name: "A"})
	old := sessionCookie(t, first).Value
	// The email at Google changed, the subject did not: still the same account.
	f := e.newFlow(p, "/next", oidctest.Claims{Sub: "same-sub", Email: "A.Renamed@Example.com", Name: "Other"})
	rec := f.callback(old)
	wantRedirect(t, rec, "/next")
	fresh := sessionCookie(t, rec).Value
	if fresh == old {
		t.Fatal("session token reused")
	}
	want(t, e.me(old), 401, "unauthenticated")
	want(t, e.me(fresh), 200, "")
	if n := e.count(`SELECT COUNT(*) FROM users`); n != 1 {
		t.Errorf("%d users, want 1", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM user_identities`); n != 1 {
		t.Errorf("%d identities, want 1", n)
	}
}

// AC4
func TestGoogleLinksVerifiedLocalAccount(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	want(t, e.register("alice@example.com", testPW, "Alice Local"), 201, "")
	if _, err := e.db.Exec(`UPDATE users SET email_verified_at = NOW(6)`); err != nil {
		t.Fatal(err)
	}
	rec := e.google(p, "", oidctest.Claims{Sub: "alice-sub", Email: "Alice@EXAMPLE.com", Name: "Alice Google"})
	wantRedirect(t, rec, "/")
	want(t, e.me(sessionCookie(t, rec).Value), 200, "")
	if n := e.count(`SELECT COUNT(*) FROM users`); n != 1 {
		t.Fatalf("%d users, want 1", n)
	}
	var name string
	_ = e.db.QueryRow(`SELECT display_name FROM users`).Scan(&name)
	if name != "Alice Local" {
		t.Errorf("display name overwritten: %q", name)
	}
	if n := e.count(`SELECT COUNT(*) FROM user_identities WHERE subject = 'alice-sub'`); n != 1 {
		t.Error("identity not linked")
	}
	// The password keeps working: the account was verified, nothing was taken over.
	want(t, e.login("alice@example.com", testPW, ""), 200, "")
}

// AC5
func TestGoogleDefeatsPreHijacking(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	reg := e.register("victim@example.com", testPW, "Attacker Chosen Name")
	want(t, reg, 201, "")
	attacker := sessionCookie(t, reg).Value
	want(t, e.me(attacker), 200, "")
	other := e.login("victim@example.com", testPW, "")
	want(t, other, 200, "")
	attacker2 := sessionCookie(t, other).Value

	rec := e.google(p, "", oidctest.Claims{Sub: "victim-sub", Email: "victim@example.com", Name: "The Victim"})
	wantRedirect(t, rec, "/")
	want(t, e.me(sessionCookie(t, rec).Value), 200, "")

	want(t, e.login("victim@example.com", testPW, ""), 401, "invalid_credentials")
	want(t, e.me(attacker), 401, "unauthenticated")
	want(t, e.me(attacker2), 401, "unauthenticated")
	if n := e.count(`SELECT COUNT(*) FROM users WHERE password_hash IS NULL AND email_verified_at IS NOT NULL`); n != 1 {
		t.Errorf("account must be social-only and verified")
	}
	if n := e.count(`SELECT COUNT(*) FROM users`); n != 1 {
		t.Errorf("%d users, want 1", n)
	}
	if n := e.sessionCount(); n != 1 {
		t.Errorf("%d sessions, want only the new one", n)
	}
}

// AC2: every rejection redirects with its fixed code and leaves nothing behind.
func TestGoogleCallbackRejections(t *testing.T) {
	notTrue := "true"
	wrongNonce := "not-the-nonce"
	good := oidctest.Claims{Sub: "s", Email: "a@example.com", Name: "A"}
	with := func(f func(*oidctest.Claims)) oidctest.Claims { c := good; f(&c); return c }

	type tc struct {
		name   string
		claims oidctest.Claims
		code   string
		// tamper changes the pending flow (and may return a different smem_oidc value).
		tamper func(e *env, f *flow)
	}
	flipLast := func(s string) string {
		if strings.HasSuffix(s, "A") {
			return s[:len(s)-1] + "B"
		}
		return s[:len(s)-1] + "A"
	}
	cases := []tc{
		{name: "no cookie", claims: good, code: "oidc_state", tamper: func(_ *env, f *flow) { f.oidc = "" }},
		{name: "cookie payload altered", claims: good, code: "oidc_state", tamper: func(_ *env, f *flow) {
			p, s, _ := strings.Cut(f.oidc, ".")
			f.oidc = flipLast(p) + "." + s
		}},
		{name: "cookie signature altered", claims: good, code: "oidc_state", tamper: func(_ *env, f *flow) {
			p, s, _ := strings.Cut(f.oidc, ".")
			f.oidc = p + "." + flipLast(s)
		}},
		{name: "cookie expired", claims: good, code: "oidc_state", tamper: func(e *env, _ *flow) { e.clock.advance(oidcTTL + time.Second) }},
		{name: "state differs", claims: good, code: "oidc_state", tamper: func(_ *env, f *flow) { f.state = "other" }},
		{name: "empty state", claims: good, code: "oidc_state", tamper: func(_ *env, f *flow) { f.state = "" }},
		{name: "no code", claims: good, code: "oidc_failed", tamper: func(_ *env, f *flow) { f.code = "" }},
		{name: "unknown code", claims: good, code: "oidc_failed", tamper: func(_ *env, f *flow) { f.code = "made-up" }},
		{name: "token endpoint fails", claims: good, code: "oidc_failed", tamper: func(_ *env, f *flow) { f.p.FailToken(true) }},
		{name: "no id_token", claims: with(func(c *oidctest.Claims) { c.OmitIDToken = true }), code: "oidc_failed"},
		{name: "bad signature", claims: with(func(c *oidctest.Claims) { c.BadSignature = true }), code: "oidc_failed"},
		{name: "wrong issuer", claims: with(func(c *oidctest.Claims) { c.Issuer = "https://evil.example" }), code: "oidc_failed"},
		{name: "wrong audience", claims: with(func(c *oidctest.Claims) { c.Audience = "someone-else" }), code: "oidc_failed"},
		{name: "expired token", claims: with(func(c *oidctest.Claims) { c.ExpiresIn = -time.Minute }), code: "oidc_failed"},
		{name: "wrong nonce", claims: with(func(c *oidctest.Claims) { c.Nonce = &wrongNonce }), code: "oidc_failed"},
		{name: "no email", claims: with(func(c *oidctest.Claims) { c.Email = "" }), code: "oidc_failed"},
		{name: "unusable email", claims: with(func(c *oidctest.Claims) { c.Email = "not an email" }), code: "oidc_failed"},
		{name: "email_verified false", claims: with(func(c *oidctest.Claims) { c.EmailVerified = false }), code: "email_unverified"},
		{name: "email_verified is the string true", claims: with(func(c *oidctest.Claims) { c.EmailVerified = notTrue }), code: "email_unverified"},
		{name: "email_verified null", claims: with(func(c *oidctest.Claims) { c.EmailVerified = nilClaim{} }), code: "email_unverified"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, p := newGoogleEnv(t, opts{})
			f := e.newFlow(p, "", c.claims)
			if c.tamper != nil {
				c.tamper(e, f)
			}
			rec := f.callback("")
			wantRedirect(t, rec, "/login?error="+c.code)
			if cookieNamed(rec, CookieName) != nil {
				t.Error("a session cookie was set")
			}
			e.nothingCreated()
			for _, secret := range []string{f.code, f.state, "fake-access-token"} {
				if secret != "" && strings.Contains(e.logs.String()+rec.Body.String(), secret) {
					t.Errorf("%q leaked into logs or body", secret)
				}
			}
		})
	}
}

// nilClaim marshals to JSON null.
type nilClaim struct{}

func (nilClaim) MarshalJSON() ([]byte, error) { return []byte("null"), nil }

func TestGoogleProviderErrorIsDenied(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	f := e.newFlow(p, "", oidctest.Claims{Email: "a@example.com"})
	rec := e.gCallback(url.Values{"error": {"access_denied"}, "error_description": {"user said no"}, "state": {f.state}}, f.oidc, "", "")
	wantRedirect(t, rec, "/login?error=oidc_denied")
	if strings.Contains(rec.Header().Get("Location"), "denied_by") || strings.Contains(e.logs.String(), "user said no") {
		t.Error("provider text leaked")
	}
	if p.Calls() != 0 {
		t.Error("the token endpoint must not be called after a provider error")
	}
	e.nothingCreated()
	// An error with a wrong state is a state failure first.
	wantRedirect(t, e.gCallback(url.Values{"error": {"access_denied"}, "state": {"other"}}, f.oidc, "", ""), "/login?error=oidc_state")
}

// A code and a state work once.
func TestGoogleReplayIsRejected(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	f := e.newFlow(p, "", oidctest.Claims{Sub: "s", Email: "a@example.com", Name: "A"})
	wantRedirect(t, f.callback(""), "/")
	wantRedirect(t, f.callback(""), "/login?error=oidc_failed") // same cookie, state, code
	wantRedirect(t, e.gCallback(url.Values{"code": {f.code}, "state": {f.state}}, "", "", ""), "/login?error=oidc_state")
	if n := e.sessionCount(); n != 1 {
		t.Errorf("%d sessions, want 1", n)
	}
}

// A flow started in one browser cannot be finished in another (login CSRF).
func TestGoogleCallbackNeedsTheStartingBrowser(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	theirs := e.newFlow(p, "", oidctest.Claims{Sub: "attacker", Email: "attacker@example.com", Name: "Mallory"})
	// The attacker feeds the victim a callback URL for the attacker's own Google account; the
	// victim's browser has its own (or no) smem_oidc cookie.
	mine := e.newFlow(p, "", oidctest.Claims{Sub: "v", Email: "v@example.com"})
	rec := e.gCallback(url.Values{"code": {theirs.code}, "state": {theirs.state}}, mine.oidc, "", "")
	wantRedirect(t, rec, "/login?error=oidc_state")
	e.nothingCreated()
}

// A failed attempt leaves the visitor's existing session alone; a successful one revokes it.
func TestGoogleExistingSessionIgnoredUntilSuccess(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	reg := e.register("bob@example.com", testPW, "Bob")
	bob := sessionCookie(t, reg).Value
	f := e.newFlow(p, "", oidctest.Claims{Sub: "s", Email: "a@example.com", EmailVerified: false})
	wantRedirect(t, f.callback(bob), "/login?error=email_unverified")
	want(t, e.me(bob), 200, "")
	ok := e.newFlow(p, "", oidctest.Claims{Sub: "s", Email: "a@example.com"})
	wantRedirect(t, ok.callback(bob), "/")
	want(t, e.me(bob), 401, "unauthenticated")
}

// AC7, end to end: only same-site relative paths survive.
func TestGoogleReturnToIsSanitised(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	for in, want := range map[string]string{
		"/yearbooks/1?x=1":             "/yearbooks/1?x=1",
		"//evil.example":               "/",
		"https://evil.example":         "/",
		"/\\evil.example":              "/",
		"javascript:alert(1)":          "/",
		"/a\r\nX-Evil: 1":              "/",
		"/" + strings.Repeat("a", 199): "/" + strings.Repeat("a", 199),
		"/" + strings.Repeat("a", 200): "/",
		"":                             "/",
	} {
		rec := e.google(p, in, oidctest.Claims{Sub: "s", Email: "a@example.com", Name: "A"})
		if got := rec.Header().Get("Location"); rec.Code != 302 || got != want {
			t.Errorf("return_to %q -> %d %q, want %q", in, rec.Code, got, want)
		}
	}
}

// AC8
func TestGoogleRateLimits(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	for i := 0; i < 30; i++ {
		if rec := e.gStart("", "198.51.100.7:1"); rec.Code != 302 {
			t.Fatalf("start %d: %d", i, rec.Code)
		}
	}
	rec := e.gStart("", "198.51.100.7:1")
	want(t, rec, 429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	if cookieNamed(rec, oidcCookie) != nil {
		t.Error("a limited start must not set a cookie")
	}
	if rec := e.gStart("", "198.51.100.8:1"); rec.Code != 302 {
		t.Errorf("another IP was limited: %d", rec.Code)
	}
	// The callback has its own budget per IP.
	f := e.newFlow(p, "", oidctest.Claims{Sub: "s", Email: "a@example.com"})
	for i := 0; i < 30; i++ {
		if rec := e.gCallback(url.Values{"state": {"x"}}, "", "", "198.51.100.9:1"); rec.Code != 302 {
			t.Fatalf("callback %d: %d", i, rec.Code)
		}
	}
	want(t, e.gCallback(url.Values{"code": {f.code}, "state": {f.state}}, f.oidc, "", "198.51.100.9:1"), 429, "rate_limited")
	e.nothingCreated()
	e.clock.advance(googleRateWindow + time.Second)
	f = e.newFlow(p, "", oidctest.Claims{Sub: "s", Email: "a@example.com"})
	wantRedirect(t, e.gCallback(url.Values{"code": {f.code}, "state": {f.state}}, f.oidc, "", "198.51.100.9:1"), "/")
}

// Two callbacks for the same new person at once: one account, no error.
func TestGoogleConcurrentCallbacksCreateOneAccount(t *testing.T) { concurrentCreate(t) }

func concurrentCreate(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	const n = 6
	flows := make([]*flow, n)
	for i := range flows {
		flows[i] = e.newFlow(p, "", oidctest.Claims{Sub: "race-sub", Email: "race@example.com", Name: "Racer"})
	}
	recs := make([]*httptest.ResponseRecorder, n)
	var wg sync.WaitGroup
	for i := range flows {
		wg.Add(1)
		go func() { defer wg.Done(); recs[i] = flows[i].callback("") }()
	}
	wg.Wait()
	for i, rec := range recs {
		if rec.Code != 302 || rec.Header().Get("Location") != "/" {
			t.Errorf("callback %d: %d %s", i, rec.Code, rec.Header().Get("Location"))
		}
	}
	if u, id := e.count(`SELECT COUNT(*) FROM users`), e.count(`SELECT COUNT(*) FROM user_identities`); u != 1 || id != 1 {
		t.Errorf("%d users, %d identities, want 1 and 1", u, id)
	}
}

// Same race when the local account exists unverified: it is cleaned once and linked once.
func TestGoogleConcurrentCallbacksLinkOnce(t *testing.T) { concurrentLink(t) }

func concurrentLink(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	want(t, e.register("race@example.com", testPW, "Local"), 201, "")
	const n = 4
	flows := make([]*flow, n)
	for i := range flows {
		flows[i] = e.newFlow(p, "", oidctest.Claims{Sub: "race-sub", Email: "race@example.com"})
	}
	var wg sync.WaitGroup
	codes := make([]int, n)
	for i := range flows {
		wg.Add(1)
		go func() { defer wg.Done(); codes[i] = flows[i].callback("").Code }()
	}
	wg.Wait()
	for i, c := range codes {
		if c != 302 {
			t.Errorf("callback %d: %d", i, c)
		}
	}
	if u, id := e.count(`SELECT COUNT(*) FROM users`), e.count(`SELECT COUNT(*) FROM user_identities`); u != 1 || id != 1 {
		t.Errorf("%d users, %d identities, want 1 and 1", u, id)
	}
	want(t, e.login("race@example.com", testPW, ""), 401, "invalid_credentials")
}

// T-047: the two races above, 30 times each on a fresh database, must never fail (CI flake).
// Run it with GOMAXPROCS=2 to match a small runner.
func TestGoogleConcurrentCallbacksStress(t *testing.T) {
	for i := range 30 {
		t.Run(fmt.Sprint(i), func(t *testing.T) { concurrentCreate(t); concurrentLink(t) })
	}
}

// Discovery trouble is a clean redirect, not a 500.
func TestGoogleProviderDown(t *testing.T) {
	p := oidctest.New(t)
	e := newEnv(t, opts{google: p})
	p.Close()
	wantRedirect(t, e.gStart("", ""), "/login?error=oidc_failed")
	if cookieNamed(e.gStart("", ""), oidcCookie) != nil {
		t.Error("no cookie when the provider is unreachable")
	}
}

// The authorization code, tokens and the ID token never reach the logs, even on success.
func TestGoogleSecretsNeverLogged(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	f := e.newFlow(p, "", oidctest.Claims{Sub: "s", Email: "a@example.com", Name: "A"})
	rec := f.callback("")
	wantRedirect(t, rec, "/")
	sess := sessionCookie(t, rec).Value
	out := e.logs.String()
	for _, secret := range []string{f.code, f.state, f.oidc, "fake-access-token", sess, oidctest.ClientSecret} {
		if secret != "" && strings.Contains(out, secret) {
			t.Errorf("%q leaked into the logs", secret)
		}
	}
	if !strings.Contains(out, "auth: google sign-in") {
		t.Error("expected an audit log line")
	}
}
