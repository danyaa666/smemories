package storage

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// rangeServer is a bucket with one object "k": HEAD gives its size, GET honours "Range: bytes=N-" and records the ranges it saw.
type rangeServer struct {
	mu     sync.Mutex
	data   []byte
	ranges []string
	cut    int // when > 0 a GET sends only this many bytes of the body it promised
}

func (s *rangeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasSuffix(r.URL.Path, "/k") {
		w.WriteHeader(http.StatusNotFound)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code></Error>`))
		}
		return
	}
	body := s.data
	if r.Method == http.MethodGet {
		rng := r.Header.Get("Range")
		s.mu.Lock()
		s.ranges = append(s.ranges, rng)
		s.mu.Unlock()
		from, _ := strconv.Atoi(strings.TrimSuffix(strings.TrimPrefix(rng, "bytes="), "-"))
		body = body[from:]
		w.WriteHeader(http.StatusPartialContent)
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	if s.cut > 0 {
		_, _ = w.Write(body[:s.cut])
		return // the handler returns short of Content-Length: the connection is cut
	}
	_, _ = w.Write(body)
}

func TestOpenStreamsWithRangedGets(t *testing.T) {
	isolateAWS(t)
	rs := &rangeServer{data: bytes.Repeat([]byte("0123456789"), 5000)}
	srv := httptest.NewServer(rs)
	t.Cleanup(srv.Close)
	s := newTestS3(t, S3Config{Endpoint: srv.URL, Credentials: fakeCreds(), MaxAttempts: 1})
	ctx := context.Background()

	if _, err := s.Open(ctx, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing key: %v, want ErrNotFound", err)
	}
	o, err := s.Open(ctx, "k")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close() })
	if end, _ := o.Seek(0, io.SeekEnd); end != int64(len(rs.data)) {
		t.Fatalf("size %d, want %d", end, len(rs.data))
	}
	if _, _ = o.Seek(0, io.SeekStart); len(rs.ranges) != 0 {
		t.Fatalf("sizing the object fetched a body: %v", rs.ranges) // HEAD requests and 304 answers must not read the store
	}
	// A range in the middle: one GET from that position, nothing before it.
	if _, err := o.Seek(12345, io.SeekStart); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, 20)
	if _, err := io.ReadFull(o, got); err != nil || !bytes.Equal(got, rs.data[12345:12365]) {
		t.Fatalf("range read: %q %v", got, err)
	}
	// Reading on continues the same GET; the rest of the object arrives and ends with EOF.
	rest, err := io.ReadAll(o)
	if err != nil || !bytes.Equal(rest, rs.data[12365:]) {
		t.Fatalf("rest: %d bytes, %v", len(rest), err)
	}
	if want := []string{"bytes=12345-"}; strings.Join(rs.ranges, ",") != strings.Join(want, ",") {
		t.Fatalf("GETs %v, want %v", rs.ranges, want)
	}
	// Seeking back starts a new GET at the new position.
	_, _ = o.Seek(49990, io.SeekStart)
	if tail, _ := io.ReadAll(o); !bytes.Equal(tail, rs.data[49990:]) || rs.ranges[len(rs.ranges)-1] != "bytes=49990-" {
		t.Fatalf("tail %q, GETs %v", tail, rs.ranges)
	}
	if _, err := o.Seek(-1, io.SeekStart); err == nil {
		t.Fatal("negative position accepted")
	}
}

func TestOpenNeverPresentsATruncatedBodyAsComplete(t *testing.T) {
	isolateAWS(t)
	rs := &rangeServer{data: bytes.Repeat([]byte("x"), 100_000), cut: 1000}
	srv := httptest.NewServer(rs)
	t.Cleanup(srv.Close)
	s := newTestS3(t, S3Config{Endpoint: srv.URL, Credentials: fakeCreds(), MaxAttempts: 1})
	o, err := s.Open(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = o.Close() })
	if _, err := io.ReadAll(o); err == nil {
		t.Fatal("a body cut short ended with a clean EOF")
	}
}

func TestMemoryOpen(t *testing.T) {
	m := NewMemory()
	_ = m.Put(context.Background(), "k", "x", []byte("abcdef"))
	o, err := m.Open(context.Background(), "k")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = o.Seek(2, io.SeekStart)
	if b, _ := io.ReadAll(o); string(b) != "cdef" || o.Close() != nil {
		t.Fatalf("got %q", b)
	}
	if _, err := m.Open(context.Background(), "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
}
