//go:build integration

package media

import (
	"bytes"
	"context"
	"errors"
	"image"
	"io"
	"log/slog"
	"strings"
	"testing"
)

func bigPhoto(t *testing.T) []byte { return jpegWith(t, quad(2600, 1300, false)) }

func (e *env) width(u user, id, size string) (int, string) {
	r := e.json(u, "GET", "/v1/media/"+id+"/content?size="+size, "").status(200, "")
	c, _, err := image.DecodeConfig(bytes.NewReader(r.Body.Bytes()))
	if err != nil {
		e.t.Fatal(err)
	}
	return c.Width, r.Header().Get("ETag")
}

func TestUploadStoresPrintVersion(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book := e.newBook(u)
	id := e.upload(u, book, "a.jpg", "image/jpeg", bigPhoto(t)).status(201, "").mediaID()

	var pk string
	_ = e.db.QueryRow(`SELECT print_key FROM media WHERE public_id = ?`, id).Scan(&pk)
	if pk != "yearbooks/"+book+"/"+id+"-print.jpg" {
		t.Fatalf("print_key %q", pk)
	}
	if _, err := e.st.Get(context.Background(), pk); err != nil {
		t.Fatalf("print object missing: %v", err)
	}
	pw, petag := e.width(u, id, "print")
	dw, detag := e.width(u, id, "display")
	if pw != 1800 || dw != 2600 || petag == detag {
		t.Fatalf("print %d px %s, display %d px %s", pw, petag, dw, detag)
	}
	got := e.json(u, "GET", "/v1/media/"+id+"/content?size=print", "").status(200, "")
	if h := got.Header(); h.Get("Content-Type") != "image/jpeg" || h.Get("X-Content-Type-Options") != "nosniff" || h.Get("Cache-Control") != "private, max-age=3600" {
		t.Fatalf("headers: %v", h)
	}
	// Same authorisation as the other sizes.
	e.json(other, "GET", "/v1/media/"+id+"/content?size=print", "").status(404, "not_found")
	e.json(user{}, "GET", "/v1/media/"+id+"/content?size=print", "").status(401, "")
	e.json(u, "GET", "/v1/media/"+id+"/content?size=Print", "").status(400, "invalid_size")

	// Deleting the photo removes the print object; so does deleting the yearbook.
	e.json(u, "DELETE", "/v1/media/"+id, "").status(204, "")
	if _, err := e.st.Get(context.Background(), pk); err == nil {
		t.Fatal("print object survived the photo delete")
	}
	e.upload(u, book, "b.jpg", "image/jpeg", bigPhoto(t)).status(201, "")
	e.json(u, "DELETE", "/v1/yearbooks/"+book, "").status(204, "")
	if n := e.st.count(yearbookPrefix(book)); n != 0 {
		t.Fatalf("%d objects left after the yearbook delete", n)
	}
}

