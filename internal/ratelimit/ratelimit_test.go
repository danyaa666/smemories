package ratelimit

import (
	"sync"
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestTakeBlocksAtLimitAndReportsRetryAfter(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	l := New(3, time.Minute, c.now)
	for i := 0; i < 3; i++ {
		if ok, _ := l.Take("k"); !ok {
			t.Fatalf("hit %d should be allowed", i)
		}
		c.t = c.t.Add(10 * time.Second)
	}
	ok, retry := l.Take("k")
	if ok || retry != 30*time.Second { // oldest hit was 30 s ago in a 60 s window
		t.Fatalf("ok=%v retry=%v, want blocked with 30s", ok, retry)
	}
	if ok, _ := l.Take("other"); !ok {
		t.Fatal("keys are independent")
	}
	c.t = c.t.Add(30 * time.Second) // oldest hit leaves the window
	if ok, _ := l.Take("k"); !ok {
		t.Fatal("should be allowed again after the window")
	}
}

func TestRefundUndoesLastHit(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	l := New(2, time.Minute, c.now)
	l.Take("k")
	l.Take("k")
	l.Refund("k")
	if ok, _ := l.Take("k"); !ok {
		t.Fatal("refund should free a slot")
	}
	if ok, _ := l.Take("k"); ok {
		t.Fatal("limit is 2")
	}
	l.Refund("never-seen") // must not panic
}

func TestSweepForgetsExpiredKeys(t *testing.T) {
	c := &clock{time.Unix(1000, 0)}
	l := New(5, time.Minute, c.now)
	for _, k := range []string{"a", "b", "c"} {
		l.Take(k)
	}
	c.t = c.t.Add(2 * time.Minute)
	l.Take("d")
	if len(l.hits) != 1 {
		t.Fatalf("expired keys should be swept, have %d", len(l.hits))
	}
}

func TestTakeIsAtomicUnderConcurrency(t *testing.T) {
	l := New(10, time.Hour, nil)
	var wg sync.WaitGroup
	var mu sync.Mutex
	allowed := 0
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Take("k"); ok {
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
