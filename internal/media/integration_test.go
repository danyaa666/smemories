//go:build integration

package media

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/storage"
	"github.com/danyaa666/smemories/internal/storage/storagetest"
	"github.com/danyaa666/smemories/internal/yearbook"
)

const (
	origin = "http://localhost:5173"
	testPW = "Tr0ub4dor&3-fake-test-password" // obviously fake
)

// flaky wraps a Storage and fails the operations switched on.
type flaky struct {
	storage.Storage
	mu                       sync.Mutex
	failPut, failDeletePrefx bool
	thumbOnly                bool            // with failPut: only thumbnail writes fail
	keys                     map[string]bool // objects written through the wrapper and not yet deleted
}

// count is how many objects exist under prefix.
func (f *flaky) count(prefix string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	n := 0
	for k := range f.keys {
		if strings.HasPrefix(k, prefix) {
			n++
		}
	}
	return n
}

func (f *flaky) Delete(ctx context.Context, keys ...string) error {
	if err := f.Storage.Delete(ctx, keys...); err != nil {
		return err
	}
	f.mu.Lock()
	for _, k := range keys {
		delete(f.keys, k)
	}
	f.mu.Unlock()
	return nil
}

func (f *flaky) set(put, purge bool) {
	f.mu.Lock()
	f.failPut, f.failDeletePrefx = put, purge
	f.mu.Unlock()
}

func (f *flaky) Put(ctx context.Context, key, ct string, data []byte) error {
	f.mu.Lock()
	fail := f.failPut
	f.mu.Unlock()
	if fail && (!f.thumbOnly || strings.HasSuffix(key, "-thumb.jpg")) {
		return errors.New("injected put failure")
	}
	if err := f.Storage.Put(ctx, key, ct, data); err != nil {
		return err
	}
	f.mu.Lock()
	f.keys[key] = true
	f.mu.Unlock()
	return nil
}

func (f *flaky) DeletePrefix(ctx context.Context, prefix string) error {
	f.mu.Lock()
	fail := f.failDeletePrefx
	f.mu.Unlock()
	if fail {
		return errors.New("injected purge failure")
	}
	if err := f.Storage.DeletePrefix(ctx, prefix); err != nil {
		return err
	}
	f.mu.Lock()
	for k := range f.keys {
		if strings.HasPrefix(k, prefix) {
			delete(f.keys, k)
		}
	}
	f.mu.Unlock()
	return nil
}

// deadlines records the connection deadlines a route asks for.
type deadlines struct {
	http.ResponseWriter
	read, write bool
}

func (d *deadlines) SetReadDeadline(time.Time) error  { d.read = true; return nil }
func (d *deadlines) SetWriteDeadline(time.Time) error { d.write = true; return nil }

type env struct {
	t     *testing.T
	db    *sql.DB
	st    *flaky
	svc   *Service
	h     http.Handler
	books []string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := dbtest.New(t)
	e := &env{t: t, db: d, st: &flaky{Storage: storagetest.New(t), keys: map[string]bool{}}}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	mail, _ := mailer.NewLog("test", io.Discard)
	hasher := auth.NewHasher(auth.HashParams{MemoryKiB: 64, Time: 1, Parallelism: 1}, 4, auth.HashWait)
	as, err := auth.NewService(auth.NewStore(d), hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000},
		auth.Mail{Mailer: mail, BaseURL: "http://localhost:5173", Logger: logger}, nil)
	if err != nil {
		t.Fatal(err)
	}
	ah := auth.NewHandler(as, auth.HandlerConfig{AllowedOrigins: []string{origin}}, logger)
	e.svc = NewService(NewStore(d), e.st, 2, nil) // two at a time, so parallel tests queue on the semaphore
	mh := NewHandler(e.svc, 10<<20, ah.RequireUser, []string{origin}, logger)
	yh := yearbook.NewHandler(yearbook.NewStore(d), e.svc, ah.RequireUser, []string{origin}, logger, nil)
	e.h = httpx.NewRouter(logger, ah.Routes, yh.Routes, mh.Routes)
	t.Cleanup(func() { // objects of every book this test created
		for _, id := range e.books {
			_ = e.st.Storage.DeletePrefix(context.Background(), yearbookPrefix(id))
		}
	})
	return e
}

