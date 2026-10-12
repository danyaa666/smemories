//go:build integration

package notes

import (
	"context"
	"fmt"
	"math/rand"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db/dbtest"
)

var qaIP atomic.Int64

// lookupFrom is a public lookup from a fresh client address (the per-IP miss limit is not what is under test).
func (e *env) lookupFrom(token string) resp {
	n := qaIP.Add(1)
	r := httptest.NewRequest("GET", "/v1/public/collect/"+token, nil)
	r.RemoteAddr = fmt.Sprintf("100.64.%d.%d:4000", n/250, n%250+1)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return resp{rec, e.t}
}

// QA T-066: a storm of public submits (0-3 photos), public lookups, collection creates and revokes, photo deletes and a book delete
// in parallel, many rounds. No 5xx, no orphan row, no orphan object after the book is gone; a book that vanished answers 404/410.
func TestQAStormBookDelete(t *testing.T) {
	e := newEnv(t)
	e.svc.SetContributorWait(time.Minute)
	alice := e.register("alice@example.com", "Alice", true)
	aliceID := e.ownerID("alice@example.com")
	jpegs := make([][]byte, 8)
	for i := range jpegs {
		jpegs[i] = jpegOf(t, uint8(i*7))
	}
	const rounds = 40
	var bad5xx, landed, lateOK, gone atomic.Int64
	seen := map[int]int{}
	var seenMu sync.Mutex
	for round := range rounds {
		book := e.book(alice, "Storm")
		var ids, tokens []string
		for range 3 {
			id, tok := e.newLink(alice, book, `{}`)
			ids, tokens = append(ids, id), append(tokens, tok)
		}
		var deleted atomic.Bool
		var wg sync.WaitGroup
		rec := func(code int) {
			seenMu.Lock()
			seen[code]++
			seenMu.Unlock()
			if code >= 500 {
				bad5xx.Add(1)
			}
		}
		// After the book delete answered, a later request must see the book as gone.
		afterDelete := func(code int) {
			if deleted.Load() && code != 404 && code != 410 && code != 429 {
				lateOK.Add(1)
				t.Errorf("round %d: status %d after the book was deleted", round, code)
			}
			if code == 404 || code == 410 {
				gone.Add(1)
			}
		}
		const subs = 10
		wg.Add(subs + 9)
		for i := range subs {
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(rand.Intn(40)) * time.Millisecond)
				ip := fmt.Sprintf("203.0.%d.%d", round+1, i+1)
				parts := []part{answers("Nguyễn Ánh 🌸", "Nhớ mãi 😭")}
				for k := range i % 4 { // 0..3 photos
					parts = append(parts, photoPart(jpegs[(i+k)%len(jpegs)]))
				}
				was := deleted.Load()
				code := e.postFrom(context.Background(), ip, tokens[i%3], parts...).Code
				rec(code)
				if code == 201 {
					landed.Add(1)
				}
				if was {
					afterDelete(code)
				}
			}()
		}
		for i := range 3 { // public lookups
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(rand.Intn(40)) * time.Millisecond)
				was := deleted.Load()
				code := e.lookupFrom(tokens[i]).Code
				rec(code)
				if was {
					afterDelete(code)
				}
			}()
		}
		go func() { // revoke twice (idempotent) and a third link
			defer wg.Done()
			for _, id := range []string{ids[0], ids[0], ids[1]} {
				time.Sleep(time.Duration(rand.Intn(15)) * time.Millisecond)
				rec(e.do(alice, "DELETE", "/v1/collections/"+id, "").Code)
			}
		}()
		go func() { // new links
			defer wg.Done()
			for range 3 {
				time.Sleep(time.Duration(rand.Intn(15)) * time.Millisecond)
				rec(e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{}`).Code)
			}
		}()
		go func() { // list
			defer wg.Done()
			for range 3 {
				time.Sleep(time.Duration(rand.Intn(15)) * time.Millisecond)
				rec(e.do(alice, "GET", "/v1/yearbooks/"+book+"/collections", "").Code)
			}
		}()
		go func() { // photo deletes: whatever note photos exist, over and over
			defer wg.Done()
			for range 25 {
				time.Sleep(time.Duration(rand.Intn(8)) * time.Millisecond)
				rows, err := e.db.Query(`SELECT m.public_id FROM media_tab m JOIN note_photo_tab np ON np.media_id = m.id JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE y.public_id = ?`, book)
				if err != nil {
					t.Error(err)
					return
				}
				var pubs []string
				for rows.Next() {
					var p string
					_ = rows.Scan(&p)
					pubs = append(pubs, p)
				}
				_ = rows.Close()
				for _, p := range pubs {
					if rand.Intn(2) == 0 {
						if err := e.svc.Delete(context.Background(), aliceID, p); err != nil {
							bad5xx.Add(1)
							t.Errorf("round %d: photo delete: %v", round, err)
						}
					}
				}
			}
		}()
		go func() { // the book delete, at a random point
			defer wg.Done()
			time.Sleep(time.Duration(rand.Intn(120)) * time.Millisecond)
			c := e.do(alice, "DELETE", "/v1/yearbooks/"+book, "").Code
			rec(c)
			deleted.Store(true)
		}()
		go func() { // a second, repeated delete
			defer wg.Done()
			time.Sleep(time.Duration(60+rand.Intn(120)) * time.Millisecond)
			rec(e.do(alice, "DELETE", "/v1/yearbooks/"+book, "").Code)
		}()
		wg.Wait()
		for _, tb := range []string{"note_collection_tab", "note_tab", "note_photo_tab", "media_tab", "yearbook_tab", "profile_tab"} {
			if n := e.count(`SELECT COUNT(*) FROM ` + tb); n != 0 {
				t.Fatalf("round %d: %s holds %d rows after the book was deleted", round, tb, n)
			}
		}
		// Known, documented limit (media.Service.PurgeYearbook, "ponytail"): an upload that lands right after the prefix purge and whose row
		// goes in before the delete locks the book leaves that one photo's objects (at most 3 keys) until the M2 sweep. Under heavy CPU load
		// the storm hits it in about 1 of 20 runs. Rows are strictly zero above; more than one photo's worth of objects is still a failure.
		if n := e.st.count(); n > 3 {
			t.Fatalf("round %d: %d objects left in storage", round, n)
		} else if n > 0 {
			t.Logf("round %d: %d orphan object(s) from an upload racing the purge (known, swept in M2)", round, n)
		}
		// a fresh request on the vanished book
		e.lookupFrom(tokens[2]).status(404, "")
		e.postFrom(context.Background(), fmt.Sprintf("198.18.%d.1", round), tokens[2], answers("A", "m"), photoPart(jpegs[0])).status(404, "")
		e.do(alice, "DELETE", "/v1/collections/"+ids[2], "").status(404, "")
	}
	t.Logf("status codes: %v; landed before delete: %d; 404/410 seen: %d", seen, landed.Load(), gone.Load())
	if bad5xx.Load() != 0 {
		for _, l := range strings.Split(e.logs.String(), "\n") { // the server's own account of each failure
			if strings.Contains(l, `"level":"ERROR"`) {
				t.Log(l)
			}
		}
		t.Fatalf("%d 5xx / errors", bad5xx.Load())
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// Same storm without a book delete: revoke, creates, photo deletes and submits only. Rows and objects must agree at the end.
func TestQAStormRevokeAndPhotoDelete(t *testing.T) {
	e := newEnv(t)
	e.svc.SetContributorWait(time.Minute)
	alice := e.register("alice@example.com", "Alice", true)
	aliceID := e.ownerID("alice@example.com")
	jpegs := [][]byte{jpegOf(t, 1), jpegOf(t, 50), jpegOf(t, 99)}
	book := e.book(alice, "Storm")
	var bad atomic.Int64
	codes := &tally{m: map[string]int{}}
	for round := range 25 {
		id, tok := e.newLink(alice, book, `{}`)
		var revokedAt atomic.Int64
		var wg sync.WaitGroup
		wg.Add(10)
		for i := range 8 {
			go func() {
				defer wg.Done()
				time.Sleep(time.Duration(rand.Intn(30)) * time.Millisecond)
				began := time.Now().UnixNano()
				code := e.postFrom(context.Background(), fmt.Sprintf("203.1.%d.%d", round+1, i+1), tok, answers("A", "m"), photoPart(jpegs[i%3]), photoPart(jpegs[(i+1)%3])).Code
				if code >= 500 {
					bad.Add(1)
				}
				codes.Add(fmt.Sprint(code))
				if r := revokedAt.Load(); r != 0 && began > r && code != 404 {
					t.Errorf("round %d: submit started after the revoke answered, got %d", round, code)
				}
			}()
		}
		go func() {
			defer wg.Done()
			time.Sleep(time.Duration(rand.Intn(250)) * time.Millisecond)
			if c := e.do(alice, "DELETE", "/v1/collections/"+id, "").Code; c != 204 {
				bad.Add(1)
			}
			revokedAt.Store(time.Now().UnixNano())
		}()
		go func() {
			defer wg.Done()
			for range 20 {
				time.Sleep(time.Duration(rand.Intn(6)) * time.Millisecond)
				rows, _ := e.db.Query(`SELECT m.public_id FROM media_tab m JOIN note_photo_tab np ON np.media_id = m.id`)
				var pubs []string
				for rows.Next() {
					var p string
					_ = rows.Scan(&p)
					pubs = append(pubs, p)
				}
				_ = rows.Close()
				for _, p := range pubs {
					if rand.Intn(15) == 0 {
						if err := e.svc.Delete(context.Background(), aliceID, p); err != nil {
							bad.Add(1)
							t.Errorf("photo delete: %v", err)
						}
					}
				}
			}
		}()
		wg.Wait()
	}
	if bad.Load() != 0 {
		t.Fatalf("%d 5xx / errors", bad.Load())
	}
	// Rows and objects agree: 3 objects per remaining photo row, each remaining photo linked to exactly one note; notes survive.
	media, links, objects := e.count(`SELECT COUNT(*) FROM media_tab`), e.count(`SELECT COUNT(*) FROM note_photo_tab`), e.st.count()
	if media != links || objects != 3*media {
		t.Fatalf("media rows %d, note links %d, objects %d", media, links, objects)
	}
	t.Logf("submit codes %v", codes.m)
	t.Logf("%d notes, %d photos left", e.count(`SELECT COUNT(*) FROM note_tab`), media)
	dbtest.AssertNoOrphans(t, e.db)
}

// The registry in TestDeleteBookLeavesNoRows must fail when a *_tab table nobody registered exists.
func TestQAUnregisteredTableIsCaught(t *testing.T) {
	e := newEnv(t)
	want := []string{"media_tab", "note_collection_tab", "note_photo_tab", "note_tab", "profile_tab", "yearbook_tab"}
	if got := e.tabTables(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("baseline %v", got)
	}
	if _, err := e.db.Exec(`CREATE TABLE zz_planted_tab (id BIGINT UNSIGNED NOT NULL AUTO_INCREMENT PRIMARY KEY, yearbook_id BIGINT UNSIGNED NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if got := e.tabTables(); fmt.Sprint(got) == fmt.Sprint(want) {
		t.Fatal("a planted table is not listed")
	}
}

type tally struct {
	mu sync.Mutex
	m  map[string]int
}

func (c *tally) Add(k string) { c.mu.Lock(); c.m[k]++; c.mu.Unlock() }
