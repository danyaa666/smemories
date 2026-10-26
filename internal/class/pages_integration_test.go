//go:build integration

package class

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/ulid"
)

// T-127: class pages and the book order. The matrix is admin-only for reads and writes; the order is a full permutation applied
// under the class lock.

func jbody(m map[string]any) string {
	b, _ := json.Marshal(m)
	return string(b)
}

func pageReq(cls, kind, title, text, placement string) string {
	return jbody(map[string]any{"class_id": cls, "kind": kind, "title": title, "body": text, "placement": placement})
}

// seedPages inserts n pages of a placement straight into the database (no write budget used) and returns their public ids in order.
func (e *env) seedPages(cls, placement string, n int) []string {
	e.t.Helper()
	now := e.now().UnixMilli()
	var out []string
	for i := range n {
		id := ulid.New(e.now())
		e.exec(`INSERT INTO class_page_tab (public_id, class_id, kind, title, body, placement, sort_order, created_at, updated_at)
			SELECT ?, id, 'custom', ?, '', ?, ?, ?, ? FROM class_tab WHERE public_id = ?`, id, fmt.Sprintf("Page %d", i), placement, i+1, now, now, cls)
		out = append(out, id)
	}
	return out
}

func (e *env) column(q string, args ...any) []string {
	e.t.Helper()
	rows, err := e.db.Query(q, args...)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			e.t.Fatal(err)
		}
		out = append(out, s)
	}
	return out
}

// order is the stored book order of a class: its members, front pages and back pages, as the class book lists them.
type order struct{ Members, Front, Back []string }

func (o order) key() string {
	return strings.Join(slices.Concat(o.Members, []string{"|"}, o.Front, []string{"|"}, o.Back), ",")
}

func (e *env) order(cls string) order {
	e.t.Helper()
	pages := func(placement string) []string {
		return e.column(`SELECT p.public_id FROM class_page_tab p JOIN class_tab c ON c.id = p.class_id WHERE c.public_id = ? AND p.placement = ? ORDER BY p.sort_order, p.id`, cls, placement)
	}
	return order{
		Members: e.column(`SELECT m.public_id FROM class_member_tab m JOIN class_tab c ON c.id = m.class_id WHERE c.public_id = ? ORDER BY m.sort_order, m.id`, cls),
		Front:   pages(PlacementFront), Back: pages(PlacementBack),
	}
}

func (o order) body(cls string) string {
	return jbody(map[string]any{"class_id": cls, "member_ids": o.Members, "front_page_ids": o.Front, "back_page_ids": o.Back})
}

func (e *env) reorder(p person, cls string, o order) resp {
	e.t.Helper()
	return e.do(p, "POST", "/api/class-book/update-order", o.body(cls))
}

func (e *env) pageCount(cls string) int {
	return e.count(`SELECT COUNT(*) FROM class_page_tab p JOIN class_tab c ON c.id = p.class_id WHERE c.public_id = ?`, cls)
}

