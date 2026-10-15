//go:build integration

package yearbook

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
)

// fakeChild is a registered ChildPurger: it records what it saw and can fail.
type fakeChild struct {
	called    []uint64
	profiles  int // profile rows it still saw inside the transaction
	fail      error
	childRows string // a table it empties, to prove its delete rolls back with the rest
}

func (f *fakeChild) DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	f.called = append(f.called, yearbookID)
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM profile_tab WHERE yearbook_id = ?`, yearbookID).Scan(&f.profiles); err != nil {
		return err
	}
	if f.childRows != "" {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+f.childRows); err != nil {
			return err
		}
	}
	return f.fail
}

func (e *env) bookRow(publicID string) uint64 {
	e.t.Helper()
	var id uint64
	if err := e.db.QueryRow(`SELECT id FROM yearbook_tab WHERE public_id = ?`, publicID).Scan(&id); err != nil {
		e.t.Fatal(err)
	}
	return id
}

// insertMedia adds a photo row to a book and returns its row id and public id (the media package is not wired here).
func (e *env) insertMedia(bookRow uint64, publicID string) uint64 {
	e.t.Helper()
	res, err := e.db.Exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		VALUES (?, ?, 'owner', 'k', 't', 'image/jpeg', 1, 1, 1, '', 1, 1)`, publicID, bookRow)
	if err != nil {
		e.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return uint64(id)
}

