//go:build integration

package bookdata

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
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/notes"
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
	t   *testing.T
	db  *sql.DB
	ah  *auth.Handler
	h   http.Handler
	log *slog.Logger
	now time.Time
}

// stack builds the book-data handler over d, which may be a counting pool.
func (e *env) stack(d *sql.DB) http.Handler {
	nsvc := notes.NewService(notes.NewStore(d), nil, nil, ratelimit.NewMemoryFactory(time.Now), e.log, nil)
	ysvc := yearbook.NewService(yearbook.NewStore(d), httpx.NewCursors([]byte(strings.Repeat("c", 32))), nil, nil)
	return httpx.NewRouter(e.log, e.ah.Routes, NewHandler(NewService(ysvc, nsvc), e.ah.RequireUser).Routes)
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := dbtest.New(t)
	e := &env{t: t, db: d, log: slog.New(slog.DiscardHandler), now: time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)}
	hasher := auth.NewHasher(auth.HashParams{MemoryKiB: 64, Time: 1, Parallelism: 1}, 4, auth.HashWait)
	mail, _ := mailer.NewLog("test", io.Discard)
	rc := redistest.New(t)
	codes, err := auth.NewCodes(context.Background(), rc, "test", []byte(strings.Repeat("k", auth.MinOTPKeyBytes)), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := auth.NewSessions(context.Background(), rc, e.log)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000},
		auth.Mail{Mailer: mail, Logger: e.log}, codes, ratelimit.NewFactory(rc, e.log, func() time.Time { return e.now }), func() time.Time { return e.now })
	if err != nil {
		t.Fatal(err)
	}
	e.ah = auth.NewHandler(svc, auth.HandlerConfig{AllowedOrigins: []string{origin}}, e.log)
	e.h = e.stack(d)
	return e
}

type user struct {
	cookie string
	row    uint64
}

func (e *env) register(email string, verified bool) user {
	e.t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email, "password": testPW, "display_name": "Owner"})
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("POST", "/api/auth/register", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	e.h.ServeHTTP(rec, r)
	if rec.Code != 200 {
		e.t.Fatalf("register: %d %s", rec.Code, rec.Body.String())
	}
	if verified {
		e.exec(`UPDATE user_tab SET email_verified_at = ? WHERE email = ?`, e.now.UnixMilli(), email)
	}
	var u user
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = ?`, email).Scan(&u.row); err != nil {
		e.t.Fatal(err)
	}
	for _, c := range rec.Result().Cookies() {
		if c.Name == auth.CookieName {
			u.cookie = c.Value
			return u
		}
	}
	e.t.Fatal("no session cookie")
	return u
}

func (e *env) exec(q string, args ...any) sql.Result {
	e.t.Helper()
	res, err := e.db.Exec(q, args...)
	if err != nil {
		e.t.Fatal(err)
	}
	return res
}

type resp struct {
	*httptest.ResponseRecorder
	t *testing.T
}

func (e *env) get(h http.Handler, u user, query string) resp {
	e.t.Helper()
	r := httptest.NewRequest("GET", "/api/book-data/get"+query, nil)
	if u.cookie != "" {
		r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: u.cookie})
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
	var env struct{ Status string }
	if err := json.Unmarshal(r.Body.Bytes(), &env); err != nil || env.Status != code {
		r.t.Fatalf("status field %q (err %v), want %q: %s", env.Status, err, code, r.Body.String())
	}
	return r
}

func (r resp) data() map[string]any {
	var env struct{ Data map[string]any }
	if err := json.Unmarshal(r.Body.Bytes(), &env); err != nil {
		r.t.Fatal(err)
	}
	return env.Data
}

// book is a yearbook row with an owner profile, a collection and a media row for its cover.
type book struct {
	public string
	row    uint64
	coll   uint64
}

func (e *env) book(u user, title string) book {
	e.t.Helper()
	ms := e.now.UnixMilli()
	b := book{public: ulid.New(e.now)}
	res := e.exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, created_at, updated_at)
		VALUES (?,?,?,'School','12A',2026,'Motto','vi','A4','classic',?,?)`, b.public, u.row, title, ms, ms)
	id, _ := res.LastInsertId()
	b.row = uint64(id)
	e.exec(`INSERT INTO profile_tab (public_id, yearbook_id, is_owner, full_name, nickname, birthday, quote, hobbies, future_plans, created_at, updated_at)
		VALUES (?,?,1,'Nguyễn Văn An','An','2008-05-06','Quote','Hobbies','Plans',?,?)`, ulid.New(e.now), b.row, ms, ms)
	res = e.exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		ulid.New(e.now), b.row, bytes.Repeat([]byte{7}, 32), "Class link", ms, ms)
	cid, _ := res.LastInsertId()
	b.coll = uint64(cid)
	return b
}

