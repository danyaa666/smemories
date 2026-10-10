//go:build integration

package redis_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/redis"
	"github.com/danyaa666/smemories/internal/redis/redistest"
)

func TestPingAndLatency(t *testing.T) {
	c := redistest.New(t)
	ctx := context.Background()
	_ = c.Ping(ctx) // warm the connection
	start := time.Now()
	if err := c.Ping(ctx); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*time.Millisecond {
		t.Errorf("ping took %v, want under 5 ms locally", d)
	}
}

func TestLoadScriptAndRun(t *testing.T) {
	c := redistest.New(t)
	ctx := context.Background()
	s, err := c.LoadScript(ctx, `redis.call('SET', KEYS[1], ARGV[1]); return redis.call('GET', KEYS[1])`)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.EvalSha(ctx, c.Client(), []string{c.Key("s", "1")}, "v").Text()
	if err != nil || got != "v" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := c.LoadScript(ctx, `this is not lua`); err == nil || errors.Is(err, redis.ErrUnavailable) {
		t.Fatalf("a script error is a reply error, not unavailability: %v", err)
	}
}

func TestHelperIsolatesKeysAndCleansUp(t *testing.T) {
	a, b := redistest.New(t), redistest.New(t)
	ctx := context.Background()
	if a.Key("x") == b.Key("x") {
		t.Fatal("two helpers share a prefix")
	}
	if err := a.Client().Set(ctx, a.Key("x"), "1", 0).Err(); err != nil {
		t.Fatal(err)
	}
	if keys, err := b.Client().Keys(ctx, b.Key("*")).Result(); err != nil || len(keys) != 0 {
		t.Fatalf("b sees keys under its own prefix: %v %v", keys, err)
	}

	var inner string
	t.Run("writes", func(t *testing.T) {
		c := redistest.New(t)
		inner = c.Key("leftover")
		if err := c.Client().Set(ctx, inner, "1", 0).Err(); err != nil {
			t.Fatal(err)
		}
	})
	if n, err := a.Client().Exists(ctx, inner).Result(); err != nil || n != 0 {
		t.Fatalf("key %s survived the test cleanup: n=%d err=%v", inner, n, err)
	}
}

// Ready also writes, so a Redis that refuses writes (full, noeviction) is not ready.
func TestReadyWritesAProbeKey(t *testing.T) {
	c := redistest.New(t)
	ctx := context.Background()
	if err := c.Ready(ctx); err != nil {
		t.Fatal(err)
	}
	if ttl := c.Client().TTL(ctx, c.Key("probe", "ready")).Val(); ttl <= 0 || ttl > 10*time.Second {
		t.Fatalf("probe key TTL %v", ttl)
	}
}
