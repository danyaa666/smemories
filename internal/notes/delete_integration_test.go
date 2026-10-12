//go:build integration

package notes

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db/dbtest"
)

func (e *env) ownerID(email string) uint64 {
	e.t.Helper()
	var id uint64
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = ?`, email).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// T-066 Requirement 4 and AC4: deleting a book that has a cover, a profile photo, two collections (one revoked) and notes in each
// status, some with photos, leaves no row in any table that can refer to it. The tables come from information_schema; the list
// below is the registry: a new *_tab table fails this test until it is added here and populated by the book built below.
func TestDeleteBookLeavesNoRows(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice", true)
	book := e.book(alice, "Class 12A")
	ctx := context.Background()

	cover, err := e.svc.Upload(ctx, e.ownerID("alice@example.com"), book, jpegOf(t, 1))
	if err != nil {
		t.Fatal(err)
	}
	e.do(alice, "PATCH", "/v1/yearbooks/"+book, fmt.Sprintf(`{"cover_media_id":%q}`, cover.ID)).status(200, "")
	e.do(alice, "PUT", "/v1/yearbooks/"+book+"/profile", fmt.Sprintf(`{"full_name":"Alice","photo_media_id":%q}`, cover.ID)).status(200, "")

	_, token1 := e.newLink(alice, book, `{"label":"first"}`)
	id2, token2 := e.newLink(alice, book, fmt.Sprintf(`{"deadline_at":%q}`, e.now().Add(48*time.Hour).Format(time.RFC3339)))
	e.post(token1, answers("A", "plain")).status(201, "")
	e.post(token1, answers("B", "one photo"), photoPart(jpegOf(t, 2))).status(201, "")
	e.post(token1, answers("C", "two photos"), photoPart(jpegOf(t, 3)), photoPart(jpegOf(t, 4))).status(201, "")
	e.post(token2, answers("D", "second link")).status(201, "")
	for i, status := range []string{StatusPending, StatusApproved, StatusHidden} { // one note per status, on the first link
		if _, err := e.db.Exec(`UPDATE note_tab SET status = ? WHERE id = (SELECT id FROM (SELECT id FROM note_tab ORDER BY id LIMIT 1 OFFSET ?) x)`, status, i); err != nil {
			t.Fatal(err)
		}
	}
	e.do(alice, "DELETE", "/v1/collections/"+id2, "").status(204, "")
	// Requirement 3: revoking keeps the notes.
	if n := e.count(`SELECT COUNT(*) FROM note_tab`); n != 4 {
		t.Fatalf("%d notes after the revoke, want 4", n)
	}

	want := []string{"media_tab", "note_collection_tab", "note_photo_tab", "note_tab", "profile_tab", "yearbook_tab"}
	tables := e.tabTables()
	if !slices.Equal(tables, want) {
		t.Fatalf("*_tab tables %v, want %v: register the new table in this test and fill it for the book above", tables, want)
	}
	for _, tb := range tables {
		if e.count(`SELECT COUNT(*) FROM `+tb) == 0 {
			t.Fatalf("%s is empty before the delete: the test book must populate every table", tb)
		}
	}

	usersBefore := e.count(`SELECT COUNT(*) FROM user_tab`)
	if usersBefore == 0 {
		t.Fatal("the test needs an account")
	}
	e.do(alice, "DELETE", "/v1/yearbooks/"+book, "").status(204, "")
	if n := e.count(`SELECT COUNT(*) FROM user_tab`); n != usersBefore {
		t.Errorf("user_tab holds %d rows after the book was deleted, want the account kept (%d)", n, usersBefore)
	}
	for _, tb := range tables {
		if n := e.count(`SELECT COUNT(*) FROM ` + tb); n != 0 {
			t.Errorf("%s holds %d rows after the book was deleted", tb, n)
		}
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// userTables is the account family. A book delete must leave it alone, so it is not in the zero-rows registry above; deleting an
// account is T-025, which moves these two into the registry. A rename or a new user table fails tabTables until it is listed here.
var userTables = []string{"user_identity_tab", "user_tab"}

// tabTables lists the *_tab tables of the test database except userTables, which must all exist.
func (e *env) tabTables() []string {
	e.t.Helper()
	rows, err := e.db.Query(`SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME LIKE '%\_tab' ORDER BY TABLE_NAME`)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			e.t.Fatal(err)
		}
		if !slices.Contains(userTables, n) {
			out = append(out, n)
		}
	}
	for _, n := range userTables {
		if e.count(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '`+n+`'`) != 1 {
			e.t.Fatalf("user table %s is missing: update userTables", n)
		}
	}
	return out
}

