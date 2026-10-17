package httpx

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"
)

func TestServeStartsAndShutsDownOnCancel(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	h, _ := newTestRouter(t, func(m *http.ServeMux) {
		m.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-release
			WriteJSON(w, 200, "done")
		})
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan error, 1)
	go func() { served <- Serve(ctx, NewServer(ln.Addr().String(), h), ln, 0, 5*time.Second) }()

	base := "http://" + ln.Addr().String()
	resp, err := http.Get(base + "/healthz")
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("healthz: %v %v", resp, err)
	}
	_ = resp.Body.Close()

	// A slow request in flight must finish even though shutdown has begun.
	type result struct {
		body string
		err  error
	}
	res := make(chan result, 1)
	go func() {
		resp, err := http.Get(base + "/slow")
		if err != nil {
			res <- result{err: err}
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		res <- result{body: string(b)}
	}()
	<-started
	cancel()
	time.Sleep(100 * time.Millisecond) // let Shutdown close the listener
	select {
	case err := <-served:
		t.Fatalf("Serve returned before the in-flight request finished: %v", err)
	default:
	}
	close(release)

	if r := <-res; r.err != nil || r.body != "\"done\"\n" {
		t.Fatalf("in-flight request was not drained: %+v", r)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve returned %v, want nil", err)
	}
	if resp, err := http.Get(base + "/healthz"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("server still accepting after shutdown")
	}
}

func TestNewServerTimeouts(t *testing.T) {
	s := NewServer(":0", http.NotFoundHandler())
	if s.ReadHeaderTimeout != 5*time.Second || s.ReadTimeout != 15*time.Second || s.WriteTimeout != 30*time.Second || s.IdleTimeout != 60*time.Second {
		t.Fatalf("timeouts: %+v", s)
	}
}

// T-022 AC2: after the shutdown signal, /readyz answers 503 for the delay while the listener still accepts, a request
// already in flight finishes with its full answer, and Serve returns nil (the process then exits 0).
func TestServeDrainsReadyzThenFinishesInFlight(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	h, _ := newTestRouter(t, Ready(fakePinger(func(context.Context) error { return nil }), nopLogger(), Draining(ctx)), func(m *http.ServeMux) {
		m.HandleFunc("GET /slow", func(w http.ResponseWriter, r *http.Request) {
			close(started)
			<-release
			WriteJSON(w, 200, "done")
		})
	})
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	served := make(chan error, 1)
	go func() {
		served <- Serve(ctx, NewServer(ln.Addr().String(), h), ln, 600*time.Millisecond, 5*time.Second)
	}()
	base := "http://" + ln.Addr().String()

	status := func() int {
		resp, err := http.Get(base + "/readyz")
		if err != nil {
			t.Fatalf("readyz: %v", err)
		}
		_ = resp.Body.Close()
		return resp.StatusCode
	}
	if got := status(); got != 200 {
		t.Fatalf("readyz before shutdown = %d, want 200", got)
	}
	res := make(chan string, 1)
	go func() {
		resp, err := http.Get(base + "/slow")
		if err != nil {
			res <- err.Error()
			return
		}
		defer func() { _ = resp.Body.Close() }()
		b, _ := io.ReadAll(resp.Body)
		res <- string(b)
	}()
	<-started
	cancel()
	time.Sleep(100 * time.Millisecond)
	if got := status(); got != 503 {
		t.Fatalf("readyz during the drain delay = %d, want 503", got)
	}
	select {
	case err := <-served:
		t.Fatalf("Serve returned during the delay: %v", err)
	default:
	}
	time.Sleep(700 * time.Millisecond) // the delay is over: the listener is closed, the slow request is still held
	if resp, err := http.Get(base + "/readyz"); err == nil {
		_ = resp.Body.Close()
		t.Fatal("listener still open after the delay")
	}
	close(release)
	if body := <-res; body != "\"done\"\n" {
		t.Fatalf("in-flight request got %q", body)
	}
	if err := <-served; err != nil {
		t.Fatalf("Serve returned %v, want nil", err)
	}
}
