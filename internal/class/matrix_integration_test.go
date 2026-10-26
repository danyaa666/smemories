//go:build integration

package class

import (
	"net/http"
	"strings"
	"testing"
)

// The permission matrix of docs/class-permissions.md, endpoint by endpoint and actor by actor (T-026 AC1). Every cell gets a
// class of its own, so a destructive endpoint does not change what the next cell sees.
//
// The actors: admin and coAdmin and student of the class under test; stranger, who is in no class at all; outsider, an admin of
// another class (the IDOR case); and a caller with no session. A non-member, whatever class they belong to, must get exactly the
// answer for "no such class".
func TestPermissionMatrix(t *testing.T) {
	e := newEnv(t)
	admin, coAdmin, student := e.user("Admin"), e.user("CoAdmin"), e.user("Student")
	stranger, outsider := e.user("Stranger"), e.user("Outsider")
	e.seedClass([]person{outsider}) // the other class, which exists but is not theirs to use here

	type want struct {
		status int
		code   string
	}
	ok := want{200, "OK"}
	forbidden := want{403, "ERROR_FORBIDDEN"}
	notFound := want{404, "ERROR_NOT_FOUND"}
	unauth := want{401, "ERROR_UNAUTHORIZED"}

	type fixture struct{ class, studentMember string }
	endpoints := []struct {
		name string
		call func(e *env, p person, f fixture) resp
		// the answers for admin, student, stranger, anonymous and outsider
		admin, student, stranger, anon, outsider want
	}{
		{"class/get", func(e *env, p person, f fixture) resp { return e.do(p, "GET", "/api/class/get?id="+f.class, "") },
			ok, ok, notFound, unauth, notFound},
		{"class/update", func(e *env, p person, f fixture) resp {
			return e.do(p, "POST", "/api/class/update", body("id", f.class, "name", "Renamed", "language", "en"))
		}, ok, forbidden, notFound, unauth, notFound},
		{"class/delete", func(e *env, p person, f fixture) resp {
			return e.do(p, "POST", "/api/class/delete", body("id", f.class))
		}, ok, forbidden, notFound, unauth, notFound},
		{"class-member/get-list", func(e *env, p person, f fixture) resp {
			return e.do(p, "GET", "/api/class-member/get-list?class_id="+f.class, "")
		}, ok, forbidden, notFound, unauth, notFound},
		{"class-member/update-role", func(e *env, p person, f fixture) resp {
			return e.do(p, "POST", "/api/class-member/update-role", body("class_id", f.class, "member_id", f.studentMember, "role", "admin"))
		}, ok, forbidden, notFound, unauth, notFound},
		{"class-member/remove", func(e *env, p person, f fixture) resp {
			return e.do(p, "POST", "/api/class-member/remove", body("class_id", f.class, "member_id", f.studentMember))
		}, ok, forbidden, notFound, unauth, notFound},
		{"class-member/leave", func(e *env, p person, f fixture) resp {
			return e.do(p, "POST", "/api/class-member/leave", body("class_id", f.class))
		}, ok, ok, notFound, unauth, notFound},
	}
	for _, ep := range endpoints {
		for _, a := range []struct {
			name string
			who  person
			want want
		}{
			{"admin", admin, ep.admin}, {"student", student, ep.student}, {"stranger", stranger, ep.stranger},
			{"anonymous", anonymous, ep.anon}, {"member of another class", outsider, ep.outsider},
		} {
			t.Run(ep.name+"/"+a.name, func(t *testing.T) {
				cls, members := e.seedClass([]person{admin, coAdmin}, student)
				f := fixture{cls, members[student.row]}
				r := ep.call(e, a.who, f)
				if r.Code != a.want.status || r.code() != a.want.code {
					t.Fatalf("got %d %s, want %d %s", r.Code, r.Body, a.want.status, a.want.code)
				}
				if a.want.status == http.StatusNotFound { // identical to "no such class", also for a member of another class
					gone := ep.call(e, a.who, fixture{"01K7Q8W3ZR5N2M4X6V8B0C1D2E", f.studentMember})
					if r.sameAnswer() != gone.sameAnswer() {
						t.Fatalf("a non-member's answer differs from the answer for a class that does not exist:\n%s\n%s", r.Body, gone.Body)
					}
				}
			})
		}
	}
}

// class/create and class/get-list are about the caller, not about one class: every verified user may create, and the list only
// ever holds the caller's own classes; without a session both are 401.
func TestMatrixCreateAndList(t *testing.T) {
	e := newEnv(t)
	a, b := e.user("A"), e.user("B")
	e.do(anonymous, "POST", "/api/class/create", classBody).status(401, "ERROR_UNAUTHORIZED")
	e.do(anonymous, "GET", "/api/class/get-list", "").status(401, "ERROR_UNAUTHORIZED")
	id := e.create(a)
	items := func(p person) []any {
		return e.do(p, "GET", "/api/class/get-list", "").status(200, "OK").data()["items"].([]any)
	}
	if got := items(a); len(got) != 1 || got[0].(map[string]any)["id"] != id || got[0].(map[string]any)["my_role"] != RoleAdmin {
		t.Fatalf("creator's list: %v", got)
	}
	if got := items(b); len(got) != 0 {
		t.Fatalf("a stranger's list shows classes: %v", got)
	}
}

// AC4: a student reads the class but never the roster, and the class answer holds no member data for anyone.
func TestStudentNeverSeesTheRoster(t *testing.T) {
	e := newEnv(t)
	admin, student, other := e.user("Admin"), e.user("Student"), e.user("Classmate")
	cls, _ := e.seedClass([]person{admin}, student, other)
	e.do(student, "GET", "/api/class-member/get-list?class_id="+cls, "").status(403, "ERROR_FORBIDDEN")
	for _, p := range []person{student, admin} {
		r := e.do(p, "GET", "/api/class/get?id="+cls, "").status(200, "OK")
		d := r.data()
		for _, k := range []string{"members", "roster", "students", "admins"} {
			if _, has := d[k]; has {
				t.Errorf("class/get carries %q", k)
			}
		}
		for _, name := range []string{"Classmate", "Admin", "Student"} {
			if strings.Contains(r.Body.String(), name) {
				t.Errorf("class/get leaks the display name %q", name)
			}
		}
	}
	if got := e.do(student, "GET", "/api/class/get?id="+cls, "").data()["my_role"]; got != RoleStudent {
		t.Fatalf("my_role = %v", got)
	}
	// the admin's roster has the three names, and no email, user id or page content
	r := e.do(admin, "GET", "/api/class-member/get-list?class_id="+cls, "").status(200, "OK")
	if got := len(r.data()["items"].([]any)); got != 3 {
		t.Fatalf("roster has %d rows", got)
	}
	for _, leak := range []string{"@example.com", "email", "user_id", "yearbook"} {
		if strings.Contains(r.Body.String(), leak) {
			t.Errorf("roster leaks %q: %s", leak, r.Body)
		}
	}
}
