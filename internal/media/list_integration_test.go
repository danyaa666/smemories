//go:build integration

package media

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"testing"
)

type listPage struct {
	Status string
	Data   struct {
		Items  []mediaBody
		NextID string `json:"next_id"`
	}
}

func (e *env) list(u user, book, query string) listPage {
	e.t.Helper()
	r := e.json(u, "GET", "/api/media/get-list?yearbook_id="+book+query, "").status(200, "")
	var p listPage
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil || p.Status != "OK" {
		e.t.Fatalf("list body %s: %v", r.Body, err)
	}
	return p
}

// seed inserts n rows straight into the table (uploads are rate limited); returns their public ids, oldest first.
func (e *env) seed(book, kind string, n int) []string {
	e.t.Helper()
	var yid uint64
	if err := e.db.QueryRow(`SELECT id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&yid); err != nil {
		e.t.Fatal(err)
	}
	var ids []string
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("%s%05d", strings.ToUpper(kind[:1]+book[:20]), len(ids)+i) // 26 chars, unique per book and kind
		if _, err := e.db.Exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
			VALUES (?,?,?,?,?,'image/jpeg',10,4,3,?,1,1)`, id, yid, kind, "k/"+id, "k/"+id+"t", strings.Repeat("a", 64)); err != nil {
			e.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func TestListPagingAndFilter(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	owner := e.seed(book, "owner", 120)
	contrib := e.seed(book, "contributor", 3)

	// Default filter is owner; 120 photos page as 50 + 50 + 20, newest first, no duplicates or gaps.
	var seen []string
	next := ""
	for pages := 0; ; pages++ {
		p := e.list(u, book, "&next_id="+url.QueryEscape(next))
		for _, m := range p.Data.Items {
			if m.YearbookID != book || m.ContentType != "image/jpeg" || m.Width != 4 || m.Height != 3 || m.Bytes != 10 || m.CreatedAt != 1 {
				t.Fatalf("item %+v", m)
			}
			seen = append(seen, m.ID)
		}
		if p.Data.NextID == "" {
			if pages != 2 || len(p.Data.Items) != 20 {
				t.Fatalf("last page %d items after %d pages", len(p.Data.Items), pages)
			}
			break
		}
		if len(p.Data.Items) != 50 {
			t.Fatalf("page of %d", len(p.Data.Items))
		}
		next = p.Data.NextID
	}
	if len(seen) != 120 || seen[0] != owner[119] || seen[119] != owner[0] {
		t.Fatalf("order/length wrong: %d, first %s", len(seen), seen[0])
	}

	// Exactly one page of results: no next_id.
	if p := e.list(u, book, "&limit=100&uploader=contributor"); len(p.Data.Items) != 3 || p.Data.NextID != "" || p.Data.Items[0].ID != contrib[2] {
		t.Fatalf("contributor: %+v", p)
	}
	if p := e.list(u, book, "&limit=100&uploader=all"); len(p.Data.Items) != 100 || p.Data.NextID == "" {
		t.Fatalf("all: %d", len(p.Data.Items))
	}
	if p := e.list(u, e.newBook(u), ""); p.Data.Items == nil || len(p.Data.Items) != 0 || p.Data.NextID != "" {
		t.Fatalf("empty book must list [] : %+v", p)
	}
}

func TestListStableWhileChanging(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	ids := e.seed(book, "owner", 6)

	p1 := e.list(u, book, "&limit=2")                                                     // ids[5], ids[4]
	e.deleteMedia(u, ids[4]).status(200, "")                                              // delete the cursor photo itself
	e.deleteMedia(u, ids[2]).status(200, "")                                              // and one not yet seen
	newer := e.upload(u, book, "n.jpg", "image/jpeg", photo(t)).status(200, "").mediaID() // an upload between pages
	p2 := e.list(u, book, "&limit=10&next_id="+p1.Data.NextID)
	var got []string
	for _, m := range p2.Data.Items {
		got = append(got, m.ID)
	}
	if want := []string{ids[3], ids[1], ids[0]}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("page 2 = %v, want %v (new upload %s must not appear behind the cursor)", got, want, newer)
	}
	if p := e.list(u, book, "&limit=1"); p.Data.Items[0].ID != newer {
		t.Fatalf("a fresh first page starts with the new upload")
	}
}

func TestListOwnershipAndLeaks(t *testing.T) {
	e := newEnv(t)
	u, other := e.register("a@example.com"), e.register("b@example.com")
	book := e.newBook(u)
	e.seed(book, "owner", 2)
	otherBook := e.newBook(other)
	e.seed(otherBook, "owner", 2)

	e.json(other, "GET", "/api/media/get-list?yearbook_id="+book, "").status(404, "ERROR_NOT_FOUND")
	e.json(u, "GET", "/api/media/get-list?yearbook_id=01J9Z3K6V8Q4M7N2P5R8T0ZZZZ", "").status(404, "ERROR_NOT_FOUND")
	e.json(user{}, "GET", "/api/media/get-list?yearbook_id="+book, "").status(401, "ERROR_UNAUTHORIZED")

	// A page token taken from another user's book (or from the same user's other book) is refused, never applied.
	foreign := e.list(other, otherBook, "&limit=1").Data.NextID
	e.json(u, "GET", "/api/media/get-list?yearbook_id="+book+"&next_id="+foreign, "").status(400, "ERROR_PARAM")
	body := e.json(u, "GET", "/api/media/get-list?yearbook_id="+book+"&uploader=all", "").status(200, "").Body.String()
	for _, banned := range []string{"object_key", "thumb_key", "sha256", "yearbooks/", "http", "uploader_kind"} {
		if strings.Contains(body, banned) {
			t.Fatalf("response leaks %q: %s", banned, body)
		}
	}
}

func TestListBadParameters(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	e.seed(book, "owner", 3)
	good := e.list(u, book, "&limit=1").Data.NextID
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	tampered := []byte(good)
	tampered[len(tampered)/2] ^= 1
	// A token the server never issued: the old plain-number cursors, the number inside a token, garbage, a changed token.
	for q, name := range map[string]string{
		"&limit=0": "limit 0", "&limit=-1": "limit -1", "&limit=101": "limit over max", "&limit=x": "limit not a number",
		"&uploader=everyone": "unknown uploader", "&uploader=Owner": "uploader is case-sensitive",
		"&next_id=!!!": "garbage", "&next_id=" + enc("5"): "plain number", "&next_id=" + enc("abc"): "plain text",
		"&next_id=" + string(tampered): "tampered token", "&next_id=" + good + "A": "extended token", "&next_id=" + good[:len(good)-4]: "cut token",
	} {
		t.Run(name, func(t *testing.T) {
			e.json(u, "GET", "/api/media/get-list?yearbook_id="+book+q, "").status(400, "ERROR_PARAM")
		})
	}
	e.json(u, "GET", "/api/media/get-list", "").status(400, "ERROR_PARAM") // no yearbook_id
	e.list(u, book, "&limit=100&next_id="+good)                            // valid edge values
}

func TestListUsesIndex(t *testing.T) {
	// keep this query in step with Store.list
	e := newEnv(t)
	u := e.register("a@example.com")
	e.seed(e.newBook(u), "owner", 30)
	var plan string
	if err := e.db.QueryRow(`EXPLAIN FORMAT=TREE SELECT id FROM media_tab FORCE INDEX (idx_yearbook_id) WHERE yearbook_id = 1 AND uploader_kind IN ('owner') AND id < 100 ORDER BY id DESC LIMIT 51`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "idx_yearbook_id") || strings.Contains(strings.ToLower(plan), "sort") {
		t.Fatalf("list is not an index range scan without a sort: %s", plan)
	}
}
