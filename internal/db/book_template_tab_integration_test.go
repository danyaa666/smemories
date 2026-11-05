//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-173 (E09): the own page template table is expand-only (one new table), keeps one row per owner and public id, and goes down and up
// again without touching the layout tables.
func TestMigrateBookTemplateTab(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	count := func(table string) string {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '`+table+`'`)[0]
	}
	if count("book_template_tab") != "1" {
		t.Fatal("the test database lacks book_template_tab")
	}
	if got := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'book_template_tab' AND INDEX_NAME IN ('uq_book_template_tab_owner_id_public_id','idx_book_template_tab_owner_id_created_at')`)[0]; got != "4" {
		t.Fatalf("index columns: %s, want 4 (two keys of two columns)", got)
	}
	ins := func(owner int, pid string) error {
		_, err := d.ExecContext(ctx, `INSERT INTO book_template_tab (public_id, owner_id, name, kind, template_id, layout_json, item_count, bytes, created_at) VALUES (?,?,'n','cover','memory-book','{}',0,2,1)`, pid, owner)
		return err
	}
	if err := ins(1, "P1"); err != nil {
		t.Fatal(err)
	}
	if err := ins(2, "P1"); err != nil {
		t.Errorf("another owner with the same public id: %v", err)
	}
	if err := ins(1, "P1"); err == nil {
		t.Error("the same owner and public id twice was accepted")
	}
	if got := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.KEY_COLUMN_USAGE WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'book_template_tab' AND REFERENCED_TABLE_NAME IS NOT NULL`)[0]; got != "0" {
		t.Errorf("%s foreign keys, want none (docs/db-conventions.md)", got)
	}

	for i := 0; i < 30 && count("book_template_tab") != "0"; i++ { // newer migrations go down first
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if count("book_template_tab") != "0" || count("book_layout_version_tab") != "1" {
		t.Fatal("down must drop book_template_tab and keep the layout tables")
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if count("book_template_tab") != "1" {
		t.Fatal("up again did not create the table")
	}
}
