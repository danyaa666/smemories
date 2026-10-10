package ratelimit

import (
	"context"
	"sync"
	"testing"
	"time"
)

func take(l Limiter, key string) (bool, time.Duration) {
	ok, retry, _ := l.Take(context.Background(), key)
	return ok, retry
}

func refund(l Limiter, key string) { _ = l.Refund(context.Background(), key) }

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestTakeBlocksAtLimitAndReportsRetryAfter(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	l := NewMemory(3, time.Minute, c.now)
	for i := 0; i < 3; i++ {
		if ok, _ := take(l, "k"); !ok {
			t.Fatalf("hit %d should be allowed", i)
		}
		c.t = c.t.Add(10 * time.Second)
	}
	ok, retry := take(l, "k")
	if ok || retry != 30*time.Second { // oldest hit was 30 s ago in a 60 s window
		t.Fatalf("ok=%v retry=%v, want blocked with 30s", ok, retry)
	}
	if ok, _ := take(l, "other"); !ok {
		t.Fatal("keys are independent")
	}
	c.t = c.t.Add(30 * time.Second) // oldest hit leaves the window
	if ok, _ := take(l, "k"); !ok {
		t.Fatal("should be allowed again after the window")
	}
}

func TestRefundUndoesLastHit(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	l := NewMemory(2, time.Minute, c.now)
	take(l, "k")
	take(l, "k")
	refund(l, "k")
	if ok, _ := take(l, "k"); !ok {
		t.Fatal("refund should free a slot")
	}
	if ok, _ := take(l, "k"); ok {
		t.Fatal("limit is 2")
	}
	refund(l, "never-seen") // must not panic
}

func TestSweepForgetsExpiredKeys(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	l := NewMemory(5, time.Minute, c.now)
	for _, k := range []string{"a", "b", "c"} {
		take(l, k)
	}
	c.t = c.t.Add(2 * time.Minute)
	take(l, "d")
	if len(l.hits) != 1 {
		t.Fatalf("expired keys should be swept, have %d", len(l.hits))
	}
}

func TestTakeIsAtomicUnderConcurrency(t *testing.T) {
	l := NewMemory(10, time.Hour, nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := take(l, "k"); ok {
				mu.Lock()
				allowed++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	if allowed != 10 {
		t.Fatalf("allowed %d, want exactly 10", allowed)
	}
}
