//go:build integration

package yearbook

import (
	"bytes"
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
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/legal"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis/redistest"
)

const (
	origin = "http://localhost:5173"
	testPW = "Tr0ub4dor&3-fake-test-password" // obviously fake
)

type env struct {
	t     *testing.T
	db    *sql.DB
	h     http.Handler
	logs  *bytes.Buffer
	mu    sync.Mutex
	clock time.Time
	ah    *auth.Handler
}

func (e *env) now() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.clock
}

func (e *env) tick() {
	e.mu.Lock()
	e.clock = e.clock.Add(time.Minute)
	e.mu.Unlock()
}

type syncWriter struct {
	mu sync.Mutex
	w  *bytes.Buffer
}

func (s *syncWriter) Write(b []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.w.Write(b)
}

func newEnv(t *testing.T) *env { return newEnvWith(t) }

// newEnvWith registers child purgers with the yearbook service.
func newEnvWith(t *testing.T, children ...ChildPurger) *env {
	t.Helper()
	d := dbtest.New(t)
	e := &env{t: t, db: d, logs: &bytes.Buffer{}, clock: time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)}
	logger := slog.New(slog.NewJSONHandler(&syncWriter{w: e.logs}, &slog.HandlerOptions{Level: slog.LevelDebug}))
	hasher := auth.NewHasher(auth.HashParams{MemoryKiB: 64, Time: 1, Parallelism: 1}, 4, auth.HashWait)
	mail, _ := mailer.NewLog("test", io.Discard)
	rc := redistest.New(t)
	codes, err := auth.NewCodes(context.Background(), rc, "test", []byte(strings.Repeat("k", auth.MinOTPKeyBytes)), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions(context.Background(), rc, logger)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 1000},
		auth.Mail{Mailer: mail, Logger: logger}, codes, ratelimit.NewFactory(rc, logger, e.now), e.now)
	if err != nil {
		t.Fatal(err)
	}
	ah := auth.NewHandler(svc, auth.HandlerConfig{AllowedOrigins: []string{origin}}, logger)
	e.ah = ah
	st := NewStore(d)
	yh := NewHandler(NewService(st, httpx.NewCursors([]byte(strings.Repeat("c", 32))), nil, e.now, children...), ah.RequireUser, []string{origin})
	e.h = httpx.NewRouter(logger, ah.Routes, yh.Routes)
	return e
}

type user struct{ cookie string }

