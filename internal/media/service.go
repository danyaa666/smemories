// Package media is photo upload, storage and retrieval (T-009). Photos come from untrusted users and
// carry personal data, so every upload is sniffed, decoded, stripped of metadata and re-encoded, and
// every read is authorised against the yearbook owner.
package media

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/danyaa666/smemories/internal/apperr"
	"github.com/danyaa666/smemories/internal/ratelimit"
	"github.com/danyaa666/smemories/internal/storage"
	"github.com/danyaa666/smemories/internal/ulid"
)

const (
	uploadsPerWindow = 60
	uploadWindow     = 10 * time.Minute
)

// defaultContributorSlotWait is how long a public submission waits for a processing slot before it gives up
// instead of queueing without bound.
const defaultContributorSlotWait = 10 * time.Second

// Service orchestrates the store, the object storage and the image pipeline.
type Service struct {
	store   *Store
	st      storage.Storage
	limiter ratelimit.Limiter
	sem     chan struct{} // bounds concurrent image processing (memory)
	now     func() time.Time

	contributorWait time.Duration
	refs            RefClearer
}

// RefClearer clears the references other tables hold to a photo, in the caller's transaction, just before the photo row
// is deleted. The yearbook store implements it (cover and profile photo): the tables have no ON DELETE SET NULL.
type RefClearer interface {
	ClearMediaRefs(ctx context.Context, tx *sql.Tx, mediaID int64) error
}

// refClearers calls each clearer in order; the first one must be the yearbook's, which takes the yearbook lock.
type refClearers []RefClearer

func (c refClearers) ClearMediaRefs(ctx context.Context, tx *sql.Tx, mediaID int64) error {
	for _, r := range c {
		if err := r.ClearMediaRefs(ctx, tx, mediaID); err != nil {
			return err
		}
	}
	return nil
}

// SetRefClearer registers who must clear references to a photo before Delete removes it (wired once at start-up). The yearbook
// store goes first: it locks the photo's yearbook row, and the others (notes) run under that lock.
func (s *Service) SetRefClearer(r ...RefClearer) { s.refs = refClearers(r) }

// NewService builds the service; maxConcurrent bounds parallel image decodes; now may be nil.
// The upload limiter fails open when Redis is down (an upload is not a guess).
func NewService(store *Store, st storage.Storage, maxConcurrent int, limiters *ratelimit.Factory, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, st: st, limiter: limiters.Open("media_upload", uploadsPerWindow, uploadWindow),
		sem: make(chan struct{}, maxConcurrent), now: now, contributorWait: defaultContributorSlotWait}
}

// SetContributorWait changes how long a contributor photo waits for a processing slot (default 10 s); for tests.
func (s *Service) SetContributorWait(d time.Duration) { s.contributorWait = d }

// Slots is how many images can be processed at once.
func (s *Service) Slots() int { return cap(s.sem) }

// SlotsInUse is how many images are being processed now (for metrics).
func (s *Service) SlotsInUse() int { return len(s.sem) }

func yearbookPrefix(yearbookID string) string { return "yearbooks/" + yearbookID + "/" }

// Upload validates and stores one photo for a yearbook the user owns.
func (s *Service) Upload(ctx context.Context, ownerID uint64, yearbookID string, data []byte) (Media, error) {
	yb, err := s.store.ownedYearbook(ctx, ownerID, yearbookID)
	if err != nil {
		return Media{}, err
	}
	if ok, retry, _ := s.limiter.Take(ctx, strconv.FormatUint(ownerID, 10)); !ok {
		return Media{}, tooManyUploads(retry)
	}
	return s.save(ctx, ownerID, yb, yearbookID, KindOwner, func() ([]byte, error) { return data, nil }, 0)
}

// UploadContributor stores a photo sent through a collection link (T-034) in the yearbook with internal
// id yearbookRow. It counts toward the owner's quotas like any photo, takes a processing slot like
// Upload but waits a bounded time (10 s) for it (ErrBusy), and has no per-user rate limit: the caller
// limits the public endpoint. On success the caller owns the photo: call Discard if the submission fails later.
func (s *Service) UploadContributor(ctx context.Context, yearbookRow uint64, data []byte) (Media, error) {
	return s.UploadContributorFrom(ctx, yearbookRow, func() ([]byte, error) { return data, nil })
}

