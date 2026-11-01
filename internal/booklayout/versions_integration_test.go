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

	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/ulid"
)

// T-163: the version history of a layout (migration 20261102120000, spec E09_T-163).

const gap = 10 * time.Minute

func restore(book, id string, base int) string {
	return fmt.Sprintf(`{"yearbook_id":%q,"id":%q,"base_version":%d}`, book, id, base)
}

func versionBody(book, id string) string { return fmt.Sprintf(`{"yearbook_id":%q,"id":%q}`, book, id) }

type snapInfo struct {
	ID        string `json:"id"`
	Version   int    `json:"version"`
	Kind      string `json:"kind"`
	CreatedAt int64  `json:"created_at"`
	ItemCount int    `json:"item_count"`
	Bytes     int    `json:"bytes"`
}

func (e *env) versions(u user, book string) []snapInfo {
	e.t.Helper()
	rec := e.call("POST", "/api/book-layout/version-list", u, reset(book)).status(200, "OK")
	var b struct{ Data struct{ Items []snapInfo } }
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		e.t.Fatal(err)
	}
	return b.Data.Items
}

// saveAt saves a text document made unique by tag at time now+d (the clock is shared by the service).
func (e *env) saveAt(u user, book string, base int, tag string) resp {
	e.t.Helper()
	return e.call("POST", "/api/book-layout/save", u, save(book, base, doc(textAt(tag))))
}

// AC1: a burst of saves is one snapshot; the snapshot is of the document before the burst; the boundary is "older than 10 minutes".
func TestSnapshotOnlyAfterAGap(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	e.saveAt(alice, id, 0, "aaaaaa1").status(200, "OK") // first save: nothing to replace
	if n := len(e.versions(alice, id)); n != 0 {
		t.Fatalf("%d snapshots after the first save", n)
	}
	e.now = e.now.Add(time.Minute)
	e.saveAt(alice, id, 1, "aaaaaa2").status(200, "OK") // replaces v1, no snapshot yet: one is taken
	vs := e.versions(alice, id)
	if len(vs) != 1 || vs[0].Version != 1 || vs[0].Kind != "auto" || vs[0].CreatedAt != e.now.UnixMilli() || vs[0].ItemCount != 1 {
		t.Fatalf("after the second save: %+v", vs)
	}
	first := e.now
	for i, d := range []time.Duration{time.Second, 5 * time.Minute, gap} { // inside the window, the last one exactly 10 minutes after the snapshot
		e.now = first.Add(d)
		e.saveAt(alice, id, 2+i, fmt.Sprintf("bbbbbb%d", i)).status(200, "OK")
		if n := len(e.versions(alice, id)); n != 1 {
			t.Fatalf("a save %v after the snapshot made %d snapshots, want 1", d, n)
		}
	}
	e.now = first.Add(gap + time.Millisecond) // older than 10 minutes by 1 ms
	e.saveAt(alice, id, 5, "cccccc1").status(200, "OK")
	vs = e.versions(alice, id)
	if len(vs) != 2 || vs[0].Version != 5 || vs[1].Version != 1 {
		t.Fatalf("after the gap: %+v", vs)
	}
	body := e.call("POST", "/api/book-layout/version-list", alice, reset(id)).Body.String()
	if strings.Contains(body, "Lớp") || strings.Contains(body, `"layout"`) {
		t.Fatalf("the list carries a document: %.300s", body)
	}
	if e.seen("snapshot:auto") != 2 || e.pruned.Load() != 0 {
		t.Fatalf("metrics: %d auto, %d pruned", e.seen("snapshot:auto"), e.pruned.Load())
	}
	// the stored body is the document before the burst
	g := e.call("POST", "/api/book-layout/version-get", alice, versionBody(id, vs[1].ID)).status(200, "OK")
	if !strings.Contains(raw(g, "layout"), "aaaaaa1") || g.data()["version"] != float64(1) || g.data()["kind"] != "auto" {
		t.Fatalf("version-get: %.400s", g.Body)
	}
}

