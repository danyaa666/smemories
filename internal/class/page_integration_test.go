//go:build integration

package class

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/ulid"
)

// The student's page (T-126): the link between a member and one of their own yearbooks, and the proof that it gives nobody a
// right over that yearbook.

func (e *env) newBook(p person) string {
	e.t.Helper()
	return e.do(p, "POST", "/api/yearbook/create", `{"title":"Mine","language":"en"}`).status(200, "OK").data()["id"].(string)
}

// setBook calls class-member/set-book; book is a yearbook id, or nil to unlink.
func (e *env) setBook(p person, cls string, book any) resp {
	e.t.Helper()
	b, _ := json.Marshal(map[string]any{"class_id": cls, "yearbook_id": book})
	return e.do(p, "POST", "/api/class-member/set-book", string(b))
}

func (e *env) linked(p person, cls string) bool {
	e.t.Helper()
	return e.do(p, "GET", "/api/class/get?id="+cls, "").status(200, "OK").data()["page"].(map[string]any)["linked"] == true
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	slices.Sort(ks)
	return ks
}

func wantKeys(t *testing.T, what string, got map[string]any, want ...string) {
	t.Helper()
	slices.Sort(want)
	if g := sortedKeys(got); !slices.Equal(g, want) {
		t.Fatalf("%s has keys %v, want exactly %v", what, g, want)
	}
}

// AC1: only the member sets or clears their own link, and only to a yearbook they own.
func TestSetBookMatrix(t *testing.T) {
	e := newEnv(t)
	admin, student, classmate, stranger, outsider := e.user("Admin"), e.user("Student"), e.user("Classmate"), e.user("Stranger"), e.user("Outsider")
	cls, _ := e.seedClass([]person{admin}, student, classmate)
	e.seedClass([]person{outsider})
	sb, ab, cb, ob := e.newBook(student), e.newBook(admin), e.newBook(classmate), e.newBook(outsider)
	noLink := func(who ...person) {
		t.Helper()
		if n := e.count(`SELECT COUNT(*) FROM class_member_tab WHERE yearbook_id IS NOT NULL`); n != len(who) {
			t.Fatalf("%d links in the database, want %d", n, len(who))
		}
	}

	e.setBook(anonymous, cls, sb).status(401, "ERROR_UNAUTHORIZED")
	// a non-member, and a member of another class, get the answer for "no such class", whichever yearbook they send
	gone := e.setBook(stranger, "01K7Q8W3ZR5N2M4X6V8B0C1D2E", nil).status(404, "ERROR_NOT_FOUND").sameAnswer()
	for who, book := range map[string]struct {
		p person
		b string
	}{"stranger": {stranger, e.newBook(stranger)}, "stranger with a member's book": {stranger, sb}, "outsider": {outsider, ob}} {
		if got := e.setBook(book.p, cls, book.b).status(404, "ERROR_NOT_FOUND").sameAnswer(); got != gone {
			t.Fatalf("%s: answer differs from \"no such class\": %s", who, got)
		}
	}
	noLink()

	// a yearbook that is somebody else's, or does not exist, is the same 404, and links nothing
	missing := e.setBook(student, cls, "01K7Q8W3ZR5N2M4X6V8B0C1D2E").status(404, "ERROR_NOT_FOUND").sameAnswer()
	for who, b := range map[string]string{"admin's book": ab, "classmate's book": cb, "outsider's book": ob} {
		if got := e.setBook(student, cls, b).status(404, "ERROR_NOT_FOUND").sameAnswer(); got != missing {
			t.Fatalf("%s: answer differs from a missing book: %s", who, got)
		}
	}
	noLink()

	// the admin cannot set a student's link: there is no way to name another member, and the admin's own link is their own
	e.do(admin, "POST", "/api/class-member/set-book", body("class_id", cls, "yearbook_id", sb, "member_id", "01K7Q8W3ZR5N2M4X6V8B0C1D2G")).status(400, "ERROR_PARAM")
	e.setBook(admin, cls, sb).status(404, "ERROR_NOT_FOUND") // not the admin's book
	noLink()

	// the member links their own book; the answer and class/get show the same status
	d := e.setBook(student, cls, sb).status(200, "OK").data()
	wantKeys(t, "set-book answer", d, "linked", "yearbook_id", "profile_complete", "approved_notes")
	if d["linked"] != true || d["yearbook_id"] != sb || d["profile_complete"] != false || d["approved_notes"] != float64(0) {
		t.Fatalf("set-book answer: %v", d)
	}
	if got := e.do(student, "GET", "/api/class/get?id="+cls, "").status(200, "OK").data()["page"]; got.(map[string]any)["yearbook_id"] != sb {
		t.Fatalf("class/get page: %v", got)
	}
	e.setBook(student, cls, sb).status(200, "OK") // idempotent
	e.setBook(admin, cls, ab).status(200, "OK")   // an admin links their own book like anyone
	noLink(student, admin)
	// the admin sees nobody's page status but their own
	if got := e.do(admin, "GET", "/api/class/get?id="+cls, "").data()["page"].(map[string]any)["yearbook_id"]; got != ab {
		t.Fatalf("the admin's class/get shows %v", got)
	}

	// replacing, clearing; absent is refused (it would silently unlink)
	sb2 := e.newBook(student)
	e.setBook(student, cls, sb2).status(200, "OK")
	if got := e.count(`SELECT COUNT(*) FROM class_member_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE y.public_id = ?`, sb2); got != 1 {
		t.Fatal("the link was not replaced")
	}
	e.do(student, "POST", "/api/class-member/set-book", body("class_id", cls)).status(400, "ERROR_PARAM")
	e.do(student, "POST", "/api/class-member/set-book", `{"class_id":"`+cls+`","yearbook_id":5}`).status(400, "ERROR_PARAM")
	e.do(student, "POST", "/api/class-member/set-book", body("class_id", cls, "yearbook_id", strings.ToLower(sb))).status(400, "ERROR_PARAM")
	e.do(student, "POST", "/api/class-member/set-book", body("class_id", cls, "yearbook_id", "")).status(400, "ERROR_PARAM")
	e.do(student, "POST", "/api/class-member/set-book", body("class_id", "x", "yearbook_id", sb)).status(400, "ERROR_PARAM")
	if !e.linked(student, cls) {
		t.Fatal("a refused request changed the link")
	}
	e.setBook(student, cls, nil).status(200, "OK")
	if e.linked(student, cls) {
		t.Fatal("not cleared")
	}
	e.setBook(student, cls, nil).status(200, "OK") // clearing twice is fine
	if got := e.do(student, "GET", "/api/class/get?id="+cls, "").data()["page"].(map[string]any); got["yearbook_id"] != nil {
		t.Fatalf("cleared page: %v", got)
	}
	noLink(admin)
	// the yearbook itself is untouched by all of it
	e.do(student, "GET", "/api/yearbook/get?id="+sb, "").status(200, "OK")
}

