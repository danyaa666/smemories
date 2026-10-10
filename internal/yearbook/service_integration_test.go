//go:build integration

package yearbook

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
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
	res, err := e.db.Exec(`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at)
		VALUES (?, ?, 'owner', 'k', 't', 'image/jpeg', 1, 1, 1, '', NOW(6))`, publicID, bookRow)
	if err != nil {
		e.t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	return uint64(id)
}

// T-064 AC4: Delete removes the profile, calls each registered child with the book's row id inside the same transaction,
// then removes the book. A failing child rolls everything back; a book of someone else is never touched.
func TestDeleteCallsChildrenAndRollsBack(t *testing.T) {
	child := &fakeChild{childRows: "media"}
	e := newEnvWith(t, child)
	alice := e.register("alice@example.com", "Alice")
	bob := e.register("bob@example.com", "Bob")
	doomed := e.create(alice, "Doomed")
	bobs := e.create(bob, "Bob's")
	row := e.bookRow(doomed)
	e.insertMedia(row, "01J9Z3K6V8Q4M7N2P5R8T0W1XY")

	// Bob cannot delete it: no child is called and nothing changes.
	e.do(bob, "DELETE", "/v1/yearbooks/"+doomed, "").status(404, "not_found")
	if len(child.called) != 0 || e.count(`SELECT COUNT(*) FROM yearbook_tab`) != 2 {
		t.Fatalf("foreign delete: children %v", child.called)
	}

	// A failing child rolls the profile delete, the child's own delete and the book back.
	child.fail = errors.New("child exploded")
	e.do(alice, "DELETE", "/v1/yearbooks/"+doomed, "").status(500, "internal_error")
	if e.count(`SELECT COUNT(*) FROM yearbook_tab WHERE public_id = ?`, doomed) != 1 || e.count(`SELECT COUNT(*) FROM profile_tab WHERE yearbook_id = ?`, row) != 1 ||
		e.count(`SELECT COUNT(*) FROM media`) != 1 {
		t.Fatal("a failed child purger left the delete half done")
	}
	e.do(alice, "GET", "/v1/yearbooks/"+doomed, "").status(200, "")

	// Without the failure: the child ran once more, saw no profile (deleted before it), and book, profile and its rows are gone.
	child.fail = nil
	child.called = nil
	e.do(alice, "DELETE", "/v1/yearbooks/"+doomed, "").status(204, "")
	if len(child.called) != 1 || child.called[0] != row || child.profiles != 0 {
		t.Fatalf("child called %v, saw %d profiles", child.called, child.profiles)
	}
	for _, q := range []string{`SELECT COUNT(*) FROM profile_tab WHERE yearbook_id = ?`, `SELECT COUNT(*) FROM yearbook_tab WHERE id = ?`} {
		if n := e.count(q, row); n != 0 {
			t.Fatalf("%s: %d rows left", q, n)
		}
	}
	if n := e.count(`SELECT COUNT(*) FROM media`); n != 0 {
		t.Fatalf("child's delete did not commit: %d media rows", n)
	}
	e.do(bob, "GET", "/v1/yearbooks/"+bobs, "").status(200, "")
	if e.count(`SELECT COUNT(*) FROM profile_tab`) != 1 {
		t.Fatal("another book's profile was deleted")
	}
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
		e.do(alice, "PATCH", "/v1/yearbooks/"+one, fmt.Sprintf(`{"cover_media_id":%q}`, bad)).status(400, "invalid_media")
		e.do(alice, "PUT", "/v1/yearbooks/"+one+"/profile", fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, bad)).status(400, "invalid_media")
	}
	b := e.do(alice, "PATCH", "/v1/yearbooks/"+one, fmt.Sprintf(`{"cover_media_id":%q}`, mine)).status(200, "").book()
	if b["cover_media_id"] != mine {
		t.Fatalf("cover not set: %v", b)
	}
	b = e.do(alice, "PUT", "/v1/yearbooks/"+one+"/profile", fmt.Sprintf(`{"full_name":"A","photo_media_id":%q}`, mine)).status(200, "").book()
	if b["cover_media_id"] != mine || b["profile"].(map[string]any)["photo_media_id"] != mine {
		t.Fatalf("photo not set (or cover lost): %v", b)
	}

	mediaRow := e.count(`SELECT id FROM media WHERE public_id = ?`, mine)
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
	b = e.do(alice, "GET", "/v1/yearbooks/"+one, "").status(200, "").book()
	if b["cover_media_id"] != nil || b["profile"].(map[string]any)["photo_media_id"] != nil {
		t.Fatalf("references survive ClearMediaRefs: %v", b)
	}
}

// countingConnector counts the statements (not BEGIN/COMMIT) that go through its connections. It hides the
// optional driver interfaces, so database/sql always prepares and executes through a statement we can see.
type countingConnector struct {
	driver.Connector
	n *atomic.Int64
}

type countingConn struct {
	driver.Conn
	n *atomic.Int64
}

type countingStmt struct {
	driver.Stmt
	n *atomic.Int64
}

func (c countingConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	return countingConn{conn, c.n}, err
}

func (c countingConn) PrepareContext(ctx context.Context, q string) (driver.Stmt, error) {
	st, err := c.Conn.(driver.ConnPrepareContext).PrepareContext(ctx, q)
	return countingStmt{st, c.n}, err
}

func (c countingConn) BeginTx(ctx context.Context, o driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, o)
}

func (c countingConn) CheckNamedValue(nv *driver.NamedValue) error {
	return c.Conn.(driver.NamedValueChecker).CheckNamedValue(nv)
}

func (s countingStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	s.n.Add(1)
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
	c := sql.OpenDB(countingConnector{inner, n})
	t.Cleanup(func() { _ = c.Close() })
	return c
}

// T-064 AC6: reading books does not join and does not loop: the number of queries is the same for 1 and for 50 books
// (book rows, profiles, media public ids).
func TestReadQueryCountIsConstant(t *testing.T) {
	e := newEnv(t)
	e.register("alice@example.com", "Alice")
	var owner uint64
	if err := e.db.QueryRow(`SELECT id FROM users WHERE email = 'alice@example.com'`).Scan(&owner); err != nil {
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
		books, err := st.list(ctx, owner, nil, limit)
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
