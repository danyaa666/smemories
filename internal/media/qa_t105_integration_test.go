//go:build integration

package media

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// QA T-105: parallel contributor uploads at the photo and byte boundaries never pass the contributor quota; the owner is untouched.
func TestQAContributorQuotaRace(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	var row uint64
	if err := e.db.QueryRow(`SELECT id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&row); err != nil {
		t.Fatal(err)
	}
	data := photo(t)
	run := func(n int) (ok, quota int32) {
		var wg sync.WaitGroup
		var o, q atomic.Int32
		for range n {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := e.svc.UploadContributor(context.Background(), row, data)
				switch {
				case err == nil:
					o.Add(1)
				case errors.Is(err, ErrContributorQuota):
					q.Add(1)
				default:
					t.Errorf("unexpected error: %v", err)
				}
			}()
		}
		wg.Wait()
		return o.Load(), q.Load()
	}
	// photo count: 597 rows, room for exactly 3
	e.fillMedia(book, KindContributor, maxContributorPhotos-3, 1, 0, 0)
	if ok, quota := run(10); ok != 3 || quota != 7 {
		t.Fatalf("count boundary: ok=%d quota=%d, want 3/7", ok, quota)
	}
	if n := e.count(`SELECT COUNT(*) FROM media_tab WHERE uploader_kind = 'contributor'`); n != maxContributorPhotos {
		t.Fatalf("%d contributor rows", n)
	}
	// objects of the refused uploads are gone: 3 photos x 3 variants
	if got := e.st.count(yearbookPrefix(book)); got != 9 {
		t.Fatalf("%d objects, want 9", got)
	}
	// the owner can still upload (200 photos, 500 MiB are his alone)
	e.upload(u, book, "a.jpg", "image/jpeg", data).status(200, "")
	var owner int
	if err := e.db.QueryRow(`SELECT COUNT(*) FROM media_tab WHERE uploader_kind = 'owner'`).Scan(&owner); err != nil || owner != 1 {
		t.Fatalf("owner rows %d %v", owner, err)
	}

	// bytes: clear the contributors, leave room for about two photos' worth of variants
	if _, err := e.db.Exec(`DELETE FROM media_tab WHERE uploader_kind = 'contributor'`); err != nil {
		t.Fatal(err)
	}
	var per int
	if err := e.db.QueryRow(`SELECT bytes + thumb_bytes + print_bytes FROM media_tab WHERE uploader_kind = 'owner'`).Scan(&per); err != nil || per == 0 {
		t.Fatalf("size of one photo %d %v", per, err)
	}
	e.fillMedia(book, KindContributor, 1, 0, maxContributorBytes-2*per-per/2, 0) // room for 2 whole photos and a half
	if ok, quota := run(8); ok != 2 || quota != 6 {
		t.Fatalf("byte boundary: ok=%d quota=%d, want 2/6", ok, quota)
	}
}
