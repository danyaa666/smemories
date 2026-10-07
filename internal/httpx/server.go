package httpx

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// DrainTimeout is how long Serve waits for in-flight requests after shutdown starts.
const DrainTimeout = 10 * time.Second

// NewServer returns an http.Server with the project's slow-client timeouts.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// Serve serves on ln until ctx is cancelled, then stops accepting and drains in-flight
// requests for up to drain. It returns nil on a clean shutdown.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, drain time.Duration) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err // failed before shutdown was requested
	case <-ctx.Done():
	}
	shutCtx, cancel := context.WithTimeout(context.Background(), drain)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		_ = srv.Close()
		return err
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