type user struct{ cookie string }

func (e *env) register(email string) user {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": testPW, "display_name": "Tester"})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/auth/register", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(rec, r)
	if rec.Code != 201 {
		e.t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return user{c.Value}
		}
	}
	e.t.Fatal("no session cookie")
	return user{}
}

type resp struct {
	*httptest.ResponseRecorder
	t *testing.T
}

func (e *env) send(u user, r *http.Request) resp {
	if u.cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
		r.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return resp{rec, e.t}
}

func (e *env) json(u user, method, path, body string) resp {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	return e.send(u, r)
}

// multipartBody builds a body with one file part; filename and partType are attacker-controlled in real life.
func multipartBody(field, filename, partType string, data []byte) (*bytes.Buffer, string) {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
	h.Set("Content-Type", partType)
	w, _ := mw.CreatePart(h)
	_, _ = w.Write(data)
	_ = mw.Close()
	return &b, mw.FormDataContentType()
}

func (e *env) upload(u user, book, filename, partType string, data []byte) resp {
	body, ct := multipartBody("file", filename, partType, data)
	r := httptest.NewRequest("POST", "/v1/yearbooks/"+book+"/media", body)
	r.Header.Set("Content-Type", ct)
	return e.send(u, r)
}

func (r resp) status(want int, code string) resp {
	r.t.Helper()
	if r.Code != want {
		r.t.Fatalf("status %d, want %d (%s)", r.Code, want, r.Body)
	}
	if code != "" {
		var env struct{ Error struct{ Code string } }
		if err := json.Unmarshal(r.Body.Bytes(), &env); err != nil || env.Error.Code != code {
			r.t.Fatalf("code %q (err %v), want %q: %s", env.Error.Code, err, code, r.Body)
		}
	}
	return r
}

func (e *env) newBook(u user) string {
	e.t.Helper()
	r := e.json(u, "POST", "/v1/yearbooks", `{"title":"Class of 2026","language":"en"}`).status(201, "")
	var out struct{ Yearbook struct{ ID string } }
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	e.books = append(e.books, out.Yearbook.ID)
	return out.Yearbook.ID
}

func (r resp) mediaID() string {
	r.t.Helper()
	var out struct {
		Media struct {
			ID            string
			Width, Height int
			Bytes         int
		}
	}
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil || out.Media.ID == "" || out.Media.Width == 0 || out.Media.Bytes == 0 {
		r.t.Fatalf("bad media body %s", r.Body)
	}
	return out.Media.ID
}

func (r resp) book() map[string]any {
	var out struct{ Yearbook map[string]any }
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	return out.Yearbook
}

func photo(t *testing.T) []byte {
	return jpegWith(t, quad(64, 32, false), exifSegment(1))
}