// media inserts a photo row with storage keys and a hash that must never appear in a response; it returns the public id.
func (e *env) media(b book) string {
	e.t.Helper()
	pub := ulid.New(e.now)
	e.exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		VALUES (?,?,'contributor',?,?,?,'image/jpeg',1,1,1,?,1,1)`, pub, b.row, "SECRETKEY/o/"+pub, "SECRETKEY/t/"+pub, "SECRETKEY/p/"+pub, strings.Repeat("e", 64))
	return pub
}

// note inserts a note and links the media ids to it in order; it returns the public id. A negative order is NULL (not approved).
func (e *env) note(b book, status string, order int, answers string, photos ...string) string {
	e.t.Helper()
	pub := ulid.New(e.now)
	var so any
	if order >= 0 {
		so = order
	}
	res := e.exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, sort_order, created_at, updated_at) VALUES (?,?,?,?,?,?,1,1)`,
		pub, b.coll, b.row, answers, status, so)
	nid, _ := res.LastInsertId()
	for i, m := range photos {
		e.exec(`INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) SELECT ?, id, ?, 1, 1 FROM media_tab WHERE public_id = ?`, nid, i, m)
	}
	return pub
}

// bulk inserts approved notes with sort_order from..to, each with its own photos (a media row belongs to one note), in one
// transaction, and returns their public ids in order.
func (e *env) bulk(b book, from, to, photos int, answers string) []string {
	e.t.Helper()
	tx, err := e.db.Begin()
	if err != nil {
		e.t.Fatal(err)
	}
	var out []string
	for i := from; i <= to; i++ {
		pub := ulid.New(e.now)
		res, err := tx.Exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, sort_order, created_at, updated_at) VALUES (?,?,?,?,'approved',?,1,1)`,
			pub, b.coll, b.row, answers, i)
		if err != nil {
			e.t.Fatal(err)
		}
		nid, _ := res.LastInsertId()
		for j := range photos {
			mres, err := tx.Exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at, updated_at)
				VALUES (?,?,'contributor','SECRETKEY/o','SECRETKEY/t','SECRETKEY/p','image/jpeg',1,1,1,?,1,1)`, ulid.New(e.now), b.row, strings.Repeat("e", 64))
			if err != nil {
				e.t.Fatal(err)
			}
			mid, _ := mres.LastInsertId()
			if _, err := tx.Exec(`INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) VALUES (?,?,?,1,1)`, nid, mid, j); err != nil {
				e.t.Fatal(err)
			}
		}
		out = append(out, pub)
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
	return out
}

func ids(data map[string]any) (out []string, photos [][]string) {
	for _, n := range data["notes"].([]any) {
		m := n.(map[string]any)
		out = append(out, m["id"].(string))
		ps := []string{}
		for _, p := range m["photo_ids"].([]any) {
			ps = append(ps, p.(string))
		}
		photos = append(photos, ps)
	}
	return out, photos
}

