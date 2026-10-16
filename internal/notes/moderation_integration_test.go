//go:build integration

package notes

import (
	"context"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// seed submits n notes (the i-th named "N<i>") through the public link, with photos[i] photos each, and returns their public
// ids in submission order.
func (e *env) seed(l link, n int, photos func(i int) int) []string {
	e.t.Helper()
	for i := range n {
		if i > 0 && i%50 == 0 {
			e.advance(2 * time.Hour) // the per-link hourly submit limit is 60
		}
		var parts []part
		parts = append(parts, answers(fmt.Sprintf("N%d", i), "hello"))
		if photos != nil {
			for j := range photos(i) {
				parts = append(parts, photoPart(jpegOf(e.t, uint8(10*i+j+1))))
			}
		}
		e.postFrom(context.Background(), fmt.Sprintf("198.51.100.%d", i%250+1), l.token, parts...).status(200, "")
	}
	rows, err := e.db.Query(`SELECT n.public_id FROM note_tab n JOIN yearbook_tab y ON y.id = n.yearbook_id WHERE y.public_id = ? ORDER BY n.id`, l.book)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			e.t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func idsJSON(ids []string) string {
	q := make([]string, len(ids))
	for i, id := range ids {
		q[i] = fmt.Sprintf("%q", id)
	}
	return "[" + strings.Join(q, ",") + "]"
}

func (e *env) noteList(u user, book, query string) resp {
	e.t.Helper()
	return e.do(u, "GET", "/api/note/get-list?yearbook_id="+url.QueryEscape(book)+query, "")
}

func (e *env) setStatus(u user, book string, ids []string, status string) resp {
	e.t.Helper()
	return e.do(u, "POST", "/api/note/batch-update-status", fmt.Sprintf(`{"yearbook_id":%q,"ids":%s,"status":%q}`, book, idsJSON(ids), status))
}

func (e *env) reorder(u user, book string, ids []string) resp {
	e.t.Helper()
	return e.do(u, "POST", "/api/note/reorder", fmt.Sprintf(`{"yearbook_id":%q,"ids":%s}`, book, idsJSON(ids)))
}

func (e *env) deleteNotes(u user, book string, ids []string) resp {
	e.t.Helper()
	return e.do(u, "POST", "/api/note/batch-delete", fmt.Sprintf(`{"yearbook_id":%q,"ids":%s}`, book, idsJSON(ids)))
}

// listed returns the ids of the items in a list response.
func listed(r resp) []string {
	var out []string
	for _, it := range r.data()["items"].([]any) {
		out = append(out, it.(map[string]any)["id"].(string))
	}
	return out
}

func (e *env) counts(r resp) [4]int {
	e.t.Helper()
	c := r.data()["counts"].(map[string]any)
	return [4]int{int(c["all"].(float64)), int(c["pending"].(float64)), int(c["approved"].(float64)), int(c["hidden"].(float64))}
}

// positions returns the status and sort_order of every note of a book, in sort_order then id order.
func (e *env) approvedOrder(book string) []string {
	e.t.Helper()
	rows, err := e.db.Query(`SELECT n.public_id, n.sort_order FROM note_tab n JOIN yearbook_tab y ON y.id = n.yearbook_id
		WHERE y.public_id = ? AND n.status = 'approved' ORDER BY n.sort_order, n.id`, book)
	if err != nil {
		e.t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	want := 1
	for rows.Next() {
		var id string
		var pos *int
		if err := rows.Scan(&id, &pos); err != nil {
			e.t.Fatal(err)
		}
		if pos == nil || *pos != want {
			e.t.Fatalf("approved note %s has sort_order %v, want %d (dense 1..n)", id, pos, want)
		}
		want++
		out = append(out, id)
	}
	if n := e.count(`SELECT COUNT(*) FROM note_tab WHERE status <> 'approved' AND sort_order IS NOT NULL`); n != 0 {
		e.t.Fatalf("%d notes outside the approved set have a sort_order", n)
	}
	return out
}

// AC1: filters, counts, order, paging, cursors, access.
func TestNoteList(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 5, func(i int) int { return i % 3 })
	e.setStatus(l.owner, l.book, []string{ids[3], ids[1], ids[0]}, StatusApproved).status(200, "")
	e.setStatus(l.owner, l.book, []string{ids[4]}, StatusHidden).status(200, "")

	all := e.noteList(l.owner, l.book, "").status(200, "")
	if c := e.counts(all); c != [4]int{5, 1, 3, 1} {
		t.Fatalf("counts %v, want all 5, pending 1, approved 3, hidden 1", c)
	}
	if got := listed(all); !slices.Equal(got, []string{ids[4], ids[3], ids[2], ids[1], ids[0]}) {
		t.Fatalf("all is newest first, got %v", got)
	}
	approved := e.noteList(l.owner, l.book, "&status=approved").status(200, "")
	if got := listed(approved); !slices.Equal(got, []string{ids[3], ids[1], ids[0]}) {
		t.Fatalf("approved is in book order, got %v", got)
	}
	first := approved.data()["items"].([]any)[0].(map[string]any)
	if first["position"] != float64(1) || first["status"] != "approved" || first["link_label"] != "" || first["created_at"] == nil {
		t.Fatalf("unexpected item %v", first)
	}
	if a, ok := first["answers"].(map[string]any); !ok || a["name"] != "N3" || a["message"] != "hello" {
		t.Fatalf("answers not returned as stored: %v", first["answers"])
	}
	if ph := first["photos"].([]any); len(ph) != 0 {
		t.Fatalf("N3 has no photo, got %v", ph)
	}
	one := approved.data()["items"].([]any)[1].(map[string]any) // N1: one photo
	if ph := one["photos"].([]any); len(ph) != 1 || len(ph[0].(map[string]any)["id"].(string)) != 26 || len(ph[0].(map[string]any)) != 1 {
		t.Fatalf("photos must be ids only, got %v", one["photos"])
	}
	if strings.Contains(approved.Body.String(), "yearbooks/") || strings.Contains(approved.Body.String(), "object_key") {
		t.Fatal("an object key leaked")
	}
	if p := e.noteList(l.owner, l.book, "&status=pending").status(200, "").data()["items"].([]any)[0].(map[string]any); p["position"] != nil {
		t.Fatalf("a pending note has no position, got %v", p["position"])
	}

	// paging: stable and complete, approved (book order) and the rest (newest first)
	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"&status=approved", []string{ids[3], ids[1], ids[0]}},
		{"", []string{ids[4], ids[3], ids[2], ids[1], ids[0]}},
	} {
		var got []string
		next := ""
		for range 10 {
			r := e.noteList(l.owner, l.book, tc.query+"&limit=2"+next).status(200, "")
			got = append(got, listed(r)...)
			n := r.data()["next_id"].(string)
			if n == "" {
				break
			}
			next = "&next_id=" + url.QueryEscape(n)
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("paging %q: %v, want %v", tc.query, got, tc.want)
		}
	}
	cur := e.noteList(l.owner, l.book, "&status=approved&limit=1").status(200, "").data()["next_id"].(string)
	e.noteList(l.owner, l.book, "&status=approved&limit=1&next_id="+url.QueryEscape(cur+"x")).status(400, "ERROR_PARAM")
	e.noteList(l.owner, l.book, "&status=hidden&limit=1&next_id="+url.QueryEscape(cur)).status(400, "ERROR_PARAM") // another list's cursor
	other := e.newOpenLink()
	e.noteList(other.owner, other.book, "&status=approved&limit=1&next_id="+url.QueryEscape(cur)).status(400, "ERROR_PARAM")
	e.noteList(l.owner, l.book, "&status=bogus").status(400, "ERROR_PARAM")
	e.noteList(l.owner, l.book, "&limit=101").status(400, "ERROR_PARAM")
	e.noteList(l.owner, "", "").status(400, "ERROR_PARAM")

	// access: another user's book is 404, no session is 401, an unverified account 403
	e.noteList(other.owner, l.book, "").status(404, "ERROR_NOT_FOUND")
	e.noteList(user{}, l.book, "").status(401, "ERROR_UNAUTHORIZED")
	bob := e.register("bob@example.com", "Bob", false)
	e.noteList(bob, e.book(bob, "B"), "").status(403, "ERROR_EMAIL_NOT_VERIFIED")
	if c := e.counts(e.noteList(l.owner, l.book, "&status=hidden").status(200, "")); c != [4]int{5, 1, 3, 1} {
		t.Fatalf("counts cover the whole book whatever the filter, got %v", c)
	}
}

