//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-152 (E09, D-43): the purpose migration is expand-only. Photos that exist before it become 'photo', the previous build (which
// does not know the column) still inserts, and Down removes the design rows (the old schema cannot tell them from photos) and keeps
// every photo; Up again gives every photo the default.
func TestMigrateMediaPurpose(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	hasPurpose := func() bool {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'media_tab' AND COLUMN_NAME = 'purpose'`)[0] == "1"
	}
	if !hasPurpose() {
		t.Fatal("the test database lacks purpose")
	}
	for i := 0; i < 30 && hasPurpose(); i++ { // newer migrations go down first
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if hasPurpose() {
		t.Fatal("down left the column behind")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('U1','u1@example.com','x','user-u1',1,1)`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO yearbook_tab (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, created_at, updated_at)
		SELECT 'B1', id, 'Book', 'School', '12A', 2026, 'Motto', 'en', 'A5', 'classic', 1, 1 FROM user_tab WHERE public_id = 'U1'`); err != nil {
		t.Fatal(err)
	}
	// Photos of the previous build: the insert names no purpose.
	insert := func(extra, pid string) {
		t.Helper()
		if _, err := d.ExecContext(ctx, `INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, `+extra+`object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
			SELECT ?, id, 'owner', `+map[string]string{"": "", "purpose, ": "'design', "}[extra]+`'k', 't', 'image/jpeg', 10, 4, 3, 'x', 1, 1 FROM yearbook_tab WHERE public_id = 'B1'`, pid); err != nil {
			t.Fatal(err)
		}
	}
	insert("", "P1")
	insert("", "P2")
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	const q = `SELECT public_id, purpose FROM media_tab ORDER BY id`
	equalLines(t, "existing photos after up", dump(t, ctx, d, q), []string{"P1|photo", "P2|photo"})
	insert("", "P3") // a rolling deploy: the previous build inserts while the column exists
	insert("purpose, ", "D1")
	equalLines(t, "after a design insert", dump(t, ctx, d, q), []string{"P1|photo", "P2|photo", "P3|photo", "D1|design"})

	for i := 0; i < 30 && hasPurpose(); i++ {
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down again: %v", err)
		}
	}
	equalLines(t, "photos after down", dump(t, ctx, d, `SELECT public_id FROM media_tab ORDER BY id`), []string{"P1", "P2", "P3"})
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "after up again", dump(t, ctx, d, q), []string{"P1|photo", "P2|photo", "P3|photo"})
}
