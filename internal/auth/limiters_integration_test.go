//go:build integration

package auth

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis"
)

// T-053 AC4: Redis keys hold hashes, numeric or public ids and IPs, never an email address.
func TestLimiterKeysHoldNoEmail(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register("Alice.Secret@Example.com", testPW, "Alice"), 201, "")
	want(t, e.login("alice.secret@example.com", "wrong-password-1", ""), 401, "invalid_credentials")
	want(t, e.forgot("Alice.Secret@Example.com", ""), 202, "")
	e.svc.Wait()

	rl, err := e.rc.Client().Keys(context.Background(), e.rc.Key("rl", "*")).Result()
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, k := range rl {
		low := strings.ToLower(k)
		if strings.Contains(low, "@") || strings.Contains(low, "alice") || strings.Contains(low, "example") {
			t.Errorf("limiter key leaks the address: %s", k)
		}
		names[strings.Split(strings.TrimPrefix(k, e.rc.Key("rl")+":"), ":")[0]] = true
	}
	for _, n := range []string{"register", "login_pair", "login_ip", "forgot_ip", "forgot_email"} {
		if !names[n] {
			t.Errorf("no key for limiter %s; keys: %v", n, rl)
		}
	}
	// the email key is the hash of the normalised address, so another casing lands on the same key
	if n, _ := e.rc.Client().Exists(context.Background(), e.rc.Key("rl", "forgot_email", hashKey("alice.secret@example.com"))).Result(); n != 1 {
		t.Errorf("forgot_email key is not the hash of the normalised address; keys: %v", rl)
	}
}

// T-053 AC3 (D-23): with the limiter store down, the lockouts and code-guess limits answer 503
// limiter_unavailable, while the cost-control limiters let the request through.
func TestLimitersWhenRedisIsDown(t *testing.T) {
	e := newEnv(t, opts{})
	c := sessionCookie(t, e.register(testMail, testPW, "Alice"))
	code := e.code(testMail)
	dead, err := redis.New(config.Config{Env: "test", RedisURL: "redis://127.0.0.1:1/0",
		RedisDialTimeout: 200 * time.Millisecond, RedisReadTimeout: 200 * time.Millisecond, RedisWriteTimeout: 200 * time.Millisecond, RedisPoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dead.Close() }()
	f := ratelimit.NewFactory(dead, slog.New(slog.DiscardHandler), nil)
	s := e.svc
	s.register, s.loginPair, s.loginIP = f.Open("register", 5, time.Hour), f.Closed("login_pair", 5, time.Hour), f.Closed("login_ip", 5, time.Hour)
	s.resend, s.verifyTries, s.resetIP = f.Open("verify_resend", 3, time.Hour), f.Closed("verify_tries", 20, time.Hour), f.Closed("reset_tries", 20, time.Hour)
	s.forgotIP, s.forgotEmail = f.Open("forgot_ip", 5, time.Hour), f.Open("forgot_email", 3, time.Hour)

	for name, rec := range map[string]int{
		"login":  e.login(testMail, testPW, "").Code,
		"verify": e.verify(code, c.Value).Code,
		"reset":  e.reset(testMail, code, "Another-Long-Passw0rd!").Code,
	} {
		if rec != 503 {
			t.Errorf("%s: %d, want 503 (fail closed)", name, rec)
		}
	}
	rec := e.login(testMail, testPW, "")
	want(t, rec, 503, "limiter_unavailable")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
	if e.verified(c.Value) {
		t.Fatal("verified while the guess limiter was down")
	}

	want(t, e.register("bob@example.com", testPW, "Bob"), 201, "") // open
	want(t, e.forgot(testMail, ""), 202, "")                       // open
	e.svc.Wait()
	want(t, e.post("/v1/auth/verify-email/resend", "", c.Value), 202, "") // open
}
