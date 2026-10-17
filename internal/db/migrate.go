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
	res, err := p.Up(ctx)
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
func CheckSchema(ctx context.Context, d *sql.DB) error {
	p, err := provider(d, 0)
	if err != nil {
		return err
	}
	known := map[int64]bool{}
	var want int64
	for _, s := range p.ListSources() {
		known[s.Version] = true
		want = max(want, s.Version)
	}
	rows, err := d.QueryContext(ctx, "SELECT version_id FROM goose_db_version WHERE is_applied AND version_id > 0")
	if err != nil {
		var me *mysql.MySQLError
		if errors.As(err, &me) && me.Number == 1146 { // no goose_db_version table: never migrated
			return schemaBehind(0, want, len(known))
		}
		return apperr.Wrap(apperr.Internal, err, "read the schema version")
	}
	defer func() { _ = rows.Close() }()
	var have int64
	missing, ahead := len(known), 0
	for rows.Next() {
		var v int64
		if err := rows.Scan(&v); err != nil {
			return apperr.Wrap(apperr.Internal, err, "read the schema version")
		}
		have = max(have, v)
		if known[v] {
			missing--
		} else {
			ahead++
		}
	}
	if err := rows.Err(); err != nil {
		return apperr.Wrap(apperr.Internal, err, "read the schema version")
	}
	if missing > 0 {
		return schemaBehind(have, want, missing)
	}
	if ahead > MaxSchemaAhead {
		return apperr.New(apperr.Unavailable, fmt.Sprintf("database schema is at version %d, this build expects %d: %d migrations ahead (at most %d are compatible); deploy the newer image", have, want, ahead, MaxSchemaAhead))
	}
	return nil
}

func schemaBehind(have, want int64, missing int) error {
	return apperr.New(apperr.Unavailable, fmt.Sprintf("database schema is at version %d, this build expects %d: %d migrations pending; run smemories-migrate up first", have, want, missing))
}