// AC1: 31 gaps keep the newest 30; the oldest is pruned, in the same transaction as the save.
func TestRetentionKeepsNewestThirty(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	base := 0
	for i := range 33 { // 33 saves 11 minutes apart: the first replaces nothing, the other 32 each store a snapshot
		e.saveAt(alice, id, base, fmt.Sprintf("dddddd%02d", i)).status(200, "OK")
		base++
		e.now = e.now.Add(gap + time.Minute)
	}
	vs := e.versions(alice, id)
	if len(vs) != 30 || vs[0].Version != 32 || vs[29].Version != 3 {
		t.Fatalf("%d snapshots, newest v%d, oldest v%d (want 30, 32, 3)", len(vs), vs[0].Version, vs[len(vs)-1].Version)
	}
	if e.count(`SELECT COUNT(*) FROM book_layout_version_tab`) != 30 || e.pruned.Load() != 2 || e.seen("snapshot:auto") != 32 {
		t.Fatalf("rows %d, pruned %d, auto %d", e.count(`SELECT COUNT(*) FROM book_layout_version_tab`), e.pruned.Load(), e.seen("snapshot:auto"))
	}
	for i := 1; i < len(vs); i++ {
		if vs[i].CreatedAt >= vs[i-1].CreatedAt {
			t.Fatalf("not newest first at %d: %+v", i, vs[i-1:i+1])
		}
	}
	// another book is not pruned by this one
	other := e.book(alice, "memory-book")
	e.saveAt(alice, other, 0, "eeeeee1").status(200, "OK")
	e.saveAt(alice, other, 1, "eeeeee2").status(200, "OK")
	if len(e.versions(alice, other)) != 1 || len(e.versions(alice, id)) != 30 {
		t.Fatal("books share a history")
	}
}

// Req 2: reset and restore store a snapshot whatever the age of the newest one; reset keeps the history.
func TestResetAndRestoreSnapshotRegardlessOfAge(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	e.saveAt(alice, id, 0, "ffffff1").status(200, "OK")
	e.saveAt(alice, id, 1, "ffffff2").status(200, "OK") // snapshot of v1 (auto)
	e.saveAt(alice, id, 2, "ffffff3").status(200, "OK") // same burst: none
	e.call("POST", "/api/book-layout/reset", alice, reset(id)).status(200, "OK")
	vs := e.versions(alice, id)
	if len(vs) != 2 || vs[0].Kind != "reset" || vs[0].Version != 3 || vs[1].Kind != "auto" {
		t.Fatalf("after reset: %+v", vs)
	}
	e.call("POST", "/api/book-layout/reset", alice, reset(id)).status(200, "OK") // nothing to keep
	if len(e.versions(alice, id)) != 2 {
		t.Fatal("a reset without a layout stored a snapshot")
	}
	// restore the reset snapshot (version 4 after the second reset), then a second restore at once
	g := e.call("GET", get(id), alice, "").data()
	r := e.call("POST", "/api/book-layout/restore", alice, restore(id, vs[0].ID, int(g["version"].(float64)))).status(200, "OK").data()
	if r["version"] != g["version"].(float64)+1 || r["dropped"] != float64(0) {
		t.Fatalf("restore: %v", r)
	}
	vs = e.versions(alice, id)
	if len(vs) != 2 { // restore replaced nothing (the layout was reset), so no snapshot
		t.Fatalf("restore over an empty book: %+v", vs)
	}
	r2 := e.call("POST", "/api/book-layout/restore", alice, restore(id, vs[1].ID, int(r["version"].(float64)))).status(200, "OK").data()
	vs = e.versions(alice, id)
	if len(vs) != 3 || vs[0].Kind != "restore" || vs[0].Version != int(r["version"].(float64)) || r2["version"] != r["version"].(float64)+1 {
		t.Fatalf("restore over a layout, seconds after the last snapshot: %+v %v", vs, r2)
	}
	if e.seen("snapshot:reset") != 1 || e.seen("snapshot:restore") != 1 || e.seen("restore:ok") != 2 {
		t.Fatalf("metrics: reset %d restore %d ok %d", e.seen("snapshot:reset"), e.seen("snapshot:restore"), e.seen("restore:ok"))
	}
}

