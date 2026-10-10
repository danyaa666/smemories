package media

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"github.com/danyaa666/smemories/internal/storage"
)

// BackfillStats is what one Backfill run did.
type BackfillStats struct {
	Created int // print objects written (dry run: that would be written)
	Skipped int // photos deleted or filled by a parallel run meanwhile
	Failed  int // photos whose display object is missing or unreadable (logged); they stay without a print object
}

// ErrStorageDown stops a backfill run after too many consecutive object-store failures.
var ErrStorageDown = errors.New("media: backfill stopped, the object store keeps failing")

// storageError marks a failure of the object store itself (not of one photo): it counts towards the consecutive limit.
type storageError struct{ error }

func (e storageError) Unwrap() error { return e.error }

// Backfill creates the print object of every photo that has none (T-057), oldest first, batch rows at a time.
// It is idempotent: a photo with a print object is never selected, and a crash between the object and the
// row only means the same object is written again. With dryRun every display object is read and decoded, as in a real
// run, but nothing is written, so "would create" is exact. One photo at a time is in memory (a display object is at most
// 3000 px). Photos that fail are logged and left for the next run; maxConsecutive object-store errors in a row (reads
// other than "not found", and writes) stop the run with ErrStorageDown, so an outage does not walk every photo.
func (s *Service) Backfill(ctx context.Context, batch, maxConsecutive int, dryRun bool, logger *slog.Logger) (BackfillStats, error) {
	var st BackfillStats
	var after int64
	var storageFails int
	for {
		rows, err := s.store.withoutPrint(ctx, after, batch)
		if err != nil || len(rows) == 0 {
			return st, err
		}
		after = rows[len(rows)-1].RowID
		for _, m := range rows {
			ok, err := s.backfillOne(ctx, m, dryRun, logger)
			var se storageError
			switch {
			case err != nil && ctx.Err() != nil:
				return st, ctx.Err()
			case errors.As(err, &se):
				st.Failed++
				storageFails++
				logger.Warn("media backfill: photo skipped", "media_id", m.ID, "error", err)
				if storageFails >= maxConsecutive {
					return st, ErrStorageDown
				}
				continue
			case err != nil:
				st.Failed++
				logger.Warn("media backfill: photo skipped", "media_id", m.ID, "error", err)
			case ok:
				st.Created++
			default:
				st.Skipped++
			}
			storageFails = 0 // the store answered, so the failures were not consecutive
		}
	}
}

// backfillOne writes the print object of m; false when the photo was deleted or filled by a parallel run meanwhile.
// Failures of the object store are returned as storageError. A photo deleted between the write and the row update
// would leave the new object behind until its yearbook is deleted, so the object is removed again.
func (s *Service) backfillOne(ctx context.Context, m Media, dryRun bool, logger *slog.Logger) (bool, error) {
	display, err := s.st.Get(ctx, m.ObjectKey)
	if errors.Is(err, storage.ErrNotFound) {
		return false, err
	} else if err != nil {
		return false, storageError{err}
	}
	data, err := printOf(display)
	if err != nil || dryRun {
		return err == nil, err
	}
	key := printKey(m.ObjectKey)
	if err := s.st.Put(ctx, key, contentTypeOf(m.ObjectKey), data); err != nil {
		return false, storageError{err}
	}
	ok, err := s.store.setPrintKey(ctx, m.RowID, key)
	if err != nil || ok {
		return ok, err
	}
	// Not updated: the photo is gone (remove the object just written) or a parallel run filled it (same key, keep it).
	if exists, err := s.store.exists(ctx, m.RowID); err == nil && !exists {
		if err := s.st.Delete(ctx, key); err != nil {
			logger.Warn("media backfill: orphan print object left", "media_id", m.ID, "key", key, "error", err)
		}
	}
	return false, nil
}

// contentTypeOf is the content type of a display or print object, from its key's extension.
func contentTypeOf(key string) string {
	if strings.HasSuffix(key, ".png") {
		return "image/png"
	}
	return "image/jpeg"
}
