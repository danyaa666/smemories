//go:build integration

package auth

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis"
)

// T-053 AC4: Redis keys hold hashes, numeric or public ids and IPs, never an email address.
func TestLimiterKeysHoldNoEmail(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register("Alice.Secret@Example.com", testPW, "Alice"), 200, "")
	want(t, e.login("alice.secret@example.com", "wrong-password-1", ""), 401, "ERROR_INVALID_CREDENTIALS")
	want(t, e.forgot("Alice.Secret@Example.com", ""), 200, "")
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
	want(t, rec, 503, "ERROR_LIMITER_UNAVAILABLE")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
	if e.verified(c.Value) {
		t.Fatal("verified while the guess limiter was down")
	}

	want(t, e.register("bob@example.com", testPW, "Bob"), 200, "") // open
	want(t, e.forgot(testMail, ""), 200, "")                       // open
	e.svc.Wait()
	want(t, e.post("/api/auth/resend-verification", "", c.Value), 200, "") // open
}

// v6 returns a RemoteAddr in the /64 numbered n, host number h.
func v6(n, h int) string { return fmt.Sprintf("[2001:db8:%x:%x::%x]:1", n>>16, n&0xffff, h) }

// T-098 AC2: the account limiter counts failures from any address; one IPv6 /64 is one address.
func TestLoginAccountLimitAcrossAddresses(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 100, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 50}})
	want(t, e.register(testMail, testPW, "A"), 200, "")
	want(t, e.register("bob@example.com", testPW, "B"), 200, "")
	for i := 1; i <= 50; i++ {
		want(t, e.login(testMail, "wrong-password-x", v6(i, 1)), 401, "ERROR_INVALID_CREDENTIALS")
	}
	blocked := e.login(testMail, "wrong-password-x", v6(51, 1))
	want(t, blocked, 429, "ERROR_RATE_LIMITED")
	if blocked.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
	want(t, e.login(testMail, testPW, v6(52, 1)), 429, "ERROR_RATE_LIMITED") // the right password too
	want(t, e.login("bob@example.com", testPW, v6(51, 1)), 200, "")          // another account is not affected
	// the blocked attempt was refunded from the other limiters: they hold exactly the 50 + bob's success refunded
	e.clock.advance(15*time.Minute + time.Second)
	want(t, e.login(testMail, testPW, v6(53, 1)), 200, "")
}

func TestSuccessfulLoginsDoNotUseUpTheAccountBudget(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 100, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 50}})
	want(t, e.register(testMail, testPW, "A"), 200, "")
	for i := 1; i <= 49; i++ {
		want(t, e.login(testMail, "wrong-password-x", v6(i, 1)), 401, "ERROR_INVALID_CREDENTIALS")
	}
	for i := 0; i < 5; i++ {
		want(t, e.login(testMail, testPW, v6(60, 1)), 200, "")
	}
	want(t, e.login(testMail, "wrong-password-x", v6(61, 1)), 401, "ERROR_INVALID_CREDENTIALS") // the 50th failure
	want(t, e.login(testMail, "wrong-password-x", v6(62, 1)), 429, "ERROR_RATE_LIMITED")
}

func TestParallelGuessesCannotOutrunTheAccountLimit(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 100, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 5}})
	want(t, e.register(testMail, testPW, "A"), 200, "")
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[int]int{}
	for i := 1; i <= 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := e.login(testMail, "wrong-guess-xxx", v6(i, 1)).Code
			mu.Lock()
			codes[c]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes[401] != 5 || codes[429] != 25 {
		t.Fatalf("got %v, want exactly 5 judged guesses", codes)
	}
}