// The link label comes from the collection the note arrived through.
func TestNoteListLinkLabel(t *testing.T) {
	e := newEnv(t)
	owner := e.register("a@example.com", "A", true)
	book := e.book(owner, "B")
	_, tok := e.newLink(owner, book, `{"label":"Café"}`)
	e.post(tok, answers("X", "hi")).status(200, "")
	it := e.noteList(owner, book, "").status(200, "").data()["items"].([]any)[0].(map[string]any)
	if it["link_label"] != "Café" || it["status"] != "pending" {
		t.Fatalf("unexpected %v", it)
	}
}

// T-066 QA: an unknown status in the table is an internal error, not a silent default.
func TestNoteListUnknownStatusIsInternal(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	e.seed(l, 2, nil)
	if _, err := e.db.Exec(`UPDATE note_tab SET status = 'weird' LIMIT 1`); err != nil {
		t.Fatal(err)
	}
	e.noteList(l.owner, l.book, "").status(500, "ERROR_INTERNAL")
}

// AC2: approve appends in the order given, an approved note keeps its place, hiding closes the gap, a foreign id changes nothing.
func TestBatchUpdateStatus(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 6, nil)

	r := e.setStatus(l.owner, l.book, []string{ids[2], ids[0], ids[1]}, StatusApproved).status(200, "")
	if c := r.data()["counts"].(map[string]any); c["approved"] != float64(3) || c["pending"] != float64(3) || c["all"] != float64(6) {
		t.Fatalf("counts %v", c)
	}
	if got := e.approvedOrder(l.book); !slices.Equal(got, []string{ids[2], ids[0], ids[1]}) {
		t.Fatalf("approved order %v", got)
	}
	// approving an approved note again does not move it; a new one goes to the end
	e.setStatus(l.owner, l.book, []string{ids[2], ids[3]}, StatusApproved).status(200, "")
	if got := e.approvedOrder(l.book); !slices.Equal(got, []string{ids[2], ids[0], ids[1], ids[3]}) {
		t.Fatalf("approved order %v", got)
	}
	// hiding one in the middle closes the gap
	e.setStatus(l.owner, l.book, []string{ids[0]}, StatusHidden).status(200, "")
	if got := e.approvedOrder(l.book); !slices.Equal(got, []string{ids[2], ids[1], ids[3]}) {
		t.Fatalf("approved order after hide %v", got)
	}
	// back to pending, and a hidden note approved again goes to the end
	e.setStatus(l.owner, l.book, []string{ids[2]}, StatusPending).status(200, "")
	e.setStatus(l.owner, l.book, []string{ids[0]}, StatusApproved).status(200, "")
	if got := e.approvedOrder(l.book); !slices.Equal(got, []string{ids[1], ids[3], ids[0]}) {
		t.Fatalf("approved order %v", got)
	}

	// all or nothing: one foreign id, one unknown id
	other := e.newOpenLink()
	foreign := e.seed(other, 1, nil)
	before := e.count(`SELECT SUM(updated_at) FROM note_tab`)
	e.setStatus(l.owner, l.book, []string{ids[4], foreign[0]}, StatusApproved).status(404, "ERROR_NOT_FOUND")
	e.setStatus(l.owner, l.book, []string{ids[4], strings.Repeat("0", 26)}, StatusApproved).status(404, "ERROR_NOT_FOUND")
	e.setStatus(other.owner, l.book, []string{ids[4]}, StatusApproved).status(404, "ERROR_NOT_FOUND") // not their book
	if e.count(`SELECT SUM(updated_at) FROM note_tab`) != before || len(e.approvedOrder(l.book)) != 3 {
		t.Fatal("a failed batch changed something")
	}

	// input rules
	e.setStatus(l.owner, l.book, ids[:1], "bogus").status(400, "ERROR_PARAM")
	e.setStatus(l.owner, l.book, nil, StatusHidden).status(400, "ERROR_PARAM")
	e.setStatus(l.owner, l.book, []string{ids[4], ids[4]}, StatusHidden).status(400, "ERROR_PARAM")
	e.setStatus(l.owner, l.book, []string{"short"}, StatusHidden).status(400, "ERROR_PARAM")
	big := make([]string, 101)
	for i := range big {
		big[i] = fmt.Sprintf("%026d", i)
	}
	e.setStatus(l.owner, l.book, big, StatusHidden).status(400, "ERROR_TOO_MANY_IDS")
	e.do(l.owner, "POST", "/api/note/batch-update-status", fmt.Sprintf(`{"yearbook_id":%q,"ids":[],"status":"hidden","x":1}`, l.book)).status(400, "ERROR_PARAM")
	e.setStatus(user{}, l.book, ids[:1], StatusHidden).status(401, "ERROR_UNAUTHORIZED")
}

