//go:build integration

package booklayout_test

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/booklayout"
)

// E09 T-179: the storage a user can fill, measured on the real tables (docs/layout-storage-budget.md quotes the lines this test logs;
// run it with `go test -tags integration -run TestMaximalUserStorage -v ./internal/booklayout`).

// biggestUsers is the operator's query: the users that hold the most layout bytes (current layouts, snapshots, own templates). The doc
// carries the same text; the test runs it and checks that.
const biggestUsers = `SELECT owner_id, SUM(bytes) AS total_bytes, COUNT(*) AS rows_held FROM (
  SELECT y.owner_id, LENGTH(l.layout_json) AS bytes FROM book_layout_tab l JOIN yearbook_tab y ON y.id = l.yearbook_id
  UNION ALL SELECT y.owner_id, v.bytes FROM book_layout_version_tab v JOIN yearbook_tab y ON y.id = v.yearbook_id
  UNION ALL SELECT t.owner_id, t.bytes FROM book_template_tab t
) held GROUP BY owner_id ORDER BY total_bytes DESC LIMIT 20`

// maximalDoc is a valid document of about 127 KB as stored: 150 text items (40 + 40 + 40 + 30) of letters 3-byte letters each. seed makes
// every call a different document, so no two snapshots are the same bytes.
func maximalDoc(seed, letters int) string {
	char := string(rune('ệ' + seed%2)) // two different 3-byte letters
	var pages []string
	for pi, k := range []string{"cover", "profile", "notes", "back"} {
		n := 40
		if k == "back" {
			n = 30
		}
		var items []string
		for i := range n {
			items = append(items, fmt.Sprintf(`{"id":"%c%05d","t":"text","text":"%s%d","x":0,"w":300,"h":200,"cy":500}`, 'a'+pi, i, strings.Repeat(char, letters), seed))
		}
		pages = append(pages, `"`+k+`":{"items":[`+strings.Join(items, ",")+`]}`)
	}
	return `{"v":1,"pages":{` + strings.Join(pages, ",") + `}}`
}

