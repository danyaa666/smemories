// Command smemories-migrate applies the embedded SQL migrations: up, status, version, and down (one step, never in prod).
// It reads the same SMEM_* environment as the API and also runs as a one-off task on ECS. `up` takes a MySQL advisory lock, so two
// tasks started together migrate one after the other. Exit codes are listed in docs/deploy.md.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db"
)

const (
	exitFailed  = 1 // configuration, connection or migration failure
	exitUsage   = 2 // unknown command or flag
	exitLocked  = 3 // the migration lock was still held after db.LockWait
	exitRefused = 4 // down (or --allow-down) in prod
)

const usage = "usage: smemories-migrate up|status|version|down --allow-down"

func main() {
	cmd, allowDown, ok := parseArgs(os.Args[1:])
	if !ok {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(exitUsage)
	}
	cfg, err := config.LoadMigrate(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(exitFailed)
	}
	if cfg.DBDSN == "" {
		fmt.Fprintln(os.Stderr, "config error: SMEM_DB_DSN is required")
		os.Exit(exitFailed)
	}
	if cfg.Env == "prod" && (cmd == "down" || allowDown) { // before any connection: prod schemas only move forward
		fmt.Fprintln(os.Stderr, "refused: down and --allow-down are never allowed with SMEM_ENV=prod; roll forward with a new migration")
		os.Exit(exitRefused)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	d, err := db.Open(ctx, cfg, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database error:", err)
		os.Exit(exitFailed)
	}
	defer func() { _ = d.Close() }()
	d.SetMaxOpenConns(max(cfg.DBMaxOpen, 2)) // one connection holds the lock, another runs the statements

	switch cmd {
	case "up":
		err = db.MigrateUpLog(ctx, d, logger, db.LockWait)
	case "down":
		err = db.MigrateDown(ctx, d)
	case "status":
		err = db.MigrateStatus(ctx, d, os.Stdout)
	case "version":
		var v int64
		if v, err = db.MigrateVersion(ctx, d); err == nil {
			fmt.Println(v)
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate", cmd, "failed:", err)
		if errors.Is(err, db.ErrLockTimeout) {
			os.Exit(exitLocked)
		}
		os.Exit(exitFailed)
	}
}

// parseArgs accepts `up`, `status`, `version`, and `down --allow-down` (down needs the flag, nothing else takes it).
func parseArgs(args []string) (cmd string, allowDown, ok bool) {
	if len(args) == 0 {
		return "", false, false
	}
	switch args[0] {
	case "down":
		return "down", true, len(args) == 2 && args[1] == "--allow-down"
	case "up", "status", "version":
		return args[0], false, len(args) == 1
	}
	return "", false, false
}
