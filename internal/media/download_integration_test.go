//go:build integration

package media

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/httpx"
)

// T-107: photo downloads stream from the object store, at most 64 at a time per account / client address, and a slow reader is dropped.

// setDisplay replaces the display object of a photo (its row, ETag and type stay) and returns the new bytes.
func (e *env) setDisplay(mediaID string, size int) []byte {
	e.t.Helper()
	var key string
	if err := e.db.QueryRow(`SELECT object_key FROM media_tab WHERE public_id = ?`, mediaID).Scan(&key); err != nil {
		e.t.Fatal(err)
	}
	b := make([]byte, size)
	_, _ = rand.Read(b)
	if err := e.st.Storage.Put(context.Background(), key, "image/jpeg", b); err != nil {
		e.t.Fatal(err)
	}
	return b
}

// holdPace is the pace of a test that holds download slots: nobody is dropped by a clock the test does not control. The default pace
// (10 s without a byte) ends a held download that long after its socket buffers filled, which under load can be before the test has
// held all 64 (T-124); the slots are freed by the test closing the connection, or by its cleanup.
var holdPace = httpx.BodyPace{Idle: time.Hour, Total: time.Hour}

// hold opens a download of path (cookie "" = anonymous) and never reads the answer: the server's write blocks once the socket buffers are full.
func (e *env) hold(srv *httptest.Server, path, cookie string) net.Conn {
	e.t.Helper()
	conn, err := net.Dial("tcp", srv.Listener.Addr().String())
	if err != nil {
		e.t.Fatal(err)
	}
	_ = conn.(*net.TCPConn).SetReadBuffer(4096)
	e.t.Cleanup(func() { _ = conn.Close() })
	c := ""
	if cookie != "" {
		c = fmt.Sprintf("Cookie: %s=%s\r\n", auth.CookieName, cookie)
	}
	fmt.Fprintf(conn, "GET %s HTTP/1.1\r\nHost: x\r\n%sConnection: close\r\n\r\n", path, c)
	// Wait for the status line so that the download is known to be streaming, not still failing (a held slot that frees itself would make a cap test flaky).
	_ = conn.SetReadDeadline(time.Now().Add(20 * time.Second))
	line, err := bufio.NewReaderSize(conn, 64).ReadString('\n')
	_ = conn.SetReadDeadline(time.Time{})
	if err != nil || !strings.HasPrefix(line, "HTTP/1.1 200") {
		e.t.Fatalf("held download: %q %v", line, err)
	}
	return conn
}

func (e *env) waitInflight(n int) {
	e.t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for e.mh.DownloadsInProgress() != n {
		if time.Now().After(deadline) {
			e.t.Fatalf("downloads in progress: %d, want %d", e.mh.DownloadsInProgress(), n)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// dlAnswer is what get returns: the whole response read and closed.
type dlAnswer struct {
	StatusCode int
	Header     http.Header
	Body       string
}

// get is a plain request to the test server, one connection each (so it is a client of its own, not a held one).
func get(t *testing.T, srv *httptest.Server, path, cookie string) dlAnswer {
	t.Helper()
	req, _ := http.NewRequest("GET", srv.URL+path, nil)
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: cookie})
	}
	res, err := (&http.Client{Transport: &http.Transport{DisableKeepAlives: true}}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = res.Body.Close() }()
	b, _ := io.ReadAll(res.Body)
	return dlAnswer{res.StatusCode, res.Header, string(b)}
}

