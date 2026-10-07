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
	mc, err := mysql.ParseDSN(dsn)
	if err != nil {
		return nil, errors.New("DSN is not valid: want user:pass@tcp(host:port)/dbname")
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
		return nil, fmt.Errorf("mysql connector: %w", err)
	}
	d := sql.OpenDB(connector)
	d.SetMaxOpenConns(cfg.DBMaxOpen)
	d.SetMaxIdleConns(cfg.DBMaxIdle)
	d.SetConnMaxLifetime(cfg.DBConnMaxLifetime)
	return d, nil
}

// Open builds the pool from cfg.DBDSN and retries the first ping for up to FirstPingWait.
// On failure the pool is closed and the error says the database is unreachable (driver
// error text only, never the DSN).
func Open(ctx context.Context, cfg config.Config, logger *slog.Logger) (*sql.DB, error) {
	mc, err := Normalize(cfg.DBDSN)
	if err != nil {
		return nil, fmt.Errorf("SMEM_DB_DSN: %w", err)
	}
	d, err := New(mc, cfg)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, FirstPingWait)
	defer cancel()
	for {
		pctx, pcancel := context.WithTimeout(ctx, pingTimeout)
		err = d.PingContext(pctx)
		pcancel()
		if err == nil {
			return d, nil
		}
		logger.Warn("database not ready, retrying", "addr", mc.Addr, "error", err)
		select {
		case <-ctx.Done():
			_ = d.Close()
			return nil, fmt.Errorf("database at %s not reachable after %s: %w", mc.Addr, FirstPingWait, err)
		case <-time.After(pingEvery):
		}
	}
}
