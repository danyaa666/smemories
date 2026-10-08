//go:build integration

package notes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/storage"
	"github.com/danyaa666/smemories/internal/ulid"
)

// flakyStore wraps a Storage, remembers the objects written through it and fails or hooks puts on demand.
type flakyStore struct {
	storage.Storage
	mu      sync.Mutex
	keys    map[string]bool
	puts    int
	failAt  int            // fail the nth put (1-based); 0 = never
	afterFn func(puts int) // called after every successful put
}

func (f *flakyStore) Put(ctx context.Context, key, ct string, data []byte) error {
	f.mu.Lock()
	f.puts++
	n := f.puts
	fail := f.failAt == n
	f.mu.Unlock()
	if fail {
		return errors.New("injected put failure")
	}
	if err := f.Storage.Put(ctx, key, ct, data); err != nil {
		return err
	}
	f.mu.Lock()
	f.keys[key] = true
	after := f.afterFn
	f.mu.Unlock()
	if after != nil {
		after(n)
	}
	return nil
}

func (f *flakyStore) Delete(ctx context.Context, keys ...string) error {
	if err := f.Storage.Delete(ctx, keys...); err != nil {
		return err
	}
	f.mu.Lock()
	for _, k := range keys {
		delete(f.keys, k)
	}
	f.mu.Unlock()
	return nil
}

func (f *flakyStore) DeletePrefix(ctx context.Context, prefix string) error {
	if err := f.Storage.DeletePrefix(ctx, prefix); err != nil {
		return err
	}
	f.mu.Lock()
	for k := range f.keys {
		if strings.HasPrefix(k, prefix) {
			delete(f.keys, k)
		}
	}
	f.mu.Unlock()
	return nil
}

func (f *flakyStore) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.keys)
}

type part struct {
	name, filename, ctype string
	data                  []byte
}

func text(name, v string) part { return part{name: name, data: []byte(v)} }

func photoPart(data []byte) part { return part{"photos", "me.jpg", "image/jpeg", data} }

// answers is the "answers" part for a name and a message.
func answers(name, message string) part {
	b, _ := json.Marshal(map[string]string{"name": name, "message": message})
	return text("answers", string(b))
}

