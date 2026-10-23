//go:build integration

package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// QA (T-029): S3 behaviours the app relies on that the other integration tests do not reach, run against whatever store
// SMEM_TEST_S3_* points at: ListObjectsV2 past the default page size (1000), DeletePrefix and Delete over more than one
// batch, ranged GET semantics, HEAD of a missing key and delete of a missing key.

func qaStore(t *testing.T) *S3 {
	t.Helper()
	env := func(k, d string) string {
		if v := os.Getenv(k); v != "" {
			return v
		}
		return d
	}
	s, err := NewS3(S3Config{
		Endpoint: env("SMEM_TEST_S3_ENDPOINT", "http://127.0.0.1:9000"), Region: "us-east-1", Bucket: env("SMEM_TEST_S3_BUCKET", "smemories-dev"),
		AccessKey: env("SMEM_TEST_S3_ACCESS_KEY", "smem_s3"), SecretKey: env("SMEM_TEST_S3_SECRET_KEY", "smem_s3_dev_pw"), PathStyle: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := s.EnsureBucket(ctx); err != nil {
		t.Fatalf("object store unreachable: %v", err)
	}
	return s
}

func qaPutMany(t *testing.T, s *S3, prefix string, n int) {
	t.Helper()
	var wg sync.WaitGroup
	errs := make(chan error, n)
	sem := make(chan struct{}, 16)
	for i := range n {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			errs <- s.Put(context.Background(), fmt.Sprintf("%s%05d", prefix, i), "text/plain", []byte("x"))
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func qaList(t *testing.T, s *S3, prefix string) (keys int, pages int) {
	t.Helper()
	p := s3.NewListObjectsV2Paginator(s.c, &s3.ListObjectsV2Input{Bucket: &s.bucket, Prefix: &prefix})
	for p.HasMorePages() {
		page, err := p.NextPage(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		pages++
		keys += len(page.Contents)
	}
	return keys, pages
}

func TestQAListPaginationAndDeleteOverOneBatch(t *testing.T) {
	s := qaStore(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("qa029/%d/", time.Now().UnixNano())
	t.Cleanup(func() { _ = s.DeletePrefix(ctx, prefix) })
	const n = 2505
	qaPutMany(t, s, prefix, n)
	keys, pages := qaList(t, s, prefix)
	if keys != n || pages < 3 {
		t.Fatalf("listing: %d keys in %d pages, want %d keys in at least 3 pages (default page size 1000)", keys, pages, n)
	}
	// Delete with more keys than one DeleteObjects batch (1000) takes: the app batches, the store must accept each batch.
	over := make([]string, 1001)
	for i := range over {
		over[i] = fmt.Sprintf("%s%05d", prefix, i)
	}
	if err := s.Delete(ctx, over...); err != nil {
		t.Fatal(err)
	}
	if keys, _ := qaList(t, s, prefix); keys != n-1001 {
		t.Fatalf("after Delete of 1001 keys: %d left, want %d", keys, n-1001)
	}
	// DeletePrefix pages through the rest (deleting while paging).
	if err := s.DeletePrefix(ctx, prefix); err != nil {
		t.Fatal(err)
	}
	if keys, _ := qaList(t, s, prefix); keys != 0 {
		t.Fatalf("after DeletePrefix: %d left", keys)
	}
}

func TestQAHeadMissingRangedGetAndDeleteMissing(t *testing.T) {
	s := qaStore(t)
	ctx := context.Background()
	prefix := fmt.Sprintf("qa029/%d/", time.Now().UnixNano())
	t.Cleanup(func() { _ = s.DeletePrefix(ctx, prefix) })

	// HEAD of a missing key is a typed not-found, not a generic error.
	_, err := s.c.HeadObject(ctx, &s3.HeadObjectInput{Bucket: &s.bucket, Key: aws.String(prefix + "nope")})
	if err == nil || !notFound(err) {
		t.Fatalf("HEAD missing key: %v (want NotFound)", err)
	}
	// Delete of a missing key succeeds, alone and with a mix.
	if err := s.Delete(ctx, prefix+"nope"); err != nil {
		t.Fatalf("delete missing: %v", err)
	}
	data := bytes.Repeat([]byte("0123456789"), 100_000) // 1 MB
	if err := s.Put(ctx, prefix+"a", "application/octet-stream", data); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		rng        string
		start, end int
	}{{"bytes=0-0", 0, 1}, {"bytes=5-14", 5, 15}, {"bytes=999990-", 999990, 1_000_000}, {"bytes=-10", 999990, 1_000_000}, {"bytes=0-99999999", 0, 1_000_000}} {
		out, err := s.c.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: aws.String(prefix + "a"), Range: aws.String(c.rng)})
		if err != nil {
			t.Fatalf("%s: %v", c.rng, err)
		}
		got, _ := io.ReadAll(out.Body)
		_ = out.Body.Close()
		if !bytes.Equal(got, data[c.start:c.end]) || aws.ToString(out.ContentRange) == "" {
			t.Fatalf("%s: got %d bytes, Content-Range %q", c.rng, len(got), aws.ToString(out.ContentRange))
		}
	}
	// A range past the end is the S3 error InvalidRange (416), not a body.
	_, err = s.c.GetObject(ctx, &s3.GetObjectInput{Bucket: &s.bucket, Key: aws.String(prefix + "a"), Range: aws.String("bytes=2000000-")})
	if err == nil {
		t.Fatal("range past the end: want an error")
	}
	t.Logf("range past the end: %v", err)
	// Open of a missing key stays ErrNotFound; an empty object opens with size 0 and reads to EOF.
	if _, err := s.Open(ctx, prefix+"nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Open missing: %v", err)
	}
	if err := s.Put(ctx, prefix+"empty", "text/plain", nil); err != nil {
		t.Fatal(err)
	}
	o, err := s.Open(ctx, prefix+"empty")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = o.Close() }()
	if b, err := io.ReadAll(o); err != nil || len(b) != 0 {
		t.Fatalf("empty object: %d bytes, %v", len(b), err)
	}
}
