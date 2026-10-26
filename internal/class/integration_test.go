//go:build integration

package class

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
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
	"github.com/danyaa666/smemories/internal/ulid"
	"github.com/danyaa666/smemories/internal/yearbook"
)

const (
	origin = "http://localhost:5173"
	testPW = "Tr0ub4dor&3-fake-test-password" // obviously fake
)

type env struct {
	t     *testing.T
	db    *sql.DB
	h     http.Handler
	svc   *Service
	mu    sync.Mutex
	clock time.Time
	n     int
}

func (e *env) now() time.Time {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.clock
}

// newEnv wires the auth, class and yearbook handlers on a private database; the class service is the yearbook's child purger,
// as in cmd/smemories-api.
func newEnv(t *testing.T) *env {
	t.Helper()
	d := dbtest.New(t)
	e := &env{t: t, db: d, clock: time.Date(2026, 10, 26, 12, 0, 0, 0, time.UTC)}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
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
	limiters := ratelimit.NewFactory(rc, logger, e.now)
	asvc, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 1000},
		auth.Mail{Mailer: mail, Logger: logger}, codes, limiters, e.now)
	if err != nil {
		t.Fatal(err)
	}
	ah := auth.NewHandler(asvc, auth.HandlerConfig{AllowedOrigins: []string{origin}}, logger)
	cursors := httpx.NewCursors([]byte(strings.Repeat("c", 32)))
	e.svc = NewService(NewStore(d), cursors, limiters, e.now)
	ch := NewHandler(e.svc, ah.RequireUser, []string{origin})
	yh := yearbook.NewHandler(yearbook.NewService(yearbook.NewStore(d), cursors, nil, e.now, e.svc), ah.RequireUser, []string{origin})
	e.h = httpx.NewRouter(logger, ah.Routes, ch.Routes, yh.Routes)
	return e
}

// person is a signed-in test user: the session cookie plus the ids the tests need.
type person struct {
	cookie string
	row    uint64 // user_tab.id
}

var anonymous = person{}

// user registers a verified user (the email flow itself is covered by the auth tests).
func (e *env) user(name string) person { return e.userWith(name, true) }

func (e *env) userWith(name string, verified bool) person {
	e.t.Helper()
	e.mu.Lock()
	e.n++
	email := name + strconv.Itoa(e.n) + "@example.com"
	e.mu.Unlock()
	b, _ := json.Marshal(map[string]string{"email": email, "password": testPW, "display_name": name, "accept_terms_version": legal.CurrentVersion})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/auth/register", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		e.t.Fatalf("register: %d %s", rec.Code, rec.Body)
	}
	p := person{}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			p.cookie = c.Value
		}
	}
	if p.cookie == "" {
		e.t.Fatal("no session cookie")
	}
	if verified {
		e.exec(`UPDATE user_tab SET email_verified_at = ? WHERE email = ?`, e.now().UnixMilli(), email)
	}
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = ?`, email).Scan(&p.row); err != nil {
		e.t.Fatal(err)
	}
	return p
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := e.db.Exec(q, args...); err != nil {
		e.t.Fatalf("%s: %v", q, err)
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

// seedClass inserts a class with the given admins and students straight into the database (no write budget used) and
// returns its public id and the public member ids by user row.
func (e *env) seedClass(admins []person, students ...person) (classID string, members map[uint64]string) {
	e.t.Helper()
	now := e.now()
	classID = ulid.New(now)
	res, err := e.db.Exec(`INSERT INTO class_tab (public_id, name, school_name, language, created_by, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
		classID, "Lớp 12A1", "THPT", "vi", admins[0].row, now.UnixMilli(), now.UnixMilli())
	if err != nil {
		e.t.Fatal(err)
	}
	cid, _ := res.LastInsertId()
	members = map[uint64]string{}
	add := func(p person, role string) {
		mid := ulid.New(now)
		e.exec(`INSERT INTO class_member_tab (public_id, class_id, user_id, role, joined_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?)`,
			mid, cid, p.row, role, now.UnixMilli(), now.UnixMilli(), now.UnixMilli())
		members[p.row] = mid
	}
	for _, p := range admins {
		add(p, RoleAdmin)
	}
	for _, p := range students {
		add(p, RoleStudent)
	}
	return classID, members
}

type resp struct {
	*httptest.ResponseRecorder
	t *testing.T
}

func (e *env) do(p person, method, path, body string) resp {
	e.t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	if p.cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: p.cookie})
		r.Header.Set("Origin", origin)
	}
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return resp{rec, e.t}
}

// code is the envelope status of the answer.
func (r resp) code() string {
	var env struct{ Status string }
	if err := json.Unmarshal(r.Body.Bytes(), &env); err != nil {
		r.t.Fatalf("not an envelope: %s", r.Body)
	}
	return env.Status
}

func (r resp) status(want int, code string) resp {
	r.t.Helper()
	if r.Code != want || r.code() != code {
		r.t.Fatalf("got %d %s, want %d %s", r.Code, r.Body, want, code)
	}
	return r
}

// data is the "data" object of a success answer.
func (r resp) data() map[string]any {
	r.t.Helper()
	var out struct {
		Status string
		Data   map[string]any
	}
	if err := json.Unmarshal(r.Body.Bytes(), &out); err != nil || out.Status != "OK" || out.Data == nil {
		r.t.Fatalf("no data in %s", r.Body)
	}
	return out.Data
}

// sameAnswer is the body without the request id, which differs per request.
func (r resp) sameAnswer() string {
	var m map[string]any
	if err := json.Unmarshal(r.Body.Bytes(), &m); err != nil {
		r.t.Fatalf("not json: %s", r.Body)
	}
	delete(m, "request_id")
	b, _ := json.Marshal(m)
	return strconv.Itoa(r.Code) + string(b)
}

const classBody = `{"name":"Lớp 12A1","school_name":"THPT Chu Văn An","graduation_year":2026,"language":"vi"}`

func (e *env) create(p person) string {
	e.t.Helper()
	return e.do(p, "POST", "/api/class/create", classBody).status(200, "OK").data()["id"].(string)
}

func body(kv ...string) string {
	m := map[string]string{}
	for i := 0; i < len(kv); i += 2 {
		m[kv[i]] = kv[i+1]
	}
	b, _ := json.Marshal(m)
	return string(b)
}
