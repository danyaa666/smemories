//go:build integration

package booklayout_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/bookdata"
	"github.com/danyaa666/smemories/internal/bookdesign"
	"github.com/danyaa666/smemories/internal/booklayout"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/legal"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis/redistest"
	"github.com/danyaa666/smemories/internal/storage"
	"github.com/danyaa666/smemories/internal/ulid"
	"github.com/danyaa666/smemories/internal/yearbook"
)

const (
	origin = "http://localhost:5173"
	testPW = "Tr0ub4dor&3-fake-test-password" // obviously fake
)

var t0 = time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)

type env struct {
	t       *testing.T
	db      *sql.DB
	h       http.Handler
	now     time.Time
	results sync.Map // metric result -> *atomic.Int64
}

func (e *env) counted(result string) int64 {
	if v, ok := e.results.Load(result); ok {
		return v.(*atomic.Int64).Load()
	}
	return 0
}

// newEnv wires the book stack as cmd/smemories-api does: the layout service with the real media service, the template-change hook
// and the layout as a child of the yearbook delete. perMinute 0 and limiters nil mean no rate limit.
func newEnv(t *testing.T, limiters *ratelimit.Factory, perMinute int) *env {
	t.Helper()
	d := dbtest.New(t)
	log := slog.New(slog.DiscardHandler)
	e := &env{t: t, db: d, now: t0}
	hasher := auth.NewHasher(auth.HashParams{MemoryKiB: 64, Time: 1, Parallelism: 1}, 4, auth.HashWait)
	mail, _ := mailer.NewLog("test", io.Discard)
	rc := redistest.New(t)
	codes, err := auth.NewCodes(context.Background(), rc, "test", []byte(strings.Repeat("k", auth.MinOTPKeyBytes)), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions(context.Background(), rc, log)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 1000},
		auth.Mail{Mailer: mail, Logger: log}, codes, ratelimit.NewFactory(rc, log, func() time.Time { return e.now }), func() time.Time { return e.now })
	if err != nil {
		t.Fatal(err)
	}
	ah := auth.NewHandler(svc, auth.HandlerConfig{AllowedOrigins: []string{origin}}, log)

	cursors := httpx.NewCursors([]byte(strings.Repeat("c", 32)))
	mediaSvc := media.NewService(media.NewStore(d), storage.NewMemory(), 2, ratelimit.NewMemoryFactory(time.Now), func() time.Time { return e.now })
	design := bookdesign.NewService(bookdesign.NewStore(d), nil, func() time.Time { return e.now })
	layout := booklayout.NewService(booklayout.NewStore(d), mediaSvc, limiters, perMinute, func() time.Time { return e.now })
	layout.SetCounter(func(r string) {
		v, _ := e.results.LoadOrStore(r, new(atomic.Int64))
		v.(*atomic.Int64).Add(1)
	})
	ystore := yearbook.NewStore(d)
	ystore.OnTemplateChange(design.DeleteByYearbook)
	ystore.OnTemplateChange(layout.DeleteByYearbook)
	ysvc := yearbook.NewService(ystore, cursors, mediaSvc, func() time.Time { return e.now }, mediaSvc, design, layout)
	nsvc := notes.NewService(notes.NewStore(d), mediaSvc, cursors, ratelimit.NewMemoryFactory(time.Now), log, nil)
	e.h = httpx.NewRouter(log, ah.Routes,
		yearbook.NewHandler(ysvc, ah.RequireUser, []string{origin}).Routes,
		booklayout.NewHandler(layout, ah.RequireUser, []string{origin}).Routes,
		bookdata.NewHandler(bookdata.NewService(ysvc, nsvc, design, layout), ah.RequireUser).Routes)
	return e
}

type user struct{ cookie string }

func (e *env) register(email string) user {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": testPW, "display_name": "Owner", "accept_terms_version": legal.CurrentVersion})
	rec := e.do("POST", "/api/auth/register", user{}, string(b))
	if rec.Code != 200 {
		e.t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	e.exec(`UPDATE user_tab SET email_verified_at = ? WHERE email = ?`, e.now.UnixMilli(), email)
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			return user{c.Value}
		}
	}
	e.t.Fatal("no session cookie")
	return user{}
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		e.t.Fatal(err)
	}
}

type resp struct {
	*httptest.ResponseRecorder
	t *testing.T
}

func (e *env) do(method, path string, u user, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", origin)
	if u.cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return rec
}

func (e *env) call(method, path string, u user, body string) resp {
	return resp{e.do(method, path, u, body), e.t}
}

