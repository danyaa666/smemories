//go:build integration

package db_test

import (
	"context"
	"testing"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// QA T-065: the media -> media_tab migration with photos that are also cover, profile photo and note photo. Ids must survive
// (the references are plain ids now), a row added while migrated survives Down,
// and Up/Down/Up gives the same rows.
func TestQAMigrateMediaTabReferencesAndNotePhotos(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "media") })
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (public_id, email, display_name, created_at, updated_at) VALUES ('U1', 'a@example.com', 'A', NOW(6), NOW(6))`)
	exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) SELECT 'B1', id, 'One', 'en', 1, 1 FROM users`)
	exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) SELECT 'B2', id, 'Two', 'en', 1, 1 FROM users`)
	for _, m := range []struct{ id, book, kind, created string }{
		{"M1", "B1", "owner", "2026-10-07 12:34:56.999999"}, // cover
		{"M2", "B1", "owner", "2026-10-07 12:34:56.000001"}, // profile photo
		{"M3", "B1", "contributor", "2026-10-07 23:59:59.999999"},
		{"M4", "B2", "contributor", "2000-02-29 00:00:00.000000"},
		{"M5", "B2", "owner", "1970-01-01 00:00:00.000999"},
	} {
		exec(`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at)
			SELECT ?, id, ?, ?, ?, 'image/png', 7, 3, 4, REPEAT('b', 64), ? FROM yearbook_tab WHERE public_id = ?`, m.id, m.kind, "k/"+m.id, "k/"+m.id+"-t", m.created, m.book)
	}
	exec(`UPDATE yearbook_tab SET cover_media_id = (SELECT id FROM media WHERE public_id = 'M1') WHERE public_id = 'B1'`)
	exec(`INSERT INTO profile_tab (public_id, yearbook_id, is_owner, full_name, photo_media_id, created_at, updated_at)
		SELECT 'P1', y.id, TRUE, 'N', (SELECT id FROM media WHERE public_id = 'M2'), 1, 1 FROM yearbook_tab y WHERE y.public_id = 'B1'`)
	exec(`INSERT INTO note_collections (public_id, yearbook_id, token_hash, created_at) SELECT 'C1', id, REPEAT('x', 32), NOW(6) FROM yearbook_tab WHERE public_id = 'B1'`)
	exec(`INSERT INTO notes (public_id, collection_id, yearbook_id, answers, created_at) SELECT 'N1', c.id, c.yearbook_id, '{}', NOW(6) FROM note_collections c`)
	exec(`INSERT INTO note_photos (note_id, media_id, position) SELECT n.id, m.id, 0 FROM notes n, media m WHERE m.public_id = 'M3'`)

	refsQ := func(photos string) string {
		return `SELECT (SELECT cover_media_id FROM yearbook_tab WHERE public_id='B1'), (SELECT photo_media_id FROM profile_tab WHERE public_id='P1'),
		(SELECT GROUP_CONCAT(media_id) FROM ` + photos + `)`
	}
	refs := dump(t, ctx, d, refsQ("note_photos"))
	idsOld := dump(t, ctx, d, `SELECT public_id, id FROM media ORDER BY id`)

	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	equalLines(t, "ids after up", dump(t, ctx, d, `SELECT public_id, id FROM media_tab ORDER BY id`), idsOld)
	equalLines(t, "refs after up", dump(t, ctx, d, refsQ("note_photo_tab")), refs) // T-066 renamed note_photos
	if got := dump(t, ctx, d, `SELECT created_at FROM media_tab WHERE public_id = 'M5'`); got[0] != "0" {
		t.Fatalf("M5 (999 microseconds after the epoch) = %v ms, want 0", got)
	}
	if got := dump(t, ctx, d, `SELECT created_at FROM media_tab WHERE public_id = 'M1'`); got[0] != "1791376496999" {
		t.Fatalf("M1 = %v", got)
	}
	// Since T-066 no foreign key is left: the note link, like the cover and profile references, is cleared by the services.
	exec(`DELETE FROM media_tab WHERE public_id = 'M3'`)
	if got := dump(t, ctx, d, `SELECT COUNT(*) FROM note_photo_tab`); got[0] != "1" {
		t.Fatalf("note_photo_tab cascaded without a foreign key: %v", got)
	}
	exec(`DELETE FROM note_photo_tab`)
	exec(`DELETE FROM media_tab WHERE public_id = 'M2'`) // no FK: the profile keeps naming the id until ClearMediaRefs does it
	exec(`DELETE FROM profile_tab`)
	// A row written by the new code, and an orphan row that only the old foreign key would have prevented.
	exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		SELECT 'M9', id, 'owner', 'k/M9', 'k/M9-t', 'k/M9-p', 'image/jpeg', 9, 9, 9, REPEAT('c', 64), 1760000000123, 1760000000999 FROM yearbook_tab WHERE public_id = 'B2'`)
	exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
		VALUES ('MORPHAN', 4242, 'owner', 'k/o', 'k/o-t', 'image/png', 1, 1, 1, REPEAT('d', 64), 5, 5)`)
	before := dump(t, ctx, d, `SELECT public_id, yearbook_id, uploader_kind, print_key, created_at FROM media_tab WHERE public_id <> 'MORPHAN' ORDER BY id`)

	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "media") })
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM media WHERE public_id = 'MORPHAN'`); n[0] != "0" {
		t.Fatalf("orphan row survived Down: %v", n)
	}
	equalLines(t, "ids after down", dump(t, ctx, d, `SELECT public_id, id FROM media WHERE public_id IN ('M1','M4','M5','M9') ORDER BY id`),
		[]string{"M1|1", "M4|4", "M5|5", "M9|6"})
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "after up/down/up", dump(t, ctx, d, `SELECT public_id, yearbook_id, uploader_kind, print_key, created_at FROM media_tab ORDER BY id`), before)
}