// AC1, AC3: the exact shape, only approved notes, book order, photos in their order, nothing internal.
func TestOwnerGetsApprovedNotesInBookOrder(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", true)
	b := e.book(alice, "Lớp 12A")
	p0, p1, p2, p3, p4 := e.media(b), e.media(b), e.media(b), e.media(b), e.media(b)
	e.exec(`UPDATE yearbook_tab SET cover_media_id = (SELECT id FROM media_tab WHERE public_id = ?) WHERE id = ?`, p4, b.row)
	e.exec(`UPDATE profile_tab SET photo_media_id = (SELECT id FROM media_tab WHERE public_id = ?) WHERE yearbook_id = ?`, p3, b.row)

	second := e.note(b, "approved", 2, `{"name":"Second","message":"hi 😀"}`, p2, p1)
	first := e.note(b, "approved", 1, `{"name":"Đầu tiên","message":"xin chào"}`, p0)
	e.note(b, "pending", -1, `{"name":"Pending"}`, p3)
	e.note(b, "hidden", -1, `{"name":"Hidden"}`)
	third := e.note(b, "approved", 3, `{"name":"Third"}`)

	r := e.get(e.h, alice, "?yearbook_id="+b.public).status(200, "OK")
	d := r.data()
	if got, photos := ids(d); !slices.Equal(got, []string{first, second, third}) ||
		!slices.Equal(photos[0], []string{p0}) || !slices.Equal(photos[1], []string{p2, p1}) || len(photos[2]) != 0 {
		t.Fatalf("notes %v photos %v", got, photos)
	}
	if d["note_count"] != float64(3) {
		t.Fatalf("note_count %v", d["note_count"])
	}
	want := map[string]any{"title": "Lớp 12A", "school_name": "School", "class_name": "12A", "graduation_year": float64(2026), "motto": "Motto",
		"language": "vi", "page_size": "A4", "template_id": "classic", "cover_media_id": p4}
	if got := d["yearbook"].(map[string]any); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("yearbook %v, want %v", got, want)
	}
	wantP := map[string]any{"full_name": "Nguyễn Văn An", "nickname": "An", "quote": "Quote", "hobbies": "Hobbies", "future_plans": "Plans", "photo_media_id": p3}
	if got := d["profile"].(map[string]any); fmt.Sprint(got) != fmt.Sprint(wantP) {
		t.Fatalf("profile %v, want %v", got, wantP)
	}
	if keys := mapKeys(d); !slices.Equal(keys, []string{"note_count", "notes", "profile", "yearbook"}) {
		t.Fatalf("data keys %v", keys)
	}
	if keys := mapKeys(d["notes"].([]any)[0].(map[string]any)); !slices.Equal(keys, []string{"answers", "id", "photo_ids"}) {
		t.Fatalf("note keys %v", keys)
	}
	if answers := d["notes"].([]any)[1].(map[string]any)["answers"].(map[string]any); answers["message"] != "hi 😀" {
		t.Fatalf("answers %v", answers)
	}
	// AC3: no storage key, hash, email, birthday or internal id anywhere in the body.
	body := r.Body.String()
	for _, leak := range []string{"SECRETKEY", strings.Repeat("e", 64), "alice@example.com", "2008-05-06", "owner_id", "token", "sha256", "object_key", "collection"} {
		if strings.Contains(body, leak) {
			t.Errorf("response contains %q: %s", leak, body)
		}
	}

	// A hidden note comes back into the book only when approved again: change a status and read again.
	e.exec(`UPDATE note_tab SET status = 'hidden' WHERE public_id = ?`, first)
	if got, _ := ids(e.get(e.h, alice, "?yearbook_id="+b.public).status(200, "OK").data()); !slices.Equal(got, []string{second, third}) {
		t.Fatalf("after hiding: %v", got)
	}
}

func mapKeys(m map[string]any) []string {
	var k []string
	for key := range m {
		k = append(k, key)
	}
	slices.Sort(k)
	return k
}

// Equal sort_order falls back to creation order (the id), like T-013's approved list.
func TestTiesFollowCreationOrder(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", true)
	b := e.book(alice, "T")
	a := e.note(b, "approved", 1, `{"name":"A"}`)
	c := e.note(b, "approved", 1, `{"name":"B"}`)
	if got, _ := ids(e.get(e.h, alice, "?yearbook_id="+b.public).status(200, "OK").data()); !slices.Equal(got, []string{a, c}) {
		t.Fatalf("order %v", got)
	}
}

// An empty book is an empty list, not null, and every optional field is null or "".
func TestEmptyBook(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", true)
	b := e.book(alice, "Empty")
	r := e.get(e.h, alice, "?yearbook_id="+b.public).status(200, "OK")
	if !strings.Contains(r.Body.String(), `"notes":[]`) || r.data()["note_count"] != float64(0) {
		t.Fatalf("empty book: %s", r.Body.String())
	}
}