// AC3 (owner route): the 65th concurrent download of one account is 429 before the database or the store is touched (the id is not even
// a photo), another account is not affected, and a freed slot is usable again.
func TestOwnerDownloadsAreCappedPerUser(t *testing.T) {
	e := newEnv(t)
	e.mh.dlPace = holdPace
	srv := e.serve()
	a, b := e.register("a@example.com"), e.register("b@example.com")
	bookA, bookB := e.newBook(a), e.newBook(b)
	idA, idB := e.uploadPhoto(a, bookA), e.uploadPhoto(b, bookB)
	e.setDisplay(idA, 8<<20)
	path := "/api/media/get-content?id=" + idA

	var held []net.Conn
	for range maxDownloadsPerClient {
		held = append(held, e.hold(srv, path, a.cookie))
	}
	e.waitInflight(maxDownloadsPerClient)

	over := get(t, srv, "/api/media/get-content?id=01J9Z3K6V8Q4M7N2P5R8T0W1XY", a.cookie) // would be 404 if it reached the database
	if over.StatusCode != 429 || !strings.Contains(over.Body, "ERROR_RATE_LIMITED") || over.Header.Get("Retry-After") == "" || over.Header.Get("Cache-Control") != "no-store" {
		t.Fatalf("65th download: %d %v %s", over.StatusCode, over.Header, over.Body)
	}
	if r := get(t, srv, "/api/media/get-content?id="+idB+"&variant=thumb", b.cookie); r.StatusCode != 200 {
		t.Fatalf("another account: %d", r.StatusCode)
	}
	_ = held[0].Close()
	e.waitInflight(maxDownloadsPerClient - 1)
	if r := get(t, srv, "/api/media/get-content?id="+idA+"&variant=thumb", a.cookie); r.StatusCode != 200 {
		t.Fatalf("after a slot was freed: %d", r.StatusCode)
	}
}

// A client at the cap waits briefly for a slot instead of being refused at once: the 65th request is served when a download ends within
// the grace, and refused (the test above) when none does.
func TestDownloadAtTheCapWaitsForAFreedSlot(t *testing.T) {
	e := newEnv(t)
	e.mh.dlPace = holdPace
	srv := e.serve()
	a := e.register("a@example.com")
	idA := e.uploadPhoto(a, e.newBook(a))
	e.setDisplay(idA, 8<<20)
	path := "/api/media/get-content?id=" + idA
	var held []net.Conn
	for range maxDownloadsPerClient {
		held = append(held, e.hold(srv, path, a.cookie))
	}
	e.waitInflight(maxDownloadsPerClient)
	time.AfterFunc(downloadGrace/5, func() { _ = held[0].Close() })
	if r := get(t, srv, path+"&variant=thumb", a.cookie); r.StatusCode != 200 {
		t.Fatalf("65th download with a slot freed during the grace: %d %s", r.StatusCode, r.Body)
	}
}

// AC3 (public route): at most 64 per client address, answered before the limiters, the database and the store; counted as rate_limited.
func TestPublicDownloadsAreCappedPerClient(t *testing.T) {
	e := newEnv(t)
	e.mh.dlPace = holdPace
	srv := e.serve()
	u := e.register("a@example.com")
	book := e.newBook(u)
	id := e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, id)).status(200, "")
	e.setDisplay(id, 8<<20)
	path := "/api/public-media/get-content?token=" + url.QueryEscape(e.token(id, book))

	for range maxDownloadsPerClient {
		e.hold(srv, path, "")
	}
	e.waitInflight(maxDownloadsPerClient)
	over := get(t, srv, "/api/public-media/get-content?token=garbage", "") // would be 404 if it reached the link check
	if over.StatusCode != 429 || !strings.Contains(over.Body, "ERROR_RATE_LIMITED") || over.Header.Get("Retry-After") == "" {
		t.Fatalf("65th download: %d %v %s", over.StatusCode, over.Header, over.Body)
	}
}

