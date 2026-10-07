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
	go func() { served <- Serve(ctx, NewServer(ln.Addr().String(), h), ln, 5*time.Second) }()

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
