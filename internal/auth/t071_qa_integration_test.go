//go:build integration

package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/auth/oidctest"
)

// T-071 QA: a wrong or missing Origin on a request that carries the session cookie is refused on every POST route,
// the session survives all of them, and a right Origin still works.
func TestQAWrongOriginOnEveryPost(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "A")).Value
	posts := map[string]string{
		"/api/auth/register":            regBody("qa@example.com", testPW, "Q"),
		"/api/auth/login":               body(testMail, testPW, ""),
		"/api/auth/logout":              "",
		"/api/user/accept-terms":        `{"version":"2026-10-21"}`,
		"/api/auth/verify-email":        `{"code":"000000"}`,
		"/api/auth/resend-verification": "",
		"/api/auth/forgot-password":     `{"email":"alice@example.com"}`,
		"/api/auth/reset-password":      `{"email":"alice@example.com","code":"000000","password":"` + testPW + `x"}`,
	}
	for path, b := range posts {
		for _, o := range []string{"", "null", "https://evil.example.com", "http://localhost:5173.evil.example", "http://localhost:5174", "https://localhost:5173"} {
			rec := e.do(req{method: "POST", path: path, cookie: c, origin: o, body: b})
			if rec.Code != 403 || errCode(t, rec) != "ERROR_CSRF_ORIGIN_MISMATCH" {
				t.Errorf("POST %s origin=%q: %d %s", path, o, rec.Code, rec.Body.String())
			}
		}
	}
	want(t, e.me(c), 200, "")
	if rec := e.do(req{method: "POST", path: "/api/auth/resend-verification", cookie: c, origin: origin}); rec.Code != 200 {
		t.Errorf("right origin: %d %s", rec.Code, rec.Body.String())
	}
}

// T-071 QA: a client that went away (cancelled request context) is 499 ERROR_CLIENT_CLOSED on both a protected and a
// public route; never 500/503, and it leaves no ERROR line in the log.
func TestQAClientCancelIs499WithoutErrorLogs(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "A")).Value
	for _, r := range []req{
		{method: "GET", path: "/api/user/get-me", cookie: c},
		{method: "POST", path: "/api/auth/login", body: body(testMail, testPW, "")},
		{method: "POST", path: "/api/auth/forgot-password", body: `{"email":"alice@example.com"}`},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		hr := httptest.NewRequestWithContext(ctx, r.method, r.path, strings.NewReader(r.body))
		if r.body != "" {
			hr.Header.Set("Content-Type", "application/json")
		}
		if r.cookie != "" {
			hr.AddCookie(&http.Cookie{Name: CookieName, Value: r.cookie})
		}
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, hr)
		if rec.Code != 499 || errCode(t, rec) != "ERROR_CLIENT_CLOSED" {
			t.Errorf("%s %s: %d %s", r.method, r.path, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Set-Cookie") != "" || strings.Contains(rec.Body.String(), "context") {
			t.Errorf("%s: cookie or internals in the answer: %v %s", r.path, rec.Header(), rec.Body.String())
		}
	}
	if strings.Contains(e.logs.String(), `"level":"ERROR"`) {
		t.Errorf("error log lines for a client cancel:\n%s", e.logs.String())
	}
	want(t, e.me(c), 200, "") // and the session is intact
}

// T-071 QA: the callback takes its redirect target only from the signed cookie, never from its own query string.
func TestQAGoogleCallbackIgnoresQueryReturnToAndRedirect(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	f := e.newFlow(p, "/yearbooks", oidctest.Claims{Sub: "s", Email: "a@example.com", Name: "A"})
	rec := e.gCallback(url.Values{"code": {f.code}, "state": {f.state}, "return_to": {"https://evil.example"}, "redirect": {"https://evil.example"}, "next": {"//evil.example"}}, f.oidc, "", "")
	wantRedirect(t, rec, "/yearbooks")

	// A callback pairing one flow's code+state with another flow's signed cookie (and so its PKCE verifier) is refused.
	e2, p2 := newGoogleEnv(t, opts{})
	a := e2.newFlow(p2, "", oidctest.Claims{Sub: "x", Email: "x@example.com", Name: "X"})
	b := e2.newFlow(p2, "", oidctest.Claims{Sub: "y", Email: "y@example.com", Name: "Y"})
	// code and state of flow a, but the (validly signed) cookie of flow b: state differs first.
	wantRedirect(t, e2.gCallback(url.Values{"code": {a.code}, "state": {a.state}}, b.oidc, "", ""), "/login?error=oidc_state")
	e2.nothingCreated()
}
