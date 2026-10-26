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
	"sync/atomic"
	"syscall"
	"time"

	"github.com/danyaa666/smemories/internal/auth"
	"github.com/danyaa666/smemories/internal/bookdata"
	"github.com/danyaa666/smemories/internal/bookexport"
	"github.com/danyaa666/smemories/internal/class"
	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/db"
	"github.com/danyaa666/smemories/internal/httpx"
	"github.com/danyaa666/smemories/internal/mailer"
	"github.com/danyaa666/smemories/internal/media"
	"github.com/danyaa666/smemories/internal/medialink"
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

	mail, err := mailer.New(mailer.Config{
		Provider: cfg.MailProvider, From: cfg.MailFrom, Env: cfg.Env, LogOut: os.Stdout,
		SES: mailer.SESConfig{ConfigurationSet: cfg.SESConfigurationSet},
	}, logger)
	if err != nil {
		logger.Error("mailer setup failed", "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	var shutdownAt atomic.Int64 // Unix ns of the signal: the shutdown budget is counted from it
	context.AfterFunc(ctx, func() { shutdownAt.Store(time.Now().UnixNano()) })
	defer stop()

	d, err := db.Open(ctx, cfg, logger)
	if err != nil {
		logger.Error("database unavailable", "error", err)
		os.Exit(1)
	}
	defer func() { _ = d.Close() }()
	cctx, ccancel := context.WithTimeout(ctx, schemaCheckTimeout)
	err = db.CheckSchemaLog(cctx, d, logger) // never migrates: the one-off smemories-migrate task runs before the API rolls (docs/deploy.md)
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
		LoginFailsPerAccount: cfg.LoginFailsPerAcct,
	}, auth.Mail{Mailer: mail, Logger: logger, OnFailure: mt.MailSendFailed}, codes, limiters, nil)
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
	linkSigner, ok := medialink.New(cfg.MediaLinkSecret)
	if !ok { // config.Load already refused a short secret; this is a programming error
		logger.Error("public photo link setup failed")
		os.Exit(1)
	}
	mediaSvc.SetPublicLinks(linkSigner)
	if cfg.MediaLinkSecretRandom {
		logger.Warn("SMEM_MEDIA_LINK_SECRET is not set: using a random per-process secret, photo links stop working when the API restarts")
	}
	cursors := httpx.NewCursors(cfg.OTPKey)
	mt.Gauge("media_processing_slots_in_use", "Images being processed now.", func() float64 { return float64(mediaSvc.SlotsInUse()) })
	mt.Gauge("media_processing_slots", "Images that can be processed at once (SMEM_MEDIA_MAX_CONCURRENT).", func() float64 { return float64(mediaSvc.Slots()) })
	mediaH := media.NewHandler(mediaSvc, cfg.MediaMaxBytes, authH.RequireUser, cfg.AllowedOrigins, cursors, logger)
	mt.Gauge("media_uploads_in_progress", "Owner photo uploads holding a shared upload slot (4 x SMEM_MEDIA_MAX_CONCURRENT slots).", func() float64 { return float64(mediaH.UploadsInProgress()) })
	mt.Gauge("media_download_inflight", "Photo downloads (owner and public) holding a per-client slot; each streams from the object store and holds only a copy buffer.", func() float64 { return float64(mediaH.DownloadsInProgress()) })
	mediaH.SetPublicCounter(mt.PublicMedia)
	mediaSvc.SetPanicReporter(logger, mt.MediaProcessPanic)
	bookStore := yearbook.NewStore(d)
	notesStore := notes.NewStore(d)
	notesSvc := notes.NewService(notesStore, mediaSvc, cursors, limiters, logger, nil)
	mediaSvc.SetRefClearer(bookStore, notesSvc) // the yearbook store first: it locks the photo's book
	classSvc := class.NewService(class.NewStore(d), cursors, limiters, nil)
	classSvc.SetPublicBaseURL(cfg.PublicBaseURL)
	bookSvc := yearbook.NewService(bookStore, cursors, mediaSvc, nil, mediaSvc, notesSvc, classSvc) // classSvc clears a deleted book's class link
	bookSvc.SetLimiters(limiters)
	classSvc.SetBookSources(bookSvc, notesSvc) // the class book reads each member's own book and approved notes
	bookH := yearbook.NewHandler(bookSvc, authH.RequireUser, cfg.AllowedOrigins)
	classH := class.NewHandler(classSvc, authH.RequireUser, cfg.AllowedOrigins, authH.ClientIP)
	bookDataH := bookdata.NewHandler(bookdata.NewService(bookSvc, notesSvc), authH.RequireUser)
	bookExportH := bookexport.NewHandler(bookexport.NewService(bookSvc, notesSvc, linkSigner, cfg.PublicBaseURL, limiters, nil), authH.RequireUser)
	templateH := templateapi.NewHandler(authH.RequireUser)
	feedbackH := printfeedback.NewHandler(printfeedback.NewService(printfeedback.NewStore(d), limiters, nil), cfg.AllowedOrigins, authH.ClientIP)
	notesH := notes.NewHandler(notesSvc, cfg.MediaMaxBytes, authH.RequireUser, cfg.AllowedOrigins, authH.ClientIP, logger)
	notesH.SetUploadLimits(cfg.PublicUploadConns, cfg.PublicUploadsPerIP, cfg.UploadTmpDir)
	mt.Gauge("public_uploads_in_progress", "Public note submissions in progress.", func() float64 { return float64(notesH.UploadsInProgress()) })
	mt.Gauge("public_drains_in_progress", "Rejected public submissions whose body is being drained (a socket, no upload slot).", func() float64 { return float64(notesH.DrainsInProgress()) })
	mt.Gauge("public_uploads_max", "Public note submissions allowed at once (SMEM_PUBLIC_UPLOAD_MAX_CONNS).", func() float64 { return float64(cfg.PublicUploadConns) })
	if err := notesH.CheckSpool(); err != nil {
		logger.Error("upload spool is not writable: mount a writable volume (tmpfs) and point SMEM_UPLOAD_TMP_DIR at it", "dir", cfg.UploadTmpDir, "error", err)
		os.Exit(1)
	}
	notesH.SweepSpool() // files a crash or kill left behind; before the listener accepts traffic

	srv := httpx.NewServer(cfg.HTTPAddr, httpx.NewRouterWith(logger, httpx.RouterConfig{RequestTimeout: cfg.RequestTimeout, TrustProxy: cfg.TrustProxy, Metrics: mt}, httpx.Ready(d, logger, httpx.Draining(ctx), httpx.Dep{Name: "redis", Ping: rc.Ready}), authH.Routes, bookH.Routes, mediaH.Routes, notesH.Routes, bookDataH.Routes, bookExportH.Routes, templateH.Routes, feedbackH.Routes, classH.Routes))
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
	// Let the emails already accepted go out, but only within what is left of SMEM_SHUTDOWN_TIMEOUT counted from the signal (a hung
	// provider must not keep the process past the platform's stop timeout); whatever is left is dropped and counted (class shutdown).
	signalled := time.Now() // (the AfterFunc goroutine may not have run yet, then the budget starts now)
	if ns := shutdownAt.Load(); ns != 0 {
		signalled = time.Unix(0, ns)
	}
	dctx, dcancel := context.WithDeadline(context.Background(), signalled.Add(cfg.ShutdownTimeout))
	svc.Drain(dctx)
	dcancel()
	logger.Info("shutdown complete")
}
