package httpx

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
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
	e := decodeError(t, rec)
	if rec.Code != http.StatusServiceUnavailable || e.Code != "not_ready" || e.RequestID == "" {
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
