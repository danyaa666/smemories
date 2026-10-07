// Package storagetest gives integration tests a client for the local MinIO (or any S3-compatible store).
package storagetest

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/storage"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// New returns an S3 storage on the bucket named by SMEM_TEST_S3_* (defaults: the dev MinIO from .env.example),
// creating the bucket when missing. An unreachable store fails the test. Keys are ULID-based, so tests
// never collide; each test removes what it wrote.
func New(t testing.TB) *storage.S3 {
	t.Helper()
	s := storage.NewS3(storage.S3Config{
		Endpoint:  env("SMEM_TEST_S3_ENDPOINT", "http://127.0.0.1:9000"),
		Region:    "us-east-1",
		Bucket:    env("SMEM_TEST_S3_BUCKET", "smemories-dev"),
		AccessKey: env("SMEM_TEST_S3_ACCESS_KEY", "smem_minio"),
		SecretKey: env("SMEM_TEST_S3_SECRET_KEY", "smem_minio_dev_pw"),
		PathStyle: true,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := s.EnsureBucket(ctx); err != nil {
		t.Fatalf("object store unreachable (run `make up`?): %v", err)
	}
	return s
}