// Deleting one book keeps another book's collection, notes and photo links; deleting a photo removes its note link and keeps the note.
func TestDeleteBookAndPhotoScopes(t *testing.T) {
	e := newEnv(t)
	a, b := e.newOpenLink(), e.newOpenLink()
	e.post(a.token, answers("A", "m"), photoPart(jpegOf(t, 1))).status(201, "")
	e.post(b.token, answers("B", "m"), photoPart(jpegOf(t, 2))).status(201, "")
	e.do(a.owner, "DELETE", "/v1/yearbooks/"+a.book, "").status(204, "")
	for _, tb := range []string{"note_collection_tab", "note_tab", "note_photo_tab"} {
		if n := e.count(`SELECT COUNT(*) FROM ` + tb); n != 1 {
			t.Errorf("%s holds %d rows after book A was deleted, want book B's 1", tb, n)
		}
	}
	var pub string
	if err := e.db.QueryRow(`SELECT m.public_id FROM media_tab m JOIN note_photo_tab np ON np.media_id = m.id`).Scan(&pub); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(context.Background(), e.ownerID("owner1@example.com"), pub); err != nil {
		t.Fatal(err)
	}
	if e.count(`SELECT COUNT(*) FROM note_photo_tab`) != 0 || e.count(`SELECT COUNT(*) FROM note_tab`) != 1 || e.count(`SELECT COUNT(*) FROM media_tab`) != 0 {
		t.Fatal("deleting the photo must remove its note link, and only that")
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// T-066 lock order: public submits (with photos), collection creates and revokes racing a delete of the book must never answer
// 5xx (MySQL deadlock 1213 was 31 of 600) and must not leave a collection, note or photo link behind: a submit lands before the
// delete or is answered like an unknown link.
func TestDeleteRacesWithSubmitsAndCollections(t *testing.T) {
	e := newEnv(t)
	e.svc.SetContributorWait(time.Minute) // a busy photo slot is a 503; this test is about locks
	alice := e.register("alice@example.com", "Alice", true)
	const rounds, submits = 12, 4
	bad, landed := 0, 0
	for round := range rounds {
		book := e.book(alice, "Race")
		id1, token1 := e.newLink(alice, book, `{}`)
		_, token2 := e.newLink(alice, book, `{}`)
		var wg sync.WaitGroup
		codes := make(chan int, 2*submits+4)
		do := func(f func() int) {
			defer wg.Done()
			codes <- f()
		}
		wg.Add(2*submits + 4)
		for i := range submits {
			ip := fmt.Sprintf("198.51.%d.%d", round, i+1)
			go do(func() int {
				return e.postFrom(context.Background(), ip, token1, answers("A", "m"), photoPart(jpegOf(t, uint8(i))), photoPart(jpegOf(t, uint8(i+10)))).Code
			})
			go do(func() int { return e.postFrom(context.Background(), ip, token2, answers("B", "m")).Code })
		}
		go do(func() int { return e.do(alice, "DELETE", "/v1/yearbooks/"+book, "").Code })
		go do(func() int { return e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{}`).Code })
		go do(func() int { return e.do(alice, "POST", "/v1/yearbooks/"+book+"/collections", `{}`).Code })
		go do(func() int { return e.do(alice, "DELETE", "/v1/collections/"+id1, "").Code })
		wg.Wait()
		close(codes)
		for c := range codes {
			if c >= 500 {
				bad++
			}
			if c == 201 {
				landed++
			}
		}
		for _, tb := range []string{"note_collection_tab", "note_tab", "note_photo_tab", "media_tab", "yearbook_tab"} {
			if n := e.count(`SELECT COUNT(*) FROM ` + tb); n != 0 {
				t.Fatalf("round %d: %s holds %d rows after the book was deleted", round, tb, n)
			}
		}
	}
	if bad != 0 {
		t.Fatalf("%d requests answered 5xx", bad)
	}
	t.Logf("%d requests created something before the delete won", landed)
	dbtest.AssertNoOrphans(t, e.db)
}
