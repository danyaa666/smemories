//go:build integration

package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"net/textproto"
	"strings"
	"testing"
)

// uploadDesign sends a design image: the purpose field comes after the file (the order a form builder is least likely to guess).
func (e *env) uploadDesign(u user, book string, data []byte) resp {
	return e.uploadWith(u, book, "design", data, false)
}

func (e *env) uploadWith(u user, book, purpose string, data []byte, purposeFirst bool) resp {
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	_ = mw.WriteField("yearbook_id", book)
	if purposeFirst {
		_ = mw.WriteField("purpose", purpose)
	}
	h := textproto.MIMEHeader{}
	h.Set("Content-Disposition", `form-data; name="file"; filename="bg.jpg"`)
	h.Set("Content-Type", "image/jpeg")
	w, _ := mw.CreatePart(h)
	_, _ = w.Write(data)
	if !purposeFirst {
		_ = mw.WriteField("purpose", purpose)
	}
	_ = mw.Close()
	r := httptest.NewRequest("POST", "/api/media/create", &b)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return e.send(u, r)
}

// fillPurpose inserts n owner rows of a purpose straight into the table (a real upload takes far too long to reach a quota).
func (e *env) fillPurpose(book, purpose string, n, bytes, thumb, print int) {
	e.t.Helper()
	base := e.count(`SELECT COUNT(*) FROM media_tab`)
	for i := range n {
		e.exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, purpose, object_key, thumb_key, content_type, bytes, thumb_bytes, print_bytes, width, height, sha256, created_at, updated_at)
			SELECT ?, id, 'owner', ?, 'k', 't', 'image/jpeg', ?, ?, ?, 1, 1, '', 1, 1 FROM yearbook_tab WHERE public_id = ?`,
			fmt.Sprintf("D%025d", base+i), purpose, bytes, thumb, print, book)
	}
}

func (e *env) ids(p listPage) []string {
	out := []string{}
	for _, it := range p.Data.Items {
		out = append(out, it.ID)
	}
	return out
}

// AC1, AC2 (T-152): a design image uploads, lists only with purpose=design, serves in every variant, and never shows in the photo
// list; the purpose of anything else is ERROR_PARAM; a design id is no cover or profile photo and a photo id no design image; no link
// token opens a design image.
func TestDesignImageIsolation(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book := e.newBook(u)
	ph := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).mediaID()
	if got := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).media(); got.Purpose != "photo" {
		t.Fatalf("a plain upload has purpose %q", got.Purpose)
	}
	d1 := e.uploadDesign(u, book, photo(t)).media()
	if d1.Purpose != "design" || d1.YearbookID != book || d1.Width != 64 {
		t.Fatalf("design upload answered %+v", d1)
	}
	d2 := e.uploadWith(u, book, "design", photo(t), true).media() // the purpose field before the file
	if d2.Purpose != "design" {
		t.Fatalf("purpose first: %+v", d2)
	}

	for _, q := range []string{"", "&uploader=all", "&uploader=owner"} {
		for _, id := range e.ids(e.list(u, book, q)) {
			if id == d1.ID || id == d2.ID {
				t.Errorf("list %q shows a design image", q)
			}
		}
	}
	if got := e.ids(e.list(u, book, "&purpose=design")); len(got) != 2 || got[0] != d2.ID || got[1] != d1.ID {
		t.Fatalf("design list: %v", got)
	}
	if p := e.list(u, book, "&purpose=design&uploader=contributor"); len(p.Data.Items) != 0 {
		t.Fatalf("design images by contributors: %v", e.ids(p))
	}
	for _, bad := range []string{"photo", "Design", "x", "design,photo"} {
		e.json(u, "GET", "/api/media/get-list?yearbook_id="+book+"&purpose="+bad, "").status(400, "ERROR_PARAM")
		e.uploadWith(u, book, bad, photo(t), false).status(400, "ERROR_PARAM")
	}
	if e.count(`SELECT COUNT(*) FROM media_tab WHERE purpose = 'design'`) != 2 {
		t.Fatal("a refused upload stored something")
	}

	// the three variants, for the owner only
	for _, v := range []string{"display", "thumb", "print"} {
		e.json(u, "GET", "/api/media/get-content?id="+d1.ID+"&variant="+v, "").status(200, "")
		e.json(other, "GET", "/api/media/get-content?id="+d1.ID+"&variant="+v, "").status(404, "ERROR_NOT_FOUND")
	}
	e.json(user{}, "GET", "/api/media/get-content?id="+d1.ID, "").status(401, "")
	e.json(other, "GET", "/api/media/get-list?yearbook_id="+book+"&purpose=design", "").status(404, "ERROR_NOT_FOUND")

	// a design id is no cover and no profile photo; the photo id is (control)
	e.updateProfile(u, book, fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, d1.ID)).status(400, "ERROR_INVALID_MEDIA")
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, d1.ID)).status(400, "ERROR_INVALID_MEDIA")
	e.updateProfile(u, book, fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, ph)).status(200, "")
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, ph)).status(200, "")

	// a photo id is no design image (the check a layout makes before it references an image)
	var uid uint64
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = 'a@example.com'`).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	got, err := e.svc.DesignImages(context.Background(), uid, book, []string{ph, d1.ID, strings.ToLower(d2.ID), "01J0000000000000000000000Z"})
	if err != nil || len(got) != 1 || !got[d1.ID] {
		t.Fatalf("DesignImages = %v, %v; want only %s", got, err, d1.ID)
	}
	var otherID uint64
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = 'b@example.com'`).Scan(&otherID); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.DesignImages(context.Background(), otherID, book, []string{d1.ID}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another user's DesignImages: %v", err)
	}

	// no link opens a design image, even a correctly signed one (the cover and the profile photo above are the control)
	e.public(e.token(d1.ID, book), "").status(404, "ERROR_NOT_FOUND")
	e.public(e.token(ph, book), "").status(200, "")
	// a design image used as nothing is also not "shareable" by a stale reference
	e.exec(`UPDATE yearbook_tab SET cover_media_id = (SELECT id FROM media_tab WHERE public_id = ?) WHERE public_id = ?`, d1.ID, book)
	e.public(e.token(d1.ID, book), "").status(404, "ERROR_NOT_FOUND")
}

// A page token of the design list is not a token of the photo list (and the other way round).
func TestDesignListCursorHasItsOwnScope(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	e.fillPurpose(book, "design", 3, 10, 1, 1)
	e.seed(book, "owner", 3)
	dp := e.list(u, book, "&purpose=design&limit=1")
	pp := e.list(u, book, "&limit=1")
	if dp.Data.NextID == "" || pp.Data.NextID == "" {
		t.Fatal("no next page")
	}
	e.json(u, "GET", "/api/media/get-list?yearbook_id="+book+"&limit=1&next_id="+dp.Data.NextID, "").status(400, "ERROR_PARAM")
	e.json(u, "GET", "/api/media/get-list?yearbook_id="+book+"&purpose=design&limit=1&next_id="+pp.Data.NextID, "").status(400, "ERROR_PARAM")
	if p := e.list(u, book, "&purpose=design&limit=5&next_id="+dp.Data.NextID); len(p.Data.Items) != 2 {
		t.Fatalf("second page of design images: %d", len(p.Data.Items))
	}
}

// AC1: 30 design images per yearbook (the 31st is 409), independent of the 200-photo cap; the byte quota is shared both ways.
func TestDesignQuotas(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book, book2 := e.newBook(u), e.newBook(u)

	e.fillPurpose(book, "design", maxDesignPerYearbook-1, 10, 1, 1)
	e.uploadDesign(u, book, photo(t)).status(200, "") // the 30th
	e.uploadDesign(u, book, photo(t)).status(409, "ERROR_QUOTA_EXCEEDED")
	e.uploadDesign(u, book2, photo(t)).status(200, "")                 // the cap is per yearbook
	e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "") // design images are not photos: they take no photo room

	e.fillPurpose(book2, "photo", maxPerYearbook, 10, 1, 1) // the photo room of book2 is full ...
	e.upload(u, book2, "a.jpg", "image/jpeg", photo(t)).status(409, "ERROR_QUOTA_EXCEEDED")
	e.uploadDesign(u, book2, photo(t)).status(200, "") // ... and does not stop design images

	// bytes: one pool for photos and design images across the user's books
	const limit = 500 << 20
	e.fillPurpose(e.newBook(u), "design", 1, 1, limit, 0)
	e.upload(u, e.newBook(u), "a.jpg", "image/jpeg", photo(t)).status(409, "ERROR_QUOTA_EXCEEDED")
	e.uploadDesign(u, e.newBook(u), photo(t)).status(409, "ERROR_QUOTA_EXCEEDED")
	e.exec(`UPDATE media_tab SET thumb_bytes = 0 WHERE thumb_bytes > 1000000`)
	e.fillPurpose(e.newBook(u), "photo", 1, 1, limit, 0)
	e.uploadDesign(u, e.newBook(u), photo(t)).status(409, "ERROR_QUOTA_EXCEEDED")

	// the other user is not affected
	o := e.register("b@example.com")
	e.uploadDesign(o, e.newBook(o), photo(t)).status(200, "")

	m := scrape(t, e)
	for _, want := range []string{`smem_media_uploads_total{purpose="design",result="quota"}`, `smem_media_uploads_total{purpose="design",result="ok"}`, `smem_media_uploads_total{purpose="photo",result="quota"}`} {
		if !strings.Contains(m, want) {
			t.Errorf("metrics lack %s:\n%s", want, grep(m, "media_uploads"))
		}
	}
	if strings.Contains(m, "bg.jpg") || strings.Contains(m, book) {
		t.Error("metrics carry a file name or an id")
	}
}

// AC4: the owner deletes a design image (idempotent, the objects go with it); deleting the yearbook deletes its design images,
// rows and objects.
func TestDesignImageDelete(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book := e.newBook(u)
	d1, d2 := e.uploadDesign(u, book, photo(t)).mediaID(), e.uploadDesign(u, book, photo(t)).mediaID()
	var key string
	if err := e.db.QueryRow(`SELECT object_key FROM media_tab WHERE public_id = ?`, d1).Scan(&key); err != nil {
		t.Fatal(err)
	}

	e.deleteMedia(other, d1).status(200, "") // someone else's id: silent, nothing happens
	if e.count(`SELECT COUNT(*) FROM media_tab WHERE purpose = 'design'`) != 2 {
		t.Fatal("another user deleted a design image")
	}
	e.deleteMedia(u, d1).status(200, "")
	e.deleteMedia(u, d1).status(200, "")
	e.json(u, "GET", "/api/media/get-content?id="+d1, "").status(404, "ERROR_NOT_FOUND")
	if _, err := e.st.Get(context.Background(), key); err == nil {
		t.Error("the object of a deleted design image is still stored")
	}
	if got := e.ids(e.list(u, book, "&purpose=design")); len(got) != 1 || got[0] != d2 {
		t.Fatalf("after delete: %v", got)
	}
	e.deleteBook(u, book).status(200, "")
	if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 {
		t.Fatalf("%d media rows left after the yearbook was deleted", n)
	}
	e.json(u, "GET", "/api/media/get-content?id="+d2, "").status(404, "ERROR_NOT_FOUND")
}
