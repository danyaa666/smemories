package httpx

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

// DrainTimeout is how long the internal metrics listener waits for in-flight requests after shutdown starts.
const DrainTimeout = 10 * time.Second

// The slow-client timeouts of NewServer.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 15 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
)

// NewServer returns an http.Server with the project's slow-client timeouts.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
	}
}

// Serve serves on ln until ctx is cancelled. It then keeps accepting for delay (so /readyz can answer 503 while a
// load balancer notices and stops sending traffic), stops accepting and drains in-flight requests for up to drain.
// It returns nil on a clean shutdown.
func Serve(ctx context.Context, srv *http.Server, ln net.Listener, delay, drain time.Duration) error {
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(ln) }()

	select {
	case err := <-errc:
		return err // failed before shutdown was requested
	case <-ctx.Done():
	}
	select {
	case err := <-errc:
		return err
	case <-time.After(delay):
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
