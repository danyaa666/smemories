//go:build integration

package db_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-065 AC1: media -> media_tab keeps every value (DATETIME(6) -> Unix ms, ENUM -> VARCHAR, updated_at = created_at), drops the
// foreign key to the yearbook, renames the index, and Down restores the old rows and the foreign key.
func TestMigrateMediaTabConventions(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "media") }) // the schema before T-065
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (public_id, email, display_name, created_at, updated_at) VALUES ('U1', 'a@example.com', 'A', NOW(6), NOW(6))`)
	exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) SELECT 'B1', id, 'One', 'en', 1, 1 FROM users`)
	// Both uploader kinds, print object set (P) and not set (NULL, the T-057 backfill gap), microseconds, before 1970, and past the
	// 32-bit limits 2038-01-19 and 2106-02-07.
	for _, m := range []struct{ id, kind, print, created string }{
		{"M1", "owner", "k/m1-print.jpg", "2026-10-07 12:34:56.123456"},
		{"M2", "contributor", "", "1969-12-31 23:59:59.123000"},
		{"M3", "owner", "", "2038-01-19 03:14:08.000000"},
		{"M4", "contributor", "k/m4-print.jpg", "2106-02-07 06:28:16.999000"},
	} {
		var print any
		if m.print != "" {
			print = m.print
		}
		exec(`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at)
			SELECT ?, id, ?, ?, ?, ?, 'image/jpeg', 1234, 640, 480, REPEAT('a', 64), ? FROM yearbook_tab`, m.id, m.kind, "k/"+m.id+".jpg", "k/"+m.id+"-thumb.jpg", print, m.created)
	}
	const oldQ = `SELECT public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, CAST(created_at AS CHAR) FROM media ORDER BY id`
	const newQ = `SELECT public_id, yearbook_id, uploader_kind, object_key, thumb_key, print_key, content_type, bytes, width, height, sha256, created_at, updated_at FROM media_tab ORDER BY id`
	oldRows := dump(t, ctx, d, oldQ)

	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	ms := func(y int, mo time.Month, day, h, mi, s, micro int) string {
		return fmt.Sprint(time.Date(y, mo, day, h, mi, s, micro*1000, time.UTC).UnixMilli())
	}
	c1, c2 := ms(2026, 10, 7, 12, 34, 56, 123456), ms(1969, 12, 31, 23, 59, 59, 123000) // c2 is -877: truncated towards zero
	c3, c4 := ms(2038, 1, 19, 3, 14, 8, 0), ms(2106, 2, 7, 6, 28, 16, 999000)
	if c2 != "-877" {
		t.Fatalf("test expectation: %s", c2)
	}
	const tail = "|image/jpeg|1234|640|480|aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa|"
	upRows := dump(t, ctx, d, newQ)
	equalLines(t, "media_tab", upRows, []string{
		"M1|1|owner|k/M1.jpg|k/M1-thumb.jpg|k/m1-print.jpg" + tail + c1 + "|" + c1,
		"M2|1|contributor|k/M2.jpg|k/M2-thumb.jpg|<nil>" + tail + c2 + "|" + c2,
		"M3|1|owner|k/M3.jpg|k/M3-thumb.jpg|<nil>" + tail + c3 + "|" + c3,
		"M4|1|contributor|k/M4.jpg|k/M4-thumb.jpg|k/m4-print.jpg" + tail + c4 + "|" + c4,
	})
	if tableExists(t, ctx, d, "media") {
		t.Fatal("old table still exists")
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'media_tab'`); n[0] != "0" {
		t.Fatalf("foreign keys left on media_tab: %v", n)
	}
	// The photo-of-a-note table pointed at the renamed table until T-066 dropped its foreign keys.
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND REFERENCED_TABLE_NAME = 'media_tab'`); n[0] != "0" {
		t.Fatalf("foreign keys into media_tab: %v, want none", n)
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'media_tab' AND INDEX_NAME = 'idx_yearbook_id' AND COLUMN_NAME = 'yearbook_id' AND SEQ_IN_INDEX = 1`); n[0] != "1" {
		t.Fatalf("idx_yearbook_id missing: %v", n)
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'media_tab'
		AND (DATA_TYPE IN ('datetime', 'timestamp', 'enum') OR UPPER(COLUMN_NAME) IN (SELECT WORD FROM information_schema.KEYWORDS WHERE RESERVED = 1))`); n[0] != "0" {
		t.Fatalf("datetime/enum/reserved columns left: %v", n)
	}

	// Down restores the old rows (M1 only loses its microseconds) and the foreign key; Up again gives the same rows.
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "media") })
	wantOld := make([]string, len(oldRows))
	for i, r := range oldRows {
		wantOld[i] = strings.Replace(r, "12:34:56.123456", "12:34:56.123000", 1)
	}
	equalLines(t, "media after down", dump(t, ctx, d, oldQ), wantOld)
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'media' AND CONSTRAINT_NAME = 'fk_media_yearbook'`); n[0] != "1" {
		t.Fatalf("foreign key after down: %v", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "media_tab after up again", dump(t, ctx, d, newQ), upRows) // the microseconds were gone in the first Up already
}
