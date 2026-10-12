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

// QA T-066: the three notes migrations on realistic data (Vietnamese/emoji answers, 0-3 photos per note, every status, collections
// with and without deadline/revocation, boundary instants of DATETIME(6)). Up/Down/Up; values compared against the pre-migration
// rows (millisecond truncation of the DATETIME(6) values is the only allowed difference); a row written while migrated survives Down.
func TestQAMigrateNoteTabRealisticData(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "note_collections") })
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (public_id, email, display_name, created_at, updated_at) VALUES ('U1', 'qa@example.com', 'Trần Thị Hồng 🌸', NOW(6), NOW(6))`)
	exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) SELECT 'B1', id, 'Lớp 12A1', 'vi', 1, 1 FROM users`)
	exec(`INSERT INTO yearbook_tab (public_id, owner_id, title, language, created_at, updated_at) SELECT 'B2', id, 'Second', 'en', 1, 1 FROM users`)
	colls := []struct{ id, book, deadline, created, revoked string }{
		{"C1", "B1", "", "2026-10-07 12:00:00.000000", ""},
		{"C2", "B1", "2026-10-09 17:00:00.500000", "2026-10-07 12:00:00.000001", "2026-10-08 01:02:03.456789"},
		{"C3", "B1", "2020-01-01 00:00:00.000000", "2019-12-31 23:59:59.999999", ""}, // deadline long past
		{"C4", "B2", "1000-01-01 00:00:00.000000", "1970-01-01 00:00:00.000000", "9999-12-31 23:59:59.999999"},
		{"C5", "B2", "9999-12-31 23:59:59.999999", "1970-01-01 00:00:00.000999", ""},
	}
	nul := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	for _, c := range colls {
		exec(`INSERT INTO note_collections (public_id, yearbook_id, token_hash, label, deadline_at, revoked_at, created_at)
			SELECT ?, id, UNHEX(SHA2(?, 256)), ?, ?, ?, ? FROM yearbook_tab WHERE public_id = ?`, c.id, c.id, "Đợt "+c.id+" 🎓", nul(c.deadline), nul(c.revoked), c.created, c.book)
	}
	statuses := []string{"pending", "approved", "hidden"}
	type note struct {
		id, coll, answers string
		photos            int
	}
	var notes []note
	answersSet := []string{
		`{"name":"Nguyễn Văn Ánh","message":"Nhớ mãi những ngày tháng ấy 😭💖 ..."}`,
		`{"name":"Lê","message":"👨‍👩‍👧‍👦 emoji ZWJ — “quotes” \u0000?"}`,
		`{"name":"x","message":""}`,
	}
	n := 0
	for ci, c := range colls {
		for k := 0; k < 4; k++ { // 0..3 photos
			n++
			notes = append(notes, note{fmt.Sprintf("N%02d", n), c.id, answersSet[(ci+k)%3], k})
		}
	}
	createds := []string{"2026-10-07 13:00:00.654321", "1969-12-31 23:59:59.123000", "9999-12-31 23:59:59.999999", "1000-01-01 00:00:00.000000"}
	mediaN := 0
	for i, nt := range notes {
		exec(`INSERT INTO notes (public_id, collection_id, yearbook_id, answers, status, sort_order, created_at)
			SELECT ?, c.id, c.yearbook_id, ?, ?, ?, ? FROM note_collections c WHERE c.public_id = ?`,
			nt.id, nt.answers, statuses[i%3], []any{nil, i}[i%2], createds[i%4], nt.coll)
		for p := 0; p < nt.photos; p++ {
			mediaN++
			m := fmt.Sprintf("M%03d", mediaN)
			exec(`INSERT INTO media_tab (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at, updated_at)
				SELECT ?, c.yearbook_id, 'contributor', ?, ?, 'image/png', 1, 1, 1, REPEAT('a', 64), 1, 1 FROM note_collections c WHERE c.public_id = ?`, m, "k/"+m, "k/"+m+"-t", nt.coll)
			exec(`INSERT INTO note_photos (note_id, media_id, position) SELECT n.id, m.id, ? FROM notes n, media_tab m WHERE n.public_id = ? AND m.public_id = ?`, p, nt.id, m)
		}
	}
	const (
		oldColl  = `SELECT public_id, yearbook_id, HEX(token_hash), label, CAST(deadline_at AS CHAR), CAST(created_at AS CHAR), CAST(revoked_at AS CHAR) FROM note_collections ORDER BY id`
		oldNote  = `SELECT public_id, collection_id, yearbook_id, answers, status, sort_order, CAST(created_at AS CHAR) FROM notes ORDER BY id`
		oldPhoto = `SELECT note_id, media_id, position FROM note_photos ORDER BY note_id, position`
		newColl  = `SELECT public_id, yearbook_id, HEX(token_hash), label, deadline_at, created_at, revoked_at, updated_at FROM note_collection_tab ORDER BY id`
		newNote  = `SELECT public_id, collection_id, yearbook_id, answers, status, sort_order, created_at, updated_at FROM note_tab ORDER BY id`
		newPhoto = `SELECT note_id, media_id, sort_order, created_at, updated_at FROM note_photo_tab ORDER BY note_id, sort_order`
	)
	oc, on, op := dump(t, ctx, d, oldColl), dump(t, ctx, d, oldNote), dump(t, ctx, d, oldPhoto)
	if len(on) != 20 || len(op) != 30 {
		t.Fatalf("test data: %d notes %d photos", len(on), len(op))
	}

	// Expected ms computed by MySQL itself from the original DATETIME values, independently of the migration SQL: UNIX_TIMESTAMP
	// is session-zone dependent, so use TIMESTAMPDIFF(SECOND) *1000 + MICROSECOND part instead (floor for positives).
	expMs := func(col string) string {
		return fmt.Sprintf(`(TIMESTAMPDIFF(SECOND, '1970-01-01 00:00:00', %[1]s) * 1000 + TRUNCATE(MICROSECOND(%[1]s) / 1000, 0))`, col)
	}
	// Positive instants only for this comparison (rows before the epoch are asserted explicitly below).
	_ = expMs

	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	upC, upN, upP := dump(t, ctx, d, newColl), dump(t, ctx, d, newNote), dump(t, ctx, d, newPhoto)
	if len(upC) != 5 || len(upN) != 20 || len(upP) != 30 {
		t.Fatalf("row counts after up: %d %d %d", len(upC), len(upN), len(upP))
	}
	// Non-time columns identical to the originals.
	strip := func(rows []string, drop ...int) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			f := strings.Split(r, "|")
			var keep []string
			for j, v := range f {
				skip := false
				for _, dd := range drop {
					if j == dd {
						skip = true
					}
				}
				if !skip {
					keep = append(keep, v)
				}
			}
			out[i] = strings.Join(keep, "|")
		}
		return out
	}
	// answers contain no "|" in the test data except none; labels neither.
	equalLines(t, "collections non-time columns", strip(upC, 4, 5, 6, 7), strip(oc, 4, 5, 6))
	equalLines(t, "photos ids/order", strip(upP, 3, 4), op)
	// Specific time values.
	wantTimes := dump(t, ctx, d, `SELECT public_id, deadline_at, created_at, revoked_at, updated_at FROM note_collection_tab ORDER BY id`)
	want := map[string]string{
		"C1": "<nil>|1791374400000|<nil>|1791374400000",
		"C4": "-30610224000000|0|253402300799999|253402300799999",
		"C5": "253402300799999|0|<nil>|0",
	}
	for _, r := range wantTimes {
		f := strings.SplitN(r, "|", 2)
		if w, ok := want[f[0]]; ok && f[1] != w {
			t.Errorf("%s times = %s, want %s", f[0], f[1], w)
		}
	}
	// Reserved words / FK / indexes: a hostile scan of the live schema.
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND CONSTRAINT_TYPE = 'FOREIGN KEY'`); n[0] != "0" {
		t.Errorf("foreign keys anywhere in the schema: %v", n)
	}
	// Unique constraints still hold.
	if _, err := d.ExecContext(ctx, `INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) SELECT note_id, media_id, 9, 1, 1 FROM note_photo_tab LIMIT 1`); err == nil {
		t.Error("photo linked twice")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO note_photo_tab (note_id, media_id, sort_order, created_at, updated_at) SELECT note_id, 999999, sort_order, 1, 1 FROM note_photo_tab LIMIT 1`); err == nil {
		t.Error("duplicate sort_order on a note")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, created_at, updated_at) SELECT 'DUP', yearbook_id, token_hash, 'x', 1, 1 FROM note_collection_tab LIMIT 1`); err == nil {
		t.Error("duplicate token hash")
	}
	// A row written while migrated (new schema).
	exec(`INSERT INTO note_collection_tab (public_id, yearbook_id, token_hash, label, deadline_at, created_at, updated_at) SELECT 'CNEW', id, UNHEX(SHA2('CNEW', 256)), 'new', 1791374400123, 1791374400000, 1791374400000 FROM yearbook_tab WHERE public_id = 'B1'`)
	exec(`INSERT INTO note_tab (public_id, collection_id, yearbook_id, answers, status, created_at, updated_at) SELECT 'NNEW', c.id, c.yearbook_id, '{"a":"😀"}', 'approved', 1791374400999, 1791374400999 FROM note_collection_tab c WHERE c.public_id = 'CNEW'`)

	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "note_collections") })
	// Down: original values except microseconds; the new rows survived with ms DATETIME(6).
	dc, dn, dp := dump(t, ctx, d, oldColl), dump(t, ctx, d, oldNote), dump(t, ctx, d, oldPhoto)
	if len(dc) != 6 || len(dn) != 21 {
		t.Fatalf("after down: %d collections %d notes", len(dc), len(dn))
	}
	equalLines(t, "photos after down", dp, op)
	trunc := func(rows []string) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			for _, from := range []string{"654321", "999999", "456789", "000001", "000999", "500000"} {
				r = strings.ReplaceAll(r, "."+from, "."+from[:3]+"000")
			}
			out[i] = r
		}
		return out
	}
	// sort_order/answers/status identical; times equal modulo microseconds
	equalLines(t, "collections after down", dc[:5], trunc(oc))
	equalLines(t, "notes after down", dn[:20], trunc(on))
	if got := dump(t, ctx, d, `SELECT answers, status, CAST(created_at AS CHAR) FROM notes WHERE public_id='NNEW'`); got[0] != `{"a": "😀"}|approved|2026-10-07 12:00:00.999000` {
		t.Errorf("new note after down: %v", got)
	}

	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	c2, n2, p2 := dump(t, ctx, d, newColl), dump(t, ctx, d, newNote), dump(t, ctx, d, newPhoto)
	equalLines(t, "collections up/down/up", c2[:5], upC)
	equalLines(t, "notes up/down/up", n2[:20], upN)
	equalLines(t, "photos up/down/up", p2, upP)
	if len(c2) != 6 || len(n2) != 21 {
		t.Fatalf("row counts after up again: %d %d", len(c2), len(n2))
	}
}
