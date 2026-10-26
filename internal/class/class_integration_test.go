//go:build integration

package class

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// AC3: a verified email is needed, and a user is admin of at most 10 classes (a class joined as a student does not count),
// also when the creates arrive at the same moment.
func TestCreateNeedsVerifiedEmailAndHonoursTheCap(t *testing.T) {
	e := newEnv(t)
	unverified := e.userWith("Unverified", false)
	e.do(unverified, "POST", "/api/class/create", classBody).status(403, "ERROR_EMAIL_NOT_VERIFIED")
	if n := e.count(`SELECT COUNT(*) FROM class_tab`); n != 0 {
		t.Fatalf("%d classes after a refused create", n)
	}

	u, boss := e.user("Teacher"), e.user("Boss")
	for range 3 {
		e.seedClass([]person{boss}, u) // u is only a student there
	}
	for range maxAdminClasses {
		e.create(u)
	}
	e.do(u, "POST", "/api/class/create", classBody).status(409, "ERROR_LIMIT_REACHED")
	if n := e.count(`SELECT COUNT(*) FROM class_member_tab WHERE user_id = ? AND role = 'admin'`, u.row); n != maxAdminClasses {
		t.Fatalf("%d admin memberships", n)
	}

	racer := e.user("Racer")
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[string]int{}
	for range maxAdminClasses + 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := e.do(racer, "POST", "/api/class/create", classBody)
			mu.Lock()
			codes[r.code()]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes["OK"] != maxAdminClasses || codes["ERROR_LIMIT_REACHED"] != 5 {
		t.Fatalf("concurrent creates: %v", codes)
	}
}

func TestValidation(t *testing.T) {
	e := newEnv(t)
	u := e.user("Teacher")
	long := strings.Repeat("x", maxName+1)
	for name, tc := range map[string]struct{ body, want string }{
		"empty name":        {`{"name":"  ","language":"en"}`, "ERROR_INVALID_NAME"},
		"missing name":      {`{"language":"en"}`, "ERROR_INVALID_NAME"},
		"long name":         {`{"name":"` + long + `","language":"en"}`, "ERROR_INVALID_NAME"},
		"control in name":   {`{"name":"a\u0007b","language":"en"}`, "ERROR_INVALID_NAME"},
		"long school":       {`{"name":"x","school_name":"` + long + `","language":"en"}`, "ERROR_INVALID_SCHOOL_NAME"},
		"year too early":    {`{"name":"x","graduation_year":1949,"language":"en"}`, "ERROR_INVALID_GRADUATION_YEAR"},
		"year too late":     {`{"name":"x","graduation_year":2101,"language":"en"}`, "ERROR_INVALID_GRADUATION_YEAR"},
		"missing language":  {`{"name":"x"}`, "ERROR_INVALID_LANGUAGE"},
		"unknown language":  {`{"name":"x","language":"fr"}`, "ERROR_INVALID_LANGUAGE"},
		"unknown field":     {`{"name":"x","language":"en","created_by":1}`, "ERROR_PARAM"},
		"year is a string":  {`{"name":"x","graduation_year":"2026","language":"en"}`, "ERROR_PARAM"},
		"not an object":     {`[]`, "ERROR_PARAM"},
		"trailing data":     {`{"name":"x","language":"en"} {}`, "ERROR_PARAM"},
		"name is a number":  {`{"name":1,"language":"en"}`, "ERROR_PARAM"},
		"role in the class": {`{"name":"x","language":"en","my_role":"admin"}`, "ERROR_PARAM"},
	} {
		t.Run(name, func(t *testing.T) {
			e.do(u, "POST", "/api/class/create", tc.body).status(400, tc.want)
		})
	}
	if n := e.count(`SELECT COUNT(*) FROM class_tab`); n != 0 {
		t.Fatalf("%d classes after refused creates", n)
	}
	// 120 runes of multi-byte text are fine (length is counted in characters)
	e.do(u, "POST", "/api/class/create", `{"name":"`+strings.Repeat("Đ", maxName)+`","language":"vi"}`).status(200, "OK")
}

