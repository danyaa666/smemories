//go:build integration

package db_test

import (
	"context"
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

	if err := db.MigrateDown(ctx, d); err != nil { // the class migration is the newest
		t.Fatalf("down: %v", err)
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
