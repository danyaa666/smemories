//go:build integration

package booklayout_test

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/booklayout"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/ulid"
)

// T-173: own page templates (book_template_tab) and apply-template (spec E09_T-173, ADR 0005 Q3).

const unknownID = "01K7QB1B2C3D4E5F6G7H8J9K0M"

func tplCreate(book, kind, name string) string {
	b, _ := json.Marshal(map[string]string{"yearbook_id": book, "kind": kind, "name": name})
	return string(b)
}

func tplDelete(id string) string { return fmt.Sprintf(`{"id":%q}`, id) }

func applyBody(book string, base int, kind, source, id string) string {
	return fmt.Sprintf(`{"yearbook_id":%q,"base_version":%d,"kind":%q,"source":%q,"template_id":%q}`, book, base, kind, source, id)
}

type listed struct {
	ID        string          `json:"id"`
	Source    string          `json:"source"`
	LabelEN   string          `json:"label_en"`
	LabelVI   string          `json:"label_vi"`
	Kind      string          `json:"kind"`
	ItemCount int             `json:"item_count"`
	Page      json.RawMessage `json:"page"`
}

func (e *env) listTemplates(u user, book, kind string) []listed {
	e.t.Helper()
	rec := e.call("POST", "/api/book-layout/template-list", u, tplList(book, kind)).status(200, "OK")
	var b struct{ Data struct{ Items []listed } }
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		e.t.Fatal(err)
	}
	return b.Data.Items
}

func (e *env) ofSource(u user, book, kind, source string) []listed {
	var out []listed
	for _, t := range e.listTemplates(u, book, kind) {
		if t.Source == source {
			out = append(out, t)
		}
	}
	return out
}

// shipped returns the id of the first shipped template of the kind.
func (e *env) shipped(u user, book, kind string) listed {
	e.t.Helper()
	l := e.ofSource(u, book, kind, "shipped")
	if len(l) == 0 {
		e.t.Fatalf("no shipped %s template", kind)
	}
	return l[0]
}

// ownTemplate saves a cover with one text item (tag) and makes an own template of it; returns its id.
func (e *env) ownTemplate(u user, book string, base int, tag string) string {
	e.t.Helper()
	e.call("POST", "/api/book-layout/save", u, save(book, base, doc(textAt(tag)))).status(200, "OK")
	return e.call("POST", "/api/book-layout/template-create", u, tplCreate(book, "cover", "Mine "+tag)).status(200, "OK").data()["id"].(string)
}

// state is everything an apply may change: the stored document with its version and the history rows.
func (e *env) state(u user, book string) string {
	e.t.Helper()
	g := e.call("GET", get(book), u, "").Body.String()
	h, _ := json.Marshal(e.versions(u, book))
	return g + string(h)
}

