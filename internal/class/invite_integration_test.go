//go:build integration

package class

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/ulid"
)

// parallel runs f(0..n-1) at the same moment and returns the answers in order.
func parallel(n int, f func(i int) resp) []resp {
	out := make([]resp, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			out[i] = f(i)
		}()
	}
	close(start)
	wg.Wait()
	return out
}

func tally(rs []resp) map[string]int {
	m := map[string]int{}
	for _, r := range rs {
		m[r.code()]++
	}
	return m
}

// fill adds n students to the class straight in the database (fake users, no sessions).
func (e *env) fill(cls string, n int) {
	e.t.Helper()
	now := e.now().UnixMilli()
	for range n {
		e.mu.Lock()
		e.n++
		k := e.n
		e.mu.Unlock()
		res, err := e.db.Exec(`INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
			ulid.New(e.now()), "fill"+strconv.Itoa(k)+"@example.com", "Filler", "filler-"+strconv.Itoa(k), now, now)
		if err != nil {
			e.t.Fatal(err)
		}
		uid, _ := res.LastInsertId()
		e.exec(`INSERT INTO class_member_tab (public_id, class_id, user_id, role, joined_at, created_at, updated_at) SELECT ?, id, ?, 'student', ?, ?, ? FROM class_tab WHERE public_id = ?`,
			ulid.New(e.now()), uid, now, now, now, cls)
	}
}

// seedInvite inserts an invite straight into the database, expiring after life (negative = already expired), and returns its public
// id and token.
func (e *env) seedInvite(cls string, life time.Duration) (id, token string) {
	e.t.Helper()
	token, hash, err := newInviteToken()
	if err != nil {
		e.t.Fatal(err)
	}
	id = ulid.New(e.now())
	now := e.now().UnixMilli()
	e.exec(`INSERT INTO class_invite_tab (public_id, class_id, token_hash, created_by, expires_at, created_at, updated_at) SELECT ?, id, ?, created_by, ?, ?, ? FROM class_tab WHERE public_id = ?`,
		id, hash[:], e.now().Add(life).UnixMilli(), now, now, cls)
	return id, token
}

// expireAll makes every invite of the database expired a minute ago, without moving the clock (sessions and limiters use it too).
func (e *env) expireAll() {
	e.t.Helper()
	e.exec(`UPDATE class_invite_tab SET expires_at = ?`, e.now().Add(-time.Minute).UnixMilli())
}

func (e *env) createInvite(p person, cls string) (id, token string) {
	e.t.Helper()
	d := e.do(p, "POST", "/api/class-invite/create", body("class_id", cls)).status(200, "OK").data()
	return d["id"].(string), d["token"].(string)
}

func tokenBody(token string) string { return body("token", token) }

func (e *env) lookup(token string) resp {
	e.t.Helper()
	return e.do(anonymous, "POST", "/api/class-invite/lookup", tokenBody(token))
}

func (e *env) accept(p person, token string) resp {
	e.t.Helper()
	return e.do(p, "POST", "/api/class-invite/accept", tokenBody(token))
}

func (e *env) role(p person, cls string) string {
	e.t.Helper()
	var role string
	err := e.db.QueryRow(`SELECT m.role FROM class_member_tab m JOIN class_tab c ON c.id = m.class_id WHERE c.public_id = ? AND m.user_id = ?`, cls, p.row).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

func (e *env) members(cls string) int {
	return e.count(`SELECT COUNT(*) FROM class_member_tab m JOIN class_tab c ON c.id = m.class_id WHERE c.public_id = ?`, cls)
}

// The whole story over HTTP: an admin makes a link, a visitor previews it, a new user joins, the admin sees the roster.
func TestInviteLifecycle(t *testing.T) {
	e := newEnv(t)
	admin, kid := e.user("Admin"), e.user("Kid")
	cls := e.create(admin)

	created := e.do(admin, "POST", "/api/class-invite/create", body("class_id", cls)).status(200, "OK").data()
	token := created["token"].(string)
	if len(token) != tokenLen || created["link"] != baseURL+"/join/"+token {
		t.Fatalf("token %q link %v", token, created["link"])
	}
	if exp := int64(created["expires_at"].(float64)); exp != e.now().Add(DefaultInviteLifetime).UnixMilli() {
		t.Fatalf("expires_at %d, want 14 days", exp)
	}
	if created["active"] != true || created["revoked_at"] != nil {
		t.Fatalf("created: %v", created)
	}

	// the list never shows the token or the hash
	list := e.do(admin, "GET", "/api/class-invite/get-list?class_id="+cls, "").status(200, "OK")
	if strings.Contains(list.Body.String(), token) || strings.Contains(list.Body.String(), "token") || strings.Contains(list.Body.String(), "link") {
		t.Fatalf("the list carries the secret: %s", list.Body)
	}
	if items := list.data()["items"].([]any); len(items) != 1 || items[0].(map[string]any)["id"] != created["id"] {
		t.Fatalf("list: %v", items)
	}

	// the public preview: the class name and school, nothing else, and no session needed
	pv := e.lookup(token).status(200, "OK").data()
	if len(pv) != 2 || pv["name"] != "Lớp 12A1" || pv["school_name"] != "THPT Chu Văn An" {
		t.Fatalf("preview: %v", pv)
	}

	joined := e.accept(kid, token).status(200, "OK").data()
	if joined["id"] != cls || joined["my_role"] != RoleStudent {
		t.Fatalf("accept: %v", joined)
	}
	if e.role(kid, cls) != RoleStudent {
		t.Fatal("the joiner is not a student")
	}
	roster := e.do(admin, "GET", "/api/class-member/get-list?class_id="+cls, "").status(200, "OK").data()["items"].([]any)
	if len(roster) != 2 {
		t.Fatalf("roster %v", roster)
	}
	// the stored row holds the hash and no token (AC5: stored column)
	var hash []byte
	if err := e.db.QueryRow(`SELECT token_hash FROM class_invite_tab`).Scan(&hash); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(token))
	if hex.EncodeToString(hash) != hex.EncodeToString(sum[:]) {
		t.Fatal("token_hash is not the SHA-256 of the token")
	}
	for _, col := range []string{"public_id", "CAST(created_by AS CHAR)", "CAST(expires_at AS CHAR)", "HEX(token_hash)"} {
		if n := e.count(`SELECT COUNT(*) FROM class_invite_tab WHERE LOCATE(?, `+col+`) > 0`, token); n != 0 {
			t.Errorf("the token is stored in %s", col)
		}
	}
}

// AC1 (invites rows of the matrix): create, list and revoke are admin only; a student is 403, a non-member 404 identical to "no such
// class", an anonymous caller 401. Accept and lookup have their own test.
func TestInviteMatrix(t *testing.T) {
	e := newEnv(t)
	admin, student, stranger, outsider := e.user("Admin"), e.user("Student"), e.user("Stranger"), e.user("Outsider")
	e.seedClass([]person{outsider})
	type fixture struct{ class, invite string }
	for _, ep := range []struct {
		name string
		call func(p person, f fixture) resp
	}{
		{"class-invite/create", func(p person, f fixture) resp {
			return e.do(p, "POST", "/api/class-invite/create", body("class_id", f.class))
		}},
		{"class-invite/get-list", func(p person, f fixture) resp {
			return e.do(p, "GET", "/api/class-invite/get-list?class_id="+f.class, "")
		}},
		{"class-invite/revoke", func(p person, f fixture) resp {
			return e.do(p, "POST", "/api/class-invite/revoke", body("class_id", f.class, "id", f.invite))
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
				inv, _ := e.seedInvite(cls, time.Hour)
				r := ep.call(a.who, fixture{cls, inv})
				if r.Code != a.status || r.code() != a.code {
					t.Fatalf("got %d %s, want %d %s", r.Code, r.Body, a.status, a.code)
				}
				if a.sameAsAbsent {
					gone := ep.call(a.who, fixture{"01K7Q8W3ZR5N2M4X6V8B0C1D2E", inv})
					if r.sameAnswer() != gone.sameAnswer() {
						t.Fatalf("differs from the answer for a class that does not exist:\n%s\n%s", r.Body, gone.Body)
					}
				}
				if a.status != 200 { // a refused caller changed nothing in this class
					if n := e.count(`SELECT COUNT(*) FROM class_invite_tab i JOIN class_tab c ON c.id = i.class_id WHERE c.public_id = ? AND i.revoked_at IS NOT NULL`, cls); n != 0 {
						t.Fatalf("a refused request revoked %d invites", n)
					}
					if n := e.count(`SELECT COUNT(*) FROM class_invite_tab i JOIN class_tab c ON c.id = i.class_id WHERE c.public_id = ? AND i.public_id <> ?`, cls, inv); n != 0 {
						t.Fatalf("a refused request made %d invites", n)
					}
				}
			})
		}
	}
}

// The accept and lookup rows: anyone signed in with a verified email may join (a stranger, a member of another class), an
// anonymous caller is 401 on accept and may only preview.
func TestInviteAcceptMatrix(t *testing.T) {
	e := newEnv(t)
	admin, student := e.user("Admin"), e.user("Student")
	stranger, outsider := e.user("Stranger"), e.user("Outsider")
	unverified := e.userWith("Unverified", false)
	noTerms := e.user("NoTerms")
	e.exec(`UPDATE user_tab SET terms_version = NULL, terms_accepted_at = NULL WHERE id = ?`, noTerms.row)
	e.seedClass([]person{outsider})
	cls, _ := e.seedClass([]person{admin}, student)
	_, token := e.seedInvite(cls, time.Hour)

	e.accept(anonymous, token).status(401, "ERROR_UNAUTHORIZED")
	e.accept(unverified, token).status(403, "ERROR_EMAIL_NOT_VERIFIED")
	e.accept(noTerms, token).status(403, "ERROR_TERMS_REQUIRED")
	if n := e.members(cls); n != 2 {
		t.Fatalf("refused accepts added members (%d)", n)
	}
	e.accept(stranger, token).status(200, "OK")
	e.accept(outsider, token).status(200, "OK")
	if e.role(stranger, cls) != RoleStudent || e.role(outsider, cls) != RoleStudent || e.members(cls) != 4 {
		t.Fatal("the joiners are not students of the class")
	}
	// the outsider keeps being admin of their own class
	if got := e.do(outsider, "GET", "/api/class/get-list", "").data()["items"].([]any); len(got) != 2 {
		t.Fatalf("outsider's classes: %v", got)
	}
}

// AC2: an unknown, a malformed, an expired and a revoked token get the same answer on lookup and on accept, byte for byte apart
// from the request id; so does a caller who already belongs to the class.
func TestInviteDeadTokensLookAlike(t *testing.T) {
	e := newEnv(t)
	admin, kid, member := e.user("Admin"), e.user("Kid"), e.user("Member")
	cls, _ := e.seedClass([]person{admin}, member)
	_, expired := e.seedInvite(cls, -time.Minute)
	revID, revoked := e.seedInvite(cls, time.Hour)
	e.do(admin, "POST", "/api/class-invite/revoke", body("class_id", cls, "id", revID)).status(200, "OK")
	unknown, _, _ := newInviteToken()

	tokens := map[string]string{
		"unknown": unknown, "expired": expired, "revoked": revoked,
		"empty": "", "short": "abc", "long": strings.Repeat("a", 200), "bad alphabet": strings.Repeat("!", tokenLen), "unicode": strings.Repeat("é", tokenLen),
	}
	for who, p := range map[string]person{"new user": kid, "member": member, "admin": admin} {
		var want string
		for name, tok := range tokens {
			r := e.accept(p, tok).status(404, "ERROR_NOT_FOUND")
			if want == "" {
				want = r.sameAnswer()
			}
			if got := r.sameAnswer(); got != want {
				t.Errorf("accept %s as %s: %s, want %s", name, who, got, want)
			}
		}
	}
	var want string
	for name, tok := range tokens {
		r := e.lookup(tok).status(404, "ERROR_NOT_FOUND")
		if want == "" {
			want = r.sameAnswer()
		}
		if got := r.sameAnswer(); got != want {
			t.Errorf("lookup %s: %s, want %s", name, got, want)
		}
	}
	if !strings.Contains(want, "invite not found") {
		t.Errorf("the dead-token answer: %s", want)
	}
	if e.members(cls) != 2 {
		t.Fatal("a dead token added a member")
	}
	// a link that is alive when made, dies when it expires (the boundary itself is dead: expires_at is the first moment it is not valid)
	soonID, soon := e.createInvite(admin, cls)
	e.lookup(soon).status(200, "OK")
	e.exec(`UPDATE class_invite_tab SET expires_at = ? WHERE public_id = ?`, e.now().UnixMilli(), soonID)
	e.lookup(soon).status(404, "ERROR_NOT_FOUND")
	e.accept(kid, soon).status(404, "ERROR_NOT_FOUND")
}

// Revoking: the link stops at once, those who joined stay, revoking twice is fine; removing a member does not revoke the link.
func TestInviteRevokeAndRemove(t *testing.T) {
	e := newEnv(t)
	admin, a, b := e.user("Admin"), e.user("A"), e.user("B")
	cls := e.create(admin)
	id, token := e.createInvite(admin, cls)
	e.accept(a, token).status(200, "OK")
	members := e.do(admin, "GET", "/api/class-member/get-list?class_id="+cls, "").data()["items"].([]any)
	var aMember string
	for _, m := range members {
		if m.(map[string]any)["role"] == RoleStudent {
			aMember = m.(map[string]any)["id"].(string)
		}
	}
	e.do(admin, "POST", "/api/class-member/remove", body("class_id", cls, "member_id", aMember)).status(200, "OK")
	e.lookup(token).status(200, "OK") // removing a member did not revoke the link
	e.accept(a, token).status(200, "OK")

	e.do(admin, "POST", "/api/class-invite/revoke", body("class_id", cls, "id", id)).status(200, "OK")
	revokedAt := func() (at int64) {
		if err := e.db.QueryRow(`SELECT revoked_at FROM class_invite_tab`).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	first := revokedAt()
	e.advance(time.Minute)
	e.do(admin, "POST", "/api/class-invite/revoke", body("class_id", cls, "id", id)).status(200, "OK") // again
	if second := revokedAt(); first == 0 || first != second {
		t.Fatalf("revoked_at changed on the second revoke: %d then %d", first, second)
	}
	e.lookup(token).status(404, "ERROR_NOT_FOUND")
	e.accept(b, token).status(404, "ERROR_NOT_FOUND")
	if e.role(a, cls) != RoleStudent { // revoking removes nobody
		t.Fatal("revoking the link removed a member")
	}
	items := e.do(admin, "GET", "/api/class-invite/get-list?class_id="+cls, "").data()["items"].([]any)
	if it := items[0].(map[string]any); it["active"] != false || it["revoked_at"] == nil {
		t.Fatalf("listed as %v", it)
	}
}

// AC3: the 6th active invite is refused (revoked and expired ones do not count), also when the creates arrive together.
func TestInviteCap(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls := e.create(admin)
	var ids []string
	for range MaxActiveInvites {
		id, _ := e.createInvite(admin, cls)
		ids = append(ids, id)
	}
	e.do(admin, "POST", "/api/class-invite/create", body("class_id", cls)).status(409, "ERROR_LIMIT_REACHED")
	e.do(admin, "POST", "/api/class-invite/revoke", body("class_id", cls, "id", ids[0])).status(200, "OK")
	e.createInvite(admin, cls) // the revoked one freed a slot
	e.do(admin, "POST", "/api/class-invite/create", body("class_id", cls)).status(409, "ERROR_LIMIT_REACHED")
	e.expireAll()
	for range MaxActiveInvites {
		e.createInvite(admin, cls)
	}
	e.do(admin, "POST", "/api/class-invite/create", body("class_id", cls)).status(409, "ERROR_LIMIT_REACHED")
	if n := e.count(`SELECT COUNT(*) FROM class_invite_tab`); n != 2*MaxActiveInvites+1 {
		t.Fatalf("%d invite rows", n)
	}

	racer := e.user("Racer")
	cls2 := e.create(racer)
	got := tally(parallel(MaxActiveInvites+5, func(int) resp {
		return e.do(racer, "POST", "/api/class-invite/create", body("class_id", cls2))
	}))
	if got["OK"] != MaxActiveInvites || got["ERROR_LIMIT_REACHED"] != 5 {
		t.Fatalf("concurrent creates: %v", got)
	}
}

// AC3: the 61st member is refused with ERROR_CLASS_FULL, a member of a full class still gets their membership back, and nobody's
// role changes.
func TestInviteClassFull(t *testing.T) {
	e := newEnv(t)
	admin, member, late := e.user("Admin"), e.user("Member"), e.user("Late")
	cls, _ := e.seedClass([]person{admin}, member)
	e.fill(cls, MaxMembers-2)
	if e.members(cls) != MaxMembers {
		t.Fatalf("%d members", e.members(cls))
	}
	_, token := e.seedInvite(cls, time.Hour)
	e.accept(late, token).status(409, "ERROR_CLASS_FULL")
	if e.members(cls) != MaxMembers || e.role(late, cls) != "" {
		t.Fatal("the 61st member got in")
	}
	e.accept(member, token).status(200, "OK")
	if d := e.accept(admin, token).status(200, "OK").data(); d["my_role"] != RoleAdmin {
		t.Fatalf("admin accepting is %v", d["my_role"])
	}
	e.lookup(token).status(200, "OK") // the preview does not say the class is full
	// a place opens, the same link works again
	e.do(admin, "POST", "/api/class-member/remove", body("class_id", cls, "member_id", e.memberID(cls, member))).status(200, "OK")
	e.accept(late, token).status(200, "OK")
}

func (e *env) memberID(cls string, p person) string {
	e.t.Helper()
	var id string
	if err := e.db.QueryRow(`SELECT m.public_id FROM class_member_tab m JOIN class_tab c ON c.id = m.class_id WHERE c.public_id = ? AND m.user_id = ?`, cls, p.row).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// AC3: accepting is idempotent and never changes a role.
func TestInviteAcceptIsIdempotent(t *testing.T) {
	e := newEnv(t)
	admin, coAdmin, kid := e.user("Admin"), e.user("CoAdmin"), e.user("Kid")
	cls, _ := e.seedClass([]person{admin, coAdmin})
	_, token := e.seedInvite(cls, time.Hour)
	for range 3 {
		if d := e.accept(admin, token).status(200, "OK").data(); d["my_role"] != RoleAdmin || d["id"] != cls {
			t.Fatalf("admin: %v", d)
		}
		if d := e.accept(kid, token).status(200, "OK").data(); d["my_role"] != RoleStudent {
			t.Fatalf("kid: %v", d)
		}
	}
	if e.members(cls) != 3 || e.role(admin, cls) != RoleAdmin {
		t.Fatalf("members %d, admin's role %q", e.members(cls), e.role(admin, cls))
	}
	// a promoted kid is not demoted by accepting again
	e.do(admin, "POST", "/api/class-member/update-role", body("class_id", cls, "member_id", e.memberID(cls, kid), "role", RoleAdmin)).status(200, "OK")
	if d := e.accept(kid, token).status(200, "OK").data(); d["my_role"] != RoleAdmin {
		t.Fatalf("a promoted member was changed by accept: %v", d)
	}
}

// AC4: ten people accept one link at the same moment into a class with room for one: exactly one gets in. Repeated, so the lock
// order is exercised, and no answer is a 5xx.
func TestInviteConcurrentAcceptsOfTheLastSeat(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	var kids []person
	for i := range 10 {
		kids = append(kids, e.user("Kid"+strconv.Itoa(i)))
	}
	for round := range 3 {
		cls, _ := e.seedClass([]person{admin})
		e.fill(cls, MaxMembers-2) // 59 members
		_, token := e.seedInvite(cls, time.Hour)
		got := tally(parallel(len(kids), func(i int) resp { return e.accept(kids[i], token) }))
		if got["OK"] != 1 || got["ERROR_CLASS_FULL"] != len(kids)-1 || e.members(cls) != MaxMembers {
			t.Fatalf("round %d: answers %v, %d members", round, got, e.members(cls))
		}
	}
	// the same people racing a revoke: every accept either joined before it or is refused, nothing joins after
	cls, _ := e.seedClass([]person{admin})
	id, token := e.seedInvite(cls, time.Hour)
	rs := parallel(len(kids)+1, func(i int) resp {
		if i == len(kids) {
			return e.do(admin, "POST", "/api/class-invite/revoke", body("class_id", cls, "id", id))
		}
		return e.accept(kids[i], token)
	})
	for _, r := range rs {
		if r.Code >= 500 {
			t.Fatalf("5xx: %s", r.Body)
		}
	}
	joined := e.members(cls) - 1
	e.accept(kids[0], token).status(404, "ERROR_NOT_FOUND")
	if after := e.members(cls) - 1; after != joined {
		t.Fatalf("someone joined after the revoke (%d, then %d)", joined, after)
	}
}

// T-026 QA, carried over: the 10-admin-classes cap also holds when a member is promoted, also under concurrency (two classes
// promoting the same user, and a promotion racing class/create).
func TestPromotionHonoursTheAdminClassCap(t *testing.T) {
	e := newEnv(t)
	boss, u := e.user("Boss"), e.user("U")
	for range maxAdminClasses {
		e.seedClass([]person{u})
	}
	cls, _ := e.seedClass([]person{boss}, u)
	promote := func(who person, cls string, to string) resp {
		return e.do(who, "POST", "/api/class-member/update-role", body("class_id", cls, "member_id", e.memberID(cls, u), "role", to))
	}
	promote(boss, cls, RoleAdmin).status(409, "ERROR_LIMIT_REACHED")
	if e.role(u, cls) != RoleStudent {
		t.Fatal("the refused promotion changed the role")
	}
	e.do(u, "POST", "/api/class/create", classBody).status(409, "ERROR_LIMIT_REACHED")
	promote(boss, cls, RoleStudent).status(200, "OK") // a no-op demotion of a student is not blocked
	// one admin class fewer, and the promotion goes through
	e.exec(`DELETE FROM class_member_tab WHERE user_id = ? AND role = 'admin' LIMIT 1`, u.row)
	promote(boss, cls, RoleAdmin).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM class_member_tab WHERE user_id = ? AND role = 'admin'`, u.row); n != maxAdminClasses {
		t.Fatalf("%d admin classes", n)
	}

	for round := range 4 {
		racer := e.user("Racer" + strconv.Itoa(round))
		for range maxAdminClasses - 1 {
			e.seedClass([]person{racer})
		}
		a1, a2 := e.user("A1"), e.user("A2")
		c1, _ := e.seedClass([]person{a1}, racer)
		c2, _ := e.seedClass([]person{a2}, racer)
		rs := parallel(3, func(i int) resp {
			switch i {
			case 0:
				return e.do(a1, "POST", "/api/class-member/update-role", body("class_id", c1, "member_id", e.memberID(c1, racer), "role", RoleAdmin))
			case 1:
				return e.do(a2, "POST", "/api/class-member/update-role", body("class_id", c2, "member_id", e.memberID(c2, racer), "role", RoleAdmin))
			}
			return e.do(racer, "POST", "/api/class/create", classBody)
		})
		got := tally(rs)
		if got["OK"] != 1 || got["ERROR_LIMIT_REACHED"] != 2 {
			t.Fatalf("round %d: %v", round, got)
		}
		if n := e.count(`SELECT COUNT(*) FROM class_member_tab WHERE user_id = ? AND role = 'admin'`, racer.row); n != maxAdminClasses {
			t.Fatalf("round %d: %d admin classes", round, n)
		}
	}
}

// T-026 QA, carried over: an admin of class A who names a member id (or an invite id) of class B gets 404 and nothing changes.
func TestCrossClassIDsChangeNothing(t *testing.T) {
	e := newEnv(t)
	adminA, adminB, victim := e.user("AdminA"), e.user("AdminB"), e.user("Victim")
	clsA, _ := e.seedClass([]person{adminA})
	clsB, _ := e.seedClass([]person{adminB}, victim)
	victimMember := e.memberID(clsB, victim)
	inviteB, tokenB := e.seedInvite(clsB, time.Hour)

	e.do(adminA, "POST", "/api/class-member/update-role", body("class_id", clsA, "member_id", victimMember, "role", RoleAdmin)).status(404, "ERROR_NOT_FOUND")
	e.do(adminA, "POST", "/api/class-member/remove", body("class_id", clsA, "member_id", victimMember)).status(404, "ERROR_NOT_FOUND")
	e.do(adminA, "POST", "/api/class-invite/revoke", body("class_id", clsA, "id", inviteB)).status(404, "ERROR_NOT_FOUND")
	if e.role(victim, clsB) != RoleStudent || e.members(clsB) != 2 {
		t.Fatal("a cross-class member id changed class B")
	}
	if n := e.count(`SELECT COUNT(*) FROM class_invite_tab WHERE revoked_at IS NOT NULL`); n != 0 {
		t.Fatal("a cross-class invite id revoked an invite of class B")
	}
	e.lookup(tokenB).status(200, "OK")
	// the answer for a member id of another class is the answer for one that exists nowhere
	ghost := "01K7Q8W3ZR5N2M4X6V8B0C1D2E"
	a := e.do(adminA, "POST", "/api/class-member/remove", body("class_id", clsA, "member_id", victimMember))
	b := e.do(adminA, "POST", "/api/class-member/remove", body("class_id", clsA, "member_id", ghost))
	if a.sameAnswer() != b.sameAnswer() {
		t.Fatalf("cross-class member id differs from an unknown one:\n%s\n%s", a.Body, b.Body)
	}
	c := e.do(adminA, "POST", "/api/class-invite/revoke", body("class_id", clsA, "id", inviteB))
	d := e.do(adminA, "POST", "/api/class-invite/revoke", body("class_id", clsA, "id", ghost))
	if c.sameAnswer() != d.sameAnswer() {
		t.Fatalf("cross-class invite id differs from an unknown one:\n%s\n%s", c.Body, d.Body)
	}
}

// AC5: the token is in no log line, no error body and no metric, whatever happens to it.
func TestInviteTokenIsNeverLogged(t *testing.T) {
	e := newEnv(t)
	admin, kid, other := e.user("Admin"), e.user("Kid"), e.user("Other")
	cls := e.create(admin)
	id, token := e.createInvite(admin, cls)
	sum := sha256.Sum256([]byte(token))
	var seen []string
	note := func(r resp) { seen = append(seen, r.Body.String()) }
	note(e.lookup(token))
	note(e.lookup(token + "x"))
	note(e.accept(kid, token))
	note(e.accept(kid, token))
	note(e.accept(kid, strings.ToLower(token)))
	note(e.do(anonymous, "POST", "/api/class-invite/lookup", `{"token":"`+token+`","extra":1}`)) // a 400
	note(e.do(anonymous, "POST", "/api/class-invite/lookup", `{"token":`+token+`}`))             // not JSON: a 400
	note(e.do(admin, "GET", "/api/class-invite/get-list?class_id="+cls, ""))
	note(e.do(admin, "POST", "/api/class-invite/revoke", body("class_id", cls, "id", id)))
	note(e.lookup(token))
	note(e.accept(other, token))
	for i, b := range seen {
		if strings.Contains(b, token) {
			t.Errorf("answer %d echoes the token: %s", i, b)
		}
	}
	logs := e.logs.String()
	if logs == "" {
		t.Fatal("no log output captured: the check would pass for nothing")
	}
	for _, secret := range []string{token, hex.EncodeToString(sum[:]), url.QueryEscape(token)} {
		if strings.Contains(logs, secret) {
			t.Errorf("a log line contains %q", secret)
		}
	}
	if strings.Contains(logs, "/join/") {
		t.Error("a log line contains an invite path")
	}
}

// Validation of the expiry and of the shape of every request.
func TestInviteValidation(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls := e.create(admin)
	now := e.now()
	ms := func(d time.Duration) string { return strconv.FormatInt(now.Add(d).UnixMilli(), 10) }
	for name, tc := range map[string]struct{ body, want string }{
		"in the past":      {`{"class_id":"` + cls + `","expires_at":` + ms(-time.Second) + `}`, "ERROR_INVALID_EXPIRES_AT"},
		"now":              {`{"class_id":"` + cls + `","expires_at":` + ms(0) + `}`, "ERROR_INVALID_EXPIRES_AT"},
		"zero":             {`{"class_id":"` + cls + `","expires_at":0}`, "ERROR_INVALID_EXPIRES_AT"},
		"31 days":          {`{"class_id":"` + cls + `","expires_at":` + ms(MaxInviteLifetime+time.Second) + `}`, "ERROR_INVALID_EXPIRES_AT"},
		"a string":         {`{"class_id":"` + cls + `","expires_at":"soon"}`, "ERROR_PARAM"},
		"unknown field":    {`{"class_id":"` + cls + `","role":"admin"}`, "ERROR_PARAM"},
		"missing class id": {`{}`, "ERROR_PARAM"},
		"lower-case id":    {`{"class_id":"` + strings.ToLower(cls) + `"}`, "ERROR_PARAM"},
	} {
		t.Run(name, func(t *testing.T) {
			e.do(admin, "POST", "/api/class-invite/create", tc.body).status(400, tc.want)
		})
	}
	if n := e.count(`SELECT COUNT(*) FROM class_invite_tab`); n != 0 {
		t.Fatalf("%d invites after refused creates", n)
	}
	d := e.do(admin, "POST", "/api/class-invite/create", `{"class_id":"`+cls+`","expires_at":`+ms(MaxInviteLifetime)+`}`).status(200, "OK").data()
	if int64(d["expires_at"].(float64)) != now.Add(MaxInviteLifetime).UnixMilli() {
		t.Fatalf("expires_at %v", d["expires_at"])
	}
	e.do(admin, "POST", "/api/class-invite/create", `{"class_id":"`+cls+`","expires_at":`+ms(time.Hour)+`}`).status(200, "OK")

	for _, id := range []string{"", strings.ToLower(cls), cls[:25], cls + "A"} {
		e.do(admin, "GET", "/api/class-invite/get-list?class_id="+url.QueryEscape(id), "").status(400, "ERROR_PARAM")
		e.do(admin, "POST", "/api/class-invite/revoke", body("class_id", cls, "id", id)).status(400, "ERROR_PARAM")
		e.do(admin, "POST", "/api/class-invite/revoke", body("class_id", id, "id", "01K7Q8W3ZR5N2M4X6V8B0C1D2E")).status(400, "ERROR_PARAM")
	}
	// a body that is not a token request is a plain 400, never a hint about tokens
	for _, b := range []string{``, `[]`, `{"token":1}`, `{"tokens":"x"}`, `{"token":"x"} {}`} {
		e.do(anonymous, "POST", "/api/class-invite/lookup", b).status(400, "ERROR_PARAM")
		e.do(admin, "POST", "/api/class-invite/accept", b).status(400, "ERROR_PARAM")
	}
	e.lookup("").status(404, "ERROR_NOT_FOUND")
	e.do(anonymous, "POST", "/api/class-invite/lookup", `{}`).status(404, "ERROR_NOT_FOUND")
}

// Paging: the list is newest first, the cursor is bound to the class and the caller.
func TestInvitePaging(t *testing.T) {
	e := newEnv(t)
	admin, co := e.user("Admin"), e.user("Co")
	cls, _ := e.seedClass([]person{admin, co})
	other, _ := e.seedClass([]person{admin})
	var want []string
	for range 4 {
		id, _ := e.seedInvite(cls, -time.Hour) // expired ones are listed too
		want = append([]string{id}, want...)
	}
	var got []string
	next := ""
	for pages := 0; ; pages++ {
		r := e.do(admin, "GET", "/api/class-invite/get-list?class_id="+cls+"&limit=3&next_id="+next, "").status(200, "OK").data()
		for _, it := range r["items"].([]any) {
			m := it.(map[string]any)
			got = append(got, m["id"].(string))
			if m["active"] != false {
				t.Errorf("expired invite listed as active: %v", m)
			}
		}
		if next = r["next_id"].(string); next == "" {
			break
		}
		e.do(admin, "GET", "/api/class-invite/get-list?class_id="+other+"&next_id="+next, "").status(400, "ERROR_PARAM")
		e.do(co, "GET", "/api/class-invite/get-list?class_id="+cls+"&next_id="+next, "").status(400, "ERROR_PARAM")
		if pages > 3 {
			t.Fatal("never ends")
		}
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("walk %v, want %v", got, want)
	}
	e.do(admin, "GET", "/api/class-invite/get-list?class_id="+cls+"&limit=51", "").status(400, "ERROR_PARAM")
}

// Rate limits: a client that keeps missing is slowed down on lookup, and accepting is limited per IP as well as per user.
func TestInviteRateLimits(t *testing.T) {
	e := newEnv(t)
	admin, kid := e.user("Admin"), e.user("Kid")
	cls := e.create(admin)
	_, token := e.createInvite(admin, cls)
	for range lookupMissesPerWindow {
		tok, _, _ := newInviteToken()
		e.lookup(tok).status(404, "ERROR_NOT_FOUND")
	}
	tok, _, _ := newInviteToken()
	if r := e.lookup(tok).status(429, "ERROR_RATE_LIMITED"); r.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.advance(lookupWindow + time.Second)
	e.lookup(token).status(200, "OK")
	for range 3 * lookupMissesPerWindow { // good lookups do not use up the guessing budget
		e.lookup(token).status(200, "OK")
	}

	for range acceptPerIPWindow {
		tok, _, _ := newInviteToken()
		e.accept(kid, tok).status(404, "ERROR_NOT_FOUND")
	}
	e.accept(kid, token).status(429, "ERROR_RATE_LIMITED")
	if e.role(kid, cls) != "" {
		t.Fatal("a rate-limited accept joined")
	}
}

// Deleting a class removes its invites with it; nothing dangles.
func TestDeletingTheClassRemovesItsInvites(t *testing.T) {
	e := newEnv(t)
	admin := e.user("Admin")
	cls := e.create(admin)
	_, token := e.createInvite(admin, cls)
	keep := e.create(admin)
	e.createInvite(admin, keep)
	e.do(admin, "POST", "/api/class/delete", body("id", cls)).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM class_invite_tab`); n != 1 {
		t.Fatalf("%d invites left, want the other class's one", n)
	}
	e.lookup(token).status(404, "ERROR_NOT_FOUND")
	dbtest.AssertNoOrphans(t, e.db)
}
