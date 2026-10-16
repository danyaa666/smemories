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

// T-067 AC1: users / user_identities -> user_tab / user_identity_tab keep every value (DATETIME(6) -> Unix ms, ENUM -> VARCHAR,
// NULL stays NULL, identity updated_at = created_at), keep the binary email collation, drop the last foreign key, and Down
// restores the old rows and the key; Up again gives the same rows.
func TestMigrateUserTabConventions(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "users") }) // the schema before T-067
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	// Verified with a password; unverified; "anh" and "ánh" are two accounts only under the binary collation; social-only
	// (NULL password hash) at the 32-bit limit; before 1970 and past 2106.
	type user struct{ id, email, hash, name, locale, verified, created, updated string }
	users := []user{
		{"U1", "a@example.com", "$argon2id$x", "A", "vi", "2026-10-08 01:02:03.500000", "2026-10-07 12:34:56.123456", "2026-10-08 01:02:03.500000"},
		{"U2", "anh@example.com", "$argon2id$x", "Anh", "en", "", "1969-12-31 23:59:59.123000", "1969-12-31 23:59:59.123000"},
		{"U3", "ánh@example.com", "$argon2id$x", "Ánh", "en", "", "2106-02-07 06:28:16.999000", "2106-02-07 06:28:16.999000"},
		{"U4", "g@example.com", "", "G", "vi", "2038-01-19 03:14:08.000000", "2038-01-19 03:14:08.000000", "2038-01-19 03:14:08.000000"},
	}
	nul := func(v string) any {
		if v == "" {
			return nil
		}
		return v
	}
	for _, u := range users {
		exec(`INSERT INTO users (public_id, email, password_hash, display_name, locale, email_verified_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			u.id, u.email, nul(u.hash), u.name, u.locale, nul(u.verified), u.created, u.updated)
	}
	exec(`INSERT INTO user_identities (user_id, provider, subject, email, created_at) SELECT id, 'google', 'Sub-Ábc', email, '2038-01-19 03:14:08.000000' FROM users WHERE public_id = 'U4'`)
	exec(`INSERT INTO user_identities (user_id, provider, subject, email, created_at) SELECT id, 'google', 'sub-abc', email, '2026-10-07 12:34:56.654321' FROM users WHERE public_id = 'U1'`)
	const oldUsers = `SELECT public_id, email, password_hash, display_name, locale, CAST(email_verified_at AS CHAR), CAST(created_at AS CHAR), CAST(updated_at AS CHAR) FROM users ORDER BY id`
	const newUsers = `SELECT public_id, email, password_hash, display_name, locale, email_verified_at, created_at, updated_at FROM user_tab ORDER BY id`
	const oldIdents = `SELECT user_id, provider, subject, email, CAST(created_at AS CHAR) FROM user_identities ORDER BY id`
	const newIdents = `SELECT user_id, provider, subject, email, created_at, updated_at FROM user_identity_tab ORDER BY id`
	oldU, oldI := dump(t, ctx, d, oldUsers), dump(t, ctx, d, oldIdents)

	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	ms := func(y int, mo time.Month, day, h, mi, s, micro int) string {
		return fmt.Sprint(time.Date(y, mo, day, h, mi, s, micro*1000, time.UTC).UnixMilli())
	}
	u1c, u1v := ms(2026, 10, 7, 12, 34, 56, 123456), ms(2026, 10, 8, 1, 2, 3, 500000)
	u2 := ms(1969, 12, 31, 23, 59, 59, 123000) // -877: truncated towards zero
	u3, u4 := ms(2106, 2, 7, 6, 28, 16, 999000), ms(2038, 1, 19, 3, 14, 8, 0)
	if u2 != "-877" {
		t.Fatalf("test expectation: %s", u2)
	}
	nl := "<nil>"
	upU := []string{
		"U1|a@example.com|$argon2id$x|A|vi|" + u1v + "|" + u1c + "|" + u1v,
		"U2|anh@example.com|$argon2id$x|Anh|en|" + nl + "|" + u2 + "|" + u2,
		"U3|ánh@example.com|$argon2id$x|Ánh|en|" + nl + "|" + u3 + "|" + u3,
		"U4|g@example.com|" + nl + "|G|vi|" + u4 + "|" + u4 + "|" + u4,
	}
	i1c := ms(2026, 10, 7, 12, 34, 56, 654321)
	upI := []string{"4|google|Sub-Ábc|g@example.com|" + u4 + "|" + u4, "1|google|sub-abc|a@example.com|" + i1c + "|" + i1c}
	equalLines(t, "user_tab", dump(t, ctx, d, newUsers), upU)
	equalLines(t, "user_identity_tab", dump(t, ctx, d, newIdents), upI)
	for _, old := range []string{"users", "user_identities"} {
		if tableExists(t, ctx, d, old) {
			t.Fatalf("old table %s still exists", old)
		}
	}
	// The email column stays binary: case and accent make different accounts, and the exact address is unique.
	exec(`INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('U5', 'A@example.com', 'A', 'user-u5', 1, 1)`)
	if _, err := d.ExecContext(ctx, `INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('U6', 'ánh@example.com', 'A', 'user-u6', 1, 1)`); err == nil {
		t.Fatal("a duplicate email was accepted")
	}
	exec(`DELETE FROM user_tab WHERE public_id = 'U5'`)
	if _, err := d.ExecContext(ctx, `INSERT INTO user_identity_tab (user_id, provider, subject, email, created_at, updated_at) VALUES (99, 'google', 'Sub-Ábc', 'x', 1, 1)`); err == nil {
		t.Fatal("a duplicate (provider, subject) was accepted")
	}
	// No foreign key: an identity of a missing user can be written (the auth service owns that rule), and every table is clean.
	exec(`INSERT INTO user_identity_tab (user_id, provider, subject, email, created_at, updated_at) VALUES (99, 'google', 'orphan', 'x', 1, 1)`)
	exec(`DELETE FROM user_identity_tab WHERE subject = 'orphan'`)
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE()`); n[0] != "0" {
		t.Fatalf("foreign keys left in the schema: %v", n)
	}
	const tabs = `('user_tab', 'user_identity_tab')`
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN `+tabs+`
		AND (DATA_TYPE IN ('datetime', 'timestamp', 'enum') OR UPPER(COLUMN_NAME) IN (SELECT WORD FROM information_schema.KEYWORDS WHERE RESERVED = 1))`); n[0] != "0" {
		t.Fatalf("datetime/enum/reserved columns left: %v", n)
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN `+tabs+`
		AND COLUMN_NAME IN ('id', 'created_at', 'updated_at') AND DATA_TYPE = 'bigint' AND IS_NULLABLE = 'NO'`); n[0] != "6" {
		t.Fatalf("id, created_at, updated_at (bigint, not null) in the 2 tables: %v, want 6", n)
	}
	if n := dump(t, ctx, d, `SELECT COLLATION_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user_tab' AND COLUMN_NAME = 'email'`); n[0] != "utf8mb4_bin" {
		t.Fatalf("email collation %v", n)
	}
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'user_identity_tab' AND COLUMN_NAME = 'user_id' AND SEQ_IN_INDEX = 1`); n[0] == "0" {
		t.Fatal("user_identity_tab.user_id is not indexed")
	}

	// Down restores the old rows (microseconds are lost to the millisecond), the enums and the key; Up again gives the same rows.
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "users") })
	subMs := regexp.MustCompile(`\.(\d{3})\d{3}`)
	msOnly := func(rows []string) []string {
		out := make([]string, len(rows))
		for i, r := range rows {
			out[i] = subMs.ReplaceAllString(r, ".${1}000")
		}
		return out
	}
	equalLines(t, "users after down", dump(t, ctx, d, oldUsers), msOnly(oldU))
	equalLines(t, "user_identities after down", dump(t, ctx, d, oldIdents), msOnly(oldI))
	if n := dump(t, ctx, d, `SELECT COUNT(*) FROM information_schema.REFERENTIAL_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND TABLE_NAME = 'user_identities'`); n[0] != "1" {
		t.Fatalf("foreign keys after down: %v, want 1", n)
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up again: %v", err)
	}
	equalLines(t, "user_tab after up again", dump(t, ctx, d, newUsers), upU)
	equalLines(t, "user_identity_tab after up again", dump(t, ctx, d, newIdents), upI)
}
