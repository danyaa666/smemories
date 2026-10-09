package ratelimit_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis"
)

type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) { s.mu.Lock(); defer s.mu.Unlock(); return s.b.Write(p) }
func (s *syncBuf) String() string              { s.mu.Lock(); defer s.mu.Unlock(); return s.b.String() }

// deadRedis is a client for a port nobody listens on, so no server is needed.
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

// AC3: with Redis down an Open limiter lets the request through and logs one ERROR per interval;
// a Closed limiter refuses with ErrUnavailable.
func TestFailPolicyWhenRedisIsDown(t *testing.T) {
	var logs syncBuf
	f := ratelimit.NewFactory(deadRedis(t), slog.New(slog.NewJSONHandler(&logs, nil)), nil)
	ctx := context.Background()

	open := f.Open("demo_open", 1, time.Minute)
	for range 5 { // limit 1, but nothing can be counted: every request passes
		ok, _, err := open.Take(ctx, "k")
		if !ok || err != nil {
			t.Fatalf("open limiter: ok=%v err=%v, want allowed", ok, err)
		}
	}
	if err := open.Refund(ctx, "k"); err != nil {
		t.Fatalf("refund must not fail: %v", err)
	}

	closed := f.Closed("demo_closed", 1, time.Minute)
	ok, _, err := closed.Take(ctx, "k")
	if ok || !errors.Is(err, ratelimit.ErrUnavailable) || !errors.Is(err, redis.ErrUnavailable) {
		t.Fatalf("closed limiter: ok=%v err=%v, want ErrUnavailable with the redis cause", ok, err)
	}

	if n := strings.Count(logs.String(), `"level":"ERROR"`); n != 1 {
		t.Fatalf("%d ERROR lines for 8 failures, want exactly 1 per interval:\n%s", n, logs.String())
	}
	if !strings.Contains(logs.String(), `"limiter":"demo_open"`) || !strings.Contains(logs.String(), `"failures":1`) {
		t.Fatalf("log lacks the limiter name and failure counter: %s", logs.String())
	}
}

func TestCancelledContextIsNotAFailure(t *testing.T) {
	f := ratelimit.NewFactory(deadRedis(t), nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := f.Closed("x", 1, time.Minute).Take(ctx, "k"); errors.Is(err, ratelimit.ErrUnavailable) || !errors.Is(err, context.Canceled) {
		t.Fatalf("a client that left is not an outage: %v", err)
	}
	if ok, _, err := f.Open("x", 1, time.Minute).Take(ctx, "k"); !ok || err != nil {
		t.Fatalf("open limiter: ok=%v err=%v", ok, err)
	}
}