func (e *env) count(q string, args ...any) int {
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func TestUploadFetchDelete(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book := e.newBook(u)

	in := photo(t)
	r := e.upload(u, book, "../../evil.php", "text/html", in).status(201, "") // name and part type are ignored
	id := r.mediaID()
	var m struct{ Media struct{ Width, Height int } }
	_ = json.Unmarshal(r.Body.Bytes(), &m)
	if m.Media.Width != 64 || m.Media.Height != 32 {
		t.Fatalf("dimensions %+v", m)
	}

	// Keys are generated from ids only (AC4), and the stored bytes carry no EXIF (AC3).
	stored, err := e.st.Get(context.Background(), "yearbooks/"+book+"/"+id+".jpg")
	if err != nil {
		t.Fatalf("display object missing: %v", err)
	}
	if bytes.Contains(stored, []byte("Exif")) || bytes.Contains(in[:100], []byte("Exif")) == false {
		t.Fatal("stored object keeps EXIF, or the fixture had none")
	}
	if _, err := e.st.Get(context.Background(), "yearbooks/"+book+"/"+id+"-thumb.jpg"); err != nil {
		t.Fatalf("thumb object missing: %v", err)
	}
	var key, tkey string
	_ = e.db.QueryRow(`SELECT object_key, thumb_key FROM media WHERE public_id = ?`, id).Scan(&key, &tkey)
	if strings.Contains(key+tkey, "evil") || strings.Contains(key+tkey, "php") {
		t.Fatalf("file name leaked into keys: %s %s", key, tkey)
	}

	// Fetch: headers, thumb, range.
	got := e.json(u, "GET", "/v1/media/"+id+"/content?size=display", "").status(200, "")
	h := got.Header()
	if h.Get("Content-Type") != "image/jpeg" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("headers: %v", h)
	}
	if !bytes.Equal(got.Body.Bytes(), stored) {
		t.Fatal("served bytes differ from stored bytes")
	}
	th := e.json(u, "GET", "/v1/media/"+id+"/content?size=thumb", "").status(200, "")
	if cfg, _, err := image.DecodeConfig(th.Body); err != nil || cfg.Width != 64 {
		t.Fatalf("thumb: %v %v", cfg, err)
	}
	rr := httptest.NewRequest("GET", "/v1/media/"+id+"/content", nil)
	rr.Header.Set("Range", "bytes=0-9")
	part := e.send(u, rr).status(206, "")
	if part.Body.Len() != 10 || !strings.HasPrefix(part.Header().Get("Content-Range"), "bytes 0-9/") || !bytes.Equal(part.Body.Bytes(), stored[:10]) {
		t.Fatalf("range: %d bytes, %q", part.Body.Len(), part.Header().Get("Content-Range"))
	}
	e.json(u, "GET", "/v1/media/"+id+"/content?size=huge", "").status(400, "invalid_size")

	// Another user sees nothing, and cannot delete.
	e.json(other, "GET", "/v1/media/"+id+"/content", "").status(404, "not_found")
	e.upload(other, book, "a.jpg", "image/jpeg", in).status(404, "not_found")
	e.json(other, "DELETE", "/v1/media/"+id, "").status(204, "")
	e.json(u, "GET", "/v1/media/"+id+"/content", "").status(200, "")
	e.json(user{}, "GET", "/v1/media/"+id+"/content", "").status(401, "")
	e.upload(user{}, book, "a.jpg", "image/jpeg", in).status(401, "")

	// Delete removes row and both objects; repeating it is fine.
	e.json(u, "DELETE", "/v1/media/"+id, "").status(204, "")
	e.json(u, "DELETE", "/v1/media/"+id, "").status(204, "")
	e.json(u, "GET", "/v1/media/"+id+"/content", "").status(404, "not_found")
	for _, k := range []string{key, tkey} {
		if _, err := e.st.Get(context.Background(), k); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("%s survived the delete: %v", k, err)
		}
	}
}

func TestUploadRejections(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	path := "/v1/yearbooks/" + book + "/media"

	html := []byte("<html><script>alert(1)</script></html>")
	e.upload(u, book, "x.jpg", "image/jpeg", html).status(415, "unsupported_media_type") // renamed HTML
	e.upload(u, book, "x.jpg", "image/jpeg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)).status(415, "unsupported_media_type")
	e.upload(u, book, "x.jpg", "image/jpeg", nil).status(415, "unsupported_media_type") // 0 bytes
	e.upload(u, book, "x.exe", "application/octet-stream", append([]byte("MZ\x90\x00"), make([]byte, 600)...)).status(415, "unsupported_media_type")
	e.upload(u, book, "x.jpg", "image/jpeg", pngHeaderClaiming(t, 60000, 60000)).status(400, "invalid_image")
	good := photo(t)
	e.upload(u, book, "x.jpg", "image/jpeg", good[:len(good)/2]).status(400, "invalid_image")

	big := bytes.Repeat([]byte{0xFF, 0xD8}, 11<<19) // 11 MiB
	e.upload(u, book, "big.jpg", "image/jpeg", big).status(413, "payload_too_large")

	// Wrong request shapes.
	r := httptest.NewRequest("POST", path, strings.NewReader(`{"file":"x"}`))
	r.Header.Set("Content-Type", "application/json")
	e.send(u, r).status(415, "unsupported_media_type")
	body, ct := multipartBody("photo", "a.jpg", "image/jpeg", good) // wrong field name
	r = httptest.NewRequest("POST", path, body)
	r.Header.Set("Content-Type", ct)
	e.send(u, r).status(400, "invalid_body")
	body, ct = multipartBody("file", "a.jpg", "image/jpeg", good)
	r = httptest.NewRequest("POST", path, body)
	r.Header.Set("Content-Type", ct)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
	r.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("cross-origin upload: %d", rec.Code)
	}
	if n := e.count(`SELECT COUNT(*) FROM media`); n != 0 {
		t.Fatalf("%d rows after only rejected uploads", n)
	}
}

