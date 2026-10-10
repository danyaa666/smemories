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
	Media []struct {
		ID           string
		UploaderKind string `json:"uploader_kind"`
		CreatedAt    string `json:"created_at"`
		Width        int
		Height       int
		Bytes        int
	}
	NextCursor *string `json:"next_cursor"`
}

func (e *env) list(u user, book, query string) listPage {
	e.t.Helper()
	r := e.json(u, "GET", "/v1/yearbooks/"+book+"/media"+query, "").status(200, "")
	var p listPage
	if err := json.Unmarshal(r.Body.Bytes(), &p); err != nil {
		e.t.Fatal(err)
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
		if _, err := e.db.Exec(`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at)
			VALUES (?,?,?,?,?,'image/jpeg',10,4,3,?,UTC_TIMESTAMP(6))`, id, yid, kind, "k/"+id, "k/"+id+"t", strings.Repeat("a", 64)); err != nil {
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
	cursor := ""
	for pages := 0; ; pages++ {
		p := e.list(u, book, "?cursor="+url.QueryEscape(cursor))
		for _, m := range p.Media {
			if m.UploaderKind != "owner" || m.Width != 4 || m.Height != 3 || m.Bytes != 10 || m.CreatedAt == "" {
				t.Fatalf("item %+v", m)
			}
			seen = append(seen, m.ID)
		}
		if p.NextCursor == nil {
			if pages != 2 || len(p.Media) != 20 {
				t.Fatalf("last page %d items after %d pages", len(p.Media), pages)
			}
			break
		}
		if len(p.Media) != 50 {
			t.Fatalf("page of %d", len(p.Media))
		}
		cursor = *p.NextCursor
	}
	if len(seen) != 120 || seen[0] != owner[119] || seen[119] != owner[0] {
		t.Fatalf("order/length wrong: %d, first %s", len(seen), seen[0])
	}

	// Exactly one page of results: no cursor.
	if p := e.list(u, book, "?limit=100&uploader=contributor"); len(p.Media) != 3 || p.NextCursor != nil || p.Media[0].ID != contrib[2] {
		t.Fatalf("contributor: %+v", p)
	}
	if p := e.list(u, book, "?limit=100&uploader=all"); len(p.Media) != 100 || p.NextCursor == nil {
		t.Fatalf("all: %d", len(p.Media))
	}
	if p := e.list(u, e.newBook(u), ""); p.Media == nil || len(p.Media) != 0 || p.NextCursor != nil {
		t.Fatalf("empty book must list [] : %+v", p)
	}
}

func TestListStableWhileChanging(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	ids := e.seed(book, "owner", 6)

	p1 := e.list(u, book, "?limit=2")                                                     // ids[5], ids[4]
	e.json(u, "DELETE", "/v1/media/"+ids[4], "").status(204, "")                          // delete the cursor photo itself
	e.json(u, "DELETE", "/v1/media/"+ids[2], "").status(204, "")                          // and one not yet seen
	newer := e.upload(u, book, "n.jpg", "image/jpeg", photo(t)).status(201, "").mediaID() // an upload between pages
	p2 := e.list(u, book, "?limit=10&cursor="+*p1.NextCursor)
	var got []string
	for _, m := range p2.Media {
		got = append(got, m.ID)
	}
	if want := []string{ids[3], ids[1], ids[0]}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("page 2 = %v, want %v (new upload %s must not appear behind the cursor)", got, want, newer)
	}
	if p := e.list(u, book, "?limit=1"); p.Media[0].ID != newer {
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

	e.json(other, "GET", "/v1/yearbooks/"+book+"/media", "").status(404, "not_found")
	e.json(u, "GET", "/v1/yearbooks/01J9Z3K6V8Q4M7N2P5R8T0ZZZZ/media", "").status(404, "not_found")
	e.json(user{}, "GET", "/v1/yearbooks/"+book+"/media", "").status(401, "")

	// A cursor taken from another user's book only narrows the caller's own book.
	foreign := *e.list(other, otherBook, "?limit=1").NextCursor
	if p := e.list(u, book, "?cursor="+foreign); len(p.Media) > 2 {
		t.Fatalf("leak: %+v", p)
	}
	body := e.json(u, "GET", "/v1/yearbooks/"+book+"/media?uploader=all", "").status(200, "").Body.String()
	for _, banned := range []string{"object_key", "thumb_key", "sha256", "yearbooks/", "http"} {
		if strings.Contains(body, banned) {
			t.Fatalf("response leaks %q: %s", banned, body)
		}
	}
}

func TestListBadParameters(t *testing.T) {
	e := newEnv(t)
	u := e.register("a@example.com")
	book := e.newBook(u)
	enc := func(s string) string { return base64.RawURLEncoding.EncodeToString([]byte(s)) }
	for q, code := range map[string]string{
		"?limit=0": "invalid_limit", "?limit=-1": "invalid_limit", "?limit=101": "invalid_limit", "?limit=x": "invalid_limit",
		"?uploader=everyone": "invalid_uploader", "?uploader=Owner": "invalid_uploader",
		"?cursor=!!!": "invalid_cursor", "?cursor=" + enc("abc"): "invalid_cursor", "?cursor=" + enc("0"): "invalid_cursor",
		"?cursor=" + enc("-5"): "invalid_cursor", "?cursor=" + enc("+5"): "invalid_cursor", "?cursor=" + enc("007"): "invalid_cursor",
		"?cursor=" + enc("99999999999999999999"): "invalid_cursor",
	} {
		t.Run(q, func(t *testing.T) { e.json(u, "GET", "/v1/yearbooks/"+book+"/media"+q, "").status(400, code) })
	}
	e.json(u, "GET", "/v1/yearbooks/"+book+"/media?limit=100&cursor="+enc("5"), "").status(200, "") // valid edge values
}

func TestListUsesIndex(t *testing.T) {
	// keep this query in step with Store.list
	e := newEnv(t)
	u := e.register("a@example.com")
	e.seed(e.newBook(u), "owner", 30)
	var plan string
	if err := e.db.QueryRow(`EXPLAIN FORMAT=TREE SELECT id FROM media FORCE INDEX (ix_media_yearbook) WHERE yearbook_id = 1 AND uploader_kind IN ('owner') AND id < 100 ORDER BY id DESC LIMIT 51`).Scan(&plan); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan, "ix_media_yearbook") || strings.Contains(strings.ToLower(plan), "sort") {
		t.Fatalf("list is not an index range scan without a sort: %s", plan)
	}
}