// jpegOf makes a small distinct JPEG.
func jpegOf(t testing.TB, seed uint8) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 64, 48))
	for y := range 48 {
		for x := range 64 {
			img.SetNRGBA(x, y, color.NRGBA{seed, uint8(x * 4), uint8(y * 5), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, nil); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func multipartOf(parts ...part) (*bytes.Buffer, string) {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	for _, p := range parts {
		h := textproto.MIMEHeader{}
		cd := fmt.Sprintf(`form-data; name=%q`, p.name)
		if p.filename != "" {
			cd += fmt.Sprintf(`; filename=%q`, p.filename)
		}
		h.Set("Content-Disposition", cd)
		if p.ctype != "" {
			h.Set("Content-Type", p.ctype)
		}
		w, _ := mw.CreatePart(h)
		_, _ = w.Write(p.data)
	}
	_ = mw.Close()
	return &b, mw.FormDataContentType()
}

func (e *env) postFrom(ctx context.Context, ip, token string, parts ...part) resp {
	e.t.Helper()
	body, ct := multipartOf(parts...)
	r := httptest.NewRequest("POST", "/v1/public/collect/"+token+"/notes", body).WithContext(ctx)
	r.Header.Set("Content-Type", ct)
	r.RemoteAddr = ip + ":4000"
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return resp{rec, e.t}
}

func (e *env) post(token string, parts ...part) resp {
	e.t.Helper()
	return e.postFrom(context.Background(), "192.0.2.1", token, parts...)
}

// link is an owner with a book and an open collection link.
type link struct {
	owner       user
	book, token string
	collection  string // public id
}

func (e *env) newOpenLink() link {
	e.t.Helper()
	n := e.count(`SELECT COUNT(*) FROM users`)
	owner := e.register(fmt.Sprintf("owner%d@example.com", n), "Owner", true)
	book := e.book(owner, "Class 12A")
	id, token := e.newLink(owner, book, `{}`)
	return link{owner, book, token, id}
}

func (e *env) noteCount() int { return e.count(`SELECT COUNT(*) FROM notes`) }

// nothingKept asserts a failed submission left no note, photo row, media row or object behind.
func (e *env) nothingKept() {
	e.t.Helper()
	for _, q := range []string{`SELECT COUNT(*) FROM notes`, `SELECT COUNT(*) FROM note_photos`, `SELECT COUNT(*) FROM media`} {
		if n := e.count(q); n != 0 {
			e.t.Fatalf("%s = %d, want 0", q, n)
		}
	}
	if n := e.st.count(); n != 0 {
		e.t.Fatalf("%d objects left in storage", n)
	}
}

// AC1, AC2: a text-only note is stored pending; emoji and Vietnamese survive the JSON column byte for byte.
func TestSubmitTextAndEmojiRoundTrip(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	msg := "Chúc bạn mãi vui 🎉👨‍👩‍👧‍👦🏳️‍🌈 ❤️ Ắ \"quoted\" <b>&</b> \\ \n\nline 2 end"
	rec := e.post(l.token, answers("  Nguyễn Văn Ánh  ", msg), text("answers2", "ignored")).status(201, "")
	var out struct{ Note struct{ ID string } }
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || len(out.Note.ID) != 26 || strings.TrimSpace(rec.Body.String()) != fmt.Sprintf(`{"note":{"id":%q}}`, out.Note.ID) {
		t.Fatalf("body %s", rec.Body.String())
	}
	var raw, status string
	var sortOrder *int
	if err := e.db.QueryRow(`SELECT answers, status, sort_order FROM notes WHERE public_id = ?`, out.Note.ID).Scan(&raw, &status, &sortOrder); err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "Nguyễn Văn Ánh" || got["message"] != msg || len(got) != 2 || status != "pending" || sortOrder != nil {
		t.Fatalf("stored %q status %s sort %v", got, status, sortOrder)
	}
	// the owner's list now counts it
	list := e.do(l.owner, "GET", "/v1/yearbooks/"+l.book+"/collections", "").status(200, "").json()["collections"].([]any)
	if list[0].(map[string]any)["note_count"] != float64(1) {
		t.Fatalf("note_count: %v", list[0])
	}
}

// AC1, AC3: photos go through the pipeline as contributor uploads, in order; the part order of the form does not matter.
func TestSubmitWithPhotos(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	rec := e.post(l.token, photoPart(jpegOf(t, 10)), answers("An", "hello"), photoPart(jpegOf(t, 200))).status(201, "")
	id := rec.json()["note"].(map[string]any)["id"].(string)
	if n := e.count(`SELECT COUNT(*) FROM media WHERE uploader_kind = 'contributor'`); n != 2 {
		t.Fatalf("%d contributor media", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM note_photos np JOIN notes n ON n.id = np.note_id WHERE n.public_id = ? AND np.position IN (0, 1)`, id); n != 2 {
		t.Fatalf("%d photo links", n)
	}
	if e.st.count() != 4 { // display + thumbnail each
		t.Fatalf("%d objects", e.st.count())
	}
	// an empty file input of a browser (no file name, no bytes) is not a photo; three real photos are the maximum
	e.post(l.token, answers("B", "m"), part{name: "photos", filename: "", ctype: "application/octet-stream"},
		photoPart(jpegOf(t, 1)), photoPart(jpegOf(t, 2)), photoPart(jpegOf(t, 3))).status(201, "")
	e.post(l.token, answers("C", "m"), photoPart(jpegOf(t, 1)), photoPart(jpegOf(t, 2)), photoPart(jpegOf(t, 3)), photoPart(jpegOf(t, 4))).status(400, "too_many_photos")
	if e.noteCount() != 2 || e.count(`SELECT COUNT(*) FROM media`) != 5 {
		t.Fatalf("notes %d media %d", e.noteCount(), e.count(`SELECT COUNT(*) FROM media`))
	}
}

// AC1, AC2: field and body errors name the field id, never the value; nothing is stored.
func TestSubmitValidation(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	secret := "SECRET-VALUE-" + strings.Repeat("x", 70)
	tests := []struct {
		name    string
		parts   []part
		status  int
		code    string
		message string // substring of the error message
	}{
		{"unknown field", []part{text("answers", `{"name":"A","message":"m","nickname":"n"}`)}, 400, "unknown_field", "nickname"},
		{"hostile unknown id is not echoed", []part{text("answers", `{"name":"A","message":"m","<script>":"n"}`)}, 400, "unknown_field", ""},
		{"missing required", []part{text("answers", `{"name":"A"}`)}, 400, "missing_answer", "message"},
		{"blank required", []part{text("answers", `{"name":"  ","message":"m"}`)}, 400, "missing_answer", "name"},
		{"too long", []part{text("answers", `{"name":"`+secret+`","message":"m"}`)}, 400, "invalid_answer", "name"},
		{"control character", []part{text("answers", `{"name":"A\u0000","message":"m"}`)}, 400, "invalid_answer", "name"},
		{"bidi override", []part{text("answers", `{"name":"A\u202eB","message":"m"}`)}, 400, "invalid_answer", "name"},
		{"newline in a short field", []part{text("answers", `{"name":"A\nB","message":"m"}`)}, 400, "invalid_answer", "name"},
		{"not an object", []part{text("answers", `["a"]`)}, 400, "invalid_body", ""},
		{"null", []part{text("answers", `null`)}, 400, "invalid_body", ""},
		{"non-string value", []part{text("answers", `{"name":"A","message":5}`)}, 400, "invalid_body", ""},
		{"not json", []part{text("answers", `name=A`)}, 400, "invalid_body", ""},
		{"invalid utf-8", []part{{name: "answers", data: []byte("{\"name\":\"A\xff\",\"message\":\"m\"}")}}, 400, "invalid_body", ""},
		{"over 16 KiB", []part{text("answers", `{"name":"A","message":"`+strings.Repeat("a", 16<<10)+`"}`)}, 400, "invalid_body", ""},
		{"answers twice", []part{answers("A", "m"), answers("A", "m")}, 400, "invalid_body", ""},
		{"no answers", []part{photoPart(jpegOf(t, 1))}, 400, "invalid_body", ""},
		{"no parts", nil, 400, "invalid_body", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := e.post(l.token, tc.parts...).status(tc.status, tc.code)
			if !strings.Contains(rec.Body.String(), tc.message) || strings.Contains(rec.Body.String(), "SECRET-VALUE") || strings.Contains(rec.Body.String(), "script") {
				t.Fatalf("message: %s", rec.Body.String())
			}
		})
	}
	e.nothingKept()

	// the 415s: anything but multipart/form-data
	r := httptest.NewRequest("POST", "/v1/public/collect/"+l.token+"/notes", strings.NewReader(`{"answers":{}}`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	resp{rec, t}.status(415, "unsupported_media_type")
	r = httptest.NewRequest("POST", "/v1/public/collect/"+l.token+"/notes", nil)
	rec = httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	resp{rec, t}.status(415, "unsupported_media_type")
}

// AC3: one bad photo rejects the whole submission and nothing is stored, whatever came before it.
func TestSubmitBadPhotoRejectsAll(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	good := jpegOf(t, 7)
	bad := map[string]struct {
		data   []byte
		status int
		code   string
	}{
		"html named jpg": {[]byte("<html><script>alert(1)</script></html>"), 415, "unsupported_media_type"},
		"empty file":     {nil, 415, "unsupported_media_type"},
		"truncated jpeg": {good[:len(good)/2], 400, "invalid_image"},
	}
	for name, b := range bad {
		t.Run(name, func(t *testing.T) {
			e.post(l.token, answers("A", "m"), photoPart(good), photoPart(b.data)).status(b.status, b.code)
			e.nothingKept()
		})
	}
	// a photo over the per-file cap
	big := append([]byte{0xff, 0xd8, 0xff}, make([]byte, 10<<20)...)
	e.post(l.token, answers("A", "m"), photoPart(good), photoPart(big)).status(413, "payload_too_large")
	e.nothingKept()
}

// AC4: a failing store and a failing insert leave no row and no object behind, also when the client goes away.
func TestSubmitCompensation(t *testing.T) {
	t.Run("store fails on the second photo", func(t *testing.T) {
		e := newEnv(t)
		l := e.newOpenLink()
		e.st.failAt = 3 // photo 1 takes puts 1-2
		e.post(l.token, answers("A", "m"), photoPart(jpegOf(t, 1)), photoPart(jpegOf(t, 2))).status(502, "storage_error")
		e.nothingKept()
	})
	t.Run("thumbnail of the last photo fails", func(t *testing.T) {
		e := newEnv(t)
		l := e.newOpenLink()
		e.st.failAt = 4
		e.post(l.token, answers("A", "m"), photoPart(jpegOf(t, 1)), photoPart(jpegOf(t, 2))).status(502, "storage_error")
		e.nothingKept()
	})
	t.Run("insert fails after the photos were stored", func(t *testing.T) {
		e := newEnv(t)
		l := e.newOpenLink()
		if _, err := e.db.Exec(`DROP TABLE note_photos`); err != nil {
			t.Fatal(err)
		}
		e.post(l.token, answers("A", "m"), photoPart(jpegOf(t, 1)), photoPart(jpegOf(t, 2))).status(500, "internal_error")
		if e.count(`SELECT COUNT(*) FROM notes`) != 0 || e.count(`SELECT COUNT(*) FROM media`) != 0 || e.st.count() != 0 {
			t.Fatal("note, media or objects left behind")
		}
	})
	t.Run("client disconnects after the first photo", func(t *testing.T) {
		e := newEnv(t)
		l := e.newOpenLink()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		e.st.afterFn = func(puts int) {
			if puts == 4 { // both photos' objects... the second one's display object is put (puts 3-4); the request is gone
				cancel()
			}
		}
		e.postFrom(ctx, "192.0.2.1", l.token, answers("A", "m"), photoPart(jpegOf(t, 1)), photoPart(jpegOf(t, 2)))
		e.nothingKept()
	})
	t.Run("client gone before anything", func(t *testing.T) {
		e := newEnv(t)
		l := e.newOpenLink()
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		e.postFrom(ctx, "192.0.2.1", l.token, answers("A", "m"), photoPart(jpegOf(t, 1)))
		e.nothingKept()
	})
}

// AC5: unknown, malformed and revoked links are 404, an expired one 410; nothing is stored or processed.
func TestSubmitClosedLinks(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	deadline := e.now().Add(time.Hour)
	_, withDeadline := e.newLink(l.owner, l.book, fmt.Sprintf(`{"deadline_at":%q}`, deadline.Format(time.RFC3339)))
	revID, revoked := e.newLink(l.owner, l.book, `{}`)
	e.do(l.owner, "DELETE", "/v1/collections/"+revID, "").status(204, "")
	for _, tok := range []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA", "short", strings.Repeat("a", 5000), revoked} {
		e.post(tok, answers("A", "m"), photoPart(jpegOf(t, 1))).status(404, "not_found")
	}
	e.post(withDeadline, answers("A", "m")).status(201, "")
	e.advance(time.Hour)
	e.post(withDeadline, answers("A", "m"), photoPart(jpegOf(t, 1))).status(410, "collection_closed")
	if e.noteCount() != 1 || e.count(`SELECT COUNT(*) FROM media`) != 0 || e.st.count() != 0 {
		t.Fatal("a closed link stored something")
	}
	// the owner revokes the link while a submission is being processed: nothing is kept
	e.st.afterFn = func(puts int) {
		if puts == 2 {
			e.do(l.owner, "DELETE", "/v1/collections/"+l.collection, "").status(204, "")
		}
	}
	e.post(l.token, answers("A", "m"), photoPart(jpegOf(t, 1))).status(404, "not_found")
	if e.noteCount() != 1 || e.count(`SELECT COUNT(*) FROM media`) != 0 || e.st.count() != 0 {
		t.Fatal("a link revoked mid-flight kept a photo or a note")
	}
}

func (e *env) fillCollection(l link, n int) {
	e.t.Helper()
	var coll, book uint64
	if err := e.db.QueryRow(`SELECT c.id, c.yearbook_id FROM note_collections c WHERE c.public_id = ?`, l.collection).Scan(&coll, &book); err != nil {
		e.t.Fatal(err)
	}
	tx, _ := e.db.Begin()
	for range n {
		if _, err := tx.Exec(`INSERT INTO notes (public_id, collection_id, yearbook_id, answers, status, created_at) VALUES (?,?,?,?,?,?)`,
			ulid.New(e.now()), coll, book, `{"name":"x","message":"y"}`, []string{"pending", "approved", "hidden"}[n%3], e.now()); err != nil {
			e.t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		e.t.Fatal(err)
	}
}

// AC6: size limits, the 300-note cap (in any status, also under a race) and the media quota.
func TestSubmitLimits(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()

	// the whole body is capped at 32 MiB, wherever the excess is
	e.post(l.token, answers("A", "m"), part{name: "padding", data: make([]byte, 33<<20)}).status(413, "payload_too_large")

	e.fillCollection(l, 295)
	var wg sync.WaitGroup
	codes := make(chan int, 10)
	for i := range 10 { // 5 places left, 10 parallel submissions
		wg.Add(1)
		go func() {
			defer wg.Done()
			codes <- e.postFrom(context.Background(), fmt.Sprintf("198.51.100.%d", i+1), l.token, answers("A", "m")).Code
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
			t.Fatalf("status %d", c)
		}
	}
	if created != 5 || e.noteCount() != 300 {
		t.Fatalf("%d created, %d notes", created, e.noteCount())
	}
	e.post(l.token, answers("A", "m"), photoPart(jpegOf(t, 1))).status(409, "collection_full")
	if e.count(`SELECT COUNT(*) FROM media`) != 0 || e.st.count() != 0 {
		t.Fatal("a full collection processed a photo")
	}
	// another collection of the same book is unaffected
	_, other := e.newLink(l.owner, l.book, `{}`)
	e.post(other, answers("A", "m")).status(201, "")

	// the yearbook's media quota (200 photos) counts contributor photos
	e2 := newEnv(t)
	l2 := e2.newOpenLink()
	var yb uint64
	if err := e2.db.QueryRow(`SELECT id FROM yearbooks WHERE public_id = ?`, l2.book).Scan(&yb); err != nil {
		t.Fatal(err)
	}
	for range 200 {
		if _, err := e2.db.Exec(`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at)
			VALUES (?,?,'owner','k','t','image/jpeg',1,1,1,'x',?)`, ulid.New(e2.now()), yb, e2.now()); err != nil {
			t.Fatal(err)
		}
	}
	e2.post(l2.token, answers("A", "m"), photoPart(jpegOf(t, 1))).status(409, "quota_exceeded")
	if e2.noteCount() != 0 || e2.st.count() != 0 || e2.count(`SELECT COUNT(*) FROM media`) != 200 {
		t.Fatal("quota failure kept something")
	}
	e2.post(l2.token, answers("A", "m")).status(201, "") // text-only still fits
}

// AC7: per-collection and per-IP limits count only requests that passed validation; a class behind one address fits.
func TestSubmitRateLimits(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	for range 40 { // 40 different students, one campus IP, one submission each
		e.post(l.token, answers("Student", "hi")).status(201, "")
	}
	for range 5 { // invalid requests are not counted
		e.post(l.token, text("answers", `{"name":"A"}`)).status(400, "missing_answer")
	}
	for range 20 {
		e.post(l.token, answers("Student", "hi")).status(201, "")
	}
	rec := e.post(l.token, answers("Student", "hi")).status(429, "rate_limited") // 61st in the hour for this collection
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	// the same address on another collection has 40 of its 100 hourly submissions left
	_, other := e.newLink(l.owner, l.book, `{}`)
	for range 40 {
		e.post(other, answers("S", "hi")).status(201, "")
	}
	e.post(other, answers("S", "hi")).status(429, "rate_limited")
	e.postFrom(context.Background(), "203.0.113.9", other, answers("S", "hi")).status(201, "") // another IP is fine
	e.advance(time.Hour + time.Second)
	e.post(other, answers("S", "hi")).status(201, "")
}

// AC8: a filled honeypot looks like success and keeps nothing; the verifier runs before any photo is processed.
func TestHoneypotAndVerifier(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	rec := e.post(l.token, answers("Bot", "buy now"), text("website", "http://spam.example"), photoPart(jpegOf(t, 1))).status(201, "")
	if id := rec.json()["note"].(map[string]any)["id"].(string); len(id) != 26 {
		t.Fatalf("honeypot body %s", rec.Body.String())
	}
	// even without a valid answers part
	e.post(l.token, text("website", "x")).status(201, "")
	e.nothingKept()
	e.post(l.token, answers("Human", "hi"), text("website", "")).status(201, "")

	e.nh.SetVerifier(verifierFunc(func(context.Context, *http.Request) error { return errors.New("no") }))
	e.post(l.token, answers("Human", "hi"), photoPart(jpegOf(t, 1))).status(403, "verification_failed")
	if e.noteCount() != 1 || e.count(`SELECT COUNT(*) FROM media`) != 0 || e.st.count() != 0 {
		t.Fatal("a rejected verification stored or processed something")
	}
	// the honeypot is checked before the verifier, so a bot does not learn it was caught
	e.post(l.token, answers("Bot", "x"), text("website", "y")).status(201, "")
}

type verifierFunc func(context.Context, *http.Request) error

func (f verifierFunc) Verify(ctx context.Context, r *http.Request) error { return f(ctx, r) }

// A submission with photos is refused with 503 busy when too many are already holding photo bytes.
func TestSubmitBusy(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	for range cap(e.nh.inflight) {
		e.nh.inflight <- struct{}{}
	}
	rec := e.post(l.token, answers("A", "m"), photoPart(jpegOf(t, 1))).status(503, "busy")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.post(l.token, answers("A", "m")).status(201, "") // text-only does not need a slot
	for range cap(e.nh.inflight) {
		<-e.nh.inflight
	}
	e.post(l.token, answers("A", "m"), photoPart(jpegOf(t, 1))).status(201, "")
	if len(e.nh.inflight) != 0 {
		t.Fatal("slot not released")
	}
}

// AC9: no cookie read or set, no CORS, no IP or user agent stored, nothing sensitive in the logs.
func TestSubmitPrivacy(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	bob := e.register("bob@example.com", "Bob", true)
	body, ct := multipartOf(answers("Ánh", "UNIQUE-MESSAGE-TEXT"), part{"photos", "secret-holiday-name.jpg", "image/jpeg", jpegOf(t, 3)})
	r := httptest.NewRequest("POST", "/v1/public/collect/"+l.token+"/notes", body)
	r.Header.Set("Content-Type", ct)
	r.Header.Set("Origin", "https://evil.example")
	r.Header.Set("User-Agent", "UNIQUE-UA-STRING")
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: bob.cookie})
	r.RemoteAddr = "198.51.100.77:5000"
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	resp{rec, t}.status(201, "")
	if rec.Header().Get("Set-Cookie") != "" {
		t.Fatal("cookie set")
	}
	for k := range rec.Header() {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			t.Fatalf("CORS header %s", k)
		}
	}
	rows, err := e.db.Query(`SELECT column_name FROM information_schema.columns WHERE table_schema = DATABASE() AND table_name = 'notes' ORDER BY ordinal_position`)
	if err != nil {
		t.Fatal(err)
	}
	var cols []string
	for rows.Next() {
		var c string
		_ = rows.Scan(&c)
		cols = append(cols, c)
	}
	_ = rows.Close()
	if got := strings.Join(cols, ","); got != "id,public_id,collection_id,yearbook_id,answers,status,sort_order,created_at" {
		t.Fatalf("notes columns: %s", got)
	}
	// the stored text does not hold the address or agent either
	if e.count(`SELECT COUNT(*) FROM notes WHERE answers LIKE '%198.51%' OR answers LIKE '%UNIQUE-UA%'`) != 0 {
		t.Fatal("client data stored")
	}
	// provoke an error log too
	e.st.failAt = e.st.puts + 1
	e.post(l.token, answers("Ánh", "UNIQUE-MESSAGE-TEXT"), part{"photos", "secret-holiday-name.jpg", "image/jpeg", jpegOf(t, 4)}).status(502, "storage_error")
	logs := e.logs.String()
	for _, s := range []string{l.token, "UNIQUE-MESSAGE-TEXT", "secret-holiday-name", "UNIQUE-UA", "198.51.100.77", "Ánh"} {
		if strings.Contains(logs, s) {
			t.Fatalf("logs contain %q", s)
		}
	}
	if !strings.Contains(logs, "POST /v1/public/collect/{token}/notes") {
		t.Fatal("access log should record the route pattern")
	}
}

// Deleting a yearbook removes its notes, links to photos, photo rows and objects.
func TestYearbookDeleteRemovesNotes(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	e.post(l.token, answers("A", "m"), photoPart(jpegOf(t, 1))).status(201, "")
	e.do(l.owner, "DELETE", "/v1/yearbooks/"+l.book, "").status(204, "")
	for _, q := range []string{`SELECT COUNT(*) FROM notes`, `SELECT COUNT(*) FROM note_photos`, `SELECT COUNT(*) FROM media`} {
		if n := e.count(q); n != 0 {
			t.Fatalf("%s = %d", q, n)
		}
	}
	if e.st.count() != 0 {
		t.Fatalf("%d objects left", e.st.count())
	}
}
