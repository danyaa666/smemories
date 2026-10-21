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

// T-102. A short pace so that the tests are quick: 1 s without a byte, or under 1 KB/s after 0.5 s, ends an upload.
var testPace = httpx.BodyPace{Idle: time.Second, Total: 20 * time.Second, MinRate: 1000, Grace: 500 * time.Millisecond}

func (e *env) serve() *httptest.Server {
	e.mh.pace = testPace
	srv := httptest.NewServer(e.h)
	e.t.Cleanup(func() { srv.CloseClientConnections(); srv.Close() })
	return srv
}

// drip opens an upload of u that promises 1 MiB, sends the first part header and then one byte every 200 ms (so the idle
// timeout never fires, only the minimum rate does). The returned func reads the answer.
func (e *env) drip(srv *httptest.Server, u user, book string) func() (int, http.Header) {
	e.t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		e.t.Fatal(err)
	}
	e.t.Cleanup(func() { _ = conn.Close() })
	fmt.Fprintf(conn, "POST /api/media/create HTTP/1.1\r\nHost: x\r\nCookie: %s=%s\r\nOrigin: %s\r\nContent-Type: multipart/form-data; boundary=B\r\nContent-Length: %d\r\n\r\n",
		auth.CookieName, u.cookie, origin, 1<<20)
	fmt.Fprintf(conn, "--B\r\nContent-Disposition: form-data; name=\"yearbook_id\"\r\n\r\n%s", book[:5])
	stop := make(chan struct{})
	e.t.Cleanup(func() { close(stop) })
	go func() {
		for {
			select {
			case <-stop:
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
	return e.mh.UploadsInProgress() == 0 && len(e.mh.perUser) == 0
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
	deadline := time.Now().Add(5 * time.Second)
	for e.mh.UploadsInProgress() != 2 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	start := time.Now()
	res := e.realUpload(srv, a, bookA)
	if res.StatusCode != 429 || res.Header.Get("Retry-After") != "5" || time.Since(start) > time.Second {
		t.Fatalf("third upload: %d Retry-After=%q after %v", res.StatusCode, res.Header.Get("Retry-After"), time.Since(start))
	}
	if got := e.realUpload(srv, b, bookB); got.StatusCode != 200 {
		t.Fatalf("another user's upload: %d", got.StatusCode)
	}
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

// AC2: eight users with one dripping upload each hold the whole shared pool (503 for a ninth) only until the pace ends them; an
// upload whose body arrives in slow pieces, but keeps moving, still succeeds.
func TestSlowUploadsDoNotStarveOthersForLong(t *testing.T) {
	e := newEnv(t)
	srv := e.serve()
	var drips []func() (int, http.Header)
	for i := range cap(e.mh.inflight) {
		u := e.register(fmt.Sprintf("fair-%d@example.com", i))
		drips = append(drips, e.drip(srv, u, e.newBook(u)))
	}
	ninth := e.register("fair-ninth@example.com")
	book := e.newBook(ninth)
	deadline := time.Now().Add(5 * time.Second)
	for e.mh.UploadsInProgress() != cap(e.mh.inflight) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	res := e.realUpload(srv, ninth, book)
	if res.StatusCode != 503 {
		t.Fatalf("with the pool full: %d", res.StatusCode)
	}
	for i, d := range drips {
		if code, _ := d(); code != 408 {
			t.Fatalf("dripper %d: %d, want 408", i, code)
		}
	}
	e.waitIdle()

	// a slow but steady body: the photo in four pieces, 300 ms apart (idle window 1 s)
	body, ct := multipartBody(book, "file", "a.jpg", "image/jpeg", photo(t))
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	fmt.Fprintf(conn, "POST /api/media/create HTTP/1.1\r\nHost: x\r\nCookie: %s=%s\r\nOrigin: %s\r\nContent-Type: %s\r\nContent-Length: %d\r\n\r\n",
		auth.CookieName, ninth.cookie, origin, ct, body.Len())
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