// T-064 AC4: Delete removes the profile, calls each registered child with the book's row id inside the same transaction,
// then removes the book. A failing child rolls everything back; a book of someone else is never touched.
func TestDeleteCallsChildrenAndRollsBack(t *testing.T) {
	child := &fakeChild{childRows: "media_tab"}
	e := newEnvWith(t, child)
	alice := e.register("alice@example.com", "Alice")
	bob := e.register("bob@example.com", "Bob")
	doomed := e.create(alice, "Doomed")
	bobs := e.create(bob, "Bob's")
	row := e.bookRow(doomed)
	e.insertMedia(row, "01J9Z3K6V8Q4M7N2P5R8T0W1XY")

	// Bob cannot delete it: no child is called and nothing changes.
	e.remove(bob, doomed).status(404, "ERROR_NOT_FOUND")
	if len(child.called) != 0 || e.count(`SELECT COUNT(*) FROM yearbook_tab`) != 2 {
		t.Fatalf("foreign delete: children %v", child.called)
	}

	// A failing child rolls the profile delete, the child's own delete and the book back.
	child.fail = errors.New("child exploded")
	e.remove(alice, doomed).status(500, "ERROR_INTERNAL")
	if e.count(`SELECT COUNT(*) FROM yearbook_tab WHERE public_id = ?`, doomed) != 1 || e.count(`SELECT COUNT(*) FROM profile_tab WHERE yearbook_id = ?`, row) != 1 ||
		e.count(`SELECT COUNT(*) FROM media_tab`) != 1 {
		t.Fatal("a failed child purger left the delete half done")
	}
	e.get(alice, doomed).status(200, "")

	// Without the failure: the child ran once more, saw no profile (deleted before it), and book, profile and its rows are gone.
	child.fail = nil
	child.called = nil
	e.remove(alice, doomed).status(200, "")
	if len(child.called) != 1 || child.called[0] != row || child.profiles != 0 {
		t.Fatalf("child called %v, saw %d profiles", child.called, child.profiles)
	}
	for _, q := range []string{`SELECT COUNT(*) FROM profile_tab WHERE yearbook_id = ?`, `SELECT COUNT(*) FROM yearbook_tab WHERE id = ?`} {
		if n := e.count(q, row); n != 0 {
			t.Fatalf("%s: %d rows left", q, n)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM media_tab`); n != 0 {
		t.Fatalf("child's delete did not commit: %d media rows", n)
	}
	e.get(bob, bobs).status(200, "")
	if e.count(`SELECT COUNT(*) FROM profile_tab`) != 1 {
		t.Fatal("another book's profile was deleted")
	}
	dbtest.AssertNoOrphans(t, e.db)
}

// T-064 AC5: a cover or profile photo must be a photo of the same book; ClearMediaRefs nulls both references.
func TestMediaReferences(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	bob := e.register("bob@example.com", "Bob")
	one, two, bobs := e.create(alice, "One"), e.create(alice, "Two"), e.create(bob, "Bob's")
	mine := "01J9Z3K6V8Q4M7N2P5R8T0AAAA"
	e.insertMedia(e.bookRow(one), mine)
	otherBook := "01J9Z3K6V8Q4M7N2P5R8T0BBBB"
	e.insertMedia(e.bookRow(two), otherBook) // the same owner, another book
	others := "01J9Z3K6V8Q4M7N2P5R8T0CCCC"
	e.insertMedia(e.bookRow(bobs), others) // someone else's book

	for _, bad := range []string{otherBook, others, "01J9Z3K6V8Q4M7N2P5R8T0DDDD", "x"} {
		e.update(alice, one, fmt.Sprintf(`{"cover_media_id":%q}`, bad)).status(400, "ERROR_INVALID_MEDIA")
		e.profile(alice, one, fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, bad)).status(400, "ERROR_INVALID_MEDIA")
	}
	b := e.update(alice, one, fmt.Sprintf(`{"cover_media_id":%q}`, mine)).status(200, "").book()
	if b["cover_media_id"] != mine {
		t.Fatalf("cover not set: %v", b)
	}
	b = e.profile(alice, one, fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, mine)).status(200, "").book()
	if b["cover_media_id"] != mine || b["profile"].(map[string]any)["photo_media_id"] != mine {
		t.Fatalf("photo not set (or cover lost): %v", b)
	}

	mediaRow := e.count(`SELECT id FROM media_tab WHERE public_id = ?`, mine)
	tx, err := e.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := NewStore(e.db).ClearMediaRefs(context.Background(), tx, int64(mediaRow)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	b = e.get(alice, one).status(200, "").book()
	if b["cover_media_id"] != nil || b["profile"].(map[string]any)["photo_media_id"] != nil {
		t.Fatalf("references survive ClearMediaRefs: %v", b)
	}
}

// countingConnector counts the statements (not BEGIN/COMMIT) that go through its connections. It hides the
// optional driver interfaces, so database/sql always prepares and executes through a statement we can see.
type countingConnector struct {
	driver.Connector
	n    *atomic.Int64
	hook func(query string) // optional: runs before each statement, to race a write in between two reads
}

type countingConn struct {
	driver.Conn
	n    *atomic.Int64
	hook func(query string)
}

type countingStmt struct {
	driver.Stmt
	n     *atomic.Int64
	hook  func(query string)
	query string
}

func (c countingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	return countingConn{conn, c.n, c.hook}, err
}

func (c countingConn) PrepareContext(ctx context.Context, q string) (driver.Stmt, error) {
	st, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, q)
	return countingStmt{st, c.n, c.hook, q}, err
}

func (c countingConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, o)
}

func (c countingConn) CheckNamedValue(nv *driver.NamedValue) error {
	return c.Conn.(driver.NamedValueChecker).CheckNamedValue(nv)
}

func (s countingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	s.n.Add(1)
	if s.hook != nil {
		s.hook(s.query)
	}
	return s.Stmt.(driver.StmtQueryContext).QueryContext(ctx, args)
}

func (s countingStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	s.n.Add(1)
	return s.Stmt.(driver.StmtExecContext).ExecContext(ctx, args)
}

func (s countingStmt) CheckNamedValue(nv *driver.NamedValue) error {
	return s.Stmt.(driver.NamedValueChecker).CheckNamedValue(nv)
}

// countingDB opens a second pool on the test database of d.
func countingDB(t *testing.T, d *sql.DB, n *atomic.Int64) *sql.DB {
	t.Helper()
	return hookedDB(t, d, n, nil)
}

// hookedDB is countingDB with a hook that runs before every statement of the returned pool.
func hookedDB(t *testing.T, d *sql.DB, n *atomic.Int64, hook func(query string)) *sql.DB {
	t.Helper()
	var name string
	if err := d.QueryRow(`SELECT DATABASE()`).Scan(&name); err != nil {
		t.Fatal(err)
	}
	dsn := os.Getenv("SMEM_TEST_DB_DSN")
	if dsn == "" {
		dsn = dbtest.DefaultDSN
	}
	mc, err := db.Normalize(dsn)
	if err != nil {
		t.Fatal(err)
	}
	mc.DBName = name
	inner, err := mysql.NewConnector(mc)
	if err != nil {
		t.Fatal(err)
	}
	c := sql.OpenDB(countingConnector{inner, n, hook})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// T-064 AC6: reading books does not join and does not loop: the number of queries is the same for 1 and for 50 books
// (book rows, profiles, media public ids).
func TestReadQueryCountIsConstant(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	var owner uint64
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = 'alice@example.com'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	// 50 books straight into the tables (the 20-book limit lives in the create path), each with a cover and a photo.
	for i := 0; i < 50; i++ {
		pub := fmt.Sprintf("01J9Z3K6V8Q4M7N2P5R8T%05d", i)
		res, err := e.db.Exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) VALUES (?, ?, 'T', 'en', ?, ?)`, pub, owner, 1000+i, 1000+i)
		if err != nil {
			t.Fatal(err)
		}
		book, _ := res.LastInsertId()
		cover := e.insertMedia(uint64(book), fmt.Sprintf("01J9Z3K6V8Q4M7N2P5R8M%05d", i))
		photo := e.insertMedia(uint64(book), fmt.Sprintf("01J9Z3K6V8Q4M7N2P5R8P%05d", i))
		if _, err := e.db.Exec(`UPDATE yearbook_tab SET cover_media_id = ? WHERE id = ?`, cover, book); err != nil {
			t.Fatal(err)
		}
		if _, err := e.db.Exec(`INSERT INTO profile_tab (public_id, yearbook_id, is_owner, full_name, photo_media_id, created_at, updated_at) VALUES (?, ?, 1, 'N', ?, 1, 1)`,
			fmt.Sprintf("01J9Z3K6V8Q4M7N2P5R8Q%05d", i), book, photo); err != nil {
			t.Fatal(err)
		}
	}
	var n atomic.Int64
	st := NewStore(countingDB(t, e.db, &n))
	ctx := context.Background()

	for _, limit := range []int{1, 50} {
		n.Store(0)
		books, _, err := st.list(ctx, owner, nil, limit)
		if err != nil || len(books) != limit {
			t.Fatalf("list(%d): %d books, %v", limit, len(books), err)
		}
		if got := n.Load(); got != 3 {
			t.Fatalf("list of %d books issued %d queries, want 3", limit, got)
		}
		if books[0].CoverMediaID == nil || books[0].Profile.PhotoMediaID == nil {
			t.Fatalf("media ids not filled: %+v", books[0])
		}
	}
	n.Store(0)
	if _, err := st.get(ctx, owner, "01J9Z3K6V8Q4M7N2P5R8T00007"); err != nil {
		t.Fatal(err)
	}
	if got := n.Load(); got != 3 {
		t.Fatalf("get issued %d queries, want 3", got)
	}

	// T-070 AC5: the same through the HTTP layer: a get-list of 1 or of 50 books and a get cost the same three queries.
	counted := httpx.NewRouter(slog.New(slog.DiscardHandler), NewHandler(NewService(NewStore(countingDB(t, e.db, &n)), httpx.NewCursors([]byte(strings.Repeat("c", 32))), nil, e.now),
		e.ah.RequireUser, []string{origin}).Routes, e.ah.Routes)
	for _, limit := range []int{1, 50} {
		n.Store(0)
		items, _ := e.doOn(counted, alice, "GET", fmt.Sprintf("/api/yearbook/get-list?limit=%d", limit), "").status(200, "").page()
		if len(items) != limit || n.Load() != 3 {
			t.Fatalf("HTTP get-list of %d books: %d items, %d queries, want 3", limit, len(items), n.Load())
		}
	}
	n.Store(0)
	e.doOn(counted, alice, "GET", "/api/yearbook/get?id=01J9Z3K6V8Q4M7N2P5R8T00007", "").status(200, "")
	if n.Load() != 3 {
		t.Fatalf("HTTP get issued %d queries, want 3", n.Load())
	}

	// The list query can read idx_owner_updated in order: no sort. (The optimizer ignores any index on a 50-row table,
	// so the index is forced to see what it would do on a big one.)
	rows, err := e.db.Query(`EXPLAIN SELECT `+bookCols+` FROM yearbook_tab FORCE INDEX (idx_owner_updated) WHERE owner_id = ?
		AND (updated_at < ? OR (updated_at = ? AND public_id < ?)) ORDER BY updated_at DESC, public_id DESC LIMIT 20`, owner, 2000, 2000, "z")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rows.Close() }()
	cols, _ := rows.Columns()
	seen := 0
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		seen++
		for i, c := range cols {
			if c == "key" && vals[i].String != "idx_owner_updated" {
				t.Fatalf("plan key %q", vals[i].String)
			}
			if c == "Extra" && strings.Contains(vals[i].String, "filesort") {
				t.Fatalf("list sorts: %s", vals[i].String)
			}
		}
	}
	if seen != 1 {
		t.Fatalf("%d plan rows", seen)
	}
}

