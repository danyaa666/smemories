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
	"github.com/danyaa666/smemories/internal/medialink"
	"github.com/danyaa666/smemories/internal/metrics"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis/redistest"
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
	thumbOnly, printOnly     bool             // with failPut: only thumbnail (print) writes fail
	keys                     map[string]bool  // objects written through the wrapper and not yet deleted
	failGet                  bool             // Get fails with an error that is not ErrNotFound (an outage)
	afterPut                 func(key string) // runs after a successful Put (a racing delete in the backfill tests)
}

func (f *flaky) Get(ctx context.Context, key string) ([]byte, error) {
	f.mu.Lock()
	fail := f.failGet
	f.mu.Unlock()
	if fail {
		return nil, errors.New("injected get failure")
	}
	return f.Storage.Get(ctx, key)
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
	if fail && (!f.thumbOnly || strings.HasSuffix(key, "-thumb.jpg")) && (!f.printOnly || strings.Contains(key, "-print.")) {
		return errors.New("injected put failure")
	}
	if err := f.Storage.Put(ctx, key, ct, data); err != nil {
		return err
	}
	f.mu.Lock()
	f.keys[key] = true
	f.mu.Unlock()
	if f.afterPut != nil {
		f.afterPut(key)
	}
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
	t       *testing.T
	db      *sql.DB
	st      *flaky
	svc     *Service
	h       http.Handler
	mh      *Handler
	books   []string
	lim     *ratelimit.Factory
	cursors *httpx.Cursors
	signer  *medialink.Signer // T-092: signs the public photo links of this env's API
	logs    *syncBuf          // everything the API logged
	metrics *metrics.Metrics
}

