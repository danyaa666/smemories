//go:build integration

package bookdesign_test

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
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/bookdata"
	"github.com/danyaa666/smemories/internal/bookdesign"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/legal"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis/redistest"
	"github.com/danyaa666/smemories/internal/yearbook"
)

const writesPerWindow = 60 // the limit of the service

const (
	origin = "http://localhost:5173"
	testPW = "Tr0ub4dor&3-fake-test-password" // obviously fake
)

type env struct {
	t   *testing.T
	db  *sql.DB
	h   http.Handler
	now time.Time
}

func newEnv(t *testing.T, limiters *ratelimit.Factory) *env {
	t.Helper()
	d := dbtest.New(t)
	log := slog.New(slog.DiscardHandler)
	e := &env{t: t, db: d, now: time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)}
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

	design := bookdesign.NewService(bookdesign.NewStore(d), limiters, func() time.Time { return e.now })
	ystore := yearbook.NewStore(d)
	ystore.OnTemplateChange(design.DeleteByYearbook)
	ysvc := yearbook.NewService(ystore, httpx.NewCursors([]byte(strings.Repeat("c", 32))), nil, func() time.Time { return e.now }, design)
	nsvc := notes.NewService(notes.NewStore(d), nil, nil, ratelimit.NewMemoryFactory(time.Now), log, nil)
	e.h = httpx.NewRouter(log, ah.Routes,
		yearbook.NewHandler(ysvc, ah.RequireUser, []string{origin}).Routes,
		bookdesign.NewHandler(design, ah.RequireUser, []string{origin}).Routes,
		bookdata.NewHandler(bookdata.NewService(ysvc, nsvc, design), ah.RequireUser).Routes)
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
	if _, err := e.db.Exec(`UPDATE user_tab SET email_verified_at = ? WHERE email = ?`, e.now.UnixMilli(), email); err != nil {
		e.t.Fatal(err)
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

// status checks the HTTP status and the envelope status.
func (r resp) status(code int, env string) resp {
	r.t.Helper()
	var b struct{ Status string }
	_ = json.Unmarshal(r.Body.Bytes(), &b)
	if r.Code != code || b.Status != env {
		r.t.Fatalf("got %d %q, want %d %q: %s", r.Code, b.Status, code, env, r.Body)
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

func update(id, options string) string {
	return fmt.Sprintf(`{"yearbook_id":%q,"options":%s}`, id, options)
}

const full = `{"v":1,"theme":"mint","fonts":"classic","sections":{"order":["cover","notes","profile","back"],"hidden":["profile"]},"texts":{"profile.heading":"Về mình"},"images":{"cover.pattern":"plain"}}`

// AC2: the owner reads (defaults first), writes, rereads and resets; the design block rides along on every answer.
func TestOwnerReadsWritesResets(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")

	d := e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data()
	if d["template_id"] != "memory-book" || d["options"] != nil || d["updated_at"] != nil || d["design"] == nil {
		t.Fatalf("fresh book: %v", d)
	}
	if th := d["design"].(map[string]any)["themes"].([]any); len(th) != 3 || th[0] != "lavender" {
		t.Errorf("themes %v", th)
	}

	u := e.call("POST", "/api/book-design/update", alice, update(id, full)).status(200, "OK").data()
	if u["updated_at"] != float64(e.now.UnixMilli()) || u["options"].(map[string]any)["theme"] != "mint" {
		t.Fatalf("update answer: %v", u)
	}
	d = e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data()
	if d["options"].(map[string]any)["fonts"] != "classic" || d["updated_at"] == nil {
		t.Fatalf("after update: %v", d)
	}

	// Full replace: a smaller document drops what the first one had.
	e.now = e.now.Add(time.Minute)
	e.call("POST", "/api/book-design/update", alice, update(id, `{"theme":"peach"}`)).status(200, "OK")
	d = e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data()
	if o := d["options"].(map[string]any); len(o) != 2 || o["theme"] != "peach" || d["updated_at"] != float64(e.now.UnixMilli()) {
		t.Fatalf("replace: %v", d)
	}
	if n := e.count(`SELECT COUNT(*) FROM book_design_tab`); n != 1 {
		t.Fatalf("%d rows, want one per yearbook", n)
	}

	e.call("POST", "/api/book-design/reset", alice, fmt.Sprintf(`{"yearbook_id":%q}`, id)).status(200, "OK")
	e.call("POST", "/api/book-design/reset", alice, fmt.Sprintf(`{"yearbook_id":%q}`, id)).status(200, "OK") // idempotent
	d = e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data()
	if d["options"] != nil || e.count(`SELECT COUNT(*) FROM book_design_tab`) != 0 {
		t.Fatalf("after reset: %v", d)
	}
}

// AC2: another user, anonymous and unknown books; malformed ids and bodies.
func TestEndpointMatrix(t *testing.T) {
	e := newEnv(t, nil)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	id := e.book(alice, "memory-book")
	e.call("POST", "/api/book-design/update", alice, update(id, `{"theme":"mint"}`)).status(200, "OK")
	unknown := "01K7QB1B2C3D4E5F6G7H8J9K0M"
	for _, c := range []struct {
		name, method, path, body string
		who                      user
		code                     int
		status                   string
	}{
		{"get: other user", "GET", "/api/book-design/get?yearbook_id=" + id, "", bob, 404, "ERROR_NOT_FOUND"},
		{"get: anonymous", "GET", "/api/book-design/get?yearbook_id=" + id, "", user{}, 401, "ERROR_UNAUTHORIZED"},
		{"get: unknown book", "GET", "/api/book-design/get?yearbook_id=" + unknown, "", alice, 404, "ERROR_NOT_FOUND"},
		{"get: lower-case id", "GET", "/api/book-design/get?yearbook_id=" + strings.ToLower(id), "", alice, 400, "ERROR_PARAM"},
		{"get: no id", "GET", "/api/book-design/get", "", alice, 400, "ERROR_PARAM"},
		{"update: other user", "POST", "/api/book-design/update", update(id, `{"theme":"peach"}`), bob, 404, "ERROR_NOT_FOUND"},
		{"update: anonymous", "POST", "/api/book-design/update", update(id, `{"theme":"peach"}`), user{}, 401, "ERROR_UNAUTHORIZED"},
		{"update: unknown book", "POST", "/api/book-design/update", update(unknown, `{"theme":"peach"}`), alice, 404, "ERROR_NOT_FOUND"},
		{"update: other user with an invalid document is still 404", "POST", "/api/book-design/update", update(id, `{"theme":"zzz"}`), bob, 404, "ERROR_NOT_FOUND"},
		{"update: bad id", "POST", "/api/book-design/update", update("nope", `{}`), alice, 400, "ERROR_PARAM"},
		{"update: no options", "POST", "/api/book-design/update", fmt.Sprintf(`{"yearbook_id":%q}`, id), alice, 400, "ERROR_PARAM"},
		{"update: options null", "POST", "/api/book-design/update", update(id, `null`), alice, 400, "ERROR_INVALID_DESIGN"},
		{"update: options is a string", "POST", "/api/book-design/update", update(id, `"theme"`), alice, 400, "ERROR_INVALID_DESIGN"},
		{"update: unknown body field", "POST", "/api/book-design/update", fmt.Sprintf(`{"yearbook_id":%q,"options":{},"x":1}`, id), alice, 400, "ERROR_PARAM"},
		{"update: not json", "POST", "/api/book-design/update", `nope`, alice, 400, "ERROR_PARAM"},
		{"update: invalid theme", "POST", "/api/book-design/update", update(id, `{"theme":"zzz"}`), alice, 400, "ERROR_INVALID_DESIGN"},
		{"reset: other user", "POST", "/api/book-design/reset", fmt.Sprintf(`{"yearbook_id":%q}`, id), bob, 404, "ERROR_NOT_FOUND"},
		{"reset: anonymous", "POST", "/api/book-design/reset", fmt.Sprintf(`{"yearbook_id":%q}`, id), user{}, 401, "ERROR_UNAUTHORIZED"},
		{"reset: unknown book", "POST", "/api/book-design/reset", fmt.Sprintf(`{"yearbook_id":%q}`, unknown), alice, 404, "ERROR_NOT_FOUND"},
		{"reset: bad id", "POST", "/api/book-design/reset", `{"yearbook_id":"x"}`, alice, 400, "ERROR_PARAM"},
	} {
		t.Run(c.name, func(t *testing.T) { e.call(c.method, c.path, c.who, c.body).status(c.code, c.status) })
	}
	// None of the refused calls touched Alice's design.
	d := e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data()
	if d["options"].(map[string]any)["theme"] != "mint" {
		t.Errorf("design changed by a refused call: %v", d)
	}
	// A foreign origin cannot change a design (CSRF guard); a GET is unaffected.
	r := httptest.NewRequest("POST", "/api/book-design/reset", strings.NewReader(fmt.Sprintf(`{"yearbook_id":%q}`, id)))
	r.Header.Set("Origin", "https://evil.example.com")
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: alice.cookie})
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 403 || e.count(`SELECT COUNT(*) FROM book_design_tab`) != 1 {
		t.Errorf("foreign origin: %d", rec.Code)
	}
}

// AC3: nothing is stored for a rejected document, not even on top of an older design.
func TestRejectedDocumentsStoreNothing(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	bad := []string{
		`{"theme":"nope"}`, `{"v":2}`, `{"extra":1}`, `{"texts":{"cover.belongs":"` + strings.Repeat("a", 5000) + `"}}`,
		`{"texts":{"cover.belongs":"` + strings.Repeat("ê", 5000) + `"}}`, `{"texts":{"cover.belongs":"a\nb"}}`, `{"texts":{"cover.belongs":"a\u0007b"}}`,
		`{"sections":{"order":["cover","cover","notes","back"]}}`, `{"sections":{"order":["profile","cover","notes","back"]}}`,
		`{"sections":{"order":["cover","profile","notes"]}}`, `{"sections":{"hidden":["cover"]}}`, `{"sections":{"hidden":"profile"}}`,
		`{"theme":["mint"]}`, `{"images":{"cover.pattern":"waves"}}`, `{"images":[]}`, `[]`,
		`{"texts":{"profile.heading":"` + strings.Repeat("x", 4090) + `"}}`,
	}
	for _, doc := range bad {
		e.call("POST", "/api/book-design/update", alice, update(id, doc)).status(400, "ERROR_INVALID_DESIGN")
	}
	if n := e.count(`SELECT COUNT(*) FROM book_design_tab`); n != 0 {
		t.Fatalf("%d rows after rejected documents", n)
	}
	e.call("POST", "/api/book-design/update", alice, update(id, `{"theme":"peach"}`)).status(200, "OK")
	e.call("POST", "/api/book-design/update", alice, update(id, `{"theme":"nope"}`)).status(400, "ERROR_INVALID_DESIGN")
	var stored string
	if err := e.db.QueryRow(`SELECT options_json FROM book_design_tab`).Scan(&stored); err != nil || stored != `{"v":1,"theme":"peach"}` {
		t.Fatalf("stored %q, %v", stored, err)
	}
}

// A book whose template has no design block cannot be customised, but can be read (design null) and reset.
func TestTemplateWithoutDesignBlock(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	for _, tpl := range []string{"navy-classic", "classic"} {
		id := e.book(alice, tpl)
		d := e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data()
		if d["template_id"] != tpl || d["design"] != nil || d["options"] != nil {
			t.Errorf("%s: %v", tpl, d)
		}
		e.call("POST", "/api/book-design/update", alice, update(id, `{}`)).status(400, "ERROR_INVALID_DESIGN")
		e.call("POST", "/api/book-design/reset", alice, fmt.Sprintf(`{"yearbook_id":%q}`, id)).status(200, "OK")
	}
	// A book with no template at all.
	id := e.call("POST", "/api/yearbook/create", alice, `{"title":"Book","language":"en"}`).status(200, "OK").data()["id"].(string)
	d := e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data()
	if d["template_id"] != nil || d["design"] != nil {
		t.Errorf("no template: %v", d)
	}
	e.call("POST", "/api/book-design/update", alice, update(id, `{}`)).status(400, "ERROR_INVALID_DESIGN")
}

// AC2: the row follows a template change and a yearbook delete, and other books' rows stay.
func TestRowFollowsTemplateChangeAndDelete(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	a, b := e.book(alice, "memory-book"), e.book(alice, "memory-book")
	for _, id := range []string{a, b} {
		e.call("POST", "/api/book-design/update", alice, update(id, full)).status(200, "OK")
	}
	// Updating something else keeps the design; setting the same template again does too.
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"title":"New","template_id":"memory-book"}`, a)).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM book_design_tab`); n != 2 {
		t.Fatalf("%d rows after a title change, want 2", n)
	}
	// Changing the template deletes that book's row only.
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"navy-classic"}`, a)).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM book_design_tab`); n != 1 {
		t.Fatalf("%d rows after the template change, want 1", n)
	}
	d := e.call("GET", "/api/book-design/get?yearbook_id="+a, alice, "").status(200, "OK").data()
	if d["options"] != nil || d["design"] != nil {
		t.Errorf("changed book: %v", d)
	}
	// Going back does not bring the old design back, and clearing the template drops it too.
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"memory-book"}`, a)).status(200, "OK")
	if d := e.call("GET", "/api/book-design/get?yearbook_id="+a, alice, "").status(200, "OK").data(); d["options"] != nil {
		t.Errorf("old design came back: %v", d)
	}
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":null}`, b)).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM book_design_tab`); n != 0 {
		t.Fatalf("%d rows after clearing the template, want 0", n)
	}
	// Delete: the row goes with the book.
	e.call("POST", "/api/book-design/update", alice, update(a, full)).status(200, "OK")
	e.call("POST", "/api/yearbook/delete", alice, fmt.Sprintf(`{"id":%q}`, a)).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM book_design_tab`); n != 0 {
		t.Fatalf("%d rows after the book was deleted, want 0", n)
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// A row for another template than the book's current one (it cannot be written through the API, but a restore or a bug could)
// counts as no design on read and in the print data.
func TestStaleTemplateRowIsIgnored(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	e.call("POST", "/api/book-design/update", alice, update(id, full)).status(200, "OK")
	if _, err := e.db.Exec(`UPDATE book_design_tab SET template_id = 'navy-classic'`); err != nil {
		t.Fatal(err)
	}
	if d := e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK").data(); d["options"] != nil || d["updated_at"] != nil {
		t.Errorf("get: %v", d)
	}
	r := e.call("GET", "/api/book-data/get?yearbook_id="+id, alice, "").status(200, "OK").data()["design"].(map[string]any)
	if r["theme"] != "lavender" || r["fonts"] != "playful" {
		t.Errorf("book-data: %v", r)
	}
}