func TestRouteDeadlinesAreExtended(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	id := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(201, "").mediaID()

	body, ct := multipartBody("file", "a.jpg", "image/jpeg", photo(t))
	up := httptest.NewRequest("POST", "/v1/yearbooks/"+book+"/media", body)
	up.Header.Set("Content-Type", ct)
	up.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
	up.Header.Set("Origin", origin)
	dl := httptest.NewRequest("GET", "/v1/media/"+id+"/content", nil)
	dl.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
	for name, c := range map[string]struct {
		r         *http.Request
		wantRead  bool
		wantWrite bool
	}{"upload": {up, true, true}, "download": {dl, false, true}} {
		w := &deadlines{ResponseWriter: httptest.NewRecorder()}
		e.h.ServeHTTP(w, c.r)
		if w.read != c.wantRead || w.write != c.wantWrite {
			t.Errorf("%s: read deadline set=%v write deadline set=%v, want %v %v", name, w.read, w.write, c.wantRead, c.wantWrite)
		}
	}
}

func TestQuotas(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	fill := func(n int, bytes int) {
		for i := range n {
			_, err := e.db.Exec(`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at)
				SELECT ?, id, 'owner', 'k', 't', 'image/jpeg', ?, 1, 1, '', NOW(6) FROM yearbooks WHERE public_id = ?`, fmt.Sprintf("FILL%022d", i+e.count(`SELECT COUNT(*) FROM media`)), bytes, book)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// 197 rows, then 6 parallel uploads: exactly three fit, the rest are refused, and refused ones leave no objects.
	fill(197, 1)
	var wg sync.WaitGroup
	codes := make([]int, 6)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes[i] = e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).Code
		}()
	}
	wg.Wait()
	ok, full := 0, 0
	for _, c := range codes {
		switch c {
		case 201:
			ok++
		case 409:
			full++
		default:
			t.Fatalf("unexpected status in %v", codes)
		}
	}
	if ok != 3 || full != 3 {
		t.Fatalf("statuses %v, want 3x201 and 3x409", codes)
	}
	if n := e.st.count(yearbookPrefix(book)); n != 6 { // 3 photos x (display + thumb)
		t.Fatalf("%d objects, want 6", n)
	}
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(409, "quota_exceeded")

	// Bytes per user, across books.
	book2 := e.newBook(u)
	if _, err := e.db.Exec(`UPDATE media SET bytes = 500*1024*1024 - 1000 WHERE id = (SELECT id FROM (SELECT MIN(id) AS id FROM media) x)`); err != nil {
		t.Fatal(err)
	}
	e.upload(u, book2, "a.jpg", "image/jpeg", photo(t)).status(409, "quota_exceeded")
	if _, err := e.db.Exec(`UPDATE media SET bytes = 1 WHERE bytes > 1000000`); err != nil {
		t.Fatal(err)
	}
	e.upload(u, book2, "a.jpg", "image/jpeg", photo(t)).status(201, "")
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t)
	e.svc.limiter = ratelimit.New(3, time.Minute, nil)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book, book2 := e.newBook(u), e.newBook(other)
	for range 3 {
		e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(201, "")
	}
	r := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(429, "rate_limited")
	if r.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.upload(other, book2, "a.jpg", "image/jpeg", photo(t)).status(201, "") // per user
}

