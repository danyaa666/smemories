// Package media is photo upload, storage and retrieval (T-009). Photos come from untrusted users and
// carry personal data, so every upload is sniffed, decoded, stripped of metadata and re-encoded, and
// every read is authorised against the yearbook owner.
package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/storage"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	uploadsPerWindow = 60
	uploadWindow     = 10 * time.Minute
)

// StorageError wraps a failure of the object store (502 storage_error).
type StorageError struct{ Err error }

func (e StorageError) Error() string { return "media: storage: " + e.Err.Error() }
func (e StorageError) Unwrap() error { return e.Err }

// RateLimitedError carries how long until the caller may upload again.
type RateLimitedError struct{ RetryAfter time.Duration }

func (RateLimitedError) Error() string { return "media: upload rate limit reached" }

// Service orchestrates the store, the object storage and the image pipeline.
type Service struct {
	store   *Store
	st      storage.Storage
	limiter *ratelimit.Limiter
	sem     chan struct{} // bounds concurrent image processing (memory)
	now     func() time.Time
}

// NewService builds the service; maxConcurrent bounds parallel image decodes; now may be nil.
func NewService(store *Store, st storage.Storage, maxConcurrent int, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, st: st, limiter: ratelimit.New(uploadsPerWindow, uploadWindow, now),
		sem: make(chan struct{}, maxConcurrent), now: now}
}

func yearbookPrefix(yearbookID string) string { return "yearbooks/" + yearbookID + "/" }

// Upload validates and stores one photo for a yearbook the user owns.
func (s *Service) Upload(ctx context.Context, ownerID uint64, yearbookID string, data []byte) (Media, error) {
	yb, err := s.store.ownedYearbook(ctx, ownerID, yearbookID)
	if err != nil {
		return Media{}, err
	}
	if ok, retry := s.limiter.Take(fmt.Sprint(ownerID)); !ok {
		return Media{}, RateLimitedError{retry}
	}
	if err := checkQuota(ctx, s.store.db, ownerID, yb, 0); err != nil { // cheap early exit; insert re-checks under a lock
		return Media{}, err
	}

	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return Media{}, ctx.Err()
	}
	p, err := process(data)
	<-s.sem
	if err != nil {
		return Media{}, err
	}

	id := ulid.New(s.now())
	sum := sha256.Sum256(p.display)
	m := Media{ID: id, ObjectKey: yearbookPrefix(yearbookID) + id + "." + p.ext, ThumbKey: yearbookPrefix(yearbookID) + id + "-thumb.jpg",
		ContentType: p.contentType, Bytes: len(p.display), Width: p.width, Height: p.height, SHA256: hex.EncodeToString(sum[:]), CreatedAt: s.now().UTC()}

	if err := s.st.Put(ctx, m.ObjectKey, m.ContentType, p.display); err != nil {
		return Media{}, StorageError{err}
	}
	if err := s.st.Put(ctx, m.ThumbKey, "image/jpeg", p.thumb); err != nil {
		s.cleanup(m)
		return Media{}, StorageError{err}
	}
	if err := s.store.insert(ctx, ownerID, yb, m); err != nil {
		s.cleanup(m) // never leave objects without a row
		return Media{}, err
	}
	return m, nil
}

// cleanup removes the objects of a failed upload; it must outlive a cancelled request.
func (s *Service) cleanup(m Media) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.st.Delete(ctx, m.ObjectKey, m.ThumbKey) // best effort: an orphan is still removed with its yearbook
}

// Content returns the stored bytes of a photo the user owns: the thumbnail or the display version.
func (s *Service) Content(ctx context.Context, ownerID uint64, mediaID string, thumb bool) (Media, []byte, string, error) {
	m, err := s.store.owned(ctx, ownerID, mediaID)
	if err != nil {
		return Media{}, nil, "", err
	}
	key, ct := m.ObjectKey, m.ContentType
	if thumb {
		key, ct = m.ThumbKey, "image/jpeg"
	}
	b, err := s.st.Get(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return Media{}, nil, "", ErrNotFound
	}
	if err != nil {
		return Media{}, nil, "", StorageError{err}
	}
	return m, b, ct, nil
}

// Delete removes a photo's objects and row. Unknown or foreign ids succeed silently, so repeating a
// delete is harmless and the answer never reveals whether someone else's photo exists.
func (s *Service) Delete(ctx context.Context, ownerID uint64, mediaID string) error {
	m, err := s.store.owned(ctx, ownerID, mediaID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := s.st.Delete(ctx, m.ObjectKey, m.ThumbKey); err != nil {
		return StorageError{err} // row stays, so a retry finds it
	}
	return s.store.remove(ctx, mediaID)
}

// PurgeYearbook removes every stored object of a yearbook (called before its rows are deleted).
// ponytail: an upload racing the delete can leave one orphan object; a periodic sweep of keys without rows fixes that in M2.
func (s *Service) PurgeYearbook(ctx context.Context, yearbookID string) error {
	if err := s.st.DeletePrefix(ctx, yearbookPrefix(yearbookID)); err != nil {
		return StorageError{err}
	}
	return nil
}
