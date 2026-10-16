//go:build integration

package db_test

import (
	"context"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
)

// T-067 QA: a bulk round trip (random users, microsecond DATETIME(6) values, several identities per user, accent/case emails)
// compares every value through Up, Down, Up, and compares the column metadata of the untouched columns before and after.
func TestQAUserTabBulkRoundTrip(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "users") })
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	rng := rand.New(rand.NewPCG(7, 11))
	base := []string{"e", "é", "E", "É", "ë", "ê", "ế", "ệ", "i", "í", "İ", "ı", "ß", "ss", "ǆ", "ǅ", "Ǆ", "ａ", "a"}
	n := 0
	for _, b := range base {
		for _, loc := range []string{"en", "vi"} {
			n++
			micro := rng.IntN(1000000)
			sec := rng.Int64N(4_000_000_000) - 100_000
			ts := time.Unix(sec, int64(micro)*1000).UTC().Format("2006-01-02 15:04:05.000000")
			var ver any
			if n%3 != 0 {
				ver = ts
			}
			var hash any = "$argon2id$h" + fmt.Sprint(n)
			if n%4 == 0 {
				hash = nil
			}
			exec(`INSERT INTO users (public_id, email, password_hash, display_name, locale, email_verified_at, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
				fmt.Sprintf("U%03d", n), b+loc+"@exämple.com", hash, "N "+b, loc, ver, ts, ts)
			if n%2 == 0 {
				exec(`INSERT INTO user_identities (user_id, provider, subject, email, created_at) SELECT id, 'google', ?, email, ? FROM users WHERE public_id = ?`, "sub-"+b+loc, ts, fmt.Sprintf("U%03d", n))
			}
		}
	}
	// A user with several identities (same provider, different subject: allowed by the unique key).
	exec(`INSERT INTO user_identities (user_id, provider, subject, email, created_at) SELECT id, 'google', 'second-sub', email, '2026-01-01 00:00:00.999999' FROM users WHERE public_id = 'U002'`)

	const oldU = `SELECT public_id, email, password_hash, display_name, locale, email_verified_at IS NULL, TIMESTAMPDIFF(MICROSECOND,'1970-01-01',IFNULL(email_verified_at,'1970-01-01')) DIV 1000, TIMESTAMPDIFF(MICROSECOND,'1970-01-01',created_at) DIV 1000, TIMESTAMPDIFF(MICROSECOND,'1970-01-01',updated_at) DIV 1000 FROM users ORDER BY id`
	const newU = `SELECT public_id, email, password_hash, display_name, locale, email_verified_at IS NULL, IFNULL(email_verified_at,0), created_at, updated_at FROM user_tab ORDER BY id`
	const oldI = `SELECT user_id, provider, subject, email, TIMESTAMPDIFF(MICROSECOND,'1970-01-01',created_at) DIV 1000 FROM user_identities ORDER BY id`
	const newI = `SELECT user_id, provider, subject, email, created_at FROM user_identity_tab ORDER BY id`
	wantU, wantI := dump(t, ctx, d, oldU), dump(t, ctx, d, oldI)
	colsOld := dump(t, ctx, d, `SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, IFNULL(COLUMN_DEFAULT,'NULL'), IFNULL(COLLATION_NAME,''), IFNULL(CHARACTER_SET_NAME,''), COLUMN_COMMENT, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('users','user_identities') AND COLUMN_NAME NOT IN ('email_verified_at','created_at','updated_at','locale','provider') ORDER BY TABLE_NAME, COLUMN_NAME`)
	statOld := dump(t, ctx, d, `SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('users','user_identities') ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`)
	t.Logf("%d users", len(wantU))

	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatal(err)
	}
	// Values: every ms column equals the old DATETIME(6) in ms (truncated); the replacement dump of the new schema must match.
	equalLines(t, "users up", dump(t, ctx, d, newU), wantU)
	equalLines(t, "identities up", dump(t, ctx, d, newI), wantI)
	colsNew := dump(t, ctx, d, `SELECT REPLACE(REPLACE(TABLE_NAME,'user_identity_tab','user_identities'),'user_tab','users'), COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, IFNULL(COLUMN_DEFAULT,'NULL'), IFNULL(COLLATION_NAME,''), IFNULL(CHARACTER_SET_NAME,''), COLUMN_COMMENT, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('user_tab','user_identity_tab') AND COLUMN_NAME NOT IN ('email_verified_at','created_at','updated_at','locale','provider','username','username_changed_at') ORDER BY 1, COLUMN_NAME`)
	equalLines(t, "untouched column metadata (incl. email collation and comment)", colsNew, colsOld)
	t.Log("new columns:\n" + strings.Join(dump(t, ctx, d, `SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, IFNULL(COLUMN_DEFAULT,'NULL') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('user_tab','user_identity_tab') ORDER BY TABLE_NAME, ORDINAL_POSITION`), "\n"))
	t.Log("indexes:\n" + strings.Join(dump(t, ctx, d, `SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('user_tab','user_identity_tab') ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`), "\n"))

	// Uniqueness: every distinct email string is unique, and a case/accent twin of an existing email is a different account.
	for i, tw := range []string{"EEN@exämple.com", "eEN@EXÄMPLE.com", "een@example.com"} {
		if _, err := d.ExecContext(ctx, `INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES (?,?, 'x', ?, 1, 1)`, fmt.Sprintf("TW%d", i), tw, fmt.Sprintf("user-tw%d", i)); err != nil {
			t.Errorf("twin %q refused: %v", tw, err)
		}
	}
	// Exact duplicate refused.
	if _, err := d.ExecContext(ctx, `INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('DUP', 'een@exämple.com', 'x', 'user-dup', 1, 1)`); err == nil {
		t.Error("exact duplicate email accepted")
	}
	if _, err := d.ExecContext(ctx, `INSERT INTO user_tab (public_id, email, display_name, username, created_at, updated_at) VALUES ('U001', 'zzz@x.com', 'x', 'user-u001x', 1, 1)`); err == nil {
		t.Error("duplicate public_id accepted")
	}
	exec(`DELETE FROM user_tab WHERE display_name = 'x'`)

	downUntil(t, ctx, d, func() bool { return tableExists(t, ctx, d, "users") })
	equalLines(t, "users down", dump(t, ctx, d, oldU), wantU)
	equalLines(t, "identities down", dump(t, ctx, d, oldI), wantI)
	equalLines(t, "column metadata after down", dump(t, ctx, d, `SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, IFNULL(COLUMN_DEFAULT,'NULL'), IFNULL(COLLATION_NAME,''), IFNULL(CHARACTER_SET_NAME,''), COLUMN_COMMENT, EXTRA FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('users','user_identities') AND COLUMN_NAME NOT IN ('email_verified_at','created_at','updated_at','locale','provider') ORDER BY TABLE_NAME, COLUMN_NAME`), colsOld)
	equalLines(t, "indexes after down", dump(t, ctx, d, `SELECT TABLE_NAME, INDEX_NAME, NON_UNIQUE, SEQ_IN_INDEX, COLUMN_NAME FROM information_schema.STATISTICS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('users','user_identities') ORDER BY TABLE_NAME, INDEX_NAME, SEQ_IN_INDEX`), statOld)
	t.Log("down column types:\n" + strings.Join(dump(t, ctx, d, `SELECT TABLE_NAME, COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, IFNULL(COLUMN_DEFAULT,'NULL') FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME IN ('users','user_identities') ORDER BY TABLE_NAME, ORDINAL_POSITION`), "\n"))
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatal(err)
	}
	equalLines(t, "users up again", dump(t, ctx, d, newU), wantU)
	equalLines(t, "identities up again", dump(t, ctx, d, newI), wantI)

	// No foreign key anywhere in the schema; no DATETIME/TIMESTAMP/ENUM column anywhere in the *_tab tables.
	if r := dump(t, ctx, d, `SELECT TABLE_NAME, CONSTRAINT_NAME FROM information_schema.TABLE_CONSTRAINTS WHERE CONSTRAINT_SCHEMA = DATABASE() AND CONSTRAINT_TYPE = 'FOREIGN KEY'`); len(r) != 0 {
		t.Errorf("foreign keys: %v", r)
	}
	if r := dump(t, ctx, d, `SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND DATA_TYPE IN ('datetime','timestamp','enum','date','time') AND TABLE_NAME <> 'goose_db_version' AND COLUMN_NAME <> 'birthday'`); len(r) != 0 {
		t.Errorf("legacy column types: %v", r)
	}
	if r := dump(t, ctx, d, `SELECT TABLE_NAME FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_TYPE='BASE TABLE' AND TABLE_NAME NOT LIKE '%\_tab' AND TABLE_NAME NOT LIKE 'goose%' AND TABLE_NAME <> 'app_meta'`); len(r) != 0 {
		t.Errorf("tables without _tab: %v", r)
	}
	if r := dump(t, ctx, d, `SELECT TABLE_NAME, COLUMN_NAME FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND UPPER(COLUMN_NAME) IN (SELECT WORD FROM information_schema.KEYWORDS WHERE RESERVED = 1)`); len(r) != 0 {
		t.Errorf("reserved-word columns: %v", r)
	}
}
