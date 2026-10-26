//go:build integration

package db_test

import (
	"context"
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

	for i := 0; i < 10 && hasTables() != "0"; i++ { // newer migrations go down first
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
	for i := 0; i < 10 && exists() != "0"; i++ { // newer migrations go down first
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
	if err := db.MigrateDown(ctx, d); err != nil {
		t.Fatalf("down: %v", err)
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