func TestStorageFailureLeavesNoRow(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	e.st.set(true, false)
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(502, "storage_error")
	e.st.set(false, false)
	if n := e.count(`SELECT COUNT(*) FROM media`); n != 0 || e.st.count("") != 0 {
		t.Fatalf("%d rows and %d objects after a failed upload", n, e.st.count(""))
	}
	e.st.set(true, false)
	e.st.thumbOnly = true // the display object is written first, then the thumbnail fails: nothing may stay behind
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(502, "storage_error")
	e.st.thumbOnly = false
	e.st.set(false, false)
	if n := e.st.count(""); n != 0 {
		t.Fatalf("%d orphan objects after a failed thumbnail write", n)
	}
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(201, "") // and a retry works
}

func TestDeleteYearbookRemovesObjects(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	a := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(201, "").mediaID()
	b := e.upload(u, book, "b.jpg", "image/jpeg", photo(t)).status(201, "").mediaID()
	e.json(u, "PATCH", "/v1/yearbooks/"+book, fmt.Sprintf(`{"cover_media_id":%q}`, a)).status(200, "") // book -> media -> book reference cycle

	// A failing store blocks the delete and keeps everything.
	e.st.set(false, true)
	e.json(u, "DELETE", "/v1/yearbooks/"+book, "").status(502, "storage_error")
	e.st.set(false, false)
	e.json(u, "GET", "/v1/yearbooks/"+book, "").status(200, "")
	e.json(u, "GET", "/v1/media/"+b+"/content", "").status(200, "")

	// Orphans that have no row (a crashed upload) go too.
	_ = e.st.Storage.Put(context.Background(), yearbookPrefix(book)+"ORPHAN.jpg", "image/jpeg", []byte("x"))
	e.json(u, "DELETE", "/v1/yearbooks/"+book, "").status(204, "")
	if n := e.count(`SELECT COUNT(*) FROM media`); n != 0 {
		t.Fatalf("%d media rows left", n)
	}
	for _, k := range []string{yearbookPrefix(book) + "ORPHAN.jpg", yearbookPrefix(book) + a + ".jpg", yearbookPrefix(book) + b + "-thumb.jpg"} {
		if _, err := e.st.Get(context.Background(), k); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("%s survived: %v", k, err)
		}
	}
	e.json(u, "DELETE", "/v1/yearbooks/"+book, "").status(404, "not_found")
	if n := e.st.count(yearbookPrefix(book)); n != 0 {
		t.Fatalf("%d objects left in the bucket", n)
	}
}