func (e *env) register(email, name string) user {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": testPW, "display_name": name, "accept_terms_version": legal.CurrentVersion})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/auth/register", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		e.t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
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

// do sends a request as u (zero user = no cookie) with a JSON body.
func (e *env) do(u user, method, path, body string) resp {
	e.t.Helper()
	return e.doOn(e.h, u, method, path, body)
}

// doOn is do against another handler (the tests that count queries build their own).
func (e *env) doOn(h http.Handler, u user, method, path, body string) resp {
	e.t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if u.cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
		r.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return resp{rec, e.t}
}

func (r resp) status(want int, code string) resp {
	r.t.Helper()
	if r.Code != want {
		r.t.Fatalf("status %d, want %d (%s)", r.Code, want, r.Body.String())
	}
	if code != "" {
		var env struct{ Status string }
		if err := json.Unmarshal(r.Body.Bytes(), &env); err != nil || env.Status != code {
			r.t.Fatalf("status field %q (err %v), want %q: %s", env.Status, err, code, r.Body.String())
		}
	}
	return r
}

// book is the yearbook in the "data" of a success answer.
func (r resp) book() map[string]any {
	r.t.Helper()
	var out struct {
		Status string
		Data   map[string]any
	}
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil || out.Status != "OK" || out.Data == nil {
		r.t.Fatalf("no yearbook in %s", r.Body.String())
	}
	return out.Data
}

// page is the "data" of a get-list answer.
func (r resp) page() (items []map[string]any, nextID string) {
	r.t.Helper()
	var out struct {
		Status string
		Data   struct {
			Items  []map[string]any `json:"items"`
			NextID *string          `json:"next_id"`
		}
	}
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil || out.Status != "OK" || out.Data.Items == nil || out.Data.NextID == nil {
		r.t.Fatalf("not a page in %s", r.Body.String())
	}
	return out.Data.Items, *out.Data.NextID
}

// withID puts "key":"id" first into the JSON object body.
func withID(key, id, body string) string {
	rest := strings.TrimPrefix(body, "{")
	if strings.TrimSpace(rest) != "}" {
		rest = "," + rest
	}
	return `{"` + key + `":"` + id + `"` + rest
}

func (e *env) get(u user, id string) resp { return e.do(u, "GET", "/api/yearbook/get?id="+id, "") }
func (e *env) update(u user, id, body string) resp {
	return e.do(u, "POST", "/api/yearbook/update", withID("id", id, body))
}
func (e *env) profile(u user, id, body string) resp {
	return e.do(u, "POST", "/api/yearbook/update-profile", withID("yearbook_id", id, body))
}
func (e *env) remove(u user, id string) resp {
	return e.do(u, "POST", "/api/yearbook/delete", withID("id", id, "{}"))
}

func (e *env) create(u user, title string) string {
	e.t.Helper()
	e.tick()
	b, _ := json.Marshal(map[string]string{"title": title, "language": "en"})
	return e.do(u, "POST", "/api/yearbook/create", string(b)).status(200, "").book()["id"].(string)
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// AC1, AC3, AC7: create, read back, ids and the profile default.
func TestCreateAndGet(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Đặng Thị Hồng 🎓")
	rec := e.do(alice, "POST", "/api/yearbook/create", "{\"title\":\"  Class of 2026 Việt \",\"school_name\":\"Fake University\",\"graduation_year\":2026,\"language\":\"vi\",\"page_size\":\"A4\"}").status(200, "")
	b := rec.book()
	if b["title"] != "Class of 2026 Việt" || b["page_size"] != "A4" || b["language"] != "vi" || b["graduation_year"] != float64(2026) || b["template_id"] != nil {
		t.Fatalf("unexpected book %v", b)
	}
	id, _ := b["id"].(string)
	if len(id) != 26 {
		t.Fatalf("id %q is not a ULID", id)
	}
	p := b["profile"].(map[string]any)
	if p["full_name"] != "Đặng Thị Hồng 🎓" || p["is_owner"] != true || p["birthday"] != nil || len(p["id"].(string)) != 26 {
		t.Fatalf("unexpected profile %v", p)
	}
	for _, k := range []string{"owner_id", "internal_id"} {
		if strings.Contains(rec.Body.String(), `"`+k+`"`) {
			t.Fatalf("response leaks %s", k)
		}
	}
	got := e.get(alice, id).status(200, "").book()
	if fmt.Sprint(got) != fmt.Sprint(b) {
		t.Fatalf("GET differs from POST:\n%v\n%v", got, b)
	}
	// default page size
	d := e.do(alice, "POST", "/api/yearbook/create", `{"title":"Plain","language":"en"}`).status(200, "").book()
	if d["page_size"] != "A5" || d["school_name"] != "" {
		t.Fatalf("defaults: %v", d)
	}
}

// T-037 AC2: a book can be created as Letter, moved between sizes and back; bad values change nothing.
func TestPageSizeLetter(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	b := e.do(alice, "POST", "/api/yearbook/create", `{"title":"L","language":"en","page_size":"Letter"}`).status(200, "").book()
	if b["page_size"] != "Letter" {
		t.Fatalf("created %v", b)
	}
	id := e.create(alice, "A5 book")
	for _, size := range []string{"Letter", "A4", "Letter", "A5"} {
		got := e.update(alice, id, `{"page_size":"`+size+`"}`).status(200, "").book()
		if got["page_size"] != size {
			t.Fatalf("patched to %s: %v", size, got)
		}
		if g := e.get(alice, id).book(); g["page_size"] != size {
			t.Fatalf("read back %s: %v", size, g)
		}
	}
	for _, bad := range []string{"letter", "LETTER", "Legal"} {
		e.update(alice, id, `{"page_size":"`+bad+`"}`).status(400, "ERROR_INVALID_PAGE_SIZE")
	}
	if g := e.get(alice, id).book(); g["page_size"] != "A5" {
		t.Fatalf("rejected patch applied: %v", g)
	}
}

// T-061: the template choice is saved by yearbook/update, read back, cleared with null, and refused when unknown or unsupported.
func TestTemplateChoice(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	id := e.create(alice, "Book")
	if got := e.update(alice, id, `{"template_id":"memory-book"}`).status(200, "").book(); got["template_id"] != "memory-book" {
		t.Fatalf("saved %v", got)
	}
	if g := e.get(alice, id).book(); g["template_id"] != "memory-book" {
		t.Fatalf("read back %v", g)
	}
	e.update(alice, id, `{"template_id":"nope"}`).status(400, "ERROR_INVALID_TEMPLATE")
	e.update(alice, id, `{"page_size":"Letter","template_id":"classic"}`).status(400, "ERROR_INVALID_TEMPLATE")
	if g := e.get(alice, id).book(); g["template_id"] != "memory-book" || g["page_size"] != "A5" {
		t.Fatalf("a refused update changed the book: %v", g)
	}
	e.update(alice, id, `{"template_id":"classic"}`).status(200, "")
	e.update(alice, id, `{"page_size":"Letter"}`).status(400, "ERROR_INVALID_TEMPLATE") // classic has no Letter
	if got := e.update(alice, id, `{"template_id":null}`).status(200, "").book(); got["template_id"] != nil {
		t.Fatalf("cleared %v", got)
	}
	e.update(alice, id, `{"page_size":"Letter"}`).status(200, "")
	// another owner's book is not found, whatever the template
	bob := e.register("bob@example.com", "Bob")
	e.update(bob, id, `{"template_id":"memory-book"}`).status(404, "ERROR_NOT_FOUND")
	// create takes it too, checked against the same call's page size
	e.do(alice, "POST", "/api/yearbook/create", `{"title":"T","language":"en","page_size":"Letter","template_id":"memory-book"}`).status(200, "")
	e.do(alice, "POST", "/api/yearbook/create", `{"title":"T","language":"en","page_size":"Letter","template_id":"classic"}`).status(400, "ERROR_INVALID_TEMPLATE")
}

// AC4, AC5: create validation over HTTP, including mass assignment.
func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	long := func(n int) string { return strings.Repeat("a", n) }
	tests := []struct {
		body   string
		status int
		code   string
		why    string
	}{
		{`{"language":"en"}`, 400, "ERROR_INVALID_TITLE", "missing title"},
		{`{"title":null,"language":"en"}`, 400, "ERROR_INVALID_TITLE", "null title"},
		{`{"title":"  ","language":"en"}`, 400, "ERROR_INVALID_TITLE", "blank title"},
		{`{"title":"` + long(121) + `","language":"en"}`, 400, "ERROR_INVALID_TITLE", "long title"},
		{`{"title":"` + long(120) + `","language":"en"}`, 200, "", "max title"},
		{`{"title":"x"}`, 400, "ERROR_INVALID_LANGUAGE", "missing language"},
		{`{"title":"x","language":"de"}`, 400, "ERROR_INVALID_LANGUAGE", "bad language"},
		{`{"title":"x","language":"en","page_size":"A3"}`, 400, "ERROR_INVALID_PAGE_SIZE", "bad page size"},
		{`{"title":"x","language":"en","page_size":"letter"}`, 400, "ERROR_INVALID_PAGE_SIZE", "lower case Letter"},
		{`{"title":"x","language":"en","page_size":"LETTER"}`, 400, "ERROR_INVALID_PAGE_SIZE", "upper case Letter"},
		{`{"title":"x","language":"en","page_size":"Legal"}`, 400, "ERROR_INVALID_PAGE_SIZE", "Legal"},
		{`{"title":"x","language":"en","page_size":"Letter"}`, 200, "", "Letter"},
		{`{"title":"x","language":"en","graduation_year":1949}`, 400, "ERROR_INVALID_GRADUATION_YEAR", "year low"},
		{`{"title":"x","language":"en","graduation_year":2101}`, 400, "ERROR_INVALID_GRADUATION_YEAR", "year high"},
		{`{"title":"x","language":"en","graduation_year":"2020"}`, 400, "ERROR_PARAM", "year as string"},
		{`{"title":"x","language":"en","motto":"` + long(201) + `"}`, 400, "ERROR_INVALID_MOTTO", "long motto"},
		{`{"title":"x\u200by","language":"en"}`, 400, "ERROR_INVALID_TITLE", "zero-width space"},
		{`{"title":"x\ny","language":"en"}`, 400, "ERROR_INVALID_TITLE", "newline"},
		{`{"title":"x","language":"en","owner_id":1}`, 400, "ERROR_PARAM", "owner_id"},
		{`{"title":"x","language":"en","id":"01J9Z3K6V8Q4M7N2P5R8T0W1XY"}`, 400, "ERROR_PARAM", "id"},
		{`{"title":"x","language":"en","public_id":"A"}`, 400, "ERROR_PARAM", "public_id"},
		{`{"title":"x","language":"en","profile":{"is_owner":false}}`, 400, "ERROR_PARAM", "nested profile"},
		{`not json`, 400, "ERROR_PARAM", "garbage"},
		{``, 400, "ERROR_PARAM", "empty body"},
	}
	for _, tc := range tests {
		t.Log(tc.why) // shown with the failure when the check below stops the test
		e.do(alice, "POST", "/api/yearbook/create", tc.body).status(tc.status, tc.code)
	}
}

// AC2: only own books, newest updated_at first, limit + cursor.
func TestListPaging(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	bob := e.register("bob@example.com", "Bob")
	var ids []string
	for i := 0; i < 5; i++ {
		ids = append(ids, e.create(alice, fmt.Sprintf("A%d", i)))
	}
	e.create(bob, "Bob's")
	// Touching A1 moves it to the front.
	e.tick()
	e.update(alice, ids[1], `{"motto":"touched"}`).status(200, "")
	want := []string{ids[1], ids[4], ids[3], ids[2], ids[0]}

	var got []string
	next := ""
	for page := 0; page < 10; page++ {
		path := "/api/yearbook/get-list?limit=2"
		if next != "" {
			path += "&next_id=" + next
		}
		items, nextID := e.do(alice, "GET", path, "").status(200, "").page()
		for _, y := range items {
			got = append(got, y["id"].(string))
		}
		if nextID == "" {
			break
		}
		next = nextID
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	// Default limit returns everything here; an empty account returns [] and "" (not null).
	carol := e.register("carol@example.com", "Carol")
	rec := e.do(carol, "GET", "/api/yearbook/get-list", "").status(200, "")
	if !strings.Contains(rec.Body.String(), `"data":{"items":[],"next_id":""}`) {
		t.Fatalf("empty list: %s", rec.Body.String())
	}
	for _, q := range []string{"limit=0", "limit=51", "limit=-1", "limit=abc", "limit=1.5", "next_id=garbage", "next_id=!!", "limit=2&next_id=" + next + "x", "next_id=" + next[:len(next)-3]} {
		e.do(alice, "GET", "/api/yearbook/get-list?"+q, "").status(400, "ERROR_PARAM")
	}
	e.do(alice, "GET", "/api/yearbook/get-list?limit=50", "").status(200, "")
	// A token taken from another user's list is refused: it is bound to the list it came from.
	e.do(bob, "GET", "/api/yearbook/get-list?next_id="+next, "").status(400, "ERROR_PARAM")
}

// AC3, AC4, AC7: the ownership matrix. Another user's book and a missing book look the same.
func TestOwnershipMatrix(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	bob := e.register("bob@example.com", "Bob")
	id := e.create(alice, "Alice's book")
	missing := "01J9Z3K6V8Q4M7N2P5R8T0W1XY"
	calls := []func(target string) resp{
		func(target string) resp { return e.get(bob, target) },
		func(target string) resp { return e.update(bob, target, `{"title":"hijack"}`) },
		func(target string) resp { return e.profile(bob, target, `{"full_name":"hijack"}`) },
		func(target string) resp { return e.remove(bob, target) },
	}
	var bodies []string
	for i, call := range calls {
		for _, target := range []string{id, missing} {
			rec := call(target).status(404, "ERROR_NOT_FOUND")
			bodies = append(bodies, fmt.Sprint(i)+rec.Body.String()[:strings.Index(rec.Body.String(), `"request_id"`)])
		}
		// T-100 (audit M3): a malformed id, or the real id in another spelling, is ERROR_PARAM before any query.
		for _, target := range []string{"not-a-ulid", "1", strings.ToLower(id), strings.ToLower(missing)} {
			call(target).status(400, "ERROR_PARAM")
		}
	}
	// identical bodies (apart from request_id) per verb for own-missing and foreign ids
	for i := 0; i < len(bodies); i += 2 {
		for _, b := range bodies[i : i+2] {
			if b != bodies[i] {
				t.Fatalf("404 bodies differ: %q vs %q", b, bodies[i])
			}
		}
	}
	// Alice's book is untouched.
	b := e.get(alice, id).status(200, "").book()
	if b["title"] != "Alice's book" || b["profile"].(map[string]any)["full_name"] != "Alice" {
		t.Fatalf("book changed: %v", b)
	}
	if e.count(`SELECT COUNT(*) FROM yearbook_tab`) != 1 {
		t.Fatal("book count changed")
	}
}

// AC4: PATCH is partial, PUT profile replaces.
func TestPatchAndProfile(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	id := e.create(alice, "Book")
	e.update(alice, id, `{"graduation_year":2027,"motto":"Carpe diem","class_name":"K65"}`).status(200, "")
	b := e.update(alice, id, `{"title":"Renamed","graduation_year":null}`).status(200, "").book()
	if b["title"] != "Renamed" || b["motto"] != "Carpe diem" || b["class_name"] != "K65" || b["graduation_year"] != nil || b["language"] != "en" {
		t.Fatalf("patch result %v", b)
	}
	// Rejected patch changes nothing.
	e.update(alice, id, `{"motto":"new","title":""}`).status(400, "ERROR_INVALID_TITLE")
	e.update(alice, id, `{"owner_id":5}`).status(400, "ERROR_PARAM")
	e.update(alice, id, `{"profile":{"full_name":"x"}}`).status(400, "ERROR_PARAM")
	if g := e.get(alice, id).book(); g["motto"] != "Carpe diem" {
		t.Fatalf("rejected patch applied: %v", g)
	}
	// Empty patch is a no-op that still succeeds.
	e.update(alice, id, `{}`).status(200, "")

	full := `{"full_name":"Nguyễn Văn A","nickname":"Bống","birthday":"2004-02-29","quote":"Stay hungry","hobbies":"Chess","future_plans":"Engineer"}`
	p := e.profile(alice, id, full).status(200, "").book()["profile"].(map[string]any)
	if p["full_name"] != "Nguyễn Văn A" || p["birthday"] != "2004-02-29" || p["nickname"] != "Bống" || p["hobbies"] != "Chess" || p["is_owner"] != true {
		t.Fatalf("profile %v", p)
	}
	// Replace: omitted optional fields are cleared.
	p = e.profile(alice, id, `{"full_name":"Only Name"}`).status(200, "").book()["profile"].(map[string]any)
	if p["nickname"] != "" || p["birthday"] != nil || p["quote"] != "" || p["full_name"] != "Only Name" {
		t.Fatalf("not replaced: %v", p)
	}
	for body, code := range map[string]string{
		`{}`: "ERROR_INVALID_FULL_NAME",
		`{"full_name":"a","birthday":"2099-01-01"}`:                      "ERROR_INVALID_BIRTHDAY",
		`{"full_name":"a","birthday":"2026-10-07"}`:                      "ERROR_INVALID_BIRTHDAY",
		`{"full_name":"a","birthday":"2001-13-01"}`:                      "ERROR_INVALID_BIRTHDAY",
		`{"full_name":"a","birthday":""}`:                                "ERROR_INVALID_BIRTHDAY",
		`{"full_name":"a","nickname":"` + strings.Repeat("x", 51) + `"}`: "ERROR_INVALID_NICKNAME",
		`{"full_name":"a","quote":"` + strings.Repeat("x", 501) + `"}`:   "ERROR_INVALID_QUOTE",
		`{"full_name":"a","is_owner":false}`:                             "ERROR_PARAM",
		`{"full_name":"a","id":"x"}`:                                     "ERROR_PARAM",
		`{"full_name":"a","photo_media_id":2}`:                           "ERROR_PARAM", // a known field since T-009, but not a string
	} {
		e.profile(alice, id, body).status(400, code)
	}
	// Exactly one profile, still the owner's.
	if e.count(`SELECT COUNT(*) FROM profile_tab WHERE is_owner`) != 1 || e.count(`SELECT COUNT(*) FROM profile_tab`) != 1 {
		t.Fatal("profile rows changed")
	}
	// A profile edit counts as an edit of the book: it moves to the front.
	other := e.create(alice, "Newer")
	e.tick()
	e.profile(alice, id, `{"full_name":"Again"}`).status(200, "")
	books, _ := e.do(alice, "GET", "/api/yearbook/get-list", "").page()
	if books[0]["id"] != id || books[1]["id"] != other {
		t.Fatalf("order after profile edit: %v", books)
	}
	// Birthday is personal data: it is never written to the logs.
	if strings.Contains(e.logs.String(), "2004-02-29") {
		t.Fatal("birthday found in logs")
	}
}

// AC6: delete removes the profile too, then the id is gone; recreate works; the limit is 20 per user.
func TestDeleteAndLimit(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	bob := e.register("bob@example.com", "Bob")
	id := e.create(alice, "Doomed")
	e.remove(alice, id).status(200, "")
	e.get(alice, id).status(404, "ERROR_NOT_FOUND")
	e.remove(alice, id).status(404, "ERROR_NOT_FOUND")
	if e.count(`SELECT COUNT(*) FROM yearbook_tab`) != 0 || e.count(`SELECT COUNT(*) FROM profile_tab`) != 0 {
		t.Fatal("rows left after delete")
	}
	e.create(alice, "Recreated")

	for i := 1; i < 20; i++ {
		e.create(alice, fmt.Sprintf("B%d", i))
	}
	e.do(alice, "POST", "/api/yearbook/create", `{"title":"21st","language":"en"}`).status(409, "ERROR_LIMIT_REACHED")
	e.create(bob, "Bob is unaffected")
	books, _ := e.do(alice, "GET", "/api/yearbook/get-list?limit=50", "").page()
	if len(books) != 20 {
		t.Fatalf("alice has %d books", len(books))
	}
	// Freeing a slot allows a new book.
	e.remove(alice, books[0]["id"].(string)).status(200, "")
	e.do(alice, "POST", "/api/yearbook/create", `{"title":"21st","language":"en"}`).status(200, "")
}

// The limit holds under concurrent creates.
func TestLimitConcurrent(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	var wg sync.WaitGroup
	codes := make(chan int, 30)
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- e.do(alice, "POST", "/api/yearbook/create", `{"title":"x","language":"en"}`).Code
		}()
	}
	wg.Wait()
	close(codes)
	created := 0
	for c := range codes {
		switch c {
		case 200:
			created++
		case 409:
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if created != 20 || e.count(`SELECT COUNT(*) FROM yearbook_tab`) != 20 {
		t.Fatalf("created %d, want 20", created)
	}
}

// AC7: every endpoint needs a session, and writes with a cookie need an allowed origin.
func TestAuthRequired(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	id := e.create(alice, "Book")
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/yearbook/create", `{"title":"x","language":"en"}`},
		{"GET", "/api/yearbook/get-list", ""},
		{"GET", "/api/yearbook/get?id=" + id, ""},
		{"POST", "/api/yearbook/update", `{"id":"` + id + `","title":"x"}`},
		{"POST", "/api/yearbook/update-profile", `{"yearbook_id":"` + id + `","full_name":"x"}`},
		{"POST", "/api/yearbook/delete", `{"id":"` + id + `"}`},
	} {
		e.do(user{}, c.method, c.path, c.body).status(401, "ERROR_UNAUTHORIZED")
		e.do(user{"bogus-token"}, c.method, c.path, c.body).status(401, "ERROR_UNAUTHORIZED")
		if c.method == "POST" { // cross-site write with a valid cookie
			r := httptest.NewRequest("POST", c.path, strings.NewReader(c.body))
			r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: alice.cookie})
			r.Header.Set("Origin", "https://evil.example")
			r.Header.Set("Content-Type", "application/json")
			rec := httptest.NewRecorder()
			e.h.ServeHTTP(rec, r)
			if rec.Code != 403 {
				t.Fatalf("cross-origin %s: %d", c.path, rec.Code)
			}
		}
	}
	e.get(alice, id).status(200, "")
	// Wrong content type on a body.
	r := httptest.NewRequest("POST", "/api/yearbook/create", strings.NewReader(`{"title":"x","language":"en"}`))
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: alice.cookie})
	r.Header.Set("Origin", origin)
	r.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 415 {
		t.Fatalf("text/plain: %d", rec.Code)
	}
}

