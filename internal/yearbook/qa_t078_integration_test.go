//go:build integration

package yearbook

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"testing"
)

// QA T-078: walk the list page by page over HTTP while deleting books between the pages (and, in the second variant,
// while the walk requests run). A surviving book is never skipped or repeated, and next_id is never empty while a
// surviving book is still unseen.
func TestQAPaginationWalkAcrossDeletions(t *testing.T) {
	for _, concurrent := range []bool{false, true} {
		t.Run(fmt.Sprintf("concurrent=%v", concurrent), func(t *testing.T) {
			e := newEnv(t)
			alice := e.register("alice@example.com", "Alice")
			rng := rand.New(rand.NewSource(78))
			for round := 0; round < 25; round++ {
				var ids []string
				for i := 0; i < 15; i++ {
					ids = append(ids, e.create(alice, fmt.Sprintf("B%d", i)))
				}
				alive := map[string]bool{}
				for _, id := range ids {
					alive[id] = true
				}
				var amu sync.Mutex
				kill := func(id string) {
					if c := e.remove(alice, id).Code; c != 200 {
						t.Errorf("delete %s: %d", id, c)
						return
					}
					amu.Lock()
					delete(alive, id)
					amu.Unlock()
				}
				seen := map[string]bool{}
				cursor := ""
				limit := 1 + rng.Intn(4)
				for pages := 0; ; pages++ {
					if pages > 50 {
						t.Fatal("walk does not end")
					}
					path := fmt.Sprintf("/api/yearbook/get-list?limit=%d", limit)
					if cursor != "" {
						path += "&next_id=" + cursor
					}
					// Doomed books: any survivor (seen or not); some deleted while this very request runs.
					var wg sync.WaitGroup
					amu.Lock()
					var live []string
					for id := range alive {
						live = append(live, id)
					}
					amu.Unlock()
					rng.Shuffle(len(live), func(i, j int) { live[i], live[j] = live[j], live[i] })
					doomed := live[:rng.Intn(min(4, len(live))+1)]
					if concurrent {
						for _, id := range doomed {
							wg.Add(1)
							go func() { defer wg.Done(); kill(id) }()
						}
					}
					r := e.do(alice, "GET", path, "")
					wg.Wait()
					if r.Code != 200 {
						t.Fatalf("list: %d %s", r.Code, r.Body.String())
					}
					var body struct {
						Data struct {
							Items  []struct{ ID string } `json:"items"`
							NextID string                `json:"next_id"`
						} `json:"data"`
					}
					if err := json.Unmarshal(r.Body.Bytes(), &body); err != nil {
						t.Fatal(err)
					}
					if len(body.Data.Items) > limit {
						t.Fatalf("page of %d > limit %d", len(body.Data.Items), limit)
					}
					for _, b := range body.Data.Items {
						if seen[b.ID] {
							t.Fatalf("round %d: %s repeated", round, b.ID)
						}
						seen[b.ID] = true
					}
					if !concurrent {
						for _, id := range doomed {
							kill(id)
						}
					}
					if body.Data.NextID == "" {
						// The walk says it is over: every book alive at the end (nothing is created during the walk) must have been seen,
						// except books alive now that were already alive and unseen => that would be a skip.
						amu.Lock()
						for id := range alive {
							if !seen[id] {
								t.Fatalf("round %d (limit %d): next_id empty but surviving %s was never returned", round, limit, id)
							}
						}
						amu.Unlock()
						break
					}
					cursor = body.Data.NextID
				}
				// clean up for the next round
				amu.Lock()
				rest := make([]string, 0, len(alive))
				for id := range alive {
					rest = append(rest, id)
				}
				amu.Unlock()
				for _, id := range rest {
					kill(id)
				}
			}
			if strings.Contains(e.logs.String(), `"level":"ERROR"`) {
				t.Fatalf("an ERROR line was logged:\n%s", e.logs.String())
			}
		})
	}
}

// QA T-078: a book whose profile row is gone (data error, not a race) reads as not found / absent from lists, but an edit
// that holds the book lock still fails loudly with a 500 and an ERROR line.
func TestQAMissingProfileLockedEditStaysLoud(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	keep, broken := e.create(alice, "Keep"), e.create(alice, "Broken")
	if _, err := e.db.Exec(`DELETE FROM profile_tab WHERE yearbook_id = ?`, e.bookRow(broken)); err != nil {
		t.Fatal(err)
	}
	e.get(alice, broken).status(404, "ERROR_NOT_FOUND")
	r := e.do(alice, "GET", "/api/yearbook/get-list", "")
	if r.Code != 200 || strings.Contains(r.Body.String(), broken) || !strings.Contains(r.Body.String(), keep) {
		t.Fatalf("list: %d %s", r.Code, r.Body.String())
	}
	if strings.Contains(e.logs.String(), `"level":"ERROR"`) {
		t.Fatalf("reads logged an ERROR:\n%s", e.logs.String())
	}
	e.update(alice, broken, `{"motto":"x"}`).status(500, "ERROR_INTERNAL")
	if !strings.Contains(e.logs.String(), "yearbook without its owner profile") {
		t.Fatalf("locked edit did not log the data error:\n%s", e.logs.String())
	}
}