// UploadContributorFrom is UploadContributor for a photo that waits on disk: load runs only once a processing slot
// is held, so photos queued for a slot cost no memory.
func (s *Service) UploadContributorFrom(ctx context.Context, yearbookRow uint64, load func() ([]byte, error)) (Media, error) {
	ownerID, publicID, err := s.store.yearbookByRow(ctx, yearbookRow)
	if err != nil {
		return Media{}, err
	}
	return s.save(ctx, ownerID, yearbookRow, publicID, KindContributor, load, s.contributorWait)
}

// save is the shared pipeline: quota pre-check, processing slot, load, process, put objects, insert the row.
// slotWait 0 waits for a slot until ctx ends.
func (s *Service) save(ctx context.Context, ownerID, yb uint64, yearbookID, kind string, load func() ([]byte, error), slotWait time.Duration) (Media, error) {
	if err := checkQuota(ctx, s.store.db, ownerID, yb, 0); err != nil { // cheap early exit; insert re-checks under a lock
		return Media{}, err
	}
	var timeout <-chan time.Time
	if slotWait > 0 {
		t := time.NewTimer(slotWait)
		defer t.Stop()
		timeout = t.C
	}
	select {
	case s.sem <- struct{}{}:
	case <-timeout:
		return Media{}, ErrBusy
	case <-ctx.Done():
		return Media{}, ctx.Err()
	}
	data, err := load()
	var p processed
	if err == nil {
		p, err = process(data)
	}
	<-s.sem
	if err != nil {
		return Media{}, err
	}

	id := ulid.New(s.now())
	sum := sha256.Sum256(p.display)
	m := Media{ID: id, ObjectKey: yearbookPrefix(yearbookID) + id + "." + p.ext, ThumbKey: yearbookPrefix(yearbookID) + id + "-thumb.jpg",
		PrintKey: printKey(yearbookPrefix(yearbookID) + id + "." + p.ext), ContentType: p.contentType, Bytes: len(p.display), Width: p.width, Height: p.height, SHA256: hex.EncodeToString(sum[:]), CreatedAt: s.now().UnixMilli()}

	if err := s.st.Put(ctx, m.ObjectKey, m.ContentType, p.display); err != nil {
		return Media{}, storageFailed(err)
	}
	if err := s.st.Put(ctx, m.ThumbKey, "image/jpeg", p.thumb); err != nil {
		s.cleanup(m)
		return Media{}, storageFailed(err)
	}
	if err := s.st.Put(ctx, m.PrintKey, m.ContentType, p.print); err != nil {
		s.cleanup(m)
		return Media{}, storageFailed(err)
	}
	saved, err := s.store.insert(ctx, ownerID, yb, kind, m)
	if err != nil {
		s.cleanup(m) // never leave objects without a row
		return Media{}, err
	}
	return saved, nil
}

// Discard removes photos (objects and rows) that were stored for a submission that then failed. It uses
// its own context, so it also runs when the client has gone away.
func (s *Service) Discard(ms ...Media) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var errs []error
	for _, m := range ms {
		// The row goes even when the objects cannot be deleted: a leftover object is removed with the yearbook's prefix.
		errs = append(errs, s.st.Delete(ctx, m.keys()...), s.store.removeRow(ctx, m.RowID, s.refs))
	}
	return errors.Join(errs...)
}

// cleanup removes the objects of a failed upload; it must outlive a cancelled request.
func (s *Service) cleanup(m Media) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.st.Delete(ctx, m.keys()...) // best effort: an orphan is still removed with its yearbook
}

// List limits and the uploader filter values.
const (
	DefaultListLimit = 50
	MaxListLimit     = 100
)

// List returns one page of a yearbook's photos, newest first, for its owner, and the position to continue from
// (0 on the last page). uploader is "owner" (also the default for ""), "contributor" or "all". before is the position
// the previous page returned (0 = first page); it only narrows a query that is already scoped to the owner's yearbook.
func (s *Service) List(ctx context.Context, ownerID uint64, yearbookID, uploader string, limit int, before uint64) ([]Media, uint64, error) {
	var kinds []string
	switch uploader {
	case "", KindOwner:
		kinds = []string{KindOwner}
	case KindContributor:
		kinds = []string{KindContributor}
	case "all":
		kinds = []string{KindOwner, KindContributor}
	default:
		return nil, 0, apperr.New(apperr.Param, "uploader must be owner, contributor or all")
	}
	yb, err := s.store.ownedYearbook(ctx, ownerID, yearbookID)
	if err != nil {
		return nil, 0, err
	}
	items, err := s.store.list(ctx, yb, kinds, before, limit+1) // one extra row says whether another page exists
	if err != nil {
		return nil, 0, err
	}
	var next uint64
	if len(items) > limit {
		items = items[:limit]
		next = items[limit-1].seq
	}
	return items, next, nil
}