// AC2: restore brings the old document back as a new, higher version; the replaced document becomes a snapshot; a stale base is a conflict
// that changes nothing.
func TestRestoreIsANewVersionAndConflictChangesNothing(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	e.saveAt(alice, id, 0, "gggggg1").status(200, "OK")
	e.now = e.now.Add(gap + time.Minute)
	e.saveAt(alice, id, 1, "gggggg2").status(200, "OK")
	snap := e.versions(alice, id)[0]
	r := e.call("POST", "/api/book-layout/restore", alice, restore(id, snap.ID, 2)).status(200, "OK").data()
	if r["version"] != float64(3) || r["dropped"] != float64(0) || !strings.Contains(fmt.Sprint(r["layout"]), "gggggg1") {
		t.Fatalf("restore: %v", r)
	}
	got := e.call("GET", get(id), alice, "").status(200, "OK")
	sameJSON(t, raw(got, "layout"), doc(textAt("gggggg1")))
	vs := e.versions(alice, id)
	if len(vs) != 2 || vs[0].Kind != "restore" || vs[0].Version != 2 {
		t.Fatalf("history: %+v", vs)
	}
	before := e.count(`SELECT COUNT(*) FROM book_layout_version_tab`)
	c := e.call("POST", "/api/book-layout/restore", alice, restore(id, snap.ID, 2)).status(409, "ERROR_VERSION_CONFLICT") // base 2 is stale now
	if c.data()["version"] != float64(3) || !strings.Contains(raw(c, "layout"), "gggggg1") {
		t.Fatalf("conflict data: %.300s", c.Body)
	}
	if e.count(`SELECT COUNT(*) FROM book_layout_version_tab`) != before || e.call("GET", get(id), alice, "").data()["version"] != float64(3) {
		t.Fatal("a conflicting restore changed something")
	}
	if e.seen("restore:ok") != 1 || e.seen("restore:conflict") != 1 {
		t.Fatalf("metrics: ok %d conflict %d", e.seen("restore:ok"), e.seen("restore:conflict"))
	}
	e.call("POST", "/api/book-layout/restore", alice, `{"yearbook_id":"`+id+`","id":"`+snap.ID+`"}`).status(400, "ERROR_PARAM") // base_version is required
}

// insertSnapshot stores a snapshot row directly (an old document: this is how a registry change or a deleted image is reproduced).
func (e *env) insertSnapshot(book, tpl, layout string, version int) string {
	e.t.Helper()
	id := ulid.New(e.now)
	e.exec(`INSERT INTO book_layout_version_tab (public_id, yearbook_id, version, kind, template_id, layout_json, item_count, bytes, created_at)
		SELECT ?, id, ?, 'auto', ?, ?, 1, ?, ? FROM yearbook_tab WHERE public_id = ?`, id, version, tpl, layout, len(layout), e.now.UnixMilli(), book)
	return id
}

