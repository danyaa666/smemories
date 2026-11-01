//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-111 (D-35): the terms migration is expand-only. Users and notes that exist before it get NULL (never accepted), the previous
// build (which names none of the new columns) can still insert, and Down then Up loses no row.
func TestMigrateTermsAcceptance(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	has := func() bool {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND ((TABLE_NAME = 'user_tab' AND COLUMN_NAME IN ('terms_version','terms_accepted_at')) OR (TABLE_NAME = 'note_tab' AND COLUMN_NAME = 'terms_version'))`)[0] == "3"
	}
	if !has() {
		t.Fatal("the test database lacks the terms columns")
	}
	for i := 0; i < 30 && has(); i++ { // newer migrations go down first
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if has() {
		t.Fatal("down left a column behind")
	}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	insertPrevious := func(suffix string) { // what the previous build inserts: no terms column
		exec(`INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES (?,?,'x',?,1,1)`, "U"+suffix, "u"+suffix+"@example.com", "user-u"+suffix)
		if suffix == "1" {
			exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, created_at, updated_at)
				SELECT 'B1', id, 'Book', 'School', '12A', 2026, 'Motto', 'en', 'A5', 'classic', 1, 1 FROM user_tab WHERE public_id = 'U1'`)
			exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at) SELECT 'C1', id, UNHEX(SHA2('C1', 256)), 'x', 1, 1 FROM yearbook_tab`)
		}
		exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, created_at, updated_at) SELECT ?, c.id, c.yearbook_id, '{}', 'pending', 1, 1 FROM note_collection_tab c`, "N"+suffix)
	}
	insertPrevious("1")
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	const users = `SELECT public_id, IFNULL(terms_version,'NULL'), IFNULL(terms_accepted_at,'NULL') FROM user_tab ORDER BY id`
	const notes = `SELECT public_id, IFNULL(terms_version,'NULL') FROM note_tab ORDER BY id`
	equalLines(t, "users after up", dump(t, ctx, d, users), []string{"U1|NULL|NULL"})
	equalLines(t, "notes after up", dump(t, ctx, d, notes), []string{"N1|NULL"})
	// The previous build keeps inserting during a rolling deploy; the new one writes the version.
	insertPrevious("2")
	exec(`UPDATE user_tab SET terms_version = '2026-10-21', terms_accepted_at = 1791374400000 WHERE public_id = 'U2'`)
	exec(`UPDATE note_tab SET terms_version = '2026-10-21' WHERE public_id = 'N2'`)
	equalLines(t, "users after writes", dump(t, ctx, d, users), []string{"U1|NULL|NULL", "U2|2026-10-21|1791374400000"})
	equalLines(t, "notes after writes", dump(t, ctx, d, notes), []string{"N1|NULL", "N2|2026-10-21"})

	for i := 0; i < 30 && has(); i++ {
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down again: %v", err)
		}
	}
	if n := dump(t, ctx, d, `SELECT (SELECT COUNT(*) FROM user_tab), (SELECT COUNT(*) FROM note_tab)`); n[0] != "2|2" {
		t.Fatalf("down lost rows: %v", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "users after up again", dump(t, ctx, d, users), []string{"U1|NULL|NULL", "U2|NULL|NULL"})
}
