//go:build integration

package db_test

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// dump returns every row of a query as one "|"-joined line per row (NULL as <nil>), for comparing states.
func dump(t *testing.T, ctx context.Context, d *sql.DB, q string) []string {
	t.Helper()
	rows, err := d.QueryContext(ctx, q)
	if err != nil {
		t.Fatalf("%s: %v", q, err)
	}
	defer func() { _ = rows.Close() }()
	cols, _ := rows.Columns()
	var out []string
	for rows.Next() {
		vals := make([]sql.NullString, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			t.Fatal(err)
		}
		parts := make([]string, len(cols))
		for i, v := range vals {
			parts[i] = "<nil>"
			if v.Valid {
				parts[i] = v.String
			}
		}
		out = append(out, strings.Join(parts, "|"))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func equalLines(t *testing.T, what string, got, want []string) {
	t.Helper()
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("%s:\n got:\n%s\nwant:\n%s", what, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// T-064 AC2: the conversion to yearbook_tab / profile_tab keeps every value (DATETIME(6) -> Unix ms, ENUM -> VARCHAR),
// drops the foreign keys, copies the book times to the profile, and Down restores the old rows.
func TestMigrateYearbookTabConventions(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "yearbooks") }) // the schema before T-064
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	exec(`INSERT INTO users (public_id, email, display_name, created_at, updated_at) VALUES ('U1', 'a@example.com', 'A', NOW(6), NOW(6))`)
	// B1: microseconds, cover and photo, every optional field set. B2: before 1970-01-02, nothing optional.
	// B3: past 2038-01-19 and 2106-02-07 (the 32-bit limits).
	exec(`INSERT INTO yearbooks (public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, created_at, updated_at)
		SELECT 'B1', id, 'One', 'School', 'K65', 2026, 'Motto', 'vi', 'Letter', 'classic', '2026-10-07 12:34:56.123456', '2026-10-08 01:02:03.500000' FROM users`)
	exec(`INSERT INTO yearbooks (public_id, owner_id, title, language, created_at, updated_at)
		SELECT 'B2', id, 'Two', 'en', '1969-12-31 23:59:59.123000', '1970-01-01 00:00:00.001000' FROM users`)
	exec(`INSERT INTO yearbooks (public_id, owner_id, title, language, page_size, created_at, updated_at)
		SELECT 'B3', id, 'Three', 'en', 'A4', '2038-01-19 03:14:08.000000', '2106-02-07 06:28:16.999000' FROM users`)
	exec(`INSERT INTO media (public_id, yearbook_id, uploader_kind, object_key, thumb_key, content_type, bytes, width, height, sha256, created_at)
		SELECT 'M1', id, 'owner', 'k', 't', 'image/jpeg', 1, 1, 1, '', NOW(6) FROM yearbooks WHERE public_id = 'B1'`)
	exec(`UPDATE yearbooks SET cover_media_id = (SELECT id FROM media WHERE public_id = 'M1') WHERE public_id = 'B1'`)
	for _, b := range []string{"B1", "B2", "B3"} {
		exec(`INSERT INTO profiles (public_id, yearbook_id, is_owner, full_name, nickname, birthday, quote, hobbies, future_plans)
			SELECT ?, id, TRUE, 'Name', 'Nick', ?, 'q', 'h', 'f' FROM yearbooks WHERE public_id = ?`, "P"+b, map[string]any{"B1": "2004-02-29", "B2": nil, "B3": "1999-12-31"}[b], b)
	}
	exec(`UPDATE profiles SET photo_media_id = (SELECT id FROM media WHERE public_id = 'M1') WHERE public_id = 'PB1'`)

	const bookQ = `SELECT public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, cover_media_id,
		CAST(created_at AS CHAR), CAST(updated_at AS CHAR) FROM yearbooks ORDER BY id`
	const profQ = `SELECT public_id, yearbook_id, is_owner, owner_flag, full_name, nickname, CAST(birthday AS CHAR), quote, hobbies, future_plans, photo_media_id FROM profiles ORDER BY id`
	oldBooks, oldProfiles := dump(t, ctx, d, bookQ), dump(t, ctx, d, profQ)

	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	ms := func(y int, mo time.Month, day, h, mi, s, micro int) string {
		return fmt.Sprint(time.Date(y, mo, day, h, mi, s, micro*1000, time.UTC).UnixMilli())
	}
	// The microsecond digits are dropped (B1), a value before 1970 truncates towards zero (B2: -877 ms).
	b1c, b1u := ms(2026, 10, 7, 12, 34, 56, 123456), ms(2026, 10, 8, 1, 2, 3, 500000)
	b2c, b2u := ms(1969, 12, 31, 23, 59, 59, 123000), ms(1970, 1, 1, 0, 0, 0, 1000)
	b3c, b3u := ms(2038, 1, 19, 3, 14, 8, 0), ms(2106, 2, 7, 6, 28, 16, 999000)
	if b2c != "-877" || b2u != "1" {
		t.Fatalf("test expectation: %s %s", b2c, b2u)
	}
	upBooks := dump(t, ctx, d, `SELECT public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, cover_media_id,
		created_at, updated_at FROM yearbook_tab ORDER BY id`)
	equalLines(t, "yearbook_tab", upBooks, []string{
		"B1|1|One|School|K65|2026|Motto|vi|Letter|classic|1|" + b1c + "|" + b1u,
		"B2|1|Two|||<nil>||en|A5|<nil>|<nil>|" + b2c + "|" + b2u,
		"B3|1|Three|||<nil>||en|A4|<nil>|<nil>|" + b3c + "|" + b3u,
	})
	upProfiles := dump(t, ctx, d, `SELECT public_id, yearbook_id, is_owner, owner_flag, full_name, nickname, CAST(birthday AS CHAR), quote, hobbies, future_plans, photo_media_id,
		created_at, updated_at FROM profile_tab ORDER BY id`)
	equalLines(t, "profile_tab", upProfiles, []string{
		"PB1|1|1|1|Name|Nick|2004-02-29|q|h|f|1|" + b1c + "|" + b1u,
		"PB2|2|1|1|Name|Nick|<nil>|q|h|f|<nil>|" + b2c + "|" + b2u,
		"PB3|3|1|1|Name|Nick|1999-12-31|q|h|f|<nil>|" + b3c + "|" + b3u,
	})

	if tableExists(t, ctx, d, "yearbooks") || tableExists(t, ctx, d, "profiles") {
		t.Fatal("old tables still exist")
	}
	// No foreign key leaves either table (media and note tables may still point at yearbook_tab until their own tasks).
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME IN ('yearbook_tab', 'profile_tab')`); n[0] != "0" {
		t.Fatalf("foreign keys left on the converted tables: %v", n)
	}
	// Every referencing column is the first column of some index.
	for _, c := range [][2]string{{"yearbook_tab", "owner_id"}, {"yearbook_tab", "cover_media_id"}, {"profile_tab", "yearbook_id"}, {"profile_tab", "photo_media_id"}} {
		q := fmt.Sprintf(`SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = '%s' AND COLUMN_NAME = '%s' AND SEQ_IN_INDEX = 1`, c[0], c[1])
		if n := dump(t, ctx, d, q); n[0] == "0" {
			t.Fatalf("%s.%s is not indexed", c[0], c[1])
		}
	}
	// No DATETIME/ENUM column, no reserved column name.
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('yearbook_tab', 'profile_tab')
		AND (DATA_TYPE IN ('datetime', 'timestamp', 'enum') OR UPPER(COLUMN_NAME) IN (SELECT WORD FROM information_schema.KEYWORDS WHERE RESERVED = 1))`); n[0] != "0" {
		t.Fatalf("datetime/enum/reserved columns left: %v", n)
	}
	// One owner profile per book is still enforced.
	if _, err := d.ExecContext(ctx, `INSERT INTO profile_tab (public_id, yearbook_id, is_owner, full_name, created_at, updated_at) VALUES ('PX', 1, 1, 'Second', 1, 1)`); err == nil {
		t.Fatal("a second owner profile was accepted")
	}

	// Down restores the old rows (B1 only loses its microseconds), Up again gives the same converted rows.
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "yearbooks") })
	wantBooks := make([]string, len(oldBooks))
	for i, r := range oldBooks {
		wantBooks[i] = strings.Replace(r, "12:34:56.123456", "12:34:56.123000", 1)
	}
	equalLines(t, "yearbooks after down", dump(t, ctx, d, bookQ), wantBooks)
	equalLines(t, "profiles after down", dump(t, ctx, d, profQ), oldProfiles)
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME IN ('yearbooks', 'profiles')`); n[0] != "4" {
		t.Fatalf("foreign keys after down: %v, want 4", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "yearbook_tab after up again", dump(t, ctx, d, `SELECT public_id, owner_id, title, school_name, class_name, graduation_year, motto, language, page_size, template_id, cover_media_id,
		created_at, updated_at FROM yearbook_tab ORDER BY id`), upBooks)
}