// T-098 AC1/AC2: two hosts of one /64 share the per-IP budgets; another /64 does not.
func TestLoginPairBudgetIsSharedInsideAnIPv6Prefix(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 100, LoginFailsPerPair: 2, LoginFailsPerIP: 1000, LoginFailsPerAccount: 1000}})
	want(t, e.register(testMail, testPW, "A"), 200, "")
	want(t, e.login(testMail, "wrong-password-x", v6(7, 1)), 401, "ERROR_INVALID_CREDENTIALS")
	want(t, e.login(testMail, "wrong-password-x", v6(7, 0xabcd)), 401, "ERROR_INVALID_CREDENTIALS")
	want(t, e.login(testMail, "wrong-password-x", v6(7, 0x9999)), 429, "ERROR_RATE_LIMITED")
	want(t, e.login(testMail, testPW, v6(8, 1)), 200, "")
}

// T-098 AC6: a successful password reset gives the account its login budget back; other accounts keep theirs.
func TestPasswordResetClearsTheAccountLoginCounter(t *testing.T) {
	e := newEnv(t, opts{limits: Limits{RegisterPerHour: 100, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 3}})
	const bob, newPW = "bob@example.com", "Another-Long-Passw0rd!"
	want(t, e.register(testMail, testPW, "A"), 200, "")
	want(t, e.register(bob, testPW, "B"), 200, "")
	for i := 1; i <= 3; i++ {
		want(t, e.login(testMail, "wrong-password-x", v6(i, 1)), 401, "ERROR_INVALID_CREDENTIALS")
	}
	for i := 1; i <= 2; i++ {
		want(t, e.login(bob, "wrong-password-x", v6(i, 1)), 401, "ERROR_INVALID_CREDENTIALS")
	}
	want(t, e.login(testMail, newPW, v6(9, 1)), 429, "ERROR_RATE_LIMITED")

	want(t, e.forgot(testMail, v6(9, 1)), 200, "")
	want(t, e.reset(" ALICE@example.com", e.code(testMail), newPW), 200, "") // another spelling of the same address
	want(t, e.login(testMail, newPW, v6(9, 1)), 200, "")

	want(t, e.login(bob, "wrong-password-x", v6(10, 1)), 401, "ERROR_INVALID_CREDENTIALS") // bob's 3rd
	want(t, e.login(bob, testPW, v6(11, 1)), 429, "ERROR_RATE_LIMITED")                    // bob is still at his limit
}

// T-098 AC3: the account limiter fails closed on its own (the other two answer from the live Redis).
func TestLoginAccountLimiterFailsClosed(t *testing.T) {
	e := newEnv(t, opts{})
	want(t, e.register(testMail, testPW, "A"), 200, "")
	dead, err := redis.New(config.Config{Env: "test", RedisURL: "redis://127.0.0.1:1/0",
		RedisDialTimeout: 200 * time.Millisecond, RedisReadTimeout: 200 * time.Millisecond, RedisWriteTimeout: 200 * time.Millisecond, RedisPoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dead.Close() }()
	e.svc.loginAcct = ratelimit.NewFactory(dead, slog.New(slog.DiscardHandler), nil).Closed("login_account", 50, time.Hour)

	rec := e.login(testMail, testPW, "")
	want(t, rec, 503, "ERROR_LIMITER_UNAVAILABLE")
	if rec.Header().Get("Retry-After") == "" {
		t.Error("Retry-After missing")
	}
	e.svc.loginAcct = ratelimit.NewFactory(e.rc, slog.New(slog.DiscardHandler), nil).Closed("login_account", 50, time.Hour)
	want(t, e.login(testMail, testPW, ""), 200, "") // the refusal refunded the pair and ip hits: nothing was used up
}

// An oversize address shares one bounded account key instead of making a key per attempt.
func TestLoginAccountKeyStaysBounded(t *testing.T) {
	e := newEnv(t, opts{})
	long := strings.Repeat("a", 5000) + "@example.com"
	e.login(long, "wrong-password-x", "")
	keys, err := e.rc.Client().Keys(context.Background(), e.rc.Key("rl", "login_account", "*")).Result()
	if err != nil || len(keys) != 1 || len(keys[0]) > 200 {
		t.Fatalf("keys = %v, err = %v", keys, err)
	}
}
