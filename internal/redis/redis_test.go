package redis

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/danyaa666/smemories/internal/config"
)

func testCfg(url string) config.Config {
	return config.Config{Env: "test", RedisURL: url, RedisDialTimeout: 500 * time.Millisecond, RedisReadTimeout: time.Second, RedisWriteTimeout: time.Second, RedisPoolSize: 2}
}

func TestClassify(t *testing.T) {
	replyErr := errors.New("ERR wrong number of arguments") // stands in for a server reply error
	for name, tc := range map[string]struct {
		err         error
		unavailable bool
	}{
		"nil":          {nil, false},
		"dial refused": {&net.OpError{Op: "dial", Err: errors.New("connection refused")}, true},
		"timeout":      {context.DeadlineExceeded, true},
		"eof":          {io.EOF, true},
		"pool":         {goredis.ErrPoolTimeout, true},
		"wrapped":      {fmt.Errorf("get: %w", io.ErrUnexpectedEOF), true},
		"reply error":  {replyErr, false},
		"nil key":      {goredis.Nil, false},
		"cancelled":    {context.Canceled, false},
	} {
		got := Classify(tc.err)
		if errors.Is(got, ErrUnavailable) != tc.unavailable {
			t.Errorf("%s: Classify(%v) = %v, unavailable want %v", name, tc.err, got, tc.unavailable)
		}
		if tc.err != nil && !errors.Is(got, tc.err) {
			t.Errorf("%s: the cause must stay in the chain, got %v", name, got)
		}
	}
	if Classify(nil) != nil {
		t.Error("Classify(nil) must be nil")
	}
}

func TestPingDownIsUnavailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close() // nothing listens there now
	c, err := New(testCfg("redis://" + addr + "/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if err := c.Ping(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("want ErrUnavailable, got %v", err)
	}
	if c.Addr() != addr {
		t.Fatalf("Addr = %q, want %q", c.Addr(), addr)
	}
}

func TestNewErrorHidesPassword(t *testing.T) {
	if _, err := New(testCfg("http://:s3cret@h:1/0")); err == nil || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("want an error without the password, got %v", err)
	}
}

func TestSeparatePasswordAndKeyPrefix(t *testing.T) {
	cfg := testCfg("redis://h:6379/0")
	cfg.RedisPassword = "pw"
	cfg.Env = "prod"
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if c.Client().Options().Password != "pw" {
		t.Error("SMEM_REDIS_PASSWORD should apply when the URL has none")
	}
	if got := c.Key("sess", "ab12"); got != "smem:prod:sess:ab12" {
		t.Errorf("Key = %q", got)
	}
	cfg.RedisURL = "redis://:urlpw@h:6379/0"
	c2, _ := New(cfg)
	defer func() { _ = c2.Close() }()
	if c2.Client().Options().Password != "urlpw" {
		t.Error("a password in the URL should win")
	}
}

func TestTLSMinVersion(t *testing.T) {
	c, err := New(testCfg("rediss://h:6380/0"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	if tc := c.Client().Options().TLSConfig; tc == nil || tc.MinVersion < 0x0303 {
		t.Fatalf("want TLS >= 1.2, got %+v", tc)
	}
}

// A server that rejects every command with a reply error (what a wrong password looks like) is reachable, so
// the error must not be classified as unavailability.
func TestWrongPasswordIsNotUnavailable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer func() { _ = conn.Close() }()
				buf := make([]byte, 512)
				for {
					if _, err := conn.Read(buf); err != nil {
						return
					}
					_, _ = conn.Write([]byte("-WRONGPASS invalid username-password pair\r\n"))
				}
			}()
		}
	}()
	cfg := testCfg("redis://" + ln.Addr().String() + "/0")
	cfg.RedisPassword = "wrong"
	c, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Close() }()
	err = c.Ping(context.Background())
	if err == nil || errors.Is(err, ErrUnavailable) || !strings.Contains(err.Error(), "WRONGPASS") {
		t.Fatalf("want the WRONGPASS reply error unclassified, got %v", err)
	}
}
