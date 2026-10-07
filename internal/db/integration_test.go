//go:build integration

package db_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/internal/httpx"
)

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
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatalf("down: %v", err)
		}
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
	defer c.Close()
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
	d.Close() // a closed pool cannot ping
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