// 100 ids is the largest batch.
func TestBatchUpdateStatusHundred(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 100, nil)
	e.setStatus(l.owner, l.book, ids, StatusApproved).status(200, "")
	if got := e.approvedOrder(l.book); !slices.Equal(got, ids) {
		t.Fatal("100 notes not approved in the given order")
	}
	e.setStatus(l.owner, l.book, ids[:50], StatusHidden).status(200, "")
	if got := e.approvedOrder(l.book); !slices.Equal(got, ids[50:]) {
		t.Fatal("positions did not close up")
	}
}

// AC3: a valid permutation applies, a stale set is 409 and changes nothing, positions stay dense.
func TestReorder(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 5, nil)
	e.setStatus(l.owner, l.book, ids[:4], StatusApproved).status(200, "")

	want := []string{ids[3], ids[0], ids[2], ids[1]}
	e.reorder(l.owner, l.book, want).status(200, "")
	if got := e.approvedOrder(l.book); !slices.Equal(got, want) {
		t.Fatalf("order %v, want %v", got, want)
	}
	if got := listed(e.noteList(l.owner, l.book, "&status=approved").status(200, "")); !slices.Equal(got, want) {
		t.Fatalf("list %v, want %v", got, want)
	}
	e.reorder(l.owner, l.book, want).status(200, "") // same order again is fine

	sum := e.count(`SELECT SUM(updated_at) FROM note_tab`)
	for name, bad := range map[string][]string{
		"missing one":     want[:3],
		"extra pending":   append(slices.Clone(want), ids[4]),
		"a hidden id":     {ids[3], ids[0], ids[2], ids[4]},
		"empty":           nil,
		"unknown id":      {ids[3], ids[0], ids[2], strings.Repeat("0", 26)},
		"repeated id":     {ids[3], ids[0], ids[2], ids[2]},
		"repeated approx": {ids[3], ids[3], ids[0], ids[2]},
	} {
		code := "ERROR_ORDER_STALE"
		status := 409
		if strings.HasPrefix(name, "repeated") {
			code, status = "ERROR_PARAM", 400
		}
		e.reorder(l.owner, l.book, bad).status(status, code)
		if got := e.approvedOrder(l.book); !slices.Equal(got, want) || e.count(`SELECT SUM(updated_at) FROM note_tab`) != sum {
			t.Fatalf("%s changed the order: %v", name, got)
		}
	}
	// a note hidden meanwhile makes the client's list stale
	e.setStatus(l.owner, l.book, []string{ids[0]}, StatusHidden).status(200, "")
	e.reorder(l.owner, l.book, want).status(409, "ERROR_ORDER_STALE")

	other := e.newOpenLink()
	e.reorder(other.owner, l.book, want).status(404, "ERROR_NOT_FOUND")
	big := make([]string, 301)
	for i := range big {
		big[i] = fmt.Sprintf("%026d", i)
	}
	e.reorder(l.owner, l.book, big).status(400, "ERROR_TOO_MANY_IDS")
	// no approved notes and no ids: nothing to do
	e.reorder(other.owner, other.book, nil).status(200, "")
}