// syncBuf is a log sink that tests read while requests may still be finishing.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func newEnv(t *testing.T) *env {
	t.Helper()
	t.Parallel() // T-077: every env has its own database, Redis prefix, ULID-keyed objects and processing semaphore, so tests never share state
	d := dbtest.New(t)
	e := &env{t: t, db: d, st: &flaky{Storage: storagetest.New(t), keys: map[string]bool{}}}
	e.logs = &syncBuf{}
	logger := slog.New(slog.NewJSONHandler(e.logs, nil))
	mail, _ := mailer.NewLog("test", io.Discard)
	hasher := auth.NewHasher(auth.HashParams{MemoryKiB: 64, Time: 1, Parallelism: 1}, 4, auth.HashWait)
	rc := redistest.New(t)
	codes, err := auth.NewCodes(context.Background(), rc, "test", []byte(strings.Repeat("k", auth.MinOTPKeyBytes)), "")
	if err != nil {
		t.Fatal(err)
	}
	e.lim = ratelimit.NewFactory(rc, logger, nil)
	sessions, err := auth.NewSessions(context.Background(), rc, logger)
	if err != nil {
		t.Fatal(err)
	}
	as, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 1000},
		auth.Mail{Mailer: mail, Logger: logger}, codes, e.lim, nil)
	if err != nil {
		t.Fatal(err)
	}
	ah := auth.NewHandler(as, auth.HandlerConfig{AllowedOrigins: []string{origin}}, logger)
	e.svc = NewService(NewStore(d), e.st, 2, e.lim, nil) // two at a time, so parallel tests queue on the semaphore
	e.cursors = httpx.NewCursors([]byte(strings.Repeat("c", 32)))
	mh := NewHandler(e.svc, 10<<20, ah.RequireUser, []string{origin}, e.cursors, logger)
	e.mh = mh
	e.signer, _ = medialink.New([]byte(strings.Repeat("L", medialink.MinSecretBytes)))
	e.svc.SetPublicLinks(e.signer)
	e.metrics = metrics.New()
	mh.SetPublicCounter(e.metrics.PublicMedia)
	bookStore := yearbook.NewStore(d)
	e.svc.SetRefClearer(bookStore)
	bookSvc := yearbook.NewService(bookStore, e.cursors, e.svc, nil, e.svc)
	bookSvc.SetLimiters(e.lim)
	yh := yearbook.NewHandler(bookSvc, ah.RequireUser, []string{origin})
	e.h = httpx.NewRouterWith(logger, httpx.RouterConfig{Metrics: e.metrics}, ah.Routes, yh.Routes, mh.Routes)
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
	r := httptest.NewRequest("POST", "/api/auth/register", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(rec, r)
	if rec.Code != 200 {
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

// multipartBody builds a body with the yearbook_id field (omitted when book is "") and one file part; filename and
// partType are attacker-controlled in real life.
func multipartBody(book, field, filename, partType string, data []byte) (*bytes.Buffer, string) {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	if book != "" {
		_ = mw.WriteField("yearbook_id", book)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name=%q; filename=%q`, field, filename))
	h.Set("Content-Type", partType)
	w, _ := mw.CreatePart(h)
	_, _ = w.Write(data)
	_ = mw.Close()
	return &b, mw.FormDataContentType()
}

func (e *env) upload(u user, book, filename, partType string, data []byte) resp {
	body, ct := multipartBody(book, "file", filename, partType, data)
	r := httptest.NewRequest("POST", "/api/media/create", body)
	r.Header.Set("Content-Type", ct)
	return e.send(u, r)
}

func (r resp) status(want int, code string) resp {
	r.t.Helper()
	if r.Code != want {
		r.t.Fatalf("status %d, want %d (%s)", r.Code, want, r.Body)
	}
	if code != "" {
		var env struct{ Status string }
		_ = json.Unmarshal(r.Body.Bytes(), &env)
		if env.Status != code {
			r.t.Fatalf("code %q, want %q: %s", env.Status, code, r.Body)
		}
	}
	return r
}

func (e *env) newBook(u user) string {
	e.t.Helper()
	id := e.json(u, "POST", "/api/yearbook/create", `{"title":"Class of 2026","language":"en"}`).status(200, "").book()["id"].(string)
	e.books = append(e.books, id)
	return id
}

// deleteMedia calls POST /api/media/delete.
func (e *env) deleteMedia(u user, id string) resp {
	return e.json(u, "POST", "/api/media/delete", fmt.Sprintf(`{"id":%q}`, id))
}

// mediaBody is the data of a v2 media object (create answers with it, get-list has it per item).
type mediaBody struct {
	ID          string `json:"id"`
	YearbookID  string `json:"yearbook_id"`
	ContentType string `json:"content_type"`
	Bytes       int
	Width       int
	Height      int
	CreatedAt   int64 `json:"created_at"`
}

func (r resp) media() mediaBody {
	r.t.Helper()
	var out struct {
		Status string
		Data   mediaBody
	}
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil || out.Status != "OK" || out.Data.ID == "" || out.Data.Width == 0 || out.Data.Bytes == 0 {
		r.t.Fatalf("bad media body %s", r.Body)
	}
	return out.Data
}

func (r resp) mediaID() string { return r.media().ID }

// book is the yearbook in the data of a yearbook answer.
func (r resp) book() map[string]any {
	var out struct{ Data map[string]any }
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	return out.Data
}

// The yearbook endpoints, which the tests use for setup and for the cross-domain checks.
func withID(key, id, body string) string {
	rest := strings.TrimPrefix(body, "{")
	if strings.TrimSpace(rest) != "}" {
		rest = "," + rest
	}
	return `{"` + key + `":"` + id + `"` + rest
}

func (e *env) getBook(u user, id string) resp {
	return e.json(u, "GET", "/api/yearbook/get?id="+id, "")
}
func (e *env) updateBook(u user, id, body string) resp {
	return e.json(u, "POST", "/api/yearbook/update", withID("id", id, body))
}
func (e *env) updateProfile(u user, id, body string) resp {
	return e.json(u, "POST", "/api/yearbook/update-profile", withID("yearbook_id", id, body))
}
func (e *env) deleteBook(u user, id string) resp {
	return e.json(u, "POST", "/api/yearbook/delete", withID("id", id, "{}"))
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

func stored(t *testing.T, e *env, book, id string) []byte {
	t.Helper()
	b, err := e.st.Get(context.Background(), "yearbooks/"+book+"/"+id+".jpg")
	if err != nil {
		t.Fatalf("display object missing: %v", err)
	}
	return b
}

func TestUploadFetchDelete(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book := e.newBook(u)

	in := photo(t)
	r := e.upload(u, book, "../../evil.php", "text/html", in).status(200, "") // name and part type are ignored
	id := r.mediaID()
	m := r.media()
	if m.Width != 64 || m.Height != 32 || m.YearbookID != book || m.ContentType != "image/jpeg" || m.Bytes != len(stored(t, e, book, id)) {
		t.Fatalf("media %+v", m)
	}
	if now := time.Now().UnixMilli(); m.CreatedAt < now-time.Minute.Milliseconds() || m.CreatedAt > now+time.Minute.Milliseconds() {
		t.Fatalf("created_at %d is not Unix ms near %d", m.CreatedAt, now)
	}
	if strings.Contains(r.Body.String(), "yearbooks/") || strings.Contains(r.Body.String(), "sha256") {
		t.Fatalf("create leaks a storage key or hash: %s", r.Body)
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
	_ = e.db.QueryRow(`SELECT object_key, thumb_key FROM media_tab WHERE public_id = ?`, id).Scan(&key, &tkey)
	if strings.Contains(key+tkey, "evil") || strings.Contains(key+tkey, "php") {
		t.Fatalf("file name leaked into keys: %s %s", key, tkey)
	}

	// Fetch: headers, thumb, range.
	got := e.json(u, "GET", "/api/media/get-content?id="+id+"&variant=display", "").status(200, "")
	h := got.Header()
	if h.Get("Content-Type") != "image/jpeg" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("headers: %v", h)
	}
	if !bytes.Equal(got.Body.Bytes(), stored) {
		t.Fatal("served bytes differ from stored bytes")
	}
	th := e.json(u, "GET", "/api/media/get-content?id="+id+"&variant=thumb", "").status(200, "")
	if cfg, _, err := image.DecodeConfig(th.Body); err != nil || cfg.Width != 64 {
		t.Fatalf("thumb: %v %v", cfg, err)
	}
	rr := httptest.NewRequest("GET", "/api/media/get-content?id="+id, nil)
	rr.Header.Set("Range", "bytes=0-9")
	part := e.send(u, rr).status(206, "")
	if part.Body.Len() != 10 || !strings.HasPrefix(part.Header().Get("Content-Range"), "bytes 0-9/") || !bytes.Equal(part.Body.Bytes(), stored[:10]) {
		t.Fatalf("range: %d bytes, %q", part.Body.Len(), part.Header().Get("Content-Range"))
	}
	e.json(u, "GET", "/api/media/get-content?id="+id+"&variant=huge", "").status(400, "ERROR_PARAM")

	// Another user sees nothing, and cannot delete.
	e.json(other, "GET", "/api/media/get-content?id="+id, "").status(404, "ERROR_NOT_FOUND")
	e.upload(other, book, "a.jpg", "image/jpeg", in).status(404, "ERROR_NOT_FOUND")
	e.deleteMedia(other, id).status(200, "")
	e.json(u, "GET", "/api/media/get-content?id="+id, "").status(200, "")
	e.json(user{}, "GET", "/api/media/get-content?id="+id, "").status(401, "")
	e.upload(user{}, book, "a.jpg", "image/jpeg", in).status(401, "")

	// Delete removes row and both objects; repeating it is fine.
	e.deleteMedia(u, id).status(200, "")
	e.deleteMedia(u, id).status(200, "")
	e.json(u, "GET", "/api/media/get-content?id="+id, "").status(404, "ERROR_NOT_FOUND")
	for _, k := range []string{key, tkey} {
		if _, err := e.st.Get(context.Background(), k); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("%s survived the delete: %v", k, err)
		}
	}
}

// get-content is binary on success and the v2 error envelope on failure; an <img> only needs the status, and an error must
// never be cached as if it were the image.
func TestContentErrorsAreEnvelopesAndNotCached(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book := e.newBook(u)
	id := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()

	for name, r := range map[string]resp{
		"unknown id":      e.json(u, "GET", "/api/media/get-content?id=01J9Z3K6V8Q4M7N2P5R8T0ZZZZ", "").status(404, "ERROR_NOT_FOUND"),
		"foreign id":      e.json(other, "GET", "/api/media/get-content?id="+id, "").status(404, "ERROR_NOT_FOUND"),
		"no session":      e.json(user{}, "GET", "/api/media/get-content?id="+id, "").status(401, "ERROR_UNAUTHORIZED"),
		"no id":           e.json(u, "GET", "/api/media/get-content", "").status(400, "ERROR_PARAM"),
		"unknown variant": e.json(u, "GET", "/api/media/get-content?id="+id+"&variant=huge", "").status(400, "ERROR_PARAM"),
	} {
		h := r.Header()
		if h.Get("Cache-Control") != "no-store" || !strings.HasPrefix(h.Get("Content-Type"), "application/json") || h.Get("ETag") != "" {
			t.Errorf("%s: an error must be uncacheable JSON, got %v", name, h)
		}
	}

	// HEAD and conditional requests work on the success path.
	head := e.send(u, httptest.NewRequest("HEAD", "/api/media/get-content?id="+id, nil)).status(200, "")
	if head.Body.Len() != 0 || head.Header().Get("Content-Length") == "" || head.Header().Get("ETag") == "" {
		t.Fatalf("HEAD: %d body bytes, headers %v", head.Body.Len(), head.Header())
	}
	cond := httptest.NewRequest("GET", "/api/media/get-content?id="+id, nil)
	cond.Header.Set("If-None-Match", head.Header().Get("ETag"))
	e.send(u, cond).status(304, "")
	// The old routes are gone.
	e.json(u, "GET", "/v1/media/"+id+"/content", "").status(404, "ERROR_NOT_FOUND")
	e.json(u, "DELETE", "/v1/media/"+id, "").status(404, "ERROR_NOT_FOUND")
	e.json(u, "GET", "/v1/yearbooks/"+book+"/media", "").status(404, "ERROR_NOT_FOUND")
	// Deleting needs a body with an id and the origin check.
	e.json(u, "POST", "/api/media/delete", `{}`).status(400, "ERROR_PARAM")
	e.json(u, "POST", "/api/media/delete", `{"id":"x","extra":1}`).status(400, "ERROR_PARAM")
	e.json(user{}, "POST", "/api/media/delete", `{"id":"`+id+`"}`).status(401, "ERROR_UNAUTHORIZED")
}

func TestUploadRejections(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	path := "/api/media/create"

	html := []byte("<html><script>alert(1)</script></html>")
	e.upload(u, book, "x.jpg", "image/jpeg", html).status(415, "ERROR_UNSUPPORTED_MEDIA_TYPE") // renamed HTML
	e.upload(u, book, "x.jpg", "image/jpeg", []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`)).status(415, "ERROR_UNSUPPORTED_MEDIA_TYPE")
	e.upload(u, book, "x.jpg", "image/jpeg", nil).status(415, "ERROR_UNSUPPORTED_MEDIA_TYPE") // 0 bytes
	e.upload(u, book, "x.exe", "application/octet-stream", append([]byte("MZ\x90\x00"), make([]byte, 600)...)).status(415, "ERROR_UNSUPPORTED_MEDIA_TYPE")
	e.upload(u, book, "x.jpg", "image/jpeg", pngHeaderClaiming(t, 60000, 60000)).status(400, "ERROR_INVALID_IMAGE")
	good := photo(t)
	e.upload(u, book, "x.jpg", "image/jpeg", good[:len(good)/2]).status(400, "ERROR_INVALID_IMAGE")

	big := bytes.Repeat([]byte{0xFF, 0xD8}, 11<<19) // 11 MiB
	e.upload(u, book, "big.jpg", "image/jpeg", big).status(413, "ERROR_TOO_LARGE")

	// Wrong request shapes.
	r := httptest.NewRequest("POST", path, strings.NewReader(`{"file":"x"}`))
	r.Header.Set("Content-Type", "application/json")
	e.send(u, r).status(415, "ERROR_UNSUPPORTED_MEDIA_TYPE")
	body, ct := multipartBody(book, "photo", "a.jpg", "image/jpeg", good) // wrong field name
	r = httptest.NewRequest("POST", path, body)
	r.Header.Set("Content-Type", ct)
	e.send(u, r).status(400, "ERROR_PARAM")
	body, ct = multipartBody("", "file", "a.jpg", "image/jpeg", good) // no yearbook_id
	r = httptest.NewRequest("POST", path, body)
	r.Header.Set("Content-Type", ct)
	e.send(u, r).status(400, "ERROR_PARAM")
	body, ct = multipartBody(book, "file", "a.jpg", "image/jpeg", good)
	r = httptest.NewRequest("POST", path, body)
	r.Header.Set("Content-Type", ct)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
	r.Header.Set("Origin", "https://evil.example.com")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("cross-origin upload: %d", rec.Code)
	}
	if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 {
		t.Fatalf("%d rows after only rejected uploads", n)
	}
}

func TestRouteDeadlinesAreExtended(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	id := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()

	body, ct := multipartBody(book, "file", "a.jpg", "image/jpeg", photo(t))
	up := httptest.NewRequest("POST", "/api/media/create", body)
	up.Header.Set("Content-Type", ct)
	up.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
	up.Header.Set("Origin", origin)
	dl := httptest.NewRequest("GET", "/api/media/get-content?id="+id, nil)
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
			_, err := e.db.Exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
				SELECT ?, id, 'owner', 'k', 't', 'image/jpeg', ?, 1, 1, '', 1, 1 FROM yearbook_tab WHERE public_id = ?`, fmt.Sprintf("FILL%022d", i+e.count(`SELECT COUNT(*) FROM media_tab`)), bytes, book)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// 197 rows, then 6 uploads, at most maxUploadsPerUser at a time (T-102: a third at once is 429): exactly three fit, the rest are
	// refused, and refused ones leave no objects.
	fill(197, 1)
	var wg sync.WaitGroup
	codes := make([]int, 6)
	turn := make(chan struct{}, maxUploadsPerUser)
	for i := range codes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			turn <- struct{}{}
			defer func() { <-turn }()
			codes[i] = e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).Code
		}()
	}
	wg.Wait()
	ok, full := 0, 0
	for _, c := range codes {
		switch c {
		case 200:
			ok++
		case 409:
			full++
		default:
			t.Fatalf("unexpected status in %v", codes)
		}
	}
	if ok != 3 || full != 3 {
		t.Fatalf("statuses %v, want 3x200 and 3x409", codes)
	}
	if n := e.st.count(yearbookPrefix(book)); n != 9 { // 3 photos x (display + thumb + print)
		t.Fatalf("%d objects, want 9", n)
	}
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(409, "ERROR_QUOTA_EXCEEDED")

	// Bytes per user, across books.
	book2 := e.newBook(u)
	if _, err := e.db.Exec(`UPDATE media_tab SET bytes = 500*1024*1024 - 1000 WHERE id = (SELECT id FROM (SELECT MIN(id) AS id FROM media_tab) x)`); err != nil {
		t.Fatal(err)
	}
	e.upload(u, book2, "a.jpg", "image/jpeg", photo(t)).status(409, "ERROR_QUOTA_EXCEEDED")
	if _, err := e.db.Exec(`UPDATE media_tab SET bytes = 1 WHERE bytes > 1000000`); err != nil {
		t.Fatal(err)
	}
	e.upload(u, book2, "a.jpg", "image/jpeg", photo(t)).status(200, "")
}

func TestRateLimit(t *testing.T) {
	e := newEnv(t)
	e.svc.limiter = e.lim.Open("media_upload", 3, time.Minute)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book, book2 := e.newBook(u), e.newBook(other)
	for range 3 {
		e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "")
	}
	r := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(429, "ERROR_RATE_LIMITED")
	if r.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.upload(other, book2, "a.jpg", "image/jpeg", photo(t)).status(200, "") // per user
}

func TestStorageFailureLeavesNoRow(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	e.st.set(true, false)
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(502, "ERROR_STORAGE_ERROR")
	e.st.set(false, false)
	if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 || e.st.count("") != 0 {
		t.Fatalf("%d rows and %d objects after a failed upload", n, e.st.count(""))
	}
	e.st.set(true, false)
	e.st.thumbOnly = true // the display object is written first, then the thumbnail fails: nothing may stay behind
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(502, "ERROR_STORAGE_ERROR")
	e.st.thumbOnly = false
	e.st.set(false, false)
	if n := e.st.count(""); n != 0 {
		t.Fatalf("%d orphan objects after a failed thumbnail write", n)
	}
	e.st.set(true, false)
	e.st.printOnly = true // display and thumbnail are written, then the print object fails
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(502, "ERROR_STORAGE_ERROR")
	e.st.printOnly = false
	e.st.set(false, false)
	if n := e.st.count(""); n != 0 {
		t.Fatalf("%d orphan objects after a failed print write", n)
	}
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "") // and a retry works
}

// T-064 AC5: deleting a photo that is the cover and the profile photo clears both references (no ON DELETE SET NULL any more).
func TestDeleteMediaClearsCoverAndPhoto(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	a := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()
	b := e.upload(u, book, "b.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, a)).status(200, "")
	e.updateProfile(u, book, fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, a)).status(200, "")
	e.deleteMedia(u, b).status(200, "") // another photo: the references stay
	got := e.getBook(u, book).status(200, "").book()
	if got["cover_media_id"] != a || got["profile"].(map[string]any)["photo_media_id"] != a {
		t.Fatalf("references lost by deleting another photo: %v", got)
	}
	e.deleteMedia(u, a).status(200, "")
	got = e.getBook(u, book).status(200, "").book()
	if got["cover_media_id"] != nil || got["profile"].(map[string]any)["photo_media_id"] != nil {
		t.Fatalf("dangling references after the photo was deleted: %v", got)
	}
	if n := e.count(`SELECT COUNT(*) FROM yearbook_tab WHERE cover_media_id IS NOT NULL`) + e.count(`SELECT COUNT(*) FROM profile_tab WHERE photo_media_id IS NOT NULL`); n != 0 {
		t.Fatalf("%d references left in the tables", n)
	}
}

func TestDeleteYearbookRemovesObjects(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	a := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()
	b := e.upload(u, book, "b.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, a)).status(200, "") // book -> media -> book reference cycle

	// A failing store blocks the delete and keeps everything.
	e.st.set(false, true)
	e.deleteBook(u, book).status(502, "ERROR_STORAGE_ERROR")
	e.st.set(false, false)
	e.getBook(u, book).status(200, "")
	e.json(u, "GET", "/api/media/get-content?id="+b, "").status(200, "")

	// Orphans that have no row (a crashed upload) go too.
	_ = e.st.Storage.Put(context.Background(), yearbookPrefix(book)+"ORPHAN.jpg", "image/jpeg", []byte("x"))
	e.deleteBook(u, book).status(200, "")
	if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 {
		t.Fatalf("%d media rows left", n)
	}
	for _, k := range []string{yearbookPrefix(book) + "ORPHAN.jpg", yearbookPrefix(book) + a + ".jpg", yearbookPrefix(book) + b + "-thumb.jpg"} {
		if _, err := e.st.Get(context.Background(), k); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("%s survived: %v", k, err)
		}
	}
	e.deleteBook(u, book).status(404, "ERROR_NOT_FOUND")
	if n := e.st.count(yearbookPrefix(book)); n != 0 {
		t.Fatalf("%d objects left in the bucket", n)
	}
}

func TestProfilePhotoAndCover(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book, book2, foreign := e.newBook(u), e.newBook(u), e.newBook(other)
	mine := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()
	inBook2 := e.upload(u, book2, "a.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()
	theirs := e.upload(other, foreign, "a.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()

	profile := func(media string) string {
		return fmt.Sprintf(`{"full_name":"Tester","photo_media_id":%s}`, media)
	}
	quote := func(s string) string { return `"` + s + `"` }

	b := e.updateProfile(u, book, profile(quote(mine))).status(200, "").book()
	if b["profile"].(map[string]any)["photo_media_id"] != mine {
		t.Fatalf("photo not set: %v", b["profile"])
	}
	b = e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, mine)).status(200, "").book()
	if b["cover_media_id"] != mine {
		t.Fatalf("cover not set: %v", b)
	}
	// Media of another book of the same user, another user's media, and unknown ids are all invalid_media.
	for _, bad := range []string{inBook2, theirs, "01ARZ3NDEKTSV4RRFFQ69G5FAV"} {
		e.updateProfile(u, book, profile(quote(bad))).status(400, "ERROR_INVALID_MEDIA")
		e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, bad)).status(400, "ERROR_INVALID_MEDIA")
	}
	// A title edit leaves the cover alone; null clears it; a missing photo_media_id in the (replacing) PUT clears the photo.
	b = e.updateBook(u, book, `{"title":"New"}`).status(200, "").book()
	if b["cover_media_id"] != mine {
		t.Fatalf("cover lost by an unrelated edit: %v", b)
	}
	b = e.updateBook(u, book, `{"cover_media_id":null}`).status(200, "").book()
	if b["cover_media_id"] != nil {
		t.Fatalf("cover not cleared: %v", b)
	}
	e.updateProfile(u, book, profile(quote(mine))).status(200, "")
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, mine)).status(200, "")
	// Deleting the media clears both references.
	e.deleteMedia(u, mine).status(200, "")
	b = e.getBook(u, book).status(200, "").book()
	if b["cover_media_id"] != nil || b["profile"].(map[string]any)["photo_media_id"] != nil {
		t.Fatalf("references survived the media delete: %v", b)
	}
	// A new book cannot start with a cover.
	e.json(u, "POST", "/api/yearbook/create", fmt.Sprintf(`{"title":"T","language":"en","cover_media_id":%q}`, mine)).status(400, "ERROR_INVALID_MEDIA")
}

// T-034: contributor uploads wait a bounded time for a processing slot, count toward the quotas and can be discarded.
func TestContributorUploadBusyAndDiscard(t *testing.T) {
	e := newEnv(t)
	u := e.register("owner@example.com")
	book := e.newBook(u)
	var ownerID, row uint64
	if err := e.db.QueryRow(`SELECT owner_id, id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&ownerID, &row); err != nil {
		t.Fatal(err)
	}
	data := photo(t)

	for range cap(e.svc.sem) { // every slot taken: the wait is bounded and ends in ErrBusy, with nothing stored
		e.svc.sem <- struct{}{}
	}
	loaded := false
	load := func() ([]byte, error) { loaded = true; return data, nil }
	// T-079: no wall-clock bound. The call returning ErrBusy while every slot is still held is the proof that the wait ended;
	// a hang is caught by the generous guard below instead of a tight elapsed-time check.
	res := make(chan error, 1)
	go func() {
		_, err := e.svc.save(context.Background(), ownerID, row, book, "contributor", load, 50*time.Millisecond)
		res <- err
	}()
	select {
	case err := <-res:
		if !errors.Is(err, ErrBusy) {
			t.Fatalf("err %v, want ErrBusy", err)
		}
	case <-time.After(time.Minute):
		t.Fatal("did not give up while every slot was held")
	}
	if loaded {
		t.Fatal("the photo was loaded into memory without a processing slot")
	}
	for range cap(e.svc.sem) {
		<-e.svc.sem
	}
	if e.count(`SELECT COUNT(*) FROM media_tab`) != 0 || e.st.count(yearbookPrefix(book)) != 0 {
		t.Fatal("a busy refusal stored something")
	}

	m, err := e.svc.UploadContributor(context.Background(), row, data)
	if err != nil || m.RowID == 0 {
		t.Fatalf("upload: %v %+v", err, m)
	}
	if e.count(`SELECT COUNT(*) FROM media_tab WHERE uploader_kind = 'contributor' AND yearbook_id = ?`, row) != 1 || e.st.count(yearbookPrefix(book)) != 3 {
		t.Fatal("contributor photo not stored as such")
	}
	if err := e.svc.Discard(*m); err != nil {
		t.Fatal(err)
	}
	if e.count(`SELECT COUNT(*) FROM media_tab`) != 0 || e.st.count(yearbookPrefix(book)) != 0 {
		t.Fatal("discard left a row or objects")
	}
	if _, err := e.svc.UploadContributor(context.Background(), row+1000, data); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown yearbook: %v", err)
	}
}
