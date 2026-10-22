//go:build integration

package notes

import (
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/ulid"
)

// QA T-105: parallel creates at the 50-link boundary never pass 50 (revoked included) and never pass 5 active.
func TestQALinkCapRace(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	e.fillLinks(book, maxLinks-2) // 48 revoked: room for exactly 2
	var ok, full, other atomic.Int32
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := e.create(alice, book, "{}")
			switch {
			case r.Code == 200:
				ok.Add(1)
			case r.Code == 409 && r.code() == "ERROR_LIMIT_REACHED":
				full.Add(1)
			default:
				other.Add(1)
				t.Errorf("unexpected %d %s", r.Code, r.Body.String())
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 2 || full.Load() != 14 {
		t.Fatalf("ok=%d full=%d other=%d, want 2/14/0", ok.Load(), full.Load(), other.Load())
	}
	if n := e.count(`SELECT COUNT(*) FROM note_collection_tab c JOIN yearbook_tab y ON y.id = c.yearbook_id WHERE y.public_id = ?`, book); n != maxLinks {
		t.Fatalf("%d links, want %d", n, maxLinks)
	}
	// the 5-active cap still holds with few revoked
	b2 := e.book(alice, "Two")
	var wg2 sync.WaitGroup
	var ok2 atomic.Int32
	for range 12 {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if e.create(alice, b2, "{}").Code == 200 {
				ok2.Add(1)
			}
		}()
	}
	wg2.Wait()
	if ok2.Load() != maxActive {
		t.Fatalf("%d active links created, want %d", ok2.Load(), maxActive)
	}
	// revoked links free active slots but not the 50 total
	e.create(alice, b2, "{}").status(409, "ERROR_LIMIT_REACHED")
}

// QA T-105: keyset paging with several links sharing one created_at (ties broken by id), a limit of 1, a limit of 100,
// a flipped character in the cursor, and a cursor made for another list.
func TestQALinkPagingTiesAndTamper(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Book")
	ms := e.now().UnixMilli()
	for range 9 { // all with the same created_at
		var h [32]byte
		copy(h[:], ulid.New(e.now()))
		if _, err := e.db.Exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at, revoked_at)
			SELECT ?, id, ?, '', ?, ?, ? FROM yearbook_tab WHERE public_id = ?`, ulid.New(e.now()), h[:], ms, ms, ms, book); err != nil {
			t.Fatal(err)
		}
		e.advance(time.Nanosecond)
	}
	get := func(q string) resp {
		return e.do(alice, "GET", "/api/note-collection/get-list?yearbook_id="+url.QueryEscape(book)+q, "")
	}
	var all []string
	next := ""
	for pages := 0; pages < 20; pages++ {
		q := "&limit=1"
		if next != "" {
			q += "&next_id=" + url.QueryEscape(next)
		}
		d := get(q).status(200, "").data()
		items := d["items"].([]any)
		if len(items) != 1 && d["next_id"].(string) != "" {
			t.Fatalf("page of %d with a next", len(items))
		}
		for _, it := range items {
			all = append(all, it.(map[string]any)["id"].(string))
		}
		next = d["next_id"].(string)
		if next == "" {
			break
		}
	}
	seen := map[string]bool{}
	for _, id := range all {
		if seen[id] {
			t.Fatalf("duplicate %s in %v", id, all)
		}
		seen[id] = true
	}
	if len(all) != 9 {
		t.Fatalf("limit=1 paging returned %d of 9 tied links", len(all))
	}
	// tamper: flip one character of a real cursor
	tok := get("&limit=2").status(200, "").data()["next_id"].(string)
	for _, pos := range []int{0, len(tok) / 2, len(tok) - 1} {
		b := []byte(tok)
		if b[pos] == 'A' {
			b[pos] = 'B'
		} else {
			b[pos] = 'A'
		}
		get("&next_id="+url.QueryEscape(string(b))).status(400, "ERROR_PARAM")
	}
	get("&next_id="+url.QueryEscape(tok+"x")).status(400, "ERROR_PARAM")
	get("&next_id="+url.QueryEscape(strings.ToUpper(tok))).status(400, "ERROR_PARAM")
	// a cursor of the note inbox of the same book is not valid here
	if r := e.noteList(alice, book, "&limit=1"); r.Code == 200 {
		if n, _ := r.data()["next_id"].(string); n != "" {
			get("&next_id="+url.QueryEscape(n)).status(400, "ERROR_PARAM")
		}
	}
	// the other owner cannot use alice's cursor, and cannot list alice's book
	bob := e.register("bob@example.com", "Bob", true)
	e.do(bob, "GET", "/api/note-collection/get-list?yearbook_id="+url.QueryEscape(book)+"&next_id="+url.QueryEscape(tok), "").status(404, "")
	// limit bounds
	get("&limit=100").status(200, "")
	get("&limit=-1").status(400, "ERROR_PARAM")
	get("&limit=1.5").status(400, "ERROR_PARAM")
}
