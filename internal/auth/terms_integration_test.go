//go:build integration

package auth

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth/oidctest"
	"github.com/danyaa666/smemories/internal/legal"
)

// T-111 AC1: register needs the current version; a refusal creates nothing; the stored values are the version and a Unix ms time.
func TestRegisterRequiresTheCurrentTermsVersion(t *testing.T) {
	e := newEnv(t, opts{})
	for name, b := range map[string]string{
		"field missing": `{"email":"a@example.com","password":"` + testPW + `","display_name":"A"}`,
		"empty":         `{"email":"a@example.com","password":"` + testPW + `","display_name":"A","accept_terms_version":""}`,
		"old version":   `{"email":"a@example.com","password":"` + testPW + `","display_name":"A","accept_terms_version":"2020-01-01"}`,
		"wrong type":    `{"email":"a@example.com","password":"` + testPW + `","display_name":"A","accept_terms_version":true}`,
	} {
		rec := e.do(req{method: "POST", path: "/api/auth/register", body: b})
		wantCode := "ERROR_TERMS_REQUIRED"
		wantStatus := 400
		if name == "wrong type" { // not a string: the body itself is invalid
			wantCode = "ERROR_PARAM"
		}
		if rec.Code != wantStatus || errCode(t, rec) != wantCode {
			t.Errorf("%s: %d %s", name, rec.Code, rec.Body.String())
		}
		if rec.Header().Get("Set-Cookie") != "" {
			t.Errorf("%s: no cookie on refusal", name)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM user_tab`); n != 0 || e.sessionCount() != 0 || len(e.mail.all()) != 0 {
		t.Fatalf("refused registrations created %d users, %d sessions, %d mails", n, e.sessionCount(), len(e.mail.all()))
	}
	want(t, e.register("a@example.com", testPW, "A"), 200, "")
	var v string
	var at int64
	if err := e.db.QueryRow(`SELECT terms_version, terms_accepted_at FROM user_tab`).Scan(&v, &at); err != nil {
		t.Fatal(err)
	}
	if v != legal.CurrentVersion || at != e.clock.now().UnixMilli() {
		t.Errorf("stored %q at %d, want %q at %d", v, at, legal.CurrentVersion, e.clock.now().UnixMilli())
	}
}

type meBody struct {
	Data struct {
		Terms struct {
			AcceptedVersion *string `json:"accepted_version"`
			CurrentVersion  string  `json:"current_version"`
			Required        bool    `json:"required"`
		} `json:"terms"`
	} `json:"data"`
}

func termsOfMe(t *testing.T, e *env, cookie string) (accepted *string, required bool) {
	t.Helper()
	rec := e.me(cookie)
	want(t, rec, 200, "")
	var m meBody
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if m.Data.Terms.CurrentVersion != legal.CurrentVersion {
		t.Errorf("current_version %q", m.Data.Terms.CurrentVersion)
	}
	return m.Data.Terms.AcceptedVersion, m.Data.Terms.Required
}

// T-111 AC2: a user with a null or old version is stopped on every signed-in endpoint but the four exceptions, and accept-terms opens the gate.
func TestTermsGate(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "A")).Value
	if acc, req := termsOfMe(t, e, c); acc == nil || *acc != legal.CurrentVersion || req {
		t.Fatalf("after register: accepted %v required %v", acc, req)
	}
	update := func() int {
		return e.do(req{method: "POST", path: "/api/user/update-username", body: `{"username":"alice"}`, cookie: c, origin: origin}).Code
	}
	if update() != 200 {
		t.Fatal("a user who accepted must pass the gate")
	}
	for name, set := range map[string]string{"never accepted": `NULL`, "an older version": `'2020-01-01'`} {
		if _, err := e.db.Exec(`UPDATE user_tab SET terms_version = ` + set); err != nil {
			t.Fatal(err)
		}
		rec := e.do(req{method: "POST", path: "/api/user/update-username", body: `{"username":"alice2"}`, cookie: c, origin: origin})
		if rec.Code != 403 || errCode(t, rec) != "ERROR_TERMS_REQUIRED" {
			t.Fatalf("%s: gated endpoint answered %d %s", name, rec.Code, rec.Body.String())
		}
		if acc, required := termsOfMe(t, e, c); !required || (name == "never accepted") != (acc == nil) {
			t.Errorf("%s: get-me accepted %v required %v", name, acc, required)
		}
		// The exceptions.
		want(t, e.do(req{method: "POST", path: "/api/auth/resend-verification", cookie: c, origin: origin}), 200, "")
		rec = e.do(req{method: "POST", path: "/api/auth/verify-email", body: `{"code":"000001"}`, cookie: c, origin: origin})
		if rec.Code != 400 || errCode(t, rec) != "ERROR_INVALID_CODE" { // reached the handler: a wrong code, not the gate
			t.Errorf("%s: verify-email %d %s", name, rec.Code, rec.Body.String())
		}
	}
	// accept-terms: a stale page, the wrong type, no session, a wrong origin; then the real one, twice.
	post := func(body, cookie, o string) int {
		return e.do(req{method: "POST", path: "/api/user/accept-terms", body: body, cookie: cookie, origin: o}).Code
	}
	if post(`{"version":"2020-01-01"}`, c, origin) != 400 || post(`{"version":""}`, c, origin) != 400 || post(`{}`, c, origin) != 400 {
		t.Error("an old, empty or missing version must be ERROR_PARAM")
	}
	if post(`{"version":"`+legal.CurrentVersion+`"}`, "", origin) != 401 {
		t.Error("accept-terms needs a session")
	}
	if post(`{"version":"`+legal.CurrentVersion+`"}`, c, "https://evil.example.com") != 403 {
		t.Error("accept-terms is behind the CSRF guard")
	}
	if update() != 403 {
		t.Fatal("refused attempts must not open the gate")
	}
	var firstAt int64
	for i := range 2 { // repeating it is harmless and keeps the time of the first acceptance
		e.clock.advance(time.Minute)
		rec := e.do(req{method: "POST", path: "/api/user/accept-terms", body: `{"version":"` + legal.CurrentVersion + `"}`, cookie: c, origin: origin})
		want(t, rec, 200, "")
		if !strings.Contains(rec.Body.String(), `"required":false`) {
			t.Errorf("accept-terms: %s", rec.Body.String())
		}
		var at int64
		if err := e.db.QueryRow(`SELECT terms_accepted_at FROM user_tab`).Scan(&at); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstAt = at
		} else if at != firstAt {
			t.Errorf("a repeated accept moved terms_accepted_at from %d to %d", firstAt, at)
		}
	}
	if acc, required := termsOfMe(t, e, c); acc == nil || *acc != legal.CurrentVersion || required {
		t.Errorf("after accept: %v %v", acc, required)
	}
	if update() != 200 {
		t.Error("after accept-terms the gated endpoint works")
	}
	// Logout needs no acceptance.
	if _, err := e.db.Exec(`UPDATE user_tab SET terms_version = NULL`); err != nil {
		t.Fatal(err)
	}
	want(t, e.do(req{method: "POST", path: "/api/auth/logout", cookie: c, origin: origin}), 200, "")
	want(t, e.me(c), 401, "ERROR_UNAUTHORIZED")
}

// T-111: a new Google account has not accepted anything, so it is gated until accept-terms.
func TestGoogleAccountMustAcceptTerms(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	sc := sessionCookie(t, e.google(p, "/", oidctest.Claims{Sub: "sub-t", Email: "terms@example.com", Name: "T"})).Value
	if acc, required := termsOfMe(t, e, sc); acc != nil || !required {
		t.Fatalf("new google account: %v %v", acc, required)
	}
	rec := e.do(req{method: "POST", path: "/api/user/update-username", body: `{"username":"tt1"}`, cookie: sc, origin: origin})
	want(t, rec, 403, "ERROR_TERMS_REQUIRED")
	want(t, e.do(req{method: "POST", path: "/api/user/accept-terms", body: `{"version":"` + legal.CurrentVersion + `"}`, cookie: sc, origin: origin}), 200, "")
	want(t, e.do(req{method: "POST", path: "/api/user/update-username", body: `{"username":"tt1"}`, cookie: sc, origin: origin}), 200, "")
}
