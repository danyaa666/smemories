//go:build integration

package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth/oidctest"
	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/redis"
)

func sessionRow(token string, uid uint64, now time.Time, ttl time.Duration) session {
	h := sha256.Sum256([]byte(token))
	return session{tokenHash: h[:], userID: uid, now: now, expires: now.Add(ttl), userAgent: "ua"}
}

func tokenHashOf(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// AC1: the key layout, and the raw token never reaches Redis.
func TestSessionKeyLayout(t *testing.T) {
	e := newEnv(t, opts{})
	ctx := context.Background()
	now := time.Now().UTC()
	if err := e.sess.Create(ctx, sessionRow("tok-a", 7, now, time.Hour)); err != nil {
		t.Fatal(err)
	}
	rdb := e.rc.Client()
	key := e.rc.Key("sess", hex.EncodeToString(tokenHashOf("tok-a")))
	m, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != 4 || m["user_id"] != "7" || m["user_agent"] != "ua" || m["created_at"] != m["last_seen_at"] {
		t.Fatalf("hash: %v", m)
	}
	if ttl := rdb.PTTL(ctx, key).Val(); ttl <= 59*time.Minute || ttl > time.Hour {
		t.Fatalf("session TTL %v", ttl)
	}
	idx := e.rc.Key("usess", "7")
	if rdb.SIsMember(ctx, idx, hex.EncodeToString(tokenHashOf("tok-a"))).Val() != true {
		t.Fatal("not in the user's index")
	}
	if ttl := rdb.PTTL(ctx, idx).Val(); ttl <= 59*time.Minute || ttl > time.Hour {
		t.Fatalf("index TTL %v", ttl)
	}
	for _, k := range rdb.Keys(ctx, e.rc.Key("*")).Val() {
		if strings.Contains(k, "tok-a") {
			t.Fatalf("raw token in key %q", k)
		}
	}

	// A later session pushes the index expiry out; an earlier-expiring one never shortens it.
	if err := e.sess.Create(ctx, sessionRow("tok-b", 7, now, 3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := e.sess.Create(ctx, sessionRow("tok-c", 7, now, 2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if ttl := rdb.PTTL(ctx, idx).Val(); ttl <= 179*time.Minute {
		t.Fatalf("index TTL %v, want about 3h", ttl)
	}
	if n := rdb.SCard(ctx, idx).Val(); n != 3 {
		t.Fatalf("%d index entries", n)
	}
}

// AC2: lookup, native expiry, extension, delete; unknown and expired tokens are simply not found.
func TestSessionLookupExpireExtendDelete(t *testing.T) {
	e := newEnv(t, opts{})
	ctx := context.Background()
	now := time.Now().UTC()
	if _, _, err := e.sess.Lookup(ctx, tokenHashOf("nobody")); !errors.Is(err, errNotFound) {
		t.Fatalf("unknown: %v", err)
	}
	if err := e.sess.Create(ctx, sessionRow("short", 1, now, 300*time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if err := e.sess.Create(ctx, sessionRow("long", 1, now, time.Hour)); err != nil {
		t.Fatal(err)
	}
	uid, seen, err := e.sess.Lookup(ctx, tokenHashOf("short"))
	if err != nil || uid != 1 || !seen.Equal(now.Truncate(time.Millisecond)) {
		t.Fatalf("lookup: %d %v %v", uid, seen, err)
	}
	time.Sleep(500 * time.Millisecond)
	if _, _, err := e.sess.Lookup(ctx, tokenHashOf("short")); !errors.Is(err, errNotFound) {
		t.Fatalf("expired session found: %v", err)
	}

	later := now.Add(time.Minute)
	ok, err := e.sess.Extend(ctx, tokenHashOf("long"), 1, later, 2*time.Hour)
	if err != nil || !ok {
		t.Fatalf("extend: %v %v", ok, err)
	}
	if _, seen, _ := e.sess.Lookup(ctx, tokenHashOf("long")); !seen.Equal(later.Truncate(time.Millisecond)) {
		t.Fatalf("last_seen_at %v", seen)
	}
	if ttl := e.rc.Client().PTTL(ctx, e.rc.Key("sess", hex.EncodeToString(tokenHashOf("long")))).Val(); ttl <= 119*time.Minute {
		t.Fatalf("extended TTL %v", ttl)
	}

	if err := e.sess.Delete(ctx, tokenHashOf("long")); err != nil {
		t.Fatal(err)
	}
	if err := e.sess.Delete(ctx, tokenHashOf("long")); err != nil { // twice is fine
		t.Fatal(err)
	}
	if _, _, err := e.sess.Lookup(ctx, tokenHashOf("long")); !errors.Is(err, errNotFound) {
		t.Fatalf("deleted session found: %v", err)
	}
	if e.rc.Client().SIsMember(ctx, e.rc.Key("usess", "1"), hex.EncodeToString(tokenHashOf("long"))).Val() {
		t.Fatal("index entry survived the logout")
	}
	if ok, err := e.sess.Extend(ctx, tokenHashOf("long"), 1, later, time.Hour); err != nil || ok {
		t.Fatalf("extend of a deleted session: %v %v", ok, err)
	}
	if n := e.sessionCount(); n != 0 {
		t.Fatalf("extend resurrected the session (%d keys)", n)
	}
}

// AC3: delete-all removes every session of the user (also when an index entry outlived its session), and only theirs.
func TestSessionDeleteAll(t *testing.T) {
	e := newEnv(t, opts{})
	ctx := context.Background()
	now := time.Now().UTC()
	for _, tok := range []string{"a", "b", "c"} {
		if err := e.sess.Create(ctx, sessionRow(tok, 5, now, time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	if err := e.sess.Create(ctx, sessionRow("other", 6, now, time.Hour)); err != nil {
		t.Fatal(err)
	}
	e.rc.Client().Del(ctx, e.rc.Key("sess", hex.EncodeToString(tokenHashOf("b")))) // index entry now outlives its session
	if err := e.sess.DeleteAll(ctx, 5); err != nil {
		t.Fatal(err)
	}
	for _, tok := range []string{"a", "b", "c"} {
		if _, _, err := e.sess.Lookup(ctx, tokenHashOf(tok)); !errors.Is(err, errNotFound) {
			t.Fatalf("session %s survived: %v", tok, err)
		}
	}
	if e.rc.Client().Exists(ctx, e.rc.Key("usess", "5")).Val() != 0 {
		t.Fatal("index survived")
	}
	if _, _, err := e.sess.Lookup(ctx, tokenHashOf("other")); err != nil {
		t.Fatalf("another user's session was removed: %v", err)
	}
	if err := e.sess.DeleteAll(ctx, 99); err != nil { // nobody: not an error
		t.Fatal(err)
	}
}

// A login sweeps the user's expired sessions out of the index and Redis, like the old table sweep.
func TestSessionCreateSweepsExpiredSiblings(t *testing.T) {
	e := newEnv(t, opts{})
	ctx := context.Background()
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := e.sess.Create(ctx, sessionRow("old", 3, t0, SessionTTL)); err != nil {
		t.Fatal(err)
	}
	if err := e.sess.Create(ctx, sessionRow("new", 3, t0.Add(SessionTTL+time.Second), SessionTTL)); err != nil {
		t.Fatal(err)
	}
	if n := e.sessionCount(); n != 1 {
		t.Fatalf("%d sessions, want only the new one", n)
	}
	if n := e.rc.Client().SCard(ctx, e.rc.Key("usess", "3")).Val(); n != 1 {
		t.Fatalf("%d index entries, want 1", n)
	}
}

// AC6: refreshes racing each other and a logout never bring a deleted session back.
func TestSessionRefreshRacesNeverResurrect(t *testing.T) {
	e := newEnv(t, opts{})
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 30; i++ {
		tok := "race-" + string(rune('a'+i%26)) + string(rune('a'+i/26))
		if err := e.sess.Create(ctx, sessionRow(tok, 9, now, time.Hour)); err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		for j := 0; j < 4; j++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if _, err := e.sess.Extend(ctx, tokenHashOf(tok), 9, now.Add(time.Minute), time.Hour); err != nil {
					t.Error(err)
				}
			}()
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := e.sess.Delete(ctx, tokenHashOf(tok)); err != nil {
				t.Error(err)
			}
		}()
		wg.Wait()
		if _, _, err := e.sess.Lookup(ctx, tokenHashOf(tok)); !errors.Is(err, errNotFound) {
			t.Fatalf("round %d: session survived the logout: %v", i, err)
		}
	}
	if n := e.sessionCount(); n != 0 {
		t.Fatalf("%d sessions left", n)
	}
}

// deadRedis is a client for a port nothing listens on.
func deadRedis(t *testing.T) *redis.Client {
	t.Helper()
	c, err := redis.New(config.Config{
		Env: "test", RedisURL: "redis://127.0.0.1:1/0",
		RedisDialTimeout: 200 * time.Millisecond, RedisReadTimeout: 200 * time.Millisecond, RedisWriteTimeout: 200 * time.Millisecond, RedisPoolSize: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// AC4: with Redis down, requests that need a session answer 503 session_store_unavailable, nothing is created, public
// paths without a session keep working, and the failure is logged once per interval.
func TestSessionStoreDownFailsClosed(t *testing.T) {
	e := newEnv(t, opts{})
	reg := e.register(testMail, testPW, "Alice")
	want(t, reg, 201, "")
	cookie := sessionCookie(t, reg).Value
	want(t, e.me(cookie), 200, "")

	e.sess.rc = deadRedis(t)
	for i := 0; i < 4; i++ {
		rec := e.do(req{method: "GET", path: "/v1/me", cookie: cookie})
		want(t, rec, 503, "session_store_unavailable")
		if rec.Header().Get("Retry-After") != "5" {
			t.Fatalf("Retry-After %q", rec.Header().Get("Retry-After"))
		}
	}
	want(t, e.do(req{method: "GET", path: "/v1/me"}), 401, "unauthenticated") // no cookie: nothing to look up
	want(t, e.login(testMail, testPW, ""), 503, "session_store_unavailable")
	want(t, e.login(testMail, "Wrong-fake-password-1", ""), 401, "invalid_credentials") // the account rules ran first
	want(t, e.do(req{method: "POST", path: "/v1/auth/logout", cookie: cookie, origin: origin}), 503, "session_store_unavailable")

	// Register: rules first, then the session; a failure leaves no account behind.
	want(t, e.register("bob@example.com", testPW, "Bob"), 503, "session_store_unavailable")
	if n := e.count(`SELECT COUNT(*) FROM users WHERE email = 'bob@example.com'`); n != 0 {
		t.Fatalf("%d accounts left behind by a failed register", n)
	}
	want(t, e.register("not-an-email", testPW, "Bob"), 400, "invalid_email")

	if n := strings.Count(e.logs.String(), "session store failing"); n != 1 {
		t.Fatalf("failure logged %d times, want once per interval", n)
	}
	if strings.Contains(e.logs.String(), cookie) {
		t.Fatal("session token in the logs")
	}

	// Back up: the old cookie works again, nothing was lost.
	e.sess.rc = e.rc
	want(t, e.me(cookie), 200, "")
}

// AC3 + AC4: a Redis failure during the Google pre-hijacking defence rolls the takeover back, so the next try still
// finds the account unverified and removes the squatter's sessions.
func TestGooglePreHijackingRollsBackWhenSessionsCannotBeDeleted(t *testing.T) {
	e, p := newGoogleEnv(t, opts{})
	reg := e.register("victim@example.com", testPW, "Squatter")
	want(t, reg, 201, "")
	squatter := sessionCookie(t, reg).Value
	f := e.newFlow(p, "", oidctest.Claims{Sub: "victim-sub", Email: "victim@example.com", Name: "Victim"})

	e.sess.rc = deadRedis(t)
	if rec := f.callback(""); rec.Code != 302 || !strings.Contains(rec.Header().Get("Location"), "error=") {
		t.Fatalf("callback with Redis down: %d %q", rec.Code, rec.Header().Get("Location"))
	}
	if n := e.count(`SELECT COUNT(*) FROM users WHERE password_hash IS NOT NULL AND email_verified_at IS NULL`); n != 1 {
		t.Fatal("the account was taken over although the squatter's sessions could not be removed")
	}

	e.sess.rc = e.rc
	want(t, e.me(squatter), 200, "") // still the squatter's, the defence has not run yet
	wantRedirect(t, e.google(p, "", oidctest.Claims{Sub: "victim-sub", Email: "victim@example.com", Name: "Victim"}), "/")
	want(t, e.me(squatter), 401, "unauthenticated")
}

// Password reset deletes all sessions of the user, including ones from several browsers (3 sessions, none survives).
func TestResetPasswordDeletesAllThreeSessions(t *testing.T) {
	e := newEnv(t, opts{})
	cookies := []string{sessionCookie(t, e.register(testMail, testPW, "A")).Value}
	for i := 0; i < 2; i++ {
		cookies = append(cookies, sessionCookie(t, e.login(testMail, testPW, "")).Value)
	}
	other := sessionCookie(t, e.register("bob@example.com", testPW, "Bob")).Value
	want(t, e.forgot(testMail, ""), 202, "")
	code := e.code(testMail)
	want(t, e.reset(testMail, code, "Brand-new-fake-pw-77"), 204, "")
	for _, c := range cookies {
		want(t, e.me(c), 401, "unauthenticated")
	}
	want(t, e.me(other), 200, "")
}
