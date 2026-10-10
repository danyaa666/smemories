//go:build integration

package notes

import (
	"bytes"
	"context"
	"crypto/sha256"
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
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notefields"
	"github.com/danyaa666/smemories/internal/redis/redistest"
	"github.com/danyaa666/smemories/internal/storage/storagetest"
	"github.com/danyaa666/smemories/internal/yearbook"
)

// What the lookup returns for the default form (the JSON shape itself is fixed by the notefields tests).
var defaultFieldsJSON = func() string {
	info, _ := notefields.Info(notefields.Default())
	b, _ := json.Marshal(info)
	return string(b)
}()

const (
	origin = "http://localhost:5173"
	testPW = "Tr0ub4dor&3-fake-test-password" // obviously fake
)

type env struct {
	t     *testing.T
	db    *sql.DB
	h     http.Handler
	nh    *Handler
	st    *flakyStore
	svc   *media.Service
	books []string // public ids, for object cleanup
	logs  *bytes.Buffer
	tmp   string // where the handler spools request bodies
	mu    sync.Mutex
	clock time.Time
}

func (e *env) now() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.clock
}

func (e *env) advance(d time.Duration) {
	e.mu.Lock()
	e.clock = e.clock.Add(d)
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
	sessions, err := auth.NewSessions(context.Background(), rc, logger)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000},
		auth.Mail{Mailer: mail, Logger: logger}, codes, e.now)
	if err != nil {
		t.Fatal(err)
	}
	ah := auth.NewHandler(svc, auth.HandlerConfig{AllowedOrigins: []string{origin}}, logger)
	e.st = &flakyStore{Storage: storagetest.New(t), keys: map[string]bool{}}
	e.svc = media.NewService(media.NewStore(d), e.st, 2, nil)
	bookStore := yearbook.NewStore(d)
	e.svc.SetRefClearer(bookStore)
	yh := yearbook.NewHandler(bookStore, yearbook.NewService(bookStore, e.svc), ah.RequireUser, []string{origin}, logger, e.now)
	e.nh = NewHandler(NewStore(d), e.svc, 10<<20, ah.RequireUser, []string{origin}, ah.ClientIP, logger, e.now)
	e.tmp = t.TempDir()
	e.nh.SetUploadLimits(defaultUploadConns, defaultUploadsPerIP, e.tmp)
	e.h = httpx.NewRouter(logger, ah.Routes, yh.Routes, e.nh.Routes)
	t.Cleanup(func() { // objects of every book this test created
		for _, id := range e.books {
			_ = e.st.Storage.DeletePrefix(context.Background(), "yearbooks/"+id+"/")
		}
	})
	return e
}

type user struct{ cookie string }