// AC2: items and a background whose design image or asset is gone are dropped and counted; the rest comes back; other problems are a 400.
func TestRestoreDropsWhatIsGone(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	img, kept, bg := e.image(id, "design"), e.image(id, "design"), e.image(id, "design")
	old := fmt.Sprintf(`{"v":1,"pages":{"cover":{"bg":{"media":%q},"items":[`+
		`{"id":"imgone1","t":"image","media":%q,"x":100,"w":400,"h":300,"cy":500},`+
		`{"id":"imgkept","t":"image","media":%q,"x":100,"w":400,"h":300,"cy":300},`+
		`{"id":"assetgn","t":"asset","asset":"shape.removed-long-ago","x":100,"w":200,"h":200,"cy":700},`+
		`{"id":"textok1",%s}]},"back":{"bg":{"asset":"shape.removed-long-ago"}}}}`, bg, img, kept, textItem)
	snap := e.insertSnapshot(id, "memory-book", old, 5)
	e.exec(`DELETE FROM media_tab WHERE public_id IN (?, ?)`, img, bg) // the design image of one item and the background were deleted
	e.exec(`UPDATE yearbook_tab SET layout_version = 7 WHERE public_id = ?`, id)
	r := e.call("POST", "/api/book-layout/restore", alice, restore(id, snap, 7)).status(200, "OK")
	if d := r.data(); d["dropped"] != float64(4) || d["version"] != float64(8) {
		t.Fatalf("restore: %.500s", r.Body)
	}
	stored := raw(e.call("GET", get(id), alice, ""), "layout")
	for _, gone := range []string{"imgone1", "assetgn", img, bg, "removed-long-ago"} {
		if strings.Contains(stored, gone) {
			t.Errorf("the stored layout still has %q: %s", gone, stored)
		}
	}
	for _, kept := range []string{"imgkept", "textok1", kept} {
		if !strings.Contains(stored, kept) {
			t.Errorf("the stored layout lost %q: %s", kept, stored)
		}
	}
	// the answer's layout is the stored one
	sameJSON(t, raw(r, "layout"), stored)

	// any other problem is invalid and changes nothing
	bad := e.insertSnapshot(id, "memory-book", `{"v":1,"pages":{"cover":{"items":[{"id":"abcdef","t":"text","text":"x","x":1,"w":400,"h":100,"cy":500,"bogus":1}]}}}`, 9)
	inv := e.call("POST", "/api/book-layout/restore", alice, restore(id, bad, 8)).status(400, "ERROR_INVALID_LAYOUT")
	if !strings.Contains(inv.Body.String(), "unknown_key") || e.call("GET", get(id), alice, "").data()["version"] != float64(8) {
		t.Fatalf("invalid restore: %.300s", inv.Body)
	}
	// a snapshot made for another template cannot be restored
	other := e.insertSnapshot(id, "navy-classic", old, 11)
	e.call("POST", "/api/book-layout/restore", alice, restore(id, other, 8)).status(409, "ERROR_VERSION_TEMPLATE_CHANGED")
	if e.seen("restore:ok") != 1 || e.seen("restore:invalid") != 1 || e.seen("restore:template_changed") != 1 {
		t.Fatalf("metrics: ok %d invalid %d template_changed %d", e.seen("restore:ok"), e.seen("restore:invalid"), e.seen("restore:template_changed"))
	}
}

// A design image of another book of the same owner is not "own" for this book: it is dropped, not restored.
func TestRestoreDropsAnotherBooksImage(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id, other := e.book(alice, "memory-book"), e.book(alice, "memory-book")
	foreign := e.image(other, "design")
	snap := e.insertSnapshot(id, "memory-book", fmt.Sprintf(`{"v":1,"pages":{"cover":{"items":[{"id":"imgfor1","t":"image","media":%q,"x":100,"w":400,"h":300,"cy":500}]}}}`, foreign), 1)
	if d := e.call("POST", "/api/book-layout/restore", alice, restore(id, snap, 0)).status(200, "OK").data(); d["dropped"] != float64(1) {
		t.Fatalf("restore: %v", d)
	}
}

