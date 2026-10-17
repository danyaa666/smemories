// Command smemories-api is the SMemories HTTP API server.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"runtime/debug"
	"syscall"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/bookdata"
	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/metrics"
	"github.com/danyaa666/smemories/internal/notes"
	"github.com/danyaa666/smemories/internal/printfeedback"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/redis"
	"github.com/danyaa666/smemories/internal/storage"
	"github.com/danyaa666/smemories/internal/templateapi"
	"github.com/danyaa666/smemories/internal/version"
	"github.com/danyaa666/smemories/internal/yearbook"
)

const (
	bytesPerMiB        = 1 << 20
	redisPingTimeout   = 5 * time.Second
	schemaCheckTimeout = 10 * time.Second
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "config error:", err)
		os.Exit(1)
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	logger.Info("starting", "version", version.Version, "commit", version.Commit)
	cfg.LogWarnings(logger)
	if cfg.MemoryLimitMiB > 0 {
		debug.SetMemoryLimit(int64(cfg.MemoryLimitMiB) * bytesPerMiB) // soft: the collector works harder near it, nothing is refused
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
	cctx, ccancel := context.WithTimeout(ctx, schemaCheckTimeout)
	err = db.CheckSchema(cctx, d) // never migrates: the one-off smemories-migrate task runs before the API rolls (docs/deploy.md)
	ccancel()
	if err != nil {
		logger.Error("database schema does not match this build", "error", err)
		os.Exit(1)
	}
	logger.Info("database pool", "max_open", cfg.DBMaxOpen, "max_idle", cfg.DBMaxIdle, "conn_max_lifetime", cfg.DBConnMaxLifetime.String())

	redis.UseLogger(logger) // go-redis' own lines go through slog, not raw stderr
	rc, err := redis.New(cfg)
	if err != nil {
		logger.Error("redis setup failed", "error", err)
		os.Exit(1)
	}
	defer func() { _ = rc.Close() }()
	pctx, pcancel := context.WithTimeout(ctx, redisPingTimeout)
	err = rc.Ping(pctx)
	pcancel()
	if err != nil {
		logger.Error("redis unavailable", "addr", rc.Addr(), "error", err)
		os.Exit(1)
	}
	logger.Info("redis connected", "addr", rc.Addr())

	mt := metrics.New()
	mt.DB(d, "smemories")
	mt.Redis(rc.Client())

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		logger.Error("listen failed", "addr", cfg.HTTPAddr, "error", err)
		os.Exit(1)
	}
	logger.Info("listening", "addr", ln.Addr().String(), "env", cfg.Env)

	hasher := auth.NewHasher(auth.HashParams{MemoryKiB: cfg.ArgonMemoryKiB, Time: cfg.ArgonTime, Parallelism: cfg.ArgonParallelism}, cfg.MaxHashes, auth.HashWait)
	limiters := ratelimit.NewFactory(rc, logger, nil) // the Redis clock, so every API task agrees
	limiters.OnReject(mt.RateLimited)
	codes, err := auth.NewCodes(ctx, rc, cfg.Env, cfg.OTPKey, cfg.DevFixedOTP) // DEV-SHORTCUT(otp): the last argument
	if err != nil {
		logger.Error("email code setup failed", "error", err)
		os.Exit(1)
	}
	if cfg.DevFixedOTP != "" { // DEV-SHORTCUT(otp)
		logger.Warn("dev fixed otp active") // DEV-SHORTCUT(otp)
	} // DEV-SHORTCUT(otp)
	sessions, err := auth.NewSessions(ctx, rc, logger)
	if err != nil {
		logger.Error("session store setup failed", "error", err)
		os.Exit(1)
	}
	svc, err := auth.NewService(auth.NewStore(d), sessions, hasher, auth.Limits{
		RegisterPerHour: cfg.RegisterPerHour, LoginFailsPerPair: cfg.LoginFailsPerPair, LoginFailsPerIP: cfg.LoginFailsPerIP,
	}, auth.Mail{Mailer: mail, Logger: logger}, codes, limiters, nil)
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
			RedirectURL: cfg.PublicBaseURL + "/api/auth/google-callback", CookieKey: cfg.OIDCCookieKey,
		}); err != nil {
			logger.Error("google sign-in setup failed", "error", err)
			os.Exit(1)
		}
	}

	store, err := storage.NewS3(storage.S3Config{
		Endpoint: cfg.S3Endpoint, Region: cfg.S3Region, Bucket: cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey, SecretKey: cfg.S3SecretKey, PathStyle: cfg.S3PathStyle,
	})
	if err != nil {
		logger.Error("storage setup failed", "error", err)
		os.Exit(1)
	}
	mediaSvc := media.NewService(media.NewStore(d), store, cfg.MediaMaxConcurrent, limiters, nil)
	cursors := httpx.NewCursors(cfg.OTPKey)
	mt.Gauge("media_processing_slots_in_use", "Images being processed now.", func() float64 { return float64(mediaSvc.SlotsInUse()) })
	mt.Gauge("media_processing_slots", "Images that can be processed at once (SMEM_MEDIA_MAX_CONCURRENT).", func() float64 { return float64(mediaSvc.Slots()) })
	mediaH := media.NewHandler(mediaSvc, cfg.MediaMaxBytes, authH.RequireUser, cfg.AllowedOrigins, cursors, logger)
	bookStore := yearbook.NewStore(d)
	notesStore := notes.NewStore(d)
	notesSvc := notes.NewService(notesStore, mediaSvc, cursors, limiters, logger, nil)
	mediaSvc.SetRefClearer(bookStore, notesSvc) // the yearbook store first: it locks the photo's book
	bookSvc := yearbook.NewService(bookStore, cursors, mediaSvc, nil, mediaSvc, notesSvc)
	bookH := yearbook.NewHandler(bookSvc, authH.RequireUser, cfg.AllowedOrigins)
	bookDataH := bookdata.NewHandler(bookdata.NewService(bookSvc, notesSvc), authH.RequireUser)
	templateH := templateapi.NewHandler(authH.RequireUser)
	feedbackH := printfeedback.NewHandler(printfeedback.NewService(printfeedback.NewStore(d), limiters, nil), cfg.AllowedOrigins, authH.ClientIP)
	notesH := notes.NewHandler(notesSvc, cfg.MediaMaxBytes, authH.RequireUser, cfg.AllowedOrigins, authH.ClientIP, logger)
	notesH.SetUploadLimits(cfg.PublicUploadConns, cfg.PublicUploadsPerIP, cfg.UploadTmpDir)
	mt.Gauge("public_uploads_in_progress", "Public note submissions in progress.", func() float64 { return float64(notesH.UploadsInProgress()) })
	mt.Gauge("public_uploads_max", "Public note submissions allowed at once (SMEM_PUBLIC_UPLOAD_MAX_CONNS).", func() float64 { return float64(cfg.PublicUploadConns) })
	if err := notesH.CheckSpool(); err != nil {
		logger.Error("upload spool is not writable: mount a writable volume (tmpfs) and point SMEM_UPLOAD_TMP_DIR at it", "dir", cfg.UploadTmpDir, "error", err)
		os.Exit(1)
	}
	notesH.SweepSpool() // files a crash or kill left behind; before the listener accepts traffic

	srv := httpx.NewServer(cfg.HTTPAddr, httpx.NewRouterWith(logger, httpx.RouterConfig{RequestTimeout: cfg.RequestTimeout, TrustProxy: cfg.TrustProxy, Metrics: mt}, httpx.Ready(d, logger, httpx.Draining(ctx), httpx.Dep{Name: "redis", Ping: rc.Ready}), authH.Routes, bookH.Routes, mediaH.Routes, notesH.Routes, bookDataH.Routes, templateH.Routes, feedbackH.Routes))
	if cfg.MetricsAddr != "" { // internal listener only; the public mux never serves /metrics
		mln, err := net.Listen("tcp", cfg.MetricsAddr)
		if err != nil {
			logger.Error("metrics listen failed", "addr", cfg.MetricsAddr, "error", err)
			os.Exit(1)
		}
		logger.Info("metrics listening", "addr", mln.Addr().String())
		mux := http.NewServeMux()
		mux.Handle("GET /metrics", mt.Handler())
		go func() {
			if err := httpx.Serve(ctx, httpx.NewServer(cfg.MetricsAddr, mux), mln, 0, httpx.DrainTimeout); err != nil {
				logger.Error("metrics server stopped", "error", err)
			}
		}()
	}
	if err := httpx.Serve(ctx, srv, ln, cfg.ShutdownDelay, cfg.ShutdownTimeout-cfg.ShutdownDelay); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
	svc.Wait() // let reset emails already accepted go out
	logger.Info("shutdown complete")
}
