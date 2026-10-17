//go:build integration

package db_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/db/dbtest"
	"github.com/danyaa666/smemories/migrations"
)

// syncBuf is a log sink several goroutines write to.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func migrationFiles(t *testing.T) int {
	t.Helper()
	files, err := migrations.FS.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, f := range files {
		if strings.HasSuffix(f.Name(), ".sql") {
			n++
		}
	}
	return n
}

// AC1: several tasks started together on an empty database; every migration is applied exactly once and all of them succeed.
func TestMigrateUpConcurrentAppliesOnce(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewEmpty(t)
	d.SetMaxOpenConns(20)
	versionOf(t, d) // creates goose_db_version first, so the tasks race on the migrations themselves
	var logs syncBuf
	logger := slog.New(slog.NewJSONHandler(&logs, nil))

	const tasks = 4
	var wg sync.WaitGroup
	errs := make([]error, tasks)
	gate := make(chan struct{})
	for i := range tasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-gate // start together
			errs[i] = db.MigrateUpLog(ctx, d, logger, 2*time.Minute)
		}()
	}
	close(gate)
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Errorf("task %d: %v", i, err)
		}
	}
	want := migrationFiles(t)
	if got := strings.Count(logs.b.String(), `"msg":"migration applied"`); got != want {
		t.Errorf("%d \"migration applied\" lines across %d tasks, want exactly %d (one per migration)", got, tasks, want)
	}
	var rows, distinct int
	if err := d.QueryRowContext(ctx, "SELECT COUNT(*), COUNT(DISTINCT version_id) FROM goose_db_version WHERE version_id > 0").Scan(&rows, &distinct); err != nil {
		t.Fatal(err)
	}
	if rows != want || distinct != want {
		t.Errorf("goose_db_version has %d rows (%d distinct) for %d migrations", rows, distinct, want)
	}
	if err := db.CheckSchema(ctx, d); err != nil {
		t.Errorf("CheckSchema after concurrent up: %v", err)
	}
}

// A task that cannot get the lock in time fails with ErrLockTimeout and applies nothing; once the holder lets go, it works.
func TestMigrateUpLockTimeout(t *testing.T) {
	ctx := context.Background()
	d := dbtest.NewEmpty(t)
	holder, err := d.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	if err := db.NewLocker(time.Second).SessionLock(ctx, holder); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	err = db.MigrateUpLog(ctx, d, nil, time.Second)
	if !errors.Is(err, db.ErrLockTimeout) || !apperr.Is(err, apperr.Unavailable) {
		t.Fatalf("want ErrLockTimeout, got %v", err)
	}
	if time.Since(start) > 10*time.Second {
		t.Errorf("waited %s for a 1s lock", time.Since(start))
	}
	var n int
	_ = d.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'user_tab'").Scan(&n)
	if n != 0 {
		t.Error("a migration ran without the lock")
	}

	if err := db.NewLocker(time.Second).SessionUnlock(ctx, holder); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateUpLog(ctx, d, nil, time.Second); err != nil {
		t.Fatalf("up after the lock was released: %v", err)
	}
}

// A lock held on another database does not block this one (the lock name carries the schema).
func TestMigrateLockIsPerDatabase(t *testing.T) {
	ctx := context.Background()
	a, b := dbtest.NewEmpty(t), dbtest.NewEmpty(t)
	holder, err := a.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = holder.Close() }()
	if err := db.NewLocker(time.Second).SessionLock(ctx, holder); err != nil {
		t.Fatal(err)
	}
	if err := db.MigrateUpLog(ctx, b, nil, time.Second); err != nil {
		t.Fatalf("up on another database: %v", err)
	}
}

