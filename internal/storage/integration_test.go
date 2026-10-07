//go:build integration

package storage_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/danyaa666/smemories/internal/storage"
	"github.com/danyaa666/smemories/internal/storage/storagetest"
	"github.com/danyaa666/smemories/internal/ulid"
)

func TestS3RoundTrip(t *testing.T) {
	s := storagetest.New(t)
	ctx := context.Background()
	prefix := "test/" + ulid.New(time.Now()) + "/"
	t.Cleanup(func() { _ = s.DeletePrefix(ctx, prefix) })

	if _, err := s.Get(ctx, prefix+"missing"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("missing key: got %v, want ErrNotFound", err)
	}
	want := []byte("hello \x00 bytes")
	if err := s.Put(ctx, prefix+"a.bin", "application/octet-stream", want); err != nil {
		t.Fatal(err)
	}
	if got, err := s.Get(ctx, prefix+"a.bin"); err != nil || !bytes.Equal(got, want) {
		t.Fatalf("get: %q, %v", got, err)
	}
	if err := s.Delete(ctx, prefix+"a.bin", prefix+"never-existed"); err != nil { // deleting a missing key is fine
		t.Fatal(err)
	}
	if _, err := s.Get(ctx, prefix+"a.bin"); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("after delete: got %v", err)
	}
}

func TestS3DeletePrefix(t *testing.T) {
	s := storagetest.New(t)
	ctx := context.Background()
	base := "test/" + ulid.New(time.Now()) + "/"
	t.Cleanup(func() { _ = s.DeletePrefix(ctx, base) })
	for i := range 5 {
		if err := s.Put(ctx, fmt.Sprintf("%smine/%d", base, i), "text/plain", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.Put(ctx, base+"mine-not/0", "text/plain", []byte("x")); err != nil { // shares the text prefix but not the "mine/" one
		t.Fatal(err)
	}
	if err := s.DeletePrefix(ctx, base+"mine/"); err != nil {
		t.Fatal(err)
	}
	for i := range 5 {
		if _, err := s.Get(ctx, fmt.Sprintf("%smine/%d", base, i)); !errors.Is(err, storage.ErrNotFound) {
			t.Fatalf("object %d survived", i)
		}
	}
	if _, err := s.Get(ctx, base+"mine-not/0"); err != nil {
		t.Fatalf("a sibling prefix was deleted: %v", err)
	}
	if err := s.DeletePrefix(ctx, base+"empty/"); err != nil { // nothing to delete is fine
		t.Fatal(err)
	}
}
