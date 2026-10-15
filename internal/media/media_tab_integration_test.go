//go:build integration

package media

import (
	"fmt"
	"sync"
	"testing"

	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-065 AC3: deleting a book deletes its media rows (3 photos, one of them the cover) and no other book's rows.
func TestDeleteYearbookDeletesMediaRows(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	doomed, kept := e.newBook(u), e.newBook(u)
	own := e.seed(doomed, "owner", 2)
	contrib := e.seed(doomed, "contributor", 1)
	e.seed(kept, "owner", 2)
	e.updateBook(u, doomed, fmt.Sprintf(`{"cover_media_id":%q}`, own[0])).status(200, "")
	_ = contrib

	e.deleteBook(u, doomed).status(200, "")
	if n := e.count(`SELECT COUNT(*) FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE y.public_id = ?`, kept); n != 2 {
		t.Fatalf("the other book has %d photos, want 2", n)
	}
	if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 2 {
		t.Fatalf("%d media rows left, want the 2 of the other book", n)
	}
	dbtest.AssertNoOrphans(t, e.db)
	e.deleteBook(u, kept).status(200, "")         // a book with its photos only
	e.deleteBook(u, e.newBook(u)).status(200, "") // a book without photos
	if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 {
		t.Fatalf("%d media rows left", n)
	}
}

// T-065 (lock order, docs/db-conventions.md): uploads, a photo delete, cover and profile photo edits and the book delete
// run in parallel on one book. The yearbook row is locked first everywhere, so no request may answer 5xx (MySQL 1213
// deadlock) and nothing may refer to a missing row afterwards.
func TestParallelUploadDeleteCoverDeleteBook(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	data := photo(t)
	for round := 0; round < 6; round++ {
		book := e.newBook(u)
		ids := e.seed(book, "owner", 3)
		e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, ids[0])).status(200, "")
		var wg sync.WaitGroup
		codes := make(chan int, 16)
		run := func(f func() resp) {
			defer wg.Done()
			codes <- f().Code
		}
		jobs := []func() resp{
			func() resp { return e.upload(u, book, "a.jpg", "image/jpeg", data) },
			func() resp { return e.upload(u, book, "b.jpg", "image/jpeg", data) },
			func() resp { return e.deleteMedia(u, ids[0]) },
			func() resp { return e.deleteMedia(u, ids[1]) },
			func() resp { return e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, ids[1])) },
			func() resp { return e.updateBook(u, book, fmt.Sprintf(`{"cover_media_id":%q}`, ids[2])) },
			func() resp {
				return e.updateProfile(u, book, fmt.Sprintf(`{"full_name":"x","photo_media_id":%q}`, ids[0]))
			},
			func() resp { return e.deleteBook(u, book) },
		}
		wg.Add(len(jobs))
		for _, j := range jobs {
			go run(j)
		}
		wg.Wait()
		close(codes)
		for c := range codes {
			if c >= 500 {
				t.Errorf("round %d: a request answered %d", round, c)
			}
		}
		e.deleteBook(u, book) // the racing delete may have lost to a 404 or won; either way the book must go
		if n := e.count(`SELECT COUNT(*) FROM yearbook_tab WHERE public_id = ?`, book); n != 0 {
			t.Fatalf("round %d: the book survived", round)
		}
		if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 {
			t.Fatalf("round %d: %d media rows left", round, n)
		}
		dbtest.AssertNoOrphans(t, e.db)
	}
}
