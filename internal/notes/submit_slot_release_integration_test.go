//go:build integration

package notes

import (
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/httpx"
)

// T-102 AC3: a request refused by a rule decided before its photos (here the per-address miss limit) gives its upload slot
// back before it drains the rest of the body; the drain is still paced, and the answer is unchanged (429, read after the body).
func TestRateLimitedSubmissionReleasesItsSlotBeforeTheDrain(t *testing.T) {
	old := submitPace
	submitPace = httpx.BodyPace{Idle: 5 * time.Second, Total: 30 * time.Second}
	t.Cleanup(func() { submitPace = old })
	e := newEnv(t)
	const ip = "10.0.0.9"
	for range missesPerWindow { // use up the address's 60 misses with unknown tokens (answered 404 at once)
		e.postFrom(t.Context(), ip, "unknown-token", answers("A", "m")).status(404, "ERROR_NOT_FOUND")
	}
	srv := e.serve()
	body, ct := multipartOf(answers("A", "m"), photoPart(make([]byte, 1<<20)))

	conns := make([]interface{ Write([]byte) (int, error) }, 0, e.nh.upPerIP)
	var all []func() int
	for range e.nh.upPerIP { // the address's own cap: all of them are rate limited
		conn := dial(t, srv)
		sendHead(conn, ip, "unknown-token", ct, body.Len())
		_, _ = conn.Write(body.Bytes()[:1000])
		conns = append(conns, conn)
		all = append(all, func() int { return statusOf(t, conn, 10*time.Second) })
	}
	time.Sleep(500 * time.Millisecond) // the server has read the heads and is waiting for the rest of the bodies (nothing to poll for that)
	if n := e.inProgress(); n != 0 {
		t.Fatalf("%d slots still held while the rejected bodies are being drained", n)
	}
	// the answer waits for the body (browsers show it only then) ...
	for _, c := range conns {
		_, _ = c.Write(body.Bytes()[1000:])
	}
	// ... and is the one it always was
	for _, status := range all {
		if got := status(); got != 429 {
			t.Fatalf("status %d, want 429", got)
		}
	}
	e.waitFor("no slot held", func() bool { return e.inProgress() == 0 })
	e.noSpool()
}

// The drain after an early release stays paced: a stalled body ends the drain within the idle window and the answer (as before) is the 429.
func TestRateLimitedDrainStaysPaced(t *testing.T) {
	old := submitPace
	submitPace.Idle = 300 * time.Millisecond
	t.Cleanup(func() { submitPace = old })
	e := newEnv(t)
	const ip = "10.0.0.8"
	for range missesPerWindow {
		e.postFrom(t.Context(), ip, "unknown-token", answers("A", "m")).status(404, "ERROR_NOT_FOUND")
	}
	conn := e.stall(e.serve(), ip, "unknown-token", 100)
	start := time.Now()
	if got := statusOf(t, conn, 5*time.Second); got != 429 || time.Since(start) > 3*time.Second {
		t.Fatalf("status %d after %v, want 429 within the idle window", got, time.Since(start))
	}
	e.waitFor("no slot held", func() bool { return e.inProgress() == 0 })
}
