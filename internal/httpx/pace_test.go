package httpx

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// A body that stalls, or that drips bytes slower than MinRate after Grace, is cut off with a read deadline error
// (408 through WriteBodyError); a body that keeps up is read to the end.
func TestPaceBody(t *testing.T) {
	pace := BodyPace{Idle: 300 * time.Millisecond, Total: 10 * time.Second, MinRate: 1000, Grace: 400 * time.Millisecond}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		PaceBody(w, r, pace)
		if _, err := io.Copy(io.Discard, r.Body); err != nil {
			WriteBodyError(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	do := func(chunks int, every time.Duration, size, unsent int) (int, time.Duration) {
		conn, err := net.Dial("tcp", srv.Listener.Addr().String())
		if err != nil {
			t.Fatal(err)
		}
		defer conn.Close()
		total := chunks*size + unsent
		_, _ = conn.Write([]byte("POST / HTTP/1.1\r\nHost: x\r\nContent-Length: " + strconv.Itoa(total) + "\r\n\r\n"))
		start := time.Now()
		go func() {
			for range chunks {
				if _, err := conn.Write([]byte(strings.Repeat("a", size))); err != nil {
					return
				}
				time.Sleep(every)
			}
		}()
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		res, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			t.Fatalf("no answer: %v", err)
		}
		_ = res.Body.Close()
		return res.StatusCode, time.Since(start)
	}

	if code, _ := do(1, 2*time.Second, 10, 100); code != 408 { // one chunk, then silence: idle deadline
		t.Errorf("stalled body: %d, want 408", code)
	}
	if code, took := do(40, 100*time.Millisecond, 1, 0); code != 408 || took > 3*time.Second { // 10 B/s: under the minimum
		t.Errorf("dripping body: %d after %v, want 408 soon", code, took)
	}
	if code, _ := do(8, 100*time.Millisecond, 4000, 0); code != 204 { // 40 KB/s: fine
		t.Errorf("healthy body: %d, want 204", code)
	}
}