func TestContributorUploadStoresPrintVersion(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	var yid uint64
	_ = e.db.QueryRow(`SELECT id FROM yearbooks WHERE public_id = ?`, book).Scan(&yid)
	m, err := e.svc.UploadContributor(context.Background(), yid, bigPhoto(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.st.Get(context.Background(), m.PrintKey); err != nil || !strings.HasSuffix(m.PrintKey, "-print.jpg") {
		t.Fatalf("print object %q: %v", m.PrintKey, err)
	}
	if err := e.svc.Discard(m); err != nil || e.st.count(yearbookPrefix(book)) != 0 {
		t.Fatalf("discard: %v, %d objects left", err, e.st.count(yearbookPrefix(book)))
	}
}

func TestBackfillPrint(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	a := e.upload(u, book, "a.jpg", "image/jpeg", bigPhoto(t)).status(201, "").mediaID()
	b := e.upload(u, book, "b.png", "image/png", encPNG(t, quad(2000, 1000, true))).status(201, "").mediaID()
	c := e.upload(u, book, "c.jpg", "image/jpeg", photo(t)).status(201, "").mediaID() // 64 px: print is a copy of the display
	broken := e.upload(u, book, "d.jpg", "image/jpeg", bigPhoto(t)).status(201, "").mediaID()

	// Make them look like photos from before T-057: no print object, no print_key.
	if _, err := e.db.Exec(`UPDATE media SET print_key = NULL`); err != nil {
		t.Fatal(err)
	}
	_ = e.st.Delete(context.Background(), yearbookPrefix(book)+a+"-print.jpg", yearbookPrefix(book)+b+"-print.png",
		yearbookPrefix(book)+c+"-print.jpg", yearbookPrefix(book)+broken+"-print.jpg")
	_ = e.st.Delete(context.Background(), yearbookPrefix(book)+broken+".jpg") // its display object is gone
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	// Until then print falls back to display, with the display ETag.
	w, etag := e.width(u, a, "print")
	if _, detag := e.width(u, a, "display"); w != 2600 || etag != detag {
		t.Fatalf("fallback served %d px (%s, display %s)", w, etag, detag)
	}

	// Dry run writes nothing.
	st, err := e.svc.Backfill(context.Background(), 2, 1, true, log)
	if err != nil || st != (BackfillStats{Created: 3, Failed: 1}) || e.count(`SELECT COUNT(*) FROM media WHERE print_key IS NOT NULL`) != 0 || e.st.count(yearbookPrefix(book)) != 7 { // 4 photos x 3 objects, minus 4 prints and 1 display
		t.Fatalf("dry run %+v err %v", st, err)
	}

	// The real run (batches of 2) fills three photos and reports the broken one; running again changes nothing.
	st, err = e.svc.Backfill(context.Background(), 2, 1, false, log)
	if err != nil || st != (BackfillStats{Created: 3, Failed: 1}) {
		t.Fatalf("run 1: %+v %v", st, err)
	}
	objects := e.st.count(yearbookPrefix(book))
	st, err = e.svc.Backfill(context.Background(), 2, 1, false, log)
	if err != nil || st != (BackfillStats{Failed: 1}) || e.st.count(yearbookPrefix(book)) != objects {
		t.Fatalf("run 2: %+v %v", st, err)
	}
	if w, _ := e.width(u, a, "print"); w != 1800 {
		t.Fatalf("print of a: %d px", w)
	}
	if w, _ := e.width(u, b, "print"); w != 1800 {
		t.Fatalf("print of b: %d px", w)
	}
	var key string
	_ = e.db.QueryRow(`SELECT print_key FROM media WHERE public_id = ?`, b).Scan(&key)
	if !strings.HasSuffix(key, "-print.png") {
		t.Fatalf("print key of the PNG: %q", key)
	}
	r := e.json(u, "GET", "/v1/media/"+b+"/content?size=print", "").status(200, "")
	if r.Header().Get("Content-Type") != "image/png" {
		t.Fatalf("content type %s", r.Header().Get("Content-Type"))
	}
	if w, _ := e.width(u, c, "print"); w != 64 {
		t.Fatalf("print of c: %d px", w)
	}
	// The backfilled print object goes with the photo.
	e.json(u, "DELETE", "/v1/media/"+a, "").status(204, "")
	if _, err := e.st.Get(context.Background(), yearbookPrefix(book)+a+"-print.jpg"); err == nil {
		t.Fatal("print object survived the delete")
	}
}

// backfillFixture is a book with n legacy-style photos (big, no print object) and the media ids.
func backfillFixture(t *testing.T, e *env, n int) (book string, ids []string) {
	u := e.register("a@example.com")
	book = e.newBook(u)
	for i := range n {
		ids = append(ids, e.upload(u, book, "a.jpg", "image/jpeg", bigPhoto(t)).status(201, "").mediaID())
		_ = e.st.Delete(context.Background(), yearbookPrefix(book)+ids[i]+"-print.jpg")
	}
	if _, err := e.db.Exec(`UPDATE media SET print_key = NULL`); err != nil {
		t.Fatal(err)
	}
	return book, ids
}

// T-075: an object-store outage stops the run after N failures in a row, in a dry run too; a photo that fails on its own does not count.
func TestBackfillStopsAfterConsecutiveStorageFailures(t *testing.T) {
	e := newEnv(t)
	_, ids := backfillFixture(t, e, 5)
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	e.st.mu.Lock()
	e.st.failGet = true
	e.st.mu.Unlock()
	for _, dry := range []bool{true, false} {
		st, err := e.svc.Backfill(context.Background(), 2, 3, dry, log)
		if !errors.Is(err, ErrStorageDown) || st != (BackfillStats{Failed: 3}) {
			t.Fatalf("dry=%v: %+v %v", dry, st, err)
		}
	}
	// The store is back: the run finishes every photo.
	e.st.mu.Lock()
	e.st.failGet = false
	e.st.mu.Unlock()
	if st, err := e.svc.Backfill(context.Background(), 2, 3, false, log); err != nil || st != (BackfillStats{Created: len(ids)}) {
		t.Fatalf("after recovery: %+v %v", st, err)
	}
}

func TestBackfillPutFailuresCountAndMissingDisplayDoesNot(t *testing.T) {
	e := newEnv(t)
	book, ids := backfillFixture(t, e, 4)
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	_ = e.st.Delete(context.Background(), yearbookPrefix(book)+ids[0]+".jpg") // not a storage outage: the object is simply gone
	e.st.set(true, false)
	st, err := e.svc.Backfill(context.Background(), 10, 2, false, log)
	if !errors.Is(err, ErrStorageDown) || st != (BackfillStats{Failed: 3}) { // 1 missing, then 2 failed writes in a row
		t.Fatalf("%+v %v", st, err)
	}
}

// T-075: a photo deleted between the print write and the row update leaves no print object; a photo filled by a parallel run keeps it.
func TestBackfillDeleteRace(t *testing.T) {
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))
	for _, tc := range []struct {
		name, sql string
		kept      bool
	}{
		{"deleted", `DELETE FROM media`, false},
		{"filled by a parallel run", `UPDATE media SET print_key = CONCAT(SUBSTRING(object_key, 1, CHAR_LENGTH(object_key) - 4), '-print.jpg')`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			book, ids := backfillFixture(t, e, 1)
			e.st.afterPut = func(key string) {
				if strings.HasSuffix(key, "-print.jpg") {
					if _, err := e.db.Exec(tc.sql); err != nil {
						t.Error(err)
					}
				}
			}
			st, err := e.svc.Backfill(context.Background(), 10, 1, false, log)
			if err != nil || st != (BackfillStats{Skipped: 1}) {
				t.Fatalf("%+v %v", st, err)
			}
			_, err = e.st.Get(context.Background(), yearbookPrefix(book)+ids[0]+"-print.jpg")
			if (err == nil) != tc.kept {
				t.Fatalf("print object present=%v, want %v", err == nil, tc.kept)
			}
		})
	}
}