// AC3: a gallery page over HTTP/2 asks for all its thumbnails at once (the browser's 6-per-host limit does not apply and an <img> does not
// retry a 429): 30 and 100 truly parallel requests of one account are all answered 200. The browser itself is covered by `make e2e`.
func TestGalleryPageOfThumbnailsLoadsAtOnce(t *testing.T) {
	e := newEnv(t)
	srv := e.serve()
	u := e.register("a@example.com")
	id := e.uploadPhoto(u, e.newBook(u))
	client := &http.Client{Transport: &http.Transport{MaxConnsPerHost: 0, MaxIdleConnsPerHost: 100}}
	t.Cleanup(client.CloseIdleConnections)
	for _, n := range []int{30, 100} {
		start := make(chan struct{})
		errs := make(chan string, n)
		for range n {
			go func() {
				<-start
				req, _ := http.NewRequest("GET", srv.URL+"/api/media/get-content?id="+id+"&variant=thumb", nil)
				req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
				res, err := client.Do(req)
				if err != nil {
					errs <- err.Error()
					return
				}
				_, _ = io.Copy(io.Discard, res.Body)
				_ = res.Body.Close()
				if res.StatusCode != 200 {
					errs <- strconv.Itoa(res.StatusCode)
					return
				}
				errs <- ""
			}()
		}
		close(start)
		for range n {
			if m := <-errs; m != "" {
				t.Fatalf("%d thumbnails at once: %s", n, m)
			}
		}
		e.waitInflight(0)
	}
}

// AC: a reader that stops reading is dropped after the idle time and its slot comes back (the 2-minute route timeout is not waited for).
func TestSlowDownloadReaderIsDropped(t *testing.T) {
	e := newEnv(t)
	e.mh.dlPace = httpx.BodyPace{Idle: time.Second, Total: 20 * time.Second, MinRate: 1000, Grace: 500 * time.Millisecond}
	srv := e.serve()
	u := e.register("a@example.com")
	book := e.newBook(u)
	id := e.uploadPhoto(u, book)
	e.setDisplay(id, 8<<20)
	conn := e.hold(srv, "/api/media/get-content?id="+id, u.cookie)
	e.waitInflight(1)
	start := time.Now()
	e.waitInflight(0)
	if d := time.Since(start); d > 8*time.Second {
		t.Fatalf("took %v to drop a reader that never reads", d)
	}
	// What it got is cut short: the promised length was not delivered.
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	n, _ := io.Copy(io.Discard, conn)
	if n >= 8<<20 {
		t.Fatalf("a dropped reader received the whole object (%d bytes)", n)
	}
}

// AC2: Range (middle, open end, suffix, unsatisfiable), HEAD, ETag and 304 on an object bigger than one copy buffer, on both routes;
// the bytes come from ranged reads of the store.
func TestRangesHeadAndConditionalOnBothRoutes(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	id := e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, id)).status(200, "")
	data := e.setDisplay(id, 300_000)
	size := len(data)
	owner := func(method string, hdr map[string]string) resp {
		r := httptest.NewRequest(method, "/api/media/get-content?id="+id, nil)
		for k, v := range hdr {
			r.Header.Set(k, v)
		}
		return e.send(u, r)
	}
	public := func(method string, hdr map[string]string) resp {
		return e.publicReq(method, e.token(id, book), "", hdr)
	}
	for name, call := range map[string]func(string, map[string]string) resp{"owner": owner, "public": public} {
		full := call("GET", nil)
		if full.Code != 200 || !bytes.Equal(full.Body.Bytes(), data) || full.Header().Get("Content-Length") != strconv.Itoa(size) || full.Header().Get("Accept-Ranges") != "bytes" {
			t.Fatalf("%s full: %d, %d bytes, headers %v", name, full.Code, full.Body.Len(), full.Header())
		}
		for _, c := range []struct {
			rng        string
			from, to   int
			crange     string
			wantStatus int
		}{
			{"bytes=100000-100999", 100000, 101000, fmt.Sprintf("bytes 100000-100999/%d", size), 206},
			{"bytes=299990-", 299990, size, fmt.Sprintf("bytes 299990-299999/%d", size), 206},
			{"bytes=-8", size - 8, size, fmt.Sprintf("bytes %d-%d/%d", size-8, size-1, size), 206},
			{"bytes=0-0", 0, 1, fmt.Sprintf("bytes 0-0/%d", size), 206},
			{"bytes=999999-", 0, 0, fmt.Sprintf("bytes */%d", size), 416},
		} {
			r := call("GET", map[string]string{"Range": c.rng})
			if r.Code != c.wantStatus || r.Header().Get("Content-Range") != c.crange {
				t.Fatalf("%s %s: %d %q", name, c.rng, r.Code, r.Header().Get("Content-Range"))
			}
			if c.wantStatus == 206 && !bytes.Equal(r.Body.Bytes(), data[c.from:c.to]) {
				t.Fatalf("%s %s: wrong bytes (%d)", name, c.rng, r.Body.Len())
			}
		}
		head := call("HEAD", nil)
		etag := full.Header().Get("ETag")
		if head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Length") != strconv.Itoa(size) || head.Header().Get("ETag") != etag || etag == "" {
			t.Fatalf("%s HEAD: %d, %d body bytes, %v", name, head.Code, head.Body.Len(), head.Header())
		}
		if nm := call("GET", map[string]string{"If-None-Match": etag}); nm.Code != 304 || nm.Body.Len() != 0 {
			t.Fatalf("%s If-None-Match: %d, %d bytes", name, nm.Code, nm.Body.Len())
		}
		// If-Range with the current ETag keeps the range, with another one sends the whole object.
		if r := call("GET", map[string]string{"Range": "bytes=0-9", "If-Range": etag}); r.Code != 206 {
			t.Fatalf("%s If-Range match: %d", name, r.Code)
		}
		if r := call("GET", map[string]string{"Range": "bytes=0-9", "If-Range": `"other"`}); r.Code != 200 || r.Body.Len() != size {
			t.Fatalf("%s If-Range mismatch: %d, %d bytes", name, r.Code, r.Body.Len())
		}
	}
}

