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

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/httpx"
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

	ln, err := net.Listen("tcp", cfg.HTTPAddr)
	if err != nil {
		logger.Error("listen failed", "addr", cfg.HTTPAddr, "error", err)
		os.Exit(1)
	}
	logger.Info("listening", "addr", ln.Addr().String(), "env", cfg.Env)

	srv := httpx.NewServer(cfg.HTTPAddr, httpx.NewRouter(logger))
	if err := httpx.Serve(ctx, srv, ln, httpx.DrainTimeout); err != nil {
		logger.Error("server stopped", "error", err)
		os.Exit(1)
	}
	logger.Info("shutdown complete")
}
