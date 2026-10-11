//go:build integration

package yearbook

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// QA (T-064): deleting a book while it is being edited must never end in a 5xx. Service.Delete takes the profile rows
// first and the book last, Store.modify takes the book first and the profile after it; with two or more concurrent edits
// the opposite lock order ends in "Error 1213 deadlock". On develop (FK cascade) this test passes.
func TestDeleteRacesWithEdits(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	bad := 0
	for round := 0; round < 25; round++ {
		id := e.create(alice, "Race")
		var wg sync.WaitGroup
		codes := make(chan int, 5)
		run := func(method, path, body string) {
			defer wg.Done()
			codes <- e.do(alice, method, path, body).Code
		}
		wg.Add(5)
		go run("DELETE", "/v1/yearbooks/"+id, "")
		go run("PATCH", "/v1/yearbooks/"+id, `{"motto":"a"}`)
		go run("PATCH", "/v1/yearbooks/"+id, `{"motto":"b"}`)
		go run("PUT", "/v1/yearbooks/"+id+"/profile", `{"full_name":"x"}`)
		go run("PUT", "/v1/yearbooks/"+id+"/profile", `{"full_name":"y"}`)
		wg.Wait()
		close(codes)
		for c := range codes {
			if c >= 500 {
				bad++
			}
		}
	}
	if bad != 0 {
		t.Fatalf("%d of 125 requests answered 5xx (deadlock between Service.Delete and Store.modify)", bad)
	}
}

// T-064 (lock order): deleting a photo (ClearMediaRefs + DELETE media, as media.Store.removeRow does) while it is being
// set as cover and profile photo must neither deadlock nor leave a dangling reference.
func TestMediaDeleteRacesWithCoverEdits(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	for round := 0; round < 25; round++ {
		id := e.create(alice, "Race")
		pub := fmt.Sprintf("01J9Z3K6V8Q4M7N2P5R8T0%04d", round)
		row := e.insertMedia(e.bookRow(id), pub)
		var wg sync.WaitGroup
		codes := make(chan int, 8)
		wg.Add(9)
		go func() {
			defer wg.Done()
			tx, err := e.db.Begin()
			if err == nil {
				defer func() { _ = tx.Rollback() }()
				if err = NewStore(e.db).ClearMediaRefs(context.Background(), tx, int64(row)); err == nil {
					_, err = tx.Exec(`DELETE FROM media WHERE id = ?`, row)
				}
				if err == nil {
					err = tx.Commit()
				}
			}
			if err != nil {
				t.Errorf("round %d: photo delete failed: %v", round, err)
			}
		}()
		run := func(method, path, body string) {
			defer wg.Done()
			codes <- e.do(alice, method, path, body).Code
		}
		for i := 0; i < 4; i++ {
			go run("PATCH", "/v1/yearbooks/"+id, fmt.Sprintf(`{"cover_media_id":%q}`, pub))
			go run("PUT", "/v1/yearbooks/"+id+"/profile", fmt.Sprintf(`{"full_name":"x","photo_media_id":%q}`, pub))
		}
		wg.Wait()
		close(codes)
		for c := range codes {
			if c >= 500 {
				t.Errorf("round %d: a request answered %d", round, c)
			}
		}
		if n := e.count(`SELECT COUNT(*) FROM yearbook_tab WHERE cover_media_id = ?`, row) + e.count(`SELECT COUNT(*) FROM profile_tab WHERE photo_media_id = ?`, row); n != 0 {
			t.Fatalf("round %d: %d dangling references to the deleted photo", round, n)
		}
		e.do(alice, "DELETE", "/v1/yearbooks/"+id, "").status(204, "") // keeps the user under the 20-book limit
	}
}
