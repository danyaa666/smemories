package db

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/migrations"
)

// LockWait is how long a migration waits for the advisory lock another smemories-migrate holds (a migration can take minutes on a big table).
const LockWait = 5 * time.Minute

// MaxSchemaAhead is how many migrations the database may have applied that this build does not know before CheckSchema refuses: the
// previous release must keep running against the next schema ("expand then contract", docs/deploy.md), anything further is the wrong image.
const MaxSchemaAhead = 3

// ErrLockTimeout is returned when the migration lock is still held after LockWait.
var ErrLockTimeout = apperr.New(apperr.Unavailable, "another smemories-migrate holds the migration lock")

// lockName is a MySQL advisory lock name (limit 64 characters) per database, so two schemas on one server do not block each other.
func lockName(ctx context.Context, conn *sql.Conn) (string, error) {
	var name sql.NullString
	if err := conn.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&name); err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(name.String))
	return "smemories_migrate_" + hex.EncodeToString(sum[:8]), nil
}

// mysqlLocker is a goose SessionLocker on GET_LOCK: the lock belongs to the connection goose migrates on, so MySQL releases it when the
// session ends, even if the process is killed.
type mysqlLocker struct{ wait time.Duration }

// NewLocker returns the advisory lock used by the migrations, waiting up to wait. Exported for the tests that hold it.
func NewLocker(wait time.Duration) lock.SessionLocker { return mysqlLocker{wait} }

func (l mysqlLocker) SessionLock(ctx context.Context, conn *sql.Conn) error {
	name, err := lockName(ctx, conn)
	if err != nil {
		return err
	}
	var got sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, ?)", name, int(l.wait/time.Second)).Scan(&got); err != nil {
		return err
	}
	if !got.Valid || got.Int64 != 1 {
		return ErrLockTimeout
	}
	return nil
}

func (mysqlLocker) SessionUnlock(ctx context.Context, conn *sql.Conn) error {
	name, err := lockName(ctx, conn)
	if err != nil {
		return err
	}
	_, err = conn.ExecContext(ctx, "SELECT RELEASE_LOCK(?)", name)
	return err
}

// provider is locked when wait > 0.
func provider(d *sql.DB, wait time.Duration) (*goose.Provider, error) {
	var opts []goose.ProviderOption
	if wait > 0 {
		opts = append(opts, goose.WithSessionLocker(mysqlLocker{wait}))
	}
	return goose.NewProvider(goose.DialectMySQL, d, migrations.FS, opts...)
}

// MigrateUp applies every pending migration under the advisory lock; see MigrateUpLog.
func MigrateUp(ctx context.Context, d *sql.DB) error { return MigrateUpLog(ctx, d, nil, LockWait) }

// MigrateUpLog applies every pending migration. It takes the advisory lock first (waiting up to wait, then ErrLockTimeout), so two
// concurrent callers never apply the same migration twice: the second finds nothing left to do. Each applied migration is logged
// (logger may be nil). The pool must allow at least two open connections: one holds the lock, one runs the statements.
func MigrateUpLog(ctx context.Context, d *sql.DB, logger *slog.Logger, wait time.Duration) error {
	p, err := provider(d, wait)
	if err != nil {
		return err
	}
	// goose's Up first calls HasPending, which ignores the session locker and creates the version table (and inserts its version 0) without
	// the lock: a second task could then take the lock between the CREATE TABLE and the INSERT and fail with "missing zero version
	// migration" (T-108). GetDBVersion does take the lock first, on the connection that then creates the table, so by the time Up runs the
	// table is complete and HasPending only reads it.
	if wait > 0 {
		if _, err := p.GetDBVersion(ctx); err != nil {
			return err
		}
	}
	res, err := p.Up(ctx)
	err = halfAppliedHint(err)
	if logger != nil {
		for _, r := range res {
			logger.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration", r.Duration.String())
		}
		if err == nil && len(res) == 0 {
			logger.Info("schema is up to date")
		}
	}
	return err
}

// MySQL errors a statement of an already (half) applied migration raises when it runs a second time: duplicate column, table already
// exists, duplicate key name, can't drop (does not exist).
var halfAppliedErrors = map[uint16]bool{1060: true, 1050: true, 1061: true, 1091: true}

// halfAppliedHint adds the way out to a migration failure that looks like the leftover of a killed run: MySQL commits DDL implicitly, so
// a process killed in the middle of a migration leaves its first statements applied and the version not recorded (docs/deploy.md).
func halfAppliedHint(err error) error {
	var me *mysql.MySQLError
	if errors.As(err, &me) && halfAppliedErrors[me.Number] {
		return apperr.Wrap(apperr.Internal, err, "this migration may be half-applied by an earlier killed run: see \"A half-applied migration\" in docs/deploy.md")
	}
	return err
}

