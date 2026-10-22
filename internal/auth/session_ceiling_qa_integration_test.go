//go:build integration

package auth

import (
	"context"
	"strconv"
	"testing"
	"time"
)

// QA T-101 AC3: the refresh near the ceiling is cut to exactly the ceiling (cookie Max-Age/Expires and Redis PEXPIRE), created_at never
// moves, the session is valid up to the ceiling and refused just after it.
func TestQASessionCeilingExactBoundary(t *testing.T) {
	e := newEnv(t, opts{})
	reg := e.register(testMail, testPW, "A")
	c := sessionCookie(t, reg).Value
	t0 := e.clock.now()
	day := 24 * time.Hour
	get := func() (code int, set string) {
		rec := e.do(req{method: "GET", path: "/api/user/get-me", cookie: c})
		return rec.Code, rec.Header().Get("Set-Cookie")
	}
	for d := 16; d <= 64; d += 16 { // refreshes on days 16, 32, 48, 64
		e.clock.advance(16 * day)
		if code, _ := get(); code != 200 {
			t.Fatalf("day %d: %d", d, code)
		}
	}
	e.clock.advance(16 * day) // day 80: the refresh is 10 days from the ceiling, not 30
	rec := e.do(req{method: "GET", path: "/api/user/get-me", cookie: c})
	want(t, rec, 200, "")
	ck := sessionCookie(t, rec)
	if ck.MaxAge != int((10*day).Seconds()) || ck.Expires.Sub(t0.Add(SessionMaxAge)).Abs() > time.Second {
		t.Fatalf("cookie Max-Age %d Expires %v, want 864000 and %v", ck.MaxAge, ck.Expires, t0.Add(SessionMaxAge))
	}
	t.Logf("Set-Cookie on the day-80 refresh: %s", rec.Header().Get("Set-Cookie"))
	ctx := context.Background()
	keys, _ := e.rc.Client().Keys(ctx, e.rc.Key("sess", "*")).Result()
	if len(keys) != 1 {
		t.Fatalf("keys %v", keys)
	}
	if ttl := e.rc.Client().PTTL(ctx, keys[0]).Val(); ttl > 10*day || ttl < 10*day-time.Minute {
		t.Fatalf("Redis TTL %v, want about 10 days", ttl)
	}
	if ca := e.rc.Client().HGet(ctx, keys[0], "created_at").Val(); ca != itoa(t0.UnixMilli()) {
		t.Fatalf("created_at moved: %s vs %d", ca, t0.UnixMilli())
	}
	e.clock.advance(10*day - time.Minute) // day 90 minus a minute
	if code, _ := get(); code != 200 {
		t.Fatalf("just before the ceiling: %d", code)
	}
	e.clock.advance(2 * time.Minute) // just past it
	if code, _ := get(); code != 401 {
		t.Fatalf("just after the ceiling: %d", code)
	}
}

// QA T-101 AC3: a session without a readable created_at is not found (401), one without an epoch (pre-T-103) still works.
func TestQASessionWithoutCreatedAtIsNotFound(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "A")).Value
	ctx := context.Background()
	keys, _ := e.rc.Client().Keys(ctx, e.rc.Key("sess", "*")).Result()
	get := func() int { return e.do(req{method: "GET", path: "/api/user/get-me", cookie: c}).Code }

	e.rc.Client().HDel(ctx, keys[0], "epoch")
	if got := get(); got != 200 {
		t.Fatalf("no epoch: %d, want 200", got)
	}
	e.rc.Client().HSet(ctx, keys[0], "created_at", "garbage")
	if got := get(); got != 401 {
		t.Fatalf("garbage created_at: %d, want 401", got)
	}
	e.rc.Client().HDel(ctx, keys[0], "created_at")
	if got := get(); got != 401 {
		t.Fatalf("no created_at: %d, want 401", got)
	}
	// and it cannot be revived by a login on another session
	want(t, e.login(testMail, testPW, ""), 200, "")
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