func TestProfilePhotoAndCover(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book, book2, foreign := e.newBook(u), e.newBook(u), e.newBook(other)
	mine := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(201, "").mediaID()
	inBook2 := e.upload(u, book2, "a.jpg", "image/jpeg", photo(t)).status(201, "").mediaID()
	theirs := e.upload(other, foreign, "a.jpg", "image/jpeg", photo(t)).status(201, "").mediaID()

	profile := func(media string) string {
		return fmt.Sprintf(`{"full_name":"Tester","photo_media_id":%s}`, media)
	}
	quote := func(s string) string { return `"` + s + `"` }

	b := e.json(u, "PUT", "/v1/yearbooks/"+book+"/profile", profile(quote(mine))).status(200, "").book()
	if b["profile"].(map[string]any)["photo_media_id"] != mine {
		t.Fatalf("photo not set: %v", b["profile"])
	}
	b = e.json(u, "PATCH", "/v1/yearbooks/"+book, fmt.Sprintf(`{"cover_media_id":%q}`, mine)).status(200, "").book()
	if b["cover_media_id"] != mine {
		t.Fatalf("cover not set: %v", b)
	}
	// Media of another book of the same user, another user's media, and unknown ids are all invalid_media.
	for _, bad := range []string{inBook2, theirs, "01ARZ3NDEKTSV4RRFFQ69G5FAV"} {
		e.json(u, "PUT", "/v1/yearbooks/"+book+"/profile", profile(quote(bad))).status(400, "invalid_media")
		e.json(u, "PATCH", "/v1/yearbooks/"+book, fmt.Sprintf(`{"cover_media_id":%q}`, bad)).status(400, "invalid_media")
	}
	// A title edit leaves the cover alone; null clears it; a missing photo_media_id in the (replacing) PUT clears the photo.
	b = e.json(u, "PATCH", "/v1/yearbooks/"+book, `{"title":"New"}`).status(200, "").book()
	if b["cover_media_id"] != mine {
		t.Fatalf("cover lost by an unrelated edit: %v", b)
	}
	b = e.json(u, "PATCH", "/v1/yearbooks/"+book, `{"cover_media_id":null}`).status(200, "").book()
	if b["cover_media_id"] != nil {
		t.Fatalf("cover not cleared: %v", b)
	}
	e.json(u, "PUT", "/v1/yearbooks/"+book+"/profile", profile(quote(mine))).status(200, "")
	e.json(u, "PATCH", "/v1/yearbooks/"+book, fmt.Sprintf(`{"cover_media_id":%q}`, mine)).status(200, "")
	// Deleting the media clears both references.
	e.json(u, "DELETE", "/v1/media/"+mine, "").status(204, "")
	b = e.json(u, "GET", "/v1/yearbooks/"+book, "").status(200, "").book()
	if b["cover_media_id"] != nil || b["profile"].(map[string]any)["photo_media_id"] != nil {
		t.Fatalf("references survived the media delete: %v", b)
	}
	// A new book cannot start with a cover.
	e.json(u, "POST", "/v1/yearbooks", fmt.Sprintf(`{"title":"T","language":"en","cover_media_id":%q}`, mine)).status(400, "invalid_media")
}

// T-034: contributor uploads wait a bounded time for a processing slot, count toward the quotas and can be discarded.
func TestContributorUploadBusyAndDiscard(t *testing.T) {
	e := newEnv(t)
	u := e.register("owner@example.com")
	book := e.newBook(u)
	var ownerID, row uint64
	if err := e.db.QueryRow(`SELECT owner_id, id FROM yearbooks WHERE public_id = ?`, book).Scan(&ownerID, &row); err != nil {
		t.Fatal(err)
	}
	data := photo(t)

	for range cap(e.svc.sem) { // every slot taken: the wait is bounded and ends in ErrBusy, with nothing stored
		e.svc.sem <- struct{}{}
	}
	start := time.Now()
	loaded := false
	load := func() ([]byte, error) { loaded = true; return data, nil }
	if _, err := e.svc.save(context.Background(), ownerID, row, book, "contributor", load, 50*time.Millisecond); !errors.Is(err, ErrBusy) {
		t.Fatalf("err %v, want ErrBusy", err)
	}
	if loaded {
		t.Fatal("the photo was loaded into memory without a processing slot")
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("did not give up in time")
	}
	for range cap(e.svc.sem) {
		<-e.svc.sem
	}
	if e.count(`SELECT COUNT(*) FROM media`) != 0 || e.st.count(yearbookPrefix(book)) != 0 {
		t.Fatal("a busy refusal stored something")
	}

	m, err := e.svc.UploadContributor(context.Background(), row, data)
	if err != nil || m.RowID == 0 {
		t.Fatalf("upload: %v %+v", err, m)
	}
	if e.count(`SELECT COUNT(*) FROM media WHERE uploader_kind = 'contributor' AND yearbook_id = ?`, row) != 1 || e.st.count(yearbookPrefix(book)) != 2 {
		t.Fatal("contributor photo not stored as such")
	}
	if err := e.svc.Discard(m); err != nil {
		t.Fatal(err)
	}
	if e.count(`SELECT COUNT(*) FROM media`) != 0 || e.st.count(yearbookPrefix(book)) != 0 {
		t.Fatal("discard left a row or objects")
	}
	if _, err := e.svc.UploadContributor(context.Background(), row+1000, data); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown yearbook: %v", err)
	}
}
