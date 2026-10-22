package httpx

import (
	"context"
	"errors"
	"net/http/httptest"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// QA (T-101): a dependency that honours the 1 s deadline but is slow. 50 concurrent probes share one check, all answer 503
// within about ReadyTimeout, and no goroutine is left behind.
func TestQAReadySlowDependencyNoGoroutineLeak(t *testing.T) {
	before := runtime.NumGoroutine()
	var pings atomic.Int32
	db := fakePinger(func(ctx context.Context) error { pings.Add(1); <-ctx.Done(); return ctx.Err() })
	h, _ := newTestRouter(t, Ready(db, nopLogger()))
	var wg sync.WaitGroup
	start := time.Now()
	codes := make(chan int, 50)
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); codes <- do(h, "GET", "/readyz", nil).Code }()
	}
	wg.Wait()
	close(codes)
	for c := range codes {
		if c != 503 {
			t.Fatalf("got %d, want 503", c)
		}
	}
	if d := time.Since(start); d > 2500*time.Millisecond {
		t.Fatalf("50 probes took %v, want about 1 s", d)
	}
	if pings.Load() != 1 {
		t.Fatalf("pings = %d, want 1", pings.Load())
	}
	time.Sleep(200 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+2 {
		t.Fatalf("goroutines %d -> %d", before, after)
	}
}

// QA (T-101): a client that hangs up during the check must not turn the cached result into "not ready" for others.
func TestQAReadyClientCancelIsNotCached(t *testing.T) {
	db := fakePinger(func(ctx context.Context) error {
		select {
		case <-time.After(100 * time.Millisecond):
			return nil
		case <-ctx.Done():
			return errors.New("cancelled: " + ctx.Err().Error())
		}
	})
	h, _ := newTestRouter(t, Ready(db, nopLogger()))
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest("GET", "/readyz", nil).WithContext(ctx)
	go func() { time.Sleep(20 * time.Millisecond); cancel() }()
	h.ServeHTTP(httptest.NewRecorder(), req)
	if rec := do(h, "GET", "/readyz", nil); rec.Code != 200 {
		t.Fatalf("next probe got %d, want 200 (cancellation must not be cached)", rec.Code)
	}
}
