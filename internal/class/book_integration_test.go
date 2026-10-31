//go:build integration

package class

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/bookdesign"
	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis"
	"github.com/danyaa666/smemories/internal/redis/redistest"
	"github.com/danyaa666/smemories/internal/ulid"
	"github.com/danyaa666/smemories/internal/yearbook"
)

// The class book (T-027): what the class admin reads, and the one read path into a member's photos. The privacy boundary comes first.

const missingID = "01K7Q8W3ZR5N2M4X6V8B0C1D2E"

// ---- fixtures -------------------------------------------------------------------------------------------------------------

func (e *env) bookRow(book string) uint64 {
	e.t.Helper()
	var id uint64
	if err := e.db.QueryRow(`SELECT id FROM yearbook_tab WHERE public_id = ?`, book).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// photo stores a photo of the book (a media row and its three objects) and returns its public id. The bytes name the photo and the variant.
func (e *env) photo(book string) string {
	e.t.Helper()
	pub := ulid.New(e.now())
	now := e.now().UnixMilli()
	e.exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		VALUES (?,?,'contributor',?,?,?,'image/jpeg',1,1,1,?,?,?)`, pub, e.bookRow(book), "k/"+pub, "t/"+pub, "p/"+pub, strings.Repeat("s", 32), now, now)
	for _, k := range []string{"k/", "t/", "p/"} {
		if err := e.store.Put(context.Background(), k+pub, "image/jpeg", []byte(k+pub)); err != nil {
			e.t.Fatal(err)
		}
	}
	return pub
}

// design is a design image (T-152: a background or decoration of the student's layout) in the book.
func (e *env) designImage(book string) string {
	e.t.Helper()
	id := e.photo(book)
	e.exec(`UPDATE media_tab SET purpose = 'design', uploader_kind = 'owner' WHERE public_id = ?`, id)
	return id
}

func (e *env) collectionOf(book string) uint64 {
	e.t.Helper()
	var id uint64
	err := e.db.QueryRow(`SELECT id FROM note_collection_tab WHERE yearbook_id = ? ORDER BY id LIMIT 1`, e.bookRow(book)).Scan(&id)
	if err == nil {
		return id
	}
	now := e.now().UnixMilli()
	pub := ulid.New(e.now())
	h := sha256.Sum256([]byte(pub))
	res, err := e.db.Exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
		pub, e.bookRow(book), h[:], "SECRET-LABEL", now, now)
	if err != nil {
		e.t.Fatal(err)
	}
	n, _ := res.LastInsertId()
	return uint64(n) //nolint:gosec // G115: AUTO_INCREMENT ids are positive
}

// note adds a note of the status (approved, pending or hidden) at the position sort, with the photos attached; it returns its public id.
func (e *env) note(book, status string, sort int, text string, photos ...string) string {
	e.t.Helper()
	pub := ulid.New(e.now())
	now := e.now().UnixMilli()
	a, _ := json.Marshal(map[string]string{"name": "Friend", "message": text})
	res, err := e.db.Exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, sort_order, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
		pub, e.collectionOf(book), e.bookRow(book), string(a), status, sort, now, now)
	if err != nil {
		e.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	for i, ph := range photos {
		e.exec(`INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) SELECT ?, id, ?, 1, 1 FROM media_tab WHERE public_id = ?`, id, i, ph)
	}
	return pub
}

// profile gives the book's owner profile a name and, when photo is not empty, that photo.
func (e *env) profile(book, name, photo string) {
	e.t.Helper()
	e.exec(`UPDATE profile_tab SET full_name = ?, photo_media_id = (SELECT id FROM media_tab WHERE public_id = ?) WHERE yearbook_id = ? AND is_owner = 1`, name, photo, e.bookRow(book))
}

// design stores a design row for the book (as book-design/update would), and gives the book that template.
func (e *env) design(book, template, options string) {
	e.t.Helper()
	id := e.bookRow(book)
	e.exec(`UPDATE yearbook_tab SET template_id = ? WHERE id = ?`, template, id)
	e.exec(`INSERT INTO book_design_tab (yearbook_id, template_id, options_json, created_at, updated_at) VALUES (?,?,?,1,1)`, id, template, options)
}

func (e *env) classBook(p person, cls string) resp {
	e.t.Helper()
	return e.do(p, "GET", "/api/class-book/get?class_id="+cls, "")
}

// photoStatus calls media/get-content and returns the status code and the body.
func (e *env) photoStatus(p person, id, variant string) (int, string) {
	e.t.Helper()
	r := e.do(p, "GET", "/api/media/get-content?id="+id+"&variant="+variant, "")
	return r.Code, r.Body.String()
}

func (e *env) wantPhoto(p person, id string) {
	e.t.Helper()
	for variant, prefix := range map[string]string{"display": "k/", "thumb": "t/", "print": "p/"} {
		if code, b := e.photoStatus(p, id, variant); code != 200 || b != prefix+id {
			e.t.Fatalf("%s of %s: %d %q, want the bytes", variant, id, code, b)
		}
	}
}

// wantNoPhoto: 404 ERROR_NOT_FOUND, byte for byte the answer for a photo that does not exist.
func (e *env) wantNoPhoto(p person, id, what string) {
	e.t.Helper()
	gone := e.do(p, "GET", "/api/media/get-content?id="+missingID, "").status(404, "ERROR_NOT_FOUND").sameAnswer()
	for _, variant := range []string{"display", "thumb", "print"} {
		r := e.do(p, "GET", "/api/media/get-content?id="+id+"&variant="+variant, "")
		if r.Code != 404 || r.sameAnswer() != gone {
			e.t.Fatalf("%s (%s): %d %s, want the answer for a missing photo", what, variant, r.Code, r.Body)
		}
	}
}

func (e *env) pngBytes() []byte {
	var b bytes.Buffer
	_ = png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 2, 2)))
	return b.Bytes()
}

// upload is media/create of a valid image to the book.
func (e *env) upload(p person, book string) resp {
	e.t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("yearbook_id", book)
	f, _ := w.CreateFormFile("file", "x.png")
	_, _ = f.Write(e.pngBytes())
	_ = w.Close()
	r := httptest.NewRequest("POST", "/api/media/create", &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	r.AddCookie(&http.Cookie{Name: auth.CookieName, Value: p.cookie})
	r.Header.Set("Origin", origin)
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return resp{rec, e.t}
}

// ---- AC1: the matrix ------------------------------------------------------------------------------------------------------

func TestClassBookMatrix(t *testing.T) {
	e := newEnv(t)
	admin, student, classmate, stranger, outsider := e.user("Admin"), e.user("Student"), e.user("Classmate"), e.user("Stranger"), e.user("Outsider")
	cls, _ := e.seedClass([]person{admin}, student, classmate)
	e.seedClass([]person{outsider})
	book := e.newBook(student)
	pp := e.photo(book)
	e.profile(book, "Linh", pp)
	ap := e.photo(book)
	e.note(book, "approved", 1, "hello", ap)
	e.setBook(student, cls, book).status(200, "OK")
	e.design(book, "navy-classic", `{"v":1,"theme":"burgundy","texts":{"profile.heading":"DESIGN-TEXT"}}`)

	r := e.classBook(admin, cls).status(200, "OK")
	if !strings.Contains(r.Body.String(), "DESIGN-TEXT") {
		t.Fatalf("the admin does not see the design: %s", r.Body)
	}
	for who, p := range map[string]person{"student": student, "classmate": classmate, "stranger": stranger, "admin of another class": outsider} {
		if strings.Contains(e.classBook(p, cls).Body.String(), "DESIGN-TEXT") {
			t.Fatalf("%s reads the design through class-book/get", who)
		}
	}
	if !strings.Contains(r.Body.String(), ap) || !strings.Contains(r.Body.String(), pp) {
		t.Fatalf("the admin does not see the photos: %s", r.Body)
	}
	gone := e.classBook(stranger, missingID).status(404, "ERROR_NOT_FOUND").sameAnswer()
	e.classBook(student, cls).status(403, "ERROR_FORBIDDEN")
	e.classBook(classmate, cls).status(403, "ERROR_FORBIDDEN")
	for who, p := range map[string]person{"stranger": stranger, "admin of another class": outsider} {
		if got := e.classBook(p, cls).status(404, "ERROR_NOT_FOUND").sameAnswer(); got != gone {
			t.Fatalf("%s: answer differs from \"no such class\": %s", who, got)
		}
	}
	e.classBook(admin, missingID).status(404, "ERROR_NOT_FOUND")
	e.classBook(anonymous, cls).status(401, "ERROR_UNAUTHORIZED")
	e.do(admin, "GET", "/api/class-book/get", "").status(400, "ERROR_PARAM")
	e.do(admin, "GET", "/api/class-book/get?class_id=nope", "").status(400, "ERROR_PARAM")
	e.do(admin, "GET", "/api/class-book/get?class_id="+strings.ToLower(cls), "").status(400, "ERROR_PARAM")
	// a write verb does not exist
	if r := e.do(admin, "POST", "/api/class-book/get", body("class_id", cls)); r.Code != http.StatusMethodNotAllowed && r.Code != http.StatusNotFound {
		t.Fatalf("POST class-book/get: %d", r.Code)
	}

	// every photo id of the book: nobody but the owner and this class's admin may open it
	for _, id := range []string{pp, ap} {
		for who, p := range map[string]person{"classmate": classmate, "stranger": stranger, "admin of another class": outsider} {
			e.wantNoPhoto(p, id, who)
		}
		e.do(anonymous, "GET", "/api/media/get-content?id="+id, "").status(401, "ERROR_UNAUTHORIZED")
	}
}

// ---- the content ----------------------------------------------------------------------------------------------------------

// AC1/AC2 (T-027 requirements 1 and 2): approved content only, in the student's order, nothing else.
func TestClassBookHasApprovedContentOnly(t *testing.T) {
	e := newEnv(t)
	admin, linh, minh, bare, empty, ghost := e.user("Admin"), e.user("Linh"), e.user("Minh"), e.user("Bare"), e.user("Empty"), e.user("Ghost")
	cls, members := e.seedClass([]person{admin}, linh, minh, bare, empty, ghost)
	// the book order: sort_order first, then joining order
	e.exec(`UPDATE class_member_tab SET sort_order = 5 WHERE public_id = ?`, members[linh.row])
	e.exec(`UPDATE class_member_tab SET sort_order = 2 WHERE public_id = ?`, members[minh.row])

	lb, mb, eb := e.newBook(linh), e.newBook(minh), e.newBook(empty)
	pp := e.photo(lb)
	e.profile(lb, "Linh Nguyen", pp)
	e.exec(`UPDATE profile_tab SET quote = 'OWN-QUOTE', birthday = '2008-01-02' WHERE yearbook_id = ?`, e.bookRow(lb))
	e.exec(`UPDATE yearbook_tab SET template_id = 'classic', page_size = 'A4', language = 'vi', motto = 'SECRET-MOTTO' WHERE id = ?`, e.bookRow(lb))
	p1, p2, ppend, phid := e.photo(lb), e.photo(lb), e.photo(lb), e.photo(lb)
	n3 := e.note(lb, "approved", 30, "third", p1, p2)
	n1 := e.note(lb, "approved", 10, "first")
	n2 := e.note(lb, "approved", 20, "second")
	e.note(lb, "pending", 5, "PENDING-TEXT", ppend)
	e.note(lb, "hidden", 6, "HIDDEN-TEXT", phid)
	e.profile(mb, "Minh", "")
	e.exec(`UPDATE profile_tab SET full_name = '' WHERE yearbook_id = ?`, e.bookRow(eb))
	e.note(mb, "pending", 1, "MINH-PENDING")
	for _, l := range []struct {
		p person
		b string
	}{{linh, lb}, {minh, mb}, {empty, eb}} {
		e.setBook(l.p, cls, l.b).status(200, "OK")
	}

	r := e.classBook(admin, cls).status(200, "OK")
	d := r.data()
	wantKeys(t, "class-book/get", d, "class", "front_pages", "members", "back_pages", "truncated")
	wantKeys(t, "class", d["class"].(map[string]any), "id", "name", "school_name", "graduation_year", "language")
	if len(d["front_pages"].([]any)) != 0 || len(d["back_pages"].([]any)) != 0 {
		t.Fatalf("pages: %v %v", d["front_pages"], d["back_pages"])
	}
	for _, secret := range []string{"PENDING-TEXT", "HIDDEN-TEXT", "MINH-PENDING", "SECRET-LABEL", "SECRET-MOTTO", "2008-01-02", ppend, phid, "token", "email", "@example.com"} {
		if strings.Contains(r.Body.String(), secret) {
			t.Fatalf("the class book leaks %q: %s", secret, r.Body)
		}
	}
	ms := d["members"].([]any)
	var order []string
	for _, m := range ms {
		order = append(order, m.(map[string]any)["name"].(string))
	}
	// admin (0, id first), bare/empty/ghost (0, joining order), minh (2), linh (5)
	if want := []string{"Admin", "Bare", "Empty", "Ghost", "Minh", "Linh"}; !slices.Equal(order, want) {
		t.Fatalf("member order %v, want %v", order, want)
	}
	by := map[string]map[string]any{}
	for _, m := range ms {
		mm := m.(map[string]any)
		wantKeys(t, "member", mm, "member_id", "name", "linked", "yearbook", "profile", "design", "notes", "note_count", "approved_total", "truncated")
		by[mm["name"].(string)] = mm
	}
	l := by["Linh"]
	if l["member_id"] != members[linh.row] || l["linked"] != true || l["note_count"] != float64(3) || l["approved_total"] != float64(3) || l["truncated"] != false {
		t.Fatalf("linh: %v", l)
	}
	wantKeys(t, "yearbook", l["yearbook"].(map[string]any), "template_id", "page_size", "language")
	if y := l["yearbook"].(map[string]any); y["template_id"] != "classic" || y["page_size"] != "A4" || y["language"] != "vi" {
		t.Fatalf("yearbook: %v", y)
	}
	wantKeys(t, "profile", l["profile"].(map[string]any), "full_name", "nickname", "quote", "hobbies", "future_plans", "photo_media_id")
	if pr := l["profile"].(map[string]any); pr["full_name"] != "Linh Nguyen" || pr["photo_media_id"] != pp {
		t.Fatalf("profile: %v", pr)
	}
	var ids []string
	for _, n := range l["notes"].([]any) {
		nn := n.(map[string]any)
		wantKeys(t, "note", nn, "id", "answers", "photo_ids", "signature")
		ids = append(ids, nn["id"].(string))
	}
	if want := []string{n1, n2, n3}; !slices.Equal(ids, want) {
		t.Fatalf("notes %v, want the student's order %v", ids, want)
	}
	if ph := l["notes"].([]any)[2].(map[string]any)["photo_ids"].([]any); len(ph) != 2 || ph[0] != p1 || ph[1] != p2 {
		t.Fatalf("photo ids %v", ph)
	}
	// a page with a profile and only pending notes is listed with its profile and no notes; an empty page and no page have no content
	if m := by["Minh"]; m["linked"] != true || m["note_count"] != float64(0) || len(m["notes"].([]any)) != 0 || m["profile"].(map[string]any)["photo_media_id"] != nil {
		t.Fatalf("minh: %v", m)
	}
	for _, name := range []string{"Bare", "Empty", "Ghost", "Admin"} {
		m := by[name]
		if m["linked"] != false || m["yearbook"] != nil || m["profile"] != nil || len(m["notes"].([]any)) != 0 || m["note_count"] != float64(0) {
			t.Fatalf("%s should have no content: %v", name, m)
		}
	}
}

// T-146: every member carries their own resolved design (the book-data/get shape); defaults when they have no row or an old row made
// for another template; null when the template has no design block; null for a member with no page.
func TestClassBookCarriesEachStudentsDesign(t *testing.T) {
	e := newEnv(t)
	admin, a, b, c, d, bare := e.user("Admin"), e.user("A"), e.user("B"), e.user("C"), e.user("D"), e.user("Bare")
	cls, _ := e.seedClass([]person{admin}, a, b, c, d, bare)
	books := map[string]string{}
	for name, p := range map[string]person{"A": a, "B": b, "C": c, "D": d} {
		books[name] = e.newBook(p)
		e.profile(books[name], name, "")
		e.setBook(p, cls, books[name]).status(200, "OK")
	}
	e.design(books["A"], "navy-classic", `{"v":1,"theme":"burgundy","fonts":"modern","sections":{"order":["notes","profile"],"hidden":["profile"]},"texts":{"profile.heading":"Về mình"}}`)
	e.design(books["B"], "navy-classic", `{"v":1}`)
	e.exec(`UPDATE yearbook_tab SET template_id = 'navy-classic' WHERE id = ?`, e.bookRow(books["C"])) // no row
	e.design(books["D"], "memory-book", `{"v":1,"theme":"mint"}`)
	e.exec(`UPDATE yearbook_tab SET template_id = 'navy-classic' WHERE id = ?`, e.bookRow(books["D"])) // the row is for another template now

	got := map[string]map[string]any{}
	for _, m := range e.classBook(admin, cls).status(200, "OK").data()["members"].([]any) {
		mm := m.(map[string]any)
		got[mm["name"].(string)] = mm
	}
	wantDesign := func(name, theme, fonts, text string, hidden []any) {
		t.Helper()
		ds, _ := got[name]["design"].(map[string]any)
		if ds == nil || ds["theme"] != theme || ds["fonts"] != fonts || fmt.Sprint(ds["sections"].(map[string]any)["hidden"]) != fmt.Sprint(hidden) {
			t.Fatalf("%s design: %v", name, got[name]["design"])
		}
		if tx := ds["texts"].(map[string]any)["profile.heading"]; (tx == nil && text != "") || (tx != nil && tx != text) {
			t.Fatalf("%s text: %v", name, tx)
		}
	}
	wantDesign("A", "burgundy", "modern", "Về mình", []any{"profile"})
	if o := got["A"]["design"].(map[string]any)["sections"].(map[string]any)["order"]; fmt.Sprint(o) != "[cover notes profile back]" {
		t.Fatalf("order %v", o)
	}
	wantDesign("B", "navy", "classic", "", []any{})
	wantDesign("C", "navy", "classic", "", []any{})
	wantDesign("D", "navy", "classic", "", []any{})
	if got["Bare"]["design"] != nil {
		t.Fatalf("a member with no page has a design: %v", got["Bare"]["design"])
	}
}

// The cap of 40 notes per member, cut from the end of the student's own order, and never silent.
func TestClassBookCapsNotesPerMember(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, _ := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	e.profile(book, "Linh", "")
	want := map[int]string{}
	for i := 1; i <= 45; i++ {
		want[i] = e.note(book, "approved", i, "n"+strconv.Itoa(i))
	}
	e.setBook(student, cls, book).status(200, "OK")
	m := e.classBook(admin, cls).status(200, "OK").data()["members"].([]any)
	var linh map[string]any
	for _, x := range m {
		if x.(map[string]any)["name"] == "Student" {
			linh = x.(map[string]any)
		}
	}
	ns := linh["notes"].([]any)
	if len(ns) != notes.MaxClassBookNotes || linh["note_count"] != float64(40) || linh["approved_total"] != float64(45) || linh["truncated"] != true {
		t.Fatalf("%d notes: %v %v %v", len(ns), linh["note_count"], linh["approved_total"], linh["truncated"])
	}
	for i, n := range ns {
		if n.(map[string]any)["id"] != want[i+1] {
			t.Fatalf("note %d is out of order", i)
		}
	}
	// exactly 40 is not cut
	e.exec(`DELETE FROM note_tab WHERE public_id IN (?,?,?,?,?)`, want[41], want[42], want[43], want[44], want[45])
	linh = e.classBook(admin, cls).status(200, "OK").data()["members"].([]any)[1].(map[string]any)
	if linh["truncated"] != false || linh["approved_total"] != float64(40) || linh["note_count"] != float64(40) {
		t.Fatalf("40 notes: %v", linh)
	}
}

// profile_complete only checks that photo_media_id is set: a photo id that points at nothing renders without the photo and never errors.
func TestClassBookToleratesADanglingPhotoID(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, _ := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	e.seedPage(book, 2, 0, 0) // sets photo_media_id = 1, and no media row exists
	e.setBook(student, cls, book).status(200, "OK")
	if st := e.do(student, "GET", "/api/class/get?id="+cls, "").data()["page"].(map[string]any); st["profile_complete"] != true {
		t.Fatalf("the dangling id should count as complete: %v", st)
	}
	var s map[string]any
	for _, m := range e.classBook(admin, cls).status(200, "OK").data()["members"].([]any) {
		if m.(map[string]any)["name"] == "Student" {
			s = m.(map[string]any)
		}
	}
	if s["linked"] != true || s["profile"].(map[string]any)["photo_media_id"] != nil || s["note_count"] != float64(2) {
		t.Fatalf("student: %v", s)
	}
	// and a photo that was deleted after it was chosen
	ph := e.photo(book)
	e.profile(book, "Linh", ph)
	e.exec(`DELETE FROM media_tab WHERE public_id = ?`, ph)
	e.classBook(admin, cls).status(200, "OK")
}

// ---- AC2, AC3: the admin's media read path ---------------------------------------------------------------------------------

func TestAdminMediaReadPath(t *testing.T) {
	e := newEnv(t)
	admin, adminB, student, classmate, stranger := e.user("Admin"), e.user("AdminB"), e.user("Student"), e.user("Classmate"), e.user("Stranger")
	removed, unlinked, other := e.user("Removed"), e.user("Unlinked"), e.user("Other")
	cls, members := e.seedClass([]person{admin}, student, classmate, removed, unlinked)
	clsB, _ := e.seedClass([]person{adminB}, other)

	sb, cb, rb, ub, ob := e.newBook(student), e.newBook(classmate), e.newBook(removed), e.newBook(unlinked), e.newBook(other)
	pp := e.photo(sb)
	e.profile(sb, "Linh", pp)
	approved1, approved2 := e.photo(sb), e.photo(sb)
	e.note(sb, "approved", 1, "a", approved1, approved2)
	pending, hidden, orphan, cover := e.photo(sb), e.photo(sb), e.photo(sb), e.photo(sb)
	e.note(sb, "pending", 2, "p", pending)
	e.note(sb, "hidden", 3, "h", hidden)
	e.exec(`UPDATE yearbook_tab SET cover_media_id = (SELECT id FROM media_tab WHERE public_id = ?) WHERE id = ?`, cover, e.bookRow(sb))
	cbPhoto, rbPhoto, ubPhoto, obPhoto := e.photo(cb), e.photo(rb), e.photo(ub), e.photo(ob)
	for book, ph := range map[string]string{cb: cbPhoto, rb: rbPhoto, ub: ubPhoto, ob: obPhoto} {
		e.profile(book, "X", ph)
		e.note(book, "approved", 1, "n", ph)
	}
	// T-152: design images (backgrounds and decorations of a layout) of every book, none used by a profile or a note
	sbDesign, cbDesign, rbDesign, ubDesign, obDesign := e.designImage(sb), e.designImage(cb), e.designImage(rb), e.designImage(ub), e.designImage(ob)
	e.setBook(student, cls, sb).status(200, "OK")
	e.setBook(classmate, cls, cb).status(200, "OK")
	e.setBook(removed, cls, rb).status(200, "OK")
	e.setBook(other, clsB, ob).status(200, "OK")
	// unlinked never linked; removed leaves the class after linking
	e.do(admin, "POST", "/api/class-member/remove", body("class_id", cls, "member_id", members[removed.row])).status(200, "OK")

	// the admin reads the profile photo and the photos of approved notes, in every variant, and the owner reads everything of theirs
	for _, id := range []string{pp, approved1, approved2, cbPhoto} {
		e.wantPhoto(admin, id)
	}
	e.wantPhoto(student, pending)
	e.wantPhoto(student, hidden)
	e.wantPhoto(student, orphan)
	e.wantPhoto(student, cover)
	// T-152 AC3: the admin reads the design images of members whose book is in the class (also the ones no layout uses yet), the
	// owner reads their own, and nobody else reads any
	e.wantPhoto(admin, sbDesign)
	e.wantPhoto(admin, cbDesign)
	e.wantPhoto(student, sbDesign)
	// AC2: nothing else, and every refusal is the answer for a photo that does not exist
	for what, id := range map[string]string{
		"pending note photo": pending, "hidden note photo": hidden, "photo of no note": orphan, "cover photo": cover,
		"unlinked yearbook": ubPhoto, "removed member": rbPhoto, "book linked to another class": obPhoto,
		"design image of an unlinked yearbook": ubDesign, "design image of a removed member": rbDesign, "design image of another class": obDesign,
	} {
		e.wantNoPhoto(admin, id, what)
	}
	// the other admin has the same rule for their class, and none for ours
	e.wantPhoto(adminB, obPhoto)
	e.wantPhoto(adminB, obDesign)
	for _, id := range []string{pp, approved1, cbPhoto, pending, sbDesign, cbDesign} {
		e.wantNoPhoto(adminB, id, "admin of another class")
		e.wantNoPhoto(stranger, id, "stranger")
	}
	// a student has no read path into a classmate's photos, approved or not
	for _, id := range []string{pp, approved1, pending, sbDesign} {
		e.wantNoPhoto(classmate, id, "classmate")
	}
	// a demoted admin has none either
	e.exec(`UPDATE class_member_tab SET role = 'student' WHERE class_id = (SELECT id FROM class_tab WHERE public_id = ?) AND user_id = ?`, cls, admin.row)
	e.wantNoPhoto(admin, pp, "demoted admin")
	e.wantNoPhoto(admin, sbDesign, "demoted admin")
	e.exec(`UPDATE class_member_tab SET role = 'admin' WHERE class_id = (SELECT id FROM class_tab WHERE public_id = ?) AND user_id = ?`, cls, admin.row)
	e.wantPhoto(admin, pp)
	// unlinking the page ends the admin's access to its design images at once
	e.setBook(student, cls, nil).status(200, "OK")
	e.wantNoPhoto(admin, sbDesign, "unlinked page")
	e.wantPhoto(student, sbDesign)
	e.setBook(student, cls, sb).status(200, "OK")

	// the path is read only: the admin deleting an allowed photo changes nothing
	n := e.count(`SELECT COUNT(*) FROM media_tab`)
	e.do(admin, "POST", "/api/media/delete", body("id", pp)).status(200, "OK")
	e.do(admin, "POST", "/api/media/delete", body("id", sbDesign)).status(200, "OK")
	if e.count(`SELECT COUNT(*) FROM media_tab`) != n {
		t.Fatal("the admin deleted a photo or a design image")
	}
	// ... and lists none: media/get-list is the owner's, even with purpose=design
	e.do(admin, "GET", "/api/media/get-list?yearbook_id="+sb+"&purpose=design", "").status(404, "ERROR_NOT_FOUND")
	e.wantPhoto(student, pp)
}

// AC3: a note approved after the first call appears in the next; a note hidden after approval disappears and its photo is a 404 at once.
func TestClassBookFollowsModeration(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, members := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	e.profile(book, "Linh", "")
	ph := e.photo(book)
	note := e.note(book, "pending", 1, "later", ph)
	e.setBook(student, cls, book).status(200, "OK")
	count := func() int {
		for _, m := range e.classBook(admin, cls).status(200, "OK").data()["members"].([]any) {
			if m.(map[string]any)["name"] == "Student" {
				return int(m.(map[string]any)["note_count"].(float64))
			}
		}
		t.Fatal("no student")
		return -1
	}
	set := func(status string) {
		e.do(student, "POST", "/api/note/batch-update-status", `{"yearbook_id":"`+book+`","ids":["`+note+`"],"status":"`+status+`"}`).status(200, "OK")
	}
	if count() != 0 {
		t.Fatal("a pending note is in the book")
	}
	e.wantNoPhoto(admin, ph, "pending")
	set("approved")
	if count() != 1 {
		t.Fatal("an approved note is not in the next call")
	}
	e.wantPhoto(admin, ph)
	set("hidden")
	if count() != 0 {
		t.Fatal("a hidden note is still in the book")
	}
	e.wantNoPhoto(admin, ph, "hidden")
	set("approved")
	e.wantPhoto(admin, ph)
	// the profile photo follows the profile: a replaced photo is no longer readable, the new one is
	pp, pp2 := e.photo(book), e.photo(book)
	e.profile(book, "Linh", pp)
	e.wantPhoto(admin, pp)
	e.profile(book, "Linh", pp2)
	e.wantNoPhoto(admin, pp, "replaced profile photo")
	e.wantPhoto(admin, pp2)
	// deleting the note removes its photo from the path
	e.do(student, "POST", "/api/note/batch-delete", `{"yearbook_id":"`+book+`","ids":["`+note+`"]}`).status(200, "OK")
	e.wantNoPhoto(admin, ph, "deleted note")
	// leaving the class, or being removed, ends the admin's view at once; the link does not come back by itself
	e.wantPhoto(admin, pp2)
	e.do(student, "POST", "/api/class-member/leave", body("class_id", cls)).status(200, "OK")
	e.wantNoPhoto(admin, pp2, "left the class")
	for _, m := range e.classBook(admin, cls).status(200, "OK").data()["members"].([]any) {
		if m.(map[string]any)["member_id"] == members[student.row] {
			t.Fatal("a member who left is still in the book")
		}
	}
}

// The book-data/get of the owner is unchanged by the class: same bytes before the link, after it, and after the admin has read everything;
// and its approved notes are the class book's notes (one moderation rule, one order).
func TestOwnersBookDataIsUnchanged(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, _ := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	pp := e.photo(book)
	e.profile(book, "Linh", pp)
	a, b := e.note(book, "approved", 2, "b", e.photo(book)), e.note(book, "approved", 1, "a")
	e.note(book, "pending", 3, "c")
	get := func() string {
		return e.do(student, "GET", "/api/book-data/get?yearbook_id="+book, "").status(200, "OK").sameAnswer()
	}
	before := get()
	e.setBook(student, cls, book).status(200, "OK")
	cb := e.classBook(admin, cls).status(200, "OK").data()["members"].([]any)
	e.wantPhoto(admin, pp)
	if get() != before {
		t.Fatal("book-data/get changed once the book was linked and read by the admin")
	}
	var ids []string
	for _, m := range cb {
		if m.(map[string]any)["name"] == "Student" {
			for _, n := range m.(map[string]any)["notes"].([]any) {
				ids = append(ids, n.(map[string]any)["id"].(string))
			}
		}
	}
	var own struct {
		Data struct{ Notes []struct{ ID string } }
	}
	_ = json.Unmarshal(e.do(student, "GET", "/api/book-data/get?yearbook_id="+book, "").Body.Bytes(), &own)
	var ownIDs []string
	for _, n := range own.Data.Notes {
		ownIDs = append(ownIDs, n.ID)
	}
	if !slices.Equal(ids, ownIDs) || !slices.Equal(ids, []string{b, a}) {
		t.Fatalf("class book notes %v, owner's %v, want %v", ids, ownIDs, []string{b, a})
	}
}

// ---- AC4: cost ------------------------------------------------------------------------------------------------------------

// serviceOn builds the class service on another pool (a spy), over a yearbook and notes service on the same pool.
func (e *env) serviceOn(d *sql.DB) *Service {
	e.t.Helper()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	limiters := ratelimit.NewFactory(redistest.New(e.t), logger, e.now)
	cursors := httpx.NewCursors([]byte(strings.Repeat("c", 32)))
	mediaSvc := media.NewService(media.NewStore(d), e.store, 2, limiters, e.now)
	notesSvc := notes.NewService(notes.NewStore(d), mediaSvc, cursors, limiters, logger, e.now)
	bookSvc := yearbook.NewService(yearbook.NewStore(d), cursors, mediaSvc, e.now, mediaSvc, notesSvc)
	s := NewService(NewStore(d), cursors, limiters, e.now)
	s.SetBookSources(bookSvc, notesSvc, bookdesign.NewService(bookdesign.NewStore(d), nil, e.now))
	return s
}

// fillClass makes a class of n students, each with a named profile with a photo and perNote approved notes with two photos.
func (e *env) fillClass(admin person, n, perStudent int) string {
	e.t.Helper()
	cls, _ := e.seedClass([]person{admin})
	cid := e.count(`SELECT id FROM class_tab WHERE public_id = ?`, cls)
	now := e.now().UnixMilli()
	for i := range n {
		u := e.user("S" + strconv.Itoa(i))
		e.exec(`INSERT INTO class_member_tab (public_id, class_id, user_id, role, sort_order, joined_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			ulid.New(e.now()), cid, u.row, RoleStudent, i, now, now, now)
		book := e.newBook(u)
		e.profile(book, "Student "+strconv.Itoa(i), e.photo(book))
		e.exec(`UPDATE class_member_tab SET yearbook_id = ? WHERE class_id = ? AND user_id = ?`, e.bookRow(book), cid, u.row)
		for j := range perStudent {
			e.note(book, "approved", j+1, strings.Repeat("Lời nhắn thân thương. ", 10), e.photo(book), e.photo(book))
		}
	}
	return cls
}

