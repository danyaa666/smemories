// Package storage is the object store behind uploaded photos: MinIO locally, S3 in production.
// Objects are always written and read whole; callers authorise every access (the bucket is private).
package storage

import (
	"context"
	"errors"
)

// ErrNotFound is returned by Get for a key that does not exist.
var ErrNotFound = errors.New("storage: object not found")

// Storage is the small slice of an S3-compatible store the app needs.
type Storage interface {
	// Put stores data under key, replacing any existing object.
	Put(ctx context.Context, key, contentType string, data []byte) error
	// Get returns the whole object, or ErrNotFound.
	// ponytail: reads the object into memory (photos are capped at a few MB); switch to a ranged
	// streaming GET if objects ever get large.
	Get(ctx context.Context, key string) ([]byte, error)
	// Delete removes the keys; a missing key is not an error.
	Delete(ctx context.Context, keys ...string) error
	// DeletePrefix removes every object whose key starts with prefix.
	DeletePrefix(ctx context.Context, prefix string) error
}
