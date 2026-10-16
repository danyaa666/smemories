// Package db opens the MySQL pool and runs the embedded goose migrations.
package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/config"
)

const (
	// FirstPingWait is how long Open keeps retrying the first ping (the database may still
	// be starting next to the API).
	FirstPingWait = 10 * time.Second
	pingTimeout   = time.Second
	pingEvery     = 500 * time.Millisecond
)

// Normalize parses dsn and forces the session settings every connection must have:
// UTC times parsed into time.Time and utf8mb4 / utf8mb4_0900_ai_ci. The error never
// includes the DSN, which carries the password.
func Normalize(dsn string) (*mysql.Config, error) {
	if dsn == "" { // ParseDSN("") succeeds and would silently dial 127.0.0.1:3306
		return nil, apperr.New(apperr.Internal, "DSN is empty: want user:pass@tcp(host:port)/dbname")
	}
	mc, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, apperr.New(apperr.Internal, "DSN is not valid: want user:pass@tcp(host:port)/dbname")
	}
	mc.ParseTime = true
	mc.Loc = time.UTC
	mc.Collation = "utf8mb4_0900_ai_ci"
	delete(mc.Params, "charset") // the collation alone picks the charset; a charset param would run SET charset=...
	return mc, nil
}

// New returns a pool for mc with the pool limits from cfg. It does not connect.
func New(mc *mysql.Config, cfg config.Config) (*sql.DB, error) {
	connector, err := mysql.NewConnector(mc)
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "mysql connector")
	}
	d := sql.OpenDB(connector)
	d.SetMaxOpenConns(cfg.DBMaxOpen)
	d.SetMaxIdleConns(cfg.DBMaxIdle)
	d.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	return d, nil
}

// Open builds the pool from cfg.DBDSN and retries the first ping for up to FirstPingWait.
// Errors that retrying cannot fix (access denied, unknown database) fail at once. On failure
// the pool is closed and the error names the address and user, never the password.
func Open(ctx context.Context, cfg config.Config, logger *slog.Logger) (*sql.DB, error) {
	mc, err := Normalize(cfg.DBDSN)
	if err != nil {
		return nil, apperr.Wrap(apperr.Internal, err, "SMEM_DB_DSN")
	}
	d, err := New(mc, cfg)
	if err != nil {
		return nil, err
	}
	if err := waitReady(ctx, mc.Addr, mc.User, d.PingContext, logger); err != nil {
		_ = d.Close()
		return nil, err
	}
	return d, nil
}

// permanent reports MySQL errors that cannot fix themselves: 1045 access denied, 1049 unknown database.
func permanent(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && (me.Number == 1045 || me.Number == 1049)
}

func waitReady(ctx context.Context, addr, user string, ping func(context.Context) error, logger *slog.Logger) error {
	ctx, cancel := context.WithTimeout(ctx, FirstPingWait)
	defer cancel()
	for {
		pctx, pcancel := context.WithTimeout(ctx, pingTimeout)
		err := ping(pctx)
		pcancel()
		if err == nil {
			return nil
		}
		if permanent(err) {
			return apperr.Wrap(apperr.Internal, err, fmt.Sprintf("database at %s refused user %q (check the credentials and database name in SMEM_DB_DSN)", addr, user))
		}
		logger.Warn("database not ready, retrying", "addr", addr, "error", err)
		select {
		case <-ctx.Done():
			return apperr.Wrap(apperr.Internal, err, fmt.Sprintf("database at %s not reachable after %s", addr, FirstPingWait))
		case <-time.After(pingEvery):
		}
	}
}
