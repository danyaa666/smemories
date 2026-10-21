//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-092 (D-32): the link_epoch migration is expand-only. Books that exist before it get epoch 0, the API of the previous build (which does
// not know the column) can still insert, the new build bumps it, and Down then Up gives every book epoch 0 again.
func TestMigrateYearbookLinkEpoch(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	hasEpoch := func() bool {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'yearbook_tab' AND COLUMN_NAME = 'link_epoch'`)[0] == "1"
	}
	if !hasEpoch() {
		t.Fatal("the test database lacks link_epoch")
	}
	for i := 0; i < 10 && hasEpoch(); i++ { // newer migrations go down first
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if hasEpoch() {
		t.Fatal("down left the column behind")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('U1','u1@example.com','x','user-u1',1,1)`); err != nil {
		t.Fatal(err)
	}
	// A book of the previous build: the insert names no link_epoch.
	for _, pid := range []string{"B1", "B2"} {
		if _, err := d.ExecContext(ctx, `INSERT INTO yearbook_tab (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, created_at, updated_at)
			SELECT ?, id, 'Book', 'School', '12A', 2026, 'Motto', 'en', 'A5', 'classic', 1, 1 FROM user_tab WHERE public_id = 'U1'`, pid); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	const q = `SELECT public_id, link_epoch FROM yearbook_tab ORDER BY id`
	equalLines(t, "existing books after up", dump(t, ctx, d, q), []string{"B1|0", "B2|0"})
	// A book inserted by the previous build while the new column exists (a rolling deploy) also starts at 0.
	if _, err := d.ExecContext(ctx, `INSERT INTO yearbook_tab (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, created_at, updated_at)
		SELECT 'B3', id, 'Book', 'School', '12A', 2026, 'Motto', 'en', 'A5', 'classic', 1, 1 FROM user_tab WHERE public_id = 'U1'`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `UPDATE yearbook_tab SET link_epoch = link_epoch + 1 WHERE public_id = 'B1'`); err != nil {
		t.Fatal(err)
	}
	equalLines(t, "after a bump", dump(t, ctx, d, q), []string{"B1|1", "B2|0", "B3|0"})

	for i := 0; i < 10 && hasEpoch(); i++ {
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down again: %v", err)
		}
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM yearbook_tab`); n[0] != "3" {
		t.Fatalf("down lost books: %v", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "after up again", dump(t, ctx, d, q), []string{"B1|0", "B2|0", "B3|0"})
}