// AC1: only admins write (and read) pages and the order; students get 403, non-members 404 (the answer for a class that does not
// exist, byte for byte), anonymous 401. A refused caller changes nothing.
func TestPageMatrix(t *testing.T) {
	e := newEnv(t)
	admin, student, stranger, outsider := e.user("Admin"), e.user("Student"), e.user("Stranger"), e.user("Outsider")
	e.seedClass([]person{outsider})
	type fixture struct {
		class, page string
		order       order
	}
	for _, ep := range []struct {
		name string
		call func(p person, f fixture) resp
	}{
		{"class-page/create", func(p person, f fixture) resp {
			return e.do(p, "POST", "/api/class-page/create", pageReq(f.class, "intro", "Hello", "Welcome", PlacementFront))
		}},
		{"class-page/update", func(p person, f fixture) resp {
			return e.do(p, "POST", "/api/class-page/update", jbody(map[string]any{"class_id": f.class, "id": f.page, "kind": "events", "title": "Changed", "body": "x", "placement": PlacementBack}))
		}},
		{"class-page/delete", func(p person, f fixture) resp {
			return e.do(p, "POST", "/api/class-page/delete", body("class_id", f.class, "id", f.page))
		}},
		{"class-page/get-list", func(p person, f fixture) resp {
			return e.do(p, "GET", "/api/class-page/get-list?class_id="+f.class, "")
		}},
		{"class-book/update-order", func(p person, f fixture) resp {
			o := f.order
			o.Members = []string{o.Members[1], o.Members[0]}
			return e.reorder(p, f.class, o)
		}},
	} {
		for _, a := range []struct {
			name         string
			who          person
			status       int
			code         string
			sameAsAbsent bool
		}{
			{"admin", admin, 200, "OK", false}, {"student", student, 403, "ERROR_FORBIDDEN", false},
			{"stranger", stranger, 404, "ERROR_NOT_FOUND", true}, {"member of another class", outsider, 404, "ERROR_NOT_FOUND", true},
			{"anonymous", anonymous, 401, "ERROR_UNAUTHORIZED", false},
		} {
			t.Run(ep.name+"/"+a.name, func(t *testing.T) {
				cls, _ := e.seedClass([]person{admin}, student)
				f := fixture{class: cls, page: e.seedPages(cls, PlacementFront, 1)[0]}
				f.order = e.order(cls)
				before, pages := f.order.key(), e.pageCount(cls)
				r := ep.call(a.who, f)
				if r.Code != a.status || r.code() != a.code {
					t.Fatalf("got %d %s, want %d %s", r.Code, r.Body, a.status, a.code)
				}
				if a.sameAsAbsent {
					gone := ep.call(a.who, fixture{class: missingID, page: f.page, order: f.order})
					if r.sameAnswer() != gone.sameAnswer() {
						t.Fatalf("differs from the answer for a class that does not exist:\n%s\n%s", r.Body, gone.Body)
					}
				}
				if a.status != 200 && (e.order(cls).key() != before || e.pageCount(cls) != pages ||
					e.count(`SELECT COUNT(*) FROM class_page_tab p JOIN class_tab c ON c.id = p.class_id WHERE c.public_id = ? AND p.title = 'Changed'`, cls) != 0) {
					t.Fatal("a refused request changed the class")
				}
			})
		}
	}
}

func TestPageLifecycle(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls := e.create(admin)
	other := e.create(admin)

	a := e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "intro", "Dear class", "Line one\nLine two", PlacementFront)).status(200, "OK").data()
	wantKeys(t, "created page", a, "body", "created_at", "id", "kind", "placement", "title", "updated_at")
	b := e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "teachers", "Teachers", "", PlacementBack)).status(200, "OK").data()
	c := e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "custom", "Thanks", "Bye", PlacementFront)).status(200, "OK").data()
	e.do(admin, "POST", "/api/class-page/create", pageReq(other, "custom", "Other class", "", PlacementFront)).status(200, "OK")
	ids := func(placement string) []string { return e.order(cls).get(placement) }
	if got := ids(PlacementFront); !slices.Equal(got, []string{a["id"].(string), c["id"].(string)}) {
		t.Fatalf("a new page must go to the end of its placement: %v", got)
	}

	list := e.do(admin, "GET", "/api/class-page/get-list?class_id="+cls, "").status(200, "OK").data()
	items := list["items"].([]any)
	if len(items) != 3 || list["next_id"] != "" || items[0].(map[string]any)["id"] != a["id"] || items[1].(map[string]any)["id"] != c["id"] || items[2].(map[string]any)["id"] != b["id"] {
		t.Fatalf("get-list must be the front pages then the back pages, in order: %v", list)
	}
	if items[0].(map[string]any)["body"] != "Line one\nLine two" {
		t.Fatalf("newlines must be kept: %v", items[0])
	}

	// update replaces every field; moving to the other side puts the page last there, staying keeps its place
	e.advance(time.Minute)
	u := e.do(admin, "POST", "/api/class-page/update", jbody(map[string]any{"class_id": cls, "id": a["id"], "kind": "events", "title": "New title", "body": "New", "placement": PlacementFront})).status(200, "OK").data()
	if u["title"] != "New title" || u["kind"] != "events" || u["created_at"] != a["created_at"] || u["updated_at"].(float64) <= a["updated_at"].(float64) {
		t.Fatalf("update answer: %v", u)
	}
	if got := ids(PlacementFront); got[0] != a["id"] {
		t.Fatalf("updating in place must keep the position: %v", got)
	}
	e.do(admin, "POST", "/api/class-page/update", jbody(map[string]any{"class_id": cls, "id": a["id"], "kind": "events", "title": "New title", "body": "New", "placement": PlacementBack})).status(200, "OK")
	if f, bk := ids(PlacementFront), ids(PlacementBack); !slices.Equal(f, []string{c["id"].(string)}) || !slices.Equal(bk, []string{b["id"].(string), a["id"].(string)}) {
		t.Fatalf("a page that changes side goes last there: front %v back %v", f, bk)
	}

	// the class book carries the same pages
	book := e.classBook(admin, cls).status(200, "OK").data()
	front, back := book["front_pages"].([]any), book["back_pages"].([]any)
	if len(front) != 1 || front[0].(map[string]any)["id"] != c["id"] || len(back) != 2 || back[1].(map[string]any)["id"] != a["id"] {
		t.Fatalf("class-book/get pages: front %v back %v", front, back)
	}
	wantKeys(t, "book page", back[0].(map[string]any), "body", "created_at", "id", "kind", "placement", "title", "updated_at")

	e.do(admin, "POST", "/api/class-page/delete", body("class_id", cls, "id", c["id"].(string))).status(200, "OK")
	e.do(admin, "POST", "/api/class-page/delete", body("class_id", cls, "id", c["id"].(string))).status(404, "ERROR_NOT_FOUND")
	e.do(admin, "POST", "/api/class-page/update", jbody(map[string]any{"class_id": cls, "id": c["id"], "kind": "custom", "title": "x", "body": "", "placement": PlacementFront})).status(404, "ERROR_NOT_FOUND")
	if e.pageCount(cls) != 2 || e.pageCount(other) != 1 {
		t.Fatal("delete touched the wrong rows")
	}
	if front := e.classBook(admin, cls).status(200, "OK").data()["front_pages"].([]any); len(front) != 0 {
		t.Fatalf("front pages after delete: %v", front)
	}
}

