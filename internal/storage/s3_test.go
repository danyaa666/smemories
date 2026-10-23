package storage

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
)

// isolateAWS keeps the developer's own AWS environment and files out of the credential-chain tests.
func isolateAWS(t *testing.T) {
	t.Helper()
	for _, k := range []string{"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_SESSION_TOKEN", "AWS_PROFILE", "AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "AWS_CONTAINER_CREDENTIALS_FULL_URI", "AWS_WEB_IDENTITY_TOKEN_FILE"} {
		t.Setenv(k, "")
	}
	t.Setenv("AWS_CONFIG_FILE", "/nonexistent/aws-config")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", "/nonexistent/aws-credentials")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
}

// authServer answers every request 404 NoSuchKey and records the Authorization header of the last one.
func authServer(t *testing.T) (url string, auth *atomic.Value) {
	t.Helper()
	auth = new(atomic.Value)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`<Error><Code>NoSuchKey</Code></Error>`))
	}))
	t.Cleanup(srv.Close)
	return srv.URL, auth
}

func newTestS3(t *testing.T, cfg S3Config) *S3 {
	t.Helper()
	cfg.Region, cfg.Bucket, cfg.PathStyle = "us-east-1", "b", true
	s, err := NewS3(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestCredentialSources(t *testing.T) {
	isolateAWS(t)
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDCHAINENV")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "chain-secret")
	url, auth := authServer(t)
	for _, c := range []struct {
		name string
		cfg  S3Config
		want string
	}{
		{"static keys win and still work for a local store", S3Config{AccessKey: "AKIDSTATIC", SecretKey: "s"}, "Credential=AKIDSTATIC/"},
		{"empty access key uses the default chain", S3Config{}, "Credential=AKIDCHAINENV/"},
		{"an injected provider wins over the chain", S3Config{Credentials: credentials.NewStaticCredentialsProvider("AKIDFAKE", "s", "")}, "Credential=AKIDFAKE/"},
	} {
		t.Run(c.name, func(t *testing.T) {
			c.cfg.Endpoint, c.cfg.MaxAttempts = url, 1
			s := newTestS3(t, c.cfg)
			if _, err := s.Get(context.Background(), "k"); err != ErrNotFound {
				t.Fatalf("Get: want ErrNotFound, got %v", err)
			}
			if got, _ := auth.Load().(string); !strings.Contains(got, c.want) {
				t.Fatalf("Authorization %q does not contain %q", got, c.want)
			}
		})
	}
}

func TestNoCredentialsFailsTheCallNotTheConstructor(t *testing.T) {
	isolateAWS(t)
	url, _ := authServer(t)
	s := newTestS3(t, S3Config{Endpoint: url, MaxAttempts: 1})
	if _, err := s.Get(context.Background(), "k"); err == nil || err == ErrNotFound {
		t.Fatalf("want a credentials error, got %v", err)
	}
}

// within runs fn and fails when it takes longer than limit (the AWS default would take about 25 s).
func within(t *testing.T, limit time.Duration, fn func() error) error {
	t.Helper()
	start := time.Now()
	err := fn()
	if d := time.Since(start); d > limit {
		t.Fatalf("took %v, want under %v (err %v)", d, limit, err)
	}
	return err
}

func fakeCreds() aws.CredentialsProvider {
	return credentials.NewStaticCredentialsProvider("k", "s", "")
}

func TestDialTimeoutOnBlackholeAddress(t *testing.T) {
	// 10.255.255.1 is not routable: the SYN is dropped (or the network is unreachable at once, which also passes).
	s := newTestS3(t, S3Config{Endpoint: "http://10.255.255.1:81", Credentials: fakeCreds(), DialTimeout: 200 * time.Millisecond, MaxAttempts: 1})
	if err := within(t, 3*time.Second, func() error { _, err := s.Get(context.Background(), "k"); return err }); err == nil {
		t.Fatal("want an error")
	}
}

func TestRefusedEndpointFailsFast(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	s := newTestS3(t, S3Config{Endpoint: "http://" + addr, Credentials: fakeCreds(), MaxAttempts: 2})
	if err := within(t, 5*time.Second, func() error { _, err := s.Get(context.Background(), "k"); return err }); err == nil {
		t.Fatal("want an error")
	}
}

func TestTLSHandshakeTimeout(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0") // accepts and says nothing
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			t.Cleanup(func() { _ = c.Close() })
		}
	}()
	s := newTestS3(t, S3Config{Endpoint: "https://" + ln.Addr().String(), Credentials: fakeCreds(), TLSHandshakeTimeout: 200 * time.Millisecond, MaxAttempts: 1})
	if err := within(t, 3*time.Second, func() error { _, err := s.Get(context.Background(), "k"); return err }); err == nil {
		t.Fatal("want an error")
	}
}

func TestResponseHeaderTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(5 * time.Second):
		}
	}))
	t.Cleanup(srv.Close)
	s := newTestS3(t, S3Config{Endpoint: srv.URL, Credentials: fakeCreds(), ResponseHeaderTimeout: 200 * time.Millisecond, MaxAttempts: 1})
	if err := within(t, 3*time.Second, func() error { _, err := s.Get(context.Background(), "k"); return err }); err == nil {
		t.Fatal("want an error")
	}
}

func TestOperationTimeoutCoversTheBody(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("partial"))
		w.(http.Flusher).Flush()
		select { // headers are in, the body stalls
		case <-r.Context().Done():
		case <-release:
		}
	}))
	t.Cleanup(func() { close(release); srv.Close() })
	s := newTestS3(t, S3Config{Endpoint: srv.URL, Credentials: fakeCreds(), OperationTimeout: 300 * time.Millisecond, MaxAttempts: 1})
	if err := within(t, 3*time.Second, func() error { _, err := s.Get(context.Background(), "k"); return err }); err == nil {
		t.Fatal("want an error")
	}
}

func TestRetriesAreBounded(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`<Error><Code>InternalError</Code></Error>`))
	}))
	t.Cleanup(srv.Close)
	s := newTestS3(t, S3Config{Endpoint: srv.URL, Credentials: fakeCreds(), MaxAttempts: 2})
	if err := within(t, 10*time.Second, func() error { return s.Put(context.Background(), "k", "text/plain", []byte("x")) }); err == nil {
		t.Fatal("want an error")
	}
	if n := hits.Load(); n != 2 {
		t.Fatalf("server saw %d requests, want exactly MaxAttempts=2", n)
	}
}
