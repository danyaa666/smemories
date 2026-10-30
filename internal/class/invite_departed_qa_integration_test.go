//go:build integration

package class

import "testing"

// T-140 QA: the departed admin's links of ANOTHER class stay live, a revoked token answers byte for byte like an unknown one on
// lookup and accept, and an admin who is promoted again does not get the old links back.
func TestDepartedAdminOtherClassAndRepromotion(t *testing.T) {
	e := newEnv(t)
	owner, gone, joiner := e.user("Owner"), e.user("Gone"), e.user("Joiner")
	cls, _ := e.seedClass([]person{owner, gone})
	cls2, _ := e.seedClass([]person{owner, gone})
	_, tok1 := e.createInvite(gone, cls)
	_, tok2 := e.createInvite(gone, cls2)

	e.do(gone, "POST", "/api/class-member/leave", body("class_id", cls)).status(200, "OK")

	e.lookup(tok1).status(404, "ERROR_NOT_FOUND")
	e.lookup(tok2).status(200, "OK") // other class untouched
	e.accept(joiner, tok2).status(200, "OK")

	// same answer as a token that never existed
	never := "x" + tok1[1:]
	if got, want := e.lookup(tok1).sameAnswer(), e.lookup(never).sameAnswer(); got != want {
		t.Fatalf("revoked lookup %s differs from unknown %s", got, want)
	}
	if got, want := e.accept(e.user("Late"), tok1).sameAnswer(), e.accept(e.user("Late2"), never).sameAnswer(); got != want {
		t.Fatalf("revoked accept %s differs from unknown %s", got, want)
	}

	// re-join and promote again: old link stays dead
	e.accept(gone, tok2).status(200, "OK") // already a member of cls2: idempotent
	_, tokNew := e.createInvite(owner, cls)
	e.accept(gone, tokNew).status(200, "OK")
	e.do(owner, "POST", "/api/class-member/update-role", body("class_id", cls, "member_id", e.memberID(cls, gone), "role", RoleAdmin)).status(200, "OK")
	e.lookup(tok1).status(404, "ERROR_NOT_FOUND")
	e.lookup(tokNew).status(200, "OK")
}