// AC4: the number of queries grows with the members, not with the notes.
func TestClassBookQueryCountIsPerMember(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	var mu sync.Mutex
	var queries []string
	spy := dbtest.Spy(t, e.db, func(q string) { mu.Lock(); queries = append(queries, q); mu.Unlock() })
	svc := e.serviceOn(spy)
	run := func(cls string) int {
		mu.Lock()
		queries = nil
		mu.Unlock()
		b, err := svc.Book(context.Background(), admin.row, cls)
		if err != nil {
			t.Fatal(err)
		}
		if len(b.Members) == 0 {
			t.Fatal("no members")
		}
		mu.Lock()
		defer mu.Unlock()
		return len(queries)
	}
	few := run(e.fillClass(admin, 6, 1))
	many := run(e.fillClass(admin, 6, 30))
	if few != many {
		t.Fatalf("queries with 1 note per student: %d, with 30: %d; they must not depend on the notes", few, many)
	}
	// authorize + members + per linked member (book, profile, media ids, notes, note photos), and the same for the admin's own unlinked row
	if perMember := (many - 2) / 6; perMember > 6 {
		t.Fatalf("%d queries for 6 members: %d each", many, perMember)
	}
	t.Logf("queries: %d for 6 members, with 1 or 30 notes each", many)
}