// A member removed from the class cannot link into it; leaving or being removed keeps the yearbook.
func TestSetBookAfterLeavingAndKeepsTheBook(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, members := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	e.setBook(student, cls, book).status(200, "OK")
	e.do(admin, "POST", "/api/class-member/remove", body("class_id", cls, "member_id", members[student.row])).status(200, "OK")
	e.do(student, "GET", "/api/yearbook/get?id="+book, "").status(200, "OK")
	e.setBook(student, cls, book).status(404, "ERROR_NOT_FOUND")
	e.setBook(student, cls, nil).status(404, "ERROR_NOT_FOUND")
	if n := e.count(`SELECT COUNT(*) FROM class_member_tab WHERE yearbook_id IS NOT NULL`); n != 0 {
		t.Fatalf("%d links after the member was removed", n)
	}
}

// AC2: a yearbook belongs to at most one class; deleting it clears the link; link and delete racing leave nothing dangling.
func TestOneYearbookOneClass(t *testing.T) {
	e := newEnv(t)
	admin1, admin2, student := e.user("Admin1"), e.user("Admin2"), e.user("Student")
	c1, _ := e.seedClass([]person{admin1}, student)
	c2, _ := e.seedClass([]person{admin2}, student)
	book := e.newBook(student)
	e.setBook(student, c1, book).status(200, "OK")
	e.setBook(student, c2, book).status(409, "ERROR_CONFLICT")
	if !e.linked(student, c1) || e.linked(student, c2) {
		t.Fatal("the refused link changed something")
	}
	e.setBook(student, c1, nil).status(200, "OK")
	e.setBook(student, c2, book).status(200, "OK")
	e.setBook(student, c1, book).status(409, "ERROR_CONFLICT")
	// a second book is fine in the other class
	e.setBook(student, c1, e.newBook(student)).status(200, "OK")
}

