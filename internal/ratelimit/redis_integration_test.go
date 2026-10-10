//go:build integration

package ratelimit_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis"
	"github.com/danyaa666/smemories/internal/redis/redistest"
)

func take(t testing.TB, l ratelimit.Limiter, key string) (bool, time.Duration) {
	t.Helper()
	ok, retry, err := l.Take(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return ok, retry
}

func keys(t testing.TB, c *redis.Client) []string {
	t.Helper()
	ks, err := c.Client().Keys(context.Background(), c.Key("*")).Result()
	if err != nil {
		t.Fatal(err)
	}
	return ks
}

// AC5: exactly N pass, then it refuses with an accurate retry-after, and the window slides.
func TestRedisLimitRetryAfterAndWindow(t *testing.T) {
	c := redistest.New(t)
	l := ratelimit.NewRedis(c, "t", 3, time.Second, nil)
	for i := range 3 {
		if ok, _ := take(t, l, "k"); !ok {
			t.Fatalf("hit %d refused", i)
		}
		time.Sleep(50 * time.Millisecond)
	}
	ok, retry := take(t, l, "k")
	// the oldest hit is about 150 ms old in a 1 s window
	if ok || retry < 700*time.Millisecond || retry > 900*time.Millisecond {
		t.Fatalf("ok=%v retry=%v, want refused with about 850ms", ok, retry)
	}
	if ok, _ := take(t, l, "other"); !ok {
		t.Fatal("keys are independent")
	}
	time.Sleep(retry + 20*time.Millisecond)
	if ok, _ := take(t, l, "k"); !ok {
		t.Fatal("the oldest hit left the window; one slot must be free")
	}
}

func TestRedisRefundRestoresOneSlot(t *testing.T) {
	c := redistest.New(t)
	l := ratelimit.NewRedis(c, "t", 2, time.Minute, nil)
	take(t, l, "k")
	take(t, l, "k")
	if err := l.Refund(context.Background(), "k"); err != nil {
		t.Fatal(err)
	}
	if ok, _ := take(t, l, "k"); !ok {
		t.Fatal("refund should free a slot")
	}
	if ok, _ := take(t, l, "k"); ok {
		t.Fatal("limit is 2")
	}
	if err := l.Refund(context.Background(), "never-seen"); err != nil {
		t.Fatal(err)
	}
	if n := len(keys(t, c)); n != 1 {
		t.Fatalf("%d keys, want 1 (a refund of an unknown key creates nothing)", n)
	}
}

func TestRedisParallelTakeAdmitsExactlyN(t *testing.T) {
	c := redistest.New(t)
	l := ratelimit.NewRedis(c, "t", 7, time.Minute, nil)
	var wg sync.WaitGroup
	var allowed atomic.Int32
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _, err := l.Take(context.Background(), "k"); err == nil && ok {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 7 {
		t.Fatalf("allowed %d, want exactly 7", allowed.Load())
	}
}

// Two limiter instances (two API tasks) share one count.
func TestRedisInstancesShareTheCount(t *testing.T) {
	c := redistest.New(t)
	a := ratelimit.NewRedis(c, "t", 4, time.Minute, nil)
	b := ratelimit.NewRedis(c, "t", 4, time.Minute, nil)
	for i := range 4 {
		l := a
		if i%2 == 1 {
			l = b
		}
		if ok, _ := take(t, l, "k"); !ok {
			t.Fatalf("hit %d refused", i)
		}
	}
	if ok, _ := take(t, a, "k"); ok {
		t.Fatal("a must see b's hits")
	}
	if ok, _ := take(t, b, "k"); ok {
		t.Fatal("b must see a's hits")
	}
}

// Two hits in the same millisecond both count, and the keys vanish with the window (memory returns to baseline).
func TestRedisSameInstantAndExpiry(t *testing.T) {
	c := redistest.New(t)
	fixed := time.Now()
	l := ratelimit.NewRedis(c, "t", 3, 300*time.Millisecond, func() time.Time { return fixed })
	for range 3 {
		if ok, _ := take(t, l, "k"); !ok {
			t.Fatal("refused under the limit")
		}
	}
	if ok, _ := take(t, l, "k"); ok {
		t.Fatal("three hits with an identical timestamp must count three times")
	}
	time.Sleep(400 * time.Millisecond)
	if ks := keys(t, c); len(ks) != 0 {
		t.Fatalf("keys left after the window: %v", ks)
	}
}

// The injected clock moves the window without sleeping (used by endpoint tests).
func TestRedisFakeClock(t *testing.T) {
	c := redistest.New(t)
	now := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	l := ratelimit.NewRedis(c, "t", 1, time.Hour, func() time.Time { return now })
	take(t, l, "k")
	ok, retry := take(t, l, "k")
	if ok || retry != time.Hour {
		t.Fatalf("ok=%v retry=%v", ok, retry)
	}
	now = now.Add(time.Hour)
	if ok, _ := take(t, l, "k"); !ok {
		t.Fatal("window over")
	}
}

// The key is smem:<env>:rl:<name>:<key> and carries the window as its expiry.
func TestRedisKeyShapeAndTTL(t *testing.T) {
	c := redistest.New(t)
	l := ratelimit.NewRedis(c, "name", 5, time.Hour, nil)
	take(t, l, "203.0.113.9")
	want := c.Key("rl", "name", "203.0.113.9")
	ttl, err := c.Client().PTTL(context.Background(), want).Result()
	if err != nil || ttl <= 0 || ttl > time.Hour {
		t.Fatalf("key %s ttl=%v err=%v", want, ttl, err)
	}
}

func BenchmarkRedisTake(b *testing.B) {
	c := redistest.New(b)
	l := ratelimit.NewRedis(c, "bench", b.N+1, time.Hour, nil)
	b.ResetTimer()
	for range b.N {
		if _, _, err := l.Take(context.Background(), "k"); err != nil {
			b.Fatal(err)
		}
	}
}