// AC4: book-data/get carries the resolved design, defaults filled; a theme the manifest no longer has resolves to the default.
func TestBookDataCarriesResolvedDesign(t *testing.T) {
	e := newEnv(t, nil)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	id := e.book(alice, "memory-book")
	get := func() map[string]any {
		return e.call("GET", "/api/book-data/get?yearbook_id="+id, alice, "").status(200, "OK").data()["design"].(map[string]any)
	}
	def := get()
	if def["theme"] != "lavender" || def["fonts"] != "playful" || len(def["sections"].(map[string]any)["order"].([]any)) != 4 ||
		len(def["sections"].(map[string]any)["hidden"].([]any)) != 0 || len(def["texts"].(map[string]any)) != 0 ||
		def["images"].(map[string]any)["cover.pattern"] != "dots" {
		t.Fatalf("defaults: %v", def)
	}
	e.call("POST", "/api/book-design/update", alice, update(id, full)).status(200, "OK")
	got := get()
	sec := got["sections"].(map[string]any)
	if got["theme"] != "mint" || got["fonts"] != "classic" || fmt.Sprint(sec["order"]) != "[cover notes profile back]" || fmt.Sprint(sec["hidden"]) != "[profile]" ||
		got["texts"].(map[string]any)["profile.heading"] != "Về mình" || got["images"].(map[string]any)["cover.pattern"] != "plain" {
		t.Fatalf("resolved: %v", got)
	}
	// A stored theme the manifest no longer has, and an unknown section, text and image, resolve to the defaults.
	if _, err := e.db.Exec(`UPDATE book_design_tab SET options_json = ?`,
		`{"v":1,"theme":"retired","fonts":"classic","sections":{"order":["gone","notes","profile"],"hidden":["gone"]},"texts":{"gone":"x"},"images":{"gone":"x"}}`); err != nil {
		t.Fatal(err)
	}
	got = get()
	if got["theme"] != "lavender" || got["fonts"] != "classic" || fmt.Sprint(got["sections"].(map[string]any)["order"]) != "[cover notes profile back]" || len(got["texts"].(map[string]any)) != 0 {
		t.Fatalf("stale ids: %v", got)
	}
	// Garbage in the column does not break the print either.
	if _, err := e.db.Exec(`UPDATE book_design_tab SET options_json = '{not json'`); err != nil {
		t.Fatal(err)
	}
	if got = get(); got["theme"] != "lavender" {
		t.Fatalf("garbage: %v", got)
	}
	// Someone else's book stays a 404; a book on a template without a design block has design null.
	e.call("GET", "/api/book-data/get?yearbook_id="+id, bob, "").status(404, "ERROR_NOT_FOUND")
	nid := e.book(alice, "navy-classic")
	nd := e.call("GET", "/api/book-data/get?yearbook_id="+nid, alice, "").status(200, "OK").data()
	if v, ok := nd["design"]; !ok || v != nil {
		t.Errorf("navy-classic design = %v (present %v), want null", v, ok)
	}
}

