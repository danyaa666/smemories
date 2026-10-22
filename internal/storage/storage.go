// Package storage is the object store behind uploaded photos: MinIO locally, S3 in production.
// Objects are always written whole and read whole (Get) or streamed (Open); callers authorise every access (the bucket is private).
package storage

import (
	"context"
	"io"

	"github.com/danyaa666/smemories/internal/apperr"
)

// ErrNotFound is returned by Get for a key that does not exist.
var ErrNotFound = apperr.New(apperr.Internal, "storage: object not found")

// Storage is the small slice of an S3-compatible store the app needs.
type Storage interface {
	// Put stores data under key, replacing any existing object.
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Get returns the whole object, or ErrNotFound.
	// Get is for the image pipeline, which needs the whole object in memory anyway. Serving a download uses Open.
	Get(ctx context.Context, key string) ([]byte, error)
	// Open returns the object as a stream, or ErrNotFound. Seek(0, io.SeekEnd) is its size; a Read after a Seek asks the
	// store for just the bytes from that position on, so the object is never held in memory (T-107). The caller closes it.
	// Reads use ctx, so cancelling it ends a download in progress.
	Open(ctx context.Context, key string) (io.ReadSeekCloser, error)
	// Delete removes the keys; a missing key is not an error.
	Delete(ctx context.Context, keys ...string) error
	// DeletePrefix removes every object whose key starts with prefix.
	DeletePrefix(ctx context.Context, prefix string) error
}
