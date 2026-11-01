//go:build integration

package db_test

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-026 (E07): the class migration is expand-only (two new tables), goes down and up again, and the keys keep the rules the
// API relies on: one membership per user and class, one class per yearbook, any number of members without a yearbook.
func TestMigrateClassTables(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	hasTables := func() string {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('class_tab','class_member_tab')`)[0]
	}
	if hasTables() != "2" {
		t.Fatal("the test database lacks the class tables")
	}

	exec := func(q string, args ...any) error {
		_, err := d.ExecContext(ctx, q, args...)
		return err
	}
	if err := exec(`INSERT INTO class_tab (public_id, name, language, created_by, created_at, updated_at) VALUES ('C1','Class','en',1,1,1)`); err != nil {
		t.Fatal(err)
	}
	member := func(pid string, user int, yearbook any) error {
		return exec(`INSERT INTO class_member_tab (public_id, class_id, user_id, role, yearbook_id, joined_at, created_at, updated_at) VALUES (?,1,?,'student',?,1,1,1)`, pid, user, yearbook)
	}
	for _, step := range []struct {
		name    string
		err     error
		wantErr bool
	}{
		{"first member, no yearbook", member("M1", 1, nil), false},
		{"second member, no yearbook (NULLs do not collide)", member("M2", 2, nil), false},
		{"same user twice in a class", member("M3", 1, nil), true},
		{"a yearbook for one member", member("M4", 3, 7), false},
		{"the same yearbook for another member", member("M5", 4, 7), true},
		{"same public id", member("M1", 5, nil), true},
	} {
		if (step.err != nil) != step.wantErr {
			t.Errorf("%s: err = %v", step.name, step.err)
		}
	}

	for i := 0; i < 30 && hasTables() != "0"; i++ { // newer migrations go down first
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if hasTables() != "0" {
		t.Fatal("down left the tables behind")
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if hasTables() != "2" {
		t.Fatal("up again did not create the tables")
	}
}

// T-125 (E07): the invite table keeps one row per token hash and per public id, and goes down and up again on its own.
func TestMigrateClassInviteTable(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	exists := func() string {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'class_invite_tab'`)[0]
	}
	invite := func(pid, hash string) error {
		_, err := d.ExecContext(ctx, `INSERT INTO class_invite_tab (public_id, class_id, token_hash, created_by, expires_at, created_at, updated_at) VALUES (?,1,UNHEX(SHA2(?,256)),1,2,1,1)`, pid, hash)
		return err
	}
	if exists() != "1" {
		t.Fatal("the test database lacks class_invite_tab")
	}
	if err := invite("I1", "t1"); err != nil {
		t.Fatal(err)
	}
	if err := invite("I2", "t1"); err == nil {
		t.Error("two invites with the same token hash")
	}
	if err := invite("I1", "t2"); err == nil {
		t.Error("two invites with the same public id")
	}
	if err := invite("I3", "t3"); err != nil {
		t.Error(err)
	}
	for i := 0; i < 30 && exists() != "0"; i++ { // newer migrations go down first
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if exists() != "0" || dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'class_tab'`)[0] != "1" {
		t.Fatal("down must drop class_invite_tab and keep class_tab")
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if exists() != "1" {
		t.Fatal("up again did not create the table")
	}
}

// T-127 (E07): the class page table keeps one row per public id, takes a 4000-character body of any text, and goes down and up on its own.
func TestMigrateClassPageTable(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	exists := func(table string) string {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '`+table+`'`)[0]
	}
	page := func(pid, body string) error {
		_, err := d.ExecContext(ctx, `INSERT INTO class_page_tab (public_id, class_id, kind, title, body, placement, sort_order, created_at, updated_at) VALUES (?,1,'intro','Title',?,'front',1,1,1)`, pid, body)
		return err
	}
	if exists("class_page_tab") != "1" {
		t.Fatal("the test database lacks class_page_tab")
	}
	if err := page("P1", strings.Repeat("🎓", 4000)); err != nil { // 4000 four-byte characters
		t.Fatal(err)
	}
	if err := page("P1", "again"); err == nil {
		t.Error("two pages with the same public id")
	}
	if got := dump(t, ctx, d, `SELECT CHAR_LENGTH(body) FROM class_page_tab WHERE public_id = 'P1'`)[0]; got != "4000" {
		t.Errorf("the body was cut: %s characters", got)
	}
	for i := 0; i < 30 && exists("class_page_tab") == "1"; i++ { // newer migrations (T-141, T-152) go down first, then this one
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if exists("class_page_tab") != "0" || exists("class_invite_tab") != "1" {
		t.Fatal("down must drop only class_page_tab")
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if exists("class_page_tab") != "1" {
		t.Fatal("up again did not create the table")
	}
}

// T-141 (E09): the book design table keeps one row per yearbook, has no foreign key, and goes down and up again on its own.
func TestMigrateBookDesignTable(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	exists := func() string {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'book_design_tab'`)[0]
	}
	design := func(yearbook int) error {
		_, err := d.ExecContext(ctx, `INSERT INTO book_design_tab (yearbook_id, template_id, options_json, created_at, updated_at) VALUES (?,'memory-book','{"v":1}',1,1)`, yearbook)
		return err
	}
	if exists() != "1" {
		t.Fatal("the test database lacks book_design_tab")
	}
	if err := design(7); err != nil { // no yearbook 7 exists: there is no foreign key
		t.Fatal(err)
	}
	if err := design(7); err == nil {
		t.Error("a second design for one yearbook was accepted")
	}
	if err := design(8); err != nil {
		t.Errorf("a design for another yearbook: %v", err)
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'book_design_tab'`)[0]; n != "0" {
		t.Errorf("%s foreign keys on book_design_tab", n)
	}
	for i := 0; i < 30 && exists() != "0"; i++ { // newer migrations go down first
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if exists() != "0" {
		t.Fatal("down left the table behind")
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if exists() != "1" {
		t.Fatal("up again did not create the table")
	}
}

// T-153 (E09 stage 2): the book layout table keeps one row per yearbook, has no foreign key, holds a 128 KiB document and goes down and up
// again on its own, newest migration first, leaving the rows of the other tables alone.
func TestMigrateBookLayoutTable(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	exists := func() string {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'book_layout_tab'`)[0]
	}
	layout := func(yearbook int, doc string) error {
		_, err := d.ExecContext(ctx, `INSERT INTO book_layout_tab (yearbook_id, template_id, layout_json, version, created_at, updated_at) VALUES (?,'memory-book',?,1,1,1)`, yearbook, doc)
		return err
	}
	if exists() != "1" {
		t.Fatal("the test database lacks book_layout_tab")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO book_design_tab (yearbook_id, template_id, options_json, created_at, updated_at) VALUES (7,'memory-book','{"v":1}',1,1)`); err != nil {
		t.Fatal(err)
	}
	big := `{"v":1,"pages":{},"pad":"` + strings.Repeat("ệ", 40000) + `"}` // about 120 KB of three-byte characters
	if err := layout(7, big); err != nil {                                 // no yearbook 7 exists: there is no foreign key
		t.Fatal(err)
	}
	if got := dump(t, ctx, d, `SELECT CHAR_LENGTH(layout_json) FROM book_layout_tab WHERE yearbook_id = 7`)[0]; got != fmt.Sprint(len([]rune(big))) {
		t.Errorf("stored %s characters, want %d", got, len([]rune(big)))
	}
	if err := layout(7, "{}"); err == nil {
		t.Error("a second layout for one yearbook was accepted")
	}
	if err := layout(8, "{}"); err != nil {
		t.Errorf("a layout for another yearbook: %v", err)
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'book_layout_tab'`)[0]; n != "0" {
		t.Errorf("%s foreign keys on book_layout_tab", n)
	}
	for range 3 { // the newest migrations: T-163's version table, T-156's layout_version column, and before them this table
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if exists() != "0" {
		t.Fatal("down left the table behind")
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM book_design_tab`)[0]; n != "1" {
		t.Errorf("down touched the design table: %s rows", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if exists() != "1" || dump(t, ctx, d, `SELECT COUNT(*) FROM book_layout_tab`)[0] != "0" {
		t.Fatal("up again did not create an empty table")
	}
}

// T-156: yearbook_tab.layout_version is backfilled from the layout rows (stored versions are preserved, a book without a layout stays 0),
// goes down without touching the rows and comes back with the same values.
func TestMigrateYearbookLayoutVersion(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	versions := func() string {
		return strings.Join(dump(t, ctx, d, `SELECT CONCAT(title, '=', layout_version) FROM yearbook_tab ORDER BY title`), ",")
	}
	for range 2 { // T-163's version table is newer than the column
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'yearbook_tab' AND COLUMN_NAME = 'layout_version'`)[0]; n != "0" {
		t.Fatal("down left the column behind")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) VALUES
		('01AAAAAAAAAAAAAAAAAAAAAAA1', 1, 'with', 'en', 1, 1), ('01AAAAAAAAAAAAAAAAAAAAAAA2', 1, 'without', 'en', 1, 1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO book_layout_tab (yearbook_id, template_id, layout_json, version, created_at, updated_at)
		SELECT id, 'memory-book', '{}', 7, 1, 1 FROM yearbook_tab WHERE title = 'with'`); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	if got := versions(); got != "with=7,without=0" {
		t.Fatalf("after up: %s", got)
	}
	for range 2 {
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down again: %v", err)
		}
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM yearbook_tab`)[0]; n != "2" {
		t.Fatalf("down touched the rows: %s", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if got := versions(); got != "with=7,without=0" {
		t.Fatalf("after up again: %s", got)
	}
}

// T-163 (E09): the layout history table has no foreign key, is unique per book and version, accepts a 128 KiB document and goes down and up
// again on its own, leaving the layout rows alone.
func TestMigrateBookLayoutVersionTable(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	exists := func() string {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'book_layout_version_tab'`)[0]
	}
	snap := func(public string, yearbook, version int, doc string) error {
		_, err := d.ExecContext(ctx, `INSERT INTO book_layout_version_tab (public_id, yearbook_id, version, kind, template_id, layout_json, item_count, bytes, created_at)
			VALUES (?,?,?,'auto','memory-book',?,0,?,1)`, public, yearbook, version, doc, len(doc))
		return err
	}
	if exists() != "1" {
		t.Fatal("the test database lacks book_layout_version_tab")
	}
	big := `{"v":1,"pages":{},"pad":"` + strings.Repeat("ệ", 40000) + `"}`
	if err := snap("01AAAAAAAAAAAAAAAAAAAAAAA1", 7, 1, big); err != nil { // no yearbook 7 exists: there is no foreign key
		t.Fatal(err)
	}
	if got := dump(t, ctx, d, `SELECT CHAR_LENGTH(layout_json) FROM book_layout_version_tab WHERE yearbook_id = 7`)[0]; got != fmt.Sprint(len([]rune(big))) {
		t.Errorf("stored %s characters, want %d", got, len([]rune(big)))
	}
	if err := snap("01AAAAAAAAAAAAAAAAAAAAAAA2", 7, 1, "{}"); err == nil {
		t.Error("the same version of one book was stored twice")
	}
	if err := snap("01AAAAAAAAAAAAAAAAAAAAAAA1", 8, 1, "{}"); err == nil {
		t.Error("a public id was used twice")
	}
	if err := snap("01AAAAAAAAAAAAAAAAAAAAAAA3", 7, 2, "{}"); err != nil {
		t.Errorf("another version of the book: %v", err)
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'book_layout_version_tab'`)[0]; n != "0" {
		t.Errorf("%s foreign keys on book_layout_version_tab", n)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO book_layout_tab (yearbook_id, template_id, layout_json, version, created_at, updated_at) VALUES (9,'memory-book','{}',1,1,1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateDown(ctx, d); err != nil { // the newest migration is this table's
		t.Fatalf("down: %v", err)
	}
	if exists() != "0" {
		t.Fatal("down left the table behind")
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM book_layout_tab`)[0]; n != "1" {
		t.Errorf("down touched the layout table: %s rows", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	if exists() != "1" || dump(t, ctx, d, `SELECT COUNT(*) FROM book_layout_version_tab`)[0] != "0" {
		t.Fatal("up again did not create an empty table")
	}
}