// AC4: a full class (60 members, 40 approved notes each, two photos each) answers within the budget and the size is measured.
func TestClassBookFullClass(t *testing.T) {
	if testing.Short() {
		t.Skip("large fixture")
	}
	e := newEnv(t)
	admin := e.user("Admin")
	cls := e.fillClass(admin, MaxMembers-1, notes.MaxClassBookNotes)
	start := time.Now()
	r := e.classBook(admin, cls).status(200, "OK")
	took := time.Since(start)
	var out struct {
		Data struct{ Members []struct{ Notes []any } }
	}
	_ = json.Unmarshal(r.Body.Bytes(), &out)
	if len(out.Data.Members) != MaxMembers || len(out.Data.Members[1].Notes) != 40 {
		t.Fatalf("%d members", len(out.Data.Members))
	}
	t.Logf("60 members x 40 notes (about 230 characters and 2 photos each): %d bytes in %v", r.Body.Len(), took)
	if r.Body.Len() > 4<<20 {
		t.Fatalf("response is %d bytes, the budget is 4 MiB", r.Body.Len())
	}
	if took > 10*time.Second {
		t.Fatalf("took %v", took)
	}
}

// AC4: EXPLAIN. Every table the main queries touch can use an index: no query reads a table without one.
func TestClassBookQueriesUseIndexes(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, _ := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	pp := e.photo(book)
	e.profile(book, "Linh", pp)
	e.note(book, "approved", 1, "x", e.photo(book))
	e.setBook(student, cls, book).status(200, "OK")
	cid := e.count(`SELECT id FROM class_tab WHERE public_id = ?`, cls)
	for name, q := range map[string]struct {
		sql  string
		args []any
	}{
		"book members": {`SELECT m.public_id, m.user_id, u.display_name, y.public_id FROM class_member_tab m JOIN user_tab u ON u.id = m.user_id
			LEFT JOIN yearbook_tab y ON y.id = m.yearbook_id AND y.owner_id = m.user_id WHERE m.class_id = ? ORDER BY m.sort_order, m.id LIMIT 60`, []any{cid}},
		"admin photo read": {`SELECT m.id FROM media_tab m JOIN yearbook_tab y ON y.id = m.yearbook_id
			WHERE m.public_id = ? AND (y.owner_id = ? OR EXISTS (
			  SELECT 1 FROM class_member_tab cm JOIN class_member_tab adm ON adm.class_id = cm.class_id AND adm.user_id = ? AND adm.role = 'admin'
			  WHERE cm.yearbook_id = y.id AND cm.user_id = y.owner_id
			    AND (EXISTS (SELECT 1 FROM profile_tab p WHERE p.yearbook_id = y.id AND p.is_owner = 1 AND p.photo_media_id = m.id)
			      OR EXISTS (SELECT 1 FROM note_photo_tab np JOIN note_tab n ON n.id = np.note_id WHERE np.media_id = m.id AND n.yearbook_id = y.id AND n.status = 'approved'))))`,
			[]any{pp, 0, admin.row}},
	} {
		rows, err := e.db.Query("EXPLAIN "+q.sql, q.args...)
		if err != nil {
			t.Fatal(err)
		}
		cols, _ := rows.Columns()
		for rows.Next() {
			vals := make([]sql.NullString, len(cols))
			ptrs := make([]any, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			if err := rows.Scan(ptrs...); err != nil {
				t.Fatal(err)
			}
			plan := map[string]string{}
			for i, c := range cols {
				plan[strings.ToLower(c)] = vals[i].String
			}
			t.Logf("%s: table=%s type=%s possible_keys=%s key=%s rows=%s", name, plan["table"], plan["type"], plan["possible_keys"], plan["key"], plan["rows"])
			if tb := plan["table"]; tb != "" && !strings.HasPrefix(tb, "<") && plan["possible_keys"] == "" {
				t.Errorf("%s reads %s without a usable index", name, tb)
			}
		}
		_ = rows.Close()
	}
}

// ---- AC1 (carried over from T-126 QA): the admin-404 matrix covers every route -------------------------------------------

// routesWithIDs lists the routes of api/openapi.yaml that take a yearbook, note or media id (a query parameter or a request body
// property named id, ids, yearbook_id, media_id or note_id), without /api/. The namespaces whose id is not one of those are skipped:
// auth and user (no ids), template, print-feedback, the public routes (a link token), class, class-invite and class-page (their own ids; the class
// matrix is TestPermissionMatrix and the invite matrix is in invite_integration_test.go).
func routesWithIDs(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile("../../api/openapi.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	ref := func(v any) any {
		for {
			m, ok := v.(map[string]any)
			if !ok || m["$ref"] == nil {
				return v
			}
			cur := any(doc)
			for _, part := range strings.Split(strings.TrimPrefix(m["$ref"].(string), "#/"), "/") {
				cur = cur.(map[string]any)[part]
			}
			v = cur
		}
	}
	idName := map[string]bool{"id": true, "ids": true, "yearbook_id": true, "media_id": true, "note_id": true}
	var props func(v any) bool
	props = func(v any) bool {
		m, ok := ref(v).(map[string]any)
		if !ok {
			return false
		}
		if ps, ok := m["properties"].(map[string]any); ok {
			for k := range ps {
				if idName[k] {
					return true
				}
			}
		}
		for _, k := range []string{"allOf", "oneOf", "anyOf"} {
			if list, ok := m[k].([]any); ok {
				for _, s := range list {
					if props(s) {
						return true
					}
				}
			}
		}
		return false
	}
	skip := []string{"auth/", "user/", "class/", "class-invite/", "class-page/", "template/", "print-feedback/", "public-"}
	var out []string
	for path, item := range doc["paths"].(map[string]any) {
		route := strings.TrimPrefix(path, "/api/")
		if slices.ContainsFunc(skip, func(p string) bool { return strings.HasPrefix(route, p) }) {
			continue
		}
		for _, op := range item.(map[string]any) {
			opm, ok := op.(map[string]any)
			if !ok {
				continue
			}
			takes := false
			params, _ := opm["parameters"].([]any)
			pathParams, _ := item.(map[string]any)["parameters"].([]any)
			for _, p := range append(params, pathParams...) {
				if pm, ok := ref(p).(map[string]any); ok && idName[pm["name"].(string)] {
					takes = true
				}
			}
			if rb, ok := ref(opm["requestBody"]).(map[string]any); ok {
				content, _ := rb["content"].(map[string]any)
				for _, c := range content {
					takes = takes || props(c.(map[string]any)["schema"])
				}
			}
			if takes && !slices.Contains(out, route) {
				out = append(out, route)
			}
		}
	}
	slices.Sort(out)
	return out
}

// The guard (T-126 QA): a new route that takes a yearbook, note or media id must be added to adminCalls, or this test fails. The only
// route on which an admin may succeed is media/get-content, and only for the two kinds of photo of TestAdminMediaReadPath.
func TestAdminRoutesAreAllInTheMatrix(t *testing.T) {
	routes := routesWithIDs(t)
	e := &env{t: t}
	matrix := map[string]bool{}
	for k := range e.adminCalls(person{}, "", "", "", "", "") {
		matrix[strings.Fields(k)[0]] = true
	}
	if len(routes) < 15 || !slices.Contains(routes, "media/create") || !slices.Contains(routes, "class-member/set-book") {
		t.Fatalf("the OpenAPI scan found %d routes (%v): it looks broken", len(routes), routes)
	}
	for _, r := range routes {
		if !matrix[r] {
			t.Errorf("%s takes a yearbook, note or media id but is not in adminCalls (the admin-404 matrix): add it", r)
		}
	}
	for r := range matrix {
		if !slices.Contains(routes, r) {
			t.Errorf("adminCalls has %s, which is not a route taking an id in api/openapi.yaml", r)
		}
	}
}

// ---- T-133: the read limiter, the size cap and the photo cache header -----------------------------------------------------

// AC1: the 31st read of an hour is a 429 with Retry-After; another class or another user is not affected; with the limiter store
// down the read is a 503 (fail closed).
func TestClassBookReadLimiter(t *testing.T) {
	e := newEnv(t)
	admin, admin2, student := e.user("Admin"), e.user("Admin2"), e.user("Student")
	cls, _ := e.seedClass([]person{admin, admin2}, student)
	other, _ := e.seedClass([]person{admin})
	for range bookReadsPerWindow {
		e.classBook(admin, cls).status(200, "OK")
	}
	if r := e.classBook(admin, cls).status(429, "ERROR_RATE_LIMITED"); r.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.classBook(admin2, cls).status(200, "OK")               // another user of the same class
	e.classBook(admin, other).status(200, "OK")              // the same user in another class
	e.classBook(student, cls).status(403, "ERROR_FORBIDDEN") // a refused read does not take from anyone's budget
	e.advance(time.Hour + time.Minute)
	e.classBook(admin, cls).status(200, "OK")

	dead, err := redis.New(config.Config{Env: "test", RedisURL: "redis://127.0.0.1:1/0",
		RedisDialTimeout: 200 * time.Millisecond, RedisReadTimeout: 200 * time.Millisecond, RedisWriteTimeout: 200 * time.Millisecond, RedisPoolSize: 2})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = dead.Close() }()
	e.svc.bookReads = ratelimit.NewFactory(dead, slog.New(slog.DiscardHandler), nil).Closed("class_book_read", bookReadsPerWindow, bookReadWindow)
	e.classBook(admin, cls).status(503, "ERROR_UNAVAILABLE")
}

