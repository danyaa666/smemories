//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-095: the signature_json migration is expand-only. Notes that exist before it have no signature (NULL), a note inserted by the
// previous build (which does not name the column) still works, the new build stores a signature, and Down then Up keeps every note.
func TestMigrateNoteSignature(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	has := func() bool {
		return dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'note_tab' AND COLUMN_NAME = 'signature_json'`)[0] == "1"
	}
	if !has() {
		t.Fatal("the test database lacks signature_json")
	}
	for i := 0; i < 10 && has(); i++ { // newer migrations go down first
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if has() {
		t.Fatal("down left the column behind")
	}
	for _, q := range []string{
		`INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('U1','u1@example.com','x','user-u1',1,1)`,
		`INSERT INTO yearbook_tab (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, created_at, updated_at)
			SELECT 'B1', id, 'Book', 'School', '12A', 2026, 'Motto', 'en', 'A5', 'classic', 1, 1 FROM user_tab WHERE public_id = 'U1'`,
		`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at) SELECT 'C1', id, UNHEX(REPEAT('11',32)), '', 1, 1 FROM yearbook_tab`,
	} {
		if _, err := d.ExecContext(ctx, q); err != nil {
			t.Fatal(err)
		}
	}
	insert := func(pid string) {
		t.Helper()
		if _, err := d.ExecContext(ctx, `INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, terms_version, created_at, updated_at)
			SELECT ?, c.id, c.yearbook_id, '{"name":"A"}', 'pending', '', 1, 1 FROM note_collection_tab c`, pid); err != nil {
			t.Fatal(err)
		}
	}
	insert("N1") // a note of the previous build
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	const q = `SELECT public_id, IFNULL(signature_json, 'NULL') FROM note_tab ORDER BY id`
	equalLines(t, "existing note after up", dump(t, ctx, d, q), []string{"N1|NULL"})
	insert("N2") // the previous build during a rolling deploy
	const sig = `{"v":1,"w":1000,"h":400,"s":[[0,0,500,0]]}`
	if _, err := d.ExecContext(ctx, `UPDATE note_tab SET signature_json = ? WHERE public_id = 'N2'`, sig); err != nil {
		t.Fatal(err)
	}
	equalLines(t, "after a signature", dump(t, ctx, d, q), []string{"N1|NULL", "N2|" + sig})
	if _, err := d.ExecContext(ctx, `UPDATE note_tab SET signature_json = ? WHERE public_id = 'N1'`, string(make([]byte, 3073))); err == nil {
		t.Error("a value over 3072 characters was stored")
	}

	for i := 0; i < 10 && has(); i++ {
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down again: %v", err)
		}
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM note_tab`); n[0] != "2" {
		t.Fatalf("down lost notes: %v", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "after up again", dump(t, ctx, d, q), []string{"N1|NULL", "N2|NULL"})
}
