//go:build integration

package yearbook

import (
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
