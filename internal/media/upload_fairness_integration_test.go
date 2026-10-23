//go:build integration

package media

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

// T-102. A short pace so that the tests are quick: 1 s without a byte ends an upload. The tests that hold slots use it as it is:
// a drip keeps the upload alive until the test calls stallDrips (T-123), so a slot is never freed by a clock the test does not control.
var testPace = httpx.BodyPace{Idle: time.Second, Total: time.Minute}

// minRatePace also drops a body under 1 KB/s after 0.5 s, however often it moves. Only TestDrippingUploadEndsOnMinRate uses it: it
// frees a slot within about 0.7 s, so any test that needs several slots held at once must not.
var minRatePace = httpx.BodyPace{Idle: time.Second, Total: 20 * time.Second, MinRate: 1000, Grace: 500 * time.Millisecond}

func (e *env) serve() *httptest.Server { return e.servePaced(testPace) }

func (e *env) servePaced(pace httpx.BodyPace) *httptest.Server {
	e.mh.pace = pace
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if ip := r.Header.Get("X-Test-IP"); ip != "" { // one test can play many addresses
			r.RemoteAddr = ip + ":4000"
		}
		e.h.ServeHTTP(w, r)
	}))
	e.t.Cleanup(func() { srv.CloseClientConnections(); srv.Close() })
	return srv
}

// stallDrips makes every drip() go silent: each upload it holds ends in 408 once the idle window (1 s) has passed.
func (e *env) stallDrips() { e.stallMu.Do(func() { close(e.stall) }) }

