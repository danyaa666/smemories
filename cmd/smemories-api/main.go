// Command smemories-api is the SMemories HTTP API server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/storage"
	"github.com/danyaa666/smemories/internal/yearbook"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	d, err := db.Open(ctx, cfg, logger)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer func() { _ = d.Close() }()

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		logger.Error("listen failed", "addr", cfg.HTTPAddr, "error", err)
		os.Exit(1)
	}
	logger.Info("listening", "addr", ln.Addr().String(), "env", cfg.Env)

	hasher := auth.NewHasher(auth.HashParams{MemoryKiB: cfg.ArgonMemoryKiB, Time: cfg.ArgonTime, Parallelism: cfg.ArgonParallelism}, cfg.MaxHashes, auth.HashWait)
	svc, err := auth.NewService(auth.NewStore(d), hasher, auth.Limits{
		RegisterPerHour: cfg.RegisterPerHour, LoginFailsPerPair: cfg.LoginFailsPerPair, LoginFailsPerIP: cfg.LoginFailsPerIP,
	}, nil)
	if err != nil {
		logger.Error("auth setup failed", "error", err)
		os.Exit(1)
	}
	authH := auth.NewHandler(svc, auth.HandlerConfig{
		AllowedOrigins: cfg.AllowedOrigins, SecureCookie: cfg.Env != "dev", TrustProxy: cfg.TrustProxy,
	}, logger)

	mediaSvc := media.NewService(media.NewStore(d), storage.NewS3(storage.S3Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PathStyle: cfg.S3PathStyle,
	}), cfg.MediaMaxConcurrent, nil)
	mediaH := media.NewHandler(mediaSvc, cfg.MediaMaxBytes, authH.RequireUser, cfg.AllowedOrigins, logger)
	bookH := yearbook.NewHandler(yearbook.NewStore(d), mediaSvc, authH.RequireUser, cfg.AllowedOrigins, logger, nil)

	srv := httpx.NewServer(cfg.HTTPAddr, httpx.NewRouter(logger, httpx.Ready(d, logger), authH.Routes, bookH.Routes, mediaH.Routes))
	if err := httpx.Serve(ctx, srv, ln, httpx.DrainTimeout); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}
