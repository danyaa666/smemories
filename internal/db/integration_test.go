//go:build integration

package db_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/pressly/goose/v3"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
)

// migrateDownAll rolls back every migration, however many exist (each Down undoes one).
func migrateDownAll(t *testing.T, ctx context.Context, d *sql.DB) {
	t.Helper()
	for i := 0; i < 100; i++ {
		err := db.MigrateDown(ctx, d)
		if errors.Is(err, goose.ErrNoNextVersion) {
			return
		}
		if err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	t.Fatal("down never reached version 0")
}

func TestMigrateCycle(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t) // already migrated up

	epoch := func() string {
		var v string
		if err := d.QueryRowContext(ctx, "SELECT v FROM app_meta WHERE k='schema_epoch'").Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	if epoch() != "1" {
		t.Fatalf("schema_epoch = %q, want 1", epoch())
	}
	for i := 0; i < 2; i++ { // down -> up, twice
		migrateDownAll(t, ctx, d)
		if _, err := d.ExecContext(ctx, "SELECT 1 FROM app_meta"); err == nil {
			t.Fatal("app_meta should be gone after down")
		}
		if err := db.MigrateUp(ctx, d); err != nil {
			t.Fatalf("up: %v", err)
		}
		if epoch() != "1" {
			t.Fatal("schema_epoch lost after up")
		}
	}
	if err := db.MigrateUp(ctx, d); err != nil { // up on an up-to-date schema is a no-op
		t.Fatalf("idempotent up: %v", err)
	}
	var out strings.Builder
	if err := db.MigrateStatus(ctx, d, &out); err != nil || !strings.Contains(out.String(), "applied") {
		t.Fatalf("status: %v %q", err, out.String())
	}
}

func TestUTF8MB4RoundTrip(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	const want = "Chúc mừng 🎓 Đặng Thị Hồng"

	if _, err := d.ExecContext(ctx, "INSERT INTO app_meta (k, v) VALUES ('greeting', ?)", want); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := d.QueryRowContext(ctx, "SELECT v FROM app_meta WHERE k='greeting'").Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("round trip: got %q (% x), want %q", got, got, want)
	}

	c, err := d.Conn(ctx) // a pooled connection, as the app uses
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	var cs, cc, cr, coll, tz string
	if err := c.QueryRowContext(ctx, "SELECT @@character_set_client, @@character_set_connection, @@character_set_results, @@collation_connection, @@session.time_zone").Scan(&cs, &cc, &cr, &coll, &tz); err != nil {
		t.Fatal(err)
	}
	if cs != "utf8mb4" || cc != "utf8mb4" || cr != "utf8mb4" || coll != "utf8mb4_0900_ai_ci" {
		t.Fatalf("connection charsets: client=%s connection=%s results=%s collation=%s", cs, cc, cr, coll)
	}
	if tz != "+00:00" {
		t.Fatalf("time_zone = %s, want +00:00", tz)
	}
	var mode string
	if err := c.QueryRowContext(ctx, "SELECT @@session.sql_mode").Scan(&mode); err != nil || !strings.Contains(mode, "STRICT_ALL_TABLES") {
		t.Fatalf("sql_mode = %q (%v), want STRICT_ALL_TABLES", mode, err)
	}
}

func TestReadyzAgainstRealDatabase(t *testing.T) {
	d := dbtest.New(t)
	h := httpx.NewRouter(slog.New(slog.NewTextHandler(io.Discard, nil)), httpx.Ready(d, slog.New(slog.NewTextHandler(io.Discard, nil))))
	get := func() *httptest.ResponseRecorder {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest("GET", "/readyz", nil))
		return rec
	}
	if rec := get(); rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ready"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	_ = d.Close() // a closed pool cannot ping
	rec := get()
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"not_ready"`) {
		t.Fatalf("got %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "sql:") {
		t.Fatalf("driver error leaked: %s", rec.Body.String())
	}
}

func TestSessionTimesAreUTC(t *testing.T) {
	d := dbtest.New(t)
	var ts time.Time
	if err := d.QueryRow("SELECT CAST('2026-10-07 01:02:03' AS DATETIME(6))").Scan(&ts); err != nil {
		t.Fatal(err)
	}
	if ts.Location() != time.UTC || ts.Hour() != 1 {
		t.Fatalf("time = %v, want 01:02:03 UTC", ts)
	}
}

// T-037: migration 0009 adds the Letter page size; its Down turns Letter books into A5 and keeps the others.
func TestMigratePageSizeLetter(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
	}
	mustExec(`INSERT INTO users (public_id, email, display_name, created_at, updated_at) VALUES ('U1', 'a@example.com', 'A', NOW(6), NOW(6))`)
	for i, size := range []string{"A5", "A4", "Letter"} {
		mustExec(`INSERT INTO yearbooks (public_id, owner_id, title, language, page_size, created_at, updated_at)
			SELECT ?, id, ?, 'en', ?, NOW(6), NOW(6) FROM users WHERE public_id = 'U1'`, "Y"+string(rune('1'+i)), size, size)
	}
	sizes := func() string {
		t.Helper()
		var s string
		if err := d.QueryRowContext(ctx, `SELECT GROUP_CONCAT(page_size ORDER BY public_id) FROM yearbooks`).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	if got := sizes(); got != "A5,A4,Letter" {
		t.Fatalf("before down: %s", got)
	}
	for range 2 { // 0010 (notes) is the latest migration and does not touch yearbooks; 0009 is the one under test
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
	}
	if got := sizes(); got != "A5,A4,A5" {
		t.Fatalf("after down: %s, want A5,A4,A5", got)
	}
	if _, err := d.ExecContext(ctx, `UPDATE yearbooks SET page_size = 'Letter' WHERE public_id = 'Y1'`); err == nil {
		t.Fatal("old enum should reject Letter")
	}
	if err := db.MigrateUp(ctx, d); err != nil {
		t.Fatalf("up: %v", err)
	}
	mustExec(`UPDATE yearbooks SET page_size = 'Letter' WHERE public_id = 'Y1'`)
	if got := sizes(); got != "Letter,A4,A5" {
		t.Fatalf("after up: %s", got)
	}
}