// waitInProgress waits until n owner uploads hold a shared slot (the slot gauge) and fails the test if they never do, so a probe
// never runs against a pool that is not in the state the test assumes.
func (e *env) waitInProgress(n int) {
	e.t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for e.mh.UploadsInProgress() != n {
		if time.Now().After(deadline) {
			e.t.Fatalf("%d uploads hold a slot, want %d", e.mh.UploadsInProgress(), n)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// drip opens an upload of u that promises 1 MiB, sends the first part header and then one byte every 200 ms (so the idle
// timeout does not fire, and with minRatePace the minimum rate does) until stallDrips. The returned func reads the answer.
func (e *env) drip(srv *httptest.Server, u user, book string) func() (int, http.Header) {
	e.t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = conn.Close() })
	fmt.Fprintf(conn, "POST /api/media/create HTTP/1.1\r\nHost: x\r\nCookie: %s=%s\r\nOrigin: %s\r\nX-Test-IP: %s\r\nContent-Type: multipart/form-data; boundary=B\r\nContent-Length: %d\r\n\r\n",
		auth.CookieName, u.cookie, origin, u.ip, 1<<20)
	fmt.Fprintf(conn, "--B\r\nContent-Disposition: form-data; name=\"yearbook_id\"\r\n\r\n%s", book[:5])
	e.t.Cleanup(e.stallDrips)
	go func() {
		for {
			select {
			case <-e.stall:
				return
			case <-time.After(200 * time.Millisecond):
				if _, err := conn.Write([]byte("x")); err != nil {
					return
				}
			}
		}
	}()
	return func() (int, http.Header) {
		_ = conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		res, err := http.ReadResponse(bufio.NewReader(conn), nil)
		if err != nil {
			e.t.Fatalf("no answer: %v", err)
		}
		_ = res.Body.Close()
		return res.StatusCode, res.Header
	}
}

func (e *env) idle() bool {
	e.mh.userMu.Lock()
	defer e.mh.userMu.Unlock()
	return e.mh.UploadsInProgress() == 0 && len(e.mh.perUser) == 0 && len(e.mh.perClient) == 0
}

func (e *env) waitIdle() {
	e.t.Helper()
	for range 500 {
		if e.idle() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	e.t.Fatal("upload slots were not released")
}

// realUpload sends a complete small photo and returns the answer, its body already closed.
type answer struct {
	StatusCode int
	Header     http.Header
}

func (e *env) realUpload(srv *httptest.Server, u user, book string) answer {
	e.t.Helper()
	body, ct := multipartBody(book, "file", "a.jpg", "image/jpeg", photo(e.t))
	req, _ := http.NewRequest("POST", srv.URL+"/api/media/create", body)
	req.Header.Set("Content-Type", ct)
	req.Header.Set("Origin", origin)
	req.Header.Set("X-Test-IP", u.ip)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	_ = res.Body.Close()
	return answer{res.StatusCode, res.Header}
}

// AC1: two dripping uploads of one user fill that user's share; the third is 429 at once with Retry-After 5 and without a body
// byte being read; another user is not affected; the drippers end in 408 and every slot is back.
func TestPerUserUploadCap(t *testing.T) {
	e := newEnv(t)
	srv := e.serve()
	a, b := e.register("fair-a@example.com"), e.register("fair-b@example.com")
	bookA, bookB := e.newBook(a), e.newBook(b)

	d1, d2 := e.drip(srv, a, bookA), e.drip(srv, a, bookA)
	e.waitInProgress(2)
	start := time.Now()
	res := e.realUpload(srv, a, bookA)
	if res.StatusCode != 429 || res.Header.Get("Retry-After") != "5" || time.Since(start) > time.Second {
		t.Fatalf("third upload: %d Retry-After=%q after %v", res.StatusCode, res.Header.Get("Retry-After"), time.Since(start))
	}
	if got := e.realUpload(srv, b, bookB); got.StatusCode != 200 {
		t.Fatalf("another user's upload: %d", got.StatusCode)
	}
	e.stallDrips()
	for i, d := range []func() (int, http.Header){d1, d2} {
		if code, _ := d(); code != 408 {
			t.Fatalf("dripper %d: %d, want 408", i, code)
		}
	}
	if time.Since(start) > 30*time.Second {
		t.Fatal("the drippers lived too long")
	}
	e.waitIdle()
	if got := e.realUpload(srv, a, bookA); got.StatusCode != 200 { // the cap is released
		t.Fatalf("after the drippers: %d", got.StatusCode)
	}
}

// AC2: eight users with one dripping upload each hold the whole shared pool (503 for a ninth) only until the pace ends them (here:
// until they fall silent, the idle window; the minimum rate has its own test); an upload whose body arrives in slow pieces, but
// keeps moving, still succeeds.
func TestSlowUploadsDoNotStarveOthersForLong(t *testing.T) {
	e := newEnv(t)
	e.mh.clientCap = 100 // this test is about the shared pool: its eight users must not meet the per-address share
	srv := e.serve()
	var drips []func() (int, http.Header)
	for i := range cap(e.mh.inflight) {
		u := e.register(fmt.Sprintf("fair-%d@example.com", i))
		drips = append(drips, e.drip(srv, u, e.newBook(u)))
	}
	ninth := e.register("fair-ninth@example.com")
	book := e.newBook(ninth)
	e.waitInProgress(cap(e.mh.inflight)) // the drips hold their slots until stallDrips, so the pool stays full for the probe
	res := e.realUpload(srv, ninth, book)
	if res.StatusCode != 503 {
		t.Fatalf("with the pool full: %d", res.StatusCode)
	}
	e.stallDrips()
	for i, d := range drips {
		if code, _ := d(); code != 408 {
			t.Fatalf("dripper %d: %d, want 408", i, code)
		}
	}
	e.waitIdle()

	// a slow but steady body: the photo in four pieces, 300 ms apart (idle window 1 s), on a server that also enforces the minimum rate
	srv = e.servePaced(minRatePace)
	body, ct := multipartBody(book, "file", "a.jpg", "image/jpeg", photo(t))
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	fmt.Fprintf(conn, "POST /api/media/create HTTP/1.1\r\nHost: x\r\nCookie: %s=%s\r\nOrigin: %s\r\nX-Test-IP: %s\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n",
		auth.CookieName, ninth.cookie, origin, ninth.ip, ct, body.Len())
	all := body.Bytes()
	for i := range 4 {
		lo, hi := len(all)*i/4, len(all)*(i+1)/4
		_, _ = conn.Write(all[lo:hi])
		time.Sleep(300 * time.Millisecond)
	}
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	got, err := http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = got.Body.Close()
	if got.StatusCode != 200 {
		t.Fatalf("slow but steady upload: %d", got.StatusCode)
	}
}

// T-102: a body that keeps moving, but under the minimum rate, ends in 408 and gives every slot back. This is the only test that
// lets the rate end an upload; it needs no second upload, so it has no timing to get wrong.
func TestDrippingUploadEndsOnMinRate(t *testing.T) {
	e := newEnv(t)
	srv := e.servePaced(minRatePace)
	u := e.register("rate-a@example.com")
	if code, _ := e.drip(srv, u, e.newBook(u))(); code != 408 {
		t.Fatalf("dripping upload: %d, want 408", code)
	}
	e.waitIdle()
}

// T-110 AC1: an unverified account is 403 ERROR_EMAIL_NOT_VERIFIED on media/create, with no slot taken and no body read (the
// answer comes while the client is still silent); a verified one uploads.
func TestUnverifiedAccountCannotUpload(t *testing.T) {
	e := newEnv(t)
	srv := e.serve()
	v, un := e.register("ver-v@example.com"), e.registerUnverified("ver-un@example.com")
	book, unBook := e.newBook(v), e.newBook(un)
	d := e.drip(srv, un, unBook) // promises 1 MiB, sends a few bytes and keeps dripping
	if code, _ := d(); code != 403 {
		t.Fatalf("unverified, dripping: %d, want 403", code)
	}
	if e.mh.UploadsInProgress() != 0 || len(e.mh.perUser) != 0 || len(e.mh.perClient) != 0 {
		t.Fatal("a slot was taken for an unverified account")
	}
	e.upload(un, unBook, "a.jpg", "image/jpeg", photo(t)).status(403, "ERROR_EMAIL_NOT_VERIFIED")
	if got := e.realUpload(srv, v, book); got.StatusCode != 200 {
		t.Fatalf("verified: %d", got.StatusCode)
	}
}

// T-110 AC2: two accounts with two drips each fill one address's share of four; the next upload from that address is 429 with
// Retry-After 5 (before its body), a victim from another address uploads, and the drippers end in 408 and give everything back.
func TestPerClientUploadShare(t *testing.T) {
	e := newEnv(t)
	srv := e.serve()
	const attacker, other = "10.1.0.1", "10.1.0.2"
	a, b, c := e.register("share-a@example.com").from(attacker), e.register("share-b@example.com").from(attacker), e.register("share-c@example.com").from(attacker)
	victim := e.register("share-v@example.com").from(other)
	bookA, bookB, bookC, bookV := e.newBook(a), e.newBook(b), e.newBook(c), e.newBook(victim)

	var drips []func() (int, http.Header)
	for _, d := range []struct {
		u    user
		book string
	}{{a, bookA}, {a, bookA}, {b, bookB}, {b, bookB}} {
		drips = append(drips, e.drip(srv, d.u, d.book))
	}
	e.waitInProgress(maxUploadsPerClient)
	start := time.Now()
	res := e.realUpload(srv, c, bookC) // a third account of the same address: its own cap is empty, the address share is full
	if res.StatusCode != 429 || res.Header.Get("Retry-After") != "5" || time.Since(start) > time.Second {
		t.Fatalf("fifth from one address: %d Retry-After=%q after %v", res.StatusCode, res.Header.Get("Retry-After"), time.Since(start))
	}
	if got := e.realUpload(srv, victim, bookV); got.StatusCode != 200 {
		t.Fatalf("victim from another address: %d", got.StatusCode)
	}
	e.stallDrips()
	for i, d := range drips {
		if code, _ := d(); code != 408 {
			t.Fatalf("dripper %d: %d, want 408", i, code)
		}
	}
	e.waitIdle()
	if got := e.realUpload(srv, c, bookC); got.StatusCode != 200 { // the share is released
		t.Fatalf("after the drippers: %d", got.StatusCode)
	}
}