// register creates an account; verified accounts get email_verified_at set directly (T-007 covers the mail flow).
func (e *env) register(email, name string, verified bool) user {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": testPW, "display_name": name})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/v1/auth/register", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(rec, r)
	if rec.Code != 201 {
		e.t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	if verified {
		if _, err := e.db.Exec(`UPDATE users SET email_verified_at = ? WHERE email = ?`, e.now(), email); err != nil {
			e.t.Fatal(err)
		}
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

func (r resp) json() map[string]any {
	r.t.Helper()
	var out map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil {
		r.t.Fatalf("not JSON: %s", r.Body.String())
	}
	return out
}

func (e *env) book(u user, title string) string {
	e.t.Helper()
	id := e.do(u, "POST", "/v1/yearbooks", fmt.Sprintf(`{"title":%q,"language":"en"}`, title)).status(201, "").
		json()["yearbook"].(map[string]any)["id"].(string)
	e.books = append(e.books, id)
	return id
}

// newLink creates a link and returns its collection id and token.
func (e *env) newLink(u user, bookID, body string) (id, token string) {
	e.t.Helper()
	c := e.do(u, "POST", "/v1/yearbooks/"+bookID+"/collections", body).status(201, "").json()["collection"].(map[string]any)
	return c["id"].(string), c["token"].(string)
}

func (e *env) lookup(token string) resp { return e.do(user{}, "GET", "/v1/public/collect/"+token, "") }

// AC1: the response, the stored hash, label and deadline handling.
func TestCreate(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Đặng Thị Hồng", true)
	book := e.book(alice, "Class 12A")
	deadline := e.now().Add(48 * time.Hour).Format(time.RFC3339)
	rec := e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", fmt.Sprintf(`{"label":"  Café friends 🎓 ","deadline_at":%q}`, deadline)).status(201, "")
	c := rec.json()["collection"].(map[string]any)
	token := c["token"].(string)
	if len(token) != 32 || len(c["id"].(string)) != 26 || c["label"] != "Café friends 🎓" || c["deadline_at"] != deadline || c["created_at"] == nil || len(c) != 5 {
		t.Fatalf("unexpected collection %v", c)
	}
	hash := sha256.Sum256([]byte(token))
	if e.count(`SELECT COUNT(*) FROM note_collections WHERE token_hash = ?`, hash[:]) != 1 {
		t.Fatal("sha256 of the token is not stored")
	}
	var stored int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM note_collections WHERE ? IN (public_id, label) OR HEX(token_hash) = ?`, token, token).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("raw token stored: %d %v", stored, err)
	}
	// no label, no deadline: both come back empty/null
	c2 := e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{}`).status(201, "").json()["collection"].(map[string]any)
	if c2["label"] != "" || c2["deadline_at"] != nil || c2["token"] == token {
		t.Fatalf("unexpected %v", c2)
	}
}

func (e *env) count(q string, args ...any) int {
	e.t.Helper()
	var n int
	if err := e.db.QueryRow(q, args...).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// AC1: validation. Each rejected request creates nothing.
func TestCreateValidation(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	at := func(d time.Duration) string { return e.now().Add(d).Format(time.RFC3339) }
	bad := []struct{ body, code string }{
		{`{"label":"` + strings.Repeat("a", 61) + `"}`, "invalid_label"},
		{`{"label":"bell\u0007"}`, "invalid_label"},
		{"{\"label\":\"zero\\u200bwidth\"}", "invalid_label"},
		{"{\"label\":\"rtl\\u202eoverride\"}", "invalid_label"},
		{`{"deadline_at":"` + at(-time.Hour) + `"}`, "invalid_deadline"},
		{`{"deadline_at":"` + at(0) + `"}`, "invalid_deadline"},
		{`{"deadline_at":"` + at(366*24*time.Hour+time.Hour) + `"}`, "invalid_deadline"},
		{`{"deadline_at":"2100-01-01T00:00:00Z"}`, "invalid_deadline"},
		{`{"deadline_at":"tomorrow"}`, "invalid_deadline"},
		{`{"deadline_at":"2026-10-09"}`, "invalid_deadline"},
		{`{"deadline_at":""}`, "invalid_deadline"},
		{`{"deadline_at":5}`, "invalid_body"},
		{`{"revoked_at":"2026-10-09T00:00:00Z"}`, "unknown_field"},
		{`{"token":"x"}`, "unknown_field"},
		{`not json`, "invalid_body"},
	}
	for _, c := range bad {
		e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", c.body).status(400, c.code)
	}
	if n := e.count(`SELECT COUNT(*) FROM note_collections`); n != 0 {
		t.Fatalf("%d rows created by rejected requests", n)
	}
	// edges that are accepted: 1 s ahead, a time zone offset, 60-character label, exactly one year
	e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{"deadline_at":"`+e.now().Add(time.Second).Format(time.RFC3339)+`"}`).status(201, "")
	c := e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{"deadline_at":"2026-10-09T09:00:00+07:00","label":"`+strings.Repeat("ế", 60)+`"}`).status(201, "").json()["collection"].(map[string]any)
	if c["deadline_at"] != "2026-10-09T02:00:00Z" {
		t.Fatalf("deadline not normalised to UTC: %v", c["deadline_at"])
	}
	e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{"deadline_at":"`+e.now().AddDate(1, 0, 0).Format(time.RFC3339)+`"}`).status(201, "")
}

// AC2, AC4: ownership matrix across every owner endpoint, and the verified-email rule.
func TestOwnership(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	bob := e.register("bob@example.com", "Bob", true)
	carol := e.register("carol@example.com", "Carol", false)
	aBook := e.book(alice, "Alice's book")
	id, token := e.newLink(alice, aBook, `{}`)

	for _, who := range []user{bob, carol} {
		e.do(who, "GET", "/v1/yearbooks/"+aBook+"/collections", "").status(404, "not_found")
		e.do(who, "DELETE", "/v1/collections/"+id, "").status(404, "not_found")
	}
	e.do(bob, "POST", "/v1/yearbooks/"+aBook+"/collections", `{}`).status(404, "not_found")
	for _, missing := range []string{"01ARZ3NDEKTSV4RRFFQ69G5FAV", "nope", strings.Repeat("x", 500)} {
		e.do(alice, "POST", "/v1/yearbooks/"+missing+"/collections", `{}`).status(404, "not_found")
		e.do(alice, "GET", "/v1/yearbooks/"+missing+"/collections", "").status(404, "not_found")
		e.do(alice, "DELETE", "/v1/collections/"+missing, "").status(404, "not_found")
	}
	e.lookup(token).status(200, "") // nothing above touched the link
	if e.count(`SELECT COUNT(*) FROM note_collections WHERE revoked_at IS NOT NULL`) != 0 {
		t.Fatal("a foreign request revoked a link")
	}
	// anonymous callers and callers using the link token as a credential
	e.do(user{}, "POST", "/v1/yearbooks/"+aBook+"/collections", `{}`).status(401, "unauthenticated")
	e.do(user{}, "GET", "/v1/yearbooks/"+aBook+"/collections", "").status(401, "unauthenticated")
	e.do(user{}, "DELETE", "/v1/collections/"+id, "").status(401, "unauthenticated")
	if rec := e.do(user{token}, "GET", "/v1/yearbooks/"+aBook+"/collections", ""); rec.Code != 401 {
		t.Fatalf("token accepted as a session: %d", rec.Code)
	}

	// unverified owner: 403 on create only; own book, so nothing else is in the way
	cBook := e.book(carol, "Carol's book")
	e.do(carol, "POST", "/v1/yearbooks/"+cBook+"/collections", `{}`).status(403, "email_not_verified")
	e.do(carol, "GET", "/v1/yearbooks/"+cBook+"/collections", "").status(200, "")
	if e.count(`SELECT COUNT(*) FROM note_collections`) != 1 {
		t.Fatal("unverified owner created a link")
	}
}

// AC2: at most five active links per book, also under concurrency; revoking frees a slot and the new token differs.
func TestLimit(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	var first, firstToken string
	for i := range 5 {
		id, tok := e.newLink(alice, book, `{}`)
		if i == 0 {
			first, firstToken = id, tok
		}
	}
	e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{}`).status(409, "limit_reached")
	e.do(alice, "DELETE", "/v1/collections/"+first, "").status(204, "")
	_, tok := e.newLink(alice, book, `{}`)
	if tok == firstToken {
		t.Fatal("new link reuses the revoked token")
	}
	e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{}`).status(409, "limit_reached")

	// another book of the same owner has its own budget
	other := e.book(alice, "Other")
	e.newLink(alice, other, `{}`)

	var wg sync.WaitGroup
	codes := make(chan int, 12)
	book2 := e.book(alice, "Racy")
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- e.do(alice, "POST", "/v1/yearbooks/"+book2+"/collections", `{}`).Code
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		switch c {
		case 201:
			ok++
		case 409:
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok != 5 || e.count(`SELECT COUNT(*) FROM note_collections c JOIN yearbook_tab y ON y.id = c.yearbook_id WHERE y.public_id = ?`, book2) != 5 {
		t.Fatalf("%d concurrent creates succeeded, want 5", ok)
	}
}

// AC3: newest first, revoked ones included, never a token or hash.
func TestList(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	e.do(alice, "GET", "/v1/yearbooks/"+book+"/collections", "").status(200, "")
	if got := e.do(alice, "GET", "/v1/yearbooks/"+book+"/collections", "").Body.String(); !strings.Contains(got, `"collections":[]`) {
		t.Fatalf("empty list should be []: %s", got)
	}
	var ids, tokens []string
	for i := range 3 {
		e.advance(time.Minute)
		id, tok := e.newLink(alice, book, fmt.Sprintf(`{"label":"link %d"}`, i))
		ids, tokens = append(ids, id), append(tokens, tok)
	}
	e.advance(time.Minute)
	e.do(alice, "DELETE", "/v1/collections/"+ids[1], "").status(204, "")
	rec := e.do(alice, "GET", "/v1/yearbooks/"+book+"/collections", "").status(200, "")
	list := rec.json()["collections"].([]any)
	if len(list) != 3 {
		t.Fatalf("got %d collections", len(list))
	}
	for i, want := range []int{2, 1, 0} {
		c := list[i].(map[string]any)
		if c["id"] != ids[want] || c["label"] != fmt.Sprintf("link %d", want) || c["note_count"] != float64(0) || len(c) != 6 {
			t.Fatalf("item %d: %v", i, c)
		}
		if (c["revoked_at"] != nil) != (want == 1) {
			t.Fatalf("item %d revoked_at %v", i, c["revoked_at"])
		}
	}
	body := rec.Body.String()
	for _, tok := range tokens {
		h := sha256.Sum256([]byte(tok))
		if strings.Contains(body, tok) || strings.Contains(body, fmt.Sprintf("%x", h)) || strings.Contains(strings.ToLower(body), "token") {
			t.Fatalf("list leaks a token or hash: %s", body)
		}
	}
}

// AC4: revoke is idempotent, keeps its first timestamp and is final.
func TestRevoke(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	id, token := e.newLink(alice, book, `{}`)
	e.lookup(token).status(200, "")
	e.do(alice, "DELETE", "/v1/collections/"+id, "").status(204, "")
	var first time.Time
	if err := e.db.QueryRow(`SELECT revoked_at FROM note_collections WHERE public_id = ?`, id).Scan(&first); err != nil {
		t.Fatal(err)
	}
	e.advance(time.Hour)
	e.do(alice, "DELETE", "/v1/collections/"+id, "").status(204, "")
	var second time.Time
	if err := e.db.QueryRow(`SELECT revoked_at FROM note_collections WHERE public_id = ?`, id).Scan(&second); err != nil || !first.Equal(second) {
		t.Fatalf("revoked_at moved: %v -> %v (%v)", first, second, err)
	}
	e.lookup(token).status(404, "not_found")
}

// AC5, AC6: the public lookup. Unknown, malformed and revoked tokens look the same; an expired one is 410.
func TestPublicLookup(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Đặng Thị Hồng", true)
	book := e.book(alice, "Class 12A")
	deadline := e.now().Add(time.Hour).Format(time.RFC3339)
	_, token := e.newLink(alice, book, fmt.Sprintf(`{"deadline_at":%q}`, deadline))
	rec := e.lookup(token).status(200, "")
	want := fmt.Sprintf(`{"yearbook":{"title":"Class 12A"},"owner":{"display_name":"Đặng Thị Hồng"},"deadline_at":%q,"open":true,"fields":%s}`, deadline, defaultFieldsJSON)
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("body %s\nwant %s", rec.Body.String(), want)
	}
	_, noDeadline := e.newLink(alice, book, `{}`)
	if got := e.lookup(noDeadline).status(200, "").json(); got["deadline_at"] != nil || got["open"] != true {
		t.Fatalf("no-deadline lookup: %v", got)
	}

	// a session cookie is ignored: another user's, or a broken one, changes nothing and nothing is set
	bob := e.register("bob@example.com", "Bob", true)
	r := e.do(bob, "GET", "/v1/public/collect/"+token, "").status(200, "")
	for _, rr := range []resp{rec, r} {
		if len(rr.Result().Cookies()) != 0 || rr.Header().Get("Set-Cookie") != "" {
			t.Fatal("public lookup set a cookie")
		}
		for k := range rr.Header() {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Fatalf("CORS header %s", k)
			}
		}
	}
	if rec := e.do(user{"garbage"}, "GET", "/v1/public/collect/"+token, ""); rec.Code != 200 {
		t.Fatalf("bad session cookie broke the public lookup: %d", rec.Code)
	}

	// 404s are indistinguishable (compared without the per-request id)
	revID, revoked := e.newLink(alice, book, `{}`)
	e.do(alice, "DELETE", "/v1/collections/"+revID, "").status(204, "")
	var bodies []string
	for _, tok := range []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "short", strings.Repeat("a", 10_000), "a%2Fb", "%00", token + "x", strings.Repeat("-", 32), revoked} {
		rec := e.lookup(tok).status(404, "not_found")
		var env map[string]map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		delete(env["error"], "request_id")
		bodies = append(bodies, fmt.Sprint(env))
	}
	for _, b := range bodies {
		if b != bodies[0] {
			t.Fatalf("404 bodies differ: %q vs %q", b, bodies[0])
		}
	}

	// expired: exactly at the deadline is closed, a second before is open
	e.advance(time.Hour - time.Second)
	e.lookup(token).status(200, "")
	e.advance(time.Second)
	e.lookup(token).status(410, "collection_closed")
	e.lookup(noDeadline).status(200, "")
}

// T-034 leader note: only misses count against the tight per-IP limit, so a class behind one address can open the link;
// a high cap on all requests stays as a cost guard. The token never reaches the logs.
func TestPublicRateLimitAndLogs(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	id, token := e.newLink(alice, book, `{}`)
	for i := range 59 { // 59 misses and 500 hits from one IP: still fine
		e.lookup(fmt.Sprintf("%032d", i)).status(404, "not_found")
	}
	for range 500 {
		e.lookup(token).status(200, "")
	}
	e.lookup("x").status(404, "not_found") // the 60th miss
	rec := e.lookup(token).status(429, "rate_limited")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.advance(15*time.Minute + time.Second)
	e.lookup(token).status(200, "")
	// the cost guard: 600 requests of any kind per 15 minutes
	for range 599 {
		e.lookup(token)
	}
	e.lookup(token).status(429, "rate_limited")
	e.advance(15*time.Minute + time.Second)
	// the owner endpoints are not limited by the public counters
	e.do(alice, "GET", "/v1/yearbooks/"+book+"/collections", "").status(200, "")
	e.do(alice, "DELETE", "/v1/collections/"+id, "").status(204, "")

	hash := sha256.Sum256([]byte(token))
	logs := e.logs.String()
	if strings.Contains(logs, token) || strings.Contains(logs, fmt.Sprintf("%x", hash)) {
		t.Fatal("token or hash in the logs")
	}
	if !strings.Contains(logs, "GET /v1/public/collect/{token}") {
		t.Fatal("access log should record the route pattern")
	}
}

// A different client IP has its own counter (the limit is per IP, not global).
func TestPublicRateLimitPerIP(t *testing.T) {
	e := newEnv(t)
	for range 60 {
		e.lookup("x")
	}
	e.lookup("x").status(429, "rate_limited")
	r := httptest.NewRequest("GET", "/v1/public/collect/x", nil)
	r.RemoteAddr = "198.51.100.7:4000"
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	if rec.Code != 404 {
		t.Fatalf("second IP: %d", rec.Code)
	}
}

// AC7: deleting a yearbook deletes its links and only its links.
func TestYearbookDeleteCascades(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	b1, b2 := e.book(alice, "One"), e.book(alice, "Two")
	_, t1 := e.newLink(alice, b1, `{}`)
	_, t2 := e.newLink(alice, b2, `{}`)
	e.do(alice, "DELETE", "/v1/yearbooks/"+b1, "").status(204, "")
	if n := e.count(`SELECT COUNT(*) FROM note_collections`); n != 1 {
		t.Fatalf("%d links left, want 1", n)
	}
	e.lookup(t1).status(404, "not_found")
	e.lookup(t2).status(200, "")
}

// The origin check of the repo's Guard covers the new write endpoints.
func TestCSRFGuard(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	id, _ := e.newLink(alice, book, `{}`)
	for _, c := range [][2]string{{"POST", "/v1/yearbooks/" + book + "/collections"}, {"DELETE", "/v1/collections/" + id}} {
		r := httptest.NewRequest(c[0], c[1], strings.NewReader(`{}`))
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("Origin", "https://evil.example")
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: alice.cookie})
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, r)
		if rec.Code != 403 {
			t.Fatalf("%s %s from a foreign origin: %d", c[0], c[1], rec.Code)
		}
	}
}
