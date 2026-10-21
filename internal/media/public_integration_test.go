//go:build integration

package media

import (
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"log/slog"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/medialink"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis"
	"github.com/danyaa666/smemories/internal/ulid"
)

// T-092 (D-32): public photo links. These tests drive the real router, MySQL, Redis and the object store.

const fixedRequestID = "public-link-test-0001" // the router reuses a client X-Request-Id, so error bodies can be compared byte for byte

func (e *env) bookRow(book string) int64 {
	e.t.Helper()
	var id int64
	if err := e.db.QueryRow(`SELECT id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

func (e *env) epoch(book string) uint32 {
	e.t.Helper()
	var n uint32
	if err := e.db.QueryRow(`SELECT link_epoch FROM yearbook_tab WHERE public_id = ?`, book).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

// token is the link token for a photo of the book at the book's current epoch.
func (e *env) token(mediaID, book string) string { return e.signer.Token(mediaID, book, e.epoch(book)) }

func (e *env) exec(q string, args ...any) int64 {
	e.t.Helper()
	res, err := e.db.Exec(q, args...)
	if err != nil {
		e.t.Fatalf("%s: %v", q, err)
	}
	id, _ := res.LastInsertId()
	return id
}

// note inserts a note of the book with the given status and links the photos to it; it returns the note's row id.
func (e *env) note(book, status string, photos ...string) int64 {
	e.t.Helper()
	row := e.bookRow(book)
	th := sha256.Sum256([]byte(ulid.New(time.Now())))
	coll := e.exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at) VALUES (?,?,?,?,1,1)`,
		ulid.New(time.Now()), row, th[:], "link")
	nid := e.exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, created_at, updated_at) VALUES (?,?,?,?,?,1,1)`,
		ulid.New(time.Now()), coll, row, "{}", status)
	for i, m := range photos {
		e.exec(`INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) SELECT ?, id, ?, 1, 1 FROM media_tab WHERE public_id = ?`, nid, i, m)
	}
	return nid
}

func (e *env) setNoteStatus(nid int64, status string) {
	e.exec(`UPDATE note_tab SET status = ? WHERE id = ?`, status, nid)
}

// public asks for a link anonymously (no cookie, no Origin) from ip ("" = the default test address).
func (e *env) public(token string, ip string) resp { return e.publicReq("GET", token, ip, nil) }

func (e *env) publicReq(method, token, ip string, hdr map[string]string) resp {
	r := httptest.NewRequest(method, "/api/public-media/get-content?token="+url.QueryEscape(token), nil)
	r.Header.Set("X-Request-Id", fixedRequestID)
	if ip != "" {
		r.RemoteAddr = ip + ":4000"
	}
	for k, v := range hdr {
		r.Header.Set(k, v)
	}
	return e.send(user{}, r)
}

// stored is the display object of a photo as the object store holds it.
func (e *env) stored(mediaID string) []byte {
	e.t.Helper()
	var key string
	if err := e.db.QueryRow(`SELECT object_key FROM media_tab WHERE public_id = ?`, mediaID).Scan(&key); err != nil {
		e.t.Fatal(err)
	}
	b, err := e.st.Storage.Get(context.Background(), key)
	if err != nil {
		e.t.Fatal(err)
	}
	return b
}

func (e *env) uploadPhoto(u user, book string) string {
	e.t.Helper()
	return e.upload(u, book, "p.jpg", "image/jpeg", photo(e.t)).status(200, "").mediaID()
}

// wantNotFound checks the one answer every refusal of a link gets, and returns its body.
func wantNotFound(t *testing.T, name string, r resp) string {
	t.Helper()
	if r.Code != 404 || !strings.Contains(r.Body.String(), `"ERROR_NOT_FOUND"`) {
		t.Fatalf("%s: %d (%d body bytes, %s), want 404 ERROR_NOT_FOUND", name, r.Code, r.Body.Len(), r.Header().Get("Content-Type"))
	}
	if r.Header().Get("Set-Cookie") != "" || r.Header().Get("Cache-Control") != "no-store" || strings.Contains(r.Header().Get("Content-Type"), "image") {
		t.Fatalf("%s: headers %v", name, r.Header())
	}
	return r.Body.String()
}

// AC2: the cover, the profile photo and the photo of an approved note are served to anyone with the link, and nothing else is set.
func TestPublicPhotoIsServedAnonymously(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-ok@example.com")
	book := e.newBook(u)
	cover, profile, inNote := e.uploadPhoto(u, book), e.uploadPhoto(u, book), e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, cover)).status(200, "")
	e.updateProfile(u, book, fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, profile)).status(200, "")
	e.note(book, ShareableNoteStatus, inNote)

	for name, id := range map[string]string{"cover": cover, "profile photo": profile, "approved note photo": inNote} {
		rec := e.public(e.token(id, book), "")
		if rec.Code != 200 {
			t.Fatalf("%s: %d %s", name, rec.Code, rec.Body)
		}
		if !bytes.Equal(rec.Body.Bytes(), e.stored(id)) {
			t.Errorf("%s: the body is not the display object", name)
		}
		h := rec.Header()
		for k, want := range map[string]string{
			"Content-Type": "image/jpeg", "X-Content-Type-Options": "nosniff", "Content-Disposition": "inline",
			"Cache-Control": "public, max-age=300", "X-Robots-Tag": "noindex, nofollow", "Referrer-Policy": "no-referrer",
			"Cross-Origin-Resource-Policy": "cross-origin",
		} {
			if h.Get(k) != want {
				t.Errorf("%s: %s = %q, want %q", name, k, h.Get(k), want)
			}
		}
		if h.Get("ETag") == "" || h.Get("Set-Cookie") != "" || len(rec.Result().Cookies()) != 0 {
			t.Errorf("%s: ETag %q, cookies %v", name, h.Get("ETag"), h["Set-Cookie"])
		}
	}

	// HEAD, Range and If-None-Match behave like media/get-content.
	tok := e.token(cover, book)
	full := e.stored(cover)
	head := e.publicReq("HEAD", tok, "", nil)
	if head.Code != 200 || head.Body.Len() != 0 || head.Header().Get("Content-Length") != fmt.Sprint(len(full)) {
		t.Errorf("HEAD: %d, %d body bytes, length %q", head.Code, head.Body.Len(), head.Header().Get("Content-Length"))
	}
	part := e.publicReq("GET", tok, "", map[string]string{"Range": "bytes=0-9"})
	if part.Code != 206 || !bytes.Equal(part.Body.Bytes(), full[:10]) {
		t.Errorf("Range: %d, %d bytes", part.Code, part.Body.Len())
	}
	etag := e.public(tok, "").Header().Get("ETag")
	if nm := e.publicReq("GET", tok, "", map[string]string{"If-None-Match": etag}); nm.Code != 304 {
		t.Errorf("If-None-Match: %d", nm.Code)
	}
	// A revoked link never answers 304 either: the check comes before the content is touched.
	e.json(u, "POST", "/api/yearbook/revoke-photo-links", fmt.Sprintf(`{"id":%q}`, book)).status(200, "OK")
	if nm := e.publicReq("GET", tok, "", map[string]string{"If-None-Match": etag}); nm.Code != 404 {
		t.Errorf("If-None-Match after revoke: %d", nm.Code)
	}
}

// AC2 + AC5: every way a link can be refused is the same 404 with the same body (the request id is pinned by the client).
func TestPublicPhotoRefusalsAreIdentical(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-no@example.com")
	other := e.register("public-other@example.com")
	book, bookB := e.newBook(u), e.newBook(other)

	pending, hidden, deletedNote, unlinked, deletedMedia := e.uploadPhoto(u, book), e.uploadPhoto(u, book), e.uploadPhoto(u, book), e.uploadPhoto(u, book), e.uploadPhoto(u, book)
	inB := e.uploadPhoto(other, bookB)
	e.note(bookB, ShareableNoteStatus, inB)
	e.note(book, "pending", pending)
	e.note(book, "hidden", hidden)
	dn := e.note(book, ShareableNoteStatus, deletedNote)
	e.note(book, ShareableNoteStatus, deletedMedia)
	staleTok := e.token(deletedNote, book)
	if e.public(staleTok, "").Code != 200 {
		t.Fatal("the approved note photo is not served before the note is deleted")
	}
	e.exec(`DELETE FROM note_photo_tab WHERE note_id = ?`, dn)
	e.exec(`DELETE FROM note_tab WHERE id = ?`, dn)
	deletedMediaTok := e.token(deletedMedia, book)
	e.deleteMedia(u, deletedMedia).status(200, "OK")

	goodTok := e.token(inB, bookB)
	mac := goodTok[strings.IndexByte(goodTok, '.')+1:]
	flip := func(s string) string { // change one character of the MAC
		if s[len(s)-5] == 'A' {
			return s[:len(s)-5] + "B" + s[len(s)-4:]
		}
		return s[:len(s)-5] + "A" + s[len(s)-4:]
	}
	otherSecret, _ := medialink.New(bytes.Repeat([]byte{9}, medialink.MinSecretBytes))
	cases := map[string]string{
		"pending note":           e.token(pending, book),
		"hidden note":            e.token(hidden, book),
		"deleted note":           staleTok,
		"deleted media":          deletedMediaTok,
		"photo no one refers to": e.token(unlinked, book),
		"another book's media with this book's MAC":  e.signer.Token(inB, book, e.epoch(book)),
		"another book's media, its book's old epoch": e.signer.Token(inB, bookB, e.epoch(bookB)+1),
		"wrong epoch":        e.signer.Token(inB, bookB, e.epoch(bookB)+7),
		"tampered MAC":       flip(goodTok),
		"another secret":     otherSecret.Token(inB, bookB, e.epoch(bookB)),
		"unknown photo":      medialinkUnknown(e, book),
		"truncated":          goodTok[:len(goodTok)-1],
		"oversized":          goodTok + strings.Repeat("A", 400),
		"id only":            inB,
		"empty":              "",
		"garbage":            "not-a-token",
		"mac of another id":  inB[:25] + "0." + mac,
		"lower-case id":      strings.ToLower(inB) + "." + mac,
		"NUL in the token":   inB + ".\x00" + mac[1:],
		"query-like garbage": "' OR 1=1 --",
	}
	// Delete the book last: its token is made before and must die with it.
	delTok := e.token(inB, bookB)
	e.deleteBook(other, bookB).status(200, "OK")
	cases["deleted book"] = delTok

	var want string
	for name, tok := range cases {
		got := wantNotFound(t, name, e.public(tok, ""))
		if want == "" {
			want = got
		}
		if got != want {
			t.Errorf("%s: body differs from the others:\n%s\n%s", name, got, want)
		}
	}
	// ... and a missing parameter is the same 404, not a 400 that would tell a guesser the shape.
	r := httptest.NewRequest("GET", "/api/public-media/get-content", nil)
	r.Header.Set("X-Request-Id", fixedRequestID)
	if got := wantNotFound(t, "no token parameter", e.send(user{}, r)); got != want {
		t.Errorf("no token parameter: body differs: %s", got)
	}
}

// medialinkUnknown is a well-formed, correctly signed token for a photo that does not exist.
func medialinkUnknown(e *env, book string) string {
	return e.signer.Token("01J0000000000000000000000Z", book, e.epoch(book))
}

// A note hidden after its link was handed out stops serving at once, and serves again when it is approved again.
func TestPublicPhotoFollowsTheNoteStatus(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-status@example.com")
	book := e.newBook(u)
	id := e.uploadPhoto(u, book)
	nid := e.note(book, ShareableNoteStatus, id)
	tok := e.token(id, book)
	if e.public(tok, "").Code != 200 {
		t.Fatal("approved note photo not served")
	}
	for _, status := range []string{"hidden", "pending"} {
		e.setNoteStatus(nid, status)
		wantNotFound(t, status, e.public(tok, ""))
	}
	e.setNoteStatus(nid, ShareableNoteStatus)
	if e.public(tok, "").Code != 200 {
		t.Fatal("not served again after approval")
	}
	// Taking the cover away unshares a photo that was only the cover.
	cover := e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, cover)).status(200, "")
	ct := e.token(cover, book)
	if e.public(ct, "").Code != 200 {
		t.Fatal("cover not served")
	}
	e.updateBook(u, book, `{"cover_media_id":null}`).status(200, "")
	wantNotFound(t, "former cover", e.public(ct, ""))
}

// AC2: revoke-photo-links kills every earlier link at once, a token for the new epoch works, and only the owner can do it.
func TestRevokePhotoLinks(t *testing.T) {
	e := newEnv(t)
	u, stranger := e.register("revoke-owner@example.com"), e.register("revoke-stranger@example.com")
	book := e.newBook(u)
	a, b := e.uploadPhoto(u, book), e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, a)).status(200, "")
	e.updateProfile(u, book, fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, b)).status(200, "")
	oldA, oldB := e.token(a, book), e.token(b, book)
	if e.public(oldA, "").Code != 200 || e.public(oldB, "").Code != 200 {
		t.Fatal("links do not work before the revoke")
	}
	revoke := func(who user, id string) resp {
		return e.json(who, "POST", "/api/yearbook/revoke-photo-links", fmt.Sprintf(`{"id":%q}`, id))
	}
	r := revoke(u, book).status(200, "OK")
	if !strings.Contains(r.Body.String(), `"data":{}`) {
		t.Errorf("data is not empty: %s", r.Body)
	}
	if e.epoch(book) != 1 {
		t.Fatalf("epoch %d, want 1", e.epoch(book))
	}
	wantNotFound(t, "old link a", e.public(oldA, ""))
	wantNotFound(t, "old link b", e.public(oldB, ""))
	if rec := e.public(e.token(a, book), ""); rec.Code != 200 {
		t.Fatalf("a token for the new epoch: %d %s", rec.Code, rec.Body)
	}

	revoke(stranger, book).status(404, "ERROR_NOT_FOUND")
	revoke(u, "01J0000000000000000000000Z").status(404, "ERROR_NOT_FOUND")
	revoke(u, "").status(400, "ERROR_PARAM")
	e.json(user{}, "POST", "/api/yearbook/revoke-photo-links", fmt.Sprintf(`{"id":%q}`, book)).status(401, "ERROR_UNAUTHORIZED")
	if e.epoch(book) != 1 {
		t.Fatalf("a refused revoke changed the epoch to %d", e.epoch(book))
	}
	// The revoke does not touch the order of the owner's books.
	var upd1, upd2 int64
	_ = e.db.QueryRow(`SELECT updated_at FROM yearbook_tab WHERE public_id = ?`, book).Scan(&upd1)
	revoke(u, book).status(200, "OK")
	_ = e.db.QueryRow(`SELECT updated_at FROM yearbook_tab WHERE public_id = ?`, book).Scan(&upd2)
	if upd1 != upd2 || e.epoch(book) != 2 {
		t.Errorf("updated_at %d -> %d, epoch %d", upd1, upd2, e.epoch(book))
	}
}

func TestRevokePhotoLinksCountsEveryParallelCall(t *testing.T) {
	e := newEnv(t)
	u := e.register("revoke-par@example.com")
	book := e.newBook(u)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			e.json(u, "POST", "/api/yearbook/revoke-photo-links", fmt.Sprintf(`{"id":%q}`, book)).status(200, "OK")
		})
	}
	wg.Wait()
	if e.epoch(book) != 8 {
		t.Fatalf("epoch %d after 8 parallel revokes, want 8", e.epoch(book))
	}
}

// AC5: 10 revokes per hour per user, the 11th is 429 with Retry-After and changes nothing.
func TestRevokePhotoLinksIsLimited(t *testing.T) {
	e := newEnv(t)
	u := e.register("revoke-limit@example.com")
	book := e.newBook(u)
	for range revokesPerWindowForTest {
		e.json(u, "POST", "/api/yearbook/revoke-photo-links", fmt.Sprintf(`{"id":%q}`, book)).status(200, "OK")
	}
	r := e.json(u, "POST", "/api/yearbook/revoke-photo-links", fmt.Sprintf(`{"id":%q}`, book)).status(429, "ERROR_RATE_LIMITED")
	if r.Header().Get("Retry-After") == "" {
		t.Error("no Retry-After")
	}
	if e.epoch(book) != revokesPerWindowForTest {
		t.Errorf("epoch %d, want %d", e.epoch(book), revokesPerWindowForTest)
	}
}

const revokesPerWindowForTest = 10 // docs/redis.md: revoke_links

// AC4: a photo uploaded with GPS EXIF, an XMP packet and a comment is served without any of it.
func TestPublicPhotoCarriesNoMetadata(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-exif@example.com")
	book := e.newBook(u)
	xmp := segment(0xE1, "http://ns.adobe.com/xap/1.0/\x00<x:xmpmeta><exif:GPSLatitude>10,45.5N</exif:GPSLatitude></x:xmpmeta>")
	in := jpegWith(t, quad(64, 32, false), exifSegment(1), xmp, segment(0xFE, "taken at the secret place"), segment(0xE2, "ICC_PROFILE\x00x"))
	for _, s := range []string{"Exif", "GPSLatitude", "secret place"} {
		if !bytes.Contains(in, []byte(s)) {
			t.Fatalf("the fixture has no %q", s)
		}
	}
	id := e.upload(u, book, "p.jpg", "image/jpeg", in).status(200, "").mediaID()
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, id)).status(200, "")
	rec := e.public(e.token(id, book), "")
	if rec.Code != 200 {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	got := rec.Body.Bytes()
	for _, s := range []string{"Exif", "GPS", "xmpmeta", "secret place", "ICC_PROFILE", "http://ns.adobe.com"} {
		if bytes.Contains(got, []byte(s)) {
			t.Errorf("the served bytes contain %q", s)
		}
	}
	for _, m := range markers(t, got) {
		if m >= 0xE1 && m <= 0xEF || m == 0xFE {
			t.Errorf("the served JPEG has a metadata segment, marker %#x", m)
		}
	}
}

// AC5: the miss limiter counts only misses (60 per 15 minutes per IP), the answer is 429 with Retry-After, and another IP is unaffected.
func TestPublicMissLimiter(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-miss@example.com")
	book := e.newBook(u)
	id := e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, id)).status(200, "")
	tok := e.token(id, book)

	for range publicMissesPerWindow + 20 { // good links do not use the budget
		if rec := e.public(tok, "203.0.113.5"); rec.Code != 200 {
			t.Fatalf("good link: %d %s", rec.Code, rec.Body)
		}
	}
	for i := range publicMissesPerWindow {
		wantNotFound(t, fmt.Sprintf("miss %d", i), e.public("garbage", "203.0.113.5"))
	}
	r := e.public("garbage", "203.0.113.5")
	if r.Code != 429 || !strings.Contains(r.Body.String(), "ERROR_RATE_LIMITED") || r.Header().Get("Retry-After") == "" {
		t.Fatalf("miss %d: %d %s %v", publicMissesPerWindow+1, r.Code, r.Body, r.Header())
	}
	if r := e.public(tok, "203.0.113.5"); r.Code != 429 { // the limit is checked before the token, so it cannot be probed past
		t.Errorf("a good link past the limit: %d", r.Code)
	}
	if r := e.public(tok, "203.0.113.99"); r.Code != 200 {
		t.Errorf("another address: %d", r.Code)
	}
	wantNotFound(t, "another address, miss", e.public("garbage", "203.0.113.99"))

	// The counters of the three results, and nothing else in the label set.
	m := scrape(t, e)
	for _, want := range []string{`smem_public_media_requests_total{result="ok"} 81`, `smem_public_media_requests_total{result="not_found"} 61`, `smem_public_media_requests_total{result="rate_limited"} 2`} {
		if !strings.Contains(m, want) {
			t.Errorf("metrics lack %q:\n%s", want, grep(m, "public_media"))
		}
	}
}

func scrape(t *testing.T, e *env) string {
	t.Helper()
	rec := httptest.NewRecorder()
	e.metrics.Handler().ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	return rec.Body.String()
}

func grep(s, sub string) string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		if strings.Contains(l, sub) {
			out = append(out, l)
		}
	}
	return strings.Join(out, "\n")
}

// AC5: 600 requests per 15 minutes per IP of any kind, good links included.
func TestPublicAllLimiter(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-all@example.com")
	book := e.newBook(u)
	id := e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, id)).status(200, "")
	tok := e.token(id, book)
	for i := range publicAllPerWindow {
		if rec := e.publicReq("HEAD", tok, "203.0.113.7", nil); rec.Code != 200 {
			t.Fatalf("request %d: %d", i+1, rec.Code)
		}
	}
	if r := e.public(tok, "203.0.113.7"); r.Code != 429 || r.Header().Get("Retry-After") == "" {
		t.Fatalf("request %d: %d %v", publicAllPerWindow+1, r.Code, r.Header())
	}
}

// AC5: with Redis down the public endpoint still answers (both limiters fail open), logging one ERROR a minute.
func TestPublicLimitersFailOpen(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-open@example.com")
	book := e.newBook(u)
	id := e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, id)).status(200, "")
	tok := e.token(id, book)

	dead, err := redis.New(config.Config{Env: "test", RedisURL: "redis://127.0.0.1:1/0",
		RedisDialTimeout: 200 * time.Millisecond, RedisReadTimeout: 200 * time.Millisecond, RedisWriteTimeout: 200 * time.Millisecond, RedisPoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dead.Close() }()
	lb := &syncBuf{}
	f := ratelimit.NewFactory(dead, slog.New(slog.NewJSONHandler(lb, nil)), nil)
	e.svc.pubMiss, e.svc.pubAll = f.Open("public_media_miss", publicMissesPerWindow, publicWindow), f.Open("public_media_all", publicAllPerWindow, publicWindow)

	for range 5 {
		if rec := e.public(tok, ""); rec.Code != 200 {
			t.Fatalf("good link with Redis down: %d %s", rec.Code, rec.Body)
		}
		wantNotFound(t, "miss with Redis down", e.public("garbage", ""))
	}
	if n := strings.Count(lb.String(), "ratelimit: limiter failed"); n != 1 {
		t.Errorf("%d limiter ERROR lines for 20 failures, want 1 per minute:\n%s", n, lb.String())
	}
}

// AC3 + requirement 7: the token and its MAC never reach a log line, an error body or /metrics, on any path.
func TestPublicTokenNeverReachesLogsOrMetrics(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-log@example.com")
	book := e.newBook(u)
	id := e.uploadPhoto(u, book)
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, id)).status(200, "")
	good := e.token(id, book)
	goodMAC := good[strings.IndexByte(good, '.')+1:]
	const marker = "RECOGNISABLE-TOKEN-MARKER-0123456789"
	secret := strings.Repeat("L", medialink.MinSecretBytes)

	bodies := []string{e.public(marker, "").Body.String(), e.public(good[:len(good)-2]+"AA", "").Body.String(),
		e.public(marker+"."+goodMAC, "").Body.String()}
	e.st.mu.Lock()
	e.st.failGet = true
	e.st.mu.Unlock()
	storageDown := e.public(good, "")
	if storageDown.Code != 502 {
		t.Fatalf("storage down: %d %s", storageDown.Code, storageDown.Body)
	}
	bodies = append(bodies, storageDown.Body.String())
	e.st.mu.Lock()
	e.st.failGet = false
	e.st.mu.Unlock()
	for range publicMissesPerWindow { // run into the 429 as well
		e.public(marker, "")
	}
	bodies = append(bodies, e.public(good, "").Body.String())

	logs, m := e.logs.String(), scrape(t, e)
	if !strings.Contains(logs, `"route":"GET /api/public-media/get-content"`) || !strings.Contains(logs, "media: storage failed") {
		t.Fatalf("the access log or the storage ERROR is missing, so this test proves nothing:\n%s", logs)
	}
	for _, what := range map[string]string{"logs": logs, "metrics": m, "bodies": strings.Join(bodies, "\n")} {
		for _, secretText := range []string{good, goodMAC, marker, secret, "SMEM_MEDIA_LINK_SECRET"} {
			if strings.Contains(what, secretText) {
				t.Errorf("%q found in the output", secretText)
			}
		}
	}
}

// The photo bytes of a photo past the limits of the encoder are still the display object: a 3000 px photo goes through whole.
func TestPublicPhotoIsTheDisplayVariantOnly(t *testing.T) {
	e := newEnv(t)
	u := e.register("public-variant@example.com")
	book := e.newBook(u)
	id := e.upload(u, book, "p.jpg", "image/jpeg", jpegWith(t, quad(2600, 1300, false))).status(200, "").mediaID()
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, id)).status(200, "")
	tok := e.token(id, book)
	for _, q := range []string{"&variant=thumb", "&variant=print", "&id=" + id} { // there is no way to ask for another size
		r := httptest.NewRequest("GET", "/api/public-media/get-content?token="+url.QueryEscape(tok)+q, nil)
		rec := e.send(user{}, r)
		if rec.Code != 200 || !bytes.Equal(rec.Body.Bytes(), e.stored(id)) {
			t.Errorf("%s: %d, display bytes %v", q, rec.Code, bytes.Equal(rec.Body.Bytes(), e.stored(id)))
		}
	}
	var printKey string
	_ = e.db.QueryRow(`SELECT print_key FROM media_tab WHERE public_id = ?`, id).Scan(&printKey)
	if bytes.Equal(e.stored(id), mustGet(t, e, printKey)) {
		t.Fatal("the print object equals the display object: this photo cannot show the difference")
	}
}

func mustGet(t *testing.T, e *env, key string) []byte {
	t.Helper()
	b, err := e.st.Storage.Get(context.Background(), key)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
