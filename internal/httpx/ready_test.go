package httpx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func nopLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

type fakePinger func(ctx context.Context) error

func (f fakePinger) PingContext(ctx context.Context) error { return f(ctx) }

func TestReadyOK(t *testing.T) {
	h, _ := newTestRouter(t, Ready(fakePinger(func(context.Context) error { return nil }), nopLogger()))
	rec := do(h, "GET", "/readyz", nil)
	if rec.Code != 200 || strings.TrimSpace(rec.Body.String()) != `{"status":"ready"}` {
		t.Fatalf("got %d %q", rec.Code, rec.Body.String())
	}
}

func TestReadyNotReadyHidesDriverError(t *testing.T) {
	var logs bytes.Buffer
	h, _ := newTestRouter(t, Ready(fakePinger(func(context.Context) error {
		return errors.New("dial tcp 10.0.0.5:3306: secret-host refused")
	}), slog.New(slog.NewJSONHandler(&logs, nil))))
	rec := do(h, "GET", "/readyz", nil)
	e := decodeV2(t, rec)
	if rec.Code != http.StatusServiceUnavailable || e.Status != "ERROR_NOT_READY" || e.RequestID == "" {
		t.Fatalf("got %d %+v", rec.Code, e)
	}
	if strings.Contains(rec.Body.String(), "secret-host") || strings.Contains(rec.Body.String(), "dial") {
		t.Fatalf("driver error leaked into the body: %s", rec.Body.String())
	}
	if !strings.Contains(logs.String(), "secret-host") {
		t.Fatalf("driver error should be logged, got %q", logs.String())
	}
}

func TestReadyTimesOut(t *testing.T) {
	h, _ := newTestRouter(t, Ready(fakePinger(func(ctx context.Context) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Second):
			return nil
		}
	}), nopLogger()))
	start := time.Now()
	rec := do(h, "GET", "/readyz", nil)
	if rec.Code != 503 || time.Since(start) > 3*time.Second {
		t.Fatalf("got %d after %v, want 503 within ~1s", rec.Code, time.Since(start))
	}
}

func TestReadyWrongMethod(t *testing.T) {
	h, _ := newTestRouter(t, Ready(fakePinger(func(context.Context) error { return nil }), nopLogger()))
	if rec := do(h, "POST", "/readyz", nil); rec.Code != 405 {
		t.Fatalf("got %d, want 405", rec.Code)
	}
}

func TestReadyFailsWhenAnyDepIsDown(t *testing.T) {
	up := fakePinger(func(context.Context) error { return nil })
	for name, tc := range map[string]struct {
		db   fakePinger
		dep  func(context.Context) error
		want int
	}{
		"both up":    {up, func(context.Context) error { return nil }, 200},
		"redis down": {up, func(context.Context) error { return errors.New("redis refused") }, 503},
		"db down":    {fakePinger(func(context.Context) error { return errors.New("db refused") }), func(context.Context) error { return nil }, 503},
	} {
		var logs bytes.Buffer
		h, _ := newTestRouter(t, Ready(tc.db, slog.New(slog.NewJSONHandler(&logs, nil)), Dep{"redis", tc.dep}))
		rec := do(h, "GET", "/readyz", nil)
		if rec.Code != tc.want || strings.Contains(rec.Body.String(), "refused") {
			t.Errorf("%s: got %d %q, want %d without the error text", name, rec.Code, rec.Body.String(), tc.want)
		}
		if name == "redis down" && !strings.Contains(logs.String(), "readyz: redis ping failed") {
			t.Errorf("log should name the failing dependency, got %q", logs.String())
		}
	}
}

func TestReadyDrainWarnIsLoggedOnce(t *testing.T) {
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	h, _ := newTestRouter(t, Ready(fakePinger(func(context.Context) error { return nil }), slog.New(slog.NewJSONHandler(&logs, nil)), Draining(ctx)))
	cancel()
	for i := 0; i < 3; i++ {
		if rec := do(h, "GET", "/readyz", nil); rec.Code != 503 {
			t.Fatalf("probe %d: got %d, want 503 while draining", i, rec.Code)
		}
	}
	if n := strings.Count(logs.String(), "readyz: shutdown ping failed"); n != 1 {
		t.Fatalf("drain WARN logged %d times, want 1: %q", n, logs.String())
	}
}

