// Command smemories-media-backfill creates the print-size object (T-057) of photos uploaded before it existed.
// It needs only the database and object-store SMEM_* settings (not Redis or the email-code key), is idempotent and can be stopped and
// restarted at any time. docs/media.md describes the run.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/storage"
)

func main() {
	dryRun := flag.Bool("dry-run", false, "count the photos without a print object, write nothing")
	batch := flag.Int("batch", 100, "photos read from the database at a time")
	maxFails := flag.Int("max-consecutive-failures", 20, "stop after this many object-store errors in a row")
	flag.Parse()
	if *batch < 1 || *batch > 10000 || *maxFails < 1 {
		fmt.Fprintln(os.Stderr, "usage: smemories-media-backfill [--dry-run] [--batch 1..10000] [--max-consecutive-failures N>=1]")
		os.Exit(2)
	}
	cfg, err := config.LoadMigrate(os.Getenv)
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
	defer func() { _ = d.Close() }()
	svc := media.NewService(media.NewStore(d), storage.NewS3(storage.S3Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PathStyle: cfg.S3PathStyle}), 1,
		ratelimit.NewFactory(nil, logger, nil), nil) // Backfill never uploads, so the upload limiter (and Redis) is never touched

	st, err := svc.Backfill(ctx, *batch, *maxFails, *dryRun, logger)
	verb := "created"
	if *dryRun {
		verb = "would create"
	}
	fmt.Printf("print objects %s: %d, skipped: %d, failed: %d\n", verb, st.Created, st.Skipped, st.Failed)
	if err != nil {
		fmt.Fprintln(os.Stderr, "backfill stopped:", err)
		os.Exit(1)
	}
	if st.Failed > 0 {
		os.Exit(1)
	}
}