// AC4: notes, note-photo rows, media rows and objects are gone; a storage failure changes nothing and a retry works;
// another user's ids are 404.
func TestBatchDelete(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 5, func(i int) int { return []int{3, 1, 0, 2, 1}[i] }) // 7 photos
	e.setStatus(l.owner, l.book, ids[:4], StatusApproved).status(200, "")
	if e.count(`SELECT COUNT(*) FROM media_tab`) != 7 || e.st.count() == 0 {
		t.Fatal("photos were not stored")
	}
	objects := e.st.count()

	// another user's ids: 404 and nothing is touched
	other := e.newOpenLink()
	e.deleteNotes(other.owner, l.book, ids[:1]).status(404, "ERROR_NOT_FOUND")
	e.deleteNotes(l.owner, l.book, []string{ids[0], e.seed(other, 1, nil)[0]}).status(404, "ERROR_NOT_FOUND")
	if e.count(`SELECT COUNT(*) FROM note_tab`) != 6 || e.st.count() != objects {
		t.Fatal("a failed delete changed something")
	}

	// the object store is down: ERROR_STORAGE_ERROR, the notes and photos stay, then a retry works
	e.st.mu.Lock()
	e.st.failDelete = true
	e.st.mu.Unlock()
	e.deleteNotes(l.owner, l.book, []string{ids[0], ids[1]}).status(502, "ERROR_STORAGE_ERROR")
	if e.count(`SELECT COUNT(*) FROM note_tab`) != 6 || e.count(`SELECT COUNT(*) FROM media_tab`) != 7 || e.count(`SELECT COUNT(*) FROM note_photo_tab`) != 7 {
		t.Fatal("a storage failure changed the database")
	}
	e.st.mu.Lock()
	e.st.failDelete = false
	e.st.mu.Unlock()

	r := e.deleteNotes(l.owner, l.book, []string{ids[0], ids[1]}).status(200, "")
	if c := e.counts(r); c != [4]int{3, 1, 2, 0} {
		t.Fatalf("counts %v", c)
	}
	if e.count(`SELECT COUNT(*) FROM note_tab WHERE public_id IN (?, ?)`, ids[0], ids[1]) != 0 ||
		e.count(`SELECT COUNT(*) FROM media_tab`) != 3 || e.count(`SELECT COUNT(*) FROM note_photo_tab`) != 3 {
		t.Fatal("rows of the deleted notes remain")
	}
	if e.st.count() != objects-4*3 { // 4 photos, three objects each (image, thumbnail, print)
		t.Fatalf("%d objects left of %d, want %d", e.st.count(), objects, objects-12)
	}
	if got := e.approvedOrder(l.book); !slices.Equal(got, []string{ids[2], ids[3]}) {
		t.Fatalf("positions did not close up: %v", got)
	}
	e.deleteNotes(l.owner, l.book, []string{ids[0]}).status(404, "ERROR_NOT_FOUND") // already gone

	// the last approved note, and everything that is left
	e.deleteNotes(l.owner, l.book, []string{ids[2], ids[3]}).status(200, "")
	e.deleteNotes(l.owner, l.book, ids[4:]).status(200, "")
	if n := e.count(`SELECT (SELECT COUNT(*) FROM note_tab WHERE yearbook_id = (SELECT id FROM yearbook_tab WHERE public_id = ?)) + (SELECT COUNT(*) FROM note_photo_tab) + (SELECT COUNT(*) FROM media_tab)`, l.book); n != 0 {
		t.Fatalf("%d rows remain after every note was deleted", n)
	}
	if e.st.count() != 0 {
		t.Fatalf("%d objects remain", e.st.count())
	}
	e.deleteNotes(l.owner, l.book, nil).status(400, "ERROR_PARAM")
}