// The write rate limit applies to update and reset, per owner, and carries Retry-After.
func TestWriteRateLimit(t *testing.T) {
	e := newEnv(t, ratelimit.NewMemoryFactory(func() time.Time { return e0 }))
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	id, bid := e.book(alice, "memory-book"), e.book(bob, "memory-book")
	for range writesPerWindow {
		e.call("POST", "/api/book-design/update", alice, update(id, `{}`)).status(200, "OK")
	}
	r := e.call("POST", "/api/book-design/reset", alice, fmt.Sprintf(`{"yearbook_id":%q}`, id)).status(429, "ERROR_RATE_LIMITED")
	if r.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	e.call("POST", "/api/book-design/update", alice, update(id, `{}`)).status(429, "ERROR_RATE_LIMITED")
	e.call("GET", "/api/book-design/get?yearbook_id="+id, alice, "").status(200, "OK") // reads are free
	e.call("POST", "/api/book-design/update", bob, update(bid, `{}`)).status(200, "OK")
}

var e0 = time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC)

// Lock order (yearbook row first): parallel design writes, template changes, title updates and a delete of one book end without a
// 5xx (no MySQL deadlock) and leave no row behind for the deleted book.
func TestParallelWritesDoNotDeadlock(t *testing.T) {
	e := newEnv(t, nil)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	var wg sync.WaitGroup
	var mu sync.Mutex
	var bad []string
	run := func(method, path, body string) {
		defer wg.Done()
		rec := e.do(method, path, alice, body)
		if rec.Code >= 500 {
			mu.Lock()
			bad = append(bad, fmt.Sprintf("%s %s: %d %s", method, path, rec.Code, rec.Body))
			mu.Unlock()
		}
	}
	for i := range 8 {
		tpl := []string{"memory-book", "navy-classic"}[i%2]
		wg.Add(5)
		go run("POST", "/api/book-design/update", update(id, `{"theme":"mint"}`))
		go run("POST", "/api/book-design/reset", fmt.Sprintf(`{"yearbook_id":%q}`, id))
		go run("POST", "/api/yearbook/update", fmt.Sprintf(`{"id":%q,"template_id":%q}`, id, tpl))
		go run("POST", "/api/yearbook/update", fmt.Sprintf(`{"id":%q,"title":"T%d"}`, id, i))
		go run("GET", "/api/book-data/get?yearbook_id="+id, "")
	}
	wg.Wait()
	wg.Add(3)
	go run("POST", "/api/yearbook/delete", fmt.Sprintf(`{"id":%q}`, id))
	go run("POST", "/api/book-design/update", update(id, `{"theme":"peach"}`))
	go run("POST", "/api/book-design/reset", fmt.Sprintf(`{"yearbook_id":%q}`, id))
	wg.Wait()
	if len(bad) > 0 {
		t.Fatalf("%d server errors: %v", len(bad), bad[:min(3, len(bad))])
	}
	if n := e.count(`SELECT COUNT(*) FROM book_design_tab`); n != 0 {
		t.Errorf("%d design rows left for a deleted book", n)
	}
	dbtest.AssertNoOrphans(t, e.db)
}