// AC3: only the owner reads or restores; another user, an unknown book, another book's snapshot and a bad id are refused and leak nothing.
func TestVersionRoutesAreOwnerOnly(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice, bob := e.register("alice@example.com"), e.register("bob@example.com")
	id, second := e.book(alice, "memory-book"), e.book(alice, "memory-book")
	e.saveAt(alice, id, 0, "hhhhhh1").status(200, "OK")
	e.now = e.now.Add(time.Minute)
	e.saveAt(alice, id, 1, "hhhhhh2").status(200, "OK")
	snap := e.versions(alice, id)[0].ID
	unknown := "01K7QB1B2C3D4E5F6G7H8J9K0M"
	for name, c := range map[string]struct {
		path, body string
		who        user
		code       int
		status     string
	}{
		"list: other user":         {"version-list", reset(id), bob, 404, "ERROR_NOT_FOUND"},
		"list: anonymous":          {"version-list", reset(id), user{}, 401, "ERROR_UNAUTHORIZED"},
		"list: unknown book":       {"version-list", reset(unknown), alice, 404, "ERROR_NOT_FOUND"},
		"list: bad id":             {"version-list", reset("nope"), alice, 400, "ERROR_PARAM"},
		"get: other user":          {"version-get", versionBody(id, snap), bob, 404, "ERROR_NOT_FOUND"},
		"get: anonymous":           {"version-get", versionBody(id, snap), user{}, 401, "ERROR_UNAUTHORIZED"},
		"get: unknown book":        {"version-get", versionBody(unknown, snap), alice, 404, "ERROR_NOT_FOUND"},
		"get: unknown snapshot":    {"version-get", versionBody(id, unknown), alice, 404, "ERROR_NOT_FOUND"},
		"get: other book's id":     {"version-get", versionBody(second, snap), alice, 404, "ERROR_NOT_FOUND"},
		"get: bad snapshot id":     {"version-get", versionBody(id, "nope"), alice, 400, "ERROR_PARAM"},
		"restore: other user":      {"restore", restore(id, snap, 2), bob, 404, "ERROR_NOT_FOUND"},
		"restore: anonymous":       {"restore", restore(id, snap, 2), user{}, 401, "ERROR_UNAUTHORIZED"},
		"restore: unknown book":    {"restore", restore(unknown, snap, 0), alice, 404, "ERROR_NOT_FOUND"},
		"restore: other book's id": {"restore", restore(second, snap, 0), alice, 404, "ERROR_NOT_FOUND"},
		"restore: unknown":         {"restore", restore(id, unknown, 2), alice, 404, "ERROR_NOT_FOUND"},
	} {
		rec := e.call("POST", "/api/book-layout/"+c.path, c.who, c.body)
		if rec.Code != c.code {
			t.Errorf("%s: %d %.200s", name, rec.Code, rec.Body)
			continue
		}
		rec.status(c.code, c.status)
		if strings.Contains(rec.Body.String(), "hhhhhh") {
			t.Errorf("%s leaks a document", name)
		}
	}
	if e.count(`SELECT COUNT(*) FROM book_layout_version_tab`) != 1 || e.call("GET", get(id), alice, "").data()["version"] != float64(2) {
		t.Fatal("a refused call changed something")
	}
}