// T-078: a book deleted between the book query and the profile query is "not found" on get and absent from list,
// never the 500 "yearbook without its owner profile". The hook deletes the book (profiles and book, as Service.Delete
// does) right before the profile query, which is the exact window of the old bug.
func TestReadOfBookDeletedMidReadIsNotFound(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	var owner uint64
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = 'alice@example.com'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	keep, gone := e.create(alice, "Keep"), e.create(alice, "Gone")
	deleteGone := func(row uint64) func(string) {
		return func(query string) {
			if !strings.Contains(query, "FROM profile_tab") {
				return
			}
			for _, q := range []string{`DELETE FROM profile_tab WHERE yearbook_id = ?`, `DELETE FROM yearbook_tab WHERE id = ?`} {
				if _, err := e.db.Exec(q, row); err != nil {
					t.Error(err)
				}
			}
		}
	}
	var n atomic.Int64
	ctx := context.Background()

	st := NewStore(hookedDB(t, e.db, &n, deleteGone(e.bookRow(gone))))
	if _, err := st.get(ctx, owner, gone); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get of a book deleted mid-read: %v, want ErrNotFound", err)
	}

	// Recreate the doomed book and list: the survivor stays, the deleted one is absent.
	gone = e.create(alice, "Gone")
	st = NewStore(hookedDB(t, e.db, &n, deleteGone(e.bookRow(gone))))
	books, _, err := st.list(ctx, owner, nil, 10)
	if err != nil {
		t.Fatalf("list with a book deleted mid-read: %v", err)
	}
	if len(books) != 1 || books[0].ID != keep {
		t.Fatalf("list = %d books, want only %s", len(books), keep)
	}
}