// AC1: 64 readers that do not read, each on an 8 MiB object, keep the heap flat. Before T-107 every one of them held its object:
// 64 x 8 MiB = 512 MiB. The bound is 48 MiB over the start (the copy buffers and socket buffers of 64 requests are a few MiB).
// The RSS of the process is logged too (run with -v; the PR records it). Serial on purpose: nothing else runs while it measures.
func TestSlowReadersDoNotHoldTheObjectInMemory(t *testing.T) {
	const readers, perUser, objSize = 64, 8, 8 << 20 // 8 accounts x 8
	e := newSerialEnv(t)
	e.mh.dlPace = httpx.BodyPace{Idle: time.Minute, Total: time.Minute, Grace: time.Minute} // nobody is dropped during the measurement
	srv := e.serve()
	var paths, cookies []string
	for i := range readers / perUser {
		u := e.register(fmt.Sprintf("reader%d@example.com", i))
		id := e.uploadPhoto(u, e.newBook(u))
		e.setDisplay(id, objSize)
		paths, cookies = append(paths, "/api/media/get-content?id="+id), append(cookies, u.cookie)
	}
	settle := func() (heap uint64, rss int64) {
		runtime.GC()
		var m runtime.MemStats
		runtime.ReadMemStats(&m)
		out, _ := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
		kb, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		return m.HeapInuse, kb << 10
	}
	heap0, rss0 := settle()
	for i := range readers {
		e.hold(srv, paths[i/perUser], cookies[i/perUser])
	}
	e.waitInflight(readers)
	time.Sleep(2 * time.Second) // every handler has reached the blocked write
	heap1, rss1 := settle()
	e.waitInflight(readers) // still all in flight: nobody finished early because the object was small
	grew := int64(heap1) - int64(heap0)
	t.Logf("64 slow readers of an %d MiB object: heap in use %d -> %d MiB (+%d MiB), process RSS %d -> %d MiB (+%d MiB); buffering would hold %d MiB",
		objSize>>20, heap0>>20, heap1>>20, grew>>20, rss0>>20, rss1>>20, (rss1-rss0)>>20, readers*objSize>>20)
	if grew > 48<<20 {
		t.Fatalf("heap grew %d MiB with %d slow readers; a streamed download must not hold the object", grew>>20, readers)
	}
}
