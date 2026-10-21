//go:build integration

package media

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

// Audit M3 (T-100). The public_id columns compare case-insensitively, so a lower-case spelling of a real id finds the real row.
// Before the fix: upload with a lower-case yearbook_id (objects under yearbooks/<lower>/), delete the book with the canonical
// id (the purge looks under yearbooks/<CANONICAL>/), the rows and the quota are gone and the photos stay in the bucket.

// AC4: the attack, replayed through the API, fails at its first step; nothing is stored.
func TestM3AttackIsRefusedAtTheEdge(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	lower := strings.ToLower(book)

	e.upload(u, lower, "a.jpg", "image/jpeg", photo(t)).status(400, "ERROR_PARAM")
	if n, rows := e.st.count("yearbooks/"), e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 || rows != 0 {
		t.Fatalf("a refused upload left %d objects and %d rows", n, rows)
	}

	// Every other place a media or yearbook id comes in.
	a := e.upload(u, book, "a.jpg", "image/jpeg", photo(t)).status(200, "").mediaID()
	objects := e.st.count("yearbooks/")
	e.json(u, "GET", "/api/media/get-list?yearbook_id="+lower, "").status(400, "ERROR_PARAM")
	e.json(u, "GET", "/api/media/get-content?id="+strings.ToLower(a), "").status(400, "ERROR_PARAM")
	e.deleteMedia(u, strings.ToLower(a)).status(400, "ERROR_PARAM")
	e.getBook(u, lower).status(400, "ERROR_PARAM")
	e.updateBook(u, lower, `{"title":"x"}`).status(400, "ERROR_PARAM")
	e.updateProfile(u, lower, `{"full_name":"x"}`).status(400, "ERROR_PARAM")
	e.deleteBook(u, lower).status(400, "ERROR_PARAM")
	e.json(u, "POST", "/api/yearbook/revoke-photo-links", fmt.Sprintf(`{"id":%q}`, lower)).status(400, "ERROR_PARAM")
	e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, strings.ToLower(a))).status(400, "ERROR_INVALID_MEDIA")
	e.updateProfile(u, book, fmt.Sprintf(`{"full_name":"x","photo_media_id":%q}`, strings.ToLower(a))).status(400, "ERROR_INVALID_MEDIA")
	if got := e.st.count("yearbooks/"); got != objects || e.count(`SELECT COUNT(*) FROM media_tab`) != 1 {
		t.Fatalf("refused requests changed the bucket (%d -> %d objects) or the rows", objects, got)
	}
	// The canonical spelling keeps working.
	e.getBook(u, book).status(200, "")
	e.deleteBook(u, book).status(200, "")
	if n := e.st.count("yearbooks/"); n != 0 {
		t.Fatalf("%d objects left after the delete", n)
	}
}

// AC3: a caller that bypasses the handlers can still only write and purge under the canonical prefix.
func TestServicesUseTheStoredIDForObjectKeys(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	lower := strings.ToLower(book)
	var owner uint64
	if err := e.db.QueryRow(`SELECT owner_id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	m, err := e.svc.Upload(ctx, owner, lower, photo(t)) // the lower-case spelling finds the row; the keys must not follow it
	if err != nil {
		t.Fatal(err)
	}
	for _, k := range m.keys() {
		if !strings.HasPrefix(k, "yearbooks/"+book+"/") {
			t.Fatalf("object key %q is not under the canonical prefix", k)
		}
	}
	if n := e.st.count("yearbooks/" + lower + "/"); n != 0 {
		t.Fatalf("%d objects under the lower-case prefix", n)
	}
	if n := e.st.count("yearbooks/" + book + "/"); n != 3 {
		t.Fatalf("%d objects under the canonical prefix, want 3", n)
	}
	for _, k := range m.keys() {
		if _, err := e.st.Get(ctx, k); err != nil {
			t.Fatalf("%s: %v", k, err)
		}
	}

	if err := e.bookSvc.Delete(ctx, owner, lower); err != nil { // the purge uses the stored id, not this spelling
		t.Fatal(err)
	}
	for _, k := range m.keys() {
		if _, err := e.st.Get(ctx, k); err == nil {
			t.Fatalf("%s survived the yearbook delete", k)
		}
	}
	if n := e.st.count("yearbooks/"); n != 0 {
		t.Fatalf("%d objects left in the bucket", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 {
		t.Fatalf("%d media rows left", n)
	}
}