func TestMaximalUserStorage(t *testing.T) {
	e := newEnv(t, nil, 0)
	alice := e.register("alice@example.com")
	book := e.book(alice, "memory-book")
	const letters = 250 // 150 items of 250 three-byte letters: the largest document the item limits allow inside 128 KiB, give or take
	if n := len(maximalDoc(0, letters)); n < 120<<10 || n > booklayout.MaxBytes {
		t.Fatalf("a maximal document is %d bytes, want 120 KiB to 128 KiB", n)
	}
	// own templates first (the page they copy is replaced afterwards): 20 pages of 40 items of 500 three-byte letters, about 64 KiB each
	page := `{"v":1,"pages":{"cover":{"items":[`
	var items []string
	for i := range 40 {
		items = append(items, fmt.Sprintf(`{"id":"t%05d","t":"text","text":"%s","x":0,"w":300,"h":200,"cy":500}`, i, strings.Repeat("ệ", 500)))
	}
	big := page + strings.Join(items, ",") + `]}}}`
	e.call("POST", "/api/book-layout/save", alice, save(book, 0, big)).status(200, "OK")
	for i := range booklayout.MaxOwnTemplates {
		e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", fmt.Sprintf("Template %d", i))).status(200, "OK")
	}
	e.call("POST", "/api/book-layout/template-create", alice, tplCreate(book, "cover", "one too many")).status(409, "ERROR_TEMPLATE_LIMIT")

	// the current layout and 30 snapshots: 31 saves, each past the snapshot gap, each a different maximal document (the first one replaces the
	// template source, which becomes a snapshot that the 31st save prunes)
	for i := range booklayout.MaxSnapshots + 1 {
		e.call("POST", "/api/book-layout/save", alice, save(book, i+1, maximalDoc(i, letters))).status(200, "OK")
		e.now = e.now.Add(booklayout.SnapshotGap + time.Minute)
	}
	if n := e.count(`SELECT COUNT(*) FROM book_layout_version_tab`); n != booklayout.MaxSnapshots {
		t.Fatalf("%d snapshots, want %d", n, booklayout.MaxSnapshots)
	}
	type size struct{ rows, payload, avg, max int }
	measure := func(table, col string) size {
		var s size
		if err := e.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*), COALESCE(SUM(LENGTH(%s)),0), COALESCE(CAST(AVG(LENGTH(%s)) AS SIGNED),0), COALESCE(MAX(LENGTH(%s)),0) FROM %s`, col, col, col, table)).
			Scan(&s.rows, &s.payload, &s.avg, &s.max); err != nil {
			t.Fatal(err)
		}
		return s
	}
	layout, snaps, tpls := measure("book_layout_tab", "layout_json"), measure("book_layout_version_tab", "layout_json"), measure("book_template_tab", "layout_json")
	t.Logf("payload  book_layout_tab          rows=%d bytes=%d max=%d", layout.rows, layout.payload, layout.max)
	t.Logf("payload  book_layout_version_tab  rows=%d bytes=%d avg=%d max=%d", snaps.rows, snaps.payload, snaps.avg, snaps.max)
	t.Logf("payload  book_template_tab        rows=%d bytes=%d avg=%d max=%d", tpls.rows, tpls.payload, tpls.avg, tpls.max)
	if layout.max > booklayout.MaxBytes || snaps.max > booklayout.MaxBytes || tpls.max > booklayout.MaxTemplateBytes {
		t.Errorf("a stored document is over its limit")
	}
	if snaps.rows != booklayout.MaxSnapshots || tpls.rows != booklayout.MaxOwnTemplates || layout.rows != 1 {
		t.Errorf("rows: %d layout, %d snapshots, %d templates", layout.rows, snaps.rows, tpls.rows)
	}
	worstBook := (1 + booklayout.MaxSnapshots) * booklayout.MaxBytes
	if layout.payload+snaps.payload > worstBook || tpls.payload > booklayout.MaxOwnTemplates*booklayout.MaxTemplateBytes {
		t.Errorf("more bytes than the worst case of the budget: book %d of %d, templates %d", layout.payload+snaps.payload, worstBook, tpls.payload)
	}

	// what the tables hold on disk: pages as MySQL allocated them, after ANALYZE
	for _, tbl := range []string{"book_layout_tab", "book_layout_version_tab", "book_template_tab"} {
		e.exec(`ANALYZE TABLE ` + tbl)
		var data, index int64
		if err := e.db.QueryRow(`SELECT data_length, index_length FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`, tbl).Scan(&data, &index); err != nil {
			t.Fatal(err)
		}
		line := fmt.Sprintf("on disk  %-26s data_length=%d index_length=%d", tbl, data, index)
		var file, alloc int64
		if err := e.db.QueryRow(`SELECT file_size, allocated_size FROM information_schema.innodb_tablespaces WHERE name = CONCAT(DATABASE(), '/', ?)`, tbl).Scan(&file, &alloc); err == nil {
			line += fmt.Sprintf(" file_size=%d allocated_size=%d", file, alloc)
		}
		t.Log(line)
	}

	// the operator's query runs and names this user first with every byte accounted for
	var owner, total, held int64
	if err := e.db.QueryRow(biggestUsers).Scan(&owner, &total, &held); err != nil {
		t.Fatalf("the operator query: %v", err)
	}
	if want := int64(layout.payload + snaps.payload + tpls.payload); total != want || held != 1+int64(booklayout.MaxSnapshots)+int64(booklayout.MaxOwnTemplates) {
		t.Errorf("operator query: %d bytes in %d rows, want %d in %d", total, held, want, 1+booklayout.MaxSnapshots+booklayout.MaxOwnTemplates)
	}
	t.Logf("operator query: owner %d holds %d bytes in %d rows", owner, total, held)
	doc, err := os.ReadFile("../../docs/layout-storage-budget.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(doc), biggestUsers) {
		t.Error("docs/layout-storage-budget.md does not carry the operator query of this test verbatim")
	}
}
