// Command smemories-migrate applies the embedded SQL migrations: up, down (one step) or status.
// It reads the same SMEM_* environment as the API and also runs as a one-off task on ECS.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db"
)

func main() {
	if len(os.Args) != 2 || (os.Args[1] != "up" && os.Args[1] != "down" && os.Args[1] != "status") {
		fmt.Fprintln(os.Stderr, "usage: smemories-migrate up|down|status")
		os.Exit(2)
	}
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	if cfg.DBDSN == "" {
		fmt.Fprintln(os.Stderr, "config error: SMEM_DB_DSN is required")
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	d, err := db.Open(ctx, cfg, logger)
	if err != nil {
		fmt.Fprintln(os.Stderr, "database error:", err)
		os.Exit(1)
	}
	defer d.Close()

	switch os.Args[1] {
	case "up":
		err = db.MigrateUp(ctx, d)
	case "down":
		err = db.MigrateDown(ctx, d)
	case "status":
		err = db.MigrateStatus(ctx, d, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "migrate", os.Args[1], "failed:", err)
		os.Exit(1)
	}
}