func (o order) get(placement string) []string {
	if placement == PlacementFront {
		return o.Front
	}
	return o.Back
}

// An id of another class, in any role, finds nothing and changes nothing.
func TestPageIDsOfAnotherClassChangeNothing(t *testing.T) {
	e := newEnv(t)
	adminA, adminB := e.user("AdminA"), e.user("AdminB")
	clsA, _ := e.seedClass([]person{adminA})
	clsB, _ := e.seedClass([]person{adminB})
	pageB := e.seedPages(clsB, PlacementFront, 1)[0]
	before := e.order(clsB).key()
	a := e.do(adminA, "POST", "/api/class-page/delete", body("class_id", clsA, "id", pageB)).status(404, "ERROR_NOT_FOUND")
	b := e.do(adminA, "POST", "/api/class-page/delete", body("class_id", clsA, "id", missingID)).status(404, "ERROR_NOT_FOUND")
	if a.sameAnswer() != b.sameAnswer() {
		t.Fatalf("a page of another class must look like an unknown one:\n%s\n%s", a.Body, b.Body)
	}
	e.do(adminA, "POST", "/api/class-page/update", jbody(map[string]any{"class_id": clsA, "id": pageB, "kind": "custom", "title": "Hijacked", "body": "", "placement": PlacementBack})).status(404, "ERROR_NOT_FOUND")
	if e.order(clsB).key() != before || e.count(`SELECT COUNT(*) FROM class_page_tab WHERE title = 'Hijacked'`) != 0 {
		t.Fatal("class B changed")
	}
	// ... and as an entry of the order
	o := e.order(clsA)
	o.Front = []string{pageB}
	e.reorder(adminA, clsA, o).status(400, "ERROR_INVALID_FRONT_PAGE_IDS")
	if e.order(clsB).key() != before {
		t.Fatal("class B changed")
	}
}

// AC4: the body is plain text, stored and returned as typed.
func TestPageBodyIsPlainText(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls := e.create(admin)
	text := "<script>alert('x')</script>\n<b>bold</b> &amp; \"quotes\" Tiếng Việt 🎓\n\n  - indented"
	id := e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "custom", "<i>Title</i>", text, PlacementBack)).status(200, "OK").data()["id"].(string)
	var stored, title string
	if err := e.db.QueryRow(`SELECT body, title FROM class_page_tab WHERE public_id = ?`, id).Scan(&stored, &title); err != nil {
		t.Fatal(err)
	}
	if stored != text || title != "<i>Title</i>" {
		t.Fatalf("stored %q %q", stored, title)
	}
	got := e.do(admin, "GET", "/api/class-page/get-list?class_id="+cls, "").status(200, "OK").data()["items"].([]any)[0].(map[string]any)
	if got["body"] != text || got["title"] != "<i>Title</i>" {
		t.Fatalf("get-list: %q", got["body"])
	}
	if bk := e.classBook(admin, cls).status(200, "OK").data()["back_pages"].([]any)[0].(map[string]any); bk["body"] != text {
		t.Fatalf("class-book/get: %q", bk["body"])
	}
	// CRLF is stored as LF, like every multi-line field
	crlf := e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "custom", "t", "a\r\nb", PlacementBack)).status(200, "OK").data()
	if crlf["body"] != "a\nb" {
		t.Fatalf("crlf: %q", crlf["body"])
	}
}

