//go:build integration

package notes

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/ulid"
)

// fillLinks inserts n revoked links straight into the table (a create is rate limited and slow), oldest first.
func (e *env) fillLinks(book string, n int) {
	e.t.Helper()
	for range n {
		e.advance(time.Second)
		ms := e.now().UnixMilli()
		var h [32]byte
		copy(h[:], ulid.New(e.now()))
		if _, err := e.db.Exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at, revoked_at)
			SELECT ?, id, ?, '', ?, ?, ? FROM yearbook_tab WHERE public_id = ?`, ulid.New(e.now()), h[:], ms, ms, ms, book); err != nil {
			e.t.Fatal(err)
		}
	}
}

// T-105 AC1: at most 50 links per book, revoked ones included; the 51st is ERROR_LIMIT_REACHED, another book is unaffected.
func TestLinkCap(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book, other := e.book(alice, "Book"), e.book(alice, "Other")
	e.fillLinks(book, maxLinks-1)
	e.create(alice, book, "{}").status(200, "") // the 50th
	rec := e.create(alice, book, "{}").status(409, "ERROR_LIMIT_REACHED")
	if e.count(`SELECT COUNT(*) FROM note_collection_tab c JOIN yearbook_tab y ON y.id = c.yearbook_id WHERE y.public_id = ?`, book) != maxLinks {
		t.Fatalf("a refused create left a row: %s", rec.Body.String())
	}
	e.create(alice, other, "{}").status(200, "")
}

// T-105 AC1: pages are newest first, cover every link once, and a link created between two requests does not shift the next page.
func TestLinkListPaging(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	e.fillLinks(book, 7)
	page := func(query string) ([]string, string) {
		t.Helper()
		d := e.do(alice, "GET", "/api/note-collection/get-list?yearbook_id="+url.QueryEscape(book)+query, "").status(200, "").data()
		var ids []string
		for _, it := range d["items"].([]any) {
			ids = append(ids, it.(map[string]any)["id"].(string))
		}
		return ids, d["next_id"].(string)
	}
	first, next := page("&limit=3")
	if len(first) != 3 || next == "" {
		t.Fatalf("first page %v next %q", first, next)
	}
	e.advance(time.Minute)
	e.newLink(alice, book, "{}") // arrives after the first page was read
	second, next := page("&limit=3&next_id=" + url.QueryEscape(next))
	third, next := page("&limit=3&next_id=" + url.QueryEscape(next))
	if len(second) != 3 || len(third) != 1 || next != "" {
		t.Fatalf("pages %v %v next %q", second, third, next)
	}
	seen := map[string]bool{}
	for _, id := range append(append(first, second...), third...) {
		if seen[id] {
			t.Fatalf("link %s listed twice", id)
		}
		seen[id] = true
	}
	if len(seen) != 7 {
		t.Fatalf("%d links over the pages, want the 7 that existed", len(seen))
	}
	if all, next := page(""); len(all) != 8 || next != "" { // default limit 50
		t.Fatalf("default page: %d items, next %q", len(all), next)
	}
	for _, bad := range []string{"&limit=0", "&limit=101", "&limit=x", "&next_id=junk"} {
		e.do(alice, "GET", "/api/note-collection/get-list?yearbook_id="+url.QueryEscape(book)+bad, "").status(400, "ERROR_PARAM")
	}
	// a token of another book (or list) is refused
	other := e.book(alice, "Other")
	e.fillLinks(other, 2)
	_, tok := page("&limit=1")
	e.do(alice, "GET", fmt.Sprintf("/api/note-collection/get-list?yearbook_id=%s&next_id=%s", url.QueryEscape(other), url.QueryEscape(tok)), "").status(400, "ERROR_PARAM")
}
