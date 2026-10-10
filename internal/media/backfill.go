package media

import (
	"context"
	"log/slog"
	"strings"
)

// BackfillStats is what one Backfill run did.
type BackfillStats struct {
	Created int // print objects written (dry run: that would be written)
	Skipped int // photos deleted or filled by a parallel run meanwhile
	Failed  int // photos whose display object is missing or unreadable (logged); they stay without a print object
}

// Backfill creates the print object of every photo that has none (T-057), oldest first, batch rows at a time.
// It is idempotent: a photo with a print object is never selected, and a crash between the object and the
// row only means the same object is written again. With dryRun nothing is written. One photo at a time is
// in memory (a display object is at most 3000 px). Photos that fail are logged and left for the next run.
func (s *Service) Backfill(ctx context.Context, batch int, dryRun bool, logger *slog.Logger) (BackfillStats, error) {
	var st BackfillStats
	var after int64
	for {
		rows, err := s.store.withoutPrint(ctx, after, batch)
		if err != nil || len(rows) == 0 {
			return st, err
		}
		after = rows[len(rows)-1].RowID
		for _, m := range rows {
			if dryRun {
				st.Created++
				continue
			}
			switch ok, err := s.backfillOne(ctx, m); {
			case err != nil && ctx.Err() != nil:
				return st, ctx.Err()
			case err != nil:
				st.Failed++
				logger.Warn("media backfill: photo skipped", "media_id", m.ID, "error", err)
			case ok:
				st.Created++
			default:
				st.Skipped++
			}
		}
	}
}

// backfillOne writes the print object of m; false when the photo was deleted or filled by a parallel run meanwhile
// (an object left behind by a deleted photo is removed with its yearbook's prefix, like any orphan).
func (s *Service) backfillOne(ctx context.Context, m Media) (bool, error) {
	display, err := s.st.Get(ctx, m.ObjectKey)
	if err != nil {
		return false, err
	}
	data, err := printOf(display)
	if err != nil {
		return false, err
	}
	key := printKey(m.ObjectKey)
	if err := s.st.Put(ctx, key, contentTypeOf(m.ObjectKey), data); err != nil {
		return false, err
	}
	return s.store.setPrintKey(ctx, m.RowID, key)
}

// contentTypeOf is the content type of a display or print object, from its key's extension.
func contentTypeOf(key string) string {
	if strings.HasSuffix(key, ".png") {
		return "image/png"
	}
	return "image/jpeg"
}