// Sizes of a stored photo.
const (
	SizeDisplay = "display"
	SizeThumb   = "thumb"
	SizePrint   = "print"
)

// printKey is the key of the print object next to the display object: <id>-print.<ext>.
func printKey(objectKey string) string {
	ext := path.Ext(objectKey)
	return strings.TrimSuffix(objectKey, ext) + "-print" + ext
}

// Content returns the stored bytes of a photo the user owns in one of the Size* versions, with the size actually
// served: a photo that has no print object yet (the backfill has not reached it) is served in display size.
func (s *Service) Content(ctx context.Context, ownerID uint64, mediaID, size string) (Media, []byte, string, string, error) {
	m, err := s.store.owned(ctx, ownerID, mediaID)
	if err != nil {
		return Media{}, nil, "", "", err
	}
	key, ct := m.ObjectKey, m.ContentType
	switch {
	case size == SizeThumb:
		key, ct = m.ThumbKey, "image/jpeg"
	case size == SizePrint && m.PrintKey != "":
		key = m.PrintKey
	default:
		size = SizeDisplay
	}
	b, err := s.st.Get(ctx, key)
	if errors.Is(err, storage.ErrNotFound) {
		return Media{}, nil, "", "", ErrNotFound
	}
	if err != nil {
		return Media{}, nil, "", "", storageFailed(err)
	}
	return m, b, ct, size, nil
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
	if err := s.st.Delete(ctx, m.keys()...); err != nil {
		return storageFailed(err) // row stays, so a retry finds it
	}
	return s.store.removeRow(ctx, m.RowID, s.refs)
}

// DeleteObjects removes the stored objects of the photos with these row ids and leaves the rows. A moderation delete runs it
// before the rows go, so an object store failure (ERROR_STORAGE_ERROR) changes nothing in the database and the caller retries;
// objects that are already gone are not an error.
func (s *Service) DeleteObjects(ctx context.Context, rowIDs []int64) error {
	ms, err := s.store.byRows(ctx, rowIDs)
	if err != nil {
		return err
	}
	var keys []string
	for _, m := range ms {
		keys = append(keys, m.keys()...)
	}
	if len(keys) == 0 {
		return nil
	}
	if err := s.st.Delete(ctx, keys...); err != nil {
		return storageFailed(err)
	}
	return nil
}

// DeleteRows deletes photo rows in the caller's transaction, which already holds the yearbook row lock: the covers, profile
// photos and note links that point at each photo are cleared first (the RefClearer path of Delete), then the rows go.
// ponytail: one clearer round trip per photo (at most 300 per moderation delete); batch it if photo deletes get hot.
func (s *Service) DeleteRows(ctx context.Context, tx *sql.Tx, rowIDs []int64) error {
	for _, id := range rowIDs {
		if s.refs != nil {
			if err := s.refs.ClearMediaRefs(ctx, tx, id); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM media_tab WHERE id = ?`, id); err != nil {
			return dbError(err, "delete photo")
		}
	}
	return nil
}

// DeleteByYearbook deletes the photo rows of a yearbook in the yearbook service's delete transaction (yearbook.ChildPurger).
// The objects were removed before, by PurgeYearbook.
func (s *Service) DeleteByYearbook(ctx context.Context, tx *sql.Tx, yearbookID uint64) error {
	return deleteByYearbook(ctx, tx, yearbookID)
}

// PurgeYearbook removes every stored object of a yearbook (called before its rows are deleted).
// ponytail: an upload racing the delete can leave one orphan object; a periodic sweep of keys without rows fixes that in M2.
func (s *Service) PurgeYearbook(ctx context.Context, yearbookID string) error {
	if err := s.st.DeletePrefix(ctx, yearbookPrefix(yearbookID)); err != nil {
		return storageFailed(err)
	}
	return nil
}