func (r resp) status(code int, env string) resp {
	r.t.Helper()
	var b struct{ Status string }
	_ = json.Unmarshal(r.Body.Bytes(), &b)
	if r.Code != code || b.Status != env {
		r.t.Fatalf("got %d %q, want %d %q: %.300s", r.Code, b.Status, code, env, r.Body)
	}
	return r
}

func (r resp) data() map[string]any {
	var b struct{ Data map[string]any }
	if err := json.Unmarshal(r.Body.Bytes(), &b); err != nil {
		r.t.Fatal(err)
	}
	return b.Data
}

func (e *env) book(u user, template string) string {
	e.t.Helper()
	body := fmt.Sprintf(`{"title":"Book","language":"en","template_id":%q}`, template)
	return e.call("POST", "/api/yearbook/create", u, body).status(200, "OK").data()["id"].(string)
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// image makes a media row of the book with the purpose (design or photo) and returns its public id.
func (e *env) image(book, purpose string) string {
	e.t.Helper()
	id := ulid.New(e.now.Add(time.Duration(e.count(`SELECT COUNT(*) FROM media_tab`)+1) * time.Millisecond))
	e.exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, purpose, object_key, thumb_key, content_type, bytes, thumb_bytes, print_bytes, width, height, sha256, created_at, updated_at)
		SELECT ?, id, 'owner', ?, 'k', 't', 'image/jpeg', 1, 1, 1, 1, 1, '', 1, 1 FROM yearbook_tab WHERE public_id = ?`, id, purpose, book)
	return id
}

func save(book string, base int, layout string) string {
	return fmt.Sprintf(`{"yearbook_id":%q,"base_version":%d,"layout":%s}`, book, base, layout)
}

func get(book string) string { return "/api/book-layout/get?yearbook_id=" + book }

func reset(book string) string { return fmt.Sprintf(`{"yearbook_id":%q}`, book) }

const textItem = `"t":"text","text":"Lớp 12A1","font":"display","size":28,"color":"accent","align":"center","bold":true,"x":100,"w":400,"h":120,"cy":850,"r":-4`

func doc(items ...string) string {
	return `{"v":1,"pages":{"cover":{"overrides":{"cover.photo":{"x":120,"w":500,"h":500,"cy":400,"r":0,"hidden":false}},"items":[` + strings.Join(items, ",") + `]}}}`
}

func textAt(id string) string { return `{"id":"` + id + `",` + textItem + `}` }

func sameJSON(t *testing.T, got, want string) {
	t.Helper()
	var a, b any
	if json.Unmarshal([]byte(got), &a) != nil || json.Unmarshal([]byte(want), &b) != nil || fmt.Sprint(a) != fmt.Sprint(b) {
		t.Fatalf("documents differ:\n got %.400s\nwant %.400s", got, want)
	}
}

func raw(r resp, key string) string {
	var b struct{ Data map[string]json.RawMessage }
	_ = json.Unmarshal(r.Body.Bytes(), &b)
	return string(b.Data[key])
}

// AC1: a valid document saves, reads back identical, bumps the version; reset deletes it (idempotent) and the next save starts at 1.
func TestSaveReadResetAndVersions(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")

	g := e.call("GET", get(id), alice, "").status(200, "OK")
	d := g.data()
	if d["layout"] != nil || d["version"] != float64(0) || d["template_id"] != "memory-book" || d["updated_at"] != nil {
		t.Fatalf("fresh book: %v", d)
	}

	one := doc(textAt("a1b2c3"))
	s := e.call("POST", "/api/book-layout/save", alice, save(id, 0, one)).status(200, "OK").data()
	if s["version"] != float64(1) || s["updated_at"] != float64(t0.UnixMilli()) {
		t.Fatalf("first save: %v", s)
	}
	g = e.call("GET", get(id), alice, "").status(200, "OK")
	sameJSON(t, raw(g, "layout"), one)
	if d := g.data(); d["version"] != float64(1) || d["updated_at"] != float64(t0.UnixMilli()) {
		t.Fatalf("after save: %v", d)
	}

	e.now = e.now.Add(time.Minute)
	two := doc(textAt("a1b2c3"), textAt("d4e5f6"))
	if s := e.call("POST", "/api/book-layout/save", alice, save(id, 1, two)).status(200, "OK").data(); s["version"] != float64(2) || s["updated_at"] != float64(e.now.UnixMilli()) {
		t.Fatalf("second save: %v", s)
	}
	// the same base_version again: 409 with the stored document and version, nothing changed
	c := e.call("POST", "/api/book-layout/save", alice, save(id, 1, one)).status(409, "ERROR_VERSION_CONFLICT")
	if c.data()["version"] != float64(2) {
		t.Fatalf("conflict data: %v", c.data())
	}
	sameJSON(t, raw(c, "layout"), two)
	// base_version 0 when a row exists, and one from the future, are conflicts too
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, one)).status(409, "ERROR_VERSION_CONFLICT")
	e.call("POST", "/api/book-layout/save", alice, save(id, 9, one)).status(409, "ERROR_VERSION_CONFLICT")
	sameJSON(t, raw(e.call("GET", get(id), alice, ""), "layout"), two)
	if n := e.count(`SELECT COUNT(*) FROM book_layout_tab`); n != 1 {
		t.Fatalf("%d rows, want one per yearbook", n)
	}

	e.call("POST", "/api/book-layout/reset", alice, reset(id)).status(200, "OK")
	e.call("POST", "/api/book-layout/reset", alice, reset(id)).status(200, "OK") // idempotent
	// T-156: the version never goes back; each reset moves it on (2 -> 3 -> 4), the layout is null
	if d := e.call("GET", get(id), alice, "").status(200, "OK").data(); d["layout"] != nil || d["version"] != float64(4) {
		t.Fatalf("after reset: %v", d)
	}
	// a client that still holds version 2 finds the row gone: 409 with null and the current version
	c = e.call("POST", "/api/book-layout/save", alice, save(id, 2, one)).status(409, "ERROR_VERSION_CONFLICT")
	if c.data()["version"] != float64(4) || c.data()["layout"] != nil {
		t.Fatalf("conflict after reset: %v", c.data())
	}
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, one)).status(409, "ERROR_VERSION_CONFLICT")
	if s := e.call("POST", "/api/book-layout/save", alice, save(id, 4, one)).status(200, "OK").data(); s["version"] != float64(5) {
		t.Fatalf("save after reset: %v", s)
	}
	if e.counted("ok") != 3 || e.counted("conflict") != 5 {
		t.Errorf("counters ok=%d conflict=%d, want 3 and 5", e.counted("ok"), e.counted("conflict"))
	}
}

// AC1: two tabs saving from the same base_version at once: exactly one wins, the others get the winner's document.
func TestConcurrentSavesExactlyOneWins(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	for round, base := range []int{0, 1} {
		var wg sync.WaitGroup
		var wins, conflicts, other atomic.Int32
		for i := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec := e.do("POST", "/api/book-layout/save", alice, save(id, base, doc(textAt(fmt.Sprintf("r%dt%03d", round, i)))))
				switch rec.Code {
				case 200:
					wins.Add(1)
				case 409:
					conflicts.Add(1)
				default:
					other.Add(1)
					t.Errorf("status %d: %s", rec.Code, rec.Body)
				}
			}()
		}
		wg.Wait()
		if wins.Load() != 1 || conflicts.Load() != 7 || other.Load() != 0 {
			t.Fatalf("base %d: %d wins, %d conflicts, %d other", base, wins.Load(), conflicts.Load(), other.Load())
		}
	}
	if v := e.call("GET", get(id), alice, "").data()["version"]; v != float64(2) {
		t.Fatalf("version %v, want 2", v)
	}
}

// AC2: invalid documents are a 400 with the problem list (path and code), nothing is stored; an unknown top-level field of the request too.
func TestInvalidDocuments(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	bad := `{"v":1,"pages":{"cover":{"items":[` +
		`{"id":"abcdef","t":"asset","asset":"sticker.gold-star","tint":"accent","x":0,"w":100,"h":100,"cy":500},` +
		`{"id":"abcdef","t":"asset","asset":"sticker.nope","x":0,"w":100,"h":100,"cy":500}]}}}`
	r := e.call("POST", "/api/book-layout/save", alice, save(id, 0, bad)).status(400, "ERROR_INVALID_LAYOUT")
	var out struct {
		Data struct{ Problems []booklayout.Problem }
	}
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	want := map[booklayout.Problem]bool{
		{Path: "pages.cover.items[0].tint", Code: "not_tintable"}: true, {Path: "pages.cover.items[1].id", Code: "duplicate"}: true,
		{Path: "pages.cover.items[1].asset", Code: "unknown_id"}: true,
	}
	for _, p := range out.Data.Problems {
		delete(want, p)
	}
	if len(want) != 0 {
		t.Fatalf("missing problems %v in %v", want, out.Data.Problems)
	}
	if e.count(`SELECT COUNT(*) FROM book_layout_tab`) != 0 {
		t.Fatal("an invalid document was stored")
	}
	for name, body := range map[string]string{
		"not json":                   save(id, 0, `{"v":`),
		"layout null":                save(id, 0, `null`),
		"unknown request field":      fmt.Sprintf(`{"yearbook_id":%q,"base_version":0,"layout":%s,"x":1}`, id, doc()),
		"no base_version":            fmt.Sprintf(`{"yearbook_id":%q,"layout":%s}`, id, doc()),
		"negative base_version":      save(id, -1, doc()),
		"base_version as a string":   fmt.Sprintf(`{"yearbook_id":%q,"base_version":"0","layout":%s}`, id, doc()),
		"no layout":                  fmt.Sprintf(`{"yearbook_id":%q,"base_version":0}`, id),
		"bad yearbook id":            save("nope", 0, doc()),
		"lower-case yearbook id":     save(strings.ToLower(id), 0, doc()),
		"trailing data after layout": save(id, 0, doc()) + `{}`,
	} {
		rec := e.do("POST", "/api/book-layout/save", alice, body)
		var b struct{ Status string }
		_ = json.Unmarshal(rec.Body.Bytes(), &b)
		if rec.Code != 400 || (b.Status != "ERROR_PARAM" && b.Status != "ERROR_INVALID_LAYOUT") {
			t.Errorf("%s: %d %.200s", name, rec.Code, rec.Body)
		}
	}
	// a template without a layout block cannot hold one
	nid := e.book(alice, "navy-classic")
	e.call("POST", "/api/book-layout/save", alice, save(nid, 0, doc())).status(400, "ERROR_INVALID_LAYOUT")
	cid := e.book(alice, "classic")
	e.call("POST", "/api/book-layout/save", alice, save(cid, 0, doc())).status(400, "ERROR_INVALID_LAYOUT")
	if e.counted("invalid") < 2 {
		t.Errorf("invalid counter %d", e.counted("invalid"))
	}
}

// Req 9 / QA 2026-10-31: every media id is checked with DesignImages: a profile photo, another book's or user's design image and an
// unknown id are refused with a precise path, the owner's own design image is accepted (as an image item and as a background).
func TestMediaIdsMustBeOwnDesignImages(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	a1, a2, b1 := e.book(alice, "memory-book"), e.book(alice, "memory-book"), e.book(bob, "memory-book")
	own, photo, otherBook, bobs := e.image(a1, "design"), e.image(a1, "photo"), e.image(a2, "design"), e.image(b1, "design")
	unknown := ulid.New(t0.Add(time.Hour))
	img := func(id, media string) string {
		return `{"id":"` + id + `","t":"image","media":"` + media + `","fit":"cover","x":0,"w":300,"h":200,"cy":500,"r":0}`
	}
	for name, media := range map[string]string{"profile or note photo": photo, "design image of another book": otherBook, "design image of another user": bobs, "unknown id": unknown, "lower-case id": strings.ToLower(own)} {
		r := e.call("POST", "/api/book-layout/save", alice, save(a1, 0, doc(img("abcdef", media)))).status(400, "ERROR_INVALID_LAYOUT")
		if !strings.Contains(r.Body.String(), `"path":"pages.cover.items[0].media"`) || !strings.Contains(r.Body.String(), "not_own_media") {
			t.Errorf("%s: %s", name, r.Body)
		}
		bg := `{"v":1,"pages":{"back":{"bg":{"media":"` + media + `"}}}}`
		r = e.call("POST", "/api/book-layout/save", alice, save(a1, 0, bg)).status(400, "ERROR_INVALID_LAYOUT")
		if !strings.Contains(r.Body.String(), `"path":"pages.back.bg.media"`) {
			t.Errorf("%s as background: %s", name, r.Body)
		}
	}
	if e.count(`SELECT COUNT(*) FROM book_layout_tab`) != 0 {
		t.Fatal("a refused document was stored")
	}
	good := `{"v":1,"pages":{"back":{"bg":{"media":"` + own + `"}},"cover":{"items":[` + img("abcdef", own) + `]}}}`
	e.call("POST", "/api/book-layout/save", alice, save(a1, 0, good)).status(200, "OK")
	// bob cannot save alice's book (404, before anything else), and his own design image is not alice's
	e.call("POST", "/api/book-layout/save", bob, save(a1, 0, good)).status(404, "ERROR_NOT_FOUND")
}

// AC2: 128 KiB passes (padded with whitespace, which the request keeps), 128 KiB + 1 byte is 413 before the document is read, a body far
// over the cap is 413 too, and nothing is stored for the refusals.
func TestSizeLimit(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	small := doc(textAt("a1b2c3"))
	pad := func(n int) string { return "{" + strings.Repeat(" ", n-len(small)) + small[1:] } // n bytes, the blanks inside the document
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, pad(booklayout.MaxBytes+1))).status(413, "ERROR_TOO_LARGE")
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, pad(booklayout.MaxBytes+4096))).status(413, "ERROR_TOO_LARGE")
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, pad(300<<10))).status(413, "ERROR_TOO_LARGE")
	if e.count(`SELECT COUNT(*) FROM book_layout_tab`) != 0 {
		t.Fatal("an oversize document was stored")
	}
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, pad(booklayout.MaxBytes))).status(200, "OK")
	if e.counted("too_large") != 3 {
		t.Errorf("too_large counter %d, want 3", e.counted("too_large"))
	}
}

// AC3: another user gets 404 on all three endpoints (also with an invalid document), an anonymous caller is refused before any lookup;
// the stored layout is untouched by the refused calls.
func TestOwnerOnly(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	id := e.book(alice, "memory-book")
	one := doc(textAt("a1b2c3"))
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, one)).status(200, "OK")
	unknown := "01K7QB1B2C3D4E5F6G7H8J9K0M"
	for name, c := range map[string]struct {
		method, path, body string
		who                user
		code               int
		status             string
	}{
		"get: other user":           {"GET", get(id), "", bob, 404, "ERROR_NOT_FOUND"},
		"get: anonymous":            {"GET", get(id), "", user{}, 401, "ERROR_UNAUTHORIZED"},
		"get: unknown book":         {"GET", get(unknown), "", alice, 404, "ERROR_NOT_FOUND"},
		"get: no id":                {"GET", "/api/book-layout/get", "", alice, 400, "ERROR_PARAM"},
		"save: other user":          {"POST", "/api/book-layout/save", save(id, 1, doc()), bob, 404, "ERROR_NOT_FOUND"},
		"save: other user, invalid": {"POST", "/api/book-layout/save", save(id, 1, `{"v":9}`), bob, 404, "ERROR_NOT_FOUND"},
		"save: anonymous":           {"POST", "/api/book-layout/save", save(id, 1, doc()), user{}, 401, "ERROR_UNAUTHORIZED"},
		"save: unknown book":        {"POST", "/api/book-layout/save", save(unknown, 0, doc()), alice, 404, "ERROR_NOT_FOUND"},
		"reset: other user":         {"POST", "/api/book-layout/reset", reset(id), bob, 404, "ERROR_NOT_FOUND"},
		"reset: anonymous":          {"POST", "/api/book-layout/reset", reset(id), user{}, 401, "ERROR_UNAUTHORIZED"},
		"reset: unknown book":       {"POST", "/api/book-layout/reset", reset(unknown), alice, 404, "ERROR_NOT_FOUND"},
		"reset: bad id":             {"POST", "/api/book-layout/reset", reset("nope"), alice, 400, "ERROR_PARAM"},
	} {
		rec := e.call(c.method, c.path, c.who, c.body)
		if rec.Code != c.code {
			t.Errorf("%s: %d %.200s", name, rec.Code, rec.Body)
			continue
		}
		rec.status(c.code, c.status)
		if strings.Contains(rec.Body.String(), "Lớp") {
			t.Errorf("%s leaks the document", name)
		}
	}
	// a request from a browser page of another origin carries no session proof: the CSRF guard refuses it
	r := httptest.NewRequest("POST", "/api/book-layout/save", strings.NewReader(save(id, 1, doc())))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Origin", "https://evil.example")
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: alice.cookie})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Errorf("foreign origin: %d %s", rec.Code, rec.Body)
	}
	sameJSON(t, raw(e.call("GET", get(id), alice, ""), "layout"), one)
}

// AC4: changing the template deletes the layout and keeps answers, photos and design images; a delete removes the row; no orphan remains.
func TestTemplateChangeAndYearbookDelete(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	img := e.image(id, "design")
	photo := e.image(id, "photo")
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, doc(textAt("a1b2c3")))).status(200, "OK")
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"title":"Same template","template_id":"memory-book"}`, id)).status(200, "OK")
	if e.count(`SELECT COUNT(*) FROM book_layout_tab`) != 1 {
		t.Fatal("an update that keeps the template deleted the layout")
	}
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"navy-classic"}`, id)).status(200, "OK")
	if d := e.call("GET", get(id), alice, "").status(200, "OK").data(); d["layout"] != nil || d["version"] != float64(2) || d["template_id"] != "navy-classic" {
		t.Fatalf("after the template change: %v", d)
	}
	if e.count(`SELECT COUNT(*) FROM book_layout_tab`) != 0 || e.count(`SELECT COUNT(*) FROM media_tab WHERE public_id IN (?, ?)`, img, photo) != 2 ||
		e.count(`SELECT COUNT(*) FROM profile_tab`) != 1 {
		t.Fatal("the template change deleted the layout, or something else")
	}
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"memory-book"}`, id)).status(200, "OK")
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, doc(textAt("a1b2c3")))).status(409, "ERROR_VERSION_CONFLICT") // not the pre-change base
	e.call("POST", "/api/book-layout/save", alice, save(id, 3, doc(textAt("a1b2c3")))).status(200, "OK")
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":null}`, id)).status(200, "OK")
	if e.count(`SELECT COUNT(*) FROM book_layout_tab`) != 0 {
		t.Fatal("removing the template kept the layout")
	}
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"memory-book"}`, id)).status(200, "OK")
	e.call("POST", "/api/book-layout/save", alice, save(id, 6, doc(textAt("a1b2c3")))).status(200, "OK")
	e.call("POST", "/api/yearbook/delete", alice, fmt.Sprintf(`{"id":%q}`, id)).status(200, "OK")
	if e.count(`SELECT COUNT(*) FROM book_layout_tab`) != 0 {
		t.Fatal("the yearbook delete kept the layout")
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// Lock order: parallel saves, resets, template changes and a delete end without a 5xx (no MySQL deadlock).
func TestParallelWritesDoNotDeadlock(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	var wg sync.WaitGroup
	var bad atomic.Int32
	run := func(method, path, body string) {
		defer wg.Done()
		if rec := e.do(method, path, alice, body); rec.Code >= 500 {
			bad.Add(1)
			t.Errorf("%s %s: %d %.200s", method, path, rec.Code, rec.Body)
		}
	}
	for i := range 8 {
		tpl := []string{"memory-book", "navy-classic"}[i%2]
		wg.Add(5)
		go run("POST", "/api/book-layout/save", save(id, i%3, doc(textAt("a1b2c3"))))
		go run("POST", "/api/book-layout/reset", reset(id))
		go run("POST", "/api/yearbook/update", fmt.Sprintf(`{"id":%q,"template_id":%q}`, id, tpl))
		go run("POST", "/api/yearbook/update", fmt.Sprintf(`{"id":%q,"title":"T%d"}`, id, i))
		go run("GET", "/api/book-data/get?yearbook_id="+id, "")
	}
	wg.Wait()
	wg.Add(3)
	go run("POST", "/api/yearbook/delete", fmt.Sprintf(`{"id":%q}`, id))
	go run("POST", "/api/book-layout/save", save(id, 0, doc()))
	go run("POST", "/api/book-layout/reset", reset(id))
	wg.Wait()
	if e.count(`SELECT COUNT(*) FROM book_layout_tab`) != 0 {
		t.Error("a layout row is left for a deleted book")
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// Req 3: saves are limited per user: the burst of 20 and then the per-minute number; reset and get are not; Retry-After is set; another user is free.
func TestSaveRateLimit(t *testing.T) {
	now := func() time.Time { return t0 }
	e := newEnv(t, ratelimit.NewMemoryFactory(now), 25)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	id, bid := e.book(alice, "memory-book"), e.book(bob, "memory-book")
	for i := range booklayout.Burst {
		e.call("POST", "/api/book-layout/save", alice, save(id, i, doc(textAt("a1b2c3")))).status(200, "OK")
	}
	r := e.call("POST", "/api/book-layout/save", alice, save(id, booklayout.Burst, doc())).status(429, "ERROR_RATE_LIMITED")
	if r.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	e.call("GET", get(id), alice, "").status(200, "OK")
	e.call("POST", "/api/book-layout/reset", alice, reset(id)).status(200, "OK")
	e.call("POST", "/api/book-layout/save", bob, save(bid, 0, doc())).status(200, "OK")
	if e.counted("rate_limited") != 1 {
		t.Errorf("rate_limited counter %d", e.counted("rate_limited"))
	}
}

// Req 3: the per-minute limit applies when the burst window has moved on (the memory limiters follow the clock).
func TestSaveRateLimitPerMinute(t *testing.T) {
	clock := t0
	e := newEnv(t, ratelimit.NewMemoryFactory(func() time.Time { return clock }), 25)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	base := 0
	saveOK := func() {
		e.call("POST", "/api/book-layout/save", alice, save(id, base, doc())).status(200, "OK")
		base++
	}
	for range 20 { // the burst
		saveOK()
	}
	clock = clock.Add(11 * time.Second)
	for range 5 { // 25 in the minute
		saveOK()
	}
	clock = clock.Add(11 * time.Second)
	e.call("POST", "/api/book-layout/save", alice, save(id, base, doc())).status(429, "ERROR_RATE_LIMITED")
	clock = clock.Add(time.Minute)
	saveOK()
}

// AC5: book-data/get carries the layout and its version (null and 0 without one), drops what no longer validates, ignores a row of
// another template and a garbage row, and stays a 404 for another user.
func TestBookDataCarriesLayout(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	id := e.book(alice, "memory-book")
	bd := func() map[string]any {
		return e.call("GET", "/api/book-data/get?yearbook_id="+id, alice, "").status(200, "OK").data()
	}
	d := bd()
	if v, ok := d["layout"]; !ok || v != nil || d["layout_version"] != float64(0) {
		t.Fatalf("no layout: %v %v", d["layout"], d["layout_version"])
	}
	img := e.image(id, "design")
	full := `{"v":1,"pages":{"cover":{"bg":{"media":"` + img + `"},"items":[` + textAt("a1b2c3") +
		`,{"id":"imgone","t":"image","media":"` + img + `","x":0,"w":300,"h":200,"cy":500},` +
		`{"id":"astone","t":"asset","asset":"shape.star","tint":"accent","x":0,"w":300,"h":200,"cy":500}]}}}`
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, full)).status(200, "OK")
	r := e.call("GET", "/api/book-data/get?yearbook_id="+id, alice, "").status(200, "OK")
	sameJSON(t, raw(r, "layout"), full)
	if raw(r, "layout_version") != "1" {
		t.Fatalf("layout_version %s", raw(r, "layout_version"))
	}
	e.call("GET", "/api/book-data/get?yearbook_id="+id, bob, "").status(404, "ERROR_NOT_FOUND")

	// the design image is deleted: the background and the image item go, the rest stays, and the page still loads
	e.exec(`DELETE FROM media_tab WHERE public_id = ?`, img)
	r = e.call("GET", "/api/book-data/get?yearbook_id="+id, alice, "").status(200, "OK")
	var got struct {
		Pages map[string]struct {
			Bg    any
			Items []struct{ ID string }
		}
	}
	_ = json.Unmarshal([]byte(raw(r, "layout")), &got)
	if c := got.Pages["cover"]; c.Bg != nil || len(c.Items) != 2 || c.Items[0].ID != "a1b2c3" || c.Items[1].ID != "astone" {
		t.Fatalf("after the image was deleted: %s", raw(r, "layout"))
	}
	if raw(r, "layout_version") != "1" {
		t.Errorf("layout_version %s", raw(r, "layout_version"))
	}
	// the stored document is untouched by the read
	if d := e.call("GET", get(id), alice, "").data(); !strings.Contains(raw(e.call("GET", get(id), alice, ""), "layout"), img) || d["version"] != float64(1) {
		t.Error("the read changed the stored layout")
	}
	// garbage and another version are read as no layout, not as a failure
	for _, stored := range []string{`{not json`, `{"v":2,"pages":{}}`} {
		e.exec(`UPDATE book_layout_tab SET layout_json = ?`, stored)
		if d := bd(); d["layout"] != nil || d["layout_version"] != float64(0) {
			t.Errorf("stored %s: %v %v", stored, d["layout"], d["layout_version"])
		}
	}
	// a row made for another template is ignored
	e.exec(`UPDATE book_layout_tab SET layout_json = ?, template_id = 'navy-classic'`, full)
	if d := bd(); d["layout"] != nil {
		t.Errorf("row of another template: %v", d["layout"])
	}
}

// AC5: a 128 KiB layout rides along in book-data/get; the answer is its size plus the rest of the book, well under the existing bound.
func TestBookDataWith128KiBLayout(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	// 40 + 40 + 40 + 30 items of 500 three-byte Vietnamese letters each is 150 items: about 230 KB, over the limit, so use fewer letters.
	var cover []string
	per := (booklayout.MaxBytes - 4096) / 150
	letters := (per - 200) / 3
	for i := range 40 {
		cover = append(cover, `{"id":"`+fmt.Sprintf("c%05d", i)+`","t":"text","text":"`+strings.Repeat("ệ", letters)+`","x":0,"w":300,"h":200,"cy":500}`)
	}
	pages := `"cover":{"items":[` + strings.Join(cover, ",") + `]}`
	for _, k := range []string{"profile", "notes", "back"} {
		var items []string
		for i := range 40 {
			items = append(items, `{"id":"`+fmt.Sprintf("%s%05d", k[:1], i)+`","t":"text","text":"`+strings.Repeat("ệ", letters)+`","x":0,"w":300,"h":200,"cy":500}`)
		}
		pages += `,"` + k + `":{"items":[` + strings.Join(items, ",") + `]}`
	}
	// 160 items is over the 150 limit: drop 10 from the back page
	big := `{"v":1,"pages":{` + pages + `}}`
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, big)).status(400, "ERROR_INVALID_LAYOUT")
	var cut []string
	for i := range 30 {
		cut = append(cut, `{"id":"`+fmt.Sprintf("b%05d", i)+`","t":"text","text":"`+strings.Repeat("ệ", letters)+`","x":0,"w":300,"h":200,"cy":500}`)
	}
	pages = pages[:strings.LastIndex(pages, `,"back"`)] + `,"back":{"items":[` + strings.Join(cut, ",") + `]}`
	big = `{"v":1,"pages":{` + pages + `}}`
	if len(big) < 100<<10 || len(big) > booklayout.MaxBytes {
		t.Fatalf("test document is %d bytes", len(big))
	}
	big = "{" + strings.Repeat(" ", booklayout.MaxBytes-len(big)) + big[1:] // exactly 128 KiB as sent
	e.call("POST", "/api/book-layout/save", alice, save(id, 0, big)).status(200, "OK")
	rec := e.call("GET", "/api/book-data/get?yearbook_id="+id, alice, "").status(200, "OK")
	if rec.Body.Len() < 100<<10 || rec.Body.Len() > booklayout.MaxBytes+8<<10 {
		t.Fatalf("book-data/get is %d bytes", rec.Body.Len())
	}
	var got struct {
		Data struct{ Layout json.RawMessage }
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &got)
	if len(got.Data.Layout) < 100<<10 || len(got.Data.Layout) > booklayout.MaxBytes {
		t.Fatalf("layout is %d bytes", len(got.Data.Layout))
	}
}

// T-156 AC1..AC3: the version of a book only grows. A stale editor cannot win after a reset or a template change, however many saves
// brought the stored version back to its number; the sequence save, reset, save, template change, save is strictly increasing.
func TestVersionNeverGoesBack(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	doc1 := doc(textAt("a1b2c3"))
	saveAt := func(base int64) resp {
		return e.call("POST", "/api/book-layout/save", alice, save(id, int(base), doc1))
	}
	version := func() float64 { return e.call("GET", get(id), alice, "").status(200, "OK").data()["version"].(float64) }

	var seen []float64
	note := func(v float64) { seen = append(seen, v) }
	for i := range 3 { // editor X reads version 3
		saveAt(int64(i)).status(200, "OK")
	}
	note(version())
	e.call("POST", "/api/book-layout/reset", alice, reset(id)).status(200, "OK")
	note(version())
	base := int64(version())
	for range 3 { // another tab saves three times
		s := saveAt(base).status(200, "OK").data()["version"].(float64)
		base = int64(s)
	}
	note(version())
	// AC1: X saves with base 3 and loses
	c := saveAt(3).status(409, "ERROR_VERSION_CONFLICT").data()
	if c["version"] != float64(base) || c["layout"] == nil {
		t.Fatalf("conflict data %v", c)
	}
	// AC2: the same with a template change instead of a reset
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"navy-classic"}`, id)).status(200, "OK")
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"memory-book"}`, id)).status(200, "OK")
	note(version())
	for _, stale := range []int64{3, base} {
		if c := saveAt(stale).status(409, "ERROR_VERSION_CONFLICT").data(); c["layout"] != nil || c["version"] != float64(version()) {
			t.Fatalf("stale %d after template change: %v", stale, c)
		}
	}
	base = int64(version())
	note(float64(saveAt(base).status(200, "OK").data()["version"].(float64)))
	// AC3: strictly increasing
	for i := 1; i < len(seen); i++ {
		if seen[i] <= seen[i-1] {
			t.Fatalf("versions %v are not strictly increasing", seen)
		}
	}
	// a fresh book still starts at 1
	other := e.book(alice, "memory-book")
	e.call("POST", "/api/book-layout/save", alice, save(other, 0, doc1)).status(200, "OK")
	if v := e.call("GET", get(other), alice, "").data()["version"]; v != float64(1) {
		t.Fatalf("fresh book at version %v", v)
	}
	dbtest.AssertNoOrphans(t, e.db)
}