func (e *env) userID(email string) uint64 {
	e.t.Helper()
	var id uint64
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = ?`, email).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// AC1: create copies the current page, strips image items and the picture background and says how many; names are plain text.
func TestTemplateCreateStripsImages(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	book := e.book(alice, "memory-book")
	// nothing saved yet: there is no page to copy
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", "Empty")).status(400, "ERROR_PARAM")

	img := e.image(book, "design")
	page := fmt.Sprintf(`{"v":1,"pages":{"cover":{"bg":{"media":%q},"items":[%s,{"id":"imgcov1","t":"image","media":%q,"x":100,"w":400,"h":300,"cy":500}]},"back":{"items":[%s]}}}`,
		img, textAt("tpltxt1"), img, textAt("backtx1"))
	e.call("POST", "/api/book-layout/save", alice, save(book, 0, page)).status(200, "OK")

	r := e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", "  My   cover ")).status(200, "OK")
	d := r.data()
	if d["name"] != "My   cover" || d["stripped"] != float64(2) || d["item_count"] != float64(1) || d["kind"] != "cover" || d["template_id"] != "memory-book" || !ulid.Valid(d["id"].(string)) {
		t.Fatalf("created: %.400s", r.Body)
	}
	var stored string
	if err := e.db.QueryRow(`SELECT layout_json FROM book_template_tab`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(stored, img) || strings.Contains(stored, "image") || strings.Contains(stored, `"bg"`) || !strings.Contains(stored, "tpltxt1") || strings.Contains(stored, "backtx1") {
		t.Errorf("stored template: %s", stored)
	}
	if n := e.count(`SELECT item_count FROM book_template_tab`); n != 1 {
		t.Errorf("item_count %d", n)
	}
	if e.seen("tpl:created") != 1 || e.seen("tpl:stripped") != 2 {
		t.Errorf("metrics: created %d stripped %d", e.seen("tpl:created"), e.seen("tpl:stripped"))
	}
	// the other page can be copied too, and names are checked
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "back", "Back")).status(200, "OK")
	for name, body := range map[string]string{
		"empty name":    tplCreate(book, "cover", "   "),
		"long name":     tplCreate(book, "cover", strings.Repeat("a", 61)),
		"control char":  tplCreate(book, "cover", "a\u0007b"),
		"unknown kind":  tplCreate(book, "poster", "x"),
		"unsaved kind":  tplCreate(book, "profile", "x"),
		"bad book id":   tplCreate("nope", "cover", "x"),
		"bad json type": `{"yearbook_id":1}`,
	} {
		if rec := e.call("POST", "/api/book-layout/template-create", alice, body); rec.Code != 400 {
			t.Errorf("%s: %d %.200s", name, rec.Code, rec.Body)
		}
	}
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", "ok-"+strings.Repeat("é", 57))).status(200, "OK") // 60 characters, more bytes
	// someone else's book and anonymous
	e.call("POST", "/api/book-layout/template-create", bob, tplCreate(book, "cover", "x")).status(404, "ERROR_NOT_FOUND")
	e.call("POST", "/api/book-layout/template-create", user{}, tplCreate(book, "cover", "x")).status(401, "ERROR_UNAUTHORIZED")
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(unknownID, "cover", "x")).status(404, "ERROR_NOT_FOUND")
	if n := e.count(`SELECT COUNT(*) FROM book_template_tab`); n != 3 {
		t.Errorf("%d rows, want 3", n)
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// A page over 64 KiB (40 items of 500 four-byte characters) is refused, nothing is stored.
func TestTemplateCreateSizeLimit(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	book := e.book(alice, "memory-book")
	big := strings.Repeat("😀", 500)
	var items []string
	for i := range 40 {
		items = append(items, fmt.Sprintf(`{"id":"big%03d","t":"text","text":%q,"font":"display","size":28,"color":"accent","align":"center","x":100,"w":400,"h":120,"cy":500,"r":0}`, i, big))
	}
	e.call("POST", "/api/book-layout/save", alice, save(book, 0, `{"v":1,"pages":{"cover":{"items":[`+strings.Join(items, ",")+`]}}}`)).status(200, "OK")
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", "Big")).status(413, "ERROR_TOO_LARGE")
	if e.count(`SELECT COUNT(*) FROM book_template_tab`) != 0 {
		t.Fatal("a template was stored")
	}
}

// AC1: the 21st template is refused with data.limit; the limit is per user, a delete frees a place, and parallel creates cannot overshoot.
func TestTemplateLimitTwenty(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	book, bobBook := e.book(alice, "memory-book"), e.book(bob, "memory-book")
	e.call("POST", "/api/book-layout/save", alice, save(book, 0, doc(textAt("limit01")))).status(200, "OK")
	e.call("POST", "/api/book-layout/save", bob, save(bobBook, 0, doc(textAt("limit02")))).status(200, "OK")
	var first string
	for i := range booklayout.MaxOwnTemplates {
		id := e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", fmt.Sprintf("T%02d", i))).status(200, "OK").data()["id"].(string)
		if i == 0 {
			first = id
		}
	}
	r := e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", "21st")).status(409, "ERROR_TEMPLATE_LIMIT")
	if r.data()["limit"] != float64(20) {
		t.Errorf("data: %.200s", r.Body)
	}
	if got := len(e.ofSource(alice, book, "", "own")); got != 20 {
		t.Errorf("lists %d own templates", got)
	}
	e.call("POST", "/api/book-layout/template-create", bob, tplCreate(bobBook, "cover", "Bob's")).status(200, "OK") // the limit is per user
	e.call("POST", "/api/book-layout/template-delete", alice, tplDelete(first)).status(200, "OK")
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", "again")).status(200, "OK")
	e.call("POST", "/api/book-layout/template-delete", alice, tplDelete(first)).status(404, "ERROR_NOT_FOUND")

	// 19 left for carol, 8 parallel creates: exactly one gets the last place
	carol := e.register("carol@example.com")
	cb := e.book(carol, "memory-book")
	e.call("POST", "/api/book-layout/save", carol, save(cb, 0, doc(textAt("limit03")))).status(200, "OK")
	for i := range 19 {
		e.call("POST", "/api/book-layout/template-create", carol, tplCreate(cb, "cover", fmt.Sprintf("C%02d", i))).status(200, "OK")
	}
	var wg sync.WaitGroup
	var ok, full, other atomic.Int32
	for i := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			switch rec := e.do("POST", "/api/book-layout/template-create", carol, tplCreate(cb, "cover", fmt.Sprintf("P%d", i))); rec.Code {
			case 200:
				ok.Add(1)
			case 409:
				full.Add(1)
			default:
				other.Add(1)
				t.Errorf("status %d: %s", rec.Code, rec.Body)
			}
		}()
	}
	wg.Wait()
	if ok.Load() != 1 || full.Load() != 7 || other.Load() != 0 || e.count(`SELECT COUNT(*) FROM book_template_tab WHERE owner_id = ?`, e.userID("carol@example.com")) != 20 {
		t.Fatalf("%d created, %d refused, %d other", ok.Load(), full.Load(), other.Load())
	}
}

// AC1/AC4: list shows the shipped ones and the owner's own; another user, another book template and another kind see none of them; delete is owner
// only and does not touch a book that used the template.
func TestTemplateListOwnAndDelete(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	mem, navy, bobBook := e.book(alice, "memory-book"), e.book(alice, "navy-classic"), e.book(bob, "memory-book")
	e.call("POST", "/api/book-layout/save", alice, save(mem, 0, doc(textAt("list001")))).status(200, "OK")
	first := e.call("POST", "/api/book-layout/template-create", alice, tplCreate(mem, "cover", "First")).status(200, "OK").data()["id"].(string)
	e.now = e.now.Add(time.Second)
	second := e.call("POST", "/api/book-layout/template-create", alice, tplCreate(mem, "cover", "Second")).status(200, "OK").data()["id"].(string)

	own := e.ofSource(alice, mem, "cover", "own")
	if len(own) != 2 || own[0].ID != second || own[1].ID != first || own[0].LabelEN != "Second" || own[0].LabelVI != "Second" || own[0].Kind != "cover" || own[0].ItemCount != 1 || !strings.Contains(string(own[0].Page), "list001") {
		t.Fatalf("own: %+v", own)
	}
	if len(e.ofSource(alice, mem, "cover", "shipped")) == 0 {
		t.Error("the shipped templates are gone")
	}
	all := e.listTemplates(alice, mem, "cover")
	for i, x := range all { // shipped first, own after
		if x.Source == "own" && i < len(all)-2 {
			t.Errorf("own template listed before a shipped one: %v", all)
		}
	}
	if len(e.ofSource(alice, mem, "profile", "own")) != 0 {
		t.Error("an own cover template is listed for profile")
	}
	if len(e.ofSource(alice, navy, "", "own")) != 0 {
		t.Error("an own memory-book template is listed for a navy-classic book")
	}
	if len(e.ofSource(bob, bobBook, "", "own")) != 0 {
		t.Error("bob sees alice's templates")
	}
	body := e.call("POST", "/api/book-layout/template-list", bob, tplList(mem, "")).status(404, "ERROR_NOT_FOUND").Body.String()
	if strings.Contains(body, "list001") {
		t.Error("a refused list leaks a template")
	}

	// apply, then delete the template: the book keeps the page
	e.call("POST", "/api/book-layout/apply-template", alice, applyBody(mem, 1, "cover", "own", first)).status(200, "OK")
	for name, c := range map[string]struct {
		who  user
		body string
		code int
		st   string
	}{
		"other user":   {bob, tplDelete(first), 404, "ERROR_NOT_FOUND"},
		"anonymous":    {user{}, tplDelete(first), 401, "ERROR_UNAUTHORIZED"},
		"unknown":      {alice, tplDelete(unknownID), 404, "ERROR_NOT_FOUND"},
		"bad id":       {alice, tplDelete("nope"), 400, "ERROR_PARAM"},
		"missing id":   {alice, `{}`, 400, "ERROR_PARAM"},
		"a shipped id": {alice, tplDelete("cover-sunny"), 400, "ERROR_PARAM"},
	} {
		e.call("POST", "/api/book-layout/template-delete", c.who, c.body).status(c.code, c.st)
		_ = name
	}
	if e.count(`SELECT COUNT(*) FROM book_template_tab`) != 2 {
		t.Fatal("a refused delete removed a row")
	}
	before := raw(e.call("GET", get(mem), alice, ""), "layout")
	e.call("POST", "/api/book-layout/template-delete", alice, tplDelete(first)).status(200, "OK")
	if got := raw(e.call("GET", get(mem), alice, ""), "layout"); got != before {
		t.Errorf("deleting a template changed the book:\n%s\n%s", before, got)
	}
	if e.count(`SELECT COUNT(*) FROM book_template_tab`) != 1 || e.seen("tpl:deleted") != 1 {
		t.Errorf("rows %d, deleted %d", e.count(`SELECT COUNT(*) FROM book_template_tab`), e.seen("tpl:deleted"))
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// AC2: applying a shipped template replaces only that page, in one new version with one apply snapshot; restoring the snapshot brings the page back.
func TestApplyShippedReplacesOnePage(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	book := e.book(alice, "memory-book")
	cur := fmt.Sprintf(`{"v":1,"pages":{"cover":{"items":[%s]},"back":{"items":[%s]}}}`, textAt("oldcov1"), textAt("oldbak1"))
	e.call("POST", "/api/book-layout/save", alice, save(book, 0, cur)).status(200, "OK")
	tpl := e.shipped(alice, book, "cover")

	r := e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 1, "cover", "shipped", tpl.ID)).status(200, "OK")
	d := r.data()
	if d["version"] != float64(2) || d["dropped"] != float64(0) || d["layout"] == nil {
		t.Fatalf("apply: %.300s", r.Body)
	}
	stored := raw(e.call("GET", get(book), alice, ""), "layout")
	if strings.Contains(stored, "oldcov1") || !strings.Contains(stored, "oldbak1") {
		t.Errorf("stored: %.400s", stored)
	}
	var doc struct{ Pages map[string]json.RawMessage }
	_ = json.Unmarshal([]byte(stored), &doc)
	if string(doc.Pages["cover"]) != string(tpl.Page) {
		t.Errorf("cover is not the template:\n%s\n%s", doc.Pages["cover"], tpl.Page)
	}
	vs := e.versions(alice, book)
	if len(vs) != 1 || vs[0].Kind != "apply" || vs[0].Version != 1 || vs[0].CreatedAt != e.now.UnixMilli() {
		t.Fatalf("history: %+v", vs)
	}
	if g := e.call("POST", "/api/book-layout/version-get", alice, versionBody(book, vs[0].ID)).status(200, "OK"); !strings.Contains(raw(g, "layout"), "oldcov1") {
		t.Errorf("the snapshot is not the document before: %.300s", g.Body)
	}
	if e.seen("snapshot:apply") != 1 || e.seen("apply:shipped:ok") != 1 {
		t.Errorf("metrics: snapshots %d applies %d", e.seen("snapshot:apply"), e.seen("apply:shipped:ok"))
	}
	// restore brings the earlier page back (undo)
	e.call("POST", "/api/book-layout/restore", alice, restore(book, vs[0].ID, 2)).status(200, "OK")
	if got := raw(e.call("GET", get(book), alice, ""), "layout"); !strings.Contains(got, "oldcov1") {
		t.Errorf("after restore: %.300s", got)
	}
	// every shipped template of every kind applies to a book that has none of those pages
	for _, kind := range []string{"cover", "profile", "notes", "back"} {
		for _, s := range e.ofSource(alice, book, kind, "shipped") {
			v := int(e.call("GET", get(book), alice, "").data()["version"].(float64))
			e.now = e.now.Add(time.Second)
			e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, v, kind, "shipped", s.ID)).status(200, "OK")
		}
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// Applying creates the layout when the book has none (no snapshot: nothing was replaced); the same after a reset.
func TestApplyCreatesALayout(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	book := e.book(alice, "navy-classic")
	tpl := e.shipped(alice, book, "profile")
	e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 5, "profile", "shipped", tpl.ID)).status(409, "ERROR_VERSION_CONFLICT")
	r := e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 0, "profile", "shipped", tpl.ID)).status(200, "OK")
	if r.data()["version"] != float64(1) || len(e.versions(alice, book)) != 0 {
		t.Fatalf("apply: %.300s, %d snapshots", r.Body, len(e.versions(alice, book)))
	}
	if g := e.call("GET", get(book), alice, "").data(); g["version"] != float64(1) || !strings.Contains(raw(e.call("GET", get(book), alice, ""), "layout"), `"profile"`) {
		t.Errorf("get: %v", g)
	}
	e.call("POST", "/api/book-layout/reset", alice, reset(book)).status(200, "OK")
	e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 2, "profile", "shipped", tpl.ID)).status(200, "OK")
}

// An own template applies to another book with the same book template; the images it never had stay out; a dropped asset or image elsewhere is counted.
func TestApplyOwnAndDropped(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	src, dst := e.book(alice, "memory-book"), e.book(alice, "memory-book")
	id := e.ownTemplate(alice, src, 0, "ownpg01")
	img := e.image(dst, "design")
	cur := fmt.Sprintf(`{"v":1,"pages":{"cover":{"items":[%s]},"back":{"items":[{"id":"bakimg1","t":"image","media":%q,"x":100,"w":400,"h":300,"cy":500}]}}}`, textAt("dstcov1"), img)
	e.call("POST", "/api/book-layout/save", alice, save(dst, 0, cur)).status(200, "OK")
	e.exec(`DELETE FROM media_tab WHERE public_id = ?`, img) // the image of the back page is gone

	r := e.call("POST", "/api/book-layout/apply-template", alice, applyBody(dst, 1, "cover", "own", id)).status(200, "OK")
	if r.data()["dropped"] != float64(1) || r.data()["version"] != float64(2) {
		t.Fatalf("apply: %.300s", r.Body)
	}
	stored := raw(e.call("GET", get(dst), alice, ""), "layout")
	if !strings.Contains(stored, "ownpg01") || strings.Contains(stored, "dstcov1") || strings.Contains(stored, "bakimg1") {
		t.Errorf("stored: %s", stored)
	}
	if e.seen("apply:own:ok") != 1 {
		t.Error("apply not counted")
	}
}

// AC3: a failed apply (stale version, wrong template, invalid page, wrong kind, unknown or foreign template) changes nothing: not the document, not the history.
func TestFailedApplyChangesNothing(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	book, bobBook := e.book(alice, "memory-book"), e.book(bob, "memory-book")
	own := e.ownTemplate(alice, book, 0, "failt01")
	e.now = e.now.Add(time.Minute)
	e.call("POST", "/api/book-layout/save", alice, save(book, 1, doc(textAt("failt02")))).status(200, "OK") // history: one auto snapshot
	shipped := e.shipped(alice, book, "cover")
	bobOwn := e.ownTemplate(bob, bobBook, 0, "bobtpl1")
	bad := ulid.New(e.now)
	e.exec(`INSERT INTO book_template_tab (public_id, owner_id, name, kind, template_id, layout_json, item_count, bytes, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		bad, e.userID("alice@example.com"), "Broken", "cover", "memory-book", `{"items":[{"id":"abcdef","t":"text","text":"x","x":1,"w":400,"h":100,"cy":500,"bogus":1}]}`, 1, 10, e.now.UnixMilli())
	wrongKind := ulid.New(e.now.Add(time.Millisecond))
	e.exec(`INSERT INTO book_template_tab (public_id, owner_id, name, kind, template_id, layout_json, item_count, bytes, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		wrongKind, e.userID("alice@example.com"), "Back page", "back", "memory-book", `{}`, 0, 2, e.now.UnixMilli())
	other := ulid.New(e.now.Add(2 * time.Millisecond))
	e.exec(`INSERT INTO book_template_tab (public_id, owner_id, name, kind, template_id, layout_json, item_count, bytes, created_at) VALUES (?,?,?,?,?,?,?,?,?)`,
		other, e.userID("alice@example.com"), "Navy", "cover", "navy-classic", `{}`, 0, 2, e.now.UnixMilli())

	before := e.state(alice, book)
	v := 2
	for name, c := range map[string]struct {
		who  user
		body string
		code int
		st   string
	}{
		"stale version":         {alice, applyBody(book, 1, "cover", "own", own), 409, "ERROR_VERSION_CONFLICT"},
		"future version":        {alice, applyBody(book, 9, "cover", "shipped", shipped.ID), 409, "ERROR_VERSION_CONFLICT"},
		"template of navy":      {alice, applyBody(book, v, "cover", "own", other), 409, "ERROR_TEMPLATE_MISMATCH"},
		"invalid page":          {alice, applyBody(book, v, "cover", "own", bad), 400, "ERROR_INVALID_LAYOUT"},
		"template of a kind":    {alice, applyBody(book, v, "cover", "own", wrongKind), 400, "ERROR_PARAM"},
		"shipped of a kind":     {alice, applyBody(book, v, "profile", "shipped", shipped.ID), 400, "ERROR_PARAM"},
		"unknown shipped":       {alice, applyBody(book, v, "cover", "shipped", "no-such-template"), 404, "ERROR_NOT_FOUND"},
		"unknown own":           {alice, applyBody(book, v, "cover", "own", unknownID), 404, "ERROR_NOT_FOUND"},
		"bob's own":             {alice, applyBody(book, v, "cover", "own", bobOwn), 404, "ERROR_NOT_FOUND"},
		"bob on alice's book":   {bob, applyBody(book, v, "cover", "shipped", shipped.ID), 404, "ERROR_NOT_FOUND"},
		"anonymous":             {user{}, applyBody(book, v, "cover", "shipped", shipped.ID), 401, "ERROR_UNAUTHORIZED"},
		"unknown book":          {alice, applyBody(unknownID, v, "cover", "shipped", shipped.ID), 404, "ERROR_NOT_FOUND"},
		"bad source":            {alice, applyBody(book, v, "cover", "other", shipped.ID), 400, "ERROR_PARAM"},
		"bad kind":              {alice, applyBody(book, v, "poster", "shipped", shipped.ID), 400, "ERROR_PARAM"},
		"own id for shipped":    {alice, applyBody(book, v, "cover", "shipped", own), 400, "ERROR_PARAM"},
		"shipped id for own":    {alice, applyBody(book, v, "cover", "own", shipped.ID), 400, "ERROR_PARAM"},
		"negative version":      {alice, applyBody(book, -1, "cover", "shipped", shipped.ID), 400, "ERROR_PARAM"},
		"missing version":       {alice, fmt.Sprintf(`{"yearbook_id":%q,"kind":"cover","source":"shipped","template_id":%q}`, book, shipped.ID), 400, "ERROR_PARAM"},
		"bob applies his own":   {bob, applyBody(book, v, "cover", "own", bobOwn), 404, "ERROR_NOT_FOUND"},
		"bob, alice's own":      {bob, applyBody(bobBook, 1, "cover", "own", own), 404, "ERROR_NOT_FOUND"},
		"bob's book, wrong ver": {bob, applyBody(bobBook, 7, "cover", "own", bobOwn), 409, "ERROR_VERSION_CONFLICT"},
	} {
		rec := e.call("POST", "/api/book-layout/apply-template", c.who, c.body)
		if rec.Code != c.code {
			t.Errorf("%s: %d %.300s", name, rec.Code, rec.Body)
			continue
		}
		rec.status(c.code, c.st)
		if strings.Contains(rec.Body.String(), "failt01") && c.who.cookie != alice.cookie {
			t.Errorf("%s leaks a template", name)
		}
		if c.who.cookie == alice.cookie && e.state(alice, book) != before {
			t.Errorf("%s changed the book", name)
		}
	}
	// the conflict answer carries the stored document and version, the invalid one the problems
	r := e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, 1, "cover", "own", own)).status(409, "ERROR_VERSION_CONFLICT")
	if r.data()["version"] != float64(2) || !strings.Contains(raw(r, "layout"), "failt02") {
		t.Errorf("conflict data: %.300s", r.Body)
	}
	r = e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, v, "cover", "own", bad)).status(400, "ERROR_INVALID_LAYOUT")
	if !strings.Contains(r.Body.String(), `"problems"`) || !strings.Contains(r.Body.String(), "unknown_key") {
		t.Errorf("problems: %.300s", r.Body)
	}
	if e.seen("apply:own:conflict") < 2 || e.seen("apply:own:invalid") < 2 || e.seen("apply:own:mismatch") != 1 || e.seen("snapshot:apply") != 0 {
		t.Errorf("metrics: %v", e.dump())
	}
	dbtest.AssertNoOrphans(t, e.db)
}

func (e *env) dump() map[string]int64 {
	out := map[string]int64{}
	e.history.Range(func(k, v any) bool { out[k.(string)] = v.(*atomic.Int64).Load(); return true })
	return out
}

// An own template made on memory-book is refused by a book that has since moved to navy-classic.
func TestApplyAfterTemplateChange(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	book := e.book(alice, "memory-book")
	own := e.ownTemplate(alice, book, 0, "chang01")
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"title":"Navy now","template_id":"navy-classic"}`, book)).status(200, "OK")
	base := int(e.call("GET", get(book), alice, "").data()["version"].(float64))
	before := e.state(alice, book)
	e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, base, "cover", "own", own)).status(409, "ERROR_TEMPLATE_MISMATCH")
	if e.state(alice, book) != before || e.seen("apply:own:mismatch") != 1 {
		t.Error("a refused apply changed the book or was not counted")
	}
	if len(e.ofSource(alice, book, "", "own")) != 0 { // and the list for the navy book does not offer it
		t.Error("the navy book is offered a memory-book template")
	}
}

