//go:build integration

package bookexport

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/medialink"
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
	secret = "SECRETKEY"                      // appears in every storage key and must never reach the text
)

var hash = strings.Repeat("e", 64)

type env struct {
	t      *testing.T
	db     *sql.DB
	st     *storage.Memory
	ah     *auth.Handler
	h      http.Handler
	books  *yearbook.Service
	signer *medialink.Signer
	log    *slog.Logger
	now    time.Time
}

// stack builds the export, the public-media route and the sign-in routes over d, which may be a counting pool.
func (e *env) stack(d *sql.DB, lim *ratelimit.Factory) http.Handler {
	cursors := httpx.NewCursors([]byte(strings.Repeat("c", 32)))
	msvc := media.NewService(media.NewStore(d), e.st, 2, lim, nil)
	msvc.SetPublicLinks(e.signer)
	mh := media.NewHandler(msvc, 10<<20, e.ah.RequireUser, []string{origin}, cursors, e.log)
	mh.SetPublicCounter(func(string) {})
	nsvc := notes.NewService(notes.NewStore(d), nil, nil, ratelimit.NewMemoryFactory(time.Now), e.log, nil)
	ysvc := yearbook.NewService(yearbook.NewStore(d), cursors, nil, nil)
	e.books = ysvc
	svc := NewService(ysvc, nsvc, e.signer, origin, lim, func() time.Time { return e.now })
	return httpx.NewRouter(e.log, e.ah.Routes, mh.Routes, NewHandler(svc, e.ah.RequireUser).Routes)
}

func newEnv(t *testing.T) *env {
	t.Helper()
	d := dbtest.New(t)
	e := &env{t: t, db: d, st: storage.NewMemory(), log: slog.New(slog.DiscardHandler), now: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)}
	e.signer, _ = medialink.New(bytes.Repeat([]byte{9}, medialink.MinSecretBytes))
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
	lim := ratelimit.NewFactory(rc, e.log, func() time.Time { return e.now })
	svc, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{RegisterPerHour: 1000, LoginFailsPerPair: 1000, LoginFailsPerIP: 1000, LoginFailsPerAccount: 1000},
		auth.Mail{Mailer: mail, Logger: e.log}, codes, lim, func() time.Time { return e.now })
	if err != nil {
		t.Fatal(err)
	}
	e.ah = auth.NewHandler(svc, auth.HandlerConfig{AllowedOrigins: []string{origin}}, e.log)
	e.h = e.stack(d, lim)
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
	r := httptest.NewRequest("GET", "/api/book-export/get-text"+query, nil)
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

func (r resp) export() Export {
	var env struct{ Data Export }
	if err := json.Unmarshal(r.Body.Bytes(), &env); err != nil {
		r.t.Fatal(err)
	}
	return env.Data
}

type bk struct {
	public string
	row    uint64
	coll   uint64
}

func (e *env) book(u user, title string) bk {
	e.t.Helper()
	ms := e.now.UnixMilli()
	b := bk{public: ulid.New(e.now)}
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

// media stores a display object and its row (the keys and the hash must never appear in a response); it returns the public id.
func (e *env) media(b bk, content string) string {
	e.t.Helper()
	pub := ulid.New(e.now)
	key := secret + "/o/" + pub
	if err := e.st.Put(context.Background(), key, "image/jpeg", []byte(content)); err != nil {
		e.t.Fatal(err)
	}
	e.exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		VALUES (?,?,'contributor',?,?,?,'image/jpeg',?,1,1,?,1,1)`, pub, b.row, key, secret+"/t/"+pub, secret+"/p/"+pub, len(content), hash)
	return pub
}

// note inserts a note and links the media ids to it in order; a negative order is NULL (not approved). It returns the row id.
func (e *env) note(b bk, status string, order int, answers string, photos ...string) int64 {
	e.t.Helper()
	var so any
	if order >= 0 {
		so = order
	}
	res := e.exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, sort_order, created_at, updated_at) VALUES (?,?,?,?,?,?,1,1)`,
		ulid.New(e.now), b.coll, b.row, answers, status, so)
	nid, _ := res.LastInsertId()
	for i, m := range photos {
		e.exec(`INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) SELECT ?, id, ?, 1, 1 FROM media_tab WHERE public_id = ?`, nid, i, m)
	}
	return nid
}