func TestPageValidation(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls := e.create(admin)
	runes := func(n int) string { return strings.Repeat("ế", n) }
	for _, c := range []struct {
		name string
		body string
		code string // "" = accepted
	}{
		{"title 120 runes", pageReq(cls, "custom", runes(120), "", PlacementFront), ""},
		{"body 4000 runes", pageReq(cls, "custom", "t", runes(4000), PlacementFront), ""},
		{"empty body", pageReq(cls, "custom", "t", "", PlacementFront), ""},
		{"every kind: superlatives", pageReq(cls, "superlatives", "t", "", PlacementBack), ""},
		{"title empty", pageReq(cls, "custom", "", "", PlacementFront), "ERROR_INVALID_TITLE"},
		{"title blank", pageReq(cls, "custom", "   ", "", PlacementFront), "ERROR_INVALID_TITLE"},
		{"title 121 runes", pageReq(cls, "custom", runes(121), "", PlacementFront), "ERROR_INVALID_TITLE"},
		{"title with a newline", pageReq(cls, "custom", "a\nb", "", PlacementFront), "ERROR_INVALID_TITLE"},
		{"body 4001 runes", pageReq(cls, "custom", "t", runes(4001), PlacementFront), "ERROR_INVALID_BODY"},
		{"body with a control character", pageReq(cls, "custom", "t", "a\x00b", PlacementFront), "ERROR_INVALID_BODY"},
		{"body with a bidi override", pageReq(cls, "custom", "t", "a\u202eb", PlacementFront), "ERROR_INVALID_BODY"},
		{"kind unknown", pageReq(cls, "poem", "t", "", PlacementFront), "ERROR_INVALID_KIND"},
		{"kind empty", pageReq(cls, "", "t", "", PlacementFront), "ERROR_INVALID_KIND"},
		{"placement unknown", pageReq(cls, "custom", "t", "", "middle"), "ERROR_INVALID_PLACEMENT"},
		{"placement missing", jbody(map[string]any{"class_id": cls, "kind": "custom", "title": "t", "body": ""}), "ERROR_INVALID_PLACEMENT"},
		{"unknown field", jbody(map[string]any{"class_id": cls, "kind": "custom", "title": "t", "body": "", "placement": "front", "sort_order": 1}), "ERROR_PARAM"},
		{"class id missing", jbody(map[string]any{"kind": "custom", "title": "t", "placement": "front"}), "ERROR_PARAM"},
		{"class id malformed", pageReq("nope", "custom", "t", "", PlacementFront), "ERROR_PARAM"},
		{"not json", `{"class_id":`, "ERROR_PARAM"},
	} {
		t.Run(c.name, func(t *testing.T) {
			before := e.pageCount(cls)
			r := e.do(admin, "POST", "/api/class-page/create", c.body)
			if c.code == "" {
				r.status(200, "OK")
				return
			}
			r.status(400, c.code)
			if e.pageCount(cls) != before {
				t.Fatal("a refused page was stored")
			}
		})
	}
	page := e.seedPages(cls, PlacementFront, 1)[0]
	e.do(admin, "POST", "/api/class-page/update", jbody(map[string]any{"class_id": cls, "id": page, "kind": "custom", "title": "t", "body": "", "placement": "nowhere"})).status(400, "ERROR_INVALID_PLACEMENT")
	e.do(admin, "POST", "/api/class-page/update", jbody(map[string]any{"class_id": cls, "id": "nope", "kind": "custom", "title": "t", "body": "", "placement": "front"})).status(400, "ERROR_PARAM")
	e.do(admin, "POST", "/api/class-page/delete", body("class_id", cls)).status(400, "ERROR_PARAM")
	e.do(admin, "GET", "/api/class-page/get-list", "").status(400, "ERROR_PARAM")
}