// T-078: a book that vanishes from a full page shortens it but must not hide the next page: next_cursor comes from the
// raw rows, and the following pages neither skip nor repeat a surviving book.
func TestListPageKeepsNextCursorWhenABookVanishes(t *testing.T) {
	e := newEnv(t)
	alice := e.register("alice@example.com", "Alice")
	var owner uint64
	if err := e.db.QueryRow(`SELECT id FROM user_tab WHERE email = 'alice@example.com'`).Scan(&owner); err != nil {
		t.Fatal(err)
	}
	var ids []string // oldest first; the list is newest first
	for i := 0; i < 5; i++ {
		ids = append(ids, e.create(alice, fmt.Sprintf("B%d", i)))
	}
	newest, vanishing := ids[4], ids[3]
	row := e.bookRow(vanishing)
	var n atomic.Int64
	ctx := context.Background()
	st := NewStore(hookedDB(t, e.db, &n, func(query string) {
		if !strings.Contains(query, "FROM profile_tab") {
			return
		}
		for _, q := range []string{`DELETE FROM profile_tab WHERE yearbook_id = ?`, `DELETE FROM yearbook_tab WHERE id = ?`} {
			if _, err := e.db.Exec(q, row); err != nil {
				t.Error(err)
			}
		}
	}))

	page, next, err := st.list(ctx, owner, nil, 2) // raw page: newest, vanishing; probe row: ids[2]
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || page[0].ID != newest {
		t.Fatalf("first page = %d books, want only the newest", len(page))
	}
	if next == nil {
		t.Fatal("next cursor is nil although two more books exist")
	}

	var got []string
	plain := NewStore(e.db)
	for next != nil {
		page, next, err = plain.list(ctx, owner, next, 2)
		if err != nil {
			t.Fatal(err)
		}
		for _, b := range page {
			got = append(got, b.ID)
		}
	}
	if want := []string{ids[2], ids[1], ids[0]}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("following pages = %v, want %v", got, want)
	}
}