// bulk inserts approved notes with sort_order from..to without photos, in one transaction.
func (e *env) bulk(b bk, from, to int, answers string) {
	e.t.Helper()
	tx, err := e.db.Begin()
	if err != nil {
		e.t.Fatal(err)
	}
	for i := from; i <= to; i++ {
		if _, err := tx.Exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, sort_order, created_at, updated_at) VALUES (?,?,?,?,'approved',?,1,1)`,
			ulid.New(e.now), b.coll, b.row, answers, i); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

// open asks for a link anonymously (no cookie) and returns the status and body.
func (e *env) open(link string) (int, string) {
	e.t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		e.t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", u.RequestURI(), nil)
	r.RemoteAddr = "198.51.100.7:4000"
	e.h.ServeHTTP(rec, r)
	if rec.Header().Get("Set-Cookie") != "" {
		e.t.Error("a public link sets a cookie")
	}
	return rec.Code, rec.Body.String()
}

var linkRE = regexp.MustCompile(`https?://\S+/api/public-media/get-content\?token=\S+`)

// AC2, AC3: approved notes only, in book order; the links open anonymously, a hidden note's photo is not in the text and stops
// working when the note is hidden later; revoke kills the old links and a new export works; nothing internal is printed.
func TestExportApprovedOnlyLinksAndRevoke(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", true)
	b := e.book(alice, "Lớp 12A")
	cover, face := e.media(b, "cover-bytes"), e.media(b, "face-bytes")
	e.exec(`UPDATE yearbook_tab SET cover_media_id = (SELECT id FROM media_tab WHERE public_id = ?) WHERE id = ?`, cover, b.row)
	e.exec(`UPDATE profile_tab SET photo_media_id = (SELECT id FROM media_tab WHERE public_id = ?) WHERE yearbook_id = ?`, face, b.row)
	p1, p2, p3 := e.media(b, "one"), e.media(b, "two"), e.media(b, "three")
	hid, pen := e.media(b, "hidden-bytes"), e.media(b, "pending-bytes")
	second := e.note(b, "approved", 2, `{"name":"Second","message":"Hai"}`, p3)
	e.note(b, "approved", 1, `{"name":"First","message":"Một\nhai"}`, p1, p2)
	e.note(b, "hidden", -1, `{"name":"HiddenName","message":"secret"}`, hid)
	e.note(b, "pending", -1, `{"name":"PendingName","message":"later"}`, pen)

	r := e.get(e.h, alice, "?yearbook_id="+b.public+"&lang=en").status(200, "OK")
	x := r.export()
	if x.NoteCount != 2 || x.ApprovedTotal != 2 || x.Truncated || x.PhotoCount != 5 || !strings.HasSuffix(x.Filename, "-20261019.txt") {
		t.Fatalf("%+v", x)
	}
	if !strings.Contains(x.Text, "Exported: 2026-10-19") || strings.Index(x.Text, "First") > strings.Index(x.Text, "Second") ||
		strings.Contains(x.Text, "HiddenName") || strings.Contains(x.Text, "PendingName") {
		t.Fatalf("content or order wrong:\n%s", x.Text)
	}
	for _, bad := range []string{hid, pen, b.public, "alice@example.com", secret, hash, "2008-05-06"} {
		if strings.Contains(r.Body.String(), bad) {
			t.Errorf("the response holds %q", bad)
		}
	}
	links := linkRE.FindAllString(x.Text, -1)
	if len(links) != 5 {
		t.Fatalf("%d links:\n%s", len(links), x.Text)
	}
	for i, want := range []string{"face-bytes", "cover-bytes", "one", "two", "three"} {
		if code, body := e.open(links[i]); code != 200 || body != want {
			t.Errorf("link %d: %d %q, want 200 %q", i, code, body, want)
		}
	}

	// A note hidden after the export: its link stops working (T-092), the other links keep working.
	e.exec(`UPDATE note_tab SET status = 'hidden' WHERE id = ?`, second)
	if code, _ := e.open(links[4]); code != 404 {
		t.Errorf("the photo of a note hidden later opens: %d", code)
	}
	if code, _ := e.open(links[2]); code != 200 {
		t.Errorf("an unrelated link broke: %d", code)
	}
	e.exec(`UPDATE note_tab SET status = 'approved' WHERE id = ?`, second)

	if err := e.books.RevokePhotoLinks(context.Background(), alice.row, b.public); err != nil {
		t.Fatal(err)
	}
	for _, l := range links {
		if code, _ := e.open(l); code != 404 {
			t.Errorf("an old link still opens after revoke: %d", code)
		}
	}
	fresh := linkRE.FindAllString(e.get(e.h, alice, "?yearbook_id="+b.public+"&lang=en").status(200, "OK").export().Text, -1)
	if len(fresh) != 5 || fresh[0] == links[0] {
		t.Fatalf("new links: %v", fresh)
	}
	for _, l := range fresh {
		if code, _ := e.open(l); code != 200 {
			t.Errorf("a new link does not open: %d", code)
		}
	}
}

// Ownership, ids, language, verification, sign-in and the rate limit.
func TestRequestRules(t *testing.T) {
	e := newEnv(t)
	alice, bob := e.register("alice@example.com", true), e.register("bob@example.com", true)
	carol := e.register("carol@example.com", false)
	b := e.book(alice, "Mine")
	e.get(e.h, alice, "?yearbook_id="+b.public).status(200, "OK")
	if x := e.get(e.h, alice, "?yearbook_id="+b.public).export(); !strings.Contains(x.Text, "Lời nhắn từ bạn bè") { // the book's language is vi
		t.Errorf("default language:\n%s", x.Text)
	}
	e.get(e.h, bob, "?yearbook_id="+b.public).status(404, "ERROR_NOT_FOUND")
	e.get(e.h, alice, "?yearbook_id="+ulid.New(e.now)).status(404, "ERROR_NOT_FOUND")
	for name, q := range map[string]string{
		"missing": "", "lower": "?yearbook_id=" + strings.ToLower(b.public), "short": "?yearbook_id=abc",
		"bad lang": "?yearbook_id=" + b.public + "&lang=fr", "upper lang": "?yearbook_id=" + b.public + "&lang=EN",
	} {
		t.Run(name, func(t *testing.T) { e.get(e.h, alice, q).status(400, "ERROR_PARAM") })
	}
	e.get(e.h, user{}, "?yearbook_id="+b.public).status(401, "ERROR_UNAUTHORIZED")
	e.get(e.h, carol, "?yearbook_id="+b.public).status(403, "ERROR_EMAIL_NOT_VERIFIED")

	// 30 per 10 minutes per user (4 were used above by alice and bob; bob has his own count).
	n := 0
	for ; n < 40; n++ {
		if e.get(e.h, bob, "?yearbook_id="+b.public).Code == 429 {
			break
		}
	}
	rec := e.get(e.h, bob, "?yearbook_id="+b.public)
	if rec.Code != 429 || rec.Header().Get("Retry-After") == "" || n < 25 || n > 31 {
		t.Errorf("rate limit: first 429 after %d calls, last status %d, Retry-After %q", n, rec.Code, rec.Header().Get("Retry-After"))
	}
	e.get(e.h, alice, "?yearbook_id="+b.public).status(200, "OK") // another user is not limited
}

// AC2: one query for the notes and one for the photo links whatever the number of notes; at the cap one more counts them;
// 1001 approved notes give truncated, the real total and a sentence in the text.
func TestQueryCountAndCap(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", true)
	b := e.book(alice, "Big")
	var mu sync.Mutex
	var queries []string
	spy := dbtest.Spy(t, e.db, func(q string) { mu.Lock(); queries = append(queries, q); mu.Unlock() })
	h := e.stack(spy, ratelimit.NewMemoryFactory(time.Now))
	run := func() ([]string, Export) {
		mu.Lock()
		queries = nil
		mu.Unlock()
		x := e.get(h, alice, "?yearbook_id="+b.public+"&lang=en").status(200, "OK").export()
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(queries), x
	}
	count := func(qs []string, table string) (n int) {
		for _, q := range qs {
			if strings.Contains(q, table) {
				n++
			}
		}
		return n
	}

	qs, _ := run()
	if count(qs, "note_tab") != 1 || count(qs, "note_photo_tab") != 0 {
		t.Fatalf("empty book queries: %v", qs)
	}
	e.note(b, "approved", 1, `{"name":"One"}`, e.media(b, "x"))
	qs, _ = run()
	// 1 notes, 1 photo links, 2 for the book and its profile (a third for media ids when it has a cover or photo).
	if count(qs, "note_tab") != 1 || count(qs, "note_photo_tab") != 1 || len(qs) != 4 {
		t.Errorf("1 note: %d queries, want 4: %v", len(qs), qs)
	}

	e.bulk(b, 2, 999, `{"name":"N"}`)
	qs, x := run()
	if x.Truncated || x.NoteCount != 999 || len(qs) != 4 {
		t.Errorf("999 notes: %+v, %d queries: %v", x.Truncated, len(qs), qs)
	}
	e.bulk(b, 1000, 1000, `{"name":"N"}`)
	qs, x = run()
	if x.Truncated || x.NoteCount != 1000 || count(qs, "COUNT(*)") != 1 || len(qs) != 5 {
		t.Errorf("1000 notes: %+v, %d queries: %v", x.Truncated, len(qs), qs)
	}
	e.bulk(b, 1001, 1001, `{"name":"N"}`)
	qs, x = run()
	if !x.Truncated || x.NoteCount != 1000 || x.ApprovedTotal != 1001 || !strings.Contains(x.Text, "1000 of 1001 notes are included") ||
		!strings.Contains(x.Text, "--- 1000 / 1000 ---") || strings.Contains(x.Text, "1001 /") || len(qs) != 5 {
		t.Errorf("1001 notes: %+v, %d queries", x, len(qs))
	}
}