// T-070 AC1, AC3: the v2 shapes. Times are Unix ms numbers, the book is the "data" itself, a list is {items,next_id}, a
// delete answers an empty object, the Vietnamese title survives the round trip, and the old routes are gone.
func TestV2Shapes(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	now := e.now().UnixMilli()
	rec := e.do(alice, "POST", "/api/yearbook/create", `{"title":"Lớp 12A1 - Trường THPT Chu Văn An","language":"vi"}`).status(200, "")
	b := rec.book()
	if b["title"] != "Lớp 12A1 - Trường THPT Chu Văn An" || b["created_at"] != float64(now) || b["updated_at"] != float64(now) || b["graduation_year"] != nil {
		t.Fatalf("created %v", b)
	}
	if strings.Contains(rec.Body.String(), `"yearbook":`) || !strings.HasPrefix(rec.Body.String(), `{"status":"OK","data":{`) {
		t.Fatalf("not the v2 envelope: %s", rec.Body.String())
	}
	id := b["id"].(string)

	e.tick()
	up := e.update(alice, id, `{"motto":"Học, học nữa, học mãi"}`).status(200, "").book()
	if up["updated_at"] != float64(e.now().UnixMilli()) || up["created_at"] != float64(now) {
		t.Fatalf("update times %v", up)
	}
	items, next := e.do(alice, "GET", "/api/yearbook/get-list", "").status(200, "").page()
	if len(items) != 1 || next != "" || items[0]["id"] != id || items[0]["updated_at"] != up["updated_at"] {
		t.Fatalf("list %v %q", items, next)
	}

	// The error envelope: status, error_message and request_id, with the HTTP status kept.
	rec = e.update(alice, "01J9Z3K6V8Q4M7N2P5R8T0W1XY", `{"title":"x"}`).status(404, "ERROR_NOT_FOUND")
	var errBody map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &errBody)
	if errBody["error_message"] == "" || errBody["request_id"] == "" || errBody["request_id"] != rec.Header().Get("X-Request-Id") {
		t.Fatalf("error body %v", errBody)
	}

	// Missing or malformed input is ERROR_PARAM, whichever endpoint.
	e.do(alice, "POST", "/api/yearbook/update", `{"title":"no id"}`).status(400, "ERROR_PARAM")
	e.do(alice, "POST", "/api/yearbook/update", `{}`).status(400, "ERROR_PARAM")
	e.do(alice, "POST", "/api/yearbook/update", ``).status(400, "ERROR_PARAM")
	e.do(alice, "POST", "/api/yearbook/update-profile", `{"full_name":"no book"}`).status(400, "ERROR_PARAM")
	e.do(alice, "POST", "/api/yearbook/delete", `{}`).status(400, "ERROR_PARAM")
	e.do(alice, "GET", "/api/yearbook/get", "").status(400, "ERROR_PARAM")
	e.do(alice, "POST", "/api/yearbook/delete", `{"id":"`+id+`","extra":1}`).status(400, "ERROR_PARAM")
	e.get(alice, id).status(200, "") // none of the rejected calls deleted it

	// A cover cannot be set on a new book: there is no photo of it yet.
	e.do(alice, "POST", "/api/yearbook/create", `{"title":"x","language":"en","cover_media_id":"01J9Z3K6V8Q4M7N2P5R8T0W1XY"}`).status(400, "ERROR_INVALID_MEDIA")

	rec = e.remove(alice, id).status(200, "")
	if got := strings.TrimSpace(rec.Body.String()); got != `{"status":"OK","data":{}}` {
		t.Fatalf("delete answered %s", got)
	}
	// The v1 routes are gone, and the new ones take their own method only.
	for _, c := range [][2]string{{"POST", "/v1/yearbooks"}, {"GET", "/v1/yearbooks"}, {"GET", "/v1/yearbooks/" + id}, {"DELETE", "/v1/yearbooks/" + id}} {
		if rec := e.do(alice, c[0], c[1], ""); (rec.Code != 404 && rec.Code != 405) || !strings.Contains(rec.Body.String(), `"status":"ERROR_`) {
			t.Errorf("%s %s: %d %s, want a v2 404 or 405", c[0], c[1], rec.Code, rec.Body)
		}
	}
	e.do(alice, "GET", "/api/yearbook/delete", "").status(405, "ERROR_METHOD_NOT_ALLOWED")
	e.do(alice, "POST", "/api/yearbook/get-list", `{}`).status(405, "ERROR_METHOD_NOT_ALLOWED")
}

// T-111 AC2: a signed-in user who has not accepted the current Terms is stopped by the one gate (RequireUser) on a yearbook
// call, and can use it again after accept-terms.
func TestTermsGateStopsYearbookCalls(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	id := e.create(alice, "Before")
	if _, err := e.db.Exec(`UPDATE user_tab SET terms_version = NULL`); err != nil {
		t.Fatal(err)
	}
	e.get(alice, id).status(403, "ERROR_TERMS_REQUIRED")
	e.do(alice, "POST", "/api/yearbook/create", `{"title":"Blocked","language":"en"}`).status(403, "ERROR_TERMS_REQUIRED")
	e.do(alice, "GET", "/api/user/get-me", "").status(200, "")
	e.do(alice, "POST", "/api/user/accept-terms", `{"version":"`+legal.CurrentVersion+`"}`).status(200, "")
	e.get(alice, id).status(200, "")
}