// MigrateDown rolls back the most recently applied migration (under the same lock).
func MigrateDown(ctx context.Context, d *sql.DB) error {
	p, err := provider(d, LockWait)
	if err != nil {
		return err
	}
	_, err = p.Down(ctx)
	return err
}

// MigrateVersion returns the highest applied migration version (0 on an empty database).
func MigrateVersion(ctx context.Context, d *sql.DB) (int64, error) {
	p, err := provider(d, 0)
	if err != nil {
		return 0, err
	}
	return p.GetDBVersion(ctx)
}

// MigrateStatus writes one line per migration (applied time or "pending") to w.
func MigrateStatus(ctx context.Context, d *sql.DB, w io.Writer) error {
	p, err := provider(d, 0)
	if err != nil {
		return err
	}
	rows, err := p.Status(ctx)
	if err != nil {
		return err
	}
	for _, s := range rows {
		applied := "pending"
		if s.State == goose.StateApplied {
			applied = "applied " + s.AppliedAt.UTC().Format("2006-01-02 15:04:05Z")
		}
		_, _ = fmt.Fprintf(w, "%04d %-20s %s\n", s.Source.Version, s.Source.Path, applied)
	}
	return nil
}

// CheckSchema is the API's start-up guard: it never migrates and writes nothing. It refuses when a migration this build embeds is not
// applied (the database is behind) or when more than MaxSchemaAhead applied migrations are unknown to it (wrong image, or a rollback of
// several releases). The error names both versions: the highest applied and the one this build expects.
func CheckSchema(ctx context.Context, d *sql.DB) error { return CheckSchemaLog(ctx, d, nil) }

// CheckSchemaLog is CheckSchema that also logs one WARN (logger may be nil) when the database is 1 to MaxSchemaAhead migrations ahead:
// the normal state while an old release runs against the next schema, so it is said once, at start-up.
func CheckSchemaLog(ctx context.Context, d *sql.DB, logger *slog.Logger) error {
	p, err := provider(d, 0)
	if err != nil {
		return err
	}
	known := map[int64]bool{}
	for _, s := range p.ListSources() {
		known[s.Version] = true
	}
	rows, err := d.QueryContext(ctx, "SELECT version_id FROM goose_db_version WHERE is_applied AND version_id > 0")
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1146 { // no goose_db_version table: never migrated
			_, err = compareSchema(known, nil)
			return err
		}
		return apperr.Wrap(apperr.Internal, err, "read the schema version")
	}
	defer func() { _ = rows.Close() }()
	var applied []int64
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return apperr.Wrap(apperr.Internal, err, "read the schema version")
		}
		applied = append(applied, v)
	}
	if err := rows.Err(); err != nil {
		return apperr.Wrap(apperr.Internal, err, "read the schema version")
	}
	ahead, err := compareSchema(known, applied)
	if err == nil && ahead > 0 && logger != nil {
		logger.Warn("database schema is ahead of this build (normal during a rolling deploy)", "database_version", slices.Max(applied), "build_version", maxVersion(known), "migrations_ahead", ahead)
	}
	return err
}

func maxVersion(known map[int64]bool) (m int64) {
	for v := range known {
		m = max(m, v)
	}
	return m
}

// compareSchema is the decision of CheckSchema on plain data: known are the embedded versions, applied the recorded ones. It returns how
// many applied migrations this build does not know, or the refusal.
func compareSchema(known map[int64]bool, applied []int64) (ahead int, err error) {
	want, have := maxVersion(known), int64(0)
	seen := map[int64]bool{}
	for _, v := range applied {
		have = max(have, v)
		seen[v] = true
		if !known[v] {
			ahead++
		}
	}
	var missing []int64 // embedded but not applied, ascending
	for v := range known {
		if !seen[v] {
			missing = append(missing, v)
		}
	}
	slices.Sort(missing)
	if len(missing) > 0 {
		if missing[0] < have { // a gap in the middle: name the first missing version and the next applied one after it
			next := have
			for _, v := range applied {
				if v > missing[0] {
					next = min(next, v)
				}
			}
			return 0, apperr.New(apperr.Unavailable, fmt.Sprintf("database schema is missing migration %d (the next applied one is %d, highest %d, this build expects %d): %d migrations not applied; run smemories-migrate up first, and see docs/deploy.md if it fails", missing[0], next, have, want, len(missing)))
		}
		return 0, apperr.New(apperr.Unavailable, fmt.Sprintf("database schema is at version %d, this build expects %d: %d migrations pending; run smemories-migrate up first", have, want, len(missing)))
	}
	if ahead > MaxSchemaAhead {
		return ahead, apperr.New(apperr.Unavailable, fmt.Sprintf("database schema is at version %d, this build expects %d: %d migrations ahead (at most %d are compatible); deploy the newer image", have, want, ahead, MaxSchemaAhead))
	}
	return ahead, nil
}