// AC2: apply snapshots count towards the 30 kept.
func TestApplyRetentionStaysThirty(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	book := e.book(alice, "memory-book")
	tpl := e.shipped(alice, book, "cover")
	own := e.ownTemplate(alice, book, 0, "reten01")
	base := 1
	for i := range 34 {
		src, id := "shipped", tpl.ID
		if i%2 == 1 {
			src, id = "own", own
		}
		e.now = e.now.Add(time.Second)
		e.call("POST", "/api/book-layout/apply-template", alice, applyBody(book, base, "cover", src, id)).status(200, "OK")
		base++
	}
	vs := e.versions(alice, book)
	if len(vs) != 30 || vs[0].Kind != "apply" || vs[0].Version != 34 || e.count(`SELECT COUNT(*) FROM book_layout_version_tab`) != 30 || e.seen("snapshot:apply") != 34 || e.pruned.Load() != 4 {
		t.Fatalf("%d snapshots, newest %+v, applies %d, pruned %d", len(vs), vs[0], e.seen("snapshot:apply"), e.pruned.Load())
	}
}

// AC3: two parallel applies on one version never both succeed (run with -race; the loop makes the spec's "repeated 20 times").
func TestConcurrentAppliesExactlyOneWins(t *testing.T) {
	e := newEnv(t, nil, 0)
	for round := range 20 {
		alice := e.register(fmt.Sprintf("racer%d@example.com", round)) // a user per round: a user keeps at most 20 yearbooks
		book := e.book(alice, "memory-book")
		e.call("POST", "/api/book-layout/save", alice, save(book, 0, doc(textAt(fmt.Sprintf("race%03d", round))))).status(200, "OK")
		shipped := e.shipped(alice, book, "cover")
		var wg sync.WaitGroup
		var wins, conflicts, other atomic.Int32
		for range 2 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				switch rec := e.do("POST", "/api/book-layout/apply-template", alice, applyBody(book, 1, "cover", "shipped", shipped.ID)); rec.Code {
				case 200:
					wins.Add(1)
				case 409:
					conflicts.Add(1)
				default:
					other.Add(1)
					t.Errorf("status %d: %s", rec.Code, rec.Body)
				}
			}()
		}
		wg.Wait()
		vs := e.versions(alice, book)
		if wins.Load() != 1 || conflicts.Load() != 1 || other.Load() != 0 || len(vs) != 1 || vs[0].Kind != "apply" {
			t.Fatalf("round %d: %d wins, %d conflicts, %d other, history %+v", round, wins.Load(), conflicts.Load(), other.Load(), vs)
		}
		if v := e.call("GET", get(book), alice, "").data()["version"]; v != float64(2) {
			t.Fatalf("round %d: version %v", round, v)
		}
	}
	// an apply racing a save and a reset of the same version: no 5xx
	alice := e.register("mixed@example.com")
	book := e.book(alice, "memory-book")
	e.call("POST", "/api/book-layout/save", alice, save(book, 0, doc(textAt("mixed01")))).status(200, "OK")
	shipped := e.shipped(alice, book, "cover")
	var wg sync.WaitGroup
	var wins atomic.Int32
	for _, f := range []func() int{
		func() int {
			return e.do("POST", "/api/book-layout/apply-template", alice, applyBody(book, 1, "cover", "shipped", shipped.ID)).Code
		},
		func() int {
			return e.do("POST", "/api/book-layout/save", alice, save(book, 1, doc(textAt("mixed02")))).Code
		},
		func() int { return e.do("POST", "/api/book-layout/reset", alice, reset(book)).Code },
	} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if c := f(); c == 200 {
				wins.Add(1)
			} else if c != 409 {
				t.Errorf("status %d", c)
			}
		}()
	}
	wg.Wait()
	if wins.Load() < 1 {
		t.Error("nobody won")
	}
}

// The three routes spend the layout's per-user limits like save.
func TestOwnTemplateRoutesAreRateLimited(t *testing.T) {
	now := func() time.Time { return t0 }
	e := newEnv(t, ratelimit.NewMemoryFactory(now), 1000)
	alice := e.register("alice@example.com")
	book := e.book(alice, "memory-book")
	for range booklayout.Burst {
		e.call("POST", "/api/book-layout/template-delete", alice, tplDelete(unknownID)).status(404, "ERROR_NOT_FOUND")
	}
	for path, body := range map[string]string{
		"template-delete": tplDelete(unknownID),
		"template-create": tplCreate(book, "cover", "x"),
		"apply-template":  applyBody(book, 0, "cover", "shipped", "cover-sunny"),
	} {
		r := e.call("POST", "/api/book-layout/"+path, alice, body).status(429, "ERROR_RATE_LIMITED")
		if r.Header().Get("Retry-After") == "" {
			t.Errorf("%s: no Retry-After", path)
		}
	}
}