// fakeClock is a hand-moved clock for the cache window.
type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func TestReadyCachesTheCheckForTwoSeconds(t *testing.T) {
	clk := &fakeClock{t: time.Unix(1000, 0)}
	var pings, redisPings atomic.Int32
	var failing atomic.Bool
	db := fakePinger(func(context.Context) error {
		pings.Add(1)
		if failing.Load() {
			return errors.New("db refused")
		}
		return nil
	})
	redis := Dep{"redis", func(context.Context) error { redisPings.Add(1); return nil }}
	h, _ := newTestRouter(t, ready(clk.now, db, nopLogger(), redis))

	for i := 0; i < 100; i++ {
		clk.t = clk.t.Add(9 * time.Millisecond) // 100 probes inside 1 s
		if rec := do(h, "GET", "/readyz", nil); rec.Code != 200 {
			t.Fatalf("probe %d: got %d", i, rec.Code)
		}
	}
	if pings.Load() != 1 || redisPings.Load() != 1 {
		t.Fatalf("100 probes pinged db %d and redis %d times, want 1 each", pings.Load(), redisPings.Load())
	}

	failing.Store(true)
	clk.t = clk.t.Add(time.Second) // still inside the window: the outage is not seen yet
	if rec := do(h, "GET", "/readyz", nil); rec.Code != 200 || pings.Load() != 1 {
		t.Fatalf("inside the window: got %d after %d pings", rec.Code, pings.Load())
	}
	clk.t = clk.t.Add(ReadyCacheTTL) // window over: a new check runs and fails
	for i := 0; i < 5; i++ {
		if rec := do(h, "GET", "/readyz", nil); rec.Code != 503 {
			t.Fatalf("failing probe %d: got %d, want 503", i, rec.Code)
		}
	}
	if pings.Load() != 2 {
		t.Fatalf("failure checked %d times in total, want 2 (cached as failing)", pings.Load())
	}

	failing.Store(false)
	clk.t = clk.t.Add(ReadyCacheTTL) // a failure is cached for 2 s only
	if rec := do(h, "GET", "/readyz", nil); rec.Code != 200 || pings.Load() != 3 {
		t.Fatalf("after the failure window: got %d after %d pings, want 200 after 3", rec.Code, pings.Load())
	}
}

func TestReadyConcurrentProbesShareOneCheck(t *testing.T) {
	var pings atomic.Int32
	db := fakePinger(func(context.Context) error { pings.Add(1); time.Sleep(50 * time.Millisecond); return nil })
	h, _ := newTestRouter(t, Ready(db, nopLogger()))
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); do(h, "GET", "/readyz", nil) }()
	}
	wg.Wait()
	if pings.Load() != 1 {
		t.Fatalf("50 concurrent probes pinged the database %d times, want 1", pings.Load())
	}
}

func TestReadyDrainIsNotHiddenByTheCache(t *testing.T) {
	var logs bytes.Buffer
	ctx, cancel := context.WithCancel(context.Background())
	var pings atomic.Int32
	db := fakePinger(func(context.Context) error { pings.Add(1); return nil })
	h, _ := newTestRouter(t, Ready(db, slog.New(slog.NewJSONHandler(&logs, nil)), Draining(ctx)))
	if rec := do(h, "GET", "/readyz", nil); rec.Code != 200 {
		t.Fatalf("before the drain: got %d", rec.Code)
	}
	cancel() // the result is cached as ready, the drain must still show at once
	if rec := do(h, "GET", "/readyz", nil); rec.Code != 503 {
		t.Fatalf("right after the drain started: got %d, want 503", rec.Code)
	}
	if pings.Load() != 1 {
		t.Fatalf("drain probe re-pinged the database (%d pings)", pings.Load())
	}
}

func TestReadyFailureIsLoggedOncePerCheck(t *testing.T) {
	var logs bytes.Buffer
	db := fakePinger(func(context.Context) error { return errors.New("db refused") })
	h, _ := newTestRouter(t, Ready(db, slog.New(slog.NewJSONHandler(&logs, nil))))
	for i := 0; i < 20; i++ {
		do(h, "GET", "/readyz", nil)
	}
	if n := strings.Count(logs.String(), "readyz: database ping failed"); n != 1 {
		t.Fatalf("failure logged %d times for one check, want 1", n)
	}
}