// AC2: members whose notes do not fit in 8 MiB lose them all and are listed; the earlier members are complete; the body is within the
// cap and valid JSON. A class that fits has truncated [].
func TestClassBookIsCutAtTheCap(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	students := []person{e.user("S1"), e.user("S2"), e.user("S3"), e.user("S4")}
	cls, members := e.seedClass([]person{admin}, students...)
	small := e.classBook(admin, cls).status(200, "OK").data()
	if tr, ok := small["truncated"].([]any); !ok || len(tr) != 0 {
		t.Fatalf("truncated = %#v, want []", small["truncated"])
	}
	huge := strings.Repeat("\U0001F600", 20000) // 80 KB of four-byte characters: 40 of them are 3.2 MB
	for _, s := range students {
		book := e.newBook(s)
		e.profile(book, "Student", "")
		e.design(book, "navy-classic", `{"v":1,"theme":"forest","texts":{"back.thanks":"Cảm ơn"}}`) // T-146: the cap counts designs too
		for j := 1; j <= notes.MaxClassBookNotes; j++ {
			e.note(book, "approved", j, huge)
		}
		e.setBook(s, cls, book).status(200, "OK")
	}
	r := e.classBook(admin, cls).status(200, "OK")
	if r.Body.Len() > MaxBookBytes {
		t.Fatalf("%d bytes is over the cap", r.Body.Len())
	}
	d := r.data()
	got := map[string]int{}
	for _, x := range d["members"].([]any) {
		m := x.(map[string]any)
		got[m["member_id"].(string)] = len(m["notes"].([]any))
	}
	var full, cut []string
	for _, s := range students {
		switch n := got[members[s.row]]; n {
		case notes.MaxClassBookNotes:
			full = append(full, members[s.row])
		case 0:
			cut = append(cut, members[s.row])
		default:
			t.Fatalf("member %s has %d notes: a member is complete or has none", members[s.row], n)
		}
	}
	if len(full) != 2 || len(cut) != 2 || fmt.Sprint(d["truncated"]) != fmt.Sprint([]any{cut[0], cut[1]}) {
		t.Fatalf("full %v, cut %v, truncated %v", full, cut, d["truncated"])
	}
	for _, x := range d["members"].([]any) {
		if m := x.(map[string]any); slices.Contains(cut, m["member_id"].(string)) && (m["truncated"] != true || m["approved_total"] != float64(40) || m["profile"] == nil || m["design"] == nil || m["note_count"] != float64(0)) {
			t.Fatalf("cut member %v", m)
		}
	}
}

// AC4: a class admin's photo response is no-store (the student can hide the note at any moment), the owner's keeps its header.
func TestPhotoCacheControlByReader(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, _ := e.seedClass([]person{admin}, student)
	book := e.newBook(student)
	pp, np := e.photo(book), e.photo(book)
	e.profile(book, "Linh", pp)
	e.note(book, "approved", 1, "n", np)
	e.setBook(student, cls, book).status(200, "OK")
	for _, id := range []string{pp, np} {
		for _, variant := range []string{"display", "thumb", "print"} {
			path := "/api/media/get-content?id=" + id + "&variant=" + variant
			for who, want := range map[string]string{"admin": "private, no-store", "owner": "private, max-age=3600"} {
				r := e.do(map[string]person{"admin": admin, "owner": student}[who], "GET", path, "")
				if r.Code != 200 || r.Header().Get("Cache-Control") != want {
					t.Fatalf("%s %s: %d Cache-Control %q, want %q", who, variant, r.Code, r.Header().Get("Cache-Control"), want)
				}
			}
		}
	}
}
