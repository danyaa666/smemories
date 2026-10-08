// Command smemories-api is the SMemories HTTP API server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/notes"
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
	if cfg.MemoryLimitMiB > 0 {
		debug.SetMemoryLimit(int64(cfg.MemoryLimitMiB) << 20) // soft: the collector works harder near it, nothing is refused
		logger.Info("memory limit set", "mib", cfg.MemoryLimitMiB)
	}

	mail, err := mailer.NewLog(cfg.Env, os.Stdout) // ponytail: the only mailer; a real provider is chosen here in M2
	if err != nil {
		logger.Error("mailer setup failed", "error", err)
		os.Exit(1)
	}

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
	}, auth.Mail{Mailer: mail, BaseURL: cfg.PublicBaseURL, Logger: logger}, nil)
	if err != nil {
		logger.Error("auth setup failed", "error", err)
		os.Exit(1)
	}
	authH := auth.NewHandler(svc, auth.HandlerConfig{
		AllowedOrigins: cfg.AllowedOrigins, SecureCookie: cfg.Env != "dev", TrustProxy: cfg.TrustProxy,
	}, logger)

	if cfg.GoogleClientID != "" {
		if err := authH.EnableGoogle(auth.GoogleConfig{
			ClientID: cfg.GoogleClientID, ClientSecret: cfg.GoogleClientSecret, Issuer: cfg.GoogleIssuer,
			RedirectURL: cfg.PublicBaseURL + "/api/v1/auth/google/callback", CookieKey: cfg.OIDCCookieKey,
		}); err != nil {
			logger.Error("google sign-in setup failed", "error", err)
			os.Exit(1)
		}
	}

	go svc.RunCleanup(ctx)

	mediaSvc := media.NewService(media.NewStore(d), storage.NewS3(storage.S3Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PathStyle: cfg.S3PathStyle,
	}), cfg.MediaMaxConcurrent, nil)
	mediaH := media.NewHandler(mediaSvc, cfg.MediaMaxBytes, authH.RequireUser, cfg.AllowedOrigins, logger)
	bookH := yearbook.NewHandler(yearbook.NewStore(d), mediaSvc, authH.RequireUser, cfg.AllowedOrigins, logger, nil)
	notesH := notes.NewHandler(notes.NewStore(d), mediaSvc, cfg.MediaMaxBytes, authH.RequireUser, cfg.AllowedOrigins, authH.ClientIP, logger, nil)
	notesH.SetUploadsPerIP(cfg.PublicUploadsPerIP)

	srv := httpx.NewServer(cfg.HTTPAddr, httpx.NewRouter(logger, httpx.Ready(d, logger), authH.Routes, bookH.Routes, mediaH.Routes, notesH.Routes))
	if err := httpx.Serve(ctx, srv, ln, httpx.DrainTimeout); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
	svc.Wait() // let reset emails already accepted go out
	logger.Info("shutdown complete")
}