// Deleting a note whose photo is also the book's cover clears the cover instead of leaving a dangling id.
func TestBatchDeleteClearsCover(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 1, func(int) int { return 1 })
	var photo string
	if err := e.db.QueryRow(`SELECT public_id FROM media_tab LIMIT 1`).Scan(&photo); err != nil {
		t.Fatal(err)
	}
	e.updateBook(l.owner, l.book, fmt.Sprintf(`{"cover_media_id":%q}`, photo)).status(200, "")
	e.deleteNotes(l.owner, l.book, ids).status(200, "")
	if n := e.count(`SELECT COUNT(*) FROM yearbook_tab WHERE cover_media_id IS NOT NULL`); n != 0 {
		t.Fatal("the cover still points at the deleted photo")
	}
}

// Requirement 5: 120 moderation writes per user per minute.
func TestModerationRateLimit(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 1, nil)
	for range moderationPerMinute {
		e.setStatus(l.owner, l.book, ids, StatusHidden).status(200, "")
	}
	rec := e.setStatus(l.owner, l.book, ids, StatusHidden).status(429, "ERROR_RATE_LIMITED")
	if rec.Header().Get("Retry-After") == "" {
		t.Fatal("no Retry-After")
	}
	e.noteList(l.owner, l.book, "").status(200, "") // reads are not counted
}