// AC3: a template change and a yearbook delete remove the snapshots; a reset keeps them; no orphan remains.
func TestSnapshotsAreDeletedWithTemplateChangeAndYearbook(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id, keep := e.book(alice, "memory-book"), e.book(alice, "memory-book")
	for _, b := range []string{id, keep} {
		e.saveAt(alice, b, 0, "iiiiii1").status(200, "OK")
		e.now = e.now.Add(time.Minute)
		e.saveAt(alice, b, 1, "iiiiii2").status(200, "OK")
	}
	e.call("POST", "/api/book-layout/reset", alice, reset(id)).status(200, "OK")
	if len(e.versions(alice, id)) != 2 {
		t.Fatal("reset deleted the history")
	}
	e.saveAt(alice, id, 3, "iiiiii3").status(200, "OK")
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"title":"Same template","template_id":"memory-book"}`, id)).status(200, "OK")
	if len(e.versions(alice, id)) != 2 {
		t.Fatal("an update that keeps the template deleted the history")
	}
	e.call("POST", "/api/yearbook/update", alice, fmt.Sprintf(`{"id":%q,"template_id":"navy-classic"}`, id)).status(200, "OK")
	if n := e.count(`SELECT COUNT(*) FROM book_layout_version_tab v JOIN yearbook_tab y ON y.id = v.yearbook_id WHERE y.public_id = ?`, id); n != 0 {
		t.Fatalf("%d snapshots left after the template change", n)
	}
	if len(e.versions(alice, keep)) != 1 {
		t.Fatal("the template change of one book touched another's history")
	}
	e.call("POST", "/api/yearbook/delete", alice, fmt.Sprintf(`{"id":%q}`, keep)).status(200, "OK")
	if e.count(`SELECT COUNT(*) FROM book_layout_version_tab`) != 0 {
		t.Fatal("the yearbook delete kept snapshots")
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// AC4 (Req 2): parallel saves never store two snapshots in one window and never lose the one a gap calls for.
func TestConcurrentSavesStoreExactlyOneSnapshot(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	e.saveAt(alice, id, 0, "jjjjjj0").status(200, "OK")
	for round := 1; round <= 3; round++ {
		e.now = e.now.Add(gap + time.Minute) // every round starts a new burst
		var wg sync.WaitGroup
		var wins, conflicts atomic.Int32
		for i := range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				switch rec := e.do("POST", "/api/book-layout/save", alice, save(id, round, doc(textAt(fmt.Sprintf("r%dt%03d", round, i))))); rec.Code {
				case 200:
					wins.Add(1)
				case 409:
					conflicts.Add(1)
				default:
					t.Errorf("status %d: %s", rec.Code, rec.Body)
				}
			}()
		}
		wg.Wait()
		if wins.Load() != 1 || conflicts.Load() != 7 {
			t.Fatalf("round %d: %d wins, %d conflicts", round, wins.Load(), conflicts.Load())
		}
		if n := len(e.versions(alice, id)); n != round {
			t.Fatalf("round %d: %d snapshots, want %d", round, n, round)
		}
	}
	// saves, restores and resets racing for one book: no 5xx, no duplicate (book, version) snapshot, nothing beyond the cap
	snap := e.versions(alice, id)[0].ID
	e.now = e.now.Add(gap + time.Minute)
	var wg sync.WaitGroup
	for i := range 6 {
		wg.Add(3)
		go func() {
			defer wg.Done()
			if rec := e.do("POST", "/api/book-layout/save", alice, save(id, 4+i%2, doc(textAt("kkkkkk1")))); rec.Code >= 500 {
				t.Errorf("save: %d %s", rec.Code, rec.Body)
			}
		}()
		go func() {
			defer wg.Done()
			if rec := e.do("POST", "/api/book-layout/restore", alice, restore(id, snap, 4+i%2)); rec.Code >= 500 {
				t.Errorf("restore: %d %s", rec.Code, rec.Body)
			}
		}()
		go func() {
			defer wg.Done()
			if rec := e.do("POST", "/api/book-layout/reset", alice, reset(id)); rec.Code >= 500 {
				t.Errorf("reset: %d %s", rec.Code, rec.Body)
			}
		}()
	}
	wg.Wait()
	if n := e.count(`SELECT COUNT(*) FROM book_layout_version_tab`); n > 30 {
		t.Fatalf("%d snapshots", n)
	}
}

// Req 2: a failed snapshot fails the write; the document and the version are untouched (never a silent loss).
func TestFailedSnapshotFailsTheSave(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	id := e.book(alice, "memory-book")
	e.saveAt(alice, id, 0, "llllll1").status(200, "OK")
	e.exec(`RENAME TABLE book_layout_version_tab TO book_layout_version_gone`)
	e.now = e.now.Add(gap + time.Minute)
	e.saveAt(alice, id, 1, "llllll2").status(500, "ERROR_INTERNAL")
	e.call("POST", "/api/book-layout/reset", alice, reset(id)).status(500, "ERROR_INTERNAL")
	e.exec(`RENAME TABLE book_layout_version_gone TO book_layout_version_tab`)
	g := e.call("GET", get(id), alice, "").data()
	if g["version"] != float64(1) || !strings.Contains(fmt.Sprint(g["layout"]), "llllll1") {
		t.Fatalf("a failed snapshot changed the layout: %v", g)
	}
}
