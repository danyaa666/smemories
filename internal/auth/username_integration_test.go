//go:build integration

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth/oidctest"
)

type userData struct {
	ID                   string `json:"id"`
	Username             string `json:"username"`
	UsernameNextChangeAt *int64 `json:"username_next_change_at"`
}

func userOf(t *testing.T, rec *httptest.ResponseRecorder) userData {
	t.Helper()
	var out struct {
		Status string
		Data   userData
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Status != "OK" {
		t.Fatalf("%v: %s", err, rec.Body.String())
	}
	return out.Data
}

// signUp registers email and returns the session cookie value and the user.
func (e *env) signUp(email string) (string, userData) {
	e.t.Helper()
	rec := e.register(email, testPW, "Test")
	want(e.t, rec, 200, "")
	return sessionCookie(e.t, rec).Value, userOf(e.t, rec)
}

func (e *env) setUsername(cookie, name string) *httptest.ResponseRecorder {
	b, _ := json.Marshal(map[string]any{"username": name})
	return e.do(req{method: "POST", path: "/api/user/update-username", body: string(b), cookie: cookie, origin: origin})
}

func (e *env) dbUsername(id string) (name string, changedAt int64) {
	e.t.Helper()
	if err := e.db.QueryRow(`SELECT username, username_changed_at FROM user_tab WHERE public_id = ?`, id).Scan(&name, &changedAt); err != nil {
		e.t.Fatal(err)
	}
	return name, changedAt
}

// T-081 AC2: registration (password and Google) assigns user-xxxxxxxx; get-me shows it with a null next change.
func TestUsernameDefaultAtRegistration(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	cookie, u := e.signUp(testMail)
	if want := "user-" + strings.ToLower(u.ID[len(u.ID)-8:]); u.Username != want || u.UsernameNextChangeAt != nil {
		t.Fatalf("register: %+v, want username %s and no next change", u, want)
	}
	me := e.me(cookie)
	want(t, me, 200, "")
	if got := userOf(t, me); got.Username != u.Username || got.UsernameNextChangeAt != nil {
		t.Fatalf("get-me: %+v", got)
	}
	if !strings.Contains(me.Body.String(), `"username_next_change_at":null`) {
		t.Fatalf("null expected: %s", me.Body.String())
	}

	rec := e.google(p, "", oidctest.Claims{Sub: "g1", Email: "g@example.com", Name: "G"})
	gu := userOf(t, e.me(sessionCookie(t, rec).Value))
	if !regexp.MustCompile(`^user-[0-9a-z]{8}$`).MatchString(gu.Username) || gu.Username != "user-"+strings.ToLower(gu.ID[len(gu.ID)-8:]) || gu.UsernameNextChangeAt != nil {
		t.Fatalf("google: %+v", gu)
	}
	// login returns it too
	if lu := userOf(t, e.login(testMail, testPW, "")); lu.Username != u.Username {
		t.Fatalf("login: %+v", lu)
	}
}

// T-081 AC2: a clash on the default falls back to 10, 14 and 26 characters; an email clash is still an email clash.
func TestUsernameDefaultFallbackLengths(t *testing.T) {
	e := newEnv(t, opts{})
	ctx := context.Background()
	const id = "01K7Q8W3ZR5N2M4X6V8B0C1D2E"
	mk := func(email string) (User, error) {
		u := User{ID: id, Email: email, DisplayName: "x", Locale: "en", CreatedAt: 1}
		err := e.svc.store.createUser(ctx, &u, "hash", func(uint64) error { return nil })
		return u, err
	}
	// Walk the ladder: nothing taken -> 8; occupy 8 and make a new id with the same tail, and so on.
	for i := range defaultSuffixLens {
		if _, err := e.db.Exec(`DELETE FROM user_tab WHERE public_id = ?`, id); err != nil {
			t.Fatal(err)
		}
		u, err := mk(fmt.Sprintf("u%d@example.com", i))
		if err != nil {
			t.Fatalf("attempt %d: %v", i, err)
		}
		if want := defaultUsername(id, i); u.Username != want {
			t.Fatalf("attempt %d: username %q, want %q", i, u.Username, want)
		}
		if got, _ := e.dbUsername(id); got != u.Username {
			t.Fatalf("stored %q, returned %q", got, u.Username)
		}
		// keep this user's name occupied for the next round by renaming the row's public id
		if _, err := e.db.Exec(`UPDATE user_tab SET public_id = ? WHERE public_id = ?`, fmt.Sprintf("KEEP%d", i), id); err != nil {
			t.Fatal(err)
		}
	}
	// a taken email stays ErrEmailTaken even though the username also clashes
	if _, err := mk("u0@example.com"); err != ErrEmailTaken {
		t.Fatalf("duplicate email: %v", err)
	}
}

// T-081 AC4: first change at once, 7-day cooldown with Retry-After, edge at exactly 7 days, no-op keeps the cooldown unset.
func TestUpdateUsernameFlowAndCooldown(t *testing.T) {
	e := newEnv(t, opts{})
	cookie, u := e.signUp(testMail)

	// the same name again (also the own default) is a successful no-op and does not start the cooldown
	rec := e.setUsername(cookie, "  "+strings.ToUpper(u.Username)+" ")
	want(t, rec, 200, "")
	if got := userOf(t, rec); got.Username != u.Username || got.UsernameNextChangeAt != nil {
		t.Fatalf("no-op: %+v", got)
	}
	if _, at := e.dbUsername(u.ID); at != 0 {
		t.Fatalf("no-op started the cooldown: %d", at)
	}

	t0 := e.clock.now()
	rec = e.setUsername(cookie, "  Alice.Bee ")
	want(t, rec, 200, "")
	got := userOf(t, rec)
	wantNext := t0.Add(7 * 24 * time.Hour).UnixMilli()
	if got.Username != "alice.bee" || got.UsernameNextChangeAt == nil || *got.UsernameNextChangeAt != wantNext {
		t.Fatalf("first change: %+v, want next %d", got, wantNext)
	}
	if name, at := e.dbUsername(u.ID); name != "alice.bee" || at != t0.UnixMilli() {
		t.Fatalf("stored %q at %d", name, at)
	}
	me := userOf(t, e.me(cookie))
	if me.Username != "alice.bee" || me.UsernameNextChangeAt == nil || *me.UsernameNextChangeAt != wantNext {
		t.Fatalf("get-me: %+v", me)
	}

	// within 7 days: 429 with Retry-After and the allowed time; the stored name is unchanged
	e.clock.advance(24 * time.Hour)
	rec = e.setUsername(cookie, "alice.cee")
	want(t, rec, 429, "ERROR_USERNAME_CHANGE_TOO_SOON")
	if ra, _ := strconv.Atoi(rec.Header().Get("Retry-After")); ra != 6*24*3600 {
		t.Fatalf("Retry-After %q, want %d", rec.Header().Get("Retry-After"), 6*24*3600)
	}
	if !strings.Contains(rec.Body.String(), t0.Add(7*24*time.Hour).UTC().Format(time.RFC3339)) {
		t.Fatalf("message lacks the allowed time: %s", rec.Body.String())
	}
	// the no-op of the current name is still fine during the cooldown
	want(t, e.setUsername(cookie, "alice.bee"), 200, "")

	e.clock.advance(6*24*time.Hour - time.Millisecond) // 1 ms short of 7 days
	want(t, e.setUsername(cookie, "alice.cee"), 429, "ERROR_USERNAME_CHANGE_TOO_SOON")
	e.clock.advance(time.Millisecond) // exactly 7 days
	rec = e.setUsername(cookie, "alice.cee")
	want(t, rec, 200, "")
	if got := userOf(t, rec); got.Username != "alice.cee" || got.UsernameNextChangeAt == nil {
		t.Fatalf("after 7 days: %+v", got)
	}
	// and the user can go back to their own default later
	e.clock.advance(7 * 24 * time.Hour)
	want(t, e.setUsername(cookie, u.Username), 200, "")
}

// T-081 AC3 over HTTP, AC6: invalid, reserved and taken names; case-insensitive clash; a deleted user's name is free.
func TestUpdateUsernameRejections(t *testing.T) {
	e := newEnv(t, opts{})
	ca, ua := e.signUp("a@example.com")
	cb, _ := e.signUp("b@example.com")
	want(t, e.setUsername(ca, "alice"), 200, "")

	for name, code := range map[string]string{
		"": "ERROR_INVALID_USERNAME", "   ": "ERROR_INVALID_USERNAME", "ab": "ERROR_INVALID_USERNAME", "a..b": "ERROR_INVALID_USERNAME",
		"-ab": "ERROR_INVALID_USERNAME", "ab_": "ERROR_INVALID_USERNAME", strings.Repeat("a", 31): "ERROR_INVALID_USERNAME",
		"ánh": "ERROR_INVALID_USERNAME", "Klice": "ERROR_INVALID_USERNAME", "al ice": "ERROR_INVALID_USERNAME",
		"<b>x</b>": "ERROR_INVALID_USERNAME", "x'; DROP TABLE user_tab;--": "ERROR_INVALID_USERNAME",
		"admin": "ERROR_USERNAME_RESERVED", "ADMIN": "ERROR_USERNAME_RESERVED", "user-abcdefgh": "ERROR_USERNAME_RESERVED",
		"alice": "ERROR_USERNAME_TAKEN", "Alice": "ERROR_USERNAME_TAKEN", " ALICE ": "ERROR_USERNAME_TAKEN",
	} {
		rec := e.setUsername(cb, name)
		if rec.Code == 200 || errCode(t, rec) != code {
			t.Errorf("%q: %d %s, want %s", name, rec.Code, rec.Body.String(), code)
		}
	}
	// rejections do not start the cooldown, and the user is untouched
	if _, at := e.dbUsername(ua.ID); at == 0 {
		t.Fatal("A's own change should have started A's cooldown")
	}
	// a body that is not {"username": string} is a plain parameter error
	for _, b := range []string{`{"username": 5}`, `{"username": null, "x": 1}`, `[]`, `{`, ``} {
		rec := e.do(req{method: "POST", path: "/api/user/update-username", body: b, cookie: cb, origin: origin})
		if rec.Code != 400 && b != "" {
			t.Errorf("%q: %d %s", b, rec.Code, rec.Body.String())
		}
	}
	// a name freed by a hard delete can be claimed
	if _, err := e.db.Exec(`DELETE FROM user_tab WHERE public_id = ?`, ua.ID); err != nil {
		t.Fatal(err)
	}
	want(t, e.setUsername(cb, "alice"), 200, "")
	// the deleted user's session no longer works
	want(t, e.setUsername(ca, "other"), 401, "ERROR_UNAUTHORIZED")
}

func TestUpdateUsernameAuthAndCSRF(t *testing.T) {
	e := newEnv(t, opts{})
	c, _ := e.signUp(testMail)
	want(t, e.do(req{method: "POST", path: "/api/user/update-username", body: `{"username":"alice"}`, origin: origin}), 401, "ERROR_UNAUTHORIZED")
	want(t, e.do(req{method: "POST", path: "/api/user/update-username", body: `{"username":"alice"}`, cookie: c}), 403, "ERROR_CSRF_ORIGIN_MISMATCH")
	want(t, e.do(req{method: "POST", path: "/api/user/update-username", body: `{"username":"alice"}`, cookie: c, origin: "https://evil.example"}), 403, "ERROR_CSRF_ORIGIN_MISMATCH")
	want(t, e.do(req{method: "GET", path: "/api/user/update-username", cookie: c}), 405, "")
	if n := e.count(`SELECT COUNT(*) FROM user_tab WHERE username = 'alice'`); n != 0 {
		t.Fatal("a rejected request changed the name")
	}
}

// T-081 AC6: the 21st failed attempt in an hour is 429; successes, no-ops and cooldown answers are not counted; the cooldown
// answer does not depend on whether the name exists.
func TestUpdateUsernameFailureLimit(t *testing.T) {
	e := newEnv(t, opts{})
	c, _ := e.signUp("a@example.com")
	cb, _ := e.signUp("b@example.com")
	want(t, e.setUsername(cb, "taken.name"), 200, "")

	// cooldown answers are free and identical for a free and for a taken name
	want(t, e.setUsername(c, "free.name"), 200, "")
	for i := 0; i < 30; i++ {
		name := "taken.name"
		if i%2 == 0 {
			name = fmt.Sprintf("free.name%d", i)
		}
		want(t, e.setUsername(c, name), 429, "ERROR_USERNAME_CHANGE_TOO_SOON")
	}
	e.clock.advance(7 * 24 * time.Hour)
	for i := 0; i < 20; i++ {
		want(t, e.setUsername(c, "taken.name"), 409, "ERROR_USERNAME_TAKEN")
	}
	rec := e.setUsername(c, "taken.name")
	want(t, rec, 429, "ERROR_RATE_LIMITED")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	// even a valid, free name waits for the window
	want(t, e.setUsername(c, "free.again"), 429, "ERROR_RATE_LIMITED")
	want(t, e.setUsername(cb, "b.second"), 200, "") // the limit is per user
	if n := e.count(`SELECT COUNT(*) FROM user_tab WHERE username = 'free.again'`); n != 0 {
		t.Fatal("a limited request changed the name")
	}
}

// T-081 AC5: two users claim one name at once -> one 200 and one 409; one user fires two changes at once -> one wins.
func TestUpdateUsernameConcurrency(t *testing.T) {
	e := newEnv(t, opts{})
	ca, _ := e.signUp("a@example.com")
	cb, _ := e.signUp("b@example.com")
	cc, uc := e.signUp("c@example.com")

	race := func(fns ...func() *httptest.ResponseRecorder) map[int]int {
		var wg sync.WaitGroup
		var mu sync.Mutex
		codes := map[int]int{}
		start := make(chan struct{})
		for _, f := range fns {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				rec := f()
				mu.Lock()
				codes[rec.Code]++
				mu.Unlock()
			}()
		}
		close(start)
		wg.Wait()
		return codes
	}
	for round := 0; round < 3; round++ {
		name := fmt.Sprintf("contested%d", round)
		got := race(func() *httptest.ResponseRecorder { return e.setUsername(ca, name) }, func() *httptest.ResponseRecorder { return e.setUsername(cb, name) })
		if got[200] != 1 || got[409] != 1 {
			t.Fatalf("round %d: %v, want one 200 and one 409", round, got)
		}
		e.clock.advance(7 * 24 * time.Hour)
	}
	if n := e.count(`SELECT COUNT(*) FROM (SELECT username FROM user_tab GROUP BY username HAVING COUNT(*) > 1) d`); n != 0 {
		t.Fatal("duplicate usernames stored")
	}
	for round := 0; round < 3; round++ {
		got := race(
			func() *httptest.ResponseRecorder { return e.setUsername(cc, fmt.Sprintf("c.first%d", round)) },
			func() *httptest.ResponseRecorder { return e.setUsername(cc, fmt.Sprintf("c.second%d", round)) },
			func() *httptest.ResponseRecorder { return e.setUsername(cc, fmt.Sprintf("c.third%d", round)) })
		if got[200] != 1 || got[429] != 2 {
			t.Fatalf("round %d: %v, want one 200 and two 429", round, got)
		}
		e.clock.advance(7 * 24 * time.Hour)
	}
	if _, at := e.dbUsername(uc.ID); at == 0 {
		t.Fatal("no change recorded")
	}
}
