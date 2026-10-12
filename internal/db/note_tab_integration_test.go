//go:build integration

package db_test

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-066 AC1: note_collections, notes and note_photos -> *_tab keep every value (DATETIME(6) -> Unix ms, NULL deadline and revocation
// stay NULL, ENUM -> VARCHAR), lose their foreign keys, and Down restores the rows and the keys; Up again gives the same rows.
func TestMigrateNoteTabConventions(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "note_collections") }) // the schema before T-066
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (public_id, email, display_name, created_at, updated_at) VALUES ('U1', 'a@example.com', 'A', NOW(6), NOW(6))`)
	exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) SELECT 'B1', id, 'One', 'en', 1, 1 FROM users`)
	// Collections: no deadline and not revoked; deadline and revoked, with microseconds; before 1970; past the 32-bit limits.
	for _, c := range []struct{ id, deadline, created, revoked string }{
		{"C1", "", "2026-10-07 12:34:56.123456", ""},
		{"C2", "2026-10-09 00:00:00.999999", "2026-10-07 12:34:56.000001", "2026-10-08 01:02:03.456789"},
		{"C3", "2038-01-19 03:14:08.000000", "1969-12-31 23:59:59.123000", ""},
		{"C4", "2106-02-07 06:28:16.999000", "2106-02-07 06:28:15.000000", "2106-02-07 06:28:16.000000"},
	} {
		nul := func(v string) any {
			if v == "" {
				return nil
			}
			return v
		}
		exec(`INSERT INTO note_collections (public_id, yearbook_id, token_hash, label, deadline_at, revoked_at, created_at)
			SELECT ?, id, UNHEX(SHA2(?, 256)), ?, ?, ?, ? FROM yearbook_tab`, c.id, c.id, "label "+c.id, nul(c.deadline), nul(c.revoked), c.created)
	}
	// Notes in every status; the second note has a sort order. Photo links at positions 0 and 1, and one on another note.
	for i, n := range []struct{ id, coll, status, created string }{
		{"N1", "C1", "pending", "2026-10-07 13:00:00.654321"},
		{"N2", "C1", "approved", "2026-10-07 13:00:01.000000"},
		{"N3", "C2", "hidden", "1969-12-31 23:59:59.123000"},
	} {
		exec(`INSERT INTO notes (public_id, collection_id, yearbook_id, answers, status, sort_order, created_at)
			SELECT ?, c.id, c.yearbook_id, '{"name":"Ánh"}', ?, ?, ? FROM note_collections c WHERE c.public_id = ?`, n.id, n.status, []any{nil, 1, nil}[i], n.created, n.coll)
	}
	for i, m := range []string{"M1", "M2", "M3"} {
		exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
			SELECT ?, id, 'contributor', ?, ?, 'image/png', 1, 1, 1, REPEAT('a', 64), 1, 1 FROM yearbook_tab`, m, "k/"+m, "k/"+m+"-t")
		note, pos := "N1", i
		if i == 2 {
			note, pos = "N2", 0
		}
		exec(`INSERT INTO note_photos (note_id, media_id, position) SELECT n.id, m.id, ? FROM notes n, media_tab m WHERE n.public_id = ? AND m.public_id = ?`, pos, note, m)
	}
	const (
		oldCollQ  = `SELECT public_id, yearbook_id, HEX(token_hash), label, CAST(deadline_at AS CHAR), CAST(created_at AS CHAR), CAST(revoked_at AS CHAR) FROM note_collections ORDER BY id`
		newCollQ  = `SELECT public_id, yearbook_id, HEX(token_hash), label, deadline_at, created_at, revoked_at, updated_at FROM note_collection_tab ORDER BY id`
		oldNoteQ  = `SELECT public_id, collection_id, yearbook_id, answers, status, sort_order, CAST(created_at AS CHAR) FROM notes ORDER BY id`
		newNoteQ  = `SELECT public_id, collection_id, yearbook_id, answers, status, sort_order, created_at, updated_at FROM note_tab ORDER BY id`
		oldPhotoQ = `SELECT note_id, media_id, position FROM note_photos ORDER BY note_id, position`
		newPhotoQ = `SELECT note_id, media_id, sort_order, created_at, updated_at FROM note_photo_tab ORDER BY note_id, sort_order`
	)
	oldColls, oldNotes, oldPhotos := dump(t, ctx, d, oldCollQ), dump(t, ctx, d, oldNoteQ), dump(t, ctx, d, oldPhotoQ)

	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	ms := func(y int, mo time.Month, day, h, mi, s, micro int) string {
		return fmt.Sprint(time.Date(y, mo, day, h, mi, s, micro*1000, time.UTC).UnixMilli())
	}
	const nul = "<nil>"
	c1, c2 := ms(2026, 10, 7, 12, 34, 56, 123456), ms(2026, 10, 7, 12, 34, 56, 1)
	c3, c4 := ms(1969, 12, 31, 23, 59, 59, 123000), ms(2106, 2, 7, 6, 28, 15, 0)
	d2, d3, d4 := ms(2026, 10, 9, 0, 0, 0, 999999), ms(2038, 1, 19, 3, 14, 8, 0), ms(2106, 2, 7, 6, 28, 16, 999000)
	r2, r4 := ms(2026, 10, 8, 1, 2, 3, 456789), ms(2106, 2, 7, 6, 28, 16, 0)
	if c3 != "-877" {
		t.Fatalf("test expectation: %s", c3) // truncated towards zero
	}
	hash := func(id string) string {
		return dump(t, ctx, d, fmt.Sprintf(`SELECT HEX(UNHEX(SHA2('%s', 256)))`, id))[0]
	}
	equalLines(t, "note_collection_tab", dump(t, ctx, d, newCollQ), []string{
		"C1|1|" + hash("C1") + "|label C1|" + nul + "|" + c1 + "|" + nul + "|" + c1, // updated_at = created_at
		"C2|1|" + hash("C2") + "|label C2|" + d2 + "|" + c2 + "|" + r2 + "|" + r2,   // ... or the revocation, if later
		"C3|1|" + hash("C3") + "|label C3|" + d3 + "|" + c3 + "|" + nul + "|" + c3,
		"C4|1|" + hash("C4") + "|label C4|" + d4 + "|" + c4 + "|" + r4 + "|" + r4,
	})
	n1, n2, n3 := ms(2026, 10, 7, 13, 0, 0, 654321), ms(2026, 10, 7, 13, 0, 1, 0), ms(1969, 12, 31, 23, 59, 59, 123000)
	equalLines(t, "note_tab", dump(t, ctx, d, newNoteQ), []string{
		`N1|1|1|{"name": "Ánh"}|pending|` + nul + "|" + n1 + "|" + n1,
		`N2|1|1|{"name": "Ánh"}|approved|1|` + n2 + "|" + n2,
		`N3|2|1|{"name": "Ánh"}|hidden|` + nul + "|" + n3 + "|" + n3,
	})
	equalLines(t, "note_photo_tab", dump(t, ctx, d, newPhotoQ), []string{
		"1|1|0|" + n1 + "|" + n1, "1|2|1|" + n1 + "|" + n1, "2|3|0|" + n2 + "|" + n2,
	})
	for _, old := range []string{"note_collections", "notes", "note_photos"} {
		if tableExists(t, ctx, d, old) {
			t.Fatalf("old table %s still exists", old)
		}
	}
	const tabs = `('note_collection_tab', 'note_tab', 'note_photo_tab')`
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND (TABLE_NAME IN `+tabs+` OR REFERENCED_TABLE_NAME IN `+tabs+`)`); n[0] != "0" {
		t.Fatalf("foreign keys left: %v", n)
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN `+tabs+`
		AND (DATA_TYPE IN ('datetime', 'timestamp', 'enum') OR UPPER(COLUMN_NAME) IN (SELECT WORD FROM information_schema.KEYWORDS WHERE RESERVED = 1))`); n[0] != "0" {
		t.Fatalf("datetime/enum/reserved columns left: %v", n)
	}
	// Every table has the id key and BIGINT created_at / updated_at; every referencing column is the first column of some index.
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN `+tabs+`
		AND COLUMN_NAME IN ('id', 'created_at', 'updated_at') AND DATA_TYPE = 'bigint' AND IS_NULLABLE = 'NO'`); n[0] != "9" {
		t.Fatalf("id, created_at, updated_at (bigint, not null) in the 3 tables: %v, want 9", n)
	}
	for _, c := range [][2]string{{"note_collection_tab", "yearbook_id"}, {"note_tab", "collection_id"}, {"note_tab", "yearbook_id"}, {"note_photo_tab", "note_id"}, {"note_photo_tab", "media_id"}} {
		q := fmt.Sprintf(`SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '%s' AND COLUMN_NAME = '%s' AND SEQ_IN_INDEX = 1`, c[0], c[1])
		if n := dump(t, ctx, d, q); n[0] == "0" {
			t.Fatalf("%s.%s is not indexed", c[0], c[1])
		}
	}
	// A photo is on one note only, and a note holds one photo per position.
	if _, err := d.ExecContext(ctx, `INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) VALUES (2, 1, 1, 1, 1)`); err == nil {
		t.Fatal("a photo was linked to two notes")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) VALUES (1, 99, 0, 1, 1)`); err == nil {
		t.Fatal("two photos at one position of a note")
	}

	// Down restores the old rows (microseconds are lost to the millisecond) and the keys; Up again gives the same rows.
	upColls, upNotes, upPhotos := dump(t, ctx, d, newCollQ), dump(t, ctx, d, newNoteQ), dump(t, ctx, d, newPhotoQ)
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "note_collections") })
	subMs := regexp.MustCompile(`\.(\d{3})\d{3}`)
	msOnly := func(rows []string) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = subMs.ReplaceAllString(r, ".${1}000")
		}
		return out
	}
	equalLines(t, "note_photos after down", dump(t, ctx, d, oldPhotoQ), oldPhotos)
	equalLines(t, "note_collections after down", dump(t, ctx, d, oldCollQ), msOnly(oldColls))
	equalLines(t, "notes after down", dump(t, ctx, d, oldNoteQ), msOnly(oldNotes))
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME IN ('note_collections', 'notes', 'note_photos')`); n[0] != "5" {
		t.Fatalf("foreign keys after down: %v, want 5", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "note_collection_tab after up again", dump(t, ctx, d, newCollQ), upColls)
	equalLines(t, "note_tab after up again", dump(t, ctx, d, newNoteQ), upNotes)
	equalLines(t, "note_photo_tab after up again", dump(t, ctx, d, newPhotoQ), upPhotos)
}
