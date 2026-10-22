package httpx

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// slowWriter takes a while per Write, like a client that drains the socket slowly.
type slowWriter struct {
	http.ResponseWriter
	per time.Duration
}

func (s slowWriter) Write(p []byte) (int, error) { time.Sleep(s.per); return len(p), nil }

// T-107: a response written under the minimum rate is cut after the grace period; one that keeps up is not; the wrapper stays unwrappable.
func TestPaceWriter(t *testing.T) {
	pace := BodyPace{Idle: time.Second, Total: 10 * time.Second, MinRate: 10_000, Grace: 100 * time.Millisecond}

	slow := PaceWriter(slowWriter{httptest.NewRecorder(), 40 * time.Millisecond}, pace) // 100 bytes per 40 ms = 2.5 KB/s
	var err error
	for range 20 {
		if _, err = slow.Write(make([]byte, 100)); err != nil {
			break
		}
	}
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("slow reader: %v, want a deadline error", err)
	}

	rec := httptest.NewRecorder()
	fast := PaceWriter(rec, pace)
	for range 20 {
		if _, err := fast.Write(make([]byte, 1000)); err != nil {
			t.Fatalf("fast reader: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if rec.Body.Len() != 20_000 {
		t.Fatalf("wrote %d bytes", rec.Body.Len())
	}
	if u, ok := fast.(interface{ Unwrap() http.ResponseWriter }); !ok || u.Unwrap() != rec {
		t.Fatal("PaceWriter hides the ResponseWriter from http.ResponseController")
	}
}
