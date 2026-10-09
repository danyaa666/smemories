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

func newEnv(t *testing.T) *env {
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
	svc, err := auth.NewService(auth.NewStore(d), hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000},
		auth.Mail{Mailer: mail, Logger: logger}, codes, ratelimit.NewFactory(rc, logger, e.now), e.now)
	if err != nil {
		t.Fatal(err)
	}
	ah := auth.NewHandler(svc, auth.HandlerConfig{AllowedOrigins: []string{origin}}, logger)
	yh := NewHandler(NewStore(d), nil, ah.RequireUser, []string{origin}, logger, e.now)
	e.h = httpx.NewRouter(logger, ah.Routes, yh.Routes)
	return e
}

type user struct{ cookie string }

func (e *env) register(email, name string) user {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": testPW, "display_name": name})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/auth/register", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(rec, r)
	if rec.Code != 201 {
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
	e.h.ServeHTTP(rec, r)
	return resp{rec, e.t}
}

func (r resp) status(want int, code string) resp {
	r.t.Helper()
	if r.Code != want {
		r.t.Fatalf("status %d, want %d (%s)", r.Code, want, r.Body.String())
	}
	if code != "" {
		var env struct{ Error struct{ Code string } }
		if err := json.Unmarshal(r.Body.Bytes(), &env); err != nil || env.Error.Code != code {
			r.t.Fatalf("code %q (err %v), want %q: %s", env.Error.Code, err, code, r.Body.String())
		}
	}
	return r
}

func (r resp) book() map[string]any {
	r.t.Helper()
	var out struct{ Yearbook map[string]any }
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil || out.Yearbook == nil {
		r.t.Fatalf("no yearbook in %s", r.Body.String())
	}
	return out.Yearbook
}

func (e *env) create(u user, title string) string {
	e.t.Helper()
	e.tick()
	b, _ := json.Marshal(map[string]string{"title": title, "language": "en"})
	return e.do(u, "POST", "/v1/yearbooks", string(b)).status(201, "").book()["id"].(string)
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
	rec := e.do(alice, "POST", "/v1/yearbooks",
		"{\"title\":\"  Class of 2026 Việt \",\"school_name\":\"Fake University\",\"graduation_year\":2026,\"language\":\"vi\",\"page_size\":\"A4\"}").status(201, "")
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
	got := e.do(alice, "GET", "/v1/yearbooks/"+id, "").status(200, "").book()
	if fmt.Sprint(got) != fmt.Sprint(b) {
		t.Fatalf("GET differs from POST:\n%v\n%v", got, b)
	}
	// default page size
	d := e.do(alice, "POST", "/v1/yearbooks", `{"title":"Plain","language":"en"}`).status(201, "").book()
	if d["page_size"] != "A5" || d["school_name"] != "" {
		t.Fatalf("defaults: %v", d)
	}
}

// T-037 AC2: a book can be created as Letter, moved between sizes and back; bad values change nothing.
func TestPageSizeLetter(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	b := e.do(alice, "POST", "/v1/yearbooks", `{"title":"L","language":"en","page_size":"Letter"}`).status(201, "").book()
	if b["page_size"] != "Letter" {
		t.Fatalf("created %v", b)
	}
	id := e.create(alice, "A5 book")
	for _, size := range []string{"Letter", "A4", "Letter", "A5"} {
		got := e.do(alice, "PATCH", "/v1/yearbooks/"+id, `{"page_size":"`+size+`"}`).status(200, "").book()
		if got["page_size"] != size {
			t.Fatalf("patched to %s: %v", size, got)
		}
		if g := e.do(alice, "GET", "/v1/yearbooks/"+id, "").book(); g["page_size"] != size {
			t.Fatalf("read back %s: %v", size, g)
		}
	}
	for _, bad := range []string{"letter", "LETTER", "Legal"} {
		e.do(alice, "PATCH", "/v1/yearbooks/"+id, `{"page_size":"`+bad+`"}`).status(400, "invalid_page_size")
	}
	if g := e.do(alice, "GET", "/v1/yearbooks/"+id, "").book(); g["page_size"] != "A5" {
		t.Fatalf("rejected patch applied: %v", g)
	}
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
		{`{"language":"en"}`, 400, "invalid_title", "missing title"},
		{`{"title":null,"language":"en"}`, 400, "invalid_title", "null title"},
		{`{"title":"  ","language":"en"}`, 400, "invalid_title", "blank title"},
		{`{"title":"` + long(121) + `","language":"en"}`, 400, "invalid_title", "long title"},
		{`{"title":"` + long(120) + `","language":"en"}`, 201, "", "max title"},
		{`{"title":"x"}`, 400, "invalid_language", "missing language"},
		{`{"title":"x","language":"de"}`, 400, "invalid_language", "bad language"},
		{`{"title":"x","language":"en","page_size":"A3"}`, 400, "invalid_page_size", "bad page size"},
		{`{"title":"x","language":"en","page_size":"letter"}`, 400, "invalid_page_size", "lower case Letter"},
		{`{"title":"x","language":"en","page_size":"LETTER"}`, 400, "invalid_page_size", "upper case Letter"},
		{`{"title":"x","language":"en","page_size":"Legal"}`, 400, "invalid_page_size", "Legal"},
		{`{"title":"x","language":"en","page_size":"Letter"}`, 201, "", "Letter"},
		{`{"title":"x","language":"en","graduation_year":1949}`, 400, "invalid_graduation_year", "year low"},
		{`{"title":"x","language":"en","graduation_year":2101}`, 400, "invalid_graduation_year", "year high"},
		{`{"title":"x","language":"en","graduation_year":"2020"}`, 400, "invalid_body", "year as string"},
		{`{"title":"x","language":"en","motto":"` + long(201) + `"}`, 400, "invalid_motto", "long motto"},
		{`{"title":"x\u200by","language":"en"}`, 400, "invalid_title", "zero-width space"},
		{`{"title":"x\ny","language":"en"}`, 400, "invalid_title", "newline"},
		{`{"title":"x","language":"en","owner_id":1}`, 400, "unknown_field", "owner_id"},
		{`{"title":"x","language":"en","id":"01J9Z3K6V8Q4M7N2P5R8T0W1XY"}`, 400, "unknown_field", "id"},
		{`{"title":"x","language":"en","public_id":"A"}`, 400, "unknown_field", "public_id"},
		{`{"title":"x","language":"en","profile":{"is_owner":false}}`, 400, "unknown_field", "nested profile"},
		{`not json`, 400, "invalid_body", "garbage"},
		{``, 400, "invalid_body", "empty body"},
	}
	for _, tc := range tests {
		t.Log(tc.why) // shown with the failure when the check below stops the test
		e.do(alice, "POST", "/v1/yearbooks", tc.body).status(tc.status, tc.code)
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
	e.do(alice, "PATCH", "/v1/yearbooks/"+ids[1], `{"motto":"touched"}`).status(200, "")
	want := []string{ids[1], ids[4], ids[3], ids[2], ids[0]}

	var got []string
	cursor := ""
	for page := 0; page < 10; page++ {
		path := "/v1/yearbooks?limit=2"
		if cursor != "" {
			path += "&cursor=" + cursor
		}
		var out struct {
			Yearbooks  []map[string]any `json:"yearbooks"`
			NextCursor *string          `json:"next_cursor"`
		}
		rec := e.do(alice, "GET", path, "").status(200, "")
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		for _, y := range out.Yearbooks {
			got = append(got, y["id"].(string))
		}
		if out.NextCursor == nil {
			break
		}
		cursor = *out.NextCursor
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	// Default limit returns everything here; an empty account returns [] not null.
	carol := e.register("carol@example.com", "Carol")
	rec := e.do(carol, "GET", "/v1/yearbooks", "").status(200, "")
	if !strings.Contains(rec.Body.String(), `"yearbooks":[]`) || !strings.Contains(rec.Body.String(), `"next_cursor":null`) {
		t.Fatalf("empty list: %s", rec.Body.String())
	}
	for _, q := range []string{"limit=0", "limit=51", "limit=-1", "limit=abc", "limit=1.5", "cursor=garbage", "cursor=!!", "limit=2&cursor=" + cursor + "x"} {
		e.do(alice, "GET", "/v1/yearbooks?"+q, "").status(400, "")
	}
	e.do(alice, "GET", "/v1/yearbooks?limit=50", "").status(200, "")
	// A cursor taken from another user's list does not widen the view.
	var out struct{ Yearbooks []map[string]any }
	rec = e.do(bob, "GET", "/v1/yearbooks?cursor="+cursor, "").status(200, "")
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	for _, y := range out.Yearbooks {
		if y["title"] != "Bob's" {
			t.Fatalf("bob sees %v", y)
		}
	}
}

// AC3, AC4, AC7: the ownership matrix. Another user's book and a missing book look the same.
func TestOwnershipMatrix(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	bob := e.register("bob@example.com", "Bob")
	id := e.create(alice, "Alice's book")
	missing := "01J9Z3K6V8Q4M7N2P5R8T0W1XY"
	calls := []struct{ method, suffix, body string }{
		{"GET", "", ""},
		{"PATCH", "", `{"title":"hijack"}`},
		{"PUT", "/profile", `{"full_name":"hijack"}`},
		{"DELETE", "", ""},
	}
	var bodies []string
	for _, c := range calls {
		for _, target := range []string{id, missing, "not-a-ulid", "1"} {
			rec := e.do(bob, c.method, "/v1/yearbooks/"+target+c.suffix, c.body).status(404, "not_found")
			bodies = append(bodies, c.method+rec.Body.String()[:strings.Index(rec.Body.String(), `"request_id"`)])
		}
	}
	// identical bodies (apart from request_id) per verb for own-missing, foreign and garbage ids
	for i := 0; i < len(bodies); i += 4 {
		for _, b := range bodies[i : i+4] {
			if b != bodies[i] {
				t.Fatalf("404 bodies differ: %q vs %q", b, bodies[i])
			}
		}
	}
	// Alice's book is untouched.
	b := e.do(alice, "GET", "/v1/yearbooks/"+id, "").status(200, "").book()
	if b["title"] != "Alice's book" || b["profile"].(map[string]any)["full_name"] != "Alice" {
		t.Fatalf("book changed: %v", b)
	}
	if e.count(`SELECT COUNT(*) FROM yearbooks`) != 1 {
		t.Fatal("book count changed")
	}
}

// AC4: PATCH is partial, PUT profile replaces.
func TestPatchAndProfile(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	id := e.create(alice, "Book")
	e.do(alice, "PATCH", "/v1/yearbooks/"+id, `{"graduation_year":2027,"motto":"Carpe diem","class_name":"K65"}`).status(200, "")
	b := e.do(alice, "PATCH", "/v1/yearbooks/"+id, `{"title":"Renamed","graduation_year":null}`).status(200, "").book()
	if b["title"] != "Renamed" || b["motto"] != "Carpe diem" || b["class_name"] != "K65" || b["graduation_year"] != nil || b["language"] != "en" {
		t.Fatalf("patch result %v", b)
	}
	// Rejected patch changes nothing.
	e.do(alice, "PATCH", "/v1/yearbooks/"+id, `{"motto":"new","title":""}`).status(400, "invalid_title")
	e.do(alice, "PATCH", "/v1/yearbooks/"+id, `{"owner_id":5}`).status(400, "unknown_field")
	e.do(alice, "PATCH", "/v1/yearbooks/"+id, `{"profile":{"full_name":"x"}}`).status(400, "unknown_field")
	if g := e.do(alice, "GET", "/v1/yearbooks/"+id, "").book(); g["motto"] != "Carpe diem" {
		t.Fatalf("rejected patch applied: %v", g)
	}
	// Empty patch is a no-op that still succeeds.
	e.do(alice, "PATCH", "/v1/yearbooks/"+id, `{}`).status(200, "")

	full := `{"full_name":"Nguyễn Văn A","nickname":"Bống","birthday":"2004-02-29","quote":"Stay hungry","hobbies":"Chess","future_plans":"Engineer"}`
	p := e.do(alice, "PUT", "/v1/yearbooks/"+id+"/profile", full).status(200, "").book()["profile"].(map[string]any)
	if p["full_name"] != "Nguyễn Văn A" || p["birthday"] != "2004-02-29" || p["nickname"] != "Bống" || p["hobbies"] != "Chess" || p["is_owner"] != true {
		t.Fatalf("profile %v", p)
	}
	// Replace: omitted optional fields are cleared.
	p = e.do(alice, "PUT", "/v1/yearbooks/"+id+"/profile", `{"full_name":"Only Name"}`).status(200, "").book()["profile"].(map[string]any)
	if p["nickname"] != "" || p["birthday"] != nil || p["quote"] != "" || p["full_name"] != "Only Name" {
		t.Fatalf("not replaced: %v", p)
	}
	for body, code := range map[string]string{
		`{}`: "invalid_full_name",
		`{"full_name":"a","birthday":"2099-01-01"}`:                      "invalid_birthday",
		`{"full_name":"a","birthday":"2026-10-07"}`:                      "invalid_birthday",
		`{"full_name":"a","birthday":"2001-13-01"}`:                      "invalid_birthday",
		`{"full_name":"a","birthday":""}`:                                "invalid_birthday",
		`{"full_name":"a","nickname":"` + strings.Repeat("x", 51) + `"}`: "invalid_nickname",
		`{"full_name":"a","quote":"` + strings.Repeat("x", 501) + `"}`:   "invalid_quote",
		`{"full_name":"a","is_owner":false}`:                             "unknown_field",
		`{"full_name":"a","yearbook_id":2}`:                              "unknown_field",
		`{"full_name":"a","photo_media_id":2}`:                           "invalid_body", // a known field since T-009, but not a string
	} {
		e.do(alice, "PUT", "/v1/yearbooks/"+id+"/profile", body).status(400, code)
	}
	// Exactly one profile, still the owner's.
	if e.count(`SELECT COUNT(*) FROM profiles WHERE is_owner`) != 1 || e.count(`SELECT COUNT(*) FROM profiles`) != 1 {
		t.Fatal("profile rows changed")
	}
	// A profile edit counts as an edit of the book: it moves to the front.
	other := e.create(alice, "Newer")
	e.tick()
	e.do(alice, "PUT", "/v1/yearbooks/"+id+"/profile", `{"full_name":"Again"}`).status(200, "")
	var out struct{ Yearbooks []map[string]any }
	_ = json.Unmarshal(e.do(alice, "GET", "/v1/yearbooks", "").Body.Bytes(), &out)
	if out.Yearbooks[0]["id"] != id || out.Yearbooks[1]["id"] != other {
		t.Fatalf("order after profile edit: %v", out.Yearbooks)
	}
	// Birthday is personal data: it is never written to the logs.
	if strings.Contains(e.logs.String(), "2004-02-29") {
		t.Fatal("birthday found in logs")
	}
}

// AC6: delete cascades, then the id is gone; recreate works; the limit is 20 per user.
func TestDeleteAndLimit(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	bob := e.register("bob@example.com", "Bob")
	id := e.create(alice, "Doomed")
	e.do(alice, "DELETE", "/v1/yearbooks/"+id, "").status(204, "")
	e.do(alice, "GET", "/v1/yearbooks/"+id, "").status(404, "not_found")
	e.do(alice, "DELETE", "/v1/yearbooks/"+id, "").status(404, "not_found")
	if e.count(`SELECT COUNT(*) FROM yearbooks`) != 0 || e.count(`SELECT COUNT(*) FROM profiles`) != 0 {
		t.Fatal("rows left after delete")
	}
	e.create(alice, "Recreated")

	for i := 1; i < 20; i++ {
		e.create(alice, fmt.Sprintf("B%d", i))
	}
	e.do(alice, "POST", "/v1/yearbooks", `{"title":"21st","language":"en"}`).status(409, "limit_reached")
	e.create(bob, "Bob is unaffected")
	first := e.do(alice, "GET", "/v1/yearbooks?limit=50", "").Body.String()
	if n := strings.Count(first, `"language"`); n != 20 {
		t.Fatalf("alice has %d books", n)
	}
	// Freeing a slot allows a new book.
	var out struct{ Yearbooks []map[string]any }
	_ = json.Unmarshal([]byte(first), &out)
	e.do(alice, "DELETE", "/v1/yearbooks/"+out.Yearbooks[0]["id"].(string), "").status(204, "")
	e.do(alice, "POST", "/v1/yearbooks", `{"title":"21st","language":"en"}`).status(201, "")

	// Deleting the account removes its books and profiles (foreign-key cascade).
	if _, err := e.db.Exec(`DELETE FROM users WHERE email = 'alice@example.com'`); err != nil {
		t.Fatal(err)
	}
	if e.count(`SELECT COUNT(*) FROM yearbooks`) != 1 || e.count(`SELECT COUNT(*) FROM profiles`) != 1 {
		t.Fatal("cascade from users did not remove alice's books")
	}
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
			codes <- e.do(alice, "POST", "/v1/yearbooks", `{"title":"x","language":"en"}`).Code
		}()
	}
	wg.Wait()
	close(codes)
	created := 0
	for c := range codes {
		switch c {
		case 201:
			created++
		case 409:
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if created != 20 || e.count(`SELECT COUNT(*) FROM yearbooks`) != 20 {
		t.Fatalf("created %d, want 20", created)
	}
}

// AC7: every endpoint needs a session, and writes with a cookie need an allowed origin.
func TestAuthRequired(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	id := e.create(alice, "Book")
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/v1/yearbooks", `{"title":"x","language":"en"}`},
		{"GET", "/v1/yearbooks", ""},
		{"GET", "/v1/yearbooks/" + id, ""},
		{"PATCH", "/v1/yearbooks/" + id, `{"title":"x"}`},
		{"PUT", "/v1/yearbooks/" + id + "/profile", `{"full_name":"x"}`},
		{"DELETE", "/v1/yearbooks/" + id, ""},
	} {
		e.do(user{}, c.method, c.path, c.body).status(401, "unauthenticated")
		e.do(user{"bogus-token"}, c.method, c.path, c.body).status(401, "unauthenticated")
	}
	// Cross-site write with a valid cookie.
	r := httptest.NewRequest("DELETE", "/v1/yearbooks/"+id, nil)
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: alice.cookie})
	r.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatalf("cross-origin delete: %d", rec.Code)
	}
	e.do(alice, "GET", "/v1/yearbooks/"+id, "").status(200, "")
	// Wrong content type on a body.
	r = httptest.NewRequest("POST", "/v1/yearbooks", strings.NewReader(`{"title":"x","language":"en"}`))
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: alice.cookie})
	r.Header.Set("Origin", origin)
	r.Header.Set("Content-Type", "text/plain")
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 415 {
		t.Fatalf("text/plain: %d", rec.Code)
	}
}
