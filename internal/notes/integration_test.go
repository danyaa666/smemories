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
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/legal"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notefields"
	"github.com/danyaa666/smemories/internal/ratelimit"
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
	ns    *Service
	st    *flakyStore
	svc   *media.Service
	books []string // public ids, for object cleanup
	logs  *bytes.Buffer
	tmp   string // where the handler spools request bodies
	lim   *ratelimit.Factory
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
	e.lim = ratelimit.NewFactory(rc, logger, e.now)
	sessions, err := auth.NewSessions(context.Background(), rc, logger)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 1000},
		auth.Mail{Mailer: mail, Logger: logger}, codes, e.lim, e.now)
	if err != nil {
		t.Fatal(err)
	}
	ah := auth.NewHandler(svc, auth.HandlerConfig{AllowedOrigins: []string{origin}}, logger)
	e.st = &flakyStore{Storage: storagetest.New(t), keys: map[string]bool{}}
	e.svc = media.NewService(media.NewStore(d), e.st, 2, e.lim, nil)
	bookStore := yearbook.NewStore(d)
	notesStore := NewStore(d)
	notesSvc := NewService(notesStore, e.svc, httpx.NewCursors([]byte(strings.Repeat("c", 32))), e.lim, logger, e.now)
	e.ns = notesSvc
	e.svc.SetRefClearer(bookStore, notesSvc)
	yh := yearbook.NewHandler(yearbook.NewService(bookStore, httpx.NewCursors([]byte(strings.Repeat("c", 32))), e.svc, e.now, e.svc, notesSvc), ah.RequireUser, []string{origin})
	e.nh = NewHandler(notesSvc, 10<<20, ah.RequireUser, []string{origin}, ah.ClientIP, logger)
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
	b, _ := json.Marshal(map[string]string{"email": email, "password": testPW, "display_name": name, "accept_terms_version": legal.CurrentVersion})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/auth/register", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		e.t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	if verified {
		if _, err := e.db.Exec(`UPDATE user_tab SET email_verified_at = ? WHERE email = ?`, e.now().UnixMilli(), email); err != nil {
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

// code is the error code of a response body ("" when the body is no error): the v2 envelope's status.
func (r resp) code() string {
	var b struct{ Status string }
	_ = json.Unmarshal(r.Body.Bytes(), &b)
	if strings.HasPrefix(b.Status, "ERROR_") {
		return b.Status
	}
	return ""
}

// status asserts the HTTP status and, when code is not empty, the error code (v2 spelling). A v2 success must say status "OK".
func (r resp) status(want int, code string) resp {
	r.t.Helper()
	if r.Code != want {
		r.t.Fatalf("status %d, want %d (%s)", r.Code, want, r.Body.String())
	}
	if code != "" && r.code() != code {
		r.t.Fatalf("code %q, want %q: %s", r.code(), code, r.Body.String())
	}
	return r
}

// data is the "data" object of a v2 success body; it fails unless the body is {"status":"OK","data":{...}}.
func (r resp) data() map[string]any {
	r.t.Helper()
	b := r.json()
	d, ok := b["data"].(map[string]any)
	if b["status"] != "OK" || !ok || len(b) != 2 {
		r.t.Fatalf("not a v2 success body: %s", r.Body.String())
	}
	return d
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
	id := e.do(u, "POST", "/api/yearbook/create", fmt.Sprintf(`{"title":%q,"language":"en"}`, title)).status(200, "").
		data()["id"].(string)
	e.books = append(e.books, id)
	return id
}

// The yearbook endpoints, which the tests use for setup and for the cross-domain checks.
func withID(key, id, body string) string {
	rest := strings.TrimPrefix(body, "{")
	if strings.TrimSpace(rest) != "}" {
		rest = "," + rest
	}
	return `{"` + key + `":"` + id + `"` + rest
}

func (e *env) updateBook(u user, id, body string) resp {
	return e.do(u, "POST", "/api/yearbook/update", withID("id", id, body))
}
func (e *env) updateProfile(u user, id, body string) resp {
	return e.do(u, "POST", "/api/yearbook/update-profile", withID("yearbook_id", id, body))
}
func (e *env) deleteBook(u user, id string) resp {
	return e.do(u, "POST", "/api/yearbook/delete", withID("id", id, "{}"))
}

// create calls POST /api/note-collection/create for a book; body is a JSON object of the other fields ("{}" for none).
func (e *env) create(u user, bookID, body string) resp {
	e.t.Helper()
	if strings.HasPrefix(body, "{") { // anything else is sent as it is (a malformed body)
		rest := "}"
		if body != "{}" {
			rest = "," + body[1:]
		}
		body = fmt.Sprintf(`{"yearbook_id":%q`, bookID) + rest
	}
	return e.do(u, "POST", "/api/note-collection/create", body)
}

func (e *env) list(u user, bookID string) resp {
	e.t.Helper()
	return e.do(u, "GET", "/api/note-collection/get-list?yearbook_id="+url.QueryEscape(bookID), "")
}

func (e *env) revoke(u user, id string) resp {
	e.t.Helper()
	return e.do(u, "POST", "/api/note-collection/revoke", fmt.Sprintf(`{"id":%q}`, id))
}

// newLink creates a link and returns its collection id and token.
func (e *env) newLink(u user, bookID, body string) (id, token string) {
	e.t.Helper()
	c := e.create(u, bookID, body).status(200, "").data()
	return c["id"].(string), c["token"].(string)
}

func lookupPath(token string) string {
	return "/api/public-collect/get?token=" + url.QueryEscape(token)
}

func (e *env) lookup(token string) resp { return e.do(user{}, "GET", lookupPath(token), "") }

// AC1: the response, the stored hash, label and deadline handling.
func TestCreate(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Đặng Thị Hồng", true)
	book := e.book(alice, "Class 12A")
	deadline := e.now().Add(48 * time.Hour).UnixMilli()
	rec := e.create(alice, book, fmt.Sprintf(`{"label":"  Café friends 🎓 ","deadline_at":%d}`, deadline)).status(200, "")
	c := rec.data()
	token := c["token"].(string)
	if len(token) != 32 || len(c["id"].(string)) != 26 || c["label"] != "Café friends 🎓" || c["deadline_at"] != float64(deadline) || c["created_at"] != float64(e.now().UnixMilli()) || len(c) != 5 {
		t.Fatalf("unexpected collection %v", c)
	}
	hash := sha256.Sum256([]byte(token))
	if e.count(`SELECT COUNT(*) FROM note_collection_tab WHERE token_hash = ?`, hash[:]) != 1 {
		t.Fatal("sha256 of the token is not stored")
	}
	var stored int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM note_collection_tab WHERE ? IN (public_id, label) OR HEX(token_hash) = ?`, token, token).Scan(&stored); err != nil || stored != 0 {
		t.Fatalf("raw token stored: %d %v", stored, err)
	}
	// no label, no deadline: both come back empty/null
	c2 := e.create(alice, book, `{}`).status(200, "").data()
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
	at := func(d time.Duration) int64 { return e.now().Add(d).UnixMilli() }
	bad := []struct{ body, code string }{
		{`{"label":"` + strings.Repeat("a", 61) + `"}`, "ERROR_INVALID_LABEL"},
		{`{"label":"bell\u0007"}`, "ERROR_INVALID_LABEL"},
		{"{\"label\":\"zero\\u200bwidth\"}", "ERROR_INVALID_LABEL"},
		{"{\"label\":\"rtl\\u202eoverride\"}", "ERROR_INVALID_LABEL"},
		{fmt.Sprintf(`{"deadline_at":%d}`, at(-time.Hour)), "ERROR_INVALID_DEADLINE"},
		{fmt.Sprintf(`{"deadline_at":%d}`, at(0)), "ERROR_INVALID_DEADLINE"},
		{fmt.Sprintf(`{"deadline_at":%d}`, at(366*24*time.Hour+time.Hour)), "ERROR_INVALID_DEADLINE"},
		{`{"deadline_at":4102444800000}`, "ERROR_INVALID_DEADLINE"},
		{`{"deadline_at":0}`, "ERROR_INVALID_DEADLINE"},
		{`{"deadline_at":-1}`, "ERROR_INVALID_DEADLINE"},
		{`{"deadline_at":"2026-10-09T00:00:00Z"}`, "ERROR_PARAM"}, // the old RFC 3339 string is not a number
		{`{"deadline_at":1.5e12}`, "ERROR_PARAM"},                 // an integer only
		{`{"deadline_at":""}`, "ERROR_PARAM"},
		{`{"yearbook_id":5}`, "ERROR_PARAM"},
		{`{"label":5}`, "ERROR_PARAM"},
		{`{"revoked_at":"2026-10-09T00:00:00Z"}`, "ERROR_PARAM"},
		{`{"token":"x"}`, "ERROR_PARAM"},
		{`not json`, "ERROR_PARAM"},
	}
	for _, c := range bad {
		e.create(alice, book, c.body).status(400, c.code)
	}
	if n := e.count(`SELECT COUNT(*) FROM note_collection_tab`); n != 0 {
		t.Fatalf("%d rows created by rejected requests", n)
	}
	// edges that are accepted: 1 s ahead, a time zone offset, 60-character label, exactly one year
	e.create(alice, book, fmt.Sprintf(`{"deadline_at":%d}`, e.now().Add(time.Second).UnixMilli())).status(200, "")
	c := e.create(alice, book, fmt.Sprintf(`{"deadline_at":null,"label":"%s"}`, strings.Repeat("ế", 60))).status(200, "").data()
	if c["deadline_at"] != nil || c["label"] != strings.Repeat("ế", 60) {
		t.Fatalf("null deadline or 60-character label: %v", c)
	}
	e.create(alice, book, fmt.Sprintf(`{"deadline_at":%d}`, e.now().AddDate(1, 0, 0).UnixMilli())).status(200, "")
	e.do(alice, "POST", "/api/note-collection/create", `{"label":"x"}`).status(400, "ERROR_PARAM") // no yearbook_id
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
		e.list(who, aBook).status(404, "ERROR_NOT_FOUND")
		e.revoke(who, id).status(404, "ERROR_NOT_FOUND")
	}
	e.create(bob, aBook, `{}`).status(404, "ERROR_NOT_FOUND")
	missing := "01ARZ3NDEKTSV4RRFFQ69G5FAV"
	e.create(alice, missing, `{}`).status(404, "ERROR_NOT_FOUND")
	e.list(alice, missing).status(404, "ERROR_NOT_FOUND")
	e.revoke(alice, missing).status(404, "ERROR_NOT_FOUND")
	// T-100 (audit M3): anything that is not a canonical id, including the real ids in another spelling, is refused before the database.
	for _, bad := range []string{"nope", strings.Repeat("x", 500), strings.ToLower(aBook), strings.ToLower(id), " " + aBook} {
		e.create(alice, bad, `{}`).status(400, "ERROR_PARAM")
		e.list(alice, bad).status(400, "ERROR_PARAM")
		e.revoke(alice, bad).status(400, "ERROR_PARAM")
	}
	e.lookup(token).status(200, "") // nothing above touched the link
	if e.count(`SELECT COUNT(*) FROM note_collection_tab WHERE revoked_at IS NOT NULL`) != 0 {
		t.Fatal("a foreign request revoked a link")
	}
	// anonymous callers and callers using the link token as a credential
	e.create(user{}, aBook, `{}`).status(401, "ERROR_UNAUTHORIZED")
	e.list(user{}, aBook).status(401, "ERROR_UNAUTHORIZED")
	e.revoke(user{}, id).status(401, "ERROR_UNAUTHORIZED")
	if rec := e.list(user{token}, aBook); rec.Code != 401 {
		t.Fatalf("token accepted as a session: %d", rec.Code)
	}

	// unverified owner: 403 on create only; own book, so nothing else is in the way
	cBook := e.book(carol, "Carol's book")
	e.create(carol, cBook, `{}`).status(403, "ERROR_EMAIL_NOT_VERIFIED")
	e.list(carol, cBook).status(200, "")
	if e.count(`SELECT COUNT(*) FROM note_collection_tab`) != 1 {
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
	e.create(alice, book, `{}`).status(409, "ERROR_LIMIT_REACHED")
	e.revoke(alice, first).status(200, "")
	_, tok := e.newLink(alice, book, `{}`)
	if tok == firstToken {
		t.Fatal("new link reuses the revoked token")
	}
	e.create(alice, book, `{}`).status(409, "ERROR_LIMIT_REACHED")

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
			codes <- e.create(alice, book2, `{}`).Code
		}()
	}
	wg.Wait()
	close(codes)
	ok := 0
	for c := range codes {
		switch c {
		case 200:
			ok++
		case 409:
		default:
			t.Fatalf("unexpected status %d", c)
		}
	}
	if ok != 5 || e.count(`SELECT COUNT(*) FROM note_collection_tab c JOIN yearbook_tab y ON y.id = c.yearbook_id WHERE y.public_id = ?`, book2) != 5 {
		t.Fatalf("%d concurrent creates succeeded, want 5", ok)
	}
}

// AC3: newest first, revoked ones included, never a token or hash.
func TestList(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	e.list(alice, book).status(200, "")
	if got := e.list(alice, book).Body.String(); !strings.Contains(got, `"items":[]`) {
		t.Fatalf("empty list should be []: %s", got)
	}
	var ids, tokens []string
	for i := range 3 {
		e.advance(time.Minute)
		id, tok := e.newLink(alice, book, fmt.Sprintf(`{"label":"link %d"}`, i))
		ids, tokens = append(ids, id), append(tokens, tok)
	}
	e.advance(time.Minute)
	e.revoke(alice, ids[1]).status(200, "")
	rec := e.list(alice, book).status(200, "")
	data := rec.data()
	list := data["items"].([]any)
	if data["next_id"] != "" || len(data) != 2 {
		t.Fatalf("list data: %v", data)
	}
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
	e.revoke(alice, id).status(200, "")
	var first int64
	if err := e.db.QueryRow(`SELECT revoked_at FROM note_collection_tab WHERE public_id = ?`, id).Scan(&first); err != nil {
		t.Fatal(err)
	}
	e.advance(time.Hour)
	e.revoke(alice, id).status(200, "")
	var second int64
	if err := e.db.QueryRow(`SELECT revoked_at FROM note_collection_tab WHERE public_id = ?`, id).Scan(&second); err != nil || first != second || first != e.now().Add(-time.Hour).UnixMilli() {
		t.Fatalf("revoked_at moved: %v -> %v (%v)", first, second, err)
	}
	e.lookup(token).status(404, "ERROR_NOT_FOUND")
}

// AC5, AC6: the public lookup. Unknown, malformed and revoked tokens look the same; an expired one is 410.
func TestPublicLookup(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Đặng Thị Hồng", true)
	book := e.book(alice, "Class 12A")
	deadline := e.now().Add(time.Hour).UnixMilli()
	_, token := e.newLink(alice, book, fmt.Sprintf(`{"deadline_at":%d}`, deadline))
	rec := e.lookup(token).status(200, "")
	want := fmt.Sprintf(`{"status":"OK","data":{"yearbook":{"title":"Class 12A"},"owner":{"display_name":"Đặng Thị Hồng"},"deadline_at":%d,"open":true,"fields":%s,"signature":{"enabled":false,"w":1000,"h":400,"max_strokes":12,"max_points":600}}}`, deadline, defaultFieldsJSON)
	if strings.TrimSpace(rec.Body.String()) != want {
		t.Fatalf("body %s\nwant %s", rec.Body.String(), want)
	}
	_, noDeadline := e.newLink(alice, book, `{}`)
	if got := e.lookup(noDeadline).status(200, "").data(); got["deadline_at"] != nil || got["open"] != true {
		t.Fatalf("no-deadline lookup: %v", got)
	}

	// a session cookie is ignored: another user's, or a broken one, changes nothing and nothing is set
	bob := e.register("bob@example.com", "Bob", true)
	r := e.do(bob, "GET", lookupPath(token), "").status(200, "")
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
	if rec := e.do(user{"garbage"}, "GET", lookupPath(token), ""); rec.Code != 200 {
		t.Fatalf("bad session cookie broke the public lookup: %d", rec.Code)
	}

	// 404s are indistinguishable (compared without the per-request id)
	revID, revoked := e.newLink(alice, book, `{}`)
	e.revoke(alice, revID).status(200, "")
	var bodies []string
	for _, tok := range []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "short", strings.Repeat("a", 10_000), "a%2Fb", "%00", token + "x", strings.Repeat("-", 32), revoked} {
		rec := e.lookup(tok).status(404, "ERROR_NOT_FOUND")
		var env map[string]string
		_ = json.Unmarshal(rec.Body.Bytes(), &env)
		delete(env, "request_id")
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
	e.lookup(token).status(410, "ERROR_COLLECTION_CLOSED")
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
		e.lookup(fmt.Sprintf("%032d", i)).status(404, "ERROR_NOT_FOUND")
	}
	for range 500 {
		e.lookup(token).status(200, "")
	}
	e.lookup("x").status(404, "ERROR_NOT_FOUND") // the 60th miss
	rec := e.lookup(token).status(429, "ERROR_RATE_LIMITED")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.advance(15*time.Minute + time.Second)
	e.lookup(token).status(200, "")
	// the cost guard: 600 requests of any kind per 15 minutes
	for range 599 {
		e.lookup(token)
	}
	e.lookup(token).status(429, "ERROR_RATE_LIMITED")
	e.advance(15*time.Minute + time.Second)
	// the owner endpoints are not limited by the public counters
	e.list(alice, book).status(200, "")
	e.revoke(alice, id).status(200, "")

	hash := sha256.Sum256([]byte(token))
	logs := e.logs.String()
	if strings.Contains(logs, token) || strings.Contains(logs, fmt.Sprintf("%x", hash)) {
		t.Fatal("token or hash in the logs")
	}
	if !strings.Contains(logs, "GET /api/public-collect/get") {
		t.Fatal("access log should record the route pattern")
	}
}

// A different client IP has its own counter (the limit is per IP, not global).
func TestPublicRateLimitPerIP(t *testing.T) {
	e := newEnv(t)
	for range 60 {
		e.lookup("x")
	}
	e.lookup("x").status(429, "ERROR_RATE_LIMITED")
	r := httptest.NewRequest("GET", lookupPath("x"), nil)
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
	e.deleteBook(alice, b1).status(200, "")
	if n := e.count(`SELECT COUNT(*) FROM note_collection_tab`); n != 1 {
		t.Fatalf("%d links left, want 1", n)
	}
	e.lookup(t1).status(404, "ERROR_NOT_FOUND")
	e.lookup(t2).status(200, "")
}

// The origin check of the repo's Guard covers the new write endpoints.
func TestCSRFGuard(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	id, _ := e.newLink(alice, book, `{}`)
	for _, c := range [][2]string{{"POST", "/api/note-collection/create"}, {"POST", "/api/note-collection/revoke"}} {
		r := httptest.NewRequest(c[0], c[1], strings.NewReader(fmt.Sprintf(`{"yearbook_id":%q,"id":%q}`, book, id)))
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

// The v1 routes are gone: they answer the router's v2 404 or 405, whatever the method.
func TestV1NotesRoutesAreGone(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	id, token := e.newLink(alice, book, `{}`)
	for _, c := range [][2]string{
		{"GET", "/v1/yearbooks/" + book + "/collections"}, {"POST", "/v1/yearbooks/" + book + "/collections"},
		{"DELETE", "/v1/collections/" + id}, {"GET", "/v1/public/collect/" + token}, {"POST", "/v1/public/collect/" + token + "/notes"},
	} {
		rec := e.do(alice, c[0], c[1], "")
		if rec.Code != 404 && rec.Code != 405 {
			t.Errorf("%s %s: %d, want 404 or 405", c[0], c[1], rec.Code)
		}
		if code := rec.code(); code != "ERROR_NOT_FOUND" && code != "ERROR_METHOD_NOT_ALLOWED" {
			t.Errorf("%s %s: code %q, want the v2 routing error: %s", c[0], c[1], code, rec.Body.String())
		}
	}
	// the new routes accept their own method only
	e.do(alice, "GET", "/api/note-collection/create", "").status(405, "ERROR_METHOD_NOT_ALLOWED")
	e.do(alice, "POST", "/api/note-collection/get-list", "{}").status(405, "ERROR_METHOD_NOT_ALLOWED")
	e.do(alice, "DELETE", "/api/note-collection/revoke", "").status(405, "ERROR_METHOD_NOT_ALLOWED")
	e.do(alice, "POST", "/api/public-collect/get", "{}").status(405, "ERROR_METHOD_NOT_ALLOWED")
}

// AC3: the public routes need no cookie and send no CORS headers, with or without an Origin; a preflight is not answered.
func TestPublicRoutesIgnoreOriginAndCookie(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	_, token := e.newLink(alice, book, `{}`)
	for _, origin := range []string{"", "https://evil.example", "null"} {
		r := httptest.NewRequest("GET", lookupPath(token), nil)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, r)
		resp{rec, t}.status(200, "")
		for k := range rec.Header() {
			if strings.HasPrefix(strings.ToLower(k), "access-control-") {
				t.Errorf("Origin %q: CORS header %s", origin, k)
			}
		}
		body, ct := multipartOf(answers("A", "m"))
		r = httptest.NewRequest("POST", submitURL(token), body)
		r.Header.Set("Content-Type", ct)
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		rec = httptest.NewRecorder()
		e.h.ServeHTTP(rec, r)
		resp{rec, t}.status(200, "")
	}
	for _, path := range []string{lookupPath(token), submitPath} {
		r := httptest.NewRequest("OPTIONS", path, nil)
		r.Header.Set("Origin", "https://evil.example")
		r.Header.Set("Access-Control-Request-Method", "POST")
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, r)
		if rec.Code != 405 || rec.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("preflight %s: %d %v", path, rec.Code, rec.Header())
		}
	}
	// the owner endpoints keep their guard: a session cookie from a foreign or missing origin is refused, a JSON body is required
	for _, origin := range []string{"https://evil.example", ""} {
		r := httptest.NewRequest("POST", "/api/note-collection/revoke", strings.NewReader(`{"id":"x"}`))
		r.Header.Set("Content-Type", "application/json")
		if origin != "" {
			r.Header.Set("Origin", origin)
		}
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: alice.cookie})
		rec := httptest.NewRecorder()
		e.h.ServeHTTP(rec, r)
		resp{rec, t}.status(403, "ERROR_CSRF_ORIGIN_MISMATCH")
	}
	r := httptest.NewRequest("POST", "/api/note-collection/revoke", strings.NewReader(`{"id":"x"}`))
	r.Header.Set("Content-Type", "text/plain")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	resp{rec, t}.status(415, "ERROR_UNSUPPORTED_MEDIA_TYPE") // checked before the session, as in every guard
}

// The token is a credential in the URL: whatever happens to the request, it never reaches the logs, and the access log
// names the route only (docs/api-contract.md section 6).
func TestTokenInQueryNeverLogged(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	_, token := e.newLink(alice, book, `{}`)
	const secret = "SECRET-QUERY-TOKEN-0123456789ab" // 31 characters: not a valid token, a plain miss
	e.do(user{}, "GET", "/api/public-collect/get?token="+secret, "").status(404, "ERROR_NOT_FOUND")
	e.do(user{}, "GET", "/api/public-collect/get?token="+secret+"&limit=abc", "").status(404, "ERROR_NOT_FOUND")
	e.lookup(token).status(200, "")
	for range missesPerWindow { // and the 429 path
		e.do(user{}, "GET", "/api/public-collect/get?token="+secret, "")
	}
	e.do(user{}, "GET", "/api/public-collect/get?token="+secret, "").status(429, "ERROR_RATE_LIMITED")
	e.advance(16 * time.Minute)
	// a failing photo store logs an error line: the body token must not be in it either
	e.st.failAt = e.st.puts + 1
	e.post(token, answers("A", "m"), photoPart(jpegOf(t, 1))).status(502, "ERROR_STORAGE_ERROR")
	e.post(secret, answers("A", "m")).status(404, "ERROR_NOT_FOUND")
	logs := e.logs.String()
	if strings.Contains(logs, secret) || strings.Contains(logs, token) || strings.Contains(logs, "SECRET") {
		t.Fatalf("a token is in the logs:\n%s", logs)
	}
	if !strings.Contains(logs, "GET /api/public-collect/get") || !strings.Contains(logs, "POST /api/public-collect/submit-note") {
		t.Fatal("the access log should record the route patterns")
	}
}

// A mistake in the query is a 404 like any other miss; the owner endpoints report their missing parameters.
func TestMissingParameters(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	e.do(user{}, "GET", "/api/public-collect/get", "").status(404, "ERROR_NOT_FOUND")
	e.do(alice, "GET", "/api/note-collection/get-list", "").status(400, "ERROR_PARAM")
	e.do(alice, "POST", "/api/note-collection/revoke", `{}`).status(400, "ERROR_PARAM")
	e.do(alice, "POST", "/api/note-collection/revoke", `{"id":"x","extra":1}`).status(400, "ERROR_PARAM")
	e.do(alice, "POST", "/api/note-collection/create", "").status(400, "ERROR_PARAM")
}