// AC1: ownership gives NOT_FOUND, a malformed id PARAM; the session and the verified email are required.
func TestAccessRules(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", true)
	bob := e.register("bob@example.com", true)
	carol := e.register("carol@example.com", false)
	b := e.book(alice, "Alice's")
	e.note(b, "approved", 1, `{"name":"A"}`)
	lower := strings.ToLower(b.public)

	e.get(e.h, bob, "?yearbook_id="+b.public).status(404, "ERROR_NOT_FOUND")
	e.get(e.h, alice, "?yearbook_id="+ulid.New(e.now)).status(404, "ERROR_NOT_FOUND")
	for name, q := range map[string]string{
		"missing":    "",
		"empty":      "?yearbook_id=",
		"lower case": "?yearbook_id=" + lower,
		"short":      "?yearbook_id=" + b.public[:25],
		"long":       "?yearbook_id=" + b.public + "0",
		"padded":     "?yearbook_id=" + b.public + "%20",
		"two ids":    "?yearbook_id=" + b.public + "&yearbook_id=" + b.public + "x",
		"not base32": "?yearbook_id=" + strings.Repeat("U", 26),
	} {
		if name == "two ids" { // the first value counts; the id is still fine
			e.get(e.h, alice, q).status(200, "OK")
			continue
		}
		t.Run(name, func(t *testing.T) { e.get(e.h, alice, q).status(400, "ERROR_PARAM") })
	}
	e.get(e.h, user{}, "?yearbook_id="+b.public).status(401, "ERROR_UNAUTHORIZED")
	e.get(e.h, carol, "?yearbook_id="+b.public).status(403, "ERROR_EMAIL_NOT_VERIFIED")
}

// AC2: one query for the notes and one for the photo links whatever the number of notes, and the cap.
func TestQueryCountAndCap(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", true)
	b := e.book(alice, "Big")
	var mu sync.Mutex
	var queries []string
	spy := dbtest.Spy(t, e.db, func(q string) { mu.Lock(); queries = append(queries, q); mu.Unlock() })
	h := e.stack(spy)
	run := func() (queries2 []string, data map[string]any) {
		mu.Lock()
		queries = nil
		mu.Unlock()
		d := e.get(h, alice, "?yearbook_id="+b.public).status(200, "OK").data()
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(queries), d
	}
	count := func(qs []string, table string) (n int) {
		for _, q := range qs {
			if strings.Contains(q, table) {
				n++
			}
		}
		return n
	}

	qs, _ := run() // no notes: the photo query is skipped
	if got := count(qs, "note_tab"); got != 1 || count(qs, "note_photo_tab") != 0 {
		t.Fatalf("empty book queries: %v", qs)
	}
	e.note(b, "approved", 1, `{"name":"One"}`, e.media(b))
	qs1, _ := run()

	// 305 approved notes with 3 photos each: the response is cut at 300, in book order, with the same number of queries.
	first300 := e.bulk(b, 2, 305, 3, `{"name":"N"}`)[:299]
	qs300, d := run()
	got, _ := ids(d)
	if len(got) != 300 || d["note_count"] != float64(300) || !slices.Equal(got[1:], first300) {
		t.Fatalf("cap: %d notes, note_count %v", len(got), d["note_count"])
	}
	for name, qs := range map[string][]string{"1 note": qs1, "300 notes": qs300} {
		// 1 for the notes, 1 for the photo links, 2 for the book and its profile (a third for media ids when it has a cover or photo).
		if count(qs, "note_tab") != 1 || count(qs, "note_photo_tab") != 1 || len(qs) != 4 {
			t.Errorf("%s: %d queries, want 4: %v", name, len(qs), qs)
		}
	}
}

// AC2: 300 notes with the longest answers of the built-in templates (name, relationship, message at their maximum) and 3
// photos each stay under 1 MiB.
func TestMaximalBookIsUnderOneMiB(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", true)
	b := e.book(alice, "Max")
	answers, _ := json.Marshal(map[string]string{"name": strings.Repeat("N", 60), "relationship": strings.Repeat("r", 60), "message": strings.Repeat("m", 2000)})
	e.bulk(b, 1, 300, 3, string(answers))
	r := e.get(e.h, alice, "?yearbook_id="+b.public).status(200, "OK")
	t.Logf("300 maximal notes: %d bytes", r.Body.Len())
	if r.Body.Len() >= 1<<20 {
		t.Fatalf("response is %d bytes, want under 1 MiB", r.Body.Len())
	}
}