func versionOf(t *testing.T, d *sql.DB) int64 {
	t.Helper()
	v, err := db.MigrateVersion(context.Background(), d)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// AC2: the API's start-up guard.
func TestCheckSchema(t *testing.T) {
	ctx := context.Background()

	t.Run("never migrated: refused, nothing written", func(t *testing.T) {
		d := dbtest.NewEmpty(t)
		err := db.CheckSchema(ctx, d)
		if err == nil || !strings.Contains(err.Error(), "version 0") || !strings.Contains(err.Error(), "expects 20") || !strings.Contains(err.Error(), "smemories-migrate up") {
			t.Fatalf("want a refusal naming both versions, got %v", err)
		}
		var n int
		_ = d.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()").Scan(&n)
		if n != 0 {
			t.Errorf("the check created %d tables", n)
		}
	})

	t.Run("behind by one migration: refused, both versions named", func(t *testing.T) {
		d := dbtest.New(t)
		want := versionOf(t, d)
		if err := db.MigrateDown(ctx, d); err != nil {
			t.Fatal(err)
		}
		have := versionOf(t, d)
		err := db.CheckSchema(ctx, d)
		if err == nil || !strings.Contains(err.Error(), "version "+itoa(have)) || !strings.Contains(err.Error(), "expects "+itoa(want)) {
			t.Fatalf("want a refusal naming %d and %d, got %v", have, want, err)
		}
		if err := db.MigrateUp(ctx, d); err != nil {
			t.Fatal(err)
		}
		if err := db.CheckSchema(ctx, d); err != nil {
			t.Errorf("at the expected version: %v", err)
		}
	})

	t.Run("ahead: tolerated up to MaxSchemaAhead, refused beyond", func(t *testing.T) {
		d := dbtest.New(t)
		for i := range db.MaxSchemaAhead {
			if _, err := d.ExecContext(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (?, 1)", 29990101000000+i); err != nil {
				t.Fatal(err)
			}
		}
		if err := db.CheckSchema(ctx, d); err != nil {
			t.Fatalf("ahead by %d should be compatible: %v", db.MaxSchemaAhead, err)
		}
		if _, err := d.ExecContext(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (29990102000000, 1)"); err != nil {
			t.Fatal(err)
		}
		err := db.CheckSchema(ctx, d)
		if err == nil || !strings.Contains(err.Error(), "29990102000000") || !strings.Contains(err.Error(), "migrations ahead") {
			t.Fatalf("want a refusal naming the versions, got %v", err)
		}
	})
}

// AC2 (T-090): a start 1 to MaxSchemaAhead migrations ahead logs exactly one WARN naming both versions; equal and refused starts log none.
func TestCheckSchemaLogsAheadWarning(t *testing.T) {
	ctx := context.Background()
	d := dbtest.New(t)
	build := versionOf(t, d)
	var logs syncBuf
	logger := slog.New(slog.NewJSONHandler(&logs, nil))
	if err := db.CheckSchemaLog(ctx, d, logger); err != nil || logs.b.Len() != 0 {
		t.Fatalf("equal schema: err=%v, log %q; want no error and no line", err, logs.b.String())
	}
	if _, err := d.ExecContext(ctx, "INSERT INTO goose_db_version (version_id, is_applied) VALUES (29990101000000, 1), (29990102000000, 1)"); err != nil {
		t.Fatal(err)
	}
	if err := db.CheckSchemaLog(ctx, d, logger); err != nil {
		t.Fatal(err)
	}
	out := logs.b.String()
	for _, w := range []string{`"level":"WARN"`, `"database_version":29990102000000`, `"build_version":` + itoa(build), `"migrations_ahead":2`} {
		if !strings.Contains(out, w) {
			t.Errorf("the WARN lacks %s: %q", w, out)
		}
	}
	if strings.Count(out, "\n") != 1 {
		t.Errorf("want exactly one line, got %q", out)
	}
}

// AC1 (T-090): a migration that runs a second time because the first run was killed half way fails with the hint (errors 1050 and 1060).
func TestMigrateUpHalfAppliedHint(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		forget  string // delete the goose rows from this version on, so up re-runs those migrations over their own leftovers
		wantErr string
	}{
		{"table already exists (1050)", "20261017100000", "Error 1050"},
		{"duplicate column (1060)", "20261016100000", "Error 1060"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := dbtest.New(t)
			if _, err := d.ExecContext(ctx, "DELETE FROM goose_db_version WHERE version_id >= "+tc.forget); err != nil {
				t.Fatal(err)
			}
			err := db.MigrateUpLog(ctx, d, nil, time.Second)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) || !strings.Contains(err.Error(), "half-applied") || !strings.Contains(err.Error(), "docs/deploy.md") {
				t.Fatalf("want %s plus the half-applied hint, got %v", tc.wantErr, err)
			}
		})
	}
}

func itoa(v int64) string { return strconv.FormatInt(v, 10) }