// QA fuzz: ids must be canonical (26 upper-case Crockford characters) before anything is queried; the database would match
// a lower-case spelling of a real id otherwise.
func TestIDsMustBeCanonical(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls, members := e.seedClass([]person{admin, e.user("Co")})
	mid := members[admin.row]
	for _, id := range []string{"", strings.ToLower(cls), cls[:25], cls + "A", " " + cls[1:], strings.Replace(cls, cls[:1], "U", 1), "../" + cls[3:]} {
		e.do(admin, "GET", "/api/class/get?id="+url.QueryEscape(id), "").status(400, "ERROR_PARAM")
		e.do(admin, "GET", "/api/class-member/get-list?class_id="+url.QueryEscape(id), "").status(400, "ERROR_PARAM")
		e.do(admin, "POST", "/api/class/delete", body("id", id)).status(400, "ERROR_PARAM")
		e.do(admin, "POST", "/api/class-member/leave", body("class_id", id)).status(400, "ERROR_PARAM")
		e.do(admin, "POST", "/api/class-member/remove", body("class_id", id, "member_id", mid)).status(400, "ERROR_PARAM")
		e.do(admin, "POST", "/api/class-member/remove", body("class_id", cls, "member_id", id)).status(400, "ERROR_PARAM")
	}
	e.do(admin, "GET", "/api/class/get?id="+strings.ToLower(cls), "").status(400, "ERROR_PARAM")
	if n := e.count(`SELECT COUNT(*) FROM class_member_tab`); n != 2 {
		t.Fatalf("%d members after refused requests", n)
	}
}

func TestUpdateReplacesAndDeleteRemovesClassRowsOnly(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	id := e.create(admin)
	e.exec(`INSERT INTO class_member_tab (public_id, class_id, user_id, role, joined_at, created_at, updated_at) SELECT ?, id, ?, 'student', 1, 1, 1 FROM class_tab WHERE public_id = ?`, "01K7Q8W3ZR5N2M4X6V8B0C1D2G", student.row, id)

	d := e.do(admin, "POST", "/api/class/update", body("id", id, "name", " Renamed ", "language", "en")).status(200, "OK").data()
	if d["name"] != "Renamed" || d["school_name"] != "" || d["graduation_year"] != nil || d["language"] != "en" || d["my_role"] != RoleAdmin {
		t.Fatalf("update did not replace the fields: %v", d)
	}
	// a book of the student's, linked to the class
	book := e.do(student, "POST", "/api/yearbook/create", `{"title":"Mine","language":"en"}`).status(200, "OK").data()["id"].(string)
	e.exec(`UPDATE class_member_tab SET yearbook_id = (SELECT id FROM yearbook_tab WHERE public_id = ?) WHERE user_id = ?`, book, student.row)
	if got := e.do(student, "GET", "/api/class/get?id="+id, "").data()["page_linked"]; got != true {
		t.Fatalf("page_linked = %v", got)
	}

	e.do(admin, "POST", "/api/class/delete", body("id", id)).status(200, "OK")
	for _, q := range []string{`SELECT COUNT(*) FROM class_tab`, `SELECT COUNT(*) FROM class_member_tab`} {
		if n := e.count(q); n != 0 {
			t.Errorf("%s = %d after delete", q, n)
		}
	}
	e.do(student, "GET", "/api/yearbook/get?id="+book, "").status(200, "OK") // the student's yearbook is untouched
	e.do(admin, "GET", "/api/class/get?id="+id, "").status(404, "ERROR_NOT_FOUND")
	dbtest.AssertNoOrphans(t, e.db)
}