// The 30-page cap is counted under the class lock: one page too many is 409, and parallel creates cannot slip past it.
func TestPageCap(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls := e.create(admin)
	e.seedPages(cls, PlacementFront, MaxPages-1)
	e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "custom", "last", "", PlacementBack)).status(200, "OK")
	e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "custom", "one too many", "", PlacementBack)).status(409, "ERROR_LIMIT_REACHED")
	if e.pageCount(cls) != MaxPages {
		t.Fatalf("%d pages", e.pageCount(cls))
	}
	// updating at the cap still works, deleting frees a slot
	e.do(admin, "POST", "/api/class-page/delete", body("class_id", cls, "id", e.order(cls).Front[0])).status(200, "OK")
	e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "custom", "again", "", PlacementFront)).status(200, "OK")

	race := e.create(admin)
	e.seedPages(race, PlacementFront, MaxPages-5)
	got := tally(parallel(12, func(i int) resp {
		return e.do(admin, "POST", "/api/class-page/create", pageReq(race, "custom", fmt.Sprintf("p%d", i), "", PlacementFront))
	}))
	if got["OK"] != 5 || got["ERROR_LIMIT_REACHED"] != 7 || e.pageCount(race) != MaxPages {
		t.Fatalf("parallel creates: %v, %d pages", got, e.pageCount(race))
	}
	// every page of the race has its own position
	if n := e.count(`SELECT COUNT(DISTINCT p.sort_order) FROM class_page_tab p JOIN class_tab c ON c.id = p.class_id WHERE c.public_id = ?`, race); n != MaxPages {
		t.Fatalf("%d distinct positions", n)
	}
}

// AC2 (and the happy path): the order is a full permutation. Anything else is a 400 for the list that is wrong, and the old order stays.
func TestUpdateOrder(t *testing.T) {
	e := newEnv(t)
	admin, s1, s2, s3 := e.user("Admin"), e.user("S1"), e.user("S2"), e.user("S3")
	cls, _ := e.seedClass([]person{admin}, s1, s2, s3)
	e.seedPages(cls, PlacementFront, 3)
	e.seedPages(cls, PlacementBack, 2)
	outsider := e.user("Outsider")
	other, _ := e.seedClass([]person{outsider}, e.user("O1"))
	foreign := e.order(other)
	foreignPage := e.seedPages(other, PlacementFront, 1)[0]
	foreignKey := e.order(other).key()

	cur := e.order(cls)
	reversed := order{Members: reverse(cur.Members), Front: reverse(cur.Front), Back: reverse(cur.Back)}

	bad := func(name, code string, mutate func(o *order)) {
		t.Run(name, func(t *testing.T) {
			o := order{Members: slices.Clone(reversed.Members), Front: slices.Clone(reversed.Front), Back: slices.Clone(reversed.Back)}
			mutate(&o)
			e.reorder(admin, cls, o).status(400, code)
			if e.order(cls).key() != cur.key() {
				t.Fatal("a refused order changed the book")
			}
		})
	}
	bad("a member missing", "ERROR_INVALID_MEMBER_IDS", func(o *order) { o.Members = o.Members[1:] })
	bad("an extra member", "ERROR_INVALID_MEMBER_IDS", func(o *order) { o.Members = append(o.Members, foreign.Members[0]) })
	bad("a member of another class instead of one", "ERROR_INVALID_MEMBER_IDS", func(o *order) { o.Members[0] = foreign.Members[0] })
	bad("a member twice", "ERROR_INVALID_MEMBER_IDS", func(o *order) { o.Members[1] = o.Members[0] })
	bad("a malformed member id", "ERROR_INVALID_MEMBER_IDS", func(o *order) { o.Members[0] = "nope" })
	bad("no members", "ERROR_INVALID_MEMBER_IDS", func(o *order) { o.Members = nil })
	bad("too many members", "ERROR_INVALID_MEMBER_IDS", func(o *order) {
		for range MaxMembers {
			o.Members = append(o.Members, ulid.New(e.now()))
		}
	})
	bad("a front page missing", "ERROR_INVALID_FRONT_PAGE_IDS", func(o *order) { o.Front = o.Front[:2] })
	bad("a front page twice", "ERROR_INVALID_FRONT_PAGE_IDS", func(o *order) { o.Front[2] = o.Front[0] })
	bad("a page of another class", "ERROR_INVALID_FRONT_PAGE_IDS", func(o *order) { o.Front[0] = foreignPage })
	bad("a back page in the front list", "ERROR_INVALID_FRONT_PAGE_IDS", func(o *order) { o.Front[0] = o.Back[0]; o.Back = o.Back[1:] })
	bad("a front page in the back list", "ERROR_INVALID_BACK_PAGE_IDS", func(o *order) { o.Back[0] = cur.Front[0] })
	bad("a back page missing", "ERROR_INVALID_BACK_PAGE_IDS", func(o *order) { o.Back = nil })
	bad("too many pages", "ERROR_INVALID_BACK_PAGE_IDS", func(o *order) {
		for range MaxPages {
			o.Back = append(o.Back, ulid.New(e.now()))
		}
	})
	e.do(admin, "POST", "/api/class-book/update-order", jbody(map[string]any{"class_id": cls, "member_ids": cur.Members, "front_page_ids": cur.Front, "back_page_ids": cur.Back, "extra": 1})).status(400, "ERROR_PARAM")
	e.do(admin, "POST", "/api/class-book/update-order", jbody(map[string]any{"class_id": cls, "member_ids": "nope"})).status(400, "ERROR_PARAM")

	e.reorder(admin, cls, reversed).status(200, "OK")
	if got := e.order(cls); got.key() != reversed.key() {
		t.Fatalf("order after update:\n%v\nwant\n%v", got, reversed)
	}
	// class-book/get lists the members and pages in the new order
	book := e.classBook(admin, cls).status(200, "OK").data()
	ids := func(key, field string) []string {
		var out []string
		for _, v := range book[key].([]any) {
			out = append(out, v.(map[string]any)[field].(string))
		}
		return out
	}
	if !slices.Equal(ids("members", "member_id"), reversed.Members) || !slices.Equal(ids("front_pages", "id"), reversed.Front) || !slices.Equal(ids("back_pages", "id"), reversed.Back) {
		t.Fatalf("class-book/get does not follow the order: %v", book)
	}
	// the same order again is fine, and an empty side needs an empty list
	e.reorder(admin, cls, reversed).status(200, "OK")
	if e.order(other).key() != foreignKey {
		t.Fatalf("the other class changed: %v", e.order(other))
	}
	empty, _ := e.seedClass([]person{admin})
	one := e.order(empty)
	e.reorder(admin, empty, one).status(200, "OK")
	e.do(admin, "POST", "/api/class-book/update-order", jbody(map[string]any{"class_id": empty, "member_ids": one.Members})).status(200, "OK")
}