func TestSameYearbookInTwoClassesAtOnce(t *testing.T) {
	e := newEnv(t)
	a1, a2, student := e.user("Admin1"), e.user("Admin2"), e.user("Student")
	c1, _ := e.seedClass([]person{a1}, student)
	c2, _ := e.seedClass([]person{a2}, student)
	for round := range 8 {
		book := e.newBook(student)
		codes := make([]int, 2)
		var wg sync.WaitGroup
		for i, c := range []string{c1, c2} {
			wg.Add(1)
			go func() { defer wg.Done(); codes[i] = e.setBook(student, c, book).Code }()
		}
		wg.Wait()
		slices.Sort(codes)
		if codes[0] != 200 || codes[1] != 409 {
			t.Fatalf("round %d: answers %v, want one 200 and one 409", round, codes)
		}
		if n := e.count(`SELECT COUNT(*) FROM class_member_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE y.public_id = ?`, book); n != 1 {
			t.Fatalf("round %d: the book is the page of %d classes", round, n)
		}
		e.exec(`UPDATE class_member_tab SET yearbook_id = NULL WHERE user_id = ?`, student.row)
	}
}

func TestDeletingTheYearbookClearsTheLinkAndLinkRacingDeleteDangles(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, _ := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	e.setBook(student, cls, book).status(200, "OK")
	e.do(student, "POST", "/api/yearbook/delete", body("id", book)).status(200, "OK")
	if e.linked(student, cls) {
		t.Fatal("the link survived the delete")
	}
	e.setBook(student, cls, book).status(404, "ERROR_NOT_FOUND") // a deleted book cannot be linked
	if n := e.count(`SELECT COUNT(*) FROM class_member_tab WHERE user_id = ?`, student.row); n != 1 {
		t.Fatal("the membership went with the yearbook")
	}

	// link and delete at the same moment: whichever wins, no member row points at a missing yearbook
	for round := range 12 {
		b := e.newBook(student)
		var wg sync.WaitGroup
		wg.Add(2)
		var linkCode, delCode int
		go func() { defer wg.Done(); linkCode = e.setBook(student, cls, b).Code }()
		go func() {
			defer wg.Done()
			delCode = e.do(student, "POST", "/api/yearbook/delete", body("id", b)).Code
		}()
		wg.Wait()
		if delCode != 200 || (linkCode != 200 && linkCode != 404) {
			t.Fatalf("round %d: link %d, delete %d", round, linkCode, delCode)
		}
		if n := e.count(`SELECT COUNT(*) FROM class_member_tab m LEFT JOIN yearbook_tab y ON y.id = m.yearbook_id WHERE m.yearbook_id IS NOT NULL AND y.id IS NULL`); n != 0 {
			t.Fatalf("round %d: %d dangling links", round, n)
		}
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// seedPage gives the book an owner photo (a profile is complete with a name and a photo) and notes of every status, each with
// text that must never reach an admin.
func (e *env) seedPage(book string, approved, pending, hidden int) {
	e.t.Helper()
	var bookRow uint64
	if err := e.db.QueryRow(`SELECT id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&bookRow); err != nil {
		e.t.Fatal(err)
	}
	now := e.now().UnixMilli()
	h := sha256.Sum256([]byte(book))
	e.exec(`UPDATE profile_tab SET photo_media_id = 1, quote = 'SECRET-QUOTE' WHERE yearbook_id = ?`, bookRow)
	res, err := e.db.Exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		ulid.New(e.now()), bookRow, h[:], "SECRET-LABEL", now, now)
	if err != nil {
		e.t.Fatal(err)
	}
	coll, _ := res.LastInsertId()
	add := func(n int, status string) {
		for range n {
			e.mu.Lock()
			e.n++
			k := e.n
			e.mu.Unlock()
			e.exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, sort_order, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
				ulid.New(e.now()), coll, bookRow, `{"message":"SECRET-NOTE-TEXT-`+status+`"}`, status, k, now, now)
		}
	}
	add(approved, "approved")
	add(pending, "pending")
	add(hidden, "hidden")
}

// AC3: the roster has counts and flags only, and the exact keys are asserted; a student sees only their own page status.
func TestRosterHasCountsAndFlagsOnly(t *testing.T) {
	e := newEnv(t)
	admin, student, bare, loner := e.user("Admin"), e.user("Student"), e.user("Bare"), e.user("Loner")
	cls, _ := e.seedClass([]person{admin}, student, bare, loner)
	sb, bb := e.newBook(student), e.newBook(bare)
	e.seedPage(sb, 2, 3, 1)
	e.setBook(student, cls, sb).status(200, "OK")
	e.setBook(bare, cls, bb).status(200, "OK") // a fresh book: a profile with a name only, no photo, no notes
	// a link to a yearbook the member does not own (the API cannot make one) counts for nothing
	foreign := e.newBook(admin)
	e.exec(`UPDATE class_member_tab SET yearbook_id = (SELECT id FROM yearbook_tab WHERE public_id = ?) WHERE user_id = ?`, foreign, loner.row)
	e.seedPage(foreign, 5, 0, 0)

	r := e.do(admin, "GET", "/api/class-member/get-list?class_id="+cls, "").status(200, "OK")
	items := r.data()["items"].([]any)
	if len(items) != 4 {
		t.Fatalf("%d members", len(items))
	}
	byName := map[string]map[string]any{}
	for _, it := range items {
		m := it.(map[string]any)
		wantKeys(t, "roster row", m, "id", "name", "role", "joined_at", "page_linked", "profile_complete", "approved_notes")
		byName[m["name"].(string)] = m
	}
	for name, want := range map[string][3]any{
		"Student": {true, true, float64(2)}, "Bare": {true, false, float64(0)}, "Loner": {false, false, float64(0)}, "Admin": {false, false, float64(0)},
	} {
		m := byName[name]
		if m["page_linked"] != want[0] || m["profile_complete"] != want[1] || m["approved_notes"] != want[2] {
			t.Errorf("%s: %v, want linked %v complete %v approved %v", name, m, want[0], want[1], want[2])
		}
	}
	for _, leak := range []string{"SECRET", "pending", "hidden", "yearbook", sb, bb, foreign, "quote", "photo", "token"} {
		if strings.Contains(r.Body.String(), leak) {
			t.Errorf("roster leaks %q: %s", leak, r.Body)
		}
	}
	// the student's own status, and no other member's
	d := e.do(student, "GET", "/api/class/get?id="+cls, "").status(200, "OK").data()
	wantKeys(t, "class page", d["page"].(map[string]any), "linked", "yearbook_id", "profile_complete", "approved_notes")
	if p := d["page"].(map[string]any); p["linked"] != true || p["yearbook_id"] != sb || p["profile_complete"] != true || p["approved_notes"] != float64(2) {
		t.Fatalf("student's page: %v", p)
	}
	// the list of classes and the other class answers carry no page object
	for _, it := range e.do(student, "GET", "/api/class/get-list", "").data()["items"].([]any) {
		if _, has := it.(map[string]any)["page"]; has {
			t.Fatal("class/get-list carries page")
		}
	}
	// a page of 60: the page flags cost the same few queries whatever the size is, and stay correct
	big, _ := e.seedClass([]person{admin})
	e.fill(big, 40)
	if n := len(e.do(admin, "GET", "/api/class-member/get-list?class_id="+big, "").data()["items"].([]any)); n != 41 {
		t.Fatalf("%d members", n)
	}
}

// adminCalls is the admin-404 matrix: every route of api/openapi.yaml that takes a yearbook, note or media id, called as the class admin
// on a student's linked book. The key is the route (without /api/); a suffix after a space tells variants of one route apart.
// TestAdminRoutesAreAllInTheMatrix fails when a route of the OpenAPI file that takes such an id is missing from this table.
func (e *env) adminCalls(admin person, cls, book, coll, note, media string) map[string]func() resp {
	return map[string]func() resp{
		"yearbook/get": func() resp { return e.do(admin, "GET", "/api/yearbook/get?id="+book, "") },
		"yearbook/update": func() resp {
			return e.do(admin, "POST", "/api/yearbook/update", body("id", book, "title", "Hijacked", "language", "en", "page_size", "A5"))
		},
		"yearbook/update-profile": func() resp {
			return e.do(admin, "POST", "/api/yearbook/update-profile", body("yearbook_id", book, "full_name", "Hijacked"))
		},
		"yearbook/revoke-photo-links": func() resp { return e.do(admin, "POST", "/api/yearbook/revoke-photo-links", body("id", book)) },
		"yearbook/delete":             func() resp { return e.do(admin, "POST", "/api/yearbook/delete", body("id", book)) },
		"note-collection/create": func() resp {
			return e.do(admin, "POST", "/api/note-collection/create", body("yearbook_id", book, "label", "mine"))
		},
		"note-collection/get-list": func() resp { return e.do(admin, "GET", "/api/note-collection/get-list?yearbook_id="+book, "") },
		"note-collection/revoke":   func() resp { return e.do(admin, "POST", "/api/note-collection/revoke", body("id", coll)) },
		"note/get-list":            func() resp { return e.do(admin, "GET", "/api/note/get-list?yearbook_id="+book+"&status=pending", "") },
		"note/get-list approved":   func() resp { return e.do(admin, "GET", "/api/note/get-list?yearbook_id="+book+"&status=approved", "") },
		"note/batch-update-status": func() resp {
			return e.do(admin, "POST", "/api/note/batch-update-status", `{"yearbook_id":"`+book+`","ids":["`+note+`"],"status":"approved"}`)
		},
		"note/reorder": func() resp {
			return e.do(admin, "POST", "/api/note/reorder", `{"yearbook_id":"`+book+`","ids":["`+note+`"]}`)
		},
		"note/batch-delete": func() resp {
			return e.do(admin, "POST", "/api/note/batch-delete", `{"yearbook_id":"`+book+`","ids":["`+note+`"]}`)
		},
		"media/create":          func() resp { return e.upload(admin, book) },
		"media/get-list":        func() resp { return e.do(admin, "GET", "/api/media/get-list?yearbook_id="+book, "") },
		"media/get-content":     func() resp { return e.do(admin, "GET", "/api/media/get-content?id="+media, "") },
		"media/delete":          func() resp { return e.do(admin, "POST", "/api/media/delete", body("id", media)) },
		"class-member/set-book": func() resp { return e.setBook(admin, cls, book) }, // the admin cannot link a student's book as their own page
		"book-data/get":         func() resp { return e.do(admin, "GET", "/api/book-data/get?yearbook_id="+book, "") },
		"book-design/get":       func() resp { return e.do(admin, "GET", "/api/book-design/get?yearbook_id="+book, "") },
		"book-design/update": func() resp {
			return e.do(admin, "POST", "/api/book-design/update", `{"yearbook_id":"`+book+`","options":{"theme":"mint"}}`)
		},
		"book-design/reset": func() resp { return e.do(admin, "POST", "/api/book-design/reset", body("yearbook_id", book)) },
		"book-layout/get":   func() resp { return e.do(admin, "GET", "/api/book-layout/get?yearbook_id="+book, "") },
		"book-layout/save": func() resp {
			return e.do(admin, "POST", "/api/book-layout/save", `{"yearbook_id":"`+book+`","base_version":0,"layout":{"v":1,"pages":{}}}`)
		},
		"book-layout/reset": func() resp { return e.do(admin, "POST", "/api/book-layout/reset", body("yearbook_id", book)) },
		"book-export/get-text": func() resp {
			return e.do(admin, "GET", "/api/book-export/get-text?yearbook_id="+book, "")
		},
	}
}

// AC4: linking changes no permission. The admin of the class gets 404 on every endpoint of the student's yearbook, notes and
// photos (media/delete answers 200 and does nothing, by design), with the book linked, and nothing of the student's changes.
func TestAdminStillHasNoRightsOverAStudentsBook(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, _ := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	e.seedPage(book, 1, 1, 1)
	e.setBook(student, cls, book).status(200, "OK")
	var bookRow uint64
	_ = e.db.QueryRow(`SELECT id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&bookRow)
	now := e.now().UnixMilli()
	media := ulid.New(e.now())
	e.exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		VALUES (?,?,'contributor',?,?,?,'image/jpeg',1,1,1,?,?,?)`, media, bookRow, "k/"+media, "t/"+media, "p/"+media, strings.Repeat("s", 32), now, now)
	var coll, note string
	_ = e.db.QueryRow(`SELECT public_id FROM note_collection_tab WHERE yearbook_id = ?`, bookRow).Scan(&coll)
	_ = e.db.QueryRow(`SELECT public_id FROM note_tab WHERE yearbook_id = ? AND status = 'pending'`, bookRow).Scan(&note)
	before := func() string {
		var s string
		for _, q := range []string{
			`SELECT CONCAT_WS('|', title, updated_at) FROM yearbook_tab WHERE id = ?`,
			`SELECT CONCAT_WS('|', full_name, quote, updated_at) FROM profile_tab WHERE yearbook_id = ?`,
			`SELECT GROUP_CONCAT(CONCAT_WS(',', public_id, status, sort_order) ORDER BY id) FROM note_tab WHERE yearbook_id = ?`,
			`SELECT GROUP_CONCAT(CONCAT_WS(',', public_id, revoked_at) ORDER BY id) FROM note_collection_tab WHERE yearbook_id = ?`,
			`SELECT COUNT(*) FROM media_tab WHERE yearbook_id = ?`,
			`SELECT GROUP_CONCAT(CONCAT_WS(',', version, updated_at, layout_json) ORDER BY id) FROM book_layout_tab WHERE yearbook_id = ?`, // T-153
		} {
			var v sql.NullString
			if err := e.db.QueryRow(q, bookRow).Scan(&v); err != nil {
				t.Fatal(err)
			}
			s += v.String + "\n"
		}
		return s
	}
	e.exec(`UPDATE yearbook_tab SET template_id = 'memory-book' WHERE id = ?`, bookRow)
	e.exec(`INSERT INTO book_layout_tab (yearbook_id, template_id, layout_json, version, created_at, updated_at) VALUES (?, 'memory-book', ?, 3, 1, 1)`,
		bookRow, `{"v":1,"pages":{"cover":{"items":[{"id":"abcdef","t":"text","text":"SECRET-LAYOUT","x":0,"w":100,"h":100,"cy":500}]}}}`)
	snapshot := before()

	calls := e.adminCalls(admin, cls, book, coll, note, media)
	// the answer a user gets for a yearbook that does not exist: the admin's answers must be the same kind (404)
	for name, call := range calls {
		r := call()
		if name == "media/delete" { // by design a silent no-op for a foreign id (media.Service.Delete), so repeating it is harmless; the snapshot proves nothing was deleted
			r.status(200, "OK")
		} else if r.Code != 404 || r.code() != "ERROR_NOT_FOUND" {
			t.Errorf("%s: admin got %d %s, want 404 ERROR_NOT_FOUND", name, r.Code, r.Body)
		}
		for _, secret := range []string{"SECRET", "Student"} {
			if strings.Contains(r.Body.String(), secret) {
				t.Errorf("%s: the answer leaks %q: %s", name, secret, r.Body)
			}
		}
	}
	if after := before(); after != snapshot {
		t.Fatalf("the admin's calls changed the student's book:\n%s\n%s", snapshot, after)
	}
	e.do(student, "GET", "/api/yearbook/get?id="+book, "").status(200, "OK") // and the owner still has it
	// media/create above was a well-formed upload: the owner's identical request works, so the admin's 404 was the ownership check
	e.upload(student, book).status(200, "OK")
	if !e.linked(student, cls) || e.linked(admin, cls) {
		t.Fatal("the admin's set-book changed a link")
	}
}

// The carried-over budget (T-125 QA): a whole class on one school network (one NAT address) joins without hitting the per-IP
// limits, with room for refreshes.
func TestSixtyStudentsJoinFromOneAddress(t *testing.T) {
	e := newEnv(t)
	adminA, adminB := e.user("AdminA"), e.user("AdminB")
	clsA, clsB := e.create(adminA), e.create(adminB)
	_, tokA := e.createInvite(adminA, clsA)
	_, tokB := e.createInvite(adminB, clsB)
	for i := range 60 { // every request of the test comes from the same RemoteAddr
		p := e.user("Kid" + strconv.Itoa(i))
		tok, cls := tokA, clsA
		if i%2 == 1 {
			tok, cls = tokB, clsB
		}
		e.lookup(tok).status(200, "OK")
		e.lookup(tok).status(200, "OK") // a refresh
		e.accept(p, tok).status(200, "OK")
		e.accept(p, tok).status(200, "OK") // a second attempt is idempotent
		if e.role(p, cls) != RoleStudent {
			t.Fatalf("student %d did not join", i)
		}
	}
	if e.members(clsA) != 31 || e.members(clsB) != 31 {
		t.Fatalf("members: %d and %d, want 31 each", e.members(clsA), e.members(clsB))
	}
}