// Deleting a yearbook clears the class link and keeps the membership; nothing is left dangling.
func TestDeletingTheYearbookClearsTheLink(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	cls, _ := e.seedClass([]person{admin}, student)
	book := e.do(student, "POST", "/api/yearbook/create", `{"title":"Mine","language":"en"}`).status(200, "OK").data()["id"].(string)
	e.exec(`UPDATE class_member_tab SET yearbook_id = (SELECT id FROM yearbook_tab WHERE public_id = ?) WHERE user_id = ?`, book, student.row)
	e.do(student, "POST", "/api/yearbook/delete", body("id", book)).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM class_member_tab WHERE user_id = ? AND yearbook_id IS NULL`, student.row); n != 1 {
		t.Fatalf("the member row lost its membership or kept the link (%d)", n)
	}
	if got := e.do(student, "GET", "/api/class/get?id="+cls, "").data()["page_linked"]; got != false {
		t.Fatalf("page_linked = %v", got)
	}
	dbtest.AssertNoOrphans(t, e.db)
}

func TestLastAdminRule(t *testing.T) {
	e := newEnv(t)
	a, b, s := e.user("A"), e.user("B"), e.user("S")
	cls, m := e.seedClass([]person{a}, s)
	m[999999] = "01K7Q8W3ZR5N2M4X6V8B0C1D2E" // a member id that is in no class
	m[b.row] = "01K7Q8W3ZR5N2M4X6V8B0C1D2F"  // b has no membership here
	leave := func(p person) resp { return e.do(p, "POST", "/api/class-member/leave", body("class_id", cls)) }
	remove := func(p person, of uint64) resp {
		return e.do(p, "POST", "/api/class-member/remove", body("class_id", cls, "member_id", m[of]))
	}
	role := func(p person, of uint64, r string) resp {
		return e.do(p, "POST", "/api/class-member/update-role", body("class_id", cls, "member_id", m[of], "role", r))
	}

	leave(a).status(403, "ERROR_LAST_ADMIN")
	remove(a, a.row).status(403, "ERROR_LAST_ADMIN")
	role(a, a.row, RoleStudent).status(403, "ERROR_LAST_ADMIN")
	role(a, a.row, RoleAdmin).status(200, "OK") // no change is not a demotion
	role(a, s.row, "owner").status(400, "ERROR_INVALID_ROLE")
	remove(a, 999999).status(404, "ERROR_NOT_FOUND")
	role(a, b.row, RoleAdmin).status(404, "ERROR_NOT_FOUND") // b is not in this class

	role(a, s.row, RoleAdmin).status(200, "OK") // s becomes the second admin
	role(s, a.row, RoleStudent).status(200, "OK")
	role(s, s.row, RoleStudent).status(403, "ERROR_LAST_ADMIN") // s is the last admin now
	role(a, s.row, RoleStudent).status(403, "ERROR_FORBIDDEN")  // a is a student now
	leave(a).status(200, "OK")
	leave(a).status(404, "ERROR_NOT_FOUND")
	e.do(s, "GET", "/api/class/get?id="+cls, "").status(200, "OK")
	leave(s).status(403, "ERROR_LAST_ADMIN")
	e.do(s, "POST", "/api/class/delete", body("id", cls)).status(200, "OK") // the way out for the last admin
}

// AC2: whatever two admins do to each other at the same moment, one admin is left, and nothing answers 5xx.
func TestLastAdminHoldsUnderConcurrency(t *testing.T) {
	e := newEnv(t)
	for _, tc := range []struct {
		name string
		act  func(e *env, cls string, m map[uint64]string, me, other person) resp
	}{
		{"both leave", func(e *env, cls string, _ map[uint64]string, me, _ person) resp {
			return e.do(me, "POST", "/api/class-member/leave", body("class_id", cls))
		}},
		{"remove each other", func(e *env, cls string, m map[uint64]string, me, other person) resp {
			return e.do(me, "POST", "/api/class-member/remove", body("class_id", cls, "member_id", m[other.row]))
		}},
		{"demote each other", func(e *env, cls string, m map[uint64]string, me, other person) resp {
			return e.do(me, "POST", "/api/class-member/update-role", body("class_id", cls, "member_id", m[other.row], "role", RoleStudent))
		}},
		{"demote self", func(e *env, cls string, m map[uint64]string, me, _ person) resp {
			return e.do(me, "POST", "/api/class-member/update-role", body("class_id", cls, "member_id", m[me.row], "role", RoleStudent))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, b := e.user("A"), e.user("B")
			for round := range 12 {
				cls, m := e.seedClass([]person{a, b})
				var wg sync.WaitGroup
				res := make([]resp, 2)
				start := make(chan struct{})
				for i, pair := range [][2]person{{a, b}, {b, a}} {
					wg.Add(1)
					go func() {
						defer wg.Done()
						<-start
						res[i] = tc.act(e, cls, m, pair[0], pair[1])
					}()
				}
				close(start)
				wg.Wait()
				okN := 0
				for _, r := range res {
					switch {
					case r.Code == http.StatusOK:
						okN++
					case r.Code == http.StatusForbidden && (r.code() == "ERROR_LAST_ADMIN" || r.code() == "ERROR_FORBIDDEN"), r.Code == http.StatusNotFound:
						// refused: the last-admin rule, or the other request already demoted or removed the caller
					default:
						t.Fatalf("round %d: %d %s", round, r.Code, r.Body)
					}
				}
				if admins := e.count(`SELECT COUNT(*) FROM class_member_tab m JOIN class_tab c ON c.id = m.class_id WHERE c.public_id = ? AND m.role = 'admin'`, cls); okN != 1 || admins != 1 {
					t.Fatalf("round %d: %d requests succeeded and %d admins are left, want 1 and 1", round, okN, admins)
				}
			}
		})
	}
}

// Many admins leaving at once: exactly one stays.
func TestManyAdminsLeavingAtOnce(t *testing.T) {
	e := newEnv(t)
	const n = 8
	var admins []person
	for i := range n {
		admins = append(admins, e.user("Admin"+strconv.Itoa(i)))
	}
	cls, _ := e.seedClass(admins)
	var wg sync.WaitGroup
	var mu sync.Mutex
	codes := map[string]int{}
	for _, p := range admins {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r := e.do(p, "POST", "/api/class-member/leave", body("class_id", cls))
			mu.Lock()
			codes[r.code()]++
			mu.Unlock()
		}()
	}
	wg.Wait()
	if codes["OK"] != n-1 || codes["ERROR_LAST_ADMIN"] != 1 {
		t.Fatalf("answers: %v", codes)
	}
	if got := e.count(`SELECT COUNT(*) FROM class_member_tab`); got != 1 {
		t.Fatalf("%d members left", got)
	}
}

func TestPaging(t *testing.T) {
	e := newEnv(t)
	boss, u, other := e.user("Boss"), e.user("U"), e.user("Other")
	var want []string
	for range 5 {
		id, _ := e.seedClass([]person{boss}, u)
		want = append([]string{id}, want...) // newest membership first
	}
	e.seedClass([]person{boss}) // u is not in this one
	var got []string
	next := ""
	for pages := 0; ; pages++ {
		r := e.do(u, "GET", "/api/class/get-list?limit=2&next_id="+next, "").status(200, "OK").data()
		for _, it := range r["items"].([]any) {
			got = append(got, it.(map[string]any)["id"].(string))
		}
		if next = r["next_id"].(string); next == "" {
			if pages != 2 {
				t.Fatalf("%d pages", pages+1)
			}
			break
		}
		if pages > 5 {
			t.Fatal("never ends")
		}
		if pages == 0 { // the token of another user's list, a tampered one and the old numeric cursor are refused
			e.do(other, "GET", "/api/class/get-list?next_id="+next, "").status(400, "ERROR_PARAM")
			e.do(u, "GET", "/api/class/get-list?next_id="+next+"x", "").status(400, "ERROR_PARAM")
			e.do(u, "GET", "/api/class/get-list?next_id=12", "").status(400, "ERROR_PARAM")
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("walk %v, want %v", got, want)
	}
	e.do(u, "GET", "/api/class/get-list?limit=0", "").status(400, "ERROR_PARAM")
	e.do(u, "GET", "/api/class/get-list?limit=51", "").status(400, "ERROR_PARAM")

	// the roster: five members, pages of two, bound to the class and the caller
	students := []person{e.user("S1"), e.user("S2"), e.user("S3"), e.user("S4")}
	cls, _ := e.seedClass([]person{boss}, students...)
	other2, _ := e.seedClass([]person{boss}, students...)
	var names []string
	next = ""
	for range 4 {
		r := e.do(boss, "GET", "/api/class-member/get-list?class_id="+cls+"&limit=2&next_id="+next, "").status(200, "OK").data()
		for _, it := range r["items"].([]any) {
			names = append(names, it.(map[string]any)["name"].(string))
		}
		if next = r["next_id"].(string); next == "" {
			break
		}
		e.do(boss, "GET", "/api/class-member/get-list?class_id="+other2+"&next_id="+next, "").status(400, "ERROR_PARAM") // another class's list
	}
	if fmt.Sprint(names) != "[Boss S1 S2 S3 S4]" {
		t.Fatalf("roster walk %v", names)
	}
	e.do(boss, "GET", "/api/class-member/get-list?class_id="+cls+"&limit=61", "").status(400, "ERROR_PARAM")
}

// A user's writes are limited per hour; the answer says when to retry.
func TestWritesAreRateLimited(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls, _ := e.seedClass([]person{admin})
	update := func() resp {
		return e.do(admin, "POST", "/api/class/update", body("id", cls, "name", "x", "language", "en"))
	}
	for range writesPerWindow {
		update().status(200, "OK")
	}
	r := update().status(429, "ERROR_RATE_LIMITED")
	if r.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.do(admin, "GET", "/api/class/get?id="+cls, "").status(200, "OK") // reads are not limited
}
