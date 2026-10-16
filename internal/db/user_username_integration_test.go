//go:build integration

package db_test

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-081 AC1: the username migration gives every existing user a unique, valid name (user- plus the last 8 characters of the
// public id, lower-cased), resolves a forced clash instead of failing, adds the unique key, and Down removes both columns and
// the key; Up again gives the same names.
func TestMigrateUserUsername(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	hasUsername := func() bool {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user_tab' AND COLUMN_NAME IN ('username', 'username_changed_at')`)[0] == "2"
	}
	if !hasUsername() {
		t.Fatal("the test database lacks the username columns")
	}
	if err := db.MigrateDown(ctx, d); err != nil { // the newest migration is the username one
		t.Fatalf("down: %v", err)
	}
	if hasUsername() || dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND INDEX_NAME = 'uq_user_tab_username'`)[0] != "0" {
		t.Fatal("down left the columns or the key behind")
	}
	// A real ULID, a short test id, and three ids with the same last 8 characters: a forced clash in the backfill.
	for i, pid := range []string{"01K7Q8W3ZR5N2M4X6V8B0C1D2E", "U1", "AAAA12345678", "bbbb12345678", "CCCC12345678"} {
		if _, err := d.ExecContext(ctx, `INSERT INTO user_tab (public_id, email, display_name, created_at, updated_at) VALUES (?,?,'x',1,1)`, pid, strings.ToLower(pid)+"@example.com"); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	const q = `SELECT public_id, username, username_changed_at FROM user_tab ORDER BY id`
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	got := dump(t, ctx, d, q)
	valid := regexp.MustCompile(`^[a-z0-9]([a-z0-9]|[._-][a-z0-9])*$`)
	seen := map[string]bool{}
	for _, row := range got {
		f := strings.Split(row, "|")
		if !valid.MatchString(f[1]) || len(f[1]) < 3 || len(f[1]) > 30 || seen[f[1]] || f[2] != "0" {
			t.Errorf("bad backfill row %q", row)
		}
		seen[f[1]] = true
	}
	if got[0] != "01K7Q8W3ZR5N2M4X6V8B0C1D2E|user-8b0c1d2e|0" {
		t.Errorf("ULID row = %q", got[0])
	}
	if got[1] != "U1|user-u1|0" {
		t.Errorf("short id row = %q", got[1])
	}
	// the first of the clashing group keeps the plain name; the others get their row id appended
	if !strings.HasSuffix(got[2], "|user-12345678|0") || !regexp.MustCompile(`\|user-12345678-\d+\|0$`).MatchString(got[3]) || !regexp.MustCompile(`\|user-12345678-\d+\|0$`).MatchString(got[4]) {
		t.Errorf("clash rows = %q %q %q", got[2], got[3], got[4])
	}
	// the key is unique and case-insensitive
	if _, err := d.ExecContext(ctx, `INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('Z1','z@example.com','x','USER-U1',1,1)`); err == nil {
		t.Error("a duplicate username (other case) was accepted")
	}

	if err := db.MigrateDown(ctx, d); err != nil {
		t.Fatalf("down again: %v", err)
	}
	if hasUsername() {
		t.Fatal("down again left the columns")
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM user_tab`); n[0] != "5" {
		t.Fatalf("down lost users: %v", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "after up again", dump(t, ctx, d, q), got)
}