func reverse(s []string) []string {
	c := slices.Clone(s)
	slices.Reverse(c)
	return c
}

// AC3: two admins reordering at the same moment end in one of the two orders, never a mix. Twenty rounds.
func TestConcurrentReordersEndInOneOfTheTwo(t *testing.T) {
	e := newEnv(t)
	a1, a2, s1, s2, s3, s4 := e.user("A1"), e.user("A2"), e.user("S1"), e.user("S2"), e.user("S3"), e.user("S4")
	cls, _ := e.seedClass([]person{a1, a2}, s1, s2, s3, s4)
	e.seedPages(cls, PlacementFront, 4)
	e.seedPages(cls, PlacementBack, 4)
	rng := rand.New(rand.NewPCG(1, 2))
	shuffled := func(o order) order {
		rng.Shuffle(len(o.Members), func(i, j int) { o.Members[i], o.Members[j] = o.Members[j], o.Members[i] })
		rng.Shuffle(len(o.Front), func(i, j int) { o.Front[i], o.Front[j] = o.Front[j], o.Front[i] })
		rng.Shuffle(len(o.Back), func(i, j int) { o.Back[i], o.Back[j] = o.Back[j], o.Back[i] })
		return o
	}
	clone := func(o order) order {
		return order{slices.Clone(o.Members), slices.Clone(o.Front), slices.Clone(o.Back)}
	}
	for round := range 20 {
		cur := e.order(cls)
		x, y := shuffled(clone(cur)), shuffled(clone(cur))
		rs := parallel(2, func(i int) resp {
			if i == 0 {
				return e.reorder(a1, cls, x)
			}
			return e.reorder(a2, cls, y)
		})
		for _, r := range rs {
			r.status(200, "OK")
		}
		if got := e.order(cls).key(); got != x.key() && got != y.key() {
			t.Fatalf("round %d: a mix of the two orders\n got %s\n  x  %s\n  y  %s", round, got, x.key(), y.key())
		}
		if n := e.count(`SELECT COUNT(DISTINCT m.sort_order) FROM class_member_tab m JOIN class_tab c ON c.id = m.class_id WHERE c.public_id = ?`, cls); n != 6 {
			t.Fatalf("round %d: %d distinct member positions", round, n)
		}
	}
}