// The public submit still counts hidden and pending notes toward the cap of its link.
func TestHiddenNotesCountTowardTheCap(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 2, nil)
	e.setStatus(l.owner, l.book, ids, StatusHidden).status(200, "")
	if n, err := e.ns.store.noteCount(context.Background(), e.collectionRow(l.collection)); err != nil || n != 2 {
		t.Fatalf("noteCount %d %v, want 2 with both hidden", n, err)
	}
}

func (e *env) collectionRow(publicID string) uint64 {
	e.t.Helper()
	var id uint64
	if err := e.db.QueryRow(`SELECT id FROM note_collection_tab WHERE public_id = ?`, publicID).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// AC5, run with -race: moderation writes of one book in parallel never deadlock (5xx stays 0), positions stay dense and the
// winners are the expected ones.
func TestModerationConcurrency(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 12, func(i int) int { return i % 2 })
	e.setStatus(l.owner, l.book, ids[:6], StatusApproved).status(200, "")

	var bad atomic.Int64
	check := func(r resp, ok ...int) {
		if r.Code >= 500 {
			bad.Add(1)
			t.Errorf("5xx: %d %s", r.Code, r.Body.String())
		}
		if !slices.Contains(ok, r.Code) {
			t.Errorf("status %d (%s), want one of %v", r.Code, r.Body.String(), ok)
		}
	}
	for round := range 8 {
		var wg sync.WaitGroup
		run := func(f func()) {
			wg.Add(1)
			go func() { defer wg.Done(); f() }()
		}
		approved := e.approvedOrder(l.book)
		rev := slices.Clone(approved)
		slices.Reverse(rev)
		pending := []string{ids[6+round%6]}
		if round%2 == 0 { // two reorders at once: both fit the same set, so both may win (200) or lose to the other writers (409)
			run(func() { check(e.reorder(l.owner, l.book, approved), 200, 409) })
			run(func() { check(e.reorder(l.owner, l.book, rev), 200, 409) })
			run(func() { check(e.setStatus(l.owner, l.book, pending, StatusApproved), 200) })
		} else { // delete while approve, and hide while reorder
			run(func() { check(e.setStatus(l.owner, l.book, pending, StatusApproved), 200, 404) })
			run(func() { check(e.deleteNotes(l.owner, l.book, pending), 200, 404) })
			run(func() { check(e.setStatus(l.owner, l.book, approved[:1], StatusHidden), 200) })
			run(func() { check(e.reorder(l.owner, l.book, approved), 200, 409) })
		}
		wg.Wait()
		e.approvedOrder(l.book) // dense, and nothing outside the set has a position
		// bring the hidden one back so the next round has a set to work with
		e.setStatus(l.owner, l.book, approved[:1], StatusApproved).status(200, "")
	}
	if bad.Load() != 0 {
		t.Fatalf("%d server errors", bad.Load())
	}
}

// A moderation racing the public submit and the book delete: no 5xx, no orphan rows.
func TestModerationRacesSubmitAndBookDelete(t *testing.T) {
	e := newEnv(t)
	l := e.newOpenLink()
	ids := e.seed(l, 6, func(i int) int { return i % 3 })
	var wg sync.WaitGroup
	var bad atomic.Int64
	note := func(r resp) {
		if r.Code >= 500 {
			bad.Add(1)
			t.Errorf("5xx: %d %s", r.Code, r.Body.String())
		}
	}
	for i := range 6 {
		wg.Add(3)
		go func() { defer wg.Done(); note(e.setStatus(l.owner, l.book, ids[i:i+1], StatusApproved)) }()
		go func() {
			defer wg.Done()
			note(e.postFrom(context.Background(), fmt.Sprintf("203.0.113.%d", i+1), l.token, answers("late", "hi"), photoPart(jpegOf(t, uint8(90+i)))))
		}()
		go func() { defer wg.Done(); note(e.deleteNotes(l.owner, l.book, ids[(i+3)%6:(i+3)%6+1])) }()
	}
	wg.Add(1)
	go func() { defer wg.Done(); note(e.deleteBook(l.owner, l.book)) }()
	wg.Wait()
	if bad.Load() != 0 {
		t.Fatalf("%d server errors", bad.Load())
	}
	for _, tb := range []string{"note_tab", "note_photo_tab", "media_tab", "note_collection_tab"} {
		if n := e.count(`SELECT COUNT(*) FROM ` + tb); n != 0 {
			t.Errorf("%s holds %d rows after the book was deleted", tb, n)
		}
	}
}
