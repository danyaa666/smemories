// Package redistest gives integration tests a client whose keys cannot collide with other tests.
package redistest

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/config"
	"github.com/danyaa666/smemories/internal/redis"
)

// DefaultURL is the docker-compose Redis from .env.example.
const DefaultURL = "redis://127.0.0.1:6379/0"

// New returns a client on the server named by SMEM_TEST_REDIS_URL (default DefaultURL) whose key prefix is unique
// to this test (smem:test<random>:), so parallel tests never see each other's keys. Every key under the prefix is
// deleted in t.Cleanup. An unreachable server fails the test (run `make up`).
func New(t testing.TB) *redis.Client {
	t.Helper()
	url := os.Getenv("SMEM_TEST_REDIS_URL")
	if url == "" {
		url = DefaultURL
	}
	var b [6]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	c, err := redis.New(config.Config{
		Env: "test" + hex.EncodeToString(b[:]), RedisURL: url,
		RedisDialTimeout: 2 * time.Second, RedisReadTimeout: time.Second, RedisWriteTimeout: time.Second, RedisPoolSize: 10,
	})
	if err != nil {
		t.Fatalf("SMEM_TEST_REDIS_URL: %v", err)
	}
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		_ = c.Close()
		t.Fatalf("test redis unreachable (run `make up`?): %v", err)
	}
	t.Cleanup(func() {
		defer func() { _ = c.Close() }()
		rdb := c.Client()
		iter := rdb.Scan(ctx, 0, c.Key("*"), 100).Iterator()
		for iter.Next(ctx) {
			rdb.Del(ctx, iter.Val())
		}
		if err := iter.Err(); err != nil {
			t.Errorf("redis cleanup: %v", err)
		}
	})
	return c
}