// A reorder racing a page create or delete or a member change either wins before it (and then refuses the stale list) or after it:
// the book never ends half-ordered, and nothing is a 5xx.
func TestReorderRacingOtherWrites(t *testing.T) {
	e := newEnv(t)
	admin, s1, s2 := e.user("Admin"), e.user("S1"), e.user("S2")
	cls, members := e.seedClass([]person{admin}, s1, s2)
	e.seedPages(cls, PlacementFront, 3)
	for round := range 5 {
		cur := e.order(cls)
		next := order{Members: reverse(cur.Members), Front: reverse(cur.Front), Back: cur.Back}
		rs := parallel(3, func(i int) resp {
			switch i {
			case 0:
				return e.reorder(admin, cls, next)
			case 1:
				return e.do(admin, "POST", "/api/class-page/create", pageReq(cls, "custom", "new", "", PlacementBack))
			default:
				return e.do(admin, "POST", "/api/class-page/delete", body("class_id", cls, "id", cur.Front[0]))
			}
		})
		for _, r := range rs {
			if r.Code >= 500 {
				t.Fatalf("round %d: %d %s", round, r.Code, r.Body)
			}
		}
		o := e.order(cls)
		if e.count(`SELECT COUNT(*) FROM class_member_tab m JOIN class_tab c ON c.id = m.class_id WHERE c.public_id = ?`, cls) != len(members) || len(o.Members) != len(members) {
			t.Fatalf("members changed: %v", o)
		}
		e.exec(`DELETE FROM class_page_tab WHERE title = 'new'`)
		e.seedPages(cls, PlacementFront, 1)
	}
}

// A member who joins after the admin ordered the others goes to the end of the book, not to the front.
func TestNewMemberGoesLast(t *testing.T) {
	e := newEnv(t)
	admin, s1, s2, late := e.user("Admin"), e.user("S1"), e.user("S2"), e.user("Late")
	cls, _ := e.seedClass([]person{admin}, s1, s2)
	cur := e.order(cls)
	reversed := order{Members: reverse(cur.Members)}
	e.reorder(admin, cls, reversed).status(200, "OK")
	_, token := e.createInvite(admin, cls)
	e.accept(late, token).status(200, "OK")
	got := e.order(cls).Members
	if len(got) != 4 || !slices.Equal(got[:3], reversed.Members) || got[3] != e.memberID(cls, late) {
		t.Fatalf("members after a late join: %v, want %v then the new one", got, reversed.Members)
	}
	// and the order that includes the newcomer is accepted, the stale one (without them) is not
	e.reorder(admin, cls, reversed).status(400, "ERROR_INVALID_MEMBER_IDS")
}

func TestDeletingTheClassRemovesItsPages(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls, keep := e.create(admin), e.create(admin)
	e.seedPages(cls, PlacementFront, 2)
	e.seedPages(cls, PlacementBack, 1)
	e.seedPages(keep, PlacementFront, 1)
	e.do(admin, "POST", "/api/class/delete", body("id", cls)).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM class_page_tab`); n != 1 {
		t.Fatalf("%d pages left, want the other class's one", n)
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// A student loses nothing and gains nothing: a demoted admin is refused at once.
func TestDemotedAdminCannotWritePages(t *testing.T) {
	e := newEnv(t)
	admin, other := e.user("Admin"), e.user("Other")
	cls, members := e.seedClass([]person{admin, other})
	e.do(admin, "POST", "/api/class-member/update-role", body("class_id", cls, "member_id", members[other.row], "role", RoleStudent)).status(200, "OK")
	e.do(other, "POST", "/api/class-page/create", pageReq(cls, "custom", "t", "", PlacementFront)).status(403, "ERROR_FORBIDDEN")
	e.do(other, "GET", "/api/class-page/get-list?class_id="+cls, "").status(403, "ERROR_FORBIDDEN")
	e.reorder(other, cls, e.order(cls)).status(403, "ERROR_FORBIDDEN")
}
